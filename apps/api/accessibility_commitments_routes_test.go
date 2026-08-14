package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/accessibilitycommitments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestAccessibilityCommitmentAPI(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	commitments, _ := accessibilitycommitments.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, commitments))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "accessibility-owner")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"inclusive"}`, owner.Credential.Token, http.StatusCreated)
	var repo repositories.Repository
	json.NewDecoder(response.Body).Decode(&repo)
	response.Body.Close()
	revision := map[string]any{"title": "Keyboard navigation", "summary": "Core journey is operable", "subject": map[string]any{"kind": "component", "resource_id": "nav", "name": "Repository navigation"}, "standards": []any{map[string]any{"name": "WCAG", "version": "2.2", "level": "AA", "criteria": []string{"2.1.1"}}}, "assistive_technologies": []any{map[string]any{"id": "keyboard", "name": "Hardware keyboard", "version": "any", "input": "keyboard", "environment_ids": []string{"chrome"}}}, "target_audiences": []any{map[string]any{"id": "motor", "name": "Keyboard users", "access_needs": []string{"no pointer"}}}, "environments": []any{map[string]any{"id": "chrome", "browser": "Chrome", "browser_version": "stable", "os": "Linux", "device": "desktop", "supported": true}}, "required_scenarios": []any{map[string]any{"id": "navigate", "name": "Navigate sections", "steps": []string{"Press Tab"}, "expected_outcome": "Focus is visible", "standard_criteria": []string{"2.1.1"}, "audience_ids": []string{"motor"}, "technology_ids": []string{"keyboard"}, "environment_ids": []string{"chrome"}, "owner_ids": []string{owner.User.ID}}}, "severity_policy": []any{map[string]any{"severity": "critical", "definition": "Cannot navigate", "response": "Block release", "resolution_days": 1}}, "owner_ids": []string{owner.User.ID}, "requirements": []any{map[string]any{"id": "visible-focus", "statement": "Focus is always visible"}}, "exceptions": []any{}, "links": []any{map[string]any{"kind": "documentation", "resource_id": "journey-1", "label": "Navigation journey"}}, "rationale": "Shared review bar"}
	payload, _ := json.Marshal(map[string]any{"revision": revision})
	createdResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/accessibility-commitments", string(payload), owner.Credential.Token, http.StatusCreated)
	var created accessibilitycommitments.Commitment
	json.NewDecoder(createdResponse.Body).Decode(&created)
	createdResponse.Body.Close()
	listed := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repo.ID+"/accessibility-commitments", "", owner.Credential.Token, http.StatusOK)
	var collection struct {
		Commitments []accessibilitycommitments.Commitment `json:"commitments"`
	}
	json.NewDecoder(listed.Body).Decode(&collection)
	listed.Body.Close()
	if len(collection.Commitments) != 1 || collection.Commitments[0].ID != created.ID {
		t.Fatalf("list = %+v", collection)
	}
	payload, _ = json.Marshal(map[string]any{"expected_version": 1, "revision": revision})
	url := server.URL + "/repositories/" + repo.ID + "/accessibility-commitments/" + created.ID + "/revisions"
	authenticatedRequest(t, http.MethodPost, url, string(payload), owner.Credential.Token, http.StatusOK).Body.Close()
	authenticatedRequest(t, http.MethodPost, url, string(payload), owner.Credential.Token, http.StatusConflict).Body.Close()
	reader, _ := credentials.Issue(owner.User.ID, auth.API, "reader", []string{"repositories:read"}, time.Hour)
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/accessibility-commitments", string(payload), reader.Token, http.StatusUnauthorized).Body.Close()
}
