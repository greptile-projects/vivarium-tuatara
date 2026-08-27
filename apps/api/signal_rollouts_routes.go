package main

import (
	"errors"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/signalrollouts"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/telemetrycontracts"
)

func registerSignalRolloutRoutes(mux *http.ServeMux, repos *repositories.Store, credentials *auth.Store, contracts *telemetrycontracts.Store, environments *deployments.Store, rollouts *signalrollouts.Store) {
	mux.HandleFunc("GET /repositories/{id}/signal-rollouts", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id")); !ok {
			return
		}
		xs, e := rollouts.List(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "signal_rollouts_unavailable", "signal rollouts could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"signal_rollouts": xs})
	})
	mux.HandleFunc("GET /repositories/{id}/signal-rollouts/{rollout_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id")); !ok {
			return
		}
		x, e := rollouts.Get(r.PathValue("id"), r.PathValue("rollout_id"))
		if e != nil {
			writeAPIError(w, 404, "signal_rollout_not_found", "signal rollout not found")
			return
		}
		writeJSON(w, 200, x)
	})
	mux.HandleFunc("POST /repositories/{id}/signal-rollouts", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if actor.AgentID != "" {
			writeAPIError(w, 403, "human_operator_required", "agents have no telemetry or environment authority")
			return
		}
		var in signalrollouts.Rollout
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an exact protected instrumentation rollout is required")
			return
		}
		contract, e := contracts.Get(r.PathValue("id"), in.ContractID)
		if e != nil || contract.Acceptance == nil || contract.Acceptance.Version != in.ContractVersion || contract.CurrentVersion != in.ContractVersion {
			writeAPIError(w, 422, "telemetry_contract_not_accepted", "the exact current accepted telemetry contract is required")
			return
		}
		env, e := environments.GetEnvironment(r.PathValue("id"), in.EnvironmentID)
		if e != nil || env.RequiredApprovals < 1 {
			writeAPIError(w, 422, "protected_environment_invalid", "a protected repository environment is required")
			return
		}
		promotion, e := environments.GetPromotion(r.PathValue("id"), in.DeploymentID)
		if e != nil || promotion.EnvironmentID != in.EnvironmentID || promotion.CommitID != in.InstrumentationRevision || promotion.State != "succeeded" {
			writeAPIError(w, 422, "instrumentation_deployment_invalid", "instrumentation must be a successful exact protected-environment deployment")
			return
		}
		in.ControllerID = actor.UserID
		var out signalrollouts.Rollout
		e = repos.WithCurrentParticipant(actor.UserID, r.PathValue("id"), func() error {
			var writeErr error
			out, writeErr = rollouts.Create(r.PathValue("id"), actor.UserID, in.RequestID, in)
			return writeErr
		})
		writeSignalRollout(w, out, e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/signal-rollouts/{rollout_id}/events", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if actor.AgentID != "" {
			writeAPIError(w, 403, "human_operator_required", "agents may not control collection or submit production evidence")
			return
		}
		var in struct {
			ExpectedVersion int                  `json:"expected_version"`
			Event           signalrollouts.Event `json:"event"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an optimistic collection event is required")
			return
		}
		current, e := rollouts.Get(r.PathValue("id"), r.PathValue("rollout_id"))
		if e != nil {
			writeAPIError(w, 404, "signal_rollout_not_found", "signal rollout not found")
			return
		}
		if current.ControllerID != actor.UserID {
			repo, _ := repos.Get(actor.UserID, r.PathValue("id"))
			if repo.OwnerID != actor.UserID {
				writeAPIError(w, 403, "operator_required", "only the active controller or repository owner may steer collection")
				return
			}
		}
		if in.Event.Observation != nil && in.Event.Observation.DeploymentID != "" {
			promotion, promotionErr := environments.GetPromotion(r.PathValue("id"), in.Event.Observation.DeploymentID)
			if promotionErr != nil || promotion.State != "succeeded" || promotion.EnvironmentID != current.EnvironmentID || promotion.CompletedAt == nil || in.Event.Observation.EndedAt.Before(*promotion.CompletedAt) {
				writeAPIError(w, 422, "signal_observation_deployment_invalid", "observation deployment must be a completed exact promotion in the rollout environment")
				return
			}
			in.Event.Observation.ReleaseID = promotion.ReleaseID
			in.Event.Observation.CommitID = promotion.CommitID
		}
		in.Event.ActorID = actor.UserID
		var out signalrollouts.Rollout
		e = repos.WithCurrentParticipant(actor.UserID, r.PathValue("id"), func() error {
			var writeErr error
			out, writeErr = rollouts.Mutate(r.PathValue("id"), current.ID, in.ExpectedVersion, in.Event)
			return writeErr
		})
		writeSignalRollout(w, out, e, 201)
	})
}
func writeSignalRollout(w http.ResponseWriter, out signalrollouts.Rollout, e error, status int) {
	switch {
	case e == nil:
		writeJSON(w, status, out)
	case errors.Is(e, signalrollouts.ErrNotFound):
		writeAPIError(w, 404, "signal_rollout_not_found", "signal rollout not found")
	case errors.Is(e, signalrollouts.ErrConflict):
		writeAPIError(w, 409, "signal_rollout_conflict", "the rollout advanced or request identity changed")
	default:
		writeAPIError(w, 422, "signal_rollout_invalid", "collection scope, evidence, budget, or transition is invalid")
	}
}
