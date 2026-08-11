package main

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
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
	authenticatedRequest(t, http.MethodPatch, server.URL+"/repositories/"+repo.ID+"/issues/"+issue.ID, `{"status":"resolved","expected_version":1}`, owner.Credential.Token, http.StatusConflict).Body.Close()
}

func TestIssueAttachmentsRejectDisallowedEvidence(t *testing.T) {
	store, _ := issues.New(t.TempDir())
	_, err := store.Create(issues.Issue{RepositoryID: "r", ReporterID: "u", Title: "x", ExpectedBehavior: "a", ObservedBehavior: "b", Severity: "low", Environment: "c", ReproductionSteps: []string{"d"}, Visibility: "public", Attachments: []issues.Attachment{{Kind: "screenshot", Name: "secret.html", MediaType: "text/html", Data: "x", Size: 1}}})
	if err == nil {
		t.Fatal("unsafe screenshot media type accepted")
	}
}
