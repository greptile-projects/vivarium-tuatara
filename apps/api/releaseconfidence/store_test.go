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
		{ID: "api", Title: "API remains compatible", Kind: "test", ResourceID: "run", OwnerIDs: []string{"owner"}, Selector: Selector{Branches: []string{"main"}, Paths: []string{"apps/api"}}},
	}
	if _, err = s.Publish("repo", "owner", 0, reqs); err != nil {
		t.Fatal(err)
	}
	for _, a := range []Attempt{
		{RequirementID: "checkout", Revision: revA, Status: "passed", ScenarioID: "scenario", Environment: "preview", Journey: "checkout", RiskClass: "critical", Locale: "fr-FR", Platform: "web", AffectedPaths: []string{"apps/web/src/checkout"}, Summary: "sample passed"},
		{RequirementID: "api", Revision: revA, Status: "passed", CheckRunID: "run", PullRequestID: "pull", Environment: "linux", AffectedPaths: []string{"apps/api"}, Summary: "tests passed"},
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

func TestAttemptMustMatchRequirementKindAndConfiguredResource(t *testing.T) {
	s, _ := New(t.TempDir())
	_, err := s.Publish("repo", "owner", 0, []Requirement{{ID: "required", Title: "Required scenario", Kind: "scenario", ResourceID: "scenario-required", OwnerIDs: []string{"owner"}}})
	if err != nil {
		t.Fatal(err)
	}
	base := Attempt{RequirementID: "required", Revision: revA, Status: "passed", ScenarioID: "scenario-unrelated", Environment: "preview", Summary: "passed"}
	if _, err = s.RecordAttempt("repo", "owner", base); err != ErrInvalid {
		t.Fatalf("unrelated scenario = %v", err)
	}
	base.ScenarioID, base.CheckRunID, base.PullRequestID = "", "scenario-required", "pull"
	if _, err = s.RecordAttempt("repo", "owner", base); err != ErrInvalid {
		t.Fatalf("wrong evidence kind = %v", err)
	}
	base.CheckRunID, base.PullRequestID, base.ScenarioID = "", "", "scenario-required"
	if _, err = s.RecordAttempt("repo", "owner", base); err != nil {
		t.Fatal(err)
	}
}

func TestMatrixIsolatesPullChecksAndReleaseSignalsAtSharedRevision(t *testing.T) {
	s, _ := New(t.TempDir())
	_, err := s.Publish("repo", "owner", 0, []Requirement{
		{ID: "check", Title: "Pull check", Kind: "test", ResourceID: "run", OwnerIDs: []string{"owner"}},
		{ID: "sample", Title: "Release sample", Kind: "scenario", ResourceID: "scenario", OwnerIDs: []string{"owner"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.RecordAttempt("repo", "owner", Attempt{RequirementID: "check", Revision: revA, Status: "passed", CheckRunID: "run", PullRequestID: "pull-b", Environment: "linux", Summary: "passed"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RecordAttempt("repo", "owner", Attempt{RequirementID: "sample", Revision: revA, Status: "passed", ScenarioID: "scenario", TargetKind: "release", TargetID: "release-a", Environment: "production", Summary: "sampled"}); err != nil {
		t.Fatal(err)
	}
	pull, err := s.Matrix("repo", Target{Kind: "pull", ID: "pull-a", Revision: revA})
	if err != nil {
		t.Fatal(err)
	}
	if pull.Ready || pull.Cells[0].State != "gap" || len(pull.Cells[0].StaleAttempts) != 1 {
		t.Fatalf("cross-pull matrix = %#v", pull)
	}
	release, err := s.Matrix("repo", Target{Kind: "release", ID: "release-b", Revision: revA})
	if err != nil {
		t.Fatal(err)
	}
	if release.Ready {
		t.Fatalf("cross-release matrix = %#v", release)
	}
	for _, cell := range release.Cells {
		if cell.Requirement.ID == "sample" && (cell.State != "gap" || len(cell.StaleAttempts) != 1) {
			t.Fatalf("release sample cell = %#v", cell)
		}
	}
}

func TestOverrideIsOwnerScopedExpiringAndFollowedUp(t *testing.T) {
	s, _ := New(t.TempDir())
	s.now = func() time.Time { return time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC) }
	_, _ = s.Publish("repo", "owner", 0, []Requirement{{ID: "risk", Title: "Risk sign-off", Kind: "exploratory_signoff", ResourceID: "session", OwnerIDs: []string{"owner"}, Selector: Selector{Branches: []string{"main"}}}})
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
