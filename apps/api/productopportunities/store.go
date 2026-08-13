// Package productopportunities retains transparent, evidence-backed need syntheses.
package productopportunities

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

var ErrNotFound = errors.New("product opportunity not found")
var ErrInvalid = errors.New("invalid product opportunity")
var ErrConflict = errors.New("product opportunity changed")

type Source struct {
	Kind         string     `json:"kind"`
	ResourceID   string     `json:"resource_id"`
	ParentID     string     `json:"parent_id,omitempty"`
	Revision     string     `json:"revision"`
	Label        string     `json:"label"`
	Claim        string     `json:"claim"`
	Relationship string     `json:"relationship"`
	Audience     string     `json:"audience"`
	Stale        bool       `json:"stale"`
	StaleReason  string     `json:"stale_reason,omitempty"`
	DetachedAt   *time.Time `json:"detached_at,omitempty"`
	DetachedBy   string     `json:"detached_by,omitempty"`
}
type Revision struct {
	Version           int       `json:"version"`
	Title             string    `json:"title"`
	Need              string    `json:"need"`
	AffectedAudiences []string  `json:"affected_audiences"`
	Severity          string    `json:"severity"`
	Reach             string    `json:"reach"`
	Confidence        string    `json:"confidence"`
	ExpectedValue     string    `json:"expected_value"`
	Uncertainty       []string  `json:"uncertainty"`
	MinorityNeeds     []string  `json:"minority_needs"`
	Contradictions    []string  `json:"contradictions"`
	Sources           []Source  `json:"sources"`
	CreatedBy         string    `json:"created_by"`
	ActorType         string    `json:"actor_type"`
	CreatedAt         time.Time `json:"created_at"`
}
type Entry struct {
	ID             string       `json:"id"`
	RepositoryID   string       `json:"repository_id"`
	Version        int          `json:"version"`
	CurrentVersion int          `json:"current_version"`
	Revisions      []Revision   `json:"revisions"`
	DuplicateOf    string       `json:"duplicate_of,omitempty"`
	Challenges     []Challenge  `json:"challenges"`
	Corrections    []Correction `json:"corrections"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
}
type Challenge struct {
	ID        string    `json:"id"`
	Body      string    `json:"body"`
	SourceIDs []string  `json:"source_ids"`
	ActorID   string    `json:"actor_id"`
	CreatedAt time.Time `json:"created_at"`
}
type Correction struct {
	ID        string    `json:"id"`
	Field     string    `json:"field"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	Reason    string    `json:"reason"`
	ActorID   string    `json:"actor_id"`
	CreatedAt time.Time `json:"created_at"`
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
func id() string                { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func text(v string, n int) bool { l := len(strings.TrimSpace(v)); return l > 0 && l <= n }
func validRevision(v Revision) bool {
	if !text(v.Title, 300) || !text(v.Need, 10000) || len(v.AffectedAudiences) == 0 || len(v.AffectedAudiences) > 20 || !one(v.Severity, "low", "medium", "high", "critical") || !one(v.Reach, "individual", "segment", "broad", "unknown") || !one(v.Confidence, "low", "medium", "high", "uncertain") || !text(v.ExpectedValue, 5000) || len(v.Sources) == 0 || len(v.Sources) > 100 {
		return false
	}
	for _, s := range v.Sources {
		if !one(s.Kind, "feedback", "issue", "preview_finding", "support_signal", "usage_evidence", "experiment_outcome") || !text(s.ResourceID, 200) || !text(s.Revision, 200) || !text(s.Label, 300) || !text(s.Claim, 5000) || !one(s.Relationship, "supports", "contradicts", "minority_need", "duplicate") || !text(s.Audience, 300) {
			return false
		}
	}
	return true
}
func one(v string, xs ...string) bool {
	for _, x := range xs {
		if v == x {
			return true
		}
	}
	return false
}
func (s *Store) lock() (*os.File, error) {
	f, e := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e == nil {
		e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
	}
	return f, e
}
func (s *Store) read(repo, key string) (Entry, error) {
	b, e := os.ReadFile(filepath.Join(s.root, repo+"-"+key+".json"))
	if errors.Is(e, os.ErrNotExist) {
		return Entry{}, ErrNotFound
	}
	var v Entry
	if e == nil {
		e = json.Unmarshal(b, &v)
	}
	return v, e
}
func (s *Store) write(v Entry) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(s.root, ".opportunity-")
	if e != nil {
		return e
	}
	n := tmp.Name()
	defer os.Remove(n)
	_ = tmp.Chmod(0600)
	if _, e = tmp.Write(b); e == nil {
		e = tmp.Sync()
	}
	ce := tmp.Close()
	if e == nil {
		e = ce
	}
	if e == nil {
		e = os.Rename(n, filepath.Join(s.root, v.RepositoryID+"-"+v.ID+".json"))
	}
	return e
}
func (s *Store) Create(repo, actor, actorType string, r Revision) (Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !text(repo, 200) || !text(actor, 200) || !one(actorType, "human", "agent") || !validRevision(r) {
		return Entry{}, ErrInvalid
	}
	f, e := s.lock()
	if e != nil {
		return Entry{}, e
	}
	defer f.Close()
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	now := s.now().UTC()
	r.Version, r.CreatedBy, r.ActorType, r.CreatedAt = 1, actor, actorType, now
	v := Entry{ID: id(), RepositoryID: repo, Version: 1, CurrentVersion: 1, Revisions: []Revision{r}, Challenges: []Challenge{}, Corrections: []Correction{}, CreatedAt: now, UpdatedAt: now}
	e = s.write(v)
	return v, e
}
func (s *Store) Mutate(repo, key string, expected int, fn func(*Entry) error) (Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, e := s.lock()
	if e != nil {
		return Entry{}, e
	}
	defer f.Close()
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	v, e := s.read(repo, key)
	if e != nil {
		return Entry{}, e
	}
	if v.Version != expected {
		return Entry{}, ErrConflict
	}
	if e = fn(&v); e != nil {
		return Entry{}, e
	}
	v.Version++
	v.UpdatedAt = s.now().UTC()
	e = s.write(v)
	return v, e
}
func (s *Store) Revise(repo, key, actor string, expected int, r Revision) (Entry, error) {
	if !validRevision(r) {
		return Entry{}, ErrInvalid
	}
	return s.Mutate(repo, key, expected, func(v *Entry) error {
		r.Version, r.CreatedBy, r.ActorType, r.CreatedAt = v.CurrentVersion+1, actor, "human", s.now().UTC()
		v.CurrentVersion++
		v.Revisions = append(v.Revisions, r)
		return nil
	})
}
func (s *Store) Challenge(repo, key, actor string, expected int, c Challenge) (Entry, error) {
	if !text(c.Body, 5000) || len(c.SourceIDs) > 100 {
		return Entry{}, ErrInvalid
	}
	return s.Mutate(repo, key, expected, func(v *Entry) error {
		c.ID, c.ActorID, c.CreatedAt = id(), actor, s.now().UTC()
		v.Challenges = append(v.Challenges, c)
		return nil
	})
}
func (s *Store) Correct(repo, key, actor string, expected int, c Correction) (Entry, error) {
	if !one(c.Field, "severity", "reach", "confidence", "duplicate_of") || !text(c.To, 300) || !text(c.Reason, 5000) {
		return Entry{}, ErrInvalid
	}
	switch c.Field {
	case "severity":
		if !one(c.To, "low", "medium", "high", "critical") {
			return Entry{}, ErrInvalid
		}
	case "reach":
		if !one(c.To, "individual", "segment", "broad", "unknown") {
			return Entry{}, ErrInvalid
		}
	case "confidence":
		if !one(c.To, "low", "medium", "high", "uncertain") {
			return Entry{}, ErrInvalid
		}
	}
	return s.Mutate(repo, key, expected, func(v *Entry) error {
		latest := &v.Revisions[len(v.Revisions)-1]
		switch c.Field {
		case "severity":
			c.From = latest.Severity
			latest.Severity = c.To
		case "reach":
			c.From = latest.Reach
			latest.Reach = c.To
		case "confidence":
			c.From = latest.Confidence
			latest.Confidence = c.To
		case "duplicate_of":
			c.From = v.DuplicateOf
			v.DuplicateOf = c.To
		}
		c.ID, c.ActorID, c.CreatedAt = id(), actor, s.now().UTC()
		v.Corrections = append(v.Corrections, c)
		return nil
	})
}
func (s *Store) DetachFeedback(repo, key, feedbackID, actor string, expected int) (Entry, error) {
	return s.Mutate(repo, key, expected, func(v *Entry) error {
		found := false
		now := s.now().UTC()
		for ri := range v.Revisions {
			for si := range v.Revisions[ri].Sources {
				s := &v.Revisions[ri].Sources[si]
				if s.Kind == "feedback" && s.ResourceID == feedbackID && s.DetachedAt == nil {
					s.DetachedAt = &now
					s.DetachedBy = actor
					found = true
				}
			}
		}
		if !found {
			return ErrNotFound
		}
		return nil
	})
}
func (s *Store) Get(repo, key string) (Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read(repo, key)
}
func (s *Store) List(repo string) ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	es, e := os.ReadDir(s.root)
	if e != nil {
		return nil, e
	}
	out := []Entry{}
	for _, x := range es {
		if x.IsDir() || !strings.HasPrefix(x.Name(), repo+"-") || filepath.Ext(x.Name()) != ".json" {
			continue
		}
		v, e := s.read(repo, strings.TrimSuffix(strings.TrimPrefix(x.Name(), repo+"-"), ".json"))
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}
