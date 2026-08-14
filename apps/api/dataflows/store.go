// Package dataflows retains repository-declared, revision-exact data-flow maps and bounded analysis.
package dataflows

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

var ErrNotFound = errors.New("data-flow map not found")
var ErrInvalid = errors.New("invalid data-flow map")
var ErrConflict = errors.New("data-flow map version conflict")

type CommitmentRef struct {
	CommitmentID string   `json:"commitment_id"`
	Version      int      `json:"version"`
	DataUseIDs   []string `json:"data_use_ids"`
}
type Node struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	ResourceID  string `json:"resource_id,omitempty"`
	Interface   string `json:"interface,omitempty"`
	Accessible  bool   `json:"accessible"`
	Uncertainty string `json:"uncertainty,omitempty"`
}
type Edge struct {
	ID             string          `json:"id"`
	From           string          `json:"from"`
	To             string          `json:"to"`
	Operation      string          `json:"operation"`
	DataCategories []string        `json:"data_categories"`
	Purpose        string          `json:"purpose"`
	RetainedCopy   bool            `json:"retained_copy"`
	CommitmentRefs []CommitmentRef `json:"commitment_refs"`
}
type Citation struct {
	Path      string `json:"path"`
	StartLine int    `json:"start_line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
	Claim     string `json:"claim"`
}
type Revision struct {
	Version        int             `json:"version"`
	CodeRevision   string          `json:"code_revision"`
	Title          string          `json:"title"`
	EntryPoints    []string        `json:"entry_points"`
	Nodes          []Node          `json:"nodes"`
	Edges          []Edge          `json:"edges"`
	CommitmentRefs []CommitmentRef `json:"commitment_refs"`
	Rationale      string          `json:"rationale"`
	CreatedBy      string          `json:"created_by"`
	CreatedAt      time.Time       `json:"created_at"`
}
type Finding struct {
	ID          string     `json:"id"`
	Kind        string     `json:"kind"`
	Severity    string     `json:"severity"`
	Summary     string     `json:"summary"`
	NodeIDs     []string   `json:"node_ids,omitempty"`
	EdgeIDs     []string   `json:"edge_ids,omitempty"`
	Citations   []Citation `json:"citations"`
	Uncertainty string     `json:"uncertainty,omitempty"`
	AddedByType string     `json:"added_by_type"`
	AddedBy     string     `json:"added_by"`
	AddedAt     time.Time  `json:"added_at"`
}
type Analysis struct {
	ID            string    `json:"id"`
	MapVersion    int       `json:"map_version"`
	CodeRevision  string    `json:"code_revision"`
	Status        string    `json:"status"`
	Bounds        []string  `json:"bounds"`
	Findings      []Finding `json:"findings"`
	CreatedByType string    `json:"created_by_type"`
	CreatedBy     string    `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
}
type Diagnostic struct {
	Kind       string `json:"kind"`
	Severity   string `json:"severity"`
	Message    string `json:"message"`
	ResourceID string `json:"resource_id,omitempty"`
}
type Map struct {
	ID             string       `json:"id"`
	RepositoryID   string       `json:"repository_id"`
	CurrentVersion int          `json:"current_version"`
	Revisions      []Revision   `json:"revisions"`
	Analyses       []Analysis   `json:"analyses"`
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

func (s *Store) Create(repo, actor string, r Revision) (Map, error) {
	var out Map
	err := s.lock(func() error {
		if validateRevision(r) != nil {
			return ErrInvalid
		}
		now := s.now()
		stamp(&r, 1, actor, now)
		out = Map{ID: randomID(), RepositoryID: repo, CurrentVersion: 1, Revisions: []Revision{r}, CreatedAt: now, UpdatedAt: now}
		return s.write(out)
	})
	return project(out), err
}
func (s *Store) Revise(repo, id string, expected int, actor string, r Revision) (Map, error) {
	var out Map
	err := s.lock(func() error {
		v, e := s.read(repo, id)
		if e != nil {
			return e
		}
		if v.CurrentVersion != expected {
			return ErrConflict
		}
		if validateRevision(r) != nil {
			return ErrInvalid
		}
		stamp(&r, expected+1, actor, s.now())
		v.CurrentVersion = r.Version
		v.Revisions = append(v.Revisions, r)
		v.UpdatedAt = r.CreatedAt
		out = v
		return s.write(v)
	})
	return project(out), err
}
func (s *Store) AddAnalysis(repo, id, actorType, actor string, a Analysis) (Map, error) {
	var out Map
	err := s.lock(func() error {
		v, e := s.read(repo, id)
		if e != nil {
			return e
		}
		if validateAnalysis(v, a) != nil {
			return ErrInvalid
		}
		now := s.now()
		a.ID = randomID()
		a.CreatedByType = actorType
		a.CreatedBy = actor
		a.CreatedAt = now
		for i := range a.Findings {
			a.Findings[i].ID = randomID()
			a.Findings[i].AddedByType = actorType
			a.Findings[i].AddedBy = actor
			a.Findings[i].AddedAt = now
		}
		v.Analyses = append(v.Analyses, a)
		v.UpdatedAt = now
		out = v
		return s.write(v)
	})
	return project(out), err
}
func (s *Store) Get(repo, id string) (Map, error) {
	var out Map
	err := s.lock(func() error { var e error; out, e = s.read(repo, id); return e })
	return project(out), err
}
func (s *Store) List(repo string) ([]Map, error) {
	out := []Map{}
	err := s.lock(func() error {
		entries, e := os.ReadDir(s.repoDir(repo))
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
			var v Map
			b, e := os.ReadFile(filepath.Join(s.repoDir(repo), x.Name()))
			if e != nil {
				return e
			}
			if json.Unmarshal(b, &v) != nil || v.RepositoryID != repo {
				return ErrInvalid
			}
			out = append(out, project(v))
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
}
func validateRevision(r Revision) error {
	if len(r.CodeRevision) != 40 || r.CodeRevision != strings.ToLower(r.CodeRevision) || r.Title == "" || len(r.EntryPoints) == 0 || len(r.Nodes) == 0 || len(r.Edges) == 0 || len(r.CommitmentRefs) == 0 {
		return ErrInvalid
	}
	kinds := map[string]bool{"interaction": true, "interface": true, "package": true, "store": true, "extension": true, "release": true, "environment": true, "audience": true, "external_recipient": true}
	ids := map[string]bool{}
	for _, n := range r.Nodes {
		if n.ID == "" || ids[n.ID] || !kinds[n.Kind] || n.Name == "" || (!n.Accessible && strings.TrimSpace(n.Uncertainty) == "") {
			return ErrInvalid
		}
		ids[n.ID] = true
	}
	for _, id := range r.EntryPoints {
		if !ids[id] {
			return ErrInvalid
		}
	}
	edgeIDs := map[string]bool{}
	for _, e := range r.Edges {
		if e.ID == "" || edgeIDs[e.ID] || !ids[e.From] || !ids[e.To] || e.Operation == "" || len(e.DataCategories) == 0 || e.Purpose == "" || len(e.CommitmentRefs) == 0 {
			return ErrInvalid
		}
		if validateRefs(e.CommitmentRefs) != nil {
			return ErrInvalid
		}
		edgeIDs[e.ID] = true
	}
	return validateRefs(r.CommitmentRefs)
}
func validateRefs(v []CommitmentRef) error {
	for _, r := range v {
		if r.CommitmentID == "" || r.Version < 1 || len(r.DataUseIDs) == 0 {
			return ErrInvalid
		}
	}
	return nil
}
func validateAnalysis(v Map, a Analysis) error {
	if a.MapVersion < 1 || a.MapVersion > v.CurrentVersion || a.CodeRevision != v.Revisions[a.MapVersion-1].CodeRevision || a.Status != "completed" || len(a.Bounds) == 0 || len(a.Bounds) > 20 || len(a.Findings) == 0 || len(a.Findings) > 100 {
		return ErrInvalid
	}
	nodes := map[string]bool{}
	edges := map[string]bool{}
	for _, n := range v.Revisions[a.MapVersion-1].Nodes {
		nodes[n.ID] = true
	}
	for _, e := range v.Revisions[a.MapVersion-1].Edges {
		edges[e.ID] = true
	}
	validKind := map[string]bool{"undeclared_flow": true, "declared_observed_difference": true, "inaccessible_dependency": true, "uncertainty": true, "confirmed": true}
	validSeverity := map[string]bool{"info": true, "warning": true, "blocking": true}
	for _, f := range a.Findings {
		if !validKind[f.Kind] || !validSeverity[f.Severity] || f.Summary == "" || len(f.Summary) > 2000 || len(f.Citations) == 0 || len(f.Citations) > 20 {
			return ErrInvalid
		}
		for _, id := range f.NodeIDs {
			if !nodes[id] {
				return ErrInvalid
			}
		}
		for _, id := range f.EdgeIDs {
			if !edges[id] {
				return ErrInvalid
			}
		}
		for _, c := range f.Citations {
			if c.Path == "" || strings.Contains(c.Path, "://") || c.Claim == "" || len(c.Claim) > 500 || c.StartLine < 1 || c.EndLine < c.StartLine {
				return ErrInvalid
			}
		}
	}
	return nil
}
func project(v Map) Map {
	if len(v.Revisions) == 0 {
		return v
	}
	r := v.Revisions[len(v.Revisions)-1]
	d := []Diagnostic{}
	add := func(k, s, m, id string) { d = append(d, Diagnostic{k, s, m, id}) }
	for _, n := range r.Nodes {
		if !n.Accessible {
			add("inaccessible_dependency", "warning", "Dependency details are unavailable at this permission boundary.", n.ID)
		}
		if n.Uncertainty != "" {
			add("uncertainty", "warning", n.Uncertainty, n.ID)
		}
	}
	for _, a := range v.Analyses {
		if a.MapVersion != v.CurrentVersion || a.CodeRevision != r.CodeRevision {
			add("stale_analysis", "warning", "Analysis targets an earlier declaration or code revision.", a.ID)
		}
		for _, f := range a.Findings {
			if f.Kind == "undeclared_flow" || f.Kind == "declared_observed_difference" {
				add(f.Kind, f.Severity, f.Summary, f.ID)
			}
		}
	}
	v.Diagnostics = d
	return v
}
func (s *Store) repoDir(repo string) string {
	return filepath.Join(s.root, "repo-"+hex.EncodeToString([]byte(repo)))
}
func (s *Store) read(repo, id string) (Map, error) {
	if id == "" || strings.ContainsAny(id, "/\\") {
		return Map{}, ErrNotFound
	}
	b, e := os.ReadFile(filepath.Join(s.repoDir(repo), id+".json"))
	if os.IsNotExist(e) {
		return Map{}, ErrNotFound
	}
	if e != nil {
		return Map{}, e
	}
	var v Map
	if json.Unmarshal(b, &v) != nil || v.ID != id || v.RepositoryID != repo {
		return Map{}, ErrInvalid
	}
	return v, nil
}
func (s *Store) write(v Map) error {
	dir := s.repoDir(v.RepositoryID)
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(dir, ".data-flow-*")
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
	closeErr := tmp.Close()
	if e == nil {
		e = closeErr
	}
	if e == nil {
		e = os.Rename(name, filepath.Join(dir, v.ID+".json"))
	}
	return e
}
func (s *Store) lock(fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	lockPath := filepath.Join(s.root, ".lock")
	f, e := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
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
func randomID() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
