package recoverycommitments

import (
	"errors"
	"testing"
	"time"
)

func sampleRevision() Revision {
	return Revision{Title: "Checkout continuity", Summary: "Restore checkout after a regional loss.", OwnerIDs: []string{"owner"}, Targets: []Target{
		{ID: "database", Kind: "deployed_service_data", Name: "Orders", Capability: "Read and place orders", OwnerIDs: []string{"owner"}, AcceptableLossMinutes: 5, RestorationTimeMinutes: 30, Retention: "35 daily copies", Jurisdictions: []string{"EU"}, ValidationCriteria: []string{"place a synthetic order"}},
		{ID: "config", Kind: "configuration", Name: "Runtime config", Capability: "Start checkout", AcceptableLossMinutes: 60, RestorationTimeMinutes: 15, Retention: "90 days", Jurisdictions: []string{"EU"}, ValidationCriteria: []string{"boot in an isolated environment"}, Dependencies: []Dependency{{TargetID: "database", Protected: false, RestorationTimeMinutes: 45}}},
	}, Links: []Link{{Kind: "service_objective", ID: "availability", Label: "Checkout availability"}}, Rationale: "Initial agreement"}
}

func TestVersioningAndDiagnostics(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	r := sampleRevision()
	r.Targets[1].OwnerIDs = nil
	r.Exceptions = []Exception{{ID: "temporary", TargetID: "config", Reason: "migration", Mitigation: "manual export", ApprovedBy: "owner", ExpiresAt: now.Add(7 * 24 * time.Hour)}}
	v, err := s.Create("repo", "owner", r)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Diagnostics) != 4 {
		t.Fatalf("diagnostics=%#v", v.Diagnostics)
	}
	if _, err = s.Revise(v.ID, 0, "owner", r); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	r.Rationale = "Confirmed responsibilities"
	r.Targets[1].OwnerIDs = []string{"owner"}
	r.Targets[1].Dependencies[0].Protected = true
	r.Targets[1].RestorationTimeMinutes = 60
	r.Exceptions = nil
	r.Links[0].AddedBy = "spoofed"
	v, err = s.Revise(v.ID, 1, "successor", r)
	if err != nil {
		t.Fatal(err)
	}
	if v.CurrentVersion != 2 || len(v.Revisions) != 2 || len(v.Diagnostics) != 0 {
		t.Fatalf("unexpected successor: %#v", v)
	}
	if v.Revisions[1].Links[0].AddedBy != "owner" {
		t.Fatalf("link attribution changed: %#v", v.Revisions[1].Links[0])
	}
}

func TestRejectsIncompleteContract(t *testing.T) {
	s, _ := New(t.TempDir())
	r := sampleRevision()
	r.Targets[0].ValidationCriteria = nil
	if _, err := s.Create("repo", "owner", r); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected invalid, got %v", err)
	}
}
