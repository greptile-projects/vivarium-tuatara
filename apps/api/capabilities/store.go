// Package capabilities persists immutable, revision-exact capability inventories.
package capabilities

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

var ErrNotFound = errors.New("capability not found")
var ErrInvalid = errors.New("invalid capability")
var ErrConflict = errors.New("capability version conflict")

var itemKinds = map[string]bool{"interface": true, "symbol": true, "flag": true, "package": true, "schema": true, "configuration": true, "documentation": true, "journey": true, "release": true}
var discoveryStates = map[string]bool{"declared": true, "dynamic": true, "unknown": true}
var evidenceStates = map[string]bool{"current": true, "stale": true, "inaccessible": true, "unknown": true}

type Item struct {
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Path     string `json:"path,omitempty"`
	Symbol   string `json:"symbol,omitempty"`
	Revision string `json:"revision"`
	Notes    string `json:"notes,omitempty"`
}
type Consumer struct {
	Name                 string     `json:"name"`
	RepositoryID         string     `json:"repository_id,omitempty"`
	OwnerIDs             []string   `json:"owner_ids"`
	Environment          string     `json:"environment"`
	Revision             string     `json:"revision,omitempty"`
	Discovery            string     `json:"discovery"`
	EvidenceState        string     `json:"evidence_state"`
	EvidenceReference    string     `json:"evidence_reference,omitempty"`
	LastObservedAt       *time.Time `json:"last_observed_at,omitempty"`
	CompatibilityPromise string     `json:"compatibility_promise"`
}
type Revision struct {
	Version          int        `json:"version"`
	Name             string     `json:"name"`
	Summary          string     `json:"summary"`
	CommitID         string     `json:"commit_id"`
	ReleaseID        string     `json:"release_id"`
	ReleaseVersion   string     `json:"release_version"`
	OwnerIDs         []string   `json:"owner_ids"`
	Items            []Item     `json:"items"`
	Consumers        []Consumer `json:"consumers"`
	UnknownUse       bool       `json:"unknown_use"`
	UnknownUseReason string     `json:"unknown_use_reason,omitempty"`
	CreatedBy        string     `json:"created_by"`
	CreatedAt        time.Time  `json:"created_at"`
}
type Diagnostic struct {
	Kind          string `json:"kind"`
	Severity      string `json:"severity"`
	Message       string `json:"message"`
	Consumer      string `json:"consumer,omitempty"`
	ConsumerIndex *int   `json:"consumer_index,omitempty"`
}
type Capability struct {
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
func (s *Store) Create(repo, actor string, r Revision) (Capability, error) {
	var out Capability
	err := s.lock(func() error {
		if validate(r) != nil || repo == "" || actor == "" {
			return ErrInvalid
		}
		now := s.now()
		stamp(&r, 1, actor, now)
		out = Capability{ID: randomID(), RepositoryID: repo, CurrentVersion: 1, Revisions: []Revision{r}, CreatedAt: now, UpdatedAt: now}
		return s.write(out)
	})
	return project(out), err
}
func (s *Store) Revise(repo, id string, expected int, actor string, r Revision) (Capability, error) {
	var out Capability
	err := s.lock(func() error {
		v, e := s.read(repo, id)
		if e != nil {
			return e
		}
		if v.CurrentVersion != expected {
			return ErrConflict
		}
		if validate(r) != nil || actor == "" {
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
func (s *Store) Get(repo, id string) (Capability, error) {
	var out Capability
	err := s.lock(func() error { var e error; out, e = s.read(repo, id); return e })
	return project(out), err
}
func (s *Store) List(repo string) ([]Capability, error) {
	var raw []Capability
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
				raw = append(raw, project(v))
			}
		}
		return nil
	})
	sort.Slice(raw, func(i, j int) bool { return raw[i].UpdatedAt.After(raw[j].UpdatedAt) })
	return raw, err
}
func validate(r Revision) error {
	if strings.TrimSpace(r.Name) == "" || strings.TrimSpace(r.Summary) == "" || len(r.CommitID) != 40 || r.ReleaseID == "" || len(r.OwnerIDs) == 0 || len(r.Items) == 0 {
		return ErrInvalid
	}
	for _, x := range r.Items {
		if !itemKinds[x.Kind] || x.Name == "" || len(x.Revision) != 40 {
			return ErrInvalid
		}
		if x.Kind != "release" && x.Path == "" {
			return ErrInvalid
		}
	}
	for _, x := range r.Consumers {
		if x.Name == "" || len(x.OwnerIDs) == 0 || x.Environment == "" || !discoveryStates[x.Discovery] || !evidenceStates[x.EvidenceState] || x.CompatibilityPromise == "" {
			return ErrInvalid
		}
		if x.Revision != "" && len(x.Revision) != 40 {
			return ErrInvalid
		}
		if x.EvidenceState == "current" && (x.Revision == "" || x.EvidenceReference == "" || x.LastObservedAt == nil) {
			return ErrInvalid
		}
		if x.EvidenceState == "current" && x.RepositoryID == "" {
			return ErrInvalid
		}
	}
	if r.UnknownUse && strings.TrimSpace(r.UnknownUseReason) == "" {
		return ErrInvalid
	}
	return nil
}
func project(v Capability) Capability {
	if len(v.Revisions) == 0 {
		return v
	}
	r := v.Revisions[len(v.Revisions)-1]
	d := []Diagnostic{}
	add := func(k, s, m, c string, consumerIndex *int) { d = append(d, Diagnostic{k, s, m, c, consumerIndex}) }
	if r.UnknownUse {
		add("unknown_use", "blocking", r.UnknownUseReason, "", nil)
	}
	for index, c := range r.Consumers {
		consumerIndex := index
		switch c.Discovery {
		case "dynamic":
			add("dynamic_use", "warning", "Runtime discovery may reveal additional use.", c.Name, &consumerIndex)
		case "unknown":
			add("unknown_consumer", "blocking", "The consumer footprint is not established.", c.Name, &consumerIndex)
		}
		switch c.EvidenceState {
		case "stale":
			add("stale_evidence", "blocking", "Usage evidence does not describe the declared revision.", c.Name, &consumerIndex)
		case "inaccessible":
			add("inaccessible_evidence", "blocking", "Usage evidence exists but is not available to this inventory.", c.Name, &consumerIndex)
		case "unknown":
			add("unknown_evidence", "blocking", "Usage has not been measured.", c.Name, &consumerIndex)
		}
	}
	v.Diagnostics = d
	return v
}
func stamp(r *Revision, v int, a string, t time.Time) {
	r.Version = v
	r.CreatedBy = a
	r.CreatedAt = t
}
func (s *Store) repoDir(repo string) string {
	return filepath.Join(s.root, "repo-"+base64.RawURLEncoding.EncodeToString([]byte(repo)))
}
func (s *Store) read(repo, id string) (Capability, error) {
	if repo == "" || id == "" || strings.ContainsAny(id, "/\\") {
		return Capability{}, ErrNotFound
	}
	v, e := s.readFile(filepath.Join(s.repoDir(repo), id+".json"))
	if e != nil {
		return v, e
	}
	if v.RepositoryID != repo || v.ID != id {
		return Capability{}, ErrNotFound
	}
	return v, nil
}
func (s *Store) readFile(name string) (Capability, error) {
	var v Capability
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
func (s *Store) write(v Capability) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	dir := s.repoDir(v.RepositoryID)
	if e = os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(dir, ".capability-")
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
	if e == nil {
		var directory *os.File
		directory, e = os.Open(dir)
		if e == nil {
			e = directory.Sync()
			closeErr := directory.Close()
			if e == nil {
				e = closeErr
			}
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
func randomID() string { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
