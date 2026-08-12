package docscollections

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestCreatePullReviewIsAtomicAcrossStores(t *testing.T) {
	root := t.TempDir()
	first, _ := New(root)
	second, _ := New(root)
	base := PullReview{RepositoryID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", PullRequestID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Revision: "1111111111111111111111111111111111111111", BaseRevision: "2222222222222222222222222222222222222222", RootPath: "docs", Pages: []ReviewPage{}, Entries: []ReviewEntry{}, Decisions: []ReviewDecision{}, Invitations: []ReviewInvitation{}}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for index, store := range []*Store{first, second} {
		go func(index int, store *Store) {
			ready.Done()
			<-start
			candidate := base
			candidate.Gaps = []ReviewGap{{ID: reviewID(), Area: "technical", Detail: string(rune('A' + index))}}
			_, err := store.CreatePullReview(candidate)
			errs <- err
		}(index, store)
	}
	ready.Wait()
	close(start)
	successes, conflicts := 0, 0
	for range 2 {
		switch err := <-errs; {
		case err == nil:
			successes++
		case errors.Is(err, ErrConflict):
			conflicts++
		default:
			t.Fatalf("create error = %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes = %d, conflicts = %d", successes, conflicts)
	}
	stored, err := first.GetPullReview(base.RepositoryID, base.PullRequestID)
	if err != nil || len(stored.Gaps) != 1 {
		t.Fatalf("stored review = %#v, error = %v", stored, err)
	}
}

func TestUpdatePullReviewSerializesAcrossStores(t *testing.T) {
	root := t.TempDir()
	creator, _ := New(root)
	base := PullReview{RepositoryID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", PullRequestID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Revision: "1111111111111111111111111111111111111111", BaseRevision: "2222222222222222222222222222222222222222", RootPath: "docs", Pages: []ReviewPage{}, Entries: []ReviewEntry{}, Decisions: []ReviewDecision{}, Invitations: []ReviewInvitation{}}
	if _, err := creator.CreatePullReview(base); err != nil {
		t.Fatal(err)
	}
	stores := make([]*Store, 12)
	for i := range stores {
		stores[i], _ = New(root)
	}
	start := make(chan struct{})
	errs := make(chan error, len(stores))
	var ready sync.WaitGroup
	ready.Add(len(stores))
	for index, store := range stores {
		go func(index int, store *Store) {
			ready.Done()
			<-start
			_, err := store.UpdatePullReview(base.RepositoryID, base.PullRequestID, func(v *PullReview) error {
				v.Entries = append(v.Entries, ReviewEntry{ID: reviewID(), Kind: "comment", Path: "docs/guide.md", Area: "technical", Body: string(rune('A' + index))})
				return nil
			})
			errs <- err
		}(index, store)
	}
	ready.Wait()
	close(start)
	for range stores {
		if err := <-errs; err != nil {
			t.Fatalf("update error = %v", err)
		}
	}
	stored, err := creator.GetPullReview(base.RepositoryID, base.PullRequestID)
	if err != nil || len(stored.Entries) != len(stores) {
		t.Fatalf("retained entries = %d, want %d; error = %v", len(stored.Entries), len(stores), err)
	}
}

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
