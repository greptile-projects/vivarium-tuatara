package main

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/contributoropportunities"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
)

type contributionHelpProjection struct {
	Workspace           workspaces.Workspace `json:"workspace"`
	ScopeChanged        bool                 `json:"scope_changed"`
	MentorAccessRevoked []string             `json:"mentor_access_revoked,omitempty"`
}

func registerContributorHelpRoutes(mux *http.ServeMux, ws *workspaces.Store, repos *repositories.Store, opportunities *contributoropportunities.Store, orgs *organizations.Store, credentials *auth.Store) {
	load := func(w http.ResponseWriter, r *http.Request) (workspaces.Workspace, auth.Credential, bool, bool) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return workspaces.Workspace{}, auth.Credential{}, false, false
		}
		item, err := ws.Get(r.PathValue("workspace_id"))
		if err != nil || item.ContributorContext == nil {
			writeAPIError(w, 404, "contribution_workspace_not_found", "contribution workspace not found")
			return workspaces.Workspace{}, auth.Credential{}, false, false
		}
		mentor := slices.Contains(item.ContributorContext.MentorIDs, actor.UserID)
		if actor.UserID != item.CreatorID && !mentor {
			writeAPIError(w, 403, "contribution_help_forbidden", "only the contributor and designated mentors can use this help thread")
			return workspaces.Workspace{}, auth.Credential{}, false, false
		}
		if mentor {
			if !currentContributionMentor(repos, item.ContributorContext.UpstreamRepositoryID, actor.UserID) {
				writeAPIError(w, 403, "contribution_mentor_access_revoked", "designated mentor access to the upstream repository was revoked")
				return workspaces.Workspace{}, auth.Credential{}, false, false
			}
		}
		return item, actor, mentor, true
	}
	mux.HandleFunc("GET /workspaces/{workspace_id}/contribution-help", func(w http.ResponseWriter, r *http.Request) {
		item, _, _, ok := load(w, r)
		if !ok {
			return
		}
		current, err := opportunities.Get(item.ContributorContext.UpstreamRepositoryID, item.ContributorContext.OpportunityID)
		changed := err != nil || current.Version != item.ContributorContext.OpportunityVersion || current.Status != "open"
		revoked := []string{}
		for _, id := range item.ContributorContext.MentorIDs {
			meta, e := repos.GetByID(item.ContributorContext.UpstreamRepositoryID)
			if e != nil {
				revoked = append(revoked, id)
				continue
			}
			collab, _ := repos.HasCollaborator(id, meta.ID)
			if id != meta.OwnerID && !collab {
				revoked = append(revoked, id)
			}
		}
		writeJSON(w, 200, contributionHelpProjection{Workspace: item, ScopeChanged: changed, MentorAccessRevoked: revoked})
	})
	mux.HandleFunc("POST /workspaces/{workspace_id}/contribution-help/entries", func(w http.ResponseWriter, r *http.Request) {
		item, actor, mentor, ok := load(w, r)
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int        `json:"expected_version"`
			Kind            string     `json:"kind"`
			Body            string     `json:"body"`
			ReplyTo         string     `json:"reply_to"`
			DecisionOwner   string     `json:"decision_owner"`
			DueAt           *time.Time `json:"due_at"`
			AgentID         string     `json:"agent_id"`
			Action          string     `json:"action"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		in.Body = strings.TrimSpace(in.Body)
		if !slices.Contains([]string{"question", "advice", "checkpoint_request", "checkpoint_response", "handoff", "intervention", "agent_action"}, in.Kind) || in.Body == "" || len(in.Body) > 4000 || !slices.Contains([]string{"contributor", "maintainer"}, in.DecisionOwner) {
			writeAPIError(w, 422, "contribution_help_invalid", "entry requires a supported kind, decision owner, and bounded body")
			return
		}
		if in.Kind == "question" && !(!mentor && in.DecisionOwner == "contributor") || slices.Contains([]string{"advice", "checkpoint_request", "handoff", "intervention"}, in.Kind) && !mentor {
			writeAPIError(w, 403, "contribution_help_role_invalid", "this help entry is not available to the current role")
			return
		}
		if in.Kind == "agent_action" {
			if !slices.Contains([]string{"explain", "diagnose", "edit"}, in.Action) {
				writeAPIError(w, 422, "contribution_agent_action_invalid", "agent action must be explain, diagnose, or edit")
				return
			}
			if mentor || !item.ContributorContext.AgentAssistance || !validContributionAgent(orgs, repos, item.ContributorContext.UpstreamRepositoryID, in.AgentID, actor.UserID) {
				writeAPIError(w, 403, "contribution_agent_forbidden", "agent assistance requires an approved agent operated by the contributor")
				return
			}
			required := "guide"
			if in.Action == "edit" {
				required = "edit"
			}
			if item.Control.PrincipalKind != "approved_agent" || item.Control.PrincipalID != in.AgentID || item.Control.Mode != required || !item.Control.ExpiresAt.After(time.Now()) {
				writeAPIError(w, 409, "contribution_agent_control_required", "grant the approved agent the explicit matching guide or edit control mode first")
				return
			}
		}
		var updated workspaces.Workspace
		err := withContributionMentorMutation(repos, item, actor.UserID, mentor, func() (mutationErr error) {
			updated, mutationErr = ws.AddContributionHelp(item.ID, actor.UserID, workspaces.ContributionHelpEntry{Kind: in.Kind, Action: in.Action, Body: in.Body, ReplyTo: in.ReplyTo, DecisionOwner: in.DecisionOwner, DueAt: in.DueAt, RequestedBy: actor.UserID, AgentID: in.AgentID}, in.ExpectedVersion)
			return mutationErr
		})
		if errors.Is(err, repositories.ErrInvalidCollaborator) || errors.Is(err, repositories.ErrNotFound) {
			writeAPIError(w, 403, "contribution_mentor_access_revoked", "designated mentor access to the upstream repository was revoked")
			return
		}
		if errors.Is(err, workspaces.ErrConflict) {
			writeAPIError(w, 409, "contribution_help_changed", "help thread changed since it was observed")
			return
		}
		if err != nil {
			writeAPIError(w, 422, "contribution_help_invalid", "help entry could not be retained")
			return
		}
		writeJSON(w, 201, updated.ContributorContext.Help)
	})
	mux.HandleFunc("POST /workspaces/{workspace_id}/contribution-help/entries/{entry_id}/status", func(w http.ResponseWriter, r *http.Request) {
		item, actor, mentor, ok := load(w, r)
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int    `json:"expected_version"`
			Status          string `json:"status"`
		}
		if decodeJSON(r, &in) != nil || !slices.Contains([]string{"resolved", "superseded"}, in.Status) {
			writeAPIError(w, 422, "contribution_help_invalid", "status must resolve or supersede open advice")
			return
		}
		var target *workspaces.ContributionHelpEntry
		for i := range item.ContributorContext.Help.Entries {
			if item.ContributorContext.Help.Entries[i].ID == r.PathValue("entry_id") {
				target = &item.ContributorContext.Help.Entries[i]
			}
		}
		if target == nil || (target.DecisionOwner == "contributor" && mentor) || (target.DecisionOwner == "maintainer" && !mentor) {
			writeAPIError(w, 403, "contribution_decision_owner_required", "only the visible decision owner can close this item")
			return
		}
		var updated workspaces.Workspace
		err := withContributionMentorMutation(repos, item, actor.UserID, mentor, func() (mutationErr error) {
			updated, mutationErr = ws.ResolveContributionHelp(item.ID, actor.UserID, target.ID, in.Status, in.ExpectedVersion)
			return mutationErr
		})
		if errors.Is(err, repositories.ErrInvalidCollaborator) || errors.Is(err, repositories.ErrNotFound) {
			writeAPIError(w, 403, "contribution_mentor_access_revoked", "designated mentor access to the upstream repository was revoked")
			return
		}
		if err != nil {
			writeAPIError(w, 409, "contribution_help_changed", "help thread changed since it was observed")
			return
		}
		writeJSON(w, 200, updated.ContributorContext.Help)
	})
	mux.HandleFunc("PUT /workspaces/{workspace_id}/contribution-help/availability", func(w http.ResponseWriter, r *http.Request) {
		item, actor, mentor, ok := load(w, r)
		if !ok {
			return
		}
		if !mentor {
			writeAPIError(w, 403, "mentor_required", "only a designated mentor can publish availability")
			return
		}
		var in struct {
			ExpectedVersion int    `json:"expected_version"`
			Status          string `json:"status"`
			ResponseHours   int    `json:"response_hours"`
			Note            string `json:"note"`
		}
		if decodeJSON(r, &in) != nil || !slices.Contains([]string{"available", "limited", "unavailable"}, in.Status) || in.ResponseHours < 0 || in.ResponseHours > 168 {
			writeAPIError(w, 422, "mentor_availability_invalid", "availability and a bounded response window are required")
			return
		}
		var updated workspaces.Workspace
		err := withContributionMentorMutation(repos, item, actor.UserID, mentor, func() (mutationErr error) {
			updated, mutationErr = ws.SetMentorAvailability(item.ID, actor.UserID, in.Status, strings.TrimSpace(in.Note), in.ResponseHours, in.ExpectedVersion)
			return mutationErr
		})
		if errors.Is(err, repositories.ErrInvalidCollaborator) || errors.Is(err, repositories.ErrNotFound) {
			writeAPIError(w, 403, "contribution_mentor_access_revoked", "designated mentor access to the upstream repository was revoked")
			return
		}
		if err != nil {
			writeAPIError(w, 409, "contribution_help_changed", "help thread changed since it was observed")
			return
		}
		writeJSON(w, 200, updated.ContributorContext.Help)
	})
	mux.HandleFunc("PUT /workspaces/{workspace_id}/contribution-help/state", func(w http.ResponseWriter, r *http.Request) {
		item, actor, mentor, ok := load(w, r)
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int    `json:"expected_version"`
			State           string `json:"state"`
			Reason          string `json:"reason"`
		}
		if decodeJSON(r, &in) != nil || !slices.Contains([]string{"active", "reassignment_requested", "exited"}, in.State) || strings.TrimSpace(in.Reason) == "" {
			writeAPIError(w, 422, "contribution_state_invalid", "state and an attributable reason are required")
			return
		}
		if in.State == "active" && !mentor {
			writeAPIError(w, 403, "mentor_required", "a designated mentor must confirm reassignment")
			return
		}
		var updated workspaces.Workspace
		err := withContributionMentorMutation(repos, item, actor.UserID, mentor, func() (mutationErr error) {
			updated, mutationErr = ws.SetContributionState(item.ID, actor.UserID, in.State, strings.TrimSpace(in.Reason), in.ExpectedVersion)
			return mutationErr
		})
		if errors.Is(err, repositories.ErrInvalidCollaborator) || errors.Is(err, repositories.ErrNotFound) {
			writeAPIError(w, 403, "contribution_mentor_access_revoked", "designated mentor access to the upstream repository was revoked")
			return
		}
		if err != nil {
			writeAPIError(w, 409, "contribution_help_changed", "help thread changed since it was observed")
			return
		}
		writeJSON(w, 200, updated.ContributorContext.Help)
	})
}

func currentContributionMentor(repos *repositories.Store, repositoryID, actorID string) bool {
	upstream, err := repos.GetByID(repositoryID)
	if err != nil {
		return false
	}
	collaborator, err := repos.HasCollaborator(actorID, repositoryID)
	return err == nil && (upstream.OwnerID == actorID || collaborator)
}

func withContributionMentorMutation(repos *repositories.Store, workspace workspaces.Workspace, actorID string, mentor bool, mutation func() error) error {
	if !mentor {
		return mutation()
	}
	return repos.WithCurrentParticipant(actorID, workspace.ContributorContext.UpstreamRepositoryID, mutation)
}

func validContributionAgent(orgs *organizations.Store, repos *repositories.Store, repositoryID, agentID, operatorID string) bool {
	if orgs == nil {
		return false
	}
	repo, err := repos.GetByID(repositoryID)
	if err != nil || repo.OrganizationID == "" {
		return false
	}
	org, err := orgs.Get(repo.OrganizationID)
	if err != nil {
		return false
	}
	for _, agent := range org.Agents {
		if agent.ID == agentID && slices.Contains(agent.OperatorIDs, operatorID) {
			return true
		}
	}
	return false
}
