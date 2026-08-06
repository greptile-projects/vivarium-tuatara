package storage_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

func TestWriteAndReadObjectsAreGitCompatible(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, err := store.Create("objects")
	if err != nil {
		t.Fatal(err)
	}

	blob := []byte("hello from storage\n")
	blobID := writeObject(t, repo, storage.BlobObject, blob)
	tree := append([]byte("100644 greeting.txt\x00"), decodeObjectID(t, blobID)...)
	treeID := writeObject(t, repo, storage.TreeObject, tree)
	commit := []byte(fmt.Sprintf("tree %s\nauthor Test Author <test@example.com> 1700000000 +0000\ncommitter Test Author <test@example.com> 1700000000 +0000\n\ninitial commit\n", treeID))
	commitID := writeObject(t, repo, storage.CommitObject, commit)
	tag := []byte(fmt.Sprintf("object %s\ntype commit\ntag v1.0.0\ntagger Test Author <test@example.com> 1700000000 +0000\n\nrelease\n", commitID))
	tagID := writeObject(t, repo, storage.TagObject, tag)

	objects := []struct {
		id      storage.ObjectID
		kind    storage.ObjectType
		content []byte
	}{
		{blobID, storage.BlobObject, blob},
		{treeID, storage.TreeObject, tree},
		{commitID, storage.CommitObject, commit},
		{tagID, storage.TagObject, tag},
	}
	for _, want := range objects {
		got, err := repo.ReadObject(want.id)
		if err != nil {
			t.Fatalf("ReadObject(%s): %v", want.id, err)
		}
		if got.ID != want.id || got.Type != want.kind || got.Size != int64(len(want.content)) || !bytes.Equal(got.Content, want.content) {
			t.Errorf("ReadObject(%s) = %#v, want type %q and exact content", want.id, got, want.kind)
		}

		gitType := gitOutput(t, repo.Path(), "cat-file", "-t", string(want.id))
		if gitType != string(want.kind)+"\n" {
			t.Errorf("git type for %s = %q, want %q", want.id, gitType, want.kind)
		}
		gitContent := gitOutputBytes(t, repo.Path(), "cat-file", string(want.kind), string(want.id))
		if !bytes.Equal(gitContent, want.content) {
			t.Errorf("git content for %s changed: got %q, want %q", want.id, gitContent, want.content)
		}
	}

	// Writing identical canonical bytes is idempotent.
	if id, err := repo.WriteObject(storage.BlobObject, blob); err != nil || id != blobID {
		t.Fatalf("duplicate WriteObject = %s, %v; want %s", id, err, blobID)
	}
	gitOutput(t, repo.Path(), "fsck", "--full")
}

func TestObjectsRejectInvalidInputsAndCorruption(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, err := store.Create("validation")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := repo.WriteObject(storage.ObjectType("note"), []byte("content")); !errors.Is(err, storage.ErrInvalidObject) {
		t.Fatalf("unsupported object type error = %v", err)
	}
	for _, id := range []storage.ObjectID{"", "abc", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "../0000000000000000000000000000000000000"} {
		if _, err := repo.ReadObject(id); !errors.Is(err, storage.ErrInvalidObject) {
			t.Errorf("ReadObject(%q) error = %v", id, err)
		}
	}
	missing := storage.ObjectID("0000000000000000000000000000000000000000")
	if _, err := repo.ReadObject(missing); !errors.Is(err, storage.ErrObjectNotFound) {
		t.Fatalf("missing object error = %v", err)
	}

	id := writeObject(t, repo, storage.BlobObject, []byte("original"))
	path := filepath.Join(repo.Path(), "objects", string(id)[:2], string(id)[2:])
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not zlib"), 0o444); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ReadObject(id); !errors.Is(err, storage.ErrCorruptObject) {
		t.Fatalf("corrupt object error = %v", err)
	}
	if _, err := repo.WriteObject(storage.BlobObject, []byte("original")); !errors.Is(err, storage.ErrCorruptObject) {
		t.Fatalf("write over corrupt object error = %v", err)
	}
}

func writeObject(t *testing.T, repo *storage.Repository, kind storage.ObjectType, content []byte) storage.ObjectID {
	t.Helper()
	id, err := repo.WriteObject(kind, content)
	if err != nil {
		t.Fatalf("WriteObject(%q): %v", kind, err)
	}
	return id
}

func decodeObjectID(t *testing.T, id storage.ObjectID) []byte {
	t.Helper()
	decoded, err := hex.DecodeString(string(id))
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

func gitOutput(t *testing.T, gitDir string, arguments ...string) string {
	t.Helper()
	return string(gitOutputBytes(t, gitDir, arguments...))
}

func gitOutputBytes(t *testing.T, gitDir string, arguments ...string) []byte {
	t.Helper()
	command := exec.Command("git", append([]string{"--git-dir=" + gitDir}, arguments...)...)
	output, err := command.Output()
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(arguments, " "), err)
	}
	return output
}

func TestRepositoryLifecycle(t *testing.T) {
	store, err := storage.New(filepath.Join(t.TempDir(), "repositories"))
	if err != nil {
		t.Fatal(err)
	}

	repo, err := store.Create("project-1")
	if err != nil {
		t.Fatal(err)
	}
	if repo.ID() != "project-1" || !filepath.IsAbs(repo.Path()) {
		t.Fatalf("unexpected identity: ID=%q Path=%q", repo.ID(), repo.Path())
	}

	info, err := repo.Inspect()
	if err != nil {
		t.Fatal(err)
	}
	if info.ID != "project-1" || info.DefaultBranch != "main" || !info.Bare || !info.Empty {
		t.Fatalf("unexpected repository info: %+v", info)
	}

	reopened, err := store.Open("project-1")
	if err != nil {
		t.Fatal(err)
	}
	if reopened.ID() != repo.ID() || reopened.Path() != repo.Path() {
		t.Fatalf("reopened repository changed identity: %#v", reopened)
	}

	git := exec.Command("git", "--git-dir="+reopened.Path(), "rev-parse", "--is-bare-repository")
	output, err := git.CombinedOutput()
	if err != nil {
		t.Fatalf("stock Git rejected repository: %v\n%s", err, output)
	}
	if string(output) != "true\n" {
		t.Fatalf("git reported unexpected repository type: %q", output)
	}

	fsck := exec.Command("git", "--git-dir="+reopened.Path(), "fsck", "--full")
	if output, err := fsck.CombinedOutput(); err != nil {
		t.Fatalf("git fsck rejected repository: %v\n%s", err, output)
	}
}

func TestCreateRejectsDuplicateAndUnsafeIDs(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("same"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("same"); !errors.Is(err, storage.ErrRepositoryExists) {
		t.Fatalf("duplicate Create error = %v", err)
	}

	for _, id := range []string{"", ".", "..", "../escape", "nested/repo", "with space"} {
		if _, err := store.Create(id); !errors.Is(err, storage.ErrInvalidID) {
			t.Errorf("Create(%q) error = %v", id, err)
		}
	}
}

func TestOpenDistinguishesMissingAndInvalidRepositories(t *testing.T) {
	root := t.TempDir()
	store, err := storage.New(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Open("missing"); !errors.Is(err, storage.ErrRepositoryNotFound) {
		t.Fatalf("missing Open error = %v", err)
	}

	if err := os.Mkdir(filepath.Join(root, "broken.git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Open("broken"); !errors.Is(err, storage.ErrInvalidRepository) {
		t.Fatalf("invalid Open error = %v", err)
	}
}

func TestOpenParsesRepositoryFormat(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		id          string
		replacement string
		valid       bool
	}{
		{id: "quoted", replacement: `repositoryformatversion = "0"`, valid: true},
		{id: "commented", replacement: "repositoryformatversion = 0 # version", valid: true},
		{id: "section-comment", replacement: "repositoryformatversion = 0", valid: true},
		{id: "unsupported", replacement: "repositoryformatversion = 999", valid: false},
		{id: "missing", replacement: "", valid: false},
	} {
		repo, err := store.Create(test.id)
		if err != nil {
			t.Fatal(err)
		}
		configPath := filepath.Join(repo.Path(), "config")
		config, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatal(err)
		}
		mutated := strings.Replace(string(config), "repositoryformatversion = 0", test.replacement, 1)
		if test.id == "section-comment" {
			mutated = strings.Replace(mutated, "[core]", "[core] # repository settings", 1)
		}
		if err := os.WriteFile(configPath, []byte(mutated), 0o644); err != nil {
			t.Fatal(err)
		}

		_, err = store.Open(test.id)
		if test.valid && err != nil {
			t.Errorf("Open(%q) rejected valid Git config: %v", test.id, err)
		}
		if !test.valid && !errors.Is(err, storage.ErrInvalidRepository) {
			t.Errorf("Open(%q) error = %v, want ErrInvalidRepository", test.id, err)
		}
	}
}
