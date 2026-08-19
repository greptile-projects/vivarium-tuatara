package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"slices"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/exploratorysessions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/qualityplans"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

func registerExploratorySessionRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, sessions *exploratorysessions.Store, pulls *pullrequests.Store, releaseStore *releases.Store, issueStore *issues.Store, plans *qualityplans.Store) {
	project := func(repo, actor string, v exploratorysessions.Session) (exploratorysessions.Session, bool) {
		if !slices.Contains(v.Access, actor) {
			return exploratorysessions.Session{}, false
		}
		v.Stale, v.StaleReason = exploratorySessionStale(repo, v, pulls, releaseStore, issueStore, plans)
		return v, true
	}
	mux.HandleFunc("GET /repositories/{id}/exploratory-sessions", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		values, e := sessions.List(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "exploratory_sessions_unavailable", "exploratory sessions could not be read")
			return
		}
		out := []exploratorysessions.Session{}
		for _, v := range values {
			if x, visible := project(r.PathValue("id"), actor.UserID, v); visible {
				out = append(out, x)
			}
		}
		writeJSON(w, 200, map[string]any{"sessions": out})
	})
	mux.HandleFunc("GET /repositories/{id}/exploratory-sessions/{session_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		v, e := sessions.Get(r.PathValue("session_id"))
		if e != nil || v.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "exploratory_session_not_found", "exploratory session not found")
			return
		}
		out, visible := project(v.RepositoryID, actor.UserID, v)
		if !visible {
			writeAPIError(w, 404, "exploratory_session_not_found", "exploratory session not found")
			return
		}
		writeJSON(w, 200, out)
	})
	mux.HandleFunc("POST /repositories/{id}/exploratory-sessions", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if actor.AgentID != "" {
			writeAPIError(w, 403, "human_control_required", "a human participant must bound an exploratory session")
			return
		}
		var in exploratorysessions.Session
		if decodeJSON(r, &in) != nil || !exploratorysessions.ValidSession(in, time.Now().UTC()) {
			writeAPIError(w, 400, "invalid_exploratory_session", "an exact source, explicit audience, bounded data/budget/actions, and risk charters are required")
			return
		}
		if !slices.Contains(in.Access, actor.UserID) || !explorationParticipants(catalog, r.PathValue("id"), in) {
			writeAPIError(w, 422, "exploratory_access_invalid", "session access and human assignees must be current repository participants")
			return
		}
		encoded, _ := json.Marshal(in)
		if reusableSecret.Match(encoded) {
			writeAPIError(w, 400, "exploratory_session_sensitive", "session metadata cannot retain credentials or secret-shaped content")
			return
		}
		if !exploratorySourceResolves(git, r.PathValue("id"), in.Source, pulls, releaseStore, issueStore, plans) {
			writeAPIError(w, 422, "exploratory_source_invalid", "the source must resolve at the exact declared repository revision")
			return
		}
		out, e := sessions.Create(r.PathValue("id"), actor.UserID, in)
		writeExploratorySession(w, out, e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/exploratory-sessions/{session_id}/events", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		current, e := sessions.Get(r.PathValue("session_id"))
		if e != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "exploratory_session_not_found", "exploratory session not found")
			return
		}
		var in exploratorysessions.EventInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_exploratory_event", "a bounded timeline event is required")
			return
		}
		encoded, _ := json.Marshal(in)
		if reusableSecret.Match(encoded) {
			writeAPIError(w, 400, "exploratory_event_sensitive", "timeline metadata cannot retain credentials or secret-shaped content")
			return
		}
		actorID := actor.UserID
		if actor.AgentID != "" {
			actorID = actor.AgentID
			in.ActorType = "agent"
			in.ActorID = actor.AgentID
		} else {
			if !slices.Contains(current.Access, actor.UserID) {
				writeAPIError(w, 404, "exploratory_session_not_found", "exploratory session not found")
				return
			}
			in.ActorType = "human"
			in.ActorID = ""
		}
		out, e := sessions.Append(current.ID, actorID, in)
		if e == nil {
			out.Stale, out.StaleReason = exploratorySessionStale(current.RepositoryID, out, pulls, releaseStore, issueStore, plans)
		}
		writeExploratorySession(w, out, e, 201)
	})
}

func explorationParticipants(catalog *repositories.Store, repo string, v exploratorysessions.Session) bool {
	valid := func(id string) bool {
		r, e := catalog.GetByID(repo)
		if e != nil {
			return false
		}
		if r.OwnerID == id {
			return true
		}
		ok, e := catalog.HasCollaborator(id, repo)
		return e == nil && ok
	}
	for _, id := range v.Access {
		if !valid(id) {
			return false
		}
	}
	for _, c := range v.Charters {
		if c.AssigneeType == "human" && !valid(c.AssigneeID) {
			return false
		}
	}
	return true
}
func exploratorySourceResolves(git *storage.Store, repo string, s exploratorysessions.Source, pulls *pullrequests.Store, releasesStore *releases.Store, issuesStore *issues.Store, plans *qualityplans.Store) bool {
	r, e := git.Open(repo)
	if e != nil {
		return false
	}
	if _, e = r.ReadCommit(storage.ObjectID(s.Revision)); e != nil {
		return false
	}
	switch s.Kind {
	case "pull_preview":
		v, e := pulls.Get(repo, s.ResourceID)
		return e == nil && v.SourceCommitID == s.Revision
	case "release_candidate":
		v, e := releasesStore.Get(repo, s.ResourceID)
		return e == nil && v.CommitID == s.Revision
	case "issue":
		v, e := issuesStore.Get(repo, s.ResourceID)
		return e == nil && v.RepositoryID == repo
	case "quality_plan":
		v, e := plans.Get(s.ResourceID)
		return e == nil && v.RepositoryID == repo
	default:
		return false
	}
}
func exploratorySessionStale(repo string, v exploratorysessions.Session, pulls *pullrequests.Store, releasesStore *releases.Store, issuesStore *issues.Store, plans *qualityplans.Store) (bool, string) {
	switch v.Source.Kind {
	case "pull_preview":
		p, e := pulls.Get(repo, v.Source.ResourceID)
		if e != nil || p.SourceCommitID != v.Source.Revision {
			return true, "pull candidate moved or is unavailable"
		}
	case "release_candidate":
		x, e := releasesStore.Get(repo, v.Source.ResourceID)
		if e != nil || x.CommitID != v.Source.Revision {
			return true, "release candidate changed or is unavailable"
		}
	case "issue":
		if _, e := issuesStore.Get(repo, v.Source.ResourceID); e != nil {
			return true, "source issue is unavailable"
		}
	case "quality_plan":
		if _, e := plans.Get(v.Source.ResourceID); e != nil {
			return true, "source quality plan is unavailable"
		}
	}
	return false, ""
}
func writeExploratorySession(w http.ResponseWriter, v exploratorysessions.Session, e error, status int) {
	switch {
	case e == nil:
		writeJSON(w, status, v)
	case errors.Is(e, exploratorysessions.ErrNotFound):
		writeAPIError(w, 404, "exploratory_session_not_found", "exploratory session not found")
	case errors.Is(e, exploratorysessions.ErrConflict):
		writeAPIError(w, 409, "exploratory_session_conflict", "session changed; reload its shared timeline")
	case errors.Is(e, exploratorysessions.ErrInvalid):
		writeAPIError(w, 400, "invalid_exploratory_event", "event violates session state, scope, budget, or agent charter")
	default:
		log.Printf("exploratory session storage: %v", e)
		writeAPIError(w, 500, "exploratory_sessions_unavailable", "exploratory session could not be persisted")
	}
}
