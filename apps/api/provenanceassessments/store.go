// Package provenanceassessments retains candidate-exact licensing and origin decisions.
package provenanceassessments

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

var ErrNotFound = errors.New("provenance assessment not found")
var ErrInvalid = errors.New("invalid provenance assessment")
var ErrConflict = errors.New("provenance assessment version conflict")
var ErrForbidden = errors.New("provenance assessment action forbidden")

type Candidate struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Revision string `json:"revision"`
}
type Finding struct {
	ID                  string   `json:"id"`
	Kind                string   `json:"kind"`
	Severity            string   `json:"severity"`
	MaterialKind        string   `json:"material_kind"`
	NodeID              string   `json:"node_id,omitempty"`
	Summary             string   `json:"summary"`
	License             string   `json:"license,omitempty"`
	Origin              string   `json:"origin,omitempty"`
	Obligations         []string `json:"obligations,omitempty"`
	DistributionTargets []string `json:"distribution_targets,omitempty"`
	OwnerIDs            []string `json:"owner_ids,omitempty"`
	DependencyRevision  string   `json:"dependency_revision,omitempty"`
	ToolRevision        string   `json:"tool_revision,omitempty"`
	PolicyRuleDigest    string   `json:"policy_rule_digest,omitempty"`
	Current             bool     `json:"current"`
	Resolved            bool     `json:"resolved"`
}
type Citation struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Revision   string `json:"revision"`
	Summary    string `json:"summary"`
}
type Event struct {
	ID                 string     `json:"id"`
	RequestID          string     `json:"request_id"`
	Kind               string     `json:"kind"`
	FindingID          string     `json:"finding_id"`
	Body               string     `json:"body"`
	Citations          []Citation `json:"citations"`
	ExceptionExpiresAt *time.Time `json:"exception_expires_at,omitempty"`
	FollowUp           string     `json:"follow_up,omitempty"`
	ActorID            string     `json:"actor_id"`
	ActorType          string     `json:"actor_type"`
	CreatedAt          time.Time  `json:"created_at"`
}
type Assessment struct {
	ID            string    `json:"id"`
	RequestID     string    `json:"request_id"`
	RepositoryID  string    `json:"repository_id"`
	Candidate     Candidate `json:"candidate"`
	GraphID       string    `json:"graph_id"`
	GraphDigest   string    `json:"graph_digest"`
	PolicyID      string    `json:"policy_id"`
	PolicyVersion int       `json:"policy_version"`
	Version       int       `json:"version"`
	Findings      []Finding `json:"findings"`
	Events        []Event   `json:"events"`
	Ready         bool      `json:"ready"`
	Stale         bool      `json:"stale"`
	CreatedBy     string    `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	Authority     string    `json:"authority"`
}
type Current struct {
	CandidateRevision, GraphDigest                        string
	PolicyVersion                                         int
	DependencyRevisions, ToolRevisions, PolicyRuleDigests map[string]string
	OwnerIDs                                              map[string]bool
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
func (s *Store) Create(a Assessment) (Assessment, error) {
	var out Assessment
	e := s.lock(func() error {
		if !validAssessment(a) {
			return ErrInvalid
		}
		xs, x := s.listRaw(a.RepositoryID)
		if x != nil {
			return x
		}
		for _, v := range xs {
			if v.RequestID == a.RequestID {
				if sameCreate(v, a) {
					out = v
					return nil
				}
				return ErrConflict
			}
		}
		now := s.now()
		a.ID = newID()
		a.Version = 1
		a.CreatedAt = now
		a.UpdatedAt = now
		a.Authority = "Provenance assessment records supply evidence and readiness only; they grant no Git, review, merge, package, release, disclosure, exception, or distribution authority."
		out = a
		return s.write(a)
	})
	return project(out, Current{CandidateRevision: out.Candidate.Revision, GraphDigest: out.GraphDigest, PolicyVersion: out.PolicyVersion}), e
}
func (s *Store) AddEvent(repo, id, actor, actorType string, expected int, ev Event, current Current) (Assessment, error) {
	var out Assessment
	e := s.lock(func() error {
		a, x := s.read(id)
		if x != nil {
			return x
		}
		if a.RepositoryID != repo {
			return ErrNotFound
		}
		if a.Version != expected {
			return ErrConflict
		}
		if actor == "" || !one(actorType, "human", "agent") || !one(ev.Kind, "challenge", "origin_evidence", "acknowledgement", "exception") || ev.RequestID == "" || ev.FindingID == "" || strings.TrimSpace(ev.Body) == "" || len(ev.Body) > 4000 {
			return ErrInvalid
		}
		found := false
		for _, f := range a.Findings {
			if f.ID == ev.FindingID {
				found = true
				if (ev.Kind == "acknowledgement" || ev.Kind == "exception") && (!current.OwnerIDs[actor] || actorType != "human") {
					return ErrForbidden
				}
			}
		}
		if !found {
			return ErrInvalid
		}
		if ev.Kind == "origin_evidence" && len(ev.Citations) == 0 {
			return ErrInvalid
		}
		for _, c := range ev.Citations {
			if c.Kind == "" || c.ResourceID == "" || c.Revision == "" || c.Summary == "" {
				return ErrInvalid
			}
		}
		if ev.Kind == "exception" {
			if ev.ExceptionExpiresAt == nil || ev.ExceptionExpiresAt.After(s.now().Add(30*24*time.Hour)) || !ev.ExceptionExpiresAt.After(s.now()) || strings.TrimSpace(ev.FollowUp) == "" {
				return ErrInvalid
			}
		}
		for _, prior := range a.Events {
			if prior.RequestID == ev.RequestID {
				if eventEqual(prior, ev, actor, actorType) {
					out = a
					return nil
				}
				return ErrConflict
			}
		}
		ev.ID = newID()
		ev.ActorID = actor
		ev.ActorType = actorType
		ev.CreatedAt = s.now()
		a.Events = append(a.Events, ev)
		a.Version++
		a.UpdatedAt = ev.CreatedAt
		out = a
		return s.write(a)
	})
	return project(out, current), e
}
func (s *Store) Get(repo, id string, current Current) (Assessment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, e := s.read(id)
	if e == nil && a.RepositoryID != repo {
		e = ErrNotFound
	}
	return project(a, current), e
}
func (s *Store) List(repo string, current func(Assessment) Current) ([]Assessment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	xs, e := s.listRaw(repo)
	if e != nil {
		return nil, e
	}
	for i := range xs {
		xs[i] = project(xs[i], current(xs[i]))
	}
	sort.Slice(xs, func(i, j int) bool { return xs[i].UpdatedAt.After(xs[j].UpdatedAt) })
	return xs, nil
}

func project(a Assessment, c Current) Assessment {
	a.Ready = true
	candidateStale := c.CandidateRevision == "" || c.CandidateRevision != a.Candidate.Revision || c.GraphDigest != a.GraphDigest
	a.Stale = candidateStale
	if candidateStale {
		a.Ready = false
	}
	for i := range a.Findings {
		f := &a.Findings[i]
		f.Current = !candidateStale && (f.DependencyRevision == "" || c.DependencyRevisions[f.NodeID] == f.DependencyRevision) && (f.ToolRevision == "" || c.ToolRevisions[f.NodeID] == f.ToolRevision) && (f.PolicyRuleDigest == "" || c.PolicyRuleDigests[f.MaterialKind] == f.PolicyRuleDigest)
		if !f.Current {
			a.Stale = true
		}
		f.Resolved = false
		for _, e := range a.Events {
			if e.FindingID != f.ID || !f.Current {
				continue
			}
			if e.Kind == "acknowledgement" && c.OwnerIDs[e.ActorID] {
				f.Resolved = true
			}
			if e.Kind == "exception" && c.OwnerIDs[e.ActorID] && e.ExceptionExpiresAt != nil && e.ExceptionExpiresAt.After(time.Now().UTC()) {
				f.Resolved = true
			}
		}
		if f.Severity == "blocking" && (!f.Current || !f.Resolved) {
			a.Ready = false
		}
	}
	return a
}
func validAssessment(a Assessment) bool {
	if a.RequestID == "" || a.RepositoryID == "" || !one(a.Candidate.Kind, "pull_request", "change_stack", "package_candidate", "release_candidate") || a.Candidate.ID == "" || len(a.Candidate.Revision) != 40 || a.GraphID == "" || a.GraphDigest == "" || a.PolicyID == "" || a.PolicyVersion < 1 || a.CreatedBy == "" {
		return false
	}
	seen := map[string]bool{}
	for _, f := range a.Findings {
		if f.ID == "" || seen[f.ID] || !one(f.Severity, "blocking", "warning", "notice") || f.Kind == "" || f.MaterialKind == "" || f.Summary == "" {
			return false
		}
		seen[f.ID] = true
	}
	return true
}
func sameCreate(v, a Assessment) bool {
	a.ID = v.ID
	a.Version = v.Version
	a.CreatedAt = v.CreatedAt
	a.UpdatedAt = v.UpdatedAt
	a.Authority = v.Authority
	b, _ := json.Marshal(a)
	c, _ := json.Marshal(v)
	return string(b) == string(c)
}
func eventEqual(v, e Event, actor, actorType string) bool {
	e.ID = v.ID
	e.ActorID = actor
	e.ActorType = actorType
	e.CreatedAt = v.CreatedAt
	b, _ := json.Marshal(e)
	c, _ := json.Marshal(v)
	return string(b) == string(c)
}
func one(v string, xs ...string) bool {
	for _, x := range xs {
		if v == x {
			return true
		}
	}
	return false
}
func newID() string                    { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func (s *Store) path(id string) string { return filepath.Join(s.root, id+".json") }
func (s *Store) read(id string) (Assessment, error) {
	var a Assessment
	b, e := os.ReadFile(s.path(id))
	if os.IsNotExist(e) {
		return a, ErrNotFound
	}
	if e != nil {
		return a, e
	}
	if json.Unmarshal(b, &a) != nil {
		return a, ErrInvalid
	}
	return a, nil
}
func (s *Store) listRaw(repo string) ([]Assessment, error) {
	es, e := os.ReadDir(s.root)
	if e != nil {
		return nil, e
	}
	out := []Assessment{}
	for _, x := range es {
		if x.IsDir() || !strings.HasSuffix(x.Name(), ".json") {
			continue
		}
		a, z := s.read(strings.TrimSuffix(x.Name(), ".json"))
		if z != nil {
			return nil, z
		}
		if a.RepositoryID == repo {
			out = append(out, a)
		}
	}
	return out, nil
}
func (s *Store) write(a Assessment) error {
	b, e := json.MarshalIndent(a, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(s.root, ".assessment-")
	if e != nil {
		return e
	}
	n := f.Name()
	defer os.Remove(n)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	if x := f.Close(); e == nil {
		e = x
	}
	if e == nil {
		e = os.Rename(n, s.path(a.ID))
	}
	if e == nil {
		d, x := os.Open(s.root)
		if x == nil {
			e = d.Sync()
			_ = d.Close()
		} else {
			e = x
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
