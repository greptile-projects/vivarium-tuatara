package regressioninvestigations

import (
	"errors"
	"os"
	"testing"
)

const commit = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func validInvestigation() Investigation {
	return Investigation{RequestID: "request-1", RepositoryID: "repo-1", Title: "Checkout regression", Source: Reference{Kind: "issue", ResourceID: "issue-1", Label: "Reported checkout failure"}, ExpectedBehavior: "Checkout completes once.", RegressedBehavior: "Checkout submits twice.", KnownGood: Boundary{Kind: "commit", Revision: commit, Label: "last known good"}, KnownBad: Boundary{Kind: "commit", Revision: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Label: "first known bad"}, Environments: []string{"staging"}, Severity: "high", OwnerIDs: []string{"owner-1"}, AcceptanceCriteria: []string{"The boundary reproduces the behavior difference."}, Comparable: true, Evidence: []Evidence{{Kind: "issue", ResourceID: "issue-1", Label: "report", Visibility: "repository", Available: true}}}
}

func TestStoreRetainsAttributedCASHistory(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.Create(validInvestigation(), "actor-1")
	if err != nil {
		t.Fatal(err)
	}
	if v.Version != 1 || v.Status != "open" || len(v.History) != 1 || v.Evidence[0].ID == "" {
		t.Fatalf("unexpected creation: %#v", v)
	}
	updated, err := s.Append(v.RepositoryID, v.ID, "owner-1", "hypothesis", "The serializer changed inside this boundary.", "", v.Version)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Version != 2 || updated.History[1].ActorID != "owner-1" || updated.History[1].Kind != "hypothesis" {
		t.Fatalf("history was not attributed: %#v", updated.History)
	}
	if _, err = s.Append(v.RepositoryID, v.ID, "actor-1", "discussion", "stale update", "", v.Version); !errors.Is(err, ErrConflict) {
		t.Fatalf("want conflict, got %v", err)
	}
}

func TestStoreRetainsScopeAndStatusChanges(t *testing.T) {
	s, _ := New(t.TempDir())
	v, err := s.Create(validInvestigation(), "actor-1")
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Append(v.RepositoryID, v.ID, "owner-1", "scope_change", "Production is affected too.", "staging, production", v.Version)
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Append(v.RepositoryID, v.ID, "owner-1", "status_change", "The search boundary is agreed.", "bounded", v.Version)
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != "bounded" || len(v.Environments) != 2 || v.History[1].From != "staging" || v.History[2].From != "open" {
		t.Fatalf("changes were not retained: %#v", v)
	}
}

func TestStoreRejectsIncompleteOrCredentialShapedContext(t *testing.T) {
	s, _ := New(t.TempDir())
	v := validInvestigation()
	v.AcceptanceCriteria = nil
	if _, err := s.Create(v, "actor-1"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("want invalid, got %v", err)
	}
	v = validInvestigation()
	v.ExpectedBehavior = "token=do-not-retain"
	if _, err := s.Create(v, "actor-1"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("want invalid sensitive context, got %v", err)
	}
}

func TestCreateRetryReconcilesPublishedRecordAfterDirectorySyncFailure(t *testing.T) {
	s, _ := New(t.TempDir())
	in := validInvestigation()
	s.syncDirectory = func(*os.File) error { return errors.New("injected directory sync failure") }
	published, err := s.Create(in, "actor-1")
	if err == nil || published.ID == "" {
		t.Fatalf("want published record and durability error, got %#v, %v", published, err)
	}
	if _, err = s.Get(in.RepositoryID, published.ID); err != nil {
		t.Fatalf("published record is not readable: %v", err)
	}
	s.syncDirectory = func(d *os.File) error { return d.Sync() }
	reconciled, err := s.Create(in, "actor-1")
	if err != nil {
		t.Fatal(err)
	}
	if reconciled.ID != published.ID {
		t.Fatalf("retry duplicated create: %s != %s", reconciled.ID, published.ID)
	}
	items, err := s.List(in.RepositoryID)
	if err != nil || len(items) != 1 {
		t.Fatalf("want one retained investigation, got %#v, %v", items, err)
	}
}

func TestCreateRejectsChangedRequestIdentityReuse(t *testing.T) {
	s, _ := New(t.TempDir())
	in := validInvestigation()
	if _, err := s.Create(in, "actor-1"); err != nil {
		t.Fatal(err)
	}
	in.Title = "Different boundary"
	if _, err := s.Create(in, "actor-1"); !errors.Is(err, ErrConflict) {
		t.Fatalf("want conflict, got %v", err)
	}
}
