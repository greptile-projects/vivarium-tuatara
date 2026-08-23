package main

import (
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
)

// registerConflictWorkspaceRoutes turns read-only conflict evidence into a
// shared environment without treating that evidence as repository authority.
func registerConflictWorkspaceRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, pulls *pullrequests.Store, workspaceStore *workspaces.Store, authStore *auth.Store, organizations *organizations.Store) {
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/conflict-workspaces", func(w http.ResponseWriter, r *http.Request) {
		actor, owner, ok := authorizeRepositoryParticipant(w, r, catalog, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in struct {
			LaunchID    string `json:"launch_id"`
			CandidateID string `json:"candidate_id,omitempty"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		in.LaunchID = strings.TrimSpace(in.LaunchID)
		if in.LaunchID == "" || len(in.LaunchID) > 100 {
			writeAPIError(w, 422, "conflict_workspace_invalid", "launch_id must be a stable identifier of at most 100 characters")
			return
		}
		existing, reused, release, err := workspaceStore.ClaimConflictLaunch(r.PathValue("id"), r.PathValue("pull_id"), in.LaunchID)
		if err != nil {
			writeAPIError(w, 500, "conflict_workspace_failed", "workspace launch could not be reserved")
			return
		}
		defer release()
		if reused {
			if existing.ConflictContext == nil || existing.ConflictContext.CandidateID != in.CandidateID {
				writeAPIError(w, 409, "conflict_workspace_launch_changed", "launch_id is already bound to different immutable conflict evidence")
				return
			}
			if existing.State == "provisioning" {
				repo, openErr := git.Open(existing.RepositoryID)
				if openErr != nil || existing.ConflictContext == nil {
					writeAPIError(w, 503, "conflict_workspace_recovery_failed", "the immutable workspace foundation is unavailable")
					return
				}
				if cleanupErr := removeWorkspaceRuntime(existing.ID); cleanupErr != nil {
					writeAPIError(w, 503, "conflict_workspace_recovery_failed", "interrupted workspace compute could not be cleared")
					return
				}
				steps, failed := provisionWorkspace(repo.Path(), workspaceStore.RuntimePath(existing.ID), existing.ID, existing.CommitID, existing.Definition)
				if !failed {
					if stageErr := stageConflictHistories(repo.Path(), existing.ID, existing.ConflictContext.Source.CommitID, existing.ConflictContext.Target.CommitID); stageErr != nil {
						failed = true
						steps = append(steps, failedSetupStep("preload immutable conflicting histories", nil, stageErr))
					}
				}
				existing, err = workspaceStore.Complete(existing.ID, steps, failed)
				if err != nil {
					writeAPIError(w, 500, "conflict_workspace_recovery_failed", "recovered workspace evidence could not be saved")
					return
				}
			}
			w.Header().Set("Location", "/workspaces/"+existing.ID)
			writeJSON(w, 200, existing)
			return
		}
		meta, err := catalog.GetByID(r.PathValue("id"))
		if writeRepositoryError(w, err) {
			return
		}
		analysis, err := pulls.AnalyzePullConflict(meta.ID, r.PathValue("pull_id"), in.CandidateID, meta.OwnerID)
		if writePullRequestError(w, err) {
			return
		}
		pull, err := pulls.Get(meta.ID, r.PathValue("pull_id"))
		if writePullRequestError(w, err) {
			return
		}
		repo, err := git.Open(meta.ID)
		if err != nil {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return
		}
		definitionBytes, err := exec.Command("git", "--git-dir="+repo.Path(), "show", analysis.Target.CommitID+":"+workspaces.DefinitionPath).Output()
		if err != nil {
			writeAPIError(w, 422, "workspace_definition_missing", "the immutable target revision must contain .vivarium/workspace.json")
			return
		}
		definition, err := parseWorkspaceDefinition(definitionBytes)
		if err != nil {
			writeAPIError(w, 422, "workspace_definition_invalid", err.Error())
			return
		}
		policy, err := workspaceStore.GetPolicy("repository", meta.ID)
		if err != nil {
			writeAPIError(w, 500, "workspace_policy_unavailable", "workspace policy could not be read")
			return
		}
		policyScope := "repository"
		if meta.OrganizationID != "" {
			orgPolicy, policyErr := workspaceStore.GetPolicy("organization", meta.OrganizationID)
			if policyErr != nil {
				writeAPIError(w, 500, "workspace_policy_unavailable", "workspace policy could not be read")
				return
			}
			policy = workspaces.Constrain(orgPolicy, policy)
			policyScope = "organization+repository"
		}
		if definition.Resources.CPUs > policy.MaxCPUs || definition.Resources.MemoryMB > policy.MaxMemoryMB || definition.Resources.StorageMB > policy.MaxStorageMB {
			writeAPIError(w, 422, "workspace_policy_resources_exceeded", "workspace definition exceeds the effective resource policy")
			return
		}
		context := workspaces.ConflictContext{PullRequestID: r.PathValue("pull_id"), CandidateID: analysis.CandidateID, BaseCommitID: analysis.BaseCommitID, Source: workspaces.ConflictRevision{Branch: analysis.Source.Branch, CommitID: analysis.Source.CommitID, OwnerIDs: analysis.Source.OwnerIDs}, Target: workspaces.ConflictRevision{Branch: analysis.Target.Branch, CommitID: analysis.Target.CommitID, OwnerIDs: analysis.Target.OwnerIDs}, Incomplete: append([]string(nil), analysis.Incomplete...), PublicationTarget: []workspaces.ConflictPublication{{RepositoryID: pull.SourceRepositoryID, Branch: analysis.Source.Branch, Revision: analysis.Source.CommitID, Authority: "ordinary source-repository branch permissions required"}, {RepositoryID: meta.ID, Branch: analysis.Target.Branch, Revision: analysis.Target.CommitID, Authority: "ordinary target-repository branch permissions required"}}}
		for _, file := range analysis.Files {
			context.Files = append(context.Files, workspaces.ConflictFileEvidence{Path: file.Path, Kinds: file.Kinds, Symbols: file.Symbols, SourceChange: file.SourceChange, TargetChange: file.TargetChange})
		}
		for _, check := range analysis.AffectedChecks {
			context.AffectedChecks = append(context.AffectedChecks, check.Name)
		}
		role := "collaborator"
		if owner {
			role = "owner"
		}
		created, err := workspaceStore.Create(workspaces.Workspace{RepositoryID: meta.ID, OrganizationID: meta.OrganizationID, CommitID: analysis.Target.CommitID, Definition: definition, Source: workspaces.Source{Kind: "conflict_reconciliation", RepositoryID: meta.ID, PullRequestID: r.PathValue("pull_id"), ConflictLaunchID: in.LaunchID}, CreatorID: actor.UserID, Access: workspaces.Access{Role: role, Scopes: []string{"repositories:read", "repositories:write"}}, Policy: policy, PolicyScope: policyScope, PolicyVersion: policy.Version, ConflictContext: &context}, definitionBytes)
		if err != nil {
			writeAPIError(w, 500, "conflict_workspace_failed", "workspace could not be created")
			return
		}
		steps, failed := provisionWorkspace(repo.Path(), workspaceStore.RuntimePath(created.ID), created.ID, analysis.Target.CommitID, definition)
		if !failed {
			if stageErr := stageConflictHistories(repo.Path(), created.ID, analysis.Source.CommitID, analysis.Target.CommitID); stageErr != nil {
				failed = true
				steps = append(steps, failedSetupStep("preload immutable conflicting histories", nil, stageErr))
			}
		}
		created, err = workspaceStore.Complete(created.ID, steps, failed)
		if err != nil {
			writeAPIError(w, 500, "conflict_workspace_failed", "workspace evidence could not be saved")
			return
		}
		w.Header().Set("Location", "/workspaces/"+created.ID)
		writeJSON(w, 201, created)
	})

	mux.HandleFunc("POST /workspaces/{workspace_id}/conflict-invitations", func(w http.ResponseWriter, r *http.Request) {
		item, actor, ok := authorizeWorkspace(w, r, workspaceStore, catalog, authStore, "repositories:write")
		if !ok {
			return
		}
		if item.ConflictContext == nil || actor.UserID != item.CreatorID {
			writeAPIError(w, 403, "conflict_invitation_forbidden", "only the reconciliation workspace creator may invite affected owners")
			return
		}
		var in struct {
			PrincipalKind string `json:"principal_kind"`
			PrincipalID   string `json:"principal_id"`
			Role          string `json:"role"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		if in.PrincipalKind != "human" && in.PrincipalKind != "approved_agent" {
			writeAPIError(w, 422, "conflict_invitation_invalid", "principal_kind must be human or approved_agent")
			return
		}
		if in.PrincipalKind == "human" {
			affected := slices.Contains(item.ConflictContext.Source.OwnerIDs, in.PrincipalID) || slices.Contains(item.ConflictContext.Target.OwnerIDs, in.PrincipalID)
			currentParticipant := false
			for _, target := range item.ConflictContext.PublicationTarget {
				meta, metaErr := catalog.GetByID(target.RepositoryID)
				collaborator, _ := catalog.HasCollaborator(in.PrincipalID, target.RepositoryID)
				if metaErr == nil && (in.PrincipalID == meta.OwnerID || collaborator) {
					currentParticipant = true
				}
			}
			if !affected || !currentParticipant {
				writeAPIError(w, 422, "conflict_invitation_invalid", "human invitee must be an affected current repository participant")
				return
			}
		} else if !workspaceApprovedAgent(organizations, catalog, item.RepositoryID, in.PrincipalID) {
			writeAPIError(w, 422, "conflict_invitation_invalid", "agent must be approved for the repository organization")
			return
		}
		updated, err := workspaceStore.Invite(item.ID, actor.UserID, in.PrincipalKind, in.PrincipalID, strings.TrimSpace(in.Role))
		if errors.Is(err, workspaces.ErrConflict) {
			writeAPIError(w, 409, "conflict_invitation_exists", "principal already has an invitation")
			return
		}
		if err != nil {
			writeAPIError(w, 422, "conflict_invitation_invalid", "invitation could not be saved")
			return
		}
		writeJSON(w, 201, updated)
	})
	mux.HandleFunc("POST /workspaces/{workspace_id}/conflict-invitations/respond", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, authStore, "repositories:read", false)
		if !ok {
			return
		}
		var in struct {
			Status string `json:"status"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		updated, err := workspaceStore.RespondInvitation(r.PathValue("workspace_id"), actor.UserID, in.Status)
		if err != nil {
			writeAPIError(w, 422, "conflict_invitation_invalid", "pending invitation was not found")
			return
		}
		writeJSON(w, 200, updated)
	})
}

func stageConflictHistories(repositoryPath, workspaceID, source, target string) error {
	tmp, err := os.MkdirTemp("", "vivarium-conflict-bundle-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	bare := filepath.Join(tmp, "history.git")
	if out, e := exec.Command("git", "init", "--bare", bare).CombinedOutput(); e != nil {
		return errors.New(strings.TrimSpace(string(out)))
	}
	for name, revision := range map[string]string{"source": source, "target": target} {
		if out, e := exec.Command("git", "--git-dir="+bare, "fetch", repositoryPath, revision+":refs/heads/"+name).CombinedOutput(); e != nil {
			return errors.New(strings.TrimSpace(string(out)))
		}
	}
	bundle := filepath.Join(tmp, "conflicting-histories.bundle")
	if out, e := exec.Command("git", "--git-dir="+bare, "bundle", "create", bundle, "--all").CombinedOutput(); e != nil {
		return errors.New(strings.TrimSpace(string(out)))
	}
	container := "vivarium-workspace-" + workspaceID
	if out, e := exec.Command("docker", "exec", container, "mkdir", "-p", "/workspace/.vivarium").CombinedOutput(); e != nil {
		return errors.New(strings.TrimSpace(string(out)))
	}
	if out, e := exec.Command("docker", "cp", bundle, container+":/workspace/.vivarium/conflicting-histories.bundle").CombinedOutput(); e != nil {
		return errors.New(strings.TrimSpace(string(out)))
	}
	if out, e := exec.Command("docker", "exec", "--workdir", "/workspace", container, "sh", "-lc", "git init && git fetch .vivarium/conflicting-histories.bundle 'refs/heads/*:refs/remotes/conflict/*' && git reset --hard conflict/target").CombinedOutput(); e != nil {
		return errors.New(strings.TrimSpace(string(out)))
	}
	return nil
}
