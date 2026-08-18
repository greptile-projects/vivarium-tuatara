package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/debugworkspaces"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/incidents"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/infrastructure"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/packages"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/serviceobjectives"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/supportthreads"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestDebugWorkspaceReadRedactsAllRestrictedEvidenceMetadata(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	workspaceStore, _ := debugworkspaces.New(t.TempDir())
	releaseStore, _ := releases.New(t.TempDir())
	deploymentStore, _ := deployments.New(t.TempDir())
	issueStore, _ := issues.New(t.TempDir())
	incidentStore, _ := incidents.New(t.TempDir())
	supportStore, _ := supportthreads.New(t.TempDir())
	objectiveStore, _ := serviceobjectives.New(t.TempDir())
	packageStore, _ := packages.New(t.TempDir())
	infrastructureStore, _ := infrastructure.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, releaseStore, deploymentStore, incidentStore, packageStore, issueStore, supportStore, objectiveStore, infrastructureStore, workspaceStore))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "debug-owner")
	reader := createTestAccount(t, server.URL, "debug-reader")
	creator := createTestAccount(t, server.URL, "debug-creator")
	access := createTestAccount(t, server.URL, "debug-access")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"runtime-service"}`, owner.Credential.Token, http.StatusCreated)
	var repo repositories.Repository
	decodeResponse(t, response, &repo)
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/collaborators", `{"user_id":"`+reader.User.ID+`"}`, owner.Credential.Token, http.StatusCreated).Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/collaborators", `{"user_id":"`+creator.User.ID+`"}`, owner.Credential.Token, http.StatusCreated).Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/collaborators", `{"user_id":"`+access.User.ID+`"}`, owner.Credential.Token, http.StatusCreated).Body.Close()
	start := time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC)
	created, err := workspaceStore.Create(debugworkspaces.Workspace{RepositoryID: repo.ID, Title: "Runtime failure", Summary: "Intermittent failure", Trigger: debugworkspaces.Reference{Kind: "manual_observation", Label: "operator report"}, Release: debugworkspaces.Reference{ResourceID: strings.Repeat("2", 32), Revision: strings.Repeat("a", 40)}, Environment: debugworkspaces.Reference{ResourceID: strings.Repeat("3", 32)}, TimeStart: start, TimeEnd: start.Add(time.Hour), UserJourney: "checkout", OwnerIDs: []string{owner.User.ID}, Severity: "high", Audience: "repository", AccessUserIDs: []string{access.User.ID}, Source: debugworkspaces.Reference{Revision: strings.Repeat("a", 40)}, Evidence: []debugworkspaces.Evidence{{Kind: "trace", Reference: "CALLER_CONTROLLED_SECRET_REFERENCE", Label: "CALLER_CONTROLLED_SECRET_LABEL", Visibility: "restricted", Sanitization: "CALLER_CONTROLLED_SECRET_SANITIZATION", Available: true}}}, creator.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	created, err = workspaceStore.RequestProbe(repo.ID, created.ID, owner.User.ID, debugworkspaces.Probe{Kind: "logs", Purpose: "inspect failures", AudienceUserIDs: []string{owner.User.ID}, ExpiresAt: time.Now().UTC().Add(time.Hour), RequestedPolicy: debugworkspaces.ProbePolicy{DataCategories: []string{"application_logs"}, Privacy: "remove_user_data", Security: "redact_secrets", RetentionHours: 1, SamplePercent: 5, MaxCostCents: 10, MaxLoadPercent: 2}}, created.Version)
	if err != nil {
		t.Fatal(err)
	}
	response = authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repo.ID+"/debugging-workspaces/"+created.ID, "", reader.Credential.Token, http.StatusOK)
	var projected debugworkspaces.Workspace
	if err = json.NewDecoder(response.Body).Decode(&projected); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if len(projected.Evidence) != 1 || projected.Evidence[0].Reference != "" || projected.Evidence[0].Label != "Restricted evidence" || projected.Evidence[0].Sanitization != "" || projected.Evidence[0].Available || projected.Evidence[0].UnavailableReason == "" {
		t.Fatalf("restricted evidence metadata escaped projection: %#v", projected.Evidence)
	}
	if len(projected.Probes) != 0 {
		t.Fatalf("probe escaped its explicit audience: %#v", projected.Probes)
	}
	for _, event := range projected.History {
		if strings.HasPrefix(event.Kind, "probe_") {
			t.Fatalf("probe history escaped its explicit audience: %#v", projected.History)
		}
	}
	response = authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repo.ID+"/debugging-workspaces/"+created.ID, "", creator.Credential.Token, http.StatusOK)
	decodeResponse(t, response, &projected)
	if len(projected.Probes) != 0 {
		t.Fatalf("probe escaped to workspace creator: %#v", projected.Probes)
	}
	response = authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repo.ID+"/debugging-workspaces/"+created.ID, "", access.Credential.Token, http.StatusOK)
	decodeResponse(t, response, &projected)
	if len(projected.Probes) != 0 {
		t.Fatalf("probe escaped to workspace access user: %#v", projected.Probes)
	}
	expiry := time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)
	requestBody := `{"expected_version":2,"probe":{"kind":"traces","purpose":"inspect checkout latency","audience_user_ids":["` + owner.User.ID + `"],"requested_policy":{"data_categories":["timing_spans"],"privacy":"hash_user_identifiers","security":"detect_secrets","retention_hours":2,"sample_percent":5,"max_cost_cents":20,"max_load_percent":2},"expires_at":"` + expiry + `"}}`
	response = authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/debugging-workspaces/"+created.ID+"/probes", requestBody, owner.Credential.Token, http.StatusCreated)
	var requested debugworkspaces.Workspace
	decodeResponse(t, response, &requested)
	if len(requested.Probes) != 2 || requested.Probes[1].Status != "pending" {
		t.Fatalf("probe request = %#v", requested.Probes)
	}
	decisionBody := `{"expected_version":3,"decision":"approved","reason":"stronger transformations protect the capture","policy":{"data_categories":["timing_spans"],"privacy":"remove_user_identifiers","security":"redact_secrets","retention_hours":1,"sample_percent":5,"max_cost_cents":20,"max_load_percent":2},"expires_at":"` + expiry + `"}`
	response = authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/debugging-workspaces/"+created.ID+"/probes/"+requested.Probes[1].ID+"/decision", decisionBody, owner.Credential.Token, http.StatusCreated)
	var approved debugworkspaces.Workspace
	decodeResponse(t, response, &approved)
	if approved.Probes[1].Status != "approved" || approved.Probes[1].DecidedBy != owner.User.ID || approved.Probes[1].ApprovedPolicy == nil || approved.Probes[1].ApprovedPolicy.Privacy != "remove_user_identifiers" {
		t.Fatalf("probe decision = %#v", approved.Probes[1])
	}
	visibleLifecycle := false
	for _, event := range approved.History {
		if event.Kind == "probe_approved" && event.To == approved.Probes[1].ID {
			visibleLifecycle = true
		}
	}
	if !visibleLifecycle {
		t.Fatalf("explicit audience lost probe history: %#v", approved.History)
	}
	eventBody := `{"expected_version":4,"kind":"hypothesis","value":"latency correlates with queue depth","message":"excluded collaborator observation"}`
	response = authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/debugging-workspaces/"+created.ID+"/events", eventBody, reader.Credential.Token, http.StatusCreated)
	var eventProjection debugworkspaces.Workspace
	decodeResponse(t, response, &eventProjection)
	if len(eventProjection.Probes) != 0 {
		t.Fatalf("event response escaped probe audience: %#v", eventProjection.Probes)
	}
	for _, event := range eventProjection.History {
		if strings.HasPrefix(event.Kind, "probe_") {
			t.Fatalf("event response escaped hidden probe history: %#v", eventProjection.History)
		}
	}
	if len(eventProjection.Hypotheses) != 1 || eventProjection.Hypotheses[0].CreatedBy != reader.User.ID {
		t.Fatalf("event response lost permitted mutation: %#v", eventProjection.Hypotheses)
	}
	claimBody := `{"expected_version":5,"claim":{"kind":"hypothesis","statement":"retry exhaustion caused the timeout","uncertainty":"the restricted trace remains unavailable","confidence":"medium"},"citations":[{"kind":"runtime_evidence","evidence_id":"` + created.Evidence[0].ID + `","label":"restricted trace selection"}]}`
	response = authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/debugging-workspaces/"+created.ID+"/claims", claimBody, reader.Credential.Token, http.StatusCreated)
	var diagnosed debugworkspaces.Workspace
	decodeResponse(t, response, &diagnosed)
	if len(diagnosed.Claims) != 1 || diagnosed.Claims[0].Status != "blocked" || diagnosed.Citations[0].Accessible || diagnosed.Citations[0].Label != "Inaccessible evidence" {
		t.Fatalf("inaccessible claim projection = %#v / %#v", diagnosed.Claims, diagnosed.Citations)
	}
	withUnselected, err := workspaceStore.AddClaim(repo.ID, created.ID, owner.User.ID, []debugworkspaces.Citation{{Kind: "commit", Revision: created.Source.Revision, Label: "unselected source context", Accessible: true}}, debugworkspaces.Claim{Kind: "finding", Statement: "unselected finding", Uncertainty: "not delegated", Confidence: "low"}, diagnosed.Version)
	if err != nil {
		t.Fatal(err)
	}
	withUnselected, err = workspaceStore.RespondClaim(repo.ID, created.ID, diagnosed.Claims[0].ID, owner.User.ID, "support", "undelegated evidence discussion", []string{withUnselected.Citations[1].ID}, withUnselected.Version)
	if err != nil {
		t.Fatal(err)
	}
	agentBody := `{"expected_version":8,"mandate":"test only the selected correlation","citation_ids":["` + diagnosed.Citations[0].ID + `"],"expires_in":300}`
	response = authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/debugging-workspaces/"+created.ID+"/agent-investigations", agentBody, owner.Credential.Token, http.StatusCreated)
	var launched struct {
		DebuggingWorkspace debugworkspaces.Workspace          `json:"debugging_workspace"`
		AgentInvestigation debugworkspaces.AgentInvestigation `json:"agent_investigation"`
		Credential         auth.IssuedCredential              `json:"credential"`
	}
	decodeResponse(t, response, &launched)
	agentClaim := `{"expected_version":9,"claim":{"kind":"uncertainty","statement":"the selected evidence cannot establish the branch","uncertainty":"runtime evidence is blocked","confidence":"low","citation_ids":["` + diagnosed.Citations[0].ID + `"]}}`
	response = authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/debugging-workspaces/"+created.ID+"/agent-investigations/"+launched.AgentInvestigation.ID+"/claims", agentClaim, launched.Credential.Token, http.StatusCreated)
	var agentPublished struct {
		Investigation debugworkspaces.AgentInvestigation `json:"investigation"`
		Citations     []debugworkspaces.Citation         `json:"citations"`
		Claims        []debugworkspaces.Claim            `json:"claims"`
	}
	decodeResponse(t, response, &agentPublished)
	if len(agentPublished.Citations) != 1 || agentPublished.Citations[0].ID != diagnosed.Citations[0].ID || len(agentPublished.Claims) != 2 || agentPublished.Claims[1].CreatedBy != launched.AgentInvestigation.AgentID {
		t.Fatalf("agent claim packet escaped selection or lost attribution = citations %#v, claims %#v", agentPublished.Citations, agentPublished.Claims)
	}
	if len(agentPublished.Claims[0].Responses) != 0 {
		t.Fatalf("POST packet exposed response based on undelegated evidence: %#v", agentPublished.Claims[0].Responses)
	}
	for _, citation := range agentPublished.Citations {
		if citation.ID == withUnselected.Citations[1].ID {
			t.Fatalf("unselected citation escaped: %#v", citation)
		}
	}
	for _, claim := range agentPublished.Claims {
		if claim.ID == withUnselected.Claims[1].ID {
			t.Fatalf("unselected claim escaped: %#v", claim)
		}
	}
	response = authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repo.ID+"/debugging-workspaces/"+created.ID+"/agent-investigations/"+launched.AgentInvestigation.ID, "", launched.Credential.Token, http.StatusOK)
	var agentRead struct {
		Claims []debugworkspaces.Claim `json:"claims"`
	}
	decodeResponse(t, response, &agentRead)
	if len(agentRead.Claims) != 2 || len(agentRead.Claims[0].Responses) != 0 {
		t.Fatalf("GET packet exposed undelegated response: %#v", agentRead.Claims)
	}
	control := `{"expected_version":10,"action":"pause","message":"wait for privacy-owner input"}`
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/debugging-workspaces/"+created.ID+"/agent-investigations/"+launched.AgentInvestigation.ID+"/controls", control, owner.Credential.Token, http.StatusCreated).Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/debugging-workspaces/"+created.ID+"/agent-investigations/"+launched.AgentInvestigation.ID+"/claims", strings.Replace(agentClaim, `"expected_version":9`, `"expected_version":11`, 1), launched.Credential.Token, http.StatusForbidden).Body.Close()

	restricted, err := workspaceStore.Create(debugworkspaces.Workspace{RepositoryID: repo.ID, Title: "Restricted diagnosis", Summary: "Need-to-know runtime context", Trigger: debugworkspaces.Reference{Kind: "manual_observation", Label: "private report"}, Release: debugworkspaces.Reference{ResourceID: strings.Repeat("4", 32), Revision: strings.Repeat("c", 40)}, Environment: debugworkspaces.Reference{ResourceID: strings.Repeat("5", 32)}, TimeStart: start, TimeEnd: start.Add(time.Hour), UserJourney: "private checkout", OwnerIDs: []string{owner.User.ID}, Severity: "critical", Audience: "restricted", AccessUserIDs: []string{owner.User.ID}, Source: debugworkspaces.Reference{Revision: strings.Repeat("c", 40)}}, owner.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	restricted, err = workspaceStore.AddClaim(repo.ID, restricted.ID, owner.User.ID, []debugworkspaces.Citation{{Kind: "commit", Revision: restricted.Source.Revision, Label: "affected source", Accessible: true}}, debugworkspaces.Claim{Kind: "hypothesis", Statement: "private hypothesis", Uncertainty: "private uncertainty", Confidence: "low"}, restricted.Version)
	if err != nil {
		t.Fatal(err)
	}
	restricted, restrictedAgent, err := workspaceStore.StartAgent(repo.ID, restricted.ID, owner.User.ID, strings.Repeat("6", 32), strings.Repeat("7", 32), "private mandate", []string{restricted.Citations[0].ID}, restricted.Version)
	if err != nil {
		t.Fatal(err)
	}
	excludedMutations := []struct{ path, body string }{
		{"/claims/" + restricted.Claims[0].ID + "/responses", `{"expected_version":3,"kind":"dispute","message":"guess","citation_ids":[]}`},
		{"/owner-requests", `{"expected_version":3,"request":{"owner_type":"code","owner_id":"` + owner.User.ID + `","question":"guess","citation_ids":[]}}`},
		{"/owner-requests/" + strings.Repeat("8", 32) + "/answer", `{"expected_version":3,"response":"guess"}`},
		{"/agent-investigations", `{"expected_version":3,"mandate":"guess","citation_ids":["` + restricted.Citations[0].ID + `"],"expires_in":300}`},
		{"/agent-investigations/" + restrictedAgent.ID + "/controls", `{"expected_version":3,"action":"pause","message":"guess"}`},
	}
	for _, mutation := range excludedMutations {
		authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/debugging-workspaces/"+restricted.ID+mutation.path, mutation.body, reader.Credential.Token, http.StatusNotFound).Body.Close()
	}
	afterRestricted, err := workspaceStore.Get(repo.ID, restricted.ID)
	if err != nil || afterRestricted.Version != restricted.Version {
		t.Fatalf("excluded mutation changed restricted workspace: version %d, err %v", afterRestricted.Version, err)
	}
}

func TestDebugRepairChecksMustMatchDeployedRevision(t *testing.T) {
	zero := 0
	source, deployed := strings.Repeat("a", 40), strings.Repeat("b", 40)
	runs := []checkruns.Run{{ID: strings.Repeat("1", 32), CommitID: source, State: "completed", ExitCode: &zero}, {ID: strings.Repeat("2", 32), CommitID: deployed, State: "completed", ExitCode: &zero}}
	selected := debugPassingChecks(runs, []string{runs[0].ID, runs[1].ID}, deployed)
	if len(selected) != 1 || selected[runs[1].ID].CommitID != deployed {
		t.Fatalf("selected checks from wrong revision: %#v", selected)
	}
}
