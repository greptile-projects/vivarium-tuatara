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
	Kind      string    `json:"kind"`
	ActorID   string    `json:"actor_id"`
	CreatedAt time.Time `json:"created_at"`
}
type Workspace struct {
	ID               string      `json:"id"`
	RepositoryID     string      `json:"repository_id"`
	CommitID         string      `json:"commit_id"`
	Definition       Definition  `json:"definition"`
	DefinitionSHA256 string      `json:"definition_sha256"`
	Source           Source      `json:"source"`
	CreatorID        string      `json:"creator_id"`
	Access           Access      `json:"effective_access"`
	State            string      `json:"state"`
	Setup            []SetupStep `json:"setup_evidence"`
	Events           []Event     `json:"events"`
	CreatedAt        time.Time   `json:"created_at"`
	UpdatedAt        time.Time   `json:"updated_at"`
	SuspendedAt      *time.Time  `json:"suspended_at,omitempty"`
	ResumedAt        *time.Time  `json:"resumed_at,omitempty"`
}

type Store struct {
	root string
	mu   sync.Mutex
	now  func() time.Time
}

func New(root string) (*Store, error) {
	if root == "" {
		return nil, ErrInvalid
	}
	if err := os.MkdirAll(filepath.Join(root, "runtime"), 0700); err != nil {
		return nil, err
	}
	return &Store{root: root, now: func() time.Time { return time.Now().UTC() }}, nil
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
	w.State = "provisioning"
	w.Events = []Event{{Kind: "created", ActorID: w.CreatorID, CreatedAt: now}}
	if err := os.MkdirAll(s.RuntimePath(w.ID), 0700); err != nil {
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
func (s *Store) Get(id string) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read(id)
}
func (s *Store) List(actor string) ([]Workspace, error) {
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
		if e == nil && w.CreatorID == actor {
			out = append(out, w)
		}
	}
	return out, nil
}
func (s *Store) Transition(id, actor, expectedFoundation, target string) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, e := s.read(id)
	if e != nil {
		return Workspace{}, e
	}
	if expectedFoundation != "" && expectedFoundation != w.DefinitionSHA256 {
		return Workspace{}, ErrConflict
	}
	now := s.now()
	switch target {
	case "suspended":
		if w.State != "running" {
			return Workspace{}, ErrConflict
		}
		w.SuspendedAt = &now
	case "running":
		if w.State != "suspended" {
			return Workspace{}, ErrConflict
		}
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
