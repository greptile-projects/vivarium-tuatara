// Package storage provides the durable boundary around Git repositories.
package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const defaultBranch = "main"

var (
	// ErrInvalidID indicates that an identifier cannot safely name a repository.
	ErrInvalidID = errors.New("invalid repository id")
	// ErrRepositoryExists indicates that Create was called for an existing ID.
	ErrRepositoryExists = errors.New("repository already exists")
	// ErrRepositoryNotFound indicates that no repository exists for an ID.
	ErrRepositoryNotFound = errors.New("repository not found")
	// ErrInvalidRepository indicates that an ID exists but is not a repository.
	ErrInvalidRepository = errors.New("invalid repository")

	validID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
)

// Store owns bare Git repositories below a filesystem directory.
type Store struct {
	root string
}

// Repository identifies an opened bare Git repository.
type Repository struct {
	id   string
	path string
}

// Info is a validated snapshot of repository metadata.
type Info struct {
	ID            string
	DefaultBranch string
	Bare          bool
	Empty         bool
}

// New returns a filesystem-backed repository store. The root is created when
// the first repository is created.
func New(root string) (*Store, error) {
	if root == "" {
		return nil, fmt.Errorf("storage root: %w", ErrInvalidRepository)
	}

	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve storage root: %w", err)
	}
	return &Store{root: abs}, nil
}

// Create atomically initializes and opens an empty bare Git repository.
func (s *Store) Create(id string) (*Repository, error) {
	path, err := s.pathFor(id)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		return nil, fmt.Errorf("create storage root: %w", err)
	}

	temp, err := os.MkdirTemp(s.root, ".creating-")
	if err != nil {
		return nil, fmt.Errorf("create repository staging directory: %w", err)
	}
	defer os.RemoveAll(temp)

	if err := initializeBareRepository(temp); err != nil {
		return nil, err
	}
	if err := os.Rename(temp, path); err != nil {
		if _, statErr := os.Stat(path); statErr == nil {
			return nil, fmt.Errorf("%s: %w", id, ErrRepositoryExists)
		}
		return nil, fmt.Errorf("publish repository: %w", err)
	}

	return s.Open(id)
}

// Open reopens an existing repository and verifies its storage boundary.
func (s *Store) Open(id string) (*Repository, error) {
	path, err := s.pathFor(id)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%s: %w", id, ErrRepositoryNotFound)
		}
		return nil, fmt.Errorf("open repository: %w", err)
	}

	repo := &Repository{id: id, path: path}
	if _, err := repo.Inspect(); err != nil {
		return nil, err
	}
	return repo, nil
}

// ID returns the stable identifier assigned by the store.
func (r *Repository) ID() string { return r.id }

// Path returns the absolute bare-repository path for Git storage operations.
func (r *Repository) Path() string { return r.path }

// Inspect validates the bare repository and reports its lifecycle metadata.
func (r *Repository) Inspect() (Info, error) {
	head, err := os.ReadFile(filepath.Join(r.path, "HEAD"))
	if err != nil {
		return Info{}, invalidRepository("read HEAD", err)
	}
	const prefix = "ref: refs/heads/"
	headValue := strings.TrimSpace(string(head))
	if !strings.HasPrefix(headValue, prefix) || len(headValue) == len(prefix) {
		return Info{}, invalidRepository("HEAD is not a branch reference", nil)
	}

	config, err := os.ReadFile(filepath.Join(r.path, "config"))
	if err != nil {
		return Info{}, invalidRepository("read config", err)
	}
	compactConfig := strings.ToLower(strings.Join(strings.Fields(string(config)), ""))
	if !strings.Contains(compactConfig, "bare=true") {
		return Info{}, invalidRepository("core.bare is not true", nil)
	}

	for _, directory := range []string{"objects", "refs"} {
		info, err := os.Stat(filepath.Join(r.path, directory))
		if err != nil || !info.IsDir() {
			return Info{}, invalidRepository("missing "+directory+" directory", err)
		}
	}

	empty, err := directoryEmpty(filepath.Join(r.path, "objects"))
	if err != nil {
		return Info{}, invalidRepository("inspect objects", err)
	}
	return Info{
		ID:            r.id,
		DefaultBranch: strings.TrimPrefix(headValue, prefix),
		Bare:          true,
		Empty:         empty,
	}, nil
}

func (s *Store) pathFor(id string) (string, error) {
	if !validID.MatchString(id) || id == "." || id == ".." {
		return "", fmt.Errorf("%q: %w", id, ErrInvalidID)
	}
	return filepath.Join(s.root, id+".git"), nil
}

func initializeBareRepository(path string) error {
	directories := []string{
		"branches", "hooks", "info", "objects/info", "objects/pack",
		"refs/heads", "refs/tags",
	}
	for _, directory := range directories {
		if err := os.MkdirAll(filepath.Join(path, directory), 0o755); err != nil {
			return fmt.Errorf("initialize repository: %w", err)
		}
	}

	files := map[string]string{
		"HEAD":        "ref: refs/heads/" + defaultBranch + "\n",
		"config":      "[core]\n\trepositoryformatversion = 0\n\tfilemode = true\n\tbare = true\n\tlogallrefupdates = false\n",
		"description": "Unnamed repository; edit this file to name the repository.\n",
	}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(path, name), []byte(contents), 0o644); err != nil {
			return fmt.Errorf("initialize repository: %w", err)
		}
	}
	return nil
}

func directoryEmpty(path string) (bool, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if entry.Name() != "info" && entry.Name() != "pack" {
			return false, nil
		}
		children, err := os.ReadDir(filepath.Join(path, entry.Name()))
		if err != nil {
			return false, err
		}
		if len(children) != 0 {
			return false, nil
		}
	}
	return true, nil
}

func invalidRepository(message string, err error) error {
	if err == nil {
		return fmt.Errorf("%s: %w", message, ErrInvalidRepository)
	}
	return fmt.Errorf("%s: %w: %v", message, ErrInvalidRepository, err)
}
