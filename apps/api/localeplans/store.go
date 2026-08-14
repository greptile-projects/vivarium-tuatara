// Package localeplans persists repository localization coverage contracts.
package localeplans

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

var ErrNotFound = errors.New("locale plan not found")
var ErrInvalid = errors.New("invalid locale plan")
var ErrConflict = errors.New("locale plan version conflict")

type Subject struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id,omitempty"`
	Name       string `json:"name"`
}
type Locale struct {
	ID             string   `json:"id"`
	Language       string   `json:"language"`
	Regions        []string `json:"regions"`
	FallbackLocale string   `json:"fallback_locale,omitempty"`
	OwnerIDs       []string `json:"owner_ids"`
	ReviewerIDs    []string `json:"reviewer_ids"`
}
type Term struct {
	ID        string   `json:"id"`
	Source    string   `json:"source"`
	Locale    string   `json:"locale"`
	Preferred string   `json:"preferred"`
	Avoid     []string `json:"avoid,omitempty"`
	Context   string   `json:"context"`
}
type Formatting struct {
	Locale    string `json:"locale"`
	Date      string `json:"date"`
	Time      string `json:"time"`
	Number    string `json:"number"`
	Currency  string `json:"currency"`
	Units     string `json:"units"`
	Direction string `json:"direction"`
}
type Journey struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	LocaleIDs []string `json:"locale_ids"`
	OwnerIDs  []string `json:"owner_ids"`
	Required  bool     `json:"required"`
}
type Resource struct {
	ID             string   `json:"id"`
	Kind           string   `json:"kind"`
	Path           string   `json:"path"`
	Format         string   `json:"format"`
	SourceRevision string   `json:"source_revision"`
	LocaleIDs      []string `json:"locale_ids"`
}
type Threshold struct {
	Locale                string   `json:"locale"`
	MinimumPercent        int      `json:"minimum_percent"`
	RequiredJourneyIDs    []string `json:"required_journey_ids"`
	RequireOwnerReview    bool     `json:"require_owner_review"`
	RequireRegionalReview bool     `json:"require_regional_review"`
}
type Diagnostic struct {
	Kind         string `json:"kind"`
	Severity     string `json:"severity"`
	Message      string `json:"message"`
	ResourceID   string `json:"resource_id,omitempty"`
	AttributedTo string `json:"attributed_to"`
}
type Revision struct {
	Version     int          `json:"version"`
	Title       string       `json:"title"`
	Summary     string       `json:"summary"`
	Subject     Subject      `json:"subject"`
	Locales     []Locale     `json:"locales"`
	Terminology []Term       `json:"terminology"`
	Formatting  []Formatting `json:"formatting_requirements"`
	Journeys    []Journey    `json:"covered_journeys"`
	Resources   []Resource   `json:"resources"`
	Thresholds  []Threshold  `json:"release_thresholds"`
	Rationale   string       `json:"rationale"`
	CreatedBy   string       `json:"created_by"`
	CreatedAt   time.Time    `json:"created_at"`
}
type Plan struct {
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
func (s *Store) Create(repo, actor string, r Revision) (Plan, error) {
	var out Plan
	err := s.lock(func() error {
		if validate(r) != nil {
			return ErrInvalid
		}
		now := s.now()
		stamp(&r, 1, actor, now)
		out = Plan{ID: id(), RepositoryID: repo, CurrentVersion: 1, Revisions: []Revision{r}, CreatedAt: now, UpdatedAt: now}
		return s.write(out)
	})
	return project(out, ""), err
}
func (s *Store) Revise(id string, expected int, actor string, r Revision) (Plan, error) {
	var out Plan
	err := s.lock(func() error {
		v, e := s.read(id)
		if e != nil {
			return e
		}
		if v.CurrentVersion != expected {
			return ErrConflict
		}
		if validate(r) != nil {
			return ErrInvalid
		}
		stamp(&r, expected+1, actor, s.now())
		v.CurrentVersion = r.Version
		v.Revisions = append(v.Revisions, r)
		v.UpdatedAt = r.CreatedAt
		out = v
		return s.write(v)
	})
	return project(out, ""), err
}
func (s *Store) Get(id, current string) (Plan, error) {
	var v Plan
	err := s.lock(func() error { var e error; v, e = s.read(id); return e })
	return project(v, current), err
}
func (s *Store) List(repo, current string) ([]Plan, error) {
	values := []Plan{}
	err := s.lock(func() error {
		entries, e := os.ReadDir(filepath.Join(s.root, repo))
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
			v, e := s.readFile(filepath.Join(s.root, repo, x.Name()))
			if e != nil {
				return e
			}
			if v.RepositoryID == repo {
				values = append(values, project(v, current))
			}
		}
		return nil
	})
	sort.Slice(values, func(i, j int) bool { return values[i].UpdatedAt.After(values[j].UpdatedAt) })
	return values, err
}
func stamp(r *Revision, v int, actor string, now time.Time) {
	r.Version = v
	r.CreatedBy = actor
	r.CreatedAt = now
}

func validate(r Revision) error {
	if len(r.Title) > 200 || strings.TrimSpace(r.Title) == "" || len(r.Summary) > 4000 || strings.TrimSpace(r.Summary) == "" || !oneOf(r.Subject.Kind, "repository", "product", "documentation_collection", "release") || strings.TrimSpace(r.Subject.Name) == "" || len(r.Locales) == 0 || len(r.Formatting) == 0 || len(r.Journeys) == 0 || len(r.Resources) == 0 || len(r.Thresholds) == 0 || strings.TrimSpace(r.Rationale) == "" {
		return ErrInvalid
	}
	locales := map[string]bool{}
	for _, l := range r.Locales {
		if strings.TrimSpace(l.ID) == "" || strings.TrimSpace(l.Language) == "" || locales[l.ID] {
			return ErrInvalid
		}
		locales[l.ID] = true
	}
	for _, l := range r.Locales {
		if l.FallbackLocale != "" && !locales[l.FallbackLocale] {
			return ErrInvalid
		}
	}
	formattingLocales := map[string]bool{}
	for _, f := range r.Formatting {
		if !locales[f.Locale] || formattingLocales[f.Locale] || f.Date == "" || f.Time == "" || f.Number == "" || f.Currency == "" || f.Units == "" || !oneOf(f.Direction, "ltr", "rtl") {
			return ErrInvalid
		}
		formattingLocales[f.Locale] = true
	}
	for locale := range locales {
		if !formattingLocales[locale] {
			return ErrInvalid
		}
	}
	for _, term := range r.Terminology {
		if !locales[term.Locale] {
			return ErrInvalid
		}
	}
	journeys := map[string]bool{}
	for _, j := range r.Journeys {
		if j.ID == "" || j.Name == "" || journeys[j.ID] || len(j.LocaleIDs) == 0 {
			return ErrInvalid
		}
		journeys[j.ID] = true
		for _, l := range j.LocaleIDs {
			if !locales[l] {
				return ErrInvalid
			}
		}
	}
	for _, x := range r.Resources {
		if x.ID == "" || x.Path == "" || x.Format == "" || len(x.SourceRevision) != 40 || len(x.LocaleIDs) == 0 {
			return ErrInvalid
		}
		for _, l := range x.LocaleIDs {
			if !locales[l] {
				return ErrInvalid
			}
		}
	}
	thresholdLocales := map[string]bool{}
	for _, t := range r.Thresholds {
		if !locales[t.Locale] || thresholdLocales[t.Locale] || t.MinimumPercent < 0 || t.MinimumPercent > 100 {
			return ErrInvalid
		}
		thresholdLocales[t.Locale] = true
		for _, j := range t.RequiredJourneyIDs {
			if !journeys[j] {
				return ErrInvalid
			}
		}
	}
	for locale := range locales {
		if !thresholdLocales[locale] {
			return ErrInvalid
		}
	}
	return nil
}
func project(v Plan, current string) Plan {
	if len(v.Revisions) == 0 {
		return v
	}
	r := v.Revisions[len(v.Revisions)-1]
	d := []Diagnostic{}
	for _, l := range r.Locales {
		if len(l.OwnerIDs) == 0 {
			d = append(d, Diagnostic{"missing_ownership", "blocking", "Locale has no accountable quality owner.", l.ID, r.CreatedBy})
		}
		if len(l.ReviewerIDs) == 0 {
			d = append(d, Diagnostic{"missing_reviewers", "warning", "Locale has no named reviewer.", l.ID, r.CreatedBy})
		}
	}
	for _, j := range r.Journeys {
		if len(j.OwnerIDs) == 0 {
			d = append(d, Diagnostic{"missing_ownership", "blocking", "Covered journey has no owner.", j.ID, r.CreatedBy})
		}
	}
	supported := map[string]bool{"json": true, "po": true, "xliff": true, "arb": true, "yaml": true, "markdown": true, "mdx": true}
	for _, x := range r.Resources {
		if !supported[strings.ToLower(x.Format)] {
			d = append(d, Diagnostic{"unsupported_format", "blocking", "Resource format is not supported for localization extraction.", x.ID, r.CreatedBy})
		}
		if current != "" && !strings.EqualFold(x.SourceRevision, current) {
			d = append(d, Diagnostic{"stale_coverage", "blocking", "Resource coverage refers to an older source revision.", x.ID, r.CreatedBy})
		}
	}
	for i, a := range r.Terminology {
		for _, b := range r.Terminology[i+1:] {
			if strings.EqualFold(a.Locale, b.Locale) && strings.EqualFold(a.Source, b.Source) && !strings.EqualFold(a.Preferred, b.Preferred) {
				d = append(d, Diagnostic{"conflicting_terminology", "blocking", "The same source term has conflicting preferred translations.", a.ID, r.CreatedBy})
			}
		}
	}
	v.Diagnostics = d
	return v
}
func oneOf(v string, vs ...string) bool {
	for _, x := range vs {
		if v == x {
			return true
		}
	}
	return false
}
func id() string { b := make([]byte, 12); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func (s *Store) write(v Plan) error {
	dir := filepath.Join(s.root, v.RepositoryID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(dir, ".locale-*")
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
	return os.Rename(name, filepath.Join(dir, v.ID+".json"))
}
func (s *Store) read(id string) (Plan, error) {
	entries, e := os.ReadDir(s.root)
	if e != nil {
		return Plan{}, e
	}
	for _, repo := range entries {
		if !repo.IsDir() {
			continue
		}
		v, e := s.readFile(filepath.Join(s.root, repo.Name(), id+".json"))
		if e == nil {
			return v, nil
		}
		if !os.IsNotExist(e) {
			return Plan{}, e
		}
	}
	return Plan{}, ErrNotFound
}
func (s *Store) readFile(path string) (Plan, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return Plan{}, e
	}
	var v Plan
	if e = json.Unmarshal(b, &v); e != nil {
		return Plan{}, e
	}
	return v, nil
}
func (s *Store) lock(fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	lf, e := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return e
	}
	defer lf.Close()
	if e = syscall.Flock(int(lf.Fd()), syscall.LOCK_EX); e != nil {
		return e
	}
	defer syscall.Flock(int(lf.Fd()), syscall.LOCK_UN)
	return fn()
}
