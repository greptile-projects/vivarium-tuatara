package main

import (
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/datacommitments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/dataflows"
)

func TestComparePrivacyClassifiesConsequencesAndRequirements(t *testing.T) {
	commitments, err := datacommitments.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	base := datacommitments.Revision{Title: "Data", Scopes: []datacommitments.Scope{{Kind: "repository", Name: "repo"}}, OwnerIDs: []string{"owner"}, Links: []datacommitments.Link{{Kind: "policy", URL: "https://example.test/policy", Label: "Policy"}, {Kind: "notice", URL: "https://example.test/notice", Label: "Notice"}}, DataUses: []datacommitments.DataUse{{ID: "events", Category: "usage", Subjects: []string{"users"}, Purposes: []string{"operate"}, Collection: "client", Processing: []string{"aggregate"}, Sharing: []string{"none"}, Retention: "7 days", Deletion: "on request", Consent: "service", OwnerIDs: []string{"owner"}, Supported: true}}}
	c, err := commitments.Create("repo", "owner", base)
	if err != nil {
		t.Fatal(err)
	}
	next := base
	next.Rationale = "analytics"
	next.DataUses = []datacommitments.DataUse{{ID: "events", Category: "usage", Subjects: []string{"users"}, Purposes: []string{"operate", "analytics"}, Collection: "client", Processing: []string{"aggregate"}, Sharing: []string{"vendor"}, Retention: "30 days", Deletion: "self-service", Consent: "opt-in", OwnerIDs: []string{"owner"}, Supported: true}}
	c, err = commitments.Revise(c.ID, 1, "owner", next)
	if err != nil {
		t.Fatal(err)
	}
	ref1 := []dataflows.CommitmentRef{{CommitmentID: c.ID, Version: 1, DataUseIDs: []string{"events"}}}
	ref2 := []dataflows.CommitmentRef{{CommitmentID: c.ID, Version: 2, DataUseIDs: []string{"events"}}}
	target := dataflows.Revision{CommitmentRefs: ref1, Edges: []dataflows.Edge{{ID: "edge", From: "a", To: "b", Operation: "send", DataCategories: []string{"usage"}, Purpose: "operate", CommitmentRefs: ref1}}}
	source := dataflows.Revision{CommitmentRefs: ref2, Edges: []dataflows.Edge{{ID: "edge", From: "a", To: "b", Operation: "send", DataCategories: []string{"usage", "device"}, Purpose: "analytics", RetainedCopy: true, CommitmentRefs: ref2}}}
	changes, requirements, ok := comparePrivacy(commitments, "repo", source, target)
	if !ok || len(changes) < 5 {
		t.Fatalf("changes = %#v, ok=%v", changes, ok)
	}
	kinds := map[string]bool{}
	for _, r := range requirements {
		kinds[r.Kind] = true
	}
	for _, kind := range []string{"owner_acknowledgement", "notice", "consent", "migration", "test", "exception"} {
		if !kinds[kind] {
			t.Errorf("missing requirement %s: %#v", kind, requirements)
		}
	}
}
