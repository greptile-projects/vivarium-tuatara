package accessibilitydelivery

import (
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/accessibilityassessments"
)

const repo = "11111111111111111111111111111111"
const owner = "22222222222222222222222222222222"
const evaluator = "33333333333333333333333333333333"
const revision = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestExactCandidateEvidenceAcknowledgementDissentAndOverride(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	p, err := s.CreatePolicy(repo, owner, Policy{Branch: "main", Paths: []string{"src/ui/*"}, RiskClasses: []string{"high"}, RequiredChecks: []string{"axe"}, RequiredScenarios: []string{"checkout-keyboard"}, RequiredRoles: []RoleRequirement{{Role: "accessibility_reviewer", UserIDs: []string{evaluator}, Minimum: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	evidence := []accessibilityassessments.Assessment{{Revision: revision, Checks: []accessibilityassessments.Check{{JourneyID: "checkout-keyboard", Outcome: "passed"}}}}
	readiness, err := s.Evaluate(repo, revision, "main", "pull", "", []string{"src/ui/button.tsx"}, nil, []string{"high"}, map[string]string{"axe": "passed"}, evidence)
	if err != nil || readiness.Ready {
		t.Fatalf("missing acknowledgement must block: %#v, %v", readiness, err)
	}
	inv, err := s.Invite(repo, owner, Invitation{PolicyID: p.ID, PullRequestID: "pull", Revision: revision, PreviewID: "preview", UserID: evaluator, Role: "accessibility_reviewer", AccessNeeds: []string{"keyboard"}, ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Respond(repo, inv.ID, evaluator, "rejected", "Focus order still loses context.", revision); err != nil {
		t.Fatal(err)
	}
	readiness, _ = s.Evaluate(repo, revision, "main", "pull", "", []string{"src/ui/button.tsx"}, nil, []string{"high"}, map[string]string{"axe": "passed"}, evidence)
	if readiness.Ready || len(readiness.Dissent) != 1 {
		t.Fatalf("rejection must remain blocking dissent: %#v", readiness)
	}
	if _, err = s.Override(repo, owner, Override{PolicyID: p.ID, Revision: revision, Rationale: "Critical restoration with a bounded known barrier.", FollowUpWork: "Repair focus order in proposal accessibility-42.", ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	readiness, _ = s.Evaluate(repo, revision, "main", "pull", "", []string{"src/ui/button.tsx"}, nil, []string{"high"}, map[string]string{"axe": "passed"}, evidence)
	if !readiness.Ready || len(readiness.ActiveExceptions) != 1 || len(readiness.Dissent) != 1 {
		t.Fatalf("override must retain dissent and follow-up: %#v", readiness)
	}
	readiness, _ = s.Evaluate(repo, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "main", "pull", "", []string{"src/ui/button.tsx"}, nil, []string{"high"}, map[string]string{"axe": "passed"}, evidence)
	if readiness.Ready {
		t.Fatalf("old assessment and override must not satisfy a new revision: %#v", readiness)
	}
}

func TestPolicySelectionIsExactUnlessWildcarded(t *testing.T) {
	if selected([]string{"src/ui"}, []string{"src/ui-extra/file"}) {
		t.Fatal("plain selectors must not use prefix matching")
	}
	if !selected([]string{"src/ui/*"}, []string{"src/ui/button.tsx"}) {
		t.Fatal("wildcard selector should match")
	}
}

func TestRejectionBlocksSatisfiedRoleMinimumWithoutOverride(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	second := "44444444444444444444444444444444"
	p, err := s.CreatePolicy(repo, owner, Policy{Branch: "main", RequiredRoles: []RoleRequirement{{Role: "accessibility_reviewer", UserIDs: []string{evaluator, second}, Minimum: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, outcome := range []struct{ user, decision string }{{evaluator, "confirmed"}, {second, "rejected"}} {
		inv, inviteErr := s.Invite(repo, owner, Invitation{PolicyID: p.ID, PullRequestID: "pull", Revision: revision, PreviewID: "preview", UserID: outcome.user, Role: "accessibility_reviewer", ExpiresAt: now.Add(time.Hour)})
		if inviteErr != nil {
			t.Fatal(inviteErr)
		}
		if _, err = s.Respond(repo, inv.ID, outcome.user, outcome.decision, "Independent exact-revision result.", revision); err != nil {
			t.Fatal(err)
		}
	}
	readiness, err := s.Evaluate(repo, revision, "main", "pull", "", nil, nil, nil, nil, nil)
	if err != nil || readiness.Ready || len(readiness.Dissent) != 1 || readiness.Requirements[0].Status != "failed" {
		t.Fatalf("rejection must block satisfied minimum: %#v, %v", readiness, err)
	}
}

func TestSameRoleConfirmationCannotSatisfyDisjointNamedRequirement(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	second := "44444444444444444444444444444444"
	p, err := s.CreatePolicy(repo, owner, Policy{Branch: "main", RequiredRoles: []RoleRequirement{
		{Role: "accessibility_reviewer", UserIDs: []string{evaluator}, Minimum: 1},
		{Role: "accessibility_reviewer", UserIDs: []string{second}, Minimum: 1},
	}})
	if err != nil {
		t.Fatal(err)
	}
	inv, err := s.Invite(repo, owner, Invitation{PolicyID: p.ID, PullRequestID: "pull", Revision: revision, PreviewID: "preview", UserID: evaluator, Role: "accessibility_reviewer", ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Respond(repo, inv.ID, evaluator, "confirmed", "Confirmed only by the first named evaluator.", revision); err != nil {
		t.Fatal(err)
	}
	readiness, err := s.Evaluate(repo, revision, "main", "pull", "", nil, nil, nil, nil, nil)
	if err != nil || readiness.Ready || len(readiness.Requirements) != 2 || readiness.Requirements[0].Status != "passed" || readiness.Requirements[1].Status != "missing" {
		t.Fatalf("disjoint named requirement must remain missing: %#v, %v", readiness, err)
	}
}

func TestAcknowledgementCannotCrossPullOrReleaseContext(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	p, err := s.CreatePolicy(repo, owner, Policy{Branch: "main", RequiredRoles: []RoleRequirement{{Role: "accessibility_reviewer", UserIDs: []string{evaluator}, Minimum: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	inv, err := s.Invite(repo, owner, Invitation{PolicyID: p.ID, PullRequestID: "pull-a", Revision: revision, PreviewID: "preview", UserID: evaluator, Role: "accessibility_reviewer", ExpiresAt: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Respond(repo, inv.ID, evaluator, "confirmed", "Confirmed only for pull A.", revision); err != nil {
		t.Fatal(err)
	}
	for _, context := range []struct{ pull, release string }{{pull: "pull-b"}, {release: "release-a"}} {
		readiness, evaluateErr := s.Evaluate(repo, revision, "main", context.pull, context.release, nil, nil, nil, nil, nil)
		if evaluateErr != nil || readiness.Ready || readiness.Requirements[0].Status != "missing" {
			t.Fatalf("pull-a acknowledgement leaked to context %#v: %#v, %v", context, readiness, evaluateErr)
		}
	}
	readiness, err := s.Evaluate(repo, revision, "main", "pull-a", "", nil, nil, nil, nil, nil)
	if err != nil || !readiness.Ready {
		t.Fatalf("acknowledgement must satisfy its own pull: %#v, %v", readiness, err)
	}
}
