package acceptance

import (
	"errors"
	"testing"
)

func TestEvaluatePinsAcceptanceToRevisionAndBlocksRejectionAndFindings(t *testing.T) {
	policy := Policy{Version: 2, Requirements: []Requirement{{ID: "checkout", Paths: []string{"web/**"}, RiskClasses: []string{"customer-facing"}, Scenarios: []Scenario{{Name: "guest checkout", Role: "contributor", Blocking: true}, {Name: "keyboard flow", Role: "owner", Blocking: true}}}}}
	old := Decision{ID: "old", Revision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", PolicyVersion: 2, RequirementID: "checkout", Scenario: "guest checkout", Role: "contributor", Outcome: "accepted"}
	current := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	e := Evaluate(policy, current, []string{"web/cart.tsx"}, []string{"customer-facing"}, []Decision{old}, nil)
	if !e.Blocking || len(e.Applicable) != 1 || len(e.Missing) != 2 || len(e.StaleDecisions) != 1 {
		t.Fatalf("stale evaluation = %#v", e)
	}
	decisions := []Decision{{ID: "new", Revision: current, PolicyVersion: 2, RequirementID: "checkout", Scenario: "guest checkout", Role: "contributor", Outcome: "accepted"}, {ID: "reject", Revision: current, PolicyVersion: 2, RequirementID: "checkout", Scenario: "keyboard flow", Role: "owner", Outcome: "rejected", Rationale: "focus is lost"}}
	e = Evaluate(policy, current, []string{"web/cart.tsx"}, []string{"customer-facing"}, decisions, []Finding{{ID: "f", Revision: current, Title: "Order cannot submit", Severity: "blocking", Status: "open"}})
	if !e.Blocking || len(e.Missing) != 1 || len(e.Findings) != 1 {
		t.Fatalf("blocking evaluation = %#v", e)
	}
	decisions = append(decisions, Decision{ID: "ordinary-accept", Revision: current, PolicyVersion: 2, RequirementID: "checkout", Scenario: "keyboard flow", Role: "owner", Outcome: "accepted"})
	e = Evaluate(policy, current, []string{"web/cart.tsx"}, []string{"customer-facing"}, decisions, nil)
	if !e.Blocking || len(e.Missing) != 1 {
		t.Fatalf("acceptance cleared rejection = %#v", e)
	}
	decisions = append(decisions, Decision{ID: "override", Revision: current, PolicyVersion: 2, RequirementID: "checkout", Scenario: "keyboard flow", Role: "owner", Outcome: "overridden", Rationale: "tracked rollback"})
	e = Evaluate(policy, current, []string{"web/cart.tsx"}, []string{"customer-facing"}, decisions, []Finding{{ID: "f", Revision: current, Title: "fixed", Severity: "blocking", Status: "resolved"}})
	if e.Blocking || len(e.Missing) != 0 {
		t.Fatalf("resolved evaluation = %#v", e)
	}
}

func TestPolicyAndDecisionsPersistAppendOnly(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.SetPolicy("repo", "main", "owner", []Requirement{{ID: "core", Scenarios: []Scenario{{Name: "happy path", Role: "owner", Blocking: true}}}})
	if err != nil || p.Version != 1 {
		t.Fatalf("policy=%#v err=%v", p, err)
	}
	d, err := s.Decide(Decision{RepositoryID: "repo", PullRequestID: "pull", Revision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", PolicyVersion: 1, IdempotencyKey: "reject-core", RequirementID: "core", Scenario: "happy path", Role: "owner", Outcome: "rejected", Rationale: "incorrect result", ActorID: "owner"})
	if err != nil || d.ID == "" {
		t.Fatalf("decision=%#v err=%v", d, err)
	}
	all, err := s.Decisions("repo", "pull")
	if err != nil || len(all) != 1 || all[0].Rationale != "incorrect result" {
		t.Fatalf("decisions=%#v err=%v", all, err)
	}
	replayed, err := s.Decide(Decision{RepositoryID: "repo", PullRequestID: "pull", Revision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", PolicyVersion: 1, IdempotencyKey: "reject-core", RequirementID: "core", Scenario: "happy path", Role: "owner", Outcome: "rejected", Rationale: "incorrect result", ActorID: "owner"})
	if err != nil || replayed.ID != d.ID {
		t.Fatalf("replay=%#v err=%v", replayed, err)
	}
	_, err = s.Decide(Decision{RepositoryID: "repo", PullRequestID: "pull", Revision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", PolicyVersion: 1, IdempotencyKey: "accept-core", RequirementID: "core", Scenario: "happy path", Role: "owner", Outcome: "accepted", ActorID: "owner"})
	if !errors.Is(err, ErrRejected) {
		t.Fatalf("ordinary acceptance after rejection err=%v", err)
	}
}
