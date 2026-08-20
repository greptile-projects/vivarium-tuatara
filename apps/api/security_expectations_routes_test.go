package main

import (
	"encoding/json"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/securityexpectations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecurityExpectationsPublicAPIIsVersionedAndPermissionScoped(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	expectations, _ := securityexpectations.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, expectations))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "security-owner")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"secure"}`, owner.Credential.Token, http.StatusCreated)
	var repo repositories.Repository
	_ = json.NewDecoder(response.Body).Decode(&repo)
	response.Body.Close()
	revision := securityAPIRevision(owner.User.ID)
	payload, _ := json.Marshal(map[string]any{"revision": revision})
	createdResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/security-expectations", string(payload), owner.Credential.Token, http.StatusCreated)
	var created securityexpectations.Expectation
	_ = json.NewDecoder(createdResponse.Body).Decode(&created)
	createdResponse.Body.Close()
	if created.CurrentVersion != 1 || len(created.Diagnostics) != 0 {
		t.Fatalf("created=%#v", created)
	}
	listed := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repo.ID+"/security-expectations", "", owner.Credential.Token, http.StatusOK)
	var collection struct {
		Expectations []securityexpectations.Expectation `json:"expectations"`
	}
	_ = json.NewDecoder(listed.Body).Decode(&collection)
	listed.Body.Close()
	if len(collection.Expectations) != 1 || collection.Expectations[0].ID != created.ID {
		t.Fatalf("expectations=%#v", collection.Expectations)
	}
	outsider := createTestAccount(t, server.URL, "security-outsider")
	revision.OwnerIDs = []string{outsider.User.ID}
	bad, _ := json.Marshal(map[string]any{"expected_version": 1, "revision": revision})
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/security-expectations/"+created.ID+"/revisions", string(bad), owner.Credential.Token, http.StatusForbidden).Body.Close()
}

func securityAPIRevision(owner string) securityexpectations.Revision {
	return securityexpectations.Revision{Title: "API security", Summary: "Protect API credentials", Scopes: []securityexpectations.Scope{{Kind: "service", ResourceID: "api", Name: "Public API"}}, Assets: []securityexpectations.Asset{{ID: "token", Name: "Access token", Classification: "secret", Protection: "Never disclose", OwnerIDs: []string{owner}}}, Boundaries: []securityexpectations.Boundary{{ID: "client-api", Name: "Client API", From: "client", To: "api", Direction: "inbound", AssetIDs: []string{"token"}, Guarantees: []string{"TLS and authentication"}}}, Actors: []securityexpectations.Actor{{ID: "attacker", Name: "Remote attacker", Kind: "attacker", Trust: "untrusted", Capabilities: []string{"send requests"}}}, AbuseCases: []securityexpectations.AbuseCase{{ID: "steal", Title: "Steal token", ActorIDs: []string{"attacker"}, AssetIDs: []string{"token"}, BoundaryIDs: []string{"client-api"}, Scenario: "Capture a token", Impact: "Account access", Severity: "critical", ControlIDs: []string{"tls"}, OwnerIDs: []string{owner}}}, Controls: []securityexpectations.Control{{ID: "tls", Name: "Transport security", Requirement: "TLS is mandatory", Kind: "prevent", OwnerIDs: []string{owner}, Evidence: "integration check", Status: "supported"}}, SeverityPolicy: []securityexpectations.SeverityPolicy{{Level: "critical", Response: "Immediate response", ReleaseRule: "Blocks release"}}, Links: []securityexpectations.Link{{Kind: "api", ResourceID: "contract-1", Summary: "Authentication contract"}}, OwnerIDs: []string{owner}, Rationale: "Initial security contract"}
}
