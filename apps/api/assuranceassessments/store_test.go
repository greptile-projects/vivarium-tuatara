package assuranceassessments

import (
	"errors"
	"testing"
	"time"
)

func TestBoundedAssessmentRolesConflictAndExpiry(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	a, err := s.Create(Assessment{RepositoryID: "repo", ProgramID: "program", ProgramVersion: 2, Title: "Independent review", OwnerID: "owner", Assessor: Assessor{UserID: "outside", Kind: "external", ConflictDisclosure: "prior advisory work"}, Scope: Scope{ControlIDs: []string{"access"}, SystemIDs: []string{"production"}, PeriodStartsAt: now.Add(-30 * 24 * time.Hour), PeriodEndsAt: now}, EvidencePackageIDs: []string{"package"}, StartsAt: now, ExpiresAt: now.Add(24 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if a.Status != "conflict_review" {
		t.Fatalf("status=%s", a.Status)
	}
	if _, err = s.Append(a.ID, a.Version, "outside", "assessor", Event{Kind: "finding", Body: "claim is unsupported", ControlID: "access"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("expected pending conflict to block assessment work, got %v", err)
	}
	// Only the owner resolves a disclosed conflict; afterward the assessor can request selected evidence.
	a, err = s.Append(a.ID, a.Version, "owner", "owner", Event{Kind: "conflict_resolution", Body: "separate engagement creates no self-review", Status: "cleared"})
	if err != nil {
		t.Fatal(err)
	}
	a, err = s.Append(a.ID, a.Version, "outside", "assessor", Event{Kind: "sample_request", Body: "select three access reviews", ControlID: "access", EvidencePackageIDs: []string{"package"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Append(a.ID, a.Version, "outside", "assessor", Event{Kind: "response", Body: "mutate owner response"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("assessor mutated owner response: %v", err)
	}
	now = a.ExpiresAt
	if _, err = s.Append(a.ID, a.Version, "outside", "assessor", Event{Kind: "question", Body: "late"}); !errors.Is(err, ErrExpired) {
		t.Fatalf("expected expiry, got %v", err)
	}
}
