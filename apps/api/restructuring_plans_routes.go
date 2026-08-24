package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os/exec"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/collaborationworkflows"
	docscollections "github.com/greptile-projects/vivarium-tuatara/apps/api/docscollections"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/federation"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/governance"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
	packageversions "github.com/greptile-projects/vivarium-tuatara/apps/api/packages"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/relationships"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/restructuringplans"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
)

func registerRestructuringPlanRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, plans *restructuringplans.Store, pulls *pullrequests.Store, issueStore *issues.Store, proposalStore *proposals.Store, releaseStore *releases.Store, packageStore *packageversions.Store, docs *docscollections.Store, policies *governance.Store, workspaceStore *workspaces.Store, workflows *collaborationworkflows.Store, consumers *relationships.Store, peers *federation.Store) {
	actorID := func(c auth.Credential) string {
		if c.AgentID != "" {
			return c.AgentID
		}
		return c.UserID
	}
	mux.HandleFunc("GET /repositories/{id}/restructuring-plans", func(w http.ResponseWriter, r *http.Request) {
		_, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		xs, e := plans.List(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "restructuring_plans_unavailable", "restructuring plans could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"restructuring_plans": xs})
	})
	mux.HandleFunc("GET /repositories/{id}/restructuring-plans/{plan_id}", func(w http.ResponseWriter, r *http.Request) {
		_, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		v, e := plans.Get(r.PathValue("id"), r.PathValue("plan_id"))
		if e != nil {
			writeAPIError(w, 404, "restructuring_plan_not_found", "restructuring plan not found")
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("POST /repositories/{id}/restructuring-plans", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if c.AgentID != "" {
			writeAPIError(w, 403, "restructuring_plan_agent_forbidden", "a human collaborator must open a restructuring plan")
			return
		}
		var in restructuringplans.Plan
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a restructuring plan is required")
			return
		}
		in.RepositoryID = r.PathValue("id")
		clean := in
		clean.ID = ""
		clean.RequestDigest = ""
		clean.CreatedBy = ""
		clean.CreatedAt = clean.CreatedAt.UTC()
		clean.Version = 0
		clean.Authority = ""
		clean.Findings = nil
		b, _ := json.Marshal(clean)
		sum := sha256.Sum256(b)
		digest := hex.EncodeToString(sum[:])
		if existing, found, reconcileErr := plans.Reconcile(in.RepositoryID, in.RequestID, digest); found {
			if errors.Is(reconcileErr, restructuringplans.ErrConflict) {
				writeAPIError(w, 409, "restructuring_plan_request_conflict", "request_id was already used for another plan")
				return
			}
			if reconcileErr != nil {
				writeAPIError(w, 500, "restructuring_plan_unavailable", "restructuring plan could not be reconciled")
				return
			}
			writeJSON(w, 200, existing)
			return
		} else if reconcileErr != nil {
			writeAPIError(w, 500, "restructuring_plan_unavailable", "restructuring plan could not be reconciled")
			return
		}
		for _, source := range in.Sources {
			if c.RepositoryID != "" && c.RepositoryID != source.RepositoryID {
				writeAPIError(w, 403, "restructuring_credential_forbidden", "a repository-bound credential cannot define another source repository")
				return
			}
			repo, e := catalog.GetByID(source.RepositoryID)
			if e != nil {
				writeAPIError(w, 422, "restructuring_source_missing", "every selected source repository must exist")
				return
			}
			participant, _ := catalog.HasCollaborator(c.UserID, source.RepositoryID)
			if repo.OwnerID != c.UserID && !participant {
				writeAPIError(w, 403, "restructuring_source_forbidden", "the creator must be a current collaborator in every source repository")
				return
			}
			gr, e := git.Open(source.RepositoryID)
			if e != nil || !gitCommitExists(gr.Path(), source.Revision) {
				writeAPIError(w, 422, "restructuring_revision_missing", "every source revision must resolve to an exact commit")
				return
			}
		}
		for _, item := range in.Inventory {
			if !restructuringInventoryCitationResolves(git, item, pulls, issueStore, proposalStore, releaseStore, packageStore, docs, policies, workspaceStore, workflows, consumers, peers) {
				writeAPIError(w, 422, "restructuring_inventory_revision_missing", "inventory citations must resolve at their retained source revision")
				return
			}
		}
		var out restructuringplans.Plan
		sourceIDs := make([]string, 0, len(in.Sources))
		for _, source := range in.Sources {
			sourceIDs = append(sourceIDs, source.RepositoryID)
		}
		e := catalog.WithCurrentParticipation(c.UserID, sourceIDs, func() error {
			var createErr error
			out, createErr = plans.Create(in, c.UserID, digest)
			return createErr
		})
		if errors.Is(e, restructuringplans.ErrConflict) {
			writeAPIError(w, 409, "restructuring_plan_request_conflict", "request_id was already used for another plan")
			return
		}
		if errors.Is(e, restructuringplans.ErrInvalid) {
			writeAPIError(w, 422, "restructuring_plan_invalid", "sources, destinations, mappings, all inventory kinds, owners, deadline, success criteria, and rollback limits are required")
			return
		}
		if errors.Is(e, repositories.ErrNotFound) || errors.Is(e, repositories.ErrInvalidCollaborator) {
			writeAPIError(w, 409, "restructuring_authority_changed", "source repository access changed before the plan was retained")
			return
		}
		if e != nil {
			writeAPIError(w, 500, "restructuring_plan_unavailable", "restructuring plan could not be opened")
			return
		}
		writeJSON(w, 201, out)
	})
	mux.HandleFunc("POST /repositories/{id}/restructuring-plans/{plan_id}/findings", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:read")
		if !ok {
			return
		}
		if c.AgentID != "" && c.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 403, "restructuring_agent_forbidden", "a read-only agent must be bound to the plan repository")
			return
		}
		var in restructuringplans.Finding
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a cited impact finding is required")
			return
		}
		kind := "human"
		if c.AgentID != "" {
			kind = "read_only_agent"
		}
		var out restructuringplans.Plan
		e := catalog.WithCurrentParticipant(c.UserID, r.PathValue("id"), func() error {
			var addErr error
			out, addErr = plans.AddFinding(r.PathValue("id"), r.PathValue("plan_id"), actorID(c), kind, in)
			return addErr
		})
		if errors.Is(e, restructuringplans.ErrNotFound) {
			writeAPIError(w, 404, "restructuring_plan_not_found", "restructuring plan not found")
			return
		}
		if errors.Is(e, restructuringplans.ErrConflict) || errors.Is(e, restructuringplans.ErrVersion) {
			writeAPIError(w, 409, "restructuring_plan_changed", "the plan or request changed; refresh before adding the finding")
			return
		}
		if errors.Is(e, restructuringplans.ErrInvalid) {
			writeAPIError(w, 422, "restructuring_finding_invalid", "findings require the current version, affected inventory items, bounded prose, and citations")
			return
		}
		if errors.Is(e, repositories.ErrNotFound) || errors.Is(e, repositories.ErrInvalidCollaborator) {
			writeAPIError(w, 409, "restructuring_authority_changed", "repository participation changed before the finding was retained")
			return
		}
		if e != nil {
			writeAPIError(w, 500, "restructuring_plan_unavailable", "the finding could not be retained")
			return
		}
		writeJSON(w, 201, out)
	})
}

func gitCommitExists(path, revision string) bool {
	if len(revision) != 40 {
		return false
	}
	out, e := exec.Command("git", "--git-dir="+path, "rev-parse", "--verify", revision+"^{commit}").Output()
	return e == nil && strings.TrimSpace(string(out)) == revision
}
func restructuringInventoryCitationResolves(git *storage.Store, item restructuringplans.InventoryItem, pulls *pullrequests.Store, issueStore *issues.Store, proposalStore *proposals.Store, releaseStore *releases.Store, packageStore *packageversions.Store, docs *docscollections.Store, policies *governance.Store, workspaceStore *workspaces.Store, workflows *collaborationworkflows.Store, consumers *relationships.Store, peers *federation.Store) bool {
	gr, e := git.Open(item.RepositoryID)
	if e != nil || !gitCommitExists(gr.Path(), item.Revision) {
		return false
	}
	if item.State != "resolved" {
		return true
	}
	switch item.Kind {
	case "ref":
		out, e := exec.Command("git", "--git-dir="+gr.Path(), "rev-parse", "--verify", item.ResourceID+"^{commit}").Output()
		return e == nil && strings.TrimSpace(string(out)) == item.Revision
	case "pull_request":
		v, e := pulls.Get(item.RepositoryID, item.ResourceID)
		return e == nil && v.SourceCommitID == item.Revision
	case "issue":
		v, e := issueStore.Get(item.RepositoryID, item.ResourceID)
		return e == nil && v.RepositoryID == item.RepositoryID
	case "task":
		parts := strings.Split(item.ResourceID, "/")
		if len(parts) != 2 {
			return false
		}
		_, e := proposalStore.GetTask(item.RepositoryID, parts[0], parts[1])
		return e == nil
	case "release":
		v, e := releaseStore.Get(item.RepositoryID, item.ResourceID)
		return e == nil && v.CommitID == item.Revision
	case "package":
		parts := strings.SplitN(item.ResourceID, "@", 2)
		if len(parts) != 2 {
			return false
		}
		v, e := packageStore.Get(parts[0], parts[1])
		return e == nil && v.RepositoryID == item.RepositoryID && v.SourceCommit == item.Revision
	case "documentation":
		xs, e := docs.List(item.RepositoryID, item.ResourceID)
		if e != nil {
			return false
		}
		for _, v := range xs {
			if v.SourceRevision == item.Revision {
				return true
			}
		}
		return false
	case "policy":
		v, e := policies.Get(item.ResourceID)
		return e == nil && v.ScopeType == "repository" && v.ScopeID == item.RepositoryID && v.Source.Revision == item.Revision
	case "workspace":
		v, e := workspaceStore.Get(item.ResourceID)
		return e == nil && v.RepositoryID == item.RepositoryID && v.CommitID == item.Revision
	case "automation":
		v, e := workflows.Get(item.ResourceID)
		if e != nil || v.RepositoryID != item.RepositoryID {
			return false
		}
		for _, revision := range v.Revisions {
			if revision.Source.Revision == item.Revision {
				return true
			}
		}
		return false
	case "consumer":
		xs, e := consumers.ListDependencies(item.RepositoryID)
		if e != nil {
			return false
		}
		for _, v := range xs {
			if v.ID == item.ResourceID && v.CommitID == item.Revision {
				return true
			}
		}
		return false
	case "federated_relationship":
		_, e := peers.Get(item.ResourceID)
		return e == nil
	}
	return false
}
