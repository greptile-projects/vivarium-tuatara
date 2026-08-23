// Package activities stores an append-only account-facing record of meaningful
// collaboration changes. Events retain stable attribution and resource IDs.
package activities

import (
	"crypto/rand"
	"crypto/sha256"
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
)

var ErrInvalid = errors.New("invalid activity event")

type Event struct {
	ID               string    `json:"id"`
	Kind             string    `json:"kind"`
	ActorID          string    `json:"actor_id"`
	RepositoryID     string    `json:"repository_id"`
	RepositoryName   string    `json:"repository_name"`
	ResourceType     string    `json:"resource_type"`
	ResourceID       string    `json:"resource_id"`
	ResourceTitle    string    `json:"resource_title"`
	ResourceRevision string    `json:"resource_revision,omitempty"`
	TargetUserID     *string   `json:"target_user_id"`
	CreatedAt        time.Time `json:"created_at"`
}

type Store struct {
	root     string
	mu       sync.Mutex
	now      func() time.Time
	observer func(Event) error
}

// SetObserver projects newly durable events into an optional downstream
// delivery ledger. The immutable activity write remains the source of truth.
func (s *Store) SetObserver(observer func(Event) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.observer = observer
}

func New(root string) (*Store, error) {
	if root == "" {
		return nil, errors.New("activity storage root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, fmt.Errorf("create activity store: %w", err)
	}
	return &Store{root: abs, now: func() time.Time { return time.Now().UTC() }}, nil
}

// Cleared returns the event IDs the user has removed from their inbox. Inbox
// state is kept beside, but separate from, the immutable activity records.
func (s *Store) Cleared(userID string) (map[string]bool, error) {
	if !validID(userID) {
		return nil, ErrInvalid
	}
	directory := filepath.Join(s.root, ".inbox", userID)
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]bool{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := make(map[string]bool, len(entries))
	for _, entry := range entries {
		id, ok := strings.CutSuffix(entry.Name(), ".cleared")
		if !entry.IsDir() && ok && validID(id) {
			result[id] = true
		}
	}
	return result, nil
}

// Clear durably marks one immutable activity event as handled for a user.
func (s *Store) Clear(userID, eventID string) error {
	if !validID(userID) || !validID(eventID) {
		return ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	parent := filepath.Join(s.root, ".inbox")
	directory := filepath.Join(parent, userID)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	path := filepath.Join(directory, eventID+".cleared")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if err = file.Sync(); err == nil {
		err = file.Close()
	} else {
		_ = file.Close()
	}
	if err == nil {
		err = syncDirectory(directory)
	}
	if err == nil {
		err = syncDirectory(parent)
	}
	if err == nil {
		err = syncDirectory(s.root)
	}
	return err
}

func (s *Store) Append(event Event) (Event, error) {
	if !validID(event.ActorID) || !validID(event.RepositoryID) || event.Kind == "" || event.ResourceType == "" || event.ResourceID == "" || strings.TrimSpace(event.RepositoryName) == "" || strings.TrimSpace(event.ResourceTitle) == "" {
		return Event{}, ErrInvalid
	}
	if event.TargetUserID != nil && !validID(*event.TargetUserID) {
		return Event{}, ErrInvalid
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return Event{}, err
	}
	event.ID = hex.EncodeToString(b)
	return s.append(event)
}

// AppendOnce uses a stable caller-owned key to make a collaboration side
// effect idempotent across recovery after an uncertain completion boundary.
func (s *Store) AppendOnce(key string, event Event) (Event, error) {
	if strings.TrimSpace(key) == "" {
		return Event{}, ErrInvalid
	}
	sum := sha256.Sum256([]byte(key))
	event.ID = hex.EncodeToString(sum[:16])
	return s.append(event)
}

func (s *Store) append(event Event) (Event, error) {
	if !validID(event.ActorID) || !validID(event.RepositoryID) || !validID(event.ID) || event.Kind == "" || event.ResourceType == "" || event.ResourceID == "" || strings.TrimSpace(event.RepositoryName) == "" || strings.TrimSpace(event.ResourceTitle) == "" {
		return Event{}, ErrInvalid
	}
	event.CreatedAt = s.now().Truncate(time.Microsecond)
	data, err := json.Marshal(event)
	if err != nil {
		return Event{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return Event{}, err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return Event{}, err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	finalPath := filepath.Join(s.root, event.ID+".json")
	if existing, readErr := os.ReadFile(finalPath); readErr == nil {
		var stored Event
		if json.Unmarshal(existing, &stored) != nil {
			return Event{}, errors.New("invalid activity event record")
		}
		return stored, nil
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return Event{}, readErr
	}
	tmp, err := os.CreateTemp(s.root, ".event-*")
	if err != nil {
		return Event{}, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err = tmp.Chmod(0o600); err == nil {
		_, err = tmp.Write(data)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(tmpName, finalPath)
	}
	if err == nil {
		err = syncDirectory(s.root)
	}
	if err != nil {
		return Event{}, err
	}
	if s.observer != nil {
		if err := s.observer(event); err != nil {
			return Event{}, fmt.Errorf("project durable activity: %w", err)
		}
	}
	return event, nil
}

func (s *Store) List() ([]Event, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	result := []Event{}
	for _, entry := range entries {
		id, ok := strings.CutSuffix(entry.Name(), ".json")
		if entry.IsDir() || !ok || !validID(id) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.root, entry.Name()))
		if err != nil {
			return nil, err
		}
		var event Event
		if json.Unmarshal(data, &event) != nil || event.ID != id || !validID(event.ActorID) || !validID(event.RepositoryID) {
			return nil, fmt.Errorf("corrupt activity event %s", id)
		}
		result = append(result, event)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID > result[j].ID
		}
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result, nil
}

// Get resolves one immutable, server-issued collaboration event by delivery ID.
func (s *Store) Get(id string) (Event, error) {
	if !validID(id) {
		return Event{}, ErrInvalid
	}
	data, err := os.ReadFile(filepath.Join(s.root, id+".json"))
	if err != nil {
		return Event{}, err
	}
	var event Event
	if json.Unmarshal(data, &event) != nil || event.ID != id || !validID(event.ActorID) || !validID(event.RepositoryID) {
		return Event{}, fmt.Errorf("corrupt activity event %s", id)
	}
	return event, nil
}

func validID(value string) bool {
	if len(value) != 32 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && value == strings.ToLower(value)
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
