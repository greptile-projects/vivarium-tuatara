package main

import (
	"errors"
	"log"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/performancegoals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

type performanceGoalInput struct {
	ExpectedVersion int                       `json:"expected_version"`
	Revision        performancegoals.Revision `json:"revision"`
}

func registerPerformanceGoalRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, store *performancegoals.Store) {
	mux.HandleFunc("POST /repositories/{id}/performance-goals", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in performanceGoalInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete performance goal revision is required")
			return
		}
		var goal performancegoals.Goal
		err := catalog.WithCurrentParticipant(actor.UserID, r.PathValue("id"), func() error {
			var e error
			goal, e = store.Create(r.PathValue("id"), actor.UserID, in.Revision)
			return e
		})
		writePerformanceGoal(w, goal, err, 201)
	})
	mux.HandleFunc("GET /repositories/{id}/performance-goals", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		goals, err := store.List(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "performance_goals_unavailable", "performance goals could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"goals": goals})
	})
	mux.HandleFunc("GET /repositories/{id}/performance-goals/{goal_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		goal, err := store.Get(r.PathValue("goal_id"))
		if err != nil || goal.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "performance_goal_not_found", "performance goal not found")
			return
		}
		writeJSON(w, 200, goal)
	})
	mux.HandleFunc("POST /repositories/{id}/performance-goals/{goal_id}/revisions", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		current, err := store.Get(r.PathValue("goal_id"))
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "performance_goal_not_found", "performance goal not found")
			return
		}
		var in performanceGoalInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "expected_version and a complete revision are required")
			return
		}
		var goal performancegoals.Goal
		err = catalog.WithCurrentParticipant(actor.UserID, current.RepositoryID, func() error {
			var e error
			goal, e = store.Revise(current.ID, in.ExpectedVersion, actor.UserID, in.Revision)
			return e
		})
		writePerformanceGoal(w, goal, err, 200)
	})
}

func writePerformanceGoal(w http.ResponseWriter, goal performancegoals.Goal, err error, success int) {
	switch {
	case err == nil:
		writeJSON(w, success, goal)
	case errors.Is(err, performancegoals.ErrConflict):
		writeAPIError(w, 409, "performance_goal_conflict", "the goal changed; reload before publishing another revision")
	case errors.Is(err, performancegoals.ErrInvalid):
		writeAPIError(w, 400, "invalid_performance_goal", "the contract must include valid workloads, metrics, targets, constraints, environments, owners, and baseline policy")
	case errors.Is(err, repositories.ErrInvalidCollaborator), errors.Is(err, repositories.ErrNotFound):
		writeAPIError(w, 403, "performance_goal_forbidden", "only a current repository participant may publish performance goals")
	default:
		log.Printf("performance goal storage: %v", err)
		writeAPIError(w, 500, "performance_goals_unavailable", "performance goals could not be persisted")
	}
}
