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
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/collaborators", `{"user_id":"`+responder.User.ID+`"}`, owner.Credential.Token, http.StatusCreated).Body.Close()
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
	reproductionBody := `{"repository_id":"` + repository.ID + `","version_line":"1.x","definition":{"name":"private parser reproduction","image":"alpine:3.22","command":"test ! -e vulnerable","timeout_seconds":30}}`
	authenticatedRequest(t, http.MethodPost, server.URL+"/security-advisories/"+advisory.ID+"/reproductions", reproductionBody, responder.Credential.Token, http.StatusForbidden).Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/security-advisories/"+advisory.ID+"/reproductions", reproductionBody, owner.Credential.Token, http.StatusCreated).Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/security-advisories/"+advisory.ID+"/repair-sessions/"+repairLaunch.RepairSession.ID+"/verifications", `{}`, responder.Credential.Token, http.StatusConflict).Body.Close()
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
	authenticatedRequest(t, http.MethodPost, server.URL+"/security-advisories/"+advisory.ID+"/repair-sessions/"+repairLaunch.RepairSession.ID+"/comments", `{"body":"Task creator review context."}`, owner.Credential.Token, http.StatusOK).Body.Close()
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

func TestDisclosureRollbackRemovesOnlyRefsCreatedByAttempt(t *testing.T) {
	gitStore, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repository, err := gitStore.Create("abcdef0123456789abcdef0123456789")
	if err != nil {
		t.Fatal(err)
	}
	commit := writeCommit(t, repository, 1, "security repair")
	preexisting := "refs/heads/security/fix-preexisting"
	created := "refs/heads/security/fix-created"
	for _, name := range []string{preexisting, created} {
		if err := repository.CreateReference(storage.Reference{Name: name, Target: string(commit)}); err != nil {
			t.Fatal(err)
		}
	}
	rollbackDisclosureRefs("test-advisory", []createdDisclosureRef{{repository: repository, name: created}})
	if _, err := repository.ReadReference(created); !errors.Is(err, storage.ErrReferenceNotFound) {
		t.Fatalf("attempt-created ref remains: %v", err)
	}
	if ref, err := repository.ReadReference(preexisting); err != nil || ref.Target != string(commit) {
		t.Fatalf("pre-existing matching ref changed: %#v, %v", ref, err)
	}
	// A cleanup failure is fail-closed because pre-publication disclosure refs
	// are staged beneath the transport-hidden repair namespace.
	staged := "refs/heads/vivarium-security/disclosures/test/fix-created"
	if err := repository.CreateReference(storage.Reference{Name: staged, Target: string(commit)}); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/git/test.git/info/refs?service=git-upload-pack", nil)
	recorder := httptest.NewRecorder()
	runGitService(recorder, request, repository, uploadPackService, true, false, "")
	if recorder.Code != http.StatusOK || strings.Contains(recorder.Body.String(), staged) {
		t.Fatalf("staged disclosure ref advertised after failed cleanup: status=%d body=%q", recorder.Code, recorder.Body.String())
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
	authenticatedRequest(t, http.MethodPost, server.URL+"/security-advisories/"+advisory.ID+"/repair-tasks", taskBody, ownerA.Credential.Token, http.StatusUnprocessableEntity).Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repositoryB.ID+"/collaborators", `{"user_id":"`+assignee.User.ID+`"}`, ownerB.Credential.Token, http.StatusCreated).Body.Close()
	repairRepository, err := gitStore.Open(repositoryB.ID)
	if err != nil {
		t.Fatal(err)
	}
	orphan := writeCommit(t, repairRepository, 1700000010, "collaborator orphan base")
	baseCommit, err := repairRepository.ReadCommit(baseB)
	if err != nil {
		t.Fatal(err)
	}
	feature := writeTestCommit(t, repairRepository, baseCommit.Tree, []storage.ObjectID{baseB}, 1700000011, "unmerged feature base")
	for _, untrusted := range []storage.ObjectID{orphan, feature} {
		untrustedBody := strings.Replace(taskBody, string(baseB), string(untrusted), 1)
		authenticatedRequest(t, http.MethodPost, server.URL+"/security-advisories/"+advisory.ID+"/repair-tasks", untrustedBody, ownerA.Credential.Token, http.StatusUnprocessableEntity).Body.Close()
	}
	response = authenticatedRequest(t, http.MethodPost, server.URL+"/security-advisories/"+advisory.ID+"/repair-tasks", taskBody, ownerA.Credential.Token, http.StatusCreated)
	var task struct {
		RepairTask securityadvisories.RepairTask `json:"repair_task"`
	}
	decodeResponse(t, response, &task)
	authenticatedRequest(t, http.MethodPost, server.URL+"/security-advisories/"+advisory.ID+"/repair-tasks/"+task.RepairTask.ID+"/sessions", `{"expires_in":300}`, ownerA.Credential.Token, http.StatusForbidden).Body.Close()
	response = authenticatedRequest(t, http.MethodPost, server.URL+"/security-advisories/"+advisory.ID+"/repair-tasks/"+task.RepairTask.ID+"/sessions", `{"expires_in":300}`, assignee.Credential.Token, http.StatusCreated)
	var launch struct {
		RepairSession securityadvisories.RepairSession `json:"repair_session"`
	}
	decodeResponse(t, response, &launch)
	authenticatedRequest(t, http.MethodPost, server.URL+"/security-advisories/"+advisory.ID+"/repair-sessions/"+launch.RepairSession.ID+"/comments", `{"body":"Unrelated collaborator mutation."}`, reporter.Credential.Token, http.StatusForbidden).Body.Close()
}

func TestRepairVerificationFreezesRequiredDefinitionFromTaskBase(t *testing.T) {
	gitStore, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repository, err := gitStore.Create("99999999999999999999999999999999")
	if err != nil {
		t.Fatal(err)
	}
	trustedConfig, err := repository.WriteObject(storage.BlobObject, []byte(`{"version":1,"checks":[{"name":"quality-gate","image":"alpine:3.22","command":"test -f SECURITY-GATE","working_directory":".","timeout_seconds":30}]}`))
	if err != nil {
		t.Fatal(err)
	}
	trustedVivarium := writeTestTree(t, repository, testTreeEntry{mode: "100644", name: "checks.json", id: trustedConfig})
	trustedTree := writeTestTree(t, repository, testTreeEntry{mode: "40000", name: ".vivarium", id: trustedVivarium})
	base := writeTestCommit(t, repository, trustedTree, nil, 1700000000, "trusted repair base")
	replacementConfig, err := repository.WriteObject(storage.BlobObject, []byte(`{"version":1,"checks":[{"name":"quality-gate","image":"alpine:3.22","command":"true","working_directory":".","timeout_seconds":30}]}`))
	if err != nil {
		t.Fatal(err)
	}
	replacementVivarium := writeTestTree(t, repository, testTreeEntry{mode: "100644", name: "checks.json", id: replacementConfig})
	replacementTree := writeTestTree(t, repository, testTreeEntry{mode: "40000", name: ".vivarium", id: replacementVivarium})
	_ = writeTestCommit(t, repository, replacementTree, []storage.ObjectID{base}, 1700000001, "candidate substitutes command")

	definitions, err := trustedRepairCheckDefinitions(repository, string(base), []string{"quality-gate"})
	if err != nil {
		t.Fatal(err)
	}
	if len(definitions) != 1 || definitions[0].Command != "test -f SECURITY-GATE" {
		t.Fatalf("trusted definitions = %#v", definitions)
	}
}

func TestOrderedVerificationRunsRecoversTerminalReservation(t *testing.T) {
	required := checkruns.Definition{Name: "trusted", Image: "alpine:3.22", Command: "true", TimeoutSeconds: 30}
	reproduction := checkruns.Definition{Name: "private reproduction", Image: "alpine:3.22", Command: "true", TimeoutSeconds: 30}
	commit := strings.Repeat("a", 40)
	runs := []checkruns.Run{
		{ID: strings.Repeat("2", 32), CommitID: commit, Definition: reproduction, State: "succeeded"},
		{ID: strings.Repeat("1", 32), CommitID: commit, Definition: required, State: "failed"},
	}

	ordered, ok := orderedVerificationRuns(runs, []checkruns.Definition{required, reproduction}, commit)
	if !ok || len(ordered) != 2 || ordered[0].ID != strings.Repeat("1", 32) || ordered[1].ID != strings.Repeat("2", 32) {
		t.Fatalf("terminal reservation was not recovered in definition order: %#v, %v", ordered, ok)
	}
}
