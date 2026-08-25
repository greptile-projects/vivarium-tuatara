package main

import (
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/reviewplans"
)

func TestReviewAgentCredentialMustCoverRouteRepository(t *testing.T) {
	if reviewCredentialCoversRepository(auth.Credential{AgentID: "agent", RepositoryID: "repo-a"}, "repo-b") {
		t.Fatal("cross-repository agent credential accepted")
	}
	if !reviewCredentialCoversRepository(auth.Credential{AgentID: "agent", RepositoryID: "repo-a"}, "repo-a") {
		t.Fatal("exact repository agent credential rejected")
	}
	if !reviewCredentialCoversRepository(auth.Credential{UserID: "human"}, "repo-b") {
		t.Fatal("ordinary human credential incorrectly treated as repository-bound")
	}
}

func TestAgentReviewResolutionActionsCannotChangeDisposition(t *testing.T) {
	for _, action := range []string{"discuss", "classify", "challenge"} {
		if !agentReviewResolutionActionAllowed(action) {
			t.Fatalf("agent discussion action %q rejected", action)
		}
	}
	for _, action := range []string{"accept", "defer", "remains_applicable", "supersede", "resolved", "accepted_risk", "exception"} {
		if agentReviewResolutionActionAllowed(action) {
			t.Fatalf("agent disposition action %q accepted", action)
		}
	}
}

func TestReviewWorkCitationsStayInsideExactArea(t *testing.T) {
	area := reviewplans.Area{Paths: []string{"api/auth.go"}, Questions: []string{"Does retry reconcile?"}, Evidence: []reviewplans.Evidence{{Description: "passing auth check"}}}
	plan := reviewplans.Plan{PolicyRequirements: []string{"human security review"}}
	valid := []reviewplans.WorkCitation{{Kind: "file", Value: "api/auth.go"}, {Kind: "symbol", Value: "api/auth.go#Authorize"}, {Kind: "requirement", Value: "Does retry reconcile?"}, {Kind: "check", Value: "check-public-id"}}
	if !validWorkCitations("repo", "pull", "revision", area, plan, valid[:3], nil, nil, nil) {
		t.Fatal("exact-area citations rejected")
	}
	for _, citation := range []reviewplans.WorkCitation{{Kind: "file", Value: "private/secret.go"}, {Kind: "symbol", Value: "private/secret.go#Token"}, {Kind: "requirement", Value: "embargoed incident"}, {Kind: "secret", Value: "vault://token"}} {
		if validWorkCitations("repo", "pull", "revision", area, plan, []reviewplans.WorkCitation{citation}, nil, nil, nil) {
			t.Fatalf("out-of-scope citation accepted: %#v", citation)
		}
	}
}

func TestReviewWorkCheckCitationBindsExactPullAndRevision(t *testing.T) {
	checks, err := checkruns.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	revision := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	runs, err := checks.Create("repo", "pull", revision, []checkruns.Definition{{Name: "quality", Image: "alpine:3.22", Command: "true"}})
	if err != nil {
		t.Fatal(err)
	}
	area := reviewplans.Area{Paths: []string{"api.go"}}
	plan := reviewplans.Plan{}
	citation := []reviewplans.WorkCitation{{Kind: "check", Value: runs[0].ID}}
	if !validWorkCitations("repo", "pull", revision, area, plan, citation, checks, nil, nil) {
		t.Fatal("exact check citation rejected")
	}
	if validWorkCitations("repo", "another-pull", revision, area, plan, citation, checks, nil, nil) {
		t.Fatal("cross-pull check citation accepted")
	}
	if validWorkCitations("repo", "pull", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", area, plan, citation, checks, nil, nil) {
		t.Fatal("stale-revision check citation accepted")
	}
	if validWorkCitations("repo", "pull", revision, area, plan, []reviewplans.WorkCitation{{Kind: "check", Value: "fabricated"}}, checks, nil, nil) {
		t.Fatal("fabricated check citation accepted")
	}
}

func TestFindingResolutionProjectionPreservesStaleReasoningAndRequiresPassingCurrentCheck(t *testing.T) {
	old := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	current := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	finding := reviewplans.WorkEntry{ID: "finding", Kind: "finding", SourceRevision: old, Body: "Retry may duplicate publication."}
	values := []reviewplans.FindingResolution{{FindingID: "finding", CandidateRevision: old, Action: "resolved", Rationale: "Old candidate changed."}}
	projected := projectFindingResolutions([]reviewplans.WorkEntry{finding}, values, current, nil, "repo", "pull")
	if projected[0]["current_state"] != "stale" || projected[0]["verified"] != false {
		t.Fatalf("moved finding projection = %#v", projected[0])
	}
	values = append(values, reviewplans.FindingResolution{FindingID: "finding", CandidateRevision: current, Action: "remains_applicable", Rationale: "The same path remains in the diff."})
	projected = projectFindingResolutions([]reviewplans.WorkEntry{finding}, values, current, nil, "repo", "pull")
	if projected[0]["current_state"] != "remains_applicable" {
		t.Fatalf("reaffirmed finding projection = %#v", projected[0])
	}
}

func TestFindingResolutionProjectionDoesNotApplyExpiredException(t *testing.T) {
	current := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	expired := time.Now().UTC().Add(-time.Minute)
	future := time.Now().UTC().Add(time.Hour)
	finding := reviewplans.WorkEntry{ID: "finding", Kind: "finding", SourceRevision: current}
	values := []reviewplans.FindingResolution{{FindingID: "finding", CandidateRevision: current, Action: "exception", ExpiresAt: &expired}}
	projected := projectFindingResolutions([]reviewplans.WorkEntry{finding}, values, current, nil, "repo", "pull")
	if projected[0]["current_state"] != "applicable" {
		t.Fatalf("expired exception = %#v", projected[0])
	}
	values = append(values, reviewplans.FindingResolution{FindingID: "finding", CandidateRevision: current, Action: "exception", ExpiresAt: &future})
	projected = projectFindingResolutions([]reviewplans.WorkEntry{finding}, values, current, nil, "repo", "pull")
	if projected[0]["current_state"] != "exception" {
		t.Fatalf("live exception = %#v", projected[0])
	}
}
