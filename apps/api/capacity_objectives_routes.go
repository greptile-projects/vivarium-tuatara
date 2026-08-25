package main

import (
	"errors"
	"log"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/capacityobjectives"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

type capacityObjectiveInput struct {
	RequestID       string                      `json:"request_id"`
	ExpectedVersion int                         `json:"expected_version"`
	Revision        capacityobjectives.Revision `json:"revision"`
}

func registerCapacityObjectiveRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, store *capacityobjectives.Store) {
	mux.HandleFunc("GET /repositories/{id}/capacity-objectives", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		values, err := store.List(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "capacity_objectives_unavailable", "capacity objectives could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"capacity_objectives": values})
	})
	mux.HandleFunc("GET /repositories/{id}/capacity-objectives/{objective_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		out, err := store.Get(r.PathValue("objective_id"))
		if err != nil || out.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "capacity_objective_not_found", "capacity objective not found")
			return
		}
		writeJSON(w, 200, out)
	})
	mux.HandleFunc("POST /repositories/{id}/capacity-objectives", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in capacityObjectiveInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete capacity objective revision is required")
			return
		}
		var out capacityobjectives.Objective
		err := catalog.WithCurrentParticipants(append([]string{actor.UserID}, in.Revision.OwnerIDs...), r.PathValue("id"), func() error {
			var e error
			out, e = store.Create(r.PathValue("id"), actor.UserID, in.RequestID, in.Revision)
			return e
		})
		writeCapacityObjective(w, out, err, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/capacity-objectives/{objective_id}/revisions", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		current, err := store.Get(r.PathValue("objective_id"))
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "capacity_objective_not_found", "capacity objective not found")
			return
		}
		var in capacityObjectiveInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "expected_version and a complete capacity objective revision are required")
			return
		}
		var out capacityobjectives.Objective
		err = catalog.WithCurrentParticipants(append([]string{actor.UserID}, in.Revision.OwnerIDs...), current.RepositoryID, func() error {
			var e error
			out, e = store.Revise(current.ID, in.ExpectedVersion, actor.UserID, in.RequestID, in.Revision)
			return e
		})
		writeCapacityObjective(w, out, err, 200)
	})
}
func writeCapacityObjective(w http.ResponseWriter, out capacityobjectives.Objective, err error, status int) {
	switch {
	case err == nil:
		writeJSON(w, status, out)
	case errors.Is(err, capacityobjectives.ErrConflict):
		writeAPIError(w, 409, "capacity_objective_conflict", "the objective changed; reload before publishing another revision")
	case errors.Is(err, capacityobjectives.ErrCommitted):
		writeAPIError(w, 503, "capacity_objective_commit_ambiguous", "the objective may have committed; retry the unchanged request_id to reconcile it")
	case errors.Is(err, capacityobjectives.ErrInvalid):
		writeAPIError(w, 400, "invalid_capacity_objective", "the objective must include valid demand, reliability, resource, dependency, regional, ownership, cost, timing, success, and rollback boundaries")
	case errors.Is(err, repositories.ErrInvalidCollaborator), errors.Is(err, repositories.ErrNotFound):
		writeAPIError(w, 403, "capacity_objective_forbidden", "all capacity owners must be current repository participants")
	default:
		log.Printf("capacity objective storage: %v", err)
		writeAPIError(w, 500, "capacity_objectives_unavailable", "capacity objectives could not be persisted")
	}
}
