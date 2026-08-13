// Package productexperiments persists versioned product-learning contracts.
package productexperiments

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

var ErrNotFound = errors.New("product experiment not found")
var ErrInvalid = errors.New("invalid product experiment")
var ErrConflict = errors.New("product experiment conflict")

type Source struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Label      string `json:"label"`
}
type Variant struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Control     bool   `json:"control"`
}
type Audience struct {
	Description string   `json:"description"`
	Eligibility []string `json:"eligibility"`
	Exclusions  []string `json:"exclusions"`
}
type Signal struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Version  int    `json:"version"`
	Event    string `json:"event"`
	Property string `json:"property,omitempty"`
	Unit     string `json:"unit"`
	Privacy  string `json:"privacy"`
	Status   string `json:"status"`
}
type Metric struct {
	Name          string  `json:"name"`
	Kind          string  `json:"kind"`
	Direction     string  `json:"direction"`
	Threshold     float64 `json:"threshold"`
	SignalID      string  `json:"signal_id"`
	SignalVersion int     `json:"signal_version"`
}
type Revision struct {
	Version         int       `json:"version"`
	Hypothesis      string    `json:"hypothesis"`
	Variants        []Variant `json:"variants"`
	Audience        Audience  `json:"target_audience"`
	Metrics         []Metric  `json:"metrics"`
	MinimumEvidence int       `json:"minimum_evidence"`
	DurationDays    int       `json:"duration_days"`
	Owners          []string  `json:"owners"`
	StopConditions  []string  `json:"stop_conditions"`
	Assumptions     []string  `json:"assumptions"`
	Rationale       string    `json:"rationale"`
	CreatedBy       string    `json:"created_by"`
	CreatedAt       time.Time `json:"created_at"`
}
type Comment struct {
	ID        string    `json:"id"`
	Body      string    `json:"body"`
	AuthorID  string    `json:"author_id"`
	CreatedAt time.Time `json:"created_at"`
}
type Approval struct {
	UserID    string    `json:"user_id"`
	Version   int       `json:"version"`
	Decision  string    `json:"decision"`
	Note      string    `json:"note,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
type Diagnostic struct {
	Kind                string `json:"kind"`
	Severity            string `json:"severity"`
	Message             string `json:"message"`
	AttributedTo        string `json:"attributed_to"`
	RelatedExperimentID string `json:"related_experiment_id,omitempty"`
}
type Experiment struct {
	ID             string       `json:"id"`
	RepositoryID   string       `json:"repository_id"`
	Source         Source       `json:"source"`
	CurrentVersion int          `json:"current_version"`
	Revisions      []Revision   `json:"revisions"`
	Signals        []Signal     `json:"signals"`
	Comments       []Comment    `json:"comments"`
	Approvals      []Approval   `json:"approvals"`
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
func (s *Store) Create(repo, actor string, source Source, revision Revision, signals []Signal) (Experiment, error) {
	var out Experiment
	err := s.lock(func() error {
		if !validSource(source) || !validRevision(revision, signals) {
			return ErrInvalid
		}
		now := s.now()
		revision.Version = 1
		revision.CreatedBy = actor
		revision.CreatedAt = now
		out = Experiment{ID: id(), RepositoryID: repo, Source: source, CurrentVersion: 1, Revisions: []Revision{revision}, Signals: signals, CreatedAt: now, UpdatedAt: now}
		return s.write(out)
	})
	if err != nil {
		return Experiment{}, err
	}
	return s.project(out), nil
}
func (s *Store) Revise(id string, expected int, actor string, revision Revision, signals []Signal) (Experiment, error) {
	var out Experiment
	err := s.lock(func() error {
		v, e := s.read(id)
		if e != nil {
			return e
		}
		if v.CurrentVersion != expected {
			return ErrConflict
		}
		if !validRevision(revision, signals) {
			return ErrInvalid
		}
		revision.Version = expected + 1
		revision.CreatedBy = actor
		revision.CreatedAt = s.now()
		v.CurrentVersion++
		v.Revisions = append(v.Revisions, revision)
		v.Signals = signals
		v.UpdatedAt = revision.CreatedAt
		out = v
		return s.write(v)
	})
	if err != nil {
		return Experiment{}, err
	}
	return s.project(out), nil
}
func (s *Store) Comment(id, actor, body string) (Experiment, error) {
	return s.mutate(id, func(v *Experiment) error {
		if strings.TrimSpace(body) == "" || len(body) > 4000 {
			return ErrInvalid
		}
		v.Comments = append(v.Comments, Comment{ID: idgen(), Body: strings.TrimSpace(body), AuthorID: actor, CreatedAt: s.now()})
		return nil
	})
}
func (s *Store) Approve(id, actor, decision, note string, expected int) (Experiment, error) {
	return s.mutate(id, func(v *Experiment) error {
		if expected != v.CurrentVersion {
			return ErrConflict
		}
		if decision != "approve" && decision != "request_changes" {
			return ErrInvalid
		}
		next := []Approval{}
		for _, a := range v.Approvals {
			if a.UserID != actor {
				next = append(next, a)
			}
		}
		v.Approvals = append(next, Approval{UserID: actor, Version: expected, Decision: decision, Note: strings.TrimSpace(note), CreatedAt: s.now()})
		return nil
	})
}
func (s *Store) mutate(key string, f func(*Experiment) error) (Experiment, error) {
	var out Experiment
	err := s.lock(func() error {
		v, e := s.read(key)
		if e != nil {
			return e
		}
		if e = f(&v); e != nil {
			return e
		}
		v.UpdatedAt = s.now()
		out = v
		return s.write(v)
	})
	if err != nil {
		return Experiment{}, err
	}
	return s.project(out), nil
}
func (s *Store) Get(key string) (Experiment, error) {
	var out Experiment
	err := s.lock(func() error { var e error; out, e = s.read(key); return e })
	if err != nil {
		return Experiment{}, err
	}
	return s.project(out), nil
}
func (s *Store) List(repo string) ([]Experiment, error) {
	out := []Experiment{}
	err := s.lock(func() error {
		entries, e := os.ReadDir(s.root)
		if e != nil {
			return e
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			v, e := s.read(strings.TrimSuffix(entry.Name(), ".json"))
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
func (s *Store) project(v Experiment) Experiment {
	v.Diagnostics = nil
	if len(v.Revisions) == 0 {
		return v
	}
	r := v.Revisions[len(v.Revisions)-1]
	signals := map[string]Signal{}
	for _, x := range v.Signals {
		signals[x.ID] = x
	}
	for _, m := range r.Metrics {
		x, ok := signals[m.SignalID]
		if !ok || x.Version != m.SignalVersion || x.Status != "available" {
			v.Diagnostics = append(v.Diagnostics, Diagnostic{"missing_instrumentation", "blocking", "Metric " + m.Name + " is not connected to an available signal at the declared version.", r.CreatedBy, ""})
		}
	}
	if len(r.Audience.Eligibility) == 0 {
		v.Diagnostics = append(v.Diagnostics, Diagnostic{"ineligible_audience", "blocking", "The target audience has no permitted eligibility rule.", r.CreatedBy, ""})
	}
	for _, a := range v.Approvals {
		if a.Version != v.CurrentVersion {
			v.Diagnostics = append(v.Diagnostics, Diagnostic{"changed_assumptions", "warning", "A prior approval no longer applies to the current plan version.", a.UserID, ""})
		}
	}
	return v
}
func Overlaps(a, b Experiment) bool {
	if len(a.Revisions) == 0 || len(b.Revisions) == 0 {
		return false
	}
	x, y := a.Revisions[len(a.Revisions)-1], b.Revisions[len(b.Revisions)-1]
	for _, xe := range x.Audience.Eligibility {
		for _, ye := range y.Audience.Eligibility {
			if xe == ye {
				for _, xm := range x.Metrics {
					for _, ym := range y.Metrics {
						if xm.SignalID == ym.SignalID {
							return true
						}
					}
				}
			}
		}
	}
	return false
}
func AddOverlap(v *Experiment, other Experiment) {
	v.Diagnostics = append(v.Diagnostics, Diagnostic{"overlapping_experiment", "warning", "This plan shares an audience and product signal with another experiment.", other.Revisions[len(other.Revisions)-1].CreatedBy, other.ID})
}
func validSource(v Source) bool {
	ok := map[string]bool{"proposal": true, "issue": true, "decision": true, "pull_request": true, "preview": true, "release": true}
	return ok[v.Kind] && strings.TrimSpace(v.ResourceID) != "" && strings.TrimSpace(v.Label) != ""
}
func validRevision(r Revision, signals []Signal) bool {
	if strings.TrimSpace(r.Hypothesis) == "" || len(r.Variants) < 2 || len(r.Metrics) == 0 || r.MinimumEvidence < 1 || r.DurationDays < 1 || len(r.Owners) == 0 || len(r.StopConditions) == 0 {
		return false
	}
	seen := map[string]bool{}
	for _, v := range r.Variants {
		if v.Key == "" || v.Name == "" || seen[v.Key] {
			return false
		}
		seen[v.Key] = true
	}
	for _, s := range signals {
		if s.ID == "" || s.Name == "" || s.Version < 1 || s.Event == "" || (s.Status != "available" && s.Status != "planned" && s.Status != "retired") || (s.Privacy != "aggregate" && s.Privacy != "pseudonymous" && s.Privacy != "consented") {
			return false
		}
	}
	for _, m := range r.Metrics {
		if m.Name == "" || (m.Kind != "success" && m.Kind != "guardrail") || m.SignalID == "" || m.SignalVersion < 1 {
			return false
		}
	}
	return true
}
func (s *Store) lock(fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	return fn()
}
func (s *Store) read(key string) (Experiment, error) {
	b, e := os.ReadFile(filepath.Join(s.root, key+".json"))
	if os.IsNotExist(e) {
		return Experiment{}, ErrNotFound
	}
	if e != nil {
		return Experiment{}, e
	}
	var v Experiment
	if json.Unmarshal(b, &v) != nil {
		return Experiment{}, ErrInvalid
	}
	return v, nil
}
func (s *Store) write(v Experiment) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(s.root, ".experiment-")
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
	if closeErr := tmp.Close(); e == nil {
		e = closeErr
	}
	if e != nil {
		return e
	}
	return os.Rename(name, filepath.Join(s.root, v.ID+".json"))
}
func id() string    { return "experiment-" + idgen() }
func idgen() string { b := make([]byte, 12); _, _ = rand.Read(b); return hex.EncodeToString(b) }
