package exploratorysessions

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func example(now time.Time) Session {
	return Session{Title: "Checkout edge exploration", Source: Source{Kind: "pull_preview", ResourceID: "pull-1", Revision: strings.Repeat("a", 40), Label: "Checkout preview"}, Access: []string{"owner", "tester"}, Limits: Limits{ExpiresAt: now.Add(time.Hour), MaxCostCents: 500, MaxAgentActions: 2, AllowedActions: []string{"navigate", "input", "screenshot", "trace", "command", "observe", "guide", "pause", "resume", "reproduce", "classify", "discard", "close"}, TestData: []string{"synthetic"}}, Charters: []Charter{{ID: "payment", Title: "Payment interruption", Risk: "high", Mission: "Interrupt checkout at every boundary", AssigneeType: "agent", AssigneeID: "agent-1", AllowedActions: []string{"navigate", "input", "screenshot", "observe"}, Coverage: []string{"checkout", "recovery"}, Uncertainty: "Gateway timing remains unknown"}}}
}

func TestTimelineIsBoundedCASAndAttributable(t *testing.T) {
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return now }
	v, err := s.Create("repo", "owner", example(now))
	if err != nil {
		t.Fatal(err)
	}
	digest := strings.Repeat("b", 64)
	v, err = s.Append(v.ID, "agent-1", EventInput{ExpectedVersion: 1, Kind: "observation", CharterID: "payment", FindingID: "duplicate-submit", Summary: "Retry duplicates the submission", Route: "/checkout", Inputs: []string{"synthetic decline"}, Coverage: []string{"retry"}, Uncertainty: "Observed once", Artifacts: []Artifact{{Kind: "screenshot", SHA256: digest, MediaType: "image/png", Description: "Duplicate submit state"}}, ActorType: "agent", ActorID: "agent-1"})
	if err != nil {
		t.Fatal(err)
	}
	if v.Version != 2 || v.Events[0].ActorID != "agent-1" || v.Events[0].Route != "/checkout" {
		t.Fatalf("unexpected event: %+v", v)
	}
	if _, err = s.Append(v.ID, "owner", EventInput{ExpectedVersion: 1, Kind: "guide", Summary: "Try keyboard retry"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	if _, err = s.Append(v.ID, "other-agent", EventInput{ExpectedVersion: 2, Kind: "observation", CharterID: "payment", Summary: "Unapproved action", ActorType: "agent", ActorID: "other-agent"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected scoped agent rejection, got %v", err)
	}
	if _, err = s.Append(v.ID, "agent-1", EventInput{ExpectedVersion: 2, Kind: "observation", CharterID: "payment", Summary: "Attempt undelegated command", Command: "curl /admin", ActorType: "agent", ActorID: "agent-1"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected action-scope rejection, got %v", err)
	}
	if _, err = s.Append(v.ID, "agent-1", EventInput{ExpectedVersion: 2, Kind: "pause", CharterID: "payment", Summary: "Attempt undelegated control", ActorType: "agent", ActorID: "agent-1"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected lifecycle-scope rejection, got %v", err)
	}
	if _, err = s.Append(v.ID, "agent-1", EventInput{ExpectedVersion: 2, Kind: "observation", CharterID: "payment", FindingID: "smuggled-decision", Summary: "Attempt classification through observation", Classification: "bug", ActorType: "agent", ActorID: "agent-1"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected observation classification rejection, got %v", err)
	}
	v, err = s.Append(v.ID, "tester", EventInput{ExpectedVersion: 2, Kind: "reproduce", CharterID: "payment", FindingID: "duplicate-submit", Summary: "Reproduced with keyboard", ReproducesEventID: v.Events[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Append(v.ID, "owner", EventInput{ExpectedVersion: 3, Kind: "classify", FindingID: "duplicate-submit", Summary: "Confirmed candidate defect", Classification: "bug"})
	if err != nil {
		t.Fatal(err)
	}
	if v.Events[2].Classification != "bug" || v.Events[2].ActorType != "human" {
		t.Fatalf("classification not retained: %+v", v.Events[2])
	}
}

func TestTimelineReferencesMustResolveWithinSession(t *testing.T) {
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return now }
	first, err := s.Create("repo", "owner", example(now))
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Create("repo", "owner", example(now))
	if err != nil {
		t.Fatal(err)
	}
	first, err = s.Append(first.ID, "tester", EventInput{ExpectedVersion: 1, Kind: "observation", CharterID: "payment", FindingID: "finding-1", Summary: "Observed failure"})
	if err != nil {
		t.Fatal(err)
	}
	second, err = s.Append(second.ID, "tester", EventInput{ExpectedVersion: 1, Kind: "observation", CharterID: "payment", FindingID: "finding-2", Summary: "Other failure"})
	if err != nil {
		t.Fatal(err)
	}
	invalid := []EventInput{
		{ExpectedVersion: 2, Kind: "classify", FindingID: "missing", Summary: "Classify missing", Classification: "bug"},
		{ExpectedVersion: 2, Kind: "discard", FindingID: "missing", Summary: "Discard missing", Classification: "discarded"},
		{ExpectedVersion: 2, Kind: "reproduce", FindingID: "finding-1", ReproducesEventID: "missing", Summary: "Reproduce missing"},
		{ExpectedVersion: 2, Kind: "reproduce", FindingID: "finding-2", ReproducesEventID: second.Events[0].ID, Summary: "Cross-session reference"},
	}
	for _, in := range invalid {
		if _, err = s.Append(first.ID, "tester", in); !errors.Is(err, ErrInvalid) {
			t.Fatalf("expected invalid reference rejection for %+v, got %v", in, err)
		}
	}
}

func TestSessionRejectsUnsafeDataAndUnboundedAuthority(t *testing.T) {
	now := time.Now().UTC()
	v := example(now)
	v.Limits.TestData = []string{"production_personal_data"}
	if ValidSession(v, now) {
		t.Fatal("production data must not be admitted")
	}
	v = example(now)
	v.Limits.ExpiresAt = now.Add(25 * time.Hour)
	if ValidSession(v, now) {
		t.Fatal("session over 24 hours must not be admitted")
	}
	v = example(now)
	v.Charters[0].AllowedActions = []string{"deploy"}
	if ValidSession(v, now) {
		t.Fatal("charter must not broaden session authority")
	}
}
