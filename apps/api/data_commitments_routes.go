package main

import (
	"errors"
	"log"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/datacommitments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/extensions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/productexperiments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

type dataCommitmentInput struct {
	ExpectedVersion int                      `json:"expected_version"`
	Revision        datacommitments.Revision `json:"revision"`
}

func registerDataCommitmentRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, store *datacommitments.Store, releaseStore *releases.Store, extensionStore *extensions.Store, experimentStore *productexperiments.Store, deploymentStore *deployments.Store) {
	mux.HandleFunc("GET /repositories/{id}/data-commitments", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		values, err := store.List(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "data_commitments_unavailable", "data commitments could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"commitments": values})
	})
	mux.HandleFunc("GET /repositories/{id}/data-commitments/{commitment_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		out, err := store.Get(r.PathValue("commitment_id"))
		if err != nil || out.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "data_commitment_not_found", "data commitment not found")
			return
		}
		writeJSON(w, 200, out)
	})
	mux.HandleFunc("POST /repositories/{id}/data-commitments", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in dataCommitmentInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete data commitment revision is required")
			return
		}
		var out datacommitments.Commitment
		owners := dataCommitmentOwners(actor.UserID, in.Revision)
		err := catalog.WithCurrentParticipants(owners, r.PathValue("id"), func() error {
			var e error
			if e = validateDataCommitmentScopes(r.PathValue("id"), in.Revision.Scopes, releaseStore, extensionStore, experimentStore, deploymentStore); e != nil {
				return e
			}
			return withDataCommitmentExtensionScopes(r.PathValue("id"), in.Revision.Scopes, extensionStore, func() error {
				out, e = store.Create(r.PathValue("id"), actor.UserID, in.Revision)
				return e
			})
		})
		writeDataCommitment(w, out, err, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/data-commitments/{commitment_id}/revisions", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		current, err := store.Get(r.PathValue("commitment_id"))
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "data_commitment_not_found", "data commitment not found")
			return
		}
		var in dataCommitmentInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "expected_version and a complete data commitment revision are required")
			return
		}
		var out datacommitments.Commitment
		owners := dataCommitmentOwners(actor.UserID, in.Revision)
		err = catalog.WithCurrentParticipants(owners, current.RepositoryID, func() error {
			var e error
			if e = validateDataCommitmentScopes(current.RepositoryID, in.Revision.Scopes, releaseStore, extensionStore, experimentStore, deploymentStore); e != nil {
				return e
			}
			return withDataCommitmentExtensionScopes(current.RepositoryID, in.Revision.Scopes, extensionStore, func() error {
				out, e = store.Revise(current.ID, in.ExpectedVersion, actor.UserID, in.Revision)
				return e
			})
		})
		writeDataCommitment(w, out, err, 200)
	})
}

func dataCommitmentOwners(actor string, revision datacommitments.Revision) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(id string) {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	add(actor)
	for _, id := range revision.OwnerIDs {
		add(id)
	}
	for _, use := range revision.DataUses {
		for _, id := range use.OwnerIDs {
			add(id)
		}
	}
	for _, exception := range revision.Exceptions {
		add(exception.ApprovedBy)
	}
	return out
}

func withDataCommitmentExtensionScopes(repositoryID string, scopes []datacommitments.Scope, extensionStore *extensions.Store, fn func() error) error {
	ids := []string{}
	for _, scope := range scopes {
		if scope.Kind == "extension" {
			ids = append(ids, scope.ResourceID)
		}
	}
	if len(ids) == 0 {
		return fn()
	}
	if extensionStore == nil {
		return datacommitments.ErrInvalid
	}
	err := extensionStore.WithInstallationsRepository(ids, repositoryID, fn)
	if errors.Is(err, extensions.ErrInvalid) || errors.Is(err, extensions.ErrNotFound) {
		return datacommitments.ErrInvalid
	}
	return err
}

func validateDataCommitmentScopes(repositoryID string, scopes []datacommitments.Scope, releaseStore *releases.Store, extensionStore *extensions.Store, experimentStore *productexperiments.Store, deploymentStore *deployments.Store) error {
	for _, scope := range scopes {
		switch scope.Kind {
		case "repository":
			if scope.ResourceID != "" && scope.ResourceID != repositoryID {
				return datacommitments.ErrInvalid
			}
		case "release":
			if releaseStore == nil {
				return datacommitments.ErrInvalid
			}
			if _, err := releaseStore.Get(repositoryID, scope.ResourceID); err != nil {
				return datacommitments.ErrInvalid
			}
		case "extension":
			if extensionStore == nil {
				return datacommitments.ErrInvalid
			}
			installation, err := extensionStore.GetInstallation(scope.ResourceID)
			if err != nil || !dataCommitmentContains(installation.RepositoryIDs, repositoryID) {
				return datacommitments.ErrInvalid
			}
		case "experiment":
			if experimentStore == nil {
				return datacommitments.ErrInvalid
			}
			experiment, err := experimentStore.Get(scope.ResourceID)
			if err != nil || experiment.RepositoryID != repositoryID {
				return datacommitments.ErrInvalid
			}
		case "environment":
			if deploymentStore == nil {
				return datacommitments.ErrInvalid
			}
			if _, err := deploymentStore.GetEnvironment(repositoryID, scope.ResourceID); err != nil {
				return datacommitments.ErrInvalid
			}
		default:
			return datacommitments.ErrInvalid
		}
	}
	return nil
}

func dataCommitmentContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
func writeDataCommitment(w http.ResponseWriter, value datacommitments.Commitment, err error, success int) {
	switch {
	case err == nil:
		writeJSON(w, success, value)
	case errors.Is(err, datacommitments.ErrConflict):
		writeAPIError(w, 409, "data_commitment_conflict", "the commitment changed; reload before publishing another revision")
	case errors.Is(err, datacommitments.ErrInvalid):
		writeAPIError(w, 400, "invalid_data_commitment", "define scope, permitted data uses, accountable owners, and applicable policy and notice links")
	case errors.Is(err, repositories.ErrInvalidCollaborator), errors.Is(err, repositories.ErrNotFound):
		writeAPIError(w, 403, "data_commitment_forbidden", "only a current repository participant may publish data commitments")
	default:
		log.Printf("data commitment storage: %v", err)
		writeAPIError(w, 500, "data_commitments_unavailable", "data commitments could not be persisted")
	}
}
