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
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func registerAPIContractApplicationRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, contracts *apicontracts.Store, userStore *users.Store, pulls *pullrequests.Store, releaseStore *releases.Store, issueStore *issues.Store, proposalStore *proposals.Store) {
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

	operationsBase := "/repositories/{id}/api-contracts/{contract_id}/applications/{application_id}/operations"
	access := func(w http.ResponseWriter, r *http.Request, allowAgent bool) (auth.Credential, apicontracts.Application, bool, bool) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return actor, apicontracts.Application{}, false, false
		}
		app, err := contracts.GetApplication(r.PathValue("application_id"))
		if err != nil || app.RepositoryID != r.PathValue("id") || app.ContractID != r.PathValue("contract_id") {
			writeAPIError(w, 404, "application_not_found", "Application not found")
			return actor, app, false, false
		}
		repo, _ := catalog.GetByID(app.RepositoryID)
		collab, _ := catalog.HasCollaborator(actor.UserID, app.RepositoryID)
		producer := actor.AgentID == "" && (actor.UserID == repo.OwnerID || collab)
		consumer := actor.AgentID == "" && actor.UserID == app.OwnerID
		if actor.AgentID != "" && allowAgent && actor.RepositoryID == app.RepositoryID {
			return actor, app, false, true
		}
		if !producer && !consumer {
			writeAPIError(w, 404, "application_not_found", "Application not found")
			return actor, app, false, false
		}
		return actor, app, producer, true
	}
	mux.HandleFunc("POST "+operationsBase+"/observations", func(w http.ResponseWriter, r *http.Request) {
		actor, app, producer, ok := access(w, r, false)
		if !ok {
			return
		}
		var in apicontracts.OperationalObservation
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_operational_observation", "Aggregate operational evidence is required")
			return
		}
		if (producer && actor.UserID != app.OwnerID && in.Visibility == "consumer_only") || (!producer && in.Visibility == "producer_only") {
			writeAPIError(w, 403, "operational_visibility_forbidden", "Evidence can be private only to the publishing side or shared")
			return
		}
		if _, releaseErr := releaseStore.Get(app.RepositoryID, in.ReleaseID); releaseErr != nil {
			writeAPIError(w, 422, "operational_release_invalid", "Evidence must name an exact provider release")
			return
		}
		out, err := contracts.AddOperationalObservation(app, actor.UserID, in)
		writeOperationalRecord(w, out, err, 201)
	})
	mux.HandleFunc("GET "+operationsBase+"/observations", func(w http.ResponseWriter, r *http.Request) {
		actor, app, producer, ok := access(w, r, true)
		if !ok {
			return
		}
		values, err := contracts.ListOperationalObservations(app.ID)
		if err != nil {
			writeAPIError(w, 500, "operational_evidence_unavailable", "Operational evidence could not be read")
			return
		}
		visible := []apicontracts.OperationalObservation{}
		agentEvidence := []string{}
		if actor.AgentID != "" {
			cases, _ := contracts.ListAPIInvestigations(app.ID)
			for _, investigation := range cases {
				if slices.Contains(investigation.InvitedAgentIDs, actor.AgentID) {
					agentEvidence = append(agentEvidence, investigation.ObservationIDs...)
				}
			}
		}
		for _, v := range values {
			if (actor.AgentID != "" && v.Visibility == "shared" && slices.Contains(agentEvidence, v.ID)) || (actor.AgentID == "" && (v.Visibility == "shared" || producer && v.Visibility == "producer_only" || actor.UserID == app.OwnerID && v.Visibility == "consumer_only")) {
				visible = append(visible, v)
			}
		}
		writeJSON(w, 200, map[string]any{"observations": visible})
	})
	mux.HandleFunc("POST "+operationsBase+"/investigations", func(w http.ResponseWriter, r *http.Request) {
		actor, app, producer, ok := access(w, r, false)
		if !ok {
			return
		}
		var in struct {
			Title          string   `json:"title"`
			ObservationIDs []string `json:"observation_ids"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_investigation", "A title and permitted evidence are required")
			return
		}
		values, evidenceErr := contracts.ListOperationalObservations(app.ID)
		if evidenceErr != nil {
			writeAPIError(w, 500, "operational_evidence_unavailable", "Operational evidence could not be read")
			return
		}
		for _, id := range in.ObservationIDs {
			if !slices.ContainsFunc(values, func(v apicontracts.OperationalObservation) bool {
				return v.ID == id && (v.Visibility == "shared" || producer && v.Visibility == "producer_only" || actor.UserID == app.OwnerID && v.Visibility == "consumer_only")
			}) {
				writeAPIError(w, 422, "operational_evidence_inaccessible", "Investigation evidence must be visible to its opener")
				return
			}
		}
		out, err := contracts.CreateAPIInvestigation(app, actor.UserID, in.Title, in.ObservationIDs)
		writeOperationalRecord(w, out, err, 201)
	})
	mux.HandleFunc("GET "+operationsBase+"/investigations", func(w http.ResponseWriter, r *http.Request) {
		actor, app, producer, ok := access(w, r, true)
		if !ok {
			return
		}
		values, err := contracts.ListAPIInvestigations(app.ID)
		if err != nil {
			writeAPIError(w, 500, "investigations_unavailable", "Investigations could not be read")
			return
		}
		if actor.AgentID != "" {
			values = slices.DeleteFunc(values, func(v apicontracts.APIInvestigation) bool { return !slices.Contains(v.InvitedAgentIDs, actor.AgentID) })
		} else {
			observations, readErr := contracts.ListOperationalObservations(app.ID)
			if readErr != nil {
				writeAPIError(w, 500, "operational_evidence_unavailable", "Operational evidence could not be read")
				return
			}
			values = slices.DeleteFunc(values, func(v apicontracts.APIInvestigation) bool {
				return slices.ContainsFunc(v.ObservationIDs, func(id string) bool {
					return !slices.ContainsFunc(observations, func(x apicontracts.OperationalObservation) bool {
						return x.ID == id && (x.Visibility == "shared" || producer && x.Visibility == "producer_only" || actor.UserID == app.OwnerID && x.Visibility == "consumer_only")
					})
				})
			})
		}
		writeJSON(w, 200, map[string]any{"investigations": values})
	})
	investigationBase := operationsBase + "/investigations/{investigation_id}"
	mux.HandleFunc("POST "+investigationBase+"/agents", func(w http.ResponseWriter, r *http.Request) {
		actor, app, _, ok := access(w, r, false)
		if !ok {
			return
		}
		var in struct {
			AgentID string `json:"agent_id"`
		}
		if decodeJSON(r, &in) != nil || strings.TrimSpace(in.AgentID) == "" {
			writeAPIError(w, 400, "invalid_agent_invitation", "A read-only agent identity is required")
			return
		}
		out, err := contracts.UpdateAPIInvestigation(r.PathValue("investigation_id"), func(v *apicontracts.APIInvestigation) error {
			if v.ApplicationID != app.ID {
				return apicontracts.ErrNotFound
			}
			observations, readErr := contracts.ListOperationalObservations(app.ID)
			if readErr != nil {
				return readErr
			}
			for _, id := range v.ObservationIDs {
				if !slices.ContainsFunc(observations, func(x apicontracts.OperationalObservation) bool { return x.ID == id && x.Visibility == "shared" }) {
					return apicontracts.ErrInvalid
				}
			}
			if !slices.Contains(v.InvitedAgentIDs, in.AgentID) {
				v.InvitedAgentIDs = append(v.InvitedAgentIDs, in.AgentID)
			}
			return nil
		})
		_ = actor
		writeOperationalRecord(w, out, err, 200)
	})
	mux.HandleFunc("POST "+investigationBase+"/findings", func(w http.ResponseWriter, r *http.Request) {
		actor, app, producer, ok := access(w, r, true)
		if !ok {
			return
		}
		var in apicontracts.InvestigationFinding
		if decodeJSON(r, &in) != nil || unsafeInvestigationFinding(in) {
			writeAPIError(w, 422, "investigation_finding_unsafe", "A bounded, sanitized, evidence-cited finding is required")
			return
		}
		out, err := contracts.UpdateAPIInvestigation(r.PathValue("investigation_id"), func(v *apicontracts.APIInvestigation) error {
			if v.ApplicationID != app.ID {
				return apicontracts.ErrNotFound
			}
			if actor.AgentID != "" && !slices.Contains(v.InvitedAgentIDs, actor.AgentID) {
				return apicontracts.ErrNotFound
			}
			if actor.AgentID == "" {
				observations, readErr := contracts.ListOperationalObservations(app.ID)
				if readErr != nil {
					return readErr
				}
				for _, id := range v.ObservationIDs {
					if !slices.ContainsFunc(observations, func(x apicontracts.OperationalObservation) bool {
						return x.ID == id && (x.Visibility == "shared" || producer && x.Visibility == "producer_only" || actor.UserID == app.OwnerID && x.Visibility == "consumer_only")
					}) {
						return apicontracts.ErrNotFound
					}
				}
			}
			for _, id := range in.EvidenceIDs {
				if !slices.Contains(v.ObservationIDs, id) {
					return apicontracts.ErrInvalid
				}
			}
			in.ID = apicontracts.NewOperationalID()
			in.ActorType = "human"
			in.ActorID = actor.UserID
			if actor.AgentID != "" {
				in.ActorType = "agent"
				in.ActorID = actor.AgentID
			}
			in.CreatedAt = time.Now().UTC()
			v.Findings = append(v.Findings, in)
			return nil
		})
		writeOperationalRecord(w, out, err, 201)
	})
	mux.HandleFunc("POST "+investigationBase+"/reproductions", func(w http.ResponseWriter, r *http.Request) {
		actor, app, producer, ok := access(w, r, false)
		if !ok {
			return
		}
		var in struct {
			ObservationID string `json:"observation_id"`
			OperationID   string `json:"operation_id"`
			Failure       string `json:"failure"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_reproduction", "A permitted synthetic reproduction is required")
			return
		}
		revision, found := contractRevision(app.RepositoryID, app.ContractID, app.ContractVersion)
		if !found {
			writeAPIError(w, 409, "application_contract_stale", "Registered contract version is unavailable")
			return
		}
		if !slices.Contains(app.ApprovedCapabilities, in.OperationID) || !slices.ContainsFunc(revision.Operations, func(v apicontracts.Operation) bool { return v.ID == in.OperationID }) {
			writeAPIError(w, 403, "capability_not_approved", "The operation is outside this approval")
			return
		}
		status, code := 200, "synthetic_success"
		switch in.Failure {
		case "":
		case "rate_limit":
			status, code = 429, "rate_limited"
		case "timeout":
			status, code = 504, "simulated_timeout"
		case "server_error":
			status, code = 503, "simulated_unavailable"
		default:
			writeAPIError(w, 400, "invalid_failure_simulation", "Failure simulation is unsupported")
			return
		}
		out, err := contracts.UpdateAPIInvestigation(r.PathValue("investigation_id"), func(v *apicontracts.APIInvestigation) error {
			if v.ApplicationID != app.ID || !slices.Contains(v.ObservationIDs, in.ObservationID) {
				return apicontracts.ErrInvalid
			}
			observations, readErr := contracts.ListOperationalObservations(app.ID)
			if readErr != nil {
				return readErr
			}
			for _, id := range v.ObservationIDs {
				if !slices.ContainsFunc(observations, func(x apicontracts.OperationalObservation) bool {
					return x.ID == id && (x.Visibility == "shared" || producer && x.Visibility == "producer_only" || actor.UserID == app.OwnerID && x.Visibility == "consumer_only")
				}) {
					return apicontracts.ErrNotFound
				}
			}
			v.Reproductions = append(v.Reproductions, apicontracts.SandboxReproduction{ID: apicontracts.NewOperationalID(), ObservationID: in.ObservationID, OperationID: in.OperationID, Failure: in.Failure, ResultStatus: status, ResultCode: code, SyntheticOnly: true, PayloadRetained: false, ActorID: actor.UserID, CreatedAt: time.Now().UTC()})
			return nil
		})
		writeOperationalRecord(w, out, err, 201)
	})
	mux.HandleFunc("POST "+investigationBase+"/handoff", func(w http.ResponseWriter, r *http.Request) {
		actor, app, producer, ok := access(w, r, false)
		if !ok {
			return
		}
		var in apicontracts.InvestigationHandoff
		if decodeJSON(r, &in) != nil || !map[string]bool{"issue": true, "proposal": true}[in.Kind] || in.ResourceID == "" || len(in.ResourceID) > 160 || len(in.AcceptanceCriteria) == 0 || len(in.AcceptanceCriteria) > 20 || slices.ContainsFunc(in.AcceptanceCriteria, func(x string) bool { return strings.TrimSpace(x) == "" || len(x) > 500 || unsafeEvidenceText(x) }) {
			writeAPIError(w, 422, "invalid_investigation_handoff", "A confirmed finding and ordinary issue or proposal are required")
			return
		}
		current, currentErr := contracts.GetAPIInvestigation(r.PathValue("investigation_id"))
		observations, observationErr := contracts.ListOperationalObservations(app.ID)
		if observationErr != nil {
			writeAPIError(w, 500, "operational_evidence_unavailable", "Operational evidence could not be read")
			return
		}
		visible := !slices.ContainsFunc(current.ObservationIDs, func(id string) bool {
			return !slices.ContainsFunc(observations, func(x apicontracts.OperationalObservation) bool {
				return x.ID == id && (x.Visibility == "shared" || producer && x.Visibility == "producer_only" || actor.UserID == app.OwnerID && x.Visibility == "consumer_only")
			})
		})
		findingIndex := slices.IndexFunc(current.Findings, func(x apicontracts.InvestigationFinding) bool {
			return x.ID == in.FindingID && x.ActorType == "human" && map[string]bool{"service": true, "contract": true, "client": true, "environment": true}[x.Classification]
		})
		if currentErr != nil || current.ApplicationID != app.ID || findingIndex < 0 || !visible {
			writeAPIError(w, 422, "invalid_investigation_handoff", "A human-confirmed finding is required")
			return
		}
		expectedRepo := app.RepositoryID
		if current.Findings[findingIndex].Classification == "client" {
			consumerRepositoryID, workErr := clientHandoffRepository(contracts, app, in.IntegrationWorkID)
			if workErr != nil {
				writeAPIError(w, 422, "invalid_investigation_handoff", "Client work requires the exact affected integration-work record")
				return
			}
			expectedRepo = consumerRepositoryID
		} else if in.IntegrationWorkID != "" {
			writeAPIError(w, 422, "invalid_investigation_handoff", "Provider work cannot name consumer integration work")
			return
		}
		if in.RepositoryID != expectedRepo {
			writeAPIError(w, 422, "invalid_investigation_handoff", "Work must belong to the classified provider or consumer repository")
			return
		}
		if in.Kind == "issue" {
			if _, resolveErr := issueStore.Get(expectedRepo, in.ResourceID); resolveErr != nil {
				writeAPIError(w, 422, "invalid_investigation_handoff", "Referenced issue does not exist")
				return
			}
		} else if _, resolveErr := proposalStore.Get(expectedRepo, in.ResourceID); resolveErr != nil {
			writeAPIError(w, 422, "invalid_investigation_handoff", "Referenced proposal does not exist")
			return
		}
		out, err := contracts.UpdateAPIInvestigation(r.PathValue("investigation_id"), func(v *apicontracts.APIInvestigation) error {
			if v.ApplicationID != app.ID {
				return apicontracts.ErrNotFound
			}
			if v.Handoff != nil {
				return apicontracts.ErrAlreadyHandedOff
			}
			finding := slices.IndexFunc(v.Findings, func(x apicontracts.InvestigationFinding) bool {
				return x.ID == in.FindingID && x.ActorType == "human" && map[string]bool{"service": true, "contract": true, "client": true, "environment": true}[x.Classification]
			})
			if finding < 0 {
				return apicontracts.ErrInvalid
			}
			if in.RepositoryID != expectedRepo || v.Findings[finding].Classification != current.Findings[findingIndex].Classification {
				return apicontracts.ErrInvalid
			}
			in.CreatedBy, in.CreatedAt = actor.UserID, time.Now().UTC()
			v.Handoff = &in
			return nil
		})
		writeOperationalRecord(w, out, err, 201)
	})
}

func clientHandoffRepository(contracts *apicontracts.Store, app apicontracts.Application, workID string) (string, error) {
	work, err := contracts.GetIntegrationWork(workID)
	if err != nil || work.ApplicationID != app.ID || work.ContractID != app.ContractID || work.ContractVersion != app.ContractVersion {
		return "", apicontracts.ErrInvalid
	}
	return work.ConsumerRepositoryID, nil
}

func unsafeInvestigationFinding(v apicontracts.InvestigationFinding) bool {
	if !map[string]bool{"service": true, "contract": true, "client": true, "environment": true, "inconclusive": true}[v.Classification] || !map[string]bool{"low": true, "medium": true, "high": true}[v.Confidence] || strings.TrimSpace(v.Summary) == "" || strings.TrimSpace(v.Uncertainty) == "" || len(v.EvidenceIDs) == 0 {
		return true
	}
	b, _ := json.Marshal(v)
	return len(b) > 32*1024 || unsafeEvidenceText(string(b))
}
func unsafeEvidenceText(v string) bool {
	v = strings.ToLower(v)
	for _, x := range []string{"authorization:", "bearer ", "password=", "token=", "secret=", "private key", "vva_", "request body", "response body"} {
		if strings.Contains(v, x) {
			return true
		}
	}
	return false
}
func writeOperationalRecord(w http.ResponseWriter, v any, err error, status int) {
	switch {
	case err == nil:
		writeJSON(w, status, v)
	case errors.Is(err, apicontracts.ErrNotFound):
		writeAPIError(w, 404, "investigation_not_found", "Investigation not found")
	case errors.Is(err, apicontracts.ErrAlreadyHandedOff):
		writeAPIError(w, 409, "investigation_already_handed_off", "Investigation already routed")
	case errors.Is(err, apicontracts.ErrInvalid):
		writeAPIError(w, 422, "operational_record_invalid", "Operational evidence is stale, unsafe, inaccessible, or invalid")
	default:
		writeAPIError(w, 500, "operational_record_unavailable", "Operational record could not be persisted")
	}
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
