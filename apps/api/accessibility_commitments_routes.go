package main

import (
	"errors"
	"log"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/accessibilitycommitments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

type accessibilityCommitmentInput struct {
	ExpectedVersion int                               `json:"expected_version"`
	Revision        accessibilitycommitments.Revision `json:"revision"`
}

func registerAccessibilityCommitmentRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, store *accessibilitycommitments.Store) {
	mux.HandleFunc("POST /repositories/{id}/accessibility-commitments", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in accessibilityCommitmentInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete accessibility commitment revision is required")
			return
		}
		var out accessibilitycommitments.Commitment
		err := catalog.WithCurrentParticipant(actor.UserID, r.PathValue("id"), func() error {
			var e error
			out, e = store.Create(r.PathValue("id"), actor.UserID, in.Revision)
			return e
		})
		writeAccessibilityCommitment(w, out, err, 201)
	})
	mux.HandleFunc("GET /repositories/{id}/accessibility-commitments", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		values, err := store.List(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "accessibility_commitments_unavailable", "accessibility commitments could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"commitments": values})
	})
	mux.HandleFunc("GET /repositories/{id}/accessibility-commitments/{commitment_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		out, err := store.Get(r.PathValue("commitment_id"))
		if err != nil || out.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "accessibility_commitment_not_found", "accessibility commitment not found")
			return
		}
		writeJSON(w, 200, out)
	})
	mux.HandleFunc("POST /repositories/{id}/accessibility-commitments/{commitment_id}/revisions", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		current, err := store.Get(r.PathValue("commitment_id"))
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "accessibility_commitment_not_found", "accessibility commitment not found")
			return
		}
		var in accessibilityCommitmentInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "expected_version and a complete revision are required")
			return
		}
		var out accessibilitycommitments.Commitment
		err = catalog.WithCurrentParticipant(actor.UserID, current.RepositoryID, func() error {
			var e error
			out, e = store.Revise(current.ID, in.ExpectedVersion, actor.UserID, in.Revision)
			return e
		})
		writeAccessibilityCommitment(w, out, err, 200)
	})
}

func writeAccessibilityCommitment(w http.ResponseWriter, value accessibilitycommitments.Commitment, err error, success int) {
	switch {
	case err == nil:
		writeJSON(w, success, value)
	case errors.Is(err, accessibilitycommitments.ErrConflict):
		writeAPIError(w, 409, "accessibility_commitment_conflict", "the commitment changed; reload before publishing another revision")
	case errors.Is(err, accessibilitycommitments.ErrInvalid):
		writeAPIError(w, 400, "invalid_accessibility_commitment", "the contract must define a supported subject, standards, assistive technologies, audiences, environments, scenarios, severity policy, and owners")
	case errors.Is(err, repositories.ErrInvalidCollaborator), errors.Is(err, repositories.ErrNotFound):
		writeAPIError(w, 403, "accessibility_commitment_forbidden", "only a current repository participant may publish accessibility commitments")
	default:
		log.Printf("accessibility commitment storage: %v", err)
		writeAPIError(w, 500, "accessibility_commitments_unavailable", "accessibility commitments could not be persisted")
	}
}
