package storage

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestForkRemovesPublishedStorageAfterLateSyncFailure(t *testing.T) {
	for _, test := range []struct {
		name       string
		failOnCall int
	}{
		{name: "repository sync", failOnCall: 1},
		{name: "storage root sync", failOnCall: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, err := New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.Create("source"); err != nil {
				t.Fatal(err)
			}
			calls := 0
			store.directorySync = func(path string) error {
				calls++
				if calls == test.failOnCall {
					return errors.New("injected late sync failure")
				}
				return syncDirectory(path)
			}
			if _, err := store.Fork("source", "fork"); err == nil || !strings.Contains(err.Error(), "injected late sync failure") {
				t.Fatalf("Fork error = %v", err)
			}
			if _, err := os.Lstat(store.root + "/fork"); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("failed fork storage remained: %v", err)
			}
			if _, err := store.Open("fork"); !errors.Is(err, ErrRepositoryNotFound) {
				t.Fatalf("Open failed fork error = %v", err)
			}
		})
	}
}

func TestForkPreservesPublicationAndCleanupFailures(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("source"); err != nil {
		t.Fatal(err)
	}
	store.directorySync = func(string) error { return errors.New("publication sync failed") }
	store.removeAll = func(string) error { return errors.New("cleanup remove failed") }
	_, err = store.Fork("source", "fork")
	if err == nil || !strings.Contains(err.Error(), "publication sync failed") || !strings.Contains(err.Error(), "cleanup remove failed") {
		t.Fatalf("Fork error = %v", err)
	}
}
