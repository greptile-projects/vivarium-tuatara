package main

import (
	"errors"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/observabilitygaps"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/signalevaluations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/signalrollouts"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/telemetrycontracts"
)

func registerSignalEvaluationRoutes(mux *http.ServeMux, repos *repositories.Store, credentials *auth.Store, gaps *observabilitygaps.Store, contracts *telemetrycontracts.Store, rollouts *signalrollouts.Store, evaluations *signalevaluations.Store) {
	mux.HandleFunc("GET /repositories/{id}/signal-evaluations", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id")); !ok {
			return
		}
		xs, e := evaluations.List(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "signal_evaluations_unavailable", "signal evaluations could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"signal_evaluations": xs})
	})
	mux.HandleFunc("GET /repositories/{id}/signal-evaluations/{evaluation_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id")); !ok {
			return
		}
		x, e := evaluations.Get(r.PathValue("id"), r.PathValue("evaluation_id"))
		writeSignalEvaluation(w, x, e, 200)
	})
	mux.HandleFunc("POST /repositories/{id}/signal-evaluations", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if actor.AgentID != "" {
			writeAPIError(w, 403, "human_owner_required", "only humans may open the accountable evaluation")
			return
		}
		var in signalevaluations.Evaluation
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an exact signal evaluation is required")
			return
		}
		gap, e := gaps.Get(in.GapID)
		if e != nil || gap.RepositoryID != r.PathValue("id") || gap.CurrentVersion != in.GapVersion {
			writeAPIError(w, 422, "signal_gap_changed", "the exact observability gap is unavailable or changed")
			return
		}
		contract, e := contracts.Get(r.PathValue("id"), in.ContractID)
		if e != nil || contract.CurrentVersion != in.ContractVersion || len(contract.Revisions) < in.ContractVersion || contract.Revisions[in.ContractVersion-1].GapID != in.GapID || contract.Revisions[in.ContractVersion-1].GapVersion != in.GapVersion {
			writeAPIError(w, 422, "signal_contract_changed", "the exact telemetry contract is unavailable or changed")
			return
		}
		rollout, e := rollouts.Get(r.PathValue("id"), in.RolloutID)
		if e != nil || rollout.Version != in.RolloutVersion || rollout.ContractID != in.ContractID || rollout.ContractVersion != in.ContractVersion {
			writeAPIError(w, 422, "signal_rollout_changed", "the exact delivered rollout is unavailable, changed, or unrelated")
			return
		}
		delivered := map[string]bool{}
		for _, observation := range rollout.Observations {
			delivered[observation.ID] = true
		}
		for _, signalID := range in.SignalIDs {
			if !delivered[signalID] {
				writeAPIError(w, 422, "signal_not_delivered", "every evaluated signal must be an observation from the exact rollout")
				return
			}
		}
		var out signalevaluations.Evaluation
		e = repos.WithCurrentParticipants(append([]string{actor.UserID}, in.OwnerIDs...), r.PathValue("id"), func() error { var x error; out, x = evaluations.Create(r.PathValue("id"), actor.UserID, in); return x })
		writeSignalEvaluation(w, out, e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/signal-evaluations/{evaluation_id}/findings", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		if actor.UserID == "" && actor.AgentID == "" {
			writeAuthenticationRequired(w, false)
			return
		}
		if actor.AgentID != "" && actor.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 403, "signal_finding_forbidden", "read-only agent must be bound to this repository")
			return
		}
		if actor.AgentID == "" {
			repo, _ := repos.GetByID(r.PathValue("id"))
			member := repo.OwnerID == actor.UserID
			if !member {
				member, _ = repos.HasCollaborator(actor.UserID, r.PathValue("id"))
			}
			if !member {
				writeAPIError(w, 403, "signal_finding_forbidden", "current participants and repository-bound read-only agents may publish findings")
				return
			}
		}
		var in signalevaluations.Finding
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a reproducible cited finding is required")
			return
		}
		x, e := evaluations.Get(r.PathValue("id"), r.PathValue("evaluation_id"))
		if e != nil {
			writeSignalEvaluation(w, x, e, 404)
			return
		}
		allowed := map[string]bool{}
		for _, id := range x.SignalIDs {
			allowed[id] = true
		}
		for _, c := range x.Correlations {
			allowed[c.ResourceID] = true
		}
		for _, c := range in.Citations {
			if !allowed[c.ResourceID] {
				writeAPIError(w, 422, "signal_citation_invalid", "citations must resolve through the evaluation's frozen signals or correlations")
				return
			}
		}
		if actor.AgentID != "" {
			in.ActorKind = "agent"
			in.ActorID = actor.AgentID
		} else {
			in.ActorKind = "human"
			in.ActorID = actor.UserID
		}
		out, e := evaluations.AddFinding(r.PathValue("id"), x.ID, in)
		writeSignalEvaluation(w, out, e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/signal-evaluations/{evaluation_id}/decisions", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if actor.AgentID != "" {
			writeAPIError(w, 403, "human_owner_required", "agents cannot change or retire collection")
			return
		}
		var in signalevaluations.Decision
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an evidence-backed lifecycle decision is required")
			return
		}
		current, e := evaluations.Get(r.PathValue("id"), r.PathValue("evaluation_id"))
		if e != nil {
			writeSignalEvaluation(w, current, e, 404)
			return
		}
		owner := false
		for _, id := range current.OwnerIDs {
			owner = owner || id == actor.UserID
		}
		if !owner {
			writeAPIError(w, 403, "signal_owner_required", "only a declared current owner may decide the signal lifecycle")
			return
		}
		in.ActorID = actor.UserID
		var out signalevaluations.Evaluation
		e = repos.WithCurrentParticipant(actor.UserID, r.PathValue("id"), func() error { var x error; out, x = evaluations.Decide(r.PathValue("id"), current.ID, in); return x })
		writeSignalEvaluation(w, out, e, 201)
	})
}
func writeSignalEvaluation(w http.ResponseWriter, out signalevaluations.Evaluation, e error, status int) {
	switch {
	case e == nil:
		writeJSON(w, status, out)
	case errors.Is(e, signalevaluations.ErrNotFound):
		writeAPIError(w, 404, "signal_evaluation_not_found", "signal evaluation not found")
	case errors.Is(e, signalevaluations.ErrConflict):
		writeAPIError(w, 409, "signal_evaluation_conflict", "the evaluation advanced or request identity changed")
	default:
		writeAPIError(w, 422, "signal_evaluation_invalid", "the evaluation, evidence, impact preview, repair, policy approval, or stop proof is incomplete")
	}
}
