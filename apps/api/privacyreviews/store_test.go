package privacyreviews

import (
	"testing"
)

func TestAcceptanceIsRevisionBoundAndRetainsDiscussion(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	revision := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	v, err := s.Create(Review{RepositoryID: "repo", PullRequestID: "pull", SourceRevision: revision, TargetRevision: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", SourceFlowID: "new", SourceFlowVersion: 2, TargetFlowID: "old", TargetFlowVersion: 1, Changes: []Change{{Kind: "collection", Summary: "new collection", SourceIDs: []string{"edge"}}}, Requirements: []Requirement{{Kind: "owner_acknowledgement", Reason: "owner review", Status: "required"}}, CreatedByType: "agent", CreatedBy: "agent-1"})
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.AddComment("repo", "pull", "human", "user-1", Comment{Kind: "challenge", Body: "This is only transient.", FindingKinds: []string{"collection"}})
	if err != nil || len(v.Comments) != 1 {
		t.Fatalf("comment: %#v %v", v, err)
	}
	if _, err = s.Accept("repo", "pull", "cccccccccccccccccccccccccccccccccccccccc", "user-1", "bounded", []string{"owner_acknowledgement"}); err != ErrConflict {
		t.Fatalf("stale acceptance = %v", err)
	}
	v, err = s.Accept("repo", "pull", revision, "user-1", "Mitigation leaves bounded logging risk.", []string{"owner_acknowledgement"})
	if err != nil || v.AcceptedBy != "user-1" || v.Requirements[0].Status != "acknowledged" || len(v.Comments) != 1 {
		t.Fatalf("accepted: %#v %v", v, err)
	}
}
