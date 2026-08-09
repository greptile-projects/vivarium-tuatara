package packages

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

var errForcedDirectory = errors.New("forced directory failure")

func TestPublishUpdateSerializesExactReservation(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	value := Update{RepositoryID: strings.Repeat("a", 32), PackageName: "core-kit", FromVersion: "1.0.0", ToVersion: "1.1.0", BaseCommit: strings.Repeat("b", 40), CreatedBy: strings.Repeat("c", 32)}
	var calls atomic.Int32
	var wait sync.WaitGroup
	results := make(chan Update, 24)
	failures := make(chan error, 24)
	for range 24 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			update, _, publishErr := store.PublishUpdate(value, func() (string, string, error) {
				calls.Add(1)
				return strings.Repeat("d", 32), strings.Repeat("e", 32), nil
			})
			if publishErr != nil {
				failures <- publishErr
				return
			}
			results <- update
		}()
	}
	wait.Wait()
	close(results)
	close(failures)
	for failure := range failures {
		t.Fatal(failure)
	}
	var id string
	count := 0
	for update := range results {
		count++
		if id == "" {
			id = update.ID
		}
		if update.ID != id {
			t.Fatalf("different updates: %s and %s", id, update.ID)
		}
	}
	if count != 24 || calls.Load() != 1 {
		t.Fatalf("results = %d, callbacks = %d", count, calls.Load())
	}
}

func TestPublishUpdateFailureRetainsReservationBeforeCollaboration(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	value := Update{RepositoryID: strings.Repeat("a", 32), PackageName: "core-kit", FromVersion: "1.0.0", ToVersion: "1.1.0", BaseCommit: strings.Repeat("b", 40), CreatedBy: strings.Repeat("c", 32)}
	var calls int
	created, published, err := store.PublishUpdate(value, func() (string, string, error) {
		calls++
		directory := filepath.Join(store.root, "updates", value.RepositoryID)
		if chmodErr := os.Chmod(directory, 0500); chmodErr != nil {
			t.Fatal(chmodErr)
		}
		return strings.Repeat("d", 32), strings.Repeat("e", 32), nil
	})
	if err == nil || !published || created.ID == "" || calls != 1 {
		t.Fatalf("first = %#v, published %v, calls %d, err %v", created, published, calls, err)
	}
	if chmodErr := os.Chmod(filepath.Join(store.root, "updates", value.RepositoryID), 0700); chmodErr != nil {
		t.Fatal(chmodErr)
	}
	pending, published, err := store.PublishUpdate(value, func() (string, string, error) { calls++; return "", "", nil })
	if !errors.Is(err, ErrUpdatePending) || published || pending.ID != created.ID || calls != 1 {
		t.Fatalf("retry = %#v, published %v, calls %d, err %v", pending, published, calls, err)
	}
}

func TestDependencyInventoriesRetainExactAttributionAndConsumerPaths(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	value := Inventory{RepositoryID: strings.Repeat("a", 32), CommitID: strings.Repeat("b", 40), RecordedBy: strings.Repeat("c", 32), Entries: []InventoryEntry{{Name: "core-kit", Version: "2.1.0", Constraint: "^2.0.0", Paths: []string{"app-kit > core-kit"}, State: "resolved", License: "MIT", Support: "https://support.example.test"}}}
	created, err := store.RecordInventory(value)
	if err != nil {
		t.Fatal(err)
	}
	retried, err := store.RecordInventory(value)
	if err != nil || retried.ID != created.ID {
		t.Fatalf("retry = %#v, %v", retried, err)
	}
	consumers, err := store.ListConsumers("core-kit", "2.1.0")
	if err != nil || len(consumers) != 1 || consumers[0].Entries[0].Paths[0] != "app-kit > core-kit" {
		t.Fatalf("consumers = %#v, %v", consumers, err)
	}
}

func TestPublishRetainsImmutableProvenanceAndArtifact(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	body := []byte("reviewed package bytes")
	item := validVersion(body)
	created, err := store.Publish(item, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.Lifecycle != "active" || created.PublisherID != item.PublisherID || created.SourceCommit != item.SourceCommit {
		t.Fatalf("created = %#v", created)
	}
	file, reopened, err := store.OpenArtifact("project-sdk", "1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	actual, _ := io.ReadAll(file)
	if !bytes.Equal(actual, body) || reopened.SHA256 != item.SHA256 {
		t.Fatalf("artifact = %q, metadata = %#v", actual, reopened)
	}
	retried, err := store.Publish(item, bytes.NewReader(body))
	if !errors.Is(err, ErrAlreadyPublished) || retried.ID != created.ID {
		t.Fatalf("retry = %#v, %v", retried, err)
	}
	item.Visibility = "private"
	if _, err = store.Publish(item, bytes.NewReader(body)); !errors.Is(err, ErrVersionExists) {
		t.Fatalf("conflicting retry error = %v", err)
	}
}

func TestFailedArtifactDoesNotExposeOrReservePackage(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	item := validVersion([]byte("expected"))
	if _, err = store.Publish(item, bytes.NewReader([]byte("corrupt!"))); !errors.Is(err, ErrChecksum) {
		t.Fatalf("error = %v", err)
	}
	if _, err = store.Get(item.Name, item.Version); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get after failed upload = %v", err)
	}
	item.RepositoryID = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if _, err = store.Publish(item, bytes.NewReader([]byte("expected"))); err != nil {
		t.Fatalf("failed upload reserved identity: %v", err)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) == 0 {
		t.Fatalf("root entries = %v, %v", entries, err)
	}
}

func TestPackageIdentityCannotMoveRepositories(t *testing.T) {
	store, _ := New(t.TempDir())
	body := []byte("bytes")
	item := validVersion(body)
	if _, err := store.Publish(item, bytes.NewReader(body)); err != nil {
		t.Fatal(err)
	}
	item.Version = "2.0.0"
	item.RepositoryID = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if _, err := store.Publish(item, bytes.NewReader(body)); !errors.Is(err, ErrIdentityConflict) {
		t.Fatalf("error = %v", err)
	}
}

func TestPublishReportsPostRenameDirectoryFailures(t *testing.T) {
	for _, test := range []struct {
		name  string
		force func(*Store)
	}{
		{name: "open", force: func(store *Store) {
			store.openDirectory = func(string) (*os.File, error) { return nil, errForcedDirectory }
		}},
		{name: "sync", force: func(store *Store) { store.syncDirectory = func(*os.File) error { return errForcedDirectory } }},
		{name: "close", force: func(store *Store) {
			store.closeDirectory = func(file *os.File) error { _ = file.Close(); return errForcedDirectory }
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, err := New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			test.force(store)
			body := []byte("durable package")
			item := validVersion(body)
			created, err := store.Publish(item, bytes.NewReader(body))
			if !errors.Is(err, ErrDurabilityUncertain) || !errors.Is(err, errForcedDirectory) || created.ID == "" {
				t.Fatalf("publish = %#v, %v", created, err)
			}
			// Rename happens before the injected failure, so the complete version
			// may be visible even though publication was not acknowledged.
			if visible, getErr := store.Get(item.Name, item.Version); getErr != nil || visible.Name != item.Name {
				t.Fatalf("visible version = %#v, %v", visible, getErr)
			}
			store.openDirectory = os.Open
			store.syncDirectory = func(directory *os.File) error { return directory.Sync() }
			store.closeDirectory = func(directory *os.File) error { return directory.Close() }
			retried, retryErr := store.Publish(item, bytes.NewReader(body))
			if !errors.Is(retryErr, ErrAlreadyPublished) || retried.ID != created.ID {
				t.Fatalf("retry = %#v, %v", retried, retryErr)
			}
		})
	}
}

func validVersion(body []byte) Version {
	sum := sha256.Sum256(body)
	return Version{Name: "project-sdk", Version: "1.2.3", RepositoryID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ReleaseID: "11111111111111111111111111111111", SourceCommit: "2222222222222222222222222222222222222222", BuildID: "33333333333333333333333333333333", BuildAttestation: BuildAttestation{Step: "package", Image: "alpine:3.22", Command: "make package", Attempt: 1, State: "succeeded"}, ArtifactID: "44444444444444444444444444444444", ArtifactPath: "dist/project.tgz", ContentType: "application/gzip", Size: int64(len(body)), SHA256: hex.EncodeToString(sum[:]), Platform: Platform{OS: "linux", Architecture: "amd64"}, Dependencies: []Dependency{{Name: "core-utils", Constraint: "^2.0.0"}}, PublisherID: "55555555555555555555555555555555", Visibility: "public"}
}
