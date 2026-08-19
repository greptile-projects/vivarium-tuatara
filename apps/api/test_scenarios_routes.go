package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/apicontracts"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/debugworkspaces"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/designproposals"
	docscollections "github.com/greptile-projects/vivarium-tuatara/apps/api/docscollections"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/qualityplans"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/testscenarios"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
)

type testScenarioSources struct {
	issues        *issues.Store
	reproductions *debugworkspaces.Store
	designs       *designproposals.Store
	contracts     *apicontracts.Store
	documentation *docscollections.Store
}

func registerTestScenarioRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, scenarios *testscenarios.Store, plans *qualityplans.Store, pulls *pullrequests.Store, workspaceStore *workspaces.Store, sources testScenarioSources) {
	mux.HandleFunc("GET /repositories/{id}/test-scenarios", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		out, e := scenarios.List(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "test_scenarios_unavailable", "test scenarios could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"scenarios": out})
	})
	mux.HandleFunc("GET /repositories/{id}/test-scenarios/{scenario_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		v, e := scenarios.Get(r.PathValue("scenario_id"))
		if e != nil || v.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "test_scenario_not_found", "test scenario not found")
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("POST /repositories/{id}/test-scenarios", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in testscenarios.Scenario
		if decodeJSON(r, &in) != nil || !testscenarios.Valid(in) {
			writeAPIError(w, 400, "invalid_test_scenario", "a complete parameterized scenario with explicit safe fixtures and executable implementation is required")
			return
		}
		repoID := r.PathValue("id")
		if !testScenarioProvenance(git, repoID, in, plans, pulls, workspaceStore, sources) {
			writeAPIError(w, 422, "test_scenario_provenance_invalid", "sources, quality requirements, branch assets, pull, and workspace must resolve at their exact declared revisions")
			return
		}
		encoded, _ := json.Marshal(in)
		if reusableSecret.Match(encoded) {
			writeAPIError(w, 400, "test_scenario_sensitive", "reusable scenarios cannot retain credentials or secret-shaped content")
			return
		}
		out, e := scenarios.Create(repoID, actor.UserID, in)
		if e != nil {
			if errors.Is(e, testscenarios.ErrInvalid) {
				writeAPIError(w, 400, "invalid_test_scenario", "scenario is invalid")
				return
			}
			log.Printf("test scenario storage: %v", e)
			writeAPIError(w, 500, "test_scenarios_unavailable", "test scenario could not be persisted")
			return
		}
		writeJSON(w, 201, out)
	})
}

func testScenarioProvenance(git *storage.Store, repoID string, v testscenarios.Scenario, plans *qualityplans.Store, pulls *pullrequests.Store, workspaceStore *workspaces.Store, sources testScenarioSources) bool {
	repo, e := git.Open(repoID)
	if e != nil {
		return false
	}
	if _, e = repo.ReadCommit(storage.ObjectID(v.Implementation.CommitID)); e != nil {
		return false
	}
	ref, e := repo.ReadReference("refs/heads/" + strings.TrimPrefix(v.Implementation.Branch, "refs/heads/"))
	if e != nil || string(ref.Target) != v.Implementation.CommitID {
		return false
	}
	for _, x := range v.Sources {
		if _, e = repo.ReadCommit(storage.ObjectID(x.Revision)); e != nil {
			return false
		}
		if x.Path != "" {
			if _, _, ok := infrastructureCommitBlob(git, repoID, x.Revision, x.Path); !ok {
				return false
			}
		}
		if !testScenarioSourceResolves(repoID, x, plans, sources) {
			return false
		}
	}
	for _, path := range v.Implementation.TestPaths {
		if _, _, ok := infrastructureCommitBlob(git, repoID, v.Implementation.CommitID, path); !ok {
			return false
		}
	}
	for _, x := range v.Fixtures {
		body, digest, ok := infrastructureCommitBlob(git, repoID, v.Implementation.CommitID, x.Path)
		if !ok || digest != x.SHA256 || reusableSecret.Match(body) {
			return false
		}
	}
	if v.QualityPlanID != "" {
		p, e := plans.Get(v.QualityPlanID)
		if e != nil || p.RepositoryID != repoID || p.CurrentVersion < v.QualityPlanVersion || v.QualityPlanVersion < 1 {
			return false
		}
		var revision *qualityplans.Revision
		for i := range p.Revisions {
			if p.Revisions[i].Version == v.QualityPlanVersion {
				revision = &p.Revisions[i]
			}
		}
		if revision == nil {
			return false
		}
		for _, id := range v.RequirementIDs {
			found := false
			for _, q := range revision.Requirements {
				found = found || q.ID == id
			}
			if !found {
				return false
			}
		}
	} else if len(v.RequirementIDs) > 0 {
		return false
	}
	if v.Implementation.PullRequestID != "" {
		if pulls == nil {
			return false
		}
		p, e := pulls.Get(repoID, v.Implementation.PullRequestID)
		if e != nil || p.SourceCommitID != v.Implementation.CommitID {
			return false
		}
	}
	if v.Implementation.WorkspaceID != "" {
		if workspaceStore == nil {
			return false
		}
		ws, e := workspaceStore.Get(v.Implementation.WorkspaceID)
		if e != nil || ws.RepositoryID != repoID || ws.CommitID != v.Implementation.CommitID {
			return false
		}
	}
	return true
}

func testScenarioSourceResolves(repoID string, source testscenarios.Source, plans *qualityplans.Store, stores testScenarioSources) bool {
	switch source.Kind {
	case "issue":
		if stores.issues == nil {
			return false
		}
		v, e := stores.issues.Get(repoID, source.ResourceID)
		if e != nil || v.RepositoryID != repoID {
			return false
		}
		if v.Triage.SuspectedRevision == source.Revision {
			return true
		}
		for _, x := range v.ReproductionAttempts {
			if x.CommitID == source.Revision {
				return true
			}
		}
		if v.Implementation != nil && v.Implementation.AffectedRevision == source.Revision {
			return true
		}
		for _, x := range v.RepairVerifications {
			if x.CandidateCommitID == source.Revision {
				return true
			}
		}
		return v.DeliveryResolution != nil && v.DeliveryResolution.ReleaseCommitID == source.Revision
	case "reproduction":
		if stores.reproductions == nil {
			return false
		}
		all, e := stores.reproductions.List(repoID)
		if e != nil {
			return false
		}
		for _, w := range all {
			for _, s := range w.ReplayScenarios {
				if s.ID == source.ResourceID && s.CommitID == source.Revision {
					return true
				}
			}
		}
		return false
	case "design_specification":
		if stores.designs == nil {
			return false
		}
		v, e := stores.designs.Get(repoID, source.ResourceID)
		return e == nil && v.RepositoryID == repoID && v.Implementation != nil && v.Implementation.BaseRevision == source.Revision
	case "api_contract":
		if stores.contracts == nil {
			return false
		}
		v, e := stores.contracts.Get(source.ResourceID)
		if e != nil || v.RepositoryID != repoID {
			return false
		}
		for _, r := range v.Revisions {
			if r.Source.CommitID == source.Revision {
				return true
			}
		}
		return false
	case "documentation":
		if stores.documentation == nil {
			return false
		}
		revisions, e := stores.documentation.List(repoID, source.ResourceID)
		if e != nil {
			return false
		}
		for _, r := range revisions {
			if r.RepositoryID == repoID && r.SourceRevision == source.Revision {
				return true
			}
		}
		return false
	case "user_journey":
		all, e := plans.List(repoID)
		if e != nil {
			return false
		}
		for _, p := range all {
			for _, r := range p.Revisions {
				for _, scope := range r.Scopes {
					if scope.Kind == "journey" && scope.ResourceID == source.ResourceID && scope.SourceRevision == source.Revision {
						return true
					}
				}
			}
		}
		return false
	default:
		return false
	}
}
