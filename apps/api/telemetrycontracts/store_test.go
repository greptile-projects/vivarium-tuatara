package telemetrycontracts

import (
	"errors"
	"strings"
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
	v, e = s.Challenge("repo", v.ID, "challenge", 2, "agent", "agent-1", "logs", "volume", "measurement contradicts estimate", []Citation{{Kind: "git_blob", ResourceID: "evidence.txt", Revision: "abc", Digest: "123", Verified: true}})
	if e != nil || len(v.Challenges) != 1 {
		t.Fatalf("challenge: %+v %v", v, e)
	}
}

func TestAcceptedDeliveryAndSanitizedExactCandidateVerification(t *testing.T) {
	s, _ := New(t.TempDir())
	c, err := s.Create("contract-repo", "author", "create", validRevision())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.RecordDelivery("contract-repo", c.ID, Delivery{RequestID: "early", ContractVersion: 1, RepositoryID: "app", ProposalID: "proposal", TaskIDs: []string{"task"}, BaseRevision: strings.Repeat("a", 40)}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unaccepted delivery: %v", err)
	}
	c, err = s.Accept("contract-repo", c.ID, "owner", "accept", "reviewed privacy and cost boundaries", 1)
	if err != nil || c.Acceptance == nil {
		t.Fatalf("accept: %+v %v", c, err)
	}
	d := Delivery{RequestID: "deliver", ContractVersion: 1, RepositoryID: "app", ProposalID: "proposal", TaskIDs: []string{"task"}, BaseRevision: strings.Repeat("a", 40), CreatedBy: "owner"}
	if _, err = s.RecordDelivery("contract-repo", c.ID, d); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RecordDelivery("contract-repo", c.ID, d); err != nil {
		t.Fatalf("retry: %v", err)
	}
	results := []VerificationResult{}
	for _, requirement := range []string{"emission", "schema", "units", "correlation", "sampling", "redaction", "access", "performance", "failure_behavior"} {
		results = append(results, VerificationResult{Requirement: requirement, Outcome: "passed"})
	}
	v := Verification{RequestID: "verify", ContractRepositoryID: "contract-repo", ContractID: c.ID, ContractVersion: 1, PullRequestID: "pull", Revision: strings.Repeat("b", 40), PreviewID: "preview", CheckRunIDs: []string{"telemetry-check"}, Journey: "checkout", FailureScenario: "collector unavailable", Isolation: "ephemeral_network_none", Results: results, Artifacts: []Artifact{{Kind: "trace", Digest: strings.Repeat("c", 64), SizeBytes: 12, Summary: "caller secret"}}, CostCents: 3, OverheadPercent: .4}
	out, err := s.AddVerification("app", "agent", "agent-1", v)
	if err != nil {
		t.Fatal(err)
	}
	if out.Artifacts[0].Summary == "caller secret" || out.AuthorID != "agent-1" || len(out.Coverage) != 9 {
		t.Fatalf("unsafe projection: %+v", out)
	}
	if _, err = s.AddVerification("app", "agent", "agent-1", v); err != nil {
		t.Fatalf("retry: %v", err)
	}
}
