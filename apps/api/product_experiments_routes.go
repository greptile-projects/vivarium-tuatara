package main

import (
	"errors"
	"log"
	"net/http"
	"slices"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/productexperiments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

type productExperimentInput struct {
	ExpectedVersion int                         `json:"expected_version"`
	Source          productexperiments.Source   `json:"source"`
	Revision        productexperiments.Revision `json:"revision"`
	Signals         []productexperiments.Signal `json:"signals"`
}
type productExperimentCommentInput struct {
	Body string `json:"body"`
}
type productExperimentApprovalInput struct {
	ExpectedVersion int    `json:"expected_version"`
	Decision        string `json:"decision"`
	Note            string `json:"note"`
}
type productExperimentWorkInput struct {
	ExpectedVersion int                         `json:"expected_version"`
	Work            productexperiments.WorkLink `json:"work"`
}
type productExperimentAudienceInput struct {
	ExpectedVersion int                                 `json:"expected_version"`
	Contract        productexperiments.AudienceContract `json:"contract"`
}
type productExperimentAssignmentInput struct {
	Subject string                               `json:"subject"`
	Context productexperiments.AssignmentContext `json:"context"`
}
type productExperimentRunInput struct {
	ContractID    string                             `json:"contract_id"`
	DeploymentIDs []string                           `json:"deployment_ids"`
	Allocation    []productexperiments.RunAllocation `json:"allocation"`
}
type productExperimentStageInput struct {
	ExpectedVersion int                                `json:"expected_version"`
	Allocation      []productexperiments.RunAllocation `json:"allocation"`
	Reason          string                             `json:"reason"`
}
type productExperimentControlInput struct {
	ExpectedVersion int    `json:"expected_version"`
	Action          string `json:"action"`
	Reason          string `json:"reason"`
}
type productExperimentAnalysisInput struct {
	Analysis productexperiments.Analysis `json:"analysis"`
}
type productExperimentOutcomeInput struct {
	ExpectedVersion int                                `json:"expected_version"`
	Decision        productexperiments.OutcomeDecision `json:"decision"`
}
type productExperimentTaskInput struct {
	ExpectedVersion int                             `json:"expected_version"`
	Evidence        productexperiments.TaskEvidence `json:"evidence"`
}

func registerProductExperimentRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, store *productexperiments.Store, proposals *proposals.Store, pulls *pullrequests.Store, checks *checkruns.Store, releaseStore *releases.Store, deploymentStore *deployments.Store, orgs *organizations.Store) {
	store.ConfigureDeploymentHealth(func(repositoryID string, deploymentIDs []string) (bool, error) {
		if deploymentStore == nil {
			return false, errors.New("deployment store unavailable")
		}
		for _, deploymentID := range deploymentIDs {
			deployment, err := deploymentStore.GetPromotion(repositoryID, deploymentID)
			if err != nil {
				return false, err
			}
			if deployment.State != "succeeded" {
				return false, nil
			}
		}
		return true, nil
	})
	store.ConfigureOutcomeEvidence(func(repositoryID, experimentID, decisionID, taskID, taskKind string, evidence productexperiments.TaskEvidence) bool {
		if pulls == nil || strings.TrimSpace(evidence.PullRequestID) == "" {
			return false
		}
		pull, err := pulls.Get(repositoryID, evidence.PullRequestID)
		if err != nil || pull.Status != pullrequests.Merged || !outcomeEvidenceTrailers(pull.Body, experimentID, decisionID, taskID, taskKind) {
			return false
		}
		switch evidence.Kind {
		case "pull_request":
			if evidence.ResourceID != evidence.PullRequestID || (taskKind != "follow_up" && taskKind != "remove_variants" && taskKind != "remove_targeting" && taskKind != "revoke_credentials" && taskKind != "stop_collection" && taskKind != "review") {
				return false
			}
			return true
		case "release":
			if releaseStore == nil || taskKind != "release" {
				return false
			}
			candidate, err := releaseStore.Get(repositoryID, evidence.ResourceID)
			return err == nil && candidate.ID != ""
		case "deployment":
			if deploymentStore == nil || (taskKind != "deployment" && taskKind != "rollout" && taskKind != "rollback") {
				return false
			}
			promotion, err := deploymentStore.GetPromotion(repositoryID, evidence.ResourceID)
			if err != nil || promotion.State != "succeeded" {
				return false
			}
			release, err := releaseStore.Get(repositoryID, promotion.ReleaseID)
			return err == nil && experimentDeploymentContainsPull(release, promotion, pull.ID)
		default:
			return false
		}
	})
	writeProjected := func(w http.ResponseWriter, experiment productexperiments.Experiment, status int) {
		all, err := store.List(experiment.RepositoryID)
		if err != nil {
			writeAPIError(w, 500, "product_experiments_unavailable", "experiments could not be projected")
			return
		}
		for _, other := range all {
			if other.ID != experiment.ID && productexperiments.Overlaps(experiment, other) {
				productexperiments.AddOverlap(&experiment, other)
			}
		}
		writeJSON(w, status, experiment)
	}
	mux.HandleFunc("POST /repositories/{id}/product-experiments", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in productExperimentInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete experiment plan is required")
			return
		}
		var out productexperiments.Experiment
		err := catalog.WithCurrentParticipant(actor.UserID, r.PathValue("id"), func() error {
			var e error
			out, e = store.Create(r.PathValue("id"), actor.UserID, in.Source, in.Revision, in.Signals)
			return e
		})
		if err != nil {
			writeProductExperimentError(w, err)
			return
		}
		writeProjected(w, out, 201)
	})
	mux.HandleFunc("GET /repositories/{id}/product-experiments", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		all, err := store.List(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "product_experiments_unavailable", "experiments could not be read")
			return
		}
		for i := range all {
			for _, other := range all {
				if all[i].ID != other.ID && productexperiments.Overlaps(all[i], other) {
					productexperiments.AddOverlap(&all[i], other)
				}
			}
		}
		writeJSON(w, 200, map[string]any{"experiments": all})
	})
	mux.HandleFunc("POST /repositories/{id}/product-experiments/{experiment_id}/revisions", func(w http.ResponseWriter, r *http.Request) {
		mutateExperiment(w, r, catalog, credentials, store, func(actor string, in productExperimentInput) (productexperiments.Experiment, error) {
			return store.Revise(r.PathValue("experiment_id"), in.ExpectedVersion, actor, in.Revision, in.Signals)
		}, writeProjected)
	})
	mux.HandleFunc("POST /repositories/{id}/product-experiments/{experiment_id}/comments", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		current, err := store.Get(r.PathValue("experiment_id"))
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "product_experiment_not_found", "experiment not found")
			return
		}
		var in productExperimentCommentInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "comment body is required")
			return
		}
		out, err := store.Comment(current.ID, actor.UserID, in.Body)
		if err != nil {
			writeProductExperimentError(w, err)
			return
		}
		writeProjected(w, out, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/product-experiments/{experiment_id}/approvals", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		current, err := store.Get(r.PathValue("experiment_id"))
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "product_experiment_not_found", "experiment not found")
			return
		}
		var in productExperimentApprovalInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a current plan decision is required")
			return
		}
		out, err := store.Approve(current.ID, actor.UserID, in.Decision, in.Note, in.ExpectedVersion)
		if err != nil {
			writeProductExperimentError(w, err)
			return
		}
		writeProjected(w, out, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/product-experiments/{experiment_id}/work", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		current, err := store.Get(r.PathValue("experiment_id"))
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "product_experiment_not_found", "experiment not found")
			return
		}
		var in productExperimentWorkInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "revision-exact experiment work is required")
			return
		}
		replayed, exactReplay, replayErr := store.ExistingWorkReplay(current.ID, actor.UserID, in.ExpectedVersion, in.Work)
		if replayErr != nil {
			writeProductExperimentError(w, replayErr)
			return
		}
		if exactReplay {
			writeProjected(w, replayed, 200)
			return
		}
		pull, pullErr := pulls.Get(r.PathValue("id"), in.Work.PullRequestID)
		if pullErr != nil || pull.SourceCommitID != in.Work.CommitID || (in.Work.ProposalID != "" && (pull.ProposalID == nil || *pull.ProposalID != in.Work.ProposalID)) || (in.Work.TaskID != "" && (pull.TaskID == nil || *pull.TaskID != in.Work.TaskID)) || (in.Work.SessionID != "" && (pull.TaskSessionID == nil || *pull.TaskSessionID != in.Work.SessionID)) || (in.Work.WorkspaceID != "" && pull.WorkspaceID != in.Work.WorkspaceID) {
			writeAPIError(w, 422, "invalid_experiment_work", "the ordinary pull and exact execution links must match the declared commit")
			return
		}
		if in.Work.ProposalID == "" || in.Work.TaskID == "" {
			writeAPIError(w, 422, "experiment_task_missing", "experiment work must retain its ordinary proposal task")
			return
		}
		task, taskErr := proposals.GetTask(r.PathValue("id"), in.Work.ProposalID, in.Work.TaskID)
		if taskErr != nil || task.Assignment == nil || task.Assignment.AssigneeType != in.Work.OwnerType || task.Assignment.AssigneeID != in.Work.OwnerID {
			writeAPIError(w, 422, "experiment_assignment_mismatch", "the declared owner must match the linked task's current human or agent assignment")
			return
		}
		runs, runErr := checks.List(r.PathValue("id"), pull.ID)
		if runErr != nil {
			writeAPIError(w, 503, "experiment_checks_unavailable", "pull checks could not be verified")
			return
		}
		available := successfulExperimentChecks(runs, pull.SourceCommitID)
		for _, name := range in.Work.CheckNames {
			if !available[name] {
				writeAPIError(w, 422, "experiment_check_missing", "every declared experiment check must exist on the exact pull commit")
				return
			}
		}
		var out productexperiments.Experiment
		err = catalog.WithCurrentParticipant(actor.UserID, r.PathValue("id"), func() error {
			var e error
			out, e = store.LinkWork(current.ID, actor.UserID, in.ExpectedVersion, in.Work)
			return e
		})
		if err != nil {
			writeProductExperimentError(w, err)
			return
		}
		writeProjected(w, out, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/product-experiments/{experiment_id}/audience-contracts", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		repository, repositoryErr := catalog.Get(actor.UserID, r.PathValue("id"))
		if repositoryErr != nil {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return
		}
		if repository.OwnerID != actor.UserID {
			writeAPIError(w, 403, "experiment_audience_forbidden", "only the repository owner may approve audience exposure")
			return
		}
		current, err := store.Get(r.PathValue("experiment_id"))
		if err != nil || current.RepositoryID != repository.ID {
			writeAPIError(w, 404, "product_experiment_not_found", "experiment not found")
			return
		}
		var in productExperimentAudienceInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete audience contract is required")
			return
		}
		release, err := releaseStore.Get(repository.ID, in.Contract.ReleaseID)
		if err != nil || release.CommitID != in.Contract.ReleaseCommitID {
			writeAPIError(w, 422, "experiment_release_mismatch", "the contract must name an exact repository release")
			return
		}
		out, err := store.ApproveAudience(current.ID, actor.UserID, in.ExpectedVersion, in.Contract)
		if err != nil {
			writeProductExperimentError(w, err)
			return
		}
		writeProjected(w, out, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/product-experiments/{experiment_id}/audience-contracts/{contract_id}/assignments", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		repository, repositoryErr := catalog.Get(actor.UserID, r.PathValue("id"))
		if repositoryErr != nil || repository.OwnerID != actor.UserID {
			writeAPIError(w, 403, "experiment_assignment_forbidden", "only the repository owner may admit assignments")
			return
		}
		current, err := store.Get(r.PathValue("experiment_id"))
		if err != nil || current.RepositoryID != repository.ID {
			writeAPIError(w, 404, "product_experiment_not_found", "experiment not found")
			return
		}
		var in productExperimentAssignmentInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "subject and consent state are required")
			return
		}
		_, receipt, err := store.Assign(current.ID, r.PathValue("contract_id"), in.Subject, in.Context)
		if err != nil {
			writeProductExperimentError(w, err)
			return
		}
		writeJSON(w, 201, receipt)
	})
	mux.HandleFunc("POST /repositories/{id}/product-experiments/{experiment_id}/runs", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		current, err := store.Get(r.PathValue("experiment_id"))
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "product_experiment_not_found", "experiment not found")
			return
		}
		var in productExperimentRunInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an approved contract, deployments, and initial allocation are required")
			return
		}
		environments := make([]string, 0, len(in.DeploymentIDs))
		for _, id := range in.DeploymentIDs {
			d, e := deploymentStore.GetPromotion(current.RepositoryID, id)
			if e != nil || d.State != "succeeded" {
				writeAPIError(w, 422, "experiment_deployment_unready", "every launch deployment must have succeeded")
				return
			}
			found := false
			for _, c := range current.AudienceContracts {
				if c.ID == in.ContractID && c.ReleaseID == d.ReleaseID && c.ReleaseCommitID == d.CommitID {
					found = true
				}
			}
			if !found {
				writeAPIError(w, 422, "experiment_deployment_mismatch", "deployments must carry the audience contract's exact release")
				return
			}
			environments = append(environments, d.EnvironmentID)
		}
		out, err := store.Launch(current.ID, actor.UserID, in.ContractID, in.DeploymentIDs, environments, in.Allocation)
		if err != nil {
			writeProductExperimentError(w, err)
			return
		}
		writeProjected(w, out, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/product-experiments/{experiment_id}/runs/{run_id}/stages", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		current, err := store.Get(r.PathValue("experiment_id"))
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "product_experiment_not_found", "experiment not found")
			return
		}
		var in productExperimentStageInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a governed allocation stage is required")
			return
		}
		out, err := store.Stage(current.ID, r.PathValue("run_id"), actor.UserID, in.ExpectedVersion, in.Allocation, in.Reason)
		if err != nil {
			writeProductExperimentError(w, err)
			return
		}
		writeProjected(w, out, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/product-experiments/{experiment_id}/runs/{run_id}/controls", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		current, err := store.Get(r.PathValue("experiment_id"))
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "product_experiment_not_found", "experiment not found")
			return
		}
		var in productExperimentControlInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a pause, resume, or stop control is required")
			return
		}
		out, err := store.Control(current.ID, r.PathValue("run_id"), actor.UserID, in.Action, in.Reason, in.ExpectedVersion)
		if err != nil {
			writeProductExperimentError(w, err)
			return
		}
		writeProjected(w, out, 200)
	})
	mux.HandleFunc("POST /repositories/{id}/product-experiments/{experiment_id}/runs/{run_id}/observations", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		current, err := store.Get(r.PathValue("experiment_id"))
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "product_experiment_not_found", "experiment not found")
			return
		}
		var in productexperiments.RunObservation
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "bounded live evidence is required")
			return
		}
		out, err := store.Observe(current.ID, r.PathValue("run_id"), actor.UserID, in)
		if err != nil {
			writeProductExperimentError(w, err)
			return
		}
		writeProjected(w, out, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/product-experiments/{experiment_id}/analyses", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		current, err := store.Get(r.PathValue("experiment_id"))
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "product_experiment_not_found", "experiment not found")
			return
		}
		var in productExperimentAnalysisInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "revision-bound analysis is required")
			return
		}
		var out productexperiments.Experiment
		if in.Analysis.InterpretedByType == "agent" {
			if !agentOperator(current.RepositoryID, in.Analysis.InterpretedByID, actor.UserID, orgs, catalog) {
				writeAPIError(w, 403, "experiment_agent_operator_required", "the participant must operate the selected approved agent")
				return
			}
			out, err = store.AnalyzeAsAgent(current.ID, actor.UserID, in.Analysis.InterpretedByID, in.Analysis)
		} else {
			out, err = store.Analyze(current.ID, actor.UserID, in.Analysis)
		}
		if err != nil {
			writeProductExperimentError(w, err)
			return
		}
		writeProjected(w, out, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/product-experiments/{experiment_id}/outcomes", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		current, err := store.Get(r.PathValue("experiment_id"))
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "product_experiment_not_found", "experiment not found")
			return
		}
		var in productExperimentOutcomeInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a versioned outcome decision is required")
			return
		}
		out, err := store.DecideOutcome(current.ID, actor.UserID, in.ExpectedVersion, in.Decision)
		if err != nil {
			writeProductExperimentError(w, err)
			return
		}
		writeProjected(w, out, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/product-experiments/{experiment_id}/outcomes/{decision_id}/tasks/{task_id}/complete", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		current, err := store.Get(r.PathValue("experiment_id"))
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "product_experiment_not_found", "experiment not found")
			return
		}
		var in productExperimentTaskInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "completion evidence is required")
			return
		}
		out, err := store.CompleteOutcomeTask(current.ID, r.PathValue("decision_id"), r.PathValue("task_id"), actor.UserID, in.Evidence, in.ExpectedVersion)
		if err != nil {
			writeProductExperimentError(w, err)
			return
		}
		writeProjected(w, out, 200)
	})
}

func experimentDeploymentContainsPull(release releases.Candidate, promotion deployments.Promotion, pullID string) bool {
	return release.ID == promotion.ReleaseID && release.CommitID == promotion.CommitID && slices.Contains(release.Inclusions.PullRequestIDs, pullID)
}

func outcomeEvidenceTrailers(body, experimentID, decisionID, taskID, action string) bool {
	wanted := map[string]string{"Experiment": experimentID, "Outcome-Decision": decisionID, "Outcome-Task": taskID, "Outcome-Action": action}
	found := map[string]string{}
	for _, line := range strings.Split(body, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if ok {
			found[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	for key, value := range wanted {
		if found[key] != value {
			return false
		}
	}
	return true
}

func successfulExperimentChecks(runs []checkruns.Run, commitID string) map[string]bool {
	available := map[string]bool{}
	for _, run := range runs {
		if run.CommitID == commitID && run.State == "succeeded" {
			available[run.Definition.Name] = true
		}
	}
	return available
}
func mutateExperiment(w http.ResponseWriter, r *http.Request, catalog *repositories.Store, credentials *auth.Store, store *productexperiments.Store, fn func(string, productExperimentInput) (productexperiments.Experiment, error), write func(http.ResponseWriter, productexperiments.Experiment, int)) {
	actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
	if !ok {
		return
	}
	current, err := store.Get(r.PathValue("experiment_id"))
	if err != nil || current.RepositoryID != r.PathValue("id") {
		writeAPIError(w, 404, "product_experiment_not_found", "experiment not found")
		return
	}
	var in productExperimentInput
	if decodeJSON(r, &in) != nil {
		writeAPIError(w, 400, "invalid_request", "a complete experiment revision is required")
		return
	}
	var out productexperiments.Experiment
	err = catalog.WithCurrentParticipant(actor.UserID, current.RepositoryID, func() error { var e error; out, e = fn(actor.UserID, in); return e })
	if err != nil {
		writeProductExperimentError(w, err)
		return
	}
	write(w, out, 200)
}
func writeProductExperimentError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, productexperiments.ErrConflict):
		writeAPIError(w, 409, "product_experiment_conflict", "the plan changed; reload before acting")
	case errors.Is(err, productexperiments.ErrInvalid):
		writeAPIError(w, 400, "invalid_product_experiment", "source, hypothesis, variants, audience, metrics, evidence, duration, owners, stop conditions, and versioned signals are required")
	case errors.Is(err, repositories.ErrInvalidCollaborator), errors.Is(err, repositories.ErrNotFound):
		writeAPIError(w, 403, "product_experiment_forbidden", "only a current repository participant may change experiments")
	default:
		log.Printf("product experiment storage: %v", err)
		writeAPIError(w, 500, "product_experiments_unavailable", "experiment could not be persisted")
	}
}
