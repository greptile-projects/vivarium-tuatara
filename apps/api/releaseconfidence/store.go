// Package releaseconfidence retains revision-exact quality policy and evidence.
package releaseconfidence

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

var ErrNotFound = errors.New("release confidence record not found")
var ErrInvalid = errors.New("invalid release confidence record")
var ErrConflict = errors.New("release confidence version conflict")

type Selector struct {
	Branches  []string `json:"branches,omitempty"`
	Journeys  []string `json:"journeys,omitempty"`
	Risks     []string `json:"risk_classes,omitempty"`
	Locales   []string `json:"locales,omitempty"`
	Platforms []string `json:"platforms,omitempty"`
	Releases  []string `json:"releases,omitempty"`
	Paths     []string `json:"paths,omitempty"`
}
type Requirement struct {
	ID         string   `json:"id"`
	Title      string   `json:"title"`
	Kind       string   `json:"kind"` // scenario, exploratory_signoff, or test
	ResourceID string   `json:"resource_id,omitempty"`
	OwnerIDs   []string `json:"owner_ids"`
	Selector   Selector `json:"selector"`
}
type Policy struct {
	ID           string        `json:"id"`
	RepositoryID string        `json:"repository_id"`
	Version      int           `json:"version"`
	Requirements []Requirement `json:"requirements"`
	CreatedBy    string        `json:"created_by"`
	CreatedAt    time.Time     `json:"created_at"`
}
type Attempt struct {
	ID              string    `json:"id"`
	RepositoryID    string    `json:"repository_id"`
	RequirementID   string    `json:"requirement_id"`
	Revision        string    `json:"revision"`
	Status          string    `json:"status"` // passed, failed, flaky, gap, quarantined
	ScenarioID      string    `json:"scenario_id,omitempty"`
	SessionID       string    `json:"exploratory_session_id,omitempty"`
	CheckRunID      string    `json:"check_run_id,omitempty"`
	PullRequestID   string    `json:"pull_request_id,omitempty"`
	Environment     string    `json:"environment"`
	Journey         string    `json:"journey,omitempty"`
	RiskClass       string    `json:"risk_class,omitempty"`
	Locale          string    `json:"locale,omitempty"`
	Platform        string    `json:"platform,omitempty"`
	AffectedPaths   []string  `json:"affected_paths,omitempty"`
	DependencyPaths []string  `json:"dependency_paths,omitempty"`
	Summary         string    `json:"summary"`
	CreatedBy       string    `json:"created_by"`
	CreatedAt       time.Time `json:"created_at"`
}
type Override struct {
	ID            string    `json:"id"`
	RepositoryID  string    `json:"repository_id"`
	RequirementID string    `json:"requirement_id"`
	Revision      string    `json:"revision"`
	Rationale     string    `json:"rationale"`
	Scope         Selector  `json:"scope"`
	OwnerID       string    `json:"owner_id"`
	ExpiresAt     time.Time `json:"expires_at"`
	FollowUpKind  string    `json:"follow_up_kind"`
	FollowUpID    string    `json:"follow_up_id"`
	CreatedAt     time.Time `json:"created_at"`
}
type Target struct {
	Kind, ID, Revision, Branch, Release string
	ChangedPaths                        []string
}
type Cell struct {
	Requirement   Requirement `json:"requirement"`
	State         string      `json:"state"`
	Attempts      []Attempt   `json:"attempts"`
	StaleAttempts []Attempt   `json:"stale_attempts"`
	Override      *Override   `json:"override,omitempty"`
}
type Matrix struct {
	TargetKind    string `json:"target_kind"`
	TargetID      string `json:"target_id"`
	Revision      string `json:"revision"`
	PolicyID      string `json:"policy_id"`
	PolicyVersion int    `json:"policy_version"`
	Ready         bool   `json:"ready"`
	Cells         []Cell `json:"requirements"`
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

func (s *Store) Publish(repo, actor string, expected int, requirements []Requirement) (Policy, error) {
	var out Policy
	err := s.lock(func() error {
		policies, err := s.readPolicies(repo)
		if err != nil {
			return err
		}
		version := 1
		id := newID()
		if len(policies) > 0 {
			prior := policies[len(policies)-1]
			if expected != prior.Version {
				return ErrConflict
			}
			version = prior.Version + 1
			id = prior.ID
		} else if expected != 0 {
			return ErrConflict
		}
		if !validRequirements(requirements) {
			return ErrInvalid
		}
		out = Policy{ID: id, RepositoryID: repo, Version: version, Requirements: requirements, CreatedBy: actor, CreatedAt: s.now()}
		return appendJSON(filepath.Join(s.root, repo, "policies.jsonl"), out)
	})
	return out, err
}
func (s *Store) RecordAttempt(repo, actor string, in Attempt) (Attempt, error) {
	var out Attempt
	err := s.lock(func() error {
		p, e := s.current(repo)
		if e != nil {
			return e
		}
		if !requirementExists(p, in.RequirementID) || !validAttempt(in) {
			return ErrInvalid
		}
		in.ID = newID()
		in.RepositoryID = repo
		in.CreatedBy = actor
		in.CreatedAt = s.now()
		out = in
		return appendJSON(filepath.Join(s.root, repo, "attempts.jsonl"), in)
	})
	return out, err
}
func (s *Store) Override(repo, actor string, in Override) (Override, error) {
	var out Override
	err := s.lock(func() error {
		p, e := s.current(repo)
		if e != nil {
			return e
		}
		if !requirementOwner(p, in.RequirementID, actor) || len(strings.TrimSpace(in.Rationale)) < 10 || in.ExpiresAt.After(s.now().Add(30*24*time.Hour)) || !in.ExpiresAt.After(s.now()) || strings.TrimSpace(in.FollowUpKind) == "" || strings.TrimSpace(in.FollowUpID) == "" || len(in.Scope.Branches)+len(in.Scope.Journeys)+len(in.Scope.Risks)+len(in.Scope.Locales)+len(in.Scope.Platforms)+len(in.Scope.Releases) == 0 {
			return ErrInvalid
		}
		in.ID = newID()
		in.RepositoryID = repo
		in.OwnerID = actor
		in.CreatedAt = s.now()
		out = in
		return appendJSON(filepath.Join(s.root, repo, "overrides.jsonl"), in)
	})
	return out, err
}
func (s *Store) Matrix(repo string, target Target) (Matrix, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, e := s.current(repo)
	if e != nil {
		return Matrix{}, e
	}
	attempts, e := readLines[Attempt](filepath.Join(s.root, repo, "attempts.jsonl"))
	if e != nil {
		return Matrix{}, e
	}
	overrides, e := readLines[Override](filepath.Join(s.root, repo, "overrides.jsonl"))
	if e != nil {
		return Matrix{}, e
	}
	m := Matrix{TargetKind: target.Kind, TargetID: target.ID, Revision: target.Revision, PolicyID: p.ID, PolicyVersion: p.Version, Ready: true, Cells: []Cell{}}
	for _, req := range p.Requirements {
		if !matches(req.Selector, target) {
			continue
		}
		cell := Cell{Requirement: req, State: "gap", Attempts: []Attempt{}, StaleAttempts: []Attempt{}}
		for _, a := range attempts {
			if a.RequirementID != req.ID {
				continue
			}
			if attemptMatches(req.Selector, a) && (a.Revision == target.Revision || !invalidated(a, target.ChangedPaths)) {
				cell.Attempts = append(cell.Attempts, a)
			} else {
				cell.StaleAttempts = append(cell.StaleAttempts, a)
			}
		}
		if len(cell.Attempts) > 0 {
			cell.State = cell.Attempts[len(cell.Attempts)-1].Status
		}
		for i := len(overrides) - 1; i >= 0; i-- {
			o := overrides[i]
			if o.RequirementID == req.ID && o.Revision == target.Revision && o.ExpiresAt.After(s.now()) && matches(o.Scope, target) {
				x := o
				cell.Override = &x
				cell.State = "overridden"
				break
			}
		}
		if cell.State != "passed" && cell.State != "overridden" {
			m.Ready = false
		}
		m.Cells = append(m.Cells, cell)
	}
	return m, nil
}
func matches(s Selector, t Target) bool {
	return one(s.Branches, t.Branch) && one(s.Releases, t.Release) && pathsIntersect(s.Paths, t.ChangedPaths)
}
func one(xs []string, v string) bool {
	if len(xs) == 0 {
		return true
	}
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
func attemptMatches(s Selector, a Attempt) bool {
	return one(s.Journeys, a.Journey) && one(s.Risks, a.RiskClass) && one(s.Locales, a.Locale) && one(s.Platforms, a.Platform)
}
func invalidated(a Attempt, changed []string) bool {
	if len(changed) == 0 {
		return true
	}
	deps := append(append([]string{}, a.AffectedPaths...), a.DependencyPaths...)
	if len(deps) == 0 {
		return true
	}
	for _, c := range changed {
		for _, p := range deps {
			if c == p || strings.HasPrefix(c, strings.TrimSuffix(p, "/")+"/") {
				return true
			}
		}
	}
	return false
}
func pathsIntersect(scope, changed []string) bool {
	if len(scope) == 0 {
		return true
	}
	for _, c := range changed {
		for _, p := range scope {
			if c == p || strings.HasPrefix(c, strings.TrimSuffix(p, "/")+"/") {
				return true
			}
		}
	}
	return false
}
func validAttempt(a Attempt) bool {
	ok := map[string]bool{"passed": true, "failed": true, "flaky": true, "gap": true, "quarantined": true}[a.Status]
	return ok && len(a.Revision) == 40 && strings.TrimSpace(a.Environment) != "" && strings.TrimSpace(a.Summary) != ""
}
func validRequirements(rs []Requirement) bool {
	if len(rs) == 0 {
		return false
	}
	seen := map[string]bool{}
	for _, r := range rs {
		if r.ID == "" || r.Title == "" || !map[string]bool{"scenario": true, "exploratory_signoff": true, "test": true}[r.Kind] || len(r.OwnerIDs) == 0 || seen[r.ID] {
			return false
		}
		seen[r.ID] = true
	}
	return true
}
func requirementExists(p Policy, id string) bool {
	for _, r := range p.Requirements {
		if r.ID == id {
			return true
		}
	}
	return false
}
func requirementOwner(p Policy, id, actor string) bool {
	for _, r := range p.Requirements {
		if r.ID == id {
			for _, o := range r.OwnerIDs {
				if o == actor {
					return true
				}
			}
		}
	}
	return false
}
func (s *Store) current(repo string) (Policy, error) {
	xs, e := s.readPolicies(repo)
	if e != nil {
		return Policy{}, e
	}
	if len(xs) == 0 {
		return Policy{}, ErrNotFound
	}
	return xs[len(xs)-1], nil
}
func (s *Store) readPolicies(repo string) ([]Policy, error) {
	return readLines[Policy](filepath.Join(s.root, repo, "policies.jsonl"))
}
func readLines[T any](path string) ([]T, error) {
	b, e := os.ReadFile(path)
	if errors.Is(e, os.ErrNotExist) {
		return []T{}, nil
	}
	if e != nil {
		return nil, e
	}
	out := []T{}
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if line == "" {
			continue
		}
		var v T
		if json.Unmarshal([]byte(line), &v) != nil {
			return nil, ErrInvalid
		}
		out = append(out, v)
	}
	return out, nil
}
func appendJSON(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, e := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	b, e := json.Marshal(v)
	if e == nil {
		_, e = f.Write(append(b, '\n'))
	}
	if e == nil {
		e = f.Sync()
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
func newID() string { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
