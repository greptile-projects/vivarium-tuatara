package performancegoals

import (
	"errors"
	"testing"
	"time"
)

func number(v float64) *float64 { return &v }
func measured(v float64, env string, at time.Time) Baseline {
	return Baseline{Value: number(v), Environment: env, MeasuredAt: &at, Source: "benchmark-1"}
}
func validRevision(at time.Time) Revision {
	return Revision{Title: "Checkout latency", Summary: "Keep checkout responsive", Subject: Subject{Kind: "user_journey", Name: "Checkout"}, Workloads: []Workload{{Name: "standard cart", Description: "Ten item checkout", Inputs: "fixture-v1", Warmup: 2, Samples: 30}}, Metrics: []Metric{{Name: "p95 latency", Unit: "ms", Direction: "lower", Maximum: number(200), Baseline: measured(260, "linux-x64", at)}}, Constraints: []Constraint{{Name: "correct total", Requirement: "Total remains exact", Verification: "checkout integration test"}}, Environments: []Environment{{Name: "linux-x64", OS: "linux", Architecture: "amd64", Runtime: "go1.25"}}, Owners: []string{"owner"}, Budgets: []Budget{{Kind: "benchmark_time", Limit: 10, Unit: "minutes"}}, Links: []Link{{Kind: "issue", ResourceID: "issue-1", Label: "Slow checkout"}}, BaselineMaxAgeDays: 30, Rationale: "Initial contract"}
}

func TestVersionedGoalDiagnostics(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	first, err := s.Create("repo", "alice", validRevision(now.Add(-time.Hour)))
	if err != nil {
		t.Fatal(err)
	}
	if first.Diagnostics[0].Kind != "target_gap" || first.Revisions[0].Links[0].AddedBy != "alice" {
		t.Fatalf("unexpected projection: %+v", first)
	}
	next := validRevision(now.Add(-60 * 24 * time.Hour))
	next.Metrics[0].Minimum = number(400)
	next.Metrics[0].Maximum = number(500)
	next.Metrics[0].Baseline.Environment = "mac-arm"
	next.Rationale = "New target"
	updated, err := s.Revise(first.ID, 1, "bob", next)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	for _, d := range updated.Diagnostics {
		kinds[d.Kind] = true
		if d.AttributedTo != "bob" {
			t.Fatalf("diagnostic attribution = %q", d.AttributedTo)
		}
	}
	for _, kind := range []string{"incomparable_environment", "stale_baseline", "conflicting_target", "target_gap"} {
		if !kinds[kind] {
			t.Fatalf("missing %s: %+v", kind, updated.Diagnostics)
		}
	}
	if _, err = s.Revise(first.ID, 1, "alice", next); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestMissingMeasurementIsExplicit(t *testing.T) {
	s, _ := New(t.TempDir())
	revision := validRevision(time.Now())
	revision.Metrics[0].Baseline = Baseline{Environment: "linux-x64"}
	goal, err := s.Create("repo", "alice", revision)
	if err != nil {
		t.Fatal(err)
	}
	if len(goal.Diagnostics) != 1 || goal.Diagnostics[0].Kind != "missing_measurement" {
		t.Fatalf("diagnostics = %+v", goal.Diagnostics)
	}
}
