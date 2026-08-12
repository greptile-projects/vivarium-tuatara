package docscollections

import (
	"testing"
	"time"
)

func TestPullReviewRetainsContentExactFeedback(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 12, 6, 0, 0, 0, time.UTC)
	review, err := store.SavePullReview(PullReview{RepositoryID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", PullRequestID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Revision: "1111111111111111111111111111111111111111", BaseRevision: "2222222222222222222222222222222222222222", RootPath: "docs", Pages: []ReviewPage{{Path: "docs/guide.md", SourceSHA256: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", Status: "current"}}, Entries: []ReviewEntry{}, Decisions: []ReviewDecision{}, Invitations: []ReviewInvitation{}})
	if err != nil {
		t.Fatal(err)
	}
	review, err = store.UpdatePullReview(review.RepositoryID, review.PullRequestID, func(v *PullReview) error {
		v.Entries = append(v.Entries, NewReviewEntry("comment", "docs/guide.md", "audience", v.Pages[0].SourceSHA256, "Explain the prerequisite.", "dddddddddddddddddddddddddddddddd", now))
		v.Decisions = append(v.Decisions, NewReviewDecision("docs/guide.md", "technical", v.Pages[0].SourceSHA256, "approved", "Example verified.", "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", now))
		v.Invitations = append(v.Invitations, NewReviewInvitation("ffffffffffffffffffffffffffffffff", "review", []string{"technical", "audience"}, now.Add(time.Hour), "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if review.Entries[0].SourceSHA256 != review.Pages[0].SourceSHA256 || review.Decisions[0].SourceSHA256 != review.Pages[0].SourceSHA256 {
		t.Fatal("review evidence was not frozen to rendered content")
	}
	if invite, ok := ActiveInvitation(review, "ffffffffffffffffffffffffffffffff", now); !ok || invite.Role != "review" {
		t.Fatalf("active invitation = %#v, %v", invite, ok)
	}
	if _, ok := ActiveInvitation(review, "ffffffffffffffffffffffffffffffff", now.Add(2*time.Hour)); ok {
		t.Fatal("expired invitation remained active")
	}
}

func TestReviewMutationRequiresExactPageAndArea(t *testing.T) {
	if ValidateReviewMutation("../guide.md", "technical", "comment") == nil {
		t.Fatal("unsafe page accepted")
	}
	if ValidateReviewMutation("docs/guide.md", "unknown", "comment") == nil {
		t.Fatal("unknown area accepted")
	}
	if ValidateReviewMutation("docs/guide.md", "audience", "  ") == nil {
		t.Fatal("empty feedback accepted")
	}
}
