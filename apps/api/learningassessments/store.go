// Package learningassessments retains revision-exact practical assessments and review evidence.
package learningassessments

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	ErrNotFound = errors.New("learning assessment not found")
	ErrInvalid  = errors.New("invalid learning assessment")
	ErrConflict = errors.New("learning assessment changed")
)

type Criterion struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Weight      int    `json:"weight"`
	Required    bool   `json:"required"`
}
type ProtectedCase struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Expected    string `json:"expected"`
}
type RetryPolicy struct {
	MaximumAttempts int `json:"maximum_attempts"`
	CooldownHours   int `json:"cooldown_hours"`
}
type Definition struct {
	ID                   string          `json:"id"`
	RequestID            string          `json:"request_id"`
	RepositoryID         string          `json:"repository_id"`
	Slug                 string          `json:"slug"`
	Version              int             `json:"version"`
	PathwaySlug          string          `json:"pathway_slug"`
	PathwayVersion       int             `json:"pathway_version"`
	ProjectRevision      string          `json:"project_revision"`
	Title                string          `json:"title"`
	Instructions         string          `json:"instructions"`
	Criteria             []Criterion     `json:"criteria"`
	ProtectedCases       []ProtectedCase `json:"protected_cases,omitempty"`
	RequiredChecks       []string        `json:"required_checks"`
	RetryPolicy          RetryPolicy     `json:"retry_policy"`
	AccommodationOptions []string        `json:"accommodation_options,omitempty"`
	PublishedBy          string          `json:"published_by"`
	PublishedAt          time.Time       `json:"published_at"`
}
type Evidence struct {
	CheckpointIDs           []string `json:"checkpoint_ids"`
	CommandOutcomeIDs       []string `json:"command_outcome_ids"`
	CheckRunIDs             []string `json:"check_run_ids"`
	AuthorshipStatement     string   `json:"authorship_statement"`
	AgentAssistanceDeclared bool     `json:"agent_assistance_declared"`
}
type RubricDecision struct {
	CriterionID string `json:"criterion_id"`
	Decision    string `json:"decision"`
	Rationale   string `json:"rationale"`
	Confidence  string `json:"confidence"`
}
type Review struct {
	ID          string           `json:"id"`
	ReviewerID  string           `json:"reviewer_id"`
	Decisions   []RubricDecision `json:"decisions"`
	Feedback    string           `json:"feedback"`
	Uncertainty string           `json:"uncertainty,omitempty"`
	Outcome     string           `json:"outcome"`
	CreatedAt   time.Time        `json:"created_at"`
}
type Appeal struct {
	ID         string    `json:"id"`
	Body       string    `json:"body"`
	ActorID    string    `json:"actor_id"`
	Resolution string    `json:"resolution,omitempty"`
	ResolvedBy string    `json:"resolved_by,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}
type Attempt struct {
	ID                    string    `json:"id"`
	RequestID             string    `json:"request_id"`
	RepositoryID          string    `json:"repository_id"`
	AssessmentSlug        string    `json:"assessment_slug"`
	AssessmentVersion     int       `json:"assessment_version"`
	WorkspaceID           string    `json:"workspace_id"`
	LearnerID             string    `json:"learner_id"`
	ProjectRevision       string    `json:"project_revision"`
	ReproducibilitySHA256 string    `json:"reproducibility_sha256"`
	WorkProductSHA256     string    `json:"work_product_sha256,omitempty"`
	Evidence              Evidence  `json:"evidence"`
	Accommodation         string    `json:"accommodation,omitempty"`
	AttemptNumber         int       `json:"attempt_number"`
	Reviews               []Review  `json:"reviews"`
	Appeals               []Appeal  `json:"appeals"`
	Status                string    `json:"status"`
	Blockers              []string  `json:"blockers"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
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
func (s *Store) Publish(v Definition, expected int) (Definition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Definition{}, err
	}
	defer unlock()
	items, _ := s.list(v.RepositoryID, v.Slug)
	for _, x := range items {
		if x.RequestID == v.RequestID {
			a, _ := json.Marshal(x)
			v.ID = x.ID
			v.Version = x.Version
			v.PublishedAt = x.PublishedAt
			b, _ := json.Marshal(v)
			if string(a) != string(b) {
				return Definition{}, ErrConflict
			}
			return x, nil
		}
	}
	if len(items) != expected || !validDefinition(v) {
		return Definition{}, func() error {
			if len(items) != expected {
				return ErrConflict
			}
			return ErrInvalid
		}()
	}
	v.ID = id()
	v.Version = len(items) + 1
	v.PublishedAt = s.now().UTC()
	dir := filepath.Join(s.root, v.RepositoryID, v.Slug)
	if os.MkdirAll(dir, 0700) != nil {
		return Definition{}, ErrInvalid
	}
	if write(filepath.Join(dir, fmt.Sprintf("definition-%09d.json", v.Version)), v) != nil {
		return Definition{}, ErrInvalid
	}
	return v, nil
}
func (s *Store) List(repo, slug string) ([]Definition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.list(repo, slug)
}
func (s *Store) list(repo, slug string) ([]Definition, error) {
	if !safeID(repo) || !safeID(slug) {
		return nil, ErrNotFound
	}
	es, err := os.ReadDir(filepath.Join(s.root, repo, slug))
	if errors.Is(err, os.ErrNotExist) {
		return []Definition{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []Definition{}
	for _, e := range es {
		if strings.HasPrefix(e.Name(), "definition-") {
			var v Definition
			if read(filepath.Join(s.root, repo, slug, e.Name()), &v) == nil && v.RepositoryID == repo && v.Slug == slug {
				out = append(out, v)
			}
		}
	}
	return out, nil
}
func (s *Store) Current(repo, slug string) (Definition, error) {
	v, e := s.List(repo, slug)
	if e != nil || len(v) == 0 {
		return Definition{}, ErrNotFound
	}
	return v[len(v)-1], nil
}
func (s *Store) Slugs(repo string) ([]string, error) {
	if !safeID(repo) {
		return nil, ErrNotFound
	}
	es, e := os.ReadDir(filepath.Join(s.root, repo))
	if errors.Is(e, os.ErrNotExist) {
		return []string{}, nil
	}
	if e != nil {
		return nil, e
	}
	o := []string{}
	for _, x := range es {
		if x.IsDir() {
			o = append(o, x.Name())
		}
	}
	return o, nil
}
func (s *Store) CreateAttempt(a Attempt, max, cooldownHours int) (Attempt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Attempt{}, err
	}
	defer unlock()
	all, _ := s.attempts(a.RepositoryID, a.AssessmentSlug)
	for _, x := range all {
		if x.LearnerID == a.LearnerID && x.RequestID == a.RequestID {
			if x.WorkspaceID != a.WorkspaceID || x.AssessmentVersion != a.AssessmentVersion {
				return Attempt{}, ErrConflict
			}
			return x, nil
		}
	}
	n := 0
	var latest time.Time
	for _, x := range all {
		if x.LearnerID == a.LearnerID && x.AssessmentVersion == a.AssessmentVersion {
			n++
			if x.CreatedAt.After(latest) {
				latest = x.CreatedAt
			}
		}
	}
	if n >= max || cooldownHours > 0 && !latest.IsZero() && s.now().Before(latest.Add(time.Duration(cooldownHours)*time.Hour)) || a.WorkspaceID == "" || a.LearnerID == "" {
		return Attempt{}, ErrInvalid
	}
	a.ID = id()
	a.AttemptNumber = n + 1
	a.Status = "submitted"
	a.Reviews = []Review{}
	a.Appeals = []Appeal{}
	a.CreatedAt = s.now().UTC()
	a.UpdatedAt = a.CreatedAt
	if write(s.attemptPath(a), a) != nil {
		return Attempt{}, ErrInvalid
	}
	return a, nil
}
func (s *Store) Attempts(repo, slug string) ([]Attempt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.attempts(repo, slug)
}

// FindAttempt resolves an opaque attempt identity within one repository. The
// repository boundary is deliberate: possessing an attempt ID is not enough
// to disclose learning evidence from another project.
func (s *Store) FindAttempt(repo, attemptID string) (Attempt, error) {
	if !safeID(repo) || !safeID(attemptID) {
		return Attempt{}, ErrNotFound
	}
	slugs, err := s.Slugs(repo)
	if err != nil {
		return Attempt{}, err
	}
	for _, slug := range slugs {
		attempts, err := s.Attempts(repo, slug)
		if err != nil {
			return Attempt{}, err
		}
		for _, attempt := range attempts {
			if attempt.ID == attemptID && attempt.RepositoryID == repo {
				return attempt, nil
			}
		}
	}
	return Attempt{}, ErrNotFound
}
func (s *Store) attempts(repo, slug string) ([]Attempt, error) {
	if !safeID(repo) || !safeID(slug) {
		return nil, ErrNotFound
	}
	es, e := os.ReadDir(filepath.Join(s.root, repo, slug, "attempts"))
	if errors.Is(e, os.ErrNotExist) {
		return []Attempt{}, nil
	}
	if e != nil {
		return nil, e
	}
	o := []Attempt{}
	for _, x := range es {
		var a Attempt
		if read(filepath.Join(s.root, repo, slug, "attempts", x.Name()), &a) == nil {
			o = append(o, a)
		}
	}
	return o, nil
}
func (s *Store) UpdateAttempt(repo, slug, attempt string, mutate func(*Attempt) error) (Attempt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Attempt{}, err
	}
	defer unlock()
	if !safeID(repo) || !safeID(slug) || !safeID(attempt) {
		return Attempt{}, ErrNotFound
	}
	p := filepath.Join(s.root, repo, slug, "attempts", attempt+".json")
	var a Attempt
	if read(p, &a) != nil {
		return Attempt{}, ErrNotFound
	}
	if e := mutate(&a); e != nil {
		return Attempt{}, e
	}
	now := s.now().UTC()
	for i := range a.Reviews {
		if a.Reviews[i].CreatedAt.IsZero() {
			a.Reviews[i].CreatedAt = now
		}
	}
	for i := range a.Appeals {
		if a.Appeals[i].CreatedAt.IsZero() {
			a.Appeals[i].CreatedAt = now
		}
	}
	a.UpdatedAt = now
	if write(p, a) != nil {
		return Attempt{}, ErrInvalid
	}
	return a, nil
}
func (s *Store) attemptPath(a Attempt) string {
	return filepath.Join(s.root, a.RepositoryID, a.AssessmentSlug, "attempts", a.ID+".json")
}
func validDefinition(v Definition) bool {
	if !safeID(v.RepositoryID) || !safeID(v.Slug) || !safeID(v.RequestID) || !safeID(v.PathwaySlug) || v.PathwayVersion < 1 || len(v.ProjectRevision) != 40 || strings.TrimSpace(v.Title) == "" || len(v.Title) > 300 || len(v.Instructions) > 10000 || len(v.Criteria) == 0 || len(v.Criteria) > 100 || v.RetryPolicy.MaximumAttempts < 1 || v.RetryPolicy.MaximumAttempts > 10 || v.RetryPolicy.CooldownHours < 0 || v.RetryPolicy.CooldownHours > 8760 {
		return false
	}
	seen := map[string]bool{}
	for _, c := range v.Criteria {
		if !safeID(c.ID) || seen[c.ID] || c.Weight < 1 || strings.TrimSpace(c.Description) == "" || len(c.Description) > 2000 {
			return false
		}
		seen[c.ID] = true
	}
	for _, c := range v.ProtectedCases {
		if !safeID(c.ID) || strings.TrimSpace(c.Description) == "" || strings.TrimSpace(c.Expected) == "" || len(c.Description) > 2000 || len(c.Expected) > 4000 {
			return false
		}
	}
	return true
}
func safeID(v string) bool {
	if len(v) < 1 || len(v) > 128 {
		return false
	}
	for _, r := range v {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}
func (s *Store) lock() (func(), error) {
	f, err := os.OpenFile(filepath.Join(s.root, ".mutation.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}
func id() string { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func write(p string, v any) error {
	if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		return e
	}
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	tmp := p + ".tmp-" + id()
	f, e := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	if _, e = f.Write(b); e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e == nil {
		e = closeErr
	}
	if e != nil {
		_ = os.Remove(tmp)
		return e
	}
	if e = os.Rename(tmp, p); e != nil {
		_ = os.Remove(tmp)
		return e
	}
	dir, e := os.Open(filepath.Dir(p))
	if e != nil {
		return e
	}
	defer dir.Close()
	return dir.Sync()
}
func read(p string, v any) error {
	b, e := os.ReadFile(p)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, v)
}
