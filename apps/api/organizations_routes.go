package main

import (
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/activities"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/incidents"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	packages "github.com/greptile-projects/vivarium-tuatara/apps/api/packages"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/relationships"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/securityadvisories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

type organizationInput struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
}
type organizationInviteInput struct {
	UserID string `json:"user_id"`
}
type organizationRepositoryInput struct {
	Name string `json:"name"`
}
type organizationTeamInput struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	ParentID    string `json:"parent_id"`
	Visibility  string `json:"visibility"`
}
type organizationTeamMemberInput struct {
	UserID          string `json:"user_id"`
	Role            string `json:"role"`
	ExpectedVersion int    `json:"expected_version"`
}
type organizationResponsibilityInput struct {
	RepositoryID    string `json:"repository_id"`
	Area            string `json:"area"`
	Description     string `json:"description"`
	ExpectedVersion int    `json:"expected_version"`
}
type organizationAgentInput struct {
	Name         string   `json:"name"`
	Slug         string   `json:"slug"`
	Description  string   `json:"description"`
	Visibility   string   `json:"visibility"`
	Capabilities []string `json:"capabilities"`
	OperatorIDs  []string `json:"operator_ids"`
	TeamIDs      []string `json:"team_ids"`
}
type organizationAgentProfileInput struct {
	ExpectedVersion int                        `json:"expected_version"`
	Profile         organizations.AgentProfile `json:"profile"`
}
type organizationAccessInput struct {
	PrincipalType string                          `json:"principal_type"`
	PrincipalID   string                          `json:"principal_id"`
	Role          string                          `json:"role"`
	Resources     []organizations.ResourceScope   `json:"resources"`
	Exceptions    []organizations.AccessException `json:"exceptions"`
	Reason        string                          `json:"reason"`
	ExpiresAt     *time.Time                      `json:"expires_at"`
}
type organizationPolicyInput struct {
	Name        string                       `json:"name"`
	Description string                       `json:"description"`
	Targets     []organizations.PolicyTarget `json:"targets"`
	Rules       organizations.PolicyRules    `json:"rules"`
}
type organizationPolicyActivationInput struct {
	ExpectedVersion int `json:"expected_version"`
}
type organizationPolicyExceptionInput struct {
	PolicyID       string    `json:"policy_id"`
	RepositoryID   string    `json:"repository_id"`
	Rule           string    `json:"rule"`
	RequestedValue string    `json:"requested_value"`
	Reason         string    `json:"reason"`
	ExpiresAt      time.Time `json:"expires_at"`
}
type organizationPolicyDecisionInput struct {
	Decision string `json:"decision"`
}
type organizationInitiativeInput struct {
	Title       string                             `json:"title"`
	Description string                             `json:"description"`
	Source      organizations.InitiativeSource     `json:"source"`
	WorkItems   []organizations.InitiativeWorkItem `json:"work_items"`
}
type organizationInitiativeItemInput struct {
	Owner           organizations.InitiativeOwner `json:"owner"`
	Status          string                        `json:"status"`
	ExpectedVersion int                           `json:"expected_version"`
}
type organizationMandateInput struct {
	Title                  string                            `json:"title"`
	DesiredOutcomes        []string                          `json:"desired_outcomes"`
	Repositories           []organizations.MandateRepository `json:"repositories"`
	TrustedSignals         []string                          `json:"trusted_signals"`
	Exclusions             []string                          `json:"exclusions"`
	Budget                 organizations.MandateBudget       `json:"budget"`
	StartsAt               time.Time                         `json:"starts_at"`
	ExpiresAt              time.Time                         `json:"expires_at"`
	AgentID                string                            `json:"agent_id"`
	AllowedActions         []string                          `json:"allowed_actions"`
	RequiredHumanDecisions []string                          `json:"required_human_decisions"`
	OpportunityPolicies    []organizations.OpportunityPolicy `json:"opportunity_policies"`
	Reason                 string                            `json:"reason"`
	ExpectedVersion        int                               `json:"expected_version"`
}

type organizationOpportunityEvaluationInput struct {
	Findings []organizations.OpportunityFinding `json:"findings"`
}
type organizationOpportunityPromotionInput struct {
	ExpectedVersion int    `json:"expected_version"`
	Title           string `json:"title"`
	Body            string `json:"body"`
	BaseRevision    string `json:"base_revision"`
	AgentMinutes    int    `json:"agent_minutes"`
	Tasks           []struct {
		Title              string `json:"title"`
		OwnerType          string `json:"owner_type"`
		OwnerID            string `json:"owner_id"`
		CompletionCriteria string `json:"completion_criteria"`
		Risk               string `json:"risk"`
		VerificationPlan   string `json:"verification_plan"`
		DependsOnPrevious  bool   `json:"depends_on_previous"`
	} `json:"tasks"`
}

func mandateRevision(in organizationMandateInput) organizations.MandateRevision {
	return organizations.MandateRevision{DesiredOutcomes: in.DesiredOutcomes, Repositories: in.Repositories, TrustedSignals: in.TrustedSignals, Exclusions: in.Exclusions, Budget: in.Budget, StartsAt: in.StartsAt.UTC(), ExpiresAt: in.ExpiresAt.UTC(), AgentID: in.AgentID, AllowedActions: in.AllowedActions, RequiredHumanDecisions: in.RequiredHumanDecisions, OpportunityPolicies: in.OpportunityPolicies, Reason: in.Reason}
}

func registerOrganizationRoutes(mux *http.ServeMux, gitStore *storage.Store, orgs *organizations.Store, repos *repositories.Store, usersStore *users.Store, credentials *auth.Store, activityStore *activities.Store, proposalStore *proposals.Store, pullStore *pullrequests.Store, releaseStore *releases.Store, packageStore *packages.Store, incidentStore *incidents.Store, relationshipStore *relationships.Store, securityStore *securityadvisories.Store) {
	project := func(v organizations.Organization, actor string) organizations.Organization {
		now := time.Now().UTC()
		for i := range v.StewardshipMandates {
			m := &v.StewardshipMandates[i]
			if m.Status != "revoked" && len(m.Revisions) > 0 && !m.Revisions[len(m.Revisions)-1].ExpiresAt.After(now) {
				m.Status = "expired"
			}
		}
		// Initiatives are returned only through the portfolio projection, where
		// private source authorization and live ownership can be revalidated.
		v.Initiatives = []organizations.Initiative{}
		if !organizations.HasRole(v, actor, "") {
			kept := []organizations.Invitation{}
			for _, x := range v.Invitations {
				if x.UserID == actor {
					kept = append(kept, x)
				}
			}
			v.Members = []organizations.Member{}
			v.Invitations = kept
			v.Transfers = []organizations.Transfer{}
			v.Teams = []organizations.Team{}
			v.Agents = []organizations.Agent{}
			v.AccessGrants = []organizations.AccessGrant{}
			v.AccessRequests = []organizations.AccessRequest{}
			v.Policies = []organizations.Policy{}
			v.PolicyExceptions = []organizations.PolicyException{}
			v.Initiatives = []organizations.Initiative{}
			v.StewardshipMandates = []organizations.StewardshipMandate{}
			v.Events = []organizations.Event{}
			return v
		}
		if !organizations.HasRole(v, actor, "owner") {
			kept := []organizations.Invitation{}
			for _, x := range v.Invitations {
				if x.UserID == actor {
					kept = append(kept, x)
				}
			}
			v.Invitations = kept
			requests := []organizations.AccessRequest{}
			for _, x := range v.AccessRequests {
				if x.RequesterID == actor {
					requests = append(requests, x)
				}
			}
			v.AccessRequests = requests
		}
		return v
	}
	require := func(w http.ResponseWriter, r *http.Request, scope string) (auth.Credential, organizations.Organization, bool) {
		actor, ok := authenticateRequest(w, r, credentials, scope, false)
		if !ok {
			return actor, organizations.Organization{}, false
		}
		v, err := orgs.Get(r.PathValue("id"))
		if err != nil || (!organizations.HasRole(v, actor.UserID, "") && !hasInvite(v, actor.UserID)) {
			writeAPIError(w, 404, "organization_not_found", "organization not found")
			return actor, v, false
		}
		return actor, v, true
	}
	mux.HandleFunc("POST /organizations", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:write", false)
		if !ok {
			return
		}
		var in organizationInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_organization", "name, slug, and optional description are required")
			return
		}
		v, err := orgs.Create(in.Name, in.Slug, in.Description, actor.UserID)
		if writeOrganizationError(w, err) {
			return
		}
		w.Header().Set("Location", "/organizations/"+v.ID)
		writeJSON(w, 201, v)
	})
	mux.HandleFunc("GET /organizations", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		items, err := orgs.ListFor(actor.UserID)
		if writeOrganizationError(w, err) {
			return
		}
		for i := range items {
			items[i] = project(items[i], actor.UserID)
		}
		writeJSON(w, 200, map[string]any{"organizations": items})
	})
	mux.HandleFunc("GET /organizations/{id}", func(w http.ResponseWriter, r *http.Request) {
		actor, v, ok := require(w, r, "repositories:read")
		if !ok {
			return
		}
		writeJSON(w, 200, project(v, actor.UserID))
	})
	mux.HandleFunc("GET /organizations/{id}/directory", func(w http.ResponseWriter, r *http.Request) {
		v, err := orgs.Get(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 404, "organization_not_found", "organization not found")
			return
		}
		actor, present, authOK := authenticateOptionalRequest(w, r, credentials, "repositories:read", false)
		if !authOK {
			return
		}
		member := present && organizations.HasRole(v, actor.UserID, "")
		visibleRepos := map[string]bool{}
		items, listErr := repos.ListOrganization(v.ID)
		if listErr != nil {
			writeRepositoryError(w, listErr)
			return
		}
		for _, repo := range items {
			if member || repo.Visibility == repositories.Public {
				visibleRepos[repo.ID] = true
			}
		}
		writeJSON(w, 200, organizations.ProjectDirectory(v, member, visibleRepos))
	})
	mux.HandleFunc("POST /organizations/{id}/stewardship-mandates", func(w http.ResponseWriter, r *http.Request) {
		actor, organization, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var in organizationMandateInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_stewardship_mandate", "bounded mandate content is required")
			return
		}
		portfolio, err := repos.ListOrganization(organization.ID)
		if err != nil {
			writeRepositoryError(w, err)
			return
		}
		owned := map[string]bool{}
		for _, repo := range portfolio {
			owned[repo.ID] = true
		}
		for _, scope := range in.Repositories {
			if !owned[scope.RepositoryID] {
				writeAPIError(w, 400, "invalid_stewardship_mandate", "every repository must belong to the organization")
				return
			}
		}
		_, mandate, err := orgs.CreateStewardshipMandate(organization.ID, actor.UserID, in.Title, mandateRevision(in))
		if writeOrganizationError(w, err) {
			return
		}
		w.Header().Set("Location", "/organizations/"+organization.ID+"/stewardship-mandates/"+mandate.ID)
		writeJSON(w, 201, mandate)
	})
	mux.HandleFunc("PUT /organizations/{id}/stewardship-mandates/{mandate_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, organization, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var in organizationMandateInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_stewardship_mandate", "revision and expected_version are required")
			return
		}
		portfolio, err := repos.ListOrganization(organization.ID)
		if err != nil {
			writeRepositoryError(w, err)
			return
		}
		owned := map[string]bool{}
		for _, repo := range portfolio {
			owned[repo.ID] = true
		}
		for _, scope := range in.Repositories {
			if !owned[scope.RepositoryID] {
				writeAPIError(w, 400, "invalid_stewardship_mandate", "every repository must belong to the organization")
				return
			}
		}
		_, mandate, err := orgs.ReviseStewardshipMandate(organization.ID, r.PathValue("mandate_id"), actor.UserID, in.ExpectedVersion, mandateRevision(in))
		if writeOrganizationError(w, err) {
			return
		}
		writeJSON(w, 200, mandate)
	})
	mux.HandleFunc("POST /organizations/{id}/stewardship-mandates/{mandate_id}/accept", func(w http.ResponseWriter, r *http.Request) {
		actor, organization, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int `json:"expected_version"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_stewardship_mandate", "expected_version is required")
			return
		}
		_, mandate, err := orgs.AcceptStewardshipMandate(organization.ID, r.PathValue("mandate_id"), actor.UserID, in.ExpectedVersion)
		if writeOrganizationError(w, err) {
			return
		}
		writeJSON(w, 200, mandate)
	})
	mux.HandleFunc("POST /organizations/{id}/stewardship-mandates/{mandate_id}/{action}", func(w http.ResponseWriter, r *http.Request) {
		action := r.PathValue("action")
		if action != "pause" && action != "resume" && action != "revoke" {
			writeAPIError(w, 404, "stewardship_mandate_not_found", "mandate action not found")
			return
		}
		actor, organization, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int `json:"expected_version"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_stewardship_mandate", "expected_version is required")
			return
		}
		_, mandate, err := orgs.ChangeStewardshipMandateState(organization.ID, r.PathValue("mandate_id"), actor.UserID, action, in.ExpectedVersion)
		if writeOrganizationError(w, err) {
			return
		}
		writeJSON(w, 200, mandate)
	})
	mux.HandleFunc("GET /organizations/{id}/stewardship-mandates/{mandate_id}/opportunities", func(w http.ResponseWriter, r *http.Request) {
		_, organization, ok := require(w, r, "repositories:read")
		if !ok {
			return
		}
		for _, mandate := range organization.StewardshipMandates {
			if mandate.ID == r.PathValue("mandate_id") {
				items := append([]organizations.StewardshipOpportunity(nil), mandate.Opportunities...)
				slices.SortStableFunc(items, func(a, b organizations.StewardshipOpportunity) int {
					if a.Rank != b.Rank {
						return a.Rank - b.Rank
					}
					return b.UpdatedAt.Compare(a.UpdatedAt)
				})
				writeJSON(w, 200, map[string]any{"items": items, "mandate_id": mandate.ID, "mandate_version": mandate.Version})
				return
			}
		}
		writeAPIError(w, 404, "stewardship_mandate_not_found", "mandate not found")
	})
	mux.HandleFunc("GET /organizations/{id}/stewardship-mandates/{mandate_id}/report", func(w http.ResponseWriter, r *http.Request) {
		actor, organization, ok := require(w, r, "repositories:read")
		if !ok {
			return
		}
		report, err := orgs.StewardshipReport(organization.ID, r.PathValue("mandate_id"), actor.UserID)
		if writeOrganizationError(w, err) {
			return
		}
		writeJSON(w, 200, report)
	})
	mux.HandleFunc("PUT /organizations/{id}/stewardship-mandates/{mandate_id}/tuning", func(w http.ResponseWriter, r *http.Request) {
		actor, organization, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int `json:"expected_version"`
			organizations.StewardshipTuning
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_stewardship_tuning", "bounded tuning and expected_version are required")
			return
		}
		_, mandate, err := orgs.TuneStewardshipMandate(organization.ID, r.PathValue("mandate_id"), actor.UserID, in.ExpectedVersion, in.StewardshipTuning)
		if writeOrganizationError(w, err) {
			return
		}
		writeJSON(w, 200, mandate)
	})
	mux.HandleFunc("POST /organizations/{id}/stewardship-mandates/{mandate_id}/outcomes", func(w http.ResponseWriter, r *http.Request) {
		actor, organization, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var in organizations.StewardshipOutcome
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_stewardship_outcome", "a bounded outcome is required")
			return
		}
		_, mandate, err := orgs.RecordStewardshipOutcome(organization.ID, r.PathValue("mandate_id"), actor.UserID, in)
		if writeOrganizationError(w, err) {
			return
		}
		writeJSON(w, 201, mandate)
	})
	mux.HandleFunc("POST /organizations/{id}/stewardship-mandates/{mandate_id}/evaluations", func(w http.ResponseWriter, r *http.Request) {
		actor, organization, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var in organizationOpportunityEvaluationInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_stewardship_evaluation", "bounded findings are required")
			return
		}
		_, items, err := orgs.PublishStewardshipOpportunities(organization.ID, r.PathValue("mandate_id"), actor.UserID, in.Findings)
		if writeOrganizationError(w, err) {
			return
		}
		writeJSON(w, 200, map[string]any{"items": items})
	})
	mux.HandleFunc("POST /organizations/{id}/stewardship-mandates/{mandate_id}/opportunities/{opportunity_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, organization, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var in organizations.OpportunityDecision
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_stewardship_decision", "an action and expected_version are required")
			return
		}
		_, item, err := orgs.DecideStewardshipOpportunity(organization.ID, r.PathValue("mandate_id"), r.PathValue("opportunity_id"), actor.UserID, in)
		if writeOrganizationError(w, err) {
			return
		}
		if (in.Action == "approve" || in.Action == "reject") && activityStore != nil {
			for _, target := range item.AffectedOwnerIDs {
				if target == actor.UserID {
					continue
				}
				targetID := target
				recordActivity(activityStore, repos, activities.Event{Kind: "stewardship_opportunity." + in.Action, ActorID: actor.UserID, RepositoryID: item.RepositoryID, ResourceType: "organization", ResourceID: organization.ID, ResourceTitle: item.Title, TargetUserID: &targetID})
			}
		}
		writeJSON(w, 200, item)
	})
	mux.HandleFunc("POST /organizations/{id}/stewardship-mandates/{mandate_id}/opportunities/{opportunity_id}/promotion", func(w http.ResponseWriter, r *http.Request) {
		actor, organization, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var in organizationOpportunityPromotionInput
		if decodeJSON(r, &in) != nil || len(in.Tasks) == 0 || len(in.Tasks) > 20 || len(in.BaseRevision) != 40 {
			writeAPIError(w, 400, "invalid_opportunity_promotion", "current base revision and one to twenty owned tasks are required")
			return
		}
		if strings.TrimSpace(in.Title) == "" || len(in.Title) > 200 || strings.TrimSpace(in.Body) == "" || len(in.Body) > 10000 {
			writeAPIError(w, 400, "invalid_opportunity_promotion", "proposal title and body are required")
			return
		}
		var currentMandate *organizations.StewardshipMandate
		for i := range organization.StewardshipMandates {
			if organization.StewardshipMandates[i].ID == r.PathValue("mandate_id") {
				currentMandate = &organization.StewardshipMandates[i]
			}
		}
		if currentMandate == nil || len(currentMandate.Revisions) == 0 {
			writeAPIError(w, 404, "stewardship_mandate_not_found", "mandate not found")
			return
		}
		stewardAgentID := currentMandate.Revisions[len(currentMandate.Revisions)-1].AgentID
		for _, task := range in.Tasks {
			if strings.TrimSpace(task.Title) == "" || len(task.Title) > 200 || strings.TrimSpace(task.CompletionCriteria) == "" || len(task.CompletionCriteria) > 4000 || strings.TrimSpace(task.Risk) == "" || len(task.Risk) > 4000 || strings.TrimSpace(task.VerificationPlan) == "" || len(task.VerificationPlan) > 4000 {
				writeAPIError(w, 400, "invalid_opportunity_promotion", "each task requires completion criteria, risk, and a verification plan")
				return
			}
			validOwner := false
			if task.OwnerType == "human" {
				for _, member := range organization.Members {
					validOwner = validOwner || member.UserID == task.OwnerID
				}
			}
			if task.OwnerType == "agent" {
				for _, agent := range organization.Agents {
					validOwner = validOwner || (agent.ID == task.OwnerID && task.OwnerID == stewardAgentID)
				}
			}
			if !validOwner {
				writeAPIError(w, 400, "invalid_opportunity_owner", "each task owner must be a current organization member or approved agent")
				return
			}
			if task.OwnerType == "agent" && in.AgentMinutes < 1 {
				writeAPIError(w, 400, "invalid_opportunity_budget", "agent-owned tasks require a positive reserved minute budget")
				return
			}
		}
		var opportunity *organizations.StewardshipOpportunity
		for _, mandate := range organization.StewardshipMandates {
			if mandate.ID == r.PathValue("mandate_id") {
				for i := range mandate.Opportunities {
					if mandate.Opportunities[i].ID == r.PathValue("opportunity_id") {
						copy := mandate.Opportunities[i]
						opportunity = &copy
					}
				}
			}
		}
		if opportunity == nil {
			writeAPIError(w, 404, "stewardship_opportunity_not_found", "opportunity not found")
			return
		}
		if proposalStore == nil {
			writeAPIError(w, 503, "proposal_state_unavailable", "proposal conflict state is unavailable")
			return
		}
		if incidentStore == nil {
			writeAPIError(w, 503, "incident_state_unavailable", "incident blocker state is unavailable")
			return
		}
		if opportunity.Work != nil {
			created, proposalErr := proposalStore.Get(opportunity.RepositoryID, opportunity.Work.ProposalID)
			createdTasks, tasksErr := proposalStore.ListTasks(opportunity.RepositoryID, opportunity.Work.ProposalID)
			matches := proposalErr == nil && tasksErr == nil && created.Title == strings.TrimSpace(in.Title) && created.Body == strings.TrimSpace(in.Body) && opportunity.Work.BaseRevision == strings.ToLower(in.BaseRevision) && len(createdTasks) == len(in.Tasks) && len(opportunity.Work.TaskIDs) == len(createdTasks)
			for i := range createdTasks {
				matches = matches && createdTasks[i].ID == opportunity.Work.TaskIDs[i] && createdTasks[i].Title == strings.TrimSpace(in.Tasks[i].Title) && createdTasks[i].Outcome == strings.TrimSpace(in.Tasks[i].CompletionCriteria) && createdTasks[i].Risk == strings.TrimSpace(in.Tasks[i].Risk) && createdTasks[i].VerificationPlan == strings.TrimSpace(in.Tasks[i].VerificationPlan) && createdTasks[i].Assignment != nil && createdTasks[i].Assignment.AssigneeType == in.Tasks[i].OwnerType && createdTasks[i].Assignment.AssigneeID == in.Tasks[i].OwnerID
			}
			if !matches {
				writeAPIError(w, 409, "opportunity_promotion_superseded", "the opportunity is already linked to different work")
				return
			}
			writeJSON(w, 200, map[string]any{"opportunity": opportunity, "proposal": created, "tasks": createdTasks})
			return
		}
		repository, err := repos.GetByID(opportunity.RepositoryID)
		if err != nil || repository.OrganizationID != organization.ID {
			writeAPIError(w, 409, "repository_stewardship_changed", "repository stewardship changed")
			return
		}
		participant, participantErr := repos.HasCollaborator(actor.UserID, repository.ID)
		if participantErr != nil || (repository.OwnerID != actor.UserID && !participant) {
			writeAPIError(w, 403, "repository_write_forbidden", "current repository participation is required")
			return
		}
		gitRepository, err := gitStore.Open(repository.ID)
		if err != nil {
			writeAPIError(w, 409, "base_revision_unavailable", "repository base is unavailable")
			return
		}
		ref, err := gitRepository.ReadReference("refs/heads/" + repository.DefaultBranch)
		if err != nil {
			writeAPIError(w, 409, "base_revision_unavailable", "default branch is unavailable")
			return
		}
		blockers := []string{}
		if ref.Target != strings.ToLower(in.BaseRevision) {
			blockers = append(blockers, "base_revision_changed")
		}
		incidentList, listErr := incidentStore.List()
		if listErr != nil {
			writeAPIError(w, 503, "incident_state_unavailable", "incident blocker state is unavailable")
			return
		}
		for _, incident := range incidentList {
			if incident.Status == "resolved" {
				continue
			}
			for _, scope := range incident.Scopes {
				if scope.RepositoryID == repository.ID {
					blockers = append(blockers, "active_incident:"+incident.ID)
				}
			}
		}
		if opportunity.EvidenceType == "security" && securityStore != nil {
			if advisory, getErr := securityStore.Get(opportunity.EvidenceID); getErr != nil {
				blockers = append(blockers, "security_embargo_state_unverified")
			} else if advisory.EmbargoState != "disclosed" {
				blockers = append(blockers, "embargoed_evidence:"+advisory.ID)
			}
		} else if opportunity.EvidenceType == "security" {
			blockers = append(blockers, "security_embargo_state_unavailable")
		}
		existing, listErr := proposalStore.List(repository.ID)
		if listErr != nil {
			writeAPIError(w, 503, "proposal_state_unavailable", "proposal conflict state is unavailable")
			return
		}
		for _, proposal := range existing {
			if proposal.Status == proposals.Open && strings.EqualFold(strings.TrimSpace(proposal.Title), strings.TrimSpace(in.Title)) {
				blockers = append(blockers, "conflicting_work:"+proposal.ID)
			}
		}
		reserved, err := orgs.ReserveStewardshipOpportunity(organization.ID, r.PathValue("mandate_id"), opportunity.ID, actor.UserID, in.ExpectedVersion, in.AgentMinutes, blockers)
		if err != nil {
			writeOrganizationError(w, err)
			return
		}
		if len(reserved.Blockers) > 0 {
			writeJSON(w, 409, reserved)
			return
		}
		items := []proposals.ReasoningItem{{ID: opportunity.ID, Kind: "opportunity", Summary: opportunity.Summary, Status: "accepted"}, {ID: opportunity.EvidenceID, Kind: opportunity.EvidenceType, Summary: opportunity.EvidenceRevision, Status: "cited"}}
		for index, citation := range opportunity.Citations {
			items = append(items, proposals.ReasoningItem{ID: opportunity.ID + "-citation-" + strconv.Itoa(index+1), Kind: "citation:" + citation.Kind, Summary: citation.Label + " @ " + citation.Revision, Status: map[bool]string{true: "stale", false: "current"}[citation.Stale]})
		}
		tasks := make([]proposals.ImplementationTaskInput, 0, len(in.Tasks))
		for index, task := range in.Tasks {
			items = append(items, proposals.ReasoningItem{ID: opportunity.ID + string(rune('a'+index)), Kind: "risk", Summary: task.Risk + " | verify: " + task.VerificationPlan, Status: "accepted"})
			tasks = append(tasks, proposals.ImplementationTaskInput{Title: task.Title, Outcome: task.CompletionCriteria, Risk: task.Risk, VerificationPlan: task.VerificationPlan, AssigneeType: task.OwnerType, AssigneeID: task.OwnerID, DependsOnPrevious: task.DependsOnPrevious})
		}
		created, createdTasks, err := proposalStore.CreateImplementation(proposals.ImplementationInput{RepositoryID: repository.ID, ActorID: actor.UserID, Title: in.Title, Body: in.Body, Origin: proposals.ReasoningOrigin{AssessmentID: opportunity.ID, AssessmentVersion: reserved.Version, Revision: ref.Target, SelectedItemIDs: []string{opportunity.ID}, Items: items, AnalysisStatus: "stewardship_opportunity", OrganizationID: organization.ID, MandateID: r.PathValue("mandate_id"), OpportunityID: opportunity.ID}, Tasks: tasks})
		if err != nil {
			writeProposalError(w, err)
			return
		}
		matches := created.Title == strings.TrimSpace(in.Title) && created.Body == strings.TrimSpace(in.Body) && len(createdTasks) == len(in.Tasks)
		for i := range createdTasks {
			matches = matches && createdTasks[i].Title == strings.TrimSpace(in.Tasks[i].Title) && createdTasks[i].Outcome == strings.TrimSpace(in.Tasks[i].CompletionCriteria) && createdTasks[i].Risk == strings.TrimSpace(in.Tasks[i].Risk) && createdTasks[i].VerificationPlan == strings.TrimSpace(in.Tasks[i].VerificationPlan) && createdTasks[i].Assignment != nil && createdTasks[i].Assignment.AssigneeType == in.Tasks[i].OwnerType && createdTasks[i].Assignment.AssigneeID == in.Tasks[i].OwnerID
		}
		if !matches {
			writeAPIError(w, 409, "opportunity_promotion_superseded", "the promotion reservation already contains different work")
			return
		}
		taskIDs := make([]string, len(createdTasks))
		for i := range createdTasks {
			taskIDs[i] = createdTasks[i].ID
		}
		linked, err := orgs.LinkStewardshipOpportunityWork(organization.ID, r.PathValue("mandate_id"), opportunity.ID, actor.UserID, created.ID, ref.Target, taskIDs)
		if err != nil {
			writeOrganizationError(w, err)
			return
		}
		recordActivity(activityStore, repos, activities.Event{Kind: "stewardship_opportunity.promoted", ActorID: actor.UserID, RepositoryID: repository.ID, ResourceType: "proposal", ResourceID: created.ID, ResourceTitle: created.Title})
		recordTaskTransitions(activityStore, repos, actor.UserID, repository.ID, created.ID, nil, createdTasks)
		writeJSON(w, 201, map[string]any{"opportunity": linked, "proposal": created, "tasks": createdTasks})
	})
	mux.HandleFunc("GET /organizations/{id}/stewardship-mandates/{mandate_id}/preview", func(w http.ResponseWriter, r *http.Request) {
		_, organization, ok := require(w, r, "repositories:read")
		if !ok {
			return
		}
		var mandate *organizations.StewardshipMandate
		for i := range organization.StewardshipMandates {
			if organization.StewardshipMandates[i].ID == r.PathValue("mandate_id") {
				mandate = &organization.StewardshipMandates[i]
			}
		}
		if mandate == nil {
			writeAPIError(w, 404, "stewardship_mandate_not_found", "mandate not found")
			return
		}
		latest := mandate.Revisions[len(mandate.Revisions)-1]
		now := time.Now().UTC()
		grants := []organizations.AccessGrant{}
		policies := map[string]organizations.EffectivePolicy{}
		for _, scope := range latest.Repositories {
			for _, grant := range organization.AccessGrants {
				if grant.PrincipalType != "agent" || grant.PrincipalID != latest.AgentID || grant.RevokedAt != nil || (grant.ExpiresAt != nil && !grant.ExpiresAt.After(now)) {
					continue
				}
				for _, resource := range grant.Resources {
					if resource.Kind == "repository" && resource.ID == scope.RepositoryID {
						grants = append(grants, grant)
					}
				}
			}
			policies[scope.RepositoryID] = organizations.EffectivePolicies(organization, scope.RepositoryID, organizations.ResponsibleTeamIDs(organization, scope.RepositoryID), false, now)
		}
		status := mandate.Status
		if status != "revoked" && !latest.ExpiresAt.After(now) {
			status = "expired"
		}
		writeJSON(w, 200, map[string]any{"mandate_id": mandate.ID, "version": mandate.Version, "status": status, "access_grants": grants, "effective_policies": policies, "implicit_authority": []string{}, "notice": "This mandate grants no repository write, Git, review, credential, deployment, or merge authority. Only separate live grants apply."})
	})
	mux.HandleFunc("POST /organizations/{id}/teams", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var in organizationTeamInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_team", "team content is required")
			return
		}
		if in.Visibility == "" {
			in.Visibility = "organization"
		}
		v, err := orgs.CreateTeam(r.PathValue("id"), actor.UserID, in.Name, in.Slug, in.Description, in.ParentID, in.Visibility)
		if writeOrganizationError(w, err) {
			return
		}
		w.Header().Set("Location", "/organizations/"+v.ID+"/teams/"+v.Teams[len(v.Teams)-1].ID)
		writeJSON(w, 201, project(v, actor.UserID))
	})
	mux.HandleFunc("PUT /organizations/{id}/teams/{team_id}/members", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var in organizationTeamMemberInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_team_member", "user_id, role, and expected_version are required")
			return
		}
		if _, err := usersStore.Get(in.UserID); err != nil {
			writeAPIError(w, 400, "invalid_team_member", "user_id must identify an existing user")
			return
		}
		v, err := orgs.AddTeamMember(r.PathValue("id"), r.PathValue("team_id"), actor.UserID, in.UserID, in.Role, in.ExpectedVersion)
		if writeOrganizationError(w, err) {
			return
		}
		writeJSON(w, 200, project(v, actor.UserID))
	})
	mux.HandleFunc("DELETE /organizations/{id}/teams/{team_id}/members/{user_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int `json:"expected_version"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_team_member", "expected_version is required")
			return
		}
		_, err := orgs.RemoveTeamMember(r.PathValue("id"), r.PathValue("team_id"), actor.UserID, r.PathValue("user_id"), in.ExpectedVersion)
		if writeOrganizationError(w, err) {
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("POST /organizations/{id}/teams/{team_id}/responsibilities", func(w http.ResponseWriter, r *http.Request) {
		actor, v, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var in organizationResponsibilityInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_responsibility", "repository_id, area, and expected_version are required")
			return
		}
		found := false
		items, err := repos.ListOrganization(v.ID)
		if err == nil {
			for _, repo := range items {
				if repo.ID == in.RepositoryID {
					found = true
				}
			}
		}
		if err != nil {
			writeRepositoryError(w, err)
			return
		}
		if !found {
			writeAPIError(w, 400, "invalid_responsibility", "repository must belong to the organization")
			return
		}
		changed, err := orgs.AddResponsibility(v.ID, r.PathValue("team_id"), actor.UserID, in.RepositoryID, in.Area, in.Description, in.ExpectedVersion, func(publish func() error) error {
			return repos.WithOrganization(in.RepositoryID, v.ID, publish)
		})
		if errors.Is(err, repositories.ErrNotFound) {
			writeAPIError(w, 409, "organization_conflict", "repository stewardship changed before responsibility publication")
			return
		}
		if writeOrganizationError(w, err) {
			return
		}
		writeJSON(w, 201, project(changed, actor.UserID))
	})
	mux.HandleFunc("POST /organizations/{id}/agents", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var in organizationAgentInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_agent", "agent identity is required")
			return
		}
		if in.Visibility == "" {
			in.Visibility = "organization"
		}
		v, err := orgs.RegisterAgent(r.PathValue("id"), actor.UserID, in.Name, in.Slug, in.Description, in.Visibility, in.Capabilities, in.OperatorIDs, in.TeamIDs)
		if writeOrganizationError(w, err) {
			return
		}
		w.Header().Set("Location", "/organizations/"+v.ID+"/agents/"+v.Agents[len(v.Agents)-1].ID)
		writeJSON(w, 201, project(v, actor.UserID))
	})
	mux.HandleFunc("PUT /organizations/{id}/agents/{agent_id}/profile", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var in organizationAgentProfileInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_agent_profile", "a complete versioned agent profile is required")
			return
		}
		v, err := orgs.PublishAgentProfile(r.PathValue("id"), r.PathValue("agent_id"), actor.UserID, in.ExpectedVersion, in.Profile)
		if writeOrganizationError(w, err) {
			return
		}
		writeJSON(w, 200, project(v, actor.UserID))
	})
	mux.HandleFunc("POST /organizations/{id}/access-requests", func(w http.ResponseWriter, r *http.Request) {
		actor, organization, ok := require(w, r, "repositories:read")
		if !ok {
			return
		}
		var in organizationAccessInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_access_request", "principal, role, resources, reason, and optional expiry are required")
			return
		}
		portfolioRepositories, err := repos.ListOrganization(organization.ID)
		if err != nil {
			writeRepositoryError(w, err)
			return
		}
		portfolio := map[string]bool{}
		for _, repository := range portfolioRepositories {
			portfolio[repository.ID] = true
		}
		for _, resource := range in.Resources {
			if resource.Kind == "repository" && !portfolio[resource.ID] {
				writeAPIError(w, 400, "invalid_access_request", "repository resources must belong to the organization")
				return
			}
		}
		v, err := orgs.CreateAccessRequest(r.PathValue("id"), actor.UserID, in.PrincipalType, in.PrincipalID, in.Role, in.Reason, in.Resources, in.Exceptions, in.ExpiresAt)
		if writeOrganizationError(w, err) {
			return
		}
		writeJSON(w, 201, project(v, actor.UserID))
	})
	mux.HandleFunc("POST /organizations/{id}/policies", func(w http.ResponseWriter, r *http.Request) {
		actor, organization, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var in organizationPolicyInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_policy", "policy content is required")
			return
		}
		portfolio, err := repos.ListOrganization(organization.ID)
		if err != nil {
			writeRepositoryError(w, err)
			return
		}
		owned := map[string]bool{}
		for _, repo := range portfolio {
			owned[repo.ID] = true
		}
		for _, target := range in.Targets {
			if target.Kind == "repository" && !owned[target.ID] {
				writeAPIError(w, 400, "invalid_policy", "repository targets must belong to the organization")
				return
			}
		}
		v, err := orgs.CreatePolicy(organization.ID, actor.UserID, in.Name, in.Description, in.Targets, in.Rules)
		if writeOrganizationError(w, err) {
			return
		}
		p := v.Policies[len(v.Policies)-1]
		w.Header().Set("Location", "/organizations/"+v.ID+"/policies/"+p.ID)
		writeJSON(w, 201, p)
	})
	mux.HandleFunc("GET /organizations/{id}/policies/preview", func(w http.ResponseWriter, r *http.Request) {
		_, organization, ok := require(w, r, "repositories:read")
		if !ok {
			return
		}
		repositoryID := r.URL.Query().Get("repository_id")
		repo, err := repos.GetByID(repositoryID)
		if err != nil || repo.OrganizationID != organization.ID {
			writeAPIError(w, 404, "repository_not_found", "organization repository not found")
			return
		}
		teams := []string{}
		for _, team := range organization.Teams {
			for _, responsibility := range team.Responsibilities {
				if responsibility.RepositoryID == repositoryID {
					teams = append(teams, team.ID)
				}
			}
		}
		writeJSON(w, 200, organizations.EffectivePolicies(organization, repositoryID, teams, true, time.Now().UTC()))
	})
	mux.HandleFunc("GET /organizations/{id}/policies/effective", func(w http.ResponseWriter, r *http.Request) {
		_, organization, ok := require(w, r, "repositories:read")
		if !ok {
			return
		}
		repositoryID := r.URL.Query().Get("repository_id")
		repo, err := repos.GetByID(repositoryID)
		if err != nil || repo.OrganizationID != organization.ID {
			writeAPIError(w, 404, "repository_not_found", "organization repository not found")
			return
		}
		teams := []string{}
		for _, team := range organization.Teams {
			for _, responsibility := range team.Responsibilities {
				if responsibility.RepositoryID == repositoryID {
					teams = append(teams, team.ID)
				}
			}
		}
		writeJSON(w, 200, organizations.EffectivePolicies(organization, repositoryID, teams, false, time.Now().UTC()))
	})
	mux.HandleFunc("POST /organizations/{id}/policies/{policy_id}/activate", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var in organizationPolicyActivationInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_policy", "expected_version is required")
			return
		}
		v, err := orgs.ActivatePolicy(r.PathValue("id"), r.PathValue("policy_id"), actor.UserID, in.ExpectedVersion)
		if writeOrganizationError(w, err) {
			return
		}
		writeJSON(w, 200, project(v, actor.UserID))
	})
	mux.HandleFunc("POST /organizations/{id}/policy-exceptions", func(w http.ResponseWriter, r *http.Request) {
		actor, organization, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var in organizationPolicyExceptionInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_policy_exception", "exception content is required")
			return
		}
		repo, err := repos.GetByID(in.RepositoryID)
		if err != nil || repo.OrganizationID != organization.ID {
			writeAPIError(w, 404, "repository_not_found", "organization repository not found")
			return
		}
		v, err := orgs.RequestPolicyException(organization.ID, actor.UserID, in.PolicyID, in.RepositoryID, in.Rule, in.RequestedValue, in.Reason, in.ExpiresAt)
		if writeOrganizationError(w, err) {
			return
		}
		x := v.PolicyExceptions[len(v.PolicyExceptions)-1]
		w.Header().Set("Location", "/organizations/"+v.ID+"/policy-exceptions/"+x.ID)
		writeJSON(w, 201, x)
	})
	mux.HandleFunc("POST /organizations/{id}/policy-exceptions/{exception_id}/decision", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var in organizationPolicyDecisionInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_policy_exception", "decision is required")
			return
		}
		v, err := orgs.DecidePolicyException(r.PathValue("id"), r.PathValue("exception_id"), actor.UserID, in.Decision)
		if writeOrganizationError(w, err) {
			return
		}
		writeJSON(w, 200, project(v, actor.UserID))
	})
	mux.HandleFunc("POST /organizations/{id}/access-requests/{request_id}/decision", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var in struct {
			Decision string `json:"decision"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_access_decision", "decision is required")
			return
		}
		v, err := orgs.DecideAccessRequest(r.PathValue("id"), r.PathValue("request_id"), actor.UserID, in.Decision, func(request organizations.AccessRequest) error {
			current, currentErr := orgs.Get(r.PathValue("id"))
			if currentErr != nil {
				return organizations.ErrConflict
			}
			for _, resource := range request.Resources {
				if resource.Kind != "repository" {
					continue
				}
				repository, repositoryErr := repos.GetByID(resource.ID)
				if repositoryErr != nil || repository.OrganizationID != r.PathValue("id") {
					return organizations.ErrConflict
				}
				if request.PrincipalType == "agent" && organizations.EffectivePolicies(current, resource.ID, organizations.ResponsibleTeamIDs(current, resource.ID), false, time.Now().UTC()).Rules.AgentAuthority == "disabled" {
					return organizations.ErrConflict
				}
			}
			return nil
		})
		if writeOrganizationError(w, err) {
			return
		}
		writeJSON(w, 200, project(v, actor.UserID))
	})
	mux.HandleFunc("DELETE /organizations/{id}/access-grants/{grant_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int `json:"expected_version"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_access_grant", "expected_version is required")
			return
		}
		_, err := orgs.RevokeAccessGrant(r.PathValue("id"), r.PathValue("grant_id"), actor.UserID, in.ExpectedVersion, func(c organizations.DerivedCredential) error {
			_, e := credentials.Revoke(c.OperatorID, c.ID)
			if errors.Is(e, auth.ErrNotFound) {
				return nil
			}
			return e
		})
		if writeOrganizationError(w, err) {
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("POST /organizations/{id}/access-grants/{grant_id}/credentials", func(w http.ResponseWriter, r *http.Request) {
		actor, v, ok := require(w, r, "repositories:read")
		if !ok {
			return
		}
		var in struct {
			AgentID      string `json:"agent_id"`
			RepositoryID string `json:"repository_id"`
			ExpiresIn    int    `json:"expires_in"`
			Purpose      string `json:"purpose"`
		}
		if decodeJSON(r, &in) != nil || in.ExpiresIn < 60 {
			writeAPIError(w, 400, "invalid_access_credential", "agent_id, repository_id, and expires_in of at least 60 seconds are required")
			return
		}
		repo, err := repos.GetByID(in.RepositoryID)
		if err != nil || repo.OrganizationID != v.ID {
			writeAPIError(w, 404, "repository_not_found", "organization repository not found")
			return
		}
		var grant *organizations.AccessGrant
		for i := range v.AccessGrants {
			if v.AccessGrants[i].ID == r.PathValue("grant_id") {
				grant = &v.AccessGrants[i]
			}
		}
		if grant == nil || grant.PrincipalType != "agent" || grant.PrincipalID != in.AgentID {
			writeAPIError(w, 404, "access_grant_not_found", "live agent grant not found")
			return
		}
		if organizations.EffectivePolicies(v, in.RepositoryID, organizations.ResponsibleTeamIDs(v, in.RepositoryID), false, time.Now().UTC()).Rules.AgentAuthority == "disabled" {
			writeAPIError(w, 409, "organization_conflict", "organization policy disables new agent authority for this repository")
			return
		}
		lifetime := time.Duration(in.ExpiresIn) * time.Second
		if grant.ExpiresAt != nil && time.Now().Add(lifetime).After(*grant.ExpiresAt) {
			lifetime = time.Until(*grant.ExpiresAt)
		}
		scopes := []string{"git:read"}
		if in.Purpose == "api_read" {
			scopes = []string{"repositories:read"}
		} else if grant.Role != "viewer" {
			scopes = append(scopes, "git:write")
		}
		issued, err := credentials.IssueOrganizationAgent(actor.UserID, "Organization agent "+in.AgentID, v.ID, grant.ID, in.AgentID, in.RepositoryID, scopes, lifetime)
		if err != nil {
			writeAPIError(w, 400, "invalid_access_credential", "grant cannot issue the requested credential")
			return
		}
		_, err = orgs.RecordDerivedCredential(v.ID, grant.ID, in.AgentID, actor.UserID, issued.ID, organizations.ResourceScope{Kind: "repository", ID: in.RepositoryID})
		if err != nil {
			_, _ = credentials.Revoke(actor.UserID, issued.ID)
			if writeOrganizationError(w, err) {
				return
			}
		}
		writeJSON(w, 201, issued)
	})
	mux.HandleFunc("POST /organizations/{id}/invitations", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var in organizationInviteInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_invitation", "user_id is required")
			return
		}
		if _, err := usersStore.Get(in.UserID); err != nil {
			writeAPIError(w, 400, "invalid_invitation", "user_id must identify an existing user")
			return
		}
		v, err := orgs.Invite(r.PathValue("id"), actor.UserID, in.UserID)
		if writeOrganizationError(w, err) {
			return
		}
		writeJSON(w, 201, project(v, actor.UserID))
	})
	mux.HandleFunc("POST /organizations/{id}/invitations/{invitation_id}/accept", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		v, err := orgs.AcceptInvitation(r.PathValue("id"), r.PathValue("invitation_id"), actor.UserID)
		if writeOrganizationError(w, err) {
			return
		}
		memberIDs := []string{}
		for _, member := range v.Members {
			memberIDs = append(memberIDs, member.UserID)
		}
		repoItems, listErr := repos.ListOrganization(v.ID)
		if writeRepositoryError(w, listErr) {
			return
		}
		for _, repo := range repoItems {
			if _, err = repos.SetOrganization(repo.OwnerID, repo.ID, v.ID, memberIDs); err != nil {
				writeRepositoryError(w, err)
				return
			}
		}
		writeJSON(w, 200, project(v, actor.UserID))
	})
	mux.HandleFunc("DELETE /organizations/{id}/members/{user_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		target := r.PathValue("user_id")
		v, err := orgs.RemoveMember(r.PathValue("id"), actor.UserID, target, func(current organizations.Organization) error {
			ids := []string{}
			for _, grant := range current.AccessGrants {
				if grant.RevokedAt != nil {
					continue
				}
				for _, derived := range grant.DerivedCredentials {
					if derived.OperatorID == target {
						ids = append(ids, derived.ID)
					}
				}
			}
			if len(ids) > 0 {
				return credentials.RevokeBatch(target, ids)
			}
			return nil
		})
		if writeOrganizationError(w, err) {
			return
		}
		repoItems, listErr := repos.ListOrganization(v.ID)
		if writeRepositoryError(w, listErr) {
			return
		}
		for _, repo := range repoItems {
			if err = repos.RemoveOrganizationMember(repo.OwnerID, repo.ID, target); err != nil {
				writeRepositoryError(w, err)
				return
			}
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("POST /organizations/{id}/repositories", func(w http.ResponseWriter, r *http.Request) {
		actor, v, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		if !organizations.HasRole(v, actor.UserID, "") {
			writeAPIError(w, 403, "forbidden", "accept the organization invitation before creating repositories")
			return
		}
		var in organizationRepositoryInput
		if decodeJSON(r, &in) != nil || strings.TrimSpace(in.Name) == "" {
			writeAPIError(w, 400, "invalid_repository", "name is required")
			return
		}
		repo, err := repos.Create(actor.UserID, in.Name)
		if writeRepositoryError(w, err) {
			return
		}
		members := []string{}
		for _, m := range v.Members {
			members = append(members, m.UserID)
		}
		repo, err = repos.SetOrganization(actor.UserID, repo.ID, v.ID, members)
		if err != nil {
			_ = repos.Delete(actor.UserID, repo.ID)
			writeRepositoryError(w, err)
			return
		}
		w.Header().Set("Location", "/repositories/"+repo.ID)
		writeJSON(w, 201, repo)
	})
	mux.HandleFunc("POST /organizations/{id}/repository-transfers", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:write", false)
		if !ok {
			return
		}
		if _, err := orgs.Get(r.PathValue("id")); err != nil {
			writeAPIError(w, 404, "organization_not_found", "organization not found")
			return
		}
		var in struct {
			RepositoryID string `json:"repository_id"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_transfer", "repository_id is required")
			return
		}
		repo, err := repos.Get(actor.UserID, in.RepositoryID)
		if err != nil || repo.OrganizationID != "" {
			writeAPIError(w, 404, "repository_not_found", "an individually owned repository is required")
			return
		}
		v, err := orgs.RequestTransfer(r.PathValue("id"), repo.ID, actor.UserID)
		if writeOrganizationError(w, err) {
			return
		}
		var transfer organizations.Transfer
		for _, candidate := range v.Transfers {
			if candidate.RepositoryID == repo.ID && candidate.FromOwnerID == actor.UserID && candidate.Status == "pending" {
				transfer = candidate
			}
		}
		writeJSON(w, 202, transfer)
	})
	mux.HandleFunc("POST /organizations/{id}/repository-transfers/{transfer_id}/accept", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		v, err := orgs.AcceptTransfer(r.PathValue("id"), r.PathValue("transfer_id"), actor.UserID, func(t organizations.Transfer, v organizations.Organization) error {
			members := []string{}
			for _, m := range v.Members {
				members = append(members, m.UserID)
			}
			_, e := repos.SetOrganization(t.FromOwnerID, t.RepositoryID, v.ID, members)
			return e
		})
		if writeOrganizationError(w, err) {
			return
		}
		writeJSON(w, 200, project(v, actor.UserID))
	})
	mux.HandleFunc("GET /organizations/{id}/portfolio", func(w http.ResponseWriter, r *http.Request) {
		actor, v, ok := require(w, r, "repositories:read")
		if !ok {
			return
		}
		if !organizations.HasRole(v, actor.UserID, "") {
			writeAPIError(w, 403, "forbidden", "accept the organization invitation to inspect its portfolio")
			return
		}
		repoItems, err := repos.ListOrganization(v.ID)
		if writeRepositoryError(w, err) {
			return
		}
		proposalItems := []proposals.Proposal{}
		pullItems := []pullrequests.PullRequest{}
		releaseItems := []releases.Candidate{}
		packageItems := []packages.Version{}
		ids := map[string]bool{}
		for _, repo := range repoItems {
			ids[repo.ID] = true
			if proposalStore != nil {
				xs, readErr := proposalStore.List(repo.ID)
				if readErr != nil {
					writeAPIError(w, 500, "portfolio_unavailable", "proposal portfolio unavailable")
					return
				}
				for _, x := range xs {
					if x.Status == proposals.Open {
						proposalItems = append(proposalItems, x)
					}
				}
			}
			if pullStore != nil {
				xs, readErr := pullStore.List(repo.ID)
				if readErr != nil {
					writeAPIError(w, 500, "portfolio_unavailable", "pull request portfolio unavailable")
					return
				}
				for _, x := range xs {
					if x.Status == pullrequests.Open {
						pullItems = append(pullItems, x)
					}
				}
			}
			if releaseStore != nil {
				xs, readErr := releaseStore.List(repo.ID)
				if readErr != nil {
					writeAPIError(w, 500, "portfolio_unavailable", "release portfolio unavailable")
					return
				}
				releaseItems = append(releaseItems, xs...)
			}
			if packageStore != nil {
				xs, readErr := packageStore.ListRepository(repo.ID)
				if readErr != nil {
					writeAPIError(w, 500, "portfolio_unavailable", "package portfolio unavailable")
					return
				}
				packageItems = append(packageItems, xs...)
			}
		}
		incidentItems := []incidents.Incident{}
		if incidentStore != nil {
			xs, readErr := incidentStore.List()
			if readErr != nil {
				writeAPIError(w, 500, "portfolio_unavailable", "incident portfolio unavailable")
				return
			}
			for _, x := range xs {
				for _, scope := range x.Scopes {
					if ids[scope.RepositoryID] && x.Status != "resolved" {
						incidentItems = append(incidentItems, x)
						break
					}
				}
			}
		}
		writeJSON(w, 200, map[string]any{"organization": project(v, actor.UserID), "repositories": repoItems, "packages": packageItems, "active_proposals": proposalItems, "active_pulls": pullItems, "releases": releaseItems, "active_incidents": incidentItems, "initiatives": projectInitiatives(v, actor.UserID, repoItems, releaseItems, repos, proposalStore, relationshipStore, incidentStore, securityStore)})
	})
	mux.HandleFunc("POST /organizations/{id}/initiatives", func(w http.ResponseWriter, r *http.Request) {
		actor, group, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var in organizationInitiativeInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_initiative", "initiative content is required")
			return
		}
		portfolio, err := repos.ListOrganization(group.ID)
		if writeRepositoryError(w, err) {
			return
		}
		_, created, err := orgs.CreateInitiative(group.ID, actor.UserID, in.Title, in.Description, in.Source, in.WorkItems, func(current organizations.Organization, source organizations.InitiativeSource, items []organizations.InitiativeWorkItem) error {
			validate := initiativeValidator(current, portfolio)
			if source.RepositoryID != "" && !repositoryInOrganization(portfolio, source.RepositoryID) {
				return organizations.ErrInvalid
			}
			if !initiativeSourceExists(source, actor.UserID, portfolio, repos, proposalStore, relationshipStore, incidentStore, securityStore) {
				return organizations.ErrInvalid
			}
			for _, item := range items {
				if err := validate(item); err != nil {
					return err
				}
				if item.Contribution != nil && !initiativeSourceExists(*item.Contribution, actor.UserID, portfolio, repos, proposalStore, relationshipStore, incidentStore, securityStore) {
					return organizations.ErrInvalid
				}
			}
			return nil
		})
		if writeOrganizationError(w, err) {
			return
		}
		w.Header().Set("Location", "/organizations/"+group.ID+"/initiatives/"+created.ID)
		writeJSON(w, 201, created)
	})
	mux.HandleFunc("PATCH /organizations/{id}/initiatives/{initiative_id}/items/{item_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, group, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var in organizationInitiativeItemInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_initiative", "owner, status, and expected_version are required")
			return
		}
		portfolio, err := repos.ListOrganization(group.ID)
		if writeRepositoryError(w, err) {
			return
		}
		v, err := orgs.UpdateInitiativeItem(group.ID, r.PathValue("initiative_id"), r.PathValue("item_id"), actor.UserID, in.Owner, in.Status, in.ExpectedVersion, func(current organizations.Organization, item organizations.InitiativeWorkItem) error {
			return initiativeValidator(current, portfolio)(item)
		})
		if writeOrganizationError(w, err) {
			return
		}
		writeJSON(w, 200, project(v, actor.UserID))
	})
}

func repositoryInOrganization(items []repositories.Repository, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}

func initiativeSourceExists(source organizations.InitiativeSource, actor string, portfolio []repositories.Repository, repositoryStore *repositories.Store, proposalStore *proposals.Store, relationshipStore *relationships.Store, incidentStore *incidents.Store, securityStore *securityadvisories.Store) bool {
	repositoryAuthorized := func(repositoryID string) bool {
		if repositoryStore == nil || repositoryID == "" || !repositoryInOrganization(portfolio, repositoryID) {
			return false
		}
		repository, err := repositoryStore.GetByID(repositoryID)
		if err == nil && repository.OwnerID == actor {
			return true
		}
		collaborator, err := repositoryStore.HasCollaborator(actor, repositoryID)
		return err == nil && collaborator
	}
	switch source.Kind {
	case "proposal":
		if proposalStore == nil || !repositoryAuthorized(source.RepositoryID) {
			return false
		}
		_, err := proposalStore.Get(source.RepositoryID, source.ID)
		return err == nil
	case "evolution":
		if relationshipStore == nil || !repositoryAuthorized(source.RepositoryID) {
			return false
		}
		_, err := relationshipStore.GetEvolution(source.RepositoryID, source.ID)
		return err == nil
	case "incident":
		if incidentStore == nil || !repositoryAuthorized(source.RepositoryID) {
			return false
		}
		incident, err := incidentStore.Get(source.ID)
		if err != nil {
			return false
		}
		inScope := false
		for _, scope := range incident.Scopes {
			if scope.RepositoryID == source.RepositoryID {
				inScope = true
				break
			}
		}
		if !inScope {
			return false
		}
		return true
	case "security":
		if securityStore == nil {
			return false
		}
		advisory, err := securityStore.Get(source.ID)
		if err != nil {
			return false
		}
		authorized := advisory.ReporterID == actor
		for _, responder := range advisory.ResponseTeam {
			authorized = authorized || responder == actor
		}
		matched := false
		for _, affected := range advisory.AffectedRepositories {
			if source.RepositoryID != "" && affected.RepositoryID != source.RepositoryID {
				continue
			}
			matched = true
			for _, repository := range portfolio {
				if repository.ID == affected.RepositoryID && repository.OwnerID == actor {
					authorized = true
				}
			}
		}
		return matched && authorized
	}
	return false
}

func initiativeValidator(group organizations.Organization, portfolio []repositories.Repository) func(organizations.InitiativeWorkItem) error {
	return func(item organizations.InitiativeWorkItem) error {
		if !repositoryInOrganization(portfolio, item.RepositoryID) {
			return organizations.ErrInvalid
		}
		switch item.Owner.Type {
		case "human":
			if !organizations.HasRole(group, item.Owner.ID, "") {
				return organizations.ErrInvalid
			}
			return nil
		case "team":
			for _, team := range group.Teams {
				if team.ID == item.Owner.ID {
					return nil
				}
			}
			return organizations.ErrInvalid
		case "agent":
			for _, agent := range group.Agents {
				if agent.ID == item.Owner.ID {
					for _, operator := range agent.OperatorIDs {
						if organizations.HasRole(group, operator, "") {
							return nil
						}
					}
				}
			}
			return organizations.ErrInvalid
		}
		return organizations.ErrInvalid
	}
}

type initiativeItemProjection struct {
	organizations.InitiativeWorkItem
	Blocked          bool     `json:"blocked"`
	BlockerIDs       []string `json:"blocker_ids"`
	OwnershipState   string   `json:"ownership_state"`
	ReassignmentNote string   `json:"reassignment_note,omitempty"`
}
type initiativeProjection struct {
	organizations.Initiative
	Items            []initiativeItemProjection      `json:"work_items"`
	PolicyExceptions []organizations.PolicyException `json:"policy_exceptions"`
	UpcomingReleases []releases.Candidate            `json:"upcoming_releases"`
}

func projectInitiatives(group organizations.Organization, actor string, portfolio []repositories.Repository, releaseItems []releases.Candidate, repositoryStore *repositories.Store, proposalStore *proposals.Store, relationshipStore *relationships.Store, incidentStore *incidents.Store, securityStore *securityadvisories.Store) []initiativeProjection {
	out := []initiativeProjection{}
	validOwner := initiativeValidator(group, portfolio)
	for _, initiative := range group.Initiatives {
		if !initiativeSourceExists(initiative.Source, actor, portfolio, repositoryStore, proposalStore, relationshipStore, incidentStore, securityStore) {
			continue
		}
		readable := true
		for _, item := range initiative.WorkItems {
			if item.Contribution != nil && !initiativeSourceExists(*item.Contribution, actor, portfolio, repositoryStore, proposalStore, relationshipStore, incidentStore, securityStore) {
				readable = false
				break
			}
		}
		if !readable {
			continue
		}
		projection := initiativeProjection{Initiative: initiative, Items: []initiativeItemProjection{}, PolicyExceptions: []organizations.PolicyException{}, UpcomingReleases: []releases.Candidate{}}
		projection.Initiative.WorkItems = nil
		states := map[string]string{}
		for _, item := range initiative.WorkItems {
			states[item.ID] = item.Status
		}
		repositoriesUsed := map[string]bool{}
		for _, item := range initiative.WorkItems {
			repositoriesUsed[item.RepositoryID] = true
			view := initiativeItemProjection{InitiativeWorkItem: item, BlockerIDs: []string{}, OwnershipState: "accountable"}
			for _, dependencyID := range item.DependencyIDs {
				if states[dependencyID] != "completed" {
					view.BlockerIDs = append(view.BlockerIDs, dependencyID)
				}
			}
			view.Blocked = len(view.BlockerIDs) > 0
			if validOwner(item) != nil {
				view.OwnershipState = "reassignment_required"
				view.ReassignmentNote = "The accountable principal or repository is no longer in the organization portfolio. Assign a current team, member, or approved agent."
			}
			projection.Items = append(projection.Items, view)
		}
		for _, exception := range group.PolicyExceptions {
			if repositoriesUsed[exception.RepositoryID] && (exception.Status == "pending" || exception.Status == "approved") {
				projection.PolicyExceptions = append(projection.PolicyExceptions, exception)
			}
		}
		for _, release := range releaseItems {
			if repositoriesUsed[release.RepositoryID] {
				projection.UpcomingReleases = append(projection.UpcomingReleases, release)
			}
		}
		out = append(out, projection)
	}
	return out
}

func hasInvite(v organizations.Organization, user string) bool {
	for _, x := range v.Invitations {
		if x.UserID == user {
			return true
		}
	}
	return false
}
func writeOrganizationError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, organizations.ErrNotFound):
		writeAPIError(w, 404, "organization_not_found", "organization not found")
	case errors.Is(err, organizations.ErrInvalid):
		writeAPIError(w, 400, "invalid_organization", "organization content is invalid")
	case errors.Is(err, organizations.ErrConflict):
		writeAPIError(w, 409, "organization_conflict", "organization state conflicts with this request")
	default:
		writeAPIError(w, 500, "organization_unavailable", "organization storage unavailable")
	}
	return true
}
