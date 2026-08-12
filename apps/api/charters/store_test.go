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

func TestRepositoryRejectsUnresolvableEligibilitySources(t *testing.T) {
	for _, source := range []string{"organization_owner", "organization_member", "team_maintainer", "approved_agent"} {
		s, _ := New(t.TempDir())
		v := validRevision()
		v.Roles[0].Eligibility = []string{source}
		if _, err := s.Publish("repository", "repo1", "owner", 0, v); !errors.Is(err, ErrInvalid) {
			t.Fatalf("source %q publish = %v", source, err)
		}
	}
}

func TestOrganizationAcceptsEveryResolvableEligibilitySource(t *testing.T) {
	for _, source := range []string{"organization_owner", "organization_member", "team_maintainer", "approved_agent"} {
		s, _ := New(t.TempDir())
		v := validRevision()
		v.Roles[0].Eligibility = []string{source}
		if _, err := s.Publish("organization", "org1", "owner", 0, v); err != nil {
			t.Fatalf("source %q publish = %v", source, err)
		}
	}
}

func TestStandingLifecycleRetainsEvidenceConflictAndAppeal(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	if _, err := s.Publish("repository", "repo1", "owner", 0, validRevision()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Approve("repository", "repo1", "owner", 1, "approved", "adopt"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Activate("repository", "repo1", "owner", 1); err != nil {
		t.Fatal(err)
	}
	record, err := s.Invite("repository", "repo1", "owner", 0, 1, "human", "contributor", "maintainer", "Represent contributor experience", []Evidence{{Kind: "contribution", ResourceID: "pull-7", Summary: "Merged accessibility work"}}, now.Add(30*24*time.Hour))
	if err != nil || record.Standings[0].Status != "invited" {
		t.Fatalf("invite = %#v, %v", record, err)
	}
	id := record.Standings[0].ID
	if _, err = s.ActOnStanding("repository", "repo1", id, "intruder", "accept", "accept", ""); !errors.Is(err, ErrInvalid) {
		t.Fatalf("foreign acceptance = %v", err)
	}
	if record, err = s.ActOnStanding("repository", "repo1", id, "contributor", "accept", "I accept the duties", ""); err != nil || record.Standings[0].Status != "active" {
		t.Fatalf("accept = %#v, %v", record, err)
	}
	if record, err = s.ActOnStanding("repository", "repo1", id, "contributor", "recuse", "I authored the proposal", "proposal-9"); err != nil || record.Standings[0].Status != "recused" || record.Standings[0].ConflictOfInterest == "" {
		t.Fatalf("recuse = %#v, %v", record, err)
	}
	if record, err = s.ActOnStanding("repository", "repo1", id, "owner", "suspend", "identity under review", ""); err != nil || record.Standings[0].Status != "suspended" {
		t.Fatalf("suspend = %#v, %v", record, err)
	}
	if record, err = s.ActOnStanding("repository", "repo1", id, "contributor", "appeal", "identity restored", ""); err != nil || len(record.Standings[0].Events) != 5 || record.Standings[0].Status != "suspended" {
		t.Fatalf("appeal = %#v, %v", record, err)
	}
}

func TestStandingRequiresActiveCharterRoleBoundedTermAndClosedEvidence(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Now()
	s.now = func() time.Time { return now }
	if _, err := s.Publish("repository", "repo1", "owner", 0, validRevision()); err != nil {
		t.Fatal(err)
	}
	inputs := []struct {
		role     string
		evidence []Evidence
		expiry   time.Time
	}{{"missing", []Evidence{{Kind: "review", ResourceID: "r", Summary: "s"}}, now.Add(time.Hour)}, {"maintainer", []Evidence{{Kind: "commit", ResourceID: "r", Summary: "s"}}, now.Add(time.Hour)}, {"maintainer", []Evidence{{Kind: "review", ResourceID: "r", Summary: "s"}}, now.Add(-time.Hour)}}
	for _, in := range inputs {
		if _, err := s.Invite("repository", "repo1", "owner", 0, 1, "human", "person", in.role, "duties", in.evidence, in.expiry); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid invite = %v", err)
		}
	}
}

func TestStandingPersistsAcrossStoreReopen(t *testing.T) {
	root := t.TempDir()
	s, _ := New(root)
	now := time.Now()
	s.now = func() time.Time { return now }
	if _, err := s.Publish("repository", "repo1", "owner", 0, validRevision()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Approve("repository", "repo1", "owner", 1, "approved", "adopt"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Activate("repository", "repo1", "owner", 1); err != nil {
		t.Fatal(err)
	}
	created, err := s.Invite("repository", "repo1", "owner", 0, 1, "human", "person", "maintainer", "Review changes", []Evidence{{Kind: "review", ResourceID: "pull-1", Summary: "Sustained review"}}, now.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	reopened, _ := New(root)
	loaded, err := reopened.Get("repository", "repo1")
	if err != nil || len(loaded.Standings) != 1 || loaded.Standings[0].ID != created.Standings[0].ID || len(loaded.Standings[0].Events) != 1 {
		t.Fatalf("reopened = %#v, %v", loaded, err)
	}
}
