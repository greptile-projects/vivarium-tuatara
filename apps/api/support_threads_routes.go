package main

import (
	"encoding/base64"
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/supportthreads"
)

func registerSupportThreadRoutes(mux *http.ServeMux, repos *repositories.Store, store *supportthreads.Store, issueStore *issues.Store, credentials *auth.Store) {
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
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		in.RepositoryID = r.PathValue("id")
		in.AuthorID = a.UserID
		in.Status = ""
		in.History = nil
		in.Related = nil
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
