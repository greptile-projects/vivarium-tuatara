// Package recoverycommitments persists versioned repository recovery contracts.
package recoverycommitments

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

var ErrNotFound = errors.New("recovery commitment not found")
var ErrInvalid = errors.New("invalid recovery commitment")
var ErrConflict = errors.New("recovery commitment version conflict")

type Dependency struct {
	TargetID               string `json:"target_id"`
	Protected              bool   `json:"protected"`
	RestorationTimeMinutes int    `json:"restoration_time_minutes,omitempty"`
}
type Target struct {
	ID                     string       `json:"id"`
	Kind                   string       `json:"kind"`
	ResourceID             string       `json:"resource_id,omitempty"`
	Name                   string       `json:"name"`
	Capability             string       `json:"capability"`
	OwnerIDs               []string     `json:"owner_ids"`
	AcceptableLossMinutes  int          `json:"acceptable_loss_minutes"`
	RestorationTimeMinutes int          `json:"restoration_time_minutes"`
	Retention              string       `json:"retention"`
	Jurisdictions          []string     `json:"jurisdictions"`
	ValidationCriteria     []string     `json:"validation_criteria"`
	Dependencies           []Dependency `json:"dependencies"`
	Exclusions             []string     `json:"exclusions,omitempty"`
}
type Link struct {
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	URL     string `json:"url,omitempty"`
	Label   string `json:"label"`
	AddedBy string `json:"added_by,omitempty"`
}
type Exception struct {
	ID         string    `json:"id"`
	TargetID   string    `json:"target_id"`
	Reason     string    `json:"reason"`
	Mitigation string    `json:"mitigation"`
	ApprovedBy string    `json:"approved_by"`
	ExpiresAt  time.Time `json:"expires_at"`
}
type Revision struct {
	Version    int         `json:"version"`
	Title      string      `json:"title"`
	Summary    string      `json:"summary"`
	OwnerIDs   []string    `json:"owner_ids"`
	Targets    []Target    `json:"targets"`
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
		stamp(&r, 1, actor, now, nil)
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
		stamp(&r, expected+1, actor, s.now(), v.Revisions[len(v.Revisions)-1].Links)
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
func stamp(r *Revision, version int, actor string, now time.Time, previous []Link) {
	r.Version = version
	r.CreatedBy = actor
	r.CreatedAt = now
	for i := range r.Links {
		r.Links[i].AddedBy = actor
		for _, existing := range previous {
			if existing.Kind == r.Links[i].Kind && existing.ID == r.Links[i].ID {
				r.Links[i].AddedBy = existing.AddedBy
				break
			}
		}
	}
}
func validate(r Revision) error {
	if strings.TrimSpace(r.Title) == "" || strings.TrimSpace(r.Summary) == "" || len(r.OwnerIDs) == 0 || len(r.Targets) == 0 {
		return ErrInvalid
	}
	kinds := map[string]bool{"repository": true, "package": true, "artifact": true, "configuration": true, "collaboration_records": true, "deployed_service_data": true}
	ids := map[string]bool{}
	for _, x := range r.Targets {
		if !kinds[x.Kind] || x.ID == "" || ids[x.ID] || x.Name == "" || x.Capability == "" || x.AcceptableLossMinutes < 0 || x.RestorationTimeMinutes <= 0 || x.Retention == "" || len(x.Jurisdictions) == 0 || len(x.ValidationCriteria) == 0 {
			return ErrInvalid
		}
		ids[x.ID] = true
	}
	for _, x := range r.Targets {
		for _, d := range x.Dependencies {
			if !ids[d.TargetID] || d.TargetID == x.ID || d.RestorationTimeMinutes < 0 {
				return ErrInvalid
			}
		}
	}
	linkKinds := map[string]bool{"service_objective": true, "environment": true, "incident": true, "privacy_rule": true, "governance": true}
	for _, x := range r.Links {
		if !linkKinds[x.Kind] || x.ID == "" || x.Label == "" {
			return ErrInvalid
		}
	}
	for _, x := range r.Exceptions {
		if x.ID == "" || !ids[x.TargetID] || x.Reason == "" || x.Mitigation == "" || x.ApprovedBy == "" || x.ExpiresAt.IsZero() {
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
	for _, x := range r.Targets {
		if len(x.OwnerIDs) == 0 {
			add("missing_ownership", "blocking", "Recovery target has no accountable restoration owner.", x.ID, r.CreatedBy)
		}
		for _, dep := range x.Dependencies {
			if !dep.Protected {
				add("unprotected_dependency", "blocking", "A required dependency has no declared recovery protection.", dep.TargetID, r.CreatedBy)
			}
			if dep.RestorationTimeMinutes > x.RestorationTimeMinutes {
				add("impossible_target", "blocking", "A dependency restores later than the capability target allows.", x.ID, r.CreatedBy)
			}
		}
	}
	for _, x := range r.Exceptions {
		if !x.ExpiresAt.After(s.now()) {
			add("expired_exception", "blocking", "A recovery exception has expired.", x.ID, x.ApprovedBy)
		} else if x.ExpiresAt.Before(s.now().Add(30 * 24 * time.Hour)) {
			add("expiring_exception", "warning", "A recovery exception expires within 30 days.", x.ID, x.ApprovedBy)
		}
	}
	v.Diagnostics = d
	return v
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
	f, e := os.CreateTemp(dir, ".recovery-")
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
