// Package workspaces stores reproducible, revision-pinned development environments.
package workspaces

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	ErrNotFound = errors.New("workspace not found")
	ErrInvalid  = errors.New("invalid workspace")
	ErrConflict = errors.New("workspace foundation changed")
	ErrControl  = errors.New("workspace control changed")
)

const DefinitionPath = ".vivarium/workspace.json"

type Resources struct {
	CPUs         float64 `json:"cpus"`
	MemoryMB     int     `json:"memory_mb"`
	StorageMB    int     `json:"storage_mb"`
	SetupSeconds int     `json:"setup_seconds"`
}
type Tool struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}
type Definition struct {
	Version      int       `json:"version"`
	Image        string    `json:"image"`
	Tools        []Tool    `json:"tools"`
	Dependencies []string  `json:"dependencies"`
	Setup        []string  `json:"setup"`
	Resources    Resources `json:"resources"`
}
type Source struct {
	Kind          string `json:"kind"`
	RepositoryID  string `json:"repository_id"`
	ProposalID    string `json:"proposal_id,omitempty"`
	TaskID        string `json:"task_id,omitempty"`
	PullRequestID string `json:"pull_request_id,omitempty"`
	IncidentID    string `json:"incident_id,omitempty"`
	RepairID      string `json:"repair_id,omitempty"`
}
type Access struct {
	Role   string   `json:"role"`
	Scopes []string `json:"scopes"`
}
type SetupStep struct {
	Command     string    `json:"command"`
	State       string    `json:"state"`
	ExitCode    int       `json:"exit_code"`
	Output      string    `json:"output,omitempty"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
}
type Event struct {
	ID        string    `json:"id,omitempty"`
	Kind      string    `json:"kind"`
	ActorID   string    `json:"actor_id"`
	Role      string    `json:"role,omitempty"`
	Detail    string    `json:"detail,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
type CommandOutcome struct {
	ID            string    `json:"id"`
	CommandSHA256 string    `json:"command_sha256"`
	Directory     string    `json:"directory"`
	ExitCode      int       `json:"exit_code"`
	Output        string    `json:"output,omitempty"`
	ActorID       string    `json:"actor_id"`
	StartedAt     time.Time `json:"started_at"`
	CompletedAt   time.Time `json:"completed_at"`
}
type Presence struct {
	ActorID  string    `json:"actor_id"`
	Focus    string    `json:"focus"`
	Path     string    `json:"path,omitempty"`
	JoinedAt time.Time `json:"joined_at"`
	SeenAt   time.Time `json:"seen_at"`
}
type Control struct {
	Version       int       `json:"version"`
	PrincipalKind string    `json:"principal_kind"`
	PrincipalID   string    `json:"principal_id"`
	Mode          string    `json:"mode"`
	Scopes        []string  `json:"scopes"`
	GrantedBy     string    `json:"granted_by"`
	GrantedAt     time.Time `json:"granted_at"`
	ExpiresAt     time.Time `json:"expires_at"`
}
type Message struct {
	ID        string    `json:"id"`
	ActorID   string    `json:"actor_id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}
type Change struct {
	Path      string    `json:"path"`
	SHA256    string    `json:"sha256"`
	Size      int       `json:"size"`
	ActorID   string    `json:"actor_id"`
	CreatedAt time.Time `json:"created_at"`
}
type Workspace struct {
	ID                string            `json:"id"`
	RepositoryID      string            `json:"repository_id"`
	OrganizationID    string            `json:"organization_id,omitempty"`
	CommitID          string            `json:"commit_id"`
	Definition        Definition        `json:"definition"`
	DefinitionSHA256  string            `json:"definition_sha256"`
	Source            Source            `json:"source"`
	CreatorID         string            `json:"creator_id"`
	Access            Access            `json:"effective_access"`
	State             string            `json:"state"`
	Setup             []SetupStep       `json:"setup_evidence"`
	Events            []Event           `json:"events"`
	Commands          []CommandOutcome  `json:"command_outcomes"`
	Changes           []Change          `json:"changes"`
	Presence          []Presence        `json:"presence"`
	Control           Control           `json:"control"`
	Messages          []Message         `json:"messages"`
	HeadCheckpointID  string            `json:"head_checkpoint_id,omitempty"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
	SuspendedAt       *time.Time        `json:"suspended_at,omitempty"`
	ResumedAt         *time.Time        `json:"resumed_at,omitempty"`
	Policy            Policy            `json:"policy"`
	PolicyScope       string            `json:"policy_scope"`
	PolicyVersion     int               `json:"policy_version"`
	LastActivityAt    time.Time         `json:"last_activity_at"`
	ExpiresAt         *time.Time        `json:"expires_at,omitempty"`
	ExpiryAnnouncedAt *time.Time        `json:"expiry_announced_at,omitempty"`
	StoppedAt         *time.Time        `json:"stopped_at,omitempty"`
	StoppedBy         string            `json:"stopped_by,omitempty"`
	StopReason        string            `json:"stop_reason,omitempty"`
	RebuildRequired   bool              `json:"rebuild_required"`
	RebuildReasons    []string          `json:"rebuild_reasons"`
	Reasoning         *ReasoningContext `json:"reasoning,omitempty"`
}
type ReasoningContext struct {
	AssessmentID      string                     `json:"assessment_id"`
	AssessmentVersion int                        `json:"assessment_version"`
	Revision          string                     `json:"revision"`
	ExplanationID     string                     `json:"explanation_id,omitempty"`
	ConclusionEntryID string                     `json:"conclusion_entry_id,omitempty"`
	Items             []ReasoningItem            `json:"items"`
	Acknowledgements  []ReasoningAcknowledgement `json:"acknowledgements,omitempty"`
}
type ReasoningItem struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Summary string `json:"summary"`
	Status  string `json:"status"`
}
type ReasoningAcknowledgement struct {
	RepositoryID   string `json:"repository_id"`
	OwnerID        string `json:"owner_id"`
	AcknowledgedBy string `json:"acknowledged_by"`
	Note           string `json:"note,omitempty"`
}

func randomID(size int) (string, error) {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *Store) Join(id, actor, focus, path string) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.read(id)
	if err != nil {
		return Workspace{}, err
	}
	now := s.now()
	found := false
	for i := range w.Presence {
		if w.Presence[i].ActorID == actor {
			w.Presence[i].Focus, w.Presence[i].Path, w.Presence[i].SeenAt = focus, path, now
			found = true
		}
	}
	if !found {
		w.Presence = append(w.Presence, Presence{ActorID: actor, Focus: focus, Path: path, JoinedAt: now, SeenAt: now})
		w.Events = append(w.Events, Event{Kind: "presence.joined", ActorID: actor, Role: "observation", CreatedAt: now})
	}
	w.UpdatedAt = now
	w.LastActivityAt = now
	return w, s.write(w)
}

func (s *Store) Leave(id, actor string) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.read(id)
	if err != nil {
		return Workspace{}, err
	}
	next := w.Presence[:0]
	found := false
	for _, p := range w.Presence {
		if p.ActorID == actor {
			found = true
		} else {
			next = append(next, p)
		}
	}
	w.Presence = next
	if found {
		now := s.now()
		w.UpdatedAt = now
		w.Events = append(w.Events, Event{Kind: "presence.left", ActorID: actor, Role: "observation", CreatedAt: now})
		err = s.write(w)
	}
	return w, err
}

func (s *Store) SetControl(id, actor, principalKind, principalID, mode string, scopes []string, expectedVersion, seconds int) (Workspace, error) {
	control := s.controlLock(id)
	control.Lock()
	defer control.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.read(id)
	if err != nil {
		return Workspace{}, err
	}
	if expectedVersion != w.Control.Version {
		return Workspace{}, ErrControl
	}
	now := s.now()
	if principalID == "" || seconds < 30 || seconds > 3600 {
		return Workspace{}, ErrInvalid
	}
	w.Control = Control{Version: expectedVersion + 1, PrincipalKind: principalKind, PrincipalID: principalID, Mode: mode, Scopes: append([]string(nil), scopes...), GrantedBy: actor, GrantedAt: now, ExpiresAt: now.Add(time.Duration(seconds) * time.Second)}
	w.UpdatedAt = now
	w.Events = append(w.Events, Event{Kind: "control.changed", ActorID: actor, Role: "instruction", Detail: principalKind + ":" + principalID, CreatedAt: now})
	return w, s.write(w)
}

func (s *Store) ReleaseControl(id, actor string, expectedVersion int) (Workspace, error) {
	control := s.controlLock(id)
	control.Lock()
	defer control.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.read(id)
	if err != nil {
		return Workspace{}, err
	}
	now := s.now()
	if expectedVersion != w.Control.Version || w.Control.PrincipalKind != "human" || w.Control.PrincipalID != actor || !w.Control.ExpiresAt.After(now) {
		return Workspace{}, ErrControl
	}
	w.Control = Control{Version: expectedVersion + 1, Mode: "observe", Scopes: []string{}, GrantedBy: actor, GrantedAt: now, ExpiresAt: now}
	w.UpdatedAt = now
	w.Events = append(w.Events, Event{Kind: "control.changed", ActorID: actor, Role: "instruction", CreatedAt: now})
	return w, s.write(w)
}

func (s *Store) AddMessage(id, actor, body string) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.read(id)
	if err != nil {
		return Workspace{}, err
	}
	mid, err := randomID(12)
	if err != nil {
		return Workspace{}, err
	}
	now := s.now()
	w.Messages = append(w.Messages, Message{ID: mid, ActorID: actor, Body: body, CreatedAt: now})
	if len(w.Messages) > 200 {
		w.Messages = w.Messages[len(w.Messages)-200:]
	}
	w.UpdatedAt = now
	w.LastActivityAt = now
	w.Events = append(w.Events, Event{ID: mid, Kind: "discussion.message", ActorID: actor, Role: "instruction", Detail: mid, CreatedAt: now})
	return w, s.write(w)
}

func (w Workspace) CanControl(actor, scope string, now time.Time) bool {
	if w.Control.PrincipalKind != "human" || w.Control.PrincipalID != actor || !w.Control.ExpiresAt.After(now) {
		return false
	}
	if w.Control.Mode != "execute" && !(w.Control.Mode == "edit" && scope == "files") {
		return false
	}
	for _, v := range w.Control.Scopes {
		if v == scope {
			return true
		}
	}
	return false
}

// WithControl serializes the final live-lease check and mutation execution with
// control transfer. Once admitted, a mutation finishes before a takeover can
// publish; a request that lost control before admission fails closed.
func (s *Store) WithControl(id, actor, scope string, operation func(Workspace) error) error {
	control := s.controlLock(id)
	control.Lock()
	defer control.Unlock()
	s.mu.Lock()
	w, err := s.read(id)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if !w.CanControl(actor, scope, s.now()) {
		return ErrControl
	}
	return operation(w)
}

func (s *Store) RecordCommand(id string, outcome CommandOutcome) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.read(id)
	if err != nil {
		return Workspace{}, err
	}
	outcomeID, err := randomID(12)
	if err != nil {
		return Workspace{}, err
	}
	outcome.ID = outcomeID
	provenance, err := s.readOrSeedProvenance(id, w)
	if err != nil {
		return Workspace{}, err
	}
	provenance.Commands = append(provenance.Commands, outcome)
	if err = s.writeProvenance(id, provenance); err != nil {
		return Workspace{}, err
	}
	w.Commands = append(w.Commands, outcome)
	if len(w.Commands) > 100 {
		w.Commands = w.Commands[len(w.Commands)-100:]
	}
	w.UpdatedAt = s.now()
	w.LastActivityAt = w.UpdatedAt
	w.Events = append(w.Events, Event{Kind: "command.completed", ActorID: outcome.ActorID, Role: "execution", Detail: outcome.ID, CreatedAt: w.UpdatedAt})
	err = s.write(w)
	return w, err
}
func (s *Store) RecordChange(id string, change Change) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.read(id)
	if err != nil {
		return Workspace{}, err
	}
	provenance, err := s.readOrSeedProvenance(id, w)
	if err != nil {
		return Workspace{}, err
	}
	provenance.Changes = append(provenance.Changes, change)
	if err = s.writeProvenance(id, provenance); err != nil {
		return Workspace{}, err
	}
	w.Changes = append(w.Changes, change)
	if len(w.Changes) > 200 {
		w.Changes = w.Changes[len(w.Changes)-200:]
	}
	w.UpdatedAt = s.now()
	w.LastActivityAt = w.UpdatedAt
	w.Events = append(w.Events, Event{Kind: "file.changed", ActorID: change.ActorID, Role: "authorship", Detail: change.Path, CreatedAt: w.UpdatedAt})
	err = s.write(w)
	return w, err
}

type Store struct {
	root       string
	mu         sync.Mutex
	controlsMu sync.Mutex
	controls   map[string]*sync.Mutex
	now        func() time.Time
}

type provenanceRecord struct {
	Changes  []Change         `json:"changes"`
	Commands []CommandOutcome `json:"commands"`
}

func (s *Store) provenancePath(id string) string {
	return filepath.Join(s.root, "provenance", id+".json")
}
func (s *Store) readProvenance(id string) (provenanceRecord, error) {
	b, err := os.ReadFile(s.provenancePath(id))
	if errors.Is(err, os.ErrNotExist) {
		return provenanceRecord{}, nil
	}
	if err != nil {
		return provenanceRecord{}, err
	}
	var value provenanceRecord
	if json.Unmarshal(b, &value) != nil {
		return provenanceRecord{}, ErrInvalid
	}
	return value, nil
}
func (s *Store) readOrSeedProvenance(id string, w Workspace) (provenanceRecord, error) {
	_, statErr := os.Stat(s.provenancePath(id))
	value, err := s.readProvenance(id)
	if err != nil {
		return value, err
	}
	if errors.Is(statErr, os.ErrNotExist) {
		value.Changes = append([]Change(nil), w.Changes...)
		value.Commands = append([]CommandOutcome(nil), w.Commands...)
	}
	return value, nil
}
func (s *Store) writeProvenance(id string, value provenanceRecord) error {
	dir := filepath.Dir(s.provenancePath(id))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".provenance-")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(b)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(name, s.provenancePath(id)); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

func New(root string) (*Store, error) {
	if root == "" {
		return nil, ErrInvalid
	}
	if err := os.MkdirAll(filepath.Join(root, "runtime"), 0700); err != nil {
		return nil, err
	}
	return &Store{root: root, controls: map[string]*sync.Mutex{}, now: func() time.Time { return time.Now().UTC() }}, nil
}
func (s *Store) controlLock(id string) *sync.Mutex {
	s.controlsMu.Lock()
	defer s.controlsMu.Unlock()
	if s.controls[id] == nil {
		s.controls[id] = &sync.Mutex{}
	}
	return s.controls[id]
}
func (s *Store) RuntimePath(id string) string { return filepath.Join(s.root, "runtime", id) }
func (s *Store) Create(w Workspace, definitionBytes []byte) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if w.RepositoryID == "" || w.CommitID == "" || w.CreatorID == "" {
		return Workspace{}, ErrInvalid
	}
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		return Workspace{}, e
	}
	w.ID = hex.EncodeToString(b)
	sum := sha256.Sum256(definitionBytes)
	w.DefinitionSHA256 = hex.EncodeToString(sum[:])
	now := s.now()
	w.CreatedAt, w.UpdatedAt = now, now
	w.LastActivityAt = now
	if w.Policy.MaxRuntimeHours > 0 {
		expires := now.Add(time.Duration(w.Policy.MaxRuntimeHours) * time.Hour)
		w.ExpiresAt = &expires
	}
	w.State = "provisioning"
	w.Control = Control{Version: 1, PrincipalKind: "human", PrincipalID: w.CreatorID, Mode: "execute", Scopes: []string{"files", "commands", "lifecycle"}, GrantedBy: w.CreatorID, GrantedAt: now, ExpiresAt: now.Add(time.Hour)}
	w.Presence = []Presence{}
	w.Messages = []Message{}
	w.Events = []Event{{Kind: "created", ActorID: w.CreatorID, Role: "authorship", CreatedAt: now}}
	if err := os.MkdirAll(s.RuntimePath(w.ID), 0700); err != nil {
		return Workspace{}, err
	}
	if err := s.writeProvenance(w.ID, provenanceRecord{}); err != nil {
		return Workspace{}, err
	}
	if err := s.write(w); err != nil {
		return Workspace{}, err
	}
	return w, nil
}
func (s *Store) Complete(id string, steps []SetupStep, failure bool) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, e := s.read(id)
	if e != nil {
		return Workspace{}, e
	}
	w.Setup = steps
	if failure {
		w.State = "failed"
	} else {
		w.State = "running"
	}
	w.UpdatedAt = s.now()
	w.Events = append(w.Events, Event{Kind: "setup_completed", ActorID: w.CreatorID, CreatedAt: w.UpdatedAt})
	e = s.write(w)
	return w, e
}

// Stop removes compute authority while retaining the workspace record,
// checkpoints, provenance ledger, and any already-published Git objects.
func (s *Store) Stop(id, actor, reason, state string) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopLocked(id, actor, reason, state)
}

func (s *Store) stopLocked(id, actor, reason, state string) (Workspace, error) {
	w, err := s.read(id)
	if err != nil {
		return Workspace{}, err
	}
	if w.State == "stopped" || w.State == "expired" {
		return Workspace{}, ErrConflict
	}
	if state != "stopped" && state != "expired" {
		return Workspace{}, ErrInvalid
	}
	now := s.now()
	w.State, w.StoppedAt, w.StoppedBy, w.StopReason = state, &now, actor, reason
	w.Control = Control{Version: w.Control.Version + 1, Mode: "observe", GrantedBy: actor, GrantedAt: now, ExpiresAt: now}
	w.Presence = []Presence{}
	w.UpdatedAt = now
	w.Events = append(w.Events, Event{Kind: state, ActorID: actor, Role: "instruction", Detail: reason, CreatedAt: now})
	return w, s.write(w)
}

// StopControlled serializes external compute teardown and terminal state
// publication with suspend/resume. The global store lock is deliberately not
// held while teardown runs, so unrelated workspaces remain available.
func (s *Store) StopControlled(id, actor, reason, state string, teardown func() error) (Workspace, error) {
	control := s.controlLock(id)
	control.Lock()
	defer control.Unlock()
	s.mu.Lock()
	w, err := s.read(id)
	s.mu.Unlock()
	if err != nil {
		return Workspace{}, err
	}
	if w.State == "stopped" || w.State == "expired" {
		return Workspace{}, ErrConflict
	}
	if err = teardown(); err != nil {
		return Workspace{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopLocked(id, actor, reason, state)
}

func (s *Store) AnnounceExpiry(id, actor string, at time.Time, reason string) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.read(id)
	if err != nil {
		return Workspace{}, err
	}
	if !at.After(s.now()) {
		return Workspace{}, ErrInvalid
	}
	now := s.now()
	w.ExpiresAt, w.ExpiryAnnouncedAt, w.UpdatedAt = &at, &now, now
	w.Events = append(w.Events, Event{Kind: "expiry.announced", ActorID: actor, Role: "instruction", Detail: reason, CreatedAt: now})
	return w, s.write(w)
}

func (s *Store) MarkRebuild(id string, reasons []string) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.read(id)
	if err != nil {
		return Workspace{}, err
	}
	w.RebuildRequired, w.RebuildReasons, w.UpdatedAt = len(reasons) > 0, append([]string(nil), reasons...), s.now()
	return w, s.write(w)
}
func (s *Store) Get(id string) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read(id)
}
func (s *Store) List(actor string) ([]Workspace, error) {
	items, err := s.ListAll()
	if err != nil {
		return nil, err
	}
	out := []Workspace{}
	for _, w := range items {
		if w.CreatorID == actor {
			out = append(out, w)
		}
	}
	return out, nil
}
func (s *Store) ListAll() ([]Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, e := os.ReadDir(s.root)
	if e != nil {
		return nil, e
	}
	out := []Workspace{}
	for _, x := range entries {
		if x.IsDir() {
			continue
		}
		w, e := s.readName(x.Name())
		if e == nil {
			out = append(out, w)
		}
	}
	return out, nil
}
func (s *Store) Transition(id, actor, expectedFoundation, target string) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.transition(id, actor, expectedFoundation, target, false)
}

func (s *Store) TransitionControlled(id, actor, expectedFoundation, target string) (Workspace, error) {
	control := s.controlLock(id)
	control.Lock()
	defer control.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.transition(id, actor, expectedFoundation, target, true)
}

func (s *Store) transition(id, actor, expectedFoundation, target string, requireControl bool) (Workspace, error) {
	w, e := s.read(id)
	if e != nil {
		return Workspace{}, e
	}
	if expectedFoundation == "" || expectedFoundation != w.DefinitionSHA256 {
		return Workspace{}, ErrConflict
	}
	if (target == "suspended" && w.State != "running") || (target == "running" && w.State != "suspended") {
		return Workspace{}, ErrConflict
	}
	if requireControl && !w.CanControl(actor, "lifecycle", s.now()) {
		return Workspace{}, ErrControl
	}
	now := s.now()
	switch target {
	case "suspended":
		w.SuspendedAt = &now
	case "running":
		if _, e = os.Stat(s.RuntimePath(id)); e != nil {
			return Workspace{}, ErrConflict
		}
		w.ResumedAt = &now
	default:
		return Workspace{}, ErrInvalid
	}
	w.State = target
	w.UpdatedAt = now
	w.Events = append(w.Events, Event{Kind: target, ActorID: actor, CreatedAt: now})
	e = s.write(w)
	return w, e
}
func (s *Store) read(id string) (Workspace, error) {
	if len(id) != 32 {
		return Workspace{}, ErrNotFound
	}
	return s.readName(id + ".json")
}
func (s *Store) readName(name string) (Workspace, error) {
	body, e := os.ReadFile(filepath.Join(s.root, name))
	if os.IsNotExist(e) {
		return Workspace{}, ErrNotFound
	}
	if e != nil {
		return Workspace{}, e
	}
	var w Workspace
	if json.Unmarshal(body, &w) != nil {
		return Workspace{}, ErrNotFound
	}
	if w.Policy.Version == 0 {
		w.Policy = DefaultPolicy()
		w.PolicyVersion = w.Policy.Version
		w.PolicyScope = "platform-default"
	}
	if w.LastActivityAt.IsZero() {
		w.LastActivityAt = w.UpdatedAt
	}
	// Presence is a renewable observation, not authority. A disconnected client
	// disappears deterministically even when it cannot publish an explicit leave.
	cutoff := s.now().Add(-20 * time.Second)
	active := w.Presence[:0]
	for _, presence := range w.Presence {
		if presence.SeenAt.After(cutoff) {
			active = append(active, presence)
		}
	}
	w.Presence = active
	return w, nil
}
func (s *Store) write(w Workspace) error {
	body, e := json.MarshalIndent(w, "", "  ")
	if e != nil {
		return e
	}
	tmp := filepath.Join(s.root, "."+w.ID+".tmp")
	if e = os.WriteFile(tmp, body, 0600); e != nil {
		return e
	}
	if e = os.Rename(tmp, filepath.Join(s.root, w.ID+".json")); e != nil {
		return e
	}
	d, e := os.Open(s.root)
	if e != nil {
		return e
	}
	defer d.Close()
	if e = d.Sync(); e != nil {
		return fmt.Errorf("sync workspace store: %w", e)
	}
	return nil
}
