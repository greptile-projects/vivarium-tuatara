package capacityobjectives

import (
	"testing"
	"time"
)

func validRevision(now time.Time) Revision {
	return Revision{Title: "Holiday capacity", Summary: "Serve launch demand", Scope: Scope{Kind: "service", Name: "catalog"}, Forecasts: []Forecast{{ID: "forecast-1", Segment: "all users", Start: now, End: now.Add(24 * time.Hour), Value: 1000, Unit: "requests/s", Confidence: "unsupported"}}, TrafficShapes: []TrafficShape{{Name: "launch", Pattern: "step", PeakMultiplier: 2, BurstDuration: "15m"}}, Seasonality: []Seasonality{{Name: "holiday", Window: "December", Multiplier: 1.5, Rationale: "prior launches"}}, ServiceLevels: []ServiceLevel{{Name: "availability", Indicator: "successful requests", Target: 99.9, Unit: "percent", Window: "30d"}}, Thresholds: []Threshold{{Resource: "CPU", Signal: "", Warning: 70, Critical: 90, Unit: "percent"}}, DependencyLimits: []DependencyLimit{{Name: "database", Limit: 5000, Unit: "connections", Signal: "db_connections"}}, Regions: []Region{{Name: "us-east", DemandShare: 1}}, OwnerIDs: []string{"owner"}, Budget: Budget{Amount: 5000, Currency: "USD", Period: "month"}, LeadTime: LeadTime{Duration: "14d", Trigger: "forecast exceeds warning capacity"}, SuccessCriteria: []Criterion{{Name: "serve peak", Condition: "p95 below 200ms", Evidence: "load check"}}, RollbackCriteria: []Criterion{{Name: "cost guard", Condition: "cost exceeds budget", Evidence: "billing signal"}}, Links: []Link{{Kind: "roadmap", ResourceID: "roadmap-1", Label: "holiday launch"}}, Assumptions: []Assumption{{ID: "assumption-1", Statement: "mix remains stable", ExpiresAt: now.Add(10 * 24 * time.Hour)}}, Rationale: "initial agreement"}
}

func TestVersionedDiagnostics(t *testing.T) {
	now := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	s, _ := New(t.TempDir())
	s.now = func() time.Time { return now }
	r := validRevision(now)
	created, err := s.Create("repo", "author", "create-1", r)
	if err != nil {
		t.Fatal(err)
	}
	if created.CurrentVersion != 1 || len(created.Diagnostics) != 4 {
		t.Fatalf("created = %+v", created)
	}
	next := validRevision(now)
	next.Forecasts[0].Evidence = []string{"metric-1"}
	next.Forecasts[0].Confidence = "supported"
	next.Thresholds[0].Signal = "cpu_utilization"
	next.Assumptions[0].Evidence = []string{"analysis-1"}
	next.Assumptions[0].ExpiresAt = now.Add(90 * 24 * time.Hour)
	next.Links[0].ResourceID = "roadmap-2"
	updated, err := s.Revise(created.ID, 1, "owner", "revise-1", next)
	if err != nil {
		t.Fatal(err)
	}
	if updated.CurrentVersion != 2 || len(updated.Revisions) != 2 || len(updated.Diagnostics) != 1 || updated.Diagnostics[0].Kind != "conflicting_commitment" {
		t.Fatalf("updated = %+v", updated)
	}
	reconciled, err := s.Revise(created.ID, 1, "owner", "revise-1", next)
	if err != nil || reconciled.CurrentVersion != 2 {
		t.Fatalf("reconcile = %+v, %v", reconciled, err)
	}
	if _, err = s.Revise(created.ID, 1, "owner", "revise-2", next); err != ErrConflict {
		t.Fatalf("changed retry conflict = %v", err)
	}
}

func TestRejectsInvalidRegionalAllocation(t *testing.T) {
	now := time.Now().UTC()
	s, _ := New(t.TempDir())
	r := validRevision(now)
	r.Regions[0].DemandShare = .8
	if _, err := s.Create("repo", "owner", "invalid-1", r); err != ErrInvalid {
		t.Fatalf("error = %v", err)
	}
}

func TestCreateReconcilesRequestAndRejectsChangedReuse(t *testing.T) {
	now := time.Now().UTC()
	s, _ := New(t.TempDir())
	r := validRevision(now)
	first, err := s.Create("repo", "owner", "create-stable", r)
	if err != nil {
		t.Fatal(err)
	}
	retried, err := s.Create("repo", "owner", "create-stable", r)
	if err != nil || retried.ID != first.ID {
		t.Fatalf("retry = %+v, %v", retried, err)
	}
	r.Title = "changed reuse"
	if _, err = s.Create("repo", "owner", "create-stable", r); err != ErrConflict {
		t.Fatalf("changed reuse = %v", err)
	}
	listed, _ := s.List("repo")
	if len(listed) != 1 {
		t.Fatalf("list = %+v", listed)
	}
}

func TestRepeatedLinkKindsCompareByStableLabelRegardlessOfOrder(t *testing.T) {
	now := time.Now().UTC()
	s, _ := New(t.TempDir())
	r := validRevision(now)
	r.Links = []Link{{Kind: "roadmap", ResourceID: "a", Label: "alpha"}, {Kind: "roadmap", ResourceID: "b", Label: "beta"}, {Kind: "roadmap", ResourceID: "c", Label: "gamma"}}
	created, err := s.Create("repo", "owner", "links-create", r)
	if err != nil {
		t.Fatal(err)
	}
	next := r
	next.Links = []Link{{Kind: "roadmap", ResourceID: "c-v2", Label: "gamma"}, {Kind: "roadmap", ResourceID: "b", Label: "beta"}, {Kind: "roadmap", ResourceID: "a", Label: "alpha"}}
	updated, err := s.Revise(created.ID, 1, "owner", "links-revise", next)
	if err != nil {
		t.Fatal(err)
	}
	conflicts := []Diagnostic{}
	for _, diagnostic := range updated.Diagnostics {
		if diagnostic.Kind == "conflicting_commitment" {
			conflicts = append(conflicts, diagnostic)
		}
	}
	if len(conflicts) != 1 || conflicts[0].ResourceID != "c-v2" {
		t.Fatalf("conflicts = %+v", conflicts)
	}
}
