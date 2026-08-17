package supportsolutions

import (
	"errors"
	"testing"
)

func fixture() Solution {
	return Solution{RepositoryID: "repo", ThreadID: "thread", AnswerID: "answer", AnswerRevisionID: "revision", VerificationAttemptID: "attempt", Title: "Retry uploads safely", Summary: "Resume after a timeout without duplicating bytes.", Instructions: "Reuse the upload idempotency key.", Audience: "public", ApplicableVersions: []string{"2.1", "2.2"}, Limitations: []string{"Does not apply before 2.1."}, Links: []Link{{Kind: "search", Label: "Upload retries"}}, Credits: []Credit{{ActorID: "asker", Role: "asker"}, {ActorID: "maintainer", Role: "answer_author"}}}
}

func TestLifecyclePreservesPublishedEvidenceAndCredits(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	v, err := s.Create(fixture(), "maintainer")
	if err != nil {
		t.Fatal(err)
	}
	originalInstructions := v.Instructions
	if v.Status != "published" || len(v.Notifications) != 2 || len(v.Events) != 1 {
		t.Fatalf("created = %#v", v)
	}
	v, err = s.Transition("repo", v.ID, "maintainer", "request_revalidation", "Test the new runtime.", "", []string{"3.0", "3.0"}, v.Version)
	if err != nil {
		t.Fatal(err)
	}
	if v.Status != "needs_revalidation" || len(v.RevalidationVersions) != 1 || v.Instructions != originalInstructions || len(v.Credits) != 2 || len(v.Events) != 2 {
		t.Fatalf("revalidation = %#v", v)
	}
	if _, err = s.Transition("repo", v.ID, "maintainer", "archive", "obsolete", "", nil, 1); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale transition = %v", err)
	}
	v, err = s.Transition("repo", v.ID, "maintainer", "archive", "superseded by 3.x", "", nil, v.Version)
	if err != nil || v.Status != "archived" || v.Instructions != originalInstructions || v.Events[len(v.Events)-1].ActorID != "maintainer" {
		t.Fatalf("archive = %#v, %v", v, err)
	}
}

func TestDuplicateMergeRequiresExplicitTarget(t *testing.T) {
	s, _ := New(t.TempDir())
	v, _ := s.Create(fixture(), "maintainer")
	if _, err := s.Transition("repo", v.ID, "maintainer", "merge_duplicate", "same answer", "", nil, v.Version); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty target = %v", err)
	}
	v, err := s.Transition("repo", v.ID, "maintainer", "merge_duplicate", "same answer", "canonical", nil, v.Version)
	if err != nil || v.Status != "merged" || v.DuplicateOf != "canonical" || v.Events[1].RelatedSolutionID != "canonical" {
		t.Fatalf("merged = %#v, %v", v, err)
	}
}
