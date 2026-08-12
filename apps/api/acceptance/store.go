// Package acceptance retains repository preview-acceptance policy and exact-revision decisions.
package acceptance

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

var ErrInvalid = errors.New("invalid preview acceptance")
var ErrNotFound = errors.New("preview acceptance not found")
var ErrConflict = errors.New("preview acceptance idempotency key conflict")
var ErrRejected = errors.New("preview acceptance rejection requires owner override")
var ErrDurabilityUncertain = errors.New("preview acceptance is visible but durability is uncertain")

type Scenario struct {
	Name     string `json:"name"`
	Role     string `json:"role"`
	Blocking bool   `json:"blocking"`
}
type Requirement struct {
	ID          string     `json:"id"`
	Paths       []string   `json:"paths"`
	RiskClasses []string   `json:"risk_classes"`
	Scenarios   []Scenario `json:"scenarios"`
}
type Policy struct {
	RepositoryID string        `json:"repository_id"`
	Branch       string        `json:"branch"`
	Version      int           `json:"version"`
	Requirements []Requirement `json:"requirements"`
	UpdatedBy    string        `json:"updated_by"`
	UpdatedAt    time.Time     `json:"updated_at"`
}
type Decision struct {
	ID             string    `json:"id"`
	RepositoryID   string    `json:"repository_id"`
	PullRequestID  string    `json:"pull_request_id"`
	Revision       string    `json:"revision"`
	PolicyVersion  int       `json:"policy_version"`
	IdempotencyKey string    `json:"idempotency_key"`
	RequirementID  string    `json:"requirement_id"`
	Scenario       string    `json:"scenario"`
	Role           string    `json:"role"`
	Outcome        string    `json:"outcome"`
	RiskClasses    []string  `json:"risk_classes,omitempty"`
	Rationale      string    `json:"rationale,omitempty"`
	ActorID        string    `json:"actor_id"`
	CreatedAt      time.Time `json:"created_at"`
}
type Finding struct {
	ID        string `json:"id"`
	PreviewID string `json:"preview_id"`
	Revision  string `json:"revision"`
	Title     string `json:"title"`
	Severity  string `json:"severity"`
	Status    string `json:"status"`
	AuthorID  string `json:"author_id"`
}
type Evaluation struct {
	Revision       string          `json:"revision"`
	PolicyVersion  int             `json:"policy_version"`
	Applicable     []Requirement   `json:"applicable"`
	Decisions      []Decision      `json:"decisions"`
	StaleDecisions []Decision      `json:"stale_decisions"`
	Findings       []Finding       `json:"findings"`
	Missing        []ScenarioState `json:"missing"`
	Blocking       bool            `json:"blocking"`
}
type ScenarioState struct {
	RequirementID string `json:"requirement_id"`
	Scenario      string `json:"scenario"`
	Role          string `json:"role"`
}

type Store struct {
	root string
	mu   sync.Mutex
	now  func() time.Time
}

func (s *Store) lock() (func(), error) {
	f, err := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}

func New(root string) (*Store, error) {
	if root == "" {
		return nil, ErrInvalid
	}
	a, e := filepath.Abs(root)
	if e != nil {
		return nil, e
	}
	if e = os.MkdirAll(a, 0700); e != nil {
		return nil, e
	}
	return &Store{root: a, now: func() time.Time { return time.Now().UTC() }}, nil
}
func clean(v string) bool {
	if strings.TrimSpace(v) != v || v == "" || len(v) > 100 {
		return false
	}
	for _, r := range v {
		if !(r == '-' || r == '_' || r == '.' || r == '/' || r == '*' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == ' ') {
			return false
		}
	}
	return true
}
func validPolicy(p Policy) bool {
	if !clean(p.RepositoryID) || !clean(p.Branch) || len(p.Requirements) > 20 {
		return false
	}
	ids := map[string]bool{}
	for _, q := range p.Requirements {
		if !clean(q.ID) || ids[q.ID] || len(q.Scenarios) == 0 || len(q.Scenarios) > 20 || len(q.Paths) > 50 || len(q.RiskClasses) > 20 {
			return false
		}
		ids[q.ID] = true
		for _, x := range append(append([]string{}, q.Paths...), q.RiskClasses...) {
			if !clean(x) {
				return false
			}
		}
		seen := map[string]bool{}
		for _, s := range q.Scenarios {
			if !clean(s.Name) || (s.Role != "owner" && s.Role != "contributor" && s.Role != "author" && s.Role != "stakeholder") || seen[s.Name+"\x00"+s.Role] {
				return false
			}
			seen[s.Name+"\x00"+s.Role] = true
		}
	}
	return true
}
func (s *Store) policyPath(repo, branch string) string {
	return filepath.Join(s.root, "policies", repo, hex.EncodeToString([]byte(branch))+".json")
}
func read[T any](path string) (T, error) {
	var v T
	b, e := os.ReadFile(path)
	if errors.Is(e, os.ErrNotExist) {
		return v, ErrNotFound
	}
	if e != nil {
		return v, e
	}
	e = json.Unmarshal(b, &v)
	return v, e
}
func write(path string, v any) (bool, error) {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return false, e
	}
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return false, e
	}
	tmp, e := os.CreateTemp(filepath.Dir(path), ".acceptance-*.tmp")
	if e != nil {
		return false, e
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if e = tmp.Chmod(0600); e == nil {
		_, e = tmp.Write(b)
	}
	if e == nil {
		e = tmp.Sync()
	}
	closeErr := tmp.Close()
	if e == nil {
		e = closeErr
	}
	if e != nil {
		return false, e
	}
	if e = os.Rename(tmpName, path); e != nil {
		return false, e
	}
	directory, e := os.Open(filepath.Dir(path))
	if e != nil {
		return true, e
	}
	e = directory.Sync()
	closeErr = directory.Close()
	if e == nil {
		e = closeErr
	}
	if e != nil {
		return true, e
	}
	return true, nil
}
func (s *Store) Policy(repo, branch string) (Policy, error) {
	p, e := read[Policy](s.policyPath(repo, branch))
	if errors.Is(e, ErrNotFound) {
		return Policy{RepositoryID: repo, Branch: branch, Requirements: []Requirement{}}, nil
	}
	return p, e
}
func (s *Store) SetPolicy(repo, branch, actor string, requirements []Requirement) (Policy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Policy{}, err
	}
	defer unlock()
	old, e := s.Policy(repo, branch)
	if e != nil {
		return Policy{}, e
	}
	p := Policy{RepositoryID: repo, Branch: branch, Version: old.Version + 1, Requirements: requirements, UpdatedBy: actor, UpdatedAt: s.now()}
	if !validPolicy(p) || !clean(actor) {
		return Policy{}, ErrInvalid
	}
	committed, writeErr := write(s.policyPath(repo, branch), p)
	if committed && writeErr != nil {
		return p, errors.Join(ErrDurabilityUncertain, writeErr)
	}
	return p, writeErr
}
func (s *Store) decisionsPath(repo, pull string) string {
	return filepath.Join(s.root, "decisions", repo, pull+".json")
}
func (s *Store) Decisions(repo, pull string) ([]Decision, error) {
	v, e := read[[]Decision](s.decisionsPath(repo, pull))
	if errors.Is(e, ErrNotFound) {
		return []Decision{}, nil
	}
	return v, e
}
func (s *Store) Decide(d Decision) (Decision, error) {
	if !clean(d.RepositoryID) || !clean(d.PullRequestID) || len(d.Revision) != 40 || d.PolicyVersion < 1 || !clean(d.IdempotencyKey) || !clean(d.RequirementID) || !clean(d.Scenario) || !clean(d.Role) || (d.Outcome != "accepted" && d.Outcome != "rejected" && d.Outcome != "overridden") || !clean(d.ActorID) || (d.Outcome != "accepted" && strings.TrimSpace(d.Rationale) == "") || len(d.Rationale) > 2000 {
		return Decision{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Decision{}, err
	}
	defer unlock()
	all, e := s.Decisions(d.RepositoryID, d.PullRequestID)
	if e != nil {
		return Decision{}, e
	}
	for _, existing := range all {
		if existing.IdempotencyKey != d.IdempotencyKey {
			continue
		}
		if existing.RepositoryID == d.RepositoryID && existing.PullRequestID == d.PullRequestID && existing.Revision == d.Revision && existing.PolicyVersion == d.PolicyVersion && existing.RequirementID == d.RequirementID && existing.Scenario == d.Scenario && existing.Role == d.Role && existing.Outcome == d.Outcome && existing.ActorID == d.ActorID && existing.Rationale == d.Rationale && slices.Equal(existing.RiskClasses, d.RiskClasses) {
			return existing, nil
		}
		return Decision{}, ErrConflict
	}
	if d.Outcome == "accepted" {
		for i := len(all) - 1; i >= 0; i-- {
			existing := all[i]
			if existing.Revision != d.Revision || existing.PolicyVersion != d.PolicyVersion || existing.RequirementID != d.RequirementID || existing.Scenario != d.Scenario || existing.Role != d.Role {
				continue
			}
			if existing.Outcome == "overridden" {
				break
			}
			if existing.Outcome == "rejected" {
				return Decision{}, ErrRejected
			}
		}
	}
	raw := make([]byte, 16)
	_, _ = rand.Read(raw)
	d.ID = hex.EncodeToString(raw)
	d.CreatedAt = s.now()
	all = append(all, d)
	committed, writeErr := write(s.decisionsPath(d.RepositoryID, d.PullRequestID), all)
	if committed && writeErr != nil {
		return d, errors.Join(ErrDurabilityUncertain, writeErr)
	}
	return d, writeErr
}
func Matches(q Requirement, paths, risks []string) bool {
	if len(q.Paths) == 0 && len(q.RiskClasses) == 0 {
		return true
	}
	pathMatch := false
	for _, pattern := range q.Paths {
		for _, p := range paths {
			if ok, _ := filepath.Match(pattern, p); ok || strings.HasSuffix(pattern, "/**") && strings.HasPrefix(p, strings.TrimSuffix(pattern, "**")) {
				pathMatch = true
			}
		}
	}
	// Risk classes are owner-authored labels on the requirement. They apply to
	// every pull on the selected branch unless a narrower path selector is used;
	// callers cannot evade a gate by omitting or inventing a classification.
	riskMatch := len(q.RiskClasses) > 0
	return pathMatch || riskMatch
}
func Evaluate(policy Policy, revision string, paths, risks []string, decisions []Decision, findings []Finding) Evaluation {
	e := Evaluation{Revision: revision, PolicyVersion: policy.Version, Applicable: []Requirement{}, Decisions: []Decision{}, StaleDecisions: []Decision{}, Findings: findings, Missing: []ScenarioState{}}
	for _, d := range decisions {
		if d.Revision == revision && d.PolicyVersion == policy.Version {
			e.Decisions = append(e.Decisions, d)
		} else {
			e.StaleDecisions = append(e.StaleDecisions, d)
		}
	}
	for _, q := range policy.Requirements {
		if !Matches(q, paths, risks) {
			continue
		}
		e.Applicable = append(e.Applicable, q)
		for _, s := range q.Scenarios {
			if !s.Blocking {
				continue
			}
			resolved := false
			for i := len(e.Decisions) - 1; i >= 0; i-- {
				d := e.Decisions[i]
				if d.RequirementID == q.ID && d.Scenario == s.Name && d.Role == s.Role {
					if d.Outcome == "overridden" {
						resolved = true
						break
					}
					if d.Outcome == "rejected" {
						resolved = false
						e.Blocking = true
						break
					}
					if d.Outcome == "accepted" {
						resolved = true
					}
				}
			}
			if !resolved {
				e.Missing = append(e.Missing, ScenarioState{q.ID, s.Name, s.Role})
				e.Blocking = true
			}
		}
	}
	for _, f := range findings {
		if f.Revision == revision && f.Severity == "blocking" && f.Status != "resolved" {
			e.Blocking = true
		}
	}
	sort.Slice(e.StaleDecisions, func(i, j int) bool { return e.StaleDecisions[i].CreatedAt.Before(e.StaleDecisions[j].CreatedAt) })
	return e
}
