package repositories

import (
	"errors"
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
