// Package observabilitygaps persists immutable, collaborative statements of missing runtime understanding.
package observabilitygaps

import (
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

var ErrNotFound = errors.New("observability gap not found")
var ErrInvalid = errors.New("invalid observability gap")
var ErrConflict = errors.New("observability gap conflict")

type Source struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id,omitempty"`
	Revision   string `json:"revision,omitempty"`
	Question   string `json:"question"`
	Status     string `json:"status"`
	Detail     string `json:"detail,omitempty"`
}
type Service struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Environment string `json:"environment"`
}
type Journey struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Behavior string `json:"behavior"`
}
type Evidence struct {
	ID              string    `json:"id"`
	Kind            string    `json:"kind"`
	Label           string    `json:"label"`
	SourceID        string    `json:"source_id,omitempty"`
	ReleaseID       string    `json:"release_id"`
	ReleaseRevision string    `json:"release_revision"`
	Environment     string    `json:"environment"`
	Status          string    `json:"status"`
	Semantics       string    `json:"semantics,omitempty"`
	ObservedAt      time.Time `json:"observed_at,omitempty"`
	AttributedTo    string    `json:"attributed_to,omitempty"`
}
type Criterion struct {
	ID               string `json:"id"`
	Statement        string `json:"statement"`
	RequiredEvidence string `json:"required_evidence"`
}
type Revision struct {
	RequestID          string      `json:"request_id"`
	Version            int         `json:"version"`
	Title              string      `json:"title"`
	Question           string      `json:"question"`
	Behavior           string      `json:"behavior"`
	AudienceIDs        []string    `json:"audience_ids"`
	Decision           string      `json:"decision"`
	Services           []Service   `json:"affected_services"`
	Journeys           []Journey   `json:"affected_journeys"`
	RequiredTimeliness string      `json:"required_timeliness"`
	Source             Source      `json:"source"`
	Evidence           []Evidence  `json:"current_evidence"`
	OwnerIDs           []string    `json:"owner_ids"`
	SuccessCriteria    []Criterion `json:"success_criteria"`
	CreatedBy          string      `json:"created_by"`
	CreatedAt          time.Time   `json:"created_at"`
}
type Diagnostic struct {
	Kind         string `json:"kind"`
	Severity     string `json:"severity"`
	Message      string `json:"message"`
	EvidenceID   string `json:"evidence_id,omitempty"`
	AttributedTo string `json:"attributed_to"`
}
type Gap struct {
	ID             string       `json:"id"`
	RequestID      string       `json:"request_id"`
	RequestDigest  string       `json:"request_digest"`
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
func (s *Store) Create(repo, actor, request string, r Revision) (Gap, error) {
	var out Gap
	err := s.lock(func() error {
		if strings.TrimSpace(request) == "" || validate(r) != nil {
			return ErrInvalid
		}
		digest := digest(r)
		id := stable(repo, actor, request)
		if v, e := s.read(id); e == nil {
			if v.RequestDigest != digest {
				return ErrConflict
			}
			out = v
			return nil
		} else if !errors.Is(e, ErrNotFound) {
			return e
		}
		now := s.now()
		stamp(&r, actor, request, 1, now)
		out = Gap{ID: id, RequestID: request, RequestDigest: digest, RepositoryID: repo, CurrentVersion: 1, Revisions: []Revision{r}, CreatedAt: now, UpdatedAt: now}
		return s.write(out)
	})
	return project(out, s.now()), err
}
func (s *Store) Revise(repo, id string, expected int, actor, request string, r Revision) (Gap, error) {
	var out Gap
	err := s.lock(func() error {
		v, e := s.read(id)
		if e != nil || v.RepositoryID != repo {
			if e == nil {
				return ErrNotFound
			}
			return e
		}
		d := digest(r)
		for _, x := range v.Revisions {
			if x.RequestID == request {
				if digest(x) != d {
					return ErrConflict
				}
				out = v
				return nil
			}
		}
		if v.CurrentVersion != expected {
			return ErrConflict
		}
		if strings.TrimSpace(request) == "" || validate(r) != nil {
			return ErrInvalid
		}
		now := s.now()
		stamp(&r, actor, request, expected+1, now)
		v.CurrentVersion++
		v.Revisions = append(v.Revisions, r)
		v.UpdatedAt = now
		out = v
		return s.write(v)
	})
	return project(out, s.now()), err
}
func (s *Store) Get(id string) (Gap, error) {
	var v Gap
	err := s.lock(func() error { var e error; v, e = s.read(id); return e })
	return project(v, s.now()), err
}

// WithCurrentVersion holds the gap mutation boundary while dependent state is
// persisted, so a successor cannot race an exact-version publication.
func (s *Store) WithCurrentVersion(repo, id string, version int, fn func() error) error {
	if fn == nil {
		return ErrInvalid
	}
	return s.lock(func() error {
		v, err := s.read(id)
		if err != nil || v.RepositoryID != repo {
			if err == nil {
				return ErrNotFound
			}
			return err
		}
		if v.CurrentVersion != version {
			return ErrConflict
		}
		return fn()
	})
}
func (s *Store) List(repo string) ([]Gap, error) {
	xs := []Gap{}
	err := s.lock(func() error {
		es, e := os.ReadDir(s.root)
		if e != nil {
			return e
		}
		for _, f := range es {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
				continue
			}
			v, e := s.read(strings.TrimSuffix(f.Name(), ".json"))
			if e != nil {
				return e
			}
			if v.RepositoryID == repo {
				xs = append(xs, project(v, s.now()))
			}
		}
		return nil
	})
	sort.Slice(xs, func(i, j int) bool { return xs[i].UpdatedAt.After(xs[j].UpdatedAt) })
	return xs, err
}
func validate(r Revision) error {
	allowedSource := map[string]bool{"service_objective": true, "incident": true, "debugging_workspace": true, "runbook": true, "support_thread": true, "deployment": true, "manual": true}
	statuses := map[string]bool{"current": true, "absent": true, "ambiguous": true, "inaccessible": true, "stale": true}
	if strings.TrimSpace(r.Title) == "" || strings.TrimSpace(r.Question) == "" || strings.TrimSpace(r.Behavior) == "" || strings.TrimSpace(r.Decision) == "" || strings.TrimSpace(r.RequiredTimeliness) == "" || !allowedSource[r.Source.Kind] || strings.TrimSpace(r.Source.Question) == "" || !statuses[r.Source.Status] || len(r.AudienceIDs) == 0 || len(r.Services) == 0 || len(r.OwnerIDs) == 0 || len(r.SuccessCriteria) == 0 {
		return ErrInvalid
	}
	if r.Source.Kind != "manual" && (strings.TrimSpace(r.Source.ResourceID) == "" || strings.TrimSpace(r.Source.Revision) == "") {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, e := range r.Evidence {
		if e.ID == "" || seen[e.ID] || !map[string]bool{"metric": true, "log": true, "trace": true, "profile": true, "event": true}[e.Kind] || e.ReleaseID == "" || e.ReleaseRevision == "" || e.Environment == "" || !statuses[e.Status] {
			return ErrInvalid
		}
		seen[e.ID] = true
	}
	for _, c := range r.SuccessCriteria {
		if c.ID == "" || c.Statement == "" || c.RequiredEvidence == "" {
			return ErrInvalid
		}
	}
	return nil
}
func stamp(r *Revision, actor, request string, version int, now time.Time) {
	r.RequestID = request
	r.Version = version
	r.CreatedBy = actor
	r.CreatedAt = now
	for i := range r.Evidence {
		r.Evidence[i].AttributedTo = actor
	}
}
func project(v Gap, now time.Time) Gap {
	if len(v.Revisions) == 0 {
		return v
	}
	r := v.Revisions[len(v.Revisions)-1]
	ds := []Diagnostic{}
	add := func(k, m, id, who string) {
		ds = append(ds, Diagnostic{Kind: k, Severity: "warning", Message: m, EvidenceID: id, AttributedTo: who})
	}
	if r.Source.Status != "current" {
		add("source_"+r.Source.Status, "The originating operational question is "+r.Source.Status+".", "", r.CreatedBy)
	}
	covered := map[string]bool{}
	for _, e := range r.Evidence {
		covered[e.Kind] = true
		if e.Status != "current" {
			add(e.Status+"_instrumentation", e.Label+" is "+e.Status+".", e.ID, e.AttributedTo)
		}
		if e.Semantics == "" {
			add("ambiguous_semantics", e.Label+" has no declared semantics.", e.ID, e.AttributedTo)
		}
		if !e.ObservedAt.IsZero() && now.Sub(e.ObservedAt) > 30*24*time.Hour {
			add("stale_instrumentation", e.Label+" was last observed more than 30 days ago.", e.ID, e.AttributedTo)
		}
	}
	for _, k := range []string{"metric", "log", "trace", "profile", "event"} {
		if !covered[k] {
			add("absent_coverage", "No "+k+" evidence is linked.", "", r.CreatedBy)
		}
	}
	v.Diagnostics = ds
	return v
}
func digest(r Revision) string {
	r.RequestID = ""
	r.Version = 0
	r.CreatedBy = ""
	r.CreatedAt = time.Time{}
	for i := range r.Evidence {
		r.Evidence[i].AttributedTo = ""
	}
	b, _ := json.Marshal(r)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func stable(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "og_" + hex.EncodeToString(h[:12])
}
func (s *Store) path(id string) string { return filepath.Join(s.root, id+".json") }
func (s *Store) read(id string) (Gap, error) {
	var v Gap
	b, e := os.ReadFile(s.path(id))
	if os.IsNotExist(e) {
		return v, ErrNotFound
	}
	if e != nil {
		return v, e
	}
	e = json.Unmarshal(b, &v)
	return v, e
}
func (s *Store) write(v Gap) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(s.root, ".gap-*")
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
	if ce := tmp.Close(); e == nil {
		e = ce
	}
	if e == nil {
		e = os.Rename(name, s.path(v.ID))
	}
	if e == nil {
		if d, x := os.Open(s.root); x == nil {
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
