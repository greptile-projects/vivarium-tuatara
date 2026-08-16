package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/incidents"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/protectionplans"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/recoveryoperations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

func registerRecoveryOperationRoutes(mux *http.ServeMux, repos *repositories.Store, credentials *auth.Store, incidentStore *incidents.Store, plans *protectionplans.Store, operations *recoveryoperations.Store) {
	operations.ConfigureValidationResolver(func(operation recoveryoperations.Operation, step recoveryoperations.Step, results []recoveryoperations.ValidationResult) bool {
		incident, err := incidentStore.Get(operation.IncidentID)
		if err != nil {
			return false
		}
		plan, err := plans.Get(operation.RecoveryPoint.PlanID)
		if err != nil {
			return false
		}
		var frozenCapture *protectionplans.Capture
		for i := range plan.Captures {
			if plan.Captures[i].ID == operation.RecoveryPoint.CaptureID && plan.Captures[i].PlanVersion == operation.RecoveryPoint.PlanVersion {
				frozenCapture = &plan.Captures[i]
			}
		}
		if frozenCapture == nil || !frozenCapture.Recoverable || frozenCapture.Validation != "verified" || frozenCapture.ManifestSHA256 != operation.RecoveryPoint.ManifestSHA256 {
			return false
		}
		allowedResources := map[string]bool{step.ResourceID: true}
		for _, resource := range frozenCapture.Resources {
			allowedResources[resource.TargetID] = true
			if resource.EnvironmentID != "" {
				allowedResources[resource.EnvironmentID] = true
			}
			if resource.Revision != "" {
				allowedResources[resource.Revision] = true
			}
		}
		for _, result := range results {
			switch result.Evidence.Kind {
			case "protection_capture":
				if result.Evidence.ResourceID != operation.RecoveryPoint.CaptureID || result.Evidence.SHA256 != operation.RecoveryPoint.ManifestSHA256 {
					return false
				}
				if frozenCapture.ID != result.Evidence.ResourceID || frozenCapture.ManifestSHA256 != result.Evidence.SHA256 {
					return false
				}
			case "incident_evidence":
				if !recoveryIncidentEvidenceMatches(incident, result.Evidence, operation.RepositoryID, allowedResources) {
					return false
				}
			default:
				return false
			}
		}
		return true
	})
	require := func(w http.ResponseWriter, r *http.Request, scope string) (auth.Credential, incidents.Incident, bool) {
		actor, ok := authenticateRequest(w, r, credentials, scope, false)
		if !ok {
			return actor, incidents.Incident{}, false
		}
		incident, err := incidentStore.Get(r.PathValue("incident_id"))
		if err != nil || !recoveryIncidentParticipant(repos, actor.UserID, incident) {
			writeAPIError(w, 404, "incident_not_found", "incident not found")
			return actor, incident, false
		}
		return actor, incident, true
	}
	mux.HandleFunc("GET /incidents/{incident_id}/recoveries", func(w http.ResponseWriter, r *http.Request) {
		_, _, ok := require(w, r, "repositories:read")
		if !ok {
			return
		}
		v, e := operations.ListIncident(r.PathValue("incident_id"))
		if e != nil {
			writeAPIError(w, 500, "recovery_workspace_unavailable", "recovery workspace could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"recoveries": v})
	})
	mux.HandleFunc("POST /incidents/{incident_id}/recoveries", func(w http.ResponseWriter, r *http.Request) {
		actor, incident, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var in struct {
			RepositoryID         string                      `json:"repository_id"`
			PlanID               string                      `json:"plan_id"`
			CaptureID            string                      `json:"capture_id"`
			EstimatedLossMinutes int                         `json:"estimated_loss_minutes"`
			Revision             recoveryoperations.Revision `json:"revision"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a recovery plan and verified point are required")
			return
		}
		if !incidentHasRepository(incident, in.RepositoryID) {
			writeAPIError(w, 422, "recovery_scope_invalid", "repository is not affected by this incident")
			return
		}
		p, e := plans.Get(in.PlanID)
		if e != nil || p.RepositoryID != in.RepositoryID {
			writeAPIError(w, 422, "recovery_point_invalid", "protection plan is not available in the affected repository")
			return
		}
		var capture *protectionplans.Capture
		for i := range p.Captures {
			if p.Captures[i].ID == in.CaptureID {
				capture = &p.Captures[i]
			}
		}
		if capture == nil || !capture.Recoverable || capture.Validation != "verified" || time.Now().After(capture.RetainUntil) {
			writeAPIError(w, 409, "recovery_point_unavailable", "selected recovery point is not verified and recoverable")
			return
		}
		for _, approver := range in.Revision.ApproverIDs {
			if !recoveryIncidentParticipant(repos, approver, incident) {
				writeAPIError(w, 422, "recovery_approver_invalid", "approvers must remain affected repository participants")
				return
			}
		}
		point := recoveryoperations.RecoveryPoint{PlanID: p.ID, PlanVersion: capture.PlanVersion, CaptureID: capture.ID, SourceRevision: capture.SourceRevision, CapturedAt: capture.CapturedAt, EstimatedLossMinutes: in.EstimatedLossMinutes, ManifestSHA256: capture.ManifestSHA256}
		v, e := operations.Create(incident.ID, in.RepositoryID, actor.UserID, point, in.Revision)
		if e != nil {
			writeRecoveryOperationError(w, e)
			return
		}
		w.Header().Set("Location", "/incidents/"+incident.ID+"/recoveries/"+v.ID)
		writeJSON(w, 201, v)
	})
	mux.HandleFunc("GET /incidents/{incident_id}/recoveries/{recovery_id}", func(w http.ResponseWriter, r *http.Request) {
		_, _, ok := require(w, r, "repositories:read")
		if !ok {
			return
		}
		v, e := operations.Get(r.PathValue("recovery_id"))
		if e != nil || v.IncidentID != r.PathValue("incident_id") {
			writeAPIError(w, 404, "recovery_not_found", "recovery operation not found")
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("POST /incidents/{incident_id}/recoveries/{recovery_id}/approvals", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		if !recoveryOperationBelongs(w, operations, r.PathValue("recovery_id"), r.PathValue("incident_id")) {
			return
		}
		if !recoveryOperationActorAllowed(w, repos, operations, actor, r.PathValue("recovery_id")) {
			return
		}
		if actor.AgentID != "" {
			writeAPIError(w, 403, "recovery_authority_required", "recovery approval requires an independently named human approver")
			return
		}
		var in struct {
			ExpectedVersion int    `json:"expected_version"`
			Decision        string `json:"decision"`
			Message         string `json:"message"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an approval decision is required")
			return
		}
		v, e := operations.Approve(r.PathValue("recovery_id"), actor.UserID, in.Decision, in.Message, in.ExpectedVersion)
		writeRecoveryOperation(w, v, e)
	})
	mux.HandleFunc("POST /incidents/{incident_id}/recoveries/{recovery_id}/steps/{step_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		if !recoveryOperationBelongs(w, operations, r.PathValue("recovery_id"), r.PathValue("incident_id")) {
			return
		}
		if !recoveryOperationActorAllowed(w, repos, operations, actor, r.PathValue("recovery_id")) {
			return
		}
		var in struct {
			ExpectedVersion   int                                   `json:"expected_version"`
			Status            string                                `json:"status"`
			Message           string                                `json:"message"`
			ValidationResults []recoveryoperations.ValidationResult `json:"validation_results"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a step transition is required")
			return
		}
		actorID := actor.UserID
		if actor.AgentID != "" {
			actorID = actor.AgentID
		}
		v, e := operations.UpdateStep(r.PathValue("recovery_id"), r.PathValue("step_id"), actorID, in.Status, in.Message, in.ValidationResults, in.ExpectedVersion)
		writeRecoveryOperation(w, v, e)
	})
	mux.HandleFunc("POST /incidents/{incident_id}/recoveries/{recovery_id}/communications", func(w http.ResponseWriter, r *http.Request) {
		actor, incident, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		if !recoveryOperationBelongs(w, operations, r.PathValue("recovery_id"), r.PathValue("incident_id")) {
			return
		}
		if !recoveryOperationControllerAllowed(w, repos, operations, actor, incident, r.PathValue("recovery_id")) {
			return
		}
		var in struct {
			ExpectedVersion int    `json:"expected_version"`
			Audience        string `json:"audience"`
			Message         string `json:"message"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an attributed communication is required")
			return
		}
		v, e := operations.Communicate(r.PathValue("recovery_id"), actor.UserID, in.Audience, in.Message, in.ExpectedVersion)
		writeRecoveryOperation(w, v, e)
	})
	mux.HandleFunc("POST /incidents/{incident_id}/recoveries/{recovery_id}/control", func(w http.ResponseWriter, r *http.Request) {
		actor, incident, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		if !recoveryOperationBelongs(w, operations, r.PathValue("recovery_id"), r.PathValue("incident_id")) {
			return
		}
		if !recoveryOperationControllerAllowed(w, repos, operations, actor, incident, r.PathValue("recovery_id")) {
			return
		}
		var in struct {
			ExpectedVersion int    `json:"expected_version"`
			Action          string `json:"action"`
			Message         string `json:"message"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a recovery control action is required")
			return
		}
		v, e := operations.Control(r.PathValue("recovery_id"), actor.UserID, in.Action, in.Message, in.ExpectedVersion)
		writeRecoveryOperation(w, v, e)
	})
}

func recoveryOperationControllerAllowed(w http.ResponseWriter, repos *repositories.Store, operations *recoveryoperations.Store, actor auth.Credential, incident incidents.Incident, operationID string) bool {
	if !recoveryOperationActorAllowed(w, repos, operations, actor, operationID) {
		return false
	}
	if actor.AgentID != "" {
		writeAPIError(w, 403, "recovery_authority_required", "delegated agents may execute only their exact recovery step")
		return false
	}
	operation, err := operations.Get(operationID)
	if err != nil || len(operation.Revisions) == 0 {
		writeAPIError(w, 404, "recovery_not_found", "recovery operation not found")
		return false
	}
	if operation.Revisions[len(operation.Revisions)-1].CreatedBy == actor.UserID {
		return true
	}
	for _, role := range incident.Roles {
		if role.UserID == actor.UserID {
			return true
		}
	}
	writeAPIError(w, 403, "recovery_authority_required", "recovery-wide control requires an assigned human incident role")
	return false
}

func recoveryOperationActorAllowed(w http.ResponseWriter, repos *repositories.Store, operations *recoveryoperations.Store, actor auth.Credential, operationID string) bool {
	operation, err := operations.Get(operationID)
	if err != nil {
		writeAPIError(w, 404, "recovery_not_found", "recovery operation not found")
		return false
	}
	if actor.RepositoryID != "" && actor.RepositoryID != operation.RepositoryID {
		writeAPIError(w, 404, "recovery_not_found", "recovery operation not found")
		return false
	}
	repository, err := repos.GetByID(operation.RepositoryID)
	if err != nil {
		writeAPIError(w, 404, "recovery_not_found", "recovery operation not found")
		return false
	}
	if actor.AgentID != "" && actor.RepositoryID == operation.RepositoryID && actor.AccessGrantID != "" {
		return true
	}
	collaborator, err := repos.HasCollaborator(actor.UserID, operation.RepositoryID)
	if err == nil && (repository.OwnerID == actor.UserID || collaborator) {
		return true
	}
	writeAPIError(w, 404, "recovery_not_found", "recovery operation not found")
	return false
}

func recoveryIncidentEvidenceMatches(incident incidents.Incident, reference recoveryoperations.EvidenceReference, repositoryID string, allowedResources map[string]bool) bool {
	for _, entry := range incident.Timeline {
		for _, evidence := range entry.Evidence {
			if evidence.RepositoryID != repositoryID || evidence.ResourceID != reference.ResourceID || !allowedResources[evidence.ResourceID] {
				continue
			}
			value, err := json.Marshal(evidence)
			if err != nil {
				return false
			}
			digest := sha256.Sum256(value)
			if hex.EncodeToString(digest[:]) == reference.SHA256 {
				return true
			}
		}
	}
	return false
}

func recoveryOperationBelongs(w http.ResponseWriter, store *recoveryoperations.Store, operationID, incidentID string) bool {
	v, err := store.Get(operationID)
	if err != nil || v.IncidentID != incidentID {
		writeAPIError(w, 404, "recovery_not_found", "recovery operation not found")
		return false
	}
	return true
}
func recoveryIncidentParticipant(repos *repositories.Store, user string, v incidents.Incident) bool {
	for _, s := range v.Scopes {
		repo, e := repos.GetByID(s.RepositoryID)
		if e == nil && repo.OwnerID == user {
			return true
		}
		ok, e := repos.HasCollaborator(user, s.RepositoryID)
		if e == nil && ok {
			return true
		}
	}
	return false
}
func incidentHasRepository(v incidents.Incident, id string) bool {
	for _, s := range v.Scopes {
		if s.RepositoryID == id {
			return true
		}
	}
	return false
}
func writeRecoveryOperation(w http.ResponseWriter, v recoveryoperations.Operation, e error) {
	if e != nil {
		writeRecoveryOperationError(w, e)
		return
	}
	writeJSON(w, 200, v)
}
func writeRecoveryOperationError(w http.ResponseWriter, e error) {
	switch {
	case errors.Is(e, recoveryoperations.ErrNotFound):
		writeAPIError(w, 404, "recovery_not_found", "recovery operation not found")
	case errors.Is(e, recoveryoperations.ErrConflict):
		writeAPIError(w, 409, "recovery_state_changed", "recovery authority, dependencies, approval, or workspace version changed; reload before continuing")
	case errors.Is(e, recoveryoperations.ErrInvalid):
		writeAPIError(w, 400, "invalid_recovery_operation", "define approvals, rollback, delegated owners, validation, and dependency-ordered steps")
	default:
		writeAPIError(w, 500, "recovery_workspace_unavailable", "recovery workspace could not be persisted")
	}
}
