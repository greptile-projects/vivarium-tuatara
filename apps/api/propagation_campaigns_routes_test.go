package main

import (
	"reflect"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/propagationcampaigns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
)

func TestPropagationSymbolEvidenceKeepsDeclarationsNotSimilarity(t *testing.T) {
	diff := "diff --git a/parser.go b/parser.go\n+++ b/parser.go\n+func Parse(input string) error {\n+  Parse(input)\n+type Result struct {\n+export interface Decoder {\n"
	got := propagationSymbolEvidence(diff)
	want := []string{"Decoder", "Parse", "Result"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected declared-symbol evidence: %#v", got)
	}
}

func TestPropagationReviewStateKeepsActiveChangeRequestBlocking(t *testing.T) {
	for _, reviews := range [][]pullrequests.Review{
		{{Decision: pullrequests.ChangesRequested}, {Decision: pullrequests.Approved}},
		{{Decision: pullrequests.Approved}, {Decision: pullrequests.ChangesRequested}},
	} {
		if got := propagationReviewState(reviews); got != "changes_requested" {
			t.Fatalf("review state = %q, want changes_requested", got)
		}
	}
}

func TestPropagationDeliveryRequiresExactCurrentAcceptedProof(t *testing.T) {
	delivery := propagationcampaigns.DeliveryPath{EquivalenceProofID: "proof", ProofVersion: 2}
	proof := propagationcampaigns.EquivalenceProof{ID: "proof", Version: 2, State: "accepted"}
	if !propagationProofCurrent([]propagationcampaigns.EquivalenceProof{proof}, delivery) {
		t.Fatal("current accepted proof was not admitted")
	}
	for name, changed := range map[string]propagationcampaigns.EquivalenceProof{
		"stale":      func() propagationcampaigns.EquivalenceProof { p := proof; p.Invalidated = true; return p }(),
		"rejected":   func() propagationcampaigns.EquivalenceProof { p := proof; p.State = "rejected"; return p }(),
		"superseded": func() propagationcampaigns.EquivalenceProof { p := proof; p.Version = 3; return p }(),
	} {
		if propagationProofCurrent([]propagationcampaigns.EquivalenceProof{changed}, delivery) {
			t.Fatalf("%s proof counted as current", name)
		}
	}
}
