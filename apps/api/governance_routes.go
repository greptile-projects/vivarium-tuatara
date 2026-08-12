package main

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/charters"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/governance"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

func registerGovernanceRoutes(mux *http.ServeMux, votes *governance.Store, charterStore *charters.Store, repos *repositories.Store, orgs *organizations.Store, credentials *auth.Store) {
	electorate := func(kind, scopeID string, revision charters.Revision) []governance.Elector {
		sources := map[string]map[string]bool{}
		add := func(source, id string) {
			if sources[source] == nil {
				sources[source] = map[string]bool{}
			}
			sources[source][id] = true
		}
		if kind == "repository" {
			if repo, err := repos.GetByID(scopeID); err == nil {
				add("repository_owner", repo.OwnerID)
				if cs, err := repos.ListCollaborators(repo.OwnerID, scopeID); err == nil {
					for _, c := range cs {
						add("repository_collaborator", c.UserID)
					}
				}
			}
		} else if orgs != nil {
			if org, err := orgs.Get(scopeID); err == nil {
				add("organization_owner", org.CreatedBy)
				for _, m := range org.Members {
					add("organization_member", m.UserID)
				}
				for _, t := range org.Teams {
					for _, m := range t.Members {
						if m.Role == "maintainer" {
							add("team_maintainer", m.UserID)
						}
					}
				}
			}
		}
		roles := map[string][]string{}
		for _, role := range revision.Roles {
			for _, source := range role.Eligibility {
				for user := range sources[source] {
					roles[user] = append(roles[user], role.Name)
				}
			}
		}
		record, err := charterStore.Get(kind, scopeID)
		if err != nil {
			return []governance.Elector{}
		}
		roles = activeGovernanceStandingRoles(record, revision.Version, roles, time.Now())
		out := make([]governance.Elector, 0, len(roles))
		for user, rs := range roles {
			out = append(out, governance.Elector{UserID: user, Roles: rs, Eligible: true})
		}
		return out
	}
	authorizeScope := func(w http.ResponseWriter, r *http.Request, kind, id string, write bool) (auth.Credential, bool) {
		if kind == "repository" {
			if write {
				a, _, ok := authorizeRepositoryParticipant(w, r, repos, credentials, id, "repositories:write")
				return a, ok
			}
			a, _, ok := authorizeRepositoryRead(w, r, repos, credentials, id)
			return a, ok
		}
		a, ok := authenticateRequest(w, r, credentials, map[bool]string{true: "repositories:write", false: "repositories:read"}[write], false)
		if !ok {
			return a, false
		}
		if orgs == nil {
			return a, false
		}
		org, e := orgs.Get(id)
		if e != nil {
			return a, false
		}
		for _, m := range org.Members {
			if m.UserID == a.UserID {
				return a, true
			}
		}
		if org.CreatedBy == a.UserID {
			return a, true
		}
		writeAPIError(w, 404, "governed_proposal_not_found", "governed proposal not found")
		return a, false
	}
	project := governedProposalProjection
	mux.HandleFunc("POST /governance/proposals", func(w http.ResponseWriter, r *http.Request) {
		var in governance.Proposal
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_governed_proposal", "proposal content is required")
			return
		}
		actor, ok := authorizeScope(w, r, in.ScopeType, in.ScopeID, true)
		if !ok {
			return
		}
		record, e := charterStore.Get(in.ScopeType, in.ScopeID)
		if e != nil || record.ActiveVersion < 1 {
			writeAPIError(w, 422, "active_charter_required", "an active charter is required")
			return
		}
		revision := record.Revisions[record.ActiveVersion-1]
		var rule *charters.DecisionClass
		for i := range revision.DecisionClasses {
			if revision.DecisionClasses[i].Name == in.Rule.DecisionClass {
				rule = &revision.DecisionClasses[i]
			}
		}
		if rule == nil {
			writeAPIError(w, 422, "decision_class_required", "decision class is not active")
			return
		}
		live := electorate(in.ScopeType, in.ScopeID, revision)
		eligibleRoles := map[string]bool{}
		for _, x := range rule.EligibleRoles {
			eligibleRoles[x] = true
		}
		filtered := []governance.Elector{}
		for _, e := range live {
			for _, role := range e.Roles {
				if eligibleRoles[role] {
					filtered = append(filtered, e)
					break
				}
			}
		}
		allowed := false
		for _, e := range filtered {
			if e.UserID == actor.UserID {
				allowed = true
			}
		}
		if !allowed {
			writeAPIError(w, 403, "governance_standing_required", "current decision eligibility is required")
			return
		}
		in.CreatedBy = actor.UserID
		in.CharterVersion = record.ActiveVersion
		in.Electorate = filtered
		in.Rule.EligibleRoles = rule.EligibleRoles
		in.Rule.Quorum = rule.Quorum
		in.Rule.Threshold = rule.Approval
		p, e := votes.Create(in)
		if e != nil {
			governanceError(w, e)
			return
		}
		writeJSON(w, 201, project(p, actor.UserID))
	})
	mux.HandleFunc("GET /governance/proposals", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		items, e := votes.List()
		if e != nil {
			governanceError(w, e)
			return
		}
		out := []governance.Proposal{}
		for _, p := range items {
			if _, yes := authorizeScopeSilent(actor.UserID, p.ScopeType, p.ScopeID, repos, orgs); yes {
				out = append(out, project(p, actor.UserID))
			}
		}
		writeJSON(w, 200, map[string]any{"proposals": out})
	})
	mux.HandleFunc("GET /governance/proposals/{id}", func(w http.ResponseWriter, r *http.Request) {
		p, e := votes.Get(r.PathValue("id"))
		if e != nil {
			governanceError(w, e)
			return
		}
		actor, ok := authorizeScope(w, r, p.ScopeType, p.ScopeID, false)
		if !ok {
			return
		}
		writeJSON(w, 200, project(p, actor.UserID))
	})
	mux.HandleFunc("POST /governance/proposals/{id}/analysis", func(w http.ResponseWriter, r *http.Request) {
		p, e := votes.Get(r.PathValue("id"))
		if e != nil {
			governanceError(w, e)
			return
		}
		actor, ok := authorizeScope(w, r, p.ScopeType, p.ScopeID, true)
		if !ok {
			return
		}
		var in struct {
			ActorType string                 `json:"actor_type"`
			AgentID   string                 `json:"agent_id"`
			Body      string                 `json:"body"`
			Position  string                 `json:"position"`
			Citations []governance.Reference `json:"citations"`
		}
		if decodeJSON(r, &in) != nil {
			governanceError(w, governance.ErrInvalid)
			return
		}
		actorID := actor.UserID
		if in.ActorType == "agent" {
			actorID = "agent:" + strings.TrimPrefix(in.AgentID, "agent:")
			if orgs == nil || p.ScopeType != "organization" {
				governanceError(w, governance.ErrIneligible)
				return
			}
			org, _ := orgs.Get(p.ScopeID)
			operates := false
			for _, a := range org.Agents {
				if "agent:"+a.ID == actorID {
					for _, op := range a.OperatorIDs {
						if op == actor.UserID {
							operates = true
						}
					}
				}
			}
			if !operates {
				governanceError(w, governance.ErrIneligible)
				return
			}
		} else {
			in.ActorType = "human"
		}
		p, e = votes.Analyze(p.ID, in.ActorType, actorID, in.Body, in.Position, in.Citations)
		if e != nil {
			governanceError(w, e)
			return
		}
		writeJSON(w, 201, project(p, actor.UserID))
	})
	mux.HandleFunc("POST /governance/proposals/{id}/ballots", func(w http.ResponseWriter, r *http.Request) {
		p, e := votes.Get(r.PathValue("id"))
		if e != nil {
			governanceError(w, e)
			return
		}
		actor, ok := authorizeScope(w, r, p.ScopeType, p.ScopeID, true)
		if !ok {
			return
		}
		record, e := charterStore.Get(p.ScopeType, p.ScopeID)
		if e != nil || record.ActiveVersion != p.CharterVersion {
			governanceError(w, governance.ErrIneligible)
			return
		}
		eligible := false
		for _, x := range electorate(p.ScopeType, p.ScopeID, record.Revisions[p.CharterVersion-1]) {
			if x.UserID == actor.UserID {
				for _, role := range x.Roles {
					for _, allowed := range p.Rule.EligibleRoles {
						if role == allowed {
							eligible = true
						}
					}
				}
			}
		}
		var in struct {
			Choice string `json:"choice"`
			Reason string `json:"reason"`
		}
		if decodeJSON(r, &in) != nil {
			governanceError(w, governance.ErrInvalid)
			return
		}
		p, e = votes.Cast(p.ID, actor.UserID, in.Choice, in.Reason, eligible, "eligible under active charter revision")
		if e != nil {
			governanceError(w, e)
			return
		}
		writeJSON(w, 201, project(p, actor.UserID))
	})
	mux.HandleFunc("POST /governance/proposals/{id}/tally", func(w http.ResponseWriter, r *http.Request) {
		p, e := votes.Get(r.PathValue("id"))
		if e != nil {
			governanceError(w, e)
			return
		}
		actor, ok := authorizeScope(w, r, p.ScopeType, p.ScopeID, true)
		if !ok {
			return
		}
		record, e := charterStore.Get(p.ScopeType, p.ScopeID)
		current := []governance.Elector{}
		contest := []string{}
		if e != nil || record.ActiveVersion != p.CharterVersion {
			contest = append(contest, "the active charter changed before tally")
		} else {
			allowed := map[string]bool{}
			for _, role := range p.Rule.EligibleRoles {
				allowed[role] = true
			}
			for _, candidate := range electorate(p.ScopeType, p.ScopeID, record.Revisions[p.CharterVersion-1]) {
				for _, role := range candidate.Roles {
					if allowed[role] {
						current = append(current, candidate)
						break
					}
				}
			}
		}
		var in struct {
			ContestReasons []string `json:"contest_reasons"`
		}
		_ = decodeJSON(r, &in)
		contest = append(contest, in.ContestReasons...)
		p, e = votes.Finalize(p.ID, actor.UserID, current, contest)
		if e != nil {
			governanceError(w, e)
			return
		}
		writeJSON(w, 200, project(p, actor.UserID))
	})
}

func authorizeScopeSilent(user, kind, id string, repos *repositories.Store, orgs *organizations.Store) (string, bool) {
	if kind == "repository" {
		repo, e := repos.GetByID(id)
		if e != nil {
			return "", false
		}
		if repo.Visibility == "public" || repo.OwnerID == user {
			return id, true
		}
		cs, _ := repos.ListCollaborators(repo.OwnerID, id)
		for _, c := range cs {
			if c.UserID == user {
				return id, true
			}
		}
		return "", false
	}
	if orgs != nil {
		org, e := orgs.Get(id)
		if e == nil {
			for _, m := range org.Members {
				if m.UserID == user {
					return id, true
				}
			}
		}
	}
	return "", false
}

func governedProposalProjection(p governance.Proposal, actor string) governance.Proposal {
	if !p.Rule.SecretBallot {
		return p
	}
	ballots := make([]governance.Ballot, 0, 1)
	for _, ballot := range p.Ballots {
		if ballot.ActorID == actor {
			ballots = append(ballots, ballot)
		}
	}
	p.Ballots = ballots
	events := p.Events[:0]
	for _, event := range p.Events {
		if event.Kind != "ballot.cast" || event.ActorID == actor {
			events = append(events, event)
		}
	}
	p.Events = events
	return p
}

func activeGovernanceStandingRoles(record charters.Record, charterVersion int, roles map[string][]string, now time.Time) map[string][]string {
	eligible := map[string][]string{}
	for user, userRoles := range roles {
		for _, role := range userRoles {
			for _, standing := range record.Standings {
				if standing.PrincipalType == "human" && standing.PrincipalID == user && standing.Role == role && standing.CharterVersion == charterVersion && standing.Status == "active" && standing.ExpiresAt.After(now) {
					eligible[user] = append(eligible[user], role)
					break
				}
			}
		}
	}
	return eligible
}

func governanceError(w http.ResponseWriter, e error) {
	switch {
	case errors.Is(e, governance.ErrNotFound):
		writeAPIError(w, 404, "governed_proposal_not_found", "governed proposal not found")
	case errors.Is(e, governance.ErrConflict):
		writeAPIError(w, 409, "governance_conflict", "the governance record changed")
	case errors.Is(e, governance.ErrDuplicateBallot):
		writeAPIError(w, 409, "duplicate_ballot", "this participant already cast a ballot")
	case errors.Is(e, governance.ErrFinalized):
		writeAPIError(w, 409, "tally_finalized", "the authoritative tally is already finalized")
	case errors.Is(e, governance.ErrClosed):
		writeAPIError(w, 409, "voting_closed", "the voting deadline does not admit this action")
	case errors.Is(e, governance.ErrIneligible):
		writeAPIError(w, 403, "electorate_changed", "current charter eligibility is required")
	case errors.Is(e, governance.ErrInvalid):
		writeAPIError(w, 422, "invalid_governed_proposal", "governed proposal content is invalid")
	default:
		writeAPIError(w, 500, "governance_storage_failed", "governance evidence could not be retained")
	}
}
