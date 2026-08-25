// Package provenancepolicies retains versioned acceptable-origin and distribution rules.
package provenancepolicies

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

var ErrNotFound = errors.New("provenance policy not found")
var ErrInvalid = errors.New("invalid provenance policy")
var ErrConflict = errors.New("provenance policy version conflict")

type MaterialRule struct {
	Kind                    string   `json:"kind"`
	PermittedOrigins        []string `json:"permitted_origins"`
	PermittedLicenses       []string `json:"permitted_licenses"`
	ProhibitedLicenses      []string `json:"prohibited_licenses,omitempty"`
	RequiredAttribution     []string `json:"required_attribution"`
	ContributorAttestations []string `json:"contributor_attestations"`
	ReviewOwnerIDs          []string `json:"review_owner_ids"`
	DistributionContexts    []string `json:"distribution_contexts"`
}

type Link struct {
	Kind         string `json:"kind"`
	ResourceID   string `json:"resource_id"`
	RepositoryID string `json:"repository_id,omitempty"`
	Boundary     string `json:"boundary"`
}

type Exception struct {
	ID            string    `json:"id"`
	MaterialKinds []string  `json:"material_kinds"`
	License       string    `json:"license,omitempty"`
	Origin        string    `json:"origin,omitempty"`
	Contexts      []string  `json:"contexts"`
	Rationale     string    `json:"rationale"`
	OwnerID       string    `json:"owner_id"`
	ExpiresAt     time.Time `json:"expires_at"`
	FollowUp      string    `json:"follow_up"`
}

type Revision struct {
	Version    int            `json:"version"`
	Title      string         `json:"title"`
	Summary    string         `json:"summary"`
	OwnerIDs   []string       `json:"owner_ids"`
	Rules      []MaterialRule `json:"rules"`
	Links      []Link         `json:"links"`
	Exceptions []Exception    `json:"exceptions"`
	CreatedBy  string         `json:"created_by"`
	CreatedAt  time.Time      `json:"created_at"`
}

type Diagnostic struct {
	Kind         string `json:"kind"`
	Severity     string `json:"severity"`
	Message      string `json:"message"`
	MaterialKind string `json:"material_kind,omitempty"`
	ExceptionID  string `json:"exception_id,omitempty"`
	AttributedTo string `json:"attributed_to"`
}

type Policy struct {
	ID             string       `json:"id"`
	ScopeKind      string       `json:"scope_kind"`
	ScopeID        string       `json:"scope_id"`
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

func (s *Store) Create(kind, scope, actor string, r Revision) (Policy, error) {
	var out Policy
	err := s.lock(func() error {
		if !valid(kind, scope, r) {
			return ErrInvalid
		}
		now := s.now()
		stamp(&r, 1, actor, now)
		out = Policy{ID: id(), ScopeKind: kind, ScopeID: scope, CurrentVersion: 1, Revisions: []Revision{r}, CreatedAt: now, UpdatedAt: now}
		return s.write(out)
	})
	return s.project(out), err
}
func (s *Store) Revise(id string, expected int, actor string, r Revision) (Policy, error) {
	var out Policy
	err := s.lock(func() error {
		p, e := s.read(id)
		if e != nil {
			return e
		}
		if p.CurrentVersion != expected {
			return ErrConflict
		}
		if !valid(p.ScopeKind, p.ScopeID, r) {
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
func (s *Store) Get(id string) (Policy, error) {
	var out Policy
	err := s.lock(func() error { var e error; out, e = s.read(id); return e })
	return s.project(out), err
}

// WithCurrent holds the policy mutation boundary while a caller validates and
// commits a decision against the supplied authoritative policy snapshot.
func (s *Store) WithCurrent(id string, fn func(Policy) error) error {
	if fn == nil {
		return ErrInvalid
	}
	return s.lock(func() error {
		p, err := s.read(id)
		if err != nil {
			return err
		}
		return fn(s.project(p))
	})
}
func (s *Store) List(kind, scope string) ([]Policy, error) {
	out := []Policy{}
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
			if p.ScopeKind == kind && p.ScopeID == scope {
				out = append(out, s.project(p))
			}
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, err
}

func valid(kind, scope string, r Revision) bool {
	if !one(kind, "repository", "organization") || scope == "" || r.Title == "" || r.Summary == "" || len(r.Rules) == 0 {
		return false
	}
	seen := map[string]bool{}
	for _, x := range r.Rules {
		if !one(x.Kind, "source", "generated_code", "asset", "model", "dataset", "package", "build_input") || seen[x.Kind] || len(x.PermittedOrigins) == 0 || len(x.PermittedLicenses) == 0 || len(x.DistributionContexts) == 0 {
			return false
		}
		seen[x.Kind] = true
	}
	for _, l := range r.Links {
		if !one(l.Kind, "contributor_pathway", "agent_contract", "package", "release", "contribution_boundary") || l.ResourceID == "" || !one(l.Boundary, "public", "private", "federated") {
			return false
		}
	}
	ids := map[string]bool{}
	for _, x := range r.Exceptions {
		if x.ID == "" || ids[x.ID] || len(x.MaterialKinds) == 0 || len(x.Contexts) == 0 || x.Rationale == "" || x.OwnerID == "" || x.ExpiresAt.IsZero() || x.FollowUp == "" || (x.License == "" && x.Origin == "") {
			return false
		}
		ids[x.ID] = true
		for _, k := range x.MaterialKinds {
			if !seen[k] {
				return false
			}
		}
	}
	return true
}
func stamp(r *Revision, v int, actor string, now time.Time) {
	r.Version = v
	r.CreatedBy = actor
	r.CreatedAt = now
}
func (s *Store) project(p Policy) Policy {
	p.Diagnostics = []Diagnostic{}
	if len(p.Revisions) == 0 {
		return p
	}
	r := p.Revisions[len(p.Revisions)-1]
	attr := r.CreatedBy
	if len(r.OwnerIDs) == 0 {
		p.Diagnostics = append(p.Diagnostics, diag("missing_owner", "blocking", "The provenance policy has no accountable owner.", "", "", attr))
	}
	for _, x := range r.Rules {
		licenses := map[string]bool{}
		for _, v := range x.PermittedLicenses {
			licenses[strings.ToLower(strings.TrimSpace(v))] = true
			if strings.EqualFold(strings.TrimSpace(v), "unknown") {
				p.Diagnostics = append(p.Diagnostics, diag("unknown_license", "blocking", "Unknown licenses require an attributable decision before acceptance.", x.Kind, "", attr))
			}
		}
		for _, v := range x.ProhibitedLicenses {
			if licenses[strings.ToLower(strings.TrimSpace(v))] {
				p.Diagnostics = append(p.Diagnostics, diag("conflicting_terms", "blocking", "A license is both permitted and prohibited.", x.Kind, "", attr))
			}
		}
		if len(x.ReviewOwnerIDs) == 0 {
			p.Diagnostics = append(p.Diagnostics, diag("missing_owner", "blocking", "The material rule has no review owner.", x.Kind, "", attr))
		}
	}
	for _, x := range r.Exceptions {
		if x.ExpiresAt.Before(s.now().Add(30 * 24 * time.Hour)) {
			p.Diagnostics = append(p.Diagnostics, diag("expiring_exception", "warning", "The exception expires within 30 days or has expired.", "", x.ID, x.OwnerID))
		}
	}
	return p
}
func diag(k, severity, message, material, exception, actor string) Diagnostic {
	return Diagnostic{Kind: k, Severity: severity, Message: message, MaterialKind: material, ExceptionID: exception, AttributedTo: actor}
}
func one(v string, values ...string) bool {
	for _, x := range values {
		if v == x {
			return true
		}
	}
	return false
}
func id() string                       { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func (s *Store) path(id string) string { return filepath.Join(s.root, id+".json") }
func (s *Store) read(id string) (Policy, error) {
	var p Policy
	b, e := os.ReadFile(s.path(id))
	if os.IsNotExist(e) {
		return p, ErrNotFound
	}
	if e != nil {
		return p, e
	}
	if json.Unmarshal(b, &p) != nil {
		return p, ErrInvalid
	}
	return p, nil
}
func (s *Store) write(p Policy) error {
	b, e := json.MarshalIndent(p, "", "  ")
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(s.root, ".policy-")
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
	if e == nil {
		e = os.Rename(name, s.path(p.ID))
	}
	if e == nil {
		var d *os.File
		d, e = os.Open(s.root)
		if e == nil {
			e = d.Sync()
			_ = d.Close()
		}
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
