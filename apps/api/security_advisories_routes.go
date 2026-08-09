package main

import (
	"errors"
	"net/http"
	"slices"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/securityadvisories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func registerSecurityAdvisoryRoutes(mux *http.ServeMux, repos *repositories.Store, identities *users.Store, store *securityadvisories.Store, credentials *auth.Store) {
	maintainer := func(userID string, v securityadvisories.Advisory) bool {
		for _, affected := range v.AffectedRepositories {
			repo, err := repos.GetByID(affected.RepositoryID)
			if err == nil && repo.OwnerID == userID {
				return true
			}
		}
		return false
	}
	visible := func(userID string, v securityadvisories.Advisory) bool {
		return userID == v.ReporterID || slices.Contains(v.ResponseTeam, userID) || maintainer(userID, v)
	}
	require := func(w http.ResponseWriter, r *http.Request) (auth.Credential, securityadvisories.Advisory, bool) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return actor, securityadvisories.Advisory{}, false
		}
		v, err := store.Get(r.PathValue("advisory_id"))
		if err != nil || !visible(actor.UserID, v) {
			writeAPIError(w, http.StatusNotFound, "security_advisory_not_found", "security advisory not found")
			return actor, v, false
		}
		return actor, v, true
	}
	writeStoreError := func(w http.ResponseWriter, err error) {
		switch {
		case errors.Is(err, securityadvisories.ErrConflict):
			writeAPIError(w, 409, "security_advisory_changed", "security advisory changed")
		case errors.Is(err, securityadvisories.ErrInvalid):
			writeAPIError(w, 422, "invalid_security_advisory", "security advisory input is invalid")
		case errors.Is(err, securityadvisories.ErrNotFound):
			writeAPIError(w, 404, "security_advisory_not_found", "security advisory not found")
		default:
			writeAPIError(w, 500, "security_advisory_write_failed", "security advisory could not be saved")
		}
	}

	mux.HandleFunc("GET /security-advisories", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		all, err := store.List()
		if err != nil {
			writeAPIError(w, 500, "security_advisory_read_failed", "security advisories could not be read")
			return
		}
		items := make([]securityadvisories.Advisory, 0)
		for _, item := range all {
			if visible(actor.UserID, item) {
				items = append(items, item)
			}
		}
		page, next, valid := paginate(r, items, func(v securityadvisories.Advisory) string { return v.ID })
		if !valid {
			writeAPIError(w, 400, "invalid_pagination", "limit or after is invalid")
			return
		}
		writeJSON(w, 200, map[string]any{"security_advisories": page, "next_cursor": next})
	})

	mux.HandleFunc("POST /security-advisories", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		var input securityadvisories.Advisory
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		for _, affected := range input.AffectedRepositories {
			repo, err := repos.GetByID(affected.RepositoryID)
			if err != nil {
				writeAPIError(w, 404, "repository_not_found", "repository not found")
				return
			}
			allowed := repo.Visibility == repositories.Public || repo.OwnerID == actor.UserID
			if !allowed {
				allowed, _ = repos.HasCollaborator(actor.UserID, repo.ID)
			}
			if !allowed {
				writeAPIError(w, 404, "repository_not_found", "repository not found")
				return
			}
		}
		input.ReporterID = actor.UserID
		created, err := store.Create(input)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, created)
	})

	mux.HandleFunc("GET /security-advisories/{advisory_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := require(w, r)
		if !ok {
			return
		}
		v, err := store.RecordAccess(r.PathValue("advisory_id"), actor.UserID)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, 200, v)
	})

	mux.HandleFunc("PATCH /security-advisories/{advisory_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, current, ok := require(w, r)
		if !ok {
			return
		}
		if !maintainer(actor.UserID, current) {
			writeAPIError(w, 403, "maintainer_required", "an affected repository owner must triage this report")
			return
		}
		var input struct {
			ExpectedVersion int    `json:"expected_version"`
			Severity        string `json:"severity"`
			EmbargoState    string `json:"embargo_state"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		v, err := store.Triage(current.ID, actor.UserID, input.ExpectedVersion, input.Severity, input.EmbargoState)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, 200, v)
	})

	mux.HandleFunc("POST /security-advisories/{advisory_id}/responders", func(w http.ResponseWriter, r *http.Request) {
		actor, current, ok := require(w, r)
		if !ok {
			return
		}
		if !maintainer(actor.UserID, current) {
			writeAPIError(w, 403, "maintainer_required", "an affected repository owner must invite responders")
			return
		}
		var input struct {
			UserID string `json:"user_id"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		if _, err := identities.Get(input.UserID); err != nil {
			writeAPIError(w, 422, "invalid_responder", "responder does not exist")
			return
		}
		v, err := store.Invite(current.ID, actor.UserID, input.UserID)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, v)
	})

	mux.HandleFunc("POST /security-advisories/{advisory_id}/messages", func(w http.ResponseWriter, r *http.Request) {
		actor, current, ok := require(w, r)
		if !ok {
			return
		}
		var input struct {
			Body string `json:"body"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		v, err := store.AddMessage(current.ID, actor.UserID, input.Body)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, v)
	})
}
