package assuranceassessments

import (
	"errors"
	"sync"
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
	a, err = s.Append(a.ID, a.Version, "owner", "owner", Event{Kind: "close", Body: "retain the final record after assessor access ends"})
	if err != nil || a.Status != "closed" {
		t.Fatalf("owner close after assessor expiry = %#v, %v", a, err)
	}
}

func TestAppendSerializesAcrossStoreProcessesAndHonorsStart(t *testing.T) {
	root := t.TempDir()
	first, _ := New(root)
	second, _ := New(root)
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	first.now = func() time.Time { return now }
	second.now = first.now
	a, err := first.Create(Assessment{RepositoryID: "repo", ProgramID: "program", ProgramVersion: 1, Title: "Future review", OwnerID: "owner", Assessor: Assessor{UserID: "outside", Kind: "external", ConflictDisclosure: "none"}, Scope: Scope{ControlIDs: []string{"control"}, PeriodStartsAt: now.Add(-time.Hour), PeriodEndsAt: now}, StartsAt: now.Add(time.Hour), ExpiresAt: now.Add(2 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = first.Append(a.ID, 1, "outside", "assessor", Event{Kind: "question", Body: "too early"}); !errors.Is(err, ErrNotStarted) {
		t.Fatalf("pre-start append = %v", err)
	}
	// Owners may prepare responses before the assessor window. At the exact boundary, two processes cannot both consume one CAS version.
	now = a.StartsAt
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i, store := range []*Store{first, second} {
		wg.Add(1)
		go func(i int, s *Store) {
			defer wg.Done()
			<-start
			_, e := s.Append(a.ID, 1, "outside", "assessor", Event{Kind: "question", Body: []string{"first", "second"}[i]})
			errs <- e
		}(i, store)
	}
	close(start)
	wg.Wait()
	close(errs)
	success, conflict := 0, 0
	for e := range errs {
		if e == nil {
			success++
		} else if errors.Is(e, ErrConflict) {
			conflict++
		} else {
			t.Fatalf("append = %v", e)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("success=%d conflict=%d", success, conflict)
	}
	got, err := first.Get(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != 2 || len(got.Events) != 1 {
		t.Fatalf("persisted version=%d events=%d", got.Version, len(got.Events))
	}
}
