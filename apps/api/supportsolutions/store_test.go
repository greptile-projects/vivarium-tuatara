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
	if _, err = s.Transition("repo", v.ID, "maintainer", "request_revalidation", "revive", "", []string{"3.0"}, v.Version); !errors.Is(err, ErrInvalid) {
		t.Fatalf("merged solution revived: %v", err)
	}
}

func TestCreateIsIdempotentForExactResolutionEvidence(t *testing.T) {
	s, _ := New(t.TempDir())
	first, err := s.Create(fixture(), "maintainer")
	if err != nil {
		t.Fatal(err)
	}
	retry := fixture()
	retry.Title = "A retry must not rewrite the published title"
	second, err := s.Create(retry, "maintainer")
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID || second.Title != first.Title {
		t.Fatalf("retry created or rewrote solution: first=%#v second=%#v", first, second)
	}
	all, err := s.List("repo")
	if err != nil || len(all) != 1 {
		t.Fatalf("solutions = %#v, %v", all, err)
	}
}

func TestDeleteResolutionCompensatesOnlyExactEvidence(t *testing.T) {
	s, _ := New(t.TempDir())
	v, err := s.Create(fixture(), "maintainer")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.DeleteResolution("repo", v.ID, v.ThreadID, v.AnswerRevisionID, "other-attempt"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("mismatched compensation = %v", err)
	}
	if _, err = s.Get("repo", v.ID); err != nil {
		t.Fatalf("mismatch removed solution: %v", err)
	}
	if err = s.DeleteResolution("repo", v.ID, v.ThreadID, v.AnswerRevisionID, v.VerificationAttemptID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Get("repo", v.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("compensated solution remains: %v", err)
	}
}
