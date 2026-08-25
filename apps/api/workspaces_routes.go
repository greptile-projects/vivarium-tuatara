package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/changesessions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/debugworkspaces"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/incidents"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/knowledgeanswers"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/supportthreads"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
)

func registerWorkspaceRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, proposalStore *proposals.Store, pullStore *pullrequests.Store, incidentStore *incidents.Store, issueStore *issues.Store, releaseStore *releases.Store, store *workspaces.Store, authStore *auth.Store, organizationStore *organizations.Store, checkStore *checkruns.Store, sessionStore *changesessions.Store, supportStores ...any) {
	var threadStore *supportthreads.Store
	var answerStore *knowledgeanswers.Store
	var debugStore *debugworkspaces.Store
	for _, candidate := range supportStores {
		switch value := candidate.(type) {
		case *supportthreads.Store:
			threadStore = value
		case *knowledgeanswers.Store:
			answerStore = value
		case *debugworkspaces.Store:
			debugStore = value
		}
	}
	registerWorkspaceGovernanceRoutes(mux, catalog, store, authStore, organizationStore)
	registerWorkspaceIDERoutes(mux, catalog, store, authStore)
	registerWorkspaceCollaborationRoutes(mux, catalog, store, authStore, organizationStore)
	registerWorkspaceCheckpointRoutes(mux, git, catalog, pullStore, store, authStore, checkStore, sessionStore)
	mux.HandleFunc("POST /workspaces", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, authStore, "repositories:write", false)
		if !ok {
			return
		}
		var input struct {
			RepositoryID       string            `json:"repository_id"`
			CommitID           string            `json:"commit_id"`
			Source             workspaces.Source `json:"source"`
			InputAttachmentIDs []string          `json:"input_attachment_ids"`
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
		if err = validateWorkspaceSource(input.Source, input.CommitID, actor.UserID, proposalStore, pullStore, incidentStore, issueStore, releaseStore, debugStore); err != nil {
			writeAPIError(w, 422, "workspace_source_invalid", err.Error())
			return
		}
		if input.Source.Kind == "support_verification" {
			if threadStore == nil || answerStore == nil {
				writeAPIError(w, 422, "workspace_source_invalid", "support verification is unavailable")
				return
			}
			thread, threadErr := threadStore.Get(input.RepositoryID, input.Source.SupportThreadID)
			answer, answerErr := answerStore.Get(input.RepositoryID, input.Source.AnswerID)
			validRevision := false
			if answerErr == nil {
				for _, revision := range answer.Revisions {
					if revision.ID == input.Source.AnswerRevisionID && answerCitesThread(revision, thread.ID) {
						validRevision = true
					}
				}
			}
			if threadErr != nil || answerErr != nil || !validRevision || thread.Target.Version == "" {
				writeAPIError(w, 422, "workspace_source_invalid", "support verification requires a readable versioned thread and exact citing answer revision")
				return
			}
		}
		var releaseExperimentClaim func()
		if input.Source.Kind == "decision_experiment" {
			existing, reused, release, claimErr := store.ClaimDecisionExperiment(input.RepositoryID, input.CommitID, actor.UserID, input.Source.DecisionID, input.Source.AlternativeID)
			if claimErr != nil {
				writeAPIError(w, 500, "workspace_create_failed", "experiment launch could not be reserved")
				return
			}
			if reused {
				w.Header().Set("Location", "/workspaces/"+existing.ID)
				writeJSON(w, 200, existing)
				return
			}
			releaseExperimentClaim = release
			defer releaseExperimentClaim()
			baseline, baselineErr := repo.ReadReference("refs/heads/" + repoMeta.DefaultBranch)
			if baselineErr != nil {
				writeAPIError(w, 503, "experiment_baseline_unavailable", "the current default-branch experiment baseline is unavailable")
				return
			}
			baselineDefinition, baselineErr := exec.Command("git", "--git-dir="+repo.Path(), "show", baseline.Target+":"+workspaces.DefinitionPath).Output()
			if baselineErr != nil {
				writeAPIError(w, 503, "experiment_baseline_unavailable", "the current default-branch experiment environment is unavailable")
				return
			}
			digest := sha256.Sum256(baselineDefinition)
			input.Source.DefaultBranchRevision, input.Source.DefaultDefinitionSHA256 = baseline.Target, hex.EncodeToString(digest[:])
		}
		var reasoning *workspaces.ReasoningContext
		if input.Source.Kind == "proposal_task" && proposalStore != nil {
			task, taskErr := proposalStore.GetTask(input.RepositoryID, input.Source.ProposalID, input.Source.TaskID)
			if taskErr == nil && task.Reasoning != nil {
				reasoning = &workspaces.ReasoningContext{AssessmentID: task.Reasoning.AssessmentID, AssessmentVersion: task.Reasoning.AssessmentVersion, DesignProposalID: task.Reasoning.DesignProposalID, DesignVersion: task.Reasoning.DesignProposalVersion, Revision: task.Reasoning.Revision, ExplanationID: task.Reasoning.ExplanationID, ConclusionEntryID: task.Reasoning.ConclusionEntryID}
				for _, item := range task.Reasoning.Items {
					reasoning.Items = append(reasoning.Items, workspaces.ReasoningItem{ID: item.ID, Kind: item.Kind, Summary: item.Summary, Status: item.Status})
				}
				for _, item := range task.Reasoning.Acknowledgements {
					reasoning.Acknowledgements = append(reasoning.Acknowledgements, workspaces.ReasoningAcknowledgement{RepositoryID: item.RepositoryID, OwnerID: item.OwnerID, AcknowledgedBy: item.AcknowledgedBy, Note: item.Note})
				}
			}
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
		repositoryPolicy, err := store.GetPolicy("repository", input.RepositoryID)
		if err != nil {
			writeAPIError(w, 500, "workspace_policy_unavailable", "workspace policy could not be read")
			return
		}
		policyScope := "repository"
		if repoMeta.OrganizationID != "" {
			organizationPolicy, policyErr := store.GetPolicy("organization", repoMeta.OrganizationID)
			if policyErr != nil {
				writeAPIError(w, 500, "workspace_policy_unavailable", "workspace policy could not be read")
				return
			}
			repositoryPolicy = workspaces.Constrain(organizationPolicy, repositoryPolicy)
			policyScope = "organization+repository"
		}
		if definition.Resources.CPUs > repositoryPolicy.MaxCPUs || definition.Resources.MemoryMB > repositoryPolicy.MaxMemoryMB || definition.Resources.StorageMB > repositoryPolicy.MaxStorageMB {
			writeAPIError(w, 422, "workspace_policy_resources_exceeded", "workspace definition exceeds the effective resource policy")
			return
		}
		role := "collaborator"
		if actor.UserID == repoMeta.OwnerID {
			role = "owner"
		}
		created, err := store.Create(workspaces.Workspace{RepositoryID: input.RepositoryID, OrganizationID: repoMeta.OrganizationID, CommitID: input.CommitID, Definition: definition, Source: input.Source, CreatorID: actor.UserID, Access: workspaces.Access{Role: role, Scopes: []string{"repositories:read", "repositories:write"}}, Policy: repositoryPolicy, PolicyScope: policyScope, PolicyVersion: repositoryPolicy.Version, Reasoning: reasoning, ReproductionInputAttachmentIDs: append([]string(nil), input.InputAttachmentIDs...)}, definitionBytes)
		if err != nil {
			writeAPIError(w, 500, "workspace_create_failed", "workspace could not be created")
			return
		}
		steps, failed := provisionWorkspace(repo.Path(), store.RuntimePath(created.ID), created.ID, input.CommitID, definition)
		if !failed && input.Source.Kind == "issue_reproduction" {
			issue, issueErr := issueStore.Get(input.RepositoryID, input.Source.IssueID)
			if issueErr != nil {
				failed, steps = true, append(steps, failedSetupStep("stage sanitized reproduction inputs", nil, issueErr))
			} else if stageErr := stageIssueInputs(created.ID, issue, input.InputAttachmentIDs); stageErr != nil {
				failed, steps = true, append(steps, failedSetupStep("stage sanitized reproduction inputs", nil, stageErr))
				_ = exec.Command("docker", "rm", "-f", "-v", "vivarium-workspace-"+created.ID).Run()
			}
		}
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
			if metaErr == nil && (meta.OwnerID == actor.UserID || collaborator || item.Source.Kind == "learning_exercise" && item.CreatorID == actor.UserID || conflictParticipantCurrent(catalog, item, workspacePrincipal(actor), actor.UserID, actor.RepositoryID)) {
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
		var input struct {
			Foundation string `json:"foundation"`
		}
		if err := decodeJSON(r, &input); err != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		updated, err := store.TransitionControlledAs(workspace.ID, workspacePrincipal(actor), actor.UserID, input.Foundation, "suspended")
		writeWorkspaceTransition(w, updated, err)
	})
	mux.HandleFunc("POST /workspaces/{workspace_id}/resume", func(w http.ResponseWriter, r *http.Request) {
		workspace, actor, ok := authorizeWorkspace(w, r, store, catalog, authStore, "repositories:write")
		if !ok {
			return
		}
		var input struct {
			Foundation string `json:"foundation"`
		}
		if err := decodeJSON(r, &input); err != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		updated, err := store.TransitionControlledAs(workspace.ID, workspacePrincipal(actor), actor.UserID, input.Foundation, "running")
		writeWorkspaceTransition(w, updated, err)
	})
}

func stageIssueInputs(workspaceID string, issue issues.Issue, selected []string) error {
	if len(selected) > 10 {
		return errors.New("at most 10 reproduction inputs are allowed")
	}
	attachments := map[string]issues.Attachment{}
	for _, attachment := range issue.Attachments {
		attachments[attachment.ID] = attachment
	}
	seen := map[string]bool{}
	for _, id := range selected {
		attachment, ok := attachments[id]
		if !ok || seen[id] || reproductionSecretLike(attachment.Name, attachment.Data) {
			return errors.New("input is missing or resembles credential material")
		}
		seen[id] = true
		name := reproductionInputName(id, attachment.Name)
		raw, err := base64.StdEncoding.DecodeString(attachment.Data)
		if err != nil {
			return err
		}
		cmd := exec.Command("docker", "exec", "-i", "vivarium-workspace-"+workspaceID, "sh", "-c", "mkdir -p /workspace/.vivarium/reproduction-inputs && umask 077 && cat > /workspace/.vivarium/reproduction-inputs/"+name)
		cmd.Stdin = bytes.NewReader(raw)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("stage input: %s: %w", out, err)
		}
	}
	return nil
}

func reproductionInputName(id, original string) string {
	sanitize := func(value string) string {
		return strings.Map(func(r rune) rune {
			if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("._-", r) {
				return r
			}
			return '_'
		}, value)
	}
	return sanitize(id) + "-" + sanitize(original)
}

func reproductionSecretLike(name, encoded string) bool {
	lower := strings.ToLower(name)
	for _, marker := range []string{".env", "credential", "secret", "token", "password", ".npmrc", ".netrc", ".pypirc", "pip.conf", "id_rsa", "id_ed25519", "id_ecdsa"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	raw, _ := base64.StdEncoding.DecodeString(encoded)
	text := strings.ToLower(string(raw))
	if strings.Contains(text, "private key-----") || strings.Contains(text, "authorization: bearer ") || strings.Contains(text, "authorization: basic ") {
		return true
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		separator := strings.IndexAny(line, "=:")
		if separator < 1 || strings.TrimSpace(line[separator+1:]) == "" {
			continue
		}
		key := strings.NewReplacer("-", "_", ".", "_", " ", "_").Replace(strings.TrimSpace(line[:separator]))
		for _, marker := range []string{"token", "secret", "password", "passwd", "api_key", "access_key", "private_key", "client_key", "auth_key"} {
			if strings.Contains(key, marker) {
				return true
			}
		}
		compactKey := strings.ReplaceAll(key, "_", "")
		for _, marker := range []string{"apikey", "accesskey", "privatekey", "clientkey", "authkey"} {
			if strings.Contains(compactKey, marker) {
				return true
			}
		}
	}
	return false
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
	invited := conflictParticipantCurrent(catalog, item, workspacePrincipal(actor), actor.UserID, actor.RepositoryID)
	learner := item.Source.Kind == "learning_exercise" && item.CreatorID == actor.UserID
	if err != nil || (actor.UserID != meta.OwnerID && !collaborator && !invited && !learner) || (item.Policy.Sharing == "private" && actor.UserID != item.CreatorID && actor.UserID != meta.OwnerID && !invited) {
		writeAPIError(w, 404, "workspace_not_found", "workspace not found")
		return item, auth.Credential{}, false
	}
	return item, actor, true
}

func workspacePrincipal(actor auth.Credential) string {
	if actor.AgentID != "" {
		return actor.AgentID
	}
	return actor.UserID
}

func conflictParticipantCurrent(catalog *repositories.Store, item workspaces.Workspace, principal, operator, credentialRepository string) bool {
	if !item.HasParticipant(principal) {
		return false
	}
	if item.ConflictContext == nil {
		return true
	}
	for _, target := range item.ConflictContext.PublicationTarget {
		if principal != operator && target.RepositoryID != credentialRepository {
			continue
		}
		meta, err := catalog.GetByID(target.RepositoryID)
		collaborator, _ := catalog.HasCollaborator(operator, target.RepositoryID)
		if err == nil && (meta.OwnerID == operator || collaborator) {
			return true
		}
	}
	return false
}
func writeWorkspaceTransition(w http.ResponseWriter, item workspaces.Workspace, err error) {
	if errors.Is(err, workspaces.ErrControl) {
		writeAPIError(w, 409, "workspace_control_required", "workspace lifecycle control is held by another participant or has expired")
		return
	}
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
	if len(d.Experiments) > 20 {
		return d, errors.New("at most 20 experiment commands are allowed")
	}
	seenExperiments := map[string]bool{}
	for _, experiment := range d.Experiments {
		name, command := strings.TrimSpace(experiment.Name), strings.TrimSpace(experiment.Command)
		if name == "" || command == "" || len(name) > 100 || len(command) > 2000 || seenExperiments[name] {
			return d, errors.New("experiment commands require unique bounded names and commands")
		}
		seenExperiments[name] = true
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
func validateWorkspaceSource(source workspaces.Source, commit, actor string, ps *proposals.Store, prs *pullrequests.Store, is *incidents.Store, issueStore *issues.Store, releaseStore *releases.Store, debugStore *debugworkspaces.Store) error {
	switch source.Kind {
	case "repository":
		return nil
	case "support_verification":
		if source.SupportThreadID == "" || source.AnswerID == "" || source.AnswerRevisionID == "" {
			return errors.New("support verification requires a thread and exact answer revision")
		}
		return nil
	case "decision_experiment":
		if strings.TrimSpace(source.DecisionID) == "" || strings.TrimSpace(source.AlternativeID) == "" {
			return errors.New("decision experiments require a decision and alternative")
		}
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
	case "issue_reproduction":
		if issueStore == nil {
			return errors.New("issues unavailable")
		}
		issue, e := issueStore.Get(source.RepositoryID, source.IssueID)
		if e != nil {
			return errors.New("issue not found")
		}
		if issue.ReleaseID != "" {
			if releaseStore == nil {
				return errors.New("affected release unavailable")
			}
			release, releaseErr := releaseStore.Get(source.RepositoryID, issue.ReleaseID)
			if releaseErr != nil || release.CommitID != commit || source.ReleaseID != issue.ReleaseID {
				return errors.New("revision must match the issue's attested release")
			}
		} else if source.ReleaseID != "" {
			return errors.New("issue has no attested release")
		}
		return nil
	case "debugging_reproduction":
		if strings.TrimSpace(source.DebuggingWorkspaceID) == "" || strings.TrimSpace(source.ReplayScenarioID) == "" {
			return errors.New("debugging reproductions require a debugging workspace and replay scenario")
		}
		if debugStore == nil {
			return errors.New("debugging reproductions unavailable")
		}
		debugging, err := debugStore.Get(source.RepositoryID, source.DebuggingWorkspaceID)
		if err != nil || !canReadDebuggingWorkspace(debugging, actor) {
			return errors.New("debugging replay scenario not found")
		}
		for _, scenario := range debugging.ReplayScenarios {
			if scenario.ID == source.ReplayScenarioID && scenario.CommitID == commit {
				return nil
			}
		}
		return errors.New("revision must match the frozen debugging replay scenario")
	default:
		return errors.New("source kind must be repository, proposal_task, pull_request, incident_repair, decision_experiment, issue_reproduction, or debugging_reproduction")
	}
}

func canReadDebuggingWorkspace(workspace debugworkspaces.Workspace, actor string) bool {
	if workspace.Audience != "restricted" || workspace.CreatedBy == actor {
		return true
	}
	for _, id := range workspace.AccessUserIDs {
		if id == actor {
			return true
		}
	}
	return false
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
