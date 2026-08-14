package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/serviceobjectives"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestServiceObjectivePublicAPI(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	contracts, _ := serviceobjectives.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, contracts))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "reliability-owner")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"dependable"}`, owner.Credential.Token, http.StatusCreated)
	var repo repositories.Repository
	_ = json.NewDecoder(response.Body).Decode(&repo)
	response.Body.Close()
	revision := completeServiceObjectiveAPIRevision(owner.User.ID)
	payload, _ := json.Marshal(map[string]any{"revision": revision})
	createdResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/service-objectives", string(payload), owner.Credential.Token, http.StatusCreated)
	var created serviceobjectives.Contract
	_ = json.NewDecoder(createdResponse.Body).Decode(&created)
	createdResponse.Body.Close()
	listed := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repo.ID+"/service-objectives", "", owner.Credential.Token, http.StatusOK)
	var collection struct {
		Values []serviceobjectives.Contract `json:"service_objectives"`
	}
	_ = json.NewDecoder(listed.Body).Decode(&collection)
	listed.Body.Close()
	if len(collection.Values) != 1 || collection.Values[0].ID != created.ID {
		t.Fatalf("collection = %#v", collection.Values)
	}
	payload, _ = json.Marshal(map[string]any{"expected_version": 0, "revision": revision})
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/service-objectives/"+created.ID+"/revisions", string(payload), owner.Credential.Token, http.StatusConflict).Body.Close()
}

func completeServiceObjectiveAPIRevision(owner string) serviceobjectives.Revision {
	return serviceobjectives.Revision{Title: "Availability", Summary: "The user journey remains available.", Scopes: []serviceobjectives.Scope{{Kind: "repository", Name: "Repository service"}}, Indicators: []serviceobjectives.Indicator{{ID: "availability", Name: "Availability", Signal: "journey.success", Calculation: "availability", Unit: "percent", GoodEvent: "success", TotalEvent: "attempt"}}, Windows: []serviceobjectives.Window{{ID: "month", Name: "Month", Duration: "720h", Rolling: true}}, Journeys: []serviceobjectives.Journey{{ID: "use", Name: "Use service", Description: "Complete work", OwnerIDs: []string{owner}}}, Objectives: []serviceobjectives.Objective{{ID: "slo", Name: "Availability SLO", IndicatorID: "availability", WindowID: "month", Target: 99.9, Comparator: "at_least", JourneyIDs: []string{"use"}, OwnerIDs: []string{owner}}}, ErrorBudgets: []serviceobjectives.ErrorBudget{{ObjectiveID: "slo", AllowedFailure: .1, Unit: "percent", BurnPolicy: "Owner review"}}, Severities: []serviceobjectives.Severity{{Level: "warning", BudgetConsumedPercent: 50, Response: "Investigate", OwnerIDs: []string{owner}}}, OwnerIDs: []string{owner}, ExceptionPolicy: serviceobjectives.ExceptionPolicy{MaximumDuration: "168h", ApprovalOwnerIDs: []string{owner}, FollowUpRequired: true}, Rationale: "Initial contract"}
}
