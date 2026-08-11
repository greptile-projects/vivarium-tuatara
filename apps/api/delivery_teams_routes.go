package main

import (
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/activities"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/changesessions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/decisions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deliveryteams"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/explanations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/incidents"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
)

type deliveryTeamInput struct {
	Outcome deliveryteams.Outcome `json:"outcome"`
	Charter deliveryteams.Charter `json:"charter"`
}
type deliveryTeamUpdateInput struct {
	ExpectedVersion int                   `json:"expected_version"`
	Charter         deliveryteams.Charter `json:"charter"`
}
type deliveryTeamResponseInput struct {
	ExpectedVersion int    `json:"expected_version"`
	Decision        string `json:"decision"`
}
type deliveryTeamPlanInput struct {
	ExpectedVersion int                     `json:"expected_version"`
	Plan            deliveryteams.PlanInput `json:"plan"`
}
type deliveryTeamPlanResponseInput struct {
	ExpectedVersion      int    `json:"expected_version"`
	ExpectedPlanRevision int    `json:"expected_plan_revision"`
	Decision             string `json:"decision"`
}
type deliveryTeamContextInput struct {
	ExpectedVersion int                       `json:"expected_version"`
	Context         deliveryteams.WorkContext `json:"context"`
}
type deliveryTeamTimelineInput struct {
	ExpectedVersion int                         `json:"expected_version"`
	Entry           deliveryteams.TimelineInput `json:"entry"`
}
type deliveryTeamHandoffInput struct {
	ExpectedVersion int                        `json:"expected_version"`
	Handoff         deliveryteams.HandoffInput `json:"handoff"`
}
type deliveryTeamHandoffAcceptanceInput struct {
	ExpectedVersion      int      `json:"expected_version"`
	VerificationEntryIDs []string `json:"verification_entry_ids"`
	Note                 string   `json:"note"`
}
type deliveryTeamStatusInput struct {
	ExpectedVersion int                       `json:"expected_version"`
	Status          deliveryteams.StatusInput `json:"status"`
}
type deliveryTeamInterventionInput struct {
	ExpectedVersion int                             `json:"expected_version"`
	Intervention    deliveryteams.InterventionInput `json:"intervention"`
}
type deliveryTeamIntegrationInput struct {
	ExpectedVersion int                                     `json:"expected_version"`
	PlanRevision    int                                     `json:"plan_revision"`
	BaseRevision    string                                  `json:"base_revision"`
	Contributions   []deliveryteams.IntegrationContribution `json:"contributions"`
}
type deliveryTeamPublishIntegrationInput struct {
	ExpectedVersion int    `json:"expected_version"`
	TargetBranch    string `json:"target_branch"`
}

func registerDeliveryTeamRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, identities *users.Store, store *deliveryteams.Store, proposalStore *proposals.Store, decisionStore *decisions.Store, incidentStore *incidents.Store, orgs *organizations.Store, activity *activities.Store, sessionStore *changesessions.Store, workspaceStore *workspaces.Store, explanationStore *explanations.Store, pulls *pullrequests.Store) {
	mux.HandleFunc("POST /repositories/{id}/delivery-teams", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in deliveryTeamInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "outcome and charter are required")
			return
		}
		if !deliveryOutcomeExists(r.PathValue("id"), in.Outcome, proposalStore, decisionStore, incidentStore, orgs, catalog) {
			writeAPIError(w, 400, "invalid_outcome", "the planned outcome does not exist in this repository")
			return
		}
		if !prepareDeliveryCharter(w, r.PathValue("id"), actor.UserID, &in.Charter, catalog, identities, orgs) {
			return
		}
		v, err := store.Create(r.PathValue("id"), in.Outcome, in.Charter, actor.UserID)
		if writeDeliveryTeamError(w, err) {
			return
		}
		recordActivity(activity, catalog, activities.Event{Kind: "delivery_team.created", ActorID: actor.UserID, RepositoryID: v.RepositoryID, ResourceType: "delivery_team", ResourceID: v.ID, ResourceTitle: v.Name})
		writeJSON(w, 201, v)
	})
	mux.HandleFunc("GET /delivery-teams", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		repos, err := catalog.ListAccessible(actor.UserID)
		if err != nil {
			writeAPIError(w, 500, "delivery_team_storage_unavailable", "delivery teams could not be loaded")
			return
		}
		allowed := map[string]bool{}
		for _, v := range repos {
			allowed[v.ID] = true
		}
		all, err := store.List()
		if writeDeliveryTeamError(w, err) {
			return
		}
		out := []deliveryteams.Team{}
		for _, v := range all {
			if allowed[v.RepositoryID] || deliveryInvitee(v, actor.UserID, orgs, catalog) {
				out = append(out, projectDeliveryAccess(v, actor.UserID, catalog, orgs))
			}
		}
		writeJSON(w, 200, map[string]any{"delivery_teams": out})
	})
	mux.HandleFunc("GET /delivery-teams/{id}", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		v, err := store.Get(r.PathValue("id"))
		if writeDeliveryTeamError(w, err) {
			return
		}
		repo, err := catalog.GetByID(v.RepositoryID)
		if err != nil {
			writeAPIError(w, 404, "delivery_team_not_found", "delivery team not found")
			return
		}
		collab, _ := catalog.HasCollaborator(actor.UserID, repo.ID)
		if actor.UserID != repo.OwnerID && !collab && !deliveryInvitee(v, actor.UserID, orgs, catalog) {
			writeAPIError(w, 404, "delivery_team_not_found", "delivery team not found")
			return
		}
		writeJSON(w, 200, projectDeliveryAccess(v, actor.UserID, catalog, orgs))
	})
	mux.HandleFunc("PUT /delivery-teams/{id}", func(w http.ResponseWriter, r *http.Request) {
		existing, err := store.Get(r.PathValue("id"))
		if writeDeliveryTeamError(w, err) {
			return
		}
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, existing.RepositoryID, "repositories:write")
		if !ok {
			return
		}
		var in deliveryTeamUpdateInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "expected_version and charter are required")
			return
		}
		if !prepareDeliveryCharter(w, existing.RepositoryID, actor.UserID, &in.Charter, catalog, identities, orgs) {
			return
		}
		v, err := store.Update(existing.ID, actor.UserID, in.ExpectedVersion, in.Charter)
		if writeDeliveryTeamError(w, err) {
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("POST /delivery-teams/{id}/participants/{participantId}/response", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		v, err := store.Get(r.PathValue("id"))
		if writeDeliveryTeamError(w, err) {
			return
		}
		authorized := false
		for _, p := range v.Participants {
			if p.ID != r.PathValue("participantId") {
				continue
			}
			authorized = p.PrincipalType == "human" && p.PrincipalID == actor.UserID
			if p.PrincipalType == "agent" {
				authorized = agentOperator(v.RepositoryID, p.PrincipalID, actor.UserID, orgs, catalog)
			}
		}
		if !authorized {
			writeAPIError(w, 403, "invitation_response_forbidden", "only the invitee or an approved agent operator may respond")
			return
		}
		var in deliveryTeamResponseInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "expected_version and decision are required")
			return
		}
		v, err = store.Respond(v.ID, r.PathValue("participantId"), actor.UserID, in.Decision, in.ExpectedVersion)
		if writeDeliveryTeamError(w, err) {
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("PUT /delivery-teams/{id}/plan", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		team, err := store.Get(r.PathValue("id"))
		if writeDeliveryTeamError(w, err) {
			return
		}
		principal, allowed := deliveryPlanningPrincipal(team, actor.UserID, orgs, catalog)
		if !allowed {
			writeAPIError(w, 403, "delivery_plan_forbidden", "only the organizer or an accepted team member may propose the plan")
			return
		}
		var in deliveryTeamPlanInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "expected_version and plan are required")
			return
		}
		for _, stream := range in.Plan.Streams {
			for _, scope := range stream.RepositoryScope {
				if _, err := catalog.GetByID(scope.RepositoryID); err != nil {
					writeAPIError(w, 400, "invalid_repository_scope", "a scoped repository does not exist")
					return
				}
			}
			for _, input := range stream.Inputs {
				if input.RepositoryID == "" {
					continue
				}
				if _, err := catalog.GetByID(input.RepositoryID); err != nil {
					writeAPIError(w, 400, "invalid_repository_input", "a repository-backed work input does not exist")
					return
				}
			}
		}
		team, err = store.PutPlan(team.ID, actor.UserID, principal, in.ExpectedVersion, in.Plan)
		if writeDeliveryTeamError(w, err) {
			return
		}
		writeJSON(w, 200, projectDeliveryAccess(team, actor.UserID, catalog, orgs))
	})
	mux.HandleFunc("POST /delivery-teams/{id}/plan/participants/{participantId}/response", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		team, err := store.Get(r.PathValue("id"))
		if writeDeliveryTeamError(w, err) {
			return
		}
		principal, allowed := deliveryParticipantPrincipal(team, r.PathValue("participantId"), actor.UserID, orgs, catalog)
		if !allowed {
			writeAPIError(w, 403, "delivery_plan_response_forbidden", "only the affected owner or an approved agent operator may respond")
			return
		}
		var in deliveryTeamPlanResponseInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "expected versions and decision are required")
			return
		}
		team, err = store.RespondPlan(team.ID, r.PathValue("participantId"), actor.UserID, principal, in.Decision, in.ExpectedVersion, in.ExpectedPlanRevision)
		if writeDeliveryTeamError(w, err) {
			return
		}
		writeJSON(w, 200, projectDeliveryAccess(team, actor.UserID, catalog, orgs))
	})
	mux.HandleFunc("POST /delivery-teams/{id}/streams/{streamId}/contexts", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		team, err := store.Get(r.PathValue("id"))
		if writeDeliveryTeamError(w, err) {
			return
		}
		principal, allowed := deliveryPlanningPrincipal(team, actor.UserID, orgs, catalog)
		if !allowed {
			writeAPIError(w, 403, "delivery_stream_forbidden", "only an accepted team member may attach work")
			return
		}
		var in deliveryTeamContextInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "expected_version and context are required")
			return
		}
		in.Context.RepositoryID = strings.TrimSpace(in.Context.RepositoryID)
		if !deliveryPrincipalCanRead(team, principal, in.Context.RepositoryID, catalog, orgs) {
			writeAPIError(w, 403, "inaccessible_team_evidence", "work context is outside the participant's current access")
			return
		}
		if !deliveryContextExists(in.Context, sessionStore, workspaceStore, explanationStore, decisionStore) {
			writeAPIError(w, 400, "invalid_work_context", "the exact work context could not be resolved")
			return
		}
		team, err = store.AttachContext(team.ID, r.PathValue("streamId"), actor.UserID, principal, in.ExpectedVersion, in.Context)
		if writeDeliveryTeamError(w, err) {
			return
		}
		writeJSON(w, 200, projectDeliveryAccess(team, actor.UserID, catalog, orgs))
	})
	mux.HandleFunc("POST /delivery-teams/{id}/timeline", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		team, err := store.Get(r.PathValue("id"))
		if writeDeliveryTeamError(w, err) {
			return
		}
		principal, allowed := deliveryPlanningPrincipal(team, actor.UserID, orgs, catalog)
		if !allowed {
			writeAPIError(w, 403, "delivery_timeline_forbidden", "only accepted team members may publish")
			return
		}
		var in deliveryTeamTimelineInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "expected_version and entry are required")
			return
		}
		for _, c := range in.Entry.Citations {
			if !deliveryPrincipalCanRead(team, principal, c.RepositoryID, catalog, orgs) {
				writeAPIError(w, 403, "inaccessible_team_evidence", "cited evidence is outside the participant's current access")
				return
			}
			context, found := deliveryCitationContext(team, in.Entry.StreamID, c)
			if !found || !deliveryContextExists(context, sessionStore, workspaceStore, explanationStore, decisionStore) {
				writeAPIError(w, 400, "invalid_team_citation", "cited evidence could not be resolved at its exact revision")
				return
			}
		}
		team, err = store.PublishTimeline(team.ID, actor.UserID, principal, in.ExpectedVersion, in.Entry)
		if writeDeliveryTeamError(w, err) {
			return
		}
		writeJSON(w, 200, projectDeliveryAccess(team, actor.UserID, catalog, orgs))
	})
	mux.HandleFunc("POST /delivery-teams/{id}/handoffs", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		team, err := store.Get(r.PathValue("id"))
		if writeDeliveryTeamError(w, err) {
			return
		}
		principal, allowed := deliveryPlanningPrincipal(team, actor.UserID, orgs, catalog)
		if !allowed {
			writeAPIError(w, 403, "delivery_handoff_forbidden", "only accepted team members may request handoff")
			return
		}
		var in deliveryTeamHandoffInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "expected_version and handoff are required")
			return
		}
		team, err = store.RequestHandoff(team.ID, actor.UserID, principal, in.ExpectedVersion, in.Handoff)
		if writeDeliveryTeamError(w, err) {
			return
		}
		writeJSON(w, 201, projectDeliveryAccess(team, actor.UserID, catalog, orgs))
	})
	mux.HandleFunc("POST /delivery-teams/{id}/handoffs/{handoffId}/accept", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		team, err := store.Get(r.PathValue("id"))
		if writeDeliveryTeamError(w, err) {
			return
		}
		principal := ""
		for _, h := range team.Handoffs {
			if h.ID == r.PathValue("handoffId") {
				principal, _ = deliveryParticipantPrincipal(team, h.ToParticipantID, actor.UserID, orgs, catalog)
			}
		}
		if principal == "" {
			writeAPIError(w, 403, "delivery_handoff_forbidden", "only the named recipient may accept handoff")
			return
		}
		var in deliveryTeamHandoffAcceptanceInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "verification entries and note are required")
			return
		}
		team, err = store.AcceptHandoff(team.ID, r.PathValue("handoffId"), actor.UserID, principal, in.ExpectedVersion, in.VerificationEntryIDs, in.Note)
		if writeDeliveryTeamError(w, err) {
			return
		}
		writeJSON(w, 200, projectDeliveryAccess(team, actor.UserID, catalog, orgs))
	})
	mux.HandleFunc("PUT /delivery-teams/{id}/streams/{streamId}/status", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		team, err := store.Get(r.PathValue("id"))
		if writeDeliveryTeamError(w, err) {
			return
		}
		principal, allowed := deliveryPlanningPrincipal(team, actor.UserID, orgs, catalog)
		if !allowed {
			writeAPIError(w, 403, "delivery_stream_forbidden", "only the accepted stream owner may report status")
			return
		}
		var in deliveryTeamStatusInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "expected_version and status are required")
			return
		}
		team, err = store.ReportStatus(team.ID, r.PathValue("streamId"), actor.UserID, principal, in.ExpectedVersion, in.Status)
		if writeDeliveryTeamError(w, err) {
			return
		}
		writeJSON(w, 200, projectDeliveryAccess(team, actor.UserID, catalog, orgs))
	})
	mux.HandleFunc("POST /delivery-teams/{id}/interventions", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		team, err := store.Get(r.PathValue("id"))
		if writeDeliveryTeamError(w, err) {
			return
		}
		principal, allowed := deliveryPlanningPrincipal(team, actor.UserID, orgs, catalog)
		if !allowed {
			writeAPIError(w, 403, "delivery_intervention_forbidden", "only the organizer or an accepted team member may intervene")
			return
		}
		var in deliveryTeamInterventionInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "expected_version and intervention are required")
			return
		}
		if in.Intervention.Action == "resume" {
			in.Intervention.ResumeAuthorized = deliveryResumeAuthorized(team, in.Intervention.Scope, in.Intervention.StreamID, catalog, orgs)
		}
		team, err = store.Intervene(team.ID, actor.UserID, principal, in.ExpectedVersion, in.Intervention)
		if writeDeliveryTeamError(w, err) {
			return
		}
		writeJSON(w, 200, projectDeliveryAccess(team, actor.UserID, catalog, orgs))
	})
	mux.HandleFunc("POST /delivery-teams/{id}/integrations", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		team, err := store.Get(r.PathValue("id"))
		if writeDeliveryTeamError(w, err) {
			return
		}
		principal, accepted := deliveryPlanningPrincipal(team, actor.UserID, orgs, catalog)
		if !accepted || !deliveryRepositoryWrite(actor.UserID, team.RepositoryID, catalog) {
			writeAPIError(w, 403, "delivery_integration_forbidden", "an accepted member with independent repository write access is required")
			return
		}
		var in deliveryTeamIntegrationInput
		if decodeJSON(r, &in) != nil || team.Plan == nil || in.PlanRevision != team.Plan.Revision {
			writeAPIError(w, 400, "invalid_request", "the current plan revision and contributions are required")
			return
		}
		blockers, valid := analyzeDeliveryIntegration(team, in.BaseRevision, in.Contributions, git, workspaceStore)
		if !valid {
			writeAPIError(w, 422, "delivery_integration_invalid", "every contribution must name its exact live branch or published checkpoint")
			return
		}
		team, err = store.PrepareIntegration(team.ID, actor.UserID, principal, in.ExpectedVersion, in.PlanRevision, in.BaseRevision, in.Contributions, blockers)
		if writeDeliveryTeamError(w, err) {
			return
		}
		writeJSON(w, 201, projectDeliveryAccess(team, actor.UserID, catalog, orgs))
	})
	mux.HandleFunc("POST /delivery-teams/{id}/integrations/{integrationId}/publish", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:write", false)
		if !ok {
			return
		}
		team, err := store.Get(r.PathValue("id"))
		if writeDeliveryTeamError(w, err) {
			return
		}
		// The generic authorizer cannot resolve the repository before loading the
		// team, so revalidate against the exact charter repository here.
		if !deliveryRepositoryWrite(actor.UserID, team.RepositoryID, catalog) {
			writeAPIError(w, 403, "delivery_integration_forbidden", "independent repository write access is required")
			return
		}
		principal, accepted := deliveryPlanningPrincipal(team, actor.UserID, orgs, catalog)
		if !accepted {
			writeAPIError(w, 403, "delivery_integration_forbidden", "only an accepted team member may publish")
			return
		}
		var in deliveryTeamPublishIntegrationInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "expected_version is required")
			return
		}
		if team.Version != in.ExpectedVersion {
			writeAPIError(w, 409, "delivery_team_changed", "delivery team version changed")
			return
		}
		target := strings.TrimSpace(in.TargetBranch)
		if target == "" {
			target = "main"
		}
		var manifest *deliveryteams.Integration
		for i := range team.Integrations {
			if team.Integrations[i].ID == r.PathValue("integrationId") {
				manifest = &team.Integrations[i]
			}
		}
		if manifest == nil {
			writeAPIError(w, 404, "delivery_integration_not_found", "integration not found")
			return
		}
		if team.Plan == nil || manifest.PlanRevision != team.Plan.Revision {
			writeAPIError(w, 409, "delivery_integration_blocked", "the current execution plan changed after reconciliation")
			return
		}
		currentContributions := append([]deliveryteams.IntegrationContribution(nil), manifest.Contributions...)
		currentBlockers, current := analyzeDeliveryIntegration(team, manifest.BaseRevision, currentContributions, git, workspaceStore)
		if !current {
			writeAPIError(w, 409, "delivery_integration_changed", "a contribution changed after reconciliation")
			return
		}
		if len(manifest.Blockers) > 0 || len(currentBlockers) > 0 || manifest.PublishedAt != nil {
			writeAPIError(w, 409, "delivery_integration_blocked", "resolve conflicts and missing acceptance evidence before publication")
			return
		}
		if pulls == nil {
			writeAPIError(w, 503, "delivery_integration_unavailable", "pull request governance is unavailable")
			return
		}
		publishStatus, publishCode, publishMessage := 0, "", ""
		team, err = store.PublishIntegration(team.ID, manifest.ID, actor.UserID, principal, in.ExpectedVersion, func(currentTeam deliveryteams.Team, currentManifest deliveryteams.Integration) ([]deliveryteams.IntegrationPull, error) {
			if currentTeam.Plan == nil || currentManifest.PlanRevision != currentTeam.Plan.Revision {
				publishStatus, publishCode, publishMessage = 409, "delivery_integration_blocked", "the current execution plan changed after reconciliation"
				return nil, deliveryteams.ErrConflict
			}
			contributions := append([]deliveryteams.IntegrationContribution(nil), currentManifest.Contributions...)
			blockers, ready := analyzeDeliveryIntegration(currentTeam, currentManifest.BaseRevision, contributions, git, workspaceStore)
			if !ready || len(blockers) > 0 {
				publishStatus, publishCode, publishMessage = 409, "delivery_integration_blocked", "current delivery readiness no longer permits publication"
				return nil, deliveryteams.ErrConflict
			}
			published := []deliveryteams.IntegrationPull{}
			for _, c := range currentManifest.Contributions {
				if !deliveryRepositoryWrite(actor.UserID, c.RepositoryID, catalog) {
					publishStatus, publishCode, publishMessage = 403, "delivery_integration_forbidden", "each contribution requires independent repository write access"
					return nil, deliveryteams.ErrForbidden
				}
				body := deliveryIntegrationPullBody(currentTeam, currentManifest, c)
				pull, createErr := pulls.FindOrCreateRecovery(c.RepositoryID, actor.UserID, c.StreamID+": "+currentTeam.Outcome.Title, body, c.Branch, target)
				if createErr == nil {
					pull, createErr = pulls.LinkDeliveryIntegration(c.RepositoryID, pull.ID, currentTeam.ID, currentManifest.ID, c.StreamID, c.IntegrationOrder)
				}
				if createErr != nil {
					publishStatus, publishCode, publishMessage = 409, "delivery_integration_publish_failed", "an ordered contribution could not be opened and linked for review"
					return nil, deliveryteams.ErrConflict
				}
				published = append(published, deliveryteams.IntegrationPull{StreamID: c.StreamID, RepositoryID: c.RepositoryID, PullRequestID: pull.ID, Order: c.IntegrationOrder})
			}
			return published, nil
		})
		if publishMessage != "" {
			writeAPIError(w, publishStatus, publishCode, publishMessage)
			return
		}
		if writeDeliveryTeamError(w, err) {
			return
		}
		writeJSON(w, 201, projectDeliveryAccess(team, actor.UserID, catalog, orgs))
	})
}

func deliveryResumeAuthorized(team deliveryteams.Team, scope, streamID string, catalog *repositories.Store, orgs *organizations.Store) bool {
	if team.Plan == nil {
		return false
	}
	for _, stream := range team.Plan.Streams {
		if scope == "stream" && stream.ID != streamID {
			continue
		}
		owner := participantByDeliveryID(team, stream.OwnerParticipantID)
		if owner == nil {
			return false
		}
		for _, repository := range stream.RepositoryScope {
			level, _ := deliveryParticipantAccess(*owner, repository.RepositoryID, catalog, orgs)
			if level != "write" {
				return false
			}
		}
	}
	return scope == "team" || scope == "stream"
}

func deliveryRepositoryWrite(actor, repositoryID string, catalog *repositories.Store) bool {
	repository, err := catalog.GetByID(repositoryID)
	if err != nil {
		return false
	}
	if repository.OwnerID == actor {
		return true
	}
	ok, err := catalog.HasCollaborator(actor, repositoryID)
	return err == nil && ok
}

func analyzeDeliveryIntegration(team deliveryteams.Team, base string, contributions []deliveryteams.IntegrationContribution, git *storage.Store, workspaceStore *workspaces.Store) ([]deliveryteams.IntegrationBlocker, bool) {
	if team.Plan == nil || len(contributions) != len(team.Plan.Streams) || len(base) != 40 {
		return nil, false
	}
	// Convert plan blockers without weakening their attribution.
	out := make([]deliveryteams.IntegrationBlocker, 0, len(team.Plan.Blockers))
	for _, b := range team.Plan.Blockers {
		out = append(out, deliveryteams.IntegrationBlocker{Kind: b.Kind, StreamIDs: b.StreamIDs, Summary: b.Summary})
	}
	byStream := map[string]*deliveryteams.IntegrationContribution{}
	paths := map[string]string{}
	timeline := map[string]deliveryteams.TimelineEntry{}
	for _, e := range team.Timeline {
		timeline[e.ID] = e
	}
	for i := range contributions {
		c := &contributions[i]
		stream := deliveryStream(team, c.StreamID)
		if stream == nil || byStream[c.StreamID] != nil || c.RepositoryID == "" || c.Branch == "" {
			return nil, false
		}
		byStream[c.StreamID] = c
		repository, err := git.Open(c.RepositoryID)
		if err != nil {
			return nil, false
		}
		ref, err := repository.ReadReference("refs/heads/" + c.Branch)
		if err != nil || ref.Target != c.CommitID {
			return nil, false
		}
		if exec.Command("git", "--git-dir", repository.Path(), "merge-base", "--is-ancestor", base, c.CommitID).Run() != nil {
			return nil, false
		}
		if c.SourceKind == "checkpoint" {
			if workspaceStore == nil || c.WorkspaceID == "" || c.CheckpointID == "" {
				return nil, false
			}
			checkpoint, err := workspaceStore.GetCheckpoint(c.WorkspaceID, c.CheckpointID)
			if err != nil || checkpoint.RepositoryID != c.RepositoryID || checkpoint.Publication == nil || checkpoint.Publication.CommitID != c.CommitID || checkpoint.Publication.Branch != c.Branch {
				return nil, false
			}
			c.Authors = append([]string(nil), checkpoint.ContributorIDs...)
			c.AgentActions = []string{}
			for _, command := range checkpoint.Commands {
				c.AgentActions = append(c.AgentActions, command.ID)
			}
		} else if c.SourceKind != "branch" {
			return nil, false
		}
		changed, err := exec.Command("git", "--git-dir", repository.Path(), "diff", "--name-only", base, c.CommitID, "--").Output()
		if err != nil {
			return nil, false
		}
		c.ChangedPaths = strings.Fields(string(changed))
		authors, err := exec.Command("git", "--git-dir", repository.Path(), "log", "--format=%an", base+".."+c.CommitID).Output()
		if err != nil {
			return nil, false
		}
		for _, author := range strings.Split(strings.TrimSpace(string(authors)), "\n") {
			author = strings.TrimSpace(author)
			if author != "" && !slices.Contains(c.Authors, author) {
				c.Authors = append(c.Authors, author)
			}
		}
		for _, entry := range team.Timeline {
			if entry.StreamID != c.StreamID {
				continue
			}
			if entry.AuthorType == "agent" && !slices.Contains(c.AgentActions, entry.ID) {
				c.AgentActions = append(c.AgentActions, entry.ID)
			}
			if entry.Kind == "decision" && !slices.Contains(c.Decisions, entry.ID) {
				c.Decisions = append(c.Decisions, entry.ID)
			}
		}
		for _, path := range c.ChangedPaths {
			if other := paths[path]; other != "" && other != c.StreamID {
				out = append(out, deliveryteams.IntegrationBlocker{Kind: "content_conflict", StreamIDs: []string{other, c.StreamID}, Paths: []string{path}, Summary: "Parallel contributions change the same path and require explicit reconciliation"})
			} else {
				paths[path] = c.StreamID
			}
		}
		missing := []string{}
		for _, criterion := range stream.AcceptanceCriteria {
			ids := c.AcceptanceEvidence[criterion]
			valid := len(ids) > 0
			for _, entryID := range ids {
				entry, ok := timeline[entryID]
				if !ok || entry.StreamID != c.StreamID {
					valid = false
				}
			}
			if !valid {
				missing = append(missing, criterion)
			}
		}
		if len(missing) > 0 {
			out = append(out, deliveryteams.IntegrationBlocker{Kind: "missing_acceptance_evidence", StreamIDs: []string{c.StreamID}, Criteria: missing, Summary: "Acceptance criteria are not backed by retained stream evidence"})
		}
		hasStatus := false
		for _, status := range team.StreamStatuses {
			if status.StreamID == c.StreamID {
				hasStatus = true
				c.Cost = status.ResourceUse
				if status.Status != "completed" {
					out = append(out, deliveryteams.IntegrationBlocker{Kind: "stream_incomplete", StreamIDs: []string{c.StreamID}, Summary: "The contribution stream has not reported completion"})
				}
			}
		}
		if !hasStatus {
			out = append(out, deliveryteams.IntegrationBlocker{Kind: "stream_incomplete", StreamIDs: []string{c.StreamID}, Summary: "The contribution stream has not reported completion"})
		}
	}
	for _, handoff := range team.Handoffs {
		if handoff.PlanRevision == team.Plan.Revision && handoff.Status != "accepted" {
			out = append(out, deliveryteams.IntegrationBlocker{Kind: "handoff_pending", StreamIDs: []string{handoff.StreamID}, Summary: "A declared handoff still requires recipient verification"})
		}
	}
	return out, true
}

func deliveryStream(team deliveryteams.Team, id string) *deliveryteams.WorkStream {
	if team.Plan == nil {
		return nil
	}
	for i := range team.Plan.Streams {
		if team.Plan.Streams[i].ID == id {
			return &team.Plan.Streams[i]
		}
	}
	return nil
}

func deliveryIntegrationPullBody(team deliveryteams.Team, integration deliveryteams.Integration, c deliveryteams.IntegrationContribution) string {
	var b strings.Builder
	b.WriteString("Part of delivery team **" + team.Name + "** for " + team.Outcome.Title + ".\n\n")
	b.WriteString("Integration manifest `" + integration.ID + "`, plan revision " + fmt.Sprint(integration.PlanRevision) + ", order " + fmt.Sprint(c.IntegrationOrder) + ". This connection grants no review or merge authority.\n\n")
	b.WriteString("Base `" + integration.BaseRevision + "`; contribution `" + c.CommitID + "`.\n\n")
	b.WriteString("Authors: " + strings.Join(c.Authors, ", ") + "\n\nAgent actions: " + strings.Join(c.AgentActions, ", ") + "\n\nDecisions: " + strings.Join(c.Decisions, "; ") + "\n\nResidual risks: " + strings.Join(c.ResidualRisks, "; ") + "\n")
	if c.Cost != nil {
		b.WriteString("\nCost: " + fmt.Sprint(c.Cost.Consumed) + " " + c.Cost.Unit + ".\n")
	}
	b.WriteString("\nAcceptance evidence:\n")
	keys := make([]string, 0, len(c.AcceptanceEvidence))
	for k := range c.AcceptanceEvidence {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		b.WriteString("- " + k + ": " + strings.Join(c.AcceptanceEvidence[k], ", ") + "\n")
	}
	return b.String()
}

func deliveryPrincipalCanRead(team deliveryteams.Team, principal, repositoryID string, catalog *repositories.Store, orgs *organizations.Store) bool {
	for _, p := range team.Participants {
		if p.PrincipalID == principal {
			level, _ := deliveryParticipantAccess(p, repositoryID, catalog, orgs)
			return level == "read" || level == "write"
		}
	}
	level, _ := deliveryParticipantAccess(deliveryteams.Participant{PrincipalType: "human", PrincipalID: principal}, repositoryID, catalog, orgs)
	return level == "read" || level == "write"
}

func deliveryContextExists(context deliveryteams.WorkContext, sessions *changesessions.Store, workspacesStore *workspaces.Store, explanationsStore *explanations.Store, decisionStore *decisions.Store) bool {
	switch context.Kind {
	case "workspace":
		if workspacesStore == nil {
			return false
		}
		value, err := workspacesStore.Get(context.ResourceID)
		return err == nil && value.RepositoryID == context.RepositoryID && value.CommitID == context.Revision
	case "investigation":
		if explanationsStore == nil {
			return false
		}
		value, err := explanationsStore.Get(context.ResourceID)
		return err == nil && value.RepositoryID == context.RepositoryID && value.Revision == context.Revision
	case "experiment":
		if decisionStore == nil || context.ParentID == "" {
			return false
		}
		value, err := decisionStore.Get(context.ParentID)
		if err != nil || value.RepositoryID != context.RepositoryID {
			return false
		}
		for _, experiment := range value.Experiments {
			if experiment.ID == context.ResourceID && experiment.Revision == context.Revision {
				return true
			}
		}
	case "change_session":
		if sessions == nil || context.ParentID == "" {
			return false
		}
		value, err := sessions.Get(context.RepositoryID, context.ParentID, context.ResourceID)
		return err == nil && value.SourceCommitID == context.Revision
	}
	return false
}

func deliveryCitationContext(team deliveryteams.Team, streamID string, citation deliveryteams.Citation) (deliveryteams.WorkContext, bool) {
	if team.Plan == nil {
		return deliveryteams.WorkContext{}, false
	}
	for _, stream := range team.Plan.Streams {
		if stream.ID != streamID {
			continue
		}
		for _, context := range stream.Contexts {
			if context.Kind == citation.Kind && context.ResourceID == citation.ResourceID && context.RepositoryID == citation.RepositoryID && context.Revision == citation.Revision {
				return context, true
			}
		}
	}
	return deliveryteams.WorkContext{}, false
}

func deliveryViewerCanRead(team deliveryteams.Team, viewer, repositoryID string, catalog *repositories.Store, orgs *organizations.Store) bool {
	if deliveryPrincipalCanRead(team, viewer, repositoryID, catalog, orgs) {
		return true
	}
	for _, participant := range team.Participants {
		if participant.PrincipalType == "agent" && agentOperator(team.RepositoryID, participant.PrincipalID, viewer, orgs, catalog) && deliveryPrincipalCanRead(team, participant.PrincipalID, repositoryID, catalog, orgs) {
			return true
		}
	}
	return false
}

func deliveryParticipantPrincipal(team deliveryteams.Team, participantID, actor string, orgs *organizations.Store, catalog *repositories.Store) (string, bool) {
	for _, p := range team.Participants {
		if p.ID != participantID {
			continue
		}
		if p.PrincipalType == "human" && p.PrincipalID == actor {
			return p.PrincipalID, true
		}
		if p.PrincipalType == "agent" && agentOperator(team.RepositoryID, p.PrincipalID, actor, orgs, catalog) {
			return p.PrincipalID, true
		}
	}
	return "", false
}
func deliveryPlanningPrincipal(team deliveryteams.Team, actor string, orgs *organizations.Store, catalog *repositories.Store) (string, bool) {
	if team.OrganizerID == actor {
		return actor, true
	}
	for _, p := range team.Participants {
		if p.Status != "accepted" {
			continue
		}
		if principal, ok := deliveryParticipantPrincipal(team, p.ID, actor, orgs, catalog); ok {
			return principal, true
		}
	}
	return "", false
}

func deliveryOutcomeExists(repo string, o deliveryteams.Outcome, ps *proposals.Store, ds *decisions.Store, is *incidents.Store, orgs *organizations.Store, catalog *repositories.Store) bool {
	switch o.Kind {
	case "proposal":
		v, e := ps.Get(repo, o.ResourceID)
		return e == nil && v.ID != ""
	case "decision":
		v, e := ds.Get(o.ResourceID)
		return e == nil && v.RepositoryID == repo
	case "incident_follow_up":
		v, e := is.Get(o.ResourceID)
		if e != nil {
			return false
		}
		for _, scope := range v.Scopes {
			if scope.RepositoryID == repo {
				return true
			}
		}
		return false
	case "initiative":
		repository, e := catalog.GetByID(repo)
		if e != nil || repository.OrganizationID == "" {
			return false
		}
		r, e := orgs.Get(repository.OrganizationID)
		if e != nil {
			return false
		}
		for _, i := range r.Initiatives {
			if i.ID == o.ResourceID {
				return true
			}
		}
		return false
	case "planned_outcome":
		return o.ResourceID != "" && o.Title != ""
	}
	return false
}
func prepareDeliveryCharter(w http.ResponseWriter, repo, organizer string, c *deliveryteams.Charter, catalog *repositories.Store, identities *users.Store, orgs *organizations.Store) bool {
	for i := range c.Participants {
		p := &c.Participants[i]
		if p.ID == "" {
			p.ID = randomAPIID()
		}
		if p.PrincipalType == "human" {
			if _, e := identities.Get(p.PrincipalID); e != nil {
				writeAPIError(w, 400, "invalid_participant", "human participant does not exist")
				return false
			}
		} else if p.PrincipalType == "agent" {
			if !agentApproved(repo, p.PrincipalID, orgs, catalog) {
				writeAPIError(w, 400, "invalid_participant", "agent is not approved for the repository organization")
				return false
			}
		}
		for _, a := range p.RequiredAccess {
			if a.RepositoryID != repo || (a.Level != "read" && a.Level != "write") {
				writeAPIError(w, 400, "invalid_authority", "requested access exceeds the organizer's repository authority")
				return false
			}
		}
	}
	return true
}
func randomAPIID() string {
	return strings.ReplaceAll(time.Now().UTC().Format("20060102150405.000000000"), ".", "") + strings.Repeat("0", 9)
}
func agentApproved(repo, agent string, orgs *organizations.Store, catalog *repositories.Store) bool {
	r, e := catalog.GetByID(repo)
	if e != nil || r.OrganizationID == "" {
		return false
	}
	o, e := orgs.Get(r.OrganizationID)
	if e != nil {
		return false
	}
	for _, a := range o.Agents {
		if a.ID == agent {
			return true
		}
	}
	return false
}
func agentOperator(repo, agent, user string, orgs *organizations.Store, catalog *repositories.Store) bool {
	all, e := orgs.ListFor(user)
	if e != nil {
		return false
	}
	for _, o := range all {
		for _, a := range o.Agents {
			if a.ID == agent {
				for _, x := range a.OperatorIDs {
					if x == user {
						return agentApproved(repo, agent, orgs, catalog)
					}
				}
			}
		}
	}
	return false
}
func grantCovers(g organizations.AccessGrant, repo string) bool {
	if g.RevokedAt != nil || (g.ExpiresAt != nil && g.ExpiresAt.Before(time.Now())) {
		return false
	}
	for _, r := range g.Resources {
		if r.Kind == "repository" && r.ID == repo {
			return true
		}
	}
	return false
}
func agentGrantAccess(o organizations.Organization, agent, repo string) (string, string) {
	level, source := "none", "no independent grant"
	for _, g := range o.AccessGrants {
		if g.PrincipalType != "agent" || g.PrincipalID != agent || !grantCovers(g, repo) {
			continue
		}
		if level == "none" {
			level, source = "read", "organization grant "+g.ID
		}
		if g.Role != "viewer" {
			level, source = "write", "organization grant "+g.ID
		}
	}
	return level, source
}
func deliveryInvitee(v deliveryteams.Team, user string, orgs *organizations.Store, catalogs ...*repositories.Store) bool {
	for _, p := range v.Participants {
		if p.PrincipalType == "human" && p.PrincipalID == user {
			return true
		}
		if p.PrincipalType == "agent" && len(catalogs) > 0 && agentOperator(v.RepositoryID, p.PrincipalID, user, orgs, catalogs[0]) {
			return true
		}
	}
	return false
}
func projectDeliveryAccess(v deliveryteams.Team, viewer string, catalog *repositories.Store, orgs *organizations.Store) deliveryteams.Team {
	if v.PlanHistory == nil {
		v.PlanHistory = []deliveryteams.ExecutionPlan{}
	}
	if v.Timeline == nil {
		v.Timeline = []deliveryteams.TimelineEntry{}
	}
	if v.Handoffs == nil {
		v.Handoffs = []deliveryteams.Handoff{}
	}
	if v.StreamStatuses == nil {
		v.StreamStatuses = []deliveryteams.StreamStatus{}
	}
	if v.Interventions == nil {
		v.Interventions = []deliveryteams.Intervention{}
	}
	if v.Integrations == nil {
		v.Integrations = []deliveryteams.Integration{}
	}
	for i := range v.Participants {
		p := &v.Participants[i]
		p.CanRespond = p.Status == "pending" && (p.PrincipalType == "human" && p.PrincipalID == viewer || p.PrincipalType == "agent" && agentOperator(v.RepositoryID, p.PrincipalID, viewer, orgs, catalog))
		p.AccessPreview = []deliveryteams.AccessPreview{}
		for _, req := range p.RequiredAccess {
			level, source := deliveryParticipantAccess(*p, req.RepositoryID, catalog, orgs)
			sufficient := level == "write" || (level == "read" && req.Level == "read")
			p.AccessPreview = append(p.AccessPreview, deliveryteams.AccessPreview{RepositoryID: req.RepositoryID, Required: req.Level, Effective: level, Source: source, Sufficient: sufficient})
		}
	}
	if v.Plan != nil {
		v.Plan.Blockers = append([]deliveryteams.PlanBlocker{}, v.Plan.Blockers...)
		for i := range v.Plan.Acceptances {
			a := &v.Plan.Acceptances[i]
			_, a.CanRespond = deliveryParticipantPrincipal(v, a.ParticipantID, viewer, orgs, catalog)
			a.CanRespond = a.CanRespond && a.Status == "pending"
		}
		for _, stream := range v.Plan.Streams {
			p := participantByDeliveryID(v, stream.OwnerParticipantID)
			if p == nil {
				continue
			}
			for _, scope := range stream.RepositoryScope {
				level, _ := deliveryParticipantAccess(*p, scope.RepositoryID, catalog, orgs)
				if level != "write" {
					v.Plan.Blockers = append(v.Plan.Blockers, deliveryteams.PlanBlocker{Kind: "unavailable_access", StreamIDs: []string{stream.ID}, OwnerParticipantIDs: []string{stream.OwnerParticipantID}, Summary: "The stream owner lacks independent write access to " + scope.RepositoryID})
					found := false
					for j := range v.StreamStatuses {
						if v.StreamStatuses[j].StreamID != stream.ID {
							continue
						}
						found = true
						if v.StreamStatuses[j].Status != "completed" && v.StreamStatuses[j].Status != "canceled" {
							v.StreamStatuses[j].Status = "paused"
							hasAccessBlocker := false
							for _, blocker := range v.StreamStatuses[j].Blockers {
								if blocker.Kind == "access_revoked" {
									hasAccessBlocker = true
								}
							}
							if !hasAccessBlocker {
								v.StreamStatuses[j].Blockers = append(v.StreamStatuses[j].Blockers, deliveryteams.StreamBlocker{Kind: "access_revoked", Summary: "The current owner lacks independent write access", Recovery: "Restore an independent grant or have the organizer explicitly reassign the stream"})
							}
							v.StreamStatuses[j].PredictedNextAction = "Escalate access loss without transferring authority"
						}
					}
					if !found {
						v.StreamStatuses = append(v.StreamStatuses, deliveryteams.StreamStatus{StreamID: stream.ID, Status: "paused", Revision: scope.Revision, Blockers: []deliveryteams.StreamBlocker{{Kind: "access_revoked", Summary: "The current owner lacks independent write access", Recovery: "Restore an independent grant or have the organizer explicitly reassign the stream"}}, Questions: []deliveryteams.StreamQuestion{}, PredictedNextAction: "Escalate access loss without transferring authority"})
					}
				}
			}
		}
	}
	visibleEntries := []deliveryteams.TimelineEntry{}
	visibleIDs := map[string]bool{}
	for _, entry := range v.Timeline {
		visible := true
		for _, citation := range entry.Citations {
			if !deliveryViewerCanRead(v, viewer, citation.RepositoryID, catalog, orgs) {
				visible = false
				break
			}
		}
		if visible {
			visibleEntries = append(visibleEntries, entry)
			visibleIDs[entry.ID] = true
		}
	}
	v.Timeline = visibleEntries
	visibleHandoffs := []deliveryteams.Handoff{}
	for _, handoff := range v.Handoffs {
		visible := true
		for _, entryID := range handoff.InputEntryIDs {
			if !visibleIDs[entryID] {
				visible = false
			}
		}
		for _, entryID := range handoff.VerificationEntryIDs {
			if !visibleIDs[entryID] {
				visible = false
			}
		}
		if visible {
			visibleHandoffs = append(visibleHandoffs, handoff)
		}
	}
	v.Handoffs = visibleHandoffs
	visibleIntegrations := []deliveryteams.Integration{}
	for _, integration := range v.Integrations {
		visible := true
		for _, contribution := range integration.Contributions {
			if !deliveryViewerCanRead(v, viewer, contribution.RepositoryID, catalog, orgs) {
				visible = false
				break
			}
		}
		if visible {
			visibleIntegrations = append(visibleIntegrations, integration)
		}
	}
	v.Integrations = visibleIntegrations
	if v.Plan != nil {
		for i := range v.Plan.Streams {
			contexts := []deliveryteams.WorkContext{}
			for _, context := range v.Plan.Streams[i].Contexts {
				if deliveryViewerCanRead(v, viewer, context.RepositoryID, catalog, orgs) {
					contexts = append(contexts, context)
				}
			}
			v.Plan.Streams[i].Contexts = contexts
		}
	}
	return v
}
func participantByDeliveryID(v deliveryteams.Team, id string) *deliveryteams.Participant {
	for i := range v.Participants {
		if v.Participants[i].ID == id {
			return &v.Participants[i]
		}
	}
	return nil
}
func deliveryParticipantAccess(p deliveryteams.Participant, repositoryID string, catalog *repositories.Store, orgs *organizations.Store) (string, string) {
	repo, err := catalog.GetByID(repositoryID)
	if err != nil {
		return "none", "repository unavailable"
	}
	level, source := "none", "no independent grant"
	if p.PrincipalType == "human" {
		collab, _ := catalog.HasCollaborator(p.PrincipalID, repositoryID)
		if p.PrincipalID == repo.OwnerID {
			level, source = "write", "repository owner"
		} else if collab {
			level, source = "write", "repository collaborator"
		} else if repo.Visibility == "public" {
			level, source = "read", "public repository"
		}
	} else if repo.OrganizationID != "" {
		if o, err := orgs.Get(repo.OrganizationID); err == nil {
			level, source = agentGrantAccess(o, p.PrincipalID, repositoryID)
		}
	}
	return level, source
}
func writeDeliveryTeamError(w http.ResponseWriter, e error) bool {
	if e == nil {
		return false
	}
	switch {
	case errors.Is(e, deliveryteams.ErrNotFound):
		writeAPIError(w, 404, "delivery_team_not_found", "delivery team not found")
	case errors.Is(e, deliveryteams.ErrConflict):
		writeAPIError(w, 409, "delivery_team_changed", "delivery team version changed")
	case errors.Is(e, deliveryteams.ErrForbidden):
		writeAPIError(w, 403, "delivery_team_forbidden", "delivery team mutation is forbidden")
	case errors.Is(e, deliveryteams.ErrInvalid):
		writeAPIError(w, 400, "invalid_delivery_team", "delivery team charter is invalid")
	default:
		writeAPIError(w, 500, "delivery_team_storage_unavailable", "delivery team storage unavailable")
	}
	return true
}
