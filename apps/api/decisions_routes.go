package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/activities"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/decisions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/explanations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/incidents"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/relationships"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
)

func organizationOwner(v organizations.Organization, id string) bool {
	for _, member := range v.Members {
		if member.UserID == id && member.Role == "owner" {
			return true
		}
	}
	return false
}
func activeDecisionPolicy(v organizations.Organization, policyID, rule, repositoryID string) bool {
	for _, p := range v.Policies {
		if p.ID != policyID || p.Status != "active" {
			continue
		}
		applies := false
		for _, target := range p.Targets {
			applies = applies || target.Kind == "organization" || (target.Kind == "repository" && target.ID == repositoryID)
			if target.Kind == "team" {
				for _, team := range v.Teams {
					if team.ID == target.ID {
						for _, responsibility := range team.Responsibilities {
							applies = applies || responsibility.RepositoryID == repositoryID
						}
					}
				}
			}
		}
		if !applies {
			continue
		}
		switch rule {
		case "minimum_reviews":
			return p.Rules.MinimumReviews > 0
		case "required_checks":
			return len(p.Rules.RequiredChecks) > 0
		case "promotion_approvals":
			return p.Rules.PromotionApprovals > 0
		case "integration":
			return p.Rules.Integration != ""
		case "release_provenance":
			return p.Rules.ReleaseProvenance != ""
		case "dependency_use":
			return p.Rules.DependencyUse != ""
		case "agent_authority":
			return p.Rules.AgentAuthority != ""
		case "repository_visibility":
			return p.Rules.RepositoryVisibility != ""
		}
	}
	return false
}

type decisionCreateInput struct {
	Source decisions.Source `json:"source"`
	Scope  decisions.Scope  `json:"scope"`
}
type decisionUpdateInput struct {
	ExpectedVersion int             `json:"expected_version"`
	Scope           decisions.Scope `json:"scope"`
	Summary         string          `json:"summary"`
}
type decisionDiscussionInput struct {
	Body string `json:"body"`
}
type decisionAlternativeInput struct {
	ExpectedVersion int                   `json:"expected_version"`
	Alternative     decisions.Alternative `json:"alternative"`
}
type decisionResearchCredentialInput struct {
	ExpiresIn     int    `json:"expires_in"`
	AlternativeID string `json:"alternative_id"`
}
type decisionExperimentInput struct {
	AlternativeID string `json:"alternative_id"`
	WorkspaceID   string `json:"workspace_id"`
}
type decisionExperimentEvidenceInput struct {
	ExpectedVersion int                          `json:"expected_version"`
	Evidence        decisions.ExperimentEvidence `json:"evidence"`
}
type decisionApprovalInput struct {
	ExpectedVersion int                       `json:"expected_version"`
	Request         decisions.ApprovalRequest `json:"request"`
}
type decisionApprovalResponseInput struct {
	Decision string `json:"decision"`
	Note     string `json:"note"`
}
type decisionPublishInput struct {
	ExpectedVersion int                  `json:"expected_version"`
	Commitment      decisions.Commitment `json:"commitment"`
}

func registerDecisionRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, identities *users.Store, store *decisions.Store, activity *activities.Store, proposalStore *proposals.Store, explanationStore *explanations.Store, incidentStore *incidents.Store, relationshipStore *relationships.Store, organizationStore *organizations.Store, workspaceStore *workspaces.Store) {
	mux.HandleFunc("POST /repositories/{id}/decisions", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in decisionCreateInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "source and scope are required")
			return
		}
		if in.Source.Kind == "repository" {
			in.Source.ResourceID = r.PathValue("id")
		}
		if !decisionSourceExists(w, r.PathValue("id"), actor.UserID, in.Source, proposalStore, explanationStore, incidentStore, relationshipStore, organizationStore) {
			return
		}
		if !validateDecisionUsers(w, identities, in.Scope) {
			return
		}
		v, err := store.Create(r.PathValue("id"), in.Source, in.Scope, actor.UserID)
		if writeDecisionError(w, err) {
			return
		}
		recordActivity(activity, catalog, activities.Event{Kind: "decision.opened", ActorID: actor.UserID, RepositoryID: v.RepositoryID, ResourceType: "decision", ResourceID: v.ID, ResourceTitle: v.Scope.Question})
		writeJSON(w, 201, v)
	})
	mux.HandleFunc("GET /decisions", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		repos, err := catalog.ListAccessible(actor.UserID)
		if err != nil {
			writeAPIError(w, 500, "decision_storage_unavailable", "decisions could not be loaded")
			return
		}
		allowed := map[string]bool{}
		for _, x := range repos {
			allowed[x.ID] = true
		}
		all, err := store.List()
		if writeDecisionError(w, err) {
			return
		}
		out := []decisions.Decision{}
		repoFilter := strings.TrimSpace(r.URL.Query().Get("repository_id"))
		sourceKind := strings.TrimSpace(r.URL.Query().Get("source_kind"))
		sourceID := strings.TrimSpace(r.URL.Query().Get("source_id"))
		for _, x := range all {
			if allowed[x.RepositoryID] && (repoFilter == "" || x.RepositoryID == repoFilter) && (sourceKind == "" || x.Source.Kind == sourceKind) && (sourceID == "" || x.Source.ResourceID == sourceID) {
				out = append(out, x)
			}
		}
		for i := range out {
			out[i] = projectDecisionExperiments(out[i], git, catalog, workspaceStore)
		}
		writeJSON(w, 200, map[string]any{"decisions": out})
	})
	mux.HandleFunc("GET /decisions/{id}", func(w http.ResponseWriter, r *http.Request) {
		v, actor, ok := authorizeDecision(w, r, catalog, credentials, store, "repositories:read")
		if !ok {
			return
		}
		_ = actor
		writeJSON(w, 200, projectDecisionExperiments(v, git, catalog, workspaceStore))
	})
	mux.HandleFunc("PUT /decisions/{id}", func(w http.ResponseWriter, r *http.Request) {
		_, actor, ok := authorizeDecision(w, r, catalog, credentials, store, "repositories:write")
		if !ok {
			return
		}
		var in decisionUpdateInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "expected_version, scope, and summary are required")
			return
		}
		if !validateDecisionUsers(w, identities, in.Scope) {
			return
		}
		v, err := store.Update(r.PathValue("id"), actor.UserID, in.ExpectedVersion, in.Scope, in.Summary)
		if writeDecisionError(w, err) {
			return
		}
		recordActivity(activity, catalog, activities.Event{Kind: "decision.scope_changed", ActorID: actor.UserID, RepositoryID: v.RepositoryID, ResourceType: "decision", ResourceID: v.ID, ResourceTitle: v.Scope.Question})
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("POST /decisions/{id}/discussion", func(w http.ResponseWriter, r *http.Request) {
		_, actor, ok := authorizeDecision(w, r, catalog, credentials, store, "repositories:write")
		if !ok {
			return
		}
		var in decisionDiscussionInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "body is required")
			return
		}
		v, err := store.Discuss(r.PathValue("id"), actor.UserID, in.Body)
		if writeDecisionError(w, err) {
			return
		}
		recordActivity(activity, catalog, activities.Event{Kind: "decision.discussed", ActorID: actor.UserID, RepositoryID: v.RepositoryID, ResourceType: "decision", ResourceID: v.ID, ResourceTitle: v.Scope.Question})
		writeJSON(w, 201, v)
	})
	mux.HandleFunc("POST /decisions/{id}/alternatives", func(w http.ResponseWriter, r *http.Request) {
		_, actor, ok := authorizeDecision(w, r, catalog, credentials, store, "repositories:write")
		if !ok {
			return
		}
		var in decisionAlternativeInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "expected_version and alternative are required")
			return
		}
		v, err := store.AddAlternative(r.PathValue("id"), actor.UserID, in.ExpectedVersion, in.Alternative)
		if writeDecisionError(w, err) {
			return
		}
		recordActivity(activity, catalog, activities.Event{Kind: "decision.alternative_proposed", ActorID: actor.UserID, RepositoryID: v.RepositoryID, ResourceType: "decision", ResourceID: v.ID, ResourceTitle: v.Scope.Question})
		writeJSON(w, 201, v)
	})
	mux.HandleFunc("POST /decisions/{id}/approval-requests", func(w http.ResponseWriter, r *http.Request) {
		v, actor, ok := authorizeDecision(w, r, catalog, credentials, store, "repositories:write")
		if !ok {
			return
		}
		var in decisionApprovalInput
		if decodeJSON(r, &in) != nil || actor.UserID != v.Scope.OwnerID {
			writeAPIError(w, 403, "decision_owner_required", "only the accountable decision owner can request approval")
			return
		}
		request := in.Request
		if request.Kind == "affected_owner" {
			affected := false
			for _, resource := range v.Scope.AffectedResources {
				affected = affected || resource.RepositoryID == request.RepositoryID
			}
			repo, err := catalog.GetByID(request.RepositoryID)
			if err != nil || !affected || repo.OwnerID != request.ApproverID {
				writeAPIError(w, 422, "invalid_affected_owner", "approval must name the current owner of an affected repository")
				return
			}
		} else if request.Kind == "policy" {
			repo, err := catalog.GetByID(v.RepositoryID)
			if err != nil || repo.OrganizationID == "" || organizationStore == nil {
				writeAPIError(w, 422, "invalid_policy_approval", "decision repository has no organization policy")
				return
			}
			org, err := organizationStore.Get(repo.OrganizationID)
			if err != nil || !activeDecisionPolicy(org, request.PolicyID, request.PolicyRule, v.RepositoryID) || !organizationOwner(org, request.ApproverID) {
				writeAPIError(w, 422, "invalid_policy_approval", "approval must cite an applicable active policy and current organization owner")
				return
			}
		}
		updated, err := store.RequestApproval(v.ID, actor.UserID, in.ExpectedVersion, request)
		if writeDecisionError(w, err) {
			return
		}
		recordActivity(activity, catalog, activities.Event{Kind: "decision.approval_requested", ActorID: actor.UserID, RepositoryID: v.RepositoryID, ResourceType: "decision", ResourceID: v.ID, ResourceTitle: v.Scope.Question})
		writeJSON(w, 201, updated)
	})
	mux.HandleFunc("POST /decisions/{id}/approval-requests/{request_id}/response", func(w http.ResponseWriter, r *http.Request) {
		v, actor, ok := authorizeDecision(w, r, catalog, credentials, store, "repositories:write")
		if !ok {
			return
		}
		var in decisionApprovalResponseInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "decision is required")
			return
		}
		updated, err := store.RespondApproval(v.ID, r.PathValue("request_id"), actor.UserID, in.Decision, in.Note)
		if writeDecisionError(w, err) {
			return
		}
		recordActivity(activity, catalog, activities.Event{Kind: "decision.approval_" + in.Decision, ActorID: actor.UserID, RepositoryID: v.RepositoryID, ResourceType: "decision", ResourceID: v.ID, ResourceTitle: v.Scope.Question})
		writeJSON(w, 200, updated)
	})
	mux.HandleFunc("POST /decisions/{id}/publish", func(w http.ResponseWriter, r *http.Request) {
		v, actor, ok := authorizeDecision(w, r, catalog, credentials, store, "repositories:write")
		if !ok {
			return
		}
		if actor.UserID != v.Scope.OwnerID {
			writeAPIError(w, 403, "decision_owner_required", "only the accountable decision owner can publish")
			return
		}
		var in decisionPublishInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "expected_version and commitment are required")
			return
		}
		updated, err := store.Publish(v.ID, actor.UserID, in.ExpectedVersion, in.Commitment)
		if writeDecisionError(w, err) {
			return
		}
		recordActivity(activity, catalog, activities.Event{Kind: "decision.published", ActorID: actor.UserID, RepositoryID: v.RepositoryID, ResourceType: "decision", ResourceID: v.ID, ResourceTitle: v.Scope.Question})
		writeJSON(w, 201, updated)
	})
	mux.HandleFunc("POST /decisions/{id}/research-credentials", func(w http.ResponseWriter, r *http.Request) {
		v, actor, ok := authorizeDecision(w, r, catalog, credentials, store, "repositories:write")
		if !ok {
			return
		}
		var in decisionResearchCredentialInput
		if decodeJSON(r, &in) != nil || in.ExpiresIn < 60 || in.ExpiresIn > 86400 {
			writeAPIError(w, 400, "invalid_request", "expires_in must be between 60 and 86400 seconds")
			return
		}
		selected := false
		for _, alternative := range v.Alternatives {
			selected = selected || alternative.ID == in.AlternativeID && alternative.SupersededBy == ""
		}
		if !selected {
			writeAPIError(w, 400, "invalid_alternative", "a current decision alternative must be selected")
			return
		}
		issued, err := credentials.IssueBound(actor.UserID, auth.API, "Decision research "+v.ID+":"+in.AlternativeID, []string{"decisions:research", "repositories:read"}, time.Duration(in.ExpiresIn)*time.Second, v.RepositoryID, "")
		if err != nil {
			writeAPIError(w, 500, "credential_storage_unavailable", "research credential could not be issued")
			return
		}
		writeJSON(w, 201, issued)
	})
	mux.HandleFunc("POST /decisions/{id}/findings", func(w http.ResponseWriter, r *http.Request) {
		v, err := store.Get(r.PathValue("id"))
		if writeDecisionError(w, err) {
			return
		}
		actor, ok := authenticateRequest(w, r, credentials, "decisions:research", false)
		if !ok {
			return
		}
		prefix := "Decision research " + v.ID + ":"
		if actor.RepositoryID != v.RepositoryID || !strings.HasPrefix(actor.Name, prefix) {
			writeAPIError(w, 404, "decision_not_found", "decision not found")
			return
		}
		var in decisions.Finding
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "finding and citations are required")
			return
		}
		if in.AlternativeID != strings.TrimPrefix(actor.Name, prefix) {
			writeAPIError(w, 404, "alternative_not_found", "alternative not found")
			return
		}
		v, err = store.AddFinding(v.ID, actor.UserID, in)
		if writeDecisionError(w, err) {
			return
		}
		writeJSON(w, 201, v)
	})
	mux.HandleFunc("POST /decisions/{id}/experiments", func(w http.ResponseWriter, r *http.Request) {
		v, actor, ok := authorizeDecision(w, r, catalog, credentials, store, "repositories:write")
		if !ok {
			return
		}
		var in decisionExperimentInput
		if decodeJSON(r, &in) != nil || workspaceStore == nil {
			writeAPIError(w, 400, "invalid_request", "alternative_id and workspace_id are required")
			return
		}
		workspace, err := workspaceStore.Get(strings.TrimSpace(in.WorkspaceID))
		if err != nil || workspace.State != "running" || workspace.RepositoryID != v.RepositoryID || workspace.Source.Kind != "decision_experiment" || workspace.Source.DecisionID != v.ID || workspace.Source.AlternativeID != in.AlternativeID {
			writeAPIError(w, 422, "experiment_workspace_invalid", "workspace must be an exact decision-experiment workspace for this alternative")
			return
		}
		for _, existing := range v.Experiments {
			if existing.WorkspaceID == workspace.ID && existing.AlternativeID == in.AlternativeID {
				writeJSON(w, 200, projectDecisionExperiments(v, git, catalog, workspaceStore))
				return
			}
		}
		commands := []string{}
		for _, command := range workspace.Definition.Experiments {
			commands = append(commands, command.Command)
		}
		v, err = store.LaunchExperiment(v.ID, actor.UserID, in.AlternativeID, workspace.ID, workspace.CommitID, workspace.DefinitionSHA256, workspace.Source.DefaultBranchRevision, workspace.Source.DefaultDefinitionSHA256, commands)
		if writeDecisionError(w, err) {
			return
		}
		recordActivity(activity, catalog, activities.Event{Kind: "decision.experiment_launched", ActorID: actor.UserID, RepositoryID: v.RepositoryID, ResourceType: "decision", ResourceID: v.ID, ResourceTitle: v.Scope.Question})
		writeJSON(w, 201, projectDecisionExperiments(v, git, catalog, workspaceStore))
	})
	mux.HandleFunc("POST /decisions/{id}/experiments/{experiment_id}/evidence", func(w http.ResponseWriter, r *http.Request) {
		v, actor, ok := authorizeDecision(w, r, catalog, credentials, store, "repositories:write")
		if !ok {
			return
		}
		var in decisionExperimentEvidenceInput
		if decodeJSON(r, &in) != nil || workspaceStore == nil {
			writeAPIError(w, 400, "invalid_request", "expected_version and evidence are required")
			return
		}
		var experiment *decisions.Experiment
		for i := range v.Experiments {
			if v.Experiments[i].ID == r.PathValue("experiment_id") {
				experiment = &v.Experiments[i]
			}
		}
		if experiment == nil {
			writeAPIError(w, 404, "experiment_not_found", "experiment not found")
			return
		}
		workspace, err := workspaceStore.Get(experiment.WorkspaceID)
		if err != nil {
			writeAPIError(w, 503, "experiment_workspace_unavailable", "experiment workspace is unavailable")
			return
		}
		commandIDs := map[string]bool{}
		allowedCommands := map[string]bool{}
		for _, declared := range workspace.Definition.Experiments {
			digest := sha256.Sum256([]byte(declared.Command))
			allowedCommands[hex.EncodeToString(digest[:])] = true
		}
		for _, command := range workspace.Commands {
			commandIDs[command.ID] = allowedCommands[command.CommandSHA256]
		}
		for _, id := range in.Evidence.CommandIDs {
			if !commandIDs[id] {
				writeAPIError(w, 422, "experiment_evidence_invalid", "every command must belong to the experiment workspace")
				return
			}
		}
		for _, id := range in.Evidence.CheckpointIDs {
			checkpoint, checkpointErr := workspaceStore.GetCheckpoint(workspace.ID, id)
			if checkpointErr != nil || checkpoint.WorkspaceID != workspace.ID {
				writeAPIError(w, 422, "experiment_evidence_invalid", "every checkpoint must belong to the experiment workspace")
				return
			}
		}
		usage := workspaces.Usage(workspace, time.Now().UTC())
		in.Evidence.CPUSeconds, in.Evidence.MemoryMBHours, in.Evidence.StorageMBHours = usage.CPUSeconds, usage.MemoryMBHours, usage.StorageMBHours
		v, err = store.AttachExperimentEvidence(v.ID, experiment.ID, actor.UserID, in.ExpectedVersion, in.Evidence)
		if writeDecisionError(w, err) {
			return
		}
		writeJSON(w, 201, projectDecisionExperiments(v, git, catalog, workspaceStore))
	})
}

func projectDecisionExperiments(v decisions.Decision, git *storage.Store, catalog *repositories.Store, workspaceStore *workspaces.Store) decisions.Decision {
	if git == nil || catalog == nil || workspaceStore == nil {
		return v
	}
	current, currentDefinition, baselineErr := currentDecisionBaseline(v.RepositoryID, git, catalog)
	for i := range v.Experiments {
		experiment := &v.Experiments[i]
		reasons := []string{}
		if baselineErr != nil {
			reasons = append(reasons, "current code revision is unavailable")
		} else if experiment.DefaultBranchRevision == "" || current != experiment.DefaultBranchRevision {
			reasons = append(reasons, "default-branch code changed after the experiment")
		}
		if baselineErr == nil && (experiment.DefaultDefinitionSHA256 == "" || currentDefinition != experiment.DefaultDefinitionSHA256) {
			reasons = append(reasons, "workspace dependencies or environment changed after the experiment")
		}
		workspace, err := workspaceStore.Get(experiment.WorkspaceID)
		if err != nil {
			reasons = append(reasons, "workspace environment is unavailable")
		} else {
			if workspace.DefinitionSHA256 != experiment.DefinitionSHA256 {
				reasons = append(reasons, "workspace environment definition changed")
			}
			if workspace.RebuildRequired {
				reasons = append(reasons, workspace.RebuildReasons...)
			}
		}
		experiment.Invalidated, experiment.InvalidationReasons = len(reasons) > 0, reasons
	}
	return v
}

func currentDecisionBaseline(repositoryID string, git *storage.Store, catalog *repositories.Store) (string, string, error) {
	if git == nil || catalog == nil {
		return "", "", errors.New("baseline unavailable")
	}
	meta, err := catalog.GetByID(repositoryID)
	if err != nil {
		return "", "", err
	}
	repo, err := git.Open(repositoryID)
	if err != nil {
		return "", "", err
	}
	ref, err := repo.ReadReference("refs/heads/" + meta.DefaultBranch)
	if err != nil {
		return "", "", err
	}
	definition, err := exec.Command("git", "--git-dir="+repo.Path(), "show", ref.Target+":"+workspaces.DefinitionPath).Output()
	if err != nil {
		return "", "", err
	}
	digest := sha256.Sum256(definition)
	return ref.Target, hex.EncodeToString(digest[:]), nil
}

func validateDecisionUsers(w http.ResponseWriter, identities *users.Store, scope decisions.Scope) bool {
	if identities == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "decision_identity_unavailable", "decision identities are unavailable")
		return false
	}
	ids := append([]decisions.Participant(nil), scope.Participants...)
	ownerFound := false
	for _, participant := range ids {
		if participant.UserID == scope.OwnerID {
			ownerFound = true
		}
		if _, err := identities.Get(participant.UserID); err != nil {
			if errors.Is(err, users.ErrNotFound) {
				writeAPIError(w, http.StatusBadRequest, "invalid_decision_participant", "every decision participant must be an existing user")
			} else {
				writeAPIError(w, http.StatusInternalServerError, "decision_identity_unavailable", "decision identities are unavailable")
			}
			return false
		}
	}
	if !ownerFound {
		writeAPIError(w, http.StatusBadRequest, "invalid_decision_owner", "the decision owner must be an existing participant")
		return false
	}
	return true
}

func decisionSourceExists(w http.ResponseWriter, repositoryID, actor string, source decisions.Source, proposalStore *proposals.Store, explanationStore *explanations.Store, incidentStore *incidents.Store, relationshipStore *relationships.Store, organizationStore *organizations.Store) bool {
	found, available := false, true
	switch source.Kind {
	case "repository":
		found = source.ResourceID == repositoryID
	case "proposal":
		if proposalStore == nil {
			available = false
		} else {
			_, err := proposalStore.Get(repositoryID, source.ResourceID)
			found = err == nil
		}
	case "investigation":
		if explanationStore == nil {
			available = false
		} else if v, err := explanationStore.Get(source.ResourceID); err == nil && v.RepositoryID == repositoryID {
			for _, p := range v.Participants {
				found = found || p.UserID == actor
			}
		}
	case "incident":
		if incidentStore == nil {
			available = false
		} else if v, err := incidentStore.Get(source.ResourceID); err == nil {
			for _, scope := range v.Scopes {
				found = found || scope.RepositoryID == repositoryID
			}
		}
	case "evolution_plan":
		if relationshipStore == nil {
			available = false
		} else {
			_, err := relationshipStore.GetEvolution(repositoryID, source.ResourceID)
			found = err == nil
		}
	case "stewardship_opportunity":
		if organizationStore == nil {
			available = false
		} else if groups, err := organizationStore.ListFor(actor); err != nil {
			available = false
		} else {
			for _, group := range groups {
				for _, mandate := range group.StewardshipMandates {
					for _, opportunity := range mandate.Opportunities {
						found = found || opportunity.ID == source.ResourceID && opportunity.RepositoryID == repositoryID
					}
				}
			}
		}
	}
	if !available {
		writeAPIError(w, http.StatusServiceUnavailable, "decision_context_unavailable", "the selected decision context is unavailable")
		return false
	}
	if !found {
		writeAPIError(w, http.StatusNotFound, "decision_context_not_found", "the selected decision context was not found")
		return false
	}
	return true
}
func authorizeDecision(w http.ResponseWriter, r *http.Request, c *repositories.Store, a *auth.Store, s *decisions.Store, scope string) (decisions.Decision, auth.Credential, bool) {
	v, e := s.Get(r.PathValue("id"))
	if errors.Is(e, decisions.ErrNotFound) {
		writeAPIError(w, 404, "decision_not_found", "decision not found")
		return v, auth.Credential{}, false
	}
	if writeDecisionError(w, e) {
		return v, auth.Credential{}, false
	}
	actor, _, ok := authorizeRepositoryParticipant(w, r, c, a, v.RepositoryID, scope)
	return v, actor, ok
}
func writeDecisionError(w http.ResponseWriter, e error) bool {
	if e == nil {
		return false
	}
	switch {
	case errors.Is(e, decisions.ErrNotFound):
		writeAPIError(w, 404, "decision_not_found", "decision not found")
	case errors.Is(e, decisions.ErrInvalid):
		writeAPIError(w, 400, "invalid_decision", "decision scope is invalid")
	case errors.Is(e, decisions.ErrConflict):
		writeAPIError(w, 409, "decision_changed", "the decision changed; reload before editing")
	default:
		writeAPIError(w, 500, "decision_storage_unavailable", "decision storage is unavailable")
	}
	return true
}
