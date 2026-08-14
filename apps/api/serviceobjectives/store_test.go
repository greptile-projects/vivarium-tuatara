package serviceobjectives

import (
	"errors"
	"testing"
	"time"
)

func completeRevision() Revision {
	return Revision{Title: "Checkout reliability", Summary: "People can complete checkout reliably.", Scopes: []Scope{{Kind: "environment", ResourceID: "production", Name: "Production checkout"}}, Indicators: []Indicator{{ID: "success", Name: "Successful checkout", Description: "Eligible checkouts that complete", Signal: "checkout.completed", Calculation: "ratio", Unit: "percent", GoodEvent: "completed", TotalEvent: "started"}}, Windows: []Window{{ID: "month", Name: "Rolling month", Duration: "720h", Rolling: true}}, Journeys: []Journey{{ID: "buy", Name: "Buy", Description: "Complete payment", OwnerIDs: []string{"owner"}}}, Objectives: []Objective{{ID: "availability", Name: "Checkout availability", IndicatorID: "success", WindowID: "month", Target: 99.9, Comparator: "at_least", JourneyIDs: []string{"buy"}, OwnerIDs: []string{"owner"}}}, Dependencies: []Dependency{{ID: "payments", Name: "Payments", Kind: "service", OwnerIDs: []string{"owner"}, ObjectiveIDs: []string{"availability"}}}, ErrorBudgets: []ErrorBudget{{ObjectiveID: "availability", AllowedFailure: .1, Unit: "percent", BurnPolicy: "Pause rollout"}}, Severities: []Severity{{Level: "warning", BudgetConsumedPercent: 50, Response: "Investigate", OwnerIDs: []string{"owner"}}, {Level: "critical", BudgetConsumedPercent: 100, Response: "Contain", OwnerIDs: []string{"owner"}}}, OwnerIDs: []string{"owner"}, CommitmentLinks: []CommitmentLink{{Kind: "performance", ID: "perf", Version: 2}, {Kind: "privacy", ID: "data", Version: 1}}, ExceptionPolicy: ExceptionPolicy{MaximumDuration: "168h", ApprovalOwnerIDs: []string{"owner"}, FollowUpRequired: true}, Rationale: "Initial shared contract"}
}

func TestVersioningAndExplicitDiagnostics(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	created, err := s.Create("repo", "owner", completeRevision())
	if err != nil || created.CurrentVersion != 1 || len(created.Diagnostics) != 0 {
		t.Fatalf("create = %#v, %v", created, err)
	}
	r := completeRevision()
	r.Indicators[0].Signal = ""
	r.Indicators[0].Calculation = "histogram_magic"
	r.Objectives[0].OwnerIDs = nil
	r.Objectives = append(r.Objectives, Objective{ID: "conflict", Name: "Conflicting target", IndicatorID: "success", WindowID: "month", Target: 99, Comparator: "at_least", JourneyIDs: []string{"buy"}, OwnerIDs: []string{"owner"}})
	r.ErrorBudgets = append(r.ErrorBudgets, ErrorBudget{ObjectiveID: "conflict", AllowedFailure: 1, Unit: "percent", BurnPolicy: "Investigate"})
	r.Exceptions = []Exception{{ID: "temporary", ObjectiveIDs: []string{"availability"}, Reason: "Migration", ApprovedBy: "owner", ExpiresAt: now.Add(48 * time.Hour), FollowUp: "proposal/1"}}
	updated, err := s.Revise(created.ID, 1, "owner", r)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"missing_signal": false, "unsupported_calculation": false, "missing_ownership": false, "conflicting_target": false, "expiring_exception": false}
	for _, d := range updated.Diagnostics {
		want[d.Kind] = true
	}
	for kind, found := range want {
		if !found {
			t.Errorf("missing %s: %#v", kind, updated.Diagnostics)
		}
	}
	if _, err = s.Revise(created.ID, 1, "owner", completeRevision()); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale error = %v", err)
	}
	got, _ := s.Get(created.ID)
	if len(got.Revisions) != 2 {
		t.Fatalf("history = %#v", got.Revisions)
	}
}

func TestRejectsBrokenReferences(t *testing.T) {
	s, _ := New(t.TempDir())
	cases := []func(*Revision){func(r *Revision) { r.Objectives[0].IndicatorID = "missing" }, func(r *Revision) { r.ErrorBudgets = nil }, func(r *Revision) { r.Severities[1].BudgetConsumedPercent = 25 }, func(r *Revision) { r.CommitmentLinks[0].Kind = "operations" }, func(r *Revision) { r.Scopes[0].ResourceID = "" }, func(r *Revision) { r.Windows[0].Duration = "0s" }, func(r *Revision) { r.Windows[0].Duration = "-1s" }, func(r *Revision) { r.ExceptionPolicy.MaximumDuration = "0s" }, func(r *Revision) { r.ExceptionPolicy.MaximumDuration = "-1s" }}
	for i, mutate := range cases {
		r := completeRevision()
		mutate(&r)
		if _, err := s.Create("repo", "owner", r); !errors.Is(err, ErrInvalid) {
			t.Errorf("case %d error = %v", i, err)
		}
	}
}
