package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/infrastructure"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
)

type infrastructureInput struct {
	ExpectedVersion int                     `json:"expected_version"`
	Revision        infrastructure.Revision `json:"revision"`
}

func registerInfrastructureRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, definitions *infrastructure.Store, pulls *pullrequests.Store, releaseStore *releases.Store, deploymentStore *deployments.Store, workspaceStore *workspaces.Store) {
	mux.HandleFunc("GET /repositories/{id}/infrastructure", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		values, err := definitions.List(r.PathValue("id"), infrastructureParticipant(catalog, r.PathValue("id"), actor, authenticated))
		if err != nil {
			writeAPIError(w, 500, "infrastructure_unavailable", "infrastructure definitions could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"definitions": values})
	})
	mux.HandleFunc("GET /repositories/{id}/infrastructure/{definition_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		out, err := definitions.Get(r.PathValue("definition_id"), infrastructureParticipant(catalog, r.PathValue("id"), actor, authenticated))
		if err != nil || out.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "infrastructure_not_found", "infrastructure definition not found")
			return
		}
		writeJSON(w, 200, out)
	})
	mux.HandleFunc("POST /repositories/{id}/infrastructure", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in infrastructureInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete infrastructure revision is required")
			return
		}
		var out infrastructure.Definition
		err := catalog.WithCurrentParticipants(infrastructureOwners(actor.UserID, in.Revision), r.PathValue("id"), func() error {
			if !infrastructureRevisionResolves(git, r.PathValue("id"), in.Revision, releaseStore, deploymentStore) {
				return infrastructure.ErrInvalid
			}
			var e error
			out, e = definitions.Create(r.PathValue("id"), actor.UserID, in.Revision)
			return e
		})
		writeInfrastructure(w, out, err, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/infrastructure/{definition_id}/revisions", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		current, err := definitions.Get(r.PathValue("definition_id"), true)
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "infrastructure_not_found", "infrastructure definition not found")
			return
		}
		var in infrastructureInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "expected_version and a complete infrastructure revision are required")
			return
		}
		var out infrastructure.Definition
		err = catalog.WithCurrentParticipants(infrastructureOwners(actor.UserID, in.Revision), current.RepositoryID, func() error {
			if !infrastructureRevisionResolves(git, current.RepositoryID, in.Revision, releaseStore, deploymentStore) {
				return infrastructure.ErrInvalid
			}
			var e error
			out, e = definitions.Revise(current.ID, in.ExpectedVersion, actor.UserID, in.Revision)
			return e
		})
		writeInfrastructure(w, out, err, 200)
	})
	mux.HandleFunc("POST /repositories/{id}/infrastructure/{definition_id}/observations", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		current, err := definitions.Get(r.PathValue("definition_id"), true)
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "infrastructure_not_found", "infrastructure definition not found")
			return
		}
		var in infrastructure.Observation
		if decodeJSON(r, &in) != nil || !infrastructureLinksResolve(r.PathValue("id"), in.EnvironmentID, in.ReleaseID, releaseStore, deploymentStore) {
			writeAPIError(w, 400, "invalid_infrastructure_observation", "observation must be sanitized and bind exact definition, provider, environment, and release revisions")
			return
		}
		out, err := definitions.Observe(current.ID, actor.UserID, in)
		writeInfrastructure(w, out, err, 201)
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/infrastructure-plans", func(w http.ResponseWriter, r *http.Request) {
		_, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		pull, err := pulls.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if err != nil {
			writeAPIError(w, 404, "pull_request_not_found", "pull request not found")
			return
		}
		items, err := definitions.ListPlans(pull.RepositoryID, pull.ID)
		if err != nil {
			writeAPIError(w, 500, "infrastructure_plan_unavailable", "infrastructure plans could not be read")
			return
		}
		projected := make([]infrastructure.ChangePlan, 0, len(items))
		for _, p := range items {
			current, e := definitions.Get(p.DefinitionID, true)
			if e != nil {
				current = infrastructure.Definition{}
			}
			projected = append(projected, definitions.ProjectPlan(p, current, pull.SourceCommitID, func(x infrastructure.PolicyEffect) bool {
				return infrastructurePolicyCurrent(git, pull.SourceRepositoryID, p.SourceRevision, x)
			}))
		}
		writeJSON(w, 200, map[string]any{"plans": projected})
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/infrastructure-plans", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in struct {
			DefinitionID    string `json:"definition_id"`
			SourceRevision  string `json:"source_revision"`
			CandidateSource struct {
				Path   string `json:"path"`
				Digest string `json:"digest"`
			} `json:"candidate_source"`
			PolicyEffects []infrastructure.PolicyEffect `json:"policy_effects"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an exact infrastructure candidate and policy effects are required")
			return
		}
		current, err := definitions.Get(in.DefinitionID, true)
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "infrastructure_not_found", "infrastructure definition not found")
			return
		}
		pull, err := pulls.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if err != nil {
			writeAPIError(w, 404, "pull_request_not_found", "pull request not found")
			return
		}
		candidateBody, candidateDigest, ok := infrastructureCommitBlob(git, pull.SourceRepositoryID, in.SourceRevision, in.CandidateSource.Path)
		if !ok || candidateDigest != in.CandidateSource.Digest {
			writeAPIError(w, 422, "infrastructure_candidate_invalid", "the candidate declaration must be an exact JSON file in the pull source commit")
			return
		}
		var candidate infrastructure.Revision
		if json.Unmarshal(candidateBody, &candidate) != nil || candidate.Revision != in.SourceRevision {
			writeAPIError(w, 422, "infrastructure_candidate_invalid", "the candidate declaration must be an exact JSON file in the pull source commit")
			return
		}
		for _, p := range in.PolicyEffects {
			if !infrastructurePolicyCurrent(git, pull.SourceRepositoryID, in.SourceRevision, p) {
				writeAPIError(w, 422, "infrastructure_policy_invalid", "each policy effect must cite an exact candidate-revision file digest")
				return
			}
		}
		var out infrastructure.ChangePlan
		err = catalog.WithCurrentParticipants(infrastructureOwners(actor.UserID, candidate), r.PathValue("id"), func() error {
			return pulls.WithSourceRevision(r.PathValue("id"), r.PathValue("pull_id"), in.SourceRevision, func(p pullrequests.PullRequest) error {
				var e error
				out, e = definitions.CreatePlan(p.RepositoryID, actor.UserID, infrastructure.PlanCreation{PullRequestID: p.ID, Revision: p.SourceCommitID, Definition: current, Candidate: candidate, CandidatePath: in.CandidateSource.Path, CandidateDigest: candidateDigest, Policies: in.PolicyEffects})
				return e
			})
		})
		writeInfrastructurePlan(w, out, err, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/infrastructure-plans/{plan_id}/events", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok || !authenticated {
			return
		}
		if actor.AgentID == "" {
			if !infrastructureParticipant(catalog, r.PathValue("id"), actor, true) {
				writeAPIError(w, 403, "infrastructure_plan_forbidden", "only current participants and repository-bound read-only agents may contribute")
				return
			}
		}
		pull, err := pulls.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if err != nil {
			writeAPIError(w, 404, "pull_request_not_found", "pull request not found")
			return
		}
		plan, err := definitions.GetPlan(r.PathValue("plan_id"))
		if err != nil || plan.RepositoryID != pull.RepositoryID || plan.PullRequestID != pull.ID {
			writeAPIError(w, 404, "infrastructure_plan_not_found", "infrastructure plan not found")
			return
		}
		current, _ := definitions.Get(plan.DefinitionID, true)
		projected := definitions.ProjectPlan(plan, current, pull.SourceCommitID, func(x infrastructure.PolicyEffect) bool {
			return infrastructurePolicyCurrent(git, pull.SourceRepositoryID, plan.SourceRevision, x)
		})
		if !projected.Fresh {
			writeAPIError(w, 409, "infrastructure_plan_stale", "source, provider, policy, or observed state changed; create a new plan")
			return
		}
		var in struct {
			ExpectedEvents int                      `json:"expected_events"`
			Event          infrastructure.PlanEvent `json:"event"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an event and expected event count are required")
			return
		}
		actorID, actorType := actor.UserID, "human"
		if actor.AgentID != "" {
			actorID, actorType = actor.AgentID, "agent"
		}
		var out infrastructure.ChangePlan
		err = pulls.WithSourceRevision(pull.RepositoryID, pull.ID, plan.SourceRevision, func(guardedPull pullrequests.PullRequest) error {
			var appendErr error
			out, appendErr = definitions.AddPlanEventCurrent(plan.ID, actorID, actorType, in.ExpectedEvents, in.Event)
			if appendErr != nil {
				return appendErr
			}
			// AddPlanEventCurrent returns the projection derived from the exact
			// definition/observation snapshot held through its durable append.
			// The guarded pull value establishes the matching source revision.
			if out.SourceRevision != guardedPull.SourceCommitID {
				return pullrequests.ErrSourceChanged
			}
			return nil
		})
		writeInfrastructurePlan(w, out, err, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/infrastructure-plans/{plan_id}/rehearsals", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		pull, err := pulls.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if err != nil {
			writeAPIError(w, 404, "pull_request_not_found", "pull request not found")
			return
		}
		plan, err := definitions.GetPlan(r.PathValue("plan_id"))
		if err != nil || plan.RepositoryID != pull.RepositoryID || plan.PullRequestID != pull.ID {
			writeAPIError(w, 404, "infrastructure_plan_not_found", "infrastructure plan not found")
			return
		}
		current, _ := definitions.Get(plan.DefinitionID, true)
		if !definitions.ProjectPlan(plan, current, pull.SourceCommitID, func(x infrastructure.PolicyEffect) bool {
			return infrastructurePolicyCurrent(git, pull.SourceRepositoryID, plan.SourceRevision, x)
		}).Fresh {
			writeAPIError(w, 409, "infrastructure_plan_stale", "only a current exact plan can be rehearsed")
			return
		}
		var in infrastructure.Rehearsal
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a scoped ephemeral rehearsal and complete repository checks are required")
			return
		}
		var out infrastructure.ChangePlan
		var rehearsal infrastructure.Rehearsal
		err = pulls.WithSourceRevision(pull.RepositoryID, pull.ID, plan.SourceRevision, func(p pullrequests.PullRequest) error {
			var e error
			out, rehearsal, e = definitions.CreateRehearsalCurrent(plan.ID, actor.UserID, in)
			return e
		})
		if err != nil {
			writeInfrastructurePlan(w, out, err, 201)
			return
		}
		writeJSON(w, 201, map[string]any{"plan": out, "rehearsal": rehearsal})
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/infrastructure-plans/{plan_id}/rehearsals/{rehearsal_id}/runs", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if workspaceStore == nil {
			writeAPIError(w, 422, "infrastructure_rehearsal_unavailable", "bounded workspace evidence is unavailable")
			return
		}
		var request struct {
			WorkspaceID string   `json:"workspace_id"`
			CheckIDs    []string `json:"check_ids"`
		}
		if decodeJSON(r, &request) != nil {
			writeAPIError(w, 400, "invalid_request", "workspace_id and exact check_ids are required")
			return
		}
		pull, err := pulls.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if err != nil {
			writeAPIError(w, 404, "pull_request_not_found", "pull request not found")
			return
		}
		plan, err := definitions.GetPlan(r.PathValue("plan_id"))
		if err != nil || plan.RepositoryID != pull.RepositoryID || plan.PullRequestID != pull.ID {
			writeAPIError(w, 404, "infrastructure_plan_not_found", "infrastructure plan not found")
			return
		}
		current, _ := definitions.Get(plan.DefinitionID, true)
		if !definitions.ProjectPlan(plan, current, pull.SourceCommitID, func(x infrastructure.PolicyEffect) bool {
			return infrastructurePolicyCurrent(git, pull.SourceRepositoryID, plan.SourceRevision, x)
		}).Fresh {
			writeAPIError(w, 409, "infrastructure_plan_stale", "stale plans cannot receive rehearsal evidence")
			return
		}
		var rehearsal *infrastructure.Rehearsal
		for i := range plan.Rehearsals {
			if plan.Rehearsals[i].ID == r.PathValue("rehearsal_id") {
				rehearsal = &plan.Rehearsals[i]
			}
		}
		ws, wsErr := workspaceStore.Get(request.WorkspaceID)
		if rehearsal == nil || wsErr != nil || ws.RepositoryID != plan.RepositoryID || ws.CreatorID != actor.UserID || ws.CommitID != plan.SourceRevision || time.Now().UTC().After(rehearsal.Scope.CredentialExpiresAt) {
			writeAPIError(w, 422, "invalid_infrastructure_rehearsal_workspace", "evidence must come from the collaborator's exact-candidate workspace before scoped credentials expire")
			return
		}
		run, valid := bindInfrastructureRehearsal(ws, plan, *rehearsal, request.CheckIDs)
		if !valid {
			writeAPIError(w, 422, "invalid_infrastructure_rehearsal_evidence", "each declared check requires one fresh sanitized retained command outcome")
			return
		}
		var out infrastructure.ChangePlan
		err = pulls.WithSourceRevision(pull.RepositoryID, pull.ID, plan.SourceRevision, func(p pullrequests.PullRequest) error {
			var appendErr error
			out, run, appendErr = definitions.AddRehearsalRunCurrent(plan.ID, rehearsal.ID, actor.UserID, run)
			return appendErr
		})
		if err != nil {
			writeInfrastructurePlan(w, out, err, 201)
			return
		}
		writeJSON(w, 201, map[string]any{"plan": out, "run": run})
	})
	mux.HandleFunc("GET /repositories/{id}/infrastructure-executions", func(w http.ResponseWriter, r *http.Request) {
		_, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		items, err := definitions.ListExecutions(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "infrastructure_execution_unavailable", "infrastructure executions could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"executions": items})
	})
	mux.HandleFunc("GET /repositories/{id}/infrastructure-executions/{execution_id}", func(w http.ResponseWriter, r *http.Request) {
		_, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		x, err := definitions.GetExecution(r.PathValue("execution_id"))
		if err != nil || x.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "infrastructure_execution_not_found", "infrastructure execution not found")
			return
		}
		writeJSON(w, 200, x)
	})
	mux.HandleFunc("POST /repositories/{id}/infrastructure-executions", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in struct {
			PlanID              string                               `json:"plan_id"`
			EnvironmentID       string                               `json:"environment_id"`
			EnvironmentPolicy   string                               `json:"environment_policy"`
			RehearsalID         string                               `json:"rehearsal_id"`
			BudgetUnits         float64                              `json:"budget_units"`
			CredentialExpiresAt time.Time                            `json:"credential_expires_at"`
			Delegations         []infrastructure.ExecutionDelegation `json:"delegations"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an exact plan, environment policy, budget, passing rehearsal, and credential expiry are required")
			return
		}
		plan, err := definitions.GetPlan(in.PlanID)
		if err != nil || plan.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "infrastructure_plan_not_found", "infrastructure plan not found")
			return
		}
		pull, err := pulls.Get(r.PathValue("id"), plan.PullRequestID)
		if err != nil || pull.Status != pullrequests.Merged || pull.MergeCommitID == nil {
			writeAPIError(w, 409, "infrastructure_execution_blocked", "the exact reviewed pull must be merged before execution")
			return
		}
		current, err := definitions.Get(plan.DefinitionID, true)
		if err != nil || !definitions.ProjectPlan(plan, current, pull.SourceCommitID, func(x infrastructure.PolicyEffect) bool {
			return infrastructurePolicyCurrent(git, pull.SourceRepositoryID, plan.SourceRevision, x)
		}).Fresh {
			writeAPIError(w, 409, "infrastructure_execution_blocked", "the plan, observations, and policies must remain current")
			return
		}
		if _, err = deploymentStore.GetEnvironment(r.PathValue("id"), in.EnvironmentID); err != nil {
			writeAPIError(w, 422, "infrastructure_environment_invalid", "execution requires an established repository environment")
			return
		}
		for _, change := range plan.Changes {
			resource := change.After
			if resource == nil {
				resource = change.Before
			}
			if resource != nil && resource.EnvironmentID != "" && resource.EnvironmentID != in.EnvironmentID {
				writeAPIError(w, 409, "infrastructure_environment_authority_mismatch", "each execution may include only resources governed by its exact environment")
				return
			}
		}
		limit := 0.0
		for _, resource := range plan.Candidate.Resources {
			if resource.EnvironmentID != "" && resource.EnvironmentID != in.EnvironmentID {
				continue
			}
			for _, constraint := range resource.Constraints {
				if constraint.Kind == "cost" {
					limit += constraint.Limit
				}
			}
		}
		if in.BudgetUnits > limit {
			writeAPIError(w, 409, "infrastructure_budget_exceeded", "execution budget exceeds the reviewed resource cost limits")
			return
		}
		out, err := definitions.CreateExecution(plan, actor.UserID, infrastructure.ExecutionCreation{MergeCommitID: *pull.MergeCommitID, EnvironmentID: in.EnvironmentID, EnvironmentPolicy: in.EnvironmentPolicy, RehearsalID: in.RehearsalID, BudgetUnits: in.BudgetUnits, CredentialExpiry: in.CredentialExpiresAt, Delegations: in.Delegations})
		writeInfrastructureExecution(w, out, err, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/infrastructure-executions/{execution_id}/reports", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok || !authenticated {
			return
		}
		if actor.AgentID == "" && !infrastructureParticipant(catalog, r.PathValue("id"), actor, true) {
			writeAPIError(w, 403, "infrastructure_execution_forbidden", "only the controller or an exactly delegated agent may report")
			return
		}
		currentExecution, executionErr := definitions.GetExecution(r.PathValue("execution_id"))
		if executionErr != nil || currentExecution.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "infrastructure_execution_not_found", "infrastructure execution not found")
			return
		}
		var in struct {
			ExpectedVersion int                       `json:"expected_version"`
			StepID          string                    `json:"step_id"`
			Report          infrastructure.StepReport `json:"report"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an exact step report and expected version are required")
			return
		}
		actorID, actorType := actor.UserID, "human"
		if actor.AgentID != "" {
			actorID, actorType = actor.AgentID, "agent"
		}
		out, err := definitions.ReportExecution(r.PathValue("execution_id"), actorID, actorType, in.StepID, in.ExpectedVersion, in.Report)
		if out.RepositoryID != "" && out.RepositoryID != r.PathValue("id") {
			err = infrastructure.ErrExecutionNotFound
		}
		writeInfrastructureExecution(w, out, err, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/infrastructure-executions/{execution_id}/controls", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		currentExecution, executionErr := definitions.GetExecution(r.PathValue("execution_id"))
		if executionErr != nil || currentExecution.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "infrastructure_execution_not_found", "infrastructure execution not found")
			return
		}
		var in struct {
			ExpectedVersion int    `json:"expected_version"`
			Action          string `json:"action"`
			Summary         string `json:"summary"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an action, summary, and expected version are required")
			return
		}
		out, err := definitions.ControlExecution(r.PathValue("execution_id"), actor.UserID, in.Action, in.Summary, in.ExpectedVersion)
		if out.RepositoryID != "" && out.RepositoryID != r.PathValue("id") {
			err = infrastructure.ErrExecutionNotFound
		}
		writeInfrastructureExecution(w, out, err, 201)
	})
}

func writeInfrastructureExecution(w http.ResponseWriter, v infrastructure.Execution, err error, status int) {
	switch {
	case err == nil:
		writeJSON(w, status, v)
	case errors.Is(err, infrastructure.ErrExecutionNotFound):
		writeAPIError(w, 404, "infrastructure_execution_not_found", "infrastructure execution not found")
	case errors.Is(err, infrastructure.ErrExecutionBlocked), errors.Is(err, infrastructure.ErrConflict):
		writeAPIError(w, 409, "infrastructure_execution_blocked", "execution admission or transition requirements are not satisfied")
	case errors.Is(err, infrastructure.ErrInvalid):
		writeAPIError(w, 400, "invalid_infrastructure_execution", "execution evidence must be sanitized, dependency ordered, within budget, and within explicit authority")
	default:
		log.Printf("infrastructure execution storage: %v", err)
		writeAPIError(w, 500, "infrastructure_execution_unavailable", "infrastructure execution could not be persisted")
	}
}

func bindInfrastructureRehearsal(ws workspaces.Workspace, plan infrastructure.ChangePlan, rehearsal infrastructure.Rehearsal, checkIDs []string) (infrastructure.RehearsalRun, bool) {
	if len(checkIDs) != len(rehearsal.Checks) {
		return infrastructure.RehearsalRun{}, false
	}
	requested := map[string]bool{}
	for _, id := range checkIDs {
		if requested[id] {
			return infrastructure.RehearsalRun{}, false
		}
		requested[id] = true
	}
	retained := map[string][]workspaces.CommandOutcome{}
	for _, c := range ws.Commands {
		retained[c.CommandSHA256] = append(retained[c.CommandSHA256], c)
	}
	run := infrastructure.RehearsalRun{WorkspaceID: ws.ID, Outcomes: []infrastructure.RehearsalOutcome{}, Artifacts: []infrastructure.RehearsalArtifact{}, ResourceGraph: plan.Candidate.Resources, Attestations: []string{}, AgentActions: []string{}}
	passed := true
	for _, check := range rehearsal.Checks {
		if !requested[check.ID] {
			return run, false
		}
		digest := infrastructure.CommandDigest(check.Command)
		matches := retained[digest]
		var selected *workspaces.CommandOutcome
		for i := len(matches) - 1; i >= 0; i-- {
			x := matches[i]
			if x.StartedAt.Before(rehearsal.CreatedAt) || x.CompletedAt.Before(x.StartedAt) || len(x.Output) > 65500 || reusableSecret.MatchString(x.Output) || !infrastructure.SafeEvidence(x.Output) {
				continue
			}
			selected = &x
			break
		}
		if selected == nil {
			return run, false
		}
		x := *selected
		status := "passed"
		if x.ExitCode != 0 {
			status, passed = "failed", false
		}
		run.Outcomes = append(run.Outcomes, infrastructure.RehearsalOutcome{CheckID: check.ID, Kind: check.Kind, Status: status, ExitCode: x.ExitCode, SanitizedLog: x.Output, DurationMS: x.CompletedAt.Sub(x.StartedAt).Milliseconds(), StartedAt: x.StartedAt, CompletedAt: x.CompletedAt, CommandDigest: digest, ActorID: x.ActorID})
		run.Attestations = append(run.Attestations, "workspace:"+ws.ID+" command_outcome:"+x.ID+" command_sha256:"+digest)
		if x.ActorID != actorForWorkspace(ws) {
			run.AgentActions = append(run.AgentActions, "actor:"+x.ActorID+" command_outcome:"+x.ID)
		}
	}
	for _, change := range ws.Changes {
		if reusableSecret.MatchString(change.Path) || !infrastructure.SafeEvidence(change.Path) {
			return run, false
		}
		run.Artifacts = append(run.Artifacts, infrastructure.RehearsalArtifact{Path: change.Path, Digest: change.SHA256, Size: change.Size})
	}
	if passed {
		run.Result = "passed"
	} else {
		run.Result = "failed"
	}
	return run, true
}

func actorForWorkspace(ws workspaces.Workspace) string { return ws.CreatorID }

func infrastructurePolicyCurrent(git *storage.Store, repoID, revision string, policy infrastructure.PolicyEffect) bool {
	_, digest, ok := infrastructureCommitBlob(git, repoID, revision, policy.Path)
	return ok && digest == policy.Digest
}

func infrastructureCommitBlob(git *storage.Store, repoID, revision, sourcePath string) ([]byte, string, bool) {
	repo, err := git.Open(repoID)
	if err != nil {
		return nil, "", false
	}
	commit, err := repo.ReadCommit(storage.ObjectID(revision))
	if err != nil {
		return nil, "", false
	}
	entries, err := repo.WalkTree(commit.Tree)
	if err != nil {
		return nil, "", false
	}
	for _, entry := range entries {
		if entry.Path != sourcePath || entry.Type != storage.BlobObject {
			continue
		}
		blob, truncated, binary, err := repo.ReadBlobPreview(entry.ID, 1<<20)
		if err != nil || truncated || binary {
			return nil, "", false
		}
		sum := sha256.Sum256(blob.Content)
		return blob.Content, hex.EncodeToString(sum[:]), true
	}
	return nil, "", false
}

func writeInfrastructurePlan(w http.ResponseWriter, v infrastructure.ChangePlan, err error, status int) {
	switch {
	case err == nil:
		writeJSON(w, status, v)
	case errors.Is(err, infrastructure.ErrConflict):
		writeAPIError(w, 409, "infrastructure_plan_conflict", "the plan discussion changed; reload before contributing")
	case errors.Is(err, infrastructure.ErrPlanStale):
		writeAPIError(w, 409, "infrastructure_plan_stale", "source, provider, policy, or observed state changed; create a new plan")
	case errors.Is(err, infrastructure.ErrInvalid):
		writeAPIError(w, 400, "invalid_infrastructure_plan", "the plan must compare exact resources, policy effects, risks, dependencies, affected owners, and rollback limits")
	case errors.Is(err, pullrequests.ErrSourceChanged), errors.Is(err, pullrequests.ErrNotReady):
		writeAPIError(w, 409, "infrastructure_plan_stale", "the pull source changed or is no longer open")
	default:
		log.Printf("infrastructure plan storage: %v", err)
		writeAPIError(w, 500, "infrastructure_plan_unavailable", "infrastructure plan could not be persisted")
	}
}

func infrastructureParticipant(catalog *repositories.Store, repo string, actor auth.Credential, authenticated bool) bool {
	if !authenticated {
		return false
	}
	if actor.AgentID != "" {
		return true
	}
	v, e := catalog.GetByID(repo)
	if e != nil {
		return false
	}
	if v.OwnerID == actor.UserID {
		return true
	}
	ok, e := catalog.HasCollaborator(actor.UserID, repo)
	return e == nil && ok
}
func infrastructureOwners(actor string, r infrastructure.Revision) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(x string) {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	add(actor)
	for _, x := range r.OwnerIDs {
		add(x)
	}
	for _, resource := range r.Resources {
		for _, x := range resource.OwnerIDs {
			add(x)
		}
	}
	return out
}
func infrastructureRevisionResolves(git *storage.Store, repo string, r infrastructure.Revision, releases *releases.Store, deployments *deployments.Store) bool {
	if git == nil {
		return false
	}
	gr, e := git.Open(repo)
	if e != nil {
		return false
	}
	if _, e = gr.ReadCommit(storage.ObjectID(r.Revision)); e != nil {
		return false
	}
	for _, x := range r.Resources {
		if !infrastructureLinksResolve(repo, x.EnvironmentID, x.ReleaseID, releases, deployments) {
			return false
		}
	}
	return true
}
func infrastructureLinksResolve(repo, environmentID, releaseID string, releases *releases.Store, deployments *deployments.Store) bool {
	if environmentID != "" {
		if deployments == nil {
			return false
		}
		if _, e := deployments.GetEnvironment(repo, environmentID); e != nil {
			return false
		}
	}
	if releaseID != "" {
		if releases == nil {
			return false
		}
		if _, e := releases.Get(repo, releaseID); e != nil {
			return false
		}
	}
	return true
}
func writeInfrastructure(w http.ResponseWriter, v infrastructure.Definition, err error, status int) {
	switch {
	case err == nil:
		writeJSON(w, status, v)
	case errors.Is(err, infrastructure.ErrConflict):
		writeAPIError(w, 409, "infrastructure_conflict", "the definition changed; reload before publishing another revision")
	case errors.Is(err, infrastructure.ErrInvalid):
		writeAPIError(w, 400, "invalid_infrastructure", "publish an exact commit with complete resources, owners, providers, boundaries, constraints, commitments, and sanitized observations")
	case errors.Is(err, repositories.ErrInvalidCollaborator), errors.Is(err, repositories.ErrNotFound):
		writeAPIError(w, 403, "infrastructure_forbidden", "every declared owner must be a current repository participant")
	default:
		log.Printf("infrastructure storage: %v", err)
		writeAPIError(w, 500, "infrastructure_unavailable", "infrastructure definition could not be persisted")
	}
}
