package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/capacityobjectives"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestCapacityObjectiveAPI(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	objectives, _ := capacityobjectives.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, objectives))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "capacity-owner")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"capacity"}`, owner.Credential.Token, http.StatusCreated)
	var repo repositories.Repository
	json.NewDecoder(response.Body).Decode(&repo)
	response.Body.Close()
	now := time.Now().UTC()
	revision := capacityobjectives.Revision{Title: "Launch", Summary: "serve demand", Scope: capacityobjectives.Scope{Kind: "api", Name: "catalog"}, Forecasts: []capacityobjectives.Forecast{{ID: "f1", Segment: "users", Start: now, End: now.Add(time.Hour), Value: 100, Unit: "rps", Confidence: "supported", Evidence: []string{"metric"}}}, TrafficShapes: []capacityobjectives.TrafficShape{{Name: "peak", Pattern: "burst", PeakMultiplier: 2, BurstDuration: "5m"}}, ServiceLevels: []capacityobjectives.ServiceLevel{{Name: "latency", Indicator: "p95", Target: 200, Unit: "ms", Window: "5m"}}, Thresholds: []capacityobjectives.Threshold{{Resource: "cpu", Signal: "cpu", Warning: 70, Critical: 90, Unit: "percent"}}, DependencyLimits: []capacityobjectives.DependencyLimit{{Name: "db", Limit: 50, Unit: "connections", Signal: "connections"}}, Regions: []capacityobjectives.Region{{Name: "us", DemandShare: 1}}, OwnerIDs: []string{owner.User.ID}, Budget: capacityobjectives.Budget{Amount: 100, Currency: "USD", Period: "month"}, LeadTime: capacityobjectives.LeadTime{Duration: "7d", Trigger: "warning"}, SuccessCriteria: []capacityobjectives.Criterion{{Name: "serve", Condition: "within SLO", Evidence: "check"}}, RollbackCriteria: []capacityobjectives.Criterion{{Name: "protect", Condition: "errors", Evidence: "monitor"}}, Rationale: "agree"}
	payload, _ := json.Marshal(map[string]any{"request_id": "create-1", "revision": revision})
	createdResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/capacity-objectives", string(payload), owner.Credential.Token, http.StatusCreated)
	var created capacityobjectives.Objective
	json.NewDecoder(createdResponse.Body).Decode(&created)
	createdResponse.Body.Close()
	listed := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repo.ID+"/capacity-objectives", "", owner.Credential.Token, http.StatusOK)
	var result struct {
		Objectives []capacityobjectives.Objective `json:"capacity_objectives"`
	}
	json.NewDecoder(listed.Body).Decode(&result)
	listed.Body.Close()
	if len(result.Objectives) != 1 {
		t.Fatalf("list = %+v", result)
	}
	payload, _ = json.Marshal(map[string]any{"request_id": "revise-1", "expected_version": 1, "revision": revision})
	url := server.URL + "/repositories/" + repo.ID + "/capacity-objectives/" + created.ID + "/revisions"
	authenticatedRequest(t, http.MethodPost, url, string(payload), owner.Credential.Token, http.StatusOK).Body.Close()
	authenticatedRequest(t, http.MethodPost, url, string(payload), owner.Credential.Token, http.StatusOK).Body.Close()
	payload, _ = json.Marshal(map[string]any{"request_id": "revise-2", "expected_version": 1, "revision": revision})
	authenticatedRequest(t, http.MethodPost, url, string(payload), owner.Credential.Token, http.StatusConflict).Body.Close()
}
