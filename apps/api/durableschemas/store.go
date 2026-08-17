// Package durableschemas persists reviewed persistent-state contracts and migration plans.
package durableschemas

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

var ErrNotFound = errors.New("durable schema not found")
var ErrInvalid = errors.New("invalid durable schema")
var ErrConflict = errors.New("durable schema version conflict")

type Link struct {
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	Label string `json:"label"`
}
type Revision struct {
	Version        int       `json:"version"`
	Name           string    `json:"name"`
	StoreKind      string    `json:"store_kind"`
	Description    string    `json:"description"`
	Definition     string    `json:"definition"`
	DefinitionPath string    `json:"definition_path"`
	OwnerIDs       []string  `json:"owner_ids"`
	Compatibility  []string  `json:"compatibility"`
	Retention      string    `json:"retention"`
	Privacy        []string  `json:"privacy"`
	Links          []Link    `json:"links"`
	PullRequestID  string    `json:"pull_request_id"`
	ReviewedCommit string    `json:"reviewed_commit"`
	Rationale      string    `json:"rationale"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
}
type Operation struct {
	ID            string   `json:"id"`
	Kind          string   `json:"kind"`
	Description   string   `json:"description"`
	OwnerIDs      []string `json:"owner_ids"`
	ConsumerIDs   []string `json:"consumer_ids"`
	Destructive   bool     `json:"destructive"`
	RollbackLimit string   `json:"rollback_limit"`
}
type Step struct {
	ID                  string   `json:"id"`
	OperationIDs        []string `json:"operation_ids"`
	Description         string   `json:"description"`
	SuccessMeasures     []string `json:"success_measures"`
	RequiredApproverIDs []string `json:"required_approver_ids"`
}
type Event struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	StepID    string    `json:"step_id,omitempty"`
	Summary   string    `json:"summary"`
	ActorID   string    `json:"actor_id"`
	CreatedAt time.Time `json:"created_at"`
}
type Migration struct {
	ID             string      `json:"id"`
	FromVersion    int         `json:"from_version"`
	ToVersion      int         `json:"to_version"`
	SourceKind     string      `json:"source_kind"`
	SourceID       string      `json:"source_id"`
	Summary        string      `json:"summary"`
	Operations     []Operation `json:"operations"`
	Steps          []Step      `json:"steps"`
	RollbackLimits []string    `json:"rollback_limits"`
	Version        int         `json:"version"`
	Events         []Event     `json:"events"`
	CreatedBy      string      `json:"created_by"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
}
type Schema struct {
	ID             string      `json:"id"`
	RepositoryID   string      `json:"repository_id"`
	CurrentVersion int         `json:"current_version"`
	Revisions      []Revision  `json:"revisions"`
	Migrations     []Migration `json:"migrations"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
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
func (s *Store) Create(repo, actor string, r Revision) (Schema, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if validateRevision(r) != nil {
		return Schema{}, ErrInvalid
	}
	now := s.now()
	r.Version = 1
	r.CreatedBy = actor
	r.CreatedAt = now
	v := Schema{ID: id(), RepositoryID: repo, CurrentVersion: 1, Revisions: []Revision{r}, Migrations: []Migration{}, CreatedAt: now, UpdatedAt: now}
	return v, s.write(v)
}
func (s *Store) Revise(repo, schema string, expected int, actor string, r Revision) (Schema, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, e := s.read(repo, schema)
	if e != nil {
		return Schema{}, e
	}
	if v.CurrentVersion != expected {
		return Schema{}, ErrConflict
	}
	if validateRevision(r) != nil {
		return Schema{}, ErrInvalid
	}
	r.Version = expected + 1
	r.CreatedBy = actor
	r.CreatedAt = s.now()
	v.CurrentVersion = r.Version
	v.Revisions = append(v.Revisions, r)
	v.UpdatedAt = r.CreatedAt
	return v, s.write(v)
}
func (s *Store) AddMigration(repo, schema, actor string, m Migration) (Schema, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, e := s.read(repo, schema)
	if e != nil {
		return Schema{}, e
	}
	if validateMigration(v, m) != nil {
		return Schema{}, ErrInvalid
	}
	now := s.now()
	m.ID = id()
	m.Version = 1
	m.CreatedBy = actor
	m.CreatedAt = now
	m.UpdatedAt = now
	m.Events = []Event{{ID: id(), Kind: "created", Summary: m.Summary, ActorID: actor, CreatedAt: now}}
	v.Migrations = append(v.Migrations, m)
	v.UpdatedAt = now
	return v, s.write(v)
}
func (s *Store) AddEvent(repo, schema, migration, actor string, expected int, e Event) (Schema, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, x := s.read(repo, schema)
	if x != nil {
		return Schema{}, x
	}
	for i := range v.Migrations {
		m := &v.Migrations[i]
		if m.ID != migration {
			continue
		}
		if m.Version != expected {
			return Schema{}, ErrConflict
		}
		if strings.TrimSpace(e.Kind) == "" || strings.TrimSpace(e.Summary) == "" {
			return Schema{}, ErrInvalid
		}
		if e.Kind == "approved" {
			authorized := false
			for _, step := range m.Steps {
				if step.ID != e.StepID {
					continue
				}
				for _, approver := range step.RequiredApproverIDs {
					if approver == actor {
						authorized = true
						break
					}
				}
			}
			if !authorized {
				return Schema{}, ErrInvalid
			}
		}
		e.ID = id()
		e.ActorID = actor
		e.CreatedAt = s.now()
		m.Version++
		m.Events = append(m.Events, e)
		m.UpdatedAt = e.CreatedAt
		v.UpdatedAt = e.CreatedAt
		return v, s.write(v)
	}
	return Schema{}, ErrNotFound
}
func (s *Store) Get(repo, schema string) (Schema, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read(repo, schema)
}
func (s *Store) List(repo string) ([]Schema, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := s.repo(repo)
	entries, e := os.ReadDir(dir)
	if os.IsNotExist(e) {
		return []Schema{}, nil
	}
	if e != nil {
		return nil, e
	}
	out := []Schema{}
	for _, x := range entries {
		if x.IsDir() || !strings.HasSuffix(x.Name(), ".json") {
			continue
		}
		b, e := os.ReadFile(filepath.Join(dir, x.Name()))
		if e != nil {
			return nil, e
		}
		var v Schema
		if json.Unmarshal(b, &v) != nil {
			return nil, ErrInvalid
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}
func validateRevision(r Revision) error {
	k := map[string]bool{"database": true, "queue": true, "index": true, "object_store": true, "event_log": true, "cache": true, "other": true}
	if r.Name == "" || !k[r.StoreKind] || r.Description == "" || r.Definition == "" || r.DefinitionPath == "" || len(r.OwnerIDs) == 0 || len(r.Compatibility) == 0 || r.Retention == "" || len(r.Privacy) == 0 || r.PullRequestID == "" || r.ReviewedCommit == "" || r.Rationale == "" {
		return ErrInvalid
	}
	for _, l := range r.Links {
		if (l.Kind != "service" && l.Kind != "environment") || l.ID == "" || l.Label == "" {
			return ErrInvalid
		}
	}
	return nil
}
func validateMigration(s Schema, m Migration) error {
	if m.FromVersion < 1 || m.ToVersion < 1 || m.FromVersion >= m.ToVersion || m.ToVersion > s.CurrentVersion || (m.SourceKind != "pull_request" && m.SourceKind != "decision") || m.SourceID == "" || m.Summary == "" || len(m.Operations) == 0 || len(m.Steps) == 0 || len(m.RollbackLimits) == 0 {
		return ErrInvalid
	}
	ops := map[string]bool{}
	k := map[string]bool{"read": true, "write": true, "backfill": true, "destructive": true}
	for _, o := range m.Operations {
		if o.ID == "" || ops[o.ID] || !k[o.Kind] || o.Description == "" || len(o.OwnerIDs) == 0 || len(o.ConsumerIDs) == 0 || o.RollbackLimit == "" || (o.Kind == "destructive" && !o.Destructive) {
			return ErrInvalid
		}
		ops[o.ID] = true
	}
	steps := map[string]bool{}
	coveredOperations := map[string]bool{}
	for _, st := range m.Steps {
		if st.ID == "" || steps[st.ID] || st.Description == "" || len(st.OperationIDs) == 0 || len(st.SuccessMeasures) == 0 || len(st.RequiredApproverIDs) == 0 {
			return ErrInvalid
		}
		steps[st.ID] = true
		for _, o := range st.OperationIDs {
			if !ops[o] {
				return ErrInvalid
			}
			coveredOperations[o] = true
		}
	}
	if len(coveredOperations) != len(ops) {
		return ErrInvalid
	}
	return nil
}
func (s *Store) repo(repo string) string {
	return filepath.Join(s.root, "repo-"+hex.EncodeToString([]byte(repo)))
}
func (s *Store) read(repo, schema string) (Schema, error) {
	b, e := os.ReadFile(filepath.Join(s.repo(repo), schema+".json"))
	if os.IsNotExist(e) {
		return Schema{}, ErrNotFound
	}
	if e != nil {
		return Schema{}, e
	}
	var v Schema
	if json.Unmarshal(b, &v) != nil || v.RepositoryID != repo || v.ID != schema {
		return Schema{}, ErrInvalid
	}
	return v, nil
}
func (s *Store) write(v Schema) error {
	d := s.repo(v.RepositoryID)
	if e := os.MkdirAll(d, 0700); e != nil {
		return e
	}
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(d, ".schema-")
	if e != nil {
		return e
	}
	name := tmp.Name()
	defer os.Remove(name)
	if e = tmp.Chmod(0600); e == nil {
		_, e = tmp.Write(b)
	}
	if e == nil {
		e = tmp.Sync()
	}
	if x := tmp.Close(); e == nil {
		e = x
	}
	if e != nil {
		return e
	}
	return os.Rename(name, filepath.Join(d, v.ID+".json"))
}
func id() string { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
