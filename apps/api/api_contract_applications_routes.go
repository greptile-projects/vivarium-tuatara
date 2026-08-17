package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/apicontracts"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func registerAPIContractApplicationRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, contracts *apicontracts.Store, userStore *users.Store, pulls *pullrequests.Store) {
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
		if strings.TrimSpace(in.OwnerID) != in.OwnerID {
			writeAPIError(w, 422, "invalid_application_owner", "successor owner must be an existing human identity")
			return
		}
		if _, err := userStore.Get(in.OwnerID); err != nil {
			writeAPIError(w, 422, "invalid_application_owner", "successor owner must be an existing human identity")
			return
		}
		out, err := contracts.TransferApplication(app.ID, actor.UserID, in.OwnerID)
		writeApplication(w, out, err, 200)
	})
	mux.HandleFunc("POST /repositories/{id}/api-contracts/{contract_id}/applications/{application_id}/sandbox", func(w http.ResponseWriter, r *http.Request) {
		secret := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		candidate, err := contracts.GetApplication(r.PathValue("application_id"))
		if err != nil || candidate.RepositoryID != r.PathValue("id") || candidate.ContractID != r.PathValue("contract_id") {
			writeAPIError(w, 401, "invalid_application_credential", "Application credential is invalid, expired, or revoked")
			return
		}
		revision, found := contractRevision(candidate.RepositoryID, candidate.ContractID, candidate.ContractVersion)
		if !found {
			writeAPIError(w, 409, "application_contract_stale", "The registered contract version is unavailable")
			return
		}
		app, err := contracts.AuthenticateApplicationRequest(candidate.ID, secret, revision.Limits.Requests, revision.Limits.WindowSeconds)
		if errors.Is(err, apicontracts.ErrQuotaExceeded) {
			writeAPIError(w, 429, "sandbox_quota_exceeded", "The application exhausted its contract-defined sandbox request window")
			return
		}
		if err != nil || app.RepositoryID != r.PathValue("id") || app.ContractID != r.PathValue("contract_id") {
			writeAPIError(w, 401, "invalid_application_credential", "Application credential is invalid, expired, or revoked")
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
	workBase := "/repositories/{id}/api-contracts/{contract_id}/applications/{application_id}/integration-work"
	mux.HandleFunc("POST "+workBase, func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:write", false)
		if !ok {
			return
		}
		app, err := contracts.GetApplication(r.PathValue("application_id"))
		if err != nil || app.RepositoryID != r.PathValue("id") || app.ContractID != r.PathValue("contract_id") {
			writeAPIError(w, 404, "application_not_found", "Application not found")
			return
		}
		var in struct {
			ConsumerRepositoryID string `json:"consumer_repository_id"`
			ConsumerRevision     string `json:"consumer_revision"`
			Kind                 string `json:"kind"`
			OwnerType            string `json:"owner_type"`
			OwnerID              string `json:"owner_id"`
			Title                string `json:"title"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_integration_work", "bounded integration work is required")
			return
		}
		if _, _, ok = authorizeRepositoryParticipant(w, r, catalog, credentials, in.ConsumerRepositoryID, "repositories:write"); !ok {
			return
		}
		consumer, openErr := git.Open(in.ConsumerRepositoryID)
		if openErr != nil {
			writeAPIError(w, 404, "consumer_repository_not_found", "Consumer repository not found")
			return
		}
		if _, openErr = consumer.ReadCommit(storage.ObjectID(strings.ToLower(in.ConsumerRevision))); openErr != nil {
			writeAPIError(w, 422, "consumer_revision_invalid", "Consumer revision must name an exact commit")
			return
		}
		if in.OwnerType == "human" {
			if _, e := userStore.Get(in.OwnerID); e != nil {
				writeAPIError(w, 422, "integration_owner_invalid", "Human owner must exist")
				return
			}
		}
		revision, found := contractRevision(app.RepositoryID, app.ContractID, app.ContractVersion)
		if !found {
			writeAPIError(w, 409, "application_contract_stale", "Registered contract version is unavailable")
			return
		}
		preload := apicontracts.IntegrationPreload{DefinitionPath: revision.Source.DefinitionPath, DefinitionCommit: revision.Source.CommitID, Environments: slices.Clone(app.Environments), Operations: slices.Clone(app.ApprovedCapabilities), SyntheticOnly: true, CredentialsIncluded: false}
		for _, link := range revision.Links {
			kind := strings.ToLower(link.Kind)
			if strings.Contains(kind, "sdk") {
				preload.SDKs = append(preload.SDKs, link)
			}
			if strings.Contains(kind, "example") {
				preload.Examples = append(preload.Examples, link)
			}
		}
		out, err := contracts.CreateIntegrationWork(app, actor.UserID, in.ConsumerRepositoryID, strings.ToLower(in.ConsumerRevision), in.Kind, in.OwnerType, in.OwnerID, in.Title, preload)
		writeIntegrationWork(w, out, err, 201)
	})
	mux.HandleFunc("GET "+workBase, func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok || !authenticated {
			return
		}
		app, err := contracts.GetApplication(r.PathValue("application_id"))
		if err != nil || app.ContractID != r.PathValue("contract_id") {
			writeAPIError(w, 404, "application_not_found", "Application not found")
			return
		}
		producer, _ := catalog.GetByID(app.RepositoryID)
		collab, _ := catalog.HasCollaborator(actor.UserID, app.RepositoryID)
		if actor.UserID != app.OwnerID && actor.UserID != producer.OwnerID && !collab {
			writeAPIError(w, 404, "application_not_found", "Application not found")
			return
		}
		out, err := contracts.ListIntegrationWork(app.ID)
		if err != nil {
			writeAPIError(w, 500, "integration_work_unavailable", "Integration work could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"integration_work": out})
	})
	mux.HandleFunc("POST "+workBase+"/{work_id}/candidates", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:write", false)
		if !ok {
			return
		}
		work, workErr := contracts.GetIntegrationWork(r.PathValue("work_id"))
		if workErr != nil || work.ApplicationID != r.PathValue("application_id") || work.ContractID != r.PathValue("contract_id") || work.ProducerRepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "integration_work_not_found", "Integration work not found")
			return
		}
		producer, _ := catalog.GetByID(work.ProducerRepositoryID)
		producerCollaborator, _ := catalog.HasCollaborator(actor.UserID, work.ProducerRepositoryID)
		consumerRepository, consumerRepositoryErr := catalog.GetByID(work.ConsumerRepositoryID)
		consumerCollaborator, _ := catalog.HasCollaborator(actor.UserID, work.ConsumerRepositoryID)
		if consumerRepositoryErr != nil || (actor.UserID != producer.OwnerID && !producerCollaborator && actor.UserID != consumerRepository.OwnerID && !consumerCollaborator) {
			writeAPIError(w, 404, "integration_work_not_found", "Integration work not found")
			return
		}
		var in struct {
			ConsumerRepositoryID  string                             `json:"consumer_repository_id"`
			ProducerPullRequestID string                             `json:"producer_pull_request_id"`
			ConsumerPullRequestID string                             `json:"consumer_pull_request_id"`
			Scenarios             []apicontracts.IntegrationScenario `json:"scenarios"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "integration_candidate_invalid", "Two exact pull candidates and scenarios are required")
			return
		}
		hasProducer, hasConsumer := false, false
		for _, scenario := range in.Scenarios {
			hasProducer = hasProducer || scenario.OwnerSide == "producer"
			hasConsumer = hasConsumer || scenario.OwnerSide == "consumer"
		}
		if !hasProducer || !hasConsumer {
			writeAPIError(w, 422, "integration_scenarios_incomplete", "Candidates require producer conformance and consumer test scenarios")
			return
		}
		if in.ConsumerRepositoryID != work.ConsumerRepositoryID {
			writeAPIError(w, 422, "integration_pull_invalid", "Consumer pull must belong to the frozen consumer repository")
			return
		}
		producerPull, e1 := pulls.Get(r.PathValue("id"), in.ProducerPullRequestID)
		consumerPull, e2 := pulls.Get(in.ConsumerRepositoryID, in.ConsumerPullRequestID)
		if e1 != nil || e2 != nil {
			writeAPIError(w, 422, "integration_pull_invalid", "Both pull requests must be readable")
			return
		}
		candidate := apicontracts.IntegrationCandidate{ProducerPullRequestID: producerPull.ID, ProducerRevision: producerPull.SourceCommitID, ConsumerPullRequestID: consumerPull.ID, ConsumerRevision: consumerPull.SourceCommitID, Scenarios: in.Scenarios}
		out, err := contracts.AddIntegrationCandidate(r.PathValue("work_id"), actor.UserID, candidate)
		writeIntegrationWork(w, out, err, 201)
	})
	mux.HandleFunc("POST "+workBase+"/{work_id}/candidates/{candidate_id}/evidence", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:write", false)
		if !ok {
			return
		}
		work, workErr := contracts.GetIntegrationWork(r.PathValue("work_id"))
		if workErr != nil || work.ApplicationID != r.PathValue("application_id") || work.ContractID != r.PathValue("contract_id") || work.ProducerRepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "integration_work_not_found", "Integration work not found")
			return
		}
		producer, _ := catalog.GetByID(work.ProducerRepositoryID)
		producerCollaborator, _ := catalog.HasCollaborator(actor.UserID, work.ProducerRepositoryID)
		consumer, consumerErr := catalog.GetByID(work.ConsumerRepositoryID)
		consumerCollaborator, _ := catalog.HasCollaborator(actor.UserID, work.ConsumerRepositoryID)
		if consumerErr != nil || (actor.UserID != producer.OwnerID && !producerCollaborator && actor.UserID != consumer.OwnerID && !consumerCollaborator) {
			writeAPIError(w, 404, "integration_work_not_found", "Integration work not found")
			return
		}
		var in apicontracts.IntegrationEvidence
		if decodeJSON(r, &in) != nil || unsafeIntegrationEvidence(in) {
			writeAPIError(w, 422, "integration_evidence_unsafe", "Evidence must be bounded, sanitized, credential-free, and contain artifact metadata only")
			return
		}
		out, err := contracts.AddIntegrationEvidence(r.PathValue("work_id"), r.PathValue("candidate_id"), actor.UserID, in)
		writeIntegrationWork(w, out, err, 201)
	})
}

func unsafeIntegrationEvidence(v apicontracts.IntegrationEvidence) bool {
	b, _ := json.Marshal(v)
	if len(b) > 128*1024 {
		return true
	}
	lower := strings.ToLower(string(b))
	for _, marker := range []string{"authorization:", "bearer ", "password=", "token=", "secret=", "private key", "vva_"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	for _, a := range v.Artifacts {
		if strings.TrimSpace(a.Name) == "" || len(a.Name) > 160 || len(a.SHA256) != 64 || a.Size < 0 || a.Size > 32*1024*1024 {
			return true
		}
	}
	return false
}

func writeIntegrationWork(w http.ResponseWriter, v apicontracts.IntegrationWork, err error, status int) {
	if err == nil {
		writeJSON(w, status, v)
		return
	}
	if errors.Is(err, apicontracts.ErrNotFound) {
		writeAPIError(w, 404, "integration_work_not_found", "Integration work not found")
	} else if errors.Is(err, apicontracts.ErrInvalid) {
		writeAPIError(w, 422, "integration_work_invalid", "Integration work is stale, incomplete, or invalid")
	} else {
		writeAPIError(w, 500, "integration_work_unavailable", "Integration work could not be persisted")
	}
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
