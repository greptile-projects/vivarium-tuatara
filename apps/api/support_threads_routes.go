package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/docscollections"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/supportthreads"
)

func registerSupportThreadRoutes(mux *http.ServeMux, git *storage.Store, repos *repositories.Store, store *supportthreads.Store, issueStore *issues.Store, proposalStore *proposals.Store, documentationStore *docscollections.Store, credentials *auth.Store) {
	const supportBodyLimit = 15 << 20
	const supportAttachmentLimit = 10
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
	visible := func(v supportthreads.Thread, actor string, member bool) bool {
		return v.Audience == "public" || v.AuthorID == actor || member
	}
	project := func(v supportthreads.Thread, actor string, member bool) supportthreads.Thread {
		if v.AuthorID != actor && !member {
			v.ContactPreferences.Email = ""
		}
		if v.AuthorID != actor {
			v.Notifications = nil
		}
		return v
	}
	withRelated := func(v supportthreads.Thread, actor string, member bool) supportthreads.Thread {
		all, _ := store.List(v.RepositoryID)
		terms := strings.Fields(strings.ToLower(v.Title + " " + v.Goal + " " + v.Body))
		related := []supportthreads.Related{}
		score := func(text string) int {
			n := 0
			text = strings.ToLower(text)
			for _, term := range terms {
				if len(term) > 2 && strings.Contains(text, term) {
					n++
				}
			}
			return n
		}
		for _, x := range all {
			if x.ID == v.ID || x.Status != "answered" || !visible(x, actor, member) {
				continue
			}
			if n := score(x.Title + " " + x.Goal + " " + x.Body); n > 0 {
				related = append(related, supportthreads.Related{Kind: "support_answer", ID: x.ID, Title: x.Title, Status: x.Status, Score: n})
			}
		}
		if issueStore != nil {
			if allIssues, e := issueStore.List(v.RepositoryID); e == nil {
				for _, x := range allIssues {
					if x.Visibility != "public" && !member {
						continue
					}
					if n := score(x.Title + " " + x.ObservedBehavior); n > 0 {
						related = append(related, supportthreads.Related{Kind: "issue", ID: x.ID, Title: x.Title, Status: x.Status, Score: n})
					}
				}
			}
		}
		sort.SliceStable(related, func(i, j int) bool { return related[i].Score > related[j].Score })
		if len(related) > 5 {
			related = related[:5]
		}
		v.Related = related
		return v
	}
	mux.HandleFunc("GET /repositories/{id}/support-threads", func(w http.ResponseWriter, r *http.Request) {
		a, _, member, ok := access(w, r)
		if !ok {
			return
		}
		all, e := store.List(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "support_read_failed", "support threads could not be read")
			return
		}
		out := []supportthreads.Thread{}
		for _, v := range all {
			if visible(v, a.UserID, member) {
				out = append(out, project(v, a.UserID, member))
			}
		}
		writeJSON(w, 200, map[string]any{"threads": out})
	})
	mux.HandleFunc("POST /repositories/{id}/support-threads", func(w http.ResponseWriter, r *http.Request) {
		a, _, _, ok := access(w, r)
		if !ok {
			return
		}
		var in supportthreads.Thread
		if decodeJSONLimit(r, &in, supportBodyLimit) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		in.RepositoryID = r.PathValue("id")
		in.AuthorID = a.UserID
		in.Status = ""
		in.History = nil
		in.Related = nil
		if len(in.Attachments) > supportAttachmentLimit {
			writeAPIError(w, 422, "invalid_support_attachment", "support threads accept at most 10 attachments")
			return
		}
		for i := range in.Attachments {
			raw, e := base64.StdEncoding.DecodeString(in.Attachments[i].Data)
			if e != nil || len(raw) > 1<<20 || len(raw) == 0 {
				writeAPIError(w, 422, "invalid_support_attachment", "attachments must be valid base64, non-empty, and at most 1 MiB")
				return
			}
			in.Attachments[i].Size = len(raw)
		}
		v, e := store.Create(in)
		if e != nil {
			writeSupportError(w, e)
			return
		}
		w.Header().Set("Location", "/repositories/"+v.RepositoryID+"/support-threads/"+v.ID)
		writeJSON(w, 201, v)
	})
	mux.HandleFunc("GET /repositories/{id}/support-threads/{thread_id}", func(w http.ResponseWriter, r *http.Request) {
		a, _, member, ok := access(w, r)
		if !ok {
			return
		}
		v, e := store.Get(r.PathValue("id"), r.PathValue("thread_id"))
		if e != nil || !visible(v, a.UserID, member) {
			writeAPIError(w, 404, "support_thread_not_found", "support thread not found")
			return
		}
		v = withRelated(v, a.UserID, member)
		writeJSON(w, 200, project(v, a.UserID, member))
	})
	mux.HandleFunc("PATCH /repositories/{id}/support-threads/{thread_id}", func(w http.ResponseWriter, r *http.Request) {
		a, repo, member, ok := access(w, r)
		if !ok {
			return
		}
		var in struct {
			Status          string `json:"status"`
			Message         string `json:"message"`
			ExpectedVersion int    `json:"expected_version"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		v, e := store.Get(repo.ID, r.PathValue("thread_id"))
		if e != nil || !visible(v, a.UserID, member) {
			writeAPIError(w, 404, "support_thread_not_found", "support thread not found")
			return
		}
		v, e = store.UpdateStatus(repo.ID, v.ID, a.UserID, in.Status, in.Message, in.ExpectedVersion, member)
		if e != nil {
			writeSupportError(w, e)
			return
		}
		writeJSON(w, 200, project(v, a.UserID, member))
	})
	mux.HandleFunc("POST /repositories/{id}/support-threads/{thread_id}/replies", func(w http.ResponseWriter, r *http.Request) {
		a, repo, member, ok := access(w, r)
		if !ok {
			return
		}
		var in struct {
			Body            string `json:"body"`
			ExpectedVersion int    `json:"expected_version"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		v, e := store.Get(repo.ID, r.PathValue("thread_id"))
		if e != nil || !visible(v, a.UserID, member) {
			writeAPIError(w, 404, "support_thread_not_found", "support thread not found")
			return
		}
		v, e = store.AddReply(repo.ID, v.ID, a.UserID, in.Body, in.ExpectedVersion, member)
		if e != nil {
			writeSupportError(w, e)
			return
		}
		writeJSON(w, 201, project(v, a.UserID, member))
	})
	mux.HandleFunc("POST /repositories/{id}/support-threads/{thread_id}/escalations", func(w http.ResponseWriter, r *http.Request) {
		a, repo, member, ok := access(w, r)
		if !ok {
			return
		}
		if !member {
			writeAPIError(w, 403, "support_escalation_forbidden", "only current repository collaborators may create governed work")
			return
		}
		var in struct {
			Classification     string   `json:"classification"`
			ResourceKind       string   `json:"resource_kind"`
			AcceptanceCriteria []string `json:"acceptance_criteria"`
			DocumentationPath  string   `json:"documentation_path"`
			Tasks              []struct {
				Title            string `json:"title"`
				Outcome          string `json:"outcome"`
				Risk             string `json:"risk"`
				VerificationPlan string `json:"verification_plan"`
				AssigneeType     string `json:"assignee_type"`
				AssigneeID       string `json:"assignee_id"`
			} `json:"tasks"`
			ExpectedVersion int `json:"expected_version"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		bare, err := git.Open(repo.ID)
		if err != nil {
			writeAPIError(w, 409, "support_escalation_base_missing", "repository default branch is required before governed work can be created")
			return
		}
		base, err := bare.ReadReference("refs/heads/" + repo.DefaultBranch)
		if err != nil || base.Symbolic {
			writeAPIError(w, 409, "support_escalation_base_missing", "repository default branch is required before governed work can be created")
			return
		}
		updated, err := store.Escalate(repo.ID, r.PathValue("thread_id"), a.UserID, in.ExpectedVersion, in.Classification, in.ResourceKind, base.Target, in.AcceptanceCriteria, func(thread supportthreads.Thread, escalationID, frozenBase string) (string, string, error) {
			criteria := strings.Join(in.AcceptanceCriteria, "\n- ")
			context := fmt.Sprintf("Escalated from support thread %s.\n\nUser need: %s\n\nAffected version: %s\n\nPermitted reproduction:\n- %s\n\nAcceptance criteria:\n- %s", thread.ID, thread.Goal, thread.Target.Version, strings.Join(thread.AttemptedSteps, "\n- "), criteria)
			switch in.ResourceKind {
			case "issue":
				if issueStore == nil {
					return "", "", errors.New("issue store unavailable")
				}
				visibility := "public"
				if thread.Audience != "public" {
					visibility = "repository"
				}
				created, createErr := issueStore.CreateEscalated(issues.Issue{RepositoryID: repo.ID, ReporterID: a.UserID, Title: thread.Title, ExpectedBehavior: thread.Goal, ObservedBehavior: thread.Body, Severity: mapUrgency(thread.Urgency), Environment: supportEnvironment(thread), ReproductionSteps: supportSteps(thread), Visibility: visibility, AffectedVersion: thread.Target.Version}, escalationID)
				return created.ID, "/repositories/" + repo.ID + "/issues/" + created.ID, createErr
			case "documentation_task":
				if documentationStore == nil {
					return "", "", errors.New("documentation store unavailable")
				}
				path := strings.TrimSpace(in.DocumentationPath)
				if existing, getErr := documentationStore.GetTask(repo.ID, escalationID); getErr == nil {
					return existing.ID, "/repositories/" + repo.ID + "/documentation/tasks/" + existing.ID, nil
				}
				created, createErr := documentationStore.CreateTask(docscollections.Task{ID: escalationID, RepositoryID: repo.ID, Title: thread.Title, Path: path, Branch: "docs/support-" + thread.ID[:8], BaseRevision: frozenBase, Source: docscollections.TaskSource{Kind: "support_thread", ResourceID: thread.ID, Revision: frozenBase, Label: context}, CreatedBy: a.UserID})
				return created.ID, "/repositories/" + repo.ID + "/documentation/tasks/" + created.ID, createErr
			case "proposal", "ordered_work":
				if proposalStore == nil {
					return "", "", errors.New("proposal store unavailable")
				}
				tasks := make([]proposals.ImplementationTaskInput, 0, len(in.Tasks))
				for i, task := range in.Tasks {
					tasks = append(tasks, proposals.ImplementationTaskInput{Title: task.Title, Outcome: task.Outcome, Risk: task.Risk, VerificationPlan: task.VerificationPlan, AssigneeType: task.AssigneeType, AssigneeID: task.AssigneeID, DependsOnPrevious: i > 0})
				}
				if len(tasks) == 0 {
					tasks = append(tasks, proposals.ImplementationTaskInput{Title: "Plan " + thread.Title, Outcome: strings.Join(in.AcceptanceCriteria, "; "), VerificationPlan: strings.Join(in.AcceptanceCriteria, "\n"), AssigneeType: "human", AssigneeID: a.UserID})
				}
				origin := proposals.ReasoningOrigin{SupportThreadID: thread.ID, SupportThreadVersion: thread.Version, Revision: frozenBase, SelectedItemIDs: []string{thread.ID}, Items: []proposals.ReasoningItem{{ID: thread.ID, Kind: "support_need", Summary: thread.Goal, Status: "unresolved"}}, AnalysisStatus: in.Classification}
				created, _, createErr := proposalStore.CreateImplementation(proposals.ImplementationInput{RepositoryID: repo.ID, ActorID: a.UserID, Title: thread.Title, Body: context, Origin: origin, Tasks: tasks})
				return created.ID, "/proposals/" + repo.ID + "/" + created.ID, createErr
			}
			return "", "", supportthreads.ErrInvalid
		})
		if err != nil {
			writeSupportError(w, err)
			return
		}
		writeJSON(w, 201, project(updated, a.UserID, member))
	})
}

func mapUrgency(value string) string {
	if value == "urgent" || value == "high" {
		return "high"
	}
	if value == "low" {
		return "low"
	}
	return "medium"
}
func supportSteps(v supportthreads.Thread) []string {
	if len(v.AttemptedSteps) > 0 {
		return append([]string(nil), v.AttemptedSteps...)
	}
	return []string{"Follow the support thread's permitted reproduction."}
}
func supportEnvironment(v supportthreads.Thread) string {
	value := strings.Trim(strings.Join([]string{v.Environment.OperatingSystem, v.Environment.Runtime, v.Environment.Deployment, v.Environment.Details}, "; "), "; ")
	if value == "" {
		return "See the support thread; environment details remain explicitly missing."
	}
	return value
}

func writeSupportError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, supportthreads.ErrInvalid):
		writeAPIError(w, 422, "invalid_support_thread", "support question requires a title, body, supported target, urgency, audience, contact route, and permitted attachments")
	case errors.Is(err, supportthreads.ErrConflict):
		writeAPIError(w, 409, "support_thread_changed", "support thread changed; reload before updating")
	case errors.Is(err, supportthreads.ErrForbidden):
		writeAPIError(w, 403, "support_transition_forbidden", "only maintainers may mark a question as needing context or answered")
	default:
		writeAPIError(w, 500, "support_write_failed", "support thread could not be saved")
	}
}
