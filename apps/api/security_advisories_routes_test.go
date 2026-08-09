package main

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/activities"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/securityadvisories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestPrivateSecurityReportTriageAndBoundedAccess(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	activity, _ := activities.New(t.TempDir())
	advisories, _ := securityadvisories.New(t.TempDir())
	checks, _ := checkruns.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, activity, nil, checks, advisories))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "security-owner")
	reporter := createTestAccount(t, server.URL, "security-reporter")
	responder := createTestAccount(t, server.URL, "security-responder")
	outsider := createTestAccount(t, server.URL, "security-outsider")
	created := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"public-library"}`, owner.Credential.Token, http.StatusCreated)
	var repository repositories.Repository
	decodeResponse(t, created, &repository)
	authenticatedRequest(t, http.MethodPatch, server.URL+"/repositories/"+repository.ID, `{"visibility":"public"}`, owner.Credential.Token, http.StatusOK).Body.Close()

	body := `{"title":"Parser boundary bypass","description":"A crafted document may escape validation.","affected_repositories":[{"repository_id":"` + repository.ID + `","versions":["1.x","2.0.0"]}],"evidence":[{"label":"Minimal reproduction","description":"A bounded reproduction is available in this protected record."}],"contact":"security-reporter@example.test"}`
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/security-advisories", body, reporter.Credential.Token, http.StatusCreated)
	var advisory securityadvisories.Advisory
	decodeResponse(t, response, &advisory)
	if advisory.ReporterID != reporter.User.ID || advisory.Severity != "untriaged" || advisory.EmbargoState != "reported" {
		t.Fatalf("created advisory = %#v", advisory)
	}
	authenticatedRequest(t, http.MethodGet, server.URL+"/security-advisories/"+advisory.ID, "", outsider.Credential.Token, http.StatusNotFound).Body.Close()
	list := authenticatedRequest(t, http.MethodGet, server.URL+"/security-advisories", "", outsider.Credential.Token, http.StatusOK)
	data, err := io.ReadAll(list.Body)
	if err != nil {
		t.Fatal(err)
	}
	list.Body.Close()
	if text := string(data); strings.Contains(text, advisory.ID) || strings.Contains(text, "Parser boundary") {
		t.Fatalf("private report leaked in collection: %s", text)
	}
	authenticatedRequest(t, http.MethodPatch, server.URL+"/security-advisories/"+advisory.ID, `{"expected_version":1,"severity":"critical","embargo_state":"embargoed"}`, reporter.Credential.Token, http.StatusForbidden).Body.Close()
	response = authenticatedRequest(t, http.MethodPatch, server.URL+"/security-advisories/"+advisory.ID, `{"expected_version":1,"severity":"critical","embargo_state":"embargoed"}`, owner.Credential.Token, http.StatusOK)
	decodeResponse(t, response, &advisory)
	authenticatedRequest(t, http.MethodPost, server.URL+"/security-advisories/"+advisory.ID+"/responders", `{"user_id":"`+responder.User.ID+`"}`, owner.Credential.Token, http.StatusCreated).Body.Close()
	bare, err := gitStore.Open(repository.ID)
	if err != nil {
		t.Fatal(err)
	}
	base := writeCommit(t, bare, 1, "published base")
	if err = bare.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(base)}); err != nil {
		t.Fatal(err)
	}
	repairBody := `{"repository_id":"` + repository.ID + `","version_line":"1.x","title":"Repair parser boundary","mandate":"Remove the boundary bypass without exposing details.","base_commit_id":"` + string(base) + `","assignee_id":"` + responder.User.ID + `","assignee_kind":"human","dependency_task_ids":[]}`
	response = authenticatedRequest(t, http.MethodPost, server.URL+"/security-advisories/"+advisory.ID+"/repair-tasks", repairBody, owner.Credential.Token, http.StatusCreated)
	var repairTask struct {
		RepairTask securityadvisories.RepairTask `json:"repair_task"`
	}
	decodeResponse(t, response, &repairTask)
	response = authenticatedRequest(t, http.MethodPost, server.URL+"/security-advisories/"+advisory.ID+"/repair-tasks/"+repairTask.RepairTask.ID+"/sessions", `{"expires_in":300}`, responder.Credential.Token, http.StatusCreated)
	var repairLaunch struct {
		RepairSession securityadvisories.RepairSession `json:"repair_session"`
		Credential    auth.IssuedCredential            `json:"credential"`
	}
	decodeResponse(t, response, &repairLaunch)
	if repairLaunch.Credential.GitWriteBranch != repairLaunch.RepairSession.Branch || !strings.HasPrefix(repairLaunch.RepairSession.Branch, "refs/heads/vivarium-security/") {
		t.Fatalf("repair launch = %#v", repairLaunch)
	}
	assertGitDiscoveryStatus(t, server.URL+repository.GitRemote+"/info/refs?service=git-receive-pack", repairLaunch.Credential.Token, http.StatusOK)
	repairRefs := authenticatedRequest(t, http.MethodGet, server.URL+repository.GitRemote+"/info/refs?service=git-upload-pack", "", repairLaunch.Credential.Token, http.StatusOK)
	repairData, _ := io.ReadAll(repairRefs.Body)
	repairRefs.Body.Close()
	if !strings.Contains(string(repairData), repairLaunch.RepairSession.Branch) || strings.Contains(string(repairData), " refs/heads/main") {
		t.Fatalf("repair credential advertisement was not exact-branch scoped: %q", repairData)
	}
	ordinary, _ := credentials.Issue(owner.User.ID, auth.Git, "ordinary owner", []string{"git:read"}, time.Hour)
	ordinaryRefs := authenticatedRequest(t, http.MethodGet, server.URL+repository.GitRemote+"/info/refs?service=git-upload-pack", "", ordinary.Token, http.StatusOK)
	ordinaryData, _ := io.ReadAll(ordinaryRefs.Body)
	ordinaryRefs.Body.Close()
	if strings.Contains(string(ordinaryData), repairLaunch.RepairSession.Branch) {
		t.Fatalf("embargoed branch leaked in ordinary advertisement")
	}
	authenticatedRequest(t, http.MethodPost, server.URL+"/security-advisories/"+advisory.ID+"/repair-sessions/"+repairLaunch.RepairSession.ID+"/comments", `{"body":"Unauthorized reporter mutation."}`, reporter.Credential.Token, http.StatusForbidden).Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/security-advisories/"+advisory.ID+"/repair-sessions/"+repairLaunch.RepairSession.ID+"/revoke", `{}`, responder.Credential.Token, http.StatusOK).Body.Close()
	if _, err = bare.ReadReference(repairLaunch.RepairSession.Branch); !errors.Is(err, storage.ErrReferenceNotFound) {
		t.Fatalf("revoked repair branch remains: %v", err)
	}
	response = authenticatedRequest(t, http.MethodPost, server.URL+"/security-advisories/"+advisory.ID+"/repair-tasks/"+repairTask.RepairTask.ID+"/sessions", `{"expires_in":300}`, responder.Credential.Token, http.StatusCreated)
	var restarted struct {
		RepairSession securityadvisories.RepairSession `json:"repair_session"`
	}
	decodeResponse(t, response, &restarted)
	if restarted.RepairSession.ID == repairLaunch.RepairSession.ID {
		t.Fatal("restart reused revoked repair session")
	}
	response = authenticatedRequest(t, http.MethodPost, server.URL+"/security-advisories/"+advisory.ID+"/messages", `{"body":"I can reproduce this against the affected versions."}`, responder.Credential.Token, http.StatusCreated)
	decodeResponse(t, response, &advisory)
	if len(advisory.Messages) != 1 || advisory.Messages[0].ActorID != responder.User.ID {
		t.Fatalf("messages = %#v", advisory.Messages)
	}
	response = authenticatedRequest(t, http.MethodGet, server.URL+"/security-advisories/"+advisory.ID, "", reporter.Credential.Token, http.StatusOK)
	decodeResponse(t, response, &advisory)
	if len(advisory.AccessLog) < 5 || advisory.AccessLog[len(advisory.AccessLog)-1].Action != "viewed" {
		t.Fatalf("access log = %#v", advisory.AccessLog)
	}
	authenticatedRequest(t, http.MethodPost, server.URL+"/security-advisories/"+advisory.ID+"/evidence", `{"kind":"release","repository_id":"`+repository.ID+`","release_id":"`+strings.Repeat("9", 32)+`","label":"Unavailable release","description":"Must fail without panicking."}`, responder.Credential.Token, http.StatusServiceUnavailable).Body.Close()
	releaseKey := strings.Repeat("8", 32)
	runs, err := checks.CreateRequested(repository.ID, releaseKey, strings.Repeat("a", 40), []checkruns.Definition{{Name: "package", Image: "alpine:3.22", Command: "true"}}, owner.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	dependencyPrefix := `{"kind":"dependency","repository_id":"` + repository.ID + `","release_id":"` + releaseKey + `","build_id":"` + runs[0].ID + `","label":"Build image","description":"Frozen build dependency.","dependency":`
	authenticatedRequest(t, http.MethodPost, server.URL+"/security-advisories/"+advisory.ID+"/evidence", dependencyPrefix+`"attacker.invalid/fake:999"}`, responder.Credential.Token, http.StatusUnprocessableEntity).Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/security-advisories/"+advisory.ID+"/evidence", dependencyPrefix+`" alpine:3.22 "}`, responder.Credential.Token, http.StatusUnprocessableEntity).Body.Close()
	response = authenticatedRequest(t, http.MethodPost, server.URL+"/security-advisories/"+advisory.ID+"/evidence", dependencyPrefix+`"alpine:3.22"}`, responder.Credential.Token, http.StatusCreated)
	decodeResponse(t, response, &advisory)
	evidenceID := advisory.Evidence[0].ID
	response = authenticatedRequest(t, http.MethodPost, server.URL+"/security-advisories/"+advisory.ID+"/findings", `{"kind":"hypothesis","statement":"The parser is reachable from the public API.","evidence_ids":["`+evidenceID+`"]}`, responder.Credential.Token, http.StatusCreated)
	decodeResponse(t, response, &advisory)
	impact := `{"expected_version":` + fmt.Sprint(advisory.Version) + `,"repository_id":"` + repository.ID + `","version_line":"1.x","environment":"production","state":"suspected","rationale":"Awaiting artifact confirmation.","evidence_ids":["` + evidenceID + `"]}`
	response = authenticatedRequest(t, http.MethodPut, server.URL+"/security-advisories/"+advisory.ID+"/impact", impact, responder.Credential.Token, http.StatusOK)
	decodeResponse(t, response, &advisory)
	response = authenticatedRequest(t, http.MethodPost, server.URL+"/security-advisories/"+advisory.ID+"/investigations", `{"mandate":"Determine whether the selected reproduction establishes exploitability.","evidence_ids":["`+evidenceID+`"],"expires_in":300}`, responder.Credential.Token, http.StatusCreated)
	var launch struct {
		Investigation securityadvisories.Investigation `json:"investigation"`
		Credential    auth.IssuedCredential            `json:"credential"`
	}
	decodeResponse(t, response, &launch)
	agentHeaders := launch.Credential.Token
	authenticatedRequest(t, http.MethodGet, server.URL+"/security-advisories/"+advisory.ID+"/investigations/"+launch.Investigation.ID, "", agentHeaders, http.StatusOK).Body.Close()
	response = authenticatedRequest(t, http.MethodPost, server.URL+"/security-advisories/"+advisory.ID+"/investigations/"+launch.Investigation.ID+"/findings", `{"kind":"uncertainty","statement":"The reproduction does not identify every shipped artifact.","evidence_ids":["`+evidenceID+`"]}`, agentHeaders, http.StatusCreated)
	response.Body.Close()
	response = authenticatedRequest(t, http.MethodGet, server.URL+"/security-advisories/"+advisory.ID, "", responder.Credential.Token, http.StatusOK)
	decodeResponse(t, response, &advisory)
	if len(advisory.Findings) != 2 || len(advisory.ImpactMatrix) != 1 || advisory.Findings[1].InvestigationID != launch.Investigation.ID {
		t.Fatalf("diagnostic workflow = %#v", advisory)
	}
}

func TestRepairSessionAuthorizationIsRepositorySpecific(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	advisories, _ := securityadvisories.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, advisories))
	defer server.Close()
	ownerA := createTestAccount(t, server.URL, "repair-owner-a")
	ownerB := createTestAccount(t, server.URL, "repair-owner-b")
	reporter := createTestAccount(t, server.URL, "repair-reporter")
	assignee := createTestAccount(t, server.URL, "repair-assignee")
	createRepository := func(owner accountResponse, name string) (repositories.Repository, storage.ObjectID) {
		response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"`+name+`"}`, owner.Credential.Token, http.StatusCreated)
		var record repositories.Repository
		decodeResponse(t, response, &record)
		repository, err := gitStore.Open(record.ID)
		if err != nil {
			t.Fatal(err)
		}
		base := writeCommit(t, repository, 1, name+" base")
		if err = repository.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(base)}); err != nil {
			t.Fatal(err)
		}
		return record, base
	}
	repositoryA, _ := createRepository(ownerA, "private-a")
	repositoryB, baseB := createRepository(ownerB, "private-b")
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repositoryA.ID+"/collaborators", `{"user_id":"`+reporter.User.ID+`"}`, ownerA.Credential.Token, http.StatusCreated).Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repositoryB.ID+"/collaborators", `{"user_id":"`+reporter.User.ID+`"}`, ownerB.Credential.Token, http.StatusCreated).Body.Close()
	body := `{"title":"Coordinated repair","description":"Two private version lines are affected.","affected_repositories":[{"repository_id":"` + repositoryA.ID + `","versions":["1.x"]},{"repository_id":"` + repositoryB.ID + `","versions":["2.x"]}],"evidence":[],"contact":"security@example.test"}`
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/security-advisories", body, reporter.Credential.Token, http.StatusCreated)
	var advisory securityadvisories.Advisory
	decodeResponse(t, response, &advisory)
	authenticatedRequest(t, http.MethodPost, server.URL+"/security-advisories/"+advisory.ID+"/responders", `{"user_id":"`+assignee.User.ID+`"}`, ownerA.Credential.Token, http.StatusCreated).Body.Close()
	taskBody := `{"repository_id":"` + repositoryB.ID + `","version_line":"2.x","title":"Repair private B","mandate":"Fix B.","base_commit_id":"` + string(baseB) + `","assignee_id":"` + assignee.User.ID + `","assignee_kind":"human","dependency_task_ids":[]}`
	response = authenticatedRequest(t, http.MethodPost, server.URL+"/security-advisories/"+advisory.ID+"/repair-tasks", taskBody, ownerA.Credential.Token, http.StatusCreated)
	var task struct {
		RepairTask securityadvisories.RepairTask `json:"repair_task"`
	}
	decodeResponse(t, response, &task)
	authenticatedRequest(t, http.MethodPost, server.URL+"/security-advisories/"+advisory.ID+"/repair-tasks/"+task.RepairTask.ID+"/sessions", `{"expires_in":300}`, ownerA.Credential.Token, http.StatusForbidden).Body.Close()
}
