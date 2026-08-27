package signalevaluations

import (
	"errors"
	"testing"
	"time"
)

func TestFindingsAndRetirementPreserveMeaningAndRequireStopProof(t *testing.T) {
	s, _ := New(t.TempDir())
	e, err := s.Create("repo", "owner", Evaluation{RequestID: "open", GapID: "gap", GapVersion: 2, ContractID: "contract", ContractVersion: 3, RolloutID: "rollout", RolloutVersion: 4, SignalIDs: []string{"observation"}, Question: "Does latency distinguish dependency saturation?", OwnerIDs: []string{"owner"}, Correlations: []Correlation{{Kind: "release", ResourceID: "release", Revision: "abc", Label: "candidate"}, {Kind: "proposal", ResourceID: "repair", Revision: "abc", Label: "repair"}}, Consumers: []Consumer{{Kind: "alert", ResourceID: "alert", Revision: "1", OwnerID: "owner", Impact: "threshold changes"}}})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now().Add(-time.Hour).UTC()
	f := Finding{RequestID: "finding", ActorKind: "agent", ActorID: "agent", Summary: "Saturation predicts latency", Method: "join trace dependency span with release window", Reproduction: "query digest q1 with frozen window", Uncertainty: "sample excludes one region", Citations: []Citation{{ID: "citation-1", Kind: "signal", ResourceID: "observation", Revision: "4", Digest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Query: "q1", WindowStart: start, WindowEnd: start.Add(time.Minute)}}, Criteria: []Criterion{{ID: "distinguish", Result: "met", Rationale: "dependency spans separate the cases", CitationIDs: []string{"citation-1"}}}}
	e, err = s.AddFinding("repo", e.ID, f)
	if err != nil {
		t.Fatal(err)
	}
	bad := Decision{RequestID: "remove", ExpectedVersion: e.Version, Action: "remove", Rationale: "question answered", FindingIDs: []string{e.Findings[0].ID}, PolicyApproval: "privacy-policy-v2"}
	if _, err = s.Decide("repo", e.ID, bad); !errors.Is(err, ErrInvalid) {
		t.Fatalf("removal without impact and stop proof should fail: %v", err)
	}
	bad.Consumers = e.Consumers
	bad.Repair = &Repair{Kind: "proposal", ResourceID: "repair", Summary: "reviewed repair"}
	bad.StopVerification = &Citation{ID: "stop", Kind: "collector_stop", ResourceID: "collector", Revision: "after-removal", Digest: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Query: "no accepted payloads for 24h", WindowStart: start, WindowEnd: start.Add(24 * time.Hour)}
	if _, err = s.Decide("repo", e.ID, bad); !errors.Is(err, ErrInvalid) {
		t.Fatalf("removal without structured repair outcome should fail: %v", err)
	}
	bad.RepairOutcome = &RepairOutcome{PullRequestID: "pull", MergedCommit: "cccccccccccccccccccccccccccccccccccccccc", ReleaseID: "release", DeploymentID: "deployment", ObservationID: "healthy", Digest: "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"}
	e, err = s.Decide("repo", e.ID, bad)
	if err != nil {
		t.Fatal(err)
	}
	if e.Status != "removed" || len(e.Findings) != 1 || len(e.Decisions) != 1 || len(e.Decisions[0].Consumers) != 1 {
		t.Fatalf("history or impacts lost: %+v", e)
	}
}

func TestChangedFindingRetryConflicts(t *testing.T) {
	s, _ := New(t.TempDir())
	e, _ := s.Create("repo", "owner", Evaluation{RequestID: "open", GapID: "gap", GapVersion: 1, ContractID: "contract", ContractVersion: 1, RolloutID: "rollout", RolloutVersion: 1, SignalIDs: []string{"signal"}, Question: "useful?", OwnerIDs: []string{"owner"}})
	n := time.Now().UTC()
	f := Finding{RequestID: "same", ActorKind: "human", ActorID: "owner", Summary: "one", Method: "query", Reproduction: "repeat", Uncertainty: "known", Citations: []Citation{{ID: "citation", Kind: "signal", ResourceID: "signal", Revision: "1", Digest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Query: "q", WindowStart: n, WindowEnd: n.Add(time.Minute)}}, Criteria: []Criterion{{ID: "c", Result: "met", Rationale: "r", CitationIDs: []string{"citation"}}}}
	if _, err := s.AddFinding("repo", e.ID, f); err != nil {
		t.Fatal(err)
	}
	f.Summary = "changed"
	if _, err := s.AddFinding("repo", e.ID, f); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed retry = %v", err)
	}
}
