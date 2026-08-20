package assuranceprograms

import (
	"testing"
	"time"
)

func complete(now time.Time) Revision {
	return Revision{Title: "Payment assurance", Summary: "Project obligations", OwnerIDs: []string{"owner"}, ReviewPeriodDays: 90, Requirements: []Requirement{{ID: "req", Kind: "regulatory", Authority: "Regulator", Citation: "Article 5", Title: "Protect data", Summary: "Protect customer data", Applicability: "All payment changes", OwnerIDs: []string{"owner"}, Interpretation: "Encryption is required", ConflictsWith: []string{}}}, Scopes: []Scope{{ID: "repo", Kind: "repository", ResourceID: "repository", Revision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Path: "apps/api", Description: "Payment API"}}, Controls: []Control{{ID: "control", Title: "Encryption", Objective: "Protect stored data", RequirementIDs: []string{"req"}, OwnerIDs: []string{"owner"}, ReviewPeriodDays: 30, Mappings: []Mapping{{ScopeID: "repo", Purpose: "Implements encryption"}}, EvidenceCriteria: []EvidenceCriterion{{ID: "check", Description: "Encryption check passes", Kind: "automated", ResourceKind: "check_run", ResourceID: "security"}}, Claim: "Storage uses managed encryption"}}, Exceptions: []Exception{{ID: "exception", RequirementIDs: []string{"req"}, Rationale: "Legacy migration", GrantedBy: "owner", ExpiresAt: now.Add(6 * 24 * time.Hour), FollowUp: "issue-1"}}}
}
func TestVersioningAndDiagnostics(t *testing.T) {
	s, e := New(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	now := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	p, e := s.Create("repository", "author", complete(now))
	if e != nil {
		t.Fatal(e)
	}
	if p.CurrentVersion != 1 || len(p.Diagnostics) != 1 || p.Diagnostics[0].Kind != "expiring_exception" {
		t.Fatalf("unexpected projection: %#v", p)
	}
	r := complete(now)
	r.Requirements[0].InheritedFrom = "organization:payments"
	p, e = s.Revise(p.ID, 1, "author", r)
	if e != nil {
		t.Fatal(e)
	}
	if p.CurrentVersion != 2 || len(p.Revisions) != 2 || len(p.Diagnostics) != 2 {
		t.Fatalf("unexpected revision: %#v", p)
	}
}
func TestRejectsBrokenReferences(t *testing.T) {
	s, _ := New(t.TempDir())
	r := complete(time.Now())
	r.Controls[0].Mappings[0].ScopeID = "missing"
	if _, e := s.Create("repository", "actor", r); e != ErrInvalid {
		t.Fatalf("got %v", e)
	}
}

func TestRejectsRepositoryScopeForAnotherResource(t *testing.T) {
	s, _ := New(t.TempDir())
	r := complete(time.Now())
	r.Scopes[0].ResourceID = "other-repository"
	if _, e := s.Create("repository", "actor", r); e != ErrInvalid {
		t.Fatalf("got %v", e)
	}
}
