package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/recoverycommitments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestRecoveryCommitmentAPI(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	commitments, _ := recoverycommitments.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, commitments))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "recovery-owner")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"survivable-project"}`, owner.Credential.Token, http.StatusCreated)
	var repo repositories.Repository
	json.NewDecoder(response.Body).Decode(&repo)
	response.Body.Close()
	revision := map[string]any{"title": "Checkout continuity", "summary": "State required to place an order", "owner_ids": []string{owner.User.ID}, "targets": []any{map[string]any{"id": "orders", "kind": "deployed_service_data", "resource_id": "primary", "name": "Order database", "capability": "Place and inspect orders", "owner_ids": []string{owner.User.ID}, "acceptable_loss_minutes": 5, "restoration_time_minutes": 30, "retention": "35 daily copies", "jurisdictions": []string{"EU"}, "validation_criteria": []string{"place a synthetic order"}, "dependencies": []any{}, "exclusions": []string{"analytics history"}}}, "links": []any{map[string]any{"kind": "service_objective", "id": "checkout-availability", "label": "Service objective"}}, "exceptions": []any{}, "rationale": "Initial shared boundary"}
	payload, _ := json.Marshal(map[string]any{"revision": revision})
	createdResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/recovery-commitments", string(payload), owner.Credential.Token, http.StatusCreated)
	var created recoverycommitments.Commitment
	json.NewDecoder(createdResponse.Body).Decode(&created)
	createdResponse.Body.Close()
	if created.CurrentVersion != 1 || created.Revisions[0].CreatedBy != owner.User.ID {
		t.Fatalf("created = %#v", created)
	}
	listed := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repo.ID+"/recovery-commitments", "", owner.Credential.Token, http.StatusOK)
	var collection struct {
		Commitments []recoverycommitments.Commitment `json:"commitments"`
	}
	json.NewDecoder(listed.Body).Decode(&collection)
	listed.Body.Close()
	if len(collection.Commitments) != 1 || collection.Commitments[0].ID != created.ID {
		t.Fatalf("list = %#v", collection)
	}
	bad := mapsClone(revision)
	bad["owner_ids"] = []string{"ffffffffffffffffffffffffffffffff"}
	badPayload, _ := json.Marshal(map[string]any{"revision": bad})
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/recovery-commitments", string(badPayload), owner.Credential.Token, http.StatusForbidden).Body.Close()
	revision["rationale"] = "Annual review"
	payload, _ = json.Marshal(map[string]any{"expected_version": 1, "revision": revision})
	url := server.URL + "/repositories/" + repo.ID + "/recovery-commitments/" + created.ID + "/revisions"
	authenticatedRequest(t, http.MethodPost, url, string(payload), owner.Credential.Token, http.StatusOK).Body.Close()
	authenticatedRequest(t, http.MethodPost, url, string(payload), owner.Credential.Token, http.StatusConflict).Body.Close()
}
