package main

import (
	"encoding/base64"
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/activities"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

func registerIssueRoutes(mux *http.ServeMux, repos *repositories.Store, store *issues.Store, releaseStore *releases.Store, credentials *auth.Store, activity *activities.Store) {
	require := func(w http.ResponseWriter, r *http.Request, scope string) (auth.Credential, bool) {
		actor, ok := authenticateRequest(w, r, credentials, scope, false)
		if !ok {
			return actor, false
		}
		repo, err := repos.GetByID(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return actor, false
		}
		collaborator, err := repos.HasCollaborator(actor.UserID, repo.ID)
		if repo.OwnerID != actor.UserID && (err != nil || !collaborator) {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return actor, false
		}
		return actor, true
	}
	visible := func(actor string, v issues.Issue) bool {
		if v.Visibility == "public" {
			return true
		}
		repo, err := repos.GetByID(v.RepositoryID)
		if err != nil {
			return false
		}
		if repo.OwnerID == actor {
			return true
		}
		ok, _ := repos.HasCollaborator(actor, v.RepositoryID)
		return ok
	}
	mux.HandleFunc("GET /repositories/{id}/issue-templates", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := require(w, r, "repositories:read"); !ok {
			return
		}
		writeJSON(w, 200, map[string]any{"templates": []map[string]any{
			{"id": "bug", "name": "Unexpected behavior", "description": "A reproducible product or code failure.", "fields": []string{"expected_behavior", "observed_behavior", "severity", "environment", "reproduction_steps"}},
			{"id": "regression", "name": "Released regression", "description": "Behavior that changed in a released version.", "fields": []string{"affected_version", "expected_behavior", "observed_behavior", "severity", "environment", "reproduction_steps"}},
		}})
	})
	mux.HandleFunc("GET /repositories/{id}/issue-suggestions", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := require(w, r, "repositories:read")
		if !ok {
			return
		}
		query := strings.TrimSpace(r.URL.Query().Get("q"))
		if len(query) < 3 {
			writeJSON(w, 200, map[string]any{"issues": []issues.Issue{}})
			return
		}
		all, err := store.List(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "issue_read_failed", "issues could not be read")
			return
		}
		terms := strings.Fields(strings.ToLower(query))
		type ranked struct {
			v issues.Issue
			n int
		}
		matches := []ranked{}
		for _, v := range all {
			if !visible(actor.UserID, v) {
				continue
			}
			text := strings.ToLower(v.Title + " " + v.ObservedBehavior)
			score := 0
			for _, term := range terms {
				if strings.Contains(text, term) {
					score++
				}
			}
			if score > 0 {
				v.Attachments = nil
				v.Discussion = nil
				matches = append(matches, ranked{v, score})
			}
		}
		sort.Slice(matches, func(i, j int) bool { return matches[i].n > matches[j].n })
		out := []issues.Issue{}
		for i := 0; i < len(matches) && i < 5; i++ {
			out = append(out, matches[i].v)
		}
		writeJSON(w, 200, map[string]any{"issues": out})
	})
	mux.HandleFunc("GET /repositories/{id}/issues", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := require(w, r, "repositories:read")
		if !ok {
			return
		}
		all, err := store.List(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "issue_read_failed", "issues could not be read")
			return
		}
		out := all[:0]
		for _, v := range all {
			if visible(actor.UserID, v) {
				out = append(out, v)
			}
		}
		writeJSON(w, 200, map[string]any{"issues": out})
	})
	mux.HandleFunc("POST /repositories/{id}/issues", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var input issues.Issue
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		input.RepositoryID, input.ReporterID = r.PathValue("id"), actor.UserID
		if input.ReleaseID != "" {
			if releaseStore == nil {
				writeAPIError(w, 503, "releases_unavailable", "releases could not be read")
				return
			}
			release, err := releaseStore.Get(input.RepositoryID, input.ReleaseID)
			if err != nil || input.AffectedVersion != "" && input.AffectedVersion != release.Version {
				writeAPIError(w, 422, "invalid_affected_release", "release_id must name the affected repository release")
				return
			}
			input.AffectedVersion = release.Version
		}
		for i := range input.Attachments {
			raw, err := base64.StdEncoding.DecodeString(input.Attachments[i].Data)
			if err != nil || len(raw) > 1<<20 || input.Attachments[i].Size != 0 && input.Attachments[i].Size != len(raw) {
				writeAPIError(w, 422, "invalid_issue_attachment", "attachment must be valid base64 and at most 1 MiB")
				return
			}
			input.Attachments[i].Size = len(raw)
		}
		created, err := store.Create(input)
		if err != nil {
			writeIssueError(w, err)
			return
		}
		recordActivity(activity, repos, activities.Event{Kind: "issue.opened", ActorID: actor.UserID, RepositoryID: created.RepositoryID, ResourceType: "issue", ResourceID: created.ID, ResourceTitle: created.Title})
		w.Header().Set("Location", "/repositories/"+created.RepositoryID+"/issues/"+created.ID)
		writeJSON(w, 201, created)
	})
	mux.HandleFunc("GET /repositories/{id}/issues/{issue_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := require(w, r, "repositories:read")
		if !ok {
			return
		}
		v, err := store.Get(r.PathValue("id"), r.PathValue("issue_id"))
		if err != nil || !visible(actor.UserID, v) {
			writeAPIError(w, 404, "issue_not_found", "issue not found")
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("POST /repositories/{id}/issues/{issue_id}/comments", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var input struct {
			Body string `json:"body"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		v, err := store.AddComment(r.PathValue("id"), r.PathValue("issue_id"), actor.UserID, input.Body)
		if err != nil {
			writeIssueError(w, err)
			return
		}
		writeJSON(w, 201, v)
	})
	mux.HandleFunc("PATCH /repositories/{id}/issues/{issue_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var input struct {
			Status          string `json:"status"`
			ExpectedVersion int    `json:"expected_version"`
			Message         string `json:"message"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		v, err := store.UpdateStatus(r.PathValue("id"), r.PathValue("issue_id"), actor.UserID, input.Status, input.ExpectedVersion, input.Message)
		if err != nil {
			writeIssueError(w, err)
			return
		}
		writeJSON(w, 200, v)
	})
}

func writeIssueError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, issues.ErrNotFound):
		writeAPIError(w, 404, "issue_not_found", "issue not found")
	case errors.Is(err, issues.ErrConflict):
		writeAPIError(w, 409, "issue_changed", "issue changed; reload and retry")
	case errors.Is(err, issues.ErrInvalid):
		writeAPIError(w, 422, "invalid_issue", "issue fields or attachments are invalid")
	default:
		writeAPIError(w, 500, "issue_write_failed", "issue could not be saved")
	}
}
