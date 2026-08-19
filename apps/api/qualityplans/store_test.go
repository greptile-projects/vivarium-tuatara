package qualityplans

import (
	"errors"
	"testing"
	"time"
)

func TestVersionedQualityPlanKeepsExplicitCollaborativeGaps(t *testing.T) {
	store, _ := New(t.TempDir())
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	revision := completeRevision()
	revision.OwnerIDs = nil
	revision.Requirements[0].OwnerIDs = nil
	revision.Requirements[0].JudgeIDs = nil
	revision.Requirements[0].Verification = ""
	revision.Requirements[0].ConflictsWith = []string{"privacy-copy"}
	revision.Requirements[0].EvidenceIDs = []string{"check"}
	revision.Evidence[0].Status = "missing"
	revision.Exceptions = []Exception{{ID: "temporary", RequirementID: "checkout", Rationale: "Platform unavailable", GrantedBy: "owner", ExpiresAt: now.Add(48 * time.Hour), FollowUp: "issue-9"}}
	created, err := store.Create("repo", "author", revision)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"missing_ownership": false, "untestable_claim": false, "contradictory_expectation": false, "missing_evidence": false, "expiring_exception": false}
	for _, diagnostic := range created.Diagnostics {
		if _, ok := want[diagnostic.Kind]; ok {
			want[diagnostic.Kind] = true
		}
		if diagnostic.AttributedTo == "" {
			t.Fatalf("unattributed diagnostic: %#v", diagnostic)
		}
	}
	for kind, found := range want {
		if !found {
			t.Errorf("missing diagnostic %s: %#v", kind, created.Diagnostics)
		}
	}

	revision = completeRevision()
	revised, err := store.Revise(created.ID, 1, "reviewer", revision)
	if err != nil || revised.CurrentVersion != 2 || len(revised.Revisions) != 2 || len(revised.Diagnostics) != 1 || revised.Diagnostics[0].Kind != "missing_evidence" {
		t.Fatalf("revised = %#v, %v", revised, err)
	}
	if got := revised.Revisions[1].Evidence[0].AddedBy; got != "reviewer" {
		t.Fatalf("changed evidence added_by = %q, want reviewer", got)
	}
	revision.Evidence = append(revision.Evidence, Evidence{ID: "manual", Kind: "manual", ResourceKind: "sign_off", ResourceID: "signoff-1", Summary: "Release review", Status: "passing"})
	revised, err = store.Revise(created.ID, 2, "reviewer", revision)
	if err != nil || revised.Revisions[2].Evidence[1].AddedBy != "reviewer" {
		t.Fatalf("new evidence provenance = %#v, %v", revised.Revisions[2].Evidence, err)
	}
	revision.Evidence[0].ResourceID = "run-2"
	revision.Evidence[0].Revision = "def"
	revision.Evidence[0].Summary = "Updated checkout suite"
	revision.Evidence[0].Status = "missing"
	revised, err = store.Revise(created.ID, 3, "reviewer", revision)
	if err != nil || revised.Revisions[3].Evidence[0].AddedBy != "reviewer" {
		t.Fatalf("changed evidence provenance = %#v, %v", revised.Revisions[3].Evidence, err)
	}
	changedDiagnostic := false
	for _, diagnostic := range revised.Diagnostics {
		if diagnostic.RequirementID == "checkout" && diagnostic.Kind == "missing_evidence" && diagnostic.AttributedTo == "reviewer" {
			changedDiagnostic = true
		}
	}
	if !changedDiagnostic {
		t.Fatalf("changed evidence diagnostic = %#v", revised.Diagnostics)
	}
	if _, err = store.Revise(created.ID, 1, "reviewer", revision); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale revise = %v", err)
	}
}

func TestUnchangedEvidenceRetainsOriginalContributor(t *testing.T) {
	store, _ := New(t.TempDir())
	revision := completeRevision()
	created, err := store.Create("repo", "author", revision)
	if err != nil {
		t.Fatal(err)
	}
	revised, err := store.Revise(created.ID, 1, "reviewer", revision)
	if err != nil {
		t.Fatal(err)
	}
	if got := revised.Revisions[1].Evidence[0].AddedBy; got != "author" {
		t.Fatalf("unchanged evidence added_by = %q, want author", got)
	}
}

func TestQualityPlanRejectsDanglingTraceability(t *testing.T) {
	store, _ := New(t.TempDir())
	r := completeRevision()
	r.Requirements[0].EvidenceIDs = []string{"unknown"}
	if _, err := store.Create("repo", "owner", r); !errors.Is(err, ErrInvalid) {
		t.Fatalf("error = %v", err)
	}
}

func TestQualityPlanScopeSourceRevisionMustBeExact(t *testing.T) {
	store, _ := New(t.TempDir())
	revision := completeRevision()
	revision.Scopes[0].SourceRevision = "not-a-commit"
	if _, err := store.Create("repo", "owner", revision); !errors.Is(err, ErrInvalid) {
		t.Fatalf("error = %v", err)
	}
}

func completeRevision() Revision {
	return Revision{Title: "Checkout quality", Summary: "Protect checkout", Scopes: []Scope{{Kind: "journey", ResourceID: "checkout", Name: "Checkout"}}, Environments: []Environment{{ID: "chrome", Name: "Chrome", Description: "Current stable", Supported: true}}, OwnerIDs: []string{"owner"}, ReviewSchedule: "Every release", Rationale: "Initial plan", Requirements: []Requirement{{ID: "checkout", SourceKind: "issue", SourceID: "issue-1", Title: "Complete checkout", Rationale: "Revenue journey", ExpectedBehavior: "A valid card completes once", Risk: "critical", TestLevels: []string{"unit", "end_to_end", "manual"}, RepresentativeData: "Synthetic card token", CoverageGoal: "All payment outcomes", OwnerIDs: []string{"owner"}, JudgeIDs: []string{"owner"}, EnvironmentIDs: []string{"chrome"}, Schedule: "Every candidate", ReleaseThreshold: "All required evidence passes", EvidenceIDs: []string{"check"}, Verification: "Observe one order and one charge"}, {ID: "privacy-copy", SourceKind: "privacy", SourceID: "decision-2", Title: "Redacted error", Rationale: "Protect users", ExpectedBehavior: "Errors omit card data", Risk: "high", TestLevels: []string{"integration"}, RepresentativeData: "Synthetic token", CoverageGoal: "All decline reasons", OwnerIDs: []string{"owner"}, JudgeIDs: []string{"owner"}, EnvironmentIDs: []string{"chrome"}, Schedule: "Every candidate", ReleaseThreshold: "No disclosure", Verification: "Inspect sanitized response"}}, Evidence: []Evidence{{ID: "check", Kind: "automated", ResourceKind: "check_run", ResourceID: "run-1", Revision: "abc", Summary: "Checkout suite", Status: "passing"}}}
}
