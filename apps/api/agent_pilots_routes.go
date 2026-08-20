package main

import (
	"errors"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/agentcandidates"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/agentpilots"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

type pilotActionInput struct {
	ExpectedVersion int                      `json:"expected_version"`
	Kind            string                   `json:"kind"`
	Session         agentpilots.Session      `json:"session"`
	SessionID       string                   `json:"session_id"`
	Event           agentpilots.SessionEvent `json:"event"`
	Feedback        agentpilots.Feedback     `json:"feedback"`
}

func registerAgentPilotRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, pulls *pullrequests.Store, candidates *agentcandidates.Store, pilots *agentpilots.Store) {
	writeErr := func(w http.ResponseWriter, e error) {
		switch {
		case errors.Is(e, agentpilots.ErrNotFound):
			writeAPIError(w, 404, "agent_pilot_not_found", "agent pilot not found")
		case errors.Is(e, agentpilots.ErrConflict):
			writeAPIError(w, 409, "agent_pilot_conflict", "the pilot changed; refresh effective access")
		case errors.Is(e, agentpilots.ErrDenied):
			writeAPIError(w, 403, "agent_pilot_denied", "pilot consent, scope, policy, candidate, or budget denied the action")
		case errors.Is(e, agentpilots.ErrInvalid):
			writeAPIError(w, 422, "agent_pilot_invalid", "the bounded pilot request is invalid")
		default:
			writeAPIError(w, 500, "agent_pilot_unavailable", "agent pilot evidence is unavailable")
		}
	}
	isParticipant := func(userID, repositoryID string) bool {
		target, e := catalog.Get(userID, repositoryID)
		if e != nil {
			return false
		}
		member, _ := catalog.HasCollaborator(userID, repositoryID)
		return target.OwnerID == userID || member
	}
	context := func(p agentpilots.Pilot, participant string) (bool, bool) {
		pull, e := pulls.Get(p.RepositoryID, p.PullRequestID)
		changed := e != nil || pull.SourceCommitID != p.CandidateRevision
		revoked := false
		for _, rid := range p.RepositoryIDs {
			if !isParticipant(participant, rid) {
				revoked = true
				break
			}
		}
		return changed, revoked
	}
	project := func(p agentpilots.Pilot, participant string) map[string]any {
		changed, revoked := context(p, participant)
		return map[string]any{"pilot": p, "effective_access": pilots.Effective(p, participant, changed, revoked)}
	}
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/agent-candidates/{candidate_id}/pilots", func(w http.ResponseWriter, r *http.Request) {
		actor, owner, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if actor.AgentID != "" || !owner {
			writeAPIError(w, 403, "agent_pilot_owner_required", "only the human repository owner can publish a candidate pilot")
			return
		}
		repo, e := catalog.GetByID(r.PathValue("id"))
		if e != nil {
			writeErr(w, e)
			return
		}
		c, e := candidates.Get(r.PathValue("candidate_id"))
		if e != nil || c.RepositoryID != repo.ID || c.PullRequestID != r.PathValue("pull_id") {
			writeErr(w, agentpilots.ErrNotFound)
			return
		}
		pull, e := pulls.Get(repo.ID, c.PullRequestID)
		if e != nil || pull.SourceCommitID != c.PullRevision {
			writeErr(w, agentpilots.ErrDenied)
			return
		}
		var in agentpilots.Pilot
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "candidate pilot scope is required")
			return
		}
		for _, rid := range in.RepositoryIDs {
			target, e := catalog.Get(actor.UserID, rid)
			if e != nil || target.OwnerID != actor.UserID {
				writeErr(w, agentpilots.ErrDenied)
				return
			}
		}
		for _, invite := range in.Invitations {
			for _, rid := range invite.RepositoryIDs {
				if !isParticipant(invite.ParticipantID, rid) {
					writeErr(w, agentpilots.ErrDenied)
					return
				}
			}
		}
		in.ID = ""
		in.RepositoryID = repo.ID
		in.PullRequestID = c.PullRequestID
		in.CandidateID = c.ID
		in.CandidateRevision = c.PullRevision
		in.OwnerID = actor.UserID
		out, e := pilots.Create(in)
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, 201, project(out, actor.UserID))
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/agent-candidates/{candidate_id}/pilots", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		items, e := pilots.List(r.PathValue("id"), r.PathValue("pull_id"))
		if e != nil {
			writeErr(w, e)
			return
		}
		out := []map[string]any{}
		for _, p := range items {
			if p.CandidateID != r.PathValue("candidate_id") {
				continue
			}
			visible := p.OwnerID == actor.UserID
			for _, x := range p.Invitations {
				visible = visible || x.ParticipantID == actor.UserID
			}
			if visible {
				out = append(out, project(p, actor.UserID))
			}
		}
		writeJSON(w, 200, map[string]any{"pilots": out})
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/agent-candidates/{candidate_id}/pilots/{pilot_id}/actions", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		p, e := pilots.Get(r.PathValue("pilot_id"))
		if e != nil || p.RepositoryID != r.PathValue("id") || p.PullRequestID != r.PathValue("pull_id") || p.CandidateID != r.PathValue("candidate_id") {
			writeErr(w, agentpilots.ErrNotFound)
			return
		}
		var in pilotActionInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a versioned pilot action is required")
			return
		}
		changed, revoked := context(p, actor.UserID)
		switch in.Kind {
		case "consent":
			p, e = pilots.Consent(p.ID, actor.UserID, in.ExpectedVersion, false)
		case "revoke_consent":
			p, e = pilots.Consent(p.ID, actor.UserID, in.ExpectedVersion, true)
		case "delegate":
			p, e = pilots.StartSession(p.ID, actor.UserID, in.ExpectedVersion, in.Session, changed, revoked)
		case "session_event":
			p, e = pilots.AppendEvent(p.ID, actor.UserID, in.ExpectedVersion, in.SessionID, in.Event, changed, revoked)
		case "feedback":
			p, e = pilots.AddFeedback(p.ID, actor.UserID, in.ExpectedVersion, in.Feedback)
		default:
			e = agentpilots.ErrInvalid
		}
		if e != nil {
			writeErr(w, e)
			return
		}
		writeJSON(w, 200, project(p, actor.UserID))
	})
}
