// Package changesessions stores durable agent collaboration workspaces.
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
	ErrRunPaused           = errors.New("agent run is paused")
	ErrRunCanceled         = errors.New("agent run is canceled")
	ErrRunCompleted        = errors.New("agent run is completed")
	ErrDurabilityUncertain = errors.New("change session is visible but durability is uncertain")
)

const Open = "open"
const Launched = "launched"
const Paused = "paused"
const Canceled = "canceled"
const Completed = "completed"

var interventionKinds = map[string]bool{
	"run.guidance": true, "question.answered": true, "run.paused": true,
	"run.resumed": true, "run.canceled": true,
}

var workEventKinds = map[string]bool{
	"run.status": true, "agent.message": true, "tool.action": true,
	"agent.question": true, "artifact.produced": true, "run.failed": true, "branch.updated": true,
}

type Session struct {
	ID             string         `json:"id"`
	RepositoryID   string         `json:"repository_id"`
	PullRequestID  string         `json:"pull_request_id,omitempty"`
	ProposalID     string         `json:"proposal_id,omitempty"`
	TaskID         string         `json:"task_id,omitempty"`
	TaskContext    *TaskContext   `json:"task_context,omitempty"`
	InitiatorID    string         `json:"initiator_id"`
	SourceCommitID string         `json:"source_commit_id"`
	CheckEvidence  *CheckEvidence `json:"check_evidence,omitempty"`
	State          string         `json:"state"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

// TaskContext freezes the shared intent supplied to an agent before a pull
// request exists. It deliberately contains displayable snapshots as well as
// stable IDs so later proposal edits cannot silently rewrite a run mandate.
type TaskContext struct {
	RepositoryName string           `json:"repository_name"`
	ProposalTitle  string           `json:"proposal_title"`
	ProposalBody   string           `json:"proposal_body"`
	TaskTitle      string           `json:"task_title"`
	TaskOutcome    string           `json:"task_outcome"`
	Mandate        string           `json:"mandate"`
	Dependencies   []TaskDependency `json:"dependencies"`
	Discussion     []TaskDiscussion `json:"discussion"`
}

type TaskDependency struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Outcome string `json:"outcome"`
	Status  string `json:"status"`
}

type TaskDiscussion struct {
	ID       string `json:"id"`
	AuthorID string `json:"author_id"`
	Body     string `json:"body"`
}

// CheckEvidence is an immutable snapshot of the automated failure that led to
// a repair session. Artifact bytes remain in the check store and are addressed
// by their stable IDs; the session retains everything needed to understand and
// retrieve that evidence later.
type CheckEvidence struct {
	RunID      string          `json:"run_id"`
	Definition CheckDefinition `json:"definition"`
	Events     []CheckEvent    `json:"events"`
	Artifacts  []CheckArtifact `json:"artifacts"`
}

type CheckDefinition struct {
	Name             string            `json:"name"`
	Image            string            `json:"image"`
	Command          string            `json:"command"`
	WorkingDirectory string            `json:"working_directory,omitempty"`
	Environment      map[string]string `json:"environment,omitempty"`
	TimeoutSeconds   int               `json:"timeout_seconds,omitempty"`
}

type CheckEvent struct {
	Sequence int64  `json:"sequence"`
	Attempt  int    `json:"attempt"`
	Kind     string `json:"kind"`
	State    string `json:"state,omitempty"`
	Stream   string `json:"stream,omitempty"`
	Message  string `json:"message,omitempty"`
	ExitCode *int   `json:"exit_code,omitempty"`
}

type CheckArtifact struct {
	ID          string    `json:"id"`
	Attempt     int       `json:"attempt"`
	Path        string    `json:"path"`
	Size        int64     `json:"size"`
	SHA256      string    `json:"sha256"`
	ContentType string    `json:"content_type"`
	CreatedAt   time.Time `json:"created_at"`
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
	Outcome             *Outcome   `json:"outcome,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type Check struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Details string `json:"details,omitempty"`
}

type ChangedFile struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

// Outcome is the durable handoff from delegated execution to ordinary review.
// Commit and file evidence is derived by the server; only narrative evidence
// and check results come from the credential-bound agent.
type Outcome struct {
	Summary      string        `json:"summary"`
	CommitID     string        `json:"commit_id"`
	Commits      []string      `json:"commits"`
	ChangedFiles []ChangedFile `json:"changed_files"`
	Checks       []Check       `json:"checks"`
	Concerns     []string      `json:"unresolved_concerns"`
	CompletedAt  time.Time     `json:"completed_at"`
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
	return s.CreateWithEvidence(repositoryID, pullRequestID, initiatorID, sourceCommitID, nil)
}

func (s *Store) CreateWithEvidence(repositoryID, pullRequestID, initiatorID, sourceCommitID string, evidence *CheckEvidence) (Session, error) {
	if !validID(repositoryID) || !validID(pullRequestID) || !validID(initiatorID) || !validObjectID(sourceCommitID) {
		return Session{}, ErrInvalid
	}
	if evidence != nil && (!validID(evidence.RunID) || evidence.Definition.Name == "" || evidence.Definition.Command == "") {
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
	session := Session{ID: sessionID, RepositoryID: repositoryID, PullRequestID: pullRequestID, InitiatorID: initiatorID, SourceCommitID: sourceCommitID, CheckEvidence: evidence, State: Open, CreatedAt: now, UpdatedAt: now}
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

// CreateForTask opens a session keyed by its task while retaining the same
// durable timeline/run boundary used by pull-request workspaces.
func (s *Store) CreateForTask(repositoryID, proposalID, taskID, initiatorID, sourceCommitID string, context TaskContext) (Session, error) {
	if !validID(repositoryID) || !validID(proposalID) || !validID(taskID) || !validID(initiatorID) || !validObjectID(sourceCommitID) || strings.TrimSpace(context.RepositoryName) == "" || strings.TrimSpace(context.ProposalTitle) == "" || strings.TrimSpace(context.TaskTitle) == "" || strings.TrimSpace(context.TaskOutcome) == "" || strings.TrimSpace(context.Mandate) == "" {
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
	context.Dependencies = append([]TaskDependency(nil), context.Dependencies...)
	context.Discussion = append([]TaskDiscussion(nil), context.Discussion...)
	session := Session{ID: sessionID, RepositoryID: repositoryID, ProposalID: proposalID, TaskID: taskID, InitiatorID: initiatorID, SourceCommitID: sourceCommitID, TaskContext: &context, State: Open, CreatedAt: now, UpdatedAt: now}
	rec := record{Session: session, Events: []Event{{ID: eventID, SessionID: sessionID, Kind: "session.opened", ActorID: initiatorID, RevisionID: sourceCommitID, State: Open, CreatedAt: now}}}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Session{}, err
	}
	defer unlock()
	if err := s.ensureDirectory(repositoryID, taskID); err != nil {
		return Session{}, fmt.Errorf("create change session directory: %w", err)
	}
	committed, err := s.write(filepath.Join(s.root, repositoryID, taskID), rec)
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
	agentID, err := newID()
	if err != nil {
		return Run{}, err
	}
	return s.LaunchRunForAgent(repositoryID, pullRequestID, sessionID, initiatorID, agentID, instructions, sourceCommitID, contextPaths, workingBranch, credentialID, credentialExpiresAt)
}

// LaunchRunForAgent preserves a generated planning identity across assignment
// and execution. Pull-request runs continue to use LaunchRun's fresh identity.
func (s *Store) LaunchRunForAgent(repositoryID, pullRequestID, sessionID, initiatorID, agentID, instructions, sourceCommitID string, contextPaths []string, workingBranch, credentialID string, credentialExpiresAt time.Time) (Run, error) {
	if !validID(initiatorID) || !validID(agentID) || !validObjectID(sourceCommitID) || !validID(credentialID) || instructions == "" || workingBranch == "" || credentialExpiresAt.IsZero() {
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
	now := s.now().Truncate(time.Microsecond)
	run := Run{ID: runID, SessionID: sessionID, InitiatorID: initiatorID, AgentID: agentID, Instructions: instructions, SourceCommitID: sourceCommitID, ContextPaths: append([]string(nil), contextPaths...), WorkingBranch: workingBranch, CredentialID: credentialID, CredentialExpiresAt: credentialExpiresAt, State: Launched, CreatedAt: now, UpdatedAt: now}

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
	if run.State == Paused {
		return Event{}, ErrRunPaused
	}
	if run.State == Canceled {
		return Event{}, ErrRunCanceled
	}
	if run.State == Completed {
		return Event{}, ErrRunCompleted
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
	run.UpdatedAt = now
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

// CompleteRun atomically records a structured, attributed review handoff.
func (s *Store) CompleteRun(repositoryID, pullRequestID, sessionID, runID, credentialID, summary, commitID string, commits []string, files []ChangedFile, checks []Check, concerns []string) (Run, Event, error) {
	summary = strings.TrimSpace(summary)
	if summary == "" || len([]rune(summary)) > 10000 || !validObjectID(commitID) || len(commits) == 0 || len(commits) > 200 || len(files) > 2000 || len(checks) > 100 || len(concerns) > 100 {
		return Run{}, Event{}, ErrInvalid
	}
	for _, id := range commits {
		if !validObjectID(id) {
			return Run{}, Event{}, ErrInvalid
		}
	}
	for i := range checks {
		checks[i].Name, checks[i].Status, checks[i].Details = strings.TrimSpace(checks[i].Name), strings.TrimSpace(checks[i].Status), strings.TrimSpace(checks[i].Details)
		if checks[i].Name == "" || len([]rune(checks[i].Name)) > 200 || (checks[i].Status != "passed" && checks[i].Status != "failed" && checks[i].Status != "skipped") || len([]rune(checks[i].Details)) > 2000 {
			return Run{}, Event{}, ErrInvalid
		}
	}
	for i := range concerns {
		concerns[i] = strings.TrimSpace(concerns[i])
		if concerns[i] == "" || len([]rune(concerns[i])) > 2000 {
			return Run{}, Event{}, ErrInvalid
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Run{}, Event{}, err
	}
	defer unlock()
	rec, err := s.read(repositoryID, pullRequestID, sessionID)
	if err != nil {
		return Run{}, Event{}, err
	}
	var run *Run
	for i := range rec.Runs {
		if rec.Runs[i].ID == runID {
			run = &rec.Runs[i]
			break
		}
	}
	if run == nil || run.CredentialID != credentialID || run.AccessRevokedAt != nil {
		return Run{}, Event{}, ErrNotFound
	}
	if run.State == Canceled {
		return Run{}, Event{}, ErrRunCanceled
	}
	if run.State == Paused {
		return Run{}, Event{}, ErrRunPaused
	}
	if run.State == Completed {
		if run.Outcome != nil && run.Outcome.CommitID == commitID {
			for _, event := range rec.Events {
				if event.RunID == runID && event.Kind == "run.completed" {
					return *run, event, nil
				}
			}
		}
		return Run{}, Event{}, ErrRunCompleted
	}
	eventID, err := newID()
	if err != nil {
		return Run{}, Event{}, err
	}
	now := s.now().Truncate(time.Microsecond)
	run.State, run.UpdatedAt = Completed, now
	run.Outcome = &Outcome{Summary: summary, CommitID: commitID, Commits: append([]string{}, commits...), ChangedFiles: append([]ChangedFile{}, files...), Checks: append([]Check{}, checks...), Concerns: append([]string{}, concerns...), CompletedAt: now}
	event := Event{ID: eventID, SessionID: sessionID, Kind: "run.completed", ActorID: run.InitiatorID, InitiatorID: run.InitiatorID, AgentID: run.AgentID, RevisionID: run.SourceCommitID, State: Completed, RunID: run.ID, Message: summary, Branch: run.WorkingBranch, CommitID: commitID, CreatedAt: now}
	rec.Events = append(rec.Events, event)
	rec.Session.UpdatedAt = now
	committed, err := s.write(filepath.Join(s.root, repositoryID, pullRequestID), rec)
	if err != nil {
		if committed {
			return *run, event, fmt.Errorf("%w: %v", ErrDurabilityUncertain, err)
		}
		return Run{}, Event{}, err
	}
	return *run, event, nil
}

// GetRunControl exposes only the mandate state and collaborator interventions
// needed by the credential-bound worker to respond to human control.
func (s *Store) GetRunControl(repositoryID, pullRequestID, sessionID, runID, credentialID string) (Run, []Event, error) {
	rec, err := s.read(repositoryID, pullRequestID, sessionID)
	if err != nil {
		return Run{}, nil, err
	}
	for _, run := range rec.Runs {
		if run.ID != runID || run.CredentialID != credentialID {
			continue
		}
		events := []Event{}
		for _, event := range rec.Events {
			if event.RunID == runID && interventionKinds[event.Kind] {
				events = append(events, event)
			}
		}
		return run, events, nil
	}
	return Run{}, nil, ErrNotFound
}

// Intervene atomically records collaborator guidance or a run state transition.
func (s *Store) Intervene(repositoryID, pullRequestID, sessionID, runID, actorID, kind, message string) (Run, Event, error) {
	if !validID(actorID) || !interventionKinds[kind] || len([]rune(message)) > 10000 || ((kind == "run.guidance" || kind == "question.answered") && strings.TrimSpace(message) == "") {
		return Run{}, Event{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Run{}, Event{}, err
	}
	defer unlock()
	rec, err := s.read(repositoryID, pullRequestID, sessionID)
	if err != nil {
		return Run{}, Event{}, err
	}
	var run *Run
	for i := range rec.Runs {
		if rec.Runs[i].ID == runID {
			run = &rec.Runs[i]
			break
		}
	}
	if run == nil {
		return Run{}, Event{}, ErrNotFound
	}
	if run.State == Canceled {
		if kind == "run.canceled" {
			for _, event := range rec.Events {
				if event.RunID == run.ID && event.Kind == kind {
					return *run, event, nil
				}
			}
		}
		return Run{}, Event{}, ErrRunCanceled
	}
	if run.State == Completed {
		return Run{}, Event{}, ErrRunCompleted
	}
	switch kind {
	case "run.paused":
		if run.State != Launched {
			return Run{}, Event{}, ErrInvalid
		}
		run.State = Paused
	case "run.resumed":
		if run.State != Paused {
			return Run{}, Event{}, ErrInvalid
		}
		run.State = Launched
	case "run.canceled":
		run.State = Canceled
	default:
		if run.State != Launched && run.State != Paused {
			return Run{}, Event{}, ErrInvalid
		}
	}
	eventID, err := newID()
	if err != nil {
		return Run{}, Event{}, err
	}
	now := s.now().Truncate(time.Microsecond)
	if kind == "run.canceled" && run.AccessRevokedAt == nil {
		run.AccessRevokedAt = &now
	}
	run.UpdatedAt = now
	event := Event{ID: eventID, SessionID: sessionID, Kind: kind, ActorID: actorID, InitiatorID: run.InitiatorID, AgentID: run.AgentID, RevisionID: run.SourceCommitID, State: run.State, RunID: run.ID, Message: strings.TrimSpace(message), CreatedAt: now}
	rec.Events = append(rec.Events, event)
	rec.Session.UpdatedAt = now
	committed, err := s.write(filepath.Join(s.root, repositoryID, pullRequestID), rec)
	if err != nil {
		if committed {
			return *run, event, fmt.Errorf("%w: %v", ErrDurabilityUncertain, err)
		}
		return Run{}, Event{}, err
	}
	return *run, event, nil
}

func (s *Store) ListRuns(repositoryID, pullRequestID, sessionID string) ([]Run, error) {
	if _, err := s.Get(repositoryID, pullRequestID, sessionID); err != nil {
		return nil, err
	}
	rec, err := s.read(repositoryID, pullRequestID, sessionID)
	if err != nil {
		return nil, err
	}
	return append([]Run{}, rec.Runs...), nil
}

// AllowsGitWrite is the fail-closed authorization boundary for a bounded run
// credential. Durable run state remains authoritative even when credential
// revocation storage is temporarily unavailable.
func (s *Store) AllowsGitWrite(repositoryID, credentialID string) (bool, error) {
	if !validID(repositoryID) || !validID(credentialID) {
		return false, ErrNotFound
	}
	pulls, err := os.ReadDir(filepath.Join(s.root, repositoryID))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("list repository change sessions: %w", err)
	}
	for _, pull := range pulls {
		if !pull.IsDir() || !validID(pull.Name()) {
			continue
		}
		entries, readErr := os.ReadDir(filepath.Join(s.root, repositoryID, pull.Name()))
		if readErr != nil {
			return false, fmt.Errorf("list pull request change sessions: %w", readErr)
		}
		for _, entry := range entries {
			id, ok := strings.CutSuffix(entry.Name(), ".json")
			if entry.IsDir() || !ok || !validID(id) {
				continue
			}
			rec, readErr := s.read(repositoryID, pull.Name(), id)
			if readErr != nil {
				return false, readErr
			}
			for _, run := range rec.Runs {
				if run.CredentialID == credentialID {
					return run.State == Launched && run.AccessRevokedAt == nil, nil
				}
			}
		}
	}
	return false, nil
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
		rec.Runs[index].UpdatedAt = now
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
	if json.Unmarshal(data, &rec) != nil || rec.Session.ID != sessionID || rec.Session.RepositoryID != repositoryID || !validID(rec.Session.InitiatorID) || !validObjectID(rec.Session.SourceCommitID) || rec.Session.State != Open || len(rec.Events) == 0 {
		return record{}, errors.New("invalid durable change session record")
	}
	if rec.Session.PullRequestID != pullRequestID && rec.Session.TaskID != pullRequestID {
		return record{}, errors.New("invalid durable change session scope")
	}
	if rec.Session.TaskID != "" && (!validID(rec.Session.ProposalID) || rec.Session.TaskContext == nil || strings.TrimSpace(rec.Session.TaskContext.Mandate) == "") {
		return record{}, errors.New("invalid durable task session context")
	}
	if evidence := rec.Session.CheckEvidence; evidence != nil {
		if !validID(evidence.RunID) || evidence.Definition.Name == "" || evidence.Definition.Command == "" {
			return record{}, errors.New("invalid durable check evidence")
		}
		for _, artifact := range evidence.Artifacts {
			if !validID(artifact.ID) || artifact.Path == "" || artifact.Size < 0 {
				return record{}, errors.New("invalid durable check artifact evidence")
			}
		}
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
		if run.UpdatedAt.IsZero() {
			run.UpdatedAt = run.CreatedAt
		}
		completionEvents := 0
		for _, event := range rec.Events {
			if event.RunID == run.ID && event.Kind == "run.completed" {
				completionEvents++
			}
		}
		validOutcome := run.Outcome == nil
		if run.Outcome != nil {
			validOutcome = run.Outcome.Summary != "" && validObjectID(run.Outcome.CommitID) && len(run.Outcome.Commits) > 0 && !run.Outcome.CompletedAt.IsZero()
			for _, commitID := range run.Outcome.Commits {
				validOutcome = validOutcome && validObjectID(commitID)
			}
		}
		if !validID(run.ID) || run.SessionID != sessionID || !validID(run.InitiatorID) || !validID(run.AgentID) || !validObjectID(run.SourceCommitID) || run.SourceCommitID != rec.Session.SourceCommitID || run.Instructions == "" || run.WorkingBranch == "" || !validID(run.CredentialID) || (run.State != Launched && run.State != Paused && run.State != Canceled && run.State != Completed) || run.CredentialExpiresAt.IsZero() || runEvents[run.ID] != 1 || !validOutcome || (run.State == Completed) != (run.Outcome != nil && completionEvents == 1) {
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
