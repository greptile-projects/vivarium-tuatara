package main

import (
	"errors"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

func registerDeploymentRoutes(mux *http.ServeMux, repositories *repositories.Store, releases *releases.Store, builds *checkruns.Store, store *deployments.Store, credentials *auth.Store) {
	executor := deployments.NewExecutor(store, builds)
	read := func(w http.ResponseWriter, r *http.Request) bool {
		_, _, ok := authorizeRepositoryRead(w, r, repositories, credentials, r.PathValue("id"))
		return ok
	}
	mux.HandleFunc("GET /repositories/{id}/environments", func(w http.ResponseWriter, r *http.Request) {
		if !read(w, r) {
			return
		}
		values, err := store.ListEnvironments(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "environment_read_failed", "environments could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"environments": values})
	})
	putEnvironment := func(w http.ResponseWriter, r *http.Request) {
		actor, owner, ok := authorizeRepositoryParticipant(w, r, repositories, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if !owner {
			writeAPIError(w, 403, "owner_required", "only repository owners can manage environments")
			return
		}
		var input struct {
			Name              string            `json:"name"`
			Position          int               `json:"position"`
			Image             string            `json:"image"`
			Command           string            `json:"command"`
			TimeoutSeconds    int               `json:"timeout_seconds"`
			Configuration     map[string]string `json:"configuration"`
			Credentials       map[string]string `json:"credentials"`
			RequiredApprovals int               `json:"required_approvals"`
			Concurrency       int               `json:"concurrency"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		value, err := store.PutEnvironment(deployments.Environment{ID: r.PathValue("environment_id"), RepositoryID: r.PathValue("id"), Name: input.Name, Position: input.Position, Image: input.Image, Command: input.Command, TimeoutSeconds: input.TimeoutSeconds, Configuration: input.Configuration, Credentials: input.Credentials, RequiredApprovals: input.RequiredApprovals, Concurrency: input.Concurrency, UpdatedBy: actor.UserID})
		if errors.Is(err, deployments.ErrInvalid) {
			writeAPIError(w, 422, "invalid_environment", "environment policy is invalid or conflicts with its ordered position")
			return
		}
		if errors.Is(err, deployments.ErrNotFound) {
			writeAPIError(w, 404, "environment_not_found", "environment not found")
			return
		}
		if err != nil {
			writeAPIError(w, 500, "environment_write_failed", "environment could not be saved")
			return
		}
		writeJSON(w, map[bool]int{true: 201, false: 200}[r.PathValue("environment_id") == ""], value)
	}
	mux.HandleFunc("POST /repositories/{id}/environments", putEnvironment)
	mux.HandleFunc("PUT /repositories/{id}/environments/{environment_id}", putEnvironment)
	mux.HandleFunc("GET /repositories/{id}/deployments", func(w http.ResponseWriter, r *http.Request) {
		if !read(w, r) {
			return
		}
		values, err := store.ListPromotions(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "deployment_read_failed", "deployments could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"deployments": values})
	})
	mux.HandleFunc("POST /repositories/{id}/deployments", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repositories, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var input struct {
			EnvironmentID string `json:"environment_id"`
			ReleaseID     string `json:"release_id"`
			BuildID       string `json:"build_id"`
			ArtifactID    string `json:"artifact_id"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		candidate, err := releases.Get(r.PathValue("id"), input.ReleaseID)
		if err != nil {
			writeAPIError(w, 422, "invalid_release", "release candidate not found")
			return
		}
		run, err := builds.Get(r.PathValue("id"), input.ReleaseID, input.BuildID)
		if err != nil || run.CommitID != candidate.CommitID || run.State != "succeeded" {
			writeAPIError(w, 422, "unverified_build", "deployment requires a successful build from the release's exact commit")
			return
		}
		var artifact *checkruns.Artifact
		for i := range run.Artifacts {
			if run.Artifacts[i].ID == input.ArtifactID {
				artifact = &run.Artifacts[i]
			}
		}
		if artifact == nil {
			writeAPIError(w, 422, "invalid_artifact", "artifact does not belong to the selected build")
			return
		}
		value, err := store.CreatePromotion(deployments.Promotion{RepositoryID: r.PathValue("id"), EnvironmentID: input.EnvironmentID, ReleaseID: input.ReleaseID, BuildID: input.BuildID, ArtifactID: input.ArtifactID, ArtifactSHA256: artifact.SHA256, InitiatedBy: actor.UserID})
		if errors.Is(err, deployments.ErrBlocked) {
			writeAPIError(w, 409, "promotion_blocked", "the environment is busy or the exact artifact has not passed the preceding environment")
			return
		}
		if err != nil {
			writeAPIError(w, 422, "invalid_promotion", "promotion request is invalid")
			return
		}
		writeJSON(w, 202, value)
		if value.State == "queued" {
			go executor.Execute(value.RepositoryID, value.ID)
		}
	})
	mux.HandleFunc("POST /repositories/{id}/deployments/{deployment_id}/approvals", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repositories, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		value, err := store.Approve(r.PathValue("id"), r.PathValue("deployment_id"), actor.UserID)
		if errors.Is(err, deployments.ErrBlocked) {
			writeAPIError(w, 409, "approval_blocked", "approval must be distinct and the deployment must still await approval")
			return
		}
		if err != nil {
			writeAPIError(w, 404, "deployment_not_found", "deployment not found")
			return
		}
		writeJSON(w, 200, value)
		if value.State == "queued" {
			go executor.Execute(value.RepositoryID, value.ID)
		}
	})
}
