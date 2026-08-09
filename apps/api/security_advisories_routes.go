package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/activities"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/securityadvisories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

type createdDisclosureRef struct {
	repository *storage.Repository
	name       string
}

func orderedVerificationRuns(runs []checkruns.Run, definitions []checkruns.Definition, commitID string) ([]checkruns.Run, bool) {
	ordered := make([]checkruns.Run, len(definitions))
	used := map[string]bool{}
	for i, definition := range definitions {
		found := false
		for _, run := range runs {
			if !used[run.ID] && run.CommitID == commitID && reflect.DeepEqual(run.Definition, definition) {
				ordered[i] = run
				used[run.ID] = true
				found = true
				break
			}
		}
		if !found {
			return nil, false
		}
	}
	return ordered, true
}

func rollbackDisclosureRefs(advisoryID string, refs []createdDisclosureRef) {
	for i := len(refs) - 1; i >= 0; i-- {
		if err := refs[i].repository.DeleteReference(refs[i].name); err != nil && !errors.Is(err, storage.ErrReferenceNotFound) {
			log.Printf("roll back disclosure ref %s for advisory %s: %v", refs[i].name, advisoryID, err)
		}
	}
}

// trustedRepairCheckDefinitions freezes executable required-check properties
// from the task base. Repair commits are only the snapshot under test; they do
// not get to redefine the checks that attest to them.
func trustedRepairCheckDefinitions(repository *storage.Repository, baseCommitID string, requiredNames []string) ([]checkruns.Definition, error) {
	if len(requiredNames) == 0 {
		return []checkruns.Definition{}, nil
	}
	body, err := exec.Command("git", "--git-dir="+repository.Path(), "show", baseCommitID+":"+checkruns.ConfigPath).Output()
	if err != nil {
		return nil, fmt.Errorf("read trusted required checks: %w", err)
	}
	config, err := checkruns.ParseConfig(body)
	if err != nil {
		return nil, fmt.Errorf("parse trusted required checks: %w", err)
	}
	wanted := map[string]bool{}
	for _, name := range requiredNames {
		wanted[name] = true
	}
	definitions := make([]checkruns.Definition, 0, len(requiredNames))
	for _, definition := range config.Checks {
		if wanted[definition.Name] {
			definitions = append(definitions, definition)
			delete(wanted, definition.Name)
		}
	}
	if len(wanted) != 0 {
		return nil, errors.New("trusted base is missing a required-check definition")
	}
	return definitions, nil
}

func commitInReferenceAncestry(repository *storage.Repository, referenceName, commitID string) bool {
	reference, err := repository.ReadReference(referenceName)
	if err != nil {
		return false
	}
	if reference.Target == commitID {
		return true
	}
	ancestry, err := repository.ListCommitAncestry(storage.ObjectID(reference.Target))
	if err != nil {
		return false
	}
	for _, commit := range ancestry {
		if string(commit.ID) == commitID {
			return true
		}
	}
	return false
}

func registerSecurityAdvisoryRoutes(mux *http.ServeMux, gitStore *storage.Store, repos *repositories.Store, identities *users.Store, store *securityadvisories.Store, releasesStore *releases.Store, builds *checkruns.Store, deploymentsStore *deployments.Store, credentials *auth.Store, activityStore *activities.Store) {
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
	// Public reads contain only the disclosure packet assembled by maintainers;
	// protected evidence and even unpublished advisory existence stay hidden.
	mux.HandleFunc("GET /security-advisories/public", func(w http.ResponseWriter, r *http.Request) {
		all, err := store.List()
		if err != nil {
			writeAPIError(w, 500, "security_advisory_read_failed", "security advisories could not be read")
			return
		}
		items := []securityadvisories.Advisory{}
		for _, v := range all {
			if v.Disclosure != nil && v.Disclosure.State == "published" {
				items = append(items, securityadvisories.Advisory{ID: v.ID, Title: v.Disclosure.PublicTitle, Severity: v.Severity, EmbargoState: v.EmbargoState, Disclosure: v.Disclosure, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt})
			}
		}
		page, next, valid := paginate(r, items, func(v securityadvisories.Advisory) string { return v.ID })
		if !valid {
			writeAPIError(w, 400, "invalid_pagination", "limit or after is invalid")
			return
		}
		writeJSON(w, 200, map[string]any{"security_advisories": page, "next_cursor": next})
	})
	mux.HandleFunc("GET /security-advisories/public/{advisory_id}", func(w http.ResponseWriter, r *http.Request) {
		v, err := store.Get(r.PathValue("advisory_id"))
		if err != nil || v.Disclosure == nil || v.Disclosure.State != "published" {
			writeAPIError(w, 404, "security_advisory_not_found", "security advisory not found")
			return
		}
		writeJSON(w, 200, securityadvisories.Advisory{ID: v.ID, Title: v.Disclosure.PublicTitle, Severity: v.Severity, EmbargoState: v.EmbargoState, Disclosure: v.Disclosure, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt})
	})

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
		repositoryRecord, e := repos.GetByID(in.RepositoryID)
		if e != nil || !commitInReferenceAncestry(repository, "refs/heads/"+repositoryRecord.DefaultBranch, in.BaseCommitID) {
			writeAPIError(w, 422, "invalid_repair_task", "base commit must belong to the owner-controlled default-branch history")
			return
		}
		v, task, e := store.AddRepairTask(current.ID, actor.UserID, in)
		if e != nil {
			writeStoreError(w, e)
			return
		}
		writeJSON(w, 201, map[string]any{"security_advisory": v, "repair_task": task})
	})
	mux.HandleFunc("POST /security-advisories/{advisory_id}/reproductions", func(w http.ResponseWriter, r *http.Request) {
		actor, current, ok := require(w, r)
		if !ok {
			return
		}
		var in securityadvisories.SecurityReproduction
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		repository, err := repos.GetByID(in.RepositoryID)
		if err != nil || repository.OwnerID != actor.UserID {
			writeAPIError(w, 403, "repository_owner_required", "the repair repository owner must define private reproductions")
			return
		}
		v, reproduction, err := store.AddSecurityReproduction(current.ID, actor.UserID, in)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, 201, map[string]any{"security_advisory": v, "reproduction": reproduction})
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
	mux.HandleFunc("POST /security-advisories/{advisory_id}/repair-sessions/{session_id}/verifications", func(w http.ResponseWriter, r *http.Request) {
		actor, current, ok := require(w, r)
		if !ok {
			return
		}
		var session *securityadvisories.RepairSession
		var task *securityadvisories.RepairTask
		for i := range current.RepairSessions {
			if current.RepairSessions[i].ID == r.PathValue("session_id") {
				session = &current.RepairSessions[i]
			}
		}
		if session != nil {
			for i := range current.RepairTasks {
				if current.RepairTasks[i].ID == session.TaskID {
					task = &current.RepairTasks[i]
				}
			}
		}
		if session == nil || task == nil {
			writeAPIError(w, 404, "repair_session_not_found", "repair session not found")
			return
		}
		if session.State != "completed" {
			writeAPIError(w, 409, "repair_not_completed", "the exact repair candidate must be completed before verification")
			return
		}
		for _, existing := range current.RepairVerifications {
			if existing.SessionID == session.ID && existing.CandidateCommitID == session.CommitID {
				writeAPIError(w, 409, "repair_verification_exists", "this exact repair candidate already has verification evidence")
				return
			}
		}
		if !repositoryParticipant(actor.UserID, task.RepositoryID) {
			writeAPIError(w, 403, "repair_session_access_denied", "verification requires current repair repository participation")
			return
		}
		if builds == nil {
			writeAPIError(w, 503, "repair_verification_unavailable", "verification execution is unavailable")
			return
		}
		repository, err := gitStore.Open(task.RepositoryID)
		if err != nil {
			writeAPIError(w, 503, "repair_verification_unavailable", "repair repository is unavailable")
			return
		}
		requiredNames, err := repos.RequiredChecks(task.RepositoryID, "main")
		if err != nil {
			writeAPIError(w, 503, "repair_verification_unavailable", "required-check policy is unavailable")
			return
		}
		requiredDefinitions, err := trustedRepairCheckDefinitions(repository, task.BaseCommitID, requiredNames)
		if err != nil {
			writeAPIError(w, 422, "required_checks_unavailable", "the frozen repair base does not provide every trusted required-check definition")
			return
		}
		reproductionDefinitions := []checkruns.Definition{}
		for _, reproduction := range current.SecurityReproductions {
			if reproduction.RepositoryID == task.RepositoryID && reproduction.VersionLine == task.VersionLine {
				reproductionDefinitions = append(reproductionDefinitions, reproduction.Definition)
			}
		}
		if len(reproductionDefinitions) == 0 {
			writeAPIError(w, 422, "security_reproduction_missing", "the affected version line requires a private security reproduction")
			return
		}
		definitions := append(append([]checkruns.Definition{}, requiredDefinitions...), reproductionDefinitions...)
		executableRuns, err := builds.CreateRequested(task.RepositoryID, session.ID, session.CommitID, definitions, actor.UserID)
		if err != nil {
			writeAPIError(w, 500, "repair_verification_failed", "verification evidence could not be reserved")
			return
		}
		persistedRuns, err := builds.List(task.RepositoryID, session.ID)
		if err != nil {
			writeAPIError(w, 500, "repair_verification_failed", "verification evidence could not be reopened")
			return
		}
		runs, complete := orderedVerificationRuns(persistedRuns, definitions, session.CommitID)
		if !complete {
			writeAPIError(w, 500, "repair_verification_failed", "verification evidence reservation is incomplete")
			return
		}
		requiredIDs := make([]string, len(requiredDefinitions))
		for i := range requiredDefinitions {
			requiredIDs[i] = runs[i].ID
		}
		reproductionIDs := make([]string, len(reproductionDefinitions))
		for i := range reproductionDefinitions {
			reproductionIDs[i] = runs[len(requiredDefinitions)+i].ID
		}
		v, verification, err := store.StartRepairVerification(current.ID, actor.UserID, task.ID, session.ID, requiredIDs, reproductionIDs)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		for _, run := range executableRuns {
			go builds.Execute(run, repository.Path())
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"security_advisory": v, "verification": verification})
	})

	verificationRuns := func(current securityadvisories.Advisory, verificationID string) (securityadvisories.RepairVerification, []checkruns.Run, bool) {
		if builds == nil {
			return securityadvisories.RepairVerification{}, nil, false
		}
		for _, verification := range current.RepairVerifications {
			if verification.ID != verificationID {
				continue
			}
			runs, err := builds.List(verification.RepositoryID, verification.SessionID)
			if err != nil {
				return verification, nil, false
			}
			return verification, runs, true
		}
		return securityadvisories.RepairVerification{}, nil, false
	}
	mux.HandleFunc("GET /security-advisories/{advisory_id}/verifications/{verification_id}", func(w http.ResponseWriter, r *http.Request) {
		_, current, ok := require(w, r)
		if !ok {
			return
		}
		verification, runs, found := verificationRuns(current, r.PathValue("verification_id"))
		if !found {
			writeAPIError(w, 404, "repair_verification_not_found", "repair verification not found")
			return
		}
		type safeRun struct {
			ID        string               `json:"id"`
			Name      string               `json:"name"`
			Kind      string               `json:"kind"`
			State     string               `json:"state"`
			CommitID  string               `json:"commit_id"`
			Artifacts []checkruns.Artifact `json:"artifacts"`
		}
		reproduction := map[string]bool{}
		for _, id := range verification.ReproductionRunIDs {
			reproduction[id] = true
		}
		projected := make([]safeRun, 0, len(runs))
		state := "passed"
		if len(runs) != len(verification.RequiredRunIDs)+len(verification.ReproductionRunIDs) {
			state = "pending"
		}
		for _, run := range runs {
			kind := "required_check"
			if reproduction[run.ID] {
				kind = "security_reproduction"
			}
			projected = append(projected, safeRun{run.ID, run.Definition.Name, kind, run.State, run.CommitID, run.Artifacts})
			if run.State == "failed" || run.State == "canceled" {
				state = "failed"
			} else if run.State != "succeeded" && state != "failed" {
				state = "pending"
			}
		}
		if state == "passed" && len(verification.Approvals) == 0 {
			state = "awaiting_approval"
		} else if state == "passed" {
			state = "integration_ready"
		}
		writeJSON(w, 200, map[string]any{"verification": verification, "state": state, "runs": projected})
	})
	mux.HandleFunc("POST /security-advisories/{advisory_id}/verifications/{verification_id}/approvals", func(w http.ResponseWriter, r *http.Request) {
		actor, current, ok := require(w, r)
		if !ok {
			return
		}
		verification, runs, found := verificationRuns(current, r.PathValue("verification_id"))
		if !found {
			writeAPIError(w, 404, "repair_verification_not_found", "repair verification not found")
			return
		}
		repository, err := repos.GetByID(verification.RepositoryID)
		if err != nil || repository.OwnerID != actor.UserID {
			writeAPIError(w, 403, "repository_owner_required", "the repair repository owner must approve exact verification evidence")
			return
		}
		if len(runs) != len(verification.RequiredRunIDs)+len(verification.ReproductionRunIDs) {
			writeAPIError(w, 409, "repair_verification_incomplete", "verification evidence is incomplete")
			return
		}
		for _, run := range runs {
			if run.CommitID != verification.CandidateCommitID || run.State != "succeeded" {
				writeAPIError(w, 409, "repair_verification_incomplete", "every exact-candidate check and reproduction must pass before approval")
				return
			}
		}
		v, saved, err := store.ApproveRepairVerification(current.ID, actor.UserID, verification.ID)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, 200, map[string]any{"security_advisory": v, "verification": saved})
	})
	mux.HandleFunc("POST /security-advisories/{advisory_id}/verifications/{verification_id}/release-attestations", func(w http.ResponseWriter, r *http.Request) {
		actor, current, ok := require(w, r)
		if !ok {
			return
		}
		verification, runs, found := verificationRuns(current, r.PathValue("verification_id"))
		if !found {
			writeAPIError(w, 404, "repair_verification_not_found", "repair verification not found")
			return
		}
		repositoryRecord, err := repos.GetByID(verification.RepositoryID)
		if err != nil || repositoryRecord.OwnerID != actor.UserID {
			writeAPIError(w, 403, "repository_owner_required", "the repair repository owner must attest a fixed release")
			return
		}
		if len(verification.Approvals) == 0 {
			writeAPIError(w, 409, "repair_verification_unapproved", "verification requires independent approval")
			return
		}
		if releasesStore == nil || builds == nil {
			writeAPIError(w, 503, "fixed_release_unavailable", "release attestation is unavailable")
			return
		}
		for _, run := range runs {
			if run.State != "succeeded" {
				writeAPIError(w, 409, "repair_verification_incomplete", "verification is no longer complete")
				return
			}
		}
		var in struct {
			ReleaseID string `json:"release_id"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		release, err := releasesStore.Get(verification.RepositoryID, in.ReleaseID)
		if err != nil {
			writeAPIError(w, 422, "fixed_release_unverified", "release candidate could not be verified")
			return
		}
		repository, err := gitStore.Open(verification.RepositoryID)
		if err != nil {
			writeAPIError(w, 503, "fixed_release_unavailable", "release repository is unavailable")
			return
		}
		ancestry, err := repository.ListCommitAncestry(storage.ObjectID(release.CommitID))
		contains := release.CommitID == verification.CandidateCommitID
		for _, commit := range ancestry {
			if string(commit.ID) == verification.CandidateCommitID {
				contains = true
			}
		}
		if err != nil || !contains {
			writeAPIError(w, 422, "fixed_release_unverified", "release commit does not contain the verified repair candidate")
			return
		}
		buildRuns, err := builds.List(verification.RepositoryID, release.ID)
		if err != nil || len(buildRuns) == 0 {
			writeAPIError(w, 422, "fixed_release_unverified", "release has no successful attested build")
			return
		}
		artifactIDs, hashes := []string{}, []string{}
		for _, run := range buildRuns {
			if run.CommitID != release.CommitID || run.State != "succeeded" {
				writeAPIError(w, 422, "fixed_release_unverified", "every exact release build step must pass")
				return
			}
			for _, artifact := range run.Artifacts {
				artifactIDs = append(artifactIDs, artifact.ID)
				hashes = append(hashes, artifact.SHA256)
			}
		}
		if len(artifactIDs) == 0 {
			writeAPIError(w, 422, "fixed_release_unverified", "release build produced no attested artifact")
			return
		}
		v, attestation, err := store.AddReleaseAttestation(current.ID, actor.UserID, securityadvisories.ReleaseAttestation{VerificationID: verification.ID, RepositoryID: verification.RepositoryID, VersionLine: verification.VersionLine, ReleaseID: release.ID, ReleaseCommitID: release.CommitID, ArtifactIDs: artifactIDs, ArtifactSHA256: hashes})
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, 201, map[string]any{"security_advisory": v, "release_attestation": attestation})
	})

	mux.HandleFunc("POST /security-advisories/{advisory_id}/disclosure", func(w http.ResponseWriter, r *http.Request) {
		actor, current, ok := require(w, r)
		if !ok {
			return
		}
		if !maintainer(actor.UserID, current) {
			writeAPIError(w, 403, "maintainer_required", "an affected repository owner must prepare disclosure")
			return
		}
		var input struct {
			ExpectedVersion int        `json:"expected_version"`
			PublicTitle     string     `json:"public_title"`
			RedactedSummary string     `json:"redacted_summary"`
			UpgradeGuidance string     `json:"upgrade_guidance"`
			Credits         []string   `json:"credits"`
			ScheduledAt     *time.Time `json:"scheduled_at"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		v, err := store.PrepareDisclosure(current.ID, actor.UserID, input.ExpectedVersion, securityadvisories.Disclosure{PublicTitle: input.PublicTitle, RedactedSummary: input.RedactedSummary, UpgradeGuidance: input.UpgradeGuidance, Credits: input.Credits, ScheduledAt: input.ScheduledAt})
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, 201, v)
	})

	mux.HandleFunc("POST /security-advisories/{advisory_id}/disclosure/publish", func(w http.ResponseWriter, r *http.Request) {
		actor, current, ok := require(w, r)
		if !ok {
			return
		}
		if !maintainer(actor.UserID, current) {
			writeAPIError(w, 403, "maintainer_required", "an affected repository owner must publish disclosure")
			return
		}
		if current.Disclosure == nil {
			writeAPIError(w, 422, "invalid_security_advisory", "prepare disclosure first")
			return
		}
		if current.Disclosure.ScheduledAt != nil && current.Disclosure.ScheduledAt.After(time.Now().UTC()) {
			writeAPIError(w, 409, "disclosure_not_due", "scheduled disclosure is not due")
			return
		}
		current, err := store.SetDisclosureState(current.ID, actor.UserID, "publishing", "", current.Disclosure.Remaining)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		remaining := []string{"publish_repaired_branches", "publish_advisory", "notify_affected_users"}
		createdRefs := []createdDisclosureRef{}
		rollbackRefs := func() { rollbackDisclosureRefs(current.ID, createdRefs) }
		// Stage only transport-hidden refs before disclosure. Even if cleanup is
		// impossible, ordinary Git readers cannot discover this namespace.
		for _, fix := range current.Disclosure.FixedVersions {
			repository, openErr := gitStore.Open(fix.RepositoryID)
			if openErr != nil {
				rollbackRefs()
				_, _ = store.SetDisclosureState(current.ID, actor.UserID, "paused", "repaired repository unavailable", remaining)
				writeAPIError(w, 503, "disclosure_paused", "publication paused; repaired repository unavailable")
				return
			}
			name := "refs/heads/vivarium-security/disclosures/" + current.ID + "/" + strings.TrimPrefix(fix.Branch, "security/")
			refErr := repository.CreateReference(storage.Reference{Name: name, Target: fix.CommitID})
			if refErr == nil {
				createdRefs = append(createdRefs, createdDisclosureRef{repository: repository, name: name})
			} else {
				existing, readErr := repository.ReadReference(name)
				if readErr != nil || existing.Target != fix.CommitID {
					rollbackRefs()
					_, _ = store.SetDisclosureState(current.ID, actor.UserID, "paused", "repaired branch could not be published", remaining)
					writeAPIError(w, 503, "disclosure_paused", "publication paused; repaired branch could not be published")
					return
				}
			}
		}
		remaining = remaining[1:]
		published, err := store.SetDisclosureState(current.ID, actor.UserID, "published", "", remaining)
		if err != nil {
			rollbackRefs()
			_, _ = store.SetDisclosureState(current.ID, actor.UserID, "paused", "public advisory state could not be saved", remaining)
			writeStoreError(w, err)
			return
		}
		remaining = remaining[1:]
		// The advisory is durably public before any public ref or recipient event.
		// From here onward failures retain public availability and retryable work;
		// an embargo can no longer truthfully be restored.
		for _, fix := range current.Disclosure.FixedVersions {
			repository, openErr := gitStore.Open(fix.RepositoryID)
			if openErr != nil {
				published, _ = store.SetDisclosureState(current.ID, actor.UserID, "published", "public repaired branch remains unpublished", []string{"publish_repaired_branches", "notify_affected_users"})
				writeAPIError(w, 503, "disclosure_incomplete", "advisory is public; repaired branch publication remains")
				return
			}
			name := "refs/heads/" + fix.Branch
			if refErr := repository.CreateReference(storage.Reference{Name: name, Target: fix.CommitID}); refErr != nil {
				existing, readErr := repository.ReadReference(name)
				if readErr != nil || existing.Target != fix.CommitID {
					published, _ = store.SetDisclosureState(current.ID, actor.UserID, "published", "public repaired branch remains unpublished", []string{"publish_repaired_branches", "notify_affected_users"})
					writeAPIError(w, 503, "disclosure_incomplete", "advisory is public; repaired branch publication remains")
					return
				}
			}
		}
		rollbackRefs()
		if activityStore != nil {
			recipients := map[string]bool{}
			for _, scope := range current.AffectedRepositories {
				repo, e := repos.GetByID(scope.RepositoryID)
				if e == nil {
					recipients[repo.OwnerID] = true
					if subscribers, listErr := repos.ListCollaborators(repo.OwnerID, repo.ID); listErr == nil {
						for _, subscriber := range subscribers {
							recipients[subscriber.UserID] = true
						}
					}
				}
				if deploymentsStore != nil {
					if promotions, e := deploymentsStore.ListPromotions(scope.RepositoryID); e == nil {
						for _, p := range promotions {
							recipients[p.InitiatedBy] = true
						}
					}
				}
			}
			for recipient := range recipients {
				target := recipient
				fix := current.Disclosure.FixedVersions[0]
				repo, _ := repos.GetByID(fix.RepositoryID)
				_, e := activityStore.AppendOnce("security-disclosure:"+current.ID+":"+recipient, activities.Event{Kind: "security_advisory_published", ActorID: actor.UserID, RepositoryID: fix.RepositoryID, RepositoryName: repo.Name, ResourceType: "security_advisory", ResourceID: current.ID, ResourceTitle: current.Disclosure.PublicTitle, TargetUserID: &target})
				if e != nil {
					published, _ = store.SetDisclosureState(current.ID, actor.UserID, "published", "notifications remain unpublished", []string{"notify_affected_users"})
					writeAPIError(w, 503, "disclosure_incomplete", "advisory is public; notifications remain unpublished")
					return
				}
			}
		}
		published, err = store.SetDisclosureState(current.ID, actor.UserID, "published", "", []string{})
		if err != nil {
			writeStoreError(w, err)
			return
		}
		writeJSON(w, 200, published)
	})
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
