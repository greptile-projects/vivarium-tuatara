package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/decisions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestDecisionAPIKeepsPendingContextCollaborativeAndVersioned(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	decisionStore, _ := decisions.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, decisionStore))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "decision-owner")
	peer := createTestAccount(t, server.URL, "decision-peer")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"decision-source"}`, owner.Credential.Token, http.StatusCreated)
	var repository repositories.Repository
	json.NewDecoder(response.Body).Decode(&repository)
	response.Body.Close()
	unknown := "ffffffffffffffffffffffffffffffff"
	invalid := fmt.Sprintf(`{"source":{"kind":"repository","resource_id":%q},"scope":{"question":"Who owns this?","constraints":["No downtime"],"success_measures":["An answer"],"deadline":"2026-09-01T00:00:00Z","affected_resources":[{"kind":"repository","repository_id":%q,"label":"Repository"}],"participants":[{"user_id":%q}],"owner_id":%q}}`, repository.ID, repository.ID, unknown, unknown)
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/decisions", invalid, owner.Credential.Token, http.StatusBadRequest).Body.Close()
	body := fmt.Sprintf(`{"source":{"kind":"repository","resource_id":%q},"scope":{"question":"How should requests be queued?","constraints":["No downtime"],"success_measures":["p95 below 100ms"],"deadline":"2026-09-01T00:00:00Z","affected_resources":[{"kind":"service","repository_id":%q,"label":"API"}],"participants":[{"user_id":%q},{"user_id":%q}],"owner_id":%q}}`, repository.ID, repository.ID, owner.User.ID, peer.User.ID, owner.User.ID)
	created := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/decisions", body, owner.Credential.Token, http.StatusCreated)
	var decision decisions.Decision
	json.NewDecoder(created.Body).Decode(&decision)
	created.Body.Close()
	if decision.Status != "pending" || decision.Source.Kind != "repository" || decision.Version != 1 {
		t.Fatalf("decision = %#v", decision)
	}
	authenticatedRequest(t, http.MethodGet, server.URL+"/decisions/"+decision.ID, "", peer.Credential.Token, http.StatusNotFound).Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/collaborators", `{"user_id":"`+peer.User.ID+`"}`, owner.Credential.Token, http.StatusCreated).Body.Close()
	discussed := authenticatedRequest(t, http.MethodPost, server.URL+"/decisions/"+decision.ID+"/discussion", `{"body":"We should compare backpressure behavior."}`, peer.Credential.Token, http.StatusCreated)
	json.NewDecoder(discussed.Body).Decode(&decision)
	discussed.Body.Close()
	if len(decision.History) != 2 || decision.History[1].ActorID != peer.User.ID {
		t.Fatalf("discussion = %#v", decision.History)
	}
	update := fmt.Sprintf(`{"expected_version":1,"summary":"Added an operability constraint","scope":{"question":"How should requests be queued?","constraints":["No downtime","Operators can drain queues"],"success_measures":["p95 below 100ms"],"deadline":"2026-09-01T00:00:00Z","affected_resources":[{"kind":"service","repository_id":%q,"label":"API"}],"participants":[{"user_id":%q},{"user_id":%q}],"owner_id":%q}}`, repository.ID, owner.User.ID, peer.User.ID, owner.User.ID)
	invalidUpdate := strings.Replace(update, peer.User.ID, unknown, 1)
	authenticatedRequest(t, http.MethodPut, server.URL+"/decisions/"+decision.ID, invalidUpdate, peer.Credential.Token, http.StatusBadRequest).Body.Close()
	changed := authenticatedRequest(t, http.MethodPut, server.URL+"/decisions/"+decision.ID, update, peer.Credential.Token, http.StatusOK)
	json.NewDecoder(changed.Body).Decode(&decision)
	changed.Body.Close()
	if decision.Version != 2 || len(decision.History) != 3 {
		t.Fatalf("changed = %#v", decision)
	}
	authenticatedRequest(t, http.MethodPut, server.URL+"/decisions/"+decision.ID, update, peer.Credential.Token, http.StatusConflict).Body.Close()
	listed := authenticatedRequest(t, http.MethodGet, server.URL+"/decisions?source_kind=repository&source_id="+repository.ID, "", peer.Credential.Token, http.StatusOK)
	var result struct {
		Decisions []decisions.Decision `json:"decisions"`
	}
	json.NewDecoder(listed.Body).Decode(&result)
	listed.Body.Close()
	if len(result.Decisions) != 1 {
		t.Fatalf("listed = %#v", result)
	}
}
