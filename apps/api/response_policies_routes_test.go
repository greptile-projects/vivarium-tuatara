package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/responsepolicies"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestResponsePolicyAPI(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	policies, _ := responsepolicies.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, policies))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "response-owner")
	responder := createTestAccount(t, server.URL, "response-responder")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"coverage"}`, owner.Credential.Token, http.StatusCreated)
	var repo repositories.Repository
	_ = json.NewDecoder(response.Body).Decode(&repo)
	response.Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/collaborators", `{"user_id":"`+responder.User.ID+`"}`, owner.Credential.Token, http.StatusCreated).Body.Close()
	revision := responsepolicies.Revision{Title: "Coverage", Summary: "before alerts", ChangeReason: "initial", Resources: []responsepolicies.Resource{{ID: "repo", Kind: "repository", Name: "coverage", OwnerTeamIDs: []string{"owners"}}}, Teams: []responsepolicies.Team{{ID: "owners", Name: "Owners", MemberIDs: []string{owner.User.ID, responder.User.ID}, Skills: []string{"operations"}, Contact: "#owners"}}, Rules: []responsepolicies.Rule{{ID: "critical", ResourceIDs: []string{"repo"}, SignalClass: "reliability", Severity: "critical", AccountableTeamID: "owners", RequiredSkills: []string{"operations"}, AcknowledgeSeconds: 300, ResolveSeconds: 3600, ExpectedActions: []string{"assess"}, CommunicationAudienceIDs: []string{"support"}, IncidentCriteria: []string{"user impact"}, Authority: responsepolicies.AuthorityBoundary{RequiredAccess: []string{"repository:read"}, PermittedActions: []string{"investigate"}, ProhibitedActions: []string{"deploy"}}}}, Exceptions: []responsepolicies.Exception{{ID: "gap", RuleID: "critical", Reason: "transition", FollowUpID: "task", ExpiresAt: time.Now().Add(24 * time.Hour)}}}
	payload, _ := json.Marshal(map[string]any{"request_id": "create-1", "revision": revision})
	createdResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/response-policies", string(payload), owner.Credential.Token, http.StatusCreated)
	var created responsepolicies.Policy
	_ = json.NewDecoder(createdResponse.Body).Decode(&created)
	createdResponse.Body.Close()
	if created.CurrentVersion != 1 || len(created.Diagnostics) != 1 || created.Diagnostics[0].Kind != "expiring_exception" {
		t.Fatalf("created=%+v", created)
	}
	listed := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repo.ID+"/response-policies", "", owner.Credential.Token, http.StatusOK)
	var result struct {
		Policies []responsepolicies.Policy `json:"response_policies"`
	}
	_ = json.NewDecoder(listed.Body).Decode(&result)
	listed.Body.Close()
	if len(result.Policies) != 1 {
		t.Fatalf("list=%+v", result)
	}
	url := server.URL + "/repositories/" + repo.ID + "/response-policies/" + created.ID + "/revisions"
	payload, _ = json.Marshal(map[string]any{"request_id": "rev-1", "expected_version": 1, "revision": revision})
	authenticatedRequest(t, http.MethodPost, url, string(payload), owner.Credential.Token, http.StatusOK).Body.Close()
	authenticatedRequest(t, http.MethodPost, url, string(payload), owner.Credential.Token, http.StatusOK).Body.Close()
	now := time.Now().UTC()
	availability := []responsepolicies.Availability{{Weekdays: []string{strings.ToLower(now.Weekday().String())}, StartLocal: "00:00", EndLocal: "23:59"}}
	rotationRevision := responsepolicies.RotationRevision{Name: "Primary", PolicyID: created.ID, TeamID: "owners", TimeZone: "UTC", HandoffMinutes: 30, Responders: []responsepolicies.Responder{{UserID: owner.User.ID, Qualifications: []string{"operations"}, Availability: availability, MaxShifts: 5}, {UserID: responder.User.ID, Qualifications: []string{"operations"}, Availability: availability, MaxShifts: 5}}, AbsenceRules: []responsepolicies.AbsenceRule{{Kind: "planned", NoticeHours: 24, Action: "use backup"}}, Shifts: []responsepolicies.Shift{{ID: "shift", StartsAt: now.Add(time.Hour), EndsAt: now.Add(2 * time.Hour), PrimaryUserID: responder.User.ID, BackupUserIDs: []string{owner.User.ID}, RequiredQualifications: []string{"operations"}}}, ChangeReason: "schedule"}
	rotationPayload, _ := json.Marshal(map[string]any{"request_id": "rotation", "revision": rotationRevision})
	rotationResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/response-rotations", string(rotationPayload), owner.Credential.Token, http.StatusCreated)
	var rotation responsepolicies.Rotation
	_ = json.NewDecoder(rotationResponse.Body).Decode(&rotation)
	rotationResponse.Body.Close()
	pendingPayload, _ := json.Marshal(map[string]any{"request_id": "pending-before-removal", "expected_version": rotation.EventVersion, "kind": "delegate", "shift_id": "shift", "to_user_id": owner.User.ID, "reason": "planned handoff", "context": []responsepolicies.DutyContext{{Kind: "active_alert", ResourceID: "alert", Revision: "1", Summary: "context"}}})
	pendingResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/response-rotations/"+rotation.ID+"/duty-events", string(pendingPayload), responder.Credential.Token, http.StatusCreated)
	_ = json.NewDecoder(pendingResponse.Body).Decode(&rotation)
	pendingResponse.Body.Close()
	removed := revision
	removed.Teams = append([]responsepolicies.Team(nil), revision.Teams...)
	removed.Teams[0].MemberIDs = []string{owner.User.ID}
	removed.ChangeReason = "remove responder"
	removedPayload, _ := json.Marshal(map[string]any{"request_id": "remove-responder", "expected_version": 2, "revision": removed})
	authenticatedRequest(t, http.MethodPost, url, string(removedPayload), owner.Credential.Token, http.StatusOK).Body.Close()
	acceptPayload, _ := json.Marshal(map[string]any{"expected_version": rotation.EventVersion})
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/response-rotations/"+rotation.ID+"/duty-events/"+rotation.Events[0].ID+"/accept", string(acceptPayload), owner.Credential.Token, http.StatusForbidden).Body.Close()
	eventPayload, _ := json.Marshal(map[string]any{"request_id": "removed-transfer", "expected_version": rotation.EventVersion, "kind": "delegate", "shift_id": "shift", "to_user_id": owner.User.ID, "reason": "stale membership", "context": []responsepolicies.DutyContext{{Kind: "active_alert", ResourceID: "alert", Revision: "1", Summary: "context"}}})
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/response-rotations/"+rotation.ID+"/duty-events", string(eventPayload), responder.Credential.Token, http.StatusForbidden).Body.Close()
	invalid := revision
	invalid.Rules = append([]responsepolicies.Rule(nil), revision.Rules...)
	invalid.Rules[0].Authority = responsepolicies.AuthorityBoundary{}
	payload, _ = json.Marshal(map[string]any{"request_id": "missing-authority", "revision": invalid})
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/response-policies", string(payload), owner.Credential.Token, http.StatusBadRequest).Body.Close()
	invalid = revision
	invalid.Rules = append([]responsepolicies.Rule(nil), revision.Rules...)
	invalid.Rules[0].Escalations = []responsepolicies.Escalation{{AfterSeconds: 900, TeamID: "owners", ExpectedAction: "coordinate"}}
	payload, _ = json.Marshal(map[string]any{"request_id": "missing-escalation-audience", "revision": invalid})
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/response-policies", string(payload), owner.Credential.Token, http.StatusBadRequest).Body.Close()
}
