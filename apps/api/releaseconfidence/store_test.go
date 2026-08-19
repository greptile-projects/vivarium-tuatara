package releaseconfidence

import (
	"testing"
	"time"
)

const revA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const revB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func TestMatrixRetainsExactAttemptsAndInvalidatesOnlyAffectedEvidence(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	reqs := []Requirement{
		{ID: "checkout", Title: "Checkout works in French", Kind: "scenario", ResourceID: "scenario", OwnerIDs: []string{"owner"}, Selector: Selector{Branches: []string{"main"}, Journeys: []string{"checkout"}, Risks: []string{"critical"}, Locales: []string{"fr-FR"}, Platforms: []string{"web"}, Paths: []string{"apps/web/src/checkout"}}},
		{ID: "api", Title: "API remains compatible", Kind: "test", OwnerIDs: []string{"owner"}, Selector: Selector{Branches: []string{"main"}, Paths: []string{"apps/api"}}},
	}
	if _, err = s.Publish("repo", "owner", 0, reqs); err != nil {
		t.Fatal(err)
	}
	for _, a := range []Attempt{
		{RequirementID: "checkout", Revision: revA, Status: "passed", ScenarioID: "scenario", Environment: "preview", Journey: "checkout", RiskClass: "critical", Locale: "fr-FR", Platform: "web", AffectedPaths: []string{"apps/web/src/checkout"}, Summary: "sample passed"},
		{RequirementID: "api", Revision: revA, Status: "passed", CheckRunID: "run", Environment: "linux", AffectedPaths: []string{"apps/api"}, Summary: "tests passed"},
	} {
		if _, err = s.RecordAttempt("repo", "owner", a); err != nil {
			t.Fatal(err)
		}
	}
	m, err := s.Matrix("repo", Target{Kind: "pull", ID: "pull", Revision: revB, Branch: "main", ChangedPaths: []string{"apps/web/src/checkout/page.tsx"}})
	if err != nil {
		t.Fatal(err)
	}
	if m.Ready || len(m.Cells) != 1 || m.Cells[0].State != "gap" || len(m.Cells[0].StaleAttempts) != 1 {
		t.Fatalf("affected matrix = %#v", m)
	}
	m, err = s.Matrix("repo", Target{Kind: "pull", ID: "pull", Revision: revB, Branch: "main", ChangedPaths: []string{"apps/api/handler.go"}})
	if err != nil {
		t.Fatal(err)
	}
	if m.Ready || len(m.Cells) != 1 || m.Cells[0].State != "gap" {
		t.Fatalf("api matrix = %#v", m)
	}
}

func TestOverrideIsOwnerScopedExpiringAndFollowedUp(t *testing.T) {
	s, _ := New(t.TempDir())
	s.now = func() time.Time { return time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC) }
	_, _ = s.Publish("repo", "owner", 0, []Requirement{{ID: "risk", Title: "Risk sign-off", Kind: "exploratory_signoff", OwnerIDs: []string{"owner"}, Selector: Selector{Branches: []string{"main"}}}})
	in := Override{RequirementID: "risk", Revision: revA, Rationale: "Accepted for this candidate only", Scope: Selector{Branches: []string{"main"}}, ExpiresAt: s.now().Add(24 * time.Hour), FollowUpKind: "issue", FollowUpID: "issue-1"}
	if _, err := s.Override("repo", "other", in); err != ErrInvalid {
		t.Fatalf("non-owner override = %v", err)
	}
	if _, err := s.Override("repo", "owner", in); err != nil {
		t.Fatal(err)
	}
	m, err := s.Matrix("repo", Target{Kind: "pull", ID: "pull", Revision: revA, Branch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if !m.Ready || m.Cells[0].State != "overridden" {
		t.Fatalf("matrix = %#v", m)
	}
}
