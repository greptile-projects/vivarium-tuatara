package main

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/activities"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/decisions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deliveryteams"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/incidents"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
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

func registerDeliveryTeamRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, identities *users.Store, store *deliveryteams.Store, proposalStore *proposals.Store, decisionStore *decisions.Store, incidentStore *incidents.Store, orgs *organizations.Store, activity *activities.Store) {
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
				}
			}
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
