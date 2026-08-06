package storage_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

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
