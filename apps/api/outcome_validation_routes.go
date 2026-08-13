package main

import (
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/outcomevalidations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/productopportunities"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/roadmaps"
)

type validationMutation struct {
	ExpectedVersion int                        `json:"expected_version"`
	Draft           outcomevalidations.Draft   `json:"draft"`
	ParticipantID   string                     `json:"participant_id"`
	Activity        string                     `json:"activity"`
	Revision        string                     `json:"revision"`
	ExpiresAt       time.Time                  `json:"expires_at"`
	InvitationID    string                     `json:"invitation_id"`
	Status          string                     `json:"status"`
	Finding         outcomevalidations.Finding `json:"finding"`
	Outcome         string                     `json:"outcome"`
	Reason          string                     `json:"reason"`
}

func registerOutcomeValidationRoutes(mux *http.ServeMux, repos *repositories.Store, credentials *auth.Store, store *outcomevalidations.Store, roadmapStore *roadmaps.Store, opportunities *productopportunities.Store) {
	identity := func(w http.ResponseWriter, r *http.Request) (auth.Credential, bool) {
		return authenticateRequest(w, r, credentials, "repositories:read", false)
	}
	participant := func(actor auth.Credential, repo repositories.Repository) bool {
		if actor.AgentID != "" {
			return false
		}
		if actor.UserID == repo.OwnerID {
			return true
		}
		ok, _ := repos.HasCollaborator(actor.UserID, repo.ID)
		return ok
	}
	resolve := func(repo, item string, version int) (roadmaps.Item, roadmaps.OpportunityDecision, bool) {
		rm, e := roadmapStore.Get(repo)
		if e != nil {
			return roadmaps.Item{}, roadmaps.OpportunityDecision{}, false
		}
		var rev *roadmaps.Revision
		for i := range rm.Revisions {
			if rm.Revisions[i].Version == version {
				rev = &rm.Revisions[i]
			}
		}
		if rev == nil {
			return roadmaps.Item{}, roadmaps.OpportunityDecision{}, false
		}
		var found roadmaps.Item
		ok := false
		for _, x := range rev.Items {
			if x.ID == item && x.Status != "cancelled" {
				found = x
				ok = true
			}
		}
		if !ok {
			return found, roadmaps.OpportunityDecision{}, false
		}
		for _, d := range rev.Decisions {
			if d.OpportunityID == found.OpportunityID && d.Outcome == "accepted" {
				return found, d, true
			}
		}
		return found, roadmaps.OpportunityDecision{}, false
	}
	mux.HandleFunc("GET /repositories/{id}/outcome-validations", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := identity(w, r)
		if !ok {
			return
		}
		repo, e := repos.GetByID(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return
		}
		if !participant(actor, repo) {
			writeAPIError(w, 403, "validation_forbidden", "repository participation is required to list validation plans")
			return
		}
		v, e := store.List(repo.ID)
		if e != nil {
			writeValidation(w, outcomevalidations.Validation{}, e, 500)
			return
		}
		writeJSON(w, 200, map[string]any{"validations": v})
	})
	mux.HandleFunc("POST /repositories/{id}/outcome-validations", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := identity(w, r)
		if !ok {
			return
		}
		repo, e := repos.GetByID(r.PathValue("id"))
		if e != nil || !participant(actor, repo) {
			writeAPIError(w, 403, "validation_forbidden", "only human repository participants may open validation")
			return
		}
		var in validationMutation
		if decodeJSON(r, &in) != nil {
			return
		}
		_, decision, ok := resolve(repo.ID, in.Draft.ItemID, in.Draft.RoadmapVersion)
		if !ok {
			writeAPIError(w, 400, "invalid_validation_source", "validation must freeze an accepted roadmap item and opportunity revision")
			return
		}
		op, e := opportunities.Get(repo.ID, decision.OpportunityID)
		if e != nil {
			writeAPIError(w, 400, "invalid_validation_source", "opportunity evidence is unavailable")
			return
		}
		source := map[string]bool{}
		for _, rev := range op.Revisions {
			if rev.Version == decision.Version {
				for _, s := range rev.Sources {
					source[s.ResourceID] = true
				}
			}
		}
		for _, m := range in.Draft.Measures {
			for _, id := range m.SourceIDs {
				if !source[id] {
					writeAPIError(w, 400, "invalid_validation_measure", "measures must derive from cited opportunity evidence")
					return
				}
			}
		}
		v, e := store.Create(repo.ID, actor.UserID, decision.OpportunityID, decision.Version, in.Draft)
		writeValidation(w, v, e, 201)
	})
	mux.HandleFunc("GET /repositories/{id}/outcome-validations/{validation_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := identity(w, r)
		if !ok {
			return
		}
		repo, e := repos.GetByID(r.PathValue("id"))
		if e != nil {
			return
		}
		v, e := store.Get(repo.ID, r.PathValue("validation_id"))
		if e != nil {
			writeValidation(w, v, e, 200)
			return
		}
		allowed := participant(actor, repo)
		if !allowed {
			for _, p := range v.Invitations {
				allowed = allowed || (p.ParticipantID == actor.UserID)
			}
		}
		if !allowed {
			writeAPIError(w, 403, "validation_forbidden", "only collaborators and invited participants may inspect this validation")
			return
		}
		writeJSON(w, 200, v)
	})
	mutate := func(w http.ResponseWriter, r *http.Request, action string) {
		actor, ok := identity(w, r)
		if !ok {
			return
		}
		repo, e := repos.GetByID(r.PathValue("id"))
		if e != nil {
			return
		}
		var in validationMutation
		if decodeJSON(r, &in) != nil {
			return
		}
		var v outcomevalidations.Validation
		switch action {
		case "invite":
			if !participant(actor, repo) {
				writeAPIError(w, 403, "validation_forbidden", "only collaborators may invite participants")
				return
			}
			v, e = store.Invite(repo.ID, r.PathValue("validation_id"), actor.UserID, in.ParticipantID, in.Activity, in.Revision, in.ExpiresAt, in.ExpectedVersion)
		case "consent":
			v, e = store.Consent(repo.ID, r.PathValue("validation_id"), in.InvitationID, actor.UserID, in.Status, in.ExpectedVersion)
		case "finding":
			v, e = store.Find(repo.ID, r.PathValue("validation_id"), in.InvitationID, actor.UserID, in.ExpectedVersion, in.Finding)
		case "conclusion":
			if !participant(actor, repo) {
				writeAPIError(w, 403, "validation_forbidden", "only collaborators may conclude validation")
				return
			}
			v, e = store.Conclude(repo.ID, r.PathValue("validation_id"), actor.UserID, in.Outcome, in.Reason, in.ExpectedVersion)
		}
		writeValidation(w, v, e, 200)
	}
	for path, action := range map[string]string{"/invitations": "invite", "/consent": "consent", "/findings": "finding", "/conclusions": "conclusion"} {
		a := action
		mux.HandleFunc("POST /repositories/{id}/outcome-validations/{validation_id}"+path, func(w http.ResponseWriter, r *http.Request) { mutate(w, r, a) })
	}
}
func writeValidation(w http.ResponseWriter, v outcomevalidations.Validation, e error, status int) {
	switch {
	case e == nil:
		writeJSON(w, status, v)
	case errors.Is(e, outcomevalidations.ErrNotFound):
		writeAPIError(w, 404, "validation_not_found", "outcome validation not found")
	case errors.Is(e, outcomevalidations.ErrConflict):
		writeAPIError(w, 409, "validation_changed", "validation changed; refresh before adding evidence")
	case errors.Is(e, outcomevalidations.ErrInvalid):
		writeAPIError(w, 400, "invalid_validation", "revision-exact consent, representative measures, and complete evidence are required")
	default:
		log.Printf("outcome validation storage: %v", e)
		writeAPIError(w, 500, "validation_unavailable", "outcome validation could not be persisted")
	}
}
