package main

import (
	"errors"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/extensions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

func registerExtensionInstallationRoutes(mux *http.ServeMux, store *extensions.Store, credentials *auth.Store, repositoriesStore *repositories.Store, organizationsStore *organizations.Store) {
	authorize := func(actor, ownerType, ownerID string, repositoryIDs []string) bool {
		if ownerType == "repository" {
			repository, err := repositoriesStore.Get(actor, ownerID)
			return err == nil && len(repositoryIDs) == 1 && repositoryIDs[0] == repository.ID
		}
		organization, err := organizationsStore.Get(ownerID)
		if err != nil || !organizations.HasRole(organization, actor, "owner") {
			return false
		}
		for _, id := range repositoryIDs {
			repository, err := repositoriesStore.GetByID(id)
			if err != nil || repository.OrganizationID != ownerID {
				return false
			}
		}
		return true
	}
	mux.HandleFunc("GET /extension-installations", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		all, err := store.ListInstallations("")
		if err != nil {
			writeAPIError(w, 500, "extension_storage_unavailable", "installations could not be loaded")
			return
		}
		v := []extensions.Installation{}
		for _, installation := range all {
			if authorize(actor.UserID, installation.OwnerType, installation.OwnerID, installation.RepositoryIDs) {
				v = append(v, installation)
			}
		}
		writeJSON(w, 200, map[string]any{"installations": v})
	})
	mux.HandleFunc("POST /extensions/{id}/installations", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:write", false)
		if !ok {
			return
		}
		var in extensions.InstallationInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_installation", "request body must be valid JSON")
			return
		}
		if !authorize(actor.UserID, in.OwnerType, in.OwnerID, in.RepositoryIDs) {
			writeAPIError(w, 404, "installation_owner_not_found", "installation owner not found")
			return
		}
		v, err := store.CreateInstallation(r.PathValue("id"), actor.UserID, in)
		if errors.Is(err, extensions.ErrInvalid) {
			writeAPIError(w, 422, "invalid_installation", "exact repositories, resource types, every capability decision, and non-secret settings are required")
			return
		}
		if errors.Is(err, extensions.ErrNotFound) {
			writeAPIError(w, 404, "extension_not_found", "extension not found")
			return
		}
		if err != nil {
			writeAPIError(w, 500, "extension_storage_unavailable", "installation could not be created")
			return
		}
		writeJSON(w, 201, v)
	})
	mux.HandleFunc("GET /extension-installations/{id}", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		v, e := store.GetInstallation(r.PathValue("id"))
		if e != nil || !authorize(actor.UserID, v.OwnerType, v.OwnerID, v.RepositoryIDs) {
			writeAPIError(w, 404, "installation_not_found", "installation not found")
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("POST /extension-installations/{id}/{action}", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:write", false)
		if !ok {
			return
		}
		current, e := store.GetInstallation(r.PathValue("id"))
		if e != nil || !authorize(actor.UserID, current.OwnerType, current.OwnerID, current.RepositoryIDs) {
			writeAPIError(w, 404, "installation_not_found", "installation not found")
			return
		}
		var in struct {
			Version      int                           `json:"version"`
			Installation *extensions.InstallationInput `json:"installation,omitempty"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_installation", "request body must be valid JSON")
			return
		}
		if r.PathValue("action") == "upgrade" && in.Installation != nil && !authorize(actor.UserID, in.Installation.OwnerType, in.Installation.OwnerID, in.Installation.RepositoryIDs) {
			writeAPIError(w, 404, "installation_owner_not_found", "installation owner not found")
			return
		}
		if r.PathValue("action") == "transfer" && in.Installation != nil {
			repositories := in.Installation.RepositoryIDs
			if in.Installation.OwnerType == "repository" {
				repositories = []string{in.Installation.OwnerID}
			}
			if !authorize(actor.UserID, in.Installation.OwnerType, in.Installation.OwnerID, repositories) {
				writeAPIError(w, 404, "installation_owner_not_found", "transfer target not found")
				return
			}
		}
		v, err := store.ChangeInstallation(current.ID, actor.UserID, r.PathValue("action"), in.Version, in.Installation, func(id string) error { _, revokeErr := credentials.Revoke(current.ExtensionID, id); return revokeErr })
		if errors.Is(err, extensions.ErrConflict) {
			writeAPIError(w, 409, "installation_changed", "installation changed; reload before retrying")
			return
		}
		if errors.Is(err, extensions.ErrInvalid) {
			writeAPIError(w, 422, "invalid_installation_change", "lifecycle change is invalid")
			return
		}
		if err != nil {
			writeAPIError(w, 500, "extension_storage_unavailable", "installation could not be changed")
			return
		}
		writeJSON(w, 200, v)
	})
}
