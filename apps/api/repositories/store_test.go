package repositories

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

const testOwnerID = "0123456789abcdef0123456789abcdef"
const testCollaboratorID = "abcdefabcdefabcdefabcdefabcdefab"

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

func TestRepositoryVisibilityDefaultsPrivateAndPersists(t *testing.T) {
	gitRoot, metadataRoot := t.TempDir(), t.TempDir()
	gitStore, _ := storage.New(gitRoot)
	store, _ := New(metadataRoot, gitStore)
	created, err := store.Create(testOwnerID, "visibility")
	if err != nil {
		t.Fatal(err)
	}
	if created.Visibility != Private {
		t.Fatalf("default visibility = %q", created.Visibility)
	}
	if _, err := store.SetVisibility("abcdefabcdefabcdefabcdefabcdefab", created.ID, Public); !errors.Is(err, ErrNotFound) {
		t.Fatalf("non-owner SetVisibility error = %v", err)
	}
	updated, err := store.SetVisibility(testOwnerID, created.ID, Public)
	if err != nil || updated.Visibility != Public {
		t.Fatalf("updated = %#v, %v", updated, err)
	}
	reopenedGit, _ := storage.New(gitRoot)
	reopened, _ := New(metadataRoot, reopenedGit)
	got, err := reopened.GetByID(created.ID)
	if err != nil || got.Visibility != Public {
		t.Fatalf("reopened = %#v, %v", got, err)
	}
}

func TestRepositoryCollaboratorsPersistAndRemainOwnerManaged(t *testing.T) {
	gitRoot, metadataRoot := t.TempDir(), t.TempDir()
	gitStore, _ := storage.New(gitRoot)
	store, _ := New(metadataRoot, gitStore)
	created, _ := store.Create(testOwnerID, "contributors")
	if _, err := store.AddCollaborator(testCollaboratorID, created.ID, testOwnerID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("non-owner grant error = %v", err)
	}
	grant, err := store.AddCollaborator(testOwnerID, created.ID, testCollaboratorID)
	hasAccess, accessErr := store.HasCollaborator(testCollaboratorID, created.ID)
	if err != nil || accessErr != nil || grant.Role != Contributor || !hasAccess {
		t.Fatalf("grant = %#v, %v", grant, err)
	}
	accessible, err := store.ListAccessible(testCollaboratorID)
	if err != nil || len(accessible) != 1 || accessible[0].ID != created.ID {
		t.Fatalf("collaborator accessible repositories = %#v, %v", accessible, err)
	}

	reopenedGit, _ := storage.New(gitRoot)
	reopened, _ := New(metadataRoot, reopenedGit)
	listed, err := reopened.ListCollaborators(testOwnerID, created.ID)
	if err != nil || len(listed) != 1 || listed[0] != grant {
		t.Fatalf("reopened collaborators = %#v, %v", listed, err)
	}
	if err := reopened.RemoveCollaborator(testOwnerID, created.ID, testCollaboratorID); err != nil {
		t.Fatal(err)
	}
	hasAccess, err = reopened.HasCollaborator(testCollaboratorID, created.ID)
	if err != nil || hasAccess {
		t.Fatal("removed collaborator retained access")
	}
	accessible, err = reopened.ListAccessible(testCollaboratorID)
	if err != nil || len(accessible) != 0 {
		t.Fatalf("repositories after collaborator removal = %#v, %v", accessible, err)
	}
}

func TestForkSynchronizationSerializesPrivateUpstreamRevocation(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	metadataRoot := t.TempDir()
	store, _ := New(metadataRoot, gitStore)
	revoker, _ := New(metadataRoot, gitStore)
	upstream, err := store.Create(testOwnerID, "private-upstream")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddCollaborator(testOwnerID, upstream.ID, testCollaboratorID); err != nil {
		t.Fatal(err)
	}
	upstreamGit, _ := gitStore.Open(upstream.ID)
	tree, err := upstreamGit.WriteObject(storage.TreeObject, nil)
	if err != nil {
		t.Fatal(err)
	}
	commit := func(parent storage.ObjectID, timestamp int64, message string) storage.ObjectID {
		parentLine := ""
		if parent != "" {
			parentLine = fmt.Sprintf("parent %s\n", parent)
		}
		content := fmt.Sprintf("tree %s\n%sauthor Test <test@example.com> %d +0000\ncommitter Test <test@example.com> %d +0000\n\n%s\n", tree, parentLine, timestamp, timestamp, message)
		id, err := upstreamGit.WriteObject(storage.CommitObject, []byte(content))
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	initial := commit("", 1700000000, "initial")
	if err := upstreamGit.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(initial)}); err != nil {
		t.Fatal(err)
	}
	fork, err := store.CreateFork(testCollaboratorID, upstream.ID, "private-fork")
	if err != nil {
		t.Fatal(err)
	}
	later := commit(initial, 1700000100, "later")
	if err := upstreamGit.UpdateReference(storage.Reference{Name: "refs/heads/main", Target: string(later)}); err != nil {
		t.Fatal(err)
	}

	authorized, release := make(chan struct{}), make(chan struct{})
	store.afterSynchronizeAuthorization = func() {
		close(authorized)
		<-release
	}
	syncDone := make(chan error, 1)
	go func() {
		_, err := store.SynchronizeFork(testCollaboratorID, fork.ID, "main")
		syncDone <- err
	}()
	<-authorized
	revokeDone := make(chan error, 1)
	go func() { revokeDone <- revoker.RemoveCollaborator(testOwnerID, upstream.ID, testCollaboratorID) }()
	select {
	case err := <-revokeDone:
		t.Fatalf("revocation completed inside authorized synchronization: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if err := <-syncDone; err != nil {
		t.Fatalf("authorized synchronization: %v", err)
	}
	if err := <-revokeDone; err != nil {
		t.Fatalf("revocation after synchronization: %v", err)
	}
	store.afterSynchronizeAuthorization = nil
	if _, err := store.SynchronizeFork(testCollaboratorID, fork.ID, "main"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("synchronization after revocation error = %v", err)
	}
}

func TestForkCreationSerializesPrivateSourceRevocation(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	metadataRoot := t.TempDir()
	store, _ := New(metadataRoot, gitStore)
	revoker, _ := New(metadataRoot, gitStore)
	source, err := store.Create(testOwnerID, "private-fork-source")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddCollaborator(testOwnerID, source.ID, testCollaboratorID); err != nil {
		t.Fatal(err)
	}

	authorized, release := make(chan struct{}), make(chan struct{})
	store.afterCreateForkAuthorization = func() {
		close(authorized)
		<-release
	}
	type forkResult struct {
		repository Repository
		err        error
	}
	forkDone := make(chan forkResult, 1)
	go func() {
		repository, err := store.CreateFork(testCollaboratorID, source.ID, "authorized-fork")
		forkDone <- forkResult{repository: repository, err: err}
	}()
	<-authorized
	revokeDone := make(chan error, 1)
	go func() { revokeDone <- revoker.RemoveCollaborator(testOwnerID, source.ID, testCollaboratorID) }()
	select {
	case err := <-revokeDone:
		t.Fatalf("revocation completed inside authorized fork creation: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	result := <-forkDone
	if result.err != nil || result.repository.UpstreamRepositoryID != source.ID {
		t.Fatalf("authorized fork = %#v, %v", result.repository, result.err)
	}
	if err := <-revokeDone; err != nil {
		t.Fatalf("revocation after fork creation: %v", err)
	}
	store.afterCreateForkAuthorization = nil
	if _, err := store.CreateFork(testCollaboratorID, source.ID, "revoked-fork"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("fork creation after revocation error = %v", err)
	}
}

func TestContributionAuthorizationSerializesPrivateTargetRevocation(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	metadataRoot := t.TempDir()
	store, _ := New(metadataRoot, gitStore)
	revoker, _ := New(metadataRoot, gitStore)
	target, err := store.Create(testOwnerID, "contribution-target")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddCollaborator(testOwnerID, target.ID, testCollaboratorID); err != nil {
		t.Fatal(err)
	}
	source, err := store.CreateFork(testCollaboratorID, target.ID, "contribution-source")
	if err != nil {
		t.Fatal(err)
	}

	authorized, release := make(chan struct{}), make(chan struct{})
	store.afterContributionAuthorization = func() {
		close(authorized)
		<-release
	}
	contributionDone := make(chan error, 1)
	go func() {
		contributionDone <- store.WithContributionAuthorization(testCollaboratorID, target.ID, source.ID, func() error { return nil })
	}()
	<-authorized
	revokeDone := make(chan error, 1)
	go func() { revokeDone <- revoker.RemoveCollaborator(testOwnerID, target.ID, testCollaboratorID) }()
	select {
	case err := <-revokeDone:
		t.Fatalf("revocation completed inside authorized contribution: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if err := <-contributionDone; err != nil {
		t.Fatalf("authorized contribution: %v", err)
	}
	if err := <-revokeDone; err != nil {
		t.Fatalf("revocation after contribution: %v", err)
	}
	store.afterContributionAuthorization = nil
	if err := store.WithContributionAuthorization(testCollaboratorID, target.ID, source.ID, func() error { return nil }); !errors.Is(err, ErrNotFound) {
		t.Fatalf("contribution after revocation error = %v", err)
	}
}

func TestCollaboratorRemovalReconcilesPostRenameSyncFailure(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	store, _ := New(t.TempDir(), gitStore)
	created, _ := store.Create(testOwnerID, "revoke-uncertain")
	if _, err := store.AddCollaborator(testOwnerID, created.ID, testCollaboratorID); err != nil {
		t.Fatal(err)
	}
	store.directorySync = func(string) error { return errors.New("injected directory sync failure") }
	if err := store.RemoveCollaborator(testOwnerID, created.ID, testCollaboratorID); err != nil {
		t.Fatalf("committed removal returned an error: %v", err)
	}
	hasAccess, err := store.HasCollaborator(testCollaboratorID, created.ID)
	if err != nil || hasAccess {
		t.Fatalf("access after committed removal = %v, %v", hasAccess, err)
	}
}

func TestCollaboratorRemovalReportsPreRenameFailure(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	store, _ := New(t.TempDir(), gitStore)
	created, _ := store.Create(testOwnerID, "revoke-failed")
	if _, err := store.AddCollaborator(testOwnerID, created.ID, testCollaboratorID); err != nil {
		t.Fatal(err)
	}
	store.rename = func(string, string) error { return errors.New("injected rename failure") }
	if err := store.RemoveCollaborator(testOwnerID, created.ID, testCollaboratorID); err == nil {
		t.Fatal("uncommitted removal returned success")
	}
	hasAccess, err := store.HasCollaborator(testCollaboratorID, created.ID)
	if err != nil || !hasAccess {
		t.Fatalf("access after failed removal = %v, %v", hasAccess, err)
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
