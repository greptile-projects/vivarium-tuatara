// Package users provides durable storage for platform identities.
package users

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	ErrNotFound       = errors.New("user not found")
	ErrHandleTaken    = errors.New("handle is already in use")
	ErrInvalidProfile = errors.New("invalid user profile")
	handlePattern     = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,37}[a-z0-9])?$`)
)

// User is a durable human identity. ID and CreatedAt never change; Handle and
// DisplayName are profile data and can evolve without changing attribution.
type User struct {
	ID          string    `json:"id"`
	Handle      string    `json:"handle"`
	DisplayName string    `json:"display_name"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Store struct {
	root string
	mu   sync.Mutex
	now  func() time.Time
}

func New(root string) (*Store, error) {
	if root == "" {
		return nil, fmt.Errorf("user storage root is empty")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("create user storage: %w", err)
	}
	return &Store{root: root, now: func() time.Time { return time.Now().UTC() }}, nil
}

func (s *Store) Create(handle, displayName string) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return User{}, err
	}
	defer unlock()
	handle, displayName, err = validateProfile(handle, displayName)
	if err != nil {
		return User{}, err
	}
	users, err := s.loadAll()
	if err != nil {
		return User{}, err
	}
	if handleExists(users, handle, "") {
		return User{}, ErrHandleTaken
	}
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return User{}, fmt.Errorf("generate user ID: %w", err)
	}
	now := s.now().Truncate(time.Microsecond)
	user := User{ID: hex.EncodeToString(idBytes), Handle: handle, DisplayName: displayName, CreatedAt: now, UpdatedAt: now}
	if err := s.write(user); err != nil {
		return User{}, err
	}
	return user, nil
}

func (s *Store) Get(id string) (User, error) {
	if !validID(id) {
		return User{}, ErrNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read(id)
}

// Delete removes an identity durably. It is used to roll back account
// bootstrap when the initial credential cannot be issued; established account
// deletion remains outside the public API until repository ownership exists.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return err
	}
	defer unlock()
	if !validID(id) {
		return ErrNotFound
	}
	if err := os.Remove(filepath.Join(s.root, id+".json")); errors.Is(err, os.ErrNotExist) {
		return ErrNotFound
	} else if err != nil {
		return fmt.Errorf("remove user record: %w", err)
	}
	dir, err := os.Open(s.root)
	if err != nil {
		return fmt.Errorf("open user storage: %w", err)
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("sync user storage: %w", err)
	}
	return nil
}

// Update replaces the editable profile fields while preserving stable identity.
func (s *Store) Update(id, handle, displayName string) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return User{}, err
	}
	defer unlock()
	if !validID(id) {
		return User{}, ErrNotFound
	}
	handle, displayName, err = validateProfile(handle, displayName)
	if err != nil {
		return User{}, err
	}
	user, err := s.read(id)
	if err != nil {
		return User{}, err
	}
	users, err := s.loadAll()
	if err != nil {
		return User{}, err
	}
	if handleExists(users, handle, id) {
		return User{}, ErrHandleTaken
	}
	user.Handle = handle
	user.DisplayName = displayName
	user.UpdatedAt = s.now().Truncate(time.Microsecond)
	if err := s.write(user); err != nil {
		return User{}, err
	}
	return user, nil
}

// ProfilePatch contains optional profile changes. Patch performs the sparse
// read/merge/write as one storage-root transaction so concurrent edits cannot
// restore fields from a stale snapshot.
type ProfilePatch struct {
	Handle      *string
	DisplayName *string
}

func (s *Store) Patch(id string, patch ProfilePatch) (User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return User{}, err
	}
	defer unlock()
	if !validID(id) {
		return User{}, ErrNotFound
	}
	user, err := s.read(id)
	if err != nil {
		return User{}, err
	}
	if patch.Handle != nil {
		user.Handle = *patch.Handle
	}
	if patch.DisplayName != nil {
		user.DisplayName = *patch.DisplayName
	}
	user.Handle, user.DisplayName, err = validateProfile(user.Handle, user.DisplayName)
	if err != nil {
		return User{}, err
	}
	allUsers, err := s.loadAll()
	if err != nil {
		return User{}, err
	}
	if handleExists(allUsers, user.Handle, id) {
		return User{}, ErrHandleTaken
	}
	user.UpdatedAt = s.now().Truncate(time.Microsecond)
	if err := s.write(user); err != nil {
		return User{}, err
	}
	return user, nil
}

// lockRoot coordinates mutations among all Store instances and API processes
// that share a storage root. The lock file is durable metadata, not a user.
func (s *Store) lockRoot() (func(), error) {
	file, err := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open user storage lock: %w", err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		file.Close()
		return nil, fmt.Errorf("lock user storage: %w", err)
	}
	return func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
	}, nil
}

func validateProfile(handle, displayName string) (string, string, error) {
	handle = strings.ToLower(strings.TrimSpace(handle))
	displayName = strings.TrimSpace(displayName)
	if !handlePattern.MatchString(handle) {
		return "", "", fmt.Errorf("%w: handle must be 1-39 lowercase letters, numbers, or single hyphen-separated words", ErrInvalidProfile)
	}
	if displayName == "" || len([]rune(displayName)) > 100 || strings.ContainsAny(displayName, "\x00\r\n") {
		return "", "", fmt.Errorf("%w: display_name must be 1-100 characters on one line", ErrInvalidProfile)
	}
	return handle, displayName, nil
}

func validID(id string) bool {
	if len(id) != 32 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil && id == strings.ToLower(id)
}

func (s *Store) read(id string) (User, error) {
	data, err := os.ReadFile(filepath.Join(s.root, id+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("read user: %w", err)
	}
	var user User
	if err := json.Unmarshal(data, &user); err != nil || user.ID != id {
		return User{}, fmt.Errorf("read user %s: corrupt record", id)
	}
	return user, nil
}

func (s *Store) loadAll() ([]User, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	users := make([]User, 0, len(entries))
	for _, entry := range entries {
		id, ok := strings.CutSuffix(entry.Name(), ".json")
		if entry.IsDir() || !ok || !validID(id) {
			continue
		}
		user, err := s.read(id)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	sort.Slice(users, func(i, j int) bool { return users[i].ID < users[j].ID })
	return users, nil
}

func handleExists(users []User, handle, exceptID string) bool {
	for _, user := range users {
		if user.ID != exceptID && user.Handle == handle {
			return true
		}
	}
	return false
}

func (s *Store) write(user User) error {
	data, err := json.Marshal(user)
	if err != nil {
		return fmt.Errorf("encode user: %w", err)
	}
	temp, err := os.CreateTemp(s.root, ".user-*.tmp")
	if err != nil {
		return fmt.Errorf("create user record: %w", err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(append(data, '\n')); err != nil {
		temp.Close()
		return fmt.Errorf("write user record: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("sync user record: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close user record: %w", err)
	}
	if err := os.Rename(tempName, filepath.Join(s.root, user.ID+".json")); err != nil {
		return fmt.Errorf("publish user record: %w", err)
	}
	dir, err := os.Open(s.root)
	if err != nil {
		return fmt.Errorf("open user storage: %w", err)
	}
	defer dir.Close()
	if err := dir.Sync(); err != nil {
		return fmt.Errorf("sync user storage: %w", err)
	}
	return nil
}
