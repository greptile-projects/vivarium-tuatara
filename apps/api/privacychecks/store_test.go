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
	r, e := s.Evaluate("repo", rev, "main", "pull", []string{"src/app.go"})
	if e != nil || r.Ready {
		t.Fatalf("missing evidence must block: %#v %v", r, e)
	}
	_, err = s.AddRun("repo", "author", Run{PullRequestID: "pull", PreviewID: "preview", Revision: rev, Journey: "erase-account", DataFlowID: "flow", DataFlowVersion: 1, Isolation: "ephemeral_network_none", Results: []Result{{Rule: "collection", Outcome: "passed", Summary: "Only the synthetic account identifier was observed"}, {Rule: "deletion", Outcome: "failed", Summary: "Synthetic record remained after expiry"}}, Artifacts: []Artifact{{Kind: "trace", Name: "deletion.json", Digest: strings.Repeat("b", 64), Summary: "Redacted synthetic deletion timing", SizeBytes: 120}}, Coverage: []string{"collection", "deletion"}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Acknowledge("repo", "privacy", p.ID, rev, "pull", "Reviewed sanitized trace and residual deletion risk")
	if err != nil {
		t.Fatal(err)
	}
	r, _ = s.Evaluate("repo", rev, "main", "pull", []string{"src/app.go"})
	if r.Ready {
		t.Fatal("failed deletion must still block after owner acknowledgement")
	}
	_, err = s.AddException("repo", "owner", Exception{PolicyID: p.ID, Revision: rev, PullRequestID: "pull", Rules: []string{"deletion"}, Rationale: "Migration overlap", FollowUpWork: "issue-42", ExpiresAt: time.Now().UTC().Add(24 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	r, _ = s.Evaluate("repo", rev, "main", "pull", []string{"src/app.go"})
	if !r.Ready {
		t.Fatalf("scoped exception should unblock only failed rule: %#v", r.Requirements)
	}
	stale, _ := s.Evaluate("repo", strings.Repeat("c", 40), "main", "pull", []string{"src/app.go"})
	if stale.Ready {
		t.Fatal("evidence, acknowledgement, and exception must not cross revisions")
	}
}

func TestRejectsProductionDataAndCanonicalizesAllCallerDisplayText(t *testing.T) {
	root := t.TempDir()
	s, _ := New(root)
	base := Run{PullRequestID: "p", PreviewID: "v", Revision: strings.Repeat("a", 40), Journey: "j", DataFlowID: "f", DataFlowVersion: 1, Isolation: "ephemeral_network_none", Results: []Result{{Rule: "consent", Outcome: "passed", Summary: "synthetic"}}}
	base.ProductionData = true
	if _, e := s.AddRun("r", "u", base); e != ErrInvalid {
		t.Fatalf("production data accepted: %v", e)
	}
	base.ProductionData = false
	base.Artifacts = []Artifact{{Kind: "log", Name: "token: abc123", Digest: strings.Repeat("b", 64), Summary: "Authorization: Bearer secret for Ada Lovelace", SizeBytes: 1}}
	base.Results[0].Summary = "token: abc123; Ada Lovelace; ada@example.com; +1 (415) 555-0100; 123 Main Street"
	created, e := s.AddRun("r", "u", base)
	if e != nil {
		t.Fatal(e)
	}
	if created.Results[0].Summary != "consent passed in isolated synthetic journey" || created.Artifacts[0].Name != "log-1" || created.Artifacts[0].Summary != "sanitized log metadata retained; payload omitted" {
		t.Fatalf("caller display text was retained: %#v", created)
	}
	reopened, _ := New(root)
	readiness, e := reopened.Evaluate("r", base.Revision, "main", "p", nil)
	if e != nil || len(readiness.Runs) != 1 {
		t.Fatalf("durable evidence missing: %#v %v", readiness, e)
	}
	encoded := readiness.Runs[0].Results[0].Summary + readiness.Runs[0].Artifacts[0].Name + readiness.Runs[0].Artifacts[0].Summary
	for _, sensitive := range []string{"abc123", "Ada Lovelace", "example.com", "555-0100", "Main Street", "Bearer"} {
		if strings.Contains(encoded, sensitive) {
			t.Fatalf("sensitive caller text persisted in readiness: %q", encoded)
		}
	}
}

func TestPullReadinessDoesNotReuseSiblingPullEvidenceAtSameRevision(t *testing.T) {
	s, _ := New(t.TempDir())
	revision := strings.Repeat("a", 40)
	policy, err := s.CreatePolicy("repo", "owner", Policy{Branch: "main", RequiredRules: []string{"consent"}, PrivacyOwnerIDs: []string{"privacy"}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.AddRun("repo", "author", Run{PullRequestID: "pull-a", PreviewID: "preview-a", Revision: revision, Journey: "signup", DataFlowID: "flow", DataFlowVersion: 1, Isolation: "ephemeral_network_none", Results: []Result{{Rule: "consent", Outcome: "passed", Summary: "Synthetic subject consent was observed"}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Acknowledge("repo", "privacy", policy.ID, revision, "pull-a", "Reviewed the current sanitized evidence")
	if err != nil {
		t.Fatal(err)
	}
	pullA, _ := s.Evaluate("repo", revision, "main", "pull-a", nil)
	pullB, _ := s.Evaluate("repo", revision, "main", "pull-b", nil)
	if !pullA.Ready {
		t.Fatalf("source pull should be ready: %#v", pullA.Requirements)
	}
	if pullB.Ready || len(pullB.Runs) != 0 {
		t.Fatalf("sibling pull reused evidence: %#v", pullB)
	}
}
