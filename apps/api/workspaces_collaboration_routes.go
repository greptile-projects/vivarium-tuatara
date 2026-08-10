package main

import (
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
)

func registerWorkspaceCollaborationRoutes(mux *http.ServeMux, catalog *repositories.Store, store *workspaces.Store, authStore *auth.Store, organizationStore *organizations.Store) {
	mux.HandleFunc("PUT /workspaces/{workspace_id}/presence", func(w http.ResponseWriter, r *http.Request) {
		item, actor, ok := authorizeWorkspace(w, r, store, catalog, authStore, "repositories:read")
		if !ok {
			return
		}
		var in struct {
			Focus string `json:"focus"`
			Path  string `json:"path"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		if !slices.Contains([]string{"workspace", "file", "terminal", "command", "preview"}, in.Focus) || len(in.Path) > 500 {
			writeAPIError(w, 422, "workspace_presence_invalid", "focus must identify a bounded workspace surface")
			return
		}
		updated, err := store.Join(item.ID, actor.UserID, in.Focus, in.Path)
		if err != nil {
			writeAPIError(w, 500, "workspace_presence_failed", "presence could not be updated")
			return
		}
		writeJSON(w, 200, updated)
	})
	mux.HandleFunc("DELETE /workspaces/{workspace_id}/presence", func(w http.ResponseWriter, r *http.Request) {
		item, actor, ok := authorizeWorkspace(w, r, store, catalog, authStore, "repositories:read")
		if !ok {
			return
		}
		_, err := store.Leave(item.ID, actor.UserID)
		if err != nil {
			writeAPIError(w, 500, "workspace_presence_failed", "presence could not be updated")
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("POST /workspaces/{workspace_id}/messages", func(w http.ResponseWriter, r *http.Request) {
		item, actor, ok := authorizeWorkspace(w, r, store, catalog, authStore, "repositories:write")
		if !ok {
			return
		}
		var in struct {
			Body string `json:"body"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		in.Body = strings.TrimSpace(in.Body)
		if in.Body == "" || len(in.Body) > 4000 {
			writeAPIError(w, 422, "workspace_message_invalid", "message must be 1-4000 characters")
			return
		}
		updated, err := store.AddMessage(item.ID, actor.UserID, in.Body)
		if err != nil {
			writeAPIError(w, 500, "workspace_message_failed", "message could not be saved")
			return
		}
		writeJSON(w, 201, updated.Messages[len(updated.Messages)-1])
	})
	mux.HandleFunc("PUT /workspaces/{workspace_id}/control", func(w http.ResponseWriter, r *http.Request) {
		item, actor, ok := authorizeWorkspace(w, r, store, catalog, authStore, "repositories:write")
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int      `json:"expected_version"`
			PrincipalKind   string   `json:"principal_kind"`
			PrincipalID     string   `json:"principal_id"`
			Mode            string   `json:"mode"`
			Scopes          []string `json:"scopes"`
			ExpiresIn       int      `json:"expires_in"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		release := in.PrincipalKind == "" && in.PrincipalID == "" && in.Mode == "observe" && len(in.Scopes) == 0
		if release {
			updated, err := store.SetControl(item.ID, actor.UserID, "", "", "observe", nil, in.ExpectedVersion, 0)
			if errors.Is(err, workspaces.ErrControl) {
				writeAPIError(w, 409, "workspace_control_changed", "control changed since it was observed")
				return
			}
			if err != nil {
				writeAPIError(w, 422, "workspace_control_invalid", "control could not be released")
				return
			}
			writeJSON(w, 200, updated)
			return
		}
		if !slices.Contains([]string{"human", "approved_agent"}, in.PrincipalKind) || !slices.Contains([]string{"observe", "guide", "edit", "execute"}, in.Mode) || len(in.PrincipalID) != 32 || len(in.Scopes) == 0 || len(in.Scopes) > 3 {
			writeAPIError(w, 422, "workspace_control_invalid", "control requires a human or approved agent, mode, bounded scope, and identity")
			return
		}
		for _, scope := range in.Scopes {
			if !slices.Contains([]string{"files", "commands", "lifecycle"}, scope) {
				writeAPIError(w, 422, "workspace_control_invalid", "scope must be files, commands, or lifecycle")
				return
			}
		}
		if in.PrincipalKind == "human" {
			if in.PrincipalID != actor.UserID {
				collaborator, _ := catalog.HasCollaborator(in.PrincipalID, item.RepositoryID)
				meta, e := catalog.GetByID(item.RepositoryID)
				if e != nil || (in.PrincipalID != meta.OwnerID && !collaborator) {
					writeAPIError(w, 422, "workspace_control_principal_invalid", "human principal must be a current participant")
					return
				}
			}
		} else if !workspaceApprovedAgent(organizationStore, catalog, item.RepositoryID, in.PrincipalID) {
			writeAPIError(w, 422, "workspace_control_principal_invalid", "agent must be approved for the repository organization")
			return
		}
		updated, err := store.SetControl(item.ID, actor.UserID, in.PrincipalKind, in.PrincipalID, in.Mode, in.Scopes, in.ExpectedVersion, in.ExpiresIn)
		if errors.Is(err, workspaces.ErrControl) {
			writeAPIError(w, 409, "workspace_control_changed", "control changed since it was observed")
			return
		}
		if err != nil {
			writeAPIError(w, 422, "workspace_control_invalid", "control could not be changed")
			return
		}
		writeJSON(w, 200, updated)
	})
}

func workspaceApprovedAgent(orgs *organizations.Store, catalog *repositories.Store, repositoryID, agentID string) bool {
	if orgs == nil {
		return false
	}
	meta, err := catalog.GetByID(repositoryID)
	if err != nil || meta.OrganizationID == "" {
		return false
	}
	org, err := orgs.Get(meta.OrganizationID)
	if err != nil {
		return false
	}
	for _, agent := range org.Agents {
		if agent.ID == agentID {
			return true
		}
	}
	return false
}
