package responsealerts

import (
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/responsepolicies"
)

func TestAlertCorrelationDeliveryAndHumanAcknowledgement(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	policy := responsepolicies.Policy{ID: "policy", RepositoryID: "repo", Revisions: []responsepolicies.Revision{{Version: 3, Teams: []responsepolicies.Team{{ID: "ops", MemberIDs: []string{"responder"}}}, Rules: []responsepolicies.Rule{{ID: "critical", ResourceIDs: []string{"service"}, SignalClass: "reliability", Severity: "critical", AccountableTeamID: "ops", AcknowledgeSeconds: 300, ResolveSeconds: 3600, ExpectedActions: []string{"triage"}, CommunicationAudienceIDs: []string{"responder"}, Authority: responsepolicies.AuthorityBoundary{PermittedActions: []string{"investigate"}, ProhibitedActions: []string{"deploy"}}}}}}}
	signal := Signal{SignalClass: "reliability", Severity: "critical", ResourceIDs: []string{"service"}, AffectedUserCount: 12, AffectedUserGroups: []string{"paid"}, Summary: "error budget exhausted", Uncertainty: "impact estimate is sampled", OccurredAt: now, SourceRevision: "commit", CorrelationKey: "service:errors", Evidence: []Evidence{{Kind: "check", ResourceID: "check-1", Revision: "run-2", Digest: "sha256:abc", Summary: "failure output", Available: true}}}
	a, err := s.Create("repo", "source", "request-1", signal, policy, []string{"responder", "responder"})
	if err != nil {
		t.Fatal(err)
	}
	if a.State != "open" || len(a.Routing) != 1 || a.AcknowledgeBy.Sub(now) != 5*time.Minute {
		t.Fatalf("unexpected alert: %#v", a)
	}
	// Transport success is explicitly not response acknowledgement.
	if a.Routing[0].Status != "delivered" || a.State == "acknowledged" {
		t.Fatal("delivery became acknowledgement")
	}
	now = now.Add(time.Minute)
	correlated, err := s.Create("repo", "source", "request-2", signal, policy, []string{"responder"})
	if err != nil {
		t.Fatal(err)
	}
	if correlated.ID != a.ID || correlated.EventCount != 2 || len(correlated.Routing) != 1 {
		t.Fatalf("correlation duplicated alert or page: %#v", correlated)
	}
	retried, err := s.Create("repo", "source", "request-2", signal, policy, []string{"responder"})
	if err != nil || retried.EventCount != 2 {
		t.Fatalf("correlated retry duplicated event: %#v %v", retried, err)
	}
	if _, err = s.Append(a.ID, "ack-1", "acknowledge", "outsider", "", false); err != ErrForbidden {
		t.Fatalf("outsider err=%v", err)
	}
	ack, err := s.Append(a.ID, "ack-1", "acknowledge", "responder", "", true)
	if err != nil {
		t.Fatal(err)
	}
	if ack.State != "acknowledged" {
		t.Fatal("response was not acknowledged")
	}
}

func TestSuppressionAndMissingDeliveryRemainVisible(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	until := now.Add(time.Hour)
	p := responsepolicies.Policy{ID: "p", Revisions: []responsepolicies.Revision{{Version: 1, Rules: []responsepolicies.Rule{{ID: "r", ResourceIDs: []string{"repo"}, SignalClass: "security", Severity: "high", AccountableTeamID: "security", AcknowledgeSeconds: 60, ResolveSeconds: 600}}}}}
	base := Signal{SignalClass: "security", Severity: "high", ResourceIDs: []string{"repo"}, Summary: "secret scanner", Uncertainty: "candidate", OccurredAt: now, SourceRevision: "c", Evidence: []Evidence{{Kind: "scan", ResourceID: "s", Revision: "1", Digest: "d", Summary: "match", Available: false}}}
	base.SuppressedUntil = &until
	a, e := s.Create("repo", "actor", "one", base, p, nil)
	if e != nil {
		t.Fatal(e)
	}
	if a.State != "suppressed" || len(a.Diagnostics) != 2 {
		t.Fatalf("suppression/evidence gap missing: %#v", a)
	}
	base.SuppressedUntil = nil
	b, e := s.Create("repo", "actor", "two", base, p, nil)
	if e != nil {
		t.Fatal(e)
	}
	if b.State != "open" || !contains(b.Diagnostics, "delivery_failed") {
		t.Fatalf("missing delivery hidden: %#v", b)
	}
}

func TestPersistedRetryAndCorrelationBoundaries(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	policy := responsepolicies.Policy{ID: "policy", Revisions: []responsepolicies.Revision{{Version: 1, Rules: []responsepolicies.Rule{{ID: "rule", ResourceIDs: []string{"service"}, SignalClass: "reliability", Severity: "critical", AccountableTeamID: "team-a", AcknowledgeSeconds: 60, ResolveSeconds: 600}}}}}
	signal := Signal{SignalClass: "reliability", Severity: "critical", ResourceIDs: []string{"service"}, Summary: "urgent", Uncertainty: "confirmed", OccurredAt: now, SourceRevision: "one", CorrelationKey: "service-errors", Evidence: []Evidence{{Kind: "check", ResourceID: "check", Revision: "one", Digest: "digest", Summary: "failed", Available: true}}}
	firstStore, _ := New(root)
	firstStore.now = func() time.Time { return now }
	first, err := firstStore.Create("repo", "source", "request-1", signal, policy, []string{"recipient-a"})
	if err != nil {
		t.Fatal(err)
	}
	reopened, _ := New(root)
	reopened.now = func() time.Time { return now }
	retry, err := reopened.Create("repo", "source", "request-1", signal, policy, []string{"recipient-a"})
	if err != nil || retry.ID != first.ID {
		t.Fatalf("persisted retry = %#v, %v", retry, err)
	}
	acknowledged, err := reopened.Append(first.ID, "ack", "acknowledge", "recipient-a", "", true)
	if err != nil || acknowledged.State != "acknowledged" {
		t.Fatal(err)
	}
	now = now.Add(time.Minute)
	signal.OccurredAt, signal.SourceRevision = now, "two"
	later, err := reopened.Create("repo", "source", "request-2", signal, policy, []string{"recipient-a"})
	if err != nil {
		t.Fatal(err)
	}
	if later.ID == first.ID || later.State != "open" || len(later.Routing) != 1 {
		t.Fatalf("non-open alert swallowed later urgency: %#v", later)
	}
	policy.Revisions[0].Version = 2
	policy.Revisions[0].Rules[0].AccountableTeamID = "team-b"
	now = now.Add(time.Minute)
	signal.OccurredAt, signal.SourceRevision = now, "three"
	rerouted, err := reopened.Create("repo", "source", "request-3", signal, policy, []string{"recipient-b"})
	if err != nil {
		t.Fatal(err)
	}
	if rerouted.ID == later.ID || rerouted.PolicyVersion != 2 || rerouted.TeamID != "team-b" || len(rerouted.Routing) != 1 || rerouted.Routing[0].RecipientID != "recipient-b" {
		t.Fatalf("policy movement retained stale routing: %#v", rerouted)
	}
}

func TestClosedCorrelatedRequestRetainsIdempotency(t *testing.T) {
	for _, terminal := range []string{"acknowledge", "resolve"} {
		t.Run(terminal, func(t *testing.T) {
			s, _ := New(t.TempDir())
			now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
			s.now = func() time.Time { return now }
			policy := responsepolicies.Policy{ID: "policy", Revisions: []responsepolicies.Revision{{Version: 1, Rules: []responsepolicies.Rule{{ID: "rule", ResourceIDs: []string{"service"}, SignalClass: "reliability", Severity: "critical", AccountableTeamID: "team", AcknowledgeSeconds: 60, ResolveSeconds: 600}}}}}
			signal := Signal{SignalClass: "reliability", Severity: "critical", ResourceIDs: []string{"service"}, Summary: "urgent", Uncertainty: "confirmed", OccurredAt: now, SourceRevision: "one", CorrelationKey: "errors", Evidence: []Evidence{{Kind: "check", ResourceID: "check", Revision: "one", Digest: "digest", Summary: "failed", Available: true}}}
			parent, err := s.Create("repo", "source", "parent", signal, policy, []string{"responder"})
			if err != nil {
				t.Fatal(err)
			}
			now = now.Add(time.Minute)
			signal.OccurredAt, signal.SourceRevision = now, "two"
			correlated, err := s.Create("repo", "source", "correlated-request", signal, policy, []string{"responder"})
			if err != nil || correlated.ID != parent.ID {
				t.Fatalf("correlation = %#v, %v", correlated, err)
			}
			closed, err := s.Append(parent.ID, "close", terminal, "responder", "", true)
			if err != nil {
				t.Fatal(err)
			}
			retry, err := s.Create("repo", "source", "correlated-request", signal, policy, []string{"responder"})
			if err != nil || retry.ID != parent.ID || retry.State != closed.State || retry.EventCount != 2 {
				t.Fatalf("closed exact retry = %#v, %v", retry, err)
			}
			changed := signal
			changed.Summary = "changed reuse"
			if _, err := s.Create("repo", "source", "correlated-request", changed, policy, []string{"responder"}); err != ErrConflict {
				t.Fatalf("changed closed retry err = %v", err)
			}
			otherActor, err := s.Create("repo", "other-source", "correlated-request", signal, policy, []string{"responder"})
			if err != nil || otherActor.ID == parent.ID {
				t.Fatalf("request identity leaked across actors: %#v, %v", otherActor, err)
			}
		})
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
