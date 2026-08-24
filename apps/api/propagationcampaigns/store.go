// Package propagationcampaigns persists shared outcomes that must reach maintained targets.
package propagationcampaigns

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

var ErrNotFound = errors.New("propagation campaign not found")
var ErrInvalid = errors.New("invalid propagation campaign")
var ErrConflict = errors.New("propagation campaign request changed")

type Source struct {
	Kind         string   `json:"kind"`
	ResourceID   string   `json:"resource_id"`
	RepositoryID string   `json:"repository_id"`
	Commits      []string `json:"commits"`
	Label        string   `json:"label"`
}
type Target struct {
	ID                 string    `json:"id"`
	Kind               string    `json:"kind"`
	RepositoryID       string    `json:"repository_id,omitempty"`
	ReleaseLine        string    `json:"release_line,omitempty"`
	Package            string    `json:"package,omitempty"`
	OwnerIDs           []string  `json:"owner_ids"`
	DependsOn          []string  `json:"depends_on,omitempty"`
	Deadline           time.Time `json:"deadline"`
	AcceptanceCriteria []string  `json:"acceptance_criteria,omitempty"`
	State              string    `json:"state"`
	Diagnostic         string    `json:"diagnostic,omitempty"`
	Authority          string    `json:"authority"`
}
type CompletionPolicy struct {
	Mode              string `json:"mode"`
	MinimumTargets    int    `json:"minimum_targets,omitempty"`
	RequireAcceptance bool   `json:"require_acceptance"`
}
type Campaign struct {
	ID                 string           `json:"id"`
	RequestID          string           `json:"request_id"`
	RequestDigest      string           `json:"request_digest,omitempty"`
	RepositoryID       string           `json:"repository_id"`
	Title              string           `json:"title"`
	Intent             string           `json:"intent"`
	AcceptanceCriteria []string         `json:"acceptance_criteria"`
	Source             Source           `json:"source"`
	Targets            []Target         `json:"targets"`
	CompletionPolicy   CompletionPolicy `json:"completion_policy"`
	CreatedBy          string           `json:"created_by"`
	CreatedAt          time.Time        `json:"created_at"`
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
	if e := os.MkdirAll(root, 0700); e != nil {
		return nil, e
	}
	return &Store{root: root, now: func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }}, nil
}
func (s *Store) Create(v Campaign, actor, digest string) (Campaign, error) {
	if validate(v) != nil {
		return Campaign{}, ErrInvalid
	}
	var out Campaign
	e := s.lock(func() error {
		values, e := s.list(v.RepositoryID)
		if e != nil {
			return e
		}
		for _, x := range values {
			if x.RequestID == v.RequestID {
				if x.RequestDigest != digest {
					return ErrConflict
				}
				out = x
				return nil
			}
		}
		v.ID = randomID()
		v.RequestDigest = digest
		v.CreatedBy = actor
		v.CreatedAt = s.now()
		out = v
		return s.write(v)
	})
	return out, e
}
func (s *Store) Get(repo, id string) (Campaign, error) {
	var out Campaign
	e := s.lock(func() error { var x error; out, x = s.read(repo, id); return x })
	return out, e
}
func (s *Store) List(repo string) ([]Campaign, error) {
	var out []Campaign
	e := s.lock(func() error { var x error; out, x = s.list(repo); return x })
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, e
}
func validate(v Campaign) error {
	kinds := map[string]bool{"merged_pull": true, "security_repair": true, "regression_correction": true, "policy_change": true, "package_release": true, "interface_evolution": true}
	if v.RequestID == "" || v.RepositoryID == "" || strings.TrimSpace(v.Title) == "" || strings.TrimSpace(v.Intent) == "" || !kinds[v.Source.Kind] || v.Source.ResourceID == "" || v.Source.RepositoryID != v.RepositoryID || len(v.Source.Commits) == 0 || len(v.AcceptanceCriteria) == 0 || len(v.Targets) == 0 {
		return ErrInvalid
	}
	ids := map[string]bool{}
	for _, t := range v.Targets {
		if t.ID == "" || ids[t.ID] || (t.Kind != "repository" && t.Kind != "package") || len(t.OwnerIDs) == 0 || t.Deadline.IsZero() {
			return ErrInvalid
		}
		ids[t.ID] = true
		if t.Kind == "repository" && (t.RepositoryID == "" || t.ReleaseLine == "") {
			return ErrInvalid
		}
		if t.Kind == "package" && (t.Package == "" || t.ReleaseLine == "") {
			return ErrInvalid
		}
	}
	for _, t := range v.Targets {
		for _, d := range t.DependsOn {
			if !ids[d] || d == t.ID {
				return ErrInvalid
			}
		}
	}
	dependencies := map[string][]string{}
	for _, t := range v.Targets {
		dependencies[t.ID] = t.DependsOn
	}
	visiting, visited := map[string]bool{}, map[string]bool{}
	var cyclic func(string) bool
	cyclic = func(id string) bool {
		if visiting[id] {
			return true
		}
		if visited[id] {
			return false
		}
		visiting[id] = true
		for _, dependency := range dependencies[id] {
			if cyclic(dependency) {
				return true
			}
		}
		visiting[id] = false
		visited[id] = true
		return false
	}
	for id := range ids {
		if cyclic(id) {
			return ErrInvalid
		}
	}
	if v.CompletionPolicy.Mode != "all" && v.CompletionPolicy.Mode != "minimum" && v.CompletionPolicy.Mode != "ordered" {
		return ErrInvalid
	}
	if v.CompletionPolicy.Mode == "minimum" && (v.CompletionPolicy.MinimumTargets < 1 || v.CompletionPolicy.MinimumTargets > len(v.Targets)) {
		return ErrInvalid
	}
	return nil
}
func (s *Store) list(repo string) ([]Campaign, error) {
	out := []Campaign{}
	entries, e := os.ReadDir(filepath.Join(s.root, repo))
	if os.IsNotExist(e) {
		return out, nil
	}
	if e != nil {
		return nil, e
	}
	for _, x := range entries {
		if x.IsDir() || !strings.HasSuffix(x.Name(), ".json") {
			continue
		}
		b, e := os.ReadFile(filepath.Join(s.root, repo, x.Name()))
		if e != nil {
			return nil, e
		}
		var v Campaign
		if json.Unmarshal(b, &v) != nil {
			return nil, ErrInvalid
		}
		out = append(out, v)
	}
	return out, nil
}
func (s *Store) read(repo, id string) (Campaign, error) {
	b, e := os.ReadFile(filepath.Join(s.root, repo, id+".json"))
	if os.IsNotExist(e) {
		return Campaign{}, ErrNotFound
	}
	if e != nil {
		return Campaign{}, e
	}
	var v Campaign
	if json.Unmarshal(b, &v) != nil {
		return Campaign{}, ErrInvalid
	}
	return v, nil
}
func (s *Store) write(v Campaign) error {
	dir := filepath.Join(s.root, v.RepositoryID)
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	tmp, e := os.CreateTemp(dir, ".campaign-*")
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
	if c := tmp.Close(); e == nil {
		e = c
	}
	if e == nil {
		e = os.Rename(name, filepath.Join(dir, v.ID+".json"))
	}
	if e == nil {
		d, x := os.Open(dir)
		if x == nil {
			e = d.Sync()
			d.Close()
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
func randomID() string { b := make([]byte, 12); _, _ = rand.Read(b); return hex.EncodeToString(b) }
