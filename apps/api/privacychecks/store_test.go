package privacychecks

import (
	"strings"
	"testing"
	"time"
)

func TestExactEvidenceAcknowledgementAndScopedExceptionGovernReadiness(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rev := strings.Repeat("a", 40)
	p, err := s.CreatePolicy("repo", "owner", Policy{Branch: "main", Paths: []string{"src/**"}, RequiredRules: []string{"collection", "deletion"}, RequiredJourneys: []string{"erase-account"}, PrivacyOwnerIDs: []string{"privacy"}})
	if err != nil {
		t.Fatal(err)
	}
	r, e := s.Evaluate("repo", rev, "main", []string{"src/app.go"})
	if e != nil || r.Ready {
		t.Fatalf("missing evidence must block: %#v %v", r, e)
	}
	_, err = s.AddRun("repo", "author", Run{PullRequestID: "pull", PreviewID: "preview", Revision: rev, Journey: "erase-account", DataFlowID: "flow", DataFlowVersion: 1, Isolation: "ephemeral_network_none", Results: []Result{{Rule: "collection", Outcome: "passed", Summary: "Only the synthetic account identifier was observed"}, {Rule: "deletion", Outcome: "failed", Summary: "Synthetic record remained after expiry"}}, Artifacts: []Artifact{{Kind: "trace", Name: "deletion.json", Digest: strings.Repeat("b", 64), Summary: "Redacted synthetic deletion timing", SizeBytes: 120}}, Coverage: []string{"collection", "deletion"}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Acknowledge("repo", "privacy", p.ID, rev, "Reviewed sanitized trace and residual deletion risk")
	if err != nil {
		t.Fatal(err)
	}
	r, _ = s.Evaluate("repo", rev, "main", []string{"src/app.go"})
	if r.Ready {
		t.Fatal("failed deletion must still block after owner acknowledgement")
	}
	_, err = s.AddException("repo", "owner", Exception{PolicyID: p.ID, Revision: rev, Rules: []string{"deletion"}, Rationale: "Migration overlap", FollowUpWork: "issue-42", ExpiresAt: time.Now().UTC().Add(24 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	r, _ = s.Evaluate("repo", rev, "main", []string{"src/app.go"})
	if !r.Ready {
		t.Fatalf("scoped exception should unblock only failed rule: %#v", r.Requirements)
	}
	stale, _ := s.Evaluate("repo", strings.Repeat("c", 40), "main", []string{"src/app.go"})
	if stale.Ready {
		t.Fatal("evidence, acknowledgement, and exception must not cross revisions")
	}
}

func TestRejectsProductionDataAndSensitiveArtifactSummaries(t *testing.T) {
	s, _ := New(t.TempDir())
	base := Run{PullRequestID: "p", PreviewID: "v", Revision: strings.Repeat("a", 40), Journey: "j", DataFlowID: "f", DataFlowVersion: 1, Isolation: "ephemeral_network_none", Results: []Result{{Rule: "consent", Outcome: "passed", Summary: "synthetic"}}}
	base.ProductionData = true
	if _, e := s.AddRun("r", "u", base); e != ErrInvalid {
		t.Fatalf("production data accepted: %v", e)
	}
	base.ProductionData = false
	base.Artifacts = []Artifact{{Kind: "log", Name: "x", Digest: strings.Repeat("b", 64), Summary: "Authorization: Bearer secret", SizeBytes: 1}}
	if _, e := s.AddRun("r", "u", base); e != ErrInvalid {
		t.Fatalf("secret-like summary accepted: %v", e)
	}
}
