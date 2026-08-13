package productexperiments

import "testing"

func plan(signalStatus string) (Revision, []Signal) {
	return Revision{Hypothesis: "A clearer action increases completed work", Variants: []Variant{{Key: "control", Name: "Current", Description: "existing", Control: true}, {Key: "treatment", Name: "Clearer", Description: "new"}}, Audience: Audience{Description: "active collaborators", Eligibility: []string{"repository_collaborator"}}, Metrics: []Metric{{Name: "completion", Kind: "success", Direction: "increase", Threshold: 5, SignalID: "completed", SignalVersion: 1}, {Name: "errors", Kind: "guardrail", Direction: "below", Threshold: 2, SignalID: "errors", SignalVersion: 1}}, MinimumEvidence: 100, DurationDays: 14, Owners: []string{"owner"}, StopConditions: []string{"errors exceed 2%"}, Assumptions: []string{"traffic is stable"}, Rationale: "initial contract"}, []Signal{{ID: "completed", Name: "Completed", Version: 1, Event: "task.completed", Unit: "percent", Privacy: "aggregate", Status: signalStatus}, {ID: "errors", Name: "Errors", Version: 1, Event: "task.failed", Unit: "percent", Privacy: "aggregate", Status: "available"}}
}
func TestPlanDiagnosticsAndVersionBoundApproval(t *testing.T) {
	s, _ := New(t.TempDir())
	revision, signals := plan("planned")
	v, err := s.Create("repo", "alice", Source{Kind: "proposal", ResourceID: "p1", Label: "Clarify action"}, revision, signals)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Diagnostics) != 1 || v.Diagnostics[0].Kind != "missing_instrumentation" {
		t.Fatalf("diagnostics = %#v", v.Diagnostics)
	}
	v, _ = s.Approve(v.ID, "bob", "approve", "safe", 1)
	revision.Rationale = "audience assumption changed"
	signals[0].Status = "available"
	v, err = s.Revise(v.ID, 1, "alice", revision, signals)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Diagnostics) != 1 || v.Diagnostics[0].Kind != "changed_assumptions" || v.Diagnostics[0].AttributedTo != "bob" {
		t.Fatalf("diagnostics = %#v", v.Diagnostics)
	}
}
func TestOverlapRequiresSharedAudienceAndSignal(t *testing.T) {
	s, _ := New(t.TempDir())
	revision, signals := plan("available")
	a, _ := s.Create("repo", "alice", Source{Kind: "issue", ResourceID: "i1", Label: "one"}, revision, signals)
	b, _ := s.Create("repo", "bob", Source{Kind: "release", ResourceID: "r1", Label: "two"}, revision, signals)
	if !Overlaps(a, b) {
		t.Fatal("expected overlap")
	}
	revision.Audience.Eligibility = []string{"organization_member"}
	c, _ := s.Create("repo", "carol", Source{Kind: "preview", ResourceID: "v1", Label: "three"}, revision, signals)
	if Overlaps(a, c) {
		t.Fatal("unexpected overlap")
	}
}
