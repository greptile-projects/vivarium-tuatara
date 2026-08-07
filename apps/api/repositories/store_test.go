package repositories

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

const testOwnerID = "0123456789abcdef0123456789abcdef"

func TestRepositoryCatalogPersistsOwnershipAndGitIdentity(t *testing.T) {
	gitRoot, metadataRoot := t.TempDir(), t.TempDir()
	gitStore, _ := storage.New(gitRoot)
	store, _ := New(metadataRoot, gitStore)
	created, err := store.Create(testOwnerID, "shared-work")
	if err != nil {
		t.Fatal(err)
	}
	if created.GitRemote != "/git/"+created.ID+".git" {
		t.Fatalf("remote = %q", created.GitRemote)
	}
	if _, err := gitStore.Open(created.ID); err != nil {
		t.Fatal(err)
	}

	reopenedGit, _ := storage.New(gitRoot)
	reopened, _ := New(metadataRoot, reopenedGit)
	got, err := reopened.Get(testOwnerID, created.ID)
	if err != nil || got != created {
		t.Fatalf("reopened = %#v, %v", got, err)
	}
	if err := reopened.Delete(testOwnerID, created.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.Get(testOwnerID, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get after delete: %v", err)
	}
	if _, err := reopenedGit.Open(created.ID); !errors.Is(err, storage.ErrRepositoryNotFound) {
		t.Fatalf("open Git after delete: %v", err)
	}
}

func TestDeleteMetadataFailureDoesNotExposeOrReserveMissingRemote(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	store, _ := New(t.TempDir(), gitStore)
	created, err := store.Create(testOwnerID, "recoverable")
	if err != nil {
		t.Fatal(err)
	}
	store.remove = func(string) error { return fmt.Errorf("injected metadata removal failure") }
	if err := store.Delete(testOwnerID, created.ID); err == nil {
		t.Fatal("Delete succeeded despite metadata removal failure")
	}
	listed, err := store.List(testOwnerID)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Fatalf("List returned deleted repository: %#v", listed)
	}
	if _, err := store.Get(testOwnerID, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get after detached Git repository: %v", err)
	}
	if _, err := store.Create(testOwnerID, "recoverable"); err != nil {
		t.Fatalf("stale metadata retained repository name: %v", err)
	}

	store.remove = os.Remove
	if err := store.Delete(testOwnerID, created.ID); err != nil {
		t.Fatalf("cleanup retry: %v", err)
	}
}

type interruptedGitDelete struct {
	git       *storage.Store
	interrupt bool
}

func (s *interruptedGitDelete) Create(id string) (*storage.Repository, error) {
	return s.git.Create(id)
}

func (s *interruptedGitDelete) Open(id string) (*storage.Repository, error) {
	return s.git.Open(id)
}

func (s *interruptedGitDelete) Delete(id string) error {
	if s.interrupt {
		s.interrupt = false
		if err := s.git.Delete(id); err != nil {
			return err
		}
		return errors.New("injected post-detach cleanup failure")
	}
	// The storage-layer regression test proves retry cleanup of its retained
	// stable tombstone. Delegation models that successful retry boundary.
	return s.git.Delete(id)
}

func TestGitDeletionFailurePreservesCatalogMetadataForRetry(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	store, _ := New(t.TempDir(), gitStore)
	created, err := store.Create(testOwnerID, "retry-git-cleanup")
	if err != nil {
		t.Fatal(err)
	}
	store.git = &interruptedGitDelete{git: gitStore, interrupt: true}
	if err := store.Delete(testOwnerID, created.ID); err == nil {
		t.Fatal("Delete succeeded despite Git cleanup failure")
	}
	if _, err := store.read(created.ID); err != nil {
		t.Fatalf("catalog metadata needed for retry was removed: %v", err)
	}
	listed, err := store.List(testOwnerID)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 0 {
		t.Fatalf("detached repository remained active: %#v", listed)
	}
	if err := store.Delete(testOwnerID, created.ID); err != nil {
		t.Fatalf("authenticated cleanup retry: %v", err)
	}
	if _, err := store.read(created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("catalog metadata remains after successful retry: %v", err)
	}
}

func TestRepositoryNameClaimIsAtomicAcrossStores(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	root := t.TempDir()
	first, _ := New(root, gitStore)
	second, _ := New(root, gitStore)
	stores := []*Store{first, second}
	errorsSeen := make(chan error, len(stores))
	var start sync.WaitGroup
	start.Add(1)
	for _, store := range stores {
		go func(store *Store) {
			start.Wait()
			_, err := store.Create(testOwnerID, "Project")
			errorsSeen <- err
		}(store)
	}
	start.Done()
	var created, conflicts int
	for range stores {
		switch err := <-errorsSeen; {
		case err == nil:
			created++
		case errors.Is(err, ErrNameTaken):
			conflicts++
		default:
			t.Fatalf("create error = %v", err)
		}
	}
	if created != 1 || conflicts != 1 {
		t.Fatalf("created = %d, conflicts = %d", created, conflicts)
	}
}
