package agentevaluations

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestHiddenEvaluationAndTrialLabels(t *testing.T) {
	s, _ := New(t.TempDir())
	rev := Revision{RepositoryRevision: strings.Repeat("a", 40), Scenarios: []Scenario{{ID: "fix", Title: "Fix", SanitizedPrompt: "repair fixture", ExpectedOutcomes: []string{"works"}, Checks: []Check{{Name: "public", Kind: "contains", Expected: "works"}}, HiddenChecks: []Check{{Name: "answer key canary", Kind: "canary", Expected: "canary"}}}}, Budget: Budget{1, 1000, 3}, ProhibitedActions: []string{"publish"}, HumanReviewCriteria: []string{"inspect patch"}, ChangeSummary: "initial", CreatedBy: "owner"}
	suite, e := s.Create(Suite{OrganizationID: "org", RepositoryID: "repo", Name: "Safety"}, rev)
	if e != nil {
		t.Fatal(e)
	}
	view, _ := s.PublicGet(suite.ID)
	if view.Revisions[0].Scenarios[0].HiddenChecks != nil {
		t.Fatal("hidden checks leaked")
	}
	run, e := s.CreateRun(suite.ID, 1, RunInput{AgentID: "agent", AgentProfileVersion: 1, Outputs: map[string]string{"fix": "works canary"}, Cost: .2, LatencyMS: 20}, "owner")
	if e != nil {
		t.Fatal(e)
	}
	if run.Contaminated || run.TrialLabel != "initial" || run.Authority.Publish {
		t.Fatalf("unexpected run %#v", run)
	}
	if len(run.CheckResults) != 1 || run.CheckResults[0].Hidden {
		t.Fatalf("protected result leaked from create: %#v", run.CheckResults)
	}
	again, _ := s.CreateRun(suite.ID, 1, RunInput{AgentID: "agent", AgentProfileVersion: 1, Outputs: map[string]string{"fix": "works"}, ToolActions: []ToolAction{{Tool: "git", Action: "publish"}}, Cost: 2, LatencyMS: 20}, "owner")
	if again.TrialLabel != "repeated" || again.PolicyPassed || again.BudgetPassed {
		t.Fatalf("bad repeated classification %#v", again)
	}
	got, _ := s.GetRun(run.ID)
	if len(got.CheckResults) != 1 || got.CheckResults[0].Hidden {
		t.Fatalf("protected result leaked from read: %#v", got.CheckResults)
	}
	evaluator, _ := s.GetEvaluatorRun(run.ID)
	if !evaluator.Contaminated || len(evaluator.CheckResults) != 1 {
		t.Fatalf("evaluator projection lost protected aggregate: %#v", evaluator)
	}
}

func TestPublicAggregatesDoNotRevealProtectedOutcomes(t *testing.T) {
	s, _ := New(t.TempDir())
	rev := Revision{RepositoryRevision: strings.Repeat("e", 40), Scenarios: []Scenario{{ID: "fix", Title: "Fix", SanitizedPrompt: "fix fixture", ExpectedOutcomes: []string{"done"}, Checks: []Check{{Name: "public", Kind: "contains", Expected: "done"}}, HiddenChecks: []Check{{Name: "protected", Kind: "contains", Expected: "private-answer"}, {Name: "canary", Kind: "canary", Expected: "canary-value"}}}}, Budget: Budget{1, 1000, 3}, ProhibitedActions: []string{"publish"}, HumanReviewCriteria: []string{"inspect"}, ChangeSummary: "initial", CreatedBy: "owner"}
	suite, err := s.Create(Suite{OrganizationID: "org", RepositoryID: "repo", Name: "Protected"}, rev)
	if err != nil {
		t.Fatal(err)
	}
	clean, _ := s.CreateRun(suite.ID, 1, RunInput{AgentID: "agent", AgentProfileVersion: 1, Outputs: map[string]string{"fix": "done"}}, "member")
	matching, _ := s.CreateRun(suite.ID, 1, RunInput{AgentID: "agent", AgentProfileVersion: 1, Outputs: map[string]string{"fix": "done private-answer"}}, "member")
	leaked, _ := s.CreateRun(suite.ID, 1, RunInput{AgentID: "agent", AgentProfileVersion: 1, Outputs: map[string]string{"fix": "done private-answer canary-value"}}, "member")
	if clean.CorrectnessPassed != matching.CorrectnessPassed || clean.CorrectnessPassed != leaked.CorrectnessPassed || clean.PolicyPassed != matching.PolicyPassed || clean.Contaminated || matching.Contaminated || leaked.Contaminated || len(clean.ContaminationReasons) != 0 || len(matching.ContaminationReasons) != 0 || len(leaked.ContaminationReasons) != 0 {
		t.Fatalf("public aggregates reveal protected result: clean=%#v matching=%#v leaked=%#v", clean, matching, leaked)
	}
	evaluatorClean, _ := s.GetEvaluatorRun(clean.ID)
	evaluatorMatching, _ := s.GetEvaluatorRun(matching.ID)
	evaluatorLeaked, _ := s.GetEvaluatorRun(leaked.ID)
	if evaluatorClean.CorrectnessPassed == evaluatorMatching.CorrectnessPassed || !evaluatorLeaked.Contaminated {
		t.Fatalf("evaluator aggregates did not retain protected result: clean=%#v matching=%#v", evaluatorClean, evaluatorMatching)
	}
}

func TestRunsRequireEveryOutputAndFailuresCannotBeApproved(t *testing.T) {
	s, _ := New(t.TempDir())
	rev := Revision{RepositoryRevision: strings.Repeat("b", 40), Scenarios: []Scenario{{ID: "safe", Title: "Safe", SanitizedPrompt: "inspect fixture", ExpectedOutcomes: []string{"no leak"}, Checks: []Check{{Name: "no secret", Kind: "not_contains", Expected: "secret"}}, HiddenChecks: []Check{{Name: "canary", Kind: "canary", Expected: "answer"}}}}, Budget: Budget{1, 1000, 3}, ProhibitedActions: []string{"publish"}, HumanReviewCriteria: []string{"inspect"}, ChangeSummary: "initial", CreatedBy: "owner"}
	suite, err := s.Create(Suite{OrganizationID: "org", RepositoryID: "repo", Name: "Negative checks"}, rev)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateRun(suite.ID, 1, RunInput{AgentID: "agent", AgentProfileVersion: 1, Outputs: map[string]string{}}, "owner"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing output error = %v", err)
	}
	failed, err := s.CreateRun(suite.ID, 1, RunInput{AgentID: "agent", AgentProfileVersion: 1, Outputs: map[string]string{"safe": "clean"}, Failure: "candidate crashed"}, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if failed.CorrectnessPassed || failed.PolicyPassed || failed.BudgetPassed {
		t.Fatalf("failed execution retained passing state: %#v", failed)
	}
	if _, err = s.Decide(failed.ID, "owner", "approved", "looks fine"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("failed run approval error = %v", err)
	}
}

func TestReproducibilityReferencesExactPriorRun(t *testing.T) {
	s, _ := New(t.TempDir())
	revision := func(commit string) Revision {
		return Revision{RepositoryRevision: commit, Scenarios: []Scenario{{ID: "fix", Title: "Fix", SanitizedPrompt: "fix fixture", ExpectedOutcomes: []string{"done"}, Checks: []Check{{Name: "done", Kind: "contains", Expected: "done"}}}}, Budget: Budget{1, 1000, 3}, ProhibitedActions: []string{"publish"}, HumanReviewCriteria: []string{"inspect"}, ChangeSummary: "initial", CreatedBy: "owner"}
	}
	suite, _ := s.Create(Suite{OrganizationID: "org", RepositoryID: "repo", Name: "One"}, revision(strings.Repeat("c", 40)))
	prior, _ := s.CreateRun(suite.ID, 1, RunInput{AgentID: "agent", AgentProfileVersion: 1, Outputs: map[string]string{"fix": "done"}}, "owner")
	valid, err := s.CreateRun(suite.ID, 1, RunInput{AgentID: "agent", AgentProfileVersion: 1, ReproducesRunID: prior.ID, Outputs: map[string]string{"fix": "done"}}, "owner")
	if err != nil || !valid.Reproducible {
		t.Fatalf("valid reproduction = %#v, %v", valid, err)
	}
	if _, err = s.CreateRun(suite.ID, 1, RunInput{AgentID: "agent", AgentProfileVersion: 1, ReproducesRunID: "does-not-exist", Outputs: map[string]string{"fix": "done"}}, "owner"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("fabricated reference error = %v", err)
	}
	other, _ := s.Create(Suite{OrganizationID: "org", RepositoryID: "repo", Name: "Two"}, revision(strings.Repeat("d", 40)))
	if _, err = s.CreateRun(other.ID, 1, RunInput{AgentID: "agent", AgentProfileVersion: 1, ReproducesRunID: prior.ID, Outputs: map[string]string{"fix": "done"}}, "owner"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("cross-suite reference error = %v", err)
	}
	if _, err = s.CreateRun(suite.ID, 1, RunInput{AgentID: "agent", AgentProfileVersion: 2, ReproducesRunID: prior.ID, Outputs: map[string]string{"fix": "done"}}, "owner"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("cross-profile reference error = %v", err)
	}
	operator, err := s.CreateRun(suite.ID, 1, RunInput{AgentID: "agent", AgentProfileVersion: 1, OperatorSupplied: true, Outputs: map[string]string{"fix": "done"}}, "owner")
	if err != nil || operator.Reproducible {
		t.Fatalf("operator trial = %#v, %v", operator, err)
	}
	if _, err = s.CreateRun(suite.ID, 1, RunInput{AgentID: "agent", AgentProfileVersion: 1, ReproducesRunID: operator.ID, Outputs: map[string]string{"fix": "done"}}, "owner"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("operator baseline reference error = %v", err)
	}
	third, err := s.CreateRun(suite.ID, 1, RunInput{AgentID: "agent", AgentProfileVersion: 1, ReproducesRunID: valid.ID, Outputs: map[string]string{"fix": "done"}}, "owner")
	if err != nil || !third.Reproducible {
		t.Fatalf("reproducible baseline = %#v, %v", third, err)
	}
}

func TestApprovedTrialBecomesBoundedRevocableParticipation(t *testing.T) {
	s, _ := New(t.TempDir())
	rev := Revision{RepositoryRevision: strings.Repeat("a", 40), Scenarios: []Scenario{{ID: "work", Title: "Work", SanitizedPrompt: "complete fixture", ExpectedOutcomes: []string{"done"}, Checks: []Check{{Name: "done", Kind: "contains", Expected: "done"}}}}, Budget: Budget{1, 1000, 3}, ProhibitedActions: []string{"merge"}, HumanReviewCriteria: []string{"inspect"}, ChangeSummary: "initial", CreatedBy: "owner"}
	suite, _ := s.Create(Suite{OrganizationID: "org", RepositoryID: "repo", Name: "Trial"}, rev)
	run, _ := s.CreateRun(suite.ID, 1, RunInput{AgentID: "agent", AgentProfileVersion: 1, Outputs: map[string]string{"work": "done"}}, "member")
	if _, err := s.CreateParticipation("org", "owner", ParticipationInput{AgentID: "agent", AgentProfileVersion: 1, EvaluationRunID: run.ID}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unevaluated evidence admitted: %v", err)
	}
	if _, err := s.Decide(run.ID, "owner", "approved", "bounded evidence passed"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	p, err := s.CreateParticipation("org", "owner", ParticipationInput{AgentID: "agent", AgentProfileVersion: 1, EvaluationRunID: run.ID, Role: "contributor", Resources: []ParticipationResource{{Kind: "repository", ID: "repo"}}, Actions: []string{"task.write", "workspace.start"}, Budget: ParticipationBudget{MaxCost: 10, MaxAgentMinutes: 60, MaxActions: 20}, StartsAt: now, ExpiresAt: now.Add(time.Hour), DataBoundaries: []string{"repository_content"}, AgreementRequirement: "sponsor", SponsorID: "human"})
	if err != nil || p.Status != "pending_agreement" || p.AuthorityIdentity != "" {
		t.Fatalf("proposal = %#v, %v", p, err)
	}
	p, err = s.AgreeParticipation(p.ID, "human", "sponsor", "I will own consequential decisions.")
	if err != nil || p.Status != "ready" {
		t.Fatalf("agreement = %#v, %v", p, err)
	}
	p, err = s.ActivateParticipation(p.ID, "owner", "agent-participation:"+p.ID, "grant", p.Version)
	if err != nil || p.Status != "active" || p.AccessGrantID != "grant" {
		t.Fatalf("activation = %#v, %v", p, err)
	}
	p, err = s.DecideParticipation(p.ID, "owner", "revoke", p.Version)
	if err != nil || p.Status != "revoked" || len(p.Events) != 4 {
		t.Fatalf("revocation = %#v, %v", p, err)
	}
}

func TestPendingSponsorCanBeReassignedWithHistory(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Now().UTC()
	rev := Revision{RepositoryRevision: strings.Repeat("b", 40), Scenarios: []Scenario{{ID: "work", Title: "Work", SanitizedPrompt: "complete fixture", ExpectedOutcomes: []string{"done"}, Checks: []Check{{Name: "done", Kind: "contains", Expected: "done"}}}}, Budget: Budget{1, 1000, 3}, ProhibitedActions: []string{"merge"}, HumanReviewCriteria: []string{"inspect"}, ChangeSummary: "initial", CreatedBy: "owner"}
	suite, _ := s.Create(Suite{OrganizationID: "org", RepositoryID: "repo", Name: "Trial"}, rev)
	run, _ := s.CreateRun(suite.ID, 1, RunInput{AgentID: "agent", AgentProfileVersion: 1, Outputs: map[string]string{"work": "done"}}, "member")
	_, _ = s.Decide(run.ID, "owner", "approved", "passed")
	p, _ := s.CreateParticipation("org", "owner", ParticipationInput{AgentID: "agent", AgentProfileVersion: 1, EvaluationRunID: run.ID, Role: "viewer", Resources: []ParticipationResource{{Kind: "repository", ID: "repo"}}, Actions: []string{"repository.read"}, Budget: ParticipationBudget{MaxAgentMinutes: 5, MaxActions: 1}, StartsAt: now, ExpiresAt: now.Add(time.Hour), DataBoundaries: []string{"repository_metadata"}, AgreementRequirement: "sponsor", SponsorID: "former"})
	p, err := s.ReassignSponsor(p.ID, "owner", "replacement", p.Version)
	if err != nil || p.SponsorID != "replacement" || p.Version != 2 || p.Events[len(p.Events)-1].Kind != "participation.sponsor_reassigned" {
		t.Fatalf("reassignment = %#v, %v", p, err)
	}
	if _, err = s.ReassignSponsor(p.ID, "owner", "another", 1); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale reassignment = %v", err)
	}
}
