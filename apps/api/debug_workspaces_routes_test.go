package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
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
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"runtime-service"}`, owner.Credential.Token, http.StatusCreated)
	var repo repositories.Repository
	decodeResponse(t, response, &repo)
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/collaborators", `{"user_id":"`+reader.User.ID+`"}`, owner.Credential.Token, http.StatusCreated).Body.Close()
	start := time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC)
	created, err := workspaceStore.Create(debugworkspaces.Workspace{RepositoryID: repo.ID, Title: "Runtime failure", Summary: "Intermittent failure", Trigger: debugworkspaces.Reference{Kind: "manual_observation", Label: "operator report"}, Release: debugworkspaces.Reference{ResourceID: strings.Repeat("2", 32), Revision: strings.Repeat("a", 40)}, Environment: debugworkspaces.Reference{ResourceID: strings.Repeat("3", 32)}, TimeStart: start, TimeEnd: start.Add(time.Hour), UserJourney: "checkout", OwnerIDs: []string{owner.User.ID}, Severity: "high", Audience: "repository", Source: debugworkspaces.Reference{Revision: strings.Repeat("a", 40)}, Evidence: []debugworkspaces.Evidence{{Kind: "trace", Reference: "CALLER_CONTROLLED_SECRET_REFERENCE", Label: "CALLER_CONTROLLED_SECRET_LABEL", Visibility: "restricted", Sanitization: "CALLER_CONTROLLED_SECRET_SANITIZATION", Available: true}}}, owner.User.ID)
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
}
