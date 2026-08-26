package main

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/runbooks"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workflowcomponents"
)

type runbookInput struct {
	RequestID       string            `json:"request_id"`
	ExpectedVersion int               `json:"expected_version"`
	Revision        runbooks.Revision `json:"revision"`
}
type runbookRehearsalInput struct {
	RequestID              string              `json:"request_id"`
	RunbookVersion         int                 `json:"runbook_version"`
	EnvironmentKind        string              `json:"environment_kind"`
	EnvironmentID          string              `json:"environment_id"`
	PolicyApprovalRevision string              `json:"policy_approval_revision"`
	Scenarios              []runbooks.Scenario `json:"scenarios"`
}
type runbookExecutionInput struct {
	RequestID      string                    `json:"request_id"`
	RunbookVersion int                       `json:"runbook_version"`
	Context        runbooks.ExecutionContext `json:"context"`
	Preconditions  []runbooks.Preconditions  `json:"preconditions"`
	CurrentAccess  []string                  `json:"current_access"`
}
type runbookExecutionActionInput = runbooks.ExecutionAction
type runbookImprovementInput struct {
	RequestID    string    `json:"request_id"`
	FindingID    string    `json:"finding_id"`
	Kind         string    `json:"kind"`
	Title        string    `json:"title"`
	Outcome      string    `json:"outcome"`
	AssigneeType string    `json:"assignee_type"`
	AssigneeID   string    `json:"assignee_id"`
	BaseRevision string    `json:"base_revision"`
	DueAt        time.Time `json:"due_at"`
}

func registerRunbookRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, store *runbooks.Store, workflows *workflowcomponents.Store, orgs *organizations.Store, proposalStore *proposals.Store) {
	mux.HandleFunc("POST /repositories/{id}/runbook-recommendations", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:read"); !ok {
			return
		}
		var context runbooks.ExecutionContext
		if decodeJSON(r, &context) != nil {
			writeAPIError(w, 400, "invalid_runbook_context", "an exact bounded originating context is required")
			return
		}
		out, err := store.Recommend(r.PathValue("id"), context)
		if errors.Is(err, runbooks.ErrInvalid) {
			writeAPIError(w, 400, "invalid_runbook_context", "the origin, signal window, affected resources, and permitted evidence must be complete and credential-free")
			return
		}
		if err != nil {
			writeAPIError(w, 500, "runbooks_unavailable", "runbook recommendations could not be evaluated")
			return
		}
		writeJSON(w, 200, map[string]any{"recommendations": out})
	})
	mux.HandleFunc("GET /repositories/{id}/runbook-executions", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:read"); !ok {
			return
		}
		books, err := store.List(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "runbooks_unavailable", "runbook executions could not be read")
			return
		}
		executions := []runbooks.Execution{}
		for _, book := range books {
			executions = append(executions, book.Executions...)
		}
		writeJSON(w, 200, map[string]any{"executions": executions})
	})
	mux.HandleFunc("GET /repositories/{id}/runbook-executions/{execution_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:read"); !ok {
			return
		}
		books, err := store.List(r.PathValue("id"))
		if err == nil {
			for _, book := range books {
				for _, execution := range book.Executions {
					if execution.ID == r.PathValue("execution_id") {
						writeJSON(w, 200, execution)
						return
					}
				}
			}
		}
		writeAPIError(w, 404, "runbook_execution_not_found", "runbook execution not found")
	})
	mux.HandleFunc("POST /repositories/{id}/runbook-executions/{execution_id}/actions", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:read")
		if !ok {
			return
		}
		var in runbookExecutionActionInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_runbook_action", "a caller-stable version-bound action is required")
			return
		}
		books, err := store.List(r.PathValue("id"))
		var bookID string
		for _, book := range books {
			for _, execution := range book.Executions {
				if execution.ID == r.PathValue("execution_id") {
					bookID = book.ID
				}
			}
		}
		if bookID == "" {
			writeAPIError(w, 404, "runbook_execution_not_found", "runbook execution not found")
			return
		}
		actorType, actorID := "human", actor.UserID
		if actor.AgentID != "" {
			actorType, actorID = "agent", actor.AgentID
		}
		persist := func() error {
			var e error
			_, e = store.Act(bookID, r.PathValue("execution_id"), actorType, actorID, in)
			return e
		}
		if actorType == "agent" {
			if orgs == nil {
				err = organizations.ErrNotFound
			} else {
				err = orgs.WithCurrentAgentGrant(actor.OrganizationID, actor.AccessGrantID, actor.AgentID, r.PathValue("id"), persist)
			}
		} else {
			err = catalog.WithCurrentParticipant(actor.UserID, r.PathValue("id"), persist)
		}
		if err == nil {
			books, _ = store.List(r.PathValue("id"))
			for _, book := range books {
				for _, execution := range book.Executions {
					if execution.ID == r.PathValue("execution_id") {
						writeJSON(w, 201, execution)
						return
					}
				}
			}
		}
		switch {
		case errors.Is(err, runbooks.ErrNotFound):
			writeAPIError(w, 404, "runbook_execution_not_found", "runbook execution not found")
		case errors.Is(err, runbooks.ErrConflict):
			writeAPIError(w, 409, "runbook_action_conflict", "the execution moved, the retry changed, or the actor lacks exact control/delegation")
		case errors.Is(err, runbooks.ErrInvalid):
			writeAPIError(w, 400, "invalid_runbook_action", "the action, step, target, cost, or optional-step policy is invalid")
		default:
			writeAPIError(w, 403, "runbook_action_forbidden", "current participation or an exact current agent grant is required")
		}
	})
	mux.HandleFunc("POST /repositories/{id}/runbook-executions/{execution_id}/assessment", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		book, execution, found := findRunbookExecution(store, r.PathValue("id"), r.PathValue("execution_id"))
		if !found {
			writeAPIError(w, 404, "runbook_execution_not_found", "runbook execution not found")
			return
		}
		revision := book.Revisions[execution.RunbookVersion-1]
		if !containsRunbookOwner(revision.OwnerIDs, actor.UserID) {
			writeAPIError(w, 403, "runbook_assessment_forbidden", "only a current declared runbook owner can assess the outcome")
			return
		}
		var in runbooks.AssessmentInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_runbook_assessment", "a complete revision-bound outcome assessment is required")
			return
		}
		if in.SuspendCurrentUse {
			fallback, err := store.Get(in.FallbackRunbookID)
			approved := err == nil && fallback.ID != book.ID && fallback.RepositoryID == book.RepositoryID && fallback.CurrentVersion == in.FallbackRunbookVersion && fallback.UseStatus != "suspended"
			if approved {
				approved = false
				for _, x := range fallback.Rehearsals {
					approved = approved || x.RunbookVersion == in.FallbackRunbookVersion && x.Status == "passed" && !x.Stale
				}
			}
			if !approved {
				writeAPIError(w, 400, "invalid_runbook_fallback", "suspension requires an active exact fallback revision with a current passing rehearsal")
				return
			}
		}
		var out runbooks.Execution
		err := catalog.WithCurrentParticipant(actor.UserID, book.RepositoryID, func() error {
			var persistErr error
			out, persistErr = store.Assess(book.ID, execution.ID, actor.UserID, in)
			return persistErr
		})
		switch {
		case err == nil:
			writeJSON(w, 201, out)
		case errors.Is(err, runbooks.ErrConflict):
			writeAPIError(w, 409, "runbook_assessment_conflict", "the execution is not terminal, moved, or was already assessed differently")
		case errors.Is(err, runbooks.ErrInvalid):
			writeAPIError(w, 400, "invalid_runbook_assessment", "declared criteria, supported findings, deviations, and participant feedback must be complete and evidence-bound")
		case errors.Is(err, repositories.ErrInvalidCollaborator), errors.Is(err, repositories.ErrNotFound):
			writeAPIError(w, 403, "runbook_assessment_forbidden", "current repository participation is required through assessment persistence")
		default:
			writeAPIError(w, 500, "runbooks_unavailable", "the assessment could not be retained")
		}
	})
	mux.HandleFunc("POST /repositories/{id}/runbook-executions/{execution_id}/improvements", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		book, execution, found := findRunbookExecution(store, r.PathValue("id"), r.PathValue("execution_id"))
		if !found {
			writeAPIError(w, 404, "runbook_execution_not_found", "runbook execution not found")
			return
		}
		if !containsRunbookOwner(book.Revisions[execution.RunbookVersion-1].OwnerIDs, actor.UserID) || proposalStore == nil {
			writeAPIError(w, 403, "runbook_improvement_forbidden", "only a current declared runbook owner can create ordinary improvement work")
			return
		}
		var in runbookImprovementInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_runbook_improvement", "supported finding work is required")
			return
		}
		var finding *runbooks.ExecutionFinding
		if execution.Assessment != nil {
			for i := range execution.Assessment.Findings {
				if execution.Assessment.Findings[i].ID == in.FindingID {
					finding = &execution.Assessment.Findings[i]
				}
			}
		}
		if finding == nil || finding.Kind != in.Kind {
			writeAPIError(w, 400, "invalid_runbook_improvement", "work must cite one supported retained finding and preserve its change kind")
			return
		}
		var p proposals.Proposal
		var task proposals.Task
		var out runbooks.Execution
		err := catalog.WithCurrentParticipant(actor.UserID, book.RepositoryID, func() error {
			reserved, reserveErr := store.ReserveImprovement(book.ID, execution.ID, actor.UserID, in.RequestID, in.FindingID, in.Kind)
			if reserveErr != nil {
				return reserveErr
			}
			for _, retained := range reserved.Assessment.Findings {
				if retained.ID == in.FindingID && retained.ProposalID != "" {
					var readErr error
					p, readErr = proposalStore.Get(book.RepositoryID, retained.ProposalID)
					if readErr != nil {
						return readErr
					}
					task, readErr = proposalStore.GetTask(book.RepositoryID, retained.ProposalID, retained.TaskID)
					if readErr != nil {
						return readErr
					}
					out = reserved
					return nil
				}
			}
			var createErr error
			p, task, createErr = proposalStore.CreateCorrectiveWork(proposals.CorrectiveWorkInput{RunbookExecutionID: execution.ID, OperationID: in.RequestID, RepositoryID: book.RepositoryID, ActorID: actor.UserID, ProposalTitle: in.Title, ProposalBody: "Governed " + in.Kind + " improvement from runbook execution " + execution.ID + " and finding " + finding.ID + ". Ordinary review and delivery authority still apply.", TaskTitle: in.Title, Outcome: in.Outcome, AssigneeType: in.AssigneeType, AssigneeID: in.AssigneeID, BaseRevision: in.BaseRevision, DueAt: in.DueAt})
			if createErr != nil {
				if errors.Is(createErr, proposals.ErrInvalid) {
					_ = store.ReleaseImprovementReservation(book.ID, execution.ID, actor.UserID, in.RequestID, in.FindingID)
				}
				return createErr
			}
			out, createErr = store.LinkImprovement(book.ID, execution.ID, actor.UserID, runbooks.ImprovementLink{RequestID: in.RequestID, FindingID: in.FindingID, Kind: in.Kind, ProposalID: p.ID, TaskID: task.ID})
			return createErr
		})
		if err != nil {
			if errors.Is(err, repositories.ErrInvalidCollaborator) || errors.Is(err, repositories.ErrNotFound) {
				writeAPIError(w, 403, "runbook_improvement_forbidden", "current repository participation is required through corrective-work creation and finding linkage")
			} else {
				writeAPIError(w, 409, "runbook_improvement_conflict", "the finding is reserved, changed, already linked, or ordinary work could not be reconciled")
			}
			return
		}
		writeJSON(w, 201, map[string]any{"execution": out, "proposal": p, "task": task})
	})
	mux.HandleFunc("GET /repositories/{id}/runbooks", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		out, e := store.List(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "runbooks_unavailable", "runbooks could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"runbooks": out})
	})
	mux.HandleFunc("GET /repositories/{id}/runbooks/{runbook_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		out, e := store.Get(r.PathValue("runbook_id"))
		if e != nil || out.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "runbook_not_found", "runbook not found")
			return
		}
		writeJSON(w, 200, out)
	})
	mux.HandleFunc("POST /repositories/{id}/runbooks/{runbook_id}/executions", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:read")
		if !ok {
			return
		}
		book, err := store.Get(r.PathValue("runbook_id"))
		if err != nil || book.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "runbook_not_found", "runbook not found")
			return
		}
		var in runbookExecutionInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_runbook_execution", "a caller-stable exact context and explicit safety decisions are required")
			return
		}
		// Preconditions and resource access are caller observations, not authority.
		// Until an authoritative adapter verifies them, retain the execution with
		// explicit blockers instead of allowing request values to create readiness.
		out, err := store.StartExecution(book.ID, actor.UserID, in.RequestID, in.RunbookVersion, in.Context, nil, nil)
		switch {
		case err == nil:
			writeJSON(w, 201, out)
		case errors.Is(err, runbooks.ErrConflict):
			writeAPIError(w, 409, "runbook_execution_conflict", "the request identity conflicts or this origin already has an active execution")
		case errors.Is(err, runbooks.ErrInvalid):
			writeAPIError(w, 400, "invalid_runbook_execution", "the execution context is incomplete, unbounded, or secret-bearing")
		default:
			writeAPIError(w, 500, "runbooks_unavailable", "the runbook execution could not be retained")
		}
	})
	publish := func(revise bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
			if !ok {
				return
			}
			var in runbookInput
			if decodeJSON(r, &in) != nil {
				writeAPIError(w, 400, "invalid_request", "a caller-stable identity and complete runbook revision are required")
				return
			}
			resolveRunbookReferences(git, catalog, workflows, orgs, actor.UserID, r.PathValue("id"), &in.Revision)
			participants := append([]string{actor.UserID}, in.Revision.OwnerIDs...)
			for _, s := range in.Revision.Steps {
				participants = append(participants, s.OwnerIDs...)
			}
			for _, e := range in.Revision.Escalations {
				participants = append(participants, e.OwnerID)
			}
			var out runbooks.Runbook
			err := catalog.WithCurrentParticipants(participants, r.PathValue("id"), func() error {
				var e error
				if revise {
					current, x := store.Get(r.PathValue("runbook_id"))
					if x != nil || current.RepositoryID != r.PathValue("id") {
						return runbooks.ErrNotFound
					}
					out, e = store.Revise(current.ID, in.ExpectedVersion, actor.UserID, in.RequestID, in.Revision)
				} else {
					out, e = store.Create(r.PathValue("id"), actor.UserID, in.RequestID, in.Revision)
				}
				return e
			})
			status := 201
			if revise {
				status = 200
			}
			switch {
			case err == nil:
				writeJSON(w, status, out)
			case errors.Is(err, runbooks.ErrNotFound):
				writeAPIError(w, 404, "runbook_not_found", "runbook not found")
			case errors.Is(err, runbooks.ErrConflict):
				writeAPIError(w, 409, "runbook_conflict", "the request identity or expected version conflicts")
			case errors.Is(err, runbooks.ErrInvalid):
				writeAPIError(w, 400, "invalid_runbook", "the runbook revision is incomplete or invalid")
			case errors.Is(err, repositories.ErrInvalidCollaborator), errors.Is(err, repositories.ErrNotFound):
				writeAPIError(w, 400, "invalid_runbook", "owners and escalation recipients must be current repository participants")
			default:
				writeAPIError(w, 500, "runbooks_unavailable", "the retained runbook could not be read or written")
			}
		}
	}
	mux.HandleFunc("POST /repositories/{id}/runbooks", publish(false))
	mux.HandleFunc("POST /repositories/{id}/runbooks/{runbook_id}/revisions", publish(true))
	mux.HandleFunc("POST /repositories/{id}/runbooks/{runbook_id}/rehearsals", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:read")
		if !ok {
			return
		}
		current, err := store.Get(r.PathValue("runbook_id"))
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "runbook_not_found", "runbook not found")
			return
		}
		var in runbookRehearsalInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_rehearsal", "a bounded complete rehearsal is required")
			return
		}
		actorType, actorID := "human", actor.UserID
		if actor.AgentID != "" {
			actorType, actorID = "agent", actor.AgentID
		}
		var out runbooks.Runbook
		persist := func() error {
			var persistErr error
			out, persistErr = store.Rehearse(current.ID, actorType, actorID, in.RequestID, in.RunbookVersion, in.EnvironmentKind, in.EnvironmentID, in.PolicyApprovalRevision, in.Scenarios)
			return persistErr
		}
		if actorType == "agent" {
			if orgs == nil {
				err = organizations.ErrNotFound
			} else {
				err = orgs.WithCurrentAgentGrant(actor.OrganizationID, actor.AccessGrantID, actor.AgentID, r.PathValue("id"), persist)
			}
		} else {
			err = catalog.WithCurrentParticipant(actor.UserID, r.PathValue("id"), persist)
		}
		switch {
		case err == nil:
			writeJSON(w, 201, out)
		case errors.Is(err, runbooks.ErrConflict):
			writeAPIError(w, 409, "runbook_rehearsal_conflict", "the rehearsal request identity conflicts")
		case errors.Is(err, runbooks.ErrInvalid):
			writeAPIError(w, 400, "invalid_rehearsal", "the rehearsal must use bounded evidence, complete step outcomes, and simulate or exclude changing steps")
		case errors.Is(err, repositories.ErrInvalidCollaborator), errors.Is(err, repositories.ErrNotFound), errors.Is(err, organizations.ErrNotFound), errors.Is(err, organizations.ErrInvalid):
			writeAPIError(w, 403, "runbook_rehearsal_forbidden", "current repository participation or an exact current agent grant is required")
		default:
			writeAPIError(w, 500, "runbooks_unavailable", "the rehearsal could not be retained")
		}
	})
}

func findRunbookExecution(store *runbooks.Store, repositoryID, executionID string) (runbooks.Runbook, runbooks.Execution, bool) {
	books, err := store.List(repositoryID)
	if err != nil {
		return runbooks.Runbook{}, runbooks.Execution{}, false
	}
	for _, book := range books {
		for _, execution := range book.Executions {
			if execution.ID == executionID {
				return book, execution, true
			}
		}
	}
	return runbooks.Runbook{}, runbooks.Execution{}, false
}
func containsRunbookOwner(ids []string, id string) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

// resolveRunbookReferences ignores caller status flags and derives them from the
// immutable resource and its current authority source.
func resolveRunbookReferences(git *storage.Store, catalog *repositories.Store, workflows *workflowcomponents.Store, orgs *organizations.Store, actorID, repositoryID string, revision *runbooks.Revision) {
	repository, _ := catalog.Get(actorID, repositoryID)
	for i := range revision.Steps {
		for j := range revision.Steps[i].References {
			ref := &revision.Steps[i].References[j]
			ref.Accessible, ref.Reviewed, ref.Approved = false, false, false
			switch ref.Kind {
			case "command", "documentation":
				_, _, ref.Accessible = infrastructureCommitBlob(git, repositoryID, ref.Revision, ref.ResourceID)
				ref.Reviewed = ref.Accessible && ref.Kind == "command" && runbookRevisionPublished(git, repositoryID, repository.DefaultBranch, ref.Revision)
			case "workflow_component":
				if workflows != nil {
					component, err := workflows.Get(ref.ResourceID)
					ref.Accessible = err == nil && component.Definition.Version == ref.Revision
					ref.Reviewed = ref.Accessible && component.Attestation.DefinitionSHA256 != ""
				}
			case "agent":
				if orgs != nil && repository.OrganizationID != "" {
					organization, err := orgs.Get(repository.OrganizationID)
					if err != nil {
						continue
					}
					for _, agent := range organization.Agents {
						if agent.ID != ref.ResourceID || strconv.Itoa(agent.Version) != ref.Revision {
							continue
						}
						ref.Accessible = true
						for _, grant := range organization.AccessGrants {
							resource := organizations.ResourceScope{Kind: "repository", ID: repositoryID}
							if !runbookAgentGrantCurrent(grant, agent.ID, resource, time.Now().UTC()) {
								continue
							}
							for _, granted := range grant.Resources {
								if granted == resource {
									ref.Approved = true
								}
							}
						}
					}
				}
			}
		}
	}
}

func runbookAgentGrantCurrent(grant organizations.AccessGrant, agentID string, resource organizations.ResourceScope, now time.Time) bool {
	if grant.PrincipalType != "agent" || grant.PrincipalID != agentID || grant.RevokedAt != nil || (grant.ExpiresAt != nil && !grant.ExpiresAt.After(now)) {
		return false
	}
	for _, exception := range grant.Exceptions {
		if exception.Resource == resource {
			return false
		}
	}
	return true
}

func runbookRevisionPublished(git *storage.Store, repositoryID, defaultBranch, revision string) bool {
	repository, err := git.Open(repositoryID)
	if err != nil {
		return false
	}
	ref, err := repository.ReadReference("refs/heads/" + defaultBranch)
	if err != nil {
		return false
	}
	commits, err := repository.ListCommitAncestry(storage.ObjectID(ref.Target))
	if err != nil {
		return false
	}
	for _, commit := range commits {
		if string(commit.ID) == revision {
			return true
		}
	}
	return false
}
