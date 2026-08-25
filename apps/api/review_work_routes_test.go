package main

import (
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/reviewplans"
)

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
