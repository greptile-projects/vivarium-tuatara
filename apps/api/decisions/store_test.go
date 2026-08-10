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
