package designgovernance

import (
	"strings"
	"testing"
	"time"
)

func TestReadinessRequiresCurrentScopedAcceptanceAndReportsExpiry(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	owner, approver, repo, pull := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32), strings.Repeat("4", 32)
	revision := strings.Repeat("a", 40)
	p, err := s.CreatePolicy(Policy{ScopeKind: "repository", ScopeID: repo, Name: "Settings language", Selectors: []Selector{{Kind: "path", Value: "ui/settings"}}, Requirements: []Requirement{{Role: "content", ApproverIDs: []string{approver}}}, ExceptionMaxHours: 24, CreatedBy: owner})
	if err != nil {
		t.Fatal(err)
	}
	blocked, err := s.Evaluate(repo, "", pull, revision, []string{"ui/settings/page.tsx"}, nil, nil, nil, nil)
	if err != nil || blocked.Ready || len(blocked.Diagnostics) != 1 {
		t.Fatalf("expected missing acceptance: %#v %v", blocked, err)
	}
	if _, err = s.Accept(repo, "", Acceptance{PolicyID: p.ID, PolicyVersion: p.Version, PullRequestID: pull, Revision: revision, Role: "content", Decision: "accepted", Rationale: "Current copy reviewed", ActorID: approver}); err != nil {
		t.Fatal(err)
	}
	ready, _ := s.Evaluate(repo, "", pull, revision, []string{"ui/settings/page.tsx"}, nil, nil, nil, nil)
	if !ready.Ready {
		t.Fatalf("expected ready: %#v", ready)
	}
	stale, _ := s.Evaluate(repo, "", pull, strings.Repeat("b", 40), []string{"ui/settings/page.tsx"}, nil, nil, nil, nil)
	if stale.Ready {
		t.Fatal("acceptance survived candidate movement")
	}
	if _, err = s.Except(repo, "", Exception{PolicyID: p.ID, PolicyVersion: p.Version, PullRequestID: pull, Revision: strings.Repeat("b", 40), Reason: "Short translation overlap", ExpiresAt: now.Add(6 * time.Hour), ActorID: owner}); err != nil {
		t.Fatal(err)
	}
	excepted, _ := s.Evaluate(repo, "", pull, strings.Repeat("b", 40), []string{"ui/settings/page.tsx"}, nil, nil, nil, nil)
	if !excepted.Ready || len(excepted.ActiveExceptions) != 1 || excepted.Diagnostics[0].Kind != "expiring_exception" {
		t.Fatalf("expected explicit expiring exception: %#v", excepted)
	}
}

func TestOrganizationPolicyAppliesWithoutGrantingApprovalToRepositoryOwner(t *testing.T) {
	s, _ := New(t.TempDir())
	org, repo, orgOwner, repoOwner, invited := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32), strings.Repeat("4", 32), strings.Repeat("5", 32)
	p, err := s.CreatePolicy(Policy{ScopeKind: "organization", ScopeID: org, Name: "Critical journeys", Selectors: []Selector{{Kind: "journey", Value: "checkout"}}, Requirements: []Requirement{{Role: "invited_user", ApproverIDs: []string{invited}}}, ExceptionMaxHours: 24, CreatedBy: orgOwner})
	if err != nil {
		t.Fatal(err)
	}
	bad := Acceptance{PolicyID: p.ID, PolicyVersion: 1, PullRequestID: strings.Repeat("6", 32), Revision: strings.Repeat("a", 40), Role: "invited_user", Decision: "accepted", Rationale: "looks good", ActorID: repoOwner}
	if _, err = s.Accept(repo, org, bad); err != ErrInvalid {
		t.Fatalf("repository owner gained invited-user authority: %v", err)
	}
}
