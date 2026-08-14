// Package accessibilitydelivery retains accessibility delivery policies and revision-exact human outcomes.
package accessibilitydelivery

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

	"github.com/greptile-projects/vivarium-tuatara/apps/api/accessibilityassessments"
)

var ErrInvalid = errors.New("invalid accessibility delivery record")
var ErrNotFound = errors.New("accessibility delivery record not found")
var ErrConflict = errors.New("accessibility delivery record conflict")

type RoleRequirement struct {
	Role    string   `json:"role"`
	UserIDs []string `json:"user_ids"`
	Minimum int      `json:"minimum"`
}
type Policy struct {
	ID                string            `json:"id"`
	RepositoryID      string            `json:"repository_id"`
	Branch            string            `json:"branch"`
	Paths             []string          `json:"paths"`
	Journeys          []string          `json:"journeys"`
	RiskClasses       []string          `json:"risk_classes"`
	RequiredChecks    []string          `json:"required_checks"`
	RequiredScenarios []string          `json:"required_scenarios"`
	RequiredRoles     []RoleRequirement `json:"required_roles"`
	CreatedBy         string            `json:"created_by"`
	CreatedAt         time.Time         `json:"created_at"`
}
type Invitation struct {
	ID            string    `json:"id"`
	RepositoryID  string    `json:"repository_id"`
	PolicyID      string    `json:"policy_id"`
	PullRequestID string    `json:"pull_request_id,omitempty"`
	ReleaseID     string    `json:"release_id,omitempty"`
	Revision      string    `json:"revision"`
	PreviewID     string    `json:"preview_id"`
	UserID        string    `json:"user_id"`
	Role          string    `json:"role"`
	AccessNeeds   []string  `json:"access_needs"`
	ExpiresAt     time.Time `json:"expires_at"`
	CreatedBy     string    `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
	Outcome       *Outcome  `json:"outcome,omitempty"`
}
type Outcome struct {
	Decision  string    `json:"decision"`
	Rationale string    `json:"rationale"`
	ActorID   string    `json:"actor_id"`
	Revision  string    `json:"revision"`
	CreatedAt time.Time `json:"created_at"`
}
type Override struct {
	ID           string    `json:"id"`
	PolicyID     string    `json:"policy_id"`
	Revision     string    `json:"revision"`
	Rationale    string    `json:"rationale"`
	FollowUpWork string    `json:"follow_up_work"`
	CreatedBy    string    `json:"created_by"`
	ExpiresAt    time.Time `json:"expires_at"`
	CreatedAt    time.Time `json:"created_at"`
}
type Requirement struct {
	PolicyID string `json:"policy_id"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	Message  string `json:"message"`
}
type Readiness struct {
	Ready            bool          `json:"ready"`
	Revision         string        `json:"revision"`
	Requirements     []Requirement `json:"requirements"`
	ActiveExceptions []Override    `json:"active_exceptions"`
	Dissent          []Outcome     `json:"dissent"`
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

func (s *Store) CreatePolicy(repo, actor string, p Policy) (Policy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return p, err
	}
	defer unlock()
	p.ID = newID()
	p.RepositoryID = repo
	p.CreatedBy = actor
	p.CreatedAt = s.now()
	normalizePolicy(&p)
	if !validPolicy(p) {
		return p, ErrInvalid
	}
	err = s.write(repo, "policies", p.ID, p)
	return p, err
}
func (s *Store) Policies(repo string) ([]Policy, error) {
	var out []Policy
	err := s.list(repo, "policies", &out)
	return out, err
}
func (s *Store) Invite(repo, actor string, v Invitation) (Invitation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return v, err
	}
	defer unlock()
	policies, _ := s.policies(repo)
	found := false
	for _, p := range policies {
		if p.ID == v.PolicyID {
			for _, rr := range p.RequiredRoles {
				if rr.Role == v.Role && contains(rr.UserIDs, v.UserID) {
					found = true
				}
			}
		}
	}
	v.ID = newID()
	v.RepositoryID = repo
	v.CreatedBy = actor
	v.CreatedAt = s.now()
	v.Outcome = nil
	if !found || (v.PullRequestID == "") == (v.ReleaseID == "") || !validID(v.UserID) || !validCommit(v.Revision) || strings.TrimSpace(v.PreviewID) == "" || !v.ExpiresAt.After(v.CreatedAt) || v.ExpiresAt.After(v.CreatedAt.Add(30*24*time.Hour)) {
		return v, ErrInvalid
	}
	err = s.write(repo, "invitations", v.ID, v)
	return v, err
}
func (s *Store) Respond(repo, id, actor, decision, rationale, revision string) (Invitation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Invitation{}, err
	}
	defer unlock()
	var invitations []Invitation
	if err = s.listUnlocked(repo, "invitations", &invitations); err != nil {
		return Invitation{}, err
	}
	for _, v := range invitations {
		if v.ID != id {
			continue
		}
		if v.UserID != actor || v.Revision != revision || !v.ExpiresAt.After(s.now()) || v.Outcome != nil || (decision != "confirmed" && decision != "rejected") || strings.TrimSpace(rationale) == "" {
			return v, ErrConflict
		}
		v.Outcome = &Outcome{Decision: decision, Rationale: strings.TrimSpace(rationale), ActorID: actor, Revision: revision, CreatedAt: s.now()}
		return v, s.write(repo, "invitations", v.ID, v)
	}
	return Invitation{}, ErrNotFound
}
func (s *Store) Override(repo, actor string, v Override) (Override, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, e := s.lock()
	if e != nil {
		return v, e
	}
	defer u()
	v.ID = newID()
	v.CreatedBy = actor
	v.CreatedAt = s.now()
	v.Rationale = strings.TrimSpace(v.Rationale)
	v.FollowUpWork = strings.TrimSpace(v.FollowUpWork)
	policies, _ := s.policies(repo)
	policyExists := false
	for _, policy := range policies {
		if policy.ID == v.PolicyID {
			policyExists = true
		}
	}
	if !policyExists || !validCommit(v.Revision) || v.Rationale == "" || v.FollowUpWork == "" || !v.ExpiresAt.After(v.CreatedAt) || v.ExpiresAt.After(v.CreatedAt.Add(90*24*time.Hour)) {
		return v, ErrInvalid
	}
	e = s.write(repo, "overrides", v.ID, v)
	return v, e
}

func (s *Store) Evaluate(repo, revision, branch string, paths, journeys, risks []string, checkStatus map[string]string, assessments []accessibilityassessments.Assessment) (Readiness, error) {
	policies, err := s.Policies(repo)
	if err != nil {
		return Readiness{}, err
	}
	var invitations []Invitation
	if err = s.list(repo, "invitations", &invitations); err != nil {
		return Readiness{}, err
	}
	var overrides []Override
	if err = s.list(repo, "overrides", &overrides); err != nil {
		return Readiness{}, err
	}
	r := Readiness{Ready: true, Revision: revision, Requirements: []Requirement{}, ActiveExceptions: []Override{}, Dissent: []Outcome{}}
	now := s.now()
	for _, p := range policies {
		if p.Branch != branch || !selected(p.Paths, paths) || !selected(p.Journeys, journeys) || !selected(p.RiskClasses, risks) {
			continue
		}
		except := false
		for _, o := range overrides {
			if o.PolicyID == p.ID && o.Revision == revision && o.ExpiresAt.After(now) {
				r.ActiveExceptions = append(r.ActiveExceptions, o)
				except = true
			}
		}
		add := func(kind, name, status, msg string) {
			r.Requirements = append(r.Requirements, Requirement{p.ID, kind, name, status, msg})
			if status != "passed" && !except {
				r.Ready = false
			}
		}
		for _, name := range p.RequiredChecks {
			status := checkStatus[name]
			if status == "" {
				status = "missing"
			}
			add("automated_check", name, status, "automated evidence must pass at the exact candidate revision")
		}
		for _, scenario := range p.RequiredScenarios {
			status := "missing"
			for _, a := range assessments {
				for _, c := range a.Checks {
					if c.JourneyID == scenario && c.InvalidatedAt == nil {
						if a.Revision != revision && status == "missing" {
							status = "stale"
						} else if a.Revision == revision && c.Outcome == "passed" {
							status = "passed"
						} else if a.Revision == revision && c.Outcome == "failed" {
							status = "failed"
						} else if a.Revision == revision {
							status = "unevaluated"
						}
					}
				}
			}
			add("scenario", scenario, status, "declared scenario requires current coverage")
		}
		for _, rr := range p.RequiredRoles {
			confirmed := map[string]bool{}
			for _, v := range invitations {
				if v.PolicyID != p.ID || v.Revision != revision || v.Role != rr.Role || !v.ExpiresAt.After(now) || v.Outcome == nil {
					continue
				}
				if v.Outcome.Decision == "confirmed" {
					confirmed[v.UserID] = true
				} else {
					r.Dissent = append(r.Dissent, *v.Outcome)
				}
			}
			status := "missing"
			if len(confirmed) >= rr.Minimum {
				status = "passed"
			}
			add("acknowledgement", rr.Role, status, "independent invited acknowledgement is required")
		}
		for _, a := range assessments {
			if a.Revision != revision {
				continue
			}
			for _, f := range a.Findings {
				if f.InvalidatedAt == nil && f.Decision != nil && f.Decision.Classification == "accepted" {
					add("barrier", f.ID, "failed", "accepted accessibility barrier remains unresolved")
				}
			}
		}
	}
	return r, nil
}

func selected(filter, values []string) bool {
	if len(filter) == 0 {
		return true
	}
	for _, f := range filter {
		for _, v := range values {
			if f == v || (strings.HasSuffix(f, "*") && strings.HasPrefix(v, strings.TrimSuffix(f, "*"))) {
				return true
			}
		}
	}
	return false
}
func normalizePolicy(p *Policy) {
	sort.Strings(p.Paths)
	sort.Strings(p.Journeys)
	sort.Strings(p.RiskClasses)
	sort.Strings(p.RequiredChecks)
	sort.Strings(p.RequiredScenarios)
	for i := range p.RequiredRoles {
		sort.Strings(p.RequiredRoles[i].UserIDs)
	}
}
func validPolicy(p Policy) bool {
	if !validID(p.RepositoryID) || p.Branch == "" || len(p.Paths) > 100 || len(p.Journeys) > 100 || len(p.RiskClasses) > 50 || len(p.RequiredChecks) > 50 || len(p.RequiredScenarios) > 100 || len(p.RequiredRoles) > 20 || len(p.RequiredChecks)+len(p.RequiredScenarios)+len(p.RequiredRoles) == 0 {
		return false
	}
	for _, r := range p.RequiredRoles {
		if r.Role != "reviewer" && r.Role != "participant" && r.Role != "accessibility_reviewer" {
			return false
		}
		if r.Minimum < 1 || r.Minimum > len(r.UserIDs) {
			return false
		}
		for _, id := range r.UserIDs {
			if !validID(id) {
				return false
			}
		}
	}
	return true
}
func contains(v []string, x string) bool {
	for _, s := range v {
		if s == x {
			return true
		}
	}
	return false
}
func validID(v string) bool {
	if len(v) != 32 {
		return false
	}
	_, e := hex.DecodeString(v)
	return e == nil && v == strings.ToLower(v)
}
func validCommit(v string) bool {
	if len(v) != 40 {
		return false
	}
	_, e := hex.DecodeString(v)
	return e == nil && v == strings.ToLower(v)
}
func newID() string { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func (s *Store) policies(repo string) ([]Policy, error) {
	var v []Policy
	e := s.listUnlocked(repo, "policies", &v)
	return v, e
}
func (s *Store) list(repo, kind string, out any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, e := s.lock()
	if e != nil {
		return e
	}
	defer u()
	return s.listUnlocked(repo, kind, out)
}
func (s *Store) listUnlocked(repo, kind string, out any) error {
	entries, e := os.ReadDir(filepath.Join(s.root, repo, kind))
	if errors.Is(e, os.ErrNotExist) {
		return nil
	}
	if e != nil {
		return e
	}
	raw := []json.RawMessage{}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		b, e := os.ReadFile(filepath.Join(s.root, repo, kind, entry.Name()))
		if e != nil {
			return e
		}
		raw = append(raw, b)
	}
	b, _ := json.Marshal(raw)
	return json.Unmarshal(b, out)
}
func (s *Store) write(repo, kind, id string, v any) error {
	dir := filepath.Join(s.root, repo, kind)
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(dir, ".record-")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	_ = f.Chmod(0600)
	if _, e = f.Write(b); e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	return os.Rename(name, filepath.Join(dir, id+".json"))
}
func (s *Store) lock() (func(), error) {
	f, e := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); e != nil {
		f.Close()
		return nil, e
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}
