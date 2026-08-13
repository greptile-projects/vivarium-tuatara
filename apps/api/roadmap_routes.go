package main

import (
	"errors"
	"log"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/productopportunities"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/roadmaps"
)

type roadmapMutation struct {
	ExpectedVersion int               `json:"expected_version"`
	Revision        roadmaps.Revision `json:"revision"`
	Rationale       string            `json:"rationale"`
	Body            string            `json:"body"`
}

func registerRoadmapRoutes(mux *http.ServeMux, repos *repositories.Store, credentials *auth.Store, store *roadmaps.Store, opportunities *productopportunities.Store) {
	authorize := func(w http.ResponseWriter, r *http.Request) (auth.Credential, repositories.Repository, bool, bool) {
		actor, _, ok := authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id"))
		if !ok {
			return actor, repositories.Repository{}, false, false
		}
		repo, e := repos.GetByID(r.PathValue("id"))
		if e != nil {
			return actor, repo, false, false
		}
		// Public reads intentionally do not authenticate in the shared read helper.
		// Recover an optional presented identity so mutations remain attributable.
		_, cookieErr := r.Cookie("vivarium_session")
		if actor.UserID == "" && actor.AgentID == "" && (r.Header.Get("Authorization") != "" || cookieErr == nil) {
			var authenticated bool
			actor, authenticated, ok = authenticateOptionalRequest(w, r, credentials, "repositories:read", false)
			if !ok || !authenticated {
				return auth.Credential{}, repo, false, false
			}
		}
		participant := actor.UserID == repo.OwnerID
		if !participant {
			participant, _ = repos.HasCollaborator(actor.UserID, repo.ID)
		}
		return actor, repo, participant, true
	}
	validate := func(repo string, r roadmaps.Revision) bool {
		for _, d := range r.Decisions {
			x, e := opportunities.Get(repo, d.OpportunityID)
			found := false
			if e == nil {
				for _, revision := range x.Revisions {
					found = found || revision.Version == d.Version
				}
			}
			if !found {
				return false
			}
		}
		return true
	}
	requireIdentity := func(w http.ResponseWriter, actor auth.Credential) bool {
		if actor.UserID == "" && actor.AgentID == "" {
			writeAuthenticationRequired(w, false)
			return false
		}
		return true
	}
	mux.HandleFunc("GET /repositories/{id}/roadmap", func(w http.ResponseWriter, r *http.Request) {
		_, repo, _, ok := authorize(w, r)
		if !ok {
			return
		}
		v, e := store.Get(repo.ID)
		writeRoadmap(w, v, e, 200)
	})
	mux.HandleFunc("PUT /repositories/{id}/roadmap", func(w http.ResponseWriter, r *http.Request) {
		actor, repo, participant, ok := authorize(w, r)
		if !ok {
			return
		}
		if !participant || actor.AgentID != "" {
			writeAPIError(w, 403, "roadmap_commit_forbidden", "only repository maintainers may commit project resources")
			return
		}
		var in roadmapMutation
		if decodeJSON(r, &in) != nil || !validate(repo.ID, in.Revision) {
			writeAPIError(w, 400, "invalid_roadmap_evidence", "every opportunity must exist at the exact compared version")
			return
		}
		v, e := store.Publish(repo.ID, actor.UserID, in.ExpectedVersion, in.Revision)
		writeRoadmap(w, v, e, 200)
	})
	mux.HandleFunc("POST /repositories/{id}/roadmap/scenarios", func(w http.ResponseWriter, r *http.Request) {
		actor, repo, _, ok := authorize(w, r)
		if !ok || !requireIdentity(w, actor) {
			return
		}
		var in roadmapMutation
		if decodeJSON(r, &in) != nil || !validate(repo.ID, in.Revision) {
			writeAPIError(w, 400, "invalid_roadmap_scenario", "scenario opportunity versions must be exact")
			return
		}
		kind, id := "human", actor.UserID
		if actor.AgentID != "" {
			kind, id = "agent", actor.AgentID
		}
		v, e := store.Propose(repo.ID, id, kind, in.ExpectedVersion, in.Revision, in.Rationale)
		writeRoadmap(w, v, e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/roadmap/comments", func(w http.ResponseWriter, r *http.Request) {
		actor, repo, _, ok := authorize(w, r)
		if !ok || !requireIdentity(w, actor) {
			return
		}
		var in roadmapMutation
		if decodeJSON(r, &in) != nil {
			return
		}
		kind, id := "human", actor.UserID
		if actor.AgentID != "" {
			kind, id = "agent", actor.AgentID
		}
		v, e := store.Comment(repo.ID, id, kind, in.ExpectedVersion, in.Body)
		writeRoadmap(w, v, e, 201)
	})
}
func writeRoadmap(w http.ResponseWriter, v roadmaps.Roadmap, e error, status int) {
	switch {
	case e == nil:
		writeJSON(w, status, v)
	case errors.Is(e, roadmaps.ErrNotFound):
		writeAPIError(w, 404, "roadmap_not_found", "roadmap not found")
	case errors.Is(e, roadmaps.ErrConflict):
		writeAPIError(w, 409, "roadmap_changed", "roadmap changed; refresh and submit an attributed replan")
	case errors.Is(e, roadmaps.ErrInvalid):
		writeAPIError(w, 400, "invalid_roadmap", "complete comparisons, accountable outcomes, sequencing, and an explicit replan reason are required")
	default:
		log.Printf("roadmap storage: %v", e)
		writeAPIError(w, 500, "roadmap_unavailable", "roadmap could not be persisted")
	}
}
