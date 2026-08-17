package main

import (
	"errors"
	"log"
	"net/http"
	"path"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/changesessions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/decisions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/durableschemas"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
)

func registerDurableSchemaRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, store *durableschemas.Store, pulls *pullrequests.Store, decisionStore *decisions.Store, proposalStore *proposals.Store, sessionStore *changesessions.Store, workspaceStore *workspaces.Store) {
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
	mux.HandleFunc("POST "+base+"/{schema_id}/migrations/{migration_id}/work", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
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
