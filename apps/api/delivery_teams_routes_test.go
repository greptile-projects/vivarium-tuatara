package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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
	planBody := fmt.Sprintf(`{"expected_version":%d,"plan":{"streams":[{"id":"risk-review","title":"Verify rollback","owner_participant_id":"reviewer","inputs":[{"name":"source","repository_id":%q,"revision":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","artifact":"current source"}],"expected_artifacts":["rollback evidence"],"dependency_ids":[],"acceptance_criteria":["rollback completes within five minutes"],"repository_scope":[{"repository_id":%q,"reference":"main","revision":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","paths":["ops"]}],"integration_order":1,"assumptions":["deployment remains reversible"]}]}}`, team.Version, repository.ID, repository.ID)
	planned := authenticatedRequest(t, http.MethodPut, server.URL+"/delivery-teams/"+team.ID+"/plan", planBody, invitee.Credential.Token, http.StatusOK)
	json.NewDecoder(planned.Body).Decode(&team)
	planned.Body.Close()
	if team.Plan == nil || team.Plan.Revision != 1 || len(team.Plan.Blockers) != 1 || team.Plan.Blockers[0].Kind != "unavailable_access" || team.Plan.Acceptances[0].Status != "accepted" {
		t.Fatalf("execution plan = %#v", team.Plan)
	}
	invalidInput := strings.Replace(planBody, fmt.Sprintf(`"expected_version":%d`, team.Version-1), fmt.Sprintf(`"expected_version":%d`, team.Version), 1)
	invalidInput = strings.Replace(invalidInput, repository.ID, strings.Repeat("f", 32), 1)
	authenticatedRequest(t, http.MethodPut, server.URL+"/delivery-teams/"+team.ID+"/plan", invalidInput, invitee.Credential.Token, http.StatusBadRequest).Body.Close()
	unchanged := authenticatedRequest(t, http.MethodGet, server.URL+"/delivery-teams/"+team.ID, "", invitee.Credential.Token, http.StatusOK)
	var unchangedTeam deliveryteams.Team
	json.NewDecoder(unchanged.Body).Decode(&unchangedTeam)
	unchanged.Body.Close()
	if unchangedTeam.Version != team.Version || unchangedTeam.Plan.Revision != team.Plan.Revision {
		t.Fatalf("invalid input persisted: %#v", unchangedTeam.Plan)
	}
	planBody = strings.Replace(planBody, fmt.Sprintf(`"expected_version":%d`, team.Version-1), fmt.Sprintf(`"expected_version":%d`, team.Version), 1)
	revised := authenticatedRequest(t, http.MethodPut, server.URL+"/delivery-teams/"+team.ID+"/plan", planBody, owner.Credential.Token, http.StatusOK)
	json.NewDecoder(revised.Body).Decode(&team)
	revised.Body.Close()
	if team.Plan.Acceptances[0].Status != "pending" || team.Plan.Acceptances[0].CanRespond {
		t.Fatalf("material replan = %#v", team.Plan)
	}
	responded := authenticatedRequest(t, http.MethodPost, server.URL+"/delivery-teams/"+team.ID+"/plan/participants/reviewer/response", fmt.Sprintf(`{"expected_version":%d,"expected_plan_revision":%d,"decision":"accepted"}`, team.Version, team.Plan.Revision), invitee.Credential.Token, http.StatusOK)
	json.NewDecoder(responded.Body).Decode(&team)
	responded.Body.Close()
	if team.Plan.Acceptances[0].Status != "accepted" || team.Plan.Acceptances[0].RespondedBy != invitee.User.ID {
		t.Fatalf("plan acceptance = %#v", team.Plan.Acceptances)
	}
	statusBody := fmt.Sprintf(`{"expected_version":%d,"status":{"status":"running","summary":"Review is active","progress_percent":40,"revision":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","resource_use":{"unit":"minutes","consumed":3},"questions":[{"id":"rollback-owner","body":"Who approves the rollback window?","ask_of":"team-owner","urgency":"urgent"}],"blockers":[{"kind":"participant_disconnected","summary":"Operations contact disconnected","recovery":"Pause review and escalate to the organizer"}],"predicted_next_action":"Reproduce rollback"}}`, team.Version)
	reported := authenticatedRequest(t, http.MethodPut, server.URL+"/delivery-teams/"+team.ID+"/streams/risk-review/status", statusBody, invitee.Credential.Token, http.StatusOK)
	json.NewDecoder(reported.Body).Decode(&team)
	reported.Body.Close()
	if len(team.StreamStatuses) != 1 || team.StreamStatuses[0].Status != "paused" || team.StreamStatuses[0].Blockers[0].Kind != "participant_disconnected" || team.StreamStatuses[0].Blockers[1].Kind != "access_revoked" {
		t.Fatalf("projected live status = %#v", team.StreamStatuses)
	}
	authenticatedRequest(t, http.MethodPost, server.URL+"/delivery-teams/"+team.ID+"/interventions", fmt.Sprintf(`{"expected_version":%d,"intervention":{"scope":"stream","stream_id":"risk-review","action":"resume","guidance":"Resume without restored access"}}`, team.Version), owner.Credential.Token, http.StatusForbidden).Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/delivery-teams/"+team.ID+"/interventions", fmt.Sprintf(`{"expected_version":%d,"intervention":{"scope":"team","action":"cancel","guidance":"Cancel without organizer authority"}}`, team.Version), invitee.Credential.Token, http.StatusForbidden).Body.Close()
	guided := authenticatedRequest(t, http.MethodPost, server.URL+"/delivery-teams/"+team.ID+"/interventions", fmt.Sprintf(`{"expected_version":%d,"intervention":{"scope":"stream","stream_id":"risk-review","action":"guide","guidance":"Preserve evidence and await the operations contact"}}`, team.Version), owner.Credential.Token, http.StatusOK)
	json.NewDecoder(guided.Body).Decode(&team)
	guided.Body.Close()
	if len(team.Interventions) != 1 || team.Interventions[0].ActorID != owner.User.ID || len(team.StreamStatuses[0].Questions) != 1 {
		t.Fatalf("guided stream = %#v", team)
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
