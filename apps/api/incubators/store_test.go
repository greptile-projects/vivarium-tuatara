package incubators

import (
	"errors"
	"testing"
)

func fixture() Incubator {
	return Incubator{Title: "Shared developer onboarding", Audience: "Developers adopting the platform", Problem: "Teams cannot explore a shared project need before choosing a repository", DesiredOutcome: "Collaborators agree on the outcome and authority first", Constraints: []string{"No repository required"}, SuccessMeasures: []string{"Every sponsor consents"}, SponsorIDs: []string{"human-a"}, DecisionRights: []DecisionRight{{Kind: "scope_change", Decision: "Change the desired outcome", PrincipalIDs: []string{"human-a"}, Rule: "owner"}}, Visibility: "participants", Source: Source{Kind: "new_idea", Label: "A new project idea", Resolution: "resolved"}}
}

func TestConsentAttributionVisibilityAndCAS(t *testing.T) {
	s, e := New(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	x, e := s.Create(fixture(), "human-a", []Invitation{{PrincipalType: "human", PrincipalID: "human-b", Role: "co-designer"}})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Get(x.ID, "stranger"); e != ErrNotFound {
		t.Fatalf("private read = %v", e)
	}
	x, e = s.Consent(x.ID, x.Invitations[0].ID, "human-b", "accepted", 1)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.AddEvent(x.ID, "human", "human-b", 2, Event{Kind: "scope_change", Body: "Replace the audience", Visibility: "participants"}); e != ErrInvalid {
		t.Fatalf("undeclared decision right = %v", e)
	}
	x, e = s.AddEvent(x.ID, "human", "human-b", 2, Event{Kind: "assumption", Body: "The first audience is internal platform teams", Visibility: "participants"})
	if e != nil {
		t.Fatal(e)
	}
	if x.Events[len(x.Events)-1].ActorID != "human-b" {
		t.Fatal("event attribution lost")
	}
	if _, e = s.AddEvent(x.ID, "human", "human-b", 2, Event{Kind: "discussion", Body: "stale", Visibility: "participants"}); e != ErrConflict {
		t.Fatalf("stale write = %v", e)
	}
}

func TestPotentialDuplicatesAreReportedNotCollapsed(t *testing.T) {
	s, _ := New(t.TempDir())
	a := fixture()
	first, e := s.Create(a, "human-a", nil)
	if e != nil {
		t.Fatal(e)
	}
	second, e := s.Create(a, "human-a", nil)
	if e != nil {
		t.Fatal(e)
	}
	if first.ID == second.ID || len(second.PotentialDuplicates) != 1 || second.PotentialDuplicates[0].IncubatorID != first.ID {
		t.Fatalf("duplicates not explicit: %#v", second.PotentialDuplicates)
	}
}

func TestScopeChangeUsesOnlyTypedRightAndEvaluatesMajority(t *testing.T) {
	s, _ := New(t.TempDir())
	x := fixture()
	x.DecisionRights = []DecisionRight{{Kind: "project_update", Decision: "Publish updates", PrincipalIDs: []string{"human-b"}, Rule: "majority"}}
	x, e := s.Create(x, "human-a", []Invitation{{PrincipalType: "human", PrincipalID: "human-b", Role: "publisher"}})
	if e != nil {
		t.Fatal(e)
	}
	x, _ = s.Consent(x.ID, x.Invitations[0].ID, "human-b", "accepted", 1)
	if _, e = s.AddEvent(x.ID, "human", "human-b", 2, Event{Kind: "scope_change", Body: "Serve external teams", Visibility: "participants"}); e != ErrInvalid {
		t.Fatalf("unrelated right authorized scope: %v", e)
	}

	y := fixture()
	y.DecisionRights = []DecisionRight{{Kind: "scope_change", Decision: "Change scope", PrincipalIDs: []string{"human-a", "human-b", "human-c"}, Rule: "majority"}}
	y, e = s.Create(y, "human-a", []Invitation{{PrincipalType: "human", PrincipalID: "human-b", Role: "designer"}, {PrincipalType: "human", PrincipalID: "human-c", Role: "sponsor"}})
	if e != nil {
		t.Fatal(e)
	}
	y, _ = s.Consent(y.ID, y.Invitations[0].ID, "human-b", "accepted", 1)
	y, _ = s.Consent(y.ID, y.Invitations[1].ID, "human-c", "accepted", 2)
	y, e = s.AddEvent(y.ID, "human", "human-b", 3, Event{Kind: "decision_support", Body: "Serve external teams", Visibility: "participants"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.AddEvent(y.ID, "human", "human-c", 4, Event{Kind: "scope_change", Body: "Serve external teams", Visibility: "participants"}); e != nil {
		t.Fatalf("majority scope change rejected: %v", e)
	}
}

func TestPostRenameSyncFailureReturnsCommittedUncertainState(t *testing.T) {
	s, _ := New(t.TempDir())
	calls := 0
	s.syncDir = func() error {
		calls++
		if calls == 1 {
			return errors.New("injected directory sync failure")
		}
		return nil
	}
	x, e := s.Create(fixture(), "human-a", nil)
	if e != nil || !x.DurabilityUncertain {
		t.Fatalf("create = %#v, %v", x, e)
	}
	stored, e := s.Get(x.ID, "human-a")
	if e != nil || !stored.DurabilityUncertain {
		t.Fatalf("stored marker = %#v, %v", stored, e)
	}
	x, e = s.AddEvent(x.ID, "human", "human-a", 1, Event{Kind: "discussion", Body: "Retry-safe follow-up", Visibility: "participants"})
	if e != nil || x.DurabilityUncertain {
		t.Fatalf("marker did not clear after synced mutation: %#v, %v", x, e)
	}
}
