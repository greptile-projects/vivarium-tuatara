package observabilitygaps

import (
	"errors"
	"testing"
	"time"
)

func validRevision(now time.Time) Revision {
	return Revision{Title: "Checkout latency blind spot", Question: "Why do checkouts stall?", Behavior: "Checkout should finish within two seconds.", AudienceIDs: []string{"audience"}, Decision: "Choose whether to roll back.", Services: []Service{{ID: "checkout", Name: "Checkout", Environment: "production"}}, Journeys: []Journey{{ID: "buy", Name: "Buy", Behavior: "Submit order"}}, RequiredTimeliness: "Within five minutes", Source: Source{Kind: "incident", ResourceID: "inc-1", Revision: "v2", Question: "Why are requests slow?", Status: "current"}, Evidence: []Evidence{{ID: "latency", Kind: "metric", Label: "Latency", ReleaseID: "rel-1", ReleaseRevision: "abc", Environment: "production", Status: "ambiguous", ObservedAt: now.Add(-31 * 24 * time.Hour)}}, OwnerIDs: []string{"owner"}, SuccessCriteria: []Criterion{{ID: "cause", Statement: "Distinguish dependency from application latency", RequiredEvidence: "Correlated spans"}}}
}

func TestGapRetainsVersionsAndProjectsAttributableCoverage(t *testing.T) {
	s, e := New(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	r := validRevision(now)
	created, e := s.Create("repo", "author", "request-1", r)
	if e != nil {
		t.Fatal(e)
	}
	if created.CurrentVersion != 1 || len(created.Diagnostics) != 7 {
		t.Fatalf("unexpected projection: %+v", created)
	}
	kinds := map[string]bool{}
	for _, d := range created.Diagnostics {
		kinds[d.Kind] = true
		if d.AttributedTo != "author" {
			t.Fatalf("missing attribution: %+v", d)
		}
	}
	for _, kind := range []string{"ambiguous_instrumentation", "ambiguous_semantics", "stale_instrumentation", "absent_coverage"} {
		if !kinds[kind] {
			t.Fatalf("missing %s: %+v", kind, created.Diagnostics)
		}
	}
	retry, e := s.Create("repo", "author", "request-1", r)
	if e != nil || retry.ID != created.ID {
		t.Fatalf("retry did not reconcile: %v", e)
	}
	r.Decision = "Investigate dependency"
	if _, e = s.Create("repo", "author", "request-1", r); !errors.Is(e, ErrConflict) {
		t.Fatalf("changed retry = %v", e)
	}
	revised, e := s.Revise("repo", created.ID, 1, "owner", "request-2", r)
	if e != nil || revised.CurrentVersion != 2 || len(revised.Revisions) != 2 {
		t.Fatalf("revision failed: %+v %v", revised, e)
	}
}

func TestGapRequiresExactSourceAndEvidenceMeaning(t *testing.T) {
	s, _ := New(t.TempDir())
	r := validRevision(time.Now())
	r.Source.Revision = ""
	if _, e := s.Create("repo", "actor", "bad", r); !errors.Is(e, ErrInvalid) {
		t.Fatalf("missing source revision accepted: %v", e)
	}
	r = validRevision(time.Now())
	r.Evidence[0].ReleaseRevision = ""
	if _, e := s.Create("repo", "actor", "bad-evidence", r); !errors.Is(e, ErrInvalid) {
		t.Fatalf("missing release revision accepted: %v", e)
	}
}
