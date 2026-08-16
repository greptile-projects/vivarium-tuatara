package main

import (
	"errors"
	"log"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/datacommitments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/governance"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/incidents"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/recoverycommitments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/serviceobjectives"
)

type recoveryCommitmentInput struct {
	ExpectedVersion int                          `json:"expected_version"`
	Revision        recoverycommitments.Revision `json:"revision"`
}

func registerRecoveryCommitmentRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, store *recoverycommitments.Store, objectives *serviceobjectives.Store, environments *deployments.Store, incidentStore *incidents.Store, privacyRules *datacommitments.Store, governanceStore *governance.Store) {
	mux.HandleFunc("GET /repositories/{id}/recovery-commitments", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		values, err := store.List(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "recovery_commitments_unavailable", "recovery commitments could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"commitments": values})
	})
	mux.HandleFunc("GET /repositories/{id}/recovery-commitments/{commitment_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		out, err := store.Get(r.PathValue("commitment_id"))
		if err != nil || out.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "recovery_commitment_not_found", "recovery commitment not found")
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
			var in recoveryCommitmentInput
			if decodeJSON(r, &in) != nil {
				writeAPIError(w, 400, "invalid_request", "a complete recovery commitment revision is required")
				return
			}
			var current recoverycommitments.Commitment
			var err error
			if revise {
				current, err = store.Get(r.PathValue("commitment_id"))
				if err != nil || current.RepositoryID != r.PathValue("id") {
					writeAPIError(w, 404, "recovery_commitment_not_found", "recovery commitment not found")
					return
				}
			}
			owners := recoveryCommitmentOwners(actor.UserID, in.Revision)
			var out recoverycommitments.Commitment
			err = catalog.WithCurrentParticipants(owners, r.PathValue("id"), func() error {
				if err = validateRecoveryCommitmentLinks(r.PathValue("id"), in.Revision.Links, objectives, environments, incidentStore, privacyRules, governanceStore); err != nil {
					return err
				}
				if revise {
					out, err = store.Revise(current.ID, in.ExpectedVersion, actor.UserID, in.Revision)
				} else {
					out, err = store.Create(r.PathValue("id"), actor.UserID, in.Revision)
				}
				return err
			})
			status := 201
			if revise {
				status = 200
			}
			writeRecoveryCommitment(w, out, err, status)
		}
	}
	mux.HandleFunc("POST /repositories/{id}/recovery-commitments", publish(false))
	mux.HandleFunc("POST /repositories/{id}/recovery-commitments/{commitment_id}/revisions", publish(true))
}

func validateRecoveryCommitmentLinks(repositoryID string, links []recoverycommitments.Link, objectives *serviceobjectives.Store, environments *deployments.Store, incidentStore *incidents.Store, privacyRules *datacommitments.Store, governanceStore *governance.Store) error {
	for _, link := range links {
		switch link.Kind {
		case "service_objective":
			if objectives == nil {
				return recoverycommitments.ErrInvalid
			}
			v, err := objectives.Get(link.ID)
			if err != nil || v.RepositoryID != repositoryID {
				return recoverycommitments.ErrInvalid
			}
		case "environment":
			if environments == nil {
				return recoverycommitments.ErrInvalid
			}
			if _, err := environments.GetEnvironment(repositoryID, link.ID); err != nil {
				return recoverycommitments.ErrInvalid
			}
		case "incident":
			if incidentStore == nil {
				return recoverycommitments.ErrInvalid
			}
			v, err := incidentStore.Get(link.ID)
			if err != nil || !incidentIncludesRepository(v, repositoryID) {
				return recoverycommitments.ErrInvalid
			}
		case "privacy_rule":
			if privacyRules == nil {
				return recoverycommitments.ErrInvalid
			}
			v, err := privacyRules.Get(link.ID)
			if err != nil || v.RepositoryID != repositoryID {
				return recoverycommitments.ErrInvalid
			}
		case "governance":
			if governanceStore == nil {
				return recoverycommitments.ErrInvalid
			}
			v, err := governanceStore.Get(link.ID)
			if err != nil || v.ScopeType != "repository" || v.ScopeID != repositoryID {
				return recoverycommitments.ErrInvalid
			}
		default:
			return recoverycommitments.ErrInvalid
		}
	}
	return nil
}

func incidentIncludesRepository(v incidents.Incident, repositoryID string) bool {
	for _, scope := range v.Scopes {
		if scope.RepositoryID == repositoryID {
			return true
		}
	}
	return false
}

func recoveryCommitmentOwners(actor string, r recoverycommitments.Revision) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	add(actor)
	for _, id := range r.OwnerIDs {
		add(id)
	}
	for _, target := range r.Targets {
		for _, id := range target.OwnerIDs {
			add(id)
		}
	}
	for _, x := range r.Exceptions {
		add(x.ApprovedBy)
	}
	return out
}
func writeRecoveryCommitment(w http.ResponseWriter, v recoverycommitments.Commitment, err error, status int) {
	switch {
	case err == nil:
		writeJSON(w, status, v)
	case errors.Is(err, recoverycommitments.ErrConflict):
		writeAPIError(w, 409, "recovery_commitment_conflict", "the commitment changed; reload before publishing another revision")
	case errors.Is(err, recoverycommitments.ErrInvalid):
		writeAPIError(w, 400, "invalid_recovery_commitment", "define complete recovery targets, retention, jurisdictions, validation, dependencies, and accountable contract owners")
	case errors.Is(err, repositories.ErrInvalidCollaborator), errors.Is(err, repositories.ErrNotFound):
		writeAPIError(w, 403, "recovery_commitment_forbidden", "only current repository participants may own or publish recovery commitments")
	default:
		log.Printf("recovery commitment storage: %v", err)
		writeAPIError(w, 500, "recovery_commitments_unavailable", "recovery commitments could not be persisted")
	}
}
