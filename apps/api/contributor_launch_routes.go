package main

import (
	"context"
	"errors"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/contributoropportunities"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/contributorpathways"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
)

type contributionLaunchResult struct {
	Fork      repositories.Repository `json:"fork"`
	Workspace workspaces.Workspace    `json:"workspace"`
}

// registerContributorLaunchRoutes turns a coordination-only match into an
// independently owned fork and reproducible workspace. It deliberately mints
// no upstream collaborator, Git, agent, or write authority.
func registerContributorLaunchRoutes(mux *http.ServeMux, git *storage.Store, repos *repositories.Store, opportunities *contributoropportunities.Store, pathways *contributorpathways.Store, workspaceStore *workspaces.Store, issueStore *issues.Store, proposalStore *proposals.Store, credentials *auth.Store) {
	mux.HandleFunc("POST /repositories/{id}/contribution-opportunities/{opportunity}/launch", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:write", false)
		if !ok {
			return
		}
		upstream, err := repos.GetByID(r.PathValue("id"))
		if err != nil {
			writeRepositoryError(w, err)
			return
		}
		if _, _, ok = authorizeRepositoryRead(w, r, repos, credentials, upstream.ID); !ok {
			return
		}
		var input struct {
			ExpectedVersion     int      `json:"expected_version"`
			ForkName            string   `json:"fork_name"`
			SampleAttachmentIDs []string `json:"sample_attachment_ids"`
		}
		if decodeJSON(r, &input) != nil || strings.TrimSpace(input.ForkName) == "" {
			writeAPIError(w, 400, "invalid_contribution_launch", "fork_name and expected_version are required")
			return
		}
		opportunity, err := opportunities.Get(upstream.ID, r.PathValue("opportunity"))
		if errors.Is(err, contributoropportunities.ErrNotFound) {
			writeAPIError(w, 404, "opportunity_not_found", "contribution opportunity not found")
			return
		}
		if err != nil {
			writeAPIError(w, 503, "opportunity_unavailable", "contribution opportunity could not be read")
			return
		}
		if opportunity.Version != input.ExpectedVersion {
			writeAPIError(w, 409, "opportunity_changed", "contribution opportunity changed")
			return
		}
		if opportunity.Status != "open" || opportunity.Claim == nil || opportunity.Claim.ActorID != actor.UserID {
			writeAPIError(w, 409, "opportunity_claim_required", "reserve this exact open opportunity version before launching")
			return
		}
		pathway, err := pathways.Current(upstream.ID)
		if err != nil {
			writeAPIError(w, 422, "contribution_guidance_missing", "maintainers must publish current contribution guidance before this opportunity can launch")
			return
		}
		upstreamGit, err := git.Open(upstream.ID)
		if err != nil {
			writeAPIError(w, 503, "repository_unavailable", "the opportunity repository could not be read")
			return
		}
		if _, err = upstreamGit.ReadCommit(storage.ObjectID(opportunity.Revision)); err != nil {
			writeAPIError(w, 422, "opportunity_revision_missing", "the recorded opportunity revision is no longer reproducible")
			return
		}
		definitionBytes, err := exec.Command("git", "--git-dir="+upstreamGit.Path(), "show", opportunity.Revision+":"+workspaces.DefinitionPath).Output()
		if err != nil {
			writeAPIError(w, 422, "workspace_definition_missing", "the recorded revision has no repository workspace definition")
			return
		}
		definition, err := parseWorkspaceDefinition(definitionBytes)
		if err != nil {
			writeAPIError(w, 422, "workspace_definition_invalid", err.Error())
			return
		}
		if len(input.SampleAttachmentIDs) > 0 && opportunity.Source.Kind != "issue" {
			writeAPIError(w, 422, "sample_data_not_permitted", "sample data may only come from the opportunity's issue evidence")
			return
		}
		var issue issues.Issue
		switch opportunity.Source.Kind {
		case "issue":
			issue, err = issueStore.Get(upstream.ID, opportunity.Source.ID)
			if err != nil {
				writeAPIError(w, 422, "opportunity_evidence_missing", "the issue evidence is no longer readable")
				return
			}
			if err = validateContributionSamples(issue, input.SampleAttachmentIDs); err != nil {
				writeAPIError(w, 422, "sample_data_not_permitted", err.Error())
				return
			}
		case "proposal":
			if proposalStore == nil {
				err = errors.New("proposal store unavailable")
			} else {
				_, err = proposalStore.Get(upstream.ID, opportunity.Source.ID)
			}
			if err != nil {
				writeAPIError(w, 422, "opportunity_evidence_missing", "the proposal evidence is no longer readable")
				return
			}
		case "task":
			if proposalStore == nil {
				err = errors.New("proposal store unavailable")
			} else {
				_, err = proposalStore.GetTask(upstream.ID, opportunity.Source.ParentID, opportunity.Source.ID)
			}
			if err != nil {
				writeAPIError(w, 422, "opportunity_evidence_missing", "the planned task evidence is no longer readable")
				return
			}
		}
		// A new fork cannot have a repository override yet, so it starts at the
		// platform default rather than inheriting upstream governance authority.
		policy := workspaces.DefaultPolicy()
		if definition.Resources.CPUs > policy.MaxCPUs || definition.Resources.MemoryMB > policy.MaxMemoryMB || definition.Resources.StorageMB > policy.MaxStorageMB {
			writeAPIError(w, 422, "workspace_policy_resources_exceeded", "workspace definition exceeds the effective fork resource policy")
			return
		}
		fork, err := repos.CreateFork(actor.UserID, upstream.ID, strings.TrimSpace(input.ForkName))
		if writeRepositoryError(w, err) {
			return
		}
		forkGit, err := git.Open(fork.ID)
		if err != nil {
			writeAPIError(w, 202, "contribution_launch_recovery_required", "the fork was retained but its workspace could not be opened")
			return
		}
		diagnostics := []string{}
		if len(pathway.Setup.VerificationCommands) == 0 {
			diagnostics = append(diagnostics, "Maintainers have not defined contribution verification commands.")
		}
		if pathway.Setup.WorkspacePath != "" && pathway.Setup.WorkspacePath != workspaces.DefinitionPath {
			diagnostics = append(diagnostics, "Contribution guidance names an obsolete workspace definition path: "+pathway.Setup.WorkspacePath)
		}
		context := &workspaces.ContributorContext{OpportunityID: opportunity.ID, OpportunityVersion: opportunity.Version, UpstreamRepositoryID: upstream.ID, PathwayVersion: pathway.Version, Guidance: pathway.Setup.Summary, Prerequisites: append([]string(nil), pathway.Prerequisites...), AcceptanceCriteria: []string{opportunity.ExpectedOutcome, opportunity.Scope}, EvidenceKind: opportunity.Source.Kind, EvidenceID: opportunity.Source.ID, EvidenceParentID: opportunity.Source.ParentID, SampleAttachmentIDs: append([]string(nil), input.SampleAttachmentIDs...), Diagnostics: diagnostics}
		created, err := workspaceStore.Create(workspaces.Workspace{RepositoryID: fork.ID, CommitID: opportunity.Revision, Definition: definition, Source: workspaces.Source{Kind: "repository", RepositoryID: fork.ID, UpstreamRepositoryID: upstream.ID, OpportunityID: opportunity.ID}, CreatorID: actor.UserID, Access: workspaces.Access{Role: "owner", Scopes: []string{"repositories:read", "repositories:write"}}, Policy: policy, PolicyScope: "repository", PolicyVersion: policy.Version, ContributorContext: context}, definitionBytes)
		if err != nil {
			writeJSON(w, 202, map[string]any{"fork": fork, "recovery_required": true, "message": "fork retained; workspace persistence must be retried"})
			return
		}
		steps, failed := provisionWorkspace(forkGit.Path(), workspaceStore.RuntimePath(created.ID), created.ID, opportunity.Revision, definition)
		if !failed && len(input.SampleAttachmentIDs) > 0 {
			if err = stageIssueInputs(created.ID, issue, input.SampleAttachmentIDs); err != nil {
				failed = true
				steps = append(steps, failedSetupStep("stage permitted sample data", nil, err))
			}
		}
		if !failed {
			steps, failed = verifyContributorSetup(created.ID, steps, pathway.Setup.VerificationCommands, time.Duration(definition.Resources.SetupSeconds)*time.Second)
		}
		if failed {
			_ = exec.Command("docker", "rm", "-f", "-v", "vivarium-workspace-"+created.ID).Run()
		}
		created, err = workspaceStore.Complete(created.ID, steps, failed)
		if err != nil {
			writeJSON(w, 202, map[string]any{"fork": fork, "workspace": created, "recovery_required": true, "message": "launch identities retained; setup evidence durability is uncertain"})
			return
		}
		w.Header().Set("Location", "/workspaces/"+created.ID)
		writeJSON(w, 201, contributionLaunchResult{Fork: fork, Workspace: created})
	})
}

func validateContributionSamples(issue issues.Issue, selected []string) error {
	if len(selected) > 10 {
		return errors.New("at most 10 permitted sample files may be preloaded")
	}
	available := map[string]issues.Attachment{}
	for _, item := range issue.Attachments {
		available[item.ID] = item
	}
	seen := map[string]bool{}
	for _, id := range selected {
		item, ok := available[id]
		if !ok || seen[id] || reproductionSecretLike(item.Name, item.Data) {
			return errors.New("sample data is missing, duplicated, or resembles secret material")
		}
		seen[id] = true
	}
	return nil
}

func verifyContributorSetup(workspaceID string, steps []workspaces.SetupStep, commands []string, timeout time.Duration) ([]workspaces.SetupStep, bool) {
	for _, command := range commands {
		started := time.Now().UTC()
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		output, err := exec.CommandContext(ctx, "docker", "exec", "vivarium-workspace-"+workspaceID, "sh", "-lc", command).CombinedOutput()
		cancel()
		step := workspaces.SetupStep{Command: command, State: "passed", Output: string(output), StartedAt: started, CompletedAt: time.Now().UTC()}
		if err != nil {
			step.State = "failed"
			step.ExitCode = 1
			steps = append(steps, step)
			return steps, true
		}
		steps = append(steps, step)
	}
	return steps, false
}
