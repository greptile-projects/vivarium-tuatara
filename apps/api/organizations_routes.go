package main

import (
	"errors"
	"net/http"
	"strings"
	"time"

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

func registerOrganizationRoutes(mux *http.ServeMux, orgs *organizations.Store, repos *repositories.Store, usersStore *users.Store, credentials *auth.Store, proposalStore *proposals.Store, pullStore *pullrequests.Store, releaseStore *releases.Store, packageStore *packages.Store, incidentStore *incidents.Store, relationshipStore *relationships.Store, securityStore *securityadvisories.Store) {
	project := func(v organizations.Organization, actor string) organizations.Organization {
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
		if grant.Role != "viewer" {
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
		writeJSON(w, 200, map[string]any{"organization": project(v, actor.UserID), "repositories": repoItems, "packages": packageItems, "active_proposals": proposalItems, "active_pulls": pullItems, "releases": releaseItems, "active_incidents": incidentItems, "initiatives": projectInitiatives(v, actor.UserID, repoItems, releaseItems, proposalStore, relationshipStore, incidentStore, securityStore)})
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
			if !initiativeSourceExists(source, actor.UserID, portfolio, proposalStore, relationshipStore, incidentStore, securityStore) {
				return organizations.ErrInvalid
			}
			for _, item := range items {
				if err := validate(item); err != nil {
					return err
				}
				if item.Contribution != nil && !initiativeSourceExists(*item.Contribution, actor.UserID, portfolio, proposalStore, relationshipStore, incidentStore, securityStore) {
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

func initiativeSourceExists(source organizations.InitiativeSource, actor string, portfolio []repositories.Repository, proposalStore *proposals.Store, relationshipStore *relationships.Store, incidentStore *incidents.Store, securityStore *securityadvisories.Store) bool {
	switch source.Kind {
	case "proposal":
		if proposalStore == nil || source.RepositoryID == "" {
			return false
		}
		_, err := proposalStore.Get(source.RepositoryID, source.ID)
		return err == nil
	case "evolution":
		if relationshipStore == nil || source.RepositoryID == "" {
			return false
		}
		_, err := relationshipStore.GetEvolution(source.RepositoryID, source.ID)
		return err == nil
	case "incident":
		if incidentStore == nil {
			return false
		}
		incident, err := incidentStore.Get(source.ID)
		if err != nil {
			return false
		}
		for _, scope := range incident.Scopes {
			if source.RepositoryID == "" || scope.RepositoryID == source.RepositoryID {
				return true
			}
		}
		return false
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
		for _, affected := range advisory.AffectedRepositories {
			if source.RepositoryID != "" && affected.RepositoryID != source.RepositoryID {
				continue
			}
			for _, repository := range portfolio {
				if repository.ID == affected.RepositoryID && repository.OwnerID == actor {
					authorized = true
				}
			}
			return authorized
		}
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

func projectInitiatives(group organizations.Organization, actor string, portfolio []repositories.Repository, releaseItems []releases.Candidate, proposalStore *proposals.Store, relationshipStore *relationships.Store, incidentStore *incidents.Store, securityStore *securityadvisories.Store) []initiativeProjection {
	out := []initiativeProjection{}
	validOwner := initiativeValidator(group, portfolio)
	for _, initiative := range group.Initiatives {
		if !initiativeSourceExists(initiative.Source, actor, portfolio, proposalStore, relationshipStore, incidentStore, securityStore) {
			continue
		}
		readable := true
		for _, item := range initiative.WorkItems {
			if item.Contribution != nil && !initiativeSourceExists(*item.Contribution, actor, portfolio, proposalStore, relationshipStore, incidentStore, securityStore) {
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
