package main

import (
	"encoding/json"
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
	body := `{"title":"Elevated checkout errors","summary":"Customers cannot complete checkout.","severity":"sev1","scopes":[{"repository_id":"` + repository.ID + `","environment_ids":["` + environment.ID + `"]}],"roles":[{"name":"incident commander","user_id":"` + owner.User.ID + `"}],"source":{"repository_id":"` + repository.ID + `","deployment_id":"` + promotion.ID + `","stage":"canary","signal":"errors"}}`
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/incidents", body, responder.Credential.Token, http.StatusCreated)
	var incident incidents.Incident
	decodeResponse(t, response, &incident)
	if incident.Source == nil || incident.Source.DeploymentID != promotion.ID || len(incident.Timeline) != 1 {
		t.Fatalf("incident = %#v", incident)
	}
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
	if json.NewDecoder(list.Body).Decode(&page) != nil || len(page.Incidents) != 1 {
		t.Fatalf("incident list = %#v", page)
	}
}
