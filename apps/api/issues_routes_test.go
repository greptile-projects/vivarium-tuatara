package main

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestRepositoryIssueReportLifecycleAndPrivateDuplicateBoundary(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	issueStore, _ := issues.New(t.TempDir())
	releaseStore, _ := releases.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, releaseStore, issueStore))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "issue-owner")
	reporter := createTestAccount(t, server.URL, "issue-reporter")
	outsider := createTestAccount(t, server.URL, "issue-outsider")
	created := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"widget"}`, owner.Credential.Token, http.StatusCreated)
	var repo repositories.Repository
	decodeResponse(t, created, &repo)
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/collaborators", `{"user_id":"`+reporter.User.ID+`"}`, owner.Credential.Token, http.StatusCreated).Body.Close()
	release, err := releaseStore.Create(releases.Candidate{RepositoryID: repo.ID, Version: "v2.1.0", Notes: "Affected release", CommitID: "0123456789012345678901234567890123456789", CreatedBy: owner.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	body := `{"release_id":"` + release.ID + `","title":"CLI exits after upload","expected_behavior":"Upload completes.","observed_behavior":"Process exits with code 2.","severity":"high","environment":"Ubuntu 24.04, CLI 2.1","reproduction_steps":["Create a sample","Run upload"],"visibility":"repository","attachments":[{"kind":"log","name":"upload.log","media_type":"text/plain","data":"ZmFpbGVkCg=="}]}`
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/issues", body, reporter.Credential.Token, http.StatusCreated)
	var issue issues.Issue
	decodeResponse(t, response, &issue)
	if issue.ReporterID != reporter.User.ID || issue.AffectedVersion != "v2.1.0" || issue.Status != "open" || issue.Attachments[0].Size != 7 || len(issue.History) != 1 {
		t.Fatalf("issue = %#v", issue)
	}
	authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repo.ID+"/issue-suggestions?q=upload+exits", "", outsider.Credential.Token, http.StatusNotFound).Body.Close()
	suggestion := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repo.ID+"/issue-suggestions?q=upload+exits", "", owner.Credential.Token, http.StatusOK)
	var found struct {
		Issues []issues.Issue `json:"issues"`
	}
	decodeResponse(t, suggestion, &found)
	if len(found.Issues) != 1 || len(found.Issues[0].Attachments) != 0 {
		t.Fatalf("suggestions leaked or missing: %#v", found)
	}
	comment := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/issues/"+issue.ID+"/comments", `{"body":"I reproduced this on arm64."}`, owner.Credential.Token, http.StatusCreated)
	decodeResponse(t, comment, &issue)
	updated := authenticatedRequest(t, http.MethodPatch, server.URL+"/repositories/"+repo.ID+"/issues/"+issue.ID, `{"status":"triaged","expected_version":`+strconv.Itoa(issue.Version)+`,"message":"Queued for investigation."}`, owner.Credential.Token, http.StatusOK)
	decodeResponse(t, updated, &issue)
	if issue.Status != "triaged" || len(issue.Discussion) != 1 || len(issue.History) != 3 {
		t.Fatalf("updated = %#v", issue)
	}
	authenticatedRequest(t, http.MethodPatch, server.URL+"/repositories/"+repo.ID+"/issues/"+issue.ID, `{"status":"resolved","expected_version":`+strconv.Itoa(issue.Version)+`}`, reporter.Credential.Token, http.StatusForbidden).Body.Close()
	authenticatedRequest(t, http.MethodPatch, server.URL+"/repositories/"+repo.ID+"/issues/"+issue.ID, `{"status":"resolved","expected_version":1}`, owner.Credential.Token, http.StatusConflict).Body.Close()
	resolved := authenticatedRequest(t, http.MethodPatch, server.URL+"/repositories/"+repo.ID+"/issues/"+issue.ID, `{"status":"resolved","expected_version":`+strconv.Itoa(issue.Version)+`}`, owner.Credential.Token, http.StatusOK)
	decodeResponse(t, resolved, &issue)
	authenticatedRequest(t, http.MethodPatch, server.URL+"/repositories/"+repo.ID+"/issues/"+issue.ID, `{"status":"triaged","expected_version":`+strconv.Itoa(issue.Version)+`}`, reporter.Credential.Token, http.StatusForbidden).Body.Close()
	closed := authenticatedRequest(t, http.MethodPatch, server.URL+"/repositories/"+repo.ID+"/issues/"+issue.ID, `{"status":"closed","expected_version":`+strconv.Itoa(issue.Version)+`}`, owner.Credential.Token, http.StatusOK)
	decodeResponse(t, closed, &issue)
	authenticatedRequest(t, http.MethodPatch, server.URL+"/repositories/"+repo.ID+"/issues/"+issue.ID, `{"status":"in_progress","expected_version":`+strconv.Itoa(issue.Version)+`}`, reporter.Credential.Token, http.StatusForbidden).Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/issues", `{"affected_version":"v999.0.0","title":"Unknown release","expected_behavior":"Works","observed_behavior":"Fails","severity":"low","environment":"Linux","reproduction_steps":["Run it"],"visibility":"public"}`, reporter.Credential.Token, http.StatusUnprocessableEntity).Body.Close()
	boundaryData := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", 1<<20)))
	boundaryBody := `{"title":"Large log","expected_behavior":"Works","observed_behavior":"Fails","severity":"low","environment":"Linux","reproduction_steps":["Run it"],"visibility":"repository","attachments":[{"kind":"log","name":"full.log","media_type":"text/plain","size":1048576,"data":"` + boundaryData + `"}]}`
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/issues", boundaryBody, reporter.Credential.Token, http.StatusCreated).Body.Close()
}

func TestIssueAttachmentsRejectDisallowedEvidence(t *testing.T) {
	store, _ := issues.New(t.TempDir())
	_, err := store.Create(issues.Issue{RepositoryID: "r", ReporterID: "u", Title: "x", ExpectedBehavior: "a", ObservedBehavior: "b", Severity: "low", Environment: "c", ReproductionSteps: []string{"d"}, Visibility: "public", Attachments: []issues.Attachment{{Kind: "screenshot", Name: "secret.html", MediaType: "text/html", Data: "x", Size: 1}}})
	if err == nil {
		t.Fatal("unsafe screenshot media type accepted")
	}
}

func TestIssueTriageRetainsCitedHumanAndBoundedAgentConclusions(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	issueRoot := t.TempDir()
	issueStore, _ := issues.New(issueRoot)
	proposalStore, _ := proposals.New(t.TempDir())
	pullStore, _ := pullrequests.New(t.TempDir(), gitStore)
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, proposalStore, pullStore, nil, nil, nil, issueStore))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "triage-owner")
	reporter := createTestAccount(t, server.URL, "triage-reporter")
	created := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"triage"}`, owner.Credential.Token, http.StatusCreated)
	var repo repositories.Repository
	decodeResponse(t, created, &repo)
	gitRepository, err := gitStore.Open(repo.ID)
	if err != nil {
		t.Fatal(err)
	}
	blob, err := gitRepository.WriteObject(storage.BlobObject, []byte("package parser"))
	if err != nil {
		t.Fatal(err)
	}
	tree := writeTestTree(t, gitRepository, testTreeEntry{mode: "100644", name: "parser.go", id: blob})
	revision := writeTestCommit(t, gitRepository, tree, nil, 1700000000, "parser")
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/collaborators", `{"user_id":"`+reporter.User.ID+`"}`, owner.Credential.Token, http.StatusCreated).Body.Close()
	v, err := issueStore.Create(issues.Issue{RepositoryID: repo.ID, ReporterID: reporter.User.ID, Title: "Failure", ExpectedBehavior: "works", ObservedBehavior: "fails", Severity: "high", Environment: "Linux", ReproductionSteps: []string{"run"}, Visibility: "repository"})
	if err != nil {
		t.Fatal(err)
	}
	v, err = issueStore.AddReproductionAttempt(repo.ID, v.ID, reporter.User.ID, issues.ReproductionAttempt{WorkspaceID: "workspace", CommitID: string(revision), DefinitionSHA256: strings.Repeat("b", 64), Commands: []issues.ReproductionCommand{{Name: "reproduce", OutcomeID: "outcome", CommandSHA256: strings.Repeat("c", 64), ExitCode: 1}}, ObservedResult: "failure", Result: "reproduced"})
	if err != nil {
		t.Fatal(err)
	}
	url := server.URL + "/repositories/" + repo.ID + "/issues/" + v.ID
	response := authenticatedRequest(t, http.MethodPut, url+"/triage", `{"expected_version":`+strconv.Itoa(v.Version)+`,"classification":"regression","priority":"urgent","assignee_id":"`+owner.User.ID+`","suspected_revision":"`+string(revision)+`","suspected_owner_ids":["`+owner.User.ID+`"]}`, owner.Credential.Token, http.StatusOK)
	decodeResponse(t, response, &v)
	authenticatedRequest(t, http.MethodPost, url+"/links", `{"expected_version":`+strconv.Itoa(v.Version)+`,"kind":"code","resource_id":"parser.go","revision":"`+string(revision)+`","label":"missing repository"}`, owner.Credential.Token, http.StatusUnprocessableEntity).Body.Close()
	authenticatedRequest(t, http.MethodPost, url+"/links", `{"expected_version":`+strconv.Itoa(v.Version)+`,"kind":"code","repository_id":"`+repo.ID+`","resource_id":"missing.go","revision":"`+string(revision)+`","label":"missing path"}`, owner.Credential.Token, http.StatusUnprocessableEntity).Body.Close()
	response = authenticatedRequest(t, http.MethodPost, url+"/links", `{"expected_version":`+strconv.Itoa(v.Version)+`,"kind":"code","repository_id":"`+repo.ID+`","resource_id":"parser.go","revision":"`+string(revision)+`","label":"parser entry point"}`, owner.Credential.Token, http.StatusCreated)
	decodeResponse(t, response, &v)
	linkID, attemptID := v.Links[0].ID, v.ReproductionAttempts[0].ID
	response = authenticatedRequest(t, http.MethodPost, url+"/findings", `{"expected_version":`+strconv.Itoa(v.Version)+`,"kind":"hypothesis","statement":"The parser changed behavior.","citation_ids":["`+linkID+`","`+attemptID+`"]}`, owner.Credential.Token, http.StatusCreated)
	decodeResponse(t, response, &v)
	response = authenticatedRequest(t, http.MethodPost, url+"/evidence-requests", `{"expected_version":`+strconv.Itoa(v.Version)+`,"body":"Please provide the exact input shape."}`, owner.Credential.Token, http.StatusCreated)
	decodeResponse(t, response, &v)
	response = authenticatedRequest(t, http.MethodPut, url+"/evidence-requests/"+v.EvidenceRequests[0].ID, `{"expected_version":`+strconv.Itoa(v.Version)+`,"response":"The input contains an empty header."}`, reporter.Credential.Token, http.StatusOK)
	decodeResponse(t, response, &v)
	response = authenticatedRequest(t, http.MethodPost, url+"/investigations", `{"expected_version":`+strconv.Itoa(v.Version)+`,"mandate":"Determine whether the parser is responsible.","reproduction_attempt_id":"`+attemptID+`","link_ids":["`+linkID+`"],"expires_in":600}`, owner.Credential.Token, http.StatusCreated)
	var launched struct {
		Issue         issues.Issue          `json:"issue"`
		Investigation issues.Investigation  `json:"investigation"`
		Credential    auth.IssuedCredential `json:"credential"`
	}
	decodeResponse(t, response, &launched)
	authenticatedRequest(t, http.MethodGet, url+"/investigations/"+launched.Investigation.ID, "", reporter.Credential.Token, http.StatusUnauthorized).Body.Close()
	response = authenticatedRequest(t, http.MethodPost, url+"/investigations/"+launched.Investigation.ID+"/findings", `{"kind":"uncertainty","statement":"The selected evidence does not isolate encoding.","citation_ids":["`+attemptID+`"]}`, launched.Credential.Token, http.StatusCreated)
	decodeResponse(t, response, &v)
	response = authenticatedRequest(t, http.MethodPost, url+"/findings", `{"expected_version":`+strconv.Itoa(v.Version)+`,"kind":"finding","statement":"The parser rejects the retained input.","citation_ids":["`+attemptID+`"]}`, owner.Credential.Token, http.StatusCreated)
	decodeResponse(t, response, &v)
	implementationBody := `{"expected_version":` + strconv.Itoa(v.Version) + `,"reproduction_attempt_id":"` + attemptID + `","finding_ids":["` + v.Findings[len(v.Findings)-1].ID + `"],"affected_revision":"` + string(revision) + `","acceptance_criteria":["retained reproduction passes","regression coverage passes"],"assignee_type":"human","assignee_id":"` + owner.User.ID + `"}`
	if err := os.Chmod(issueRoot, 0o500); err != nil {
		t.Fatal(err)
	}
	response = authenticatedRequest(t, http.MethodPost, url+"/implementation", implementationBody, owner.Credential.Token, http.StatusAccepted)
	if response.Header.Get("Vivarium-Recovery-Implementation") != "pending" {
		t.Fatalf("recovery header = %q", response.Header.Get("Vivarium-Recovery-Implementation"))
	}
	response.Body.Close()
	if err := os.Chmod(issueRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	unchanged, err := issueStore.Get(repo.ID, v.ID)
	if err != nil || unchanged.Implementation != nil || unchanged.Version != v.Version {
		t.Fatalf("failed issue publication changed issue: %#v, %v", unchanged, err)
	}
	response = authenticatedRequest(t, http.MethodPost, url+"/implementation", implementationBody, owner.Credential.Token, http.StatusCreated)
	var implementation struct {
		Issue    issues.Issue       `json:"issue"`
		Proposal proposals.Proposal `json:"proposal"`
		Task     proposals.Task     `json:"task"`
	}
	decodeResponse(t, response, &implementation)
	allProposals, err := proposalStore.List(repo.ID)
	if err != nil || len(allProposals) != 1 {
		t.Fatalf("recovery duplicated implementation proposals: %#v, %v", allProposals, err)
	}
	if implementation.Issue.Implementation == nil || implementation.Task.Assignment == nil || implementation.Task.Assignment.AssigneeType != "human" || len(implementation.Task.Assignment.Access.Scopes) != 0 || implementation.Proposal.Reasoning.IssueID != v.ID {
		t.Fatalf("implementation did not preserve governed issue handoff: %#v", implementation)
	}
	authenticatedRequest(t, http.MethodGet, url+"/implementation", "", reporter.Credential.Token, http.StatusOK).Body.Close()
	v = implementation.Issue
	if v.Triage.Classification != "regression" || v.Triage.AssigneeID != owner.User.ID || v.EvidenceRequests[0].State != "answered" || len(v.Findings) != 3 || v.Findings[1].ActorID != launched.Investigation.AgentID {
		t.Fatalf("triage projection = %#v", v)
	}
	authenticatedRequest(t, http.MethodPost, url+"/investigations/"+launched.Investigation.ID+"/findings", `{"kind":"finding","statement":"Unselected claim.","citation_ids":["missing"]}`, launched.Credential.Token, http.StatusUnprocessableEntity).Body.Close()
	response = authenticatedRequest(t, http.MethodPost, url+"/investigations", `{"expected_version":`+strconv.Itoa(v.Version)+`,"mandate":"Check access revocation.","reproduction_attempt_id":"`+attemptID+`","link_ids":["`+linkID+`"],"expires_in":600}`, reporter.Credential.Token, http.StatusCreated)
	var revoked struct {
		Investigation issues.Investigation  `json:"investigation"`
		Credential    auth.IssuedCredential `json:"credential"`
	}
	decodeResponse(t, response, &revoked)
	authenticatedRequest(t, http.MethodDelete, server.URL+"/repositories/"+repo.ID+"/collaborators/"+reporter.User.ID, "", owner.Credential.Token, http.StatusNoContent).Body.Close()
	authenticatedRequest(t, http.MethodPost, url+"/investigations/"+revoked.Investigation.ID+"/findings", `{"kind":"finding","statement":"Must not publish.","citation_ids":["`+attemptID+`"]}`, revoked.Credential.Token, http.StatusForbidden).Body.Close()
}
