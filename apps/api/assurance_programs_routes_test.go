package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/assuranceprograms"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestAssuranceProgramAPIIsVersionedAndOwnerScoped(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	programs, _ := assuranceprograms.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, programs))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "assurance-owner")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"assured"}`, owner.Credential.Token, http.StatusCreated)
	var repo repositories.Repository
	_ = json.NewDecoder(response.Body).Decode(&repo)
	response.Body.Close()
	revision := completeAssuranceRevision(owner.User.ID, repo.ID)
	invalid := revision
	invalid.Scopes = append([]assuranceprograms.Scope(nil), revision.Scopes...)
	invalid.Scopes[0].ResourceID = "another-repository"
	invalidPayload, _ := json.Marshal(map[string]any{"revision": invalid})
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/assurance-programs", string(invalidPayload), owner.Credential.Token, http.StatusBadRequest).Body.Close()
	payload, _ := json.Marshal(map[string]any{"revision": revision})
	createdResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/assurance-programs", string(payload), owner.Credential.Token, http.StatusCreated)
	var created assuranceprograms.Program
	_ = json.NewDecoder(createdResponse.Body).Decode(&created)
	createdResponse.Body.Close()
	if created.CurrentVersion != 1 || len(created.Diagnostics) != 0 {
		t.Fatalf("created = %#v", created)
	}
	listed := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repo.ID+"/assurance-programs", "", owner.Credential.Token, http.StatusOK)
	var collection struct {
		Programs []assuranceprograms.Program `json:"programs"`
	}
	_ = json.NewDecoder(listed.Body).Decode(&collection)
	listed.Body.Close()
	if len(collection.Programs) != 1 || collection.Programs[0].ID != created.ID {
		t.Fatalf("programs = %#v", collection.Programs)
	}
	outsider := createTestAccount(t, server.URL, "assurance-outsider")
	revision.Controls[0].OwnerIDs = []string{outsider.User.ID}
	bad, _ := json.Marshal(map[string]any{"expected_version": 1, "revision": revision})
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/assurance-programs/"+created.ID+"/revisions", string(bad), owner.Credential.Token, http.StatusForbidden).Body.Close()
}

func completeAssuranceRevision(owner, repo string) assuranceprograms.Revision {
	return assuranceprograms.Revision{Title: "Repository assurance", Summary: "Concrete project obligations", OwnerIDs: []string{owner}, ReviewPeriodDays: 90, Requirements: []assuranceprograms.Requirement{{ID: "req", Kind: "contractual", Authority: "Customer agreement", Citation: "Schedule A", Title: "Protect data", Summary: "Customer data is protected", Applicability: "Changes to stored customer data", OwnerIDs: []string{owner}, Interpretation: "Encrypt stored data"}}, Scopes: []assuranceprograms.Scope{{ID: "repo", Kind: "repository", ResourceID: repo, Description: "This repository"}}, Controls: []assuranceprograms.Control{{ID: "control", Title: "Storage encryption", Objective: "Protect stored data", RequirementIDs: []string{"req"}, OwnerIDs: []string{owner}, ReviewPeriodDays: 30, Mappings: []assuranceprograms.Mapping{{ScopeID: "repo", Purpose: "Implements the storage boundary"}}, EvidenceCriteria: []assuranceprograms.EvidenceCriterion{{ID: "check", Description: "Encryption configuration is verified", Kind: "automated", ResourceKind: "check_run", ResourceID: "security"}}, Claim: "Managed storage encryption protects customer records"}}}
}
