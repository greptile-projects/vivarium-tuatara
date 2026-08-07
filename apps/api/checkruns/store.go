// Package checkruns persists and executes repository-defined verification for exact commits.
package checkruns

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	ErrNotFound            = errors.New("check run not found")
	ErrInvalidState        = errors.New("check run state does not allow this action")
	ErrDurabilityUncertain = errors.New("check run durability uncertain")
)

const ConfigPath = ".vivarium/checks.json"

type Definition struct {
	Name             string            `json:"name"`
	Image            string            `json:"image"`
	Command          string            `json:"command"`
	WorkingDirectory string            `json:"working_directory,omitempty"`
	Environment      map[string]string `json:"environment,omitempty"`
	TimeoutSeconds   int               `json:"timeout_seconds,omitempty"`
}

type Config struct {
	Version int          `json:"version"`
	Checks  []Definition `json:"checks"`
}

type Run struct {
	ID             string     `json:"id"`
	RepositoryID   string     `json:"repository_id"`
	PullRequestID  string     `json:"pull_request_id"`
	CommitID       string     `json:"commit_id"`
	Definition     Definition `json:"definition"`
	State          string     `json:"state"`
	ExitCode       *int       `json:"exit_code,omitempty"`
	Failure        string     `json:"failure,omitempty"`
	CleanupFailure string     `json:"cleanup_failure,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	Attempts       []Attempt  `json:"attempts"`
	Artifacts      []Artifact `json:"artifacts"`
	RequestedBy    string     `json:"requested_by,omitempty"`
	Controls       []Control  `json:"controls,omitempty"`
}

type Control struct {
	ID        string    `json:"id"`
	Action    string    `json:"action"`
	ActorID   string    `json:"actor_id"`
	Attempt   int       `json:"attempt"`
	CreatedAt time.Time `json:"created_at"`
}

type Attempt struct {
	Number      int        `json:"number"`
	State       string     `json:"state"`
	StartedAt   time.Time  `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	ExitCode    *int       `json:"exit_code,omitempty"`
	Failure     string     `json:"failure,omitempty"`
	ActorID     string     `json:"actor_id,omitempty"`
}

type Artifact struct {
	ID          string    `json:"id"`
	Attempt     int       `json:"attempt"`
	Path        string    `json:"path"`
	Size        int64     `json:"size"`
	SHA256      string    `json:"sha256"`
	ContentType string    `json:"content_type"`
	CreatedAt   time.Time `json:"created_at"`
}

type Event struct {
	Sequence  int64     `json:"sequence"`
	Attempt   int       `json:"attempt"`
	Kind      string    `json:"kind"`
	Timestamp time.Time `json:"timestamp"`
	State     string    `json:"state,omitempty"`
	Stream    string    `json:"stream,omitempty"`
	Message   string    `json:"message,omitempty"`
	ExitCode  *int      `json:"exit_code,omitempty"`
	Artifact  *Artifact `json:"artifact,omitempty"`
	ActorID   string    `json:"actor_id,omitempty"`
	ControlID string    `json:"control_id,omitempty"`
}

type Store struct {
	root          string
	mu            sync.Mutex
	now           func() time.Time
	syncDirectory func(*os.File) error
}

func New(root string) (*Store, error) {
	if root == "" {
		return nil, errors.New("check run storage root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, err
	}
	return &Store{root: abs, now: func() time.Time { return time.Now().UTC() }, syncDirectory: func(directory *os.File) error { return directory.Sync() }}, nil
}

func ParseConfig(data []byte) (Config, error) {
	var c Config
	d := json.NewDecoder(strings.NewReader(string(data)))
	d.DisallowUnknownFields()
	if err := d.Decode(&c); err != nil || c.Version != 1 || len(c.Checks) == 0 || len(c.Checks) > 20 {
		return Config{}, errors.New("invalid check configuration")
	}
	seen := map[string]bool{}
	for i := range c.Checks {
		x := &c.Checks[i]
		if strings.TrimSpace(x.Name) == "" || len(x.Name) > 100 || strings.TrimSpace(x.Command) == "" || len(x.Command) > 4000 || seen[x.Name] || x.TimeoutSeconds < 0 || x.TimeoutSeconds > 3600 {
			return Config{}, errors.New("invalid check definition")
		}
		if !validImage(x.Image) {
			return Config{}, errors.New("invalid check image")
		}
		if x.WorkingDirectory == "" {
			x.WorkingDirectory = "."
		}
		clean := filepath.Clean(x.WorkingDirectory)
		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return Config{}, errors.New("invalid check working directory")
		}
		x.WorkingDirectory = clean
		if x.TimeoutSeconds == 0 {
			x.TimeoutSeconds = 600
		}
		for k, v := range x.Environment {
			if k == "" || strings.Contains(k, "=") || k == "PATH" || k == "HOME" || strings.HasPrefix(k, "GIT_") || len(k)+len(v) > 4096 {
				return Config{}, errors.New("invalid check environment")
			}
		}
		seen[x.Name] = true
	}
	return c, nil
}

func validImage(image string) bool {
	if image == "" || len(image) > 200 || strings.ContainsAny(image, " \t\r\n@") {
		return false
	}
	for _, r := range image {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("./:_-", r)) {
			return false
		}
	}
	return true
}

func (s *Store) Create(repositoryID, pullRequestID, commitID string, definitions []Definition) ([]Run, error) {
	now := s.now().Truncate(time.Microsecond)
	runs := make([]Run, 0, len(definitions))
	for _, definition := range definitions {
		id, err := newID()
		if err != nil {
			return nil, err
		}
		runs = append(runs, Run{ID: id, RepositoryID: repositoryID, PullRequestID: pullRequestID, CommitID: commitID, Definition: definition, State: "queued", CreatedAt: now, Attempts: []Attempt{}, Artifacts: []Artifact{}})
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := filepath.Join(s.root, repositoryID, pullRequestID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var existingCommit bool
	var resumable []Run
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		body, readErr := os.ReadFile(filepath.Join(dir, entry.Name()))
		if readErr != nil {
			return nil, readErr
		}
		var existing Run
		if json.Unmarshal(body, &existing) != nil {
			return nil, errors.New("invalid check run record")
		}
		if existing.CommitID == commitID {
			existingCommit = true
			if nonterminal(existing.State) {
				resumable = append(resumable, existing)
			}
		}
	}
	if existingCommit {
		return resumable, nil
	}
	for _, run := range runs {
		if err := s.write(dir, run); err != nil && !errors.Is(err, ErrDurabilityUncertain) {
			return nil, err
		}
		if err := s.appendEventLocked(run, Event{Kind: "status", Timestamp: now, State: "queued"}); err != nil {
			return nil, err
		}
	}
	return runs, nil
}

func (s *Store) List(repositoryID, pullRequestID string) ([]Run, error) {
	dir := filepath.Join(s.root, repositoryID, pullRequestID)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []Run{}, nil
	}
	if err != nil {
		return nil, err
	}
	runs := make([]Run, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var r Run
		if json.Unmarshal(b, &r) != nil {
			return nil, errors.New("invalid check run record")
		}
		if r.Attempts == nil {
			r.Attempts = []Attempt{}
		}
		if r.Artifacts == nil {
			r.Artifacts = []Artifact{}
		}
		runs = append(runs, r)
	}
	sort.Slice(runs, func(i, j int) bool {
		if runs[i].CreatedAt.Equal(runs[j].CreatedAt) {
			return runs[i].ID < runs[j].ID
		}
		return runs[i].CreatedAt.Before(runs[j].CreatedAt)
	})
	return runs, nil
}

func (s *Store) Get(repositoryID, pullRequestID, runID string) (Run, error) {
	if !validID(runID) {
		return Run{}, ErrNotFound
	}
	body, err := os.ReadFile(filepath.Join(s.root, repositoryID, pullRequestID, runID+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return Run{}, ErrNotFound
	}
	if err != nil {
		return Run{}, err
	}
	var run Run
	if json.Unmarshal(body, &run) != nil || run.ID != runID || run.RepositoryID != repositoryID || run.PullRequestID != pullRequestID {
		return Run{}, errors.New("invalid check run record")
	}
	if run.Attempts == nil {
		run.Attempts = []Attempt{}
	}
	if run.Artifacts == nil {
		run.Artifacts = []Artifact{}
	}
	return run, nil
}

// Events returns immutable execution evidence after the supplied sequence.
func (s *Store) Events(repositoryID, pullRequestID, runID string, after int64) ([]Event, error) {
	run, err := s.Get(repositoryID, pullRequestID, runID)
	if err != nil {
		return nil, err
	}
	events, err := s.readEvents(run)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, event := range events {
		if event.ControlID != "" {
			seen[event.ControlID] = true
		}
	}
	for _, control := range run.Controls {
		if !seen[control.ID] {
			if err := s.appendEvent(run, Event{Attempt: control.Attempt, Kind: "control", Timestamp: control.CreatedAt, State: run.State, Message: control.Action, ActorID: control.ActorID, ControlID: control.ID}); err != nil {
				return nil, err
			}
		}
	}
	if len(run.Controls) > 0 {
		events, err = s.readEvents(run)
	}
	if err != nil {
		return nil, err
	}
	filtered := events[:0]
	for _, event := range events {
		if event.Sequence > after {
			filtered = append(filtered, event)
		}
	}
	return filtered, nil
}

func (s *Store) readEvents(run Run) ([]Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := os.Open(filepath.Join(s.root, run.RepositoryID, run.PullRequestID, run.ID+".events"))
	if errors.Is(err, os.ErrNotExist) {
		return []Event{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	body, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	events := []Event{}
	lines := strings.Split(string(body), "\n")
	for _, line := range lines[:len(lines)-1] {
		var event Event
		if json.Unmarshal([]byte(line), &event) != nil || event.Sequence < 1 {
			return nil, errors.New("invalid check evidence")
		}
		events = append(events, event)
	}
	return events, nil
}

func (s *Store) OpenArtifact(repositoryID, pullRequestID, runID, artifactID string) (*os.File, Artifact, error) {
	if !validID(artifactID) {
		return nil, Artifact{}, ErrNotFound
	}
	run, err := s.Get(repositoryID, pullRequestID, runID)
	if err != nil {
		return nil, Artifact{}, err
	}
	for _, artifact := range run.Artifacts {
		if artifact.ID == artifactID {
			file, err := os.Open(filepath.Join(s.root, repositoryID, pullRequestID, "artifacts", runID, artifactID))
			if errors.Is(err, os.ErrNotExist) {
				return nil, Artifact{}, errors.New("check artifact missing")
			}
			return file, artifact, err
		}
	}
	return nil, Artifact{}, ErrNotFound
}

func (s *Store) appendEvent(run Run, event Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.appendEventLocked(run, event)
}

func (s *Store) appendEventLocked(run Run, event Event) error {
	path := filepath.Join(s.root, run.RepositoryID, run.PullRequestID, run.ID+".events")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	sequence := int64(1)
	if info.Size() > 0 {
		if _, err = file.Seek(0, io.SeekStart); err != nil {
			return err
		}
		reader := bufio.NewReader(file)
		var completeBytes int64
		for {
			line, readErr := reader.ReadBytes('\n')
			if readErr == nil {
				sequence++
				completeBytes += int64(len(line))
				continue
			}
			if errors.Is(readErr, io.EOF) {
				if len(line) > 0 {
					if err = file.Truncate(completeBytes); err != nil {
						return err
					}
				}
				break
			}
			return readErr
		}
		if _, err = file.Seek(0, io.SeekEnd); err != nil {
			return err
		}
	}
	event.Sequence = sequence
	body, err := json.Marshal(event)
	if err == nil {
		_, err = file.Write(append(body, '\n'))
	}
	if err == nil {
		err = file.Sync()
	}
	return err
}

// Nonterminal returns durable work that must be relaunched after interruption.
func (s *Store) Nonterminal() ([]Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var runs []Run
	seen := map[string]bool{}
	err := filepath.WalkDir(s.root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if strings.HasSuffix(entry.Name(), ".cancel") {
			runPath := strings.TrimSuffix(path, ".cancel") + ".json"
			body, readErr := os.ReadFile(runPath)
			if readErr != nil {
				return readErr
			}
			var run Run
			if json.Unmarshal(body, &run) != nil {
				return errors.New("invalid check run record")
			}
			if !seen[run.ID] {
				runs, seen[run.ID] = append(runs, run), true
			}
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".json") {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var run Run
		if json.Unmarshal(body, &run) != nil {
			return errors.New("invalid check run record")
		}
		if nonterminal(run.State) && !seen[run.ID] {
			runs, seen[run.ID] = append(runs, run), true
		}
		return nil
	})
	return runs, err
}

func nonterminal(state string) bool {
	return state == "queued" || state == "running" || state == "cleanup_pending"
}

func (s *Store) Update(run Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.write(filepath.Join(s.root, run.RepositoryID, run.PullRequestID), run)
}

// Rerun queues another attempt on the same exact commit while retaining all
// prior attempts, evidence, and artifacts.
func (s *Store) Rerun(repositoryID, pullRequestID, runID, actorID string) (Run, error) {
	return s.control(repositoryID, pullRequestID, runID, actorID, "rerun")
}

// Cancel stops a nonterminal run and records the collaborator who requested it.
func (s *Store) Cancel(repositoryID, pullRequestID, runID, actorID string) (Run, error) {
	run, err := s.Get(repositoryID, pullRequestID, runID)
	if err != nil {
		return Run{}, err
	}
	if !nonterminal(run.State) {
		return Run{}, ErrInvalidState
	}
	control, err := newControl("cancel", actorID, len(run.Attempts), s.now().Truncate(time.Microsecond))
	if err != nil {
		return Run{}, err
	}
	if err := s.writeCancelIntent(run, control); err != nil {
		return Run{}, err
	}
	// Force removal interrupts a live executor. The execution lock below waits
	// for that executor, which must honor the durable intent before publishing.
	_ = removeContainer("vivarium-check-" + run.ID)
	lockPath := filepath.Join(s.root, repositoryID, pullRequestID, runID+".execution.lock")
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return Run{}, err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return Run{}, err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	current, err := s.Get(repositoryID, pullRequestID, runID)
	if err != nil {
		return Run{}, err
	}
	if current.State == "canceled" {
		return current, nil
	}
	return s.finalizeCancellation(current, control)
}

func (s *Store) control(repositoryID, pullRequestID, runID, actorID, action string) (Run, error) {
	lockPath := filepath.Join(s.root, repositoryID, pullRequestID, runID+".execution.lock")
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return Run{}, err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return Run{}, err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	run, err := s.Get(repositoryID, pullRequestID, runID)
	if err != nil {
		return Run{}, err
	}
	now := s.now().Truncate(time.Microsecond)
	switch action {
	case "rerun":
		if nonterminal(run.State) {
			return Run{}, ErrInvalidState
		}
		run.State, run.ExitCode, run.Failure, run.CleanupFailure = "queued", nil, "", ""
		run.StartedAt, run.CompletedAt = nil, nil
		run.RequestedBy = actorID
	default:
		return Run{}, ErrInvalidState
	}
	control, err := newControl(action, actorID, len(run.Attempts)+1, now)
	if err != nil {
		return Run{}, err
	}
	run.Controls = append(run.Controls, control)
	if err := s.Update(run); err != nil && !errors.Is(err, ErrDurabilityUncertain) {
		return Run{}, err
	}
	// The run record is the durable source of control attribution. Events()
	// repairs this projection if either append is temporarily unavailable.
	_ = s.appendEvent(run, Event{Attempt: control.Attempt, Kind: "control", Timestamp: now, State: run.State, Message: action, ActorID: actorID, ControlID: control.ID})
	if action == "rerun" {
		_ = s.appendEvent(run, Event{Attempt: len(run.Attempts) + 1, Kind: "status", Timestamp: now, State: "queued", ActorID: actorID})
	}
	return run, nil
}

func newControl(action, actorID string, attempt int, at time.Time) (Control, error) {
	id, err := newID()
	return Control{ID: id, Action: action, ActorID: actorID, Attempt: attempt, CreatedAt: at}, err
}

func (s *Store) cancelIntent(run Run) (Control, bool) {
	body, err := os.ReadFile(filepath.Join(s.root, run.RepositoryID, run.PullRequestID, run.ID+".cancel"))
	if err != nil {
		return Control{}, false
	}
	var control Control
	return control, json.Unmarshal(body, &control) == nil && control.Action == "cancel"
}

func (s *Store) writeCancelIntent(run Run, control Control) error {
	dir := filepath.Join(s.root, run.RepositoryID, run.PullRequestID)
	body, err := json.Marshal(control)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".cancel-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err = tmp.Write(body); err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(name, filepath.Join(dir, run.ID+".cancel"))
	}
	if err == nil {
		directory, openErr := os.Open(dir)
		if openErr != nil {
			return openErr
		}
		err = s.syncDirectory(directory)
		if closeErr := directory.Close(); err == nil {
			err = closeErr
		}
	}
	return err
}

func (s *Store) finalizeCancellation(run Run, control Control) (Run, error) {
	now := s.now().Truncate(time.Microsecond)
	run.State, run.CompletedAt, run.Failure = "canceled", &now, "canceled by collaborator"
	if len(run.Attempts) > 0 && run.Attempts[len(run.Attempts)-1].State == "running" {
		last := &run.Attempts[len(run.Attempts)-1]
		last.State, last.CompletedAt, last.Failure = "canceled", &now, run.Failure
	}
	found := false
	for _, existing := range run.Controls {
		found = found || existing.ID == control.ID
	}
	if !found {
		run.Controls = append(run.Controls, control)
	}
	if err := s.Update(run); err != nil && !errors.Is(err, ErrDurabilityUncertain) {
		return Run{}, err
	}
	_ = s.appendEvent(run, Event{Attempt: control.Attempt, Kind: "control", Timestamp: control.CreatedAt, State: "canceled", Message: "cancel", ActorID: control.ActorID, ControlID: control.ID})
	_ = os.Remove(filepath.Join(s.root, run.RepositoryID, run.PullRequestID, run.ID+".cancel"))
	return run, nil
}

// RecordFailure terminalizes work that cannot enter the executor, while
// preserving the same attempt and ordered evidence contract as commands.
func (s *Store) RecordFailure(run Run, failure string) error {
	now := s.now().Truncate(time.Microsecond)
	code := 1
	run.State, run.StartedAt, run.CompletedAt, run.ExitCode, run.Failure = "failed", &now, &now, &code, failure
	run.Attempts = append(run.Attempts, Attempt{Number: len(run.Attempts) + 1, State: "failed", StartedAt: now, CompletedAt: &now, ExitCode: &code, Failure: failure})
	if err := s.Update(run); err != nil && !errors.Is(err, ErrDurabilityUncertain) {
		return err
	}
	if err := s.appendEvent(run, Event{Attempt: len(run.Attempts), Kind: "status", Timestamp: now, State: "running"}); err != nil {
		return err
	}
	if err := s.appendEvent(run, Event{Attempt: len(run.Attempts), Kind: "command", Timestamp: now, State: "failed", ExitCode: &code, Message: failure}); err != nil {
		return err
	}
	return s.appendEvent(run, Event{Attempt: len(run.Attempts), Kind: "status", Timestamp: now, State: "failed", ExitCode: &code, Message: failure})
}

func (s *Store) write(dir string, run Run) error {
	b, err := json.Marshal(run)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".check-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0o600); err == nil {
		_, err = tmp.Write(b)
	}
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := os.Rename(name, filepath.Join(dir, run.ID+".json")); err != nil {
		return err
	}
	directory, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDurabilityUncertain, err)
	}
	err = s.syncDirectory(directory)
	closeErr = directory.Close()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDurabilityUncertain, err)
	}
	if closeErr != nil {
		return fmt.Errorf("%w: %v", ErrDurabilityUncertain, closeErr)
	}
	return nil
}

// Execute runs a queued check in a disposable exact-commit snapshot inside a
// capability-free, network-disabled container. Only preinstalled images run.
func (s *Store) Execute(run Run, repositoryPath string) {
	lockPath := filepath.Join(s.root, run.RepositoryID, run.PullRequestID, run.ID+".execution.lock")
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	if control, ok := s.cancelIntent(run); ok {
		current, getErr := s.Get(run.RepositoryID, run.PullRequestID, run.ID)
		if getErr == nil {
			_, _ = s.finalizeCancellation(current, control)
		}
		return
	}
	containerName := "vivarium-check-" + run.ID
	if run.State == "cleanup_pending" {
		if cleanupErr := removeContainer(containerName); cleanupErr != nil {
			run.CleanupFailure = cleanupErr.Error()
			_ = s.Update(run)
			return
		}
		now := s.now().Truncate(time.Microsecond)
		run.CompletedAt = &now
		run.CleanupFailure = ""
		if run.ExitCode != nil && *run.ExitCode == 0 && run.Failure == "" {
			run.State = "succeeded"
		} else {
			run.State = "failed"
		}
		if len(run.Attempts) > 0 {
			last := &run.Attempts[len(run.Attempts)-1]
			last.State, last.CompletedAt, last.ExitCode, last.Failure = run.State, &now, run.ExitCode, run.Failure
		}
		if !s.updatePublished(run) {
			return
		}
		_ = s.appendEvent(run, Event{Attempt: len(run.Attempts), Kind: "status", Timestamp: now, State: run.State, ExitCode: run.ExitCode, Message: run.Failure})
		return
	}
	now := s.now().Truncate(time.Microsecond)
	interruptedAttempt := 0
	interruptedFailure := ""
	if len(run.Attempts) > 0 && run.Attempts[len(run.Attempts)-1].State == "running" {
		previous := &run.Attempts[len(run.Attempts)-1]
		previous.State, previous.CompletedAt, previous.Failure = "failed", &now, "execution interrupted before reconnect"
		interruptedAttempt, interruptedFailure = previous.Number, previous.Failure
	}
	attemptNumber := len(run.Attempts) + 1
	run.Attempts = append(run.Attempts, Attempt{Number: attemptNumber, State: "running", StartedAt: now, ActorID: run.RequestedBy})
	run.RequestedBy = ""
	run.State = "running"
	run.StartedAt = &now
	if updateErr := s.Update(run); updateErr != nil && !errors.Is(updateErr, ErrDurabilityUncertain) {
		return
	}
	if interruptedAttempt != 0 {
		_ = s.appendEvent(run, Event{Attempt: interruptedAttempt, Kind: "status", Timestamp: now, State: "failed", Message: interruptedFailure})
	}
	_ = s.appendEvent(run, Event{Attempt: attemptNumber, Kind: "status", Timestamp: now, State: "running"})
	workspace, err := os.MkdirTemp("", "vivarium-check-*")
	if err == nil {
		defer os.RemoveAll(workspace)
		archive := exec.Command("git", "--git-dir="+repositoryPath, "archive", run.CommitID)
		extract := exec.Command("tar", "-x", "-C", workspace)
		pipe, p := extract.StdinPipe()
		if p == nil {
			if p = extract.Start(); p == nil {
				archive.Stdout = pipe
				p = archive.Run()
				_ = pipe.Close()
				if p == nil {
					p = extract.Wait()
				}
			}
		}
		err = p
	}
	exit := 0
	if err == nil {
		dir := filepath.Join(workspace, run.Definition.WorkingDirectory)
		info, e := os.Stat(dir)
		if e != nil || !info.IsDir() {
			err = errors.New("working directory does not exist")
		} else {
			outputDirectory, outputErr := os.MkdirTemp("", "vivarium-output-*")
			if outputErr != nil {
				err = outputErr
			} else {
				defer os.RemoveAll(outputDirectory)
				outputErr = os.Chmod(outputDirectory, 0o777)
			}
			if outputErr != nil {
				err = outputErr
			}
			if err != nil {
				goto executionDone
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(run.Definition.TimeoutSeconds)*time.Second)
			outputExceeded := make(chan struct{}, 1)
			stopOutputWatch := make(chan struct{})
			go watchOutputLimit(ctx, cancel, outputDirectory, outputExceeded, stopOutputWatch)
			// A previous API process may have died while Docker kept the container.
			// The execution lock makes removing that abandoned tree safe before
			// relaunching this exact durable run.
			_ = removeContainer(containerName)
			args := []string{"run", "--name", containerName, "--pull=never", "--network=none", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--pids-limit=128", "--memory=1g", "--cpus=2", "--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()), "--mount", "type=bind,src=" + workspace + ",dst=/workspace,readonly", "--mount", "type=bind,src=" + outputDirectory + ",dst=/output", "--tmpfs", "/tmp:rw,nosuid,nodev,noexec,size=64m,mode=1777", "--workdir", "/workspace/" + run.Definition.WorkingDirectory, "--env", "HOME=/tmp", "--env", "VIVARIUM_OUTPUT=/output", "--env", "CI=true", "--env", "VIVARIUM_COMMIT_SHA=" + run.CommitID}
			for k, v := range run.Definition.Environment {
				args = append(args, "--env", k+"="+v)
			}
			args = append(args, run.Definition.Image, "sh", "-c", run.Definition.Command)
			cmd := exec.CommandContext(ctx, "docker", args...)
			cmd.Stdout = &evidenceWriter{store: s, run: run, attempt: attemptNumber, stream: "stdout"}
			cmd.Stderr = &evidenceWriter{store: s, run: run, attempt: attemptNumber, stream: "stderr"}
			err = cmd.Run()
			close(stopOutputWatch)
			artifactErr := s.collectArtifacts(&run, attemptNumber, outputDirectory)
			if err == nil && artifactErr != nil {
				err = artifactErr
			}
			// CommandContext may terminate only the Docker client. Force-removing the
			// named container synchronously kills and reaps every check descendant.
			cleanupErr := removeContainer(containerName)
			cancel()
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				err = errors.New("check timed out")
			} else {
				select {
				case <-outputExceeded:
					err = errors.New("check output exceeded 256 MiB")
				default:
				}
			}
			if cleanupErr != nil {
				if control, ok := s.cancelIntent(run); ok {
					current, getErr := s.Get(run.RepositoryID, run.PullRequestID, run.ID)
					if getErr == nil {
						_, _ = s.finalizeCancellation(current, control)
					}
					return
				}
				run.State = "cleanup_pending"
				run.CleanupFailure = cleanupErr.Error()
				if ee := new(exec.ExitError); errors.As(err, &ee) {
					exit = ee.ExitCode()
				}
				run.ExitCode = &exit
				if err != nil {
					run.Failure = err.Error()
				}
				last := &run.Attempts[len(run.Attempts)-1]
				last.State, last.ExitCode, last.Failure = "cleanup_pending", run.ExitCode, run.Failure
				if !s.updatePublished(run) {
					return
				}
				evidenceTime := s.now().Truncate(time.Microsecond)
				_ = s.appendEvent(run, Event{Attempt: attemptNumber, Kind: "command", Timestamp: evidenceTime, State: "cleanup_pending", ExitCode: run.ExitCode, Message: run.Failure})
				_ = s.appendEvent(run, Event{Attempt: attemptNumber, Kind: "status", Timestamp: evidenceTime, State: "cleanup_pending", ExitCode: run.ExitCode, Message: run.Failure})
				return
			}
			if ee := new(exec.ExitError); errors.As(err, &ee) {
				exit = ee.ExitCode()
			}
		}
	}
executionDone:
	done := s.now().Truncate(time.Microsecond)
	if control, ok := s.cancelIntent(run); ok {
		current, getErr := s.Get(run.RepositoryID, run.PullRequestID, run.ID)
		if getErr == nil {
			_, _ = s.finalizeCancellation(current, control)
		}
		return
	}
	run.CompletedAt = &done
	run.ExitCode = &exit
	if err != nil {
		run.State = "failed"
		run.Failure = err.Error()
	} else {
		run.State = "succeeded"
	}
	last := &run.Attempts[len(run.Attempts)-1]
	last.State, last.CompletedAt, last.ExitCode, last.Failure = run.State, &done, run.ExitCode, run.Failure
	s.publishTerminal(run, attemptNumber, done)
}

func (s *Store) updatePublished(run Run) bool {
	err := s.Update(run)
	return err == nil || errors.Is(err, ErrDurabilityUncertain)
}

func (s *Store) publishTerminal(run Run, attempt int, at time.Time) bool {
	if !s.updatePublished(run) {
		return false
	}
	_ = s.appendEvent(run, Event{Attempt: attempt, Kind: "command", Timestamp: at, State: run.State, ExitCode: run.ExitCode, Message: run.Failure})
	_ = s.appendEvent(run, Event{Attempt: attempt, Kind: "status", Timestamp: at, State: run.State, ExitCode: run.ExitCode, Message: run.Failure})
	return true
}

func watchOutputLimit(ctx context.Context, cancel context.CancelFunc, directory string, exceeded chan<- struct{}, stop <-chan struct{}) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			var size int64
			_ = filepath.WalkDir(directory, func(path string, entry os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !entry.IsDir() {
					if info, e := entry.Info(); e == nil {
						size += info.Size()
					}
				}
				if size > 256*1024*1024 {
					return errors.New("limit")
				}
				return nil
			})
			if size > 256*1024*1024 {
				select {
				case exceeded <- struct{}{}:
				default:
				}
				cancel()
				return
			}
		case <-ctx.Done():
			return
		case <-stop:
			return
		}
	}
}

type evidenceWriter struct {
	store     *Store
	run       Run
	attempt   int
	stream    string
	written   int
	truncated bool
}

func (w *evidenceWriter) Write(body []byte) (int, error) {
	const limit = 10 * 1024 * 1024
	original := len(body)
	if w.written >= limit {
		if !w.truncated {
			w.truncated = true
			if err := w.store.appendEvent(w.run, Event{Attempt: w.attempt, Kind: "log", Timestamp: w.store.now().Truncate(time.Microsecond), Stream: w.stream, Message: "\n[output truncated after 10 MiB]\n"}); err != nil {
				return 0, err
			}
		}
		return original, nil
	}
	if len(body) > limit-w.written {
		body = body[:limit-w.written]
	}
	const chunk = 32 * 1024
	for start := 0; start < len(body); start += chunk {
		end := start + chunk
		if end > len(body) {
			end = len(body)
		}
		if err := w.store.appendEvent(w.run, Event{Attempt: w.attempt, Kind: "log", Timestamp: w.store.now().Truncate(time.Microsecond), Stream: w.stream, Message: string(body[start:end])}); err != nil {
			return start, err
		}
	}
	w.written += len(body)
	return original, nil
}

func (s *Store) collectArtifacts(run *Run, attempt int, temporary string) error {
	destination := filepath.Join(s.root, run.RepositoryID, run.PullRequestID, "artifacts", run.ID)
	if err := os.MkdirAll(destination, 0o700); err != nil {
		return err
	}
	var total int64
	return filepath.WalkDir(temporary, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("check artifact is not a regular file")
		}
		total += info.Size()
		if total > 256*1024*1024 {
			return errors.New("check output exceeded 256 MiB")
		}
		relative, err := filepath.Rel(temporary, path)
		if err != nil {
			return err
		}
		source, err := os.Open(path)
		if err != nil {
			return err
		}
		defer source.Close()
		id, err := newID()
		if err != nil {
			return err
		}
		target, err := os.OpenFile(filepath.Join(destination, id), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		hash := sha256.New()
		written, copyErr := io.Copy(io.MultiWriter(target, hash), io.LimitReader(source, 256*1024*1024+1))
		syncErr := target.Sync()
		closeErr := target.Close()
		if copyErr != nil {
			return copyErr
		}
		if syncErr != nil {
			return syncErr
		}
		if closeErr != nil {
			return closeErr
		}
		if written > 256*1024*1024 {
			return errors.New("check artifact exceeds size limit")
		}
		artifact := Artifact{ID: id, Attempt: attempt, Path: filepath.ToSlash(relative), Size: written, SHA256: hex.EncodeToString(hash.Sum(nil)), ContentType: mime.TypeByExtension(filepath.Ext(relative)), CreatedAt: s.now().Truncate(time.Microsecond)}
		if artifact.ContentType == "" {
			artifact.ContentType = "application/octet-stream"
		}
		run.Artifacts = append(run.Artifacts, artifact)
		if err := s.Update(*run); err != nil && !errors.Is(err, ErrDurabilityUncertain) {
			return err
		}
		return s.appendEvent(*run, Event{Attempt: attempt, Kind: "artifact", Timestamp: artifact.CreatedAt, Artifact: &artifact})
	})
}

func removeContainer(name string) error {
	removeErr := exec.Command("docker", "rm", "--force", name).Run()
	if removeErr == nil {
		return nil
	}
	output, listErr := exec.Command("docker", "ps", "--all", "--quiet", "--filter", "name=^/"+name+"$").Output()
	if listErr == nil && len(strings.TrimSpace(string(output))) == 0 {
		return nil
	}
	return removeErr
}

func newID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func validID(value string) bool {
	if len(value) != 32 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && value == strings.ToLower(value)
}
