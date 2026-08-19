package main

import (
	"errors"
	"log"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/qualityplans"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

type qualityPlanInput struct {
	ExpectedVersion int                   `json:"expected_version"`
	Revision        qualityplans.Revision `json:"revision"`
}

func registerQualityPlanRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, store *qualityplans.Store) {
	mux.HandleFunc("GET /repositories/{id}/quality-plans", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		values, e := store.List(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "quality_plans_unavailable", "quality plans could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"plans": values})
	})
	mux.HandleFunc("GET /repositories/{id}/quality-plans/{plan_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		v, e := store.Get(r.PathValue("plan_id"))
		if e != nil || v.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "quality_plan_not_found", "quality plan not found")
			return
		}
		writeJSON(w, 200, v)
	})
	publish := func(revise bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
			if !ok {
				return
			}
			var in qualityPlanInput
			if decodeJSON(r, &in) != nil {
				writeAPIError(w, 400, "invalid_request", "a complete quality plan revision is required")
				return
			}
			repo := r.PathValue("id")
			participants := qualityPlanParticipants(actor.UserID, in.Revision)
			var out qualityplans.Plan
			var e error
			e = catalog.WithCurrentParticipants(participants, repo, func() error {
				if revise {
					current, x := store.Get(r.PathValue("plan_id"))
					if x != nil || current.RepositoryID != repo {
						return qualityplans.ErrNotFound
					}
					out, x = store.Revise(current.ID, in.ExpectedVersion, actor.UserID, in.Revision)
					return x
				}
				out, e = store.Create(repo, actor.UserID, in.Revision)
				return e
			})
			status := 201
			if revise {
				status = 200
			}
			writeQualityPlan(w, out, e, status)
		}
	}
	mux.HandleFunc("POST /repositories/{id}/quality-plans", publish(false))
	mux.HandleFunc("POST /repositories/{id}/quality-plans/{plan_id}/revisions", publish(true))
}
func qualityPlanParticipants(actor string, r qualityplans.Revision) []string {
	v := []string{actor}
	v = append(v, r.OwnerIDs...)
	for _, q := range r.Requirements {
		v = append(v, q.OwnerIDs...)
		v = append(v, q.JudgeIDs...)
	}
	for _, x := range r.Exceptions {
		v = append(v, x.GrantedBy)
	}
	return v
}
func writeQualityPlan(w http.ResponseWriter, v qualityplans.Plan, e error, status int) {
	switch {
	case e == nil:
		writeJSON(w, status, v)
	case errors.Is(e, qualityplans.ErrNotFound):
		writeAPIError(w, 404, "quality_plan_not_found", "quality plan not found")
	case errors.Is(e, qualityplans.ErrConflict):
		writeAPIError(w, 409, "quality_plan_conflict", "the plan changed; reload before publishing another revision")
	case errors.Is(e, qualityplans.ErrInvalid):
		writeAPIError(w, 400, "invalid_quality_plan", "the plan must contain valid scope, environments, expectations, evidence references, schedules, and thresholds")
	case errors.Is(e, repositories.ErrInvalidCollaborator), errors.Is(e, repositories.ErrNotFound):
		writeAPIError(w, 403, "quality_plan_forbidden", "quality owners, judges, and exception grantors must be current repository participants")
	default:
		log.Printf("quality plan storage: %v", e)
		writeAPIError(w, 500, "quality_plans_unavailable", "quality plans could not be persisted")
	}
}
