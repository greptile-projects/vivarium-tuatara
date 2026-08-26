package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/capacitymodels"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/capacityobjectives"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/capacitytests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/durableschemas"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/infrastructure"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func routeCapacityPlan(objectiveID string, component capacitytests.Component) capacitytests.Plan {
	candidate := func(id, strategy string) capacitytests.Candidate {
		return capacitytests.Candidate{ID: id, Name: id, Strategy: strategy, Components: []capacitytests.Component{component}, ExpectedCost: 1, Currency: "USD"}
	}
	return capacitytests.Plan{
		Title: "Exact alternatives", ObjectiveID: objectiveID, ObjectiveVersion: 1, EnvironmentKind: "isolated",
		Candidates: []capacitytests.Candidate{candidate("up", "vertical"), candidate("out", "horizontal")},
		Scenarios:  []capacitytests.Scenario{{ID: "peak", Name: "peak", Kind: "load", Command: []string{"./capacity-test"}, Workload: capacitytests.Workload{Kind: "synthetic", SourcePath: "capacity/peak.json", Sanitization: "generated"}, Limits: capacitytests.Limits{MaxDurationSeconds: 60, MaxRequests: 100, MaxConcurrency: 5, MaxCost: 2, CoordinatedLoadKey: "repo/peak"}, CorrectnessCriteria: []string{"fixture matches"}}},
	}
}

func TestCapacityTestRoutesResolveEveryComponentKind(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	objectives, _ := capacityobjectives.New(t.TempDir())
	models, _ := capacitymodels.New(t.TempDir())
	releaseStore, _ := releases.New(t.TempDir())
	tests, _ := capacitytests.New(t.TempDir())
	infrastructureStore, _ := infrastructure.New(t.TempDir())
	schemaStore, _ := durableschemas.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, objectives, models, releaseStore, tests, infrastructureStore, schemaStore))
	defer server.Close()

	owner := createTestAccount(t, server.URL, "capacity-route-owner")
	other := createTestAccount(t, server.URL, "capacity-route-other")
	createRepo := func(name, token string) repositories.Repository {
		response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"`+name+`"}`, token, http.StatusCreated)
		defer response.Body.Close()
		var repository repositories.Repository
		if e := json.NewDecoder(response.Body).Decode(&repository); e != nil {
			t.Fatal(e)
		}
		return repository
	}
	repository := createRepo("capacity-route", owner.Credential.Token)
	otherRepository := createRepo("capacity-route-other", other.Credential.Token)
	objective, e := objectives.Create(repository.ID, owner.User.ID, "capacity-route-objective", capacityObjectiveRevision(time.Now().UTC(), owner.User.ID))
	if e != nil {
		t.Fatal(e)
	}
	commit := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	release, _ := releaseStore.Create(releases.Candidate{RepositoryID: repository.ID, Version: "1", Notes: "candidate", CommitID: commit, CreatedBy: owner.User.ID})
	otherRelease, _ := releaseStore.Create(releases.Candidate{RepositoryID: otherRepository.ID, Version: "1", Notes: "other", CommitID: commit, CreatedBy: other.User.ID})

	post := func(requestID string, component capacitytests.Component, status int) {
		body, _ := json.Marshal(capacityTestInput{RequestID: requestID, Plan: routeCapacityPlan(objective.ID, component)})
		authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/capacity-tests", string(body), owner.Credential.Token, status).Body.Close()
	}
	post("valid-release", capacitytests.Component{Kind: "release", ResourceID: release.ID, Revision: commit}, http.StatusCreated)
	post("cross-repository-release", capacitytests.Component{Kind: "release", ResourceID: otherRelease.ID, Revision: commit}, http.StatusUnprocessableEntity)
	post("unknown-infrastructure", capacitytests.Component{Kind: "infrastructure", ResourceID: "missing", Revision: "revision-1"}, http.StatusUnprocessableEntity)
	post("unknown-schema", capacitytests.Component{Kind: "schema", ResourceID: "missing", Revision: "1"}, http.StatusUnprocessableEntity)
	post("nonexact-dependency", capacitytests.Component{Kind: "dependency_configuration", ResourceID: "go.mod", Revision: "main"}, http.StatusUnprocessableEntity)
}
