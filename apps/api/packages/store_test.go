package packages

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"testing"
)

var errForcedDirectory = errors.New("forced directory failure")

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
	if _, err := store.Publish(item, bytes.NewReader(body)); !errors.Is(err, ErrVersionExists) {
		t.Fatalf("duplicate error = %v", err)
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
			if _, err = store.Publish(item, bytes.NewReader(body)); !errors.Is(err, errForcedDirectory) {
				t.Fatalf("publish error = %v", err)
			}
			// Rename happens before the injected failure, so the complete version
			// may be visible even though publication was not acknowledged.
			if visible, getErr := store.Get(item.Name, item.Version); getErr != nil || visible.Name != item.Name {
				t.Fatalf("visible version = %#v, %v", visible, getErr)
			}
		})
	}
}

func validVersion(body []byte) Version {
	sum := sha256.Sum256(body)
	return Version{Name: "project-sdk", Version: "1.2.3", RepositoryID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ReleaseID: "11111111111111111111111111111111", SourceCommit: "2222222222222222222222222222222222222222", BuildID: "33333333333333333333333333333333", BuildAttestation: BuildAttestation{Step: "package", Image: "alpine:3.22", Command: "make package", Attempt: 1, State: "succeeded"}, ArtifactID: "44444444444444444444444444444444", ArtifactPath: "dist/project.tgz", ContentType: "application/gzip", Size: int64(len(body)), SHA256: hex.EncodeToString(sum[:]), Platform: Platform{OS: "linux", Architecture: "amd64"}, Dependencies: []Dependency{{Name: "core-utils", Constraint: "^2.0.0"}}, PublisherID: "55555555555555555555555555555555", Visibility: "public"}
}
