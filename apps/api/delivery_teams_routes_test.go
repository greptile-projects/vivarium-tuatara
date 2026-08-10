package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deliveryteams"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestDeliveryTeamAPIInvitesWithoutGrantingRepositoryAuthority(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	teamStore, _ := deliveryteams.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, teamStore))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "team-owner")
	invitee := createTestAccount(t, server.URL, "team-invitee")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"team-source"}`, owner.Credential.Token, http.StatusCreated)
	var repository repositories.Repository
	json.NewDecoder(response.Body).Decode(&repository)
	response.Body.Close()
	body := fmt.Sprintf(`{"outcome":{"kind":"planned_outcome","resource_id":"launch","title":"Launch safely"},"charter":{"name":"Launch team","purpose":"Deliver one safe launch","escalation_path":"Escalate risk to the owner","participants":[{"id":"reviewer","principal_type":"human","principal_id":%q,"role":"risk reviewer","responsibility":"Verify rollback readiness","why":"Owns operational review","escalation":"Stop and notify the owner","required_access":[{"repository_id":%q,"level":"write"}]}]}}`, invitee.User.ID, repository.ID)
	created := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/delivery-teams", body, owner.Credential.Token, http.StatusCreated)
	var team deliveryteams.Team
	json.NewDecoder(created.Body).Decode(&team)
	created.Body.Close()
	if team.Participants[0].Status != "pending" {
		t.Fatalf("participant = %#v", team.Participants[0])
	}
	listed := authenticatedRequest(t, http.MethodGet, server.URL+"/delivery-teams", "", invitee.Credential.Token, http.StatusOK)
	var list struct {
		DeliveryTeams []deliveryteams.Team `json:"delivery_teams"`
	}
	json.NewDecoder(listed.Body).Decode(&list)
	listed.Body.Close()
	if len(list.DeliveryTeams) != 1 || list.DeliveryTeams[0].Participants[0].AccessPreview[0].Sufficient {
		t.Fatalf("access preview = %#v", list)
	}
	authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repository.ID, "", invitee.Credential.Token, http.StatusNotFound).Body.Close()
	accepted := authenticatedRequest(t, http.MethodPost, server.URL+"/delivery-teams/"+team.ID+"/participants/reviewer/response", fmt.Sprintf(`{"expected_version":%d,"decision":"accepted"}`, team.Version), invitee.Credential.Token, http.StatusOK)
	json.NewDecoder(accepted.Body).Decode(&team)
	accepted.Body.Close()
	if team.Participants[0].Status != "accepted" || team.Events[1].ActorID != invitee.User.ID {
		t.Fatalf("acceptance = %#v", team)
	}
	authenticatedRequest(t, http.MethodPut, server.URL+"/delivery-teams/"+team.ID, fmt.Sprintf(`{"expected_version":%d,"charter":{}}`, team.Version), invitee.Credential.Token, http.StatusNotFound).Body.Close()
}

func TestAgentGrantAccessRetainsStrongestOverlappingGrant(t *testing.T) {
	o := organizations.Organization{AccessGrants: []organizations.AccessGrant{
		{ID: "write", PrincipalType: "agent", PrincipalID: "agent", Role: "maintainer", Resources: []organizations.ResourceScope{{Kind: "repository", ID: "repo"}}},
		{ID: "viewer", PrincipalType: "agent", PrincipalID: "agent", Role: "viewer", Resources: []organizations.ResourceScope{{Kind: "repository", ID: "repo"}}},
	}}
	level, source := agentGrantAccess(o, "agent", "repo")
	if level != "write" || source != "organization grant write" {
		t.Fatalf("access = %s, %s", level, source)
	}
}
