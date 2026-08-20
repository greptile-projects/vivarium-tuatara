package main

import (
	"errors"
	"net/http"
	"slices"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/agentcandidates"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/agentpilots"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/agentreleases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

func registerAgentReleaseUnavailableRoutes(mux *http.ServeMux) {
	unavailable := func(w http.ResponseWriter, _ *http.Request) {
		writeAPIError(w, 503, "agent_release_unavailable", "agent release storage could not be initialized")
	}
	mux.HandleFunc("POST /repositories/{id}/agent-candidates/{candidate_id}/release-approvals", unavailable)
	mux.HandleFunc("POST /repositories/{id}/agent-releases", unavailable)
	mux.HandleFunc("GET /repositories/{id}/agent-releases", unavailable)
	mux.HandleFunc("POST /repositories/{id}/agent-releases/{release_id}/deployments", unavailable)
	mux.HandleFunc("POST /repositories/{id}/agent-deployments/{deployment_id}/actions", unavailable)
}

func registerAgentReleaseRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, orgs *organizations.Store, pulls *pullrequests.Store, candidates *agentcandidates.Store, pilots *agentpilots.Store, releases *agentreleases.Store) {
	writeErr := func(w http.ResponseWriter, e error) {
		switch {
		case errors.Is(e, agentreleases.ErrNotFound):
			writeAPIError(w, 404, "agent_release_not_found", "agent release or deployment not found")
		case errors.Is(e, agentreleases.ErrConflict):
			writeAPIError(w, 409, "agent_release_conflict", "the deployment changed; refresh and retry")
		case errors.Is(e, agentreleases.ErrDenied):
			writeAPIError(w, 422, "agent_release_prerequisite_missing", "current review, pilot, policy, resource, role, or rollback evidence is missing")
		case errors.Is(e, agentreleases.ErrInvalid):
			writeAPIError(w, 422, "agent_release_invalid", "the agent release or deployment contract is invalid")
		default:
			writeAPIError(w, 500, "agent_release_unavailable", "agent release governance is unavailable")
		}
	}
	isOwner := func(w http.ResponseWriter, r *http.Request) (auth.Credential, bool) {
		actor, owner, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return actor, false
		}
		if actor.AgentID != "" || !owner {
			writeAPIError(w, 403, "agent_release_owner_required", "only the human repository owner can govern an agent release")
			return actor, false
		}
		return actor, true
	}
	type approvalInput struct {
		Kind       string `json:"kind"`
		Evidence   string `json:"evidence"`
		EvidenceID string `json:"evidence_id"`
	}
	mux.HandleFunc("POST /repositories/{id}/agent-candidates/{candidate_id}/release-approvals", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if actor.AgentID != "" {
			writeAPIError(w, 403, "agent_release_human_approval_required", "release approvals require an attributable human participant")
			return
		}
		candidate, e := candidates.Get(r.PathValue("candidate_id"))
		if e != nil || candidate.RepositoryID != r.PathValue("id") {
			writeErr(w, agentreleases.ErrNotFound)
			return
		}
		var in approvalInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an approval decision is required")
			return
		}
		evidence := in.Evidence
		switch in.Kind {
		case "evaluation":
			runs, er := candidates.Runs(candidate.ID)
			passed, found := er == nil, false
			for _, run := range runs {
				if run.ID != in.EvidenceID {
					continue
				}
				found = true
				evidence = run.ID
				if run.Contaminated {
					passed = false
				}
				for _, result := range run.Results {
					if !result.TaskSuccess || !result.PolicyAdherence || result.EvaluatorDecision != "passed" {
						passed = false
					}
				}
			}
			if !found || !passed {
				writeErr(w, agentreleases.ErrDenied)
				return
			}
		case "pilot_acceptance":
			pilot, er := pilots.Get(in.EvidenceID)
			accepted := er == nil && pilot.CandidateID == candidate.ID && !pilot.Paused && len(pilot.Feedback) > 0
			if !accepted {
				writeErr(w, agentreleases.ErrDenied)
				return
			}
			evidence = pilot.ID
		case "domain_review", "data_policy", "resources":
			// The authenticated decision is the evidence record; prose is rationale, not an authority reference.
		default:
			writeErr(w, agentreleases.ErrInvalid)
			return
		}
		out, e := releases.CreateApproval(candidate.ID, in.Kind, actor.UserID, evidence)
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, 201, out)
	})
	mux.HandleFunc("POST /repositories/{id}/agent-releases", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := isOwner(w, r)
		if !ok {
			return
		}
		var in agentreleases.Release
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an attested agent release is required")
			return
		}
		c, e := candidates.Get(in.CandidateID)
		if e != nil || c.RepositoryID != r.PathValue("id") || c.PullRevision != in.CandidateRevision || c.ProjectID != in.ProjectID || c.ProjectVersion != in.ProjectVersion || c.ContractDigest != in.ContractDigest {
			writeErr(w, agentreleases.ErrDenied)
			return
		}
		pull, e := pulls.Get(c.RepositoryID, c.PullRequestID)
		if e != nil || pull.SourceCommitID != c.PullRevision {
			writeErr(w, agentreleases.ErrDenied)
			return
		}
		p, e := pilots.Get(in.PilotID)
		if e != nil || p.CandidateID != c.ID || p.Paused || len(p.Feedback) == 0 {
			writeErr(w, agentreleases.ErrDenied)
			return
		}
		in.Approvals = nil
		for _, approvalID := range in.ApprovalIDs {
			approval, er := releases.GetApproval(approvalID)
			if er != nil || approval.CandidateID != c.ID {
				writeErr(w, agentreleases.ErrDenied)
				return
			}
			in.Approvals = append(in.Approvals, approval)
			if _, e := catalog.Get(approval.OwnerID, c.RepositoryID); e != nil {
				writeErr(w, agentreleases.ErrDenied)
				return
			}
		}
		org, e := orgs.Get(in.OrganizationID)
		if e != nil {
			writeErr(w, agentreleases.ErrDenied)
			return
		}
		agentOK := false
		for _, a := range org.Agents {
			if a.ID == in.AgentID && slices.Contains(a.OperatorIDs, actor.UserID) {
				agentOK = true
			}
		}
		if !agentOK {
			writeErr(w, agentreleases.ErrDenied)
			return
		}
		in.ID = ""
		in.RepositoryID = c.RepositoryID
		in.CreatedBy = actor.UserID
		out, e := releases.CreateRelease(in)
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, 201, out)
	})
	mux.HandleFunc("GET /repositories/{id}/agent-releases", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		items, e := releases.ListReleases(r.PathValue("id"))
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, 200, map[string]any{"releases": items})
	})
	mux.HandleFunc("POST /repositories/{id}/agent-releases/{release_id}/deployments", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := isOwner(w, r)
		if !ok {
			return
		}
		rel, e := releases.GetRelease(r.PathValue("release_id"))
		if e != nil || rel.RepositoryID != r.PathValue("id") {
			writeErr(w, agentreleases.ErrNotFound)
			return
		}
		var in agentreleases.Deployment
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a bounded agent deployment is required")
			return
		}
		in.ID = ""
		in.ReleaseID = rel.ID
		in.CreatedBy = actor.UserID
		out, e := releases.CreateDeployment(in)
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, 201, out)
	})
	type action struct {
		ExpectedVersion int                  `json:"expected_version"`
		Kind            string               `json:"kind"`
		Summary         string               `json:"summary"`
		Signal          agentreleases.Signal `json:"signal"`
	}
	mux.HandleFunc("POST /repositories/{id}/agent-deployments/{deployment_id}/actions", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := isOwner(w, r)
		if !ok {
			return
		}
		d, e := releases.GetDeployment(r.PathValue("deployment_id"))
		if e != nil {
			writeErr(w, e)
			return
		}
		rel, e := releases.GetRelease(d.ReleaseID)
		if e != nil || rel.RepositoryID != r.PathValue("id") {
			writeErr(w, agentreleases.ErrNotFound)
			return
		}
		var in action
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a versioned deployment action is required")
			return
		}
		var out agentreleases.Deployment
		if in.Kind == "signal" {
			out, e = releases.Signal(d.ID, actor.UserID, in.ExpectedVersion, in.Signal)
		} else {
			out, e = releases.Control(d.ID, actor.UserID, in.ExpectedVersion, in.Kind, in.Summary)
		}
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, 200, out)
	})
}
