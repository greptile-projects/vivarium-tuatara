package pullrequests

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

func TestCreateSnapshotsBranchesAndListsByRepository(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	repository, _ := gitStore.Create(testID('1'))
	tree, err := repository.WriteObject(storage.TreeObject, nil)
	if err != nil {
		t.Fatal(err)
	}
	base := writeCommit(t, repository, tree, "base")
	head := writeCommit(t, repository, tree, "head")
	if err := repository.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(base)}); err != nil {
		t.Fatal(err)
	}
	if err := repository.CreateReference(storage.Reference{Name: "refs/heads/topic", Target: string(head)}); err != nil {
		t.Fatal(err)
	}
	store, _ := New(t.TempDir(), gitStore)
	store.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }
	proposalID := testID('3')
	pullRequest, err := store.Create(repository.ID(), testID('2'), " Ship it ", " Why this matters. ", "topic", "main", &proposalID)
	if err != nil {
		t.Fatal(err)
	}
	if pullRequest.SourceCommitID != string(head) || pullRequest.TargetCommitID != string(base) || pullRequest.Title != "Ship it" || pullRequest.Status != Open {
		t.Fatalf("pull request = %#v", pullRequest)
	}
	got, err := store.Get(repository.ID(), pullRequest.ID)
	if err != nil || got.ID != pullRequest.ID {
		t.Fatalf("Get = %#v, %v", got, err)
	}
	listed, err := store.List(repository.ID())
	if err != nil || len(listed) != 1 || listed[0].ID != pullRequest.ID {
		t.Fatalf("List = %#v, %v", listed, err)
	}
}

func TestCreateRejectsMissingAndNonCommitBranches(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	repository, _ := gitStore.Create(testID('4'))
	blob, _ := repository.WriteObject(storage.BlobObject, []byte("not a commit"))
	if err := repository.CreateReference(storage.Reference{Name: "refs/heads/blob", Target: string(blob)}); err != nil {
		t.Fatal(err)
	}
	store, _ := New(t.TempDir(), gitStore)
	_, err := store.Create(repository.ID(), testID('5'), "Change", "", "missing", "blob", nil)
	if !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("Create error = %v", err)
	}
	_, err = store.Create(repository.ID(), testID('5'), "Change", "", "same", "same", nil)
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("same-branch error = %v", err)
	}
	if err := os.Remove(filepath.Join(repository.Path(), "objects", string(blob)[:2], string(blob)[2:])); err != nil {
		t.Fatal(err)
	}
	_, err = store.Create(repository.ID(), testID('5'), "Change", "", "blob", "missing", nil)
	if err == nil || errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("corrupt branch error = %v, want preserved storage failure", err)
	}
}

func TestCreateReturnsIdentityAfterUncertainDurability(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	repository, _ := gitStore.Create(testID('6'))
	tree, _ := repository.WriteObject(storage.TreeObject, nil)
	base := writeCommit(t, repository, tree, "base")
	head := writeCommit(t, repository, tree, "head")
	_ = repository.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(base)})
	_ = repository.CreateReference(storage.Reference{Name: "refs/heads/topic", Target: string(head)})
	store, _ := New(t.TempDir(), gitStore)
	store.directorySync = func(string) error { return errors.New("injected sync failure") }
	pullRequest, err := store.Create(repository.ID(), testID('7'), "Change", "Purpose", "topic", "main", nil)
	if !errors.Is(err, ErrDurabilityUncertain) || pullRequest.ID == "" {
		t.Fatalf("Create = %#v, %v", pullRequest, err)
	}
	persisted, getErr := store.Get(repository.ID(), pullRequest.ID)
	if getErr != nil || persisted.ID != pullRequest.ID {
		t.Fatalf("persisted = %#v, %v", persisted, getErr)
	}
}

func writeCommit(t *testing.T, repository *storage.Repository, tree storage.ObjectID, message string) storage.ObjectID {
	t.Helper()
	content := []byte(fmt.Sprintf("tree %s\nauthor Test <test@example.com> 1700000000 +0000\ncommitter Test <test@example.com> 1700000000 +0000\n\n%s\n", tree, message))
	id, err := repository.WriteObject(storage.CommitObject, content)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func testID(character byte) string {
	result := make([]byte, 32)
	for i := range result {
		result[i] = character
	}
	return string(result)
}
