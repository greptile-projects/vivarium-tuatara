package capacitymodels

import (
	"testing"
	"time"
)

func validRevision(now time.Time) Revision {
	return Revision{Title: "Checkout forecast", ObjectiveID: "objective-1", ObjectiveVersion: 1, Method: "linear demand fitted to sanitized hourly peaks", Evidence: []Evidence{{ID: "usage-1", Kind: "usage", Label: "hourly peaks", ResourceID: "metric-1", ReleaseID: "release-1", ReleaseRevision: "abc123", Window: Window{Start: now.Add(-24 * time.Hour), End: now}, Sanitization: "aggregated; identifiers removed", InstrumentationVersion: "otel-v2", AudienceIDs: []string{"author"}}}, Assumptions: []Assumption{{ID: "mix", Statement: "request mix is stable", EvidenceIDs: []string{"usage-1"}, Confidence: .7}}, Segments: []Segment{{ID: "buyers", Name: "buyers", DemandUnit: "requests/s", Baseline: 100, GrowthRate: .1, EvidenceIDs: []string{"usage-1"}}}, Saturations: []Saturation{{ID: "db-limit", SegmentID: "buyers", Resource: "database connections", Limit: 500, Unit: "connections", ExpectedAt: now.Add(30 * 24 * time.Hour), LowerAt: now.Add(20 * 24 * time.Hour), UpperAt: now.Add(45 * 24 * time.Hour), EvidenceIDs: []string{"usage-1"}, AssumptionIDs: []string{"mix"}, Explanation: "observed connections per request cross the configured pool"}}, CostCurve: []CostPoint{{Demand: 100, Cost: 10, DemandUnit: "requests/s", Currency: "USD", Period: "hour"}, {Demand: 500, Cost: 35, DemandUnit: "requests/s", Currency: "USD", Period: "hour"}}, Scenarios: []Scenario{{ID: "base", Name: "Base", Description: "roadmap as planned", DemandMultiplier: 1, AssumptionIDs: []string{"mix"}, SaturationIDs: []string{"db-limit"}}}}
}
func TestProjectionRetainsRestrictedGapAndDisagreement(t *testing.T) {
	now := time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC)
	s, _ := New(t.TempDir())
	s.now = func() time.Time { return now }
	created, e := s.Create("repo", "human", "author", "create", validRevision(now))
	if e != nil {
		t.Fatal(e)
	}
	if created.Revisions[0].Evidence[0].ResourceID == "" {
		t.Fatal("author lost permitted evidence")
	}
	event := Event{RequestID: "challenge-1", Kind: "challenge", Statement: "growth changes after launch", EvidenceIDs: []string{"usage-1"}}
	challenged, e := s.AddEvent("repo", created.ID, "agent", "forecast-agent", 1, event)
	if e != nil {
		t.Fatal(e)
	}
	reader, e := s.Get(challenged.ID, "reader")
	if e != nil {
		t.Fatal(e)
	}
	if reader.Revisions[0].Evidence[0].ResourceID != "" || reader.Revisions[0].Evidence[0].Label != "Restricted evidence" {
		t.Fatalf("evidence leaked: %+v", reader.Revisions[0].Evidence[0])
	}
	kinds := map[string]bool{}
	for _, d := range reader.Diagnostics {
		kinds[d.Kind] = true
	}
	if !kinds["inaccessible_evidence"] || !kinds["forecast_disagreement"] {
		t.Fatalf("diagnostics=%+v", reader.Diagnostics)
	}
}
func TestRevisionAndEventRetriesAreStable(t *testing.T) {
	now := time.Now().UTC()
	s, _ := New(t.TempDir())
	created, _ := s.Create("repo", "human", "author", "create", validRevision(now))
	next := validRevision(now)
	next.Title = "Updated"
	updated, e := s.Revise("repo", created.ID, 1, "author", "revise", next)
	if e != nil {
		t.Fatal(e)
	}
	again, e := s.Revise("repo", created.ID, 2, "author", "revise", next)
	if e != nil || len(again.Revisions) != 2 {
		t.Fatalf("retry=%+v %v", again, e)
	}
	event := Event{RequestID: "event", Kind: "supersede", Statement: "version one used pre-launch mix", SupersedesVersion: 1}
	if _, e = s.AddEvent("repo", created.ID, "human", "author", 2, event); e != nil {
		t.Fatal(e)
	}
	if out, e := s.AddEvent("repo", created.ID, "human", "author", 2, event); e != nil || len(out.Events) != 1 {
		t.Fatalf("event retry=%+v %v", out, e)
	}
	_ = updated
}

func TestMutationsAreRepositoryScoped(t *testing.T) {
	s, _ := New(t.TempDir())
	created, _ := s.Create("repo-b", "human", "author", "create", validRevision(time.Now().UTC()))
	if _, err := s.Revise("repo-a", created.ID, 1, "attacker", "revise", validRevision(time.Now().UTC())); err != ErrNotFound {
		t.Fatalf("cross-repository revision = %v", err)
	}
	if _, err := s.AddEvent("repo-a", created.ID, "human", "attacker", 1, Event{RequestID: "event", Kind: "challenge", Statement: "attack"}); err != ErrNotFound {
		t.Fatalf("cross-repository event = %v", err)
	}
}
