package proposals

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

const (
	repositoryID = "0123456789abcdef0123456789abcdef"
	authorID     = "abcdefabcdefabcdefabcdefabcdefab"
	commenterID  = "11111111111111111111111111111111"
)

func TestProposalAndConversationPersistWithAttribution(t *testing.T) {
	root := t.TempDir()
	store, _ := New(root)
	created, err := store.Create(repositoryID, authorID, "An idea", "Shared context")
	if err != nil {
		t.Fatal(err)
	}
	comment, err := store.AddComment(repositoryID, created.ID, commenterID, "Useful feedback")
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.Update(repositoryID, created.ID, Patch{Title: pointer("A refined idea"), Status: pointer(Closed)})
	if err != nil {
		t.Fatal(err)
	}
	if updated.AuthorID != authorID || updated.Status != Closed || updated.ClosedAt == nil {
		t.Fatalf("updated = %#v", updated)
	}

	reopened, _ := New(root)
	got, err := reopened.Get(repositoryID, created.ID)
	if err != nil || got.ID != updated.ID || got.Title != updated.Title || got.Status != Closed || got.ClosedAt == nil || !got.ClosedAt.Equal(*updated.ClosedAt) {
		t.Fatalf("reopened = %#v, %v", got, err)
	}
	comments, err := reopened.ListComments(repositoryID, created.ID)
	if err != nil || len(comments) != 1 || comments[0] != comment || comments[0].AuthorID != commenterID {
		t.Fatalf("comments = %#v, %v", comments, err)
	}
	if _, err := reopened.Update(repositoryID, created.ID, Patch{Status: pointer(Closed)}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("second close: %v", err)
	}
}

func TestConcurrentCommentsAreNotLostAcrossStores(t *testing.T) {
	root := t.TempDir()
	first, _ := New(root)
	second, _ := New(root)
	proposal, _ := first.Create(repositoryID, authorID, "Discuss", "")
	stores := []*Store{first, second}
	var wg sync.WaitGroup
	for i, store := range stores {
		wg.Add(1)
		go func(store *Store, body string) {
			defer wg.Done()
			if _, err := store.AddComment(repositoryID, proposal.ID, commenterID, body); err != nil {
				t.Errorf("comment: %v", err)
			}
		}(store, []string{"first", "second"}[i])
	}
	wg.Wait()
	comments, err := first.ListComments(repositoryID, proposal.ID)
	if err != nil || len(comments) != 2 {
		t.Fatalf("comments = %#v, %v", comments, err)
	}
}

func TestGetPreservesCorruptRecordError(t *testing.T) {
	root := t.TempDir()
	store, _ := New(root)
	proposal, err := store.Create(repositoryID, authorID, "An idea", "Context")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, proposal.ID+".json"), []byte("not json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(repositoryID, proposal.ID); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("Get error = %v, want preserved corruption error", err)
	}
	if _, err := store.Get(repositoryID, "22222222222222222222222222222222"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing Get error = %v", err)
	}
}

func TestMutationsReconcilePostRenameDirectorySyncFailure(t *testing.T) {
	store, _ := New(t.TempDir())
	failReads := false
	store.directorySync = func(string) error {
		failReads = true
		return errors.New("injected directory sync failure")
	}
	store.readFile = func(path string) ([]byte, error) {
		if failReads {
			return nil, errors.New("injected verification read failure")
		}
		return os.ReadFile(path)
	}
	proposal, err := store.Create(repositoryID, authorID, "Durable idea", "Context")
	if !errors.Is(err, ErrDurabilityUncertain) || proposal.ID == "" {
		t.Fatalf("committed create result = %#v, %v", proposal, err)
	}
	failReads = false
	listed, err := store.List(repositoryID)
	if err != nil || len(listed) != 1 || listed[0].ID != proposal.ID {
		t.Fatalf("proposals after create = %#v, %v", listed, err)
	}
	comment, err := store.AddComment(repositoryID, proposal.ID, commenterID, "Feedback")
	if !errors.Is(err, ErrDurabilityUncertain) || comment.ID == "" {
		t.Fatalf("committed comment result = %#v, %v", comment, err)
	}
	failReads = false
	comments, err := store.ListComments(repositoryID, proposal.ID)
	if err != nil || len(comments) != 1 || comments[0].ID != comment.ID {
		t.Fatalf("comments after append = %#v, %v", comments, err)
	}
	updated, err := store.Update(repositoryID, proposal.ID, Patch{Status: pointer(Closed)})
	if !errors.Is(err, ErrDurabilityUncertain) || updated.Status != Closed {
		t.Fatalf("committed update = %#v, %v", updated, err)
	}
}

func pointer(value string) *string { return &value }
