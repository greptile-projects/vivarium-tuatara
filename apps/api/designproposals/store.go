// Package designproposals persists product behavior proposals before implementation.
package designproposals

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

var ErrNotFound = errors.New("design proposal not found")
var ErrInvalid = errors.New("invalid design proposal")
var ErrConflict = errors.New("design proposal version conflict")

type Source struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Summary    string `json:"summary"`
}
type Journey struct {
	Name  string   `json:"name"`
	Actor string   `json:"actor"`
	Goal  string   `json:"goal"`
	Steps []string `json:"steps"`
}
type State struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     string `json:"content"`
}
type Evidence struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Summary    string `json:"summary"`
	Accessible bool   `json:"accessible"`
	Gap        string `json:"gap,omitempty"`
}
type Artifact struct {
	ID           string   `json:"id"`
	Kind         string   `json:"kind"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	Content      string   `json:"content"`
	Interactions []string `json:"interactions"`
	Audience     []string `json:"audience"`
}
type Revision struct {
	Version            int        `json:"version"`
	Title              string     `json:"title"`
	UserGoal           string     `json:"user_goal"`
	Source             Source     `json:"source"`
	Journeys           []Journey  `json:"journeys"`
	States             []State    `json:"states"`
	Content            []string   `json:"content"`
	Constraints        []string   `json:"constraints"`
	Alternatives       []string   `json:"alternatives"`
	SuccessMeasures    []string   `json:"success_measures"`
	AffectedComponents []string   `json:"affected_components"`
	Evidence           []Evidence `json:"evidence"`
	Artifacts          []Artifact `json:"artifacts"`
	Uncertainty        []string   `json:"uncertainty"`
	CreatedBy          string     `json:"created_by"`
	CreatedAt          time.Time  `json:"created_at"`
}
type Comment struct {
	ID        string     `json:"id"`
	Revision  int        `json:"revision"`
	Body      string     `json:"body"`
	Kind      string     `json:"kind"`
	Evidence  []Evidence `json:"evidence"`
	AuthorID  string     `json:"author_id"`
	CreatedAt time.Time  `json:"created_at"`
}
type Acknowledgement struct {
	Revision  int       `json:"revision"`
	OwnerID   string    `json:"owner_id"`
	Status    string    `json:"status"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"created_at"`
}
type Proposal struct {
	ID               string            `json:"id"`
	RepositoryID     string            `json:"repository_id"`
	OwnerIDs         []string          `json:"owner_ids"`
	CurrentVersion   int               `json:"current_version"`
	Revisions        []Revision        `json:"revisions"`
	Comments         []Comment         `json:"comments"`
	Acknowledgements []Acknowledgement `json:"acknowledgements"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
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
func (s *Store) Create(repo, actor string, owners []string, r Revision) (Proposal, error) {
	var out Proposal
	err := s.lock(func() error {
		if repo == "" || actor == "" || !validRevision(r) || !validOwners(owners) {
			return ErrInvalid
		}
		now := s.now()
		stamp(&r, 1, actor, now)
		out = Proposal{ID: id(), RepositoryID: repo, OwnerIDs: owners, CurrentVersion: 1, Revisions: []Revision{r}, CreatedAt: now, UpdatedAt: now}
		return s.write(out)
	})
	return out, err
}
func (s *Store) Revise(repo, pid, actor string, expected int, r Revision) (Proposal, error) {
	var out Proposal
	err := s.lock(func() error {
		v, e := s.read(repo, pid)
		if e != nil {
			return e
		}
		if v.CurrentVersion != expected {
			return ErrConflict
		}
		if !validRevision(r) {
			return ErrInvalid
		}
		stamp(&r, expected+1, actor, s.now())
		v.CurrentVersion++
		v.Revisions = append(v.Revisions, r)
		v.UpdatedAt = r.CreatedAt
		out = v
		return s.write(v)
	})
	return out, err
}
func (s *Store) Comment(repo, pid, actor string, c Comment) (Proposal, error) {
	var out Proposal
	err := s.lock(func() error {
		v, e := s.read(repo, pid)
		if e != nil {
			return e
		}
		if c.Revision < 1 || c.Revision > v.CurrentVersion || strings.TrimSpace(c.Body) == "" || (c.Kind != "comment" && c.Kind != "dissent" && c.Kind != "question") {
			return ErrInvalid
		}
		c.ID = id()
		c.AuthorID = actor
		c.CreatedAt = s.now()
		v.Comments = append(v.Comments, c)
		v.UpdatedAt = c.CreatedAt
		out = v
		return s.write(v)
	})
	return out, err
}
func (s *Store) Acknowledge(repo, pid, actor string, a Acknowledgement) (Proposal, error) {
	var out Proposal
	err := s.lock(func() error {
		v, e := s.read(repo, pid)
		if e != nil {
			return e
		}
		if a.Revision != v.CurrentVersion || (a.Status != "acknowledged" && a.Status != "changes_requested") || !contains(v.OwnerIDs, actor) {
			return ErrInvalid
		}
		a.OwnerID = actor
		a.CreatedAt = s.now()
		v.Acknowledgements = append(v.Acknowledgements, a)
		v.UpdatedAt = a.CreatedAt
		out = v
		return s.write(v)
	})
	return out, err
}
func (s *Store) Get(repo, pid string) (Proposal, error) {
	var out Proposal
	err := s.lock(func() error { var e error; out, e = s.read(repo, pid); return e })
	return out, err
}
func (s *Store) List(repo string) ([]Proposal, error) {
	out := []Proposal{}
	err := s.lock(func() error {
		entries, e := os.ReadDir(s.dir(repo))
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
func validRevision(r Revision) bool {
	sourceKinds := map[string]bool{"feedback": true, "issue": true, "roadmap_outcome": true, "accessibility_finding": true, "pull_request": true}
	if strings.TrimSpace(r.Title) == "" || strings.TrimSpace(r.UserGoal) == "" || !sourceKinds[r.Source.Kind] || r.Source.ResourceID == "" || r.Source.Summary == "" || len(r.Journeys) == 0 || len(r.States) == 0 || len(r.Constraints) == 0 || len(r.Alternatives) == 0 || len(r.SuccessMeasures) == 0 || len(r.AffectedComponents) == 0 {
		return false
	}
	for _, j := range r.Journeys {
		if j.Name == "" || j.Actor == "" || j.Goal == "" || len(j.Steps) == 0 {
			return false
		}
	}
	for _, x := range r.States {
		if x.Name == "" || x.Description == "" || x.Content == "" {
			return false
		}
	}
	for _, a := range r.Artifacts {
		if a.ID == "" || a.Title == "" || a.Description == "" || (a.Kind != "wireframe" && a.Kind != "prototype") || len(a.Audience) == 0 {
			return false
		}
	}
	return true
}
func validOwners(v []string) bool { return len(v) > 0 }
func contains(v []string, x string) bool {
	for _, y := range v {
		if y == x {
			return true
		}
	}
	return false
}
func stamp(r *Revision, v int, a string, t time.Time) {
	r.Version = v
	r.CreatedBy = a
	r.CreatedAt = t
}
func (s *Store) dir(repo string) string { return filepath.Join(s.root, repo) }
func (s *Store) read(repo, pid string) (Proposal, error) {
	if pid == "" || strings.ContainsAny(pid, "/\\") {
		return Proposal{}, ErrNotFound
	}
	b, e := os.ReadFile(filepath.Join(s.dir(repo), pid+".json"))
	if os.IsNotExist(e) {
		return Proposal{}, ErrNotFound
	}
	if e != nil {
		return Proposal{}, e
	}
	var v Proposal
	if json.Unmarshal(b, &v) != nil || v.ID != pid || v.RepositoryID != repo {
		return Proposal{}, ErrNotFound
	}
	return v, nil
}
func (s *Store) write(v Proposal) error {
	if e := os.MkdirAll(s.dir(v.RepositoryID), 0700); e != nil {
		return e
	}
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	tmp := filepath.Join(s.dir(v.RepositoryID), "."+v.ID+".tmp")
	if e = os.WriteFile(tmp, append(b, '\n'), 0600); e != nil {
		return e
	}
	return os.Rename(tmp, filepath.Join(s.dir(v.RepositoryID), v.ID+".json"))
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
func id() string { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
