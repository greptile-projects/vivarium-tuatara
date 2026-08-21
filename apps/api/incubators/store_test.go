package incubators

import "testing"

func fixture() Incubator {
	return Incubator{Title: "Shared developer onboarding", Audience: "Developers adopting the platform", Problem: "Teams cannot explore a shared project need before choosing a repository", DesiredOutcome: "Collaborators agree on the outcome and authority first", Constraints: []string{"No repository required"}, SuccessMeasures: []string{"Every sponsor consents"}, SponsorIDs: []string{"human-a"}, DecisionRights: []DecisionRight{{Decision: "Change the desired outcome", PrincipalIDs: []string{"human-a"}, Rule: "owner"}}, Visibility: "participants", Source: Source{Kind: "new_idea", Label: "A new project idea", Resolution: "resolved"}}
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
