// Package dataobservations retains sanitized production-derived evidence and governed remediation.
package dataobservations

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
	"syscall"
	"time"
)

var ErrNotFound = errors.New("data observation not found")
var ErrInvalid = errors.New("invalid data observation")
var ErrConflict = errors.New("data observation changed")

type Evidence struct {
	Kind        string    `json:"kind"`
	Digest      string    `json:"digest"`
	WindowStart time.Time `json:"window_start"`
	WindowEnd   time.Time `json:"window_end"`
	SampleCount int       `json:"sample_count"`
}
type Scope struct {
	Revision                string `json:"revision"`
	DataFlowID              string `json:"data_flow_id"`
	DataFlowVersion         int    `json:"data_flow_version"`
	CommitmentID            string `json:"commitment_id"`
	CommitmentVersion       int    `json:"commitment_version"`
	DataUseID               string `json:"data_use_id"`
	ReleaseID               string `json:"release_id"`
	EnvironmentID           string `json:"environment_id"`
	DeploymentID            string `json:"deployment_id"`
	ExtensionInstallationID string `json:"extension_installation_id,omitempty"`
}
type Action struct {
	ID             string     `json:"id"`
	Kind           string     `json:"kind"`
	Rationale      string     `json:"rationale"`
	ParticipantIDs []string   `json:"participant_ids,omitempty"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	CreatedBy      string     `json:"created_by"`
	CreatedAt      time.Time  `json:"created_at"`
}
type Repair struct {
	ProposalID   string    `json:"proposal_id"`
	TaskID       string    `json:"task_id"`
	AssigneeType string    `json:"assignee_type"`
	AssigneeID   string    `json:"assignee_id"`
	CreatedBy    string    `json:"created_by"`
	CreatedAt    time.Time `json:"created_at"`
}
type Observation struct {
	ID            string     `json:"id"`
	RepositoryID  string     `json:"repository_id"`
	Version       int        `json:"version"`
	SignalKind    string     `json:"signal_kind"`
	Severity      string     `json:"severity"`
	Scope         Scope      `json:"scope"`
	OwnerIDs      []string   `json:"owner_ids"`
	Evidence      []Evidence `json:"evidence"`
	Status        string     `json:"status"`
	Actions       []Action   `json:"actions"`
	Repair        *Repair    `json:"repair,omitempty"`
	CreatedByType string     `json:"created_by_type"`
	CreatedBy     string     `json:"created_by"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
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
func (s *Store) Create(repo, actorType, actor string, v Observation) (Observation, error) {
	var out Observation
	err := s.lock(func() error {
		if validate(v) != nil {
			return ErrInvalid
		}
		now := s.now()
		v.ID = randomID()
		v.RepositoryID = repo
		v.Version = 1
		v.Status = "open"
		v.Actions = []Action{}
		v.Repair = nil
		v.CreatedByType = actorType
		v.CreatedBy = actor
		v.CreatedAt = now
		v.UpdatedAt = now
		out = v
		return s.write(v)
	})
	return out, err
}
func (s *Store) Get(repo, id string) (Observation, error) {
	var out Observation
	err := s.lock(func() error { var e error; out, e = s.read(repo, id); return e })
	return out, err
}
func (s *Store) List(repo string) ([]Observation, error) {
	out := []Observation{}
	err := s.lock(func() error {
		entries, e := os.ReadDir(s.repoDir(repo))
		if os.IsNotExist(e) {
			return nil
		}
		if e != nil {
			return e
		}
		for _, x := range entries {
			if x.IsDir() || !strings.HasSuffix(x.Name(), ".json") {
				continue
			}
			v, e := s.read(repo, strings.TrimSuffix(x.Name(), ".json"))
			if e != nil {
				return e
			}
			out = append(out, v)
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, err
}
func (s *Store) AddAction(repo, id, actor string, expected int, a Action) (Observation, error) {
	var out Observation
	err := s.lock(func() error {
		v, e := s.read(repo, id)
		if e != nil {
			return e
		}
		if v.Version != expected {
			return ErrConflict
		}
		if !validAction(a, s.now()) {
			return ErrInvalid
		}
		now := s.now()
		a.ID = randomID()
		a.CreatedBy = actor
		a.CreatedAt = now
		v.Actions = append(v.Actions, a)
		v.Version++
		v.UpdatedAt = now
		if a.Kind == "contain" {
			v.Status = "contained"
		}
		out = v
		return s.write(v)
	})
	return out, err
}
func (s *Store) LinkRepair(repo, id, actor string, expected int, r Repair) (Observation, error) {
	var out Observation
	err := s.lock(func() error {
		v, e := s.read(repo, id)
		if e != nil {
			return e
		}
		if v.Version != expected {
			return ErrConflict
		}
		if v.Repair != nil {
			return ErrConflict
		}
		if r.ProposalID == "" || r.TaskID == "" || (r.AssigneeType != "human" && r.AssigneeType != "agent") || r.AssigneeID == "" {
			return ErrInvalid
		}
		now := s.now()
		r.CreatedBy = actor
		r.CreatedAt = now
		v.Repair = &r
		v.Version++
		v.Status = "repair_planned"
		v.UpdatedAt = now
		out = v
		return s.write(v)
	})
	return out, err
}
func validate(v Observation) error {
	kinds := map[string]bool{"undeclared_flow": true, "excessive_retention": true, "failed_deletion": true, "consent_mismatch": true, "unexpected_recipient": true}
	sevs := map[string]bool{"warning": true, "blocking": true}
	if !kinds[v.SignalKind] || !sevs[v.Severity] || len(v.OwnerIDs) == 0 || len(v.OwnerIDs) > 20 || len(v.Evidence) == 0 || len(v.Evidence) > 20 || len(v.Scope.Revision) != 40 || v.Scope.DataFlowID == "" || v.Scope.DataFlowVersion < 1 || v.Scope.CommitmentID == "" || v.Scope.CommitmentVersion < 1 || v.Scope.DataUseID == "" || v.Scope.ReleaseID == "" || v.Scope.EnvironmentID == "" || v.Scope.DeploymentID == "" {
		return ErrInvalid
	}
	if _, err := hex.DecodeString(v.Scope.Revision); err != nil || v.Scope.Revision != strings.ToLower(v.Scope.Revision) {
		return ErrInvalid
	}
	for _, e := range v.Evidence {
		if (e.Kind != "aggregate" && e.Kind != "trace" && e.Kind != "deletion_receipt" && e.Kind != "recipient_audit" && e.Kind != "consent_audit") || len(e.Digest) != 64 || e.SampleCount < 1 || e.SampleCount > 1000000 || e.WindowStart.IsZero() || !e.WindowEnd.After(e.WindowStart) || e.WindowEnd.Sub(e.WindowStart) > 31*24*time.Hour {
			return ErrInvalid
		}
		if _, x := hex.DecodeString(e.Digest); x != nil {
			return ErrInvalid
		}
	}
	return nil
}
func validAction(a Action, now time.Time) bool {
	if a.Rationale == "" || len(a.Rationale) > 2000 || len(a.ParticipantIDs) > 20 {
		return false
	}
	seen := map[string]bool{}
	for _, id := range a.ParticipantIDs {
		if strings.TrimSpace(id) == "" || seen[id] {
			return false
		}
		seen[id] = true
	}
	switch a.Kind {
	case "contain":
		return a.ExpiresAt == nil
	case "notify", "private_incident":
		return a.ExpiresAt == nil && len(a.ParticipantIDs) > 0
	case "governed_exception":
		return a.ExpiresAt != nil && len(a.ParticipantIDs) > 0 && a.ExpiresAt.After(now) && a.ExpiresAt.Before(now.Add(91*24*time.Hour))
	default:
		return false
	}
}
func (s *Store) repoDir(repo string) string {
	return filepath.Join(s.root, "repo-"+hex.EncodeToString([]byte(repo)))
}
func (s *Store) read(repo, id string) (Observation, error) {
	if id == "" || strings.ContainsAny(id, "/\\") {
		return Observation{}, ErrNotFound
	}
	b, e := os.ReadFile(filepath.Join(s.repoDir(repo), id+".json"))
	if os.IsNotExist(e) {
		return Observation{}, ErrNotFound
	}
	if e != nil {
		return Observation{}, e
	}
	var v Observation
	if json.Unmarshal(b, &v) != nil || v.ID != id || v.RepositoryID != repo {
		return Observation{}, ErrInvalid
	}
	return v, nil
}
func (s *Store) write(v Observation) error {
	d := s.repoDir(v.RepositoryID)
	if e := os.MkdirAll(d, 0700); e != nil {
		return e
	}
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	t, e := os.CreateTemp(d, ".observation-*")
	if e != nil {
		return e
	}
	n := t.Name()
	defer os.Remove(n)
	if e = t.Chmod(0600); e == nil {
		_, e = t.Write(b)
	}
	if e == nil {
		e = t.Sync()
	}
	ce := t.Close()
	if e == nil {
		e = ce
	}
	if e == nil {
		e = os.Rename(n, filepath.Join(d, v.ID+".json"))
	}
	return e
}
func (s *Store) lock(fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, e := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); e != nil {
		return e
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return fn()
}
func randomID() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
