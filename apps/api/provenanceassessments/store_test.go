package provenanceassessments

import (
	"strings"
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

func TestCandidateOrGraphStalenessAlwaysClearsReadiness(t *testing.T) {
	s, _ := New(t.TempDir())
	a, err := s.Create(Assessment{RequestID: "warning", RepositoryID: "repo", Candidate: Candidate{Kind: "release_candidate", ID: "release", Revision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, GraphID: "graph", GraphDigest: "digest", PolicyID: "policy", PolicyVersion: 1, CreatedBy: "owner", Findings: []Finding{{ID: "generated", Kind: "generated_output", Severity: "warning", MaterialKind: "generated_code", Summary: "review generator"}}})
	if err != nil {
		t.Fatal(err)
	}
	matching := Current{CandidateRevision: a.Candidate.Revision, GraphDigest: a.GraphDigest}
	if a, err = s.Get("repo", a.ID, matching); err != nil || !a.Ready || a.Stale {
		t.Fatalf("matching warning assessment = %#v, %v", a, err)
	}
	for name, current := range map[string]Current{"candidate": {CandidateRevision: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", GraphDigest: a.GraphDigest}, "graph": {CandidateRevision: a.Candidate.Revision, GraphDigest: "changed"}} {
		projected, getErr := s.Get("repo", a.ID, current)
		if getErr != nil || projected.Ready || !projected.Stale {
			t.Fatalf("%s stale assessment = %#v, %v", name, projected, getErr)
		}
	}
}

func TestOwnerLinksCleanRoomRepairWithFrozenContext(t *testing.T) {
	s, _ := New(t.TempDir())
	revision := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	a, err := s.Create(Assessment{RequestID: "assessment", RepositoryID: "repo", Candidate: Candidate{Kind: "pull_request", ID: "pull", Revision: revision}, GraphID: "graph", GraphDigest: "digest", PolicyID: "policy", PolicyVersion: 3, CreatedBy: "author", Findings: []Finding{{ID: "origin", Kind: "unattributed_material", Severity: "blocking", MaterialKind: "source", Summary: "origin missing", Obligations: []string{"preserve notice"}, PolicyRuleDigest: "rule"}}})
	if err != nil {
		t.Fatal(err)
	}
	current := Current{CandidateRevision: revision, GraphDigest: "digest", PolicyVersion: 3, OwnerIDs: map[string]bool{"owner": true}, DependencyRevisions: map[string]string{}, ToolRevisions: map[string]string{}, PolicyRuleDigests: map[string]string{"source": "rule"}}
	repair := Repair{RequestID: "repair", WorkDigest: strings.Repeat("d", 64), FindingID: "origin", AffectedRevision: revision, Strategy: "reimplement", PermittedEvidence: []EvidenceReference{{Kind: "specification", ResourceID: "public-api", Revision: revision, Access: "repository"}}, AcceptanceCriteria: []string{"replacement has independently attributable origin"}, CleanRoom: true, ProposalID: "proposal", TaskIDs: []string{"task"}}
	out, err := s.LinkRepair("repo", a.ID, "owner", a.Version, repair, current)
	if err != nil {
		t.Fatal(err)
	}
	got := out.Repairs[0]
	if got.PolicyID != "policy" || got.PolicyVersion != 3 || got.PolicyRuleDigest != "rule" || got.AuthorizedBy != "owner" || len(got.Obligations) != 1 || got.CreatedAt.IsZero() {
		t.Fatalf("repair did not freeze context: %#v", got)
	}
	if replay, replayErr := s.LinkRepair("repo", a.ID, "owner", a.Version, repair, current); replayErr != nil || replay.Version != out.Version {
		t.Fatalf("exact stale-version retry did not reconcile: %#v, %v", replay, replayErr)
	}
	changed := repair
	changed.WorkDigest = strings.Repeat("c", 64)
	if _, changedErr := s.LinkRepair("repo", a.ID, "owner", out.Version, changed, current); changedErr != ErrConflict {
		t.Fatalf("changed request identity reuse = %v", changedErr)
	}
	_, err = s.LinkRepair("repo", a.ID, "owner", out.Version, Repair{RequestID: "restricted", WorkDigest: strings.Repeat("e", 64), FindingID: "origin", AffectedRevision: revision, Strategy: "reimplement", PermittedEvidence: []EvidenceReference{{Kind: "source", ResourceID: "restricted-origin", Revision: revision, Access: "restricted"}}, AcceptanceCriteria: []string{"clean"}, CleanRoom: true, ProposalID: "proposal-2", TaskIDs: []string{"task-2"}}, current)
	if err != ErrInvalid {
		t.Fatalf("clean room admitted restricted evidence: %v", err)
	}
	restricted, err := s.LinkRepair("repo", a.ID, "owner", out.Version, Repair{RequestID: "governed", WorkDigest: strings.Repeat("f", 64), FindingID: "origin", AffectedRevision: revision, Strategy: "obtain_permission", PermittedEvidence: []EvidenceReference{{Kind: "legal_record", ResourceID: "confidential-case-123", Revision: revision, Access: "restricted"}}, AcceptanceCriteria: []string{"permission retained"}, ProposalID: "proposal-4", TaskIDs: []string{"task-4"}}, current)
	if err != nil || restricted.Repairs[1].PermittedEvidence[0].ResourceID == "confidential-case-123" || !strings.HasPrefix(restricted.Repairs[1].PermittedEvidence[0].ResourceID, "restricted:") {
		t.Fatalf("restricted evidence identity was disclosed: %#v, %v", restricted.Repairs, err)
	}
	_, err = s.LinkRepair("repo", a.ID, "reviewer", restricted.Version, Repair{RequestID: "unauthorized", WorkDigest: strings.Repeat("a", 64), FindingID: "origin", AffectedRevision: revision, Strategy: "remove", AcceptanceCriteria: []string{"removed"}, ProposalID: "proposal-3", TaskIDs: []string{"task-3"}}, current)
	if err != ErrForbidden {
		t.Fatalf("reviewer gained repair authority: %v", err)
	}
}
