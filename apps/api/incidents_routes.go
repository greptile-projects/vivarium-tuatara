package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/activities"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/incidents"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

func registerIncidentRoutes(mux *http.ServeMux, gitStore *storage.Store, repos *repositories.Store, store *incidents.Store, deploymentStore *deployments.Store, releaseStore *releases.Store, pullStore *pullrequests.Store, credentials *auth.Store, activity *activities.Store) {
	participant := func(user string, incident incidents.Incident) bool {
		for _, scope := range incident.Scopes {
			repo, e := repos.GetByID(scope.RepositoryID)
			if e == nil && repo.OwnerID == user {
				return true
			}
			ok, e := repos.HasCollaborator(user, scope.RepositoryID)
			if e == nil && ok {
				return true
			}
		}
		return false
	}
	participantInScopes := func(user string, scopes []incidents.Scope) bool {
		return participant(user, incidents.Incident{Scopes: scopes})
	}
	require := func(w http.ResponseWriter, r *http.Request, scope string) (auth.Credential, incidents.Incident, bool) {
		actor, ok := authenticateRequest(w, r, credentials, scope, false)
		if !ok {
			return actor, incidents.Incident{}, false
		}
		v, e := store.Get(r.PathValue("incident_id"))
		if e != nil || !participant(actor.UserID, v) {
			writeAPIError(w, 404, "incident_not_found", "incident not found")
			return actor, v, false
		}
		return actor, v, true
	}
	mutate := func(current incidents.Incident, actorID string, roles []incidents.Role, fn func() error) error {
		repositoryIDs := make([]string, 0, len(current.Scopes))
		for _, scope := range current.Scopes {
			repositoryIDs = append(repositoryIDs, scope.RepositoryID)
		}
		roleIDs := make([]string, 0, len(roles))
		for _, role := range roles {
			roleIDs = append(roleIDs, role.UserID)
		}
		return repos.WithIncidentAuthorization(actorID, repositoryIDs, roleIDs, fn)
	}
	mux.HandleFunc("GET /incidents", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		all, e := store.List()
		if e != nil {
			writeAPIError(w, 500, "incident_read_failed", "incidents could not be read")
			return
		}
		visible := all[:0]
		for _, v := range all {
			if participant(actor.UserID, v) {
				visible = append(visible, v)
			}
		}
		page, next, valid := paginate(r, visible, func(v incidents.Incident) string { return v.ID })
		if !valid {
			writeAPIError(w, 400, "invalid_pagination", "limit or after is invalid")
			return
		}
		writeJSON(w, 200, map[string]any{"incidents": page, "next_cursor": next})
	})
	mux.HandleFunc("POST /incidents", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:write", false)
		if !ok {
			return
		}
		var input struct {
			Title    string            `json:"title"`
			Summary  string            `json:"summary"`
			Severity string            `json:"severity"`
			Scopes   []incidents.Scope `json:"scopes"`
			Roles    []incidents.Role  `json:"roles"`
			Source   *incidents.Source `json:"source"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		for _, scope := range input.Scopes {
			_, _, ok := authorizeRepositoryParticipant(w, r, repos, credentials, scope.RepositoryID, "repositories:write")
			if !ok {
				return
			}
			if deploymentStore != nil {
				for _, id := range scope.EnvironmentIDs {
					if _, e := deploymentStore.GetEnvironment(scope.RepositoryID, id); e != nil {
						writeAPIError(w, 422, "invalid_incident_scope", "affected environment does not belong to its repository")
						return
					}
				}
			}
		}
		if input.Source != nil {
			inScope := false
			for _, scope := range input.Scopes {
				if scope.RepositoryID == input.Source.RepositoryID {
					inScope = true
				}
			}
			if !inScope {
				writeAPIError(w, 422, "invalid_incident_source", "deployment source must be an affected repository")
				return
			}
			if deploymentStore == nil {
				writeAPIError(w, 422, "invalid_incident_source", "deployment source is unavailable")
				return
			}
			p, e := deploymentStore.GetPromotion(input.Source.RepositoryID, input.Source.DeploymentID)
			if e != nil {
				writeAPIError(w, 422, "invalid_incident_source", "deployment not found")
				return
			}
			matched := input.Source.Signal == ""
			for _, x := range p.Evidence {
				if x.Stage == input.Source.Stage && x.Signal == input.Source.Signal {
					matched = true
				}
			}
			if !matched {
				writeAPIError(w, 422, "invalid_incident_source", "health signal evidence was not found")
				return
			}
		}
		for _, role := range input.Roles {
			if !participantInScopes(role.UserID, input.Scopes) {
				writeAPIError(w, 422, "invalid_incident_role", "response roles must name an affected repository participant")
				return
			}
		}
		repositoryIDs, roleIDs := make([]string, 0, len(input.Scopes)), make([]string, 0, len(input.Roles))
		for _, scope := range input.Scopes {
			repositoryIDs = append(repositoryIDs, scope.RepositoryID)
		}
		for _, role := range input.Roles {
			roleIDs = append(roleIDs, role.UserID)
		}
		var v incidents.Incident
		e := repos.WithIncidentDeclarationAuthorization(actor.UserID, repositoryIDs, roleIDs, func() error {
			var mutationErr error
			v, mutationErr = store.Create(incidents.Incident{Title: input.Title, Summary: input.Summary, Severity: input.Severity, Status: "investigating", Scopes: input.Scopes, Roles: input.Roles, Source: input.Source, DeclaredBy: actor.UserID})
			return mutationErr
		})
		if e != nil {
			if errors.Is(e, repositories.ErrInvalidCollaborator) || errors.Is(e, repositories.ErrNotFound) {
				writeAPIError(w, 404, "repository_not_found", "repository not found")
				return
			}
			writeIncidentError(w, e)
			return
		}
		for _, scope := range v.Scopes {
			recordActivity(activity, repos, activities.Event{Kind: "incident.declared", ActorID: actor.UserID, RepositoryID: scope.RepositoryID, ResourceType: "incident", ResourceID: v.ID, ResourceTitle: v.Title})
		}
		w.Header().Set("Location", "/incidents/"+v.ID)
		writeJSON(w, 201, v)
	})
	mux.HandleFunc("GET /incidents/{incident_id}", func(w http.ResponseWriter, r *http.Request) {
		_, v, ok := require(w, r, "repositories:read")
		if ok {
			writeJSON(w, 200, v)
		}
	})
	mux.HandleFunc("PATCH /incidents/{incident_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, current, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var input struct {
			ExpectedVersion int              `json:"expected_version"`
			Severity        string           `json:"severity"`
			Status          string           `json:"status"`
			Roles           []incidents.Role `json:"roles"`
			Message         string           `json:"message"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		var v incidents.Incident
		e := mutate(current, actor.UserID, input.Roles, func() error {
			var mutationErr error
			v, mutationErr = store.Update(current.ID, actor.UserID, input.ExpectedVersion, input.Severity, input.Status, input.Roles, input.Message)
			return mutationErr
		})
		if e != nil {
			if errors.Is(e, repositories.ErrInvalidCollaborator) || errors.Is(e, repositories.ErrNotFound) {
				writeAPIError(w, 404, "incident_not_found", "incident not found")
				return
			}
			writeIncidentError(w, e)
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("POST /incidents/{incident_id}/updates", func(w http.ResponseWriter, r *http.Request) {
		actor, current, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var input struct {
			OperationID string `json:"operation_id"`
			Message     string `json:"message"`
			Audience    string `json:"audience"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		var v incidents.Incident
		e := mutate(current, actor.UserID, current.Roles, func() error {
			var mutationErr error
			v, mutationErr = store.AddUpdate(current.ID, input.OperationID, actor.UserID, input.Message, input.Audience)
			return mutationErr
		})
		if e != nil {
			if errors.Is(e, repositories.ErrInvalidCollaborator) || errors.Is(e, repositories.ErrNotFound) {
				writeAPIError(w, 404, "incident_not_found", "incident not found")
				return
			}
			writeIncidentError(w, e)
			return
		}
		writeJSON(w, 201, v)
	})
	mux.HandleFunc("POST /incidents/{incident_id}/findings", func(w http.ResponseWriter, r *http.Request) {
		actor, current, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var input struct {
			OperationID string               `json:"operation_id"`
			Kind        string               `json:"kind"`
			Message     string               `json:"message"`
			Audience    string               `json:"audience"`
			Evidence    []incidents.Evidence `json:"evidence"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		for i := range input.Evidence {
			source := &input.Evidence[i]
			inScope := false
			for _, scope := range current.Scopes {
				if scope.RepositoryID == source.RepositoryID {
					inScope = true
					break
				}
			}
			if !inScope {
				writeAPIError(w, 422, "invalid_incident_evidence", "evidence must belong to an affected repository")
				return
			}
			label, valid := incidentEvidenceLabel(gitStore, store, deploymentStore, releaseStore, pullStore, *source)
			if !valid {
				writeAPIError(w, 422, "invalid_incident_evidence", "evidence source is unavailable or invalid")
				return
			}
			source.Label = label
		}
		var v incidents.Incident
		e := mutate(current, actor.UserID, current.Roles, func() error {
			var mutationErr error
			v, mutationErr = store.AddFinding(current.ID, input.OperationID, actor.UserID, input.Kind, input.Message, input.Audience, input.Evidence)
			return mutationErr
		})
		if e != nil {
			writeIncidentError(w, e)
			return
		}
		writeJSON(w, 201, v)
	})
	mux.HandleFunc("POST /incidents/{incident_id}/timeline/{entry_id}/acknowledgements", func(w http.ResponseWriter, r *http.Request) {
		actor, current, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var v incidents.Incident
		e := mutate(current, actor.UserID, current.Roles, func() error {
			var mutationErr error
			v, mutationErr = store.Acknowledge(current.ID, r.PathValue("entry_id"), actor.UserID)
			return mutationErr
		})
		if e != nil {
			if errors.Is(e, repositories.ErrInvalidCollaborator) || errors.Is(e, repositories.ErrNotFound) {
				writeAPIError(w, 404, "incident_not_found", "incident not found")
				return
			}
			writeIncidentError(w, e)
			return
		}
		writeJSON(w, 200, v)
	})
	// Investigation credentials carry only this purpose-built scope. They can
	// read their frozen packet and append diagnostic evidence, but cannot call
	// repository mutation, deployment, credential, or secret-management APIs.
	mux.HandleFunc("POST /incidents/{incident_id}/investigations", func(w http.ResponseWriter, r *http.Request) {
		actor, current, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var input struct {
			Mandate   string               `json:"mandate"`
			Evidence  []incidents.Evidence `json:"evidence"`
			Revisions []incidents.Revision `json:"revisions"`
			ExpiresIn int64                `json:"expires_in"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		if input.ExpiresIn == 0 {
			input.ExpiresIn = 3600
		}
		if input.ExpiresIn < 300 || input.ExpiresIn > 86400 {
			writeAPIError(w, 422, "invalid_investigation", "investigation expiry must be between 5 minutes and 24 hours")
			return
		}
		inScope := func(id string) bool {
			for _, x := range current.Scopes {
				if x.RepositoryID == id {
					return true
				}
			}
			return false
		}
		for i := range input.Evidence {
			if !inScope(input.Evidence[i].RepositoryID) {
				writeAPIError(w, 422, "invalid_investigation_evidence", "selected evidence must belong to the incident")
				return
			}
			label, valid := incidentEvidenceLabel(gitStore, store, deploymentStore, releaseStore, pullStore, input.Evidence[i])
			if !valid {
				writeAPIError(w, 422, "invalid_investigation_evidence", "selected evidence is unavailable")
				return
			}
			input.Evidence[i].Label = label
		}
		for i := range input.Revisions {
			revision := &input.Revisions[i]
			if !inScope(revision.RepositoryID) {
				writeAPIError(w, 422, "invalid_investigation_revision", "selected revision must belong to the incident")
				return
			}
			repo, e := gitStore.Open(revision.RepositoryID)
			if e != nil {
				writeAPIError(w, 422, "invalid_investigation_revision", "selected revision is unavailable")
				return
			}
			if _, e = repo.ReadCommit(storage.ObjectID(revision.CommitID)); e != nil {
				writeAPIError(w, 422, "invalid_investigation_revision", "selected revision must be a verified commit")
				return
			}
			revision.Label = "commit " + revision.CommitID[:12]
		}
		bytes := make([]byte, 16)
		if _, e := rand.Read(bytes); e != nil {
			writeAPIError(w, 500, "investigation_start_failed", "investigation could not be started")
			return
		}
		agentID := hex.EncodeToString(bytes)
		issued, e := credentials.Issue(actor.UserID, auth.API, "Incident investigation", []string{"incidents:investigate"}, time.Duration(input.ExpiresIn)*time.Second)
		if e != nil {
			writeAPIError(w, 500, "investigation_start_failed", "read-only agent access could not be issued")
			return
		}
		var v incidents.Incident
		var investigation incidents.Investigation
		e = mutate(current, actor.UserID, current.Roles, func() error {
			var x error
			v, investigation, x = store.StartInvestigation(current.ID, actor.UserID, agentID, issued.ID, input.Mandate, input.Evidence, input.Revisions)
			return x
		})
		if e != nil {
			_, _ = credentials.Revoke(actor.UserID, issued.ID)
			writeIncidentError(w, e)
			return
		}
		w.Header().Set("Location", r.URL.Path+"/"+investigation.ID)
		writeJSON(w, 201, map[string]any{"incident": v, "investigation": investigation, "credential": issued})
	})
	mux.HandleFunc("GET /incidents/{incident_id}/investigations/{investigation_id}", func(w http.ResponseWriter, r *http.Request) {
		credential, ok := authenticateRequest(w, r, credentials, "incidents:investigate", false)
		if !ok {
			return
		}
		_, x, e := store.Investigation(r.PathValue("incident_id"), r.PathValue("investigation_id"))
		if e != nil || x.CredentialID != credential.ID || x.State == "cancelled" {
			writeAPIError(w, 404, "investigation_not_found", "investigation not found")
			return
		}
		context := make([]any, 0, len(x.Evidence))
		for _, source := range x.Evidence {
			var resource any
			switch source.Kind {
			case "log", "health_signal", "deployment":
				if deploymentStore != nil {
					resource, _ = deploymentStore.GetPromotion(source.RepositoryID, source.ResourceID)
				}
			case "release":
				if releaseStore != nil {
					resource, _ = releaseStore.Get(source.RepositoryID, source.ResourceID)
				}
			case "pull_request":
				if pullStore != nil {
					resource, _ = pullStore.Get(source.RepositoryID, source.ResourceID)
				}
			case "incident":
				resource, _ = store.Get(source.ResourceID)
			case "commit":
				if repository, openErr := gitStore.Open(source.RepositoryID); openErr == nil {
					resource, _ = repository.ReadCommit(storage.ObjectID(source.ResourceID))
				}
			}
			context = append(context, map[string]any{"selection": source, "resource": resource})
		}
		writeJSON(w, 200, map[string]any{"investigation": x, "operational_context": context})
	})
	mux.HandleFunc("POST /incidents/{incident_id}/investigations/{investigation_id}/events", func(w http.ResponseWriter, r *http.Request) {
		credential, ok := authenticateRequest(w, r, credentials, "incidents:investigate", false)
		if !ok {
			return
		}
		var input struct {
			Kind    string `json:"kind"`
			Message string `json:"message"`
			Tool    string `json:"tool"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_agent_event", "agent event is invalid")
			return
		}
		v, e := store.AddInvestigationEvent(r.PathValue("incident_id"), r.PathValue("investigation_id"), credential.ID, strings.TrimSpace(input.Kind), input.Message, input.Tool)
		if errors.Is(e, incidents.ErrConflict) {
			writeAPIError(w, 409, "investigation_inactive", "investigation is paused or cancelled")
			return
		}
		if e != nil {
			writeIncidentError(w, e)
			return
		}
		writeJSON(w, 201, v)
	})
	mux.HandleFunc("POST /incidents/{incident_id}/investigations/{investigation_id}/controls", func(w http.ResponseWriter, r *http.Request) {
		actor, current, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var input struct {
			Action  string `json:"action"`
			Message string `json:"message"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_investigation_control", "control is invalid")
			return
		}
		var v incidents.Incident
		var x incidents.Investigation
		e := mutate(current, actor.UserID, current.Roles, func() error {
			var z error
			v, x, z = store.ControlInvestigation(current.ID, r.PathValue("investigation_id"), actor.UserID, strings.TrimSpace(input.Action), input.Message)
			return z
		})
		if errors.Is(e, incidents.ErrConflict) {
			writeAPIError(w, 409, "invalid_investigation_transition", "control is invalid for the current state")
			return
		}
		if e != nil {
			writeIncidentError(w, e)
			return
		}
		if input.Action == "cancel" {
			_, _ = credentials.Revoke(x.InitiatorID, x.CredentialID)
		}
		writeJSON(w, 201, v)
	})
}

func incidentEvidenceLabel(gitStore *storage.Store, incidentStore *incidents.Store, deploymentStore *deployments.Store, releaseStore *releases.Store, pullStore *pullrequests.Store, source incidents.Evidence) (string, bool) {
	switch source.Kind {
	case "log", "health_signal", "deployment":
		if deploymentStore == nil {
			return "", false
		}
		v, e := deploymentStore.GetPromotion(source.RepositoryID, source.ResourceID)
		if e != nil {
			return "", false
		}
		if source.Kind == "health_signal" {
			matched := false
			for _, item := range v.Evidence {
				if source.Query == item.Stage+"/"+item.Signal && source.WindowStart != nil && source.WindowEnd != nil && !item.CreatedAt.Before(*source.WindowStart) && !item.CreatedAt.After(*source.WindowEnd) {
					matched = true
				}
			}
			if !matched {
				return "", false
			}
		}
		if source.Kind == "log" {
			matched := false
			for _, item := range v.Events {
				if source.WindowStart != nil && source.WindowEnd != nil && !item.CreatedAt.Before(*source.WindowStart) && !item.CreatedAt.After(*source.WindowEnd) {
					matched = true
				}
			}
			if !matched {
				return "", false
			}
		}
		return "deployment " + v.ID[:8] + " · " + v.State, true
	case "release":
		if releaseStore == nil {
			return "", false
		}
		v, e := releaseStore.Get(source.RepositoryID, source.ResourceID)
		if e != nil {
			return "", false
		}
		return "release " + v.Version + " · " + v.CommitID[:12], true
	case "pull_request":
		if pullStore == nil {
			return "", false
		}
		v, e := pullStore.Get(source.RepositoryID, source.ResourceID)
		if e != nil {
			return "", false
		}
		return "pull request · " + v.Title, true
	case "commit":
		if gitStore == nil {
			return "", false
		}
		repo, e := gitStore.Open(source.RepositoryID)
		if e != nil {
			return "", false
		}
		obj, e := repo.ReadObject(storage.ObjectID(source.ResourceID))
		if e != nil || obj.Type != storage.CommitObject {
			return "", false
		}
		return "commit " + source.ResourceID[:12], true
	case "incident":
		v, e := incidentStore.Get(source.ResourceID)
		if e != nil {
			return "", false
		}
		found := false
		for _, scope := range v.Scopes {
			if scope.RepositoryID == source.RepositoryID {
				found = true
			}
		}
		if !found {
			return "", false
		}
		return "incident · " + v.Title, true
	}
	return "", false
}

func writeIncidentError(w http.ResponseWriter, e error) {
	if errors.Is(e, incidents.ErrNotFound) {
		writeAPIError(w, 404, "incident_not_found", "incident not found")
	} else if errors.Is(e, incidents.ErrConflict) {
		writeAPIError(w, 409, "incident_changed", "incident changed; reload before updating")
	} else if errors.Is(e, incidents.ErrInvalid) {
		writeAPIError(w, 422, "invalid_incident", "incident content is invalid")
	} else {
		writeAPIError(w, 500, "incident_write_failed", "incident could not be saved")
	}
}
