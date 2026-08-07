package users

import (
	"errors"
	"path/filepath"
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
