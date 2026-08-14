package projectfunds

import (
	"encoding/base64"
	"errors"
	"testing"
	"time"
)

func outcomeTerms() OutcomeTerms {
	return OutcomeTerms{
		Title: "Fund the latency repair", Source: OutcomeSource{Kind: "issue", ID: "issue-1", Revision: "3", Visibility: "public"},
		Scope: "Reduce p95 latency without changing response semantics.", AcceptanceCriteria: []string{"p95 below 200ms", "all correctness checks pass"}, EvidenceRequirements: []string{"exact-revision benchmark", "merged pull and production observation"},
		Budget: 10000, Deadline: time.Now().UTC().Add(7 * 24 * time.Hour), ContributorEligibility: []string{"repository_participant", "approved_agent_with_live_grant"}, AllocationMethod: "milestone_claim", CancellationTerms: "Return backing for unaccepted milestones; retain attributable evidence.",
		Dependencies: []string{"benchmark environment available"}, Risks: []string{"noisy production traffic"}, Conflicts: []string{"open optimization pull"},
		Milestones: []Milestone{{ID: "diagnose", Title: "Reproduce and diagnose", Budget: 3000, AcceptanceCriteria: []string{"owner confirms diagnosis"}, EvidenceRequirements: []string{"sanitized flame trace"}}, {ID: "deliver", Title: "Deliver verified repair", Budget: 7000, AcceptanceCriteria: []string{"production target met"}, EvidenceRequirements: []string{"release observation"}, Dependencies: []string{"diagnose"}}},
	}
}

func outcomeStore(t *testing.T) (*Store, Fund) {
	t.Helper()
	s, err := New(t.TempDir(), map[string]string{"card": base64Key(testPublicKey)})
	if err != nil {
		t.Fatal(err)
	}
	f, err := s.Create("repo", "owner", Terms{Name: "Fund", Purpose: "Work", Stewards: []string{"owner"}, FundingSources: []string{"card"}, Unit: "USD", Precision: 2, SpendingLimits: []Limit{{Period: "monthly", Amount: 100000}}, ApprovalRules: []ApprovalRule{{RequiredApprovals: 1, EligibleApprovers: []string{"owner"}}}, EligibleRecipients: []string{"contributors"}, RefundPolicy: "Return unallocated funds", LedgerVisibility: "public"})
	if err != nil {
		t.Fatal(err)
	}
	return s, f
}

func base64Key(key []byte) string { return base64.StdEncoding.EncodeToString(key) }

func TestOutcomeFundingLifecycleMakesAcceptanceAndReplanningExplicit(t *testing.T) {
	s, fund := outcomeStore(t)
	terms := outcomeTerms()
	out, err := s.CreateOutcome("repo", fund.ID, "owner", terms)
	if err != nil {
		t.Fatal(err)
	}
	if out.Diagnostics[0].Kind != "insufficient_funds" || out.Revisions[0].Terms.EvidenceRequirements[0] == "" {
		t.Fatalf("initial projection = %+v", out)
	}
	out, err = s.PledgeOutcome(out.ID, "backer", "diagnose", 3000, "pledge-1", "Fund diagnosis", out.Version)
	if err != nil {
		t.Fatal(err)
	}
	out, err = s.PledgeOutcome(out.ID, "backer", "deliver", 7000, "pledge-2", "Fund delivery", out.Version)
	if err != nil {
		t.Fatal(err)
	}
	if out.Pledged != terms.Budget || out.MilestonePledged["deliver"] != 7000 {
		t.Fatalf("pledges = %+v", out)
	}
	terms.Scope = "Reduce p95 and document the fallback."
	out, err = s.ReviseOutcome(out.ID, "owner", out.Version, terms, "Production risk requires a documented fallback.")
	if err != nil {
		t.Fatal(err)
	}
	if out.Pledged != 0 || out.Pledges[0].Status != "reconfirmation_required" || out.Diagnostics[0].Kind != "insufficient_funds" {
		t.Fatalf("changed scope = %+v", out)
	}
	out, err = s.ChangePledge(out.ID, out.Pledges[0].ID, "backer", "reconfirm", "Still worth pursuing under the revised evidence contract.", out.Version)
	if err != nil {
		t.Fatal(err)
	}
	out, err = s.ChangePledge(out.ID, out.Pledges[1].ID, "backer", "withdraw", "Deadline no longer fits.", out.Version)
	if err != nil {
		t.Fatal(err)
	}
	if out.Pledged != 3000 || out.Replans[len(out.Replans)-1].Kind != "withdrawn_backing" {
		t.Fatalf("withdrawal = %+v", out)
	}
}

func TestOutcomeFundingRejectsAmbiguousMilestonesAndStaleWrites(t *testing.T) {
	s, fund := outcomeStore(t)
	terms := outcomeTerms()
	terms.Milestones[0].Budget++
	if _, err := s.CreateOutcome("repo", fund.ID, "owner", terms); !errors.Is(err, ErrInvalid) {
		t.Fatalf("milestone budget err = %v", err)
	}
	terms = outcomeTerms()
	out, err := s.CreateOutcome("repo", fund.ID, "owner", terms)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.PledgeOutcome(out.ID, "backer", "unknown", 10, "key", "", out.Version); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown milestone err = %v", err)
	}
	if _, err := s.PledgeOutcome(out.ID, "backer", "", 10, "key", "", out.Version+1); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale pledge err = %v", err)
	}
}

func TestOutcomeFundingProjectsOverlappingAndEmbargoedWork(t *testing.T) {
	s, fund := outcomeStore(t)
	terms := outcomeTerms()
	terms.Source.Kind = "security_repair"
	terms.Source.Visibility = "embargoed"
	a, err := s.CreateOutcome("repo", fund.ID, "owner", terms)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateOutcome("repo", fund.ID, "owner", terms); err != nil {
		t.Fatal(err)
	}
	a, err = s.GetOutcome(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	for _, d := range a.Diagnostics {
		kinds[d.Kind] = true
	}
	if !kinds["embargoed_work"] || !kinds["overlapping_award"] {
		t.Fatalf("diagnostics = %+v", a.Diagnostics)
	}
}

func TestOutcomeFundingRejectsStorageTraversalIdentifiers(t *testing.T) {
	s, _ := outcomeStore(t)
	if _, err := s.GetOutcome("../fund"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("traversal read err = %v", err)
	}
	if _, err := s.CreateOutcome("repo", "../fund", "owner", outcomeTerms()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("traversal fund err = %v", err)
	}
}
