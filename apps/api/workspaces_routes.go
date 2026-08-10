package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/incidents"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
)

func registerWorkspaceRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, proposalStore *proposals.Store, pullStore *pullrequests.Store, incidentStore *incidents.Store, store *workspaces.Store, authStore *auth.Store, organizationStore *organizations.Store) {
	registerWorkspaceIDERoutes(mux, catalog, store, authStore)
	registerWorkspaceCollaborationRoutes(mux, catalog, store, authStore, organizationStore)
	mux.HandleFunc("POST /workspaces", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, authStore, "repositories:write", false)
		if !ok {
			return
		}
		var input struct {
			RepositoryID string            `json:"repository_id"`
			CommitID     string            `json:"commit_id"`
			Source       workspaces.Source `json:"source"`
		}
		if err := decodeJSON(r, &input); err != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		input.RepositoryID = strings.TrimSpace(input.RepositoryID)
		input.CommitID = strings.ToLower(strings.TrimSpace(input.CommitID))
		input.Source.RepositoryID = input.RepositoryID
		repoMeta, err := catalog.GetByID(input.RepositoryID)
		if err != nil {
			writeRepositoryError(w, err)
			return
		}
		collaborator, _ := catalog.HasCollaborator(actor.UserID, input.RepositoryID)
		if actor.UserID != repoMeta.OwnerID && !collaborator {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return
		}
		repo, err := git.Open(input.RepositoryID)
		if err != nil {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return
		}
		if _, err = repo.ReadCommit(storage.ObjectID(input.CommitID)); err != nil {
			writeAPIError(w, 422, "workspace_revision_invalid", "commit_id must name an exact repository commit")
			return
		}
		if err = validateWorkspaceSource(input.Source, input.CommitID, proposalStore, pullStore, incidentStore); err != nil {
			writeAPIError(w, 422, "workspace_source_invalid", err.Error())
			return
		}
		definitionBytes, err := exec.Command("git", "--git-dir="+repo.Path(), "show", input.CommitID+":"+workspaces.DefinitionPath).Output()
		if err != nil {
			writeAPIError(w, 422, "workspace_definition_missing", "the exact revision must contain .vivarium/workspace.json")
			return
		}
		definition, err := parseWorkspaceDefinition(definitionBytes)
		if err != nil {
			writeAPIError(w, 422, "workspace_definition_invalid", err.Error())
			return
		}
		role := "collaborator"
		if actor.UserID == repoMeta.OwnerID {
			role = "owner"
		}
		created, err := store.Create(workspaces.Workspace{RepositoryID: input.RepositoryID, CommitID: input.CommitID, Definition: definition, Source: input.Source, CreatorID: actor.UserID, Access: workspaces.Access{Role: role, Scopes: []string{"repositories:read", "repositories:write"}}}, definitionBytes)
		if err != nil {
			writeAPIError(w, 500, "workspace_create_failed", "workspace could not be created")
			return
		}
		steps, failed := provisionWorkspace(repo.Path(), store.RuntimePath(created.ID), created.ID, input.CommitID, definition)
		created, err = store.Complete(created.ID, steps, failed)
		if err != nil {
			writeAPIError(w, 500, "workspace_create_failed", "workspace evidence could not be saved")
			return
		}
		w.Header().Set("Location", "/workspaces/"+created.ID)
		writeJSON(w, 201, created)
	})
	mux.HandleFunc("GET /workspaces", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, authStore, "repositories:read", false)
		if !ok {
			return
		}
		all, err := store.ListAll()
		if err != nil {
			writeAPIError(w, 500, "workspace_list_failed", "workspaces could not be listed")
			return
		}
		items := []workspaces.Workspace{}
		for _, item := range all {
			meta, metaErr := catalog.GetByID(item.RepositoryID)
			collaborator, _ := catalog.HasCollaborator(actor.UserID, item.RepositoryID)
			if metaErr == nil && (meta.OwnerID == actor.UserID || collaborator) {
				items = append(items, item)
			}
		}
		writeJSON(w, 200, map[string]any{"items": items})
	})
	mux.HandleFunc("GET /workspaces/{workspace_id}", func(w http.ResponseWriter, r *http.Request) {
		workspace, _, ok := authorizeWorkspace(w, r, store, catalog, authStore, "repositories:read")
		if ok {
			writeJSON(w, 200, workspace)
		}
	})
	mux.HandleFunc("POST /workspaces/{workspace_id}/suspend", func(w http.ResponseWriter, r *http.Request) {
		workspace, actor, ok := authorizeWorkspace(w, r, store, catalog, authStore, "repositories:write")
		if !ok {
			return
		}
		if !workspace.CanControl(actor.UserID, "lifecycle", time.Now().UTC()) {
			writeAPIError(w, 409, "workspace_control_required", "workspace lifecycle control is held by another participant or has expired")
			return
		}
		var input struct {
			Foundation string `json:"foundation"`
		}
		if err := decodeJSON(r, &input); err != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		updated, err := store.Transition(workspace.ID, actor.UserID, input.Foundation, "suspended")
		writeWorkspaceTransition(w, updated, err)
	})
	mux.HandleFunc("POST /workspaces/{workspace_id}/resume", func(w http.ResponseWriter, r *http.Request) {
		workspace, actor, ok := authorizeWorkspace(w, r, store, catalog, authStore, "repositories:write")
		if !ok {
			return
		}
		if !workspace.CanControl(actor.UserID, "lifecycle", time.Now().UTC()) {
			writeAPIError(w, 409, "workspace_control_required", "workspace lifecycle control is held by another participant or has expired")
			return
		}
		var input struct {
			Foundation string `json:"foundation"`
		}
		if err := decodeJSON(r, &input); err != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		updated, err := store.Transition(workspace.ID, actor.UserID, input.Foundation, "running")
		writeWorkspaceTransition(w, updated, err)
	})
}

func authorizeWorkspace(w http.ResponseWriter, r *http.Request, store *workspaces.Store, catalog *repositories.Store, authStore *auth.Store, scope string) (workspaces.Workspace, auth.Credential, bool) {
	actor, ok := authenticateRequest(w, r, authStore, scope, false)
	if !ok {
		return workspaces.Workspace{}, auth.Credential{}, false
	}
	item, err := store.Get(r.PathValue("workspace_id"))
	if err != nil {
		writeAPIError(w, 404, "workspace_not_found", "workspace not found")
		return item, auth.Credential{}, false
	}
	meta, err := catalog.GetByID(item.RepositoryID)
	collaborator, _ := catalog.HasCollaborator(actor.UserID, item.RepositoryID)
	if err != nil || (actor.UserID != meta.OwnerID && !collaborator) {
		writeAPIError(w, 404, "workspace_not_found", "workspace not found")
		return item, auth.Credential{}, false
	}
	return item, actor, true
}
func writeWorkspaceTransition(w http.ResponseWriter, item workspaces.Workspace, err error) {
	if errors.Is(err, workspaces.ErrConflict) {
		writeAPIError(w, 409, "workspace_foundation_changed", "workspace state or frozen foundation changed")
		return
	}
	if err != nil {
		writeAPIError(w, 500, "workspace_transition_failed", "workspace lifecycle could not be updated")
		return
	}
	writeJSON(w, 200, item)
}
func parseWorkspaceDefinition(body []byte) (workspaces.Definition, error) {
	var d workspaces.Definition
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&d); err != nil {
		return d, errors.New("definition must be valid JSON with known fields")
	}
	if d.Version != 1 || d.Image == "" || len(d.Image) > 200 || len(d.Setup) > 20 {
		return d, errors.New("version 1, an image, and at most 20 setup commands are required")
	}
	if len(d.Tools) > 50 || len(d.Dependencies) > 100 {
		return d, errors.New("tools and dependencies must be bounded")
	}
	for _, tool := range d.Tools {
		if strings.TrimSpace(tool.Name) == "" || strings.TrimSpace(tool.Version) == "" || len(tool.Name) > 100 || len(tool.Version) > 100 {
			return d, errors.New("tools require bounded names and versions")
		}
	}
	for _, dependency := range d.Dependencies {
		if strings.TrimSpace(dependency) == "" || len(dependency) > 300 {
			return d, errors.New("dependencies must be non-empty and bounded")
		}
	}
	if d.Resources.CPUs <= 0 || d.Resources.CPUs > 8 || d.Resources.MemoryMB < 128 || d.Resources.MemoryMB > 16384 || d.Resources.StorageMB < 128 || d.Resources.StorageMB > 102400 || d.Resources.SetupSeconds < 1 || d.Resources.SetupSeconds > 1800 {
		return d, errors.New("resources exceed platform bounds")
	}
	for _, command := range d.Setup {
		if strings.TrimSpace(command) == "" || len(command) > 4000 {
			return d, errors.New("setup commands must be non-empty and bounded")
		}
	}
	return d, nil
}
func validateWorkspaceSource(source workspaces.Source, commit string, ps *proposals.Store, prs *pullrequests.Store, is *incidents.Store) error {
	switch source.Kind {
	case "repository":
		return nil
	case "proposal_task":
		if ps == nil {
			return errors.New("proposal tasks unavailable")
		}
		task, e := ps.GetTask(source.RepositoryID, source.ProposalID, source.TaskID)
		if e != nil {
			return errors.New("proposal task not found")
		}
		if task.Assignment != nil && task.Assignment.Access.BaseRevision != commit {
			return errors.New("revision does not match the task foundation")
		}
		return nil
	case "pull_request":
		if prs == nil {
			return errors.New("pull requests unavailable")
		}
		pull, e := prs.Get(source.RepositoryID, source.PullRequestID)
		if e != nil {
			return errors.New("pull request not found")
		}
		if pull.SourceCommitID != commit && pull.TargetCommitID != commit {
			return errors.New("revision is not a recorded pull revision")
		}
		return nil
	case "incident_repair":
		if is == nil {
			return errors.New("incidents unavailable")
		}
		incident, e := is.Get(source.IncidentID)
		if e != nil {
			return errors.New("incident not found")
		}
		for _, action := range incident.Actions {
			if action.ID == source.RepairID && action.Kind == "emergency_repair" && action.RepositoryID == source.RepositoryID {
				return nil
			}
		}
		return errors.New("incident repair not found")
	default:
		return errors.New("source kind must be repository, proposal_task, pull_request, or incident_repair")
	}
}
func provisionWorkspace(gitPath, runtime, id, commit string, d workspaces.Definition) ([]workspaces.SetupStep, bool) {
	container := "vivarium-workspace-" + id
	inspectOutput, inspectErr := exec.Command("docker", "image", "inspect", d.Image).CombinedOutput()
	if inspectErr != nil {
		return []workspaces.SetupStep{failedSetupStep("validate workspace image volumes", inspectOutput, inspectErr)}, true
	}
	var imageMetadata []struct {
		Config struct {
			Volumes map[string]json.RawMessage `json:"Volumes"`
		} `json:"Config"`
	}
	if err := json.Unmarshal(inspectOutput, &imageMetadata); err != nil || len(imageMetadata) != 1 {
		if err == nil {
			err = errors.New("docker returned unexpected image metadata")
		}
		return []workspaces.SetupStep{failedSetupStep("validate workspace image volumes", nil, err)}, true
	}
	if len(imageMetadata[0].Config.Volumes) != 0 {
		return []workspaces.SetupStep{failedSetupStep("validate workspace image volumes", nil, errors.New("workspace images must not declare writable volumes"))}, true
	}
	// StorageMB is one total budget. Reserve bounded scratch space for tools
	// that require /tmp and make every other image path read-only, so setup
	// cannot escape the declared budget through the container writable layer.
	const scratchMB = 16
	workspaceMB := d.Resources.StorageMB - scratchMB
	createArgs := []string{"create", "--name", container, "--network=none", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--pids-limit=256", "--cpus", strconv.FormatFloat(d.Resources.CPUs, 'f', -1, 64), "--memory", fmt.Sprintf("%dm", d.Resources.MemoryMB), "--tmpfs", fmt.Sprintf("/workspace:rw,nosuid,nodev,size=%dm", workspaceMB), "--tmpfs", fmt.Sprintf("/tmp:rw,nosuid,nodev,noexec,size=%dm", scratchMB), "--env", "HOME=/workspace/.home", "--env", "TMPDIR=/tmp", "--workdir", "/workspace", d.Image, "sh", "-lc", "while :; do sleep 3600; done"}
	if out, err := exec.Command("docker", createArgs...).CombinedOutput(); err != nil {
		return []workspaces.SetupStep{failedSetupStep("create bounded workspace", out, err)}, true
	}
	cleanup := func() { _ = exec.Command("docker", "rm", "-f", "-v", container).Run() }
	if out, err := exec.Command("docker", "start", container).CombinedOutput(); err != nil {
		cleanup()
		return []workspaces.SetupStep{failedSetupStep("start bounded workspace", out, err)}, true
	}
	archive := exec.Command("git", "--git-dir="+gitPath, "archive", commit)
	copyIntoContainer := exec.Command("docker", "exec", "-i", container, "tar", "-x", "-C", "/workspace")
	pipe, err := archive.StdoutPipe()
	if err != nil {
		cleanup()
		return []workspaces.SetupStep{failedSetupStep("materialize exact revision", nil, err)}, true
	}
	copyIntoContainer.Stdin = pipe
	if err = archive.Start(); err == nil {
		err = copyIntoContainer.Run()
		if archiveErr := archive.Wait(); err == nil {
			err = archiveErr
		}
	}
	if err != nil {
		cleanup()
		return []workspaces.SetupStep{failedSetupStep("materialize exact revision", nil, err)}, true
	}
	steps := []workspaces.SetupStep{}
	failed := false
	for _, command := range d.Setup {
		start := time.Now().UTC()
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(d.Resources.SetupSeconds)*time.Second)
		args := []string{"exec", "--workdir", "/workspace", container, "sh", "-lc", command}
		out, e := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
		timedOut := errors.Is(ctx.Err(), context.DeadlineExceeded)
		cancel()
		code := 0
		state := "passed"
		if e != nil {
			state = "failed"
			failed = true
			code = -1
			if x, ok := e.(*exec.ExitError); ok {
				code = x.ExitCode()
			}
		}
		if len(out) > 65536 {
			out = out[:65536]
		}
		steps = append(steps, workspaces.SetupStep{Command: command, State: state, ExitCode: code, Output: string(out), StartedAt: start, CompletedAt: time.Now().UTC()})
		if failed {
			// Killing the Docker client does not guarantee that the named workload
			// stopped. Force removal is the terminal cleanup boundary for every
			// failed command, and especially for a client-side timeout.
			cleanup()
			if timedOut {
				steps[len(steps)-1].Output += "\nsetup timed out; bounded container force-removed"
			}
			break
		}
	}
	_ = runtime // the durable directory is a presence marker; source lives in the quota-bound container.
	return steps, failed
}

func failedSetupStep(command string, output []byte, err error) workspaces.SetupStep {
	now := time.Now().UTC()
	message := strings.TrimSpace(string(output))
	if message == "" {
		message = err.Error()
	}
	if len(message) > 65536 {
		message = message[:65536]
	}
	return workspaces.SetupStep{Command: command, State: "failed", ExitCode: -1, Output: message, StartedAt: now, CompletedAt: now}
}
