// Package performancegoals persists versioned, collaborative performance contracts.
package performancegoals

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

var ErrNotFound = errors.New("performance goal not found")
var ErrInvalid = errors.New("invalid performance goal")
var ErrConflict = errors.New("performance goal version conflict")

type Subject struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id,omitempty"`
	Name       string `json:"name"`
}
type Workload struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Inputs      string `json:"inputs"`
	Warmup      int    `json:"warmup"`
	Samples     int    `json:"samples"`
}
type Environment struct {
	Name         string `json:"name"`
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
	Runtime      string `json:"runtime"`
	Hardware     string `json:"hardware"`
}
type Baseline struct {
	Value       *float64   `json:"value,omitempty"`
	Environment string     `json:"environment"`
	MeasuredAt  *time.Time `json:"measured_at,omitempty"`
	Source      string     `json:"source,omitempty"`
}
type Metric struct {
	Name      string   `json:"name"`
	Unit      string   `json:"unit"`
	Direction string   `json:"direction"`
	Minimum   *float64 `json:"minimum,omitempty"`
	Maximum   *float64 `json:"maximum,omitempty"`
	Baseline  Baseline `json:"baseline"`
}
type Constraint struct {
	Name         string `json:"name"`
	Requirement  string `json:"requirement"`
	Verification string `json:"verification"`
}
type Budget struct {
	Kind  string  `json:"kind"`
	Limit float64 `json:"limit"`
	Unit  string  `json:"unit"`
}
type Link struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Label      string `json:"label"`
	AddedBy    string `json:"added_by,omitempty"`
}
type Diagnostic struct {
	Kind         string `json:"kind"`
	Severity     string `json:"severity"`
	Message      string `json:"message"`
	Metric       string `json:"metric,omitempty"`
	AttributedTo string `json:"attributed_to"`
}
type Revision struct {
	Version            int           `json:"version"`
	Title              string        `json:"title"`
	Summary            string        `json:"summary"`
	Subject            Subject       `json:"subject"`
	Workloads          []Workload    `json:"workloads"`
	Metrics            []Metric      `json:"metrics"`
	Constraints        []Constraint  `json:"correctness_constraints"`
	Environments       []Environment `json:"supported_environments"`
	Owners             []string      `json:"owners"`
	Budgets            []Budget      `json:"budgets"`
	Links              []Link        `json:"links"`
	BaselineMaxAgeDays int           `json:"baseline_max_age_days"`
	CreatedBy          string        `json:"created_by"`
	CreatedAt          time.Time     `json:"created_at"`
	Rationale          string        `json:"rationale"`
}
type Goal struct {
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
func (s *Store) Create(repositoryID, actor string, revision Revision) (Goal, error) {
	var result Goal
	err := s.lock(func() error {
		if err := validate(revision); err != nil {
			return err
		}
		now := s.now()
		revision.Version = 1
		revision.CreatedBy = actor
		revision.CreatedAt = now
		stampLinks(revision.Links, actor)
		result = Goal{ID: randomID(), RepositoryID: repositoryID, CurrentVersion: 1, Revisions: []Revision{revision}, CreatedAt: now, UpdatedAt: now}
		return s.write(result)
	})
	return s.project(result), err
}
func (s *Store) Revise(id string, expected int, actor string, revision Revision) (Goal, error) {
	var result Goal
	err := s.lock(func() error {
		current, err := s.read(id)
		if err != nil {
			return err
		}
		if current.CurrentVersion != expected {
			return ErrConflict
		}
		if err = validate(revision); err != nil {
			return err
		}
		revision.Version = expected + 1
		revision.CreatedBy = actor
		revision.CreatedAt = s.now()
		stampLinks(revision.Links, actor)
		current.CurrentVersion = revision.Version
		current.Revisions = append(current.Revisions, revision)
		current.UpdatedAt = revision.CreatedAt
		result = current
		return s.write(current)
	})
	return s.project(result), err
}
func (s *Store) Get(id string) (Goal, error) {
	var v Goal
	err := s.lock(func() error { var e error; v, e = s.read(id); return e })
	return s.project(v), err
}
func (s *Store) List(repositoryID string) ([]Goal, error) {
	values := []Goal{}
	err := s.lock(func() error {
		entries, err := os.ReadDir(s.root)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			v, e := s.read(strings.TrimSuffix(entry.Name(), ".json"))
			if e != nil {
				return e
			}
			if v.RepositoryID == repositoryID {
				values = append(values, s.project(v))
			}
		}
		return nil
	})
	sort.Slice(values, func(i, j int) bool { return values[i].UpdatedAt.After(values[j].UpdatedAt) })
	return values, err
}
func (s *Store) project(v Goal) Goal {
	if len(v.Revisions) == 0 {
		return v
	}
	r := v.Revisions[len(v.Revisions)-1]
	d := []Diagnostic{}
	env := map[string]bool{}
	for _, e := range r.Environments {
		env[e.Name] = true
	}
	for _, m := range r.Metrics {
		if m.Baseline.Value == nil || m.Baseline.MeasuredAt == nil {
			d = append(d, Diagnostic{"missing_measurement", "blocking", "No measured baseline is attached.", m.Name, r.CreatedBy})
			continue
		}
		if !env[m.Baseline.Environment] {
			d = append(d, Diagnostic{"incomparable_environment", "blocking", "The baseline environment is outside the supported contract.", m.Name, r.CreatedBy})
		}
		if r.BaselineMaxAgeDays > 0 && m.Baseline.MeasuredAt.Add(time.Duration(r.BaselineMaxAgeDays)*24*time.Hour).Before(s.now()) {
			d = append(d, Diagnostic{"stale_baseline", "warning", "The baseline is older than the accepted age.", m.Name, r.CreatedBy})
		}
		if (m.Minimum != nil && *m.Baseline.Value < *m.Minimum) || (m.Maximum != nil && *m.Baseline.Value > *m.Maximum) {
			d = append(d, Diagnostic{"target_gap", "info", "The baseline is outside the target range.", m.Name, r.CreatedBy})
		}
	}
	for _, other := range v.Revisions[:len(v.Revisions)-1] {
		for _, old := range other.Metrics {
			for _, m := range r.Metrics {
				if old.Name == m.Name && old.Unit == m.Unit && rangesDisjoint(old, m) {
					d = append(d, Diagnostic{"conflicting_target", "warning", "This target does not overlap the prior version's range.", m.Name, r.CreatedBy})
				}
			}
		}
	}
	v.Diagnostics = d
	return v
}
func rangesDisjoint(a, b Metric) bool {
	return a.Maximum != nil && b.Minimum != nil && *a.Maximum < *b.Minimum || b.Maximum != nil && a.Minimum != nil && *b.Maximum < *a.Minimum
}
func validate(r Revision) error {
	validSubject := map[string]bool{"repository": true, "release": true, "user_journey": true, "api": true, "command": true, "service": true}
	if !validSubject[r.Subject.Kind] || strings.TrimSpace(r.Subject.Name) == "" || strings.TrimSpace(r.Title) == "" || len(r.Workloads) == 0 || len(r.Metrics) == 0 || len(r.Constraints) == 0 || len(r.Environments) == 0 || len(r.Owners) == 0 || r.BaselineMaxAgeDays < 1 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, e := range r.Environments {
		if strings.TrimSpace(e.Name) == "" || seen[e.Name] {
			return ErrInvalid
		}
		seen[e.Name] = true
	}
	for _, w := range r.Workloads {
		if strings.TrimSpace(w.Name) == "" || strings.TrimSpace(w.Description) == "" || w.Samples < 1 {
			return ErrInvalid
		}
	}
	for _, m := range r.Metrics {
		if strings.TrimSpace(m.Name) == "" || strings.TrimSpace(m.Unit) == "" || (m.Direction != "lower" && m.Direction != "higher" && m.Direction != "range") || (m.Minimum == nil && m.Maximum == nil) || (m.Minimum != nil && (!finite(*m.Minimum))) || (m.Maximum != nil && !finite(*m.Maximum)) || (m.Minimum != nil && m.Maximum != nil && *m.Minimum > *m.Maximum) || (m.Baseline.Value != nil && !finite(*m.Baseline.Value)) {
			return ErrInvalid
		}
	}
	for _, c := range r.Constraints {
		if strings.TrimSpace(c.Name) == "" || strings.TrimSpace(c.Requirement) == "" || strings.TrimSpace(c.Verification) == "" {
			return ErrInvalid
		}
	}
	for _, b := range r.Budgets {
		if strings.TrimSpace(b.Kind) == "" || strings.TrimSpace(b.Unit) == "" || !finite(b.Limit) || b.Limit <= 0 {
			return ErrInvalid
		}
	}
	validLink := map[string]bool{"issue": true, "incident": true, "preview": true, "release": true, "decision": true}
	for _, l := range r.Links {
		if !validLink[l.Kind] || strings.TrimSpace(l.ResourceID) == "" {
			return ErrInvalid
		}
	}
	return nil
}
func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func stampLinks(v []Link, actor string) {
	for i := range v {
		v[i].AddedBy = actor
	}
}
func (s *Store) read(id string) (Goal, error) {
	var v Goal
	b, err := os.ReadFile(filepath.Join(s.root, id+".json"))
	if os.IsNotExist(err) {
		return v, ErrNotFound
	}
	if err != nil {
		return v, err
	}
	if json.Unmarshal(b, &v) != nil {
		return v, ErrInvalid
	}
	return v, nil
}
func (s *Store) write(v Goal) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.root, ".goal-")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(b)
	}
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(name, filepath.Join(s.root, v.ID+".json"))
	}
	if err == nil {
		if dir, openErr := os.Open(s.root); openErr != nil {
			err = openErr
		} else {
			err = dir.Sync()
			_ = dir.Close()
		}
	}
	return err
}
func (s *Store) lock(fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return fn()
}
func randomID() string { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
