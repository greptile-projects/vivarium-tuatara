package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os/exec"
	"regexp"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/activities"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/agentprojects"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/collaborationworkflows"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/federation"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/packages"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workflowcomponents"
)

type collaborationWorkflowSourceInput struct {
	ExpectedVersion int    `json:"expected_version"`
	ActivationID    string `json:"activation_id"`
	Revision        string `json:"revision"`
	Path            string `json:"path"`
}

var exactCommit = regexp.MustCompile(`^[0-9a-f]{40}$`)

func registerCollaborationWorkflowRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, workflows *collaborationworkflows.Store, components *workflowcomponents.Store, packageStore *packages.Store, peers *federation.Store, agents *agentprojects.Store, pulls *pullrequests.Store, deliveries *activities.Store) {
	mux.HandleFunc("GET /repositories/{id}/collaboration-workflows", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		out, err := workflows.List(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "collaboration_workflows_unavailable", "collaboration workflows could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"workflows": out})
	})
	mux.HandleFunc("GET /repositories/{id}/collaboration-workflows/{workflow_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		out, err := workflows.Get(r.PathValue("workflow_id"))
		if err != nil || out.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "collaboration_workflow_not_found", "collaboration workflow not found")
			return
		}
		writeJSON(w, 200, out)
	})
	handle := func(activate, revise bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
			if !ok {
				return
			}
			var in collaborationWorkflowSourceInput
			if decodeJSON(r, &in) != nil {
				writeAPIError(w, 400, "invalid_request", "exact revision and configuration path are required")
				return
			}
			definition, source, err := readWorkflowDefinition(git, r.PathValue("id"), in.Revision, in.Path)
			if err != nil {
				writeAPIError(w, 400, "invalid_workflow_source", err.Error())
				return
			}
			check := func(inv collaborationworkflows.Invocation) (bool, string) {
				switch inv.Kind {
				case "manual":
					return true, ""
				case "platform_action":
					if !stringIn(inv.Action, "create_issue", "comment", "request_review", "dispatch_check", "update_project", "notify") {
						return false, "platform action is not in the permitted workflow action set"
					}
				case "component":
					if components == nil {
						return false, "reusable component resolver is unavailable"
					}
					component, installation, ok := components.Resolve(r.PathValue("id"), inv.Component)
					if !ok {
						return false, "component must be an exact installed version reviewed in this repository"
					}
					grants := map[string]bool{}
					for _, mapping := range installation.Revisions[len(installation.Revisions)-1].Mappings {
						grants[mapping.LocalPermission] = true
					}
					for _, requested := range inv.Authority {
						if !grants[requested] {
							return false, "component authority must use an explicitly mapped local permission"
						}
					}
					if component.Definition.Compatibility.WorkflowFormat != 1 {
						return false, "component is incompatible with this workflow format"
					}
					if !workflowComponentCurrentlyTrusted(catalog, packageStore, peers, component) {
						return false, "component publisher, package, or federation trust is no longer current"
					}
				case "agent":
					if agents == nil {
						return false, "approved agent project resolver is unavailable"
					}
					project, e := agents.Get(inv.AgentID)
					if e != nil || project.RepositoryID != r.PathValue("id") || len(project.Revisions) == 0 || len(project.Diagnostics) > 0 {
						return false, "agent must resolve to a gap-free reviewed project in this repository"
					}
				case "workflow":
					target, e := workflows.Get(inv.WorkflowID)
					if e != nil || target.RepositoryID != r.PathValue("id") || target.Status != "active" {
						return false, "reusable workflow is not an active readable workflow in this repository"
					}
					if revise {
						currentID := r.PathValue("workflow_id")
						if inv.WorkflowID == currentID || workflowReaches(workflows, inv.WorkflowID, currentID, map[string]bool{}) {
							return false, "trigger loop: workflow invocation would create a recursive dependency"
						}
					}
				}
				return true, ""
			}
			preview := workflows.Preview(r.PathValue("id"), definition, source, check)
			if !activate {
				writeJSON(w, 200, preview)
				return
			}
			owners := append([]string{}, definition.OwnerIDs...)
			for _, step := range definition.Steps {
				owners = append(owners, step.OwnerIDs...)
			}
			var out collaborationworkflows.Workflow
			err = catalog.WithCurrentParticipants(owners, r.PathValue("id"), func() error {
				if revise {
					current, e := workflows.Get(r.PathValue("workflow_id"))
					if e != nil || current.RepositoryID != r.PathValue("id") {
						return collaborationworkflows.ErrNotFound
					}
					out, e = workflows.Revise(current.ID, in.ExpectedVersion, actor.UserID, preview)
					return e
				}
				out, err = workflows.Create(r.PathValue("id"), actor.UserID, in.ActivationID, preview)
				return err
			})
			writeCollaborationWorkflow(w, out, err, revise)
		}
	}
	mux.HandleFunc("POST /repositories/{id}/collaboration-workflows/preview", handle(false, false))
	mux.HandleFunc("POST /repositories/{id}/collaboration-workflows", handle(true, false))
	mux.HandleFunc("POST /repositories/{id}/collaboration-workflows/{workflow_id}/revisions", handle(true, true))

	type startInput struct {
		DeliveryID      string `json:"delivery_id"`
		WorkflowVersion int    `json:"workflow_version"`
	}
	mux.HandleFunc("POST /repositories/{id}/collaboration-workflows/{workflow_id}/executions", func(w http.ResponseWriter, r *http.Request) {
		_, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in startInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a revision-exact triggering event is required")
			return
		}
		wf, err := workflows.Get(r.PathValue("workflow_id"))
		if err != nil || wf.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "collaboration_workflow_not_found", "collaboration workflow not found")
			return
		}
		if in.WorkflowVersion < 1 || in.WorkflowVersion != wf.CurrentVersion || in.WorkflowVersion > len(wf.Revisions) {
			writeAPIError(w, 409, "stale_workflow_input", "workflow revision is no longer current")
			return
		}
		event, ok := workflowEventFromDelivery(git, pulls, deliveries, r.PathValue("id"), in.DeliveryID)
		if !ok {
			writeAPIError(w, 409, "stale_workflow_input", "a trusted current repository event delivery is required")
			return
		}
		out, err := workflows.StartExecution(wf.ID, in.WorkflowVersion, event)
		writeWorkflowExecution(w, out, err, 201)
	})
	mux.HandleFunc("GET /repositories/{id}/collaboration-workflows/{workflow_id}/executions", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		out, err := workflows.ListExecutions(r.PathValue("id"), r.PathValue("workflow_id"))
		if err != nil {
			writeAPIError(w, 500, "workflow_executions_unavailable", "executions could not be read")
			return
		}
		for i := range out {
			out[i] = collaborationworkflows.PublicExecution(out[i])
		}
		writeJSON(w, 200, map[string]any{"executions": out})
	})
	mux.HandleFunc("GET /repositories/{id}/collaboration-workflows/{workflow_id}/executions/{execution_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		out, err := workflows.GetExecution(r.PathValue("execution_id"))
		if err != nil || out.RepositoryID != r.PathValue("id") || out.WorkflowID != r.PathValue("workflow_id") {
			writeAPIError(w, 404, "workflow_execution_not_found", "workflow execution not found")
			return
		}
		writeJSON(w, 200, collaborationworkflows.PublicExecution(out))
	})
	type claimInput struct {
		ExpectedVersion int `json:"expected_version"`
	}
	mux.HandleFunc("POST /repositories/{id}/collaboration-workflows/{workflow_id}/executions/{execution_id}/steps/{step_id}/claim", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in claimInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "expected execution version is required")
			return
		}
		ex, err := workflows.GetExecution(r.PathValue("execution_id"))
		if err != nil || ex.WorkflowID != r.PathValue("workflow_id") || ex.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "workflow_execution_not_found", "workflow execution not found")
			return
		}
		wf, _ := workflows.Get(ex.WorkflowID)
		stepOwner := false
		if ex.WorkflowVersion > 0 && ex.WorkflowVersion <= len(wf.Revisions) {
			for _, st := range wf.Revisions[ex.WorkflowVersion-1].Definition.Steps {
				if st.ID == r.PathValue("step_id") {
					for _, owner := range st.OwnerIDs {
						if owner == actor.UserID {
							stepOwner = true
						}
					}
					if st.Manual {
						for _, run := range ex.Steps {
							if run.StepID == st.ID && run.TakenOverBy == actor.UserID {
								stepOwner = true
							}
						}
					}
				}
			}
		}
		out, err := workflows.ClaimStep(ex.ID, r.PathValue("step_id"), in.ExpectedVersion, stepOwner)
		if err != nil {
			writeWorkflowLeaseError(w, err)
			return
		}
		writeJSON(w, 200, out)
	})
	type completeInput struct {
		Token        string                                `json:"token"`
		Actions      int                                   `json:"actions"`
		Outputs      map[string]any                        `json:"outputs"`
		FailureCode  string                                `json:"failure_code"`
		Logs         []collaborationworkflows.StepLog      `json:"logs"`
		Artifacts    []collaborationworkflows.StepArtifact `json:"artifacts"`
		AgentSession *collaborationworkflows.AgentSession  `json:"agent_session"`
		CostUnits    float64                               `json:"cost_units"`
		Provenance   []string                              `json:"provenance"`
	}
	mux.HandleFunc("POST /repositories/{id}/collaboration-workflows/{workflow_id}/executions/{execution_id}/steps/{step_id}/complete", func(w http.ResponseWriter, r *http.Request) {
		var in completeInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "the step credential and result are required")
			return
		}
		ex, err := workflows.GetExecution(r.PathValue("execution_id"))
		if err != nil || ex.WorkflowID != r.PathValue("workflow_id") || ex.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "workflow_execution_not_found", "workflow execution not found")
			return
		}
		out, err := workflows.CompleteStepEvidence(ex.ID, r.PathValue("step_id"), in.Token, in.Actions, in.Outputs, in.FailureCode, in.Logs, in.Artifacts, in.AgentSession, in.CostUnits, in.Provenance)
		writeWorkflowExecution(w, out, err, 200)
	})
	type interventionInput struct {
		ExpectedVersion int    `json:"expected_version"`
		Kind            string `json:"kind"`
		StepID          string `json:"step_id"`
		Reason          string `json:"reason"`
		InputName       string `json:"input_name"`
		Value           any    `json:"value"`
	}
	mux.HandleFunc("POST /repositories/{id}/collaboration-workflows/{workflow_id}/executions/{execution_id}/interventions", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in interventionInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a versioned intervention is required")
			return
		}
		ex, err := workflows.GetExecution(r.PathValue("execution_id"))
		if err != nil || ex.WorkflowID != r.PathValue("workflow_id") || ex.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "workflow_execution_not_found", "workflow execution not found")
			return
		}
		out, err := workflows.Intervene(ex.ID, actor.UserID, in.Kind, in.StepID, in.Reason, in.InputName, in.Value, in.ExpectedVersion)
		writeWorkflowExecution(w, collaborationworkflows.PublicExecution(out), err, 200)
	})
	type cancelInput struct {
		Code string `json:"code"`
	}
	mux.HandleFunc("POST /repositories/{id}/collaboration-workflows/{workflow_id}/executions/{execution_id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in cancelInput
		if decodeJSON(r, &in) != nil || in.Code == "" {
			writeAPIError(w, 400, "invalid_request", "a cancellation code is required")
			return
		}
		ex, err := workflows.GetExecution(r.PathValue("execution_id"))
		if err != nil || ex.WorkflowID != r.PathValue("workflow_id") || ex.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "workflow_execution_not_found", "workflow execution not found")
			return
		}
		out, err := workflows.Intervene(ex.ID, actor.UserID, "cancel", "", in.Code, "", nil, ex.Version)
		writeWorkflowExecution(w, out, err, 200)
	})
}

func workflowEventFromDelivery(git *storage.Store, pulls *pullrequests.Store, deliveries *activities.Store, repo, deliveryID string) (collaborationworkflows.TriggerEvent, bool) {
	if deliveries == nil {
		return collaborationworkflows.TriggerEvent{}, false
	}
	delivery, err := deliveries.Get(deliveryID)
	if err != nil || delivery.RepositoryID != repo || delivery.ResourceType != "pull_request" || delivery.ResourceRevision == "" {
		return collaborationworkflows.TriggerEvent{}, false
	}
	names := map[string]string{"pull_request.created": "pull.opened", "pull_request.synchronized": "pull.synchronized", "pull_request.merged": "pull.merged"}
	name := names[delivery.Kind]
	if name == "" || pulls == nil {
		return collaborationworkflows.TriggerEvent{}, false
	}
	pull, err := pulls.Get(repo, delivery.ResourceID)
	if err != nil || pull.SourceCommitID != delivery.ResourceRevision || !workflowCommitReachable(git, repo, delivery.ResourceRevision) {
		return collaborationworkflows.TriggerEvent{}, false
	}
	return collaborationworkflows.TriggerEvent{ID: delivery.ID, Kind: "repository_event", Name: name, ActorID: delivery.ActorID, OccurredAt: delivery.CreatedAt, Inputs: map[string]any{"pull_id": delivery.ResourceID}, ResourceRevisions: map[string]string{"pull_id": delivery.ResourceRevision}}, true
}

func workflowCommitReachable(git *storage.Store, repo, revision string) bool {
	if git == nil || !exactCommit.MatchString(revision) {
		return false
	}
	r, err := git.Open(repo)
	if err != nil {
		return false
	}
	branches, err := exec.Command("git", "--git-dir="+r.Path(), "for-each-ref", "--format=%(refname)", "refs/heads").Output()
	if err != nil {
		return false
	}
	for _, branch := range strings.Fields(string(branches)) {
		if strings.HasPrefix(branch, "refs/heads/vivarium-security/") {
			continue
		}
		if exec.Command("git", "--git-dir="+r.Path(), "merge-base", "--is-ancestor", revision, branch).Run() == nil {
			return true
		}
	}
	return false
}
func writeWorkflowLeaseError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, collaborationworkflows.ErrNotFound):
		writeAPIError(w, 404, "workflow_step_not_found", "workflow step not found")
	case errors.Is(err, collaborationworkflows.ErrExecutionConflict):
		writeAPIError(w, 409, "workflow_execution_conflict", "execution changed or the step already has a live credential")
	case errors.Is(err, collaborationworkflows.ErrExecutionBlocked):
		writeAPIError(w, 409, "workflow_step_blocked", "dependencies, policy, retry, concurrency, rate, or budget prevent this step")
	default:
		writeAPIError(w, 500, "workflow_executions_unavailable", "step could not be scheduled")
	}
}
func writeWorkflowExecution(w http.ResponseWriter, out collaborationworkflows.Execution, err error, status int) {
	switch {
	case err == nil:
		writeJSON(w, status, collaborationworkflows.PublicExecution(out))
	case errors.Is(err, collaborationworkflows.ErrInvalid):
		writeAPIError(w, 400, "invalid_workflow_data", "event, evidence, output, or intervention data violates the reviewed workflow contract")
	case errors.Is(err, collaborationworkflows.ErrCredential):
		writeAPIError(w, 401, "workflow_step_credential_invalid", "step credential is invalid, expired, or revoked")
	case errors.Is(err, collaborationworkflows.ErrExecutionConflict):
		writeAPIError(w, 409, "workflow_execution_conflict", "duplicate event or execution version conflicts with durable state")
	case errors.Is(err, collaborationworkflows.ErrExecutionBlocked):
		writeAPIError(w, 409, "workflow_execution_blocked", "policy, concurrency, rate, retry, stale input, or budget limits block execution")
	case errors.Is(err, collaborationworkflows.ErrNotFound):
		writeAPIError(w, 404, "workflow_execution_not_found", "workflow execution not found")
	default:
		log.Printf("workflow execution storage: %v", err)
		writeAPIError(w, 500, "workflow_executions_unavailable", "workflow execution could not be persisted")
	}
}

func workflowReaches(store *collaborationworkflows.Store, start, goal string, seen map[string]bool) bool {
	if start == goal {
		return true
	}
	if seen[start] {
		return false
	}
	seen[start] = true
	w, err := store.Get(start)
	if err != nil || len(w.Revisions) == 0 {
		return false
	}
	for _, step := range w.Revisions[len(w.Revisions)-1].Definition.Steps {
		if step.Invocation.Kind == "workflow" && workflowReaches(store, step.Invocation.WorkflowID, goal, seen) {
			return true
		}
	}
	return false
}

func readWorkflowDefinition(git *storage.Store, repo, revision, file string) (collaborationworkflows.Definition, collaborationworkflows.Source, error) {
	var d collaborationworkflows.Definition
	var src collaborationworkflows.Source
	if git == nil || !exactCommit.MatchString(revision) || file == "" || strings.HasPrefix(file, "/") || strings.Contains(file, "..") {
		return d, src, errors.New("source must name an exact 40-character commit and repository-relative path")
	}
	r, err := git.Open(repo)
	if err != nil {
		return d, src, errors.New("repository source is inaccessible")
	}
	branches, err := exec.Command("git", "--git-dir="+r.Path(), "for-each-ref", "--format=%(refname)", "refs/heads").Output()
	if err != nil {
		return d, src, errors.New("repository branches could not be resolved")
	}
	reachable := false
	for _, branch := range strings.Fields(string(branches)) {
		if strings.HasPrefix(branch, "refs/heads/vivarium-security/") {
			continue
		}
		if exec.Command("git", "--git-dir="+r.Path(), "merge-base", "--is-ancestor", revision, branch).Run() == nil {
			reachable = true
			break
		}
	}
	if !reachable {
		return d, src, errors.New("configuration commit must be reachable from a non-security branch")
	}
	b, err := exec.Command("git", "--git-dir="+r.Path(), "show", revision+":"+file).Output()
	if err != nil {
		return d, src, errors.New("configuration file is missing or is not a readable blob at that commit")
	}
	if len(b) > 256*1024 || json.Unmarshal(b, &d) != nil {
		return d, src, errors.New("configuration must be valid workflow JSON no larger than 256 KiB")
	}
	h := sha256.Sum256(b)
	src = collaborationworkflows.Source{Revision: revision, Path: file, SHA256: hex.EncodeToString(h[:])}
	return d, src, nil
}

func stringIn(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
func writeCollaborationWorkflow(w http.ResponseWriter, out collaborationworkflows.Workflow, err error, revised bool) {
	status := 201
	if revised {
		status = 200
	}
	switch {
	case err == nil:
		writeJSON(w, status, out)
	case errors.Is(err, collaborationworkflows.ErrInvalid):
		writeAPIError(w, 400, "workflow_activation_blocked", "workflow diagnostics must be resolved before activation")
	case errors.Is(err, collaborationworkflows.ErrConflict):
		writeAPIError(w, 409, "collaboration_workflow_conflict", "the workflow changed; reload before activating a revision")
	case errors.Is(err, collaborationworkflows.ErrNotFound):
		writeAPIError(w, 404, "collaboration_workflow_not_found", "collaboration workflow not found")
	case errors.Is(err, repositories.ErrInvalidCollaborator), errors.Is(err, repositories.ErrNotFound):
		writeAPIError(w, 403, "collaboration_workflow_forbidden", "workflow and step owners must be current repository participants")
	default:
		log.Printf("collaboration workflow storage: %v", err)
		writeAPIError(w, 500, "collaboration_workflows_unavailable", "collaboration workflow could not be persisted")
	}
}
