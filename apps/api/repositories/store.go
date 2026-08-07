// Package repositories connects application ownership metadata to durable Git storage.
package repositories

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

var (
	ErrNotFound    = errors.New("repository not found")
	ErrNameTaken   = errors.New("repository name is already in use")
	ErrInvalidName = errors.New("invalid repository name")
)

type Repository struct {
	ID            string    `json:"id"`
	OwnerID       string    `json:"owner_id"`
	Name          string    `json:"name"`
	DefaultBranch string    `json:"default_branch"`
	GitRemote     string    `json:"git_remote"`
	CreatedAt     time.Time `json:"created_at"`
}

type gitStore interface {
	Create(string) (*storage.Repository, error)
	Open(string) (*storage.Repository, error)
	Delete(string) error
}

type Store struct {
	root   string
	git    gitStore
	mu     sync.Mutex
	now    func() time.Time
	remove func(string) error
}

func New(root string, git *storage.Store) (*Store, error) {
	if root == "" || git == nil {
		return nil, errors.New("repository catalog requires metadata and Git storage")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, fmt.Errorf("create repository catalog: %w", err)
	}
	return &Store{root: abs, git: git, now: func() time.Time { return time.Now().UTC() }, remove: os.Remove}, nil
}

func (s *Store) Create(ownerID, name string) (Repository, error) {
	name, err := validateName(name)
	if err != nil {
		return Repository{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return Repository{}, err
	}
	defer unlock()
	all, err := s.loadActive()
	if err != nil {
		return Repository{}, err
	}
	for _, repository := range all {
		if repository.OwnerID == ownerID && strings.EqualFold(repository.Name, name) {
			return Repository{}, ErrNameTaken
		}
	}
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return Repository{}, err
	}
	id := hex.EncodeToString(idBytes)
	if _, err := s.git.Create(id); err != nil {
		return Repository{}, fmt.Errorf("create Git repository: %w", err)
	}
	repository := Repository{ID: id, OwnerID: ownerID, Name: name, DefaultBranch: "main", GitRemote: "/git/" + id + ".git", CreatedAt: s.now().Truncate(time.Microsecond)}
	if err := s.write(repository); err != nil {
		if persisted, readErr := s.read(id); readErr == nil && persisted == repository {
			return repository, nil
		}
		if deleteErr := s.git.Delete(id); deleteErr != nil {
			return Repository{}, fmt.Errorf("publish repository metadata: %v; rollback Git repository: %w", err, deleteErr)
		}
		return Repository{}, err
	}
	return repository, nil
}

func (s *Store) Get(ownerID, id string) (Repository, error) {
	repository, err := s.read(id)
	if err != nil || repository.OwnerID != ownerID {
		return Repository{}, ErrNotFound
	}
	if _, err := s.git.Open(id); err != nil {
		if errors.Is(err, storage.ErrRepositoryNotFound) {
			return Repository{}, ErrNotFound
		}
		return Repository{}, fmt.Errorf("open Git repository: %w", err)
	}
	return repository, nil
}

func (s *Store) List(ownerID string) ([]Repository, error) {
	all, err := s.loadActive()
	if err != nil {
		return nil, err
	}
	result := []Repository{}
	for _, repository := range all {
		if repository.OwnerID == ownerID {
			result = append(result, repository)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result, nil
}

func (s *Store) Delete(ownerID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return err
	}
	defer unlock()
	repository, err := s.read(id)
	if err != nil || repository.OwnerID != ownerID {
		return ErrNotFound
	}
	if err := s.git.Delete(id); err != nil {
		// Git storage retains a stable tombstone after post-detach cleanup
		// failures. Preserve ownership metadata so an authenticated retry can
		// invoke Delete again and finish that cleanup.
		return fmt.Errorf("delete Git repository: %w", err)
	}
	if err := s.remove(s.path(id)); err != nil {
		return fmt.Errorf("delete repository metadata: %w", err)
	}
	return syncDirectory(s.root)
}

func validateName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 100 || name == "." || name == ".." || strings.ContainsAny(name, "\x00/\\\r\n") {
		return "", ErrInvalidName
	}
	return name, nil
}

func (s *Store) path(id string) string { return filepath.Join(s.root, id+".json") }

func (s *Store) read(id string) (Repository, error) {
	if !validID(id) {
		return Repository{}, ErrNotFound
	}
	data, err := os.ReadFile(s.path(id))
	if errors.Is(err, os.ErrNotExist) {
		return Repository{}, ErrNotFound
	}
	if err != nil {
		return Repository{}, err
	}
	var repository Repository
	if json.Unmarshal(data, &repository) != nil || repository.ID != id || !validID(repository.OwnerID) || repository.GitRemote != "/git/"+id+".git" || repository.DefaultBranch != "main" {
		return Repository{}, fmt.Errorf("corrupt repository metadata %s", id)
	}
	return repository, nil
}

func (s *Store) loadAll() ([]Repository, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	result := []Repository{}
	for _, entry := range entries {
		id, ok := strings.CutSuffix(entry.Name(), ".json")
		if entry.IsDir() || !ok || !validID(id) {
			continue
		}
		repository, err := s.read(id)
		if err != nil {
			return nil, err
		}
		result = append(result, repository)
	}
	return result, nil
}

// loadActive reconciles the catalog with the Git lifecycle boundary. A Git
// repository is atomically detached before its metadata is removed, so a
// retained record after an interrupted cleanup represents a completed delete,
// not an active remote. The record remains available to a later Delete retry.
func (s *Store) loadActive() ([]Repository, error) {
	all, err := s.loadAll()
	if err != nil {
		return nil, err
	}
	active := make([]Repository, 0, len(all))
	for _, repository := range all {
		if _, err := s.git.Open(repository.ID); err != nil {
			if errors.Is(err, storage.ErrRepositoryNotFound) {
				continue
			}
			return nil, fmt.Errorf("open Git repository %s: %w", repository.ID, err)
		}
		active = append(active, repository)
	}
	return active, nil
}

func (s *Store) write(repository Repository) error {
	data, err := json.Marshal(repository)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(s.root, ".writing-")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err == nil {
		_, err = temp.Write(append(data, '\n'))
	}
	if err == nil {
		err = temp.Sync()
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := os.Rename(tempPath, s.path(repository.ID)); err != nil {
		return err
	}
	return syncDirectory(s.root)
}

func (s *Store) lockRoot() (func(), error) {
	file, err := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		file.Close()
		return nil, err
	}
	return func() { _ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN); _ = file.Close() }, nil
}

func validID(id string) bool {
	if len(id) != 32 || id != strings.ToLower(id) {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
