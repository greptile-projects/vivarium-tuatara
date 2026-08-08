package main

import (
	"errors"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/activities"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/incidents"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

func registerIncidentRoutes(mux *http.ServeMux, repos *repositories.Store, store *incidents.Store, deploymentStore *deployments.Store, credentials *auth.Store, activity *activities.Store) {
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
		v, e := store.Create(incidents.Incident{Title: input.Title, Summary: input.Summary, Severity: input.Severity, Status: "investigating", Scopes: input.Scopes, Roles: input.Roles, Source: input.Source, DeclaredBy: actor.UserID})
		if e != nil {
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
		for _, role := range input.Roles {
			if !participantInScopes(role.UserID, current.Scopes) {
				writeAPIError(w, 422, "invalid_incident_role", "response roles must name an affected repository participant")
				return
			}
		}
		v, e := store.Update(current.ID, actor.UserID, input.ExpectedVersion, input.Severity, input.Status, input.Roles, input.Message)
		if e != nil {
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
			Message  string `json:"message"`
			Audience string `json:"audience"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		v, e := store.AddUpdate(current.ID, actor.UserID, input.Message, input.Audience)
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
		v, e := store.Acknowledge(current.ID, r.PathValue("entry_id"), actor.UserID)
		if e != nil {
			writeIncidentError(w, e)
			return
		}
		writeJSON(w, 200, v)
	})
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
