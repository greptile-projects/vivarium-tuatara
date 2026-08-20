package agentpilots

import (
	"errors"
	"testing"
	"time"
)

func fixture(now time.Time) Pilot {
	return Pilot{RepositoryID: "repo", PullRequestID: "pull", CandidateID: "candidate", CandidateRevision: "0123456789abcdef0123456789abcdef01234567", OwnerID: "owner", RepositoryIDs: []string{"repo"}, Roles: []string{"reviewer"}, TaskKinds: []string{"issue"}, Actions: []string{"repository.read", "draft.create"}, Budget: Budget{MaxMinutes: 10, MaxActions: 2, MaxCost: 5}, StartsAt: now, ExpiresAt: now.Add(24 * time.Hour), Invitations: []Invitation{{ParticipantID: "user", Role: "reviewer", RepositoryIDs: []string{"repo"}, TaskKinds: []string{"issue"}, Actions: []string{"repository.read", "draft.create"}}}}
}

func TestPilotConsentDelegationPolicyAndFeedback(t *testing.T) {
	now := time.Date(2026, 8, 20, 20, 0, 0, 0, time.UTC)
	s, e := New(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	s.now = func() time.Time { return now }
	p, e := s.Create(fixture(now))
	if e != nil {
		t.Fatal(e)
	}
	if got := s.Effective(p, "user", false, false); got.Status != "paused" || got.PauseReasons[0] != "consent_pending" {
		t.Fatalf("pre-consent access = %#v", got)
	}
	p, e = s.Consent(p.ID, "user", p.Version, false)
	if e != nil {
		t.Fatal(e)
	}
	p, e = s.StartSession(p.ID, "user", p.Version, Session{RepositoryID: "repo", TaskKind: "issue", TaskID: "42", ExpectedOutcome: "draft a reproducible correction"}, false, false)
	if e != nil {
		t.Fatal(e)
	}
	sid := p.Sessions[0].ID
	p, e = s.AppendEvent(p.ID, "user", p.Version, sid, SessionEvent{Kind: "guidance", Summary: "keep the existing API contract"}, false, false)
	if e != nil {
		t.Fatal(e)
	}
	p, e = s.AppendEvent(p.ID, "user", p.Version, sid, SessionEvent{Kind: "result", Summary: "attempted merge", Action: "repository.merge", Cost: 2, Minutes: 3}, false, false)
	if e != nil {
		t.Fatal(e)
	}
	last := p.Sessions[0].Events[1]
	if last.Kind != "policy_denial" || last.Cost != 0 {
		t.Fatalf("authoritative action = %#v", last)
	}
	p, e = s.AddFeedback(p.ID, "user", p.Version, Feedback{CandidateRevision: p.CandidateRevision, SessionID: sid, Outcome: "draft missed retry behavior", ExpectedOutcome: "idempotent retry", Correction: "reuse the request key"})
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Feedback) != 1 || len(p.Sessions[0].Events) != 2 {
		t.Fatalf("retained pilot evidence = %#v", p)
	}
}

func TestPilotPausesWithoutDiscardingEvidence(t *testing.T) {
	now := time.Date(2026, 8, 20, 20, 0, 0, 0, time.UTC)
	s, _ := New(t.TempDir())
	s.now = func() time.Time { return now }
	p, _ := s.Create(fixture(now))
	p, _ = s.Consent(p.ID, "user", p.Version, false)
	p, _ = s.StartSession(p.ID, "user", p.Version, Session{RepositoryID: "repo", TaskKind: "issue", TaskID: "7", ExpectedOutcome: "safe draft"}, false, false)
	sid := p.Sessions[0].ID
	p, e := s.AppendEvent(p.ID, "user", p.Version, sid, SessionEvent{Kind: "result", Summary: "unsafe output stopped", Action: "draft.create", Cost: 1, Minutes: 1, Unsafe: true}, false, false)
	if e != nil {
		t.Fatal(e)
	}
	if !p.Paused || p.PauseReason != "unsafe_behavior" || len(p.Sessions[0].Events) != 1 {
		t.Fatalf("unsafe pause = %#v", p)
	}
	if _, e = s.StartSession(p.ID, "user", p.Version, Session{RepositoryID: "repo", TaskKind: "issue", TaskID: "8", ExpectedOutcome: "more"}, false, false); !errors.Is(e, ErrDenied) {
		t.Fatalf("paused delegation = %v", e)
	}
	if got := s.Effective(p, "user", true, false); got.Status != "paused" || !contains(got.PauseReasons, "candidate_changed") {
		t.Fatalf("changed candidate = %#v", got)
	}
	p, e = s.Consent(p.ID, "user", p.Version, true)
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Sessions[0].Events) != 1 || p.Invitations[0].RevokedAt == nil {
		t.Fatalf("revocation discarded evidence = %#v", p)
	}
}

func contains(v []string, w string) bool {
	for _, x := range v {
		if x == w {
			return true
		}
	}
	return false
}
