package capabilities

import (
	"testing"
	"time"
)

func retirementFixture(now time.Time) Revision {
	return Revision{Name: "legacy", Summary: "legacy endpoint", CommitID: string(make([]byte, 40)), ReleaseID: "release", OwnerIDs: []string{"provider"}, Items: []Item{{Kind: "interface", Name: "v1", Path: "api.go", Revision: string(make([]byte, 40))}}, Consumers: []Consumer{{Name: "mobile", OwnerIDs: []string{"mobile-owner"}, Environment: "production", Discovery: "declared", EvidenceState: "unknown", CompatibilityPromise: "supported through 2027"}}}
}
func planFixture(now time.Time) RetirementPlan {
	return RetirementPlan{Rationale: "replace an unsafe protocol", Replacements: []Replacement{{Name: "v2", Reference: "capability:v2", MigrationGuide: "docs/migrate.md", Supported: true}}, Audiences: []Audience{{Name: "mobile", OwnerIDs: []string{"mobile-owner"}, Impact: "v1 requests stop working", Commitment: "supported through 2027", EmbargoedDependency: true}}, Stages: []CompatibilityStage{{Name: "warn", StartsAt: now.Add(time.Hour), Behavior: "serve with warning", ExitCriteria: []string{"owners notified"}}, {Name: "disable", StartsAt: now.Add(48 * time.Hour), Behavior: "reject v1", ExitCriteria: []string{"traffic zero"}}}, Deadline: now.Add(72 * time.Hour), ApprovalDueAt: now.Add(24 * time.Hour), SuccessCriteria: []string{"v2 success rate is stable"}, RollbackCriteria: []string{"consumer errors exceed one percent"}, Communication: CommunicationPolicy{Channels: []string{"owner inbox"}, NoticeDays: 30, Updates: "weekly", Escalation: "repository owner"}, RequiredOwnerIDs: []string{"mobile-owner"}}
}

func TestRetirementPlanRetainsBlockersAndAcknowledgements(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	v, err := s.Create("repo", "provider", retirementFixture(now))
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.OpenRetirement("repo", v.ID, "provider", planFixture(now))
	if err != nil {
		t.Fatal(err)
	}
	p := v.RetirementPlans[0]
	kinds := map[string]bool{}
	for _, b := range p.Blockers {
		kinds[b.Kind] = true
	}
	for _, k := range []string{"inventory_unknown_evidence", "owner_approval_required", "conflicting_commitment", "embargoed_dependency"} {
		if !kinds[k] {
			t.Fatalf("missing %s in %#v", k, p.Blockers)
		}
	}
	v, err = s.AppendRetirementEvent("repo", v.ID, p.ID, "agent-1", "read_only_agent", 0, RetirementEvent{Type: "challenge", Summary: "traffic remains", Evidence: []string{"usage:sha256:abc"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AppendRetirementEvent("repo", v.ID, p.ID, "agent-1", "read_only_agent", 1, RetirementEvent{Type: "approval", Summary: "approve", OwnerID: "agent-1", Decision: "approved"}); err != ErrInvalid {
		t.Fatalf("agent approved: %v", err)
	}
	v, err = s.AppendRetirementEvent("repo", v.ID, p.ID, "mobile-owner", "human", 1, RetirementEvent{Type: "approval", Summary: "migration understood", OwnerID: "mobile-owner", Decision: "approved"})
	if err != nil {
		t.Fatal(err)
	}
	if len(v.RetirementPlans[0].Events) != 2 {
		t.Fatalf("events %#v", v.RetirementPlans[0].Events)
	}
}

func TestRetirementChangedUsageAndBoundedDecisions(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	r := retirementFixture(now)
	v, _ := s.Create("repo", "provider", r)
	v, _ = s.OpenRetirement("repo", v.ID, "provider", planFixture(now))
	p := v.RetirementPlans[0]
	far := now.Add(31 * 24 * time.Hour)
	if _, err := s.AppendRetirementEvent("repo", v.ID, p.ID, "provider", "human", 0, RetirementEvent{Type: "policy_decision", Summary: "wait", Decision: "defer", ExpiresAt: &far, FollowUp: "issue:1"}); err != ErrInvalid {
		t.Fatalf("unbounded decision: %v", err)
	}
	r.Summary = "new usage"
	v, err := s.Revise("repo", v.ID, 1, "provider", r)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, b := range v.RetirementPlans[0].Blockers {
		found = found || b.Kind == "changed_usage"
	}
	if !found {
		t.Fatal("changed usage did not block")
	}
}
