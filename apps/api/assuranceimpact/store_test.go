package assuranceimpact

import "testing"

func TestAssessmentInvalidatesOnlyChangedControlAndRequiresOwners(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	impacts := []ControlImpact{
		{ControlID: "access", ControlTitle: "Access review", ControlDigest: "digest-a", Applicability: "affected", Rationale: "role policy changed", RequiredOwnerIDs: []string{"owner"}, Current: true},
		{ControlID: "retention", ControlTitle: "Retention", ControlDigest: "digest-b", Applicability: "not_affected", Rationale: "no stored data changed", Current: true},
	}
	assessment, err := store.Create("repo", "reviewer", Candidate{Kind: "pull_request", ID: "pull", Revision: "commit-1"}, "program", 1, impacts)
	if err != nil {
		t.Fatal(err)
	}
	if assessment.Ready {
		t.Fatal("unacknowledged affected control was ready")
	}
	assessment, err = store.AddEvent("repo", assessment.ID, assessment.Version, "agent", "agent", Event{Kind: "challenge", ControlID: "access", Body: "The service role broadens the regulated export path.", Citations: []Citation{{Kind: "policy", ResourceID: "policy/access.rego", Revision: "commit-1", Summary: "export role rule"}}}, Current{CandidateRevision: "commit-1", ControlDigests: map[string]string{"access": "digest-a", "retention": "digest-b"}})
	if err != nil || len(assessment.Events) != 1 || assessment.Events[0].ActorType != "agent" {
		t.Fatalf("event = %#v, %v", assessment, err)
	}
	assessment, err = store.Acknowledge("repo", assessment.ID, "access", "owner", assessment.Version, Current{CandidateRevision: "commit-1", ControlDigests: map[string]string{"access": "digest-a", "retention": "digest-b"}})
	if err != nil || !assessment.Ready {
		t.Fatalf("acknowledged = %#v, %v", assessment, err)
	}
	current, err := store.Get("repo", assessment.ID, Current{CandidateRevision: "commit-1", ControlDigests: map[string]string{"access": "digest-a", "retention": "changed"}})
	if err != nil {
		t.Fatal(err)
	}
	if current.Ready || !current.Stale || !current.Impacts[0].Current || current.Impacts[1].Current {
		t.Fatalf("affected-only invalidation = %#v", current)
	}
}
