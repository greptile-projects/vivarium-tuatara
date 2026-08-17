// Package supportverifications persists immutable, rerunnable proof for support guidance.
package supportverifications

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var ErrNotFound = errors.New("support verification not found")
var ErrInvalid = errors.New("invalid support verification")

type Environment struct {
	OperatingSystem string   `json:"operating_system,omitempty"`
	Runtime         string   `json:"runtime,omitempty"`
	Dependencies    []string `json:"dependencies,omitempty"`
	Deployment      string   `json:"deployment,omitempty"`
	Details         string   `json:"details,omitempty"`
}
type Command struct {
	Command     string    `json:"command"`
	Directory   string    `json:"directory,omitempty"`
	OutcomeID   string    `json:"outcome_id"`
	ExitCode    int       `json:"exit_code"`
	Output      string    `json:"output,omitempty"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
}
type Artifact struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	MediaType string `json:"media_type"`
	Size      int    `json:"size"`
	SHA256    string `json:"sha256"`
	Data      string `json:"data,omitempty"`
}
type Cost struct {
	ComputeSeconds int     `json:"compute_seconds"`
	CostUnits      float64 `json:"cost_units"`
}
type Attempt struct {
	ID                 string      `json:"id"`
	RepositoryID       string      `json:"repository_id"`
	ThreadID           string      `json:"thread_id"`
	AnswerID           string      `json:"answer_id"`
	AnswerRevisionID   string      `json:"answer_revision_id"`
	WorkspaceID        string      `json:"workspace_id"`
	CommitID           string      `json:"commit_id"`
	DefinitionSHA256   string      `json:"definition_sha256"`
	SoftwareVersion    string      `json:"software_version"`
	Environment        Environment `json:"environment"`
	InputAttachmentIDs []string    `json:"input_attachment_ids"`
	InputSHA256        string      `json:"input_sha256"`
	Instructions       string      `json:"instructions"`
	InstructionsSHA256 string      `json:"instructions_sha256"`
	Commands           []Command   `json:"commands"`
	Artifacts          []Artifact  `json:"artifacts"`
	Cost               Cost        `json:"cost"`
	Result             string      `json:"result"`
	Notes              string      `json:"notes,omitempty"`
	ActorID            string      `json:"actor_id"`
	RerunOf            string      `json:"rerun_of,omitempty"`
	Stale              bool        `json:"stale"`
	StaleReasons       []string    `json:"stale_reasons"`
	CreatedAt          time.Time   `json:"created_at"`
}
type Store struct {
	root string
	mu   sync.Mutex
	now  func() time.Time
}

func New(root string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, ErrInvalid
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	return &Store{root: root, now: func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }}, nil
}
func (s *Store) Create(v Attempt) (Attempt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !valid(v) {
		return Attempt{}, ErrInvalid
	}
	v.ID = id()
	v.CreatedAt = s.now()
	v.Stale = false
	v.StaleReasons = []string{}
	for i := range v.Artifacts {
		v.Artifacts[i].ID = id()
	}
	if err := s.write(v); err != nil {
		return Attempt{}, err
	}
	return v, nil
}
func (s *Store) Get(repo, thread, attempt string) (Attempt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read(repo, thread, attempt)
}
func (s *Store) List(repo, thread string) ([]Attempt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := filepath.Join(s.root, repo, thread)
	es, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return []Attempt{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []Attempt{}
	for _, e := range es {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		v, x := s.read(repo, thread, strings.TrimSuffix(e.Name(), ".json"))
		if x != nil {
			return nil, x
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}
func valid(v Attempt) bool {
	if v.RepositoryID == "" || v.ThreadID == "" || v.AnswerID == "" || v.AnswerRevisionID == "" || v.WorkspaceID == "" || v.CommitID == "" || v.DefinitionSHA256 == "" || v.SoftwareVersion == "" || v.InputSHA256 == "" || v.InstructionsSHA256 == "" || strings.TrimSpace(v.Instructions) == "" || v.ActorID == "" || !map[string]bool{"passed": true, "failed": true, "inconclusive": true}[v.Result] || len(v.Commands) == 0 || len(v.Commands) > 20 || len(v.Artifacts) > 10 {
		return false
	}
	for _, c := range v.Commands {
		if strings.TrimSpace(c.Command) == "" || c.OutcomeID == "" || len(c.Command) > 4000 {
			return false
		}
	}
	for _, a := range v.Artifacts {
		if a.Name == "" || a.MediaType == "" || a.Size < 1 || a.Size > 1<<20 || len(a.SHA256) != 64 || a.Data == "" {
			return false
		}
	}
	return true
}
func (s *Store) read(repo, thread, id string) (Attempt, error) {
	var v Attempt
	b, e := os.ReadFile(filepath.Join(s.root, repo, thread, id+".json"))
	if os.IsNotExist(e) {
		return v, ErrNotFound
	}
	if e != nil {
		return v, e
	}
	if json.Unmarshal(b, &v) != nil || v.RepositoryID != repo || v.ThreadID != thread {
		return v, ErrInvalid
	}
	return v, nil
}
func (s *Store) write(v Attempt) error {
	d := filepath.Join(s.root, v.RepositoryID, v.ThreadID)
	if e := os.MkdirAll(d, 0700); e != nil {
		return e
	}
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(d, ".verification-*")
	if e != nil {
		return e
	}
	name := tmp.Name()
	defer os.Remove(name)
	_ = tmp.Chmod(0600)
	if _, e = tmp.Write(b); e == nil {
		e = tmp.Sync()
	}
	if c := tmp.Close(); e == nil {
		e = c
	}
	if e == nil {
		e = os.Rename(name, filepath.Join(d, v.ID+".json"))
	}
	return e
}
func id() string { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
