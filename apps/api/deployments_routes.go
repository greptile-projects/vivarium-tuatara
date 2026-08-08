package main

import (
	"errors"
	"net/http"
	"os/exec"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/activities"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/changesessions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

func registerDeploymentRoutes(mux *http.ServeMux, gitStore *storage.Store, repositories *repositories.Store, releases *releases.Store, builds *checkruns.Store, store *deployments.Store, credentials *auth.Store, activityStore *activities.Store, pulls *pullrequests.Store, sessions *changesessions.Store) {
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
		repository, openErr := gitStore.Open(r.PathValue("id"))
		if openErr != nil {
			writeAPIError(w, 422, "rollout_definition_unavailable", "release repository is unavailable")
			return
		}
		body, definitionErr := exec.Command("git", "--git-dir="+repository.Path(), "show", candidate.CommitID+":"+deployments.ConfigPath).Output()
		if definitionErr != nil {
			writeAPIError(w, 422, "rollout_definition_missing", "the release commit must contain .vivarium/deployment.json")
			return
		}
		rollout, definitionErr := deployments.ParseRolloutDefinition(body)
		if definitionErr != nil {
			writeAPIError(w, 422, "invalid_rollout_definition", "deployment rollout definition is invalid")
			return
		}
		value, err := store.CreatePromotion(deployments.Promotion{RepositoryID: r.PathValue("id"), EnvironmentID: input.EnvironmentID, ReleaseID: input.ReleaseID, BuildID: input.BuildID, ArtifactID: input.ArtifactID, ArtifactSHA256: artifact.SHA256, CommitID: candidate.CommitID, Rollout: rollout, InitiatedBy: actor.UserID})
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
	mux.HandleFunc("POST /repositories/{id}/deployments/{deployment_id}/controls", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repositories, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var input struct {
			Action        string `json:"action"`
			ExpectedState string `json:"expected_state"`
			Reason        string `json:"reason"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		value, err := store.Control(r.PathValue("id"), r.PathValue("deployment_id"), actor.UserID, input.Action, input.ExpectedState, input.Reason)
		if errors.Is(err, deployments.ErrBlocked) {
			writeAPIError(w, 409, "deployment_control_blocked", "deployment state changed or the requested control is unavailable")
			return
		}
		if errors.Is(err, deployments.ErrNotFound) {
			writeAPIError(w, 404, "deployment_not_found", "deployment not found")
			return
		}
		if err != nil {
			writeAPIError(w, 422, "invalid_deployment_control", "control action or reason is invalid")
			return
		}
		target := value.InitiatedBy
		recordActivity(activityStore, repositories, activities.Event{Kind: "deployment." + input.Action, ActorID: actor.UserID, RepositoryID: value.RepositoryID, ResourceType: "deployment", ResourceID: value.ID, ResourceTitle: "Deployment to environment " + value.EnvironmentID, TargetUserID: &target})
		writeJSON(w, 200, value)
	})
	mux.HandleFunc("POST /repositories/{id}/deployments/{deployment_id}/recoveries", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repositories, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var input struct {
			Action        string `json:"action"`
			ExpectedState string `json:"expected_state"`
		}
		if decodeJSON(r, &input) != nil || (input.Action != "rollback" && input.Action != "repair") {
			writeAPIError(w, 400, "invalid_recovery", "recovery action must be rollback or repair")
			return
		}
		failed, err := store.GetPromotion(r.PathValue("id"), r.PathValue("deployment_id"))
		if errors.Is(err, deployments.ErrNotFound) {
			writeAPIError(w, 404, "deployment_not_found", "deployment not found")
			return
		}
		if err != nil {
			writeAPIError(w, 500, "deployment_read_failed", "deployment could not be read")
			return
		}
		if input.ExpectedState != "" && input.ExpectedState != failed.State {
			writeAPIError(w, 409, "recovery_state_changed", "deployment state changed before recovery began")
			return
		}
		if failed.State != "failed" && failed.State != "canceled" {
			writeAPIError(w, 409, "deployment_not_unhealthy", "recovery requires a failed or canceled deployment")
			return
		}
		if input.Action == "rollback" {
			value, target, createErr := store.CreateRollback(failed.RepositoryID, failed.ID, actor.UserID)
			if errors.Is(createErr, deployments.ErrRecoveryStateChanged) {
				writeAPIError(w, 409, "recovery_state_changed", "deployment state changed before rollback publication")
				return
			}
			if errors.Is(createErr, deployments.ErrBlocked) {
				if target.ID == "" {
					writeAPIError(w, 409, "rollback_unavailable", "no earlier successful artifact exists for this environment")
				} else {
					writeAPIError(w, 409, "rollback_blocked", "the recovery environment is busy or its governed promotion prerequisites are no longer satisfied")
				}
				return
			}
			if createErr != nil {
				writeAPIError(w, 500, "deployment_read_failed", "rollback evidence could not be read")
				return
			}
			recordActivity(activityStore, repositories, activities.Event{Kind: "deployment.rollback_requested", ActorID: actor.UserID, RepositoryID: failed.RepositoryID, ResourceType: "deployment", ResourceID: value.ID, ResourceTitle: "Rollback for deployment " + failed.ID})
			writeJSON(w, 202, map[string]any{"deployment": value, "restores_deployment": target.ID})
			if value.State == "queued" {
				go executor.Execute(value.RepositoryID, value.ID)
			}
			return
		}
		if pulls == nil || sessions == nil {
			writeAPIError(w, 503, "repair_unavailable", "repair sessions are not configured")
			return
		}
		repositoryRecord, repoErr := repositories.GetByID(failed.RepositoryID)
		if repoErr != nil {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return
		}
		repository, openErr := gitStore.Open(failed.RepositoryID)
		if openErr != nil {
			writeAPIError(w, 500, "repair_unavailable", "repository storage is unavailable")
			return
		}
		release, releaseErr := releases.Get(failed.RepositoryID, failed.ReleaseID)
		if releaseErr != nil {
			writeAPIError(w, 500, "repair_session_failed", "release context could not be loaded")
			return
		}
		branch := "agent/recovery/" + failed.ID
		ref := "refs/heads/" + branch
		var pull pullrequests.PullRequest
		pullUncertain, newPull := false, false
		existingPulls, listErr := pulls.List(failed.RepositoryID)
		if listErr != nil {
			writeAPIError(w, 500, "repair_unavailable", "repair pull requests could not be inspected")
			return
		}
		for _, candidate := range existingPulls {
			if candidate.SourceRepositoryID == failed.RepositoryID && candidate.SourceBranch == branch {
				pull = candidate
				break
			}
		}
		if pull.ID == "" {
			base, readErr := repository.ReadReference("refs/heads/" + repositoryRecord.DefaultBranch)
			if readErr != nil || base.Symbolic {
				writeAPIError(w, 500, "repair_unavailable", "default branch revision could not be resolved")
				return
			}
			if err = repository.CreateReference(storage.Reference{Name: ref, Target: base.Target}); err != nil && !errors.Is(err, storage.ErrReferenceExists) {
				writeAPIError(w, 500, "repair_unavailable", "isolated repair branch could not be created")
				return
			}
			if errors.Is(err, storage.ErrReferenceExists) {
				existing, existingErr := repository.ReadReference(ref)
				if existingErr != nil || existing.Symbolic {
					writeAPIError(w, 409, "repair_branch_changed", "the unpublished repair branch could not be safely reconciled")
					return
				}
				if existing.Target != base.Target {
					ancestry, ancestryErr := repository.ListCommitAncestry(storage.ObjectID(base.Target))
					ancestor := false
					for _, commit := range ancestry {
						ancestor = ancestor || string(commit.ID) == existing.Target
					}
					if ancestryErr != nil || !ancestor {
						writeAPIError(w, 409, "repair_branch_changed", "the unpublished repair branch diverged from the default branch")
						return
					}
					if updateErr := repository.UpdateReferenceIfTarget(storage.Reference{Name: ref, Target: base.Target}, existing.Target); updateErr != nil {
						writeAPIError(w, 409, "repair_branch_changed", "the unpublished repair branch changed during recovery")
						return
					}
				}
			}
			body := "Diagnose and repair unhealthy deployment " + failed.ID + ". The attached change session freezes release, deployment, log, health, artifact, and source evidence. This branch has no environment authority."
			pull, err = pulls.Create(failed.RepositoryID, actor.UserID, "Repair deployment for release "+failed.ReleaseID, body, branch, repositoryRecord.DefaultBranch, nil)
			pullUncertain = errors.Is(err, pullrequests.ErrDurabilityUncertain)
			if err != nil && !pullUncertain {
				writeAPIError(w, 500, "repair_unavailable", "repair pull request could not be created")
				return
			}
			newPull = true
			startCheckRuns(gitStore, builds, pull)
		}
		evidence := &changesessions.DeploymentEvidence{DeploymentID: failed.ID, ReleaseID: failed.ReleaseID, ReleaseVersion: release.Version, ReleaseNotes: release.Notes, EnvironmentID: failed.EnvironmentID, ArtifactID: failed.ArtifactID, ArtifactSHA256: failed.ArtifactSHA256, CommitID: failed.CommitID, State: failed.State, CurrentStage: failed.CurrentStage, Evidence: make([]changesessions.DeploymentSignal, 0, len(failed.Evidence)), Events: make([]changesessions.DeploymentEvent, 0, len(failed.Events))}
		for _, item := range failed.Evidence {
			evidence.Evidence = append(evidence.Evidence, changesessions.DeploymentSignal{Stage: item.Stage, Signal: item.Signal, State: item.State, Message: item.Message})
		}
		for _, item := range failed.Events {
			evidence.Events = append(evidence.Events, changesessions.DeploymentEvent{Sequence: item.Sequence, Kind: item.Kind, ActorID: item.ActorID, State: item.State, Message: item.Message})
		}
		existingSessions, sessionListErr := sessions.List(failed.RepositoryID, pull.ID)
		if sessionListErr != nil {
			writeAPIError(w, 500, "repair_session_failed", "repair session could not be inspected for retry")
			return
		}
		var session changesessions.Session
		var sessionErr error
		if len(existingSessions) > 0 {
			session = existingSessions[0]
		} else if pull.Status != pullrequests.Open {
			writeAPIError(w, 409, "repair_pull_closed", "the existing repair pull request is closed")
			return
		} else {
			session, sessionErr = sessions.CreateWithRecoveryEvidence(failed.RepositoryID, pull.ID, actor.UserID, pull.SourceCommitID, nil, evidence)
		}
		if sessionErr != nil && !errors.Is(sessionErr, changesessions.ErrDurabilityUncertain) {
			// The pull remains a valid, visible review boundary even if the
			// workspace could not be published; do not erase durable review state.
			writeAPIError(w, 500, "repair_session_failed", "repair pull request was created but its change session could not be published")
			return
		}
		if len(existingSessions) == 0 {
			recordActivity(activityStore, repositories, activities.Event{Kind: "deployment.repair_opened", ActorID: actor.UserID, RepositoryID: failed.RepositoryID, ResourceType: "pull_request", ResourceID: pull.ID, ResourceTitle: pull.Title})
		}
		w.Header().Set("Location", "/repositories/"+failed.RepositoryID+"/pulls/"+pull.ID+"/sessions/"+session.ID)
		response := map[string]any{"pull_request": pull, "session": session}
		if pullUncertain || errors.Is(sessionErr, changesessions.ErrDurabilityUncertain) {
			writeUncertainMutation(w, response)
			return
		}
		if newPull || len(existingSessions) == 0 {
			writeJSON(w, 201, response)
		} else {
			writeJSON(w, 200, response)
		}
	})
}
