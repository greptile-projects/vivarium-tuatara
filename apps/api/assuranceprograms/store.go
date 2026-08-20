// Package assuranceprograms retains versioned, repository-scoped obligation and control maps.
package assuranceprograms

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

var ErrNotFound = errors.New("assurance program not found")
var ErrInvalid = errors.New("invalid assurance program")
var ErrConflict = errors.New("assurance program version conflict")

type Requirement struct {
	ID             string   `json:"id"`
	Kind           string   `json:"kind"`
	Authority      string   `json:"authority"`
	Citation       string   `json:"citation"`
	Title          string   `json:"title"`
	Summary        string   `json:"summary"`
	Applicability  string   `json:"applicability"`
	InheritedFrom  string   `json:"inherited_from,omitempty"`
	OwnerIDs       []string `json:"owner_ids"`
	Interpretation string   `json:"interpretation"`
	ConflictsWith  []string `json:"conflicts_with,omitempty"`
}
type Scope struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	ResourceID  string `json:"resource_id"`
	Revision    string `json:"revision,omitempty"`
	Path        string `json:"path,omitempty"`
	Description string `json:"description"`
}
type Mapping struct {
	ScopeID string `json:"scope_id"`
	Purpose string `json:"purpose"`
}
type EvidenceCriterion struct {
	ID           string `json:"id"`
	Description  string `json:"description"`
	Kind         string `json:"kind"`
	ResourceKind string `json:"resource_kind,omitempty"`
	ResourceID   string `json:"resource_id,omitempty"`
	Revision     string `json:"revision,omitempty"`
}
type Control struct {
	ID               string              `json:"id"`
	Title            string              `json:"title"`
	Objective        string              `json:"objective"`
	RequirementIDs   []string            `json:"requirement_ids"`
	OwnerIDs         []string            `json:"owner_ids"`
	ReviewPeriodDays int                 `json:"review_period_days"`
	Mappings         []Mapping           `json:"mappings"`
	EvidenceCriteria []EvidenceCriterion `json:"evidence_criteria"`
	Claim            string              `json:"claim"`
}
type Exception struct {
	ID             string    `json:"id"`
	RequirementIDs []string  `json:"requirement_ids"`
	ControlIDs     []string  `json:"control_ids"`
	Rationale      string    `json:"rationale"`
	GrantedBy      string    `json:"granted_by"`
	ExpiresAt      time.Time `json:"expires_at"`
	FollowUp       string    `json:"follow_up"`
}
type Revision struct {
	Version          int           `json:"version"`
	Title            string        `json:"title"`
	Summary          string        `json:"summary"`
	OwnerIDs         []string      `json:"owner_ids"`
	ReviewPeriodDays int           `json:"review_period_days"`
	Requirements     []Requirement `json:"requirements"`
	Scopes           []Scope       `json:"scopes"`
	Controls         []Control     `json:"controls"`
	Exceptions       []Exception   `json:"exceptions"`
	CreatedBy        string        `json:"created_by"`
	CreatedAt        time.Time     `json:"created_at"`
}
type Diagnostic struct {
	Kind          string `json:"kind"`
	Severity      string `json:"severity"`
	Message       string `json:"message"`
	RequirementID string `json:"requirement_id,omitempty"`
	ControlID     string `json:"control_id,omitempty"`
	AttributedTo  string `json:"attributed_to"`
}
type Program struct {
	ID             string       `json:"id"`
	RepositoryID   string       `json:"repository_id"`
	CurrentVersion int          `json:"current_version"`
	Revisions      []Revision   `json:"revisions"`
	Diagnostics    []Diagnostic `json:"diagnostics"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
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
func (s *Store) Create(repo, actor string, r Revision) (Program, error) {
	var out Program
	err := s.lock(func() error {
		if !valid(repo, r) {
			return ErrInvalid
		}
		now := s.now()
		stamp(&r, 1, actor, now)
		out = Program{ID: id(), RepositoryID: repo, CurrentVersion: 1, Revisions: []Revision{r}, CreatedAt: now, UpdatedAt: now}
		return s.write(out)
	})
	return s.project(out), err
}
func (s *Store) Revise(pid string, expected int, actor string, r Revision) (Program, error) {
	var out Program
	err := s.lock(func() error {
		p, e := s.read(pid)
		if e != nil {
			return e
		}
		if p.CurrentVersion != expected {
			return ErrConflict
		}
		if !valid(p.RepositoryID, r) {
			return ErrInvalid
		}
		stamp(&r, expected+1, actor, s.now())
		p.CurrentVersion = r.Version
		p.Revisions = append(p.Revisions, r)
		p.UpdatedAt = r.CreatedAt
		out = p
		return s.write(p)
	})
	return s.project(out), err
}
func (s *Store) Get(pid string) (Program, error) {
	var out Program
	err := s.lock(func() error { var e error; out, e = s.read(pid); return e })
	return s.project(out), err
}
func (s *Store) List(repo string) ([]Program, error) {
	out := []Program{}
	err := s.lock(func() error {
		es, e := os.ReadDir(s.root)
		if e != nil {
			return e
		}
		for _, x := range es {
			if x.IsDir() || !strings.HasSuffix(x.Name(), ".json") {
				continue
			}
			p, e := s.read(strings.TrimSuffix(x.Name(), ".json"))
			if e != nil {
				return e
			}
			if p.RepositoryID == repo {
				out = append(out, s.project(p))
			}
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, err
}
func stamp(r *Revision, v int, actor string, now time.Time) {
	r.Version = v
	r.CreatedBy = actor
	r.CreatedAt = now
}

func valid(repo string, r Revision) bool {
	if strings.TrimSpace(r.Title) == "" || strings.TrimSpace(r.Summary) == "" || r.ReviewPeriodDays < 1 || len(r.Requirements) == 0 || len(r.Scopes) == 0 || len(r.Controls) == 0 {
		return false
	}
	reqs, scopes, controls := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, q := range r.Requirements {
		if q.ID == "" || reqs[q.ID] || !one(q.Kind, "regulatory", "contractual", "organization") || q.Authority == "" || q.Citation == "" || q.Title == "" || q.Summary == "" || q.Applicability == "" || q.Interpretation == "" {
			return false
		}
		reqs[q.ID] = true
	}
	for _, x := range r.Scopes {
		if x.ID == "" || scopes[x.ID] || !one(x.Kind, "repository", "policy", "data_flow", "infrastructure", "environment", "release", "procedure") || x.ResourceID == "" || x.Description == "" || (x.Revision != "" && !revision(x.Revision)) {
			return false
		}
		if x.Kind == "repository" && x.ResourceID != repo {
			return false
		}
		scopes[x.ID] = true
	}
	for _, c := range r.Controls {
		if c.ID == "" || controls[c.ID] || c.Title == "" || c.Objective == "" || c.ReviewPeriodDays < 1 || len(c.RequirementIDs) == 0 || len(c.Mappings) == 0 || len(c.EvidenceCriteria) == 0 {
			return false
		}
		controls[c.ID] = true
		for _, q := range c.RequirementIDs {
			if !reqs[q] {
				return false
			}
		}
		for _, m := range c.Mappings {
			if !scopes[m.ScopeID] || m.Purpose == "" {
				return false
			}
		}
		for _, e := range c.EvidenceCriteria {
			if e.ID == "" || e.Description == "" || !one(e.Kind, "automated", "manual", "attestation", "record") {
				return false
			}
		}
	}
	for _, q := range r.Requirements {
		for _, c := range q.ConflictsWith {
			if !reqs[c] || c == q.ID {
				return false
			}
		}
	}
	for _, x := range r.Exceptions {
		if x.ID == "" || x.Rationale == "" || x.GrantedBy == "" || x.FollowUp == "" || x.ExpiresAt.IsZero() || len(x.RequirementIDs)+len(x.ControlIDs) == 0 {
			return false
		}
		for _, q := range x.RequirementIDs {
			if !reqs[q] {
				return false
			}
		}
		for _, c := range x.ControlIDs {
			if !controls[c] {
				return false
			}
		}
	}
	return true
}
func (s *Store) project(p Program) Program {
	if len(p.Revisions) == 0 {
		return p
	}
	r := p.Revisions[len(p.Revisions)-1]
	d := []Diagnostic{}
	attr := r.CreatedBy
	if len(r.OwnerIDs) == 0 {
		d = append(d, diag("missing_owner", "blocking", "The assurance program has no accountable owner.", "", "", attr))
	}
	mapped := map[string]bool{}
	for _, c := range r.Controls {
		if len(c.OwnerIDs) == 0 {
			d = append(d, diag("missing_owner", "blocking", "The control has no accountable owner.", "", c.ID, attr))
		}
		if strings.TrimSpace(c.Claim) == "" {
			d = append(d, diag("unsupported_claim", "blocking", "The control does not state how the project claims to satisfy its objective.", "", c.ID, attr))
		}
		for _, q := range c.RequirementIDs {
			mapped[q] = true
		}
	}
	for _, q := range r.Requirements {
		if len(q.OwnerIDs) == 0 {
			d = append(d, diag("missing_owner", "blocking", "The obligation has no accountable owner.", q.ID, "", attr))
		}
		if q.InheritedFrom != "" {
			d = append(d, diag("inherited_obligation", "info", "This obligation is inherited from "+q.InheritedFrom+".", q.ID, "", attr))
		}
		if len(q.ConflictsWith) > 0 {
			d = append(d, diag("conflicting_interpretation", "blocking", "The retained interpretation conflicts with another selected obligation.", q.ID, "", attr))
		}
		if !mapped[q.ID] {
			d = append(d, diag("unsupported_claim", "blocking", "No control maps this obligation to project work.", q.ID, "", attr))
		}
	}
	now := s.now()
	for _, x := range r.Exceptions {
		if !x.ExpiresAt.After(now) {
			d = append(d, diag("expired_exception", "blocking", "The exception has expired.", first(x.RequirementIDs), first(x.ControlIDs), x.GrantedBy))
		} else if !x.ExpiresAt.After(now.Add(7 * 24 * time.Hour)) {
			d = append(d, diag("expiring_exception", "warning", "The exception expires within seven days.", first(x.RequirementIDs), first(x.ControlIDs), x.GrantedBy))
		}
	}
	p.Diagnostics = d
	return p
}
func diag(k, s, m, q, c, a string) Diagnostic { return Diagnostic{k, s, m, q, c, a} }
func first(v []string) string {
	if len(v) > 0 {
		return v[0]
	}
	return ""
}
func one(v string, x ...string) bool {
	for _, y := range x {
		if v == y {
			return true
		}
	}
	return false
}
func revision(v string) bool {
	if len(v) != 40 {
		return false
	}
	_, e := hex.DecodeString(v)
	return e == nil
}
func id() string                      { b := make([]byte, 12); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func (s *Store) path(v string) string { return filepath.Join(s.root, v+".json") }
func (s *Store) read(v string) (Program, error) {
	if strings.ContainsAny(v, "/\\") {
		return Program{}, ErrNotFound
	}
	b, e := os.ReadFile(s.path(v))
	if os.IsNotExist(e) {
		return Program{}, ErrNotFound
	}
	if e != nil {
		return Program{}, e
	}
	var p Program
	if json.Unmarshal(b, &p) != nil || p.ID != v {
		return Program{}, ErrInvalid
	}
	return p, nil
}
func (s *Store) write(p Program) error {
	b, e := json.MarshalIndent(p, "", "  ")
	if e != nil {
		return e
	}
	tmp := s.path(p.ID) + ".tmp-" + id()
	if e = os.WriteFile(tmp, b, 0600); e != nil {
		return e
	}
	if e = os.Rename(tmp, s.path(p.ID)); e != nil {
		_ = os.Remove(tmp)
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
