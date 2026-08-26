package telemetrycontracts

import (
	"errors"
	"testing"
)

func validRevision() Revision {
	return Revision{GapID: "gap", GapVersion: 1, Title: "Checkout evidence", OwnerIDs: []string{"owner"}, ConsumerIDs: []string{"consumer"}, SupportedCollectors: []string{"otel"}, Impact: Impact{Privacy: "pseudonymous", Security: "restricted", Residency: "EU", Performance: "under 1%", Cardinality: 100, StorageBytesPerDay: 1000, MonthlyCostCents: 400, Assumptions: []string{"100 requests/day"}}, Alternatives: []Alternative{{ID: "logs", Name: "structured logs", Tradeoffs: "higher storage", MonthlyCostCents: 600, Privacy: "same fields"}}, Signals: []Signal{{ID: "latency", Name: "checkout.duration", Kind: "metric", Description: "checkout latency", Unit: "ms", Fields: []Field{{Name: "duration", Type: "number", Unit: "ms", Description: "elapsed time"}}, Dimensions: []Dimension{{Name: "region", Bounded: true, MaximumValues: 5, Source: "deployment region"}}, Sampling: "100%", Aggregation: "p50,p95", Correlation: []string{"trace_id"}, RetentionDays: 30, ExpectedEventsPerDay: 100, QualityThresholds: map[string]float64{"completeness": .99}, Collector: "otel", SourceSymbols: []SourceSymbol{{RepositoryID: "repo", Revision: "abc", Path: "checkout.go", Symbol: "Checkout"}}, ServiceBoundaries: []string{"checkout-api"}}}}
}

func TestVersionedContractsAndCitedChallenges(t *testing.T) {
	s, _ := New(t.TempDir())
	r := validRevision()
	v, e := s.Create("repo", "author", "create", r)
	if e != nil || !v.Complete {
		t.Fatalf("create: %+v %v", v, e)
	}
	retry, e := s.Create("repo", "author", "create", r)
	if e != nil || retry.ID != v.ID {
		t.Fatalf("retry: %v", e)
	}
	r.Title = "changed"
	if _, e = s.Create("repo", "author", "create", r); !errors.Is(e, ErrConflict) {
		t.Fatalf("changed retry: %v", e)
	}
	r = validRevision()
	r.Signals[0].Dimensions[0].Bounded = false
	r.Signals[0].Fields[0].Sensitive = true
	v, e = s.Revise("repo", v.ID, 1, "owner", "revise", r)
	if e != nil || v.Complete || len(v.Diagnostics) != 2 {
		t.Fatalf("diagnostics: %+v %v", v, e)
	}
	v, e = s.Challenge("repo", v.ID, "challenge", 2, "agent", "agent-1", "logs", "volume", "measurement contradicts estimate", []Citation{{Kind: "check", ResourceID: "check-1", Revision: "abc", Digest: "sha256:123"}})
	if e != nil || len(v.Challenges) != 1 {
		t.Fatalf("challenge: %+v %v", v, e)
	}
}
