package main

import (
	"errors"
	"net/http"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/contributoropportunities"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/contributorpathways"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
)

type contributionPublicationAssessment struct {
	Ready               bool                               `json:"ready"`
	ProjectRequirements []pullrequests.ContributionFinding `json:"project_requirements"`
	CoachingNeeds       []pullrequests.ContributionFinding `json:"coaching_needs"`
	AcceptanceCriteria  []string                           `json:"acceptance_criteria"`
}

// registerContributorPublicationRoutes is the fork-to-upstream bridge for a
// guided contribution. The created pull remains an entirely ordinary pull.
func registerContributorPublicationRoutes(mux *http.ServeMux, git *storage.Store, repos *repositories.Store, pulls *pullrequests.Store, checks *checkruns.Store, opportunities *contributoropportunities.Store, pathways *contributorpathways.Store, ws *workspaces.Store, credentials *auth.Store) {
	mux.HandleFunc("GET /workspaces/{workspace_id}/checkpoints/{checkpoint_id}/contribution-publication", func(w http.ResponseWriter, r *http.Request) {
		workspace, checkpoint, actor, ok := authorizeContributionPublication(w, r, repos, ws, credentials)
		if !ok {
			return
		}
		assessment, err := assessContributionPublication(workspace, checkpoint, actor.UserID, nil, opportunities, pathways)
		if err != nil {
			writeAPIError(w, 503, "contribution_preflight_unavailable", "contribution requirements could not be checked")
			return
		}
		writeJSON(w, 200, assessment)
	})

	mux.HandleFunc("POST /workspaces/{workspace_id}/checkpoints/{checkpoint_id}/contribution-publication", func(w http.ResponseWriter, r *http.Request) {
		workspace, checkpoint, actor, ok := authorizeContributionPublication(w, r, repos, ws, credentials)
		if !ok {
			return
		}
		var in struct {
			Branch                 string   `json:"branch"`
			TargetBranch           string   `json:"target_branch"`
			Title                  string   `json:"title"`
			SatisfiedCriteria      []string `json:"satisfied_criteria"`
			MaintainerEditsAllowed bool     `json:"maintainer_edits_allowed"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		assessment, err := assessContributionPublication(workspace, checkpoint, actor.UserID, in.SatisfiedCriteria, opportunities, pathways)
		if err != nil {
			writeAPIError(w, 503, "contribution_preflight_unavailable", "contribution requirements could not be checked")
			return
		}
		if !assessment.Ready {
			writeJSON(w, 422, map[string]any{"error": map[string]string{"code": "contribution_requirements_missing", "message": "fix the blocking project requirements before publication"}, "assessment": assessment})
			return
		}
		in.Branch, in.TargetBranch, in.Title = strings.TrimSpace(in.Branch), strings.TrimSpace(in.TargetBranch), strings.TrimSpace(in.Title)
		if in.TargetBranch == "" {
			in.TargetBranch = "main"
		}
		if in.Title == "" {
			in.Title = checkpoint.Title
		}
		if in.Branch == "" || in.Branch == in.TargetBranch || len(in.Title) > 200 || exec.Command("git", "check-ref-format", "--branch", in.Branch).Run() != nil {
			writeAPIError(w, 422, "contribution_publication_invalid", "a valid fork branch, upstream target branch, and bounded title are required")
			return
		}
		if checkpoint.Publication != nil {
			writeJSON(w, 200, checkpoint.Public())
			return
		}
		forkGit, err := git.Open(workspace.RepositoryID)
		if err != nil {
			writeAPIError(w, 503, "fork_unavailable", "the contribution fork could not be opened")
			return
		}
		upstreamGit, err := git.Open(workspace.ContributorContext.UpstreamRepositoryID)
		if err != nil {
			writeAPIError(w, 503, "upstream_unavailable", "the upstream repository could not be opened")
			return
		}
		if _, err = upstreamGit.ReadReference("refs/heads/" + in.TargetBranch); err != nil {
			writeAPIError(w, 422, "contribution_publication_invalid", "target_branch must identify an upstream branch")
			return
		}
		checkpoint, release, err := ws.ClaimCheckpointPublication(workspace.ID, checkpoint.ID)
		if errors.Is(err, workspaces.ErrCheckpointConflict) {
			writeJSON(w, 200, checkpoint.Public())
			return
		}
		if err != nil {
			writeAPIError(w, 503, "contribution_publication_unavailable", "publication could not be reserved")
			return
		}
		defer release()
		if _, refErr := forkGit.ReadReference("refs/heads/" + in.Branch); refErr == nil {
			writeAPIError(w, 409, "workspace_branch_changed", "the contribution branch already exists")
			return
		}
		commitID, err := commitCheckpoint(forkGit.Path(), checkpoint, actor.UserID)
		if err != nil {
			writeAPIError(w, 500, "checkpoint_commit_failed", "checkpoint could not be committed")
			return
		}
		if err = forkGit.CreateReference(storage.Reference{Name: "refs/heads/" + in.Branch, Target: commitID}); err != nil {
			writeAPIError(w, 409, "workspace_branch_changed", "the contribution branch changed while publishing")
			return
		}
		contributors, commandIDs := workspacePublicationEvidence(checkpoint)
		evidence := contributionEvidence(workspace, assessment, in.SatisfiedCriteria)
		body := contributionPullBody(workspace, checkpoint, evidence)
		pull, err := pulls.CreateFrom(workspace.ContributorContext.UpstreamRepositoryID, workspace.RepositoryID, actor.UserID, in.Title, body, in.Branch, in.TargetBranch, nil)
		if err != nil {
			_ = forkGit.DeleteReferenceIfTarget("refs/heads/"+in.Branch, commitID)
			writeAPIError(w, 409, "contribution_pull_failed", "the fork branch could not enter upstream review")
			return
		}
		pull, err = pulls.LinkWorkspace(pull.RepositoryID, pull.ID, workspace.ID, checkpoint.ID, contributors, commandIDs)
		if err == nil {
			pull, err = pulls.LinkContributionEvidence(pull.RepositoryID, pull.ID, evidence)
		}
		if err == nil && in.MaintainerEditsAllowed {
			pull, err = pulls.UpdatePolicy(pull.RepositoryID, pull.ID, true)
		}
		publication := workspaces.Publication{Branch: in.Branch, CommitID: commitID, PullRequestID: pull.ID, ContributorIDs: contributors, CommandIDs: commandIDs, PublishedBy: actor.UserID, PublishedAt: time.Now().UTC()}
		checkpoint, recordErr := ws.RecordCheckpointPublication(workspace.ID, checkpoint.ID, publication)
		if err != nil || recordErr != nil {
			w.Header().Set("Vivarium-Recovery-Publication", "pending")
			writeJSON(w, 202, map[string]any{"checkpoint": checkpoint.Public(), "pull_request": pull, "assessment": assessment})
			return
		}
		startCheckRuns(git, checks, pull)
		w.Header().Set("Location", "/repositories/"+pull.RepositoryID+"/pulls/"+pull.ID)
		writeJSON(w, 201, map[string]any{"checkpoint": checkpoint.Public(), "pull_request": pull, "assessment": assessment})
	})
}

func authorizeContributionPublication(w http.ResponseWriter, r *http.Request, repos *repositories.Store, ws *workspaces.Store, credentials *auth.Store) (workspaces.Workspace, workspaces.Checkpoint, auth.Credential, bool) {
	actor, ok := authenticateRequest(w, r, credentials, "repositories:write", false)
	if !ok {
		return workspaces.Workspace{}, workspaces.Checkpoint{}, auth.Credential{}, false
	}
	item, err := ws.Get(r.PathValue("workspace_id"))
	if err != nil || item.ContributorContext == nil || item.CreatorID != actor.UserID {
		writeAPIError(w, 404, "contribution_workspace_not_found", "contribution workspace not found")
		return item, workspaces.Checkpoint{}, actor, false
	}
	owned, _ := repos.GetByID(item.RepositoryID)
	if owned.OwnerID != actor.UserID {
		writeAPIError(w, 403, "contribution_publication_forbidden", "only the fork owner can publish this contribution")
		return item, workspaces.Checkpoint{}, actor, false
	}
	if _, _, ok = authorizeRepositoryRead(w, r, repos, credentials, item.ContributorContext.UpstreamRepositoryID); !ok {
		return item, workspaces.Checkpoint{}, actor, false
	}
	checkpoint, err := ws.CheckpointSnapshot(item.ID, r.PathValue("checkpoint_id"))
	if err != nil {
		writeAPIError(w, 404, "checkpoint_not_found", "checkpoint not found")
		return item, checkpoint, actor, false
	}
	return item, checkpoint, actor, true
}

func assessContributionPublication(item workspaces.Workspace, checkpoint workspaces.Checkpoint, actor string, satisfied []string, opportunities *contributoropportunities.Store, pathways *contributorpathways.Store) (contributionPublicationAssessment, error) {
	result := contributionPublicationAssessment{ProjectRequirements: []pullrequests.ContributionFinding{}, CoachingNeeds: []pullrequests.ContributionFinding{}, AcceptanceCriteria: append([]string(nil), item.ContributorContext.AcceptanceCriteria...)}
	pathway, err := pathways.Current(item.ContributorContext.UpstreamRepositoryID)
	if err != nil {
		return result, err
	}
	if pathway.Version != item.ContributorContext.PathwayVersion {
		result.ProjectRequirements = append(result.ProjectRequirements, pullrequests.ContributionFinding{Code: "pathway_changed", Message: "The contribution pathway changed after this workspace launched.", Fix: "Review and acknowledge the current pathway, then relaunch or rebase the guided work."})
	}
	acks, err := pathways.Acknowledgements(item.ContributorContext.UpstreamRepositoryID)
	if err != nil {
		return result, err
	}
	acknowledged := false
	for _, ack := range acks {
		if ack.ActorID == actor && ack.Version == item.ContributorContext.PathwayVersion {
			acknowledged = true
		}
	}
	if !acknowledged {
		result.ProjectRequirements = append(result.ProjectRequirements, pullrequests.ContributionFinding{Code: "pathway_not_acknowledged", Message: "The frozen contribution pathway has not been acknowledged.", Fix: "Acknowledge pathway revision before publishing."})
	}
	if current, getErr := opportunities.Get(item.ContributorContext.UpstreamRepositoryID, item.ContributorContext.OpportunityID); getErr != nil {
		return result, getErr
	} else if current.Version != item.ContributorContext.OpportunityVersion || current.Status != "in_progress" {
		result.ProjectRequirements = append(result.ProjectRequirements, pullrequests.ContributionFinding{Code: "opportunity_changed", Message: "The opportunity is no longer the launched revision.", Fix: "Ask a maintainer to reconcile the opportunity before publication."})
	}
	if len(checkpoint.Files) == 0 {
		result.ProjectRequirements = append(result.ProjectRequirements, pullrequests.ContributionFinding{Code: "empty_checkpoint", Message: "The checkpoint contains no project files.", Fix: "Save the intended project changes in a new checkpoint."})
	}
	for _, step := range item.Setup {
		if step.State != "passed" || step.ExitCode != 0 {
			result.ProjectRequirements = append(result.ProjectRequirements, pullrequests.ContributionFinding{Code: "setup_failed", Message: "A required setup or verification command did not pass: " + step.Command, Fix: "Run the command successfully and relaunch setup evidence."})
		}
	}
	wanted := map[string]bool{}
	for _, criterion := range satisfied {
		wanted[strings.TrimSpace(criterion)] = true
	}
	for _, criterion := range item.ContributorContext.AcceptanceCriteria {
		if !wanted[criterion] {
			result.ProjectRequirements = append(result.ProjectRequirements, pullrequests.ContributionFinding{Code: "criterion_unconfirmed", Message: criterion, Fix: "Confirm this criterion is met or update the work before publishing."})
		}
	}
	for _, diagnostic := range item.ContributorContext.Diagnostics {
		result.CoachingNeeds = append(result.CoachingNeeds, pullrequests.ContributionFinding{Code: "launch_diagnostic", Message: diagnostic, Fix: "Discuss this with a mentor if it affects confidence in the change."})
	}
	for _, entry := range item.ContributorContext.Help.Entries {
		if entry.Status == "open" {
			result.CoachingNeeds = append(result.CoachingNeeds, pullrequests.ContributionFinding{Code: "open_guidance", Message: entry.Body, Fix: "Resolve or carry this coaching thread into review."})
		}
	}
	result.Ready = len(result.ProjectRequirements) == 0
	return result, nil
}

func contributionEvidence(item workspaces.Workspace, assessment contributionPublicationAssessment, satisfied []string) pullrequests.ContributionEvidence {
	setup := make([]pullrequests.ContributionSetup, 0, len(item.Setup))
	for _, step := range item.Setup {
		setup = append(setup, pullrequests.ContributionSetup{Command: step.Command, State: step.State, ExitCode: step.ExitCode})
	}
	mentor, agent := []string{}, []string{}
	for _, entry := range item.ContributorContext.Help.Entries {
		if entry.AgentID != "" {
			agent = append(agent, entry.ID)
		} else {
			mentor = append(mentor, entry.ID)
		}
	}
	sort.Strings(mentor)
	sort.Strings(agent)
	return pullrequests.ContributionEvidence{OpportunityID: item.ContributorContext.OpportunityID, OpportunityVersion: item.ContributorContext.OpportunityVersion, PathwayVersion: item.ContributorContext.PathwayVersion, UpstreamRevision: item.CommitID, SetupEvidence: setup, MentorGuidanceIDs: mentor, AgentAssistanceIDs: agent, AcceptanceCriteria: append([]string(nil), assessment.AcceptanceCriteria...), SatisfiedCriteria: append([]string(nil), satisfied...), ProjectRequirements: assessment.ProjectRequirements, CoachingNeeds: assessment.CoachingNeeds}
}

func contributionPullBody(item workspaces.Workspace, checkpoint workspaces.Checkpoint, evidence pullrequests.ContributionEvidence) string {
	var b strings.Builder
	b.WriteString("Guided contribution for opportunity " + evidence.OpportunityID + "\n\n")
	b.WriteString("Pathway revision: ")
	b.WriteString(intString(evidence.PathwayVersion))
	b.WriteString("\n")
	b.WriteString("Upstream revision: " + evidence.UpstreamRevision + "\nWorkspace: " + item.ID + "\nCheckpoint: " + checkpoint.ID + "\n\nAcceptance criteria:\n")
	for _, criterion := range evidence.AcceptanceCriteria {
		b.WriteString("- [x] " + criterion + "\n")
	}
	if len(evidence.CoachingNeeds) > 0 {
		b.WriteString("\nCoaching context (non-blocking):\n")
		for _, need := range evidence.CoachingNeeds {
			b.WriteString("- " + need.Message + "\n")
		}
	}
	b.WriteString("\nThis pull uses ordinary discussion, review, reproduction, checks, acknowledgements, queue, and merge permissions.")
	return b.String()
}

func intString(v int) string { // small local helper avoids formatting user text
	if v == 0 {
		return "0"
	}
	digits := [20]byte{}
	i := len(digits)
	for v > 0 {
		i--
		digits[i] = byte('0' + v%10)
		v /= 10
	}
	return string(digits[i:])
}
