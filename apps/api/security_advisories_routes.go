package main

import (
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/securityadvisories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func registerSecurityAdvisoryRoutes(mux *http.ServeMux, gitStore *storage.Store, repos *repositories.Store, identities *users.Store, store *securityadvisories.Store, releasesStore *releases.Store, builds *checkruns.Store, deploymentsStore *deployments.Store, credentials *auth.Store) {
	maintainer := func(userID string, v securityadvisories.Advisory) bool {
		for _, affected := range v.AffectedRepositories {
			repo, err := repos.GetByID(affected.RepositoryID)
			if err == nil && repo.OwnerID == userID {
				return true
			}
		}
		return false
	}
	visible := func(userID string, v securityadvisories.Advisory) bool {
		return userID == v.ReporterID || slices.Contains(v.ResponseTeam, userID) || maintainer(userID, v)
	}
	repositoryParticipant := func(userID, repositoryID string) bool {
		repository, err := repos.GetByID(repositoryID)
		if err != nil {
			return false
		}
		if repository.OwnerID == userID {
			return true
		}
		allowed, err := repos.HasCollaborator(userID, repositoryID)
		return err == nil && allowed
	}
	require := func(w http.ResponseWriter, r *http.Request) (auth.Credential, securityadvisories.Advisory, bool) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return actor, securityadvisories.Advisory{}, false
		}
		v, err := store.Get(r.PathValue("advisory_id"))
		if err != nil || !visible(actor.UserID, v) {
			writeAPIError(w, http.StatusNotFound, "security_advisory_not_found", "security advisory not found")
			return actor, v, false
		}
		return actor, v, true
	}
	writeStoreError := func(w http.ResponseWriter, err error) {
		switch {
		case errors.Is(err, securityadvisories.ErrConflict):
			writeAPIError(w, 409, "security_advisory_changed", "security advisory changed")
		case errors.Is(err, securityadvisories.ErrInvalid):
			writeAPIError(w, 422, "invalid_security_advisory", "security advisory input is invalid")
		case errors.Is(err, securityadvisories.ErrNotFound):
			writeAPIError(w, 404, "security_advisory_not_found", "security advisory not found")
		default:
			writeAPIError(w, 500, "security_advisory_write_failed", "security advisory could not be saved")
		}
	}

	mux.HandleFunc("GET /security-advisories", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		all, err := store.List()
		if err != nil {
			writeAPIError(w, 500, "security_advisory_read_failed", "security advisories could not be read")
			return
		}
		items := make([]securityadvisories.Advisory, 0)
		for _, item := range all {
			if visible(actor.UserID, item) {
				items = append(items, item)
			}
		}
		page, next, valid := paginate(r, items, func(v securityadvisories.Advisory) string { return v.ID })
		if !valid {
			writeAPIError(w, 400, "invalid_pagination", "limit or after is invalid")
			return
		}
		writeJSON(w, 200, map[string]any{"security_advisories": page, "next_cursor": next})
	})

	mux.HandleFunc("POST /security-advisories", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		var input securityadvisories.Advisory
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		for _, affected := range input.AffectedRepositories {
			repo, err := repos.GetByID(affected.RepositoryID)
			if err != nil {
				writeAPIError(w, 404, "repository_not_found", "repository not found")
				return
			}
			allowed := repo.Visibility == repositories.Public || repo.OwnerID == actor.UserID
			if !allowed {
				allowed, _ = repos.HasCollaborator(actor.UserID, repo.ID)
			}
			if !allowed {
				writeAPIError(w, 404, "repository_not_found", "repository not found")
				return
			}
		}
		input.ReporterID = actor.UserID
		created, err := store.Create(input)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, created)
	})

	mux.HandleFunc("GET /security-advisories/{advisory_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := require(w, r)
		if !ok {
			return
		}
		v, err := store.RecordAccess(r.PathValue("advisory_id"), actor.UserID)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, 200, v)
	})

	mux.HandleFunc("PATCH /security-advisories/{advisory_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, current, ok := require(w, r)
		if !ok {
			return
		}
		if !maintainer(actor.UserID, current) {
			writeAPIError(w, 403, "maintainer_required", "an affected repository owner must triage this report")
			return
		}
		var input struct {
			ExpectedVersion int    `json:"expected_version"`
			Severity        string `json:"severity"`
			EmbargoState    string `json:"embargo_state"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		v, err := store.Triage(current.ID, actor.UserID, input.ExpectedVersion, input.Severity, input.EmbargoState)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, 200, v)
	})

	mux.HandleFunc("POST /security-advisories/{advisory_id}/responders", func(w http.ResponseWriter, r *http.Request) {
		actor, current, ok := require(w, r)
		if !ok {
			return
		}
		if !maintainer(actor.UserID, current) {
			writeAPIError(w, 403, "maintainer_required", "an affected repository owner must invite responders")
			return
		}
		var input struct {
			UserID string `json:"user_id"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		if _, err := identities.Get(input.UserID); err != nil {
			writeAPIError(w, 422, "invalid_responder", "responder does not exist")
			return
		}
		v, err := store.Invite(current.ID, actor.UserID, input.UserID)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, v)
	})

	mux.HandleFunc("POST /security-advisories/{advisory_id}/messages", func(w http.ResponseWriter, r *http.Request) {
		actor, current, ok := require(w, r)
		if !ok {
			return
		}
		var input struct {
			Body string `json:"body"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		v, err := store.AddMessage(current.ID, actor.UserID, input.Body)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, v)
	})

	// Evidence links are verified against the authoritative workflow stores before
	// their immutable identity is copied into the embargoed record.
	mux.HandleFunc("POST /security-advisories/{advisory_id}/evidence", func(w http.ResponseWriter, r *http.Request) {
		actor, current, ok := require(w, r)
		if !ok {
			return
		}
		var in securityadvisories.Evidence
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		affected := false
		for _, x := range current.AffectedRepositories {
			if x.RepositoryID == in.RepositoryID {
				affected = true
			}
		}
		if !affected {
			writeAPIError(w, 422, "invalid_advisory_evidence", "evidence must belong to an affected repository")
			return
		}
		valid := false
		unavailable := false
		switch in.Kind {
		case "commit":
			if repo, e := gitStore.Open(in.RepositoryID); e == nil {
				_, e = repo.ReadCommit(storage.ObjectID(in.CommitID))
				valid = e == nil
			}
		case "release":
			if releasesStore == nil {
				unavailable = true
			} else {
				_, e := releasesStore.Get(in.RepositoryID, in.ReleaseID)
				valid = e == nil
			}
		case "build":
			if builds == nil {
				unavailable = true
			} else {
				_, e := builds.Get(in.RepositoryID, in.ReleaseID, in.BuildID)
				valid = e == nil
			}
		case "artifact":
			if builds == nil {
				unavailable = true
				break
			}
			if run, e := builds.Get(in.RepositoryID, in.ReleaseID, in.BuildID); e == nil {
				for _, a := range run.Artifacts {
					if a.ID == in.ArtifactID {
						valid = true
					}
				}
			}
		case "deployment":
			if deploymentsStore == nil {
				unavailable = true
			} else {
				_, e := deploymentsStore.GetPromotion(in.RepositoryID, in.DeploymentID)
				valid = e == nil
			}
		case "dependency":
			if builds == nil {
				unavailable = true
				break
			}
			if run, e := builds.Get(in.RepositoryID, in.ReleaseID, in.BuildID); e == nil {
				valid = in.Dependency == run.Definition.Image
			}
		}
		if unavailable {
			writeAPIError(w, http.StatusServiceUnavailable, "advisory_evidence_unavailable", "evidence verification is temporarily unavailable")
			return
		}
		if !valid {
			writeAPIError(w, 422, "invalid_advisory_evidence", "evidence could not be verified")
			return
		}
		v, e := store.AddEvidence(current.ID, actor.UserID, in)
		if e != nil {
			writeStoreError(w, e)
			return
		}
		writeJSON(w, 201, v)
	})
	mux.HandleFunc("POST /security-advisories/{advisory_id}/findings", func(w http.ResponseWriter, r *http.Request) {
		actor, current, ok := require(w, r)
		if !ok {
			return
		}
		var in struct {
			Kind        string   `json:"kind"`
			Statement   string   `json:"statement"`
			EvidenceIDs []string `json:"evidence_ids"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		v, e := store.AddFinding(current.ID, actor.UserID, in.Kind, in.Statement, "", in.EvidenceIDs)
		if e != nil {
			writeStoreError(w, e)
			return
		}
		writeJSON(w, 201, v)
	})
	mux.HandleFunc("PUT /security-advisories/{advisory_id}/impact", func(w http.ResponseWriter, r *http.Request) {
		actor, current, ok := require(w, r)
		if !ok {
			return
		}
		var in struct {
			ExpectedVersion int `json:"expected_version"`
			securityadvisories.Impact
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		affected := false
		for _, x := range current.AffectedRepositories {
			if x.RepositoryID == in.RepositoryID {
				affected = true
			}
		}
		if !affected {
			writeAPIError(w, 422, "invalid_security_advisory", "impact must name an affected repository")
			return
		}
		v, e := store.SetImpact(current.ID, actor.UserID, in.ExpectedVersion, in.Impact)
		if e != nil {
			writeStoreError(w, e)
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("POST /security-advisories/{advisory_id}/investigations", func(w http.ResponseWriter, r *http.Request) {
		actor, current, ok := require(w, r)
		if !ok {
			return
		}
		var in struct {
			Mandate     string   `json:"mandate"`
			EvidenceIDs []string `json:"evidence_ids"`
			ExpiresIn   int      `json:"expires_in"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		if in.ExpiresIn < 300 || in.ExpiresIn > 86400 {
			writeAPIError(w, 422, "invalid_investigation", "expiry must be between 5 minutes and 24 hours")
			return
		}
		issued, e := credentials.Issue(actor.UserID, auth.API, "Security advisory investigation", []string{"security:investigate"}, time.Duration(in.ExpiresIn)*time.Second)
		if e != nil {
			writeAPIError(w, 500, "investigation_start_failed", "read-only access could not be issued")
			return
		}
		v, x, e := store.StartInvestigation(current.ID, actor.UserID, issued.ID, issued.ID, in.Mandate, in.EvidenceIDs)
		if e != nil {
			_, _ = credentials.Revoke(actor.UserID, issued.ID)
			writeStoreError(w, e)
			return
		}
		writeJSON(w, 201, map[string]any{"security_advisory": v, "investigation": x, "credential": issued})
	})
	mux.HandleFunc("POST /security-advisories/{advisory_id}/repair-tasks", func(w http.ResponseWriter, r *http.Request) {
		actor, current, ok := require(w, r)
		if !ok {
			return
		}
		var in securityadvisories.RepairTask
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		if !visible(in.AssigneeID, current) {
			writeAPIError(w, 422, "invalid_repair_task", "assignee must be on the response team")
			return
		}
		if !repositoryParticipant(in.AssigneeID, in.RepositoryID) {
			writeAPIError(w, 422, "invalid_repair_task", "assignee must currently participate in the repair repository")
			return
		}
		repository, e := gitStore.Open(in.RepositoryID)
		if e != nil {
			writeAPIError(w, 422, "invalid_repair_task", "repair repository is unavailable")
			return
		}
		if _, e = repository.ReadCommit(storage.ObjectID(in.BaseCommitID)); e != nil {
			writeAPIError(w, 422, "invalid_repair_task", "base commit could not be verified")
			return
		}
		v, task, e := store.AddRepairTask(current.ID, actor.UserID, in)
		if e != nil {
			writeStoreError(w, e)
			return
		}
		writeJSON(w, 201, map[string]any{"security_advisory": v, "repair_task": task})
	})
	mux.HandleFunc("POST /security-advisories/{advisory_id}/repair-tasks/{task_id}/sessions", func(w http.ResponseWriter, r *http.Request) {
		actor, current, ok := require(w, r)
		if !ok {
			return
		}
		var task *securityadvisories.RepairTask
		for i := range current.RepairTasks {
			if current.RepairTasks[i].ID == r.PathValue("task_id") {
				task = &current.RepairTasks[i]
			}
		}
		if task == nil {
			writeAPIError(w, 404, "repair_task_not_found", "repair task not found")
			return
		}
		if !repositoryParticipant(actor.UserID, task.RepositoryID) {
			writeAPIError(w, 403, "repair_assignee_required", "repair access requires current participation in the task repository")
			return
		}
		var in struct {
			ExpiresIn int `json:"expires_in"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		if in.ExpiresIn == 0 {
			in.ExpiresIn = 3600
		}
		if in.ExpiresIn < 300 || in.ExpiresIn > 86400 {
			writeAPIError(w, 422, "invalid_repair_session", "expiry must be between 5 minutes and 24 hours")
			return
		}
		branch := "refs/heads/vivarium-security/" + current.ID + "/" + task.ID
		repository, e := gitStore.Open(task.RepositoryID)
		if e != nil {
			writeAPIError(w, 500, "repair_session_failed", "repository is unavailable")
			return
		}
		if e = repository.CreateReference(storage.Reference{Name: branch, Target: task.BaseCommitID}); e != nil {
			writeAPIError(w, 409, "repair_session_exists", "an isolated repair branch already exists")
			return
		}
		issued, e := credentials.IssueBound(actor.UserID, auth.Git, "Embargoed repair", []string{"git:read", "git:write"}, time.Duration(in.ExpiresIn)*time.Second, task.RepositoryID, branch)
		if e != nil {
			_ = repository.DeleteReference(branch)
			writeAPIError(w, 500, "repair_session_failed", "scoped Git access could not be issued")
			return
		}
		v, session, e := store.StartRepairSession(current.ID, actor.UserID, task.ID, issued.ID, branch)
		if e != nil {
			_, _ = credentials.Revoke(actor.UserID, issued.ID)
			_ = repository.DeleteReference(branch)
			writeStoreError(w, e)
			return
		}
		writeJSON(w, 201, map[string]any{"security_advisory": v, "repair_session": session, "credential": issued})
	})
	mutateRepair := func(action string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			actor, current, ok := require(w, r)
			if !ok {
				return
			}
			var in struct {
				Body     string `json:"body"`
				Decision string `json:"decision"`
				CommitID string `json:"commit_id"`
			}
			if decodeJSON(r, &in) != nil {
				writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
				return
			}
			var existing *securityadvisories.RepairSession
			for i := range current.RepairSessions {
				if current.RepairSessions[i].ID == r.PathValue("session_id") {
					existing = &current.RepairSessions[i]
				}
			}
			if existing == nil {
				writeAPIError(w, 404, "repair_session_not_found", "repair session not found")
				return
			}
			var taskCreator string
			for _, task := range current.RepairTasks {
				if task.ID == existing.TaskID {
					taskCreator = task.CreatedBy
					break
				}
			}
			creatorAuthorized := actor.UserID == taskCreator && repositoryParticipant(actor.UserID, existing.RepositoryID)
			if actor.UserID != existing.WorkerID && actor.UserID != existing.InitiatorID && !creatorAuthorized {
				writeAPIError(w, 403, "repair_session_access_denied", "only the worker, initiator, or authorized task creator may update this session")
				return
			}
			if action == "complete" {
				repository, e := gitStore.Open(existing.RepositoryID)
				if e != nil {
					writeAPIError(w, 500, "repair_session_failed", "repository is unavailable")
					return
				}
				ref, e := repository.ReadReference(existing.Branch)
				if e != nil || ref.Target != in.CommitID {
					writeAPIError(w, 409, "repair_commit_changed", "completion must name the exact live repair branch commit")
					return
				}
				ancestry, e := repository.ListCommitAncestry(storage.ObjectID(in.CommitID))
				descendant := e == nil
				if in.CommitID != existing.BaseCommitID {
					descendant = false
					for _, c := range ancestry {
						if string(c.ID) == existing.BaseCommitID {
							descendant = true
						}
					}
				}
				if !descendant || in.CommitID == existing.BaseCommitID {
					writeAPIError(w, 422, "invalid_repair_commit", "repair commit must descend from the frozen base")
					return
				}
			}
			if action == "revoke" {
				// Revoke access before removing the ref so a partial failure cannot
				// leave a usable credential for repository work the advisory treats
				// as revoked. A retry completes any remaining cleanup.
				if _, e := credentials.Revoke(existing.InitiatorID, existing.CredentialID); e != nil && !errors.Is(e, auth.ErrNotFound) {
					writeAPIError(w, 500, "repair_revocation_failed", "repair access could not be revoked")
					return
				}
				repository, e := gitStore.Open(existing.RepositoryID)
				if e != nil {
					writeAPIError(w, 500, "repair_revocation_failed", "repair repository is unavailable")
					return
				}
				if e = repository.DeleteReference(existing.Branch); e != nil && !errors.Is(e, storage.ErrReferenceNotFound) {
					writeAPIError(w, 500, "repair_revocation_failed", "repair branch could not be removed")
					return
				}
			}
			v, session, e := store.UpdateRepairSession(current.ID, actor.UserID, existing.ID, action, in.Body, in.Decision, in.CommitID)
			if e != nil {
				writeStoreError(w, e)
				return
			}
			if action == "complete" {
				_, _ = credentials.Revoke(existing.InitiatorID, existing.CredentialID)
			}
			writeJSON(w, 200, map[string]any{"security_advisory": v, "repair_session": session})
		}
	}
	mux.HandleFunc("POST /security-advisories/{advisory_id}/repair-sessions/{session_id}/comments", mutateRepair("comment"))
	mux.HandleFunc("POST /security-advisories/{advisory_id}/repair-sessions/{session_id}/reviews", mutateRepair("review"))
	mux.HandleFunc("POST /security-advisories/{advisory_id}/repair-sessions/{session_id}/complete", mutateRepair("complete"))
	mux.HandleFunc("POST /security-advisories/{advisory_id}/repair-sessions/{session_id}/revoke", mutateRepair("revoke"))
	mux.HandleFunc("GET /security-advisories/{advisory_id}/investigations/{investigation_id}", func(w http.ResponseWriter, r *http.Request) {
		credential, ok := authenticateRequest(w, r, credentials, "security:investigate", false)
		if !ok {
			return
		}
		_, x, e := store.Investigation(r.PathValue("advisory_id"), r.PathValue("investigation_id"), credential.ID)
		if e != nil {
			writeAPIError(w, 404, "investigation_not_found", "investigation not found")
			return
		}
		writeJSON(w, 200, map[string]any{"investigation": x, "evidence": x.Evidence})
	})
	mux.HandleFunc("POST /security-advisories/{advisory_id}/investigations/{investigation_id}/findings", func(w http.ResponseWriter, r *http.Request) {
		credential, ok := authenticateRequest(w, r, credentials, "security:investigate", false)
		if !ok {
			return
		}
		_, x, e := store.Investigation(r.PathValue("advisory_id"), r.PathValue("investigation_id"), credential.ID)
		if e != nil {
			writeAPIError(w, 404, "investigation_not_found", "investigation not found")
			return
		}
		var in struct {
			Kind        string   `json:"kind"`
			Statement   string   `json:"statement"`
			EvidenceIDs []string `json:"evidence_ids"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		for _, id := range in.EvidenceIDs {
			selected := false
			for _, evidence := range x.Evidence {
				if evidence.ID == id {
					selected = true
					break
				}
			}
			if !selected {
				writeAPIError(w, 422, "invalid_security_advisory", "finding evidence was not delegated")
				return
			}
		}
		v, e := store.AddFinding(r.PathValue("advisory_id"), x.AgentID, in.Kind, in.Statement, x.ID, in.EvidenceIDs)
		if e != nil {
			writeStoreError(w, e)
			return
		}
		writeJSON(w, 201, map[string]any{"finding": v.Findings[len(v.Findings)-1]})
	})
}
