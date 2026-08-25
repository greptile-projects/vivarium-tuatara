package main

import (
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/reviewplans"
)

func TestReviewWorkCitationsStayInsideExactArea(t *testing.T) {
	area := reviewplans.Area{Paths: []string{"api/auth.go"}, Questions: []string{"Does retry reconcile?"}, Evidence: []reviewplans.Evidence{{Description: "passing auth check"}}}
	plan := reviewplans.Plan{PolicyRequirements: []string{"human security review"}}
	valid := []reviewplans.WorkCitation{{Kind: "file", Value: "api/auth.go"}, {Kind: "symbol", Value: "api/auth.go#Authorize"}, {Kind: "requirement", Value: "Does retry reconcile?"}, {Kind: "check", Value: "check-public-id"}}
	if !validWorkCitations(area, plan, valid) {
		t.Fatal("exact-area citations rejected")
	}
	for _, citation := range []reviewplans.WorkCitation{{Kind: "file", Value: "private/secret.go"}, {Kind: "symbol", Value: "private/secret.go#Token"}, {Kind: "requirement", Value: "embargoed incident"}, {Kind: "secret", Value: "vault://token"}} {
		if validWorkCitations(area, plan, []reviewplans.WorkCitation{citation}) {
			t.Fatalf("out-of-scope citation accepted: %#v", citation)
		}
	}
}
