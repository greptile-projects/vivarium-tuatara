package governance

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"
)

func proposal(now time.Time) Proposal {
	return Proposal{ScopeType: "repository", ScopeID: "repo", CharterVersion: 1, Source: Reference{Kind: "technical_decision", ResourceID: "d1", Label: "Decision"}, Title: "Adopt protocol", Summary: "Choose together", Scope: "Repository protocol", Alternatives: []Alternative{{ID: "yes", Title: "Adopt", Summary: "Adopt it"}, {ID: "no", Title: "Decline", Summary: "Keep current"}}, Evidence: []Reference{{Kind: "experiment", ResourceID: "e1", Label: "Benchmark"}}, AffectedResources: []Reference{{Kind: "branch", ResourceID: "main", Label: "main"}}, DisclosureRequirements: []string{"conflicts"}, ImplementationEffects: []string{"open implementation work"}, Rule: Rule{DecisionClass: "direction", EligibleRoles: []string{"maintainer"}, Quorum: 2, Threshold: "majority", OpensAt: now.Add(-time.Hour), ClosesAt: now.Add(time.Hour)}, Electorate: []Elector{{UserID: "a", Roles: []string{"maintainer"}, Eligible: true}, {UserID: "b", Roles: []string{"maintainer"}, Eligible: true}}, CreatedBy: "a"}
}

func TestBallotsAreUniqueAndEligibilityChangesAtTally(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	s, _ := New(t.TempDir())
	s.now = func() time.Time { return now }
	p, e := s.Create(proposal(now))
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Cast(p.ID, "a", "yes", "because", true, "live"); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Cast(p.ID, "a", "no", "again", true, "live"); !errors.Is(e, ErrDuplicateBallot) {
		t.Fatalf("duplicate=%v", e)
	}
	if _, e = s.Cast(p.ID, "b", "abstain", "", true, "live"); e != nil {
		t.Fatal(e)
	}
	now = now.Add(2 * time.Hour)
	p, e = s.Finalize(p.ID, "a", []Elector{{UserID: "a", Eligible: true}}, nil)
	if e != nil {
		t.Fatal(e)
	}
	if p.Tally.Participating != 1 || p.Tally.Eligible != 1 || p.Tally.QuorumMet || p.Ballots[1].EligibleAtTally {
		t.Fatalf("tally=%#v ballots=%#v", p.Tally, p.Ballots)
	}
}

func TestConcurrentStoresAdmitOnlyOneBallot(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	seed, _ := New(root)
	p, e := seed.Create(proposal(now))
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, _ := New(root)
			_, e := s.Cast(p.ID, "a", "yes", "", true, "live")
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	success, conflict := 0, 0
	for e := range errs {
		if e == nil {
			success++
		} else if errors.Is(e, ErrDuplicateBallot) {
			conflict++
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("success=%d conflict=%d", success, conflict)
	}
}

func TestSecretBallotDataPersistsForVerifiableTally(t *testing.T) {
	now := time.Now()
	s, _ := New(t.TempDir())
	s.now = func() time.Time { return now }
	in := proposal(now)
	in.Rule.SecretBallot = true
	p, _ := s.Create(in)
	p, _ = s.Cast(p.ID, "a", "yes", "retained dissent", true, "live")
	p, _ = s.Cast(p.ID, "b", "no", "minority", true, "live")
	now = now.Add(2 * time.Hour)
	p, e := s.Finalize(p.ID, "a", p.Electorate, []string{"result challenged"})
	if e != nil {
		t.Fatal(e)
	}
	if !p.Tally.Contested || p.Tally.VerificationSHA256 == "" || p.Tally.Counts["yes"] != 1 || p.Tally.Counts["no"] != 1 || p.Ballots[1].Reason != "minority" {
		t.Fatalf("proposal=%#v", p)
	}
}

func TestFinalizedTallyCannotBeReplaced(t *testing.T) {
	now := time.Now()
	s, _ := New(t.TempDir())
	s.now = func() time.Time { return now }
	p, _ := s.Create(proposal(now))
	p, _ = s.Cast(p.ID, "a", "yes", "", true, "live")
	now = now.Add(2 * time.Hour)
	p, err := s.Finalize(p.ID, "a", []Elector{{UserID: "a", Eligible: true}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	first := *p.Tally
	if _, err = s.Finalize(p.ID, "a", nil, []string{"replace result"}); !errors.Is(err, ErrFinalized) {
		t.Fatalf("second finalize = %v", err)
	}
	p, _ = s.Get(p.ID)
	if !reflect.DeepEqual(*p.Tally, first) || len(p.Events) != 3 {
		t.Fatalf("final tally changed = %#v, events = %#v", p.Tally, p.Events)
	}
}

func TestAcceptedDecisionReceiptFreezesImplementationAuthority(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Now()
	s.now = func() time.Time { return now }
	created, err := s.Create(proposal(now))
	if err != nil {
		t.Fatal(err)
	}
	created, err = s.change(created.ID, func(v *Proposal) error {
		v.Tally = &Tally{Status: "accepted", Result: "yes", VerificationSHA256: "tally"}
		v.Status = "closed"
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	in := Implementation{Kind: "task_plan", RepositoryID: "repo", Scope: "main branch", Cost: "two engineer days", Assumptions: []string{"dependency remains supported"}, ProtectedEffects: []string{"branch:main"}}
	got, err := s.BeginImplementation(created.ID, "owner", in)
	if err != nil {
		t.Fatal(err)
	}
	if got.Implementation == nil || got.Implementation.Receipt.AuthorizationSHA256 == "" || got.Implementation.Steps[0].RequiredApproval == "" {
		t.Fatalf("implementation = %#v", got.Implementation)
	}
	retry, err := s.BeginImplementation(created.ID, "owner", in)
	if err != nil || retry.Implementation.Receipt.ID != got.Implementation.Receipt.ID {
		t.Fatalf("retry = %#v, %v", retry.Implementation, err)
	}
	in.Cost = "unbounded"
	if _, err = s.BeginImplementation(created.ID, "owner", in); !errors.Is(err, ErrMaterialChange) {
		t.Fatalf("material change = %v", err)
	}
	linked, err := s.LinkImplementation(created.ID, "owner", "ordinary-proposal")
	if err != nil || linked.Implementation.Steps[0].Status != "in_progress" {
		t.Fatalf("linked = %#v, %v", linked.Implementation, err)
	}
}
