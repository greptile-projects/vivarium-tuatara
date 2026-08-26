package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/debugworkspaces"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/incidents"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/observabilitygaps"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/runbooks"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/serviceobjectives"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/supportthreads"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestObservabilityGapPublicAPIKeepsMissingUnderstandingExplicit(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	releaseStore, _ := releases.New(t.TempDir())
	deploymentStore, _ := deployments.New(t.TempDir())
	objectiveStore, _ := serviceobjectives.New(t.TempDir())
	incidentStore, _ := incidents.New(t.TempDir())
	debugStore, _ := debugworkspaces.New(t.TempDir())
	runbookStore, _ := runbooks.New(t.TempDir())
	supportStore, _ := supportthreads.New(t.TempDir())
	gaps, _ := observabilitygaps.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, releaseStore, deploymentStore, objectiveStore, incidentStore, debugStore, runbookStore, supportStore, gaps))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "observer")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"runtime"}`, owner.Credential.Token, http.StatusCreated)
	var repo repositories.Repository
	_ = json.NewDecoder(response.Body).Decode(&repo)
	response.Body.Close()
	release, _ := releaseStore.Create(releases.Candidate{RepositoryID: repo.ID, Version: "v1", Notes: "runtime", CommitID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", CreatedBy: owner.User.ID})
	environment, err := deploymentStore.PutEnvironment(deployments.Environment{RepositoryID: repo.ID, Name: "production", Position: 1, Image: "alpine:3.22", Command: "true", TimeoutSeconds: 30, RequiredApprovals: 0, Concurrency: 1, UpdatedBy: owner.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	promotion, err := deploymentStore.CreatePromotion(deployments.Promotion{RepositoryID: repo.ID, EnvironmentID: environment.ID, ReleaseID: release.ID, BuildID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", ArtifactID: "cccccccccccccccccccccccccccccccc", ArtifactSHA256: "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd", CommitID: release.CommitID, Rollout: deployments.RolloutDefinition{Version: 1, Stages: []deployments.RolloutStage{{Name: "all", ObservationSeconds: 1, Signals: []deployments.HealthSignal{{Name: "health", Command: "true"}}}}}, InitiatedBy: owner.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	promotion, err = deploymentStore.Transition(repo.ID, promotion.ID, "running", "started")
	if err == nil {
		promotion, err = deploymentStore.Transition(repo.ID, promotion.ID, "succeeded", "healthy")
	}
	if err != nil {
		t.Fatal(err)
	}
	revision := observabilitygaps.Revision{Title: "Unknown latency", Question: "Why is latency high?", Behavior: "Requests should finish quickly", AudienceIDs: []string{owner.User.ID}, Decision: "Decide whether to roll back", Services: []observabilitygaps.Service{{ID: "api", Name: "API", Environment: environment.ID}}, Journeys: []observabilitygaps.Journey{{ID: "checkout", Name: "Checkout", Behavior: "Submit"}}, RequiredTimeliness: "five minutes", Source: observabilitygaps.Source{Kind: "manual", Question: "Why?", Status: "current"}, Evidence: []observabilitygaps.Evidence{{ID: "m", Kind: "metric", Label: "Latency", ReleaseID: release.ID, ReleaseRevision: release.CommitID, Environment: environment.ID, Status: "ambiguous", ObservedAt: time.Now().Add(-31 * 24 * time.Hour)}}, OwnerIDs: []string{owner.User.ID}, SuccessCriteria: []observabilitygaps.Criterion{{ID: "ready", Statement: "Cause is distinguishable", RequiredEvidence: "correlated trace"}}}
	payload, _ := json.Marshal(map[string]any{"request_id": "gap-request", "revision": revision})
	created := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/observability-gaps", string(payload), owner.Credential.Token, http.StatusCreated)
	var gap observabilitygaps.Gap
	_ = json.NewDecoder(created.Body).Decode(&gap)
	created.Body.Close()
	if gap.CurrentVersion != 1 || len(gap.Diagnostics) < 6 {
		t.Fatalf("missing explicit diagnostics: %+v", gap)
	}
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/observability-gaps", string(payload), owner.Credential.Token, http.StatusCreated).Body.Close()
	invalidEnvironment := revision
	invalidEnvironment.Evidence = append([]observabilitygaps.Evidence(nil), revision.Evidence...)
	invalidEnvironment.Evidence[0].Environment = "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	payload, _ = json.Marshal(map[string]any{"request_id": "bad-environment", "revision": invalidEnvironment})
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/observability-gaps", string(payload), owner.Credential.Token, http.StatusUnprocessableEntity).Body.Close()
	unpromoted, err := deploymentStore.PutEnvironment(deployments.Environment{RepositoryID: repo.ID, Name: "staging", Position: 2, Image: "alpine:3.22", Command: "true", TimeoutSeconds: 30, RequiredApprovals: 0, Concurrency: 1, UpdatedBy: owner.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	invalidPromotion := revision
	invalidPromotion.Evidence = append([]observabilitygaps.Evidence(nil), revision.Evidence...)
	invalidPromotion.Evidence[0].Environment = unpromoted.ID
	payload, _ = json.Marshal(map[string]any{"request_id": "bad-promotion", "revision": invalidPromotion})
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/observability-gaps", string(payload), owner.Credential.Token, http.StatusUnprocessableEntity).Body.Close()
	for _, kind := range []string{"service_objective", "incident", "debugging_workspace", "runbook", "support_thread", "deployment"} {
		invalidSource := revision
		invalidSource.Source = observabilitygaps.Source{Kind: kind, ResourceID: "ffffffffffffffffffffffffffffffff", Revision: "1", Question: "fabricated", Status: "current"}
		payload, _ = json.Marshal(map[string]any{"request_id": "bad-source-" + kind, "revision": invalidSource})
		authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/observability-gaps", string(payload), owner.Credential.Token, http.StatusUnprocessableEntity).Body.Close()
	}
	revision.Evidence[0].ReleaseRevision = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	payload, _ = json.Marshal(map[string]any{"request_id": "bad-release", "revision": revision})
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/observability-gaps", string(payload), owner.Credential.Token, http.StatusUnprocessableEntity).Body.Close()
}
