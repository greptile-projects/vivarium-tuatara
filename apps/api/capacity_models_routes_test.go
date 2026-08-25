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
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func routeModelRevision(now time.Time, objectiveID, releaseID, commitID string) capacitymodels.Revision {
	return capacitymodels.Revision{Title: "Forecast", ObjectiveID: objectiveID, ObjectiveVersion: 1, Method: "linear", Evidence: []capacitymodels.Evidence{{ID: "usage", Kind: "usage", Label: "usage", ResourceID: "metric", ReleaseID: releaseID, ReleaseRevision: commitID, Window: capacitymodels.Window{Start: now.Add(-time.Hour), End: now}, Sanitization: "aggregated", InstrumentationVersion: "v1"}}, Assumptions: []capacitymodels.Assumption{{ID: "mix", Statement: "stable", Confidence: .5}}, Segments: []capacitymodels.Segment{{ID: "users", Name: "users", DemandUnit: "rps", Baseline: 1}}, Saturations: []capacitymodels.Saturation{{ID: "cpu", SegmentID: "users", Resource: "cpu", Limit: 1, Unit: "core", ExpectedAt: now.Add(2 * time.Hour), LowerAt: now.Add(time.Hour), UpperAt: now.Add(3 * time.Hour), Explanation: "limit"}}, CostCurve: []capacitymodels.CostPoint{{Demand: 1, Cost: 1, DemandUnit: "rps", Currency: "USD", Period: "month"}, {Demand: 2, Cost: 2, DemandUnit: "rps", Currency: "USD", Period: "month"}}, Scenarios: []capacitymodels.Scenario{{ID: "base", Name: "base", DemandMultiplier: 1, SaturationIDs: []string{"cpu"}}}}
}

func TestCapacityModelRoutesEnforceRepositoryAndReleaseBindings(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	objectives, _ := capacityobjectives.New(t.TempDir())
	models, _ := capacitymodels.New(t.TempDir())
	releaseStore, _ := releases.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, objectives, models, releaseStore))
	defer server.Close()
	ownerA := createTestAccount(t, server.URL, "model-owner-a")
	ownerB := createTestAccount(t, server.URL, "model-owner-b")
	createRepo := func(name, token string) repositories.Repository {
		response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"`+name+`"}`, token, http.StatusCreated)
		defer response.Body.Close()
		var repo repositories.Repository
		json.NewDecoder(response.Body).Decode(&repo)
		return repo
	}
	repoA, repoB := createRepo("models-a", ownerA.Credential.Token), createRepo("models-b", ownerB.Credential.Token)
	now := time.Now().UTC()
	objectiveA, _ := objectives.Create(repoA.ID, ownerA.User.ID, "objective-a", capacityObjectiveRevision(now, ownerA.User.ID))
	objectiveB, _ := objectives.Create(repoB.ID, ownerB.User.ID, "objective-b", capacityObjectiveRevision(now, ownerB.User.ID))
	commitA, commitB := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	releaseA, _ := releaseStore.Create(releases.Candidate{RepositoryID: repoA.ID, Version: "1", Notes: "a", CommitID: commitA, CreatedBy: ownerA.User.ID})
	releaseB, _ := releaseStore.Create(releases.Candidate{RepositoryID: repoB.ID, Version: "1", Notes: "b", CommitID: commitB, CreatedBy: ownerB.User.ID})

	invalid := routeModelRevision(now, objectiveA.ID, "00000000000000000000000000000000", commitA)
	payload, _ := json.Marshal(map[string]any{"request_id": "invalid-release", "revision": invalid})
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repoA.ID+"/capacity-models", string(payload), ownerA.Credential.Token, http.StatusUnprocessableEntity).Body.Close()
	invalid.Evidence[0].ReleaseID, invalid.Evidence[0].ReleaseRevision = releaseA.ID, commitB
	payload, _ = json.Marshal(map[string]any{"request_id": "mismatched-release", "revision": invalid})
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repoA.ID+"/capacity-models", string(payload), ownerA.Credential.Token, http.StatusUnprocessableEntity).Body.Close()

	modelB, err := models.Create(repoB.ID, "human", ownerB.User.ID, "model-b", routeModelRevision(now, objectiveB.ID, releaseB.ID, commitB))
	if err != nil {
		t.Fatal(err)
	}
	attackRevision := routeModelRevision(now, objectiveA.ID, releaseA.ID, commitA)
	payload, _ = json.Marshal(map[string]any{"request_id": "attack-revision", "expected_version": 1, "revision": attackRevision})
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repoA.ID+"/capacity-models/"+modelB.ID+"/revisions", string(payload), ownerA.Credential.Token, http.StatusNotFound).Body.Close()
	payload, _ = json.Marshal(map[string]any{"expected_version": 1, "event": map[string]any{"request_id": "attack-event", "kind": "challenge", "statement": "attack"}})
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repoA.ID+"/capacity-models/"+modelB.ID+"/events", string(payload), ownerA.Credential.Token, http.StatusNotFound).Body.Close()
	retained, _ := models.Get(modelB.ID, ownerB.User.ID)
	if retained.CurrentVersion != 1 || len(retained.Events) != 0 {
		t.Fatalf("cross-repository mutation persisted: %+v", retained)
	}
}

func capacityObjectiveRevision(now time.Time, owner string) capacityobjectives.Revision {
	return capacityobjectives.Revision{Title: "Objective", Scope: capacityobjectives.Scope{Kind: "api", Name: "api"}, Forecasts: []capacityobjectives.Forecast{{ID: "f", Segment: "users", Start: now, End: now.Add(time.Hour), Value: 1, Unit: "rps", Confidence: "supported", Evidence: []string{"metric"}}}, TrafficShapes: []capacityobjectives.TrafficShape{{Name: "peak", Pattern: "burst", PeakMultiplier: 1, BurstDuration: "1m"}}, ServiceLevels: []capacityobjectives.ServiceLevel{{Name: "latency", Indicator: "p95", Target: 1, Unit: "ms", Window: "1m"}}, Thresholds: []capacityobjectives.Threshold{{Resource: "cpu", Signal: "cpu", Warning: 1, Critical: 2, Unit: "core"}}, DependencyLimits: []capacityobjectives.DependencyLimit{{Name: "db", Limit: 1, Unit: "connection", Signal: "db"}}, Regions: []capacityobjectives.Region{{Name: "region", DemandShare: 1}}, OwnerIDs: []string{owner}, Budget: capacityobjectives.Budget{Amount: 1, Currency: "USD", Period: "month"}, LeadTime: capacityobjectives.LeadTime{Duration: "1d", Trigger: "warning"}, SuccessCriteria: []capacityobjectives.Criterion{{Name: "serve", Condition: "yes", Evidence: "check"}}, RollbackCriteria: []capacityobjectives.Criterion{{Name: "protect", Condition: "yes", Evidence: "check"}}}
}
