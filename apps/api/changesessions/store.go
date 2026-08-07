// Package changesessions stores durable, pull-request-scoped agent collaboration workspaces.
package changesessions

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"syscall"
	"time"
)

var (
	ErrNotFound            = errors.New("change session not found")
	ErrInvalid             = errors.New("invalid change session")
	ErrDurabilityUncertain = errors.New("change session is visible but durability is uncertain")
)

const Open = "open"

type Session struct {
	ID             string    `json:"id"`
	RepositoryID   string    `json:"repository_id"`
	PullRequestID  string    `json:"pull_request_id"`
	InitiatorID    string    `json:"initiator_id"`
	SourceCommitID string    `json:"source_commit_id"`
	State          string    `json:"state"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type Event struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	Kind      string    `json:"kind"`
	ActorID   string    `json:"actor_id"`
	State     string    `json:"state"`
	CreatedAt time.Time `json:"created_at"`
}

type record struct {
	Session Session `json:"session"`
	Events  []Event `json:"events"`
}

type Store struct {
	root          string
	mu            sync.Mutex
	now           func() time.Time
	directorySync func(string) error
}

func New(root string) (*Store, error) {
	if root == "" {
		return nil, errors.New("change session storage root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, fmt.Errorf("create change session store: %w", err)
	}
	return &Store{root: abs, now: func() time.Time { return time.Now().UTC() }, directorySync: syncDirectory}, nil
}

func (s *Store) Create(repositoryID, pullRequestID, initiatorID, sourceCommitID string) (Session, error) {
	if !validID(repositoryID) || !validID(pullRequestID) || !validID(initiatorID) || !validObjectID(sourceCommitID) {
		return Session{}, ErrInvalid
	}
	sessionID, err := newID()
	if err != nil {
		return Session{}, err
	}
	eventID, err := newID()
	if err != nil {
		return Session{}, err
	}
	now := s.now().Truncate(time.Microsecond)
	session := Session{ID: sessionID, RepositoryID: repositoryID, PullRequestID: pullRequestID, InitiatorID: initiatorID, SourceCommitID: sourceCommitID, State: Open, CreatedAt: now, UpdatedAt: now}
	rec := record{Session: session, Events: []Event{{ID: eventID, SessionID: sessionID, Kind: "session.opened", ActorID: initiatorID, State: Open, CreatedAt: now}}}

	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Session{}, err
	}
	defer unlock()
	directory := filepath.Join(s.root, repositoryID, pullRequestID)
	if err := s.ensureDirectory(repositoryID, pullRequestID); err != nil {
		return Session{}, fmt.Errorf("create change session directory: %w", err)
	}
	committed, err := s.write(directory, rec)
	if err != nil {
		if committed {
			return session, fmt.Errorf("%w: %v", ErrDurabilityUncertain, err)
		}
		return Session{}, err
	}
	return session, nil
}

func (s *Store) ensureDirectory(repositoryID, pullRequestID string) error {
	repositoryDirectory := filepath.Join(s.root, repositoryID)
	if err := mkdirAndSyncParent(repositoryDirectory, s.root); err != nil {
		return err
	}
	return mkdirAndSyncParent(filepath.Join(repositoryDirectory, pullRequestID), repositoryDirectory)
}

func mkdirAndSyncParent(path, parent string) error {
	err := os.Mkdir(path, 0o700)
	if err == nil {
		return syncDirectory(parent)
	}
	if errors.Is(err, os.ErrExist) {
		info, statErr := os.Stat(path)
		if statErr != nil {
			return statErr
		}
		if !info.IsDir() {
			return fmt.Errorf("%s is not a directory", path)
		}
		return nil
	}
	return err
}

func (s *Store) Get(repositoryID, pullRequestID, sessionID string) (Session, error) {
	rec, err := s.read(repositoryID, pullRequestID, sessionID)
	if err != nil {
		return Session{}, err
	}
	if err := s.directorySync(filepath.Join(s.root, repositoryID, pullRequestID)); err != nil {
		return rec.Session, fmt.Errorf("%w: %v", ErrDurabilityUncertain, err)
	}
	return rec.Session, nil
}

func (s *Store) List(repositoryID, pullRequestID string) ([]Session, error) {
	if !validID(repositoryID) || !validID(pullRequestID) {
		return nil, ErrNotFound
	}
	directory := filepath.Join(s.root, repositoryID, pullRequestID)
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return []Session{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list change sessions: %w", err)
	}
	if err := s.directorySync(directory); err != nil {
		return nil, fmt.Errorf("confirm change session directory: %w", err)
	}
	sessions := make([]Session, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := entry.Name()[:len(entry.Name())-len(".json")]
		rec, err := s.read(repositoryID, pullRequestID, id)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, rec.Session)
	}
	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].CreatedAt.Equal(sessions[j].CreatedAt) {
			return sessions[i].ID < sessions[j].ID
		}
		return sessions[i].CreatedAt.Before(sessions[j].CreatedAt)
	})
	return sessions, nil
}

func (s *Store) ListEvents(repositoryID, pullRequestID, sessionID string) ([]Event, error) {
	if _, err := s.Get(repositoryID, pullRequestID, sessionID); err != nil {
		return nil, err
	}
	rec, err := s.read(repositoryID, pullRequestID, sessionID)
	if err != nil {
		return nil, err
	}
	return append([]Event(nil), rec.Events...), nil
}

func (s *Store) read(repositoryID, pullRequestID, sessionID string) (record, error) {
	if !validID(repositoryID) || !validID(pullRequestID) || !validID(sessionID) {
		return record{}, ErrNotFound
	}
	data, err := os.ReadFile(filepath.Join(s.root, repositoryID, pullRequestID, sessionID+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return record{}, ErrNotFound
	}
	if err != nil {
		return record{}, fmt.Errorf("read change session: %w", err)
	}
	var rec record
	if json.Unmarshal(data, &rec) != nil || rec.Session.ID != sessionID || rec.Session.RepositoryID != repositoryID || rec.Session.PullRequestID != pullRequestID || !validID(rec.Session.InitiatorID) || !validObjectID(rec.Session.SourceCommitID) || rec.Session.State != Open || len(rec.Events) == 0 {
		return record{}, errors.New("invalid durable change session record")
	}
	for _, event := range rec.Events {
		if !validID(event.ID) || event.SessionID != sessionID || !validID(event.ActorID) || event.Kind == "" || event.State == "" {
			return record{}, errors.New("invalid durable change session event")
		}
	}
	return rec, nil
}

func (s *Store) write(directory string, rec record) (bool, error) {
	data, err := json.Marshal(rec)
	if err != nil {
		return false, err
	}
	temporary, err := os.CreateTemp(directory, ".session-*.tmp")
	if err != nil {
		return false, err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return false, err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return false, err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return false, err
	}
	if err := temporary.Close(); err != nil {
		return false, err
	}
	if err := os.Rename(name, filepath.Join(directory, rec.Session.ID+".json")); err != nil {
		return false, err
	}
	if err := s.directorySync(directory); err != nil {
		return true, err
	}
	return true, nil
}

func (s *Store) lock() (func(), error) {
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

func newID() (string, error) {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}

func validID(value string) bool {
	if len(value) != 32 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validObjectID(value string) bool {
	if len(value) != 40 {
		return false
	}
	_, err := hex.DecodeString(value)
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
