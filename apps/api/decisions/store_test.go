package decisions

import (
	"errors"
	"testing"
	"time"
)

func TestDecisionRetainsVersionedScopeAndDiscussion(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	deadline := now.Add(24 * time.Hour)
	v, err := s.Create("repo", Source{Kind: "proposal", ResourceID: "proposal"}, Scope{Question: "Which queue?", Constraints: []string{"No downtime"}, SuccessMeasures: []string{"p95 improves"}, Deadline: &deadline, OwnerID: "owner", Participants: []Participant{{UserID: "owner"}, {UserID: "peer"}}, AffectedResources: []Resource{{Kind: "service", Label: "API"}}}, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != "pending" || v.Version != 1 || len(v.History) != 1 {
		t.Fatalf("created = %#v", v)
	}
	originalJoined := v.Scope.Participants[1].AddedAt
	now = now.Add(time.Hour)
	scope := v.Scope
	scope.Constraints = []string{"No downtime", "No new broker"}
	scope.Participants = append(scope.Participants, Participant{UserID: "reviewer", AddedBy: "forged", AddedAt: time.Now()})
	v, err = s.Update(v.ID, "peer", 1, scope, "Added the operating constraint")
	if err != nil {
		t.Fatal(err)
	}
	if v.Version != 2 || len(v.History) != 2 || v.Scope.Participants[1].AddedAt != originalJoined || v.Scope.Participants[2].AddedBy != "peer" || !v.Scope.Participants[2].AddedAt.Equal(now) {
		t.Fatalf("updated = %#v", v)
	}
	if _, err = s.Update(v.ID, "peer", 1, scope, "stale"); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale err = %v", err)
	}
	now = now.Add(time.Hour)
	v, err = s.Discuss(v.ID, "reviewer", "We need migration evidence.")
	if err != nil {
		t.Fatal(err)
	}
	if v.Version != 2 || len(v.History) != 3 || v.History[2].Kind != "discussion" || v.History[2].Body == "" {
		t.Fatalf("discussed = %#v", v)
	}
}

func TestDecisionRejectsUnknownParticipantAndInvalidScope(t *testing.T) {
	s, _ := New(t.TempDir())
	deadline := time.Now().Add(time.Hour)
	v, err := s.Create("repo", Source{Kind: "repository"}, Scope{Question: "Question?", Constraints: []string{"constraint"}, SuccessMeasures: []string{"measure"}, AffectedResources: []Resource{{Kind: "repository", Label: "repo"}}, Deadline: &deadline, OwnerID: "owner", Participants: []Participant{{UserID: "owner"}}}, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Discuss(v.ID, "outsider", "hello"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("outsider err = %v", err)
	}
	bad := v.Scope
	bad.OwnerID = "missing"
	if _, err = s.Update(v.ID, "owner", 1, bad, "bad"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("owner err = %v", err)
	}
}

func TestAlternativesCompareCommonCriteriaAndPreserveSupersededDissent(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Date(2026, 8, 10, 19, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	actor := "11111111111111111111111111111111"
	deadline := now.Add(time.Hour)
	v, err := s.Create("22222222222222222222222222222222", Source{Kind: "repository"}, Scope{Question: "Queue model?", Constraints: []string{"No downtime"}, SuccessMeasures: []string{"p95 under 100ms"}, Deadline: &deadline, AffectedResources: []Resource{{Kind: "repository", Label: "API"}}, Participants: []Participant{{UserID: actor}}, OwnerID: actor}, actor)
	if err != nil {
		t.Fatal(err)
	}
	evidence := []Evidence{{Kind: "usage", ResourceID: "latency-dashboard", Revision: "window:2026-08-10T18:00Z/19:00Z", Label: "production p95"}}
	alt := Alternative{Title: "Bounded FIFO", Summary: "Bound the queue.", Assumptions: []string{"Traffic remains bursty"}, Tradeoffs: []string{"Reject overload"}, Risks: []string{"Retry storms"}, CompatibilityImpact: "No wire change", Cost: "Two engineer-days", ExpectedOutcomes: []string{"Stable tail latency"}, Evidence: evidence, Criteria: []CriterionAssessment{{Criterion: "p95 under 100ms", Outcome: "Model predicts 82ms", Evidence: evidence}}, EvidenceStatus: EvidenceStatus{MissingKinds: []string{"fabricated"}, MissingCriteria: []string{"fabricated"}}}
	v, err = s.AddAlternative(v.ID, actor, 1, alt)
	if err != nil || len(v.Alternatives) != 1 || v.Version != 2 {
		t.Fatalf("alternative = %#v, %v", v, err)
	}
	if len(v.Alternatives[0].EvidenceStatus.MissingKinds) != 4 || v.Alternatives[0].EvidenceStatus.MissingKinds[0] != "code" {
		t.Fatalf("creation evidence status = %#v", v.Alternatives[0].EvidenceStatus)
	}
	stored, _ := s.read(v.ID)
	if len(stored.Alternatives[0].EvidenceStatus.MissingKinds) != 0 || len(stored.Alternatives[0].EvidenceStatus.MissingCriteria) != 0 {
		t.Fatalf("persisted client projection = %#v", stored.Alternatives[0].EvidenceStatus)
	}
	projected, _ := s.Get(v.ID)
	if len(projected.Alternatives[0].EvidenceStatus.MissingKinds) != 4 || projected.Alternatives[0].EvidenceStatus.MissingKinds[0] != "code" {
		t.Fatalf("evidence status = %#v", projected.Alternatives[0].EvidenceStatus)
	}
	bad := alt
	bad.Criteria = nil
	if _, err = s.AddAlternative(v.ID, actor, 2, bad); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing common criterion = %v", err)
	}
	v, err = s.AddFinding(v.ID, actor, Finding{AlternativeID: v.Alternatives[0].ID, Body: "The model excludes retry amplification.", Position: "oppose", Uncertainty: "Retry distribution is sampled, not traced.", Citations: evidence})
	if err != nil {
		t.Fatal(err)
	}
	first := v.Findings[0].ID
	v, err = s.AddFinding(v.ID, actor, Finding{AlternativeID: v.Alternatives[0].ID, Body: "A trace confirms retry amplification.", Position: "oppose", Uncertainty: "One region only.", Citations: evidence, SupersedesID: first})
	if err != nil || !v.Findings[0].Superseded || v.Findings[1].SupersedesID != first {
		t.Fatalf("findings = %#v, %v", v.Findings, err)
	}
}
