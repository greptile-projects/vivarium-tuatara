package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/activities"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/incidents"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestIncidentOperatingPictureFromHealthSignal(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	activity, _ := activities.New(t.TempDir())
	deploymentsStore, _ := deployments.New(t.TempDir())
	incidentStore, _ := incidents.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, activity, nil, nil, deploymentsStore, incidentStore))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "incident-owner")
	responder := createTestAccount(t, server.URL, "incident-responder")
	created := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"service"}`, owner.Credential.Token, http.StatusCreated)
	var repository repositories.Repository
	decodeResponse(t, created, &repository)
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/collaborators", `{"user_id":"`+responder.User.ID+`"}`, owner.Credential.Token, http.StatusCreated).Body.Close()
	environment, err := deploymentsStore.PutEnvironment(deployments.Environment{RepositoryID: repository.ID, Name: "production", Position: 1, Image: "alpine:3.22", Command: "true", TimeoutSeconds: 30, Concurrency: 1, UpdatedBy: owner.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	promotion, err := deploymentsStore.CreatePromotion(deployments.Promotion{RepositoryID: repository.ID, EnvironmentID: environment.ID, ReleaseID: strings.Repeat("a", 32), BuildID: strings.Repeat("b", 32), ArtifactID: strings.Repeat("c", 32), ArtifactSHA256: strings.Repeat("d", 64), CommitID: strings.Repeat("e", 40), Rollout: deployments.RolloutDefinition{Version: 1, Stages: []deployments.RolloutStage{{Name: "canary", Signals: []deployments.HealthSignal{{Name: "errors", Command: "false"}}}}}, InitiatedBy: owner.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	executorID := strings.Repeat("f", 32)
	promotion, err = deploymentsStore.Claim(repository.ID, promotion.ID, executorID, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	promotion, err = deploymentsStore.RecordStage(repository.ID, promotion.ID, executorID, 0, deployments.SignalEvidence{Stage: "canary", Signal: "errors", State: "failed", Message: "error rate exceeded"})
	if err != nil {
		t.Fatal(err)
	}
	promotion, err = deploymentsStore.Complete(repository.ID, promotion.ID, executorID, "failed", "canary signal failed")
	if err != nil {
		t.Fatal(err)
	}
	body := `{"title":"Elevated checkout errors","summary":"Customers cannot complete checkout.","severity":"sev1","scopes":[{"repository_id":"` + repository.ID + `","environment_ids":["` + environment.ID + `"]}],"roles":[{"name":"incident commander","user_id":"` + owner.User.ID + `"}],"source":{"repository_id":"` + repository.ID + `","deployment_id":"` + promotion.ID + `","stage":"canary","signal":"errors"}}`
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/incidents", body, responder.Credential.Token, http.StatusCreated)
	var incident incidents.Incident
	decodeResponse(t, response, &incident)
	if incident.Source == nil || incident.Source.DeploymentID != promotion.ID || len(incident.Timeline) != 1 {
		t.Fatalf("incident = %#v", incident)
	}
	windowStart, windowEnd := time.Now().Add(-10*time.Minute).UTC().Format(time.RFC3339), time.Now().Add(time.Minute).UTC().Format(time.RFC3339)
	findingBody := `{"operation_id":"` + strings.Repeat("8", 32) + `","kind":"hypothesis","message":"The failure began with the canary error spike.","audience":"participants","evidence":[{"kind":"health_signal","repository_id":"` + repository.ID + `","resource_id":"` + promotion.ID + `","query":"canary/errors","window_start":"` + windowStart + `","window_end":"` + windowEnd + `"}]}`
	finding := authenticatedRequest(t, http.MethodPost, server.URL+"/incidents/"+incident.ID+"/findings", findingBody, responder.Credential.Token, http.StatusCreated)
	decodeResponse(t, finding, &incident)
	attached := incident.Timeline[len(incident.Timeline)-1]
	if attached.Kind != "hypothesis" || len(attached.Evidence) != 1 || attached.Evidence[0].Label == "" || attached.Evidence[0].CapturedAt.IsZero() {
		t.Fatalf("finding = %#v", attached)
	}
	retry := authenticatedRequest(t, http.MethodPost, server.URL+"/incidents/"+incident.ID+"/findings", findingBody, responder.Credential.Token, http.StatusCreated)
	decodeResponse(t, retry, &incident)
	if len(incident.Timeline) != 2 {
		t.Fatalf("retry duplicated finding: %#v", incident.Timeline)
	}
	mitigationBody := `{"operation_id":"` + strings.Repeat("6", 32) + `","kind":"pause_rollout","repository_id":"` + repository.ID + `","deployment_id":"` + promotion.ID + `","rationale":"Stop further exposure while the error signal is investigated.","evidence":` + mustJSON(t, attached.Evidence) + `,"health_criteria":[{"stage":"canary","signal":"errors"}]}`
	proposed := authenticatedRequest(t, http.MethodPost, server.URL+"/incidents/"+incident.ID+"/actions", mitigationBody, responder.Credential.Token, http.StatusCreated)
	decodeResponse(t, proposed, &incident)
	if len(incident.Actions) != 1 || incident.Actions[0].Status != "proposed" {
		t.Fatalf("mitigation = %#v", incident.Actions)
	}
	actionID := incident.Actions[0].ID
	authenticatedRequest(t, http.MethodPost, server.URL+"/incidents/"+incident.ID+"/actions/"+actionID+"/decisions", `{"decision":"approve","message":"The bounded pause is justified by the attached failure."}`, responder.Credential.Token, http.StatusConflict).Body.Close()
	approved := authenticatedRequest(t, http.MethodPost, server.URL+"/incidents/"+incident.ID+"/actions/"+actionID+"/decisions", `{"decision":"approve","message":"The bounded pause is justified by the attached failure."}`, owner.Credential.Token, http.StatusCreated)
	decodeResponse(t, approved, &incident)
	governedOperation := strings.Repeat("4", 32)
	if _, _, err = incidentStore.RecordActionAttempt(incident.ID, actionID, governedOperation, owner.User.ID, "pending", "", "Execution reserved."); err != nil {
		t.Fatal(err)
	}
	if _, _, err = incidentStore.RecordActionAttempt(incident.ID, actionID, governedOperation, owner.User.ID, "started", promotion.ID, "Affected rollout paused."); err != nil {
		t.Fatal(err)
	}
	staging, err := deploymentsStore.PutEnvironment(deployments.Environment{RepositoryID: repository.ID, Name: "staging", Position: 2, Image: "alpine:3.22", Command: "true", TimeoutSeconds: 30, Concurrency: 1, UpdatedBy: owner.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	recoveryRelease, recoveryBuild, recoveryArtifact, recoverySHA, recoveryCommit := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32), strings.Repeat("4", 64), strings.Repeat("5", 40)
	knownGood, err := deploymentsStore.CreatePromotion(deployments.Promotion{RepositoryID: repository.ID, EnvironmentID: environment.ID, ReleaseID: recoveryRelease, BuildID: recoveryBuild, ArtifactID: recoveryArtifact, ArtifactSHA256: recoverySHA, CommitID: recoveryCommit, Rollout: promotion.Rollout, InitiatedBy: owner.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	knownOwner := strings.Repeat("1", 32)
	knownGood, err = deploymentsStore.Claim(repository.ID, knownGood.ID, knownOwner, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	_, err = deploymentsStore.RecordStage(repository.ID, knownGood.ID, knownOwner, 0, deployments.SignalEvidence{Stage: "canary", Signal: "errors", State: "passed"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = deploymentsStore.Complete(repository.ID, knownGood.ID, knownOwner, "succeeded", "healthy")
	if err != nil {
		t.Fatal(err)
	}
	unrelated, err := deploymentsStore.CreatePromotion(deployments.Promotion{RepositoryID: repository.ID, EnvironmentID: staging.ID, ReleaseID: recoveryRelease, BuildID: recoveryBuild, ArtifactID: recoveryArtifact, ArtifactSHA256: recoverySHA, CommitID: recoveryCommit, Rollout: promotion.Rollout, InitiatedBy: owner.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	unrelated, err = deploymentsStore.Claim(repository.ID, unrelated.ID, strings.Repeat("3", 32), time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	_, err = deploymentsStore.RecordStage(repository.ID, unrelated.ID, strings.Repeat("3", 32), 0, deployments.SignalEvidence{Stage: "canary", Signal: "errors", State: "passed"})
	if err != nil {
		t.Fatal(err)
	}
	authenticatedRequest(t, http.MethodPost, server.URL+"/incidents/"+incident.ID+"/actions/"+actionID+"/attempts", `{"operation_id":"`+strings.Repeat("2", 32)+`","outcome":"recovered","resource_id":"`+unrelated.ID+`","message":"unrelated staging is healthy"}`, owner.Credential.Token, http.StatusConflict).Body.Close()
	attempted := authenticatedRequest(t, http.MethodPost, server.URL+"/incidents/"+incident.ID+"/actions/"+actionID+"/attempts", `{"operation_id":"`+strings.Repeat("5", 32)+`","outcome":"failed","message":"Deployment was already terminal; no environment mutation occurred."}`, owner.Credential.Token, http.StatusCreated)
	decodeResponse(t, attempted, &incident)
	if incident.Actions[0].Status != "failed" || len(incident.Actions[0].Attempts) != 2 || incident.Actions[0].Attempts[1].Outcome != "failed" {
		t.Fatalf("attempt = %#v", incident.Actions[0])
	}
	legacy, err := deploymentsStore.CreatePromotion(deployments.Promotion{RepositoryID: repository.ID, EnvironmentID: environment.ID, ReleaseID: strings.Repeat("7", 32), BuildID: strings.Repeat("8", 32), ArtifactID: strings.Repeat("9", 32), ArtifactSHA256: strings.Repeat("a", 64), CommitID: strings.Repeat("b", 40), Rollout: promotion.Rollout, InitiatedBy: owner.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	legacyOwner := strings.Repeat("c", 32)
	legacy, err = deploymentsStore.Claim(repository.ID, legacy.ID, legacyOwner, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	legacyRationale := "Pause the newly identified rollout."
	_, err = deploymentsStore.Control(repository.ID, legacy.ID, owner.User.ID, "pause", "running", legacyRationale)
	if err != nil {
		t.Fatal(err)
	}
	legacyBody := `{"operation_id":"` + strings.Repeat("d", 32) + `","kind":"pause_rollout","repository_id":"` + repository.ID + `","deployment_id":"` + legacy.ID + `","rationale":"` + legacyRationale + `","evidence":[{"kind":"deployment","repository_id":"` + repository.ID + `","resource_id":"` + legacy.ID + `"}],"health_criteria":[{"stage":"canary","signal":"errors"}]}`
	legacyProposed := authenticatedRequest(t, http.MethodPost, server.URL+"/incidents/"+incident.ID+"/actions", legacyBody, responder.Credential.Token, http.StatusCreated)
	decodeResponse(t, legacyProposed, &incident)
	legacyAction := incident.Actions[len(incident.Actions)-1]
	authenticatedRequest(t, http.MethodPost, server.URL+"/incidents/"+incident.ID+"/actions/"+legacyAction.ID+"/decisions", `{"decision":"approve","message":"approve a new pause only"}`, owner.Credential.Token, http.StatusCreated).Body.Close()
	legacyOperation := strings.Repeat("e", 32)
	authenticatedRequest(t, http.MethodPost, server.URL+"/incidents/"+incident.ID+"/actions/"+legacyAction.ID+"/attempts", `{"operation_id":"`+legacyOperation+`","outcome":"pending","message":"reserve a new pause"}`, owner.Credential.Token, http.StatusCreated).Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/incidents/"+incident.ID+"/actions/"+legacyAction.ID+"/attempts", `{"operation_id":"`+legacyOperation+`","outcome":"started","resource_id":"`+legacy.ID+`","message":"claim old pause"}`, owner.Credential.Token, http.StatusUnprocessableEntity).Body.Close()
	repo, err := gitStore.Open(repository.ID)
	if err != nil {
		t.Fatal(err)
	}
	commit := writeCommit(t, repo, time.Now().Unix(), "diagnostic revision")
	referenced, err := incidentStore.Create(incidents.Incident{Title: "Related degradation", Summary: "Initial context only.", Severity: "sev2", Status: "investigating", DeclaredBy: owner.User.ID, Scopes: []incidents.Scope{{RepositoryID: repository.ID}}})
	if err != nil {
		t.Fatal(err)
	}
	selectedEvidence := append([]incidents.Evidence{}, attached.Evidence...)
	selectedEvidence = append(selectedEvidence, incidents.Evidence{Kind: "incident", RepositoryID: repository.ID, ResourceID: referenced.ID})
	delegationBody := `{"mandate":"Determine whether the canary failure is consistent with this revision; report uncertainty.","evidence":` + mustJSON(t, selectedEvidence) + `,"revisions":[{"repository_id":"` + repository.ID + `","commit_id":"` + string(commit) + `"}],"expires_in":3600}`
	delegated := authenticatedRequest(t, http.MethodPost, server.URL+"/incidents/"+incident.ID+"/investigations", delegationBody, responder.Credential.Token, http.StatusCreated)
	var launch struct {
		Incident      incidents.Incident      `json:"incident"`
		Investigation incidents.Investigation `json:"investigation"`
		Credential    auth.IssuedCredential   `json:"credential"`
	}
	decodeResponse(t, delegated, &launch)
	if launch.Credential.Scopes[0] != "incidents:investigate" || len(launch.Investigation.Access) != 3 || launch.Investigation.State != "running" {
		t.Fatalf("launch = %#v", launch)
	}
	if _, err = incidentStore.AddUpdate(referenced.ID, strings.Repeat("7", 32), owner.User.ID, "POST_DELEGATION_SECRET", "participants"); err != nil {
		t.Fatal(err)
	}
	contextResponse := authenticatedRequest(t, http.MethodGet, server.URL+"/incidents/"+incident.ID+"/investigations/"+launch.Investigation.ID, "", launch.Credential.Token, http.StatusOK)
	contextBytes, err := io.ReadAll(contextResponse.Body)
	contextResponse.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(contextBytes, []byte("POST_DELEGATION_SECRET")) || !bytes.Contains(contextBytes, []byte("Initial context only.")) {
		t.Fatalf("investigation context was not frozen: %s", contextBytes)
	}
	// The purpose credential can inspect only its frozen packet and stream
	// attributable diagnosis. It cannot exercise a responder mutation scope.
	authenticatedRequest(t, http.MethodPatch, server.URL+"/incidents/"+incident.ID, `{}`, launch.Credential.Token, http.StatusUnauthorized).Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/incidents/"+incident.ID+"/investigations/"+launch.Investigation.ID+"/events", `{"kind":"tool_action","tool":"log.query","message":"Read the selected canary window; no mutation requested."}`, launch.Credential.Token, http.StatusCreated).Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/incidents/"+incident.ID+"/investigations/"+launch.Investigation.ID+"/events", `{"kind":"uncertainty","message":"The signal correlates with the revision but does not prove causation."}`, launch.Credential.Token, http.StatusCreated).Body.Close()
	paused := authenticatedRequest(t, http.MethodPost, server.URL+"/incidents/"+incident.ID+"/investigations/"+launch.Investigation.ID+"/controls", `{"action":"pause"}`, owner.Credential.Token, http.StatusCreated)
	decodeResponse(t, paused, &incident)
	authenticatedRequest(t, http.MethodPost, server.URL+"/incidents/"+incident.ID+"/investigations/"+launch.Investigation.ID+"/events", `{"kind":"finding","message":"blocked"}`, launch.Credential.Token, http.StatusConflict).Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/incidents/"+incident.ID+"/investigations/"+launch.Investigation.ID+"/controls", `{"action":"resume"}`, owner.Credential.Token, http.StatusCreated).Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/incidents/"+incident.ID+"/investigations/"+launch.Investigation.ID+"/controls", `{"action":"cancel"}`, owner.Credential.Token, http.StatusCreated).Body.Close()
	authenticatedRequest(t, http.MethodGet, server.URL+"/incidents/"+incident.ID+"/investigations/"+launch.Investigation.ID, "", launch.Credential.Token, http.StatusUnauthorized).Body.Close()
	update := authenticatedRequest(t, http.MethodPost, server.URL+"/incidents/"+incident.ID+"/updates", `{"operation_id":"`+strings.Repeat("9", 32)+`","message":"Mitigation is holding.","audience":"public"}`, owner.Credential.Token, http.StatusCreated)
	decodeResponse(t, update, &incident)
	entry := incident.Timeline[len(incident.Timeline)-1]
	ack := authenticatedRequest(t, http.MethodPost, server.URL+"/incidents/"+incident.ID+"/timeline/"+entry.ID+"/acknowledgements", "{}", responder.Credential.Token, http.StatusOK)
	decodeResponse(t, ack, &incident)
	patch := authenticatedRequest(t, http.MethodPatch, server.URL+"/incidents/"+incident.ID, `{"expected_version":`+strconv.Itoa(incident.Version)+`,"severity":"sev1","status":"monitoring","roles":[{"name":"incident commander","user_id":"`+owner.User.ID+`"}],"message":"Error rate returned to baseline."}`, owner.Credential.Token, http.StatusOK)
	decodeResponse(t, patch, &incident)
	if incident.Status != "monitoring" {
		t.Fatalf("status = %s", incident.Status)
	}
	list := authenticatedRequest(t, http.MethodGet, server.URL+"/incidents", "", responder.Credential.Token, http.StatusOK)
	defer list.Body.Close()
	var page struct {
		Incidents []incidents.Incident `json:"incidents"`
	}
	if json.NewDecoder(list.Body).Decode(&page) != nil || len(page.Incidents) != 2 || page.Incidents[0].ID != incident.ID {
		t.Fatalf("incident list = %#v", page)
	}
}

func TestInvestigationPromotionContextDoesNotEscapeEvidenceSelection(t *testing.T) {
	now := time.Now().UTC()
	start, end := now.Add(-time.Minute), now.Add(time.Minute)
	selected := deployments.SignalEvidence{Stage: "canary", Signal: "selected", State: "failed", Message: "SELECTED", CreatedAt: now}
	unrelated := deployments.SignalEvidence{Stage: "canary", Signal: "unrelated", State: "failed", Message: "UNRELATED_SIGNAL", CreatedAt: now}
	promotion := deployments.Promotion{ID: strings.Repeat("1", 32), RepositoryID: strings.Repeat("2", 32), EnvironmentID: strings.Repeat("3", 32), Evidence: []deployments.SignalEvidence{selected, unrelated}, Events: []deployments.Event{
		{Sequence: 1, Kind: "rollout.signal_failed", Message: "canary / selected: SELECTED", CreatedAt: now},
		{Sequence: 2, Kind: "rollout.signal_failed", Message: "canary / unrelated: UNRELATED_SIGNAL", CreatedAt: now},
		{Sequence: 3, Kind: "promotion.approved", Message: "UNRELATED_EVENT", CreatedAt: now},
		{Sequence: 4, Kind: "rollout.signal_failed", Message: "canary / selected: OUTSIDE_WINDOW", CreatedAt: now.Add(-time.Hour)},
	}}
	projection := boundedPromotionContext(incidents.Evidence{Kind: "health_signal", Query: "canary/selected", WindowStart: &start, WindowEnd: &end}, promotion)
	encoded, err := json.Marshal(projection)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	if !strings.Contains(text, "SELECTED") || strings.Contains(text, "UNRELATED_SIGNAL") || strings.Contains(text, "UNRELATED_EVENT") || strings.Contains(text, "OUTSIDE_WINDOW") {
		t.Fatalf("projection escaped selection: %s", text)
	}
	logProjection := boundedPromotionContext(incidents.Evidence{Kind: "log", Query: "selected", WindowStart: &start, WindowEnd: &end}, promotion)
	encoded, err = json.Marshal(logProjection)
	if err != nil {
		t.Fatal(err)
	}
	text = string(encoded)
	if !strings.Contains(text, "SELECTED") || strings.Contains(text, "UNRELATED_SIGNAL") || strings.Contains(text, "UNRELATED_EVENT") || strings.Contains(text, "OUTSIDE_WINDOW") {
		t.Fatalf("log projection escaped selection: %s", text)
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
