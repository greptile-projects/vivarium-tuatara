package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/charters"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestCharterOwnerStandingProjectionAdvertisesLifecycleActions(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	charterStore, _ := charters.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, charterStore))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "charter-owner-actions")
	participant := createTestAccount(t, server.URL, "charter-participant-actions")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"governed"}`, owner.Credential.Token, http.StatusCreated)
	var repository repositories.Repository
	json.NewDecoder(response.Body).Decode(&repository)
	response.Body.Close()
	revision := charters.Revision{Title: "Community charter", Summary: "Bounded community voice.", Roles: []charters.Role{{Name: "maintainer", Description: "Represent contributors", Eligibility: []string{"repository_owner"}}}, DecisionClasses: []charters.DecisionClass{{Name: "project direction", Description: "Set direction", EligibleRoles: []string{"maintainer"}, Participation: 1, Quorum: 1, Approval: "majority", ProtectedResources: []string{"branch:main"}}}, Procedures: charters.Procedures{Terms: "Thirty days", Removal: "Attributed suspension", Succession: "Nomination", Amendments: "New revision"}}
	if _, err := charterStore.Publish("repository", repository.ID, owner.User.ID, 0, revision); err != nil {
		t.Fatal(err)
	}
	if _, err := charterStore.Approve("repository", repository.ID, owner.User.ID, 1, "approved", "Adopt"); err != nil {
		t.Fatal(err)
	}
	if _, err := charterStore.Activate("repository", repository.ID, owner.User.ID, 1); err != nil {
		t.Fatal(err)
	}
	record, err := charterStore.Invite("repository", repository.ID, owner.User.ID, 0, 1, "human", participant.User.ID, "maintainer", "Represent contributors", []charters.Evidence{{Kind: "contribution", ResourceID: "pull-1", Summary: "Sustained contribution"}}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	standingID := record.Standings[0].ID
	if _, err = charterStore.ActOnStanding("repository", repository.ID, standingID, participant.User.ID, "accept", "Accept", ""); err != nil {
		t.Fatal(err)
	}
	assertOwnerStandingActions(t, server.URL+"/repositories/"+repository.ID+"/charter", owner.Credential.Token, []string{"suspend", "revoke"})
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/charter/standing/"+standingID+"/actions", `{"action":"suspend","reason":"Review standing"}`, owner.Credential.Token, http.StatusOK).Body.Close()
	assertOwnerStandingActions(t, server.URL+"/repositories/"+repository.ID+"/charter", owner.Credential.Token, []string{"reinstate", "revoke"})
}

func assertOwnerStandingActions(t *testing.T, url, token string, want []string) {
	t.Helper()
	response := authenticatedRequest(t, http.MethodGet, url, "", token, http.StatusOK)
	defer response.Body.Close()
	var body struct {
		Standing []charterStandingView `json:"standing"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Standing) != 1 || !sameStandingActions(body.Standing[0].AvailableActions, want) {
		t.Fatalf("available actions = %#v, want %#v", body.Standing, want)
	}
}

func sameStandingActions(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
