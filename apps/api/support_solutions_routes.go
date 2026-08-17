package main

import (
	"errors"
	"net/http"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/contributorpathways"
	docscollections "github.com/greptile-projects/vivarium-tuatara/apps/api/docscollections"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/knowledgeanswers"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/packages"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/supportsolutions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/supportthreads"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/supportverifications"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
)

func registerSupportSolutionRoutes(mux *http.ServeMux, repos *repositories.Store, threads *supportthreads.Store, answers *knowledgeanswers.Store, attempts *supportverifications.Store, workspaces *workspaces.Store, solutions *supportsolutions.Store, releaseStore *releases.Store, packageStore *packages.Store, docs *docscollections.Store, pathways *contributorpathways.Store, credentials *auth.Store) {
	access := func(w http.ResponseWriter, r *http.Request) (auth.Credential, repositories.Repository, bool, bool) {
		a, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return a, repositories.Repository{}, false, false
		}
		repo, e := repos.GetByID(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return a, repo, false, false
		}
		member := repo.OwnerID == a.UserID
		if !member {
			member, _ = repos.HasCollaborator(a.UserID, repo.ID)
		}
		if repo.Visibility != "public" && !member {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return a, repo, false, false
		}
		return a, repo, member, true
	}
	visible := func(v supportsolutions.Solution, member bool) bool { return v.Audience == "public" || member }
	linksValid := func(repo string, links []supportsolutions.Link) bool {
		for _, x := range links {
			switch x.Kind {
			case "search":
			case "documentation":
				if docs == nil {
					return false
				}
				if _, e := docs.Current(repo, x.ResourceID); e != nil {
					return false
				}
			case "package":
				parts := strings.SplitN(x.ResourceID, "@", 2)
				if packageStore == nil || len(parts) != 2 {
					return false
				}
				p, e := packageStore.Get(parts[0], parts[1])
				if e != nil || p.RepositoryID != repo {
					return false
				}
			case "release":
				if releaseStore == nil {
					return false
				}
				if _, e := releaseStore.Get(repo, x.ResourceID); e != nil {
					return false
				}
			case "contributor_guidance":
				if pathways == nil {
					return false
				}
				if _, e := pathways.Current(repo); e != nil {
					return false
				}
			default:
				return false
			}
		}
		return true
	}
	mux.HandleFunc("GET /repositories/{id}/support-solutions", func(w http.ResponseWriter, r *http.Request) {
		_, _, member, ok := access(w, r)
		if !ok {
			return
		}
		all, e := solutions.List(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "support_solution_unavailable", "reusable solutions could not be read")
			return
		}
		q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
		out := []supportsolutions.Solution{}
		for _, v := range all {
			if !visible(v, member) || (v.Status != "published" && v.Status != "needs_revalidation") {
				continue
			}
			if q != "" && !strings.Contains(strings.ToLower(v.Title+" "+v.Summary+" "+v.Instructions+" "+strings.Join(v.ApplicableVersions, " ")+" "+strings.Join(v.Limitations, " ")), q) {
				continue
			}
			out = append(out, v)
		}
		writeJSON(w, 200, map[string]any{"solutions": out})
	})
	mux.HandleFunc("GET /repositories/{id}/support-solutions/{solution_id}", func(w http.ResponseWriter, r *http.Request) {
		_, _, member, ok := access(w, r)
		if !ok {
			return
		}
		v, e := solutions.Get(r.PathValue("id"), r.PathValue("solution_id"))
		if e != nil || !visible(v, member) {
			writeAPIError(w, 404, "support_solution_not_found", "reusable solution not found")
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("POST /repositories/{id}/support-threads/{thread_id}/solutions", func(w http.ResponseWriter, r *http.Request) {
		a, repo, member, ok := access(w, r)
		if !ok {
			return
		}
		thread, e := threads.Get(repo.ID, r.PathValue("thread_id"))
		if e != nil || (thread.AuthorID != a.UserID && !member) {
			writeAPIError(w, 403, "support_resolution_forbidden", "only the asker or a current repository participant may resolve this question")
			return
		}
		var in struct {
			AnswerID              string                  `json:"answer_id"`
			AnswerRevisionID      string                  `json:"answer_revision_id"`
			VerificationAttemptID string                  `json:"verification_attempt_id"`
			Title                 string                  `json:"title"`
			Summary               string                  `json:"summary"`
			Audience              string                  `json:"audience"`
			ApplicableVersions    []string                `json:"applicable_versions"`
			Limitations           []string                `json:"limitations"`
			Links                 []supportsolutions.Link `json:"links"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		answer, e := answers.Get(repo.ID, in.AnswerID)
		if e != nil {
			writeAPIError(w, 422, "support_solution_answer_invalid", "answer was not found")
			return
		}
		var revision *knowledgeanswers.Revision
		for i := range answer.Revisions {
			if answer.Revisions[i].ID == in.AnswerRevisionID {
				revision = &answer.Revisions[i]
			}
		}
		attempt, e := attempts.Get(repo.ID, thread.ID, in.VerificationAttemptID)
		if revision == nil || e != nil || attempt.AnswerID != answer.ID || attempt.AnswerRevisionID != revision.ID || attempt.Result != "passed" {
			writeAPIError(w, 422, "support_solution_evidence_invalid", "publication requires a passing attempt for the exact answer revision")
			return
		}
		stale := answer.CurrentRevisionID != revision.ID || thread.Target.Version != attempt.SoftwareVersion || environmentHash(thread.Environment) != environmentVerificationHash(attempt.Environment)
		if ws, e := workspaces.Get(attempt.WorkspaceID); e != nil || ws.CommitID != attempt.CommitID || ws.DefinitionSHA256 != attempt.DefinitionSHA256 {
			stale = true
		}
		if stale {
			writeAPIError(w, 409, "support_solution_evidence_stale", "the selected verification attempt is stale and must be rerun")
			return
		}
		if in.Audience == "public" && (repo.Visibility != "public" || thread.Audience != "public" || answer.Audience != "public") {
			writeAPIError(w, 422, "support_solution_audience_invalid", "public solutions require public repository, thread, and answer evidence")
			return
		}
		if !linksValid(repo.ID, in.Links) {
			writeAPIError(w, 422, "support_solution_link_invalid", "every publication link must resolve within this repository")
			return
		}
		credits := uniqueSupportCredits([]supportsolutions.Credit{{ActorID: thread.AuthorID, Role: "asker"}, {ActorID: revision.AuthorID, Role: "answer_author"}, {ActorID: attempt.ActorID, Role: "verifier"}, {ActorID: a.UserID, Role: "publisher"}})
		var v supportsolutions.Solution
		_, e = threads.Resolve(repo.ID, thread.ID, a.UserID, "Resolved by reusable solution from answer revision "+revision.ID, thread.Version, member, func() error {
			var createErr error
			v, createErr = solutions.Create(supportsolutions.Solution{RepositoryID: repo.ID, ThreadID: thread.ID, AnswerID: answer.ID, AnswerRevisionID: revision.ID, VerificationAttemptID: attempt.ID, Title: in.Title, Summary: in.Summary, Instructions: revision.Body, Audience: in.Audience, ApplicableVersions: in.ApplicableVersions, Limitations: in.Limitations, Links: in.Links, Credits: credits}, a.UserID)
			return createErr
		})
		if e != nil {
			if errors.Is(e, supportthreads.ErrConflict) {
				writeAPIError(w, 409, "support_thread_changed", "support thread changed; reload before publishing its resolution")
				return
			}
			writeSupportSolutionError(w, e)
			return
		}
		w.Header().Set("Location", "/repositories/"+repo.ID+"/support-solutions/"+v.ID)
		writeJSON(w, 201, v)
	})
	mux.HandleFunc("POST /repositories/{id}/support-solutions/{solution_id}/actions", func(w http.ResponseWriter, r *http.Request) {
		a, repo, member, ok := access(w, r)
		if !ok {
			return
		}
		if !member {
			writeAPIError(w, 403, "support_solution_maintainer_required", "only current repository participants may govern reusable advice")
			return
		}
		var in struct {
			Action            string   `json:"action"`
			ExpectedVersion   int      `json:"expected_version"`
			Message           string   `json:"message"`
			RelatedSolutionID string   `json:"related_solution_id"`
			Versions          []string `json:"versions"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		if in.Action == "merge_duplicate" {
			target, e := solutions.Get(repo.ID, in.RelatedSolutionID)
			source, x := solutions.Get(repo.ID, r.PathValue("solution_id"))
			if e != nil || x != nil || target.Audience != source.Audience || target.Status == "merged" || target.Status == "archived" {
				writeAPIError(w, 422, "support_solution_duplicate_invalid", "duplicate target must be an active solution with the same audience")
				return
			}
		}
		v, e := solutions.Transition(repo.ID, r.PathValue("solution_id"), a.UserID, in.Action, in.Message, in.RelatedSolutionID, in.Versions, in.ExpectedVersion)
		if e != nil {
			writeSupportSolutionError(w, e)
			return
		}
		writeJSON(w, 200, v)
	})
}

func uniqueSupportCredits(in []supportsolutions.Credit) []supportsolutions.Credit {
	out := []supportsolutions.Credit{}
	seen := map[string]bool{}
	for _, x := range in {
		if x.ActorID != "" && !seen[x.ActorID] {
			seen[x.ActorID] = true
			out = append(out, x)
		}
	}
	return out
}
func writeSupportSolutionError(w http.ResponseWriter, e error) {
	switch {
	case errors.Is(e, supportsolutions.ErrNotFound):
		writeAPIError(w, 404, "support_solution_not_found", "reusable solution not found")
	case errors.Is(e, supportsolutions.ErrConflict):
		writeAPIError(w, 409, "support_solution_changed", "reusable solution changed; reload before updating")
	case errors.Is(e, supportsolutions.ErrInvalid):
		writeAPIError(w, 422, "support_solution_invalid", "tested answer, audience, versions, reusable summary, and valid lifecycle input are required")
	default:
		writeAPIError(w, 500, "support_solution_write_failed", "reusable solution could not be saved")
	}
}
