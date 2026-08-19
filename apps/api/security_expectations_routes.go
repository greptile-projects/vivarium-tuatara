package main

import (
	"errors"
	"log"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/securityexpectations"
)

type securityExpectationInput struct {
	ExpectedVersion int                           `json:"expected_version"`
	Revision        securityexpectations.Revision `json:"revision"`
}

func registerSecurityExpectationRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, store *securityexpectations.Store) {
	mux.HandleFunc("GET /repositories/{id}/security-expectations", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		values, e := store.List(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "security_expectations_unavailable", "security expectations could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"expectations": values})
	})
	mux.HandleFunc("GET /repositories/{id}/security-expectations/{expectation_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		v, e := store.Get(r.PathValue("expectation_id"))
		if e != nil || v.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "security_expectation_not_found", "security expectation not found")
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
			var in securityExpectationInput
			if decodeJSON(r, &in) != nil {
				writeAPIError(w, 400, "invalid_request", "a complete security expectation revision is required")
				return
			}
			repo := r.PathValue("id")
			participants := securityExpectationParticipants(actor.UserID, in.Revision)
			var out securityexpectations.Expectation
			var e error
			e = catalog.WithCurrentParticipants(participants, repo, func() error {
				if revise {
					current, x := store.Get(r.PathValue("expectation_id"))
					if x != nil || current.RepositoryID != repo {
						return securityexpectations.ErrNotFound
					}
					out, x = store.Revise(current.ID, in.ExpectedVersion, actor.UserID, in.Revision)
					return x
				}
				out, e = store.Create(repo, actor.UserID, in.Revision)
				return e
			})
			status := http.StatusCreated
			if revise {
				status = http.StatusOK
			}
			writeSecurityExpectation(w, out, e, status)
		}
	}
	mux.HandleFunc("POST /repositories/{id}/security-expectations", publish(false))
	mux.HandleFunc("POST /repositories/{id}/security-expectations/{expectation_id}/revisions", publish(true))
}
func securityExpectationParticipants(actor string, r securityexpectations.Revision) []string {
	v := []string{actor}
	v = append(v, r.OwnerIDs...)
	for _, x := range r.Assets {
		v = append(v, x.OwnerIDs...)
	}
	for _, x := range r.AbuseCases {
		v = append(v, x.OwnerIDs...)
	}
	for _, x := range r.Controls {
		v = append(v, x.OwnerIDs...)
	}
	for _, x := range r.Exceptions {
		v = append(v, x.GrantedBy)
	}
	return v
}
func writeSecurityExpectation(w http.ResponseWriter, v securityexpectations.Expectation, e error, status int) {
	switch {
	case e == nil:
		writeJSON(w, status, v)
	case errors.Is(e, securityexpectations.ErrNotFound):
		writeAPIError(w, 404, "security_expectation_not_found", "security expectation not found")
	case errors.Is(e, securityexpectations.ErrConflict):
		writeAPIError(w, 409, "security_expectation_conflict", "the expectation changed; reload before publishing another revision")
	case errors.Is(e, securityexpectations.ErrInvalid):
		writeAPIError(w, 400, "invalid_security_expectation", "security intent must contain valid scopes, assets, boundaries, actors, abuse cases, controls, severity policy, links, and exceptions")
	case errors.Is(e, repositories.ErrInvalidCollaborator), errors.Is(e, repositories.ErrNotFound):
		writeAPIError(w, 403, "security_expectation_forbidden", "security owners and exception grantors must be current repository participants")
	default:
		log.Printf("security expectation storage: %v", e)
		writeAPIError(w, 500, "security_expectations_unavailable", "security expectations could not be persisted")
	}
}
