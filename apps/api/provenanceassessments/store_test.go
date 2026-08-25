package provenanceassessments

import (
	"testing"
	"time"
)

func TestAssessmentSelectivelyInvalidatesAndRequiresOwnerDecision(t *testing.T) {
	s, _ := New(t.TempDir())
	a, err := s.Create(Assessment{RequestID: "request", RepositoryID: "repo", Candidate: Candidate{Kind: "pull_request", ID: "pull", Revision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, GraphID: "graph", GraphDigest: "digest", PolicyID: "policy", PolicyVersion: 1, CreatedBy: "author", Findings: []Finding{{ID: "dependency-license", Kind: "incompatible_license", Severity: "blocking", MaterialKind: "package", NodeID: "dep", Summary: "incompatible", DependencyRevision: "v1", PolicyRuleDigest: "rule-a"}, {ID: "notice", Kind: "required_notice", Severity: "blocking", MaterialKind: "source", NodeID: "file", Summary: "notice", PolicyRuleDigest: "rule-b"}}})
	if err != nil {
		t.Fatal(err)
	}
	current := Current{CandidateRevision: a.Candidate.Revision, GraphDigest: "digest", PolicyVersion: 2, DependencyRevisions: map[string]string{"dep": "v1"}, ToolRevisions: map[string]string{}, PolicyRuleDigests: map[string]string{"package": "rule-a", "source": "rule-b"}, OwnerIDs: map[string]bool{"owner": true}}
	if a, err = s.AddEvent("repo", a.ID, "agent", "agent", 1, Event{RequestID: "challenge", Kind: "challenge", FindingID: "dependency-license", Body: "scanner match is uncertain", Citations: []Citation{{Kind: "file", ResourceID: "evidence", Revision: a.Candidate.Revision, Summary: "exact lines"}}}, current); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AddEvent("repo", a.ID, "agent", "agent", 2, Event{RequestID: "forged", Kind: "acknowledgement", FindingID: "dependency-license", Body: "approve"}, current); err != ErrForbidden {
		t.Fatalf("agent acknowledgement = %v", err)
	}
	if a, err = s.AddEvent("repo", a.ID, "owner", "human", 2, Event{RequestID: "ack", Kind: "acknowledgement", FindingID: "dependency-license", Body: "reviewed exact terms"}, current); err != nil {
		t.Fatal(err)
	}
	expires := time.Now().UTC().Add(7 * 24 * time.Hour)
	if a, err = s.AddEvent("repo", a.ID, "owner", "human", 3, Event{RequestID: "exception", Kind: "exception", FindingID: "notice", Body: "bounded internal candidate", ExceptionExpiresAt: &expires, FollowUp: "ship notice before public release"}, current); err != nil {
		t.Fatal(err)
	}
	if !a.Ready || a.Stale {
		t.Fatalf("current decisions = %#v", a)
	}
	current.DependencyRevisions["dep"] = "v2"
	a, err = s.Get("repo", a.ID, current)
	if err != nil {
		t.Fatal(err)
	}
	if a.Ready || !a.Stale || a.Findings[1].Current == false || a.Findings[1].Resolved == false {
		t.Fatalf("selective invalidation = %#v", a)
	}
}
