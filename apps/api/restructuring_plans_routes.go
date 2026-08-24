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
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
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

func registerRestructuringPlanRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, organizationsStore *organizations.Store, plans *restructuringplans.Store, pulls *pullrequests.Store, issueStore *issues.Store, proposalStore *proposals.Store, releaseStore *releases.Store, packageStore *packageversions.Store, docs *docscollections.Store, policies *governance.Store, workspaceStore *workspaces.Store, workflows *collaborationworkflows.Store, consumers *relationships.Store, peers *federation.Store) {
	registerRestructuringCandidateRoutes(mux, git, catalog, credentials, plans)
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
		clean.CandidateSets = nil
		clean.CollaborationMappings = nil
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
		resolvedOwners := make(map[string][]string, len(in.Inventory))
		for _, item := range in.Inventory {
			owners, resolveErr := restructuringInventoryOwners(item, catalog, pulls, issueStore, proposalStore, releaseStore, packageStore, docs, policies, workspaceStore, workflows, consumers)
			if resolveErr != nil || len(owners) == 0 || !sameOwnerSet(item.OwnerIDs, owners) {
				writeAPIError(w, 422, "restructuring_inventory_owners_invalid", "inventory owners must exactly match the authoritative source resource participants")
				return
			}
			resolvedOwners[item.ID] = owners
		}
		var out restructuringplans.Plan
		sourceIDs := make([]string, 0, len(in.Sources))
		for _, source := range in.Sources {
			sourceIDs = append(sourceIDs, source.RepositoryID)
		}
		e := catalog.WithCurrentParticipation(c.UserID, sourceIDs, func() error {
			var createErr error
			out, createErr = plans.CreateResolved(in, c.UserID, digest, resolvedOwners)
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
		if errors.Is(e, repositories.ErrNotFound) || errors.Is(e, repositories.ErrInvalidCollaborator) || errors.Is(e, organizations.ErrNotFound) || errors.Is(e, organizations.ErrInvalid) {
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
		persist := func() error {
			var addErr error
			out, addErr = plans.AddFinding(r.PathValue("id"), r.PathValue("plan_id"), actorID(c), kind, in)
			return addErr
		}
		var e error
		if c.AgentID != "" && c.OrganizationID != "" && c.AccessGrantID != "" {
			e = organizationsStore.WithCurrentAgentGrant(c.OrganizationID, c.AccessGrantID, c.AgentID, r.PathValue("id"), func() error {
				return catalog.WithCurrentRepositories([]string{r.PathValue("id")}, func(repositories []repositories.Repository) error {
					if len(repositories) != 1 || repositories[0].OrganizationID != c.OrganizationID {
						return organizations.ErrNotFound
					}
					return persist()
				})
			})
		} else {
			e = catalog.WithCurrentParticipant(c.UserID, r.PathValue("id"), persist)
		}
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
		if errors.Is(e, repositories.ErrNotFound) || errors.Is(e, repositories.ErrInvalidCollaborator) || errors.Is(e, organizations.ErrNotFound) || errors.Is(e, organizations.ErrInvalid) {
			writeAPIError(w, 409, "restructuring_authority_changed", "repository participation changed before the finding was retained")
			return
		}
		if e != nil {
			writeAPIError(w, 500, "restructuring_plan_unavailable", "the finding could not be retained")
			return
		}
		writeJSON(w, 201, out)
	})
	mux.HandleFunc("POST /repositories/{id}/restructuring-plans/{plan_id}/collaboration-mappings", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if c.AgentID != "" {
			writeAPIError(w, 403, "restructuring_mapping_agent_forbidden", "a human participant must propose active collaboration mappings")
			return
		}
		var in restructuringplans.CollaborationMapping
		var body struct {
			ExpectedVersion int                                     `json:"expected_version"`
			Mapping         restructuringplans.CollaborationMapping `json:"mapping"`
		}
		if decodeJSON(r, &body) != nil {
			writeAPIError(w, 400, "invalid_request", "an exact collaboration mapping is required")
			return
		}
		in = body.Mapping
		var out restructuringplans.Plan
		err := catalog.WithCurrentParticipant(c.UserID, r.PathValue("id"), func() error {
			var e error
			out, e = plans.AddCollaborationMapping(r.PathValue("id"), r.PathValue("plan_id"), c.UserID, body.ExpectedVersion, in)
			return e
		})
		if errors.Is(err, restructuringplans.ErrVersion) || errors.Is(err, restructuringplans.ErrConflict) {
			writeAPIError(w, 409, "restructuring_mapping_changed", "the plan or mapping changed; refresh before retrying")
			return
		}
		if errors.Is(err, restructuringplans.ErrInvalid) {
			writeAPIError(w, 422, "restructuring_mapping_invalid", "mapping must bind an inventoried resource and exact revision, retained intent, authors, destinations, dependencies, and acceptance criteria")
			return
		}
		if errors.Is(err, restructuringplans.ErrNotFound) {
			writeAPIError(w, 404, "restructuring_plan_not_found", "restructuring plan not found")
			return
		}
		if err != nil {
			writeAPIError(w, 500, "restructuring_mapping_unavailable", "collaboration mapping could not be retained")
			return
		}
		writeJSON(w, 201, out)
	})
	mux.HandleFunc("POST /repositories/{id}/restructuring-plans/{plan_id}/collaboration-mappings/{mapping_id}/decisions", func(w http.ResponseWriter, r *http.Request) {
		c, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:read")
		if !ok {
			return
		}
		if c.AgentID != "" {
			writeAPIError(w, 403, "restructuring_decision_agent_forbidden", "only a named human source participant can decide a mapping")
			return
		}
		var body struct {
			ExpectedVersion int                                `json:"expected_version"`
			Decision        restructuringplans.MappingDecision `json:"decision"`
		}
		if decodeJSON(r, &body) != nil {
			writeAPIError(w, 400, "invalid_request", "a revision-bound decision is required")
			return
		}
		var out restructuringplans.Plan
		err := catalog.WithCurrentParticipant(c.UserID, r.PathValue("id"), func() error {
			var e error
			out, e = plans.DecideCollaborationMapping(r.PathValue("id"), r.PathValue("plan_id"), r.PathValue("mapping_id"), c.UserID, body.ExpectedVersion, body.Decision)
			return e
		})
		if errors.Is(err, restructuringplans.ErrVersion) || errors.Is(err, restructuringplans.ErrConflict) {
			writeAPIError(w, 409, "restructuring_mapping_changed", "the plan, request, or source revision changed")
			return
		}
		if errors.Is(err, restructuringplans.ErrInvalid) {
			writeAPIError(w, 422, "restructuring_decision_invalid", "the decision must come from a retained author and cite the exact source revision")
			return
		}
		if errors.Is(err, restructuringplans.ErrNotFound) {
			writeAPIError(w, 404, "restructuring_mapping_not_found", "collaboration mapping not found")
			return
		}
		if err != nil {
			writeAPIError(w, 500, "restructuring_mapping_unavailable", "mapping decision could not be retained")
			return
		}
		writeJSON(w, 201, out)
	})
}

func restructuringInventoryOwners(item restructuringplans.InventoryItem, catalog *repositories.Store, pulls *pullrequests.Store, issueStore *issues.Store, proposalStore *proposals.Store, releaseStore *releases.Store, packageStore *packageversions.Store, docs *docscollections.Store, policies *governance.Store, workspaceStore *workspaces.Store, workflows *collaborationworkflows.Store, consumers *relationships.Store) ([]string, error) {
	repo, err := catalog.GetByID(item.RepositoryID)
	if err != nil {
		return nil, err
	}
	owners := []string{repo.OwnerID}
	switch item.Kind {
	case "pull_request":
		v, e := pulls.Get(item.RepositoryID, item.ResourceID)
		if e != nil {
			return nil, e
		}
		owners = []string{v.AuthorID}
	case "issue":
		v, e := issueStore.Get(item.RepositoryID, item.ResourceID)
		if e != nil {
			return nil, e
		}
		owners = []string{v.ReporterID}
	case "task":
		parts := strings.Split(item.ResourceID, "/")
		if len(parts) != 2 {
			return nil, restructuringplans.ErrInvalid
		}
		v, e := proposalStore.GetTask(item.RepositoryID, parts[0], parts[1])
		if e != nil {
			return nil, e
		}
		owners = []string{v.CreatedBy}
	case "release":
		v, e := releaseStore.Get(item.RepositoryID, item.ResourceID)
		if e != nil {
			return nil, e
		}
		owners = []string{v.CreatedBy}
	case "package":
		parts := strings.SplitN(item.ResourceID, "@", 2)
		if len(parts) != 2 {
			return nil, restructuringplans.ErrInvalid
		}
		v, e := packageStore.Get(parts[0], parts[1])
		if e != nil {
			return nil, e
		}
		owners = []string{v.PublisherID}
	case "documentation":
		xs, e := docs.List(item.RepositoryID, item.ResourceID)
		if e != nil {
			return nil, e
		}
		owners = nil
		for _, v := range xs {
			if v.SourceRevision == item.Revision {
				owners = []string{v.PublishedBy}
				break
			}
		}
	case "policy":
		v, e := policies.Get(item.ResourceID)
		if e != nil {
			return nil, e
		}
		owners = []string{v.CreatedBy}
	case "workspace":
		v, e := workspaceStore.Get(item.ResourceID)
		if e != nil {
			return nil, e
		}
		owners = []string{v.CreatorID}
	case "automation":
		v, e := workflows.Get(item.ResourceID)
		if e != nil {
			return nil, e
		}
		owners = nil
		for _, revision := range v.Revisions {
			if revision.Source.Revision == item.Revision {
				owners = []string{revision.ActivatedBy}
				break
			}
		}
	case "consumer":
		xs, e := consumers.ListDependencies(item.RepositoryID)
		if e != nil {
			return nil, e
		}
		owners = nil
		for _, v := range xs {
			if v.ID == item.ResourceID && v.CommitID == item.Revision {
				owners = []string{v.DeclaredBy}
				break
			}
		}
	}
	if len(owners) == 0 || strings.TrimSpace(owners[0]) == "" {
		return nil, restructuringplans.ErrInvalid
	}
	return owners, nil
}

func sameOwnerSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]bool{}
	for _, x := range a {
		if seen[x] {
			return false
		}
		seen[x] = true
	}
	for _, x := range b {
		if !seen[x] {
			return false
		}
	}
	return true
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
		return e == nil && v.RepositoryID == item.RepositoryID && v.Implementation != nil && v.Implementation.AffectedRevision == item.Revision
	case "task":
		parts := strings.Split(item.ResourceID, "/")
		if len(parts) != 2 {
			return false
		}
		v, e := proposalStore.GetTask(item.RepositoryID, parts[0], parts[1])
		return e == nil && v.Assignment != nil && v.Assignment.Access.RepositoryID == item.RepositoryID && v.Assignment.Access.BaseRevision == item.Revision
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
		relationship, e := peers.Contribution(item.ResourceID)
		return e == nil && len(relationship.InstanceIDs) > 0 && ((relationship.SourceRepositoryID == item.RepositoryID && relationship.SourceRevision == item.Revision) || (relationship.TargetRepositoryID == item.RepositoryID && relationship.TargetRevision == item.Revision))
	}
	return false
}
