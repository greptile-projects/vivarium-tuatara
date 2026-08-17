package main

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/apicontracts"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

func registerAPIContractApplicationRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, contracts *apicontracts.Store) {
	contractRevision := func(repo, id string, version int) (apicontracts.Revision, bool) {
		contract, err := contracts.Get(id)
		if err != nil || contract.RepositoryID != repo {
			return apicontracts.Revision{}, false
		}
		for _, revision := range contract.Revisions {
			if revision.Version == version {
				return revision, true
			}
		}
		return apicontracts.Revision{}, false
	}
	canSee := func(actor auth.Credential, participant bool, app apicontracts.Application) bool {
		return participant || actor.UserID == app.OwnerID
	}

	mux.HandleFunc("POST /repositories/{id}/api-contracts/{contract_id}/applications", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		if _, _, ok = authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		var in struct {
			Name            string   `json:"name"`
			ProjectURL      string   `json:"project_url"`
			ContractVersion int      `json:"contract_version"`
			Environments    []string `json:"environments"`
			Capabilities    []string `json:"capabilities"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a bounded application request is required")
			return
		}
		revision, found := contractRevision(r.PathValue("id"), r.PathValue("contract_id"), in.ContractVersion)
		if !found {
			writeAPIError(w, 404, "api_contract_version_not_found", "API contract version not found")
			return
		}
		for _, environment := range in.Environments {
			if !slices.ContainsFunc(revision.Environments, func(v apicontracts.Environment) bool { return v.ID == environment && v.Availability != "unavailable" }) {
				writeAPIError(w, 422, "invalid_application_environment", "select environments available in the exact contract version")
				return
			}
		}
		allowed := []string{}
		for _, operation := range revision.Operations {
			allowed = append(allowed, operation.ID)
		}
		for _, capability := range in.Capabilities {
			if !slices.Contains(allowed, capability) {
				writeAPIError(w, 422, "invalid_application_capability", "request operation IDs from the exact contract version")
				return
			}
		}
		out, err := contracts.CreateApplication(r.PathValue("id"), r.PathValue("contract_id"), actor.UserID, in.Name, in.ProjectURL, in.ContractVersion, in.Environments, in.Capabilities)
		writeApplication(w, out, err, 201)
	})
	mux.HandleFunc("GET /repositories/{id}/api-contracts/{contract_id}/applications", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok || !authenticated {
			return
		}
		repo, _ := catalog.GetByID(r.PathValue("id"))
		collab, _ := catalog.HasCollaborator(actor.UserID, r.PathValue("id"))
		participant := actor.UserID == repo.OwnerID || collab
		values, err := contracts.ListApplications(r.PathValue("id"), r.PathValue("contract_id"))
		if err != nil {
			writeAPIError(w, 500, "applications_unavailable", "Applications could not be read")
			return
		}
		visible := []apicontracts.Application{}
		for _, v := range values {
			if canSee(actor, participant, v) {
				visible = append(visible, v)
			}
		}
		writeJSON(w, 200, map[string]any{"applications": visible})
	})
	mux.HandleFunc("POST /repositories/{id}/api-contracts/{contract_id}/applications/{application_id}/decision", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		app, err := contracts.GetApplication(r.PathValue("application_id"))
		if err != nil || app.RepositoryID != r.PathValue("id") || app.ContractID != r.PathValue("contract_id") {
			writeAPIError(w, 404, "application_not_found", "Application not found")
			return
		}
		var in struct {
			Status       string    `json:"status"`
			Reason       string    `json:"reason"`
			Capabilities []string  `json:"capabilities"`
			ExpiresAt    time.Time `json:"expires_at"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an attributable approval or denial is required")
			return
		}
		out, err := contracts.DecideApplication(app.ID, actor.UserID, in.Status, in.Reason, in.Capabilities, in.ExpiresAt)
		writeApplication(w, out, err, 200)
	})
	mux.HandleFunc("POST /repositories/{id}/api-contracts/{contract_id}/applications/{application_id}/credentials", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		app, err := contracts.GetApplication(r.PathValue("application_id"))
		if err != nil || app.OwnerID != actor.UserID || app.RepositoryID != r.PathValue("id") || app.ContractID != r.PathValue("contract_id") {
			writeAPIError(w, 404, "application_not_found", "Application not found")
			return
		}
		var in struct {
			LifetimeHours int `json:"lifetime_hours"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "credential lifetime is required")
			return
		}
		out, issued, err := contracts.IssueApplicationCredential(app.ID, actor.UserID, time.Duration(in.LifetimeHours)*time.Hour)
		if err != nil {
			writeApplication(w, out, err, 0)
			return
		}
		writeJSON(w, 201, map[string]any{"application": out, "credential": issued, "warning": "This secret is shown once. Store it outside source control and report exposure to revoke it."})
	})
	revoke := func(event string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
			if !ok {
				return
			}
			app, err := contracts.GetApplication(r.PathValue("application_id"))
			if err != nil || app.OwnerID != actor.UserID || app.RepositoryID != r.PathValue("id") || app.ContractID != r.PathValue("contract_id") {
				writeAPIError(w, 404, "application_not_found", "Application not found")
				return
			}
			out, err := contracts.RevokeApplication(app.ID, actor.UserID, event)
			writeApplication(w, out, err, 200)
		}
	}
	mux.HandleFunc("POST /repositories/{id}/api-contracts/{contract_id}/applications/{application_id}/revoke", revoke("revoked"))
	mux.HandleFunc("POST /repositories/{id}/api-contracts/{contract_id}/applications/{application_id}/exposure", revoke("secret_exposure_reported"))
	mux.HandleFunc("POST /repositories/{id}/api-contracts/{contract_id}/applications/{application_id}/ownership", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		var in struct {
			OwnerID string `json:"owner_id"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a successor owner is required")
			return
		}
		app, err := contracts.GetApplication(r.PathValue("application_id"))
		if err != nil || app.OwnerID != actor.UserID || app.RepositoryID != r.PathValue("id") || app.ContractID != r.PathValue("contract_id") {
			writeAPIError(w, 404, "application_not_found", "Application not found")
			return
		}
		out, err := contracts.TransferApplication(app.ID, actor.UserID, in.OwnerID)
		writeApplication(w, out, err, 200)
	})
	mux.HandleFunc("POST /repositories/{id}/api-contracts/{contract_id}/applications/{application_id}/sandbox", func(w http.ResponseWriter, r *http.Request) {
		secret := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		app, err := contracts.AuthenticateApplication(r.PathValue("application_id"), secret)
		if err != nil || app.RepositoryID != r.PathValue("id") || app.ContractID != r.PathValue("contract_id") {
			writeAPIError(w, 401, "invalid_application_credential", "Application credential is invalid, expired, or revoked")
			return
		}
		revision, found := contractRevision(app.RepositoryID, app.ContractID, app.ContractVersion)
		if !found {
			writeAPIError(w, 409, "application_contract_stale", "The registered contract version is unavailable")
			return
		}
		var in struct {
			OperationID string         `json:"operation_id"`
			Failure     string         `json:"failure"`
			Request     map[string]any `json:"request"`
		}
		if decodeJSON(r, &in) != nil || !slices.Contains(app.ApprovedCapabilities, in.OperationID) {
			writeAPIError(w, 403, "capability_not_approved", "The operation is outside this approval")
			return
		}
		operationIndex := slices.IndexFunc(revision.Operations, func(v apicontracts.Operation) bool { return v.ID == in.OperationID })
		if operationIndex < 0 {
			writeAPIError(w, 409, "application_contract_stale", "The approved operation is unavailable")
			return
		}
		op := revision.Operations[operationIndex]
		status := 200
		body := map[string]any{"synthetic": true, "operation_id": op.ID, "message": "Deterministic synthetic response", "request": in.Request}
		if in.Failure != "" {
			switch in.Failure {
			case "rate_limit":
				status = 429
				body = map[string]any{"code": "rate_limited", "retry_after_seconds": revision.Limits.WindowSeconds}
			case "timeout":
				status = 504
				body = map[string]any{"code": "simulated_timeout", "deterministic": true}
			case "server_error":
				status = 503
				body = map[string]any{"code": "simulated_unavailable", "deterministic": true}
			default:
				writeAPIError(w, 400, "invalid_failure_simulation", "failure must be rate_limit, timeout, or server_error")
				return
			}
		}
		writeJSON(w, 200, map[string]any{"request": map[string]any{"method": op.Method, "path": op.Path, "body": in.Request}, "response": map[string]any{"status": status, "body": body}, "quota": map[string]any{"requests": revision.Limits.Requests, "window_seconds": revision.Limits.WindowSeconds, "synthetic_only": true}})
	})
}

func writeApplication(w http.ResponseWriter, v apicontracts.Application, err error, status int) {
	switch {
	case err == nil:
		writeJSON(w, status, v)
	case errors.Is(err, apicontracts.ErrNotFound):
		writeAPIError(w, 404, "application_not_found", "Application not found")
	case errors.Is(err, apicontracts.ErrConflict):
		writeAPIError(w, 409, "application_conflict", "Application state, approval, or expiry no longer permits this action")
	case errors.Is(err, apicontracts.ErrInvalid):
		writeAPIError(w, 400, "invalid_application", "Application request or decision is invalid")
	default:
		writeAPIError(w, 500, "applications_unavailable", "Application could not be persisted")
	}
}
