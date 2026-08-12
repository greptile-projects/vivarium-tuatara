package charters

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func validRevision() Revision {
	return Revision{Title: "Project charter", Summary: "Who decides and how.", Roles: []Role{{Name: "maintainer", Description: "Stewards the project", Eligibility: []string{"repository_owner"}}}, DecisionClasses: []DecisionClass{{Name: "protected change", Description: "Changes protected resources", EligibleRoles: []string{"maintainer"}, Participation: 1, Quorum: 1, Approval: "majority", ProtectedResources: []string{"branch:main"}}}, Procedures: Procedures{Terms: "Annual", Removal: "Attributed vote", Succession: "Named successor", Amendments: "New approved revision"}}
}

func TestExceptionMustMatchActiveRule(t *testing.T) {
	s, _ := New(t.TempDir())
	if _, err := s.Publish("repository", "repo1", "owner", 0, validRevision()); err != nil {
		t.Fatal(err)
	}
	expires := time.Now().Add(time.Hour)
	if _, err := s.Except("repository", "repo1", "owner", 1, "protected change", "branch:main", "draft", expires); !errors.Is(err, ErrInvalid) {
		t.Fatalf("draft exception = %v", err)
	}
	if _, err := s.Approve("repository", "repo1", "owner", 1, "approved", "adopt"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Activate("repository", "repo1", "owner", 1); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ class, resource string }{{"unknown", "branch:main"}, {"protected change", "branch:other"}} {
		if _, err := s.Except("repository", "repo1", "owner", 1, tc.class, tc.resource, "invalid", expires); !errors.Is(err, ErrInvalid) {
			t.Fatalf("exception %v = %v", tc, err)
		}
	}
	if _, err := s.Except("repository", "repo1", "owner", 1, "protected change", "branch:main", "temporary", expires); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish("repository", "repo1", "owner", 1, validRevision()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Approve("repository", "repo1", "owner", 2, "approved", "replace"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Activate("repository", "repo1", "owner", 2); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Except("repository", "repo1", "owner", 1, "protected change", "branch:main", "superseded", expires); !errors.Is(err, ErrInvalid) {
		t.Fatalf("superseded exception = %v", err)
	}
}

func TestIndependentStoresSerializeApprovals(t *testing.T) {
	root := t.TempDir()
	seed, _ := New(root)
	if _, err := seed.Publish("repository", "repo1", "owner", 0, validRevision()); err != nil {
		t.Fatal(err)
	}
	const writers = 24
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s, _ := New(root)
			_, err := s.Approve("repository", "repo1", string(rune('a'+i)), 1, "approved", "concurrent")
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	record, err := seed.Get("repository", "repo1")
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Approvals) != writers {
		t.Fatalf("approvals = %d, want %d", len(record.Approvals), writers)
	}
}

func TestRevisionApprovalActivationAndHistory(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	r, err := s.Publish("repository", "repo1", "owner", 0, validRevision())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Activate("repository", "repo1", "owner", 1); !errors.Is(err, ErrInvalid) {
		t.Fatalf("activation without approval = %v", err)
	}
	r, err = s.Approve("repository", "repo1", "owner", 1, "approved", "adopt")
	if err != nil {
		t.Fatal(err)
	}
	r, err = s.Activate("repository", "repo1", "owner", 1)
	if err != nil {
		t.Fatal(err)
	}
	if r.ActiveVersion != 1 || r.Revisions[0].Status != "active" || len(r.Approvals) != 1 {
		t.Fatalf("record = %#v", r)
	}
	r, err = s.Publish("repository", "repo1", "owner", 1, validRevision())
	if err != nil {
		t.Fatal(err)
	}
	if r.Revisions[0].Status != "active" || r.Revisions[1].Status != "draft" {
		t.Fatalf("history changed = %#v", r.Revisions)
	}
}

func TestRejectsImpossibleInternalRules(t *testing.T) {
	s, _ := New(t.TempDir())
	v := validRevision()
	v.DecisionClasses[0].Quorum = 2
	if _, err := s.Publish("repository", "repo1", "owner", 0, v); !errors.Is(err, ErrInvalid) {
		t.Fatalf("publish = %v", err)
	}
}
