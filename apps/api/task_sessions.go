package main

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/changesessions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/relationships"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

// registerTaskChangeSessionRoutes connects proposal planning to the existing
// durable session protocol without manufacturing a placeholder pull request.
func registerTaskChangeSessionRoutes(mux *http.ServeMux, gitStore *storage.Store, catalog *repositories.Store, proposalStore *proposals.Store, pullStore *pullrequests.Store, sessionStore *changesessions.Store, authStore *auth.Store, relationStores ...*relationships.Store) {
	var relationStore *relationships.Store
	if len(relationStores) > 0 {
		relationStore = relationStores[0]
	}
	key := func(r *http.Request) (string, string, string) {
		return r.PathValue("id"), r.PathValue("proposal_id"), r.PathValue("task_id")
	}
	authorize := func(w http.ResponseWriter, r *http.Request, scope string) (auth.Credential, bool) {
		credential, _, ok := authorizeRepositoryParticipant(w, r, catalog, authStore, r.PathValue("id"), scope)
		return credential, ok
	}
	load := func(w http.ResponseWriter, r *http.Request) (proposals.Proposal, proposals.Task, bool) {
		repositoryID, proposalID, taskID := key(r)
		proposal, err := proposalStore.Get(repositoryID, proposalID)
		if writeProposalError(w, err) {
			return proposals.Proposal{}, proposals.Task{}, false
		}
		task, err := proposalStore.GetTask(repositoryID, proposalID, taskID)
		if writeProposalError(w, err) {
			return proposals.Proposal{}, proposals.Task{}, false
		}
		return proposal, task, true
	}

	type startInput struct {
		ExpectedAssignmentID string   `json:"expected_assignment_id"`
		ContextPaths         []string `json:"context_paths"`
		ExpiresIn            int64    `json:"expires_in"`
	}
	mux.HandleFunc("POST /repositories/{id}/proposals/{proposal_id}/tasks/{task_id}/sessions", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authorize(w, r, "repositories:write")
		if !ok {
			return
		}
		var input startInput
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_task_start", "task start input is invalid")
			return
		}
		if input.ExpiresIn == 0 {
			input.ExpiresIn = 3600
		}
		if input.ExpiresIn < 300 || input.ExpiresIn > 86400 {
			writeAPIError(w, 409, "task_not_startable", "task must be ready, open, and carry the expected agent assignment")
			return
		}
		if relationStore != nil {
			plan, link, findErr := relationStore.FindEvolutionMigrationTask(r.PathValue("id"), r.PathValue("task_id"))
			if findErr != nil && !errors.Is(findErr, relationships.ErrNotFound) {
				writeAPIError(w, 503, "migration_plan_unavailable", "migration dependencies could not be verified")
				return
			}
			if findErr == nil {
				completed := map[string]bool{}
				for _, candidate := range plan.MigrationTasks {
					if task, taskErr := proposalStore.GetTask(candidate.RepositoryID, candidate.ProposalID, candidate.TaskID); taskErr == nil {
						completed[candidate.ID] = task.Status == proposals.TaskCompleted
					}
				}
				for _, dependency := range link.DependencyIDs {
					if !completed[dependency] {
						writeAPIError(w, 409, "migration_dependencies_incomplete", "earlier migration tasks must merge before this agent session can start")
						return
					}
				}
			}
		}
		repositoryRecord, err := catalog.GetByID(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return
		}
		repository, err := gitStore.Open(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "internal_error", "repository storage unavailable")
			return
		}
		if len(input.ContextPaths) > 50 {
			writeAPIError(w, 400, "invalid_task_context", "at most 50 repository context paths may be selected")
			return
		}
		var session changesessions.Session
		var run changesessions.Run
		var issued auth.IssuedCredential
		var uncertain bool
		responseWritten := errors.New("task start response written")
		err = proposalStore.WithStartableAgentTask(r.PathValue("id"), r.PathValue("proposal_id"), r.PathValue("task_id"), input.ExpectedAssignmentID, func(proposal proposals.Proposal, task proposals.Task, allTasks []proposals.Task, comments []proposals.Comment) error {
			existing, listErr := sessionStore.List(r.PathValue("id"), task.ID)
			if listErr != nil {
				writeChangeSessionError(w, listErr)
				return responseWritten
			}
			for _, candidate := range existing {
				sameAssignment := candidate.TaskContext != nil && candidate.TaskContext.AssignmentID == task.Assignment.ID
				if candidate.TaskContext == nil || candidate.TaskContext.AssignmentID == "" {
					runs, runErr := sessionStore.ListRuns(r.PathValue("id"), task.ID, candidate.ID)
					if runErr != nil {
						writeChangeSessionError(w, runErr)
						return responseWritten
					}
					for _, existingRun := range runs {
						sameAssignment = sameAssignment || strings.HasSuffix(existingRun.WorkingBranch, "-"+task.Assignment.ID[:8])
					}
				}
				if sameAssignment {
					writeAPIError(w, 409, "task_session_exists", "this task assignment already has a change session")
					return responseWritten
				}
			}
			commit, readErr := repository.ReadCommit(storage.ObjectID(task.Assignment.Access.BaseRevision))
			if readErr != nil {
				writeAPIError(w, 409, "base_revision_unavailable", "assigned base revision is unavailable")
				return responseWritten
			}
			entries, walkErr := repository.WalkTree(commit.Tree)
			if walkErr != nil {
				writeAPIError(w, 500, "internal_error", "repository context is unavailable")
				return responseWritten
			}
			available := map[string]bool{}
			for _, entry := range entries {
				available[entry.Path] = true
			}
			if len(input.ContextPaths) == 0 {
				root, rootErr := repository.ReadTree(commit.Tree)
				if rootErr != nil {
					writeAPIError(w, 500, "internal_error", "repository context is unavailable")
					return responseWritten
				}
				for _, entry := range root {
					if len(input.ContextPaths) == 50 {
						break
					}
					input.ContextPaths = append(input.ContextPaths, entry.Name)
				}
			}
			seen := map[string]bool{}
			for i, selected := range input.ContextPaths {
				selected = strings.TrimSpace(selected)
				if selected == "" || !available[selected] || seen[selected] || len(selected) > 1000 {
					writeAPIError(w, 400, "invalid_task_context", "repository context paths must be unique existing paths")
					return responseWritten
				}
				seen[selected], input.ContextPaths[i] = true, selected
			}
			dependencies := []changesessions.TaskDependency{}
			for _, dependencyID := range task.DependencyIDs {
				for _, candidate := range allTasks {
					if candidate.ID == dependencyID {
						dependencies = append(dependencies, changesessions.TaskDependency{ID: candidate.ID, Title: candidate.Title, Outcome: candidate.Outcome, Status: candidate.Status})
					}
				}
			}
			discussion := []changesessions.TaskDiscussion{}
			for _, commentID := range task.DiscussionCommentIDs {
				for _, comment := range comments {
					if comment.ID == commentID {
						discussion = append(discussion, changesessions.TaskDiscussion{ID: comment.ID, AuthorID: comment.AuthorID, Body: comment.Body})
					}
				}
			}
			branch := "agent/tasks/" + task.ID + "-" + task.Assignment.ID[:8]
			refName := "refs/heads/" + branch
			if createRefErr := repository.CreateReference(storage.Reference{Name: refName, Target: task.Assignment.Access.BaseRevision}); createRefErr != nil {
				if errors.Is(createRefErr, storage.ErrReferenceExists) {
					writeAPIError(w, 409, "task_branch_exists", "the isolated task branch already exists")
					return responseWritten
				}
				writeAPIError(w, 500, "internal_error", "task branch could not be created")
				return responseWritten
			}
			var issueErr error
			issued, issueErr = authStore.IssueBound(actor.UserID, auth.Git, "Agent task "+task.ID, []string{"git:read", "git:write"}, time.Duration(input.ExpiresIn)*time.Second, r.PathValue("id"), refName)
			if issueErr != nil {
				_ = repository.DeleteReference(refName)
				writeAPIError(w, 500, "internal_error", "agent access could not be issued")
				return responseWritten
			}
			context := changesessions.TaskContext{AssignmentID: task.Assignment.ID, ContextRevision: task.ContextRevision, RepositoryName: repositoryRecord.Name, ProposalTitle: proposal.Title, ProposalBody: proposal.Body, TaskTitle: task.Title, TaskOutcome: task.Outcome, Mandate: task.Assignment.Mandate, Dependencies: dependencies, Discussion: discussion}
			var createErr error
			session, run, createErr = sessionStore.CreateForTaskWithRun(r.PathValue("id"), proposal.ID, task.ID, actor.UserID, task.Assignment.AssigneeID, task.Assignment.Access.BaseRevision, context, input.ContextPaths, branch, issued.ID, issued.ExpiresAt)
			if createErr != nil && !errors.Is(createErr, changesessions.ErrDurabilityUncertain) {
				_, _ = authStore.Revoke(actor.UserID, issued.ID)
				_ = repository.DeleteReference(refName)
				writeChangeSessionError(w, createErr)
				return responseWritten
			}
			uncertain = errors.Is(createErr, changesessions.ErrDurabilityUncertain)
			return nil
		})
		if errors.Is(err, responseWritten) {
			return
		}
		if errors.Is(err, proposals.ErrTaskAssignmentConflict) {
			writeAPIError(w, 409, "task_not_startable", "task must be ready, open, and carry the expected agent assignment")
			return
		}
		if writeProposalError(w, err) {
			return
		}
		response := map[string]any{"session": session, "run": run, "credential": issued}
		w.Header().Set("Location", r.URL.Path+"/"+session.ID)
		if uncertain {
			writeUncertainMutation(w, response)
			return
		}
		writeJSON(w, http.StatusCreated, response)
	})

	mux.HandleFunc("GET /repositories/{id}/proposals/{proposal_id}/tasks/{task_id}/sessions", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authorize(w, r, "repositories:read"); !ok {
			return
		}
		if _, _, ok := load(w, r); !ok {
			return
		}
		items, err := sessionStore.List(r.PathValue("id"), r.PathValue("task_id"))
		if writeChangeSessionError(w, err) {
			return
		}
		page, next, valid := paginate(r, items, func(item changesessions.Session) string { return item.ID })
		if !valid {
			writeAPIError(w, 400, "invalid_pagination", "limit or after is invalid")
			return
		}
		writeJSON(w, 200, map[string]any{"sessions": page, "next_cursor": next})
	})
	mux.HandleFunc("GET /repositories/{id}/proposals/{proposal_id}/tasks/{task_id}/sessions/{session_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authorize(w, r, "repositories:read"); !ok {
			return
		}
		if _, _, ok := load(w, r); !ok {
			return
		}
		session, err := sessionStore.Get(r.PathValue("id"), r.PathValue("task_id"), r.PathValue("session_id"))
		if writeChangeSessionError(w, err) {
			return
		}
		writeJSON(w, 200, session)
	})
	mux.HandleFunc("GET /repositories/{id}/proposals/{proposal_id}/tasks/{task_id}/sessions/{session_id}/events", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authorize(w, r, "repositories:read"); !ok {
			return
		}
		items, err := sessionStore.ListEvents(r.PathValue("id"), r.PathValue("task_id"), r.PathValue("session_id"))
		if writeChangeSessionError(w, err) {
			return
		}
		page, next, valid := paginate(r, items, func(item changesessions.Event) string { return item.ID })
		if !valid {
			writeAPIError(w, 400, "invalid_pagination", "limit or after is invalid")
			return
		}
		writeJSON(w, 200, map[string]any{"events": page, "next_cursor": next})
	})
	mux.HandleFunc("GET /repositories/{id}/proposals/{proposal_id}/tasks/{task_id}/sessions/{session_id}/runs", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := authorize(w, r, "repositories:read"); !ok {
			return
		}
		items, err := sessionStore.ListRuns(r.PathValue("id"), r.PathValue("task_id"), r.PathValue("session_id"))
		if writeChangeSessionError(w, err) {
			return
		}
		writeJSON(w, 200, map[string]any{"runs": items, "next_cursor": nil})
	})

	type interventionInput struct {
		Kind    string `json:"kind"`
		Message string `json:"message"`
	}
	mux.HandleFunc("POST /repositories/{id}/proposals/{proposal_id}/tasks/{task_id}/sessions/{session_id}/runs/{run_id}/interventions", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authorize(w, r, "repositories:write")
		if !ok {
			return
		}
		var input interventionInput
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_run_intervention", "run intervention is invalid")
			return
		}
		run, event, err := sessionStore.Intervene(r.PathValue("id"), r.PathValue("task_id"), r.PathValue("session_id"), r.PathValue("run_id"), actor.UserID, strings.TrimSpace(input.Kind), strings.TrimSpace(input.Message))
		if errors.Is(err, changesessions.ErrRunCanceled) || errors.Is(err, changesessions.ErrRunCompleted) || errors.Is(err, changesessions.ErrInvalid) {
			writeAPIError(w, 409, "invalid_run_transition", "intervention is invalid for the current run state")
			return
		}
		if err != nil && !errors.Is(err, changesessions.ErrDurabilityUncertain) {
			writeChangeSessionError(w, err)
			return
		}
		if input.Kind == "run.canceled" {
			_, _ = authStore.Revoke(run.InitiatorID, run.CredentialID)
		}
		response := map[string]any{"run": run, "event": event}
		if errors.Is(err, changesessions.ErrDurabilityUncertain) {
			writeUncertainMutation(w, response)
			return
		}
		writeJSON(w, http.StatusCreated, response)
	})

	type workInput struct {
		Kind     string `json:"kind"`
		State    string `json:"state"`
		Message  string `json:"message"`
		Tool     string `json:"tool"`
		Artifact string `json:"artifact"`
		Branch   string `json:"branch"`
		CommitID string `json:"commit_id"`
	}
	mux.HandleFunc("POST /repositories/{id}/proposals/{proposal_id}/tasks/{task_id}/sessions/{session_id}/runs/{run_id}/events", func(w http.ResponseWriter, r *http.Request) {
		credential, ok := authenticateRequest(w, r, authStore, "git:write", false)
		if !ok || credential.RepositoryID != r.PathValue("id") {
			return
		}
		var input workInput
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_agent_event", "agent event is invalid")
			return
		}
		appendEvent := func() (changesessions.Event, error) {
			return sessionStore.AppendWorkEvent(r.PathValue("id"), r.PathValue("task_id"), r.PathValue("session_id"), r.PathValue("run_id"), credential.ID, input.Kind, input.State, input.Message, input.Tool, input.Artifact, input.Branch, input.CommitID)
		}
		var event changesessions.Event
		var err error
		if input.Kind == "branch.updated" {
			repository, openErr := gitStore.Open(r.PathValue("id"))
			if openErr != nil {
				writeAPIError(w, 500, "internal_error", "repository storage unavailable")
				return
			}
			err = repository.WithReferenceTarget("refs/heads/"+input.Branch, input.CommitID, func() error { event, err = appendEvent(); return err })
		} else {
			event, err = appendEvent()
		}
		if errors.Is(err, changesessions.ErrRunPaused) {
			writeAPIError(w, 409, "agent_run_paused", "agent run is paused")
			return
		}
		if errors.Is(err, changesessions.ErrRunCanceled) {
			writeAPIError(w, 409, "agent_run_canceled", "agent run is canceled")
			return
		}
		if errors.Is(err, changesessions.ErrInvalid) {
			writeAPIError(w, 400, "invalid_agent_event", "agent event does not match the run mandate")
			return
		}
		if writeChangeSessionError(w, err) {
			return
		}
		writeJSON(w, http.StatusCreated, event)
	})
	type completionInput struct {
		Summary            string                 `json:"summary"`
		CommitID           string                 `json:"commit_id"`
		Checks             []changesessions.Check `json:"checks"`
		UnresolvedConcerns []string               `json:"unresolved_concerns"`
	}
	mux.HandleFunc("POST /repositories/{id}/proposals/{proposal_id}/tasks/{task_id}/sessions/{session_id}/runs/{run_id}/completion", func(w http.ResponseWriter, r *http.Request) {
		credential, ok := authenticateRequest(w, r, authStore, "git:write", false)
		if !ok || credential.RepositoryID != r.PathValue("id") {
			return
		}
		var input completionInput
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_run_completion", "run completion is invalid")
			return
		}
		input.Summary, input.CommitID = strings.TrimSpace(input.Summary), strings.TrimSpace(input.CommitID)
		run, _, err := sessionStore.GetRunControl(r.PathValue("id"), r.PathValue("task_id"), r.PathValue("session_id"), r.PathValue("run_id"), credential.ID)
		if errors.Is(err, changesessions.ErrNotFound) {
			writeAPIError(w, 404, "agent_run_not_found", "agent run not found")
			return
		}
		if writeChangeSessionError(w, err) {
			return
		}
		if pullStore == nil {
			writeAPIError(w, 500, "internal_error", "pull request storage unavailable")
			return
		}
		repository, err := gitStore.Open(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "internal_error", "repository storage unavailable")
			return
		}
		var completed changesessions.Run
		var event changesessions.Event
		complete := func() error {
			headHistory, historyErr := repository.ListCommitAncestry(storage.ObjectID(input.CommitID))
			if historyErr != nil {
				return changesessions.ErrInvalid
			}
			baseHistory, baseErr := repository.ListCommitAncestry(storage.ObjectID(run.SourceCommitID))
			if baseErr != nil {
				return baseErr
			}
			baseSet := map[storage.ObjectID]bool{}
			for _, commit := range baseHistory {
				baseSet[commit.ID] = true
			}
			containsBase, commits := false, []string{}
			for _, commit := range headHistory {
				if commit.ID == storage.ObjectID(run.SourceCommitID) {
					containsBase = true
				}
				if !baseSet[commit.ID] {
					commits = append(commits, string(commit.ID))
				}
			}
			if !containsBase || len(commits) == 0 {
				return changesessions.ErrInvalid
			}
			changes, changeErr := pullStore.CompareCommits(r.PathValue("id"), run.SourceCommitID, input.CommitID)
			if changeErr != nil {
				return changeErr
			}
			files := make([]changesessions.ChangedFile, len(changes))
			for i, change := range changes {
				files[i] = changesessions.ChangedFile{Path: change.Path, Status: change.Status}
			}
			completed, event, err = sessionStore.CompleteRun(r.PathValue("id"), r.PathValue("task_id"), run.SessionID, run.ID, credential.ID, input.Summary, input.CommitID, commits, files, input.Checks, input.UnresolvedConcerns)
			return err
		}
		err = repository.WithReferenceTarget("refs/heads/"+run.WorkingBranch, input.CommitID, complete)
		if completed.ID != "" {
			if _, revokeErr := authStore.Revoke(run.InitiatorID, credential.ID); revokeErr != nil && !errors.Is(revokeErr, auth.ErrNotFound) {
				writeAPIError(w, 500, "internal_error", "work was completed but agent access revocation must be retried")
				return
			}
			if revoked, revokeErr := sessionStore.RevokeRunAccess(r.PathValue("id"), r.PathValue("task_id"), run.SessionID, run.ID); revokeErr == nil || errors.Is(revokeErr, changesessions.ErrDurabilityUncertain) {
				completed = revoked
			} else {
				writeChangeSessionError(w, revokeErr)
				return
			}
		}
		if errors.Is(err, storage.ErrReferenceExists) || errors.Is(err, storage.ErrReferenceNotFound) || errors.Is(err, storage.ErrReferenceLocked) {
			writeAPIError(w, 409, "branch_tip_changed", "completion must identify the published branch tip")
			return
		}
		if errors.Is(err, changesessions.ErrRunPaused) {
			writeAPIError(w, 409, "agent_run_paused", "resume the run before publishing completion")
			return
		}
		if errors.Is(err, changesessions.ErrRunCanceled) || errors.Is(err, changesessions.ErrRunCompleted) {
			writeAPIError(w, 409, "agent_run_terminal", "agent run is already terminal")
			return
		}
		if errors.Is(err, changesessions.ErrInvalid) || errors.Is(err, storage.ErrInvalidReference) {
			writeAPIError(w, 400, "invalid_run_completion", "completion must identify new descendant commits and valid review evidence")
			return
		}
		response := map[string]any{"run": completed, "event": event}
		if errors.Is(err, changesessions.ErrDurabilityUncertain) {
			writeUncertainMutation(w, response)
			return
		}
		if writeChangeSessionError(w, err) {
			return
		}
		w.Header().Set("Location", strings.TrimSuffix(r.URL.Path, "/completion")+"#outcome")
		writeJSON(w, http.StatusCreated, response)
	})
	mux.HandleFunc("GET /repositories/{id}/proposals/{proposal_id}/tasks/{task_id}/sessions/{session_id}/runs/{run_id}/control", func(w http.ResponseWriter, r *http.Request) {
		credential, ok := authenticateRequest(w, r, authStore, "git:read", false)
		if !ok || credential.RepositoryID != r.PathValue("id") {
			return
		}
		run, interventions, err := sessionStore.GetRunControl(r.PathValue("id"), r.PathValue("task_id"), r.PathValue("session_id"), r.PathValue("run_id"), credential.ID)
		if writeChangeSessionError(w, err) {
			return
		}
		session, err := sessionStore.Get(r.PathValue("id"), r.PathValue("task_id"), r.PathValue("session_id"))
		if writeChangeSessionError(w, err) {
			return
		}
		writeJSON(w, 200, map[string]any{"run": run, "interventions": interventions, "task_context": session.TaskContext})
	})
}
