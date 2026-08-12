package main

import (
	"errors"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/extensions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

func registerExtensionContributionRoutes(mux *http.ServeMux, store *extensions.Store, credentials *auth.Store, repos *repositories.Store, pulls *pullrequests.Store) {
	resource := func(w http.ResponseWriter, r *http.Request, revision string) (auth.Credential, bool) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return actor, false
		}
		if _, e := repos.Get(actor.UserID, r.PathValue("id")); e != nil {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return actor, false
		}
		if r.PathValue("resource_type") != "pull_requests" {
			writeAPIError(w, 422, "unsupported_extension_resource", "only pull_requests are currently supported")
			return actor, false
		}
		pull, e := pulls.Get(r.PathValue("id"), r.PathValue("resource_id"))
		if e != nil {
			writeAPIError(w, 404, "resource_not_found", "resource not found")
			return actor, false
		}
		if revision != "" && pull.SourceCommitID != revision {
			writeAPIError(w, 409, "resource_revision_changed", "resource changed; publish or invoke against the current revision")
			return actor, false
		}
		return actor, true
	}
	mux.HandleFunc("POST /extension-contributions", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "extensions:contribute", false)
		if !ok {
			return
		}
		var in extensions.ContributionInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_contribution", "request body must be valid JSON")
			return
		}
		installationID := r.Header.Get("Vivarium-Installation-ID")
		v, e := store.GetInstallation(installationID)
		if e != nil || v.ExtensionID != actor.UserID {
			writeAPIError(w, 403, "installation_credential_invalid", "credential is not derived from this installation")
			return
		}
		found := false
		for _, id := range v.DerivedCredentialIDs {
			found = found || id == actor.ID
		}
		if !found {
			writeAPIError(w, 403, "installation_credential_invalid", "credential is not live for this installation")
			return
		}
		if in.ResourceType != "pull_requests" {
			writeAPIError(w, 422, "unsupported_extension_resource", "only pull_requests are currently supported")
			return
		}
		pull, e := pulls.Get(in.RepositoryID, in.ResourceID)
		if e != nil || pull.SourceCommitID != in.Revision {
			writeAPIError(w, 409, "resource_revision_changed", "resource does not exist at the supplied current revision")
			return
		}
		created, e := store.PublishContribution(v, in)
		if errors.Is(e, extensions.ErrConflict) {
			writeAPIError(w, 409, "idempotency_conflict", "idempotency key was already used with different content")
			return
		}
		if errors.Is(e, extensions.ErrLimit) {
			writeAPIError(w, 429, "extension_budget_exceeded", "installation contribution rate or payload budget exceeded")
			return
		}
		if e != nil {
			writeAPIError(w, 422, "invalid_contribution", "contribution exceeds installation authority or validation limits")
			return
		}
		writeJSON(w, 201, created)
	})
	mux.HandleFunc("GET /repositories/{id}/extension-contributions/{resource_type}/{resource_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := resource(w, r, ""); !ok {
			return
		}
		items, e := store.ListContributions(r.PathValue("id"), r.PathValue("resource_type"), r.PathValue("resource_id"))
		if e != nil {
			writeAPIError(w, 500, "extension_storage_unavailable", "contributions unavailable")
			return
		}
		writeJSON(w, 200, map[string]any{"contributions": items})
	})
	mux.HandleFunc("POST /repositories/{id}/extension-contributions/{resource_type}/{resource_id}/{contribution_id}/actions/{action_id}", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Revision string            `json:"revision"`
			Inputs   map[string]string `json:"inputs"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_action", "request body must be valid JSON")
			return
		}
		actor, ok := resource(w, r, in.Revision)
		if !ok {
			return
		}
		v, e := store.Invoke(r.PathValue("id"), r.PathValue("resource_type"), r.PathValue("resource_id"), r.PathValue("contribution_id"), r.PathValue("action_id"), actor.UserID, in.Revision, in.Inputs)
		if errors.Is(e, extensions.ErrConflict) {
			writeAPIError(w, 409, "resource_revision_changed", "action was declared for a different resource revision")
			return
		}
		if errors.Is(e, extensions.ErrLimit) {
			writeAPIError(w, 429, "action_budget_exceeded", "action invocation budget exceeded")
			return
		}
		if e != nil {
			writeAPIError(w, 422, "invalid_action", "action or declared inputs are invalid")
			return
		}
		writeJSON(w, 202, v)
	})
}
