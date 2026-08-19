package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/qualityplans"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestQualityPlanPublicAPIIsVersionedAndPermissionScoped(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	plans, _ := qualityplans.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, plans))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "quality-owner")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"quality"}`, owner.Credential.Token, http.StatusCreated)
	var repo repositories.Repository
	_ = json.NewDecoder(response.Body).Decode(&repo)
	response.Body.Close()
	revision := qualityAPIRevision(owner.User.ID)
	payload, _ := json.Marshal(map[string]any{"revision": revision})
	createdResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/quality-plans", string(payload), owner.Credential.Token, http.StatusCreated)
	var created qualityplans.Plan
	_ = json.NewDecoder(createdResponse.Body).Decode(&created)
	createdResponse.Body.Close()
	if created.CurrentVersion != 1 || len(created.Diagnostics) != 0 {
		t.Fatalf("created = %#v", created)
	}

	listed := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repo.ID+"/quality-plans", "", owner.Credential.Token, http.StatusOK)
	var collection struct {
		Plans []qualityplans.Plan `json:"plans"`
	}
	_ = json.NewDecoder(listed.Body).Decode(&collection)
	listed.Body.Close()
	if len(collection.Plans) != 1 || collection.Plans[0].ID != created.ID {
		t.Fatalf("plans = %#v", collection.Plans)
	}

	outsider := createTestAccount(t, server.URL, "quality-outsider")
	revision.OwnerIDs = []string{outsider.User.ID}
	bad, _ := json.Marshal(map[string]any{"expected_version": 1, "revision": revision})
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/quality-plans/"+created.ID+"/revisions", string(bad), owner.Credential.Token, http.StatusForbidden).Body.Close()
}

func qualityAPIRevision(owner string) qualityplans.Revision {
	return qualityplans.Revision{Title: "Checkout", Summary: "Protect purchase behavior", Scopes: []qualityplans.Scope{{Kind: "release", ResourceID: "release-1", Name: "1.0"}}, Environments: []qualityplans.Environment{{ID: "web", Name: "Web", Description: "Current Chrome", Supported: true}}, Requirements: []qualityplans.Requirement{{ID: "pay", SourceKind: "decision", SourceID: "decision-1", Title: "Pay once", Rationale: "Avoid duplicate charges", ExpectedBehavior: "One submission creates one order", Risk: "critical", TestLevels: []string{"unit", "end_to_end", "manual"}, RepresentativeData: "Synthetic payment token", CoverageGoal: "All retry paths", OwnerIDs: []string{owner}, JudgeIDs: []string{owner}, EnvironmentIDs: []string{"web"}, Schedule: "Every candidate", ReleaseThreshold: "All evidence passes", EvidenceIDs: []string{"run"}, Verification: "Observe exactly one order"}}, Evidence: []qualityplans.Evidence{{ID: "run", Kind: "automated", ResourceKind: "check_run", ResourceID: "run-1", Revision: "abc", Summary: "Checkout journey", Status: "passing"}}, OwnerIDs: []string{owner}, ReviewSchedule: "Every release", Rationale: "Initial shared plan"}
}
