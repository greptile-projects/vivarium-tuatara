package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/responsepolicies"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestResponsePolicyAPI(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	policies, _ := responsepolicies.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, policies))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "response-owner")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"coverage"}`, owner.Credential.Token, http.StatusCreated)
	var repo repositories.Repository
	_ = json.NewDecoder(response.Body).Decode(&repo)
	response.Body.Close()
	revision := responsepolicies.Revision{Title: "Coverage", Summary: "before alerts", ChangeReason: "initial", Resources: []responsepolicies.Resource{{ID: "repo", Kind: "repository", Name: "coverage", OwnerTeamIDs: []string{"owners"}}}, Teams: []responsepolicies.Team{{ID: "owners", Name: "Owners", MemberIDs: []string{owner.User.ID}, Skills: []string{"operations"}, Contact: "#owners"}}, Rules: []responsepolicies.Rule{{ID: "critical", ResourceIDs: []string{"repo"}, SignalClass: "reliability", Severity: "critical", AccountableTeamID: "owners", RequiredSkills: []string{"operations"}, AcknowledgeSeconds: 300, ResolveSeconds: 3600, ExpectedActions: []string{"assess"}, CommunicationAudienceIDs: []string{"support"}, IncidentCriteria: []string{"user impact"}, Authority: responsepolicies.AuthorityBoundary{PermittedActions: []string{"investigate"}, ProhibitedActions: []string{"deploy"}}}}, Exceptions: []responsepolicies.Exception{{ID: "gap", RuleID: "critical", Reason: "transition", FollowUpID: "task", ExpiresAt: time.Now().Add(24 * time.Hour)}}}
	payload, _ := json.Marshal(map[string]any{"request_id": "create-1", "revision": revision})
	createdResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/response-policies", string(payload), owner.Credential.Token, http.StatusCreated)
	var created responsepolicies.Policy
	_ = json.NewDecoder(createdResponse.Body).Decode(&created)
	createdResponse.Body.Close()
	if created.CurrentVersion != 1 || len(created.Diagnostics) != 1 || created.Diagnostics[0].Kind != "expiring_exception" {
		t.Fatalf("created=%+v", created)
	}
	listed := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repo.ID+"/response-policies", "", owner.Credential.Token, http.StatusOK)
	var result struct {
		Policies []responsepolicies.Policy `json:"response_policies"`
	}
	_ = json.NewDecoder(listed.Body).Decode(&result)
	listed.Body.Close()
	if len(result.Policies) != 1 {
		t.Fatalf("list=%+v", result)
	}
	url := server.URL + "/repositories/" + repo.ID + "/response-policies/" + created.ID + "/revisions"
	payload, _ = json.Marshal(map[string]any{"request_id": "rev-1", "expected_version": 1, "revision": revision})
	authenticatedRequest(t, http.MethodPost, url, string(payload), owner.Credential.Token, http.StatusOK).Body.Close()
	authenticatedRequest(t, http.MethodPost, url, string(payload), owner.Credential.Token, http.StatusOK).Body.Close()
}
