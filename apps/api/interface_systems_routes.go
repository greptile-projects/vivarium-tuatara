package main

import (
	"errors"
	"log"
	"net/http"
	"path"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/interfacesystems"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

type interfaceSystemInput struct {
	ExpectedVersion int                       `json:"expected_version"`
	Revision        interfacesystems.Revision `json:"revision"`
}

func registerInterfaceSystemRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, systems *interfacesystems.Store, releaseStore *releases.Store) {
	mux.HandleFunc("GET /repositories/{id}/interface-systems", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		values, err := systems.List(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "interface_systems_unavailable", "interface systems could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"interface_systems": values})
	})
	mux.HandleFunc("GET /repositories/{id}/interface-systems/{system_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		out, err := systems.Get(r.PathValue("system_id"))
		if err != nil || out.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "interface_system_not_found", "interface system not found")
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
			var in interfaceSystemInput
			if decodeJSON(r, &in) != nil {
				writeAPIError(w, 400, "invalid_request", "a complete interface-system revision is required")
				return
			}
			if !interfaceSystemProvenanceResolves(git, releaseStore, r.PathValue("id"), &in.Revision) {
				writeAPIError(w, 400, "invalid_interface_system_provenance", "the release, exact commit, and implementation source paths must resolve in this repository")
				return
			}
			owners := interfaceSystemOwners(actor.UserID, in.Revision)
			var out interfacesystems.System
			var err error
			err = catalog.WithCurrentParticipants(owners, r.PathValue("id"), func() error {
				if revise {
					current, e := systems.Get(r.PathValue("system_id"))
					if e != nil || current.RepositoryID != r.PathValue("id") {
						return interfacesystems.ErrNotFound
					}
					out, err = systems.Revise(current.ID, in.ExpectedVersion, actor.UserID, in.Revision)
				} else {
					out, err = systems.Create(r.PathValue("id"), actor.UserID, in.Revision)
				}
				return err
			})
			status := 201
			if revise {
				status = 200
			}
			writeInterfaceSystem(w, out, err, status)
		}
	}
	mux.HandleFunc("POST /repositories/{id}/interface-systems", publish(false))
	mux.HandleFunc("POST /repositories/{id}/interface-systems/{system_id}/revisions", publish(true))
}

func interfaceSystemProvenanceResolves(git *storage.Store, releaseStore *releases.Store, repositoryID string, revision *interfacesystems.Revision) bool {
	if git == nil || releaseStore == nil {
		return false
	}
	release, err := releaseStore.Get(repositoryID, revision.ReleaseID)
	if err != nil || release.CommitID != strings.ToLower(revision.CommitID) {
		return false
	}
	revision.CommitID = release.CommitID
	revision.ReleaseVersion = release.Version
	repository, err := git.Open(repositoryID)
	if err != nil {
		return false
	}
	commit, err := repository.ReadCommit(storage.ObjectID(revision.CommitID))
	if err != nil {
		return false
	}
	entries, err := repository.WalkTree(commit.Tree)
	if err != nil {
		return false
	}
	files := map[string]bool{}
	for _, e := range entries {
		if e.Type == storage.BlobObject {
			files[e.Path] = true
		}
	}
	for _, group := range [][]interfacesystems.Definition{revision.Components, revision.InteractionPatterns, revision.ContentRules} {
		for _, definition := range group {
			candidate := definition.SourcePath
			if candidate == "" || strings.HasPrefix(candidate, "/") || path.Clean(candidate) != candidate || candidate == "." || strings.HasPrefix(candidate, "../") || !files[candidate] {
				return false
			}
		}
	}
	for index := range revision.Implementations {
		implementation := &revision.Implementations[index]
		if implementation.RepositoryID != "" && implementation.RepositoryID != repositoryID {
			// This repository-scoped mutation grants no authority to make
			// attributable provenance claims for another repository.
			return false
		}
		implementationRepositoryID := repositoryID
		implementationRelease, err := releaseStore.Get(implementationRepositoryID, implementation.ReleaseID)
		if err != nil || implementationRelease.CommitID != strings.ToLower(implementation.CommitID) {
			return false
		}
		if implementation.Status == "current" && (implementation.ReleaseID != revision.ReleaseID || implementationRelease.CommitID != revision.CommitID) {
			return false
		}
		implementationRepository, err := git.Open(implementationRepositoryID)
		if err != nil {
			return false
		}
		if _, err = implementationRepository.ReadCommit(storage.ObjectID(implementationRelease.CommitID)); err != nil {
			return false
		}
		implementation.RepositoryID = implementationRepositoryID
		implementation.CommitID = implementationRelease.CommitID
	}
	return true
}
func interfaceSystemOwners(actor string, r interfacesystems.Revision) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(v string) {
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	add(actor)
	for _, v := range r.OwnerIDs {
		add(v)
	}
	for _, v := range r.Tokens {
		for _, id := range v.OwnerIDs {
			add(id)
		}
	}
	for _, group := range [][]interfacesystems.Definition{r.Components, r.InteractionPatterns, r.ContentRules} {
		for _, v := range group {
			for _, id := range v.OwnerIDs {
				add(id)
			}
		}
	}
	for _, v := range r.ResponsiveRules {
		for _, id := range v.OwnerIDs {
			add(id)
		}
	}
	return out
}
func writeInterfaceSystem(w http.ResponseWriter, v interfacesystems.System, err error, status int) {
	switch {
	case err == nil:
		writeJSON(w, status, v)
	case errors.Is(err, interfacesystems.ErrNotFound):
		writeAPIError(w, 404, "interface_system_not_found", "interface system not found")
	case errors.Is(err, interfacesystems.ErrConflict):
		writeAPIError(w, 409, "interface_system_conflict", "the interface system changed; reload before publishing another revision")
	case errors.Is(err, interfacesystems.ErrInvalid):
		writeAPIError(w, 400, "invalid_interface_system", "define complete revision provenance, examples, constraints, themes, ownership, and adoption policy")
	case errors.Is(err, repositories.ErrInvalidCollaborator), errors.Is(err, repositories.ErrNotFound):
		writeAPIError(w, 403, "interface_system_forbidden", "only current repository participants may own or publish interface systems")
	default:
		log.Printf("interface system storage: %v", err)
		writeAPIError(w, 500, "interface_systems_unavailable", "interface systems could not be persisted")
	}
}
