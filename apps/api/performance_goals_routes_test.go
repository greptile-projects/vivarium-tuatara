package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/performancegoals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestPerformanceGoalAPI(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	goalStore, _ := performancegoals.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, goalStore))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "performance-owner")
	collaborator := createTestAccount(t, server.URL, "performance-collaborator")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"performance"}`, owner.Credential.Token, http.StatusCreated)
	var repo repositories.Repository
	json.NewDecoder(response.Body).Decode(&repo)
	response.Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/collaborators", `{"user_id":"`+collaborator.User.ID+`"}`, owner.Credential.Token, http.StatusCreated).Body.Close()
	body := map[string]any{"revision": map[string]any{"title": "API latency", "summary": "Protect response time", "subject": map[string]any{"kind": "api", "name": "GET /items"}, "workloads": []any{map[string]any{"name": "catalog", "description": "list 100 items", "inputs": "fixture-v1", "warmup": 2, "samples": 20}}, "metrics": []any{map[string]any{"name": "p95", "unit": "ms", "direction": "lower", "maximum": 200, "baseline": map[string]any{"value": 240, "environment": "linux", "measured_at": time.Now().UTC()}}}, "correctness_constraints": []any{map[string]any{"name": "complete list", "requirement": "returns all items", "verification": "API test"}}, "supported_environments": []any{map[string]any{"name": "linux", "os": "linux", "architecture": "amd64", "runtime": "go"}}, "owners": []string{"owner"}, "budgets": []any{map[string]any{"kind": "runtime", "limit": 5, "unit": "minutes"}}, "links": []any{map[string]any{"kind": "incident", "resource_id": "inc-1", "label": "Latency incident"}}, "baseline_max_age_days": 30, "rationale": "Agree before optimizing"}}
	payload, _ := json.Marshal(body)
	createdResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/performance-goals", string(payload), collaborator.Credential.Token, http.StatusCreated)
	var created performancegoals.Goal
	json.NewDecoder(createdResponse.Body).Decode(&created)
	createdResponse.Body.Close()
	listedResponse := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repo.ID+"/performance-goals", "", owner.Credential.Token, http.StatusOK)
	var listed struct {
		Goals []performancegoals.Goal `json:"goals"`
	}
	json.NewDecoder(listedResponse.Body).Decode(&listed)
	listedResponse.Body.Close()
	if len(listed.Goals) != 1 {
		t.Fatalf("list = %+v", listed)
	}
	body["expected_version"] = 1
	payload, _ = json.Marshal(body)
	url := server.URL + "/repositories/" + repo.ID + "/performance-goals/" + created.ID + "/revisions"
	authenticatedRequest(t, http.MethodPost, url, string(payload), owner.Credential.Token, http.StatusOK).Body.Close()
	authenticatedRequest(t, http.MethodPost, url, string(payload), owner.Credential.Token, http.StatusConflict).Body.Close()
}
