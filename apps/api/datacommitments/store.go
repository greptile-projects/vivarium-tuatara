// Package datacommitments persists versioned declarations of permitted product data use.
package datacommitments

import (
	"crypto/rand"
	"encoding/base64"
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

var ErrNotFound = errors.New("data commitment not found")
var ErrInvalid = errors.New("invalid data commitment")
var ErrConflict = errors.New("data commitment version conflict")

type Scope struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id,omitempty"`
	Name       string `json:"name"`
}
type DataUse struct {
	ID            string   `json:"id"`
	Category      string   `json:"category"`
	Subjects      []string `json:"subjects"`
	Purposes      []string `json:"purposes"`
	Collection    string   `json:"collection"`
	Processing    []string `json:"processing"`
	Sharing       []string `json:"sharing"`
	Retention     string   `json:"retention"`
	Residency     []string `json:"residency"`
	Deletion      string   `json:"deletion"`
	Consent       string   `json:"consent"`
	OwnerIDs      []string `json:"owner_ids"`
	Guarantee     string   `json:"guarantee,omitempty"`
	Supported     bool     `json:"supported"`
	ConflictsWith []string `json:"conflicts_with,omitempty"`
}
type Link struct {
	Kind    string `json:"kind"`
	URL     string `json:"url"`
	Label   string `json:"label"`
	AddedBy string `json:"added_by,omitempty"`
}
type Exception struct {
	ID         string    `json:"id"`
	DataUseID  string    `json:"data_use_id"`
	Reason     string    `json:"reason"`
	Mitigation string    `json:"mitigation"`
	ApprovedBy string    `json:"approved_by"`
	ExpiresAt  time.Time `json:"expires_at"`
}
type Revision struct {
	Version    int         `json:"version"`
	Title      string      `json:"title"`
	Summary    string      `json:"summary"`
	Scopes     []Scope     `json:"scopes"`
	DataUses   []DataUse   `json:"data_uses"`
	OwnerIDs   []string    `json:"owner_ids"`
	Links      []Link      `json:"links"`
	Exceptions []Exception `json:"exceptions"`
	Rationale  string      `json:"rationale"`
	CreatedBy  string      `json:"created_by"`
	CreatedAt  time.Time   `json:"created_at"`
}
type Diagnostic struct {
	Kind         string `json:"kind"`
	Severity     string `json:"severity"`
	Message      string `json:"message"`
	ResourceID   string `json:"resource_id,omitempty"`
	AttributedTo string `json:"attributed_to"`
}
type Commitment struct {
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
func (s *Store) Create(repo, actor string, r Revision) (Commitment, error) {
	var out Commitment
	err := s.lock(func() error {
		if validate(r) != nil {
			return ErrInvalid
		}
		now := s.now()
		stamp(&r, 1, actor, now)
		out = Commitment{ID: randomID(), RepositoryID: repo, CurrentVersion: 1, Revisions: []Revision{r}, CreatedAt: now, UpdatedAt: now}
		return s.write(out)
	})
	return s.project(out), err
}
func (s *Store) Revise(id string, expected int, actor string, r Revision) (Commitment, error) {
	var out Commitment
	err := s.lock(func() error {
		v, e := s.read(id)
		if e != nil {
			return e
		}
		if v.CurrentVersion != expected {
			return ErrConflict
		}
		if validate(r) != nil {
			return ErrInvalid
		}
		stamp(&r, expected+1, actor, s.now())
		v.CurrentVersion = r.Version
		v.Revisions = append(v.Revisions, r)
		v.UpdatedAt = r.CreatedAt
		out = v
		return s.write(v)
	})
	return s.project(out), err
}
func (s *Store) Get(id string) (Commitment, error) {
	var out Commitment
	err := s.lock(func() error { var e error; out, e = s.read(id); return e })
	return s.project(out), err
}
func (s *Store) List(repo string) ([]Commitment, error) {
	out := []Commitment{}
	err := s.lock(func() error {
		entries, e := os.ReadDir(s.repoDir(repo))
		if os.IsNotExist(e) {
			return nil
		}
		if e != nil {
			return e
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			v, e := s.readFile(filepath.Join(s.repoDir(repo), entry.Name()))
			if e != nil {
				return e
			}
			if v.RepositoryID == repo {
				out = append(out, s.project(v))
			}
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, err
}
func stamp(r *Revision, version int, actor string, now time.Time) {
	r.Version = version
	r.CreatedBy = actor
	r.CreatedAt = now
	for i := range r.Links {
		r.Links[i].AddedBy = actor
	}
}
func validate(r Revision) error {
	validScope := map[string]bool{"repository": true, "release": true, "extension": true, "experiment": true, "environment": true}
	if strings.TrimSpace(r.Title) == "" || len(r.Scopes) == 0 || len(r.DataUses) == 0 || len(r.OwnerIDs) == 0 {
		return ErrInvalid
	}
	for _, x := range r.Scopes {
		if !validScope[x.Kind] || strings.TrimSpace(x.Name) == "" || (x.Kind != "repository" && strings.TrimSpace(x.ResourceID) == "") {
			return ErrInvalid
		}
	}
	ids := map[string]bool{}
	for _, x := range r.DataUses {
		if strings.TrimSpace(x.ID) == "" || ids[x.ID] || strings.TrimSpace(x.Category) == "" || len(x.Subjects) == 0 || len(x.Purposes) == 0 || strings.TrimSpace(x.Collection) == "" || len(x.Processing) == 0 || strings.TrimSpace(x.Retention) == "" || strings.TrimSpace(x.Deletion) == "" || strings.TrimSpace(x.Consent) == "" {
			return ErrInvalid
		}
		ids[x.ID] = true
	}
	kinds := map[string]bool{"policy": true, "notice": true}
	seen := map[string]bool{}
	for _, x := range r.Links {
		if !kinds[x.Kind] || strings.TrimSpace(x.URL) == "" || strings.TrimSpace(x.Label) == "" {
			return ErrInvalid
		}
		seen[x.Kind] = true
	}
	if !seen["policy"] || !seen["notice"] {
		return ErrInvalid
	}
	for _, x := range r.Exceptions {
		if x.ID == "" || !ids[x.DataUseID] || x.Reason == "" || x.Mitigation == "" || x.ApprovedBy == "" || x.ExpiresAt.IsZero() {
			return ErrInvalid
		}
	}
	return nil
}
func (s *Store) project(v Commitment) Commitment {
	if len(v.Revisions) == 0 {
		return v
	}
	r := v.Revisions[len(v.Revisions)-1]
	d := []Diagnostic{}
	add := func(k, sev, msg, id, actor string) { d = append(d, Diagnostic{k, sev, msg, id, actor}) }
	ids := map[string]bool{}
	for _, x := range r.DataUses {
		ids[x.ID] = true
		if len(x.OwnerIDs) == 0 {
			add("missing_ownership", "blocking", "Data use has no accountable owner.", x.ID, r.CreatedBy)
		}
		if !x.Supported {
			add("unsupported_guarantee", "blocking", "The declared data handling guarantee is not currently supported.", x.ID, r.CreatedBy)
		}
	}
	for _, x := range r.DataUses {
		for _, other := range x.ConflictsWith {
			if containsUse(r.DataUses, other) {
				add("conflicting_commitment", "warning", "Current data uses declare an unresolved conflict.", x.ID, r.CreatedBy)
				break
			}
		}
	}
	for _, x := range r.Exceptions {
		if !x.ExpiresAt.After(s.now()) {
			add("expired_exception", "blocking", "A permitted exception has expired.", x.ID, x.ApprovedBy)
		} else if x.ExpiresAt.Before(s.now().Add(30 * 24 * time.Hour)) {
			add("expiring_exception", "warning", "A permitted exception expires within 30 days.", x.ID, x.ApprovedBy)
		}
	}
	v.Diagnostics = d
	return v
}
func containsUse(v []DataUse, id string) bool {
	for _, x := range v {
		if x.ID == id {
			return true
		}
	}
	return false
}
func (s *Store) repoDir(repo string) string {
	return filepath.Join(s.root, "repo-"+base64.RawURLEncoding.EncodeToString([]byte(repo)))
}
func (s *Store) read(id string) (Commitment, error) {
	entries, e := os.ReadDir(s.root)
	if e != nil {
		return Commitment{}, e
	}
	for _, entry := range entries {
		if entry.IsDir() {
			v, x := s.readFile(filepath.Join(s.root, entry.Name(), id+".json"))
			if !errors.Is(x, ErrNotFound) {
				return v, x
			}
		}
	}
	return Commitment{}, ErrNotFound
}
func (s *Store) readFile(name string) (Commitment, error) {
	var v Commitment
	b, e := os.ReadFile(name)
	if os.IsNotExist(e) {
		return v, ErrNotFound
	}
	if e != nil {
		return v, e
	}
	if json.Unmarshal(b, &v) != nil {
		return v, ErrInvalid
	}
	return v, nil
}
func (s *Store) write(v Commitment) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	dir := s.repoDir(v.RepositoryID)
	if e = os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(dir, ".commitment-")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e == nil {
		e = os.Rename(name, filepath.Join(dir, v.ID+".json"))
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
func randomID() string { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
