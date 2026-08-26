package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/observabilitygaps"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestObservabilityGapPublicAPIKeepsMissingUnderstandingExplicit(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	releaseStore, _ := releases.New(t.TempDir())
	gaps, _ := observabilitygaps.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, releaseStore, gaps))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "observer")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"runtime"}`, owner.Credential.Token, http.StatusCreated)
	var repo repositories.Repository
	_ = json.NewDecoder(response.Body).Decode(&repo)
	response.Body.Close()
	release, _ := releaseStore.Create(releases.Candidate{RepositoryID: repo.ID, Version: "v1", Notes: "runtime", CommitID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", CreatedBy: owner.User.ID})
	revision := observabilitygaps.Revision{Title: "Unknown latency", Question: "Why is latency high?", Behavior: "Requests should finish quickly", AudienceIDs: []string{owner.User.ID}, Decision: "Decide whether to roll back", Services: []observabilitygaps.Service{{ID: "api", Name: "API", Environment: "production"}}, Journeys: []observabilitygaps.Journey{{ID: "checkout", Name: "Checkout", Behavior: "Submit"}}, RequiredTimeliness: "five minutes", Source: observabilitygaps.Source{Kind: "manual", Question: "Why?", Status: "current"}, Evidence: []observabilitygaps.Evidence{{ID: "m", Kind: "metric", Label: "Latency", ReleaseID: release.ID, ReleaseRevision: release.CommitID, Environment: "production", Status: "ambiguous", ObservedAt: time.Now().Add(-31 * 24 * time.Hour)}}, OwnerIDs: []string{owner.User.ID}, SuccessCriteria: []observabilitygaps.Criterion{{ID: "ready", Statement: "Cause is distinguishable", RequiredEvidence: "correlated trace"}}}
	payload, _ := json.Marshal(map[string]any{"request_id": "gap-request", "revision": revision})
	created := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/observability-gaps", string(payload), owner.Credential.Token, http.StatusCreated)
	var gap observabilitygaps.Gap
	_ = json.NewDecoder(created.Body).Decode(&gap)
	created.Body.Close()
	if gap.CurrentVersion != 1 || len(gap.Diagnostics) < 6 {
		t.Fatalf("missing explicit diagnostics: %+v", gap)
	}
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/observability-gaps", string(payload), owner.Credential.Token, http.StatusCreated).Body.Close()
	revision.Evidence[0].ReleaseRevision = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	payload, _ = json.Marshal(map[string]any{"request_id": "bad-release", "revision": revision})
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/observability-gaps", string(payload), owner.Credential.Token, http.StatusUnprocessableEntity).Body.Close()
}
