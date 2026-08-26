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
func contains(v []string, w string) bool {
	for _, x := range v {
		if x == w {
			return true
		}
	}
	return false
}
