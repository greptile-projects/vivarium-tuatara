package main

import (
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/capacityplans"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/capacityrollouts"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

func registerCapacityRolloutRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, plans *capacityplans.Store, environments *deployments.Store, rollouts *capacityrollouts.Store) {
	read := func(w http.ResponseWriter, r *http.Request) bool {
		_, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		return ok
	}
	mux.HandleFunc("GET /repositories/{id}/capacity-rollouts", func(w http.ResponseWriter, r *http.Request) {
		if !read(w, r) {
			return
		}
		xs, e := rollouts.List(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "capacity_rollouts_unavailable", "capacity rollouts could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"capacity_rollouts": xs})
	})
	mux.HandleFunc("GET /repositories/{id}/capacity-rollouts/{rollout_id}", func(w http.ResponseWriter, r *http.Request) {
		if !read(w, r) {
			return
		}
		x, e := rollouts.Get(r.PathValue("id"), r.PathValue("rollout_id"))
		if e != nil {
			writeAPIError(w, 404, "capacity_rollout_not_found", "capacity rollout not found")
			return
		}
		writeJSON(w, 200, x)
	})
	mux.HandleFunc("POST /repositories/{id}/capacity-rollouts", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if c.AgentID != "" {
			writeAPIError(w, 403, "human_operator_required", "only a human operator can stage a production capacity rollout")
			return
		}
		var in struct {
			RequestID string                   `json:"request_id"`
			PlanID    string                   `json:"plan_id"`
			Phases    []capacityrollouts.Phase `json:"phases"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a plan and protected-environment phases are required")
			return
		}
		p, e := plans.Get(r.PathValue("id"), in.PlanID)
		if e != nil || p.Delivery == nil || p.Delivery.Status != "created" || len(in.Phases) != len(p.Phases) {
			writeAPIError(w, 422, "capacity_plan_not_delivered", "an exactly delivered capacity plan is required")
			return
		}
		envIDs := []string{}
		for i, phase := range in.Phases {
			if phase.ID != p.Phases[i].ID || phase.ControllerID != p.Phases[i].OwnerID || phase.ControllerType != "human" && phase.ControllerType != "agent" {
				writeAPIError(w, 422, "capacity_rollout_invalid", "phases must preserve plan order, owner, and controller type")
				return
			}
			if _, x := environments.GetEnvironment(r.PathValue("id"), phase.EnvironmentID); x != nil {
				writeAPIError(w, 422, "protected_environment_invalid", "every phase requires a repository protected environment")
				return
			}
			for j, id := range phase.DeploymentIDs {
				d, x := environments.GetPromotion(r.PathValue("id"), id)
				if x != nil || j >= len(phase.DeployedRevisions) || d.CommitID != phase.DeployedRevisions[j] || d.EnvironmentID != phase.EnvironmentID {
					writeAPIError(w, 422, "deployed_revision_invalid", "deployment evidence must resolve to the exact protected environment revision")
					return
				}
			}
			envIDs = append(envIDs, phase.EnvironmentID)
		}
		out, e := rollouts.Create(r.PathValue("id"), c.UserID, in.RequestID, capacityrollouts.Rollout{PlanID: p.ID, EnvironmentIDs: envIDs, Phases: in.Phases})
		if errors.Is(e, capacityrollouts.ErrConflict) {
			writeAPIError(w, 409, "capacity_rollout_conflict", "request identity was reused with changed content")
			return
		}
		if e != nil {
			writeAPIError(w, 422, "capacity_rollout_invalid", "the production rollout is incomplete")
			return
		}
		writeJSON(w, 201, out)
	})
	mux.HandleFunc("POST /repositories/{id}/capacity-rollouts/{rollout_id}/events", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int                        `json:"expected_version"`
			Event           capacityrollouts.Event     `json:"event"`
			Evidence        *capacityrollouts.Evidence `json:"evidence"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an optimistic event is required")
			return
		}
		current, e := rollouts.Get(r.PathValue("id"), r.PathValue("rollout_id"))
		if e != nil {
			writeAPIError(w, 404, "capacity_rollout_not_found", "capacity rollout not found")
			return
		}
		actor := c.UserID
		typ := "human"
		if c.AgentID != "" {
			actor = c.AgentID
			typ = "agent"
			if c.RepositoryID != r.PathValue("id") {
				writeAPIError(w, 403, "agent_delegation_required", "agent credential must be bound to this repository")
				return
			}
		}
		phaseIndex := slices.IndexFunc(current.Phases, func(p capacityrollouts.Phase) bool { return p.ID == in.Event.PhaseID })
		if phaseIndex < 0 {
			writeAPIError(w, 422, "capacity_phase_invalid", "event phase does not exist")
			return
		}
		phase := current.Phases[phaseIndex]
		if typ == "agent" && (phase.ControllerType != "agent" || phase.ControllerID != actor) {
			writeAPIError(w, 403, "agent_delegation_required", "agents may execute only the exact phase explicitly delegated to them")
			return
		}
		if typ == "human" && phase.ControllerID != actor {
			repo, _ := catalog.Get(actor, r.PathValue("id"))
			if repo.OwnerID != actor {
				writeAPIError(w, 403, "operator_required", "only the phase controller or repository owner may steer scaling")
				return
			}
		}
		if typ == "agent" && slices.Contains([]string{"stage", "rollback", "replan"}, in.Event.Kind) {
			writeAPIError(w, 403, "human_operator_required", "agents cannot stage, roll back, or replan capacity")
			return
		}
		if in.Evidence != nil {
			for i, id := range in.Evidence.DeploymentIDs {
				d, x := environments.GetPromotion(r.PathValue("id"), id)
				if x != nil || d.EnvironmentID != phase.EnvironmentID || i >= len(in.Evidence.DeployedRevisions) || d.CommitID != in.Evidence.DeployedRevisions[i] || !strings.EqualFold(d.State, "succeeded") {
					writeAPIError(w, 422, "production_evidence_invalid", "current evidence requires successful exact protected-environment deployments")
					return
				}
			}
		}
		in.Event.ActorID = actor
		in.Event.ActorType = typ
		out, e := rollouts.Mutate(r.PathValue("id"), current.ID, in.ExpectedVersion, in.Event, in.Evidence)
		if errors.Is(e, capacityrollouts.ErrConflict) {
			writeAPIError(w, 409, "capacity_rollout_conflict", "the rollout advanced or request identity changed")
			return
		}
		if e != nil {
			writeAPIError(w, 422, "capacity_transition_invalid", "the requested control or production evidence is invalid")
			return
		}
		writeJSON(w, 201, out)
	})
}
