package main

import (
	"errors"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
)

func registerWorkspaceGovernanceRoutes(mux *http.ServeMux, repos *repositories.Store, store *workspaces.Store, credentials *auth.Store, orgs *organizations.Store) {
	policy := func(scope string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			actor, ok := authenticateRequest(w, r, credentials, "repositories:write", false)
			if !ok {
				return
			}
			id := r.PathValue(scope + "_id")
			if scope == "repository" {
				repo, err := repos.GetByID(id)
				if err != nil || repo.OwnerID != actor.UserID {
					writeAPIError(w, 404, "repository_not_found", "repository not found")
					return
				}
			} else {
				org, err := orgs.Get(id)
				if err != nil || !organizations.HasRole(org, actor.UserID, "owner") {
					writeAPIError(w, 404, "organization_not_found", "organization not found")
					return
				}
			}
			if r.Method == "GET" {
				p, err := store.GetPolicy(scope, id)
				if err != nil {
					writeAPIError(w, 500, "workspace_policy_unavailable", "workspace policy could not be read")
					return
				}
				writeJSON(w, 200, p)
				return
			}
			var in struct {
				ExpectedVersion int     `json:"expected_version"`
				MaxCPUs         float64 `json:"max_cpus"`
				MaxMemoryMB     int     `json:"max_memory_mb"`
				MaxStorageMB    int     `json:"max_storage_mb"`
				Network         string  `json:"network"`
				IdleMinutes     int     `json:"idle_minutes"`
				MaxRuntimeHours int     `json:"max_runtime_hours"`
				RetentionHours  int     `json:"retention_hours"`
				Sharing         string  `json:"sharing"`
				AgentExecution  bool    `json:"agent_execution"`
			}
			if decodeJSON(r, &in) != nil {
				writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
				return
			}
			p, err := store.PutPolicy(scope, id, actor.UserID, workspaces.Policy{MaxCPUs: in.MaxCPUs, MaxMemoryMB: in.MaxMemoryMB, MaxStorageMB: in.MaxStorageMB, Network: in.Network, IdleMinutes: in.IdleMinutes, MaxRuntimeHours: in.MaxRuntimeHours, RetentionHours: in.RetentionHours, Sharing: in.Sharing, AgentExecution: in.AgentExecution}, in.ExpectedVersion)
			if errors.Is(err, workspaces.ErrConflict) {
				writeAPIError(w, 409, "workspace_policy_changed", "workspace policy changed since it was observed")
				return
			}
			if err != nil {
				writeAPIError(w, 422, "workspace_policy_invalid", "workspace policy values are invalid")
				return
			}
			writeJSON(w, 200, p)
		}
	}
	mux.HandleFunc("GET /repositories/{repository_id}/workspace-policy", policy("repository"))
	mux.HandleFunc("PUT /repositories/{repository_id}/workspace-policy", policy("repository"))
	mux.HandleFunc("GET /organizations/{organization_id}/workspace-policy", policy("organization"))
	mux.HandleFunc("PUT /organizations/{organization_id}/workspace-policy", policy("organization"))
	mux.HandleFunc("GET /repositories/{repository_id}/workspace-usage", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		repo, err := repos.GetByID(r.PathValue("repository_id"))
		if err != nil || repo.OwnerID != actor.UserID {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return
		}
		all, err := store.ListAll()
		if err != nil {
			writeAPIError(w, 500, "workspace_usage_unavailable", "workspace usage could not be read")
			return
		}
		items := []workspaces.Consumption{}
		now := time.Now().UTC()
		for _, v := range all {
			if v.RepositoryID == repo.ID {
				items = append(items, workspaces.Usage(v, now))
			}
		}
		writeJSON(w, 200, map[string]any{"items": items})
	})
	mux.HandleFunc("POST /workspaces/{workspace_id}/expiry", func(w http.ResponseWriter, r *http.Request) {
		item, actor, ok := workspaceOwner(w, r, store, repos, credentials)
		if !ok {
			return
		}
		var in struct {
			ExpiresAt time.Time `json:"expires_at"`
			Reason    string    `json:"reason"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		updated, err := store.AnnounceExpiry(item.ID, actor.UserID, in.ExpiresAt, strings.TrimSpace(in.Reason))
		if err != nil {
			writeAPIError(w, 422, "workspace_expiry_invalid", "expiry must be in the future")
			return
		}
		writeJSON(w, 200, updated)
	})
	mux.HandleFunc("POST /workspaces/{workspace_id}/stop", func(w http.ResponseWriter, r *http.Request) {
		item, actor, ok := workspaceOwner(w, r, store, repos, credentials)
		if !ok {
			return
		}
		var in struct {
			Reason string `json:"reason"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		if err := removeWorkspaceRuntime(item.ID); err != nil {
			writeAPIError(w, 503, "workspace_teardown_failed", "workspace compute could not be removed; retry the stop")
			return
		}
		updated, err := store.Stop(item.ID, actor.UserID, strings.TrimSpace(in.Reason), "stopped")
		if err != nil {
			writeAPIError(w, 409, "workspace_stop_failed", "workspace could not be stopped")
			return
		}
		writeJSON(w, 200, updated)
	})
	mux.HandleFunc("POST /workspaces/{workspace_id}/reconcile", func(w http.ResponseWriter, r *http.Request) {
		item, actor, ok := workspaceOwner(w, r, store, repos, credentials)
		if !ok {
			return
		}
		item, err := reconcileWorkspaceLifecycle(store, item, actor.UserID, time.Now().UTC())
		if err != nil {
			writeAPIError(w, 503, "workspace_teardown_failed", "expired workspace compute could not be removed; retry reconciliation")
			return
		}
		writeJSON(w, 200, item)
	})
}

func removeWorkspaceRuntime(id string) error {
	output, err := exec.Command("docker", "rm", "-f", "-v", "vivarium-workspace-"+id).CombinedOutput()
	if err != nil && !strings.Contains(strings.ToLower(string(output)), "no such container") {
		return err
	}
	return nil
}

func reconcileWorkspaceLifecycle(store *workspaces.Store, item workspaces.Workspace, actor string, now time.Time) (workspaces.Workspace, error) {
	if item.State != "running" && item.State != "suspended" {
		return item, nil
	}
	reason := ""
	if item.ExpiresAt != nil && !item.ExpiresAt.After(now) {
		reason = "workspace runtime deadline reached"
	} else if item.Policy.IdleMinutes > 0 && !item.LastActivityAt.Add(time.Duration(item.Policy.IdleMinutes)*time.Minute).After(now) {
		reason = "workspace idle limit reached"
	}
	if reason == "" {
		return item, nil
	}
	if err := removeWorkspaceRuntime(item.ID); err != nil {
		return item, err
	}
	return store.Stop(item.ID, actor, reason, "expired")
}

func startWorkspaceRecovery(store *workspaces.Store) {
	recover := func() {
		items, err := store.ListAll()
		if err != nil {
			log.Printf("recover workspaces: %v", err)
			return
		}
		now := time.Now().UTC()
		for _, item := range items {
			if _, err := reconcileWorkspaceLifecycle(store, item, "workspace-lifecycle", now); err != nil {
				log.Printf("recover workspace %s: %v", item.ID, err)
			}
		}
	}
	recover()
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			recover()
		}
	}()
}

func workspaceOwner(w http.ResponseWriter, r *http.Request, store *workspaces.Store, repos *repositories.Store, credentials *auth.Store) (workspaces.Workspace, auth.Credential, bool) {
	actor, ok := authenticateRequest(w, r, credentials, "repositories:write", false)
	if !ok {
		return workspaces.Workspace{}, auth.Credential{}, false
	}
	item, err := store.Get(r.PathValue("workspace_id"))
	if err != nil {
		writeAPIError(w, 404, "workspace_not_found", "workspace not found")
		return item, actor, false
	}
	repo, err := repos.GetByID(item.RepositoryID)
	if err != nil || repo.OwnerID != actor.UserID {
		writeAPIError(w, 404, "workspace_not_found", "workspace not found")
		return item, actor, false
	}
	return item, actor, true
}
