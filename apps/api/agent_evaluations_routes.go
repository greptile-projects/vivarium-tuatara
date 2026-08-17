package main

import (
	"errors"
	"net/http"
	"os/exec"
	"slices"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/agentevaluations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

type evaluationSuiteInput struct {
	Name            string                    `json:"name"`
	RepositoryID    string                    `json:"repository_id"`
	ExpectedVersion int                       `json:"expected_version"`
	Revision        agentevaluations.Revision `json:"revision"`
}
type evaluationRunInput struct {
	SuiteVersion int                       `json:"suite_version"`
	Trial        agentevaluations.RunInput `json:"trial"`
}
type evaluationDecisionInput struct {
	Decision  string `json:"decision"`
	Rationale string `json:"rationale"`
}
type participationCreateInput struct {
	ExpectedVersion int                                 `json:"expected_version"`
	Participation   agentevaluations.ParticipationInput `json:"participation"`
}
type participationMutationInput struct {
	ExpectedVersion int    `json:"expected_version"`
	Statement       string `json:"statement"`
	Decision        string `json:"decision"`
	SponsorID       string `json:"sponsor_id"`
}

func registerAgentEvaluationRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, orgs *organizations.Store, evaluations *agentevaluations.Store) {
	require := func(w http.ResponseWriter, r *http.Request, scope string) (auth.Credential, organizations.Organization, bool) {
		actor, ok := authenticateRequest(w, r, credentials, scope, false)
		if !ok {
			return actor, organizations.Organization{}, false
		}
		org, err := orgs.Get(r.PathValue("id"))
		if err != nil || !organizations.HasRole(org, actor.UserID, "") {
			writeAPIError(w, 404, "organization_not_found", "organization not found")
			return actor, org, false
		}
		return actor, org, true
	}
	belongs := func(orgID, repositoryID string) bool {
		items, err := catalog.ListOrganization(orgID)
		if err != nil {
			return false
		}
		for _, x := range items {
			if x.ID == repositoryID {
				return true
			}
		}
		return false
	}
	writeErr := func(w http.ResponseWriter, err error) {
		switch {
		case errors.Is(err, agentevaluations.ErrNotFound):
			writeAPIError(w, 404, "evaluation_not_found", "evaluation not found")
		case errors.Is(err, agentevaluations.ErrConflict):
			writeAPIError(w, 409, "evaluation_conflict", "evaluation suite changed; reload its current version")
		case errors.Is(err, agentevaluations.ErrInvalid):
			writeAPIError(w, 422, "evaluation_invalid", "evaluation evidence is incomplete, unsafe, or outside its bounds")
		default:
			writeAPIError(w, 500, "evaluation_unavailable", "evaluation evidence is unavailable")
		}
	}
	mux.HandleFunc("GET /organizations/{id}/agent-evaluation-suites", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := require(w, r, "repositories:read"); !ok {
			return
		}
		v, e := evaluations.PublicList(r.PathValue("id"))
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, 200, map[string]any{"suites": v})
	})
	mux.HandleFunc("POST /organizations/{id}/agent-evaluation-suites", func(w http.ResponseWriter, r *http.Request) {
		actor, org, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var in evaluationSuiteInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete evaluation suite is required")
			return
		}
		if !belongs(org.ID, in.RepositoryID) {
			writeAPIError(w, 422, "evaluation_repository_invalid", "suite repository must belong to this organization")
			return
		}
		repo, e := git.Open(in.RepositoryID)
		if e != nil || exec.Command("git", "--git-dir="+repo.Path(), "cat-file", "-e", in.Revision.RepositoryRevision+"^{commit}").Run() != nil {
			writeAPIError(w, 422, "evaluation_revision_invalid", "the exact repository revision is unavailable")
			return
		}
		in.Revision.CreatedBy = actor.UserID
		var v agentevaluations.Suite
		if in.ExpectedVersion == 0 {
			e = catalog.WithCurrentParticipant(actor.UserID, in.RepositoryID, func() error {
				var x error
				v, x = evaluations.Create(agentevaluations.Suite{OrganizationID: org.ID, RepositoryID: in.RepositoryID, Name: in.Name}, in.Revision)
				return x
			})
		} else {
			writeAPIError(w, 422, "evaluation_invalid", "create requires expected_version zero")
			return
		}
		if e != nil {
			writeErr(w, e)
			return
		}
		view, _ := evaluations.PublicGet(v.ID)
		writeJSON(w, 201, view)
	})
	mux.HandleFunc("PUT /organizations/{id}/agent-evaluation-suites/{suite_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, org, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var in evaluationSuiteInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete successor suite is required")
			return
		}
		current, e := evaluations.Get(r.PathValue("suite_id"))
		if e != nil || current.OrganizationID != org.ID {
			writeErr(w, agentevaluations.ErrNotFound)
			return
		}
		in.Revision.CreatedBy = actor.UserID
		repo, e := git.Open(current.RepositoryID)
		if e != nil || exec.Command("git", "--git-dir="+repo.Path(), "cat-file", "-e", in.Revision.RepositoryRevision+"^{commit}").Run() != nil {
			writeAPIError(w, 422, "evaluation_revision_invalid", "the exact repository revision is unavailable")
			return
		}
		var v agentevaluations.Suite
		e = catalog.WithCurrentParticipant(actor.UserID, current.RepositoryID, func() error {
			var x error
			v, x = evaluations.Revise(current.ID, in.ExpectedVersion, in.Revision)
			return x
		})
		if e != nil {
			writeErr(w, e)
			return
		}
		view, _ := evaluations.PublicGet(v.ID)
		writeJSON(w, 200, view)
	})
	mux.HandleFunc("GET /organizations/{id}/agent-evaluation-runs", func(w http.ResponseWriter, r *http.Request) {
		actor, org, ok := require(w, r, "repositories:read")
		if !ok {
			return
		}
		var v []agentevaluations.Run
		var e error
		if organizations.HasRole(org, actor.UserID, "owner") {
			v, e = evaluations.ListEvaluatorRuns(org.ID)
		} else {
			v, e = evaluations.ListRuns(org.ID)
		}
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, 200, map[string]any{"runs": v})
	})
	mux.HandleFunc("POST /organizations/{id}/agent-evaluation-suites/{suite_id}/runs", func(w http.ResponseWriter, r *http.Request) {
		actor, org, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		var in evaluationRunInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "bounded trial evidence is required")
			return
		}
		suite, e := evaluations.Get(r.PathValue("suite_id"))
		if e != nil || suite.OrganizationID != org.ID {
			writeErr(w, agentevaluations.ErrNotFound)
			return
		}
		validAgent := false
		for _, a := range org.Agents {
			if a.ID == in.Trial.AgentID && in.Trial.AgentProfileVersion > 0 && in.Trial.AgentProfileVersion <= len(a.Profiles) {
				validAgent = true
			}
		}
		if !validAgent {
			writeAPIError(w, 422, "evaluation_agent_invalid", "candidate and exact published profile version are required")
			return
		}
		var run agentevaluations.Run
		e = catalog.WithCurrentParticipant(actor.UserID, suite.RepositoryID, func() error {
			var x error
			run, x = evaluations.CreateRun(suite.ID, in.SuiteVersion, in.Trial, actor.UserID)
			return x
		})
		if e != nil {
			writeErr(w, e)
			return
		}
		run, e = evaluations.GetRun(run.ID)
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, 201, run)
	})
	mux.HandleFunc("POST /organizations/{id}/agent-evaluation-runs/{run_id}/decisions", func(w http.ResponseWriter, r *http.Request) {
		actor, org, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		if !organizations.HasRole(org, actor.UserID, "owner") {
			writeAPIError(w, 403, "evaluation_owner_required", "only organization owners can decide protected evaluation evidence")
			return
		}
		run, e := evaluations.GetEvaluatorRun(r.PathValue("run_id"))
		if e != nil || run.OrganizationID != org.ID {
			writeErr(w, agentevaluations.ErrNotFound)
			return
		}
		var in evaluationDecisionInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a decision and rationale are required")
			return
		}
		var v agentevaluations.Run
		e = catalog.WithCurrentParticipant(actor.UserID, run.RepositoryID, func() error {
			var x error
			v, x = evaluations.Decide(run.ID, actor.UserID, in.Decision, in.Rationale)
			return x
		})
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, 201, v)
	})
	mux.HandleFunc("GET /organizations/{id}/agent-participations", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := require(w, r, "repositories:read"); !ok {
			return
		}
		v, e := evaluations.ListParticipations(r.PathValue("id"))
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, 200, map[string]any{"participations": v})
	})
	mux.HandleFunc("POST /organizations/{id}/agent-participations", func(w http.ResponseWriter, r *http.Request) {
		actor, org, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		if !organizations.HasRole(org, actor.UserID, "owner") {
			writeAPIError(w, 403, "participation_owner_required", "an organization owner must define agent participation")
			return
		}
		var in participationCreateInput
		if decodeJSON(r, &in) != nil || in.ExpectedVersion != 0 {
			writeAPIError(w, 400, "invalid_participation", "a new bounded participation with expected_version zero is required")
			return
		}
		for _, scope := range in.Participation.Resources {
			if scope.Kind == "repository" && !belongs(org.ID, scope.ID) {
				writeAPIError(w, 422, "participation_resource_invalid", "repository resources must belong to this organization")
				return
			}
		}
		if in.Participation.AgreementRequirement == "sponsor" && !organizations.HasRole(org, in.Participation.SponsorID, "") {
			writeAPIError(w, 422, "participation_sponsor_invalid", "the named sponsor must be a current organization member")
			return
		}
		v, e := evaluations.CreateParticipation(org.ID, actor.UserID, in.Participation)
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, 201, v)
	})
	mux.HandleFunc("GET /organizations/{id}/agent-participations/{participation_id}/preview", func(w http.ResponseWriter, r *http.Request) {
		_, org, ok := require(w, r, "repositories:read")
		if !ok {
			return
		}
		p, e := evaluations.GetParticipation(r.PathValue("participation_id"))
		if e != nil || p.OrganizationID != org.ID {
			writeErr(w, agentevaluations.ErrNotFound)
			return
		}
		blockers := []string{}
		if p.Status == "pending_agreement" {
			blockers = append(blockers, p.AgreementRequirement+" agreement required")
		}
		if time.Now().UTC().Before(p.StartsAt) {
			blockers = append(blockers, "schedule has not started")
		}
		if !p.ExpiresAt.After(time.Now().UTC()) {
			blockers = append(blockers, "approval expired")
		}
		for _, scope := range p.Resources {
			if scope.Kind == "repository" && organizations.EffectivePolicies(org, scope.ID, organizations.ResponsibleTeamIDs(org, scope.ID), false, time.Now().UTC()).Rules.AgentAuthority == "disabled" {
				blockers = append(blockers, "policy disables agent authority for repository "+scope.ID)
			}
		}
		writeJSON(w, 200, map[string]any{"participation": p, "would_issue_identity": "agent-participation:" + p.ID, "would_create_access_grant": true, "effective": false, "blockers": blockers, "actions": p.Actions, "budget": p.Budget, "data_boundaries": p.DataBoundaries, "policy_exception_ids": p.PolicyExceptionIDs})
	})
	mux.HandleFunc("POST /organizations/{id}/agent-participations/{participation_id}/agreement", func(w http.ResponseWriter, r *http.Request) {
		actor, org, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		p, e := evaluations.GetParticipation(r.PathValue("participation_id"))
		if e != nil || p.OrganizationID != org.ID {
			writeErr(w, agentevaluations.ErrNotFound)
			return
		}
		var in participationMutationInput
		if decodeJSON(r, &in) != nil || in.ExpectedVersion != p.Version {
			writeErr(w, agentevaluations.ErrConflict)
			return
		}
		allowed := p.AgreementRequirement == "sponsor" && actor.UserID == p.SponsorID
		if p.AgreementRequirement == "operator" {
			for _, a := range org.Agents {
				if a.ID == p.AgentID && slices.Contains(a.OperatorIDs, actor.UserID) {
					allowed = true
				}
			}
		}
		if !allowed {
			writeAPIError(w, 403, "participation_agreement_required", "only the named sponsor or a current agent operator may agree")
			return
		}
		v, e := evaluations.AgreeParticipation(p.ID, actor.UserID, p.AgreementRequirement, in.Statement)
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("POST /organizations/{id}/agent-participations/{participation_id}/sponsor", func(w http.ResponseWriter, r *http.Request) {
		actor, org, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		if !organizations.HasRole(org, actor.UserID, "owner") {
			writeAPIError(w, 403, "participation_owner_required", "an organization owner must replace a participation sponsor")
			return
		}
		var in participationMutationInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 422, "participation_sponsor_invalid", "replacement sponsor must be a current organization member")
			return
		}
		p, e := evaluations.GetParticipation(r.PathValue("participation_id"))
		if e != nil || p.OrganizationID != org.ID {
			writeErr(w, agentevaluations.ErrNotFound)
			return
		}
		var v agentevaluations.Participation
		e = orgs.WithCurrentMember(org.ID, in.SponsorID, func() error {
			var x error
			v, x = evaluations.ReassignSponsor(p.ID, actor.UserID, in.SponsorID, in.ExpectedVersion)
			return x
		})
		if e != nil {
			if errors.Is(e, organizations.ErrNotFound) {
				writeAPIError(w, 422, "participation_sponsor_invalid", "replacement sponsor must be a current organization member")
				return
			}
			writeErr(w, e)
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("POST /organizations/{id}/agent-participations/{participation_id}/activate", func(w http.ResponseWriter, r *http.Request) {
		actor, org, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		if !organizations.HasRole(org, actor.UserID, "owner") {
			writeAPIError(w, 403, "participation_owner_required", "an organization owner must activate agent participation")
			return
		}
		var in participationMutationInput
		if decodeJSON(r, &in) != nil {
			writeErr(w, agentevaluations.ErrInvalid)
			return
		}
		p, e := evaluations.GetParticipation(r.PathValue("participation_id"))
		if e != nil || p.OrganizationID != org.ID {
			writeErr(w, agentevaluations.ErrNotFound)
			return
		}
		if p.Version != in.ExpectedVersion || p.Status != "ready" || time.Now().UTC().Before(p.StartsAt) || !p.ExpiresAt.After(time.Now().UTC()) {
			writeErr(w, agentevaluations.ErrConflict)
			return
		}
		resources := make([]organizations.ResourceScope, 0, len(p.Resources))
		for _, x := range p.Resources {
			resources = append(resources, organizations.ResourceScope{Kind: x.Kind, ID: x.ID})
		}
		expires := p.ExpiresAt
		authority := &organizations.AgentParticipationAuthority{ParticipationID: p.ID, AllowedActions: p.Actions, DataBoundaries: p.DataBoundaries, MaxCost: p.Budget.MaxCost, MaxAgentMinutes: p.Budget.MaxAgentMinutes, MaxActions: p.Budget.MaxActions}
		changed, e := orgs.CreateAccessRequest(org.ID, actor.UserID, "agent", p.AgentID, p.Role, "Evaluated participation "+p.ID, resources, nil, &expires, authority)
		if e != nil {
			writeOrganizationError(w, e)
			return
		}
		req := changed.AccessRequests[len(changed.AccessRequests)-1]
		changed, e = orgs.DecideAccessRequest(org.ID, req.ID, actor.UserID, "approve", func(request organizations.AccessRequest) error {
			for _, x := range request.Resources {
				if x.Kind == "repository" && organizations.EffectivePolicies(org, x.ID, organizations.ResponsibleTeamIDs(org, x.ID), false, time.Now().UTC()).Rules.AgentAuthority == "disabled" {
					return organizations.ErrConflict
				}
			}
			return nil
		})
		if e != nil {
			writeOrganizationError(w, e)
			return
		}
		grant := changed.AccessRequests[len(changed.AccessRequests)-1].GrantID
		v, e := evaluations.ActivateParticipation(p.ID, actor.UserID, "agent-participation:"+p.ID, grant, in.ExpectedVersion)
		if e != nil {
			_, _ = orgs.RevokeAccessGrant(org.ID, grant, actor.UserID, 1, func(c organizations.DerivedCredential) error {
				_, x := credentials.Revoke(c.OperatorID, c.ID)
				if errors.Is(x, auth.ErrNotFound) {
					return nil
				}
				return x
			})
			writeErr(w, e)
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("POST /organizations/{id}/agent-participations/{participation_id}/decision", func(w http.ResponseWriter, r *http.Request) {
		actor, org, ok := require(w, r, "repositories:write")
		if !ok {
			return
		}
		if !organizations.HasRole(org, actor.UserID, "owner") {
			writeAPIError(w, 403, "participation_owner_required", "an organization owner must deny or revoke participation")
			return
		}
		var in participationMutationInput
		if decodeJSON(r, &in) != nil {
			return
		}
		p, e := evaluations.GetParticipation(r.PathValue("participation_id"))
		if e != nil || p.OrganizationID != org.ID {
			writeErr(w, agentevaluations.ErrNotFound)
			return
		}
		if p.Version != in.ExpectedVersion || (in.Decision == "revoke" && p.Status != "active") || (in.Decision == "deny" && p.Status == "active") || (in.Decision != "deny" && in.Decision != "revoke") {
			writeErr(w, agentevaluations.ErrConflict)
			return
		}
		if in.Decision == "revoke" && p.AccessGrantID != "" {
			if _, e = orgs.RevokeAccessGrant(org.ID, p.AccessGrantID, actor.UserID, 1, func(c organizations.DerivedCredential) error {
				_, x := credentials.Revoke(c.OperatorID, c.ID)
				if errors.Is(x, auth.ErrNotFound) {
					return nil
				}
				return x
			}); e != nil {
				writeOrganizationError(w, e)
				return
			}
		}
		v, e := evaluations.DecideParticipation(p.ID, actor.UserID, in.Decision, in.ExpectedVersion)
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, 200, v)
	})
}
