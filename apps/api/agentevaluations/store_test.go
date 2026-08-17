package agentevaluations

import (
	"strings"
	"testing"
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
	if !run.Contaminated || run.TrialLabel != "initial" || run.Authority.Publish {
		t.Fatalf("unexpected run %#v", run)
	}
	again, _ := s.CreateRun(suite.ID, 1, RunInput{AgentID: "agent", AgentProfileVersion: 1, Outputs: map[string]string{"fix": "works"}, ToolActions: []ToolAction{{Tool: "git", Action: "publish"}}, Cost: 2, LatencyMS: 20}, "owner")
	if again.TrialLabel != "repeated" || again.PolicyPassed || again.BudgetPassed {
		t.Fatalf("bad repeated classification %#v", again)
	}
	got, _ := s.GetRun(run.ID)
	for _, r := range got.CheckResults {
		if r.Hidden && r.Name != "protected criterion" {
			t.Fatal("hidden name leaked")
		}
	}
}
