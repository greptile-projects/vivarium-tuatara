// Package designgovernance retains scoped interface acceptance policy and its attributable decisions.
package designgovernance

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

var ErrInvalid = errors.New("invalid design governance record")
var ErrNotFound = errors.New("design governance record not found")
var ErrConflict = errors.New("design governance version conflict")

type Selector struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}
type Requirement struct {
	Role        string   `json:"role"`
	ApproverIDs []string `json:"approver_ids"`
}
type Policy struct {
	ID                string        `json:"id"`
	ScopeKind         string        `json:"scope_kind"`
	ScopeID           string        `json:"scope_id"`
	Version           int           `json:"version"`
	Name              string        `json:"name"`
	Selectors         []Selector    `json:"selectors"`
	Requirements      []Requirement `json:"requirements"`
	ExceptionMaxHours int           `json:"exception_max_hours"`
	CreatedBy         string        `json:"created_by"`
	CreatedAt         time.Time     `json:"created_at"`
}
type Acceptance struct {
	ID            string    `json:"id"`
	PolicyID      string    `json:"policy_id"`
	PolicyVersion int       `json:"policy_version"`
	RepositoryID  string    `json:"repository_id"`
	PullRequestID string    `json:"pull_request_id"`
	Revision      string    `json:"revision"`
	Role          string    `json:"role"`
	Decision      string    `json:"decision"`
	Rationale     string    `json:"rationale"`
	ActorID       string    `json:"actor_id"`
	CreatedAt     time.Time `json:"created_at"`
}
type Exception struct {
	ID            string    `json:"id"`
	PolicyID      string    `json:"policy_id"`
	PolicyVersion int       `json:"policy_version"`
	RepositoryID  string    `json:"repository_id"`
	PullRequestID string    `json:"pull_request_id"`
	Revision      string    `json:"revision"`
	Reason        string    `json:"reason"`
	ExpiresAt     time.Time `json:"expires_at"`
	ActorID       string    `json:"actor_id"`
	CreatedAt     time.Time `json:"created_at"`
}
type Diagnostic struct {
	Kind       string     `json:"kind"`
	Severity   string     `json:"severity"`
	Message    string     `json:"message"`
	ResourceID string     `json:"resource_id,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}
type Readiness struct {
	Ready            bool         `json:"ready"`
	Revision         string       `json:"revision"`
	Policies         []Policy     `json:"policies"`
	Acceptances      []Acceptance `json:"acceptances"`
	ActiveExceptions []Exception  `json:"active_exceptions"`
	Diagnostics      []Diagnostic `json:"diagnostics"`
	Authority        string       `json:"authority"`
}

type record struct {
	Policies    []Policy     `json:"policies"`
	Acceptances []Acceptance `json:"acceptances"`
	Exceptions  []Exception  `json:"exceptions"`
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
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	return &Store{root: root, now: func() time.Time { return time.Now().UTC() }}, nil
}
func id() string { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func (s *Store) file(scopeKind, scopeID string) string {
	return filepath.Join(s.root, scopeKind+"-"+scopeID+".json")
}
func (s *Store) read(kind, scope string) (record, error) {
	var v record
	b, e := os.ReadFile(s.file(kind, scope))
	if os.IsNotExist(e) {
		return record{Policies: []Policy{}, Acceptances: []Acceptance{}, Exceptions: []Exception{}}, nil
	}
	if e == nil {
		e = json.Unmarshal(b, &v)
	}
	return v, e
}
func (s *Store) write(kind, scope string, v record) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	tmp := s.file(kind, scope) + ".tmp"
	if e = os.WriteFile(tmp, b, 0600); e == nil {
		e = os.Rename(tmp, s.file(kind, scope))
	}
	return e
}
func validPolicy(p Policy) bool {
	if (p.ScopeKind != "repository" && p.ScopeKind != "organization") || p.ScopeID == "" || strings.TrimSpace(p.Name) == "" || len(p.Selectors) == 0 || len(p.Requirements) == 0 || p.ExceptionMaxHours < 1 || p.ExceptionMaxHours > 720 {
		return false
	}
	roles := map[string]bool{"design_owner": true, "accessibility": true, "content": true, "localization": true, "invited_user": true}
	kinds := map[string]bool{"component": true, "journey": true, "path": true, "risk_class": true}
	for _, x := range p.Selectors {
		if !kinds[x.Kind] || strings.TrimSpace(x.Value) == "" {
			return false
		}
	}
	for _, x := range p.Requirements {
		if !roles[x.Role] || len(x.ApproverIDs) == 0 {
			return false
		}
	}
	return true
}
func (s *Store) CreatePolicy(p Policy) (Policy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validPolicy(p) {
		return p, ErrInvalid
	}
	v, e := s.read(p.ScopeKind, p.ScopeID)
	if e != nil {
		return p, e
	}
	p.ID = id()
	p.Version = 1
	p.CreatedAt = s.now()
	v.Policies = append(v.Policies, p)
	return p, s.write(p.ScopeKind, p.ScopeID, v)
}
func (s *Store) Policies(kind, scope string) ([]Policy, error) {
	v, e := s.read(kind, scope)
	return v.Policies, e
}
func (s *Store) findPolicy(repo, org, pid string) (Policy, error) {
	for _, pair := range [][2]string{{"repository", repo}, {"organization", org}} {
		if pair[1] == "" {
			continue
		}
		v, e := s.read(pair[0], pair[1])
		if e != nil {
			return Policy{}, e
		}
		for _, p := range v.Policies {
			if p.ID == pid {
				return p, nil
			}
		}
	}
	return Policy{}, ErrNotFound
}
func (s *Store) Accept(repo, org string, a Acceptance) (Acceptance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, e := s.findPolicy(repo, org, a.PolicyID)
	if e != nil {
		return a, e
	}
	allowed := false
	for _, r := range p.Requirements {
		if r.Role == a.Role {
			for _, u := range r.ApproverIDs {
				allowed = allowed || u == a.ActorID
			}
		}
	}
	if !allowed || a.PolicyVersion != p.Version || len(a.Revision) != 40 || (a.Decision != "accepted" && a.Decision != "rejected") || strings.TrimSpace(a.Rationale) == "" {
		return a, ErrInvalid
	}
	v, e := s.read("repository", repo)
	if e != nil {
		return a, e
	}
	a.ID = id()
	a.RepositoryID = repo
	a.CreatedAt = s.now()
	v.Acceptances = append(v.Acceptances, a)
	return a, s.write("repository", repo, v)
}
func (s *Store) Except(repo, org string, x Exception) (Exception, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, e := s.findPolicy(repo, org, x.PolicyID)
	if e != nil {
		return x, e
	}
	if x.ActorID != p.CreatedBy || x.PolicyVersion != p.Version || len(x.Revision) != 40 || strings.TrimSpace(x.Reason) == "" || !x.ExpiresAt.After(s.now()) || x.ExpiresAt.After(s.now().Add(time.Duration(p.ExceptionMaxHours)*time.Hour)) {
		return x, ErrInvalid
	}
	v, e := s.read("repository", repo)
	if e != nil {
		return x, e
	}
	x.ID = id()
	x.RepositoryID = repo
	x.CreatedAt = s.now()
	v.Exceptions = append(v.Exceptions, x)
	return x, s.write("repository", repo, v)
}
func matches(p Policy, paths []string, components, journeys, risk []string) bool {
	for _, s := range p.Selectors {
		switch s.Kind {
		case "path":
			for _, v := range paths {
				if v == s.Value || strings.HasPrefix(v, strings.TrimSuffix(s.Value, "/")+"/") {
					return true
				}
			}
		case "component":
			for _, v := range components {
				if v == s.Value {
					return true
				}
			}
		case "journey":
			for _, v := range journeys {
				if v == s.Value {
					return true
				}
			}
		case "risk_class":
			for _, v := range risk {
				if v == s.Value {
					return true
				}
			}
		}
	}
	return false
}
func (s *Store) Evaluate(repo, org, pull, revision string, paths, components, journeys, risks []string, extra []Diagnostic) (Readiness, error) {
	policies := []Policy{}
	for _, pair := range [][2]string{{"repository", repo}, {"organization", org}} {
		if pair[1] == "" {
			continue
		}
		v, e := s.read(pair[0], pair[1])
		if e != nil {
			return Readiness{}, e
		}
		for _, p := range v.Policies {
			if matches(p, paths, components, journeys, risks) {
				policies = append(policies, p)
			}
		}
	}
	v, e := s.read("repository", repo)
	if e != nil {
		return Readiness{}, e
	}
	out := Readiness{Revision: revision, Policies: policies, Acceptances: []Acceptance{}, ActiveExceptions: []Exception{}, Diagnostics: append([]Diagnostic{}, extra...), Authority: "Design acceptance is evidence only and grants no code review, merge, release, deployment, or repository authority."}
	now := s.now()
	for _, p := range policies {
		excepted := false
		for _, x := range v.Exceptions {
			if x.PolicyID == p.ID && x.PolicyVersion == p.Version && x.PullRequestID == pull && x.Revision == revision && x.ExpiresAt.After(now) {
				out.ActiveExceptions = append(out.ActiveExceptions, x)
				excepted = true
				if x.ExpiresAt.Before(now.Add(7 * 24 * time.Hour)) {
					at := x.ExpiresAt
					out.Diagnostics = append(out.Diagnostics, Diagnostic{"expiring_exception", "warning", "A design-policy exception expires within seven days.", x.ID, &at})
				}
			}
		}
		for _, req := range p.Requirements {
			accepted := false
			for _, a := range v.Acceptances {
				if a.PolicyID == p.ID && a.PolicyVersion == p.Version && a.PullRequestID == pull && a.Revision == revision && a.Role == req.Role {
					out.Acceptances = append(out.Acceptances, a)
					accepted = a.Decision == "accepted"
				}
			}
			if !accepted && !excepted {
				out.Diagnostics = append(out.Diagnostics, Diagnostic{"missing_acceptance", "blocking", "Current " + req.Role + " acceptance is required.", p.ID, nil})
			}
		}
	}
	for _, d := range out.Diagnostics {
		if d.Severity == "blocking" {
			out.Ready = false
			return out, nil
		}
	}
	out.Ready = true
	sort.Slice(out.Policies, func(i, j int) bool { return out.Policies[i].ID < out.Policies[j].ID })
	return out, nil
}
