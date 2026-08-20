package main

import (
	"errors"
	"log"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/assuranceprograms"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

type assuranceProgramInput struct {
	ExpectedVersion int                        `json:"expected_version"`
	Revision        assuranceprograms.Revision `json:"revision"`
}

func registerAssuranceProgramRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, store *assuranceprograms.Store) {
	mux.HandleFunc("GET /repositories/{id}/assurance-programs", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		v, e := store.List(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "assurance_programs_unavailable", "assurance programs could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"programs": v})
	})
	mux.HandleFunc("GET /repositories/{id}/assurance-programs/{program_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		v, e := store.Get(r.PathValue("program_id"))
		if e != nil || v.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "assurance_program_not_found", "assurance program not found")
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
			var in assuranceProgramInput
			if decodeJSON(r, &in) != nil {
				writeAPIError(w, 400, "invalid_request", "a complete assurance program revision is required")
				return
			}
			repo := r.PathValue("id")
			participants := []string{actor.UserID}
			participants = append(participants, in.Revision.OwnerIDs...)
			for _, q := range in.Revision.Requirements {
				participants = append(participants, q.OwnerIDs...)
			}
			for _, c := range in.Revision.Controls {
				participants = append(participants, c.OwnerIDs...)
			}
			for _, x := range in.Revision.Exceptions {
				participants = append(participants, x.GrantedBy)
			}
			var out assuranceprograms.Program
			var e error
			e = catalog.WithCurrentParticipants(participants, repo, func() error {
				if revise {
					current, x := store.Get(r.PathValue("program_id"))
					if x != nil || current.RepositoryID != repo {
						return assuranceprograms.ErrNotFound
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
			writeAssuranceProgram(w, out, e, status)
		}
	}
	mux.HandleFunc("POST /repositories/{id}/assurance-programs", publish(false))
	mux.HandleFunc("POST /repositories/{id}/assurance-programs/{program_id}/revisions", publish(true))
}
func writeAssuranceProgram(w http.ResponseWriter, v assuranceprograms.Program, e error, status int) {
	switch {
	case e == nil:
		writeJSON(w, status, v)
	case errors.Is(e, assuranceprograms.ErrNotFound):
		writeAPIError(w, 404, "assurance_program_not_found", "assurance program not found")
	case errors.Is(e, assuranceprograms.ErrConflict):
		writeAPIError(w, 409, "assurance_program_conflict", "the program changed; reload before publishing")
	case errors.Is(e, assuranceprograms.ErrInvalid):
		writeAPIError(w, 400, "invalid_assurance_program", "requirements, scope, controls, mappings, evidence criteria, and exceptions must be complete and internally consistent")
	case errors.Is(e, repositories.ErrInvalidCollaborator), errors.Is(e, repositories.ErrNotFound):
		writeAPIError(w, 403, "assurance_program_forbidden", "program, requirement, control, and exception owners must be current repository participants")
	default:
		log.Printf("assurance program storage: %v", e)
		writeAPIError(w, 500, "assurance_programs_unavailable", "assurance programs could not be persisted")
	}
}
