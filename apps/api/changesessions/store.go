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
	"strings"
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
const Launched = "launched"

var workEventKinds = map[string]bool{
	"run.status": true, "agent.message": true, "tool.action": true,
	"artifact.produced": true, "run.failed": true, "branch.updated": true,
}

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
	ID          string    `json:"id"`
	SessionID   string    `json:"session_id"`
	Kind        string    `json:"kind"`
	ActorID     string    `json:"actor_id"`
	State       string    `json:"state"`
	RunID       string    `json:"run_id,omitempty"`
	InitiatorID string    `json:"initiator_id,omitempty"`
	AgentID     string    `json:"agent_id,omitempty"`
	RevisionID  string    `json:"revision_id,omitempty"`
	Message     string    `json:"message,omitempty"`
	Tool        string    `json:"tool,omitempty"`
	Artifact    string    `json:"artifact,omitempty"`
	Branch      string    `json:"branch,omitempty"`
	CommitID    string    `json:"commit_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

type Run struct {
	ID                  string     `json:"id"`
	SessionID           string     `json:"session_id"`
	InitiatorID         string     `json:"initiator_id"`
	AgentID             string     `json:"agent_id"`
	Instructions        string     `json:"instructions"`
	SourceCommitID      string     `json:"source_commit_id"`
	ContextPaths        []string   `json:"context_paths"`
	WorkingBranch       string     `json:"working_branch"`
	CredentialID        string     `json:"credential_id"`
	CredentialExpiresAt time.Time  `json:"credential_expires_at"`
	AccessRevokedAt     *time.Time `json:"access_revoked_at,omitempty"`
	State               string     `json:"state"`
	CreatedAt           time.Time  `json:"created_at"`
}

type record struct {
	Session Session `json:"session"`
	Events  []Event `json:"events"`
	Runs    []Run   `json:"runs,omitempty"`
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

func (s *Store) LaunchRun(repositoryID, pullRequestID, sessionID, initiatorID, instructions, sourceCommitID string, contextPaths []string, workingBranch, credentialID string, credentialExpiresAt time.Time) (Run, error) {
	if !validID(initiatorID) || !validObjectID(sourceCommitID) || !validID(credentialID) || instructions == "" || workingBranch == "" || credentialExpiresAt.IsZero() {
		return Run{}, ErrInvalid
	}
	runID, err := newID()
	if err != nil {
		return Run{}, err
	}
	eventID, err := newID()
	if err != nil {
		return Run{}, err
	}
	agentID, err := newID()
	if err != nil {
		return Run{}, err
	}
	now := s.now().Truncate(time.Microsecond)
	run := Run{ID: runID, SessionID: sessionID, InitiatorID: initiatorID, AgentID: agentID, Instructions: instructions, SourceCommitID: sourceCommitID, ContextPaths: append([]string(nil), contextPaths...), WorkingBranch: workingBranch, CredentialID: credentialID, CredentialExpiresAt: credentialExpiresAt, State: Launched, CreatedAt: now}

	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Run{}, err
	}
	defer unlock()
	rec, err := s.read(repositoryID, pullRequestID, sessionID)
	if err != nil {
		return Run{}, err
	}
	if rec.Session.SourceCommitID != sourceCommitID {
		return Run{}, ErrInvalid
	}
	rec.Runs = append(rec.Runs, run)
	rec.Events = append(rec.Events, Event{ID: eventID, SessionID: sessionID, Kind: "run.launched", ActorID: initiatorID, InitiatorID: initiatorID, AgentID: agentID, RevisionID: sourceCommitID, State: Launched, RunID: runID, CreatedAt: now})
	rec.Session.UpdatedAt = now
	committed, err := s.write(filepath.Join(s.root, repositoryID, pullRequestID), rec)
	if err != nil {
		if committed {
			return run, fmt.Errorf("%w: %v", ErrDurabilityUncertain, err)
		}
		return Run{}, err
	}
	return run, nil
}

// AppendWorkEvent publishes agent-reported progress through the durable session boundary.
func (s *Store) AppendWorkEvent(repositoryID, pullRequestID, sessionID, runID, credentialID, kind, state, message, tool, artifact, branch, commitID string) (Event, error) {
	if !workEventKinds[kind] || strings.TrimSpace(state) == "" || len(state) > 100 || len([]rune(message)) > 10000 || len(tool) > 200 || len(artifact) > 1000 || len(branch) > 200 || (commitID != "" && !validObjectID(commitID)) {
		return Event{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Event{}, err
	}
	defer unlock()
	rec, err := s.read(repositoryID, pullRequestID, sessionID)
	if err != nil {
		return Event{}, err
	}
	var run *Run
	for i := range rec.Runs {
		if rec.Runs[i].ID == runID {
			run = &rec.Runs[i]
			break
		}
	}
	if run == nil || run.CredentialID != credentialID || run.AccessRevokedAt != nil {
		return Event{}, ErrNotFound
	}
	if strings.TrimSpace(message) == "" || (kind == "tool.action" && tool == "") || (kind == "artifact.produced" && artifact == "") || (kind == "branch.updated" && (branch != run.WorkingBranch || commitID == "")) {
		return Event{}, ErrInvalid
	}
	eventID, err := newID()
	if err != nil {
		return Event{}, err
	}
	now := s.now().Truncate(time.Microsecond)
	event := Event{ID: eventID, SessionID: sessionID, Kind: kind, ActorID: run.InitiatorID, InitiatorID: run.InitiatorID, AgentID: run.AgentID, RevisionID: run.SourceCommitID, State: state, RunID: run.ID, Message: message, Tool: tool, Artifact: artifact, Branch: branch, CommitID: commitID, CreatedAt: now}
	rec.Events = append(rec.Events, event)
	rec.Session.UpdatedAt = now
	committed, err := s.write(filepath.Join(s.root, repositoryID, pullRequestID), rec)
	if err != nil {
		if committed {
			return event, fmt.Errorf("%w: %v", ErrDurabilityUncertain, err)
		}
		return Event{}, err
	}
	return event, nil
}

func (s *Store) ListRuns(repositoryID, pullRequestID, sessionID string) ([]Run, error) {
	if _, err := s.Get(repositoryID, pullRequestID, sessionID); err != nil {
		return nil, err
	}
	rec, err := s.read(repositoryID, pullRequestID, sessionID)
	if err != nil {
		return nil, err
	}
	return append([]Run(nil), rec.Runs...), nil
}

func (s *Store) RevokeRunAccess(repositoryID, pullRequestID, sessionID, runID string) (Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Run{}, err
	}
	defer unlock()
	rec, err := s.read(repositoryID, pullRequestID, sessionID)
	if err != nil {
		return Run{}, err
	}
	index := -1
	for i := range rec.Runs {
		if rec.Runs[i].ID == runID {
			index = i
			break
		}
	}
	if index < 0 {
		return Run{}, ErrNotFound
	}
	if rec.Runs[index].AccessRevokedAt == nil {
		now := s.now().Truncate(time.Microsecond)
		rec.Runs[index].AccessRevokedAt = &now
		rec.Session.UpdatedAt = now
		if committed, err := s.write(filepath.Join(s.root, repositoryID, pullRequestID), rec); err != nil {
			if committed {
				return rec.Runs[index], fmt.Errorf("%w: %v", ErrDurabilityUncertain, err)
			}
			return Run{}, err
		}
	}
	return rec.Runs[index], nil
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
		if !validID(event.ID) || event.SessionID != sessionID || !validID(event.ActorID) || event.Kind == "" || event.State == "" || (event.Kind == "run.launched" && !validID(event.RunID)) || (workEventKinds[event.Kind] && (!validID(event.RunID) || !validID(event.InitiatorID) || !validID(event.AgentID) || !validObjectID(event.RevisionID))) {
			return record{}, errors.New("invalid durable change session event")
		}
	}
	runEvents := map[string]int{}
	for _, event := range rec.Events {
		if event.Kind == "run.launched" {
			runEvents[event.RunID]++
		}
	}
	for i := range rec.Runs {
		run := &rec.Runs[i]
		// Runs created before agent identities were introduced use their unique
		// credential identity as a stable compatibility identity.
		if run.AgentID == "" && validID(run.CredentialID) {
			run.AgentID = run.CredentialID
		}
		if !validID(run.ID) || run.SessionID != sessionID || !validID(run.InitiatorID) || !validID(run.AgentID) || !validObjectID(run.SourceCommitID) || run.SourceCommitID != rec.Session.SourceCommitID || run.Instructions == "" || run.WorkingBranch == "" || !validID(run.CredentialID) || run.State != Launched || run.CredentialExpiresAt.IsZero() || runEvents[run.ID] != 1 {
			return record{}, errors.New("invalid durable agent run")
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
