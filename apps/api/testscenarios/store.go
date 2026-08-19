// Package testscenarios persists immutable, revision-exact executable behavior specifications.
package testscenarios

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

var ErrNotFound = errors.New("test scenario not found")
var ErrInvalid = errors.New("invalid test scenario")

type Source struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Revision   string `json:"revision"`
	Path       string `json:"path,omitempty"`
	Summary    string `json:"summary"`
}
type Parameter struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Example     string `json:"example,omitempty"`
}
type Step struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	Operation   string   `json:"operation"`
	Parameters  []string `json:"parameters,omitempty"`
}
type Assertion struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Matcher     string `json:"matcher"`
	Expected    string `json:"expected"`
}
type Fixture struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"`
	Description string   `json:"description"`
	Path        string   `json:"path"`
	SHA256      string   `json:"sha256"`
	DataClass   string   `json:"data_class"`
	Generator   string   `json:"generator,omitempty"`
	Assumptions []string `json:"assumptions"`
	SourceIDs   []string `json:"source_ids,omitempty"`
}
type Environment struct {
	ID           string   `json:"id"`
	Description  string   `json:"description"`
	Runtime      string   `json:"runtime"`
	Requirements []string `json:"requirements"`
}
type Case struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Values          map[string]string `json:"values"`
	Assumptions     []string          `json:"assumptions"`
	ExpectedOutcome string            `json:"expected_outcome"`
}
type Implementation struct {
	AuthoredByType string   `json:"authored_by_type"`
	Branch         string   `json:"branch"`
	CommitID       string   `json:"commit_id"`
	PullRequestID  string   `json:"pull_request_id,omitempty"`
	WorkspaceID    string   `json:"workspace_id,omitempty"`
	TestPaths      []string `json:"test_paths"`
	Command        string   `json:"command"`
	Framework      string   `json:"framework"`
	Generated      bool     `json:"generated"`
	Assumptions    []string `json:"assumptions"`
	Provenance     []string `json:"provenance"`
}
type Scenario struct {
	ID                 string         `json:"id"`
	RepositoryID       string         `json:"repository_id"`
	Title              string         `json:"title"`
	Purpose            string         `json:"purpose"`
	QualityPlanID      string         `json:"quality_plan_id,omitempty"`
	QualityPlanVersion int            `json:"quality_plan_version,omitempty"`
	RequirementIDs     []string       `json:"requirement_ids,omitempty"`
	Sources            []Source       `json:"sources"`
	Parameters         []Parameter    `json:"parameters"`
	Preconditions      []Step         `json:"preconditions"`
	Actions            []Step         `json:"actions"`
	Assertions         []Assertion    `json:"assertions"`
	Fixtures           []Fixture      `json:"fixtures"`
	Environments       []Environment  `json:"environments"`
	Cases              []Case         `json:"cases"`
	Implementation     Implementation `json:"implementation"`
	CreatedBy          string         `json:"created_by"`
	CreatedAt          time.Time      `json:"created_at"`
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
func (s *Store) Create(repo, actor string, v Scenario) (Scenario, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !Valid(v) {
		return Scenario{}, ErrInvalid
	}
	v.ID = id()
	v.RepositoryID = repo
	v.CreatedBy = actor
	v.CreatedAt = s.now()
	if err := s.write(v); err != nil {
		return Scenario{}, err
	}
	return v, nil
}
func (s *Store) Get(id string) (Scenario, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.read(id) }
func (s *Store) List(repo string) ([]Scenario, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, e := os.ReadDir(s.root)
	if e != nil {
		return nil, e
	}
	out := []Scenario{}
	for _, x := range entries {
		if x.IsDir() || !strings.HasSuffix(x.Name(), ".json") {
			continue
		}
		v, e := s.read(strings.TrimSuffix(x.Name(), ".json"))
		if e != nil {
			return nil, e
		}
		if v.RepositoryID == repo {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func Valid(v Scenario) bool {
	if blank(v.Title, v.Purpose, v.Implementation.Branch, v.Implementation.CommitID, v.Implementation.Command, v.Implementation.Framework) || len(v.Sources) == 0 || len(v.Preconditions) == 0 || len(v.Actions) == 0 || len(v.Assertions) == 0 || len(v.Environments) == 0 || len(v.Cases) == 0 || len(v.Implementation.TestPaths) == 0 {
		return false
	}
	if len(v.Implementation.CommitID) != 40 || !hexText(v.Implementation.CommitID) || !one(v.Implementation.AuthoredByType, "human", "agent") {
		return false
	}
	sourceIDs := map[string]bool{}
	for _, x := range v.Sources {
		if !one(x.Kind, "issue", "reproduction", "design_specification", "api_contract", "documentation", "user_journey") || blank(x.ResourceID, x.Revision, x.Summary) || len(x.Revision) != 40 || !hexText(x.Revision) {
			return false
		}
		sourceIDs[x.ResourceID] = true
	}
	params := map[string]bool{}
	for _, x := range v.Parameters {
		if blank(x.Name, x.Description) || !one(x.Type, "string", "number", "boolean", "enum", "json") || params[x.Name] {
			return false
		}
		params[x.Name] = true
	}
	if !steps(v.Preconditions, params) || !steps(v.Actions, params) {
		return false
	}
	seen := map[string]bool{}
	for _, x := range v.Assertions {
		if blank(x.ID, x.Description, x.Matcher, x.Expected) || seen[x.ID] {
			return false
		}
		seen[x.ID] = true
	}
	for _, x := range v.Fixtures {
		if blank(x.ID, x.Kind, x.Description, x.Path, x.SHA256, x.DataClass) || len(x.SHA256) != 64 || !hexText(x.SHA256) || !one(x.Kind, "synthetic", "generated", "template") || !one(x.DataClass, "synthetic", "anonymized", "public") || len(x.Assumptions) == 0 {
			return false
		}
		for _, id := range x.SourceIDs {
			if !sourceIDs[id] {
				return false
			}
		}
	}
	envs := map[string]bool{}
	for _, x := range v.Environments {
		if blank(x.ID, x.Description, x.Runtime) || envs[x.ID] {
			return false
		}
		envs[x.ID] = true
	}
	for _, x := range v.Cases {
		if blank(x.ID, x.Name, x.ExpectedOutcome) || len(x.Assumptions) == 0 {
			return false
		}
		for k := range x.Values {
			if !params[k] {
				return false
			}
		}
	}
	return true
}
func steps(v []Step, p map[string]bool) bool {
	seen := map[string]bool{}
	for _, x := range v {
		if blank(x.ID, x.Description, x.Operation) || seen[x.ID] {
			return false
		}
		seen[x.ID] = true
		for _, q := range x.Parameters {
			if !p[q] {
				return false
			}
		}
	}
	return true
}
func blank(v ...string) bool {
	for _, x := range v {
		if strings.TrimSpace(x) == "" || len(x) > 4096 {
			return true
		}
	}
	return false
}
func one(v string, x ...string) bool {
	for _, q := range x {
		if v == q {
			return true
		}
	}
	return false
}
func hexText(v string) bool            { _, e := hex.DecodeString(v); return e == nil }
func id() string                       { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func (s *Store) path(id string) string { return filepath.Join(s.root, id+".json") }
func (s *Store) write(v Scenario) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(s.root, ".scenario-*")
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
func (s *Store) read(id string) (Scenario, error) {
	var v Scenario
	if strings.ContainsAny(id, "/\\") {
		return v, ErrNotFound
	}
	b, e := os.ReadFile(s.path(id))
	if os.IsNotExist(e) {
		return v, ErrNotFound
	}
	if e != nil {
		return v, e
	}
	if json.Unmarshal(b, &v) != nil || v.ID != id || !Valid(v) {
		return Scenario{}, ErrInvalid
	}
	return v, nil
}
func (s *Store) Lock(fn func() error) error {
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
