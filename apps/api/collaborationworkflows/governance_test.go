package collaborationworkflows

import (
	"errors"
	"testing"
	"time"
)

func governedPreview(t *testing.T, s *Store) Preview {
	t.Helper()
	d := validDefinition()
	d.OwnerIDs = []string{"author"}
	d.Triggers[0].ID = "pull-case"
	d.Steps[0].Invocation.Action = "merge"
	d.Steps[0].Approval = "merge"
	p := s.Preview("repo", d, Source{Revision: "0123456789012345678901234567890123456789", Path: ".vivarium/workflow.json", SHA256: "source-digest"}, func(Invocation) (bool, string) { return true, "" })
	if !p.Activatable {
		t.Fatalf("preview blocked: %#v", p.Diagnostics)
	}
	return p
}

func TestGovernanceRequiresIndependentReviewScenarioAndOwners(t *testing.T) {
	s, _ := New(t.TempDir())
	policy, err := s.SetGovernancePolicy("repo", "repo-owner", 0, GovernancePolicy{RequiredReviews: 1, RequiredScenarioIDs: []string{"pull-case"}, RequireOwnerAcknowledged: true, RequireSeparation: true, ApprovalTTLSeconds: 3600, ProtectedActionClasses: []string{"merge"}, ResourceOwnerIDs: []string{"resource-owner"}})
	if err != nil || policy.Version != 1 {
		t.Fatal(policy, err)
	}
	c, err := s.EvaluateCandidate("repo", "", "author", 0, governedPreview(t, s))
	if err != nil || c.Ready || len(c.SimulationCases) != 1 || c.EstimatedActionCost != 10 {
		t.Fatalf("candidate = %#v, %v", c, err)
	}
	if listed, listErr := s.List("repo"); listErr != nil || len(listed) != 0 {
		t.Fatalf("governance records leaked into workflows: %#v %v", listed, listErr)
	}
	if err := s.RequireApprovedCandidate("repo", "", c.Source.SHA256, 0); !errors.Is(err, ErrGovernanceBlocked) {
		t.Fatalf("ungoverned activation = %v", err)
	}
	c, _ = s.DecideCandidate(c.ID, "reviewer", "review", "", "", "", nil)
	c, _ = s.DecideCandidate(c.ID, "reviewer", "scenario_pass", "", "pull-case", "", nil)
	c, err = s.DecideCandidate(c.ID, "resource-owner", "owner_acknowledgement", "resource-owner", "", "", nil)
	if err != nil || !c.Ready {
		t.Fatalf("approved candidate = %#v, %v", c, err)
	}
	if err := s.RequireApprovedCandidate("repo", "", c.Source.SHA256, 0); err != nil {
		t.Fatalf("approved activation = %v", err)
	}

	_, err = s.SetGovernancePolicy("repo", "repo-owner", 1, GovernancePolicy{RequiredReviews: 2, RequiredScenarioIDs: []string{"pull-case"}, RequireOwnerAcknowledged: true, RequireSeparation: true, ApprovalTTLSeconds: 3600, ProtectedActionClasses: []string{"merge"}, ResourceOwnerIDs: []string{"resource-owner"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RequireApprovedCandidate("repo", "", c.Source.SHA256, 0); !errors.Is(err, ErrGovernanceBlocked) {
		t.Fatalf("policy drift = %v", err)
	}
}

func TestExceptionsExpireAndControlPreservesHistory(t *testing.T) {
	s, _ := New(t.TempDir())
	s.now = func() time.Time { return time.Date(2026, 8, 23, 9, 0, 0, 0, time.UTC) }
	_, _ = s.SetGovernancePolicy("repo", "repo-owner", 0, GovernancePolicy{RequiredReviews: 2, ApprovalTTLSeconds: 3600, ProtectedActionClasses: []string{"merge"}, ResourceOwnerIDs: []string{"repo-owner"}})
	c, _ := s.EvaluateCandidate("repo", "", "author", 0, governedPreview(t, s))
	expires := s.now().Add(time.Hour)
	c, err := s.DecideCandidate(c.ID, "repo-owner", "exception", "", "", "incident mitigation", &expires)
	if err != nil || !c.Ready {
		t.Fatalf("exception = %#v, %v", c, err)
	}
	_, err = s.SetGovernancePolicy("repo", "repo-owner", 1, GovernancePolicy{RequiredReviews: 2, ApprovalTTLSeconds: 3600, ProtectedActionClasses: []string{"merge"}, ResourceOwnerIDs: []string{"successor-owner"}})
	if err != nil {
		t.Fatal(err)
	}
	c, _ = s.GetCandidate(c.ID)
	if c.Ready || !contains(c.Blockers, "policy_changed") {
		t.Fatalf("superseded exception remained ready: %#v", c)
	}
	c, err = s.EvaluateCandidate("repo", "", "author", 0, governedPreview(t, s))
	if err != nil {
		t.Fatal(err)
	}
	expires = s.now().Add(time.Hour)
	c, err = s.DecideCandidate(c.ID, "successor-owner", "exception", "", "", "successor mitigation", &expires)
	if err != nil || !c.Ready {
		t.Fatalf("successor exception = %#v, %v", c, err)
	}
	s.now = func() time.Time { return expires.Add(time.Second) }
	c, _ = s.GetCandidate(c.ID)
	if c.Ready {
		t.Fatal("expired exception remained ready")
	}

	p := governedPreview(t, s)
	// A repository without a policy remains backward compatible.
	s2, _ := New(t.TempDir())
	w, err := s2.Create("repo", "author", "activation", p)
	if err != nil {
		t.Fatal(err)
	}
	d := validDefinition()
	d.Outcome = "successor"
	w, _ = s2.Revise(w.ID, 1, "author", s2.Preview("repo", d, Source{SHA256: "two"}, func(Invocation) (bool, string) { return true, "" }))
	w, err = s2.Control(w.ID, "repo-owner", "rollback", 1)
	if err != nil || w.CurrentVersion != 3 || len(w.Revisions) != 3 || w.Revisions[2].Source != w.Revisions[0].Source {
		t.Fatalf("rollback rewrote history: %#v %v", w, err)
	}
	w, _ = s2.Control(w.ID, "repo-owner", "disable", 0)
	if _, err = s2.StartExecution(w.ID, 1, TriggerEvent{}); !errors.Is(err, ErrExecutionBlocked) {
		t.Fatalf("disabled start = %v", err)
	}
}

func TestPolicyCreatedAfterExecutionBlocksFurtherClaims(t *testing.T) {
	s, w, event := executionFixture(t)
	ex, err := s.StartExecution(w.ID, 1, event)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.SetGovernancePolicy("repo", "repo-owner", 0, GovernancePolicy{RequiredReviews: 1, ApprovalTTLSeconds: 3600, ProtectedActionClasses: []string{"merge"}, ResourceOwnerIDs: []string{"owner"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ClaimStep(ex.ID, "classify", ex.Version, true); !errors.Is(err, ErrExecutionBlocked) {
		t.Fatalf("claim after policy creation = %v", err)
	}
}

func TestGovernedRollbackRequiresExactCurrentCandidate(t *testing.T) {
	s, _ := New(t.TempDir())
	first := validDefinition()
	first.OwnerIDs = []string{"author"}
	first.Steps[0].OwnerIDs = []string{"author"}
	first.Steps[1].OwnerIDs = []string{"author"}
	firstSource := Source{Revision: "0123456789012345678901234567890123456789", Path: ".vivarium/workflow.json", SHA256: "first-source"}
	w, err := s.Create("repo", "author", "first", s.Preview("repo", first, firstSource, func(Invocation) (bool, string) { return true, "" }))
	if err != nil {
		t.Fatal(err)
	}
	second := first
	second.Outcome = "successor"
	w, err = s.Revise(w.ID, 1, "author", s.Preview("repo", second, Source{Revision: firstSource.Revision, Path: firstSource.Path, SHA256: "second-source"}, func(Invocation) (bool, string) { return true, "" }))
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.SetGovernancePolicy("repo", "repo-owner", 0, GovernancePolicy{RequiredReviews: 1, RequiredScenarioIDs: []string{"pull"}, RequireOwnerAcknowledged: true, RequireSeparation: true, ApprovalTTLSeconds: 3600, ResourceOwnerIDs: []string{"resource-owner"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Control(w.ID, "repo-owner", "rollback", 1); !errors.Is(err, ErrGovernanceBlocked) {
		t.Fatalf("unapproved rollback = %v", err)
	}
	c, err := s.EvaluateCandidate("repo", w.ID, "author", 2, s.Preview("repo", first, firstSource, func(Invocation) (bool, string) { return true, "" }))
	if err != nil {
		t.Fatal(err)
	}
	c, _ = s.DecideCandidate(c.ID, "reviewer", "review", "", "", "", nil)
	c, _ = s.DecideCandidate(c.ID, "reviewer", "scenario_pass", "", "pull", "", nil)
	c, err = s.DecideCandidate(c.ID, "resource-owner", "owner_acknowledgement", "resource-owner", "", "", nil)
	if err != nil || !c.Ready {
		t.Fatalf("rollback candidate = %#v, %v", c, err)
	}
	w, err = s.Control(w.ID, "repo-owner", "rollback", 1)
	if err != nil || w.CurrentVersion != 3 || w.Revisions[2].Source.SHA256 != firstSource.SHA256 {
		t.Fatalf("approved rollback = %#v, %v", w, err)
	}
}
