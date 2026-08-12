// Package charters retains immutable project governance revisions and attributed decisions.
package charters

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

var (
	ErrNotFound = errors.New("charter not found")
	ErrInvalid  = errors.New("invalid charter")
	ErrConflict = errors.New("charter version changed")
)

type Role struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Eligibility []string `json:"eligibility"`
}
type DecisionClass struct {
	Name               string   `json:"name"`
	Description        string   `json:"description"`
	EligibleRoles      []string `json:"eligible_roles"`
	Participation      int      `json:"participation"`
	Quorum             int      `json:"quorum"`
	Approval           string   `json:"approval"`
	ProtectedResources []string `json:"protected_resources"`
}
type Procedures struct {
	Terms      string `json:"terms"`
	Removal    string `json:"removal"`
	Succession string `json:"succession"`
	Amendments string `json:"amendments"`
}
type Revision struct {
	ID              string          `json:"id"`
	ScopeType       string          `json:"scope_type"`
	ScopeID         string          `json:"scope_id"`
	Version         int             `json:"version"`
	Status          string          `json:"status"`
	Title           string          `json:"title"`
	Summary         string          `json:"summary"`
	Roles           []Role          `json:"roles"`
	DecisionClasses []DecisionClass `json:"decision_classes"`
	Procedures      Procedures      `json:"procedures"`
	CreatedBy       string          `json:"created_by"`
	CreatedAt       time.Time       `json:"created_at"`
	ActivatedBy     string          `json:"activated_by,omitempty"`
	ActivatedAt     *time.Time      `json:"activated_at,omitempty"`
}
type Approval struct {
	ID        string    `json:"id"`
	Version   int       `json:"version"`
	ActorID   string    `json:"actor_id"`
	Decision  string    `json:"decision"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}
type Exception struct {
	ID            string    `json:"id"`
	Version       int       `json:"version"`
	DecisionClass string    `json:"decision_class"`
	Resource      string    `json:"resource"`
	Reason        string    `json:"reason"`
	ExpiresAt     time.Time `json:"expires_at"`
	CreatedBy     string    `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
}
type Record struct {
	ScopeType     string      `json:"scope_type"`
	ScopeID       string      `json:"scope_id"`
	ActiveVersion int         `json:"active_version"`
	Revisions     []Revision  `json:"revisions"`
	Approvals     []Approval  `json:"approvals"`
	Exceptions    []Exception `json:"exceptions"`
}
type Store struct {
	root string
	mu   sync.Mutex
	now  func() time.Time
}

func New(root string) (*Store, error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	return &Store{root: root, now: time.Now}, nil
}
func (s *Store) Get(kind, id string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read(kind, id)
}
func (s *Store) Publish(kind, id, actor string, expected int, in Revision) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.readOrEmpty(kind, id)
	if err != nil {
		return r, err
	}
	if len(r.Revisions) != expected {
		return r, ErrConflict
	}
	in.ScopeType, in.ScopeID, in.Version, in.Status, in.CreatedBy, in.CreatedAt = kind, id, len(r.Revisions)+1, "draft", actor, s.now().UTC().Truncate(time.Microsecond)
	in.ID = randomID()
	if !valid(in) {
		return r, ErrInvalid
	}
	r.Revisions = append(r.Revisions, in)
	return r, s.write(r)
}
func (s *Store) Approve(kind, id, actor string, version int, decision, reason string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.read(kind, id)
	if err != nil {
		return r, err
	}
	if version < 1 || version > len(r.Revisions) || (decision != "approved" && decision != "rejected") {
		return r, ErrInvalid
	}
	for _, a := range r.Approvals {
		if a.Version == version && a.ActorID == actor {
			return r, ErrConflict
		}
	}
	r.Approvals = append(r.Approvals, Approval{ID: randomID(), Version: version, ActorID: actor, Decision: decision, Reason: strings.TrimSpace(reason), CreatedAt: s.now().UTC()})
	return r, s.write(r)
}
func (s *Store) Activate(kind, id, actor string, version int) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.read(kind, id)
	if err != nil {
		return r, err
	}
	if version < 1 || version > len(r.Revisions) || r.Revisions[version-1].Status != "draft" {
		return r, ErrConflict
	}
	approved := false
	for _, a := range r.Approvals {
		if a.Version == version && a.Decision == "approved" {
			approved = true
		}
	}
	if !approved {
		return r, ErrInvalid
	}
	now := s.now().UTC()
	r.ActiveVersion = version
	r.Revisions[version-1].Status = "active"
	r.Revisions[version-1].ActivatedBy = actor
	r.Revisions[version-1].ActivatedAt = &now
	return r, s.write(r)
}
func (s *Store) Except(kind, id, actor string, version int, class, resource, reason string, expires time.Time) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.read(kind, id)
	if err != nil {
		return r, err
	}
	if version < 1 || version > len(r.Revisions) || strings.TrimSpace(class) == "" || strings.TrimSpace(resource) == "" || strings.TrimSpace(reason) == "" || !expires.After(s.now()) {
		return r, ErrInvalid
	}
	r.Exceptions = append(r.Exceptions, Exception{ID: randomID(), Version: version, DecisionClass: class, Resource: resource, Reason: reason, ExpiresAt: expires.UTC(), CreatedBy: actor, CreatedAt: s.now().UTC()})
	return r, s.write(r)
}
func valid(v Revision) bool {
	if (v.ScopeType != "repository" && v.ScopeType != "organization") || strings.TrimSpace(v.Title) == "" || strings.TrimSpace(v.Summary) == "" || len(v.Roles) == 0 || len(v.DecisionClasses) == 0 || v.Procedures.Terms == "" || v.Procedures.Removal == "" || v.Procedures.Succession == "" || v.Procedures.Amendments == "" {
		return false
	}
	names := map[string]bool{}
	for _, x := range v.Roles {
		if x.Name == "" || x.Description == "" || len(x.Eligibility) == 0 || names[x.Name] {
			return false
		}
		names[x.Name] = true
	}
	for _, d := range v.DecisionClasses {
		if d.Name == "" || d.Description == "" || len(d.EligibleRoles) == 0 || d.Participation < 1 || d.Quorum < 1 || d.Quorum > d.Participation || len(d.ProtectedResources) == 0 || (d.Approval != "majority" && d.Approval != "consensus" && d.Approval != "supermajority") {
			return false
		}
		for _, n := range d.EligibleRoles {
			if !names[n] {
				return false
			}
		}
	}
	return true
}
func (s *Store) path(k, id string) string { return filepath.Join(s.root, k+"-"+id+".json") }
func (s *Store) readOrEmpty(k, id string) (Record, error) {
	r, e := s.read(k, id)
	if errors.Is(e, ErrNotFound) {
		return Record{ScopeType: k, ScopeID: id, Revisions: []Revision{}, Approvals: []Approval{}, Exceptions: []Exception{}}, nil
	}
	return r, e
}
func (s *Store) read(k, id string) (Record, error) {
	if !safe(k) || !safe(id) {
		return Record{}, ErrNotFound
	}
	b, e := os.ReadFile(s.path(k, id))
	if errors.Is(e, os.ErrNotExist) {
		return Record{}, ErrNotFound
	}
	if e != nil {
		return Record{}, e
	}
	var r Record
	if json.Unmarshal(b, &r) != nil || r.ScopeType != k || r.ScopeID != id {
		return Record{}, ErrNotFound
	}
	return r, nil
}
func (s *Store) write(r Record) error {
	b, e := json.MarshalIndent(r, "", "  ")
	if e != nil {
		return e
	}
	p := s.path(r.ScopeType, r.ScopeID)
	tmp := p + ".tmp"
	f, e := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if e != nil {
		return e
	}
	if _, e = f.Write(b); e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e != nil {
		_ = os.Remove(tmp)
		return e
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if e = os.Rename(tmp, p); e != nil {
		_ = os.Remove(tmp)
		return e
	}
	d, e := os.Open(s.root)
	if e != nil {
		return e
	}
	e = d.Sync()
	if closeErr = d.Close(); e == nil {
		e = closeErr
	}
	return e
}
func safe(v string) bool { return v != "" && !strings.ContainsAny(v, "/\\.") }
func randomID() string   { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func SortExceptions(v []Exception) {
	sort.Slice(v, func(i, j int) bool { return v[i].CreatedAt.Before(v[j].CreatedAt) })
}
