package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"path"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/apicontracts"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

type apiContractInput struct {
	ExpectedVersion int                   `json:"expected_version"`
	Revision        apicontracts.Revision `json:"revision"`
}

func registerAPIContractRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, store *apicontracts.Store, pulls *pullrequests.Store, releaseStore *releases.Store) {
	present := func(repo string, value apicontracts.Contract) apicontracts.Contract {
		if len(value.Revisions) == 0 {
			return value
		}
		current := value.Revisions[len(value.Revisions)-1]
		if repository, e := catalog.GetByID(repo); e == nil {
			if gr, e := git.Open(repo); e == nil {
				if ref, e := gr.ReadReference("refs/heads/" + repository.DefaultBranch); e != nil || ref.Target != current.Source.CommitID {
					value.Diagnostics = append(value.Diagnostics, apicontracts.Diagnostic{Code: "stale_documentation", Severity: "warning", Detail: "The default branch has advanced beyond the reviewed contract and its documentation must be revalidated."})
				}
			}
		}
		if current.Source.ReleaseID != "" {
			if release, e := releaseStore.Get(repo, current.Source.ReleaseID); e != nil || release.CommitID != current.Source.CommitID {
				value.Diagnostics = append(value.Diagnostics, apicontracts.Diagnostic{Code: "release_unavailable", Severity: "error", Detail: "The implementation release no longer resolves to this contract commit."})
			}
		}
		return value
	}
	mux.HandleFunc("GET /repositories/{id}/api-contracts", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		values, err := store.List(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "api_contracts_unavailable", "API contracts could not be read")
			return
		}
		for i := range values {
			values[i] = present(r.PathValue("id"), values[i])
		}
		writeJSON(w, 200, map[string]any{"contracts": values})
	})
	mux.HandleFunc("GET /repositories/{id}/api-contracts/{contract_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		out, err := store.Get(r.PathValue("contract_id"))
		if err != nil || out.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "api_contract_not_found", "API contract not found")
			return
		}
		writeJSON(w, 200, present(r.PathValue("id"), out))
	})
	publish := func(revise bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
			if !ok {
				return
			}
			var in apiContractInput
			if decodeJSON(r, &in) != nil {
				writeAPIError(w, 400, "invalid_request", "a complete API contract revision is required")
				return
			}
			repo := r.PathValue("id")
			pull, err := pulls.Get(repo, in.Revision.Source.PullRequestID)
			if err != nil || pull.Status != pullrequests.Merged || pull.MergeCommitID == nil || *pull.MergeCommitID != in.Revision.Source.CommitID {
				writeAPIError(w, 422, "unreviewed_api_contract_source", "the contract must cite the exact merge commit of a reviewed repository pull request")
				return
			}
			if in.Revision.Source.ReleaseID != "" {
				release, e := releaseStore.Get(repo, in.Revision.Source.ReleaseID)
				if e != nil || release.CommitID != in.Revision.Source.CommitID {
					writeAPIError(w, 422, "unreleased_api_contract_source", "the cited release must contain the exact reviewed implementation commit")
					return
				}
			}
			if !apiContractSourcePathsResolve(git, repo, in.Revision.Source) {
				writeAPIError(w, 422, "invalid_api_contract_source_paths", "definition_path must be a valid OpenAPI JSON file and documentation_path must be a file in the exact reviewed merge commit")
				return
			}
			owners := append([]string{actor.UserID}, in.Revision.OwnerIDs...)
			for _, operation := range in.Revision.Operations {
				owners = append(owners, operation.OwnerIDs...)
			}
			var out apicontracts.Contract
			err = catalog.WithCurrentParticipants(owners, repo, func() error {
				if revise {
					current, e := store.Get(r.PathValue("contract_id"))
					if e != nil || current.RepositoryID != repo {
						return apicontracts.ErrNotFound
					}
					out, e = store.Revise(current.ID, in.ExpectedVersion, actor.UserID, in.Revision)
					return e
				}
				out, err = store.Create(repo, actor.UserID, in.Revision)
				return err
			})
			status := 201
			if revise {
				status = 200
			}
			writeAPIContract(w, out, err, status)
		}
	}
	mux.HandleFunc("POST /repositories/{id}/api-contracts", publish(false))
	mux.HandleFunc("POST /repositories/{id}/api-contracts/{contract_id}/revisions", publish(true))
}

func apiContractSourcePathsResolve(git *storage.Store, repositoryID string, source apicontracts.Source) bool {
	paths := []string{source.DefinitionPath, source.DocumentationPath}
	for _, candidate := range paths {
		if candidate == "" || strings.HasPrefix(candidate, "/") || path.Clean(candidate) != candidate || candidate == "." || strings.HasPrefix(candidate, "../") {
			return false
		}
	}
	repository, err := git.Open(repositoryID)
	if err != nil {
		return false
	}
	commit, err := repository.ReadCommit(storage.ObjectID(source.CommitID))
	if err != nil {
		return false
	}
	entries, err := repository.WalkTree(commit.Tree)
	if err != nil {
		return false
	}
	found := map[string]storage.ObjectID{}
	for _, entry := range entries {
		if entry.Type == storage.BlobObject {
			found[entry.Path] = entry.ID
		}
	}
	definitionID, definitionFound := found[source.DefinitionPath]
	_, documentationFound := found[source.DocumentationPath]
	if !definitionFound || !documentationFound {
		return false
	}
	definition, err := repository.ReadObject(definitionID)
	if err != nil || definition.Type != storage.BlobObject {
		return false
	}
	var document struct {
		OpenAPI string          `json:"openapi"`
		Swagger string          `json:"swagger"`
		Paths   json.RawMessage `json:"paths"`
	}
	if json.Unmarshal(definition.Content, &document) != nil || (document.OpenAPI == "" && document.Swagger != "2.0") {
		return false
	}
	if document.OpenAPI != "" && !strings.HasPrefix(document.OpenAPI, "3.") {
		return false
	}
	var pathDefinitions map[string]json.RawMessage
	return len(document.Paths) > 0 && json.Unmarshal(document.Paths, &pathDefinitions) == nil && pathDefinitions != nil
}
func writeAPIContract(w http.ResponseWriter, v apicontracts.Contract, err error, status int) {
	switch {
	case err == nil:
		writeJSON(w, status, v)
	case errors.Is(err, apicontracts.ErrNotFound):
		writeAPIError(w, 404, "api_contract_not_found", "API contract not found")
	case errors.Is(err, apicontracts.ErrConflict):
		writeAPIError(w, 409, "api_contract_conflict", "the contract changed; reload before publishing another version")
	case errors.Is(err, apicontracts.ErrInvalid):
		writeAPIError(w, 400, "invalid_api_contract", "define operations, schemas, errors, authentication, environments, limits, ownership, stability, support, compatibility, provenance, and links")
	case errors.Is(err, repositories.ErrInvalidCollaborator), errors.Is(err, repositories.ErrNotFound):
		writeAPIError(w, 403, "api_contract_forbidden", "only current repository participants may own or publish API contracts")
	default:
		log.Printf("api contract storage: %v", err)
		writeAPIError(w, 500, "api_contracts_unavailable", "API contract could not be persisted")
	}
}
