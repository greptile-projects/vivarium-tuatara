package users

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestIdentityPersistsAndProfileChangesWithoutChangingID(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.Create("Ada-Lovelace", "Ada Lovelace")
	if err != nil {
		t.Fatal(err)
	}
	if created.Handle != "ada-lovelace" || len(created.ID) != 32 {
		t.Fatalf("created user = %#v", created)
	}

	reopened, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	reopened.now = func() time.Time { return created.UpdatedAt.Add(time.Hour) }
	updated, err := reopened.Update(created.ID, "ada", "Ada King")
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != created.ID || updated.CreatedAt != created.CreatedAt || updated.Handle != "ada" || updated.DisplayName != "Ada King" {
		t.Fatalf("updated user = %#v, created = %#v", updated, created)
	}
	if !updated.UpdatedAt.After(created.UpdatedAt) {
		t.Fatal("updated_at did not advance")
	}
	persisted, err := reopened.Get(created.ID)
	if err != nil || persisted != updated {
		t.Fatalf("persisted = %#v, err = %v", persisted, err)
	}
	if _, err := reopened.Get(filepath.Base(root)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("invalid ID error = %v", err)
	}
}

func TestIndependentStoresCoordinateHandleClaims(t *testing.T) {
	root := t.TempDir()
	first, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errorsSeen := make(chan error, 2)
	var wait sync.WaitGroup
	for index, store := range []*Store{first, second} {
		wait.Add(1)
		go func(index int, store *Store) {
			defer wait.Done()
			<-start
			_, err := store.Create("shared-handle", []string{"First", "Second"}[index])
			errorsSeen <- err
		}(index, store)
	}
	close(start)
	wait.Wait()
	close(errorsSeen)
	successes, collisions := 0, 0
	for err := range errorsSeen {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrHandleTaken):
			collisions++
		default:
			t.Fatalf("Create error = %v", err)
		}
	}
	if successes != 1 || collisions != 1 {
		t.Fatalf("successes = %d, collisions = %d", successes, collisions)
	}
	all, err := first.loadAll()
	if err != nil || len(all) != 1 {
		t.Fatalf("persisted users = %#v, err = %v", all, err)
	}
}

func TestConcurrentSparsePatchesPreserveBothChanges(t *testing.T) {
	root := t.TempDir()
	first, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	user, err := first.Create("original", "Original Name")
	if err != nil {
		t.Fatal(err)
	}
	handle, displayName := "changed", "Changed Name"
	start := make(chan struct{})
	errorsSeen := make(chan error, 2)
	go func() { <-start; _, err := first.Patch(user.ID, ProfilePatch{Handle: &handle}); errorsSeen <- err }()
	go func() {
		<-start
		_, err := second.Patch(user.ID, ProfilePatch{DisplayName: &displayName})
		errorsSeen <- err
	}()
	close(start)
	for range 2 {
		if err := <-errorsSeen; err != nil {
			t.Fatal(err)
		}
	}
	final, err := first.Get(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if final.Handle != handle || final.DisplayName != displayName {
		t.Fatalf("final profile = %#v", final)
	}
}

func TestHandlesAreUniqueAndProfilesAreValidated(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.Create("grace", "Grace Hopper")
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Create("alan", "Alan Turing")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("GRACE", "Someone Else"); !errors.Is(err, ErrHandleTaken) {
		t.Fatalf("duplicate error = %v", err)
	}
	if _, err := store.Update(second.ID, first.Handle, second.DisplayName); !errors.Is(err, ErrHandleTaken) {
		t.Fatalf("update collision error = %v", err)
	}
	for _, profile := range [][2]string{{"-bad", "Name"}, {"bad_underscore", "Name"}, {"valid", ""}, {"valid", "two\nlines"}} {
		if _, err := store.Create(profile[0], profile[1]); !errors.Is(err, ErrInvalidProfile) {
			t.Fatalf("Create(%q, %q) error = %v", profile[0], profile[1], err)
		}
	}
}

func TestBootstrapFailureDoesNotPublishUserOrReserveHandle(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bootstrapErr := errors.New("bootstrap unavailable")
	var prospective User
	if _, err := store.CreateWithBootstrap("retryable", "First Attempt", func(user User) error { prospective = user; return bootstrapErr }); !errors.Is(err, bootstrapErr) {
		t.Fatalf("CreateWithBootstrap error = %v", err)
	}
	if _, err := store.Get(prospective.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get unpublished user error = %v", err)
	}
	if _, err := store.Create("retryable", "Second Attempt"); err != nil {
		t.Fatalf("reuse bootstrap handle: %v", err)
	}
}
