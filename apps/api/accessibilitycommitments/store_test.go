package accessibilitycommitments

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func validRevision(now time.Time) Revision {
	return Revision{Title: "Accessible checkout", Summary: "Checkout works without sight or pointer input", Subject: Subject{Kind: "documented_journey", ResourceID: "docs-checkout", Name: "Checkout"}, Standards: []Standard{{Name: "WCAG", Version: "2.2", Level: "AA", Criteria: []string{"2.1.1", "4.1.2"}}}, AssistiveTechnologies: []AssistiveTechnology{{ID: "screen-reader", Name: "NVDA", Version: "2026.1", Input: "keyboard and speech", EnvironmentIDs: []string{"windows-chrome"}}}, Audiences: []Audience{{ID: "blind-keyboard", Name: "Blind keyboard users", AccessNeeds: []string{"non-visual output", "keyboard input"}}}, Environments: []Environment{{ID: "windows-chrome", Browser: "Chrome", BrowserVersion: "stable", OS: "Windows 11", Device: "desktop", Supported: true}}, Scenarios: []Scenario{{ID: "complete-checkout", Name: "Complete checkout", Steps: []string{"Open cart", "Submit order"}, ExpectedOutcome: "Confirmation is announced", StandardCriteria: []string{"2.1.1", "4.1.2"}, AudienceIDs: []string{"blind-keyboard"}, TechnologyIDs: []string{"screen-reader"}, EnvironmentIDs: []string{"windows-chrome"}, OwnerIDs: []string{"alice"}}}, SeverityPolicy: []SeverityRule{{Severity: "critical", Definition: "Journey cannot complete", Response: "Block release", ResolutionDays: 1}}, OwnerIDs: []string{"alice"}, Requirements: []Requirement{{ID: "keyboard", Statement: "All controls work by keyboard"}}, Exceptions: []Exception{{ID: "legacy-date", Scope: "date picker", Reason: "replacement underway", ApprovedBy: "maintainer", ExpiresAt: now.Add(20 * 24 * time.Hour), Mitigation: "text input"}}, Links: []Link{{Kind: "roadmap_outcome", ResourceID: "outcome-1", Label: "Inclusive checkout"}, {Kind: "documentation", ResourceID: "docs-checkout", Label: "Checkout journey"}, {Kind: "preview", ResourceID: "preview-1", Label: "Current preview"}, {Kind: "release_policy", ResourceID: "policy-1", Label: "AA release gate"}}, Rationale: "Initial contract"}
}

func TestVersioningAndExplicitDiagnostics(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	first, err := s.Create("repo", "alice", validRevision(now))
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Diagnostics) != 1 || first.Diagnostics[0].Kind != "expiring_exception" || first.Revisions[0].Links[0].AddedBy != "alice" {
		t.Fatalf("unexpected projection: %+v", first)
	}
	next := validRevision(now)
	next.Environments[0].Supported = false
	next.Scenarios[0].OwnerIDs = nil
	next.Requirements = append(next.Requirements, Requirement{ID: "pointer-only", Statement: "Use drag gestures", ConflictsWith: []string{"keyboard"}})
	next.Exceptions[0].ExpiresAt = now.Add(-time.Hour)
	updated, err := s.Revise(first.ID, 1, "bob", next)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	for _, d := range updated.Diagnostics {
		kinds[d.Kind] = true
	}
	for _, kind := range []string{"missing_coverage", "unsupported_environment", "conflicting_requirement", "expired_exception"} {
		if !kinds[kind] {
			t.Fatalf("missing %s: %+v", kind, updated.Diagnostics)
		}
	}
	if _, err = s.Revise(first.ID, 1, "alice", next); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
}
func TestRejectsIncompleteContract(t *testing.T) {
	s, _ := New(t.TempDir())
	r := validRevision(time.Now())
	r.Standards = nil
	if _, err := s.Create("repo", "alice", r); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected invalid, got %v", err)
	}
}

func TestRejectsInvalidOrUndeclaredStandardCriteria(t *testing.T) {
	s, _ := New(t.TempDir())
	r := validRevision(time.Now())
	r.Standards[0] = Standard{Name: " ", Criteria: []string{" "}}
	if _, err := s.Create("repo", "alice", r); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected blank standard to be invalid, got %v", err)
	}
	r = validRevision(time.Now())
	r.Scenarios[0].StandardCriteria = []string{"9.9.9"}
	if _, err := s.Create("repo", "alice", r); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected undeclared criterion to be invalid, got %v", err)
	}
}

func TestListIsolatesCorruptRecords(t *testing.T) {
	root := t.TempDir()
	s, _ := New(root)
	want, err := s.Create("repo-target", "alice", validRevision(time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.Create("repo-other", "alice", validRevision(time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(s.repositoryDir("repo-other"), other.ID+".json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	values, err := s.List("repo-target")
	if err != nil || len(values) != 1 || values[0].ID != want.ID {
		t.Fatalf("healthy repository list = %+v, %v", values, err)
	}
}

func TestListSurfacesCorruptRecordForRequestedRepository(t *testing.T) {
	s, _ := New(t.TempDir())
	value, err := s.Create("repo-target", "alice", validRevision(time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(s.repositoryDir("repo-target"), value.ID+".json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.List("repo-target"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected target corruption to be explicit, got %v", err)
	}
}
