package deliveryteams

import (
	"errors"
	"testing"
)

func charter(person, role string) Charter {
	return Charter{Name: "Ship the migration", Purpose: "Deliver one governed result", EscalationPath: "Escalate scope to the accountable owner", Participants: []Participant{{ID: person + "-slot", PrincipalType: "human", PrincipalID: person, Role: role, Responsibility: "Own compatibility verification", Why: "Maintains the affected surface", Escalation: "Raise unresolved risk to the organizer", RequiredAccess: []AccessRequirement{{RepositoryID: "repo", Level: "read"}}}}}
}

func TestCharterAcceptanceAndReplacementRemainAttributable(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.Create("repo", Outcome{Kind: "proposal", ResourceID: "proposal", Title: "Migration"}, charter("alice", "compatibility lead"), "organizer")
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Respond(v.ID, "alice-slot", "alice", "accepted", v.Version)
	if err != nil || v.Participants[0].Status != "accepted" {
		t.Fatalf("accept: %#v %v", v.Participants, err)
	}
	next := charter("bob", "replacement lead")
	v, err = s.Update(v.ID, "organizer", v.Version, next)
	if err != nil {
		t.Fatal(err)
	}
	if v.Participants[0].Status != "pending" || len(v.Events) != 3 || v.Events[2].ActorID != "organizer" {
		t.Fatalf("replacement history: %#v", v)
	}
	if _, err = s.Update(v.ID, "alice", v.Version, next); !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-organizer update = %v", err)
	}
}

func TestCharterRejectsDuplicatePrincipalAndStaleResponse(t *testing.T) {
	s, _ := New(t.TempDir())
	c := charter("alice", "lead")
	c.Participants = append(c.Participants, c.Participants[0])
	if _, err := s.Create("repo", Outcome{Kind: "planned_outcome", ResourceID: "outcome", Title: "Outcome"}, c, "organizer"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("duplicate = %v", err)
	}
	v, err := s.Create("repo", Outcome{Kind: "planned_outcome", ResourceID: "outcome", Title: "Outcome"}, charter("alice", "lead"), "organizer")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Respond(v.ID, "alice-slot", "alice", "accepted", v.Version+1); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale response = %v", err)
	}
}
