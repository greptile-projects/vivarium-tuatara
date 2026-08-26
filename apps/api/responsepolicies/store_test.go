package responsepolicies

import (
	"errors"
	"testing"
	"time"
)

func fixture(now time.Time) Revision {
	return Revision{Title: "Primary response coverage", Summary: "Who responds before alerts arrive", ChangeReason: "initial coverage", Resources: []Resource{{ID: "repo", Kind: "repository", Name: "project", OwnerTeamIDs: []string{"platform"}}, {ID: "checkout", Kind: "user_journey", Name: "Checkout", OwnerTeamIDs: []string{"commerce"}}}, Teams: []Team{{ID: "platform", Name: "Platform", MemberIDs: []string{"alice"}, Skills: []string{"operations"}, Contact: "#platform"}, {ID: "commerce", Name: "Commerce", MemberIDs: []string{"bob"}, Skills: []string{"security"}, Contact: "#commerce"}}, Rules: []Rule{{ID: "availability-critical", ResourceIDs: []string{"repo"}, SignalClass: "reliability", Severity: "critical", AccountableTeamID: "platform", RequiredSkills: []string{"operations"}, AcknowledgeSeconds: 300, ResolveSeconds: 3600, ExpectedActions: []string{"assess user impact"}, Escalations: []Escalation{{AfterSeconds: 900, TeamID: "commerce", AudienceIDs: []string{"owners"}, ExpectedAction: "assume coordination"}}, CommunicationAudienceIDs: []string{"support"}, IncidentCriteria: []string{"user impact exceeds five minutes"}, Authority: AuthorityBoundary{RequiredAccess: []string{"repository:read"}, PermittedActions: []string{"investigate"}, ProhibitedActions: []string{"deploy"}, PrivacyRuleIDs: []string{"privacy-standard"}, SecurityRuleIDs: []string{"security-standard"}, ContinuityRuleIDs: []string{"continuity-standard"}}}}, Exceptions: []Exception{{ID: "temporary-gap", RuleID: "availability-critical", Reason: "staffing transition", FollowUpID: "task-1", ExpiresAt: now.Add(10 * 24 * time.Hour)}}}
}

func TestVersionedProjectionAndRetry(t *testing.T) {
	now := time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return now }
	r := fixture(now)
	p, err := s.Create("repo", "alice", "create-1", r)
	if err != nil {
		t.Fatal(err)
	}
	if p.CurrentVersion != 1 {
		t.Fatalf("version=%d", p.CurrentVersion)
	}
	kinds := map[string]bool{}
	for _, d := range p.Diagnostics {
		kinds[d.Kind] = true
	}
	if !kinds["uncovered_resource"] || !kinds["expiring_exception"] {
		t.Fatalf("diagnostics=%+v", p.Diagnostics)
	}
	same, err := s.Create("repo", "alice", "create-1", r)
	if err != nil || same.ID != p.ID {
		t.Fatalf("retry: %+v %v", same, err)
	}
	r.Summary = "changed"
	if _, err = s.Create("repo", "alice", "create-1", r); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed retry=%v", err)
	}
	next := fixture(now)
	next.Rules = append(next.Rules, Rule{ID: "journey", ResourceIDs: []string{"checkout"}, SignalClass: "security", Severity: "critical", AccountableTeamID: "platform", RequiredSkills: []string{"security"}, AcknowledgeSeconds: 0, ResolveSeconds: 0, ExpectedActions: []string{"contain"}, CommunicationAudienceIDs: []string{"security"}, IncidentCriteria: []string{"confirmed compromise"}, Authority: AuthorityBoundary{RequiredAccess: []string{"security:read"}, PermittedActions: []string{"investigate"}, ProhibitedActions: []string{"deploy"}}})
	p, err = s.Revise(p.ID, 1, "alice", "rev-1", next)
	if err != nil {
		t.Fatal(err)
	}
	kinds = map[string]bool{}
	for _, d := range p.Diagnostics {
		kinds[d.Kind] = true
	}
	if !kinds["unavailable_skill"] || !kinds["impossible_target"] || !kinds["conflicting_ownership"] {
		t.Fatalf("diagnostics=%+v", p.Diagnostics)
	}
}

func TestRejectsDanglingCoverage(t *testing.T) {
	s, _ := New(t.TempDir())
	r := fixture(time.Now())
	r.Rules[0].ResourceIDs = []string{"missing"}
	if _, err := s.Create("repo", "alice", "x", r); !errors.Is(err, ErrInvalid) {
		t.Fatalf("err=%v", err)
	}
}

func TestRejectsIncompleteAuthorityAndEscalationAudience(t *testing.T) {
	s, _ := New(t.TempDir())
	r := fixture(time.Now())
	r.Rules[0].Authority = AuthorityBoundary{}
	if _, err := s.Create("repo", "alice", "authority", r); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty authority err=%v", err)
	}
	r = fixture(time.Now())
	r.Rules[0].Escalations[0].AudienceIDs = nil
	if _, err := s.Create("repo", "alice", "audience", r); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty escalation audience err=%v", err)
	}
}
