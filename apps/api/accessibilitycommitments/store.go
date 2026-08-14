// Package accessibilitycommitments persists versioned accessibility contracts.
package accessibilitycommitments

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

var ErrNotFound = errors.New("accessibility commitment not found")
var ErrInvalid = errors.New("invalid accessibility commitment")
var ErrConflict = errors.New("accessibility commitment version conflict")

type Subject struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id,omitempty"`
	Name       string `json:"name"`
}
type Standard struct {
	Name     string   `json:"name"`
	Version  string   `json:"version"`
	Level    string   `json:"level"`
	Criteria []string `json:"criteria"`
}
type AssistiveTechnology struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Version        string   `json:"version"`
	Input          string   `json:"input"`
	EnvironmentIDs []string `json:"environment_ids"`
}
type Audience struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	AccessNeeds []string `json:"access_needs"`
}
type Environment struct {
	ID             string `json:"id"`
	Browser        string `json:"browser"`
	BrowserVersion string `json:"browser_version"`
	OS             string `json:"os"`
	Device         string `json:"device"`
	Supported      bool   `json:"supported"`
	Note           string `json:"note,omitempty"`
}
type Scenario struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Steps            []string `json:"steps"`
	ExpectedOutcome  string   `json:"expected_outcome"`
	StandardCriteria []string `json:"standard_criteria"`
	AudienceIDs      []string `json:"audience_ids"`
	TechnologyIDs    []string `json:"technology_ids"`
	EnvironmentIDs   []string `json:"environment_ids"`
	OwnerIDs         []string `json:"owner_ids"`
}
type SeverityRule struct {
	Severity       string `json:"severity"`
	Definition     string `json:"definition"`
	Response       string `json:"response"`
	ResolutionDays int    `json:"resolution_days"`
}
type Exception struct {
	ID         string    `json:"id"`
	Scope      string    `json:"scope"`
	Reason     string    `json:"reason"`
	ApprovedBy string    `json:"approved_by"`
	ExpiresAt  time.Time `json:"expires_at"`
	Mitigation string    `json:"mitigation"`
}
type Requirement struct {
	ID            string   `json:"id"`
	Statement     string   `json:"statement"`
	ConflictsWith []string `json:"conflicts_with,omitempty"`
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
	ResourceID   string `json:"resource_id,omitempty"`
	AttributedTo string `json:"attributed_to"`
}
type Revision struct {
	Version               int                   `json:"version"`
	Title                 string                `json:"title"`
	Summary               string                `json:"summary"`
	Subject               Subject               `json:"subject"`
	Standards             []Standard            `json:"standards"`
	AssistiveTechnologies []AssistiveTechnology `json:"assistive_technologies"`
	Audiences             []Audience            `json:"target_audiences"`
	Environments          []Environment         `json:"environments"`
	Scenarios             []Scenario            `json:"required_scenarios"`
	SeverityPolicy        []SeverityRule        `json:"severity_policy"`
	OwnerIDs              []string              `json:"owner_ids"`
	Requirements          []Requirement         `json:"requirements"`
	Exceptions            []Exception           `json:"exceptions"`
	Links                 []Link                `json:"links"`
	Rationale             string                `json:"rationale"`
	CreatedBy             string                `json:"created_by"`
	CreatedAt             time.Time             `json:"created_at"`
}
type Commitment struct {
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
func (s *Store) Create(repo, actor string, r Revision) (Commitment, error) {
	var out Commitment
	err := s.lock(func() error {
		if validate(r) != nil {
			return ErrInvalid
		}
		now := s.now()
		stamp(&r, 1, actor, now)
		out = Commitment{ID: randomID(), RepositoryID: repo, CurrentVersion: 1, Revisions: []Revision{r}, CreatedAt: now, UpdatedAt: now}
		return s.write(out)
	})
	return s.project(out), err
}
func (s *Store) Revise(id string, expected int, actor string, r Revision) (Commitment, error) {
	var out Commitment
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
	return s.project(out), err
}
func (s *Store) Get(id string) (Commitment, error) {
	var v Commitment
	err := s.lock(func() error { var e error; v, e = s.read(id); return e })
	return s.project(v), err
}
func (s *Store) List(repo string) ([]Commitment, error) {
	values := []Commitment{}
	err := s.lock(func() error {
		// New records are repository-scoped so corruption ownership remains
		// knowable without decoding the record itself.
		dir := s.repositoryDir(repo)
		entries, e := os.ReadDir(dir)
		if os.IsNotExist(e) {
			entries = nil
			e = nil
		}
		if e != nil {
			return e
		}
		for _, x := range entries {
			if x.IsDir() || !strings.HasSuffix(x.Name(), ".json") {
				continue
			}
			v, e := s.readFile(filepath.Join(dir, x.Name()))
			if e != nil {
				return e
			}
			if v.RepositoryID == repo {
				values = append(values, s.project(v))
			}
		}
		return nil
	})
	sort.Slice(values, func(i, j int) bool { return values[i].UpdatedAt.After(values[j].UpdatedAt) })
	return values, err
}
func stamp(r *Revision, version int, actor string, now time.Time) {
	r.Version = version
	r.CreatedBy = actor
	r.CreatedAt = now
	for i := range r.Links {
		r.Links[i].AddedBy = actor
	}
}
func validate(r Revision) error {
	validSubject := map[string]bool{"repository": true, "documented_journey": true, "component": true, "release": true}
	if !validSubject[r.Subject.Kind] || strings.TrimSpace(r.Subject.Name) == "" || strings.TrimSpace(r.Title) == "" || len(r.Standards) == 0 || len(r.AssistiveTechnologies) == 0 || len(r.Audiences) == 0 || len(r.Environments) == 0 || len(r.Scenarios) == 0 || len(r.SeverityPolicy) == 0 || len(r.OwnerIDs) == 0 {
		return ErrInvalid
	}
	ids := map[string]bool{}
	criteria := map[string]bool{}
	add := func(id string) bool {
		if strings.TrimSpace(id) == "" || ids[id] {
			return false
		}
		ids[id] = true
		return true
	}
	for _, x := range r.Standards {
		if strings.TrimSpace(x.Name) == "" || strings.TrimSpace(x.Version) == "" || strings.TrimSpace(x.Level) == "" || len(x.Criteria) == 0 {
			return ErrInvalid
		}
		for _, criterion := range x.Criteria {
			criterion = strings.TrimSpace(criterion)
			if criterion == "" {
				return ErrInvalid
			}
			criteria[criterion] = true
		}
	}
	for _, x := range r.AssistiveTechnologies {
		if !add("at:"+x.ID) || strings.TrimSpace(x.Name) == "" {
			return ErrInvalid
		}
	}
	for _, x := range r.Audiences {
		if !add("aud:"+x.ID) || strings.TrimSpace(x.Name) == "" || len(x.AccessNeeds) == 0 {
			return ErrInvalid
		}
	}
	for _, x := range r.Environments {
		if !add("env:"+x.ID) || strings.TrimSpace(x.Browser) == "" || strings.TrimSpace(x.OS) == "" {
			return ErrInvalid
		}
	}
	for _, x := range r.Scenarios {
		if !add("scenario:"+x.ID) || strings.TrimSpace(x.Name) == "" || len(x.Steps) == 0 || strings.TrimSpace(x.ExpectedOutcome) == "" {
			return ErrInvalid
		}
		for _, criterion := range x.StandardCriteria {
			if !criteria[strings.TrimSpace(criterion)] {
				return ErrInvalid
			}
		}
	}
	validSeverity := map[string]bool{"critical": true, "major": true, "minor": true, "advisory": true}
	for _, x := range r.SeverityPolicy {
		if !validSeverity[x.Severity] || strings.TrimSpace(x.Definition) == "" || strings.TrimSpace(x.Response) == "" || x.ResolutionDays < 1 {
			return ErrInvalid
		}
	}
	validLink := map[string]bool{"roadmap_outcome": true, "documentation": true, "preview": true, "release_policy": true}
	for _, x := range r.Links {
		if !validLink[x.Kind] || strings.TrimSpace(x.ResourceID) == "" {
			return ErrInvalid
		}
	}
	for _, x := range r.Exceptions {
		if strings.TrimSpace(x.ID) == "" || strings.TrimSpace(x.Scope) == "" || strings.TrimSpace(x.Reason) == "" || strings.TrimSpace(x.ApprovedBy) == "" || x.ExpiresAt.IsZero() {
			return ErrInvalid
		}
	}
	return nil
}
func (s *Store) project(v Commitment) Commitment {
	if len(v.Revisions) == 0 {
		return v
	}
	r := v.Revisions[len(v.Revisions)-1]
	d := []Diagnostic{}
	add := func(k, sev, msg, id, actor string) { d = append(d, Diagnostic{k, sev, msg, id, actor}) }
	aud, tech, env := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, x := range r.Audiences {
		aud[x.ID] = true
	}
	for _, x := range r.AssistiveTechnologies {
		tech[x.ID] = true
		for _, id := range x.EnvironmentIDs {
			if !containsEnvironment(r.Environments, id, true) {
				add("unsupported_environment", "blocking", "Assistive technology names an environment that is absent or unsupported.", id, r.CreatedBy)
			}
		}
	}
	for _, x := range r.Environments {
		env[x.ID] = x.Supported
	}
	for _, x := range r.Scenarios {
		if len(x.StandardCriteria) == 0 || len(x.AudienceIDs) == 0 || len(x.TechnologyIDs) == 0 || len(x.EnvironmentIDs) == 0 || len(x.OwnerIDs) == 0 {
			add("missing_coverage", "blocking", "Required scenario lacks standards, audience, technology, environment, or owner coverage.", x.ID, r.CreatedBy)
		}
		for _, id := range x.AudienceIDs {
			if !aud[id] {
				add("missing_coverage", "blocking", "Scenario references an undefined target audience.", x.ID, r.CreatedBy)
			}
		}
		for _, id := range x.TechnologyIDs {
			if !tech[id] {
				add("missing_coverage", "blocking", "Scenario references an undefined assistive technology.", x.ID, r.CreatedBy)
			}
		}
		for _, id := range x.EnvironmentIDs {
			if supported, ok := env[id]; !ok || !supported {
				add("unsupported_environment", "blocking", "Required scenario depends on an absent or unsupported environment.", x.ID, r.CreatedBy)
			}
		}
	}
	req := map[string]bool{}
	for _, x := range r.Requirements {
		req[x.ID] = true
	}
	for _, x := range r.Requirements {
		for _, id := range x.ConflictsWith {
			if req[id] {
				add("conflicting_requirement", "warning", "Current requirements declare an unresolved conflict.", x.ID, r.CreatedBy)
			}
		}
	}
	for _, x := range r.Exceptions {
		actor := x.ApprovedBy
		if !x.ExpiresAt.After(s.now()) {
			add("expired_exception", "blocking", "Permitted exception has expired.", x.ID, actor)
		} else if x.ExpiresAt.Before(s.now().Add(30 * 24 * time.Hour)) {
			add("expiring_exception", "warning", "Permitted exception expires within 30 days.", x.ID, actor)
		}
	}
	v.Diagnostics = d
	return v
}
func containsEnvironment(values []Environment, id string, supported bool) bool {
	for _, x := range values {
		if x.ID == id && (!supported || x.Supported) {
			return true
		}
	}
	return false
}
func (s *Store) read(id string) (Commitment, error) {
	entries, e := os.ReadDir(s.root)
	if e != nil {
		return Commitment{}, e
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if v, readErr := s.readFile(filepath.Join(s.root, entry.Name(), id+".json")); !errors.Is(readErr, ErrNotFound) {
			return v, readErr
		}
	}
	return Commitment{}, ErrNotFound
}
func (s *Store) readFile(name string) (Commitment, error) {
	var v Commitment
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
func (s *Store) write(v Commitment) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	dir := s.repositoryDir(v.RepositoryID)
	if e = os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(dir, ".commitment-")
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
	return e
}
func (s *Store) repositoryDir(repositoryID string) string {
	return filepath.Join(s.root, "repo-"+base64.RawURLEncoding.EncodeToString([]byte(repositoryID)))
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
