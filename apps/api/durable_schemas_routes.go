package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/changesessions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/decisions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/durableschemas"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
)

func registerDurableSchemaRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, store *durableschemas.Store, pulls *pullrequests.Store, decisionStore *decisions.Store, proposalStore *proposals.Store, sessionStore *changesessions.Store, workspaceStore *workspaces.Store, deploymentStore *deployments.Store, releaseStore *releases.Store) {
	base := "/repositories/{id}/durable-schemas"
	mux.HandleFunc("GET "+base, func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		v, e := store.List(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "durable_schemas_unavailable", "durable schemas could not be read")
			return
		}
		for i := range v {
			projectDurableMigrationWork(&v[i], actor.UserID, catalog, proposalStore, pulls, sessionStore, workspaceStore)
		}
		writeJSON(w, 200, map[string]any{"schemas": v})
	})
	mux.HandleFunc("GET "+base+"/{schema_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		v, e := store.Get(r.PathValue("id"), r.PathValue("schema_id"))
		if e != nil {
			writeAPIError(w, 404, "durable_schema_not_found", "durable schema not found")
			return
		}
		projectDurableMigrationWork(&v, actor.UserID, catalog, proposalStore, pulls, sessionStore, workspaceStore)
		writeJSON(w, 200, v)
	})
	publish := func(revise bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
			if !ok {
				return
			}
			var in struct {
				ExpectedVersion int                     `json:"expected_version"`
				Revision        durableschemas.Revision `json:"revision"`
			}
			if decodeJSON(r, &in) != nil {
				writeAPIError(w, 400, "invalid_request", "a complete reviewed schema revision is required")
				return
			}
			pr, e := pulls.Get(r.PathValue("id"), in.Revision.PullRequestID)
			if e != nil || pr.Status != pullrequests.Merged || pr.MergeCommitID == nil || *pr.MergeCommitID != in.Revision.ReviewedCommit {
				writeAPIError(w, 400, "invalid_reviewed_history", "schema revisions must cite the exact merge commit of a merged repository pull request")
				return
			}
			if !durableSchemaDefinitionResolves(git, r.PathValue("id"), in.Revision) {
				writeAPIError(w, 400, "invalid_reviewed_schema_definition", "definition_path must resolve to the exact submitted definition in the reviewed merge commit")
				return
			}
			owners := append([]string{}, in.Revision.OwnerIDs...)
			var out durableschemas.Schema
			e = catalog.WithCurrentParticipants(owners, r.PathValue("id"), func() error {
				if revise {
					out, e = store.Revise(r.PathValue("id"), r.PathValue("schema_id"), in.ExpectedVersion, actor.UserID, in.Revision)
				} else {
					out, e = store.Create(r.PathValue("id"), actor.UserID, in.Revision)
				}
				return e
			})
			writeDurableSchema(w, out, e, map[bool]int{false: 201, true: 200}[revise])
		}
	}
	mux.HandleFunc("POST "+base, publish(false))
	mux.HandleFunc("POST "+base+"/{schema_id}/revisions", publish(true))
	mux.HandleFunc("POST "+base+"/{schema_id}/migrations", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in durableschemas.Migration
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete migration plan is required")
			return
		}
		valid := false
		if in.SourceKind == "pull_request" {
			p, e := pulls.Get(r.PathValue("id"), in.SourceID)
			valid = e == nil && (p.Status == pullrequests.Open || p.Status == pullrequests.Merged)
		} else if in.SourceKind == "decision" && decisionStore != nil {
			d, e := decisionStore.Get(in.SourceID)
			if e == nil {
				for _, x := range d.Scope.AffectedResources {
					if x.RepositoryID == r.PathValue("id") {
						valid = true
						break
					}
				}
			}
		}
		if !valid {
			writeAPIError(w, 400, "invalid_migration_source", "migration source must be a visible repository pull request or affected decision")
			return
		}
		owners := []string{}
		for _, o := range in.Operations {
			owners = append(owners, o.OwnerIDs...)
		}
		for _, s := range in.Steps {
			owners = append(owners, s.RequiredApproverIDs...)
		}
		var out durableschemas.Schema
		e := catalog.WithCurrentParticipants(owners, r.PathValue("id"), func() error {
			var x error
			out, x = store.AddMigration(r.PathValue("id"), r.PathValue("schema_id"), actor.UserID, in)
			return x
		})
		writeDurableSchema(w, out, e, 201)
	})
	mux.HandleFunc("POST "+base+"/{schema_id}/migrations/{migration_id}/events", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int                  `json:"expected_version"`
			Event           durableschemas.Event `json:"event"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an attributable migration event is required")
			return
		}
		out, e := store.AddEvent(r.PathValue("id"), r.PathValue("schema_id"), r.PathValue("migration_id"), actor.UserID, in.ExpectedVersion, in.Event)
		writeDurableSchema(w, out, e, 200)
	})
	mux.HandleFunc("POST "+base+"/{schema_id}/migrations/{migration_id}/executions", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int                      `json:"expected_version"`
			Execution       durableschemas.Execution `json:"execution"`
		}
		if decodeJSON(r, &in) != nil || deploymentStore == nil || releaseStore == nil {
			writeAPIError(w, 400, "invalid_execution", "a governed production execution is required")
			return
		}
		if _, err := deploymentStore.GetEnvironment(r.PathValue("id"), in.Execution.EnvironmentID); err != nil {
			writeAPIError(w, 422, "invalid_execution_environment", "execution must use an established repository environment")
			return
		}
		if _, err := releaseStore.Get(r.PathValue("id"), in.Execution.ReleaseID); err != nil {
			writeAPIError(w, 422, "invalid_execution_release", "execution must freeze an existing exact release")
			return
		}
		out, execution, err := store.CreateExecution(r.PathValue("id"), r.PathValue("schema_id"), r.PathValue("migration_id"), actor.UserID, in.ExpectedVersion, in.Execution)
		if errors.Is(err, durableschemas.ErrConflict) {
			writeAPIError(w, 409, "migration_execution_changed", "migration execution changed or is already active")
			return
		}
		if errors.Is(err, durableschemas.ErrInvalid) {
			writeAPIError(w, 422, "migration_execution_not_ready", "all required approvals, passing rehearsal evidence, compatibility, privacy, rollback, and cost bounds are required")
			return
		}
		if err != nil {
			writeAPIError(w, 404, "migration_not_found", "migration not found")
			return
		}
		writeJSON(w, 201, map[string]any{"schema": out, "execution": execution})
	})
	mux.HandleFunc("POST "+base+"/{schema_id}/migrations/{migration_id}/executions/{execution_id}/controls", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in struct {
			Action          string   `json:"action"`
			ExpectedVersion int      `json:"expected_version"`
			Phase           string   `json:"phase"`
			ProgressPercent int      `json:"progress_percent"`
			LagSeconds      int64    `json:"lag_seconds"`
			Invariants      []string `json:"invariants"`
			ServiceHealth   string   `json:"service_health"`
			Blockers        []string `json:"blockers"`
			NextActions     []string `json:"next_actions"`
			CostUnits       int64    `json:"cost_units"`
			ThrottlePercent int      `json:"throttle_percent"`
			Summary         string   `json:"summary"`
			AgentID         string   `json:"agent_id"`
			DeploymentID    string   `json:"deployment_id"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_execution_control", "a bounded execution control is required")
			return
		}
		if in.AgentID != "" && in.AgentID != actor.AgentID {
			writeAPIError(w, 403, "agent_attribution_forbidden", "agent attribution must come from the authenticated agent credential")
			return
		}
		if in.DeploymentID != "" {
			if deploymentStore == nil {
				writeAPIError(w, 422, "deployment_evidence_unavailable", "deployment evidence cannot be verified")
				return
			}
			d, err := deploymentStore.GetPromotion(r.PathValue("id"), in.DeploymentID)
			if err != nil {
				writeAPIError(w, 422, "invalid_deployment_evidence", "deployment evidence does not resolve")
				return
			}
			schema, err := store.Get(r.PathValue("id"), r.PathValue("schema_id"))
			if err != nil {
				writeAPIError(w, 404, "migration_not_found", "migration not found")
				return
			}
			valid := false
			for _, m := range schema.Migrations {
				if m.ID == r.PathValue("migration_id") {
					for _, x := range m.Executions {
						if x.ID == r.PathValue("execution_id") && x.EnvironmentID == d.EnvironmentID && x.ReleaseID == d.ReleaseID && d.State == "succeeded" {
							valid = true
						}
					}
				}
			}
			if !valid {
				writeAPIError(w, 422, "invalid_deployment_evidence", "deployment must be the successful frozen release in the frozen environment")
				return
			}
		}
		out, execution, err := store.UpdateExecution(r.PathValue("id"), r.PathValue("schema_id"), r.PathValue("migration_id"), r.PathValue("execution_id"), actor.UserID, durableschemas.ExecutionUpdate{Action: in.Action, ExpectedVersion: in.ExpectedVersion, Phase: in.Phase, ProgressPercent: in.ProgressPercent, LagSeconds: in.LagSeconds, Invariants: in.Invariants, ServiceHealth: in.ServiceHealth, Blockers: in.Blockers, NextActions: in.NextActions, CostUnits: in.CostUnits, ThrottlePercent: in.ThrottlePercent, Summary: in.Summary, AgentID: actor.AgentID, DeploymentID: in.DeploymentID})
		if errors.Is(err, durableschemas.ErrConflict) {
			writeAPIError(w, 409, "migration_execution_changed", "execution changed; reload before intervening")
			return
		}
		if errors.Is(err, durableschemas.ErrInvalid) {
			writeAPIError(w, 422, "execution_control_blocked", "control is unavailable, unsafe, over budget, or outside an agent's explicit delegation")
			return
		}
		if err != nil {
			writeAPIError(w, 404, "migration_execution_not_found", "migration execution not found")
			return
		}
		writeJSON(w, 200, map[string]any{"schema": out, "execution": execution})
	})
	mux.HandleFunc("POST "+base+"/{schema_id}/migrations/{migration_id}/work", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion    int                         `json:"expected_version"`
			Kind               string                      `json:"kind"`
			StepID             string                      `json:"step_id"`
			RepositoryID       string                      `json:"repository_id"`
			Title              string                      `json:"title"`
			CompletionCriteria string                      `json:"completion_criteria"`
			AssigneeType       string                      `json:"assignee_type"`
			AssigneeID         string                      `json:"assignee_id"`
			Mandate            string                      `json:"mandate"`
			BaseRevision       string                      `json:"base_revision"`
			DependencyIDs      []string                    `json:"dependency_ids"`
			Contract           durableschemas.WorkContract `json:"contract"`
		}
		if decodeJSON(r, &in) != nil || proposalStore == nil || (in.AssigneeType != "human" && in.AssigneeType != "agent") {
			writeAPIError(w, 400, "invalid_migration_work", "ordered migration work, assignment, exact base, and compatibility contract are required")
			return
		}
		target, err := catalog.GetByID(in.RepositoryID)
		collaborator, _ := catalog.HasCollaborator(actor.UserID, in.RepositoryID)
		if err != nil || (target.OwnerID != actor.UserID && !collaborator) {
			writeAPIError(w, 403, "migration_work_forbidden", "a current target-repository participant must create migration work")
			return
		}
		if strings.TrimSpace(in.Title) == "" || strings.ContainsAny(in.Title, "\r\n") || len([]rune(in.Title)) > 200 || strings.TrimSpace(in.CompletionCriteria) == "" || len([]rune(in.CompletionCriteria)) > 2000 || strings.TrimSpace(in.Mandate) == "" || len([]rune(in.Mandate)) > 4000 {
			writeAPIError(w, 422, "invalid_migration_work", "title, completion criteria, and mandate are required and bounded")
			return
		}
		repository, err := git.Open(in.RepositoryID)
		if err != nil {
			writeAPIError(w, 422, "invalid_migration_repository", "target repository storage is unavailable")
			return
		}
		if _, err = repository.ReadCommit(storage.ObjectID(strings.ToLower(in.BaseRevision))); err != nil {
			writeAPIError(w, 422, "invalid_base_revision", "base revision must be an existing target-repository commit")
			return
		}
		if in.AssigneeType == "human" {
			participant, _ := catalog.HasCollaborator(in.AssigneeID, in.RepositoryID)
			if in.AssigneeID != target.OwnerID && !participant {
				writeAPIError(w, 422, "invalid_task_assignee", "human assignee must already participate in the target repository")
				return
			}
		}
		work := durableschemas.MigrationWork{Kind: in.Kind, StepID: in.StepID, RepositoryID: in.RepositoryID, DependencyIDs: in.DependencyIDs, Contract: in.Contract}
		var proposal proposals.Proposal
		var assigned proposals.Task
		out, link, err := store.CreateMigrationWork(r.PathValue("id"), r.PathValue("schema_id"), r.PathValue("migration_id"), actor.UserID, in.ExpectedVersion, work, func() (string, string, error) {
			body := migrationWorkBody(r.PathValue("schema_id"), r.PathValue("migration_id"), work)
			var publishErr error
			proposal, publishErr = proposalStore.Create(in.RepositoryID, actor.UserID, in.Title, body)
			if publishErr != nil && !errors.Is(publishErr, proposals.ErrDurabilityUncertain) {
				return "", "", publishErr
			}
			task, taskErr := proposalStore.CreateTask(in.RepositoryID, proposal.ID, actor.UserID, in.Title, in.CompletionCriteria, nil, nil)
			if taskErr != nil && !errors.Is(taskErr, proposals.ErrDurabilityUncertain) {
				_ = proposalStore.DeleteMigrationWork(in.RepositoryID, proposal.ID, "", "")
				return "", "", taskErr
			}
			assigned, publishErr = proposalStore.AssignTask(in.RepositoryID, proposal.ID, task.ID, actor.UserID, proposals.TaskAssignmentInput{AssigneeType: in.AssigneeType, AssigneeID: in.AssigneeID, Mandate: in.Mandate, RepositoryID: in.RepositoryID, BaseRevision: in.BaseRevision})
			if publishErr != nil && !errors.Is(publishErr, proposals.ErrDurabilityUncertain) {
				_ = proposalStore.DeleteMigrationWork(in.RepositoryID, proposal.ID, task.ID, "")
				return "", "", publishErr
			}
			return proposal.ID, task.ID, nil
		})
		if errors.Is(err, durableschemas.ErrConflict) {
			writeAPIError(w, 409, "migration_changed", "migration plan changed; reload before adding work")
			return
		}
		if err != nil {
			if proposal.ID != "" && assigned.Assignment != nil {
				_ = proposalStore.DeleteMigrationWork(in.RepositoryID, proposal.ID, assigned.ID, assigned.Assignment.ID)
			}
			writeDurableSchema(w, out, err, 201)
			return
		}
		projectDurableMigrationWork(&out, actor.UserID, catalog, proposalStore, pulls, sessionStore, workspaceStore)
		writeJSON(w, 201, map[string]any{"schema": out, "migration_work": link, "task": assigned})
	})
	mux.HandleFunc("POST "+base+"/{schema_id}/migrations/{migration_id}/rehearsals", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int                      `json:"expected_version"`
			Rehearsal       durableschemas.Rehearsal `json:"rehearsal"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a bounded rehearsal plan is required")
			return
		}
		repository, err := git.Open(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 422, "invalid_application_revision", "repository storage is unavailable")
			return
		}
		if _, err = repository.ReadCommit(storage.ObjectID(strings.ToLower(in.Rehearsal.ApplicationRevision))); err != nil {
			writeAPIError(w, 422, "invalid_application_revision", "application_revision must be an exact repository commit")
			return
		}
		out, rehearsal, err := store.CreateRehearsal(r.PathValue("id"), r.PathValue("schema_id"), r.PathValue("migration_id"), actor.UserID, in.ExpectedVersion, in.Rehearsal)
		if err != nil {
			writeDurableSchema(w, out, err, 201)
			return
		}
		writeJSON(w, 201, map[string]any{"schema": out, "rehearsal": rehearsal})
	})
	mux.HandleFunc("POST "+base+"/{schema_id}/migrations/{migration_id}/rehearsals/{rehearsal_id}/runs", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in durableschemas.RehearsalRun
		if decodeJSON(r, &in) != nil || workspaceStore == nil {
			writeAPIError(w, 400, "invalid_rehearsal_run", "complete sanitized rehearsal evidence is required")
			return
		}
		if !safeRehearsalRunRequest(in) {
			writeAPIError(w, 422, "untrusted_rehearsal_evidence", "counts, artifacts, costs, and attestations must come from platform-retained evidence, not the request")
			return
		}
		ws, err := workspaceStore.Get(in.WorkspaceID)
		if err != nil || ws.RepositoryID != r.PathValue("id") || ws.CreatorID != actor.UserID {
			writeAPIError(w, 422, "invalid_rehearsal_workspace", "run evidence must come from the caller's bounded workspace in this repository")
			return
		}
		schema, err := store.Get(r.PathValue("id"), r.PathValue("schema_id"))
		var rehearsal *durableschemas.Rehearsal
		for mi := range schema.Migrations {
			if schema.Migrations[mi].ID == r.PathValue("migration_id") {
				for ri := range schema.Migrations[mi].Rehearsals {
					if schema.Migrations[mi].Rehearsals[ri].ID == r.PathValue("rehearsal_id") {
						rehearsal = &schema.Migrations[mi].Rehearsals[ri]
					}
				}
			}
		}
		if err != nil || rehearsal == nil || ws.CommitID != rehearsal.ApplicationRevision {
			writeAPIError(w, 422, "invalid_rehearsal_workspace", "workspace must use the exact candidate and retain every repository-defined check outcome")
			return
		}
		in, ok = bindRehearsalRun(ws, *rehearsal, in)
		if !ok {
			writeAPIError(w, 422, "invalid_rehearsal_workspace", "each check must have one unambiguous, sanitized retained command outcome")
			return
		}
		out, run, err := store.AddRehearsalRun(r.PathValue("id"), r.PathValue("schema_id"), r.PathValue("migration_id"), r.PathValue("rehearsal_id"), actor.UserID, in)
		if err != nil {
			writeDurableSchema(w, out, err, 201)
			return
		}
		writeJSON(w, 201, map[string]any{"schema": out, "run": run})
	})
	mux.HandleFunc("POST "+base+"/{schema_id}/migrations/{migration_id}/rehearsals/{rehearsal_id}/notes", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in struct {
			RunID string `json:"run_id"`
			Body  string `json:"body"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a run and bounded investigation note are required")
			return
		}
		if reusableSecret.MatchString(in.Body) {
			writeAPIError(w, 422, "unsafe_rehearsal_evidence", "investigation notes must not contain credentials or secrets")
			return
		}
		out, n, err := store.AddRehearsalNote(r.PathValue("id"), r.PathValue("schema_id"), r.PathValue("migration_id"), r.PathValue("rehearsal_id"), actor.UserID, in.RunID, in.Body)
		if err != nil {
			writeDurableSchema(w, out, err, 201)
			return
		}
		writeJSON(w, 201, map[string]any{"schema": out, "note": n})
	})
}

func bindRehearsalRun(ws workspaces.Workspace, rehearsal durableschemas.Rehearsal, run durableschemas.RehearsalRun) (durableschemas.RehearsalRun, bool) {
	retained := map[string][]workspaces.CommandOutcome{}
	for _, outcome := range ws.Commands {
		retained[outcome.CommandSHA256] = append(retained[outcome.CommandSHA256], outcome)
	}
	checks := map[string]durableschemas.RehearsalCheck{}
	for _, check := range rehearsal.Checks {
		checks[check.ID] = check
	}
	allPassed := true
	attestations := make([]string, 0, len(run.Outcomes))
	for i := range run.Outcomes {
		check, exists := checks[run.Outcomes[i].CheckID]
		if !exists {
			return run, false
		}
		commandDigest := sha256.Sum256([]byte(check.Command))
		invariantDigest := sha256.Sum256([]byte(check.InvariantCommand))
		matches := retained[hex.EncodeToString(commandDigest[:])]
		invariantMatches := retained[hex.EncodeToString(invariantDigest[:])]
		if len(matches) != 1 || len(invariantMatches) != 1 || !rehearsalCommandCurrent(matches[0], rehearsal.CreatedAt) || !rehearsalCommandCurrent(invariantMatches[0], rehearsal.CreatedAt) || len(matches[0].Output)+len(invariantMatches[0].Output) > 65500 || reusableSecret.MatchString(matches[0].Output) || reusableSecret.MatchString(invariantMatches[0].Output) {
			return run, false
		}
		evidence := matches[0]
		invariantEvidence := invariantMatches[0]
		invariantPassed := invariantEvidence.ExitCode == 0
		passed := evidence.ExitCode == 0 && invariantPassed
		run.Outcomes[i].ExitCode = evidence.ExitCode
		run.Outcomes[i].Status = map[bool]string{true: "passed", false: "failed"}[passed]
		run.Outcomes[i].InvariantPassed = invariantPassed
		run.Outcomes[i].SanitizedLog = "command:\n" + evidence.Output + "\ninvariant:\n" + invariantEvidence.Output
		run.Outcomes[i].DurationMS = evidence.CompletedAt.Sub(evidence.StartedAt).Milliseconds() + invariantEvidence.CompletedAt.Sub(invariantEvidence.StartedAt).Milliseconds()
		if run.Outcomes[i].DurationMS < 0 {
			return run, false
		}
		allPassed = allPassed && passed
		attestations = append(attestations, "workspace:"+ws.ID+" command_outcome:"+evidence.ID+" command_sha256:"+evidence.CommandSHA256)
		attestations = append(attestations, "workspace:"+ws.ID+" invariant_outcome:"+invariantEvidence.ID+" command_sha256:"+invariantEvidence.CommandSHA256)
	}
	run.Result = map[bool]string{true: "passed", false: "failed"}[allPassed]
	run.Attestations = attestations
	return run, true
}

func rehearsalCommandCurrent(outcome workspaces.CommandOutcome, createdAt time.Time) bool {
	return !outcome.StartedAt.Before(createdAt) && !outcome.CompletedAt.Before(createdAt) && !outcome.CompletedAt.Before(outcome.StartedAt)
}

func safeRehearsalRunRequest(run durableschemas.RehearsalRun) bool {
	if len(run.Attestations) != 0 {
		return false
	}
	for _, outcome := range run.Outcomes {
		if outcome.RowsBefore != 0 || outcome.RowsAfter != 0 || outcome.ObjectsBefore != 0 || outcome.ObjectsAfter != 0 || outcome.CostUnits != 0 || len(outcome.ArtifactDigests) != 0 {
			return false
		}
	}
	return true
}

func migrationWorkBody(schemaID, migrationID string, w durableschemas.MigrationWork) string {
	c := w.Contract
	return "Durable-state migration " + migrationID + " for schema " + schemaID + ".\n\nWork kind: " + w.Kind + "\nStep: " + w.StepID + "\n\nCompatibility contract\nOld readers: " + strings.Join(c.OldReaders, "; ") + "\nNew readers: " + strings.Join(c.NewReaders, "; ") + "\nOld writers: " + strings.Join(c.OldWriters, "; ") + "\nNew writers: " + strings.Join(c.NewWriters, "; ") + "\nRollout flags: " + strings.Join(c.RolloutFlags, "; ") + "\nIdempotency: " + c.Idempotency + "\nTransformations: " + strings.Join(c.Transformations, "; ") + "\nOwnership: " + strings.Join(c.Ownership, "; ") + "\nRollback assumptions: " + strings.Join(c.RollbackAssumptions, "; ") + "\n\nThis context grants no repository, agent, review, merge, deployment, or data-store authority."
}

func projectDurableMigrationWork(schema *durableschemas.Schema, actorID string, catalog *repositories.Store, proposalStore *proposals.Store, pulls *pullrequests.Store, sessions *changesessions.Store, workspaceStore *workspaces.Store) {
	var actorWorkspaces []workspaces.Workspace
	if workspaceStore != nil && actorID != "" {
		actorWorkspaces, _ = workspaceStore.List(actorID)
	}
	for mi := range schema.Migrations {
		completed := map[string]bool{}
		visible := schema.Migrations[mi].Work[:0]
		for _, work := range schema.Migrations[mi].Work {
			repo, err := catalog.GetByID(work.RepositoryID)
			collaborator, _ := catalog.HasCollaborator(actorID, work.RepositoryID)
			if err != nil || (repo.Visibility != repositories.Public && repo.OwnerID != actorID && !collaborator) {
				continue
			}
			if task, err := proposalStore.GetTask(work.RepositoryID, work.ProposalID, work.TaskID); err == nil {
				work.Status = task.Status
				completed[work.ID] = task.Status == proposals.TaskCompleted
				if task.Assignment != nil {
					work.AssignmentID = task.Assignment.ID
					work.AssigneeType = task.Assignment.AssigneeType
					work.AssigneeID = task.Assignment.AssigneeID
					work.BaseRevision = task.Assignment.Access.BaseRevision
				}
				if task.Contribution != nil {
					work.PullRequestID = task.Contribution.PullRequestID
					work.ContributionStatus = task.Contribution.Status
				}
			}
			if sessions != nil {
				if list, err := sessions.List(work.RepositoryID, work.TaskID); err == nil && len(list) > 0 {
					work.SessionID = list[len(list)-1].ID
				}
			}
			for _, workspace := range actorWorkspaces {
				if workspace.RepositoryID == work.RepositoryID && workspace.Source.Kind == "proposal_task" && workspace.Source.ProposalID == work.ProposalID && workspace.Source.TaskID == work.TaskID {
					work.WorkspaceID = workspace.ID
					break
				}
			}
			visible = append(visible, work)
		}
		for i := range visible {
			ready := visible[i].Status == proposals.TaskTodo
			for _, dep := range visible[i].DependencyIDs {
				ready = ready && completed[dep]
			}
			visible[i].Ready = ready
		}
		schema.Migrations[mi].Work = visible
	}
}

func durableSchemaDefinitionResolves(git *storage.Store, repositoryID string, revision durableschemas.Revision) bool {
	candidate := revision.DefinitionPath
	if candidate == "" || strings.HasPrefix(candidate, "/") || path.Clean(candidate) != candidate || candidate == "." || strings.HasPrefix(candidate, "../") {
		return false
	}
	repository, err := git.Open(repositoryID)
	if err != nil {
		return false
	}
	commit, err := repository.ReadCommit(storage.ObjectID(revision.ReviewedCommit))
	if err != nil {
		return false
	}
	entries, err := repository.WalkTree(commit.Tree)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.Path != candidate || entry.Type != storage.BlobObject {
			continue
		}
		definition, readErr := repository.ReadObject(entry.ID)
		return readErr == nil && definition.Type == storage.BlobObject && string(definition.Content) == revision.Definition
	}
	return false
}
func writeDurableSchema(w http.ResponseWriter, v durableschemas.Schema, e error, status int) {
	switch {
	case e == nil:
		writeJSON(w, status, v)
	case errors.Is(e, durableschemas.ErrConflict):
		writeAPIError(w, 409, "durable_schema_conflict", "the durable-state record changed; reload before writing")
	case errors.Is(e, durableschemas.ErrInvalid):
		writeAPIError(w, 400, "invalid_durable_schema", "define reviewed schema provenance, owners, compatibility, retention, privacy, and a completely sequenced migration")
	case errors.Is(e, repositories.ErrInvalidCollaborator), errors.Is(e, repositories.ErrNotFound):
		writeAPIError(w, 403, "durable_schema_forbidden", "all owners and approvers must be current repository participants")
	case errors.Is(e, durableschemas.ErrNotFound):
		writeAPIError(w, 404, "durable_schema_not_found", "durable schema not found")
	default:
		log.Printf("durable schema storage: %v", e)
		writeAPIError(w, 500, "durable_schemas_unavailable", "durable schema could not be persisted")
	}
}
