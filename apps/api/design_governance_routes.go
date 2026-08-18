package main

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/designgovernance"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/interfacechecks"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/interfacesystems"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

func registerDesignGovernanceRoutes(mux *http.ServeMux, repos *repositories.Store, orgs *organizations.Store, credentials *auth.Store, governance *designgovernance.Store, pulls *pullrequests.Store, releasesStore *releases.Store, checks *interfacechecks.Store, systems *interfacesystems.Store, proposalStore *proposals.Store) {
	repoOrg := func(id string) string {
		v, e := repos.GetByID(id)
		if e != nil {
			return ""
		}
		return v.OrganizationID
	}
	readiness := func(repo, pull, revision string, paths []string) (designgovernance.Readiness, error) {
		components, journeys := []string{}, []string{}
		diagnostics := []designgovernance.Diagnostic{}
		all, err := checks.List(repo, pull)
		if err != nil {
			return designgovernance.Readiness{}, err
		}
		currentPreview := false
		for _, c := range all {
			if c.Revision != revision {
				continue
			}
			currentPreview = true
			if c.Status != "passed" {
				diagnostics = append(diagnostics, designgovernance.Diagnostic{Kind: "failed_interface_check", Severity: "blocking", Message: "A current interface check did not pass.", ResourceID: c.ID})
				continue
			}
			journeys = append(journeys, c.Journey)
			components = append(components, c.Coverage...)
			for _, d := range c.Differences {
				classified := false
				for _, x := range c.Classifications {
					if x.DifferenceID == d.ID {
						classified = true
						if x.Outcome == "regression" {
							diagnostics = append(diagnostics, designgovernance.Diagnostic{Kind: "unresolved_deviation", Severity: "blocking", Message: "A classified interface regression remains unresolved.", ResourceID: d.ID})
						}
					}
				}
				if !classified {
					diagnostics = append(diagnostics, designgovernance.Diagnostic{Kind: "unresolved_deviation", Severity: "blocking", Message: "An interface difference has no current classification.", ResourceID: d.ID})
				}
			}
		}
		if len(all) > 0 && !currentPreview {
			diagnostics = append(diagnostics, designgovernance.Diagnostic{Kind: "stale_preview", Severity: "blocking", Message: "Interface preview evidence is stale for this revision."})
		}
		listed, err := systems.List(repo)
		if err != nil {
			return designgovernance.Readiness{}, err
		}
		for _, system := range listed {
			for _, d := range system.Diagnostics {
				if d.Kind == "stale_implementation" || d.Kind == "unsupported_consumer" {
					diagnostics = append(diagnostics, designgovernance.Diagnostic{Kind: "obsolete_component_use", Severity: "blocking", Message: d.Message, ResourceID: d.Definition})
				}
			}
		}
		return governance.Evaluate(repo, repoOrg(repo), pull, revision, paths, components, journeys, nil, diagnostics)
	}
	pulls.ConfigureDesignReadiness(func(p pullrequests.PullRequest, changes []pullrequests.FileChange) (any, []pullrequests.ReadinessBlocker, error) {
		paths := make([]string, 0, len(changes))
		for _, change := range changes {
			paths = append(paths, change.Path)
		}
		projection, err := readiness(p.RepositoryID, p.ID, p.SourceCommitID, paths)
		if err != nil {
			return nil, nil, err
		}
		blockers := []pullrequests.ReadinessBlocker{}
		for _, diagnostic := range projection.Diagnostics {
			if diagnostic.Severity == "blocking" {
				blockers = append(blockers, pullrequests.ReadinessBlocker{Code: "design_" + diagnostic.Kind, Message: diagnostic.Message})
			}
		}
		return projection, blockers, nil
	})
	mux.HandleFunc("POST /repositories/{id}/design-acceptance-policies", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var p designgovernance.Policy
		if decodeJSON(r, &p) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		p.ScopeKind = "repository"
		p.ScopeID = r.PathValue("id")
		p.CreatedBy = actor.UserID
		out, e := governance.CreatePolicy(p)
		writeDesignGovernance(w, out, e, 201)
	})
	mux.HandleFunc("GET /repositories/{id}/design-acceptance-policies", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id")); !ok {
			return
		}
		local, e := governance.Policies("repository", r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "design_governance_unavailable", "design policy is unavailable")
			return
		}
		inherited := []designgovernance.Policy{}
		if oid := repoOrg(r.PathValue("id")); oid != "" {
			inherited, _ = governance.Policies("organization", oid)
		}
		writeJSON(w, 200, map[string]any{"policies": local, "inherited_policies": inherited})
	})
	mux.HandleFunc("POST /organizations/{id}/design-acceptance-policies", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:write", false)
		if !ok {
			return
		}
		org, e := orgs.Get(r.PathValue("id"))
		owner := false
		for _, member := range org.Members {
			owner = owner || member.UserID == actor.UserID && member.Role == "owner"
		}
		if e != nil || !owner {
			writeAPIError(w, 404, "organization_not_found", "organization not found")
			return
		}
		var p designgovernance.Policy
		if decodeJSON(r, &p) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		p.ScopeKind = "organization"
		p.ScopeID = org.ID
		p.CreatedBy = actor.UserID
		out, e := governance.CreatePolicy(p)
		writeDesignGovernance(w, out, e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/design-acceptances", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:write", false)
		if !ok {
			return
		}
		p, e := pulls.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if writePullRequestError(w, e) {
			return
		}
		var in designgovernance.Acceptance
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		in.PullRequestID = p.ID
		in.Revision = p.SourceCommitID
		in.ActorID = actor.UserID
		out, e := governance.Accept(p.RepositoryID, repoOrg(p.RepositoryID), in)
		writeDesignGovernance(w, out, e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/design-exceptions", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		p, e := pulls.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if writePullRequestError(w, e) {
			return
		}
		var in designgovernance.Exception
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		in.PullRequestID = p.ID
		in.Revision = p.SourceCommitID
		in.ActorID = actor.UserID
		out, e := governance.Except(p.RepositoryID, repoOrg(p.RepositoryID), in)
		writeDesignGovernance(w, out, e, 201)
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/design-readiness", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id")); !ok {
			return
		}
		p, e := pulls.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if writePullRequestError(w, e) {
			return
		}
		paths := []string{}
		for _, x := range mustChanges(pulls, p) {
			paths = append(paths, x.Path)
		}
		out, e := readiness(p.RepositoryID, p.ID, p.SourceCommitID, paths)
		if e != nil {
			writeAPIError(w, 500, "design_governance_unavailable", "design readiness is unavailable")
			return
		}
		writeJSON(w, 200, out)
	})
	mux.HandleFunc("GET /repositories/{id}/releases/{release_id}/design-readiness", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id")); !ok {
			return
		}
		release, e := releasesStore.Get(r.PathValue("id"), r.PathValue("release_id"))
		if e != nil {
			writeAPIError(w, 404, "release_not_found", "release candidate not found")
			return
		}
		out := designgovernance.Readiness{Ready: true, Revision: release.CommitID, Policies: []designgovernance.Policy{}, Acceptances: []designgovernance.Acceptance{}, ActiveExceptions: []designgovernance.Exception{}, Diagnostics: []designgovernance.Diagnostic{{Kind: "release_evidence_scope", Severity: "warning", Message: "Release readiness preserves included pull-bound acceptance; a release does not create new design authority."}}, Authority: "Design acceptance is evidence only and grants no code review, merge, release, deployment, or repository authority."}
		if len(release.Inclusions.PullRequestIDs) == 0 {
			var e error
			out, e = governance.Evaluate(release.RepositoryID, repoOrg(release.RepositoryID), "", release.CommitID, release.ChangedPaths, nil, nil, nil, out.Diagnostics)
			if e != nil {
				writeAPIError(w, 500, "design_governance_unavailable", "design readiness is unavailable")
				return
			}
		} else {
			for _, pullID := range release.Inclusions.PullRequestIDs {
				included, pullErr := pulls.Get(release.RepositoryID, pullID)
				if pullErr != nil {
					writeAPIError(w, 500, "design_governance_unavailable", "included pull evidence is unavailable")
					return
				}
				includedChanges, changeErr := pulls.Changes(release.RepositoryID, included.ID)
				if changeErr != nil {
					writeAPIError(w, 500, "design_governance_unavailable", "included pull paths are unavailable")
					return
				}
				paths := make([]string, 0, len(includedChanges))
				for _, change := range includedChanges {
					paths = append(paths, change.Path)
				}
				projected, evalErr := readiness(release.RepositoryID, included.ID, included.SourceCommitID, paths)
				if evalErr != nil {
					writeAPIError(w, 500, "design_governance_unavailable", "design readiness is unavailable")
					return
				}
				out.Ready = out.Ready && projected.Ready
				out.Policies = append(out.Policies, projected.Policies...)
				out.Acceptances = append(out.Acceptances, projected.Acceptances...)
				out.ActiveExceptions = append(out.ActiveExceptions, projected.ActiveExceptions...)
				out.Diagnostics = append(out.Diagnostics, projected.Diagnostics...)
			}
		}
		writeJSON(w, 200, out)
	})
	mux.HandleFunc("POST /repositories/{id}/interface-systems/{system_id}/migration-work", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		system, e := systems.Get(r.PathValue("id"), r.PathValue("system_id"))
		if e != nil {
			writeAPIError(w, 404, "interface_system_not_found", "interface system not found")
			return
		}
		var in struct {
			RepositoryID  string `json:"repository_id"`
			Title         string `json:"title"`
			Outcome       string `json:"outcome"`
			Documentation bool   `json:"documentation"`
		}
		if decodeJSON(r, &in) != nil || in.RepositoryID == "" {
			writeAPIError(w, 400, "invalid_migration_work", "target repository, title, and outcome are required")
			return
		}
		if _, _, ok := authorizeRepositoryParticipant(w, r, repos, credentials, in.RepositoryID, "repositories:write"); !ok {
			return
		}
		proposal, e := proposalStore.Create(in.RepositoryID, actor.UserID, in.Title, fmt.Sprintf("Migrate to interface system %s version %d. Documentation required: %s", system.ID, system.CurrentVersion, map[bool]string{true: "yes", false: "no"}[in.Documentation]))
		if e != nil {
			writeAPIError(w, 422, "migration_work_invalid", "migration proposal could not be created")
			return
		}
		task, e := proposalStore.CreateTask(in.RepositoryID, proposal.ID, actor.UserID, in.Title, in.Outcome, nil, nil)
		if e != nil {
			writeAPIError(w, 500, "migration_work_unavailable", "migration task could not be created")
			return
		}
		writeJSON(w, 201, map[string]any{"interface_system_id": system.ID, "interface_system_version": system.CurrentVersion, "proposal": proposal, "task": task, "authority": "Migration work uses ordinary repository task, review, check, and merge authority."})
	})
	mux.HandleFunc("POST /repositories/{id}/releases/{release_id}/design-repairs", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		release, err := releasesStore.Get(r.PathValue("id"), r.PathValue("release_id"))
		if err != nil {
			writeAPIError(w, 404, "release_not_found", "release candidate not found")
			return
		}
		var in struct {
			SourceKind string `json:"source_kind"`
			SourceID   string `json:"source_id"`
			Title      string `json:"title"`
			Outcome    string `json:"outcome"`
		}
		if decodeJSON(r, &in) != nil || (in.SourceKind != "feedback" && in.SourceKind != "observed_regression") || strings.TrimSpace(in.SourceID) == "" {
			writeAPIError(w, 422, "invalid_design_repair", "feedback or observed regression provenance, title, and outcome are required")
			return
		}
		proposal, err := proposalStore.Create(release.RepositoryID, actor.UserID, in.Title, "Repair "+in.SourceKind+" "+in.SourceID+" observed after release "+release.ID+" at exact commit "+release.CommitID+".")
		if err != nil {
			writeAPIError(w, 422, "invalid_design_repair", "repair proposal could not be created")
			return
		}
		task, err := proposalStore.CreateTask(release.RepositoryID, proposal.ID, actor.UserID, in.Title, in.Outcome, nil, nil)
		if err != nil {
			writeAPIError(w, 500, "design_repair_unavailable", "repair task could not be created")
			return
		}
		writeJSON(w, 201, map[string]any{"release_id": release.ID, "release_commit_id": release.CommitID, "source_kind": in.SourceKind, "source_id": in.SourceID, "proposal": proposal, "task": task, "authority": "Repair work uses ordinary repository task, review, check, merge, release, and deployment authority."})
	})
}

func mustChanges(store *pullrequests.Store, p pullrequests.PullRequest) []pullrequests.FileChange {
	v, _ := store.Changes(p.RepositoryID, p.ID)
	return v
}
func writeDesignGovernance(w http.ResponseWriter, out any, err error, status int) {
	switch {
	case err == nil:
		writeJSON(w, status, out)
	case errors.Is(err, designgovernance.ErrNotFound):
		writeAPIError(w, 404, "design_policy_not_found", "design policy not found")
	case errors.Is(err, designgovernance.ErrInvalid):
		writeAPIError(w, 422, "invalid_design_governance", "scope, current policy, actor, rationale, or expiry is invalid")
	default:
		writeAPIError(w, 500, "design_governance_unavailable", "design governance could not be persisted")
	}
}
