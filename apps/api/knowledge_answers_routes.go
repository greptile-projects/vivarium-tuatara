package main

import (
	"errors"
	"net/http"
	"os/exec"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/knowledgeanswers"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/packages"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/supportthreads"
)

func registerKnowledgeAnswerRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, answers *knowledgeanswers.Store, support *supportthreads.Store, issueStore *issues.Store, releaseStore *releases.Store, packageStore *packages.Store) {
	access := func(w http.ResponseWriter, r *http.Request) (auth.Credential, repositories.Repository, bool, bool) {
		a, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return a, repositories.Repository{}, false, false
		}
		repo, e := catalog.GetByID(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return a, repo, false, false
		}
		member := repo.OwnerID == a.UserID
		if !member {
			member, _ = catalog.HasCollaborator(a.UserID, repo.ID)
		}
		if repo.Visibility != "public" && !member {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return a, repo, false, false
		}
		if a.AgentID != "" && a.RepositoryID != repo.ID {
			writeAPIError(w, 403, "knowledge_scope_forbidden", "agent credential is not scoped to this repository")
			return a, repo, false, false
		}
		return a, repo, member, true
	}
	visible := func(v knowledgeanswers.Answer, member bool) bool { return v.Audience == "public" || member }
	validate := func(repo repositories.Repository, member bool, audience string, claims []knowledgeanswers.Claim) bool {
		gr, e := git.Open(repo.ID)
		if e != nil {
			return false
		}
		for _, cl := range claims {
			for _, c := range cl.Citations {
				switch c.Kind {
				case "source", "symbol", "documentation":
					if !accessibilityRevisionIsVisible(git, repo.ID, c.Revision) || c.Path == "" || ((c.Kind == "source" || c.Kind == "symbol") && c.StartLine == 0) || (c.Kind == "symbol" && strings.TrimSpace(c.Symbol) == "") || exec.Command("git", "--git-dir="+gr.Path(), "cat-file", "-e", c.Revision+":"+c.Path).Run() != nil {
						return false
					}
					if c.StartLine < 0 || c.EndLine < c.StartLine {
						return false
					}
					if c.StartLine > 0 {
						body, err := exec.Command("git", "--git-dir="+gr.Path(), "show", c.Revision+":"+c.Path).Output()
						if err != nil || c.EndLine == 0 || c.EndLine > 1+strings.Count(string(body), "\n") {
							return false
						}
					}
				case "support_thread":
					v, e := support.Get(repo.ID, c.ResourceID)
					if e != nil || (v.Audience != "public" && (!member || audience == "public")) {
						return false
					}
				case "known_issue":
					v, e := issueStore.Get(repo.ID, c.ResourceID)
					if e != nil || (v.Visibility != "public" && (!member || audience == "public")) {
						return false
					}
				case "release":
					if _, e := releaseStore.Get(repo.ID, c.ResourceID); e != nil {
						return false
					}
				case "package":
					parts := strings.SplitN(c.ResourceID, "@", 2)
					if len(parts) != 2 {
						return false
					}
					v, e := packageStore.Get(parts[0], parts[1])
					if e != nil || v.RepositoryID != repo.ID || (audience == "public" && v.Visibility != "public") {
						return false
					}
				default:
					return false
				}
			}
		}
		return true
	}
	mux.HandleFunc("GET /repositories/{id}/knowledge-answers", func(w http.ResponseWriter, r *http.Request) {
		_, _, member, ok := access(w, r)
		if !ok {
			return
		}
		all, e := answers.List(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "knowledge_read_failed", "project guidance could not be read")
			return
		}
		out := []knowledgeanswers.Answer{}
		for _, v := range all {
			if visible(v, member) {
				out = append(out, v)
			}
		}
		writeJSON(w, 200, map[string]any{"answers": out})
	})
	mux.HandleFunc("GET /repositories/{id}/knowledge-answers/{answer_id}", func(w http.ResponseWriter, r *http.Request) {
		_, _, member, ok := access(w, r)
		if !ok {
			return
		}
		v, e := answers.Get(r.PathValue("id"), r.PathValue("answer_id"))
		if e != nil || !visible(v, member) {
			writeAPIError(w, 404, "knowledge_answer_not_found", "project guidance not found")
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("POST /repositories/{id}/knowledge-answers", func(w http.ResponseWriter, r *http.Request) {
		a, repo, member, ok := access(w, r)
		if !ok {
			return
		}
		if !member && a.AgentID == "" {
			writeAPIError(w, 403, "knowledge_contributor_required", "only repository participants or scoped agents may propose guidance")
			return
		}
		var in struct {
			Question string                   `json:"question"`
			Audience string                   `json:"audience"`
			Summary  string                   `json:"summary"`
			Body     string                   `json:"body"`
			Claims   []knowledgeanswers.Claim `json:"claims"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		if !validate(repo, member, in.Audience, in.Claims) {
			writeAPIError(w, 422, "inaccessible_knowledge_evidence", "every citation must resolve to exact evidence visible to the contributor")
			return
		}
		actor, typ := a.UserID, "human"
		if a.AgentID != "" {
			actor, typ = a.AgentID, "agent"
		}
		v, e := answers.Create(knowledgeanswers.Answer{RepositoryID: repo.ID, Question: in.Question, Audience: in.Audience}, knowledgeanswers.Revision{Summary: in.Summary, Body: in.Body, AuthorID: actor, AuthorType: typ, Claims: in.Claims})
		if e != nil {
			writeKnowledgeError(w, e)
			return
		}
		w.Header().Set("Location", "/repositories/"+repo.ID+"/knowledge-answers/"+v.ID)
		writeJSON(w, 201, v)
	})
	mux.HandleFunc("POST /repositories/{id}/knowledge-answers/{answer_id}/revisions", func(w http.ResponseWriter, r *http.Request) {
		a, repo, member, ok := access(w, r)
		if !ok {
			return
		}
		if !member && a.AgentID == "" {
			writeAPIError(w, 403, "knowledge_contributor_required", "only repository participants or scoped agents may revise guidance")
			return
		}
		var in struct {
			ExpectedVersion int                      `json:"expected_version"`
			Summary         string                   `json:"summary"`
			Body            string                   `json:"body"`
			Claims          []knowledgeanswers.Claim `json:"claims"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		answer, answerErr := answers.Get(repo.ID, r.PathValue("answer_id"))
		if answerErr != nil {
			writeKnowledgeError(w, answerErr)
			return
		}
		if !validate(repo, member, answer.Audience, in.Claims) {
			writeAPIError(w, 422, "inaccessible_knowledge_evidence", "every citation must resolve to exact evidence visible to the contributor")
			return
		}
		actor, typ := a.UserID, "human"
		if a.AgentID != "" {
			actor, typ = a.AgentID, "agent"
		}
		v, e := answers.Revise(repo.ID, r.PathValue("answer_id"), in.ExpectedVersion, knowledgeanswers.Revision{Summary: in.Summary, Body: in.Body, AuthorID: actor, AuthorType: typ, Claims: in.Claims})
		if e != nil {
			writeKnowledgeError(w, e)
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("POST /repositories/{id}/knowledge-answers/{answer_id}/responses", func(w http.ResponseWriter, r *http.Request) {
		a, _, member, ok := access(w, r)
		if !ok {
			return
		}
		if a.AgentID != "" {
			writeAPIError(w, 403, "knowledge_human_review_required", "agent credentials cannot perform human review actions")
			return
		}
		if !member {
			writeAPIError(w, 403, "knowledge_participant_required", "only current human participants may review guidance")
			return
		}
		var in struct {
			ExpectedVersion int    `json:"expected_version"`
			RevisionID      string `json:"revision_id"`
			Kind            string `json:"kind"`
			Body            string `json:"body"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		v, e := answers.Respond(r.PathValue("id"), r.PathValue("answer_id"), a.UserID, in.RevisionID, in.Kind, in.Body, in.ExpectedVersion)
		if e != nil {
			writeKnowledgeError(w, e)
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("PATCH /repositories/{id}/knowledge-answers/{answer_id}", func(w http.ResponseWriter, r *http.Request) {
		a, repo, _, ok := access(w, r)
		if !ok {
			return
		}
		if a.AgentID != "" {
			writeAPIError(w, 403, "knowledge_human_decision_required", "agent credentials cannot make maintainer guidance decisions")
			return
		}
		if repo.OwnerID != a.UserID {
			writeAPIError(w, 403, "knowledge_maintainer_required", "only the repository owner may verify, request context for, or retire guidance")
			return
		}
		var in struct {
			ExpectedVersion int    `json:"expected_version"`
			Status          string `json:"status"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		v, e := answers.SetStatus(repo.ID, r.PathValue("answer_id"), a.UserID, in.Status, in.ExpectedVersion)
		if e != nil {
			writeKnowledgeError(w, e)
			return
		}
		writeJSON(w, 200, v)
	})
}
func writeKnowledgeError(w http.ResponseWriter, e error) {
	switch {
	case errors.Is(e, knowledgeanswers.ErrNotFound):
		writeAPIError(w, 404, "knowledge_answer_not_found", "project guidance not found")
	case errors.Is(e, knowledgeanswers.ErrConflict):
		writeAPIError(w, 409, "knowledge_answer_changed", "project guidance changed; reload before updating")
	case errors.Is(e, knowledgeanswers.ErrInvalid):
		writeAPIError(w, 422, "invalid_knowledge_answer", "guidance requires a question, answer, cited claims, applicable versions, and explicit uncertainty for agent claims")
	default:
		writeAPIError(w, 500, "knowledge_write_failed", "project guidance could not be saved")
	}
}
