package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/datacommitments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/extensions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/productexperiments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestDataCommitmentAPI(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	commitments, _ := datacommitments.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, commitments))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "data-owner")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"explicit-data"}`, owner.Credential.Token, http.StatusCreated)
	var repo repositories.Repository
	json.NewDecoder(response.Body).Decode(&repo)
	response.Body.Close()
	revision := map[string]any{"title": "Telemetry", "summary": "Operational use", "scopes": []any{map[string]any{"kind": "repository", "name": "Project"}}, "owner_ids": []string{owner.User.ID}, "data_uses": []any{map[string]any{"id": "events", "category": "usage", "subjects": []string{"users"}, "purposes": []string{"reliability"}, "collection": "client events", "processing": []string{"aggregate"}, "sharing": []string{"none"}, "retention": "30 days", "residency": []string{"EU"}, "deletion": "within 24 hours", "consent": "opt in", "owner_ids": []string{owner.User.ID}, "supported": true}}, "links": []any{map[string]any{"kind": "policy", "url": "https://example.test/policy", "label": "Policy"}, map[string]any{"kind": "notice", "url": "https://example.test/notice", "label": "Notice"}}, "exceptions": []any{}, "rationale": "Initial boundary"}
	payload, _ := json.Marshal(map[string]any{"revision": revision})
	invalidRevision := mapsClone(revision)
	invalidRevision["owner_ids"] = []string{"ffffffffffffffffffffffffffffffff"}
	invalidUses := append([]any(nil), revision["data_uses"].([]any)...)
	invalidUse := mapsClone(invalidUses[0].(map[string]any))
	invalidUse["owner_ids"] = []string{"ffffffffffffffffffffffffffffffff"}
	invalidRevision["data_uses"] = []any{invalidUse}
	invalidPayload, _ := json.Marshal(map[string]any{"revision": invalidRevision})
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/data-commitments", string(invalidPayload), owner.Credential.Token, http.StatusForbidden).Body.Close()
	invalidApproverRevision := mapsClone(revision)
	invalidApproverRevision["exceptions"] = []any{map[string]any{"id": "temporary", "data_use_id": "events", "reason": "migration", "mitigation": "manual deletion", "approved_by": "ffffffffffffffffffffffffffffffff", "expires_at": time.Now().UTC().Add(24 * time.Hour)}}
	invalidApproverPayload, _ := json.Marshal(map[string]any{"revision": invalidApproverRevision})
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/data-commitments", string(invalidApproverPayload), owner.Credential.Token, http.StatusForbidden).Body.Close()
	createdResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/data-commitments", string(payload), owner.Credential.Token, http.StatusCreated)
	var created datacommitments.Commitment
	json.NewDecoder(createdResponse.Body).Decode(&created)
	createdResponse.Body.Close()
	listed := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repo.ID+"/data-commitments", "", owner.Credential.Token, http.StatusOK)
	var collection struct {
		Commitments []datacommitments.Commitment `json:"commitments"`
	}
	json.NewDecoder(listed.Body).Decode(&collection)
	listed.Body.Close()
	if len(collection.Commitments) != 1 || collection.Commitments[0].ID != created.ID {
		t.Fatalf("list = %+v", collection)
	}
	payload, _ = json.Marshal(map[string]any{"expected_version": 1, "revision": revision})
	url := server.URL + "/repositories/" + repo.ID + "/data-commitments/" + created.ID + "/revisions"
	invalidApproverPayload, _ = json.Marshal(map[string]any{"expected_version": 1, "revision": invalidApproverRevision})
	authenticatedRequest(t, http.MethodPost, url, string(invalidApproverPayload), owner.Credential.Token, http.StatusForbidden).Body.Close()
	authenticatedRequest(t, http.MethodPost, url, string(payload), owner.Credential.Token, http.StatusOK).Body.Close()
	authenticatedRequest(t, http.MethodPost, url, string(payload), owner.Credential.Token, http.StatusConflict).Body.Close()
}

func mapsClone(value map[string]any) map[string]any {
	out := map[string]any{}
	for key, item := range value {
		out[key] = item
	}
	return out
}

func TestDataCommitmentTypedScopesMustResolveInRepository(t *testing.T) {
	releaseStore, _ := releases.New(t.TempDir())
	extensionStore, _ := extensions.New(t.TempDir())
	experimentStore, _ := productexperiments.New(t.TempDir())
	deploymentStore, _ := deployments.New(t.TempDir())
	repositoryID := "11111111111111111111111111111111"
	for _, kind := range []string{"release", "extension", "experiment", "environment"} {
		err := validateDataCommitmentScopes(repositoryID, []datacommitments.Scope{{Kind: kind, ResourceID: "22222222222222222222222222222222", Name: "foreign"}}, releaseStore, extensionStore, experimentStore, deploymentStore)
		if !errors.Is(err, datacommitments.ErrInvalid) {
			t.Fatalf("%s scope error = %v", kind, err)
		}
	}
	if err := validateDataCommitmentScopes(repositoryID, []datacommitments.Scope{{Kind: "repository", ResourceID: "22222222222222222222222222222222", Name: "foreign"}}, releaseStore, extensionStore, experimentStore, deploymentStore); !errors.Is(err, datacommitments.ErrInvalid) {
		t.Fatalf("cross-repository scope error = %v", err)
	}
}
