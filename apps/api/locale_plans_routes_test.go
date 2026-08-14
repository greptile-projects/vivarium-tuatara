package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/localeplans"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestLocalePlanPublicAPIRequiresExactRepositoryRevision(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	plans, _ := localeplans.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, plans))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "locale-owner")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"global"}`, owner.Credential.Token, http.StatusCreated)
	var repo repositories.Repository
	json.NewDecoder(response.Body).Decode(&repo)
	response.Body.Close()
	gr, _ := gitStore.Open(repo.ID)
	tree, _ := gr.WriteObject(storage.TreeObject, []byte{})
	commit, _ := gr.WriteObject(storage.CommitObject, []byte("tree "+string(tree)+"\nauthor Test <test@example.com> 0 +0000\ncommitter Test <test@example.com> 0 +0000\n\nsource\n"))
	_ = gr.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(commit)})
	revision := completeLocaleAPIRevision(owner.User.ID, string(commit))
	payload, _ := json.Marshal(map[string]any{"revision": revision})
	createdResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/locale-plans", string(payload), owner.Credential.Token, http.StatusCreated)
	var created localeplans.Plan
	json.NewDecoder(createdResponse.Body).Decode(&created)
	createdResponse.Body.Close()
	listed := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repo.ID+"/locale-plans", "", owner.Credential.Token, http.StatusOK)
	var collection struct {
		Plans []localeplans.Plan `json:"plans"`
	}
	json.NewDecoder(listed.Body).Decode(&collection)
	listed.Body.Close()
	if len(collection.Plans) != 1 || collection.Plans[0].ID != created.ID || len(collection.Plans[0].Diagnostics) != 0 {
		t.Fatalf("plans = %#v", collection.Plans)
	}
	revision.Resources[0].SourceRevision = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	bad, _ := json.Marshal(map[string]any{"expected_version": 1, "revision": revision})
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/locale-plans/"+created.ID+"/revisions", string(bad), owner.Credential.Token, http.StatusUnprocessableEntity).Body.Close()
}

func completeLocaleAPIRevision(owner, commit string) localeplans.Revision {
	return localeplans.Revision{Title: "French support", Summary: "Checkout works in French", Subject: localeplans.Subject{Kind: "product", ResourceID: "web", Name: "Web"}, Locales: []localeplans.Locale{{ID: "fr-CA", Language: "French", Regions: []string{"CA"}, OwnerIDs: []string{owner}, ReviewerIDs: []string{owner}}}, Terminology: []localeplans.Term{{ID: "checkout", Source: "Checkout", Locale: "fr-CA", Preferred: "Paiement", Context: "button"}}, Formatting: []localeplans.Formatting{{Locale: "fr-CA", Date: "yyyy-MM-dd", Time: "24h", Number: "decimal comma", Currency: "CAD", Units: "metric", Direction: "ltr"}}, Journeys: []localeplans.Journey{{ID: "buy", Name: "Buy", LocaleIDs: []string{"fr-CA"}, OwnerIDs: []string{owner}, Required: true}}, Resources: []localeplans.Resource{{ID: "messages", Kind: "messages", Path: "locales/en.json", Format: "json", SourceRevision: commit, LocaleIDs: []string{"fr-CA"}}}, Thresholds: []localeplans.Threshold{{Locale: "fr-CA", MinimumPercent: 100, RequiredJourneyIDs: []string{"buy"}, RequireOwnerReview: true, RequireRegionalReview: true}}, Rationale: "Initial contract"}
}
