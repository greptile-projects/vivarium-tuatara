package storage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDeleteRetryCleansStableTombstoneAfterCleanupFailure(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repository, err := store.Create("recoverable")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.WriteObject(BlobObject, []byte("retained until cleanup")); err != nil {
		t.Fatal(err)
	}
	store.removeAll = func(string) error { return errors.New("injected cleanup failure") }
	if err := store.Delete("recoverable"); err == nil {
		t.Fatal("Delete succeeded despite tombstone cleanup failure")
	}
	if _, err := os.Stat(filepath.Join(store.root, ".deleting-recoverable")); err != nil {
		t.Fatalf("stable tombstone was not retained: %v", err)
	}
	if _, err := store.Create("recoverable"); !errors.Is(err, ErrRepositoryExists) {
		t.Fatalf("Create reused ID with pending deletion: %v", err)
	}

	store.removeAll = os.RemoveAll
	if err := store.Delete("recoverable"); err != nil {
		t.Fatalf("Delete retry: %v", err)
	}
	if _, err := os.Stat(filepath.Join(store.root, ".deleting-recoverable")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("tombstone remains after retry: %v", err)
	}
	if _, err := store.Open("recoverable"); !errors.Is(err, ErrRepositoryNotFound) {
		t.Fatalf("Open after retry: %v", err)
	}
}
