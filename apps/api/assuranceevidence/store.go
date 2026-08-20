// Package assuranceevidence retains control-bound evidence definitions and immutable packages.
package assuranceevidence

import (
	"crypto/rand"
	"crypto/sha256"
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

var ErrNotFound = errors.New("assurance evidence not found")
var ErrInvalid = errors.New("invalid assurance evidence")

var kinds = map[string]bool{"review": true, "check": true, "access": true, "dependency": true, "build": true, "release": true, "deployment": true, "incident": true, "continuity": true, "security": true, "privacy": true, "governance": true}

type Query struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	ResourceID  string `json:"resource_id,omitempty"`
	Revision    string `json:"revision,omitempty"`
	Path        string `json:"path,omitempty"`
	Required    bool   `json:"required"`
	MaxAgeHours int    `json:"max_age_hours"`
}
type Definition struct {
	ID             string    `json:"id"`
	RepositoryID   string    `json:"repository_id"`
	ProgramID      string    `json:"program_id"`
	ProgramVersion int       `json:"program_version"`
	ControlID      string    `json:"control_id"`
	OwnerID        string    `json:"owner_id"`
	Title          string    `json:"title"`
	PeriodStartsAt time.Time `json:"period_starts_at"`
	PeriodEndsAt   time.Time `json:"period_ends_at"`
	Schedule       string    `json:"schedule"`
	Audience       []string  `json:"audience"`
	Queries        []Query   `json:"queries"`
	CreatedAt      time.Time `json:"created_at"`
}
type Source struct {
	QueryID         string    `json:"query_id"`
	Kind            string    `json:"kind"`
	ResourceID      string    `json:"resource_id,omitempty"`
	Revision        string    `json:"revision,omitempty"`
	OccurredAt      time.Time `json:"occurred_at,omitempty"`
	Provenance      string    `json:"provenance,omitempty"`
	Summary         string    `json:"summary,omitempty"`
	Transformations []string  `json:"transformations,omitempty"`
	Gap             string    `json:"gap,omitempty"`
	Contradiction   string    `json:"contradiction,omitempty"`
	Accessible      bool      `json:"accessible"`
	Digest          string    `json:"digest,omitempty"`
}
type Package struct {
	ID             string    `json:"id"`
	RepositoryID   string    `json:"repository_id"`
	DefinitionID   string    `json:"definition_id"`
	ProgramID      string    `json:"program_id"`
	ProgramVersion int       `json:"program_version"`
	ControlID      string    `json:"control_id"`
	PeriodStartsAt time.Time `json:"period_starts_at"`
	PeriodEndsAt   time.Time `json:"period_ends_at"`
	CollectedAt    time.Time `json:"collected_at"`
	CollectedBy    string    `json:"collected_by"`
	Sources        []Source  `json:"sources"`
	Coverage       int       `json:"coverage_percent"`
	Gaps           []string  `json:"gaps"`
	Contradictions []string  `json:"contradictions"`
	ManifestHash   string    `json:"manifest_hash"`
	Attestation    string    `json:"attestation"`
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
	if err := os.MkdirAll(filepath.Join(root, "definitions"), 0700); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(root, "packages"), 0700); err != nil {
		return nil, err
	}
	return &Store{root: root, now: func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }}, nil
}

func (s *Store) CreateDefinition(d Definition) (Definition, error) {
	if !validDefinition(d) {
		return Definition{}, ErrInvalid
	}
	d.ID = id()
	d.CreatedAt = s.now()
	return d, s.put("definitions", d.ID, d)
}
func (s *Store) GetDefinition(id string) (Definition, error) {
	var d Definition
	e := s.get("definitions", id, &d)
	return d, e
}
func (s *Store) ListDefinitions(repo string) ([]Definition, error) {
	var out []Definition
	e := s.list("definitions", func(b []byte) error {
		var d Definition
		if json.Unmarshal(b, &d) != nil {
			return ErrInvalid
		}
		if d.RepositoryID == repo {
			out = append(out, d)
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, e
}
func (s *Store) CreatePackage(d Definition, actor string, sources []Source) (Package, error) {
	if len(sources) == 0 {
		return Package{}, ErrInvalid
	}
	q := map[string]Query{}
	for _, x := range d.Queries {
		q[x.ID] = x
	}
	seen := map[string]bool{}
	now := s.now()
	for i := range sources {
		x := &sources[i]
		query, ok := q[x.QueryID]
		if !ok || seen[x.QueryID] || x.Kind != query.Kind {
			return Package{}, ErrInvalid
		}
		seen[x.QueryID] = true
		if !x.Accessible {
			x.ResourceID = ""
			x.Revision = ""
			x.Provenance = "restricted source"
			x.Summary = ""
			x.Transformations = nil
			if x.Gap == "" {
				x.Gap = "source is inaccessible"
			}
		}
		if x.Accessible && (x.ResourceID == "" || x.Provenance == "" || x.OccurredAt.IsZero() || x.OccurredAt.Before(d.PeriodStartsAt) || x.OccurredAt.After(d.PeriodEndsAt)) {
			return Package{}, ErrInvalid
		}
		if x.Accessible && ((query.ResourceID != "" && x.ResourceID != query.ResourceID) || (query.Revision != "" && x.Revision != query.Revision)) {
			return Package{}, ErrInvalid
		}
		if x.Accessible && now.Sub(x.OccurredAt) > time.Duration(query.MaxAgeHours)*time.Hour && x.Gap == "" {
			x.Gap = "source is outside the required freshness window"
		}
		if containsCredential(x.Provenance) || containsCredential(x.Summary) || containsCredential(x.Gap) || containsCredential(x.Contradiction) {
			return Package{}, ErrInvalid
		}
		x.Digest = digest(*x)
	}
	covered := 0
	gaps := []string{}
	contradictions := []string{}
	for _, query := range d.Queries {
		found := false
		for _, x := range sources {
			if x.QueryID == query.ID {
				found = true
				if x.Accessible && x.Gap == "" {
					covered++
				}
				if x.Gap != "" {
					gaps = append(gaps, query.ID+": "+x.Gap)
				}
				if x.Contradiction != "" {
					contradictions = append(contradictions, query.ID+": "+x.Contradiction)
				}
			}
		}
		if query.Required && !found {
			gaps = append(gaps, query.ID+": no source record collected")
		}
	}
	p := Package{ID: id(), RepositoryID: d.RepositoryID, DefinitionID: d.ID, ProgramID: d.ProgramID, ProgramVersion: d.ProgramVersion, ControlID: d.ControlID, PeriodStartsAt: d.PeriodStartsAt, PeriodEndsAt: d.PeriodEndsAt, CollectedAt: now, CollectedBy: actor, Sources: sources, Coverage: covered * 100 / len(d.Queries), Gaps: gaps, Contradictions: contradictions}
	p.ManifestHash = digest(p)
	p.Attestation = "sha256:" + digest(struct {
		Manifest, Actor string
		At              time.Time
	}{p.ManifestHash, actor, now})
	return p, s.put("packages", p.ID, p)
}

func containsCredential(value string) bool {
	lower := strings.ToLower(value)
	return strings.Contains(lower, "-----begin private key-----") || strings.Contains(lower, "authorization: bearer ") || strings.Contains(lower, "ghp_") || strings.Contains(lower, "sk-")
}
func (s *Store) ListPackages(repo string) ([]Package, error) {
	var out []Package
	e := s.list("packages", func(b []byte) error {
		var p Package
		if json.Unmarshal(b, &p) != nil {
			return ErrInvalid
		}
		if p.RepositoryID == repo {
			out = append(out, p)
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].CollectedAt.After(out[j].CollectedAt) })
	return out, e
}

func validDefinition(d Definition) bool {
	if d.RepositoryID == "" || d.ProgramID == "" || d.ProgramVersion < 1 || d.ControlID == "" || d.OwnerID == "" || d.Title == "" || !d.PeriodEndsAt.After(d.PeriodStartsAt) || d.PeriodEndsAt.After(time.Now().UTC().Add(366*24*time.Hour)) || !one(d.Schedule, "manual", "daily", "weekly", "monthly", "quarterly") || len(d.Audience) == 0 || len(d.Queries) == 0 {
		return false
	}
	seen := map[string]bool{}
	for _, q := range d.Queries {
		if q.ID == "" || seen[q.ID] || !kinds[q.Kind] || q.MaxAgeHours < 1 {
			return false
		}
		seen[q.ID] = true
	}
	return true
}
func one(v string, x ...string) bool {
	for _, y := range x {
		if v == y {
			return true
		}
	}
	return false
}
func digest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func id() string                             { b := make([]byte, 12); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func (s *Store) path(kind, id string) string { return filepath.Join(s.root, kind, id+".json") }
func (s *Store) put(kind, id string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return s.lock(func() error {
		tmp := s.path(kind, id) + ".tmp-" + id
		if e := os.WriteFile(tmp, b, 0600); e != nil {
			return e
		}
		if e := os.Rename(tmp, s.path(kind, id)); e != nil {
			_ = os.Remove(tmp)
			return e
		}
		return nil
	})
}
func (s *Store) get(kind, id string, v any) error {
	if strings.ContainsAny(id, "/\\") {
		return ErrNotFound
	}
	b, e := os.ReadFile(s.path(kind, id))
	if os.IsNotExist(e) {
		return ErrNotFound
	}
	if e != nil {
		return e
	}
	if json.Unmarshal(b, v) != nil {
		return ErrInvalid
	}
	return nil
}
func (s *Store) list(kind string, fn func([]byte) error) error {
	return s.lock(func() error {
		es, e := os.ReadDir(filepath.Join(s.root, kind))
		if e != nil {
			return e
		}
		for _, x := range es {
			if x.IsDir() || !strings.HasSuffix(x.Name(), ".json") {
				continue
			}
			b, e := os.ReadFile(filepath.Join(s.root, kind, x.Name()))
			if e != nil {
				return e
			}
			if e = fn(b); e != nil {
				return e
			}
		}
		return nil
	})
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
