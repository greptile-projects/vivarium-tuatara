package main

import (
	"errors"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/runbooks"
	"net/http"
)

type runbookInput struct {
	RequestID       string            `json:"request_id"`
	ExpectedVersion int               `json:"expected_version"`
	Revision        runbooks.Revision `json:"revision"`
}

func registerRunbookRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, store *runbooks.Store) {
	mux.HandleFunc("GET /repositories/{id}/runbooks", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		out, e := store.List(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "runbooks_unavailable", "runbooks could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"runbooks": out})
	})
	mux.HandleFunc("GET /repositories/{id}/runbooks/{runbook_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		out, e := store.Get(r.PathValue("runbook_id"))
		if e != nil || out.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "runbook_not_found", "runbook not found")
			return
		}
		writeJSON(w, 200, out)
	})
	publish := func(revise bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
			if !ok {
				return
			}
			var in runbookInput
			if decodeJSON(r, &in) != nil {
				writeAPIError(w, 400, "invalid_request", "a caller-stable identity and complete runbook revision are required")
				return
			}
			participants := append([]string{actor.UserID}, in.Revision.OwnerIDs...)
			for _, s := range in.Revision.Steps {
				participants = append(participants, s.OwnerIDs...)
			}
			for _, e := range in.Revision.Escalations {
				participants = append(participants, e.OwnerID)
			}
			var out runbooks.Runbook
			err := catalog.WithCurrentParticipants(participants, r.PathValue("id"), func() error {
				var e error
				if revise {
					current, x := store.Get(r.PathValue("runbook_id"))
					if x != nil || current.RepositoryID != r.PathValue("id") {
						return runbooks.ErrNotFound
					}
					out, e = store.Revise(current.ID, in.ExpectedVersion, actor.UserID, in.RequestID, in.Revision)
				} else {
					out, e = store.Create(r.PathValue("id"), actor.UserID, in.RequestID, in.Revision)
				}
				return e
			})
			status := 201
			if revise {
				status = 200
			}
			switch {
			case err == nil:
				writeJSON(w, status, out)
			case errors.Is(err, runbooks.ErrNotFound):
				writeAPIError(w, 404, "runbook_not_found", "runbook not found")
			case errors.Is(err, runbooks.ErrConflict):
				writeAPIError(w, 409, "runbook_conflict", "the request identity or expected version conflicts")
			case errors.Is(err, runbooks.ErrInvalid):
				writeAPIError(w, 400, "invalid_runbook", "the runbook revision is incomplete or invalid")
			default:
				writeAPIError(w, 400, "invalid_runbook", "owners and escalation recipients must be current repository participants")
			}
		}
	}
	mux.HandleFunc("POST /repositories/{id}/runbooks", publish(false))
	mux.HandleFunc("POST /repositories/{id}/runbooks/{runbook_id}/revisions", publish(true))
}
