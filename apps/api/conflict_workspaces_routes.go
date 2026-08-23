package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/packages"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
)

// registerConflictWorkspaceRoutes turns read-only conflict evidence into a
// shared environment without treating that evidence as repository authority.
func registerConflictWorkspaceRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, pulls *pullrequests.Store, workspaceStore *workspaces.Store, authStore *auth.Store, organizations *organizations.Store, checkStores ...*checkruns.Store) {
	var checkStore *checkruns.Store
	if len(checkStores) > 0 {
		checkStore = checkStores[0]
	}
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
		if len(context.AffectedChecks) > 0 {
			definitionBytes, definitionErr := exec.Command("git", "--git-dir="+repo.Path(), "show", analysis.Target.CommitID+":"+checkruns.ConfigPath).Output()
			if definitionErr != nil {
				writeAPIError(w, 422, "conflict_required_checks_unavailable", "the exact target revision does not define every affected required check")
				return
			}
			config, configErr := checkruns.ParseConfig(definitionBytes)
			if configErr != nil {
				writeAPIError(w, 422, "conflict_required_checks_unavailable", "the exact target revision has an invalid required-check definition")
				return
			}
			byName := map[string]checkruns.Definition{}
			for _, definition := range config.Checks {
				byName[definition.Name] = definition
			}
			for _, name := range context.AffectedChecks {
				definition, found := byName[name]
				if !found {
					writeAPIError(w, 422, "conflict_required_checks_unavailable", "the exact target revision does not define every affected required check")
					return
				}
				context.RequiredChecks = append(context.RequiredChecks, workspaces.ConflictRequiredCheck{Name: name, Definition: definition})
			}
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

	mux.HandleFunc("GET /workspaces/{workspace_id}/conflict-comparison", func(w http.ResponseWriter, r *http.Request) {
		item, actor, ok := authorizeRunningWorkspace(w, r, workspaceStore, catalog, authStore, "repositories:read")
		if !ok {
			return
		}
		pathValue := strings.TrimSpace(r.URL.Query().Get("path"))
		if !conflictPath(item, pathValue) {
			writeAPIError(w, 422, "conflict_path_invalid", "path must name a file retained in the immutable conflict evidence")
			return
		}
		type side struct {
			Revision string `json:"revision"`
			Content  string `json:"content"`
			SHA256   string `json:"sha256"`
			Missing  bool   `json:"missing"`
		}
		readGit := func(revision string) side {
			out, err := workspaceAuthorizedExec(catalog, item, actor, false, 10*time.Second, "/workspace", nil, "git", "show", revision+":"+pathValue)
			if err != nil {
				return side{Revision: revision, Missing: true}
			}
			if len(out) > workspaceOutputLimit {
				return side{Revision: revision, Missing: true}
			}
			digest := sha256.Sum256(out)
			return side{Revision: revision, Content: string(out), SHA256: hex.EncodeToString(digest[:])}
		}
		proposedBytes, proposedErr := workspaceAuthorizedExec(catalog, item, actor, false, 10*time.Second, "/workspace", nil, "sh", "-c", "test -f \"$1\" && test ! -L \"$1\" && cat -- \"$1\"", "sh", pathValue)
		proposed := side{Revision: "workspace:" + item.ID, Missing: proposedErr != nil}
		if proposedErr == nil && len(proposedBytes) <= workspaceOutputLimit {
			digest := sha256.Sum256(proposedBytes)
			proposed.Content, proposed.SHA256 = string(proposedBytes), hex.EncodeToString(digest[:])
		} else if len(proposedBytes) > workspaceOutputLimit {
			proposed.Missing = true
		}
		writeJSON(w, 200, map[string]any{"path": pathValue, "base": readGit(item.ConflictContext.BaseCommitID), "source": readGit(item.ConflictContext.Source.CommitID), "target": readGit(item.ConflictContext.Target.CommitID), "proposed": proposed})
	})

	mux.HandleFunc("POST /workspaces/{workspace_id}/conflict-questions", func(w http.ResponseWriter, r *http.Request) {
		item, actor, ok := authorizeWorkspace(w, r, workspaceStore, catalog, authStore, "repositories:read")
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int                           `json:"expected_version"`
			Body            string                        `json:"body"`
			Uncertainty     string                        `json:"uncertainty"`
			Citations       []workspaces.ConflictCitation `json:"citations"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		if !validConflictStatement(item, in.Body, in.Uncertainty, in.Citations) {
			writeAPIError(w, 422, "conflict_question_invalid", "question, uncertainty, and one to ten exact conflict citations are required")
			return
		}
		updated, err := workspaceStore.AddConflictQuestion(item.ID, in.ExpectedVersion, workspaces.ConflictQuestion{Body: strings.TrimSpace(in.Body), Uncertainty: strings.TrimSpace(in.Uncertainty), Citations: in.Citations, Authorship: conflictAuthorship(actor)})
		writeConflictMutation(w, updated, err)
	})

	mux.HandleFunc("POST /workspaces/{workspace_id}/conflict-questions/{question_id}/answer", func(w http.ResponseWriter, r *http.Request) {
		item, actor, ok := authorizeWorkspace(w, r, workspaceStore, catalog, authStore, "repositories:read")
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int                           `json:"expected_version"`
			Body            string                        `json:"body"`
			Uncertainty     string                        `json:"uncertainty"`
			Citations       []workspaces.ConflictCitation `json:"citations"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		if !validConflictStatement(item, in.Body, in.Uncertainty, in.Citations) {
			writeAPIError(w, 422, "conflict_answer_invalid", "answer, uncertainty, and one to ten exact conflict citations are required")
			return
		}
		updated, err := workspaceStore.AnswerConflictQuestion(item.ID, r.PathValue("question_id"), in.ExpectedVersion, workspaces.ConflictAnswer{Body: strings.TrimSpace(in.Body), Uncertainty: strings.TrimSpace(in.Uncertainty), Citations: in.Citations, Authorship: conflictAuthorship(actor)})
		writeConflictMutation(w, updated, err)
	})

	mux.HandleFunc("POST /workspaces/{workspace_id}/conflict-resolutions", func(w http.ResponseWriter, r *http.Request) {
		item, actor, ok := authorizeWorkspace(w, r, workspaceStore, catalog, authStore, "repositories:write")
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int                               `json:"expected_version"`
			Path            string                            `json:"path"`
			Summary         string                            `json:"summary"`
			ProposedContent string                            `json:"proposed_content"`
			ExpectedSHA256  string                            `json:"expected_sha256"`
			Uncertainty     string                            `json:"uncertainty"`
			Preservation    []workspaces.ConflictPreservation `json:"preservation"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		if !conflictPath(item, in.Path) || strings.TrimSpace(in.Summary) == "" || len(in.Summary) > 1000 || len(in.ProposedContent) > workspaceOutputLimit || !validWorkspaceDigest(in.ExpectedSHA256) || strings.TrimSpace(in.Uncertainty) == "" || len(in.Uncertainty) > 1000 || !validPreservation(item, in.Preservation) {
			writeAPIError(w, 422, "conflict_resolution_invalid", "resolution must be bounded, target retained evidence, state uncertainty, and explain at least one preserved or intentionally changed outcome")
			return
		}
		updated, err := workspaceStore.AddConflictResolution(item.ID, in.ExpectedVersion, workspaces.ConflictResolution{Path: in.Path, Summary: strings.TrimSpace(in.Summary), ProposedContent: in.ProposedContent, ExpectedSHA256: in.ExpectedSHA256, Uncertainty: strings.TrimSpace(in.Uncertainty), Preservation: in.Preservation, Authorship: conflictAuthorship(actor)})
		writeConflictMutation(w, updated, err)
	})

	mux.HandleFunc("POST /workspaces/{workspace_id}/conflict-resolutions/{resolution_id}/{action}", func(w http.ResponseWriter, r *http.Request) {
		item, actor, ok := authorizeRunningWorkspace(w, r, workspaceStore, catalog, authStore, "repositories:write")
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int `json:"expected_version"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		var resolution *workspaces.ConflictResolution
		for i := range item.ConflictContext.Resolutions {
			if item.ConflictContext.Resolutions[i].ID == r.PathValue("resolution_id") {
				resolution = &item.ConflictContext.Resolutions[i]
			}
		}
		applying := r.PathValue("action") == "apply"
		validApplyState := applying && slices.Contains([]string{"proposed", "applying", "applied"}, resolutionState(resolution))
		validUndoState := !applying && r.PathValue("action") == "undo" && slices.Contains([]string{"applied", "undoing", "undone"}, resolutionState(resolution))
		if resolution == nil || (!validApplyState && !validUndoState) {
			writeAPIError(w, 422, "conflict_resolution_action_invalid", "resolution is not in a state that permits this action")
			return
		}
		name, valid := workspaceFilePath(w, resolution.Path)
		if !valid {
			return
		}
		inspect := func(current workspaces.Workspace, retained workspaces.ConflictResolution) (string, string, error) {
			previous, readErr := workspaceAuthorizedExec(catalog, current, actor, false, 10*time.Second, "/workspace", nil, "sh", "-c", "test -f \"$1\" && test ! -L \"$1\" && cat -- \"$1\"", "sh", retained.Path)
			if readErr != nil {
				return "", "", readErr
			}
			digest := sha256.Sum256(previous)
			return string(previous), hex.EncodeToString(digest[:]), nil
		}
		mutate := func(current workspaces.Workspace, retained workspaces.ConflictResolution, content, expected string) error {
			_, writeErr := workspaceAuthorizedExec(catalog, current, actor, true, 10*time.Second, filepath.Dir(name), strings.NewReader(content), "sh", "-c", workspaceFileWriteScript, "sh", filepath.Base(name), expected)
			return writeErr
		}
		updated, err := workspaceStore.ActConflictResolution(item.ID, resolution.ID, in.ExpectedVersion, applying, workspacePrincipal(actor), conflictAuthorship(actor), inspect, mutate)
		if errors.Is(err, workspaces.ErrControl) {
			writeAPIError(w, 409, "workspace_control_required", "live file control is held by another participant or has expired")
			return
		}
		if errors.Is(err, workspaces.ErrConflict) {
			writeAPIError(w, 409, "conflict_resolution_changed", "proposed file changed; compare all four versions before retrying")
			return
		}
		if err != nil {
			writeAPIError(w, 503, "conflict_resolution_pending", "resolution execution was interrupted; refresh and retry the visible pending action to reconcile it")
			return
		}
		writeConflictMutation(w, updated, err)
	})

	mux.HandleFunc("POST /workspaces/{workspace_id}/conflict-checkpoints", func(w http.ResponseWriter, r *http.Request) {
		item, actor, ok := authorizeRunningWorkspace(w, r, workspaceStore, catalog, authStore, "repositories:write")
		if !ok {
			return
		}
		principal := workspacePrincipal(actor)
		if !item.CanControl(principal, "commands", time.Now().UTC()) {
			writeAPIError(w, 409, "workspace_control_required", "live command control is required to assemble and verify a candidate")
			return
		}
		var in struct {
			ExpectedVersion int `json:"expected_version"`
			Criteria        []struct {
				Kind          string   `json:"kind"`
				Name          string   `json:"name"`
				Origin        string   `json:"origin"`
				Command       string   `json:"command"`
				ExactCriteria []string `json:"exact_criteria"`
				Coverage      []string `json:"coverage"`
				OwnerIDs      []string `json:"owner_ids"`
				Artifacts     []string `json:"artifacts"`
				Cost          float64  `json:"cost"`
			} `json:"criteria"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		if item.ConflictContext == nil || in.ExpectedVersion != item.ConflictContext.Version || len(in.Criteria) == 0 || len(in.Criteria) > 30 {
			writeAPIError(w, 409, "conflict_workspace_changed", "meaning ledger changed or checkpoint criteria are empty")
			return
		}
		allowedKinds := []string{"required_check", "reproduction", "contract", "schema", "preview_acceptance", "conflict_test"}
		criteria := make([]workspaces.ConflictCriterion, 0, len(in.Criteria))
		checkRunScopeID := fmt.Sprintf("%s-%d", item.ID, in.ExpectedVersion)
		seenKinds, seenRequired := map[string]bool{}, map[string]bool{}
		requiredDefinitions := map[string]workspaces.ConflictRequiredCheck{}
		for _, definition := range item.ConflictContext.RequiredChecks {
			requiredDefinitions[definition.Name] = definition
		}
		affectedOwners := append(append([]string{}, item.ConflictContext.Source.OwnerIDs...), item.ConflictContext.Target.OwnerIDs...)
		for _, requested := range in.Criteria {
			seenKinds[requested.Kind] = true
			if requested.Kind == "required_check" {
				if seenRequired[requested.Name] {
					writeAPIError(w, 422, "conflict_checkpoint_invalid", "each affected required check must appear exactly once")
					return
				}
				seenRequired[requested.Name] = true
				definition, found := requiredDefinitions[requested.Name]
				if !found || requested.Command != definition.Definition.Command {
					writeAPIError(w, 422, "conflict_checkpoint_required_check_changed", "required-check commands must match the immutable repository definition")
					return
				}
			}
		}
		for _, kind := range allowedKinds {
			if !seenKinds[kind] {
				writeAPIError(w, 422, "conflict_checkpoint_incomplete", "checkpoint must evaluate required checks, reproductions, contracts, schemas, preview acceptance, and repository conflict tests")
				return
			}
		}
		for _, check := range item.ConflictContext.AffectedChecks {
			if !seenRequired[check] {
				writeAPIError(w, 422, "conflict_checkpoint_incomplete", "every affected required check must run against the candidate")
				return
			}
		}
		// Create an unreferenced two-parent Git object before executing anything.
		// Every criterion is reset to this object, preventing one check's generated
		// files or edits from changing what a later check evaluates.
		candidate, candidateErr := workspaceAuthorizedExec(catalog, item, actor, true, time.Minute, "/workspace", nil, "sh", "-c", "git diff --check && git add -A && tree=$(git write-tree) && commit=$(printf '%s\\n' 'Vivarium reconciliation checkpoint' | GIT_AUTHOR_NAME=vivarium GIT_AUTHOR_EMAIL=checkpoint@invalid GIT_COMMITTER_NAME=vivarium GIT_COMMITTER_EMAIL=checkpoint@invalid git commit-tree \"$tree\" -p \"$1\" -p \"$2\") && printf '%s %s' \"$commit\" \"$tree\"", "sh", item.ConflictContext.Source.CommitID, item.ConflictContext.Target.CommitID)
		candidateParts := strings.Fields(string(candidate))
		if candidateErr != nil || len(candidateParts) != 2 || len(candidateParts[0]) != 40 || len(candidateParts[1]) != 40 {
			writeAPIError(w, 422, "conflict_candidate_invalid", "workspace must form a clean immutable Git candidate before evidence can be retained")
			return
		}
		var checkRepositoryPath string
		var removeCheckRepository func()
		if len(seenRequired) > 0 {
			if checkStore == nil {
				writeAPIError(w, 503, "conflict_check_executor_unavailable", "the repository check executor is unavailable")
				return
			}
			var prepareErr error
			checkRepositoryPath, removeCheckRepository, prepareErr = prepareConflictCheckRepository(catalog, item, actor, candidateParts[0])
			if prepareErr != nil {
				writeAPIError(w, 503, "conflict_check_candidate_unavailable", "the immutable candidate could not be prepared for isolated checks")
				return
			}
			defer removeCheckRepository()
		}
		dependencyBody, dependencyErr := readConflictDependencyManifest(checkRepositoryPath, candidateParts[0])
		if dependencyErr != nil {
			writeAPIError(w, 503, "conflict_dependency_revision_unavailable", "the candidate dependency manifest could not be resolved authoritatively")
			return
		}
		dependencySum := sha256.Sum256(dependencyBody)
		for _, requested := range in.Criteria {
			if !slices.Contains(allowedKinds, requested.Kind) || !slices.Contains([]string{"source", "target", "both"}, requested.Origin) || strings.TrimSpace(requested.Name) == "" || strings.TrimSpace(requested.Command) == "" || len(requested.Command) > 4000 || len(requested.ExactCriteria) == 0 || len(requested.ExactCriteria) > 20 || len(requested.Coverage) == 0 || len(requested.OwnerIDs) == 0 || requested.Cost < 0 {
				writeAPIError(w, 422, "conflict_checkpoint_invalid", "each bounded criterion needs a supported kind, source, command, exact criteria, coverage, owners, and non-negative cost")
				return
			}
			for _, path := range requested.Coverage {
				if !conflictPath(item, path) {
					writeAPIError(w, 422, "conflict_checkpoint_invalid", "coverage must name affected conflict paths")
					return
				}
			}
			for _, ownerID := range requested.OwnerIDs {
				if !slices.Contains(affectedOwners, ownerID) {
					writeAPIError(w, 422, "conflict_checkpoint_invalid", "criterion owners must be affected source or target owners")
					return
				}
			}
			started := time.Now()
			if requested.Kind == "required_check" {
				definition := requiredDefinitions[requested.Name].Definition
				runs, createErr := checkStore.CreateRequested(item.RepositoryID, checkRunScopeID, candidateParts[0], []checkruns.Definition{definition}, actor.UserID)
				if createErr != nil || len(runs) != 1 {
					writeAPIError(w, 503, "conflict_check_failed", "the isolated required check could not be created")
					return
				}
				checkStore.Execute(runs[0], checkRepositoryPath)
				run, getErr := checkStore.Get(item.RepositoryID, checkRunScopeID, runs[0].ID)
				if getErr != nil {
					writeAPIError(w, 503, "conflict_check_failed", "the isolated required check result is unavailable")
					return
				}
				events, _ := checkStore.Events(item.RepositoryID, checkRunScopeID, run.ID, 0)
				var logs strings.Builder
				for _, event := range events {
					if event.Stream != "" && event.Message != "" {
						logs.WriteString(event.Stream + ": " + event.Message + "\n")
					}
				}
				exitCode := 1
				if run.ExitCode != nil {
					exitCode = *run.ExitCode
				}
				state := "failed"
				if run.State == "succeeded" {
					state = "passed"
				}
				artifacts := make([]workspaces.ConflictCheckpointArtifact, 0, len(run.Artifacts))
				for _, artifact := range run.Artifacts {
					artifacts = append(artifacts, workspaces.ConflictCheckpointArtifact{Path: artifact.Path, SHA256: artifact.SHA256, Size: artifact.Size})
				}
				logText := logs.String()
				if len(logText) > workspaceOutputLimit {
					logText = logText[:workspaceOutputLimit]
				}
				criteria = append(criteria, workspaces.ConflictCriterion{Kind: requested.Kind, Name: requested.Name, Origin: requested.Origin, Command: definition.Command, ExactCriteria: requested.ExactCriteria, Coverage: requested.Coverage, OwnerIDs: requested.OwnerIDs, State: state, ExitCode: exitCode, Logs: logText, Artifacts: artifacts, Cost: requested.Cost + time.Since(started).Seconds()/3600, CheckRunID: run.ID, CheckRunScopeID: checkRunScopeID})
				continue
			}
			output, runErr := workspaceAuthorizedExec(catalog, item, actor, true, 30*time.Minute, "/workspace", nil, "sh", "-c", "git reset --hard --quiet \"$1\" && git clean -fdx --quiet && sh -lc \"$2\"", "sh", candidateParts[0], requested.Command)
			if len(output) > workspaceOutputLimit {
				output = output[:workspaceOutputLimit]
			}
			exitCode := 0
			state := "passed"
			if runErr != nil {
				state, exitCode = "failed", 1
				var exitErr *exec.ExitError
				if errors.As(runErr, &exitErr) {
					exitCode = exitErr.ExitCode()
				}
			}
			artifacts := []workspaces.ConflictCheckpointArtifact{}
			for _, path := range requested.Artifacts {
				if _, valid := workspaceFilePath(w, path); !valid {
					return
				}
				meta, artifactErr := workspaceAuthorizedExec(catalog, item, actor, false, 10*time.Second, "/workspace", nil, "sh", "-c", "test -f \"$1\" && test ! -L \"$1\" && sha256sum -- \"$1\" && stat -c %s -- \"$1\"", "sh", path)
				parts := strings.Fields(string(meta))
				if artifactErr != nil || len(parts) < 3 || !validWorkspaceDigest(parts[0]) {
					writeAPIError(w, 422, "conflict_checkpoint_artifact_invalid", "artifact must be a bounded regular workspace file")
					return
				}
				var size int64
				if _, scanErr := fmt.Sscan(parts[2], &size); scanErr != nil {
					writeAPIError(w, 422, "conflict_checkpoint_artifact_invalid", "artifact size is unavailable")
					return
				}
				artifacts = append(artifacts, workspaces.ConflictCheckpointArtifact{Path: path, SHA256: parts[0], Size: size})
			}
			criteria = append(criteria, workspaces.ConflictCriterion{Kind: requested.Kind, Name: strings.TrimSpace(requested.Name), Origin: requested.Origin, Command: requested.Command, ExactCriteria: requested.ExactCriteria, Coverage: requested.Coverage, OwnerIDs: requested.OwnerIDs, State: state, ExitCode: exitCode, Logs: string(output), Artifacts: artifacts, Cost: requested.Cost + time.Since(started).Seconds()/3600})
		}
		currentPolicy, policyErr := currentConflictPolicy(workspaceStore, item)
		if policyErr != nil {
			writeAPIError(w, 503, "workspace_policy_unavailable", "current effective workspace policy could not be resolved")
			return
		}
		policyBody, _ := json.Marshal(currentPolicy)
		policySum := sha256.Sum256(policyBody)
		if _, verifyErr := workspaceAuthorizedExec(catalog, item, actor, true, time.Minute, "/workspace", nil, "sh", "-c", "git reset --hard --quiet \"$1\" && git clean -fdx --quiet && test \"$(git write-tree)\" = \"$2\"", "sh", candidateParts[0], candidateParts[1]); verifyErr != nil {
			writeAPIError(w, 409, "conflict_candidate_changed", "verification did not leave the immutable candidate reproducible")
			return
		}
		checkpoint := workspaces.ConflictCheckpoint{CandidateCommitID: candidateParts[0], CandidateTreeID: candidateParts[1], SourceRevision: item.ConflictContext.Source.CommitID, TargetRevision: item.ConflictContext.Target.CommitID, DependencyRevision: hex.EncodeToString(dependencySum[:]), PolicyRevision: hex.EncodeToString(policySum[:]), Criteria: criteria, CreatedBy: conflictAuthorship(actor)}
		updated, err := workspaceStore.AddConflictCheckpoint(item.ID, in.ExpectedVersion, principal, item.Control.Version, checkpoint)
		if errors.Is(err, workspaces.ErrControl) {
			writeAPIError(w, 409, "workspace_control_required", "workspace stopped or command control changed before checkpoint persistence")
			return
		}
		writeConflictMutation(w, updated, err)
	})

	mux.HandleFunc("POST /workspaces/{workspace_id}/conflict-checkpoints/{checkpoint_id}/criteria/{criterion_id}/decision", func(w http.ResponseWriter, r *http.Request) {
		item, actor, ok := authorizeWorkspace(w, r, workspaceStore, catalog, authStore, "repositories:read")
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int    `json:"expected_version"`
			Decision        string `json:"decision"`
			Rationale       string `json:"rationale"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		if !slices.Contains([]string{"accepted", "rejected", "withdrawn"}, in.Decision) || strings.TrimSpace(in.Rationale) == "" || len(in.Rationale) > 1000 {
			writeAPIError(w, 422, "conflict_checkpoint_decision_invalid", "decision must accept, reject, or withdraw acceptance for the exact criterion with a rationale")
			return
		}
		updated, err := workspaceStore.DecideConflictCheckpoint(item.ID, r.PathValue("checkpoint_id"), r.PathValue("criterion_id"), in.ExpectedVersion, workspaces.ConflictCheckpointDecision{OwnerID: actor.UserID, Decision: in.Decision, Rationale: strings.TrimSpace(in.Rationale)})
		writeConflictMutation(w, updated, err)
	})

	mux.HandleFunc("POST /workspaces/{workspace_id}/conflict-checkpoints/{checkpoint_id}/publications", func(w http.ResponseWriter, r *http.Request) {
		item, actor, ok := authorizeWorkspace(w, r, workspaceStore, catalog, authStore, "repositories:write")
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int    `json:"expected_version"`
			PublicationID   string `json:"publication_id"`
			Mode            string `json:"mode"`
			Branch          string `json:"branch"`
			Title           string `json:"title,omitempty"`
			Body            string `json:"body,omitempty"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		in.PublicationID, in.Mode, in.Branch = strings.TrimSpace(in.PublicationID), strings.TrimSpace(in.Mode), strings.TrimSpace(in.Branch)
		if in.PublicationID == "" || len(in.PublicationID) > 100 || !slices.Contains([]string{"source_branch", "resolution_pull"}, in.Mode) || in.Branch == "" || strings.HasPrefix(in.Branch, "refs/") {
			writeAPIError(w, 422, "conflict_publication_invalid", "publication requires a stable ID, supported mode, and repository branch")
			return
		}
		if item.ConflictContext == nil {
			writeAPIError(w, 409, "conflict_workspace_changed", "workspace has no conflict evidence")
			return
		}
		var checkpoint *workspaces.ConflictCheckpoint
		for i := range item.ConflictContext.Checkpoints {
			if item.ConflictContext.Checkpoints[i].ID == r.PathValue("checkpoint_id") {
				checkpoint = &item.ConflictContext.Checkpoints[i]
				break
			}
		}
		if checkpoint == nil {
			writeAPIError(w, 404, "conflict_checkpoint_not_found", "checkpoint not found")
			return
		}
		pull, err := pulls.Get(item.RepositoryID, item.ConflictContext.PullRequestID)
		if writePullRequestError(w, err) {
			return
		}
		repositoryID, expectedTip := item.RepositoryID, ""
		if in.Mode == "source_branch" {
			repositoryID, in.Branch, expectedTip = pull.SourceRepositoryID, pull.SourceBranch, checkpoint.SourceRevision
		}
		publication := workspaces.ConflictPublicationRecord{ID: in.PublicationID, CheckpointID: checkpoint.ID, Mode: in.Mode, RepositoryID: repositoryID, Branch: in.Branch, ExpectedBranchTip: expectedTip, PublishedBy: conflictAuthorship(actor)}
		_, reserved, err := workspaceStore.ReserveConflictPublication(item.ID, in.ExpectedVersion, publication)
		if errors.Is(err, workspaces.ErrInvalid) {
			writeAPIError(w, 409, "conflict_checkpoint_not_accepted", "every current criterion must pass and be accepted by every affected owner")
			return
		}
		if errors.Is(err, workspaces.ErrConflict) {
			writeAPIError(w, 409, "conflict_workspace_changed", "publication identity or conflict evidence changed")
			return
		}
		if err != nil {
			writeAPIError(w, 500, "conflict_publication_failed", "publication could not be reserved")
			return
		}
		if reserved.Status == "published" || reserved.Status == "action_required" {
			writeJSON(w, 200, reserved)
			return
		}
		meta, metaErr := catalog.GetByID(repositoryID)
		collaborator, collabErr := catalog.HasCollaborator(actor.UserID, repositoryID)
		if metaErr != nil || collabErr != nil || (actor.UserID != meta.OwnerID && !collaborator) {
			_, _ = workspaceStore.CompleteConflictPublication(item.ID, in.PublicationID, "", "", "action_required", "current publication-branch permission is required")
			writeAPIError(w, 403, "conflict_publication_forbidden", "workspace access does not grant publication-branch authority")
			return
		}
		// Recheck both inputs after reserving and before the first ref mutation.
		// A retry after branch publication instead reconciles the frozen commit and
		// continues ordinary pull/check publication.
		targetRepo, openErr := git.Open(item.RepositoryID)
		sourceRepo, sourceErr := git.Open(pull.SourceRepositoryID)
		var targetRef, sourceRef storage.Reference
		var targetErr, sourceRefErr error
		if openErr == nil {
			targetRef, targetErr = targetRepo.ReadReference("refs/heads/" + pull.TargetBranch)
		}
		if sourceErr == nil {
			sourceRef, sourceRefErr = sourceRepo.ReadReference("refs/heads/" + pull.SourceBranch)
		}
		if reserved.Status == "publishing" && (openErr != nil || sourceErr != nil || targetErr != nil || sourceRefErr != nil || targetRef.Target != checkpoint.TargetRevision || sourceRef.Target != checkpoint.SourceRevision || pull.Status != pullrequests.Open || pull.SourceCommitID != checkpoint.SourceRevision) {
			_, _ = workspaceStore.CompleteConflictPublication(item.ID, in.PublicationID, "", "", "action_required", "source, target, or pull snapshot moved; assemble and accept a successor checkpoint")
			writeAPIError(w, 409, "conflict_publication_stale", "source, target, or pull snapshot changed after verification")
			return
		}
		candidateRepoPath, cleanup, prepErr := prepareConflictCheckRepository(catalog, item, actor, checkpoint.CandidateCommitID)
		if prepErr != nil {
			writeAPIError(w, 503, "conflict_publication_failed", "accepted candidate could not be exported")
			return
		}
		defer cleanup()
		message := conflictPublicationMessage(item, *checkpoint, actor.UserID)
		cmd := exec.Command("git", "--git-dir="+candidateRepoPath, "commit-tree", checkpoint.CandidateTreeID, "-p", checkpoint.SourceRevision, "-p", checkpoint.TargetRevision)
		publicationTime := reserved.CreatedAt.Format(time.RFC3339)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME="+actor.UserID, "GIT_AUTHOR_EMAIL="+actor.UserID+"@vivarium.invalid", "GIT_COMMITTER_NAME=Vivarium conflict publication", "GIT_COMMITTER_EMAIL=conflicts@vivarium.invalid", "GIT_AUTHOR_DATE="+publicationTime, "GIT_COMMITTER_DATE="+publicationTime)
		cmd.Stdin = strings.NewReader(message)
		publishedBytes, commitErr := cmd.Output()
		publishedCommit := strings.TrimSpace(string(publishedBytes))
		destination, destinationErr := git.Open(repositoryID)
		var importErr error
		if commitErr == nil && destinationErr == nil {
			importCommand := exec.Command("git", "-c", "fetch.unpackLimit=2147483647", "--git-dir="+destination.Path(), "fetch", "--no-tags", "--no-write-fetch-head", candidateRepoPath, publishedCommit)
			if output, runErr := importCommand.CombinedOutput(); runErr != nil {
				importErr = fmt.Errorf("%w: %s", runErr, strings.TrimSpace(string(output)))
			} else if _, runErr = destination.ReadCommit(storage.ObjectID(publishedCommit)); runErr != nil {
				importErr = runErr
			}
		}
		if commitErr != nil || destinationErr != nil || importErr != nil {
			writeAPIError(w, 503, "conflict_publication_failed", "provenance commit could not be imported")
			return
		}
		ref := storage.Reference{Name: "refs/heads/" + in.Branch, Target: publishedCommit}
		_, err = workspaceStore.PublishConflictBranch(item.ID, in.PublicationID, publishedCommit, func() error {
			var refErr error
			if in.Mode == "source_branch" {
				refErr = destination.UpdateReferenceIfTarget(ref, expectedTip)
			} else {
				refErr = destination.CreateReference(ref)
			}
			if refErr != nil {
				if current, readErr := destination.ReadReference(ref.Name); readErr == nil && current.Target == publishedCommit {
					return nil
				}
			}
			return refErr
		})
		if errors.Is(err, workspaces.ErrInvalid) {
			_, _ = workspaceStore.CompleteConflictPublication(item.ID, in.PublicationID, publishedCommit, "", "action_required", "approval was withdrawn before branch publication; obtain current acceptance")
			writeAPIError(w, 409, "conflict_publication_approval_withdrawn", "owner acceptance changed before branch publication")
			return
		}
		if err != nil {
			_, _ = workspaceStore.CompleteConflictPublication(item.ID, in.PublicationID, publishedCommit, "", "action_required", "publication branch was concurrently updated or already exists; choose a current permitted branch and publish a successor checkpoint")
			writeAPIError(w, 409, "conflict_publication_branch_changed", "publication branch changed concurrently")
			return
		}
		var governedPull pullrequests.PullRequest
		if in.Mode == "source_branch" {
			governedPull, err = pulls.SynchronizeSourceAfter(item.RepositoryID, pull.ID, nil)
		} else {
			title := strings.TrimSpace(in.Title)
			if title == "" {
				title = "Resolve conflicts for " + pull.Title
			}
			body := strings.TrimSpace(in.Body)
			if body == "" {
				body = "Publishes accepted conflict checkpoint " + checkpoint.ID + " from workspace " + item.ID + "."
			}
			governedPull, err = pulls.FindOrCreateRecovery(item.RepositoryID, actor.UserID, title, body, in.Branch, pull.TargetBranch)
			if err == nil {
				governedPull, err = pulls.SynchronizeSourceAfter(item.RepositoryID, governedPull.ID, nil)
				if err == nil && governedPull.SourceCommitID != publishedCommit {
					err = pullrequests.ErrSourceChanged
				}
			}
		}
		if err != nil {
			writeAPIError(w, 503, "conflict_publication_pull_pending", "branch is published; retry to reconcile the ordinary pull request")
			return
		}
		if len(item.ConflictContext.RequiredChecks) > 0 {
			if checkStore == nil {
				writeAPIError(w, 503, "conflict_publication_checks_pending", "branch and pull are published; retry when required-check storage is available")
				return
			}
			definitions := make([]checkruns.Definition, 0, len(item.ConflictContext.RequiredChecks))
			for _, required := range item.ConflictContext.RequiredChecks {
				definitions = append(definitions, required.Definition)
			}
			runs, runErr := checkStore.CreateRequested(item.RepositoryID, governedPull.ID, publishedCommit, definitions, actor.UserID)
			if runErr != nil {
				writeAPIError(w, 503, "conflict_publication_checks_pending", "branch and pull are published; retry to reconcile required checks")
				return
			}
			for _, run := range runs {
				checkStore.Execute(run, destination.Path())
			}
		}
		updated, finishErr := workspaceStore.CompleteConflictPublication(item.ID, in.PublicationID, publishedCommit, governedPull.ID, "published", "")
		if finishErr != nil {
			writeAPIError(w, 503, "conflict_publication_reconciliation_pending", "published contribution is durable but workspace reconciliation remains pending")
			return
		}
		w.Header().Set("Location", "/repositories/"+item.RepositoryID+"/pulls/"+governedPull.ID)
		writeJSON(w, 201, map[string]any{"workspace": updated, "publication": updated.ConflictContext.Publications[len(updated.ConflictContext.Publications)-1], "pull_request": governedPull})
	})
}

func conflictPublicationMessage(item workspaces.Workspace, checkpoint workspaces.ConflictCheckpoint, actor string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Publish accepted conflict resolution\n\nWorkspace: %s\nCheckpoint: %s\nPublished-by: %s\nSource-input: %s\nTarget-input: %s\n", item.ID, checkpoint.ID, actor, checkpoint.SourceRevision, checkpoint.TargetRevision)
	for _, resolution := range item.ConflictContext.Resolutions {
		if resolution.State == "applied" {
			fmt.Fprintf(&b, "Resolution: %s %s by %s\n", resolution.ID, resolution.Path, resolution.Authorship.ActorID)
		}
	}
	for _, criterion := range checkpoint.Criteria {
		fmt.Fprintf(&b, "Verified-command: %s :: %s\n", criterion.Name, criterion.Command)
	}
	for _, decision := range checkpoint.Decisions {
		fmt.Fprintf(&b, "Approval: %s %s %s\n", decision.OwnerID, decision.CriterionID, decision.Decision)
	}
	return b.String()
}

func resolutionState(resolution *workspaces.ConflictResolution) string {
	if resolution == nil {
		return ""
	}
	return resolution.State
}

func currentConflictPolicy(store *workspaces.Store, item workspaces.Workspace) (workspaces.Policy, error) {
	policy, err := store.GetPolicy("repository", item.RepositoryID)
	if err != nil {
		return workspaces.Policy{}, err
	}
	if item.OrganizationID == "" {
		return policy, nil
	}
	organizationPolicy, err := store.GetPolicy("organization", item.OrganizationID)
	if err != nil {
		return workspaces.Policy{}, err
	}
	return workspaces.Constrain(organizationPolicy, policy), nil
}

func prepareConflictCheckRepository(catalog *repositories.Store, item workspaces.Workspace, actor auth.Credential, candidate string) (string, func(), error) {
	bundle, err := workspaceAuthorizedExec(catalog, item, actor, true, time.Minute, "/workspace", nil, "sh", "-c", "git update-ref refs/vivarium/checkpoint \"$1\" && git bundle create - refs/vivarium/checkpoint; status=$?; git update-ref -d refs/vivarium/checkpoint; exit $status", "sh", candidate)
	if err != nil {
		return "", func() {}, err
	}
	directory, err := os.MkdirTemp("", "vivarium-conflict-check-*")
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(directory) }
	bundlePath := filepath.Join(directory, "candidate.bundle")
	if err = os.WriteFile(bundlePath, bundle, 0600); err != nil {
		cleanup()
		return "", func() {}, err
	}
	repositoryPath := filepath.Join(directory, "repository.git")
	if output, initErr := exec.Command("git", "init", "--bare", repositoryPath).CombinedOutput(); initErr != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("initialize candidate repository: %w: %s", initErr, strings.TrimSpace(string(output)))
	}
	// The checkpoint is intentionally exported under a private ref. A normal
	// bare clone fetches heads and tags only and can silently produce an empty
	// repository, so name the frozen ref explicitly and retain its object ID.
	if output, fetchErr := exec.Command("git", "--git-dir="+repositoryPath, "fetch", "--no-tags", bundlePath, "refs/vivarium/checkpoint:refs/vivarium/checkpoint").CombinedOutput(); fetchErr != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("fetch candidate bundle: %w: %s", fetchErr, strings.TrimSpace(string(output)))
	}
	if output, verifyErr := exec.Command("git", "--git-dir="+repositoryPath, "rev-parse", "--verify", candidate+"^{commit}").CombinedOutput(); verifyErr != nil || strings.TrimSpace(string(output)) != candidate {
		cleanup()
		return "", func() {}, errors.New("candidate bundle did not retain the frozen commit")
	}
	return repositoryPath, cleanup, nil
}

func readConflictDependencyManifest(repositoryPath, candidate string) ([]byte, error) {
	listing, err := exec.Command("git", "--git-dir="+repositoryPath, "ls-tree", candidate, "--", packages.InventoryConfigPath).Output()
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(listing))) == 0 {
		return []byte("absent"), nil
	}
	body, err := exec.Command("git", "--git-dir="+repositoryPath, "show", candidate+":"+packages.InventoryConfigPath).Output()
	if err != nil {
		return nil, err
	}
	return body, nil
}

func conflictAuthorship(actor auth.Credential) workspaces.ConflictAuthorship {
	return workspaces.ConflictAuthorship{ActorID: actor.UserID, AgentID: actor.AgentID}
}
func conflictPath(item workspaces.Workspace, candidate string) bool {
	if item.ConflictContext == nil || candidate == "" {
		return false
	}
	for _, file := range item.ConflictContext.Files {
		if file.Path == candidate {
			return true
		}
	}
	return false
}
func validConflictStatement(item workspaces.Workspace, body, uncertainty string, citations []workspaces.ConflictCitation) bool {
	if strings.TrimSpace(body) == "" || len(body) > 2000 || strings.TrimSpace(uncertainty) == "" || len(uncertainty) > 1000 || len(citations) < 1 || len(citations) > 10 {
		return false
	}
	for _, citation := range citations {
		if !validConflictCitation(item, citation) {
			return false
		}
	}
	return true
}
func validConflictCitation(item workspaces.Workspace, citation workspaces.ConflictCitation) bool {
	if !conflictPath(item, citation.Path) {
		return false
	}
	expected := map[string]string{"base": item.ConflictContext.BaseCommitID, "source": item.ConflictContext.Source.CommitID, "target": item.ConflictContext.Target.CommitID, "proposed": "workspace:" + item.ID}[citation.Side]
	return expected != "" && citation.Revision == expected && len(citation.EvidenceID) <= 200
}
func validPreservation(item workspaces.Workspace, records []workspaces.ConflictPreservation) bool {
	if len(records) < 1 || len(records) > 20 {
		return false
	}
	for _, record := range records {
		if !slices.Contains([]string{"acceptance_criterion", "design_decision", "migration", "user_behavior"}, record.Kind) || !slices.Contains([]string{"preserved", "intentionally_changed"}, record.Disposition) || strings.TrimSpace(record.Reference) == "" || len(record.Reference) > 500 || strings.TrimSpace(record.Rationale) == "" || len(record.Rationale) > 1000 || len(record.Citations) < 1 || len(record.Citations) > 10 {
			return false
		}
		for _, citation := range record.Citations {
			if !validConflictCitation(item, citation) {
				return false
			}
		}
	}
	return true
}
func writeConflictMutation(w http.ResponseWriter, item workspaces.Workspace, err error) {
	if errors.Is(err, workspaces.ErrConflict) {
		writeAPIError(w, 409, "conflict_workspace_changed", "meaning ledger changed; refresh before retrying")
		return
	}
	if errors.Is(err, workspaces.ErrInvalid) {
		writeAPIError(w, 422, "conflict_record_invalid", "the referenced conflict record is unavailable or already complete")
		return
	}
	if err != nil {
		writeAPIError(w, 500, "conflict_record_failed", "conflict record could not be saved")
		return
	}
	writeJSON(w, 200, item)
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
	bundleReader, err := os.Open(bundle)
	if err != nil {
		return err
	}
	defer bundleReader.Close()
	transfer := exec.Command("docker", "exec", "-i", container, "sh", "-c", "umask 077; cat > /workspace/.vivarium/conflicting-histories.bundle")
	transfer.Stdin = bundleReader
	if out, e := transfer.CombinedOutput(); e != nil {
		return errors.New(strings.TrimSpace(string(out)))
	}
	for _, args := range [][]string{{"git", "init"}, {"git", "fetch", ".vivarium/conflicting-histories.bundle", "refs/heads/*:refs/remotes/conflict/*"}} {
		command := append([]string{"exec", "--workdir", "/workspace", container}, args...)
		if out, e := exec.Command("docker", command...).CombinedOutput(); e != nil {
			return errors.New(strings.TrimSpace(string(out)))
		}
	}
	for name, expected := range map[string]string{"source": source, "target": target} {
		out, e := exec.Command("docker", "exec", "--workdir", "/workspace", container, "git", "rev-parse", "refs/remotes/conflict/"+name).CombinedOutput()
		if e != nil || strings.TrimSpace(string(out)) != expected {
			return errors.New("staged conflict/" + name + " does not match its frozen revision")
		}
	}
	if out, e := exec.Command("docker", "exec", "--workdir", "/workspace", container, "git", "reset", "--hard", "refs/remotes/conflict/target").CombinedOutput(); e != nil {
		return errors.New(strings.TrimSpace(string(out)))
	}
	return nil
}
