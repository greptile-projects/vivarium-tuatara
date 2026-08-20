// Package agentprojects retains reviewable, revision-exact agent intent and boundaries.
package agentprojects

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

var ErrNotFound = errors.New("agent project not found")
var ErrInvalid = errors.New("invalid agent project")
var ErrConflict = errors.New("agent project version conflict")

type Source struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	RepositoryID string `json:"repository_id"`
	Revision     string `json:"revision"`
	Path         string `json:"path"`
	Purpose      string `json:"purpose"`
}
type Tool struct {
	Name     string   `json:"name"`
	Purpose  string   `json:"purpose"`
	Actions  []string `json:"actions"`
	Boundary string   `json:"boundary"`
}
type Model struct {
	Provider string `json:"provider"`
	Name     string `json:"name"`
	Version  string `json:"version"`
	Purpose  string `json:"purpose"`
}
type Budget struct {
	MaxCostUSD        float64 `json:"max_cost_usd"`
	MaxTokens         int     `json:"max_tokens"`
	MaxToolActions    int     `json:"max_tool_actions"`
	MaxRuntimeSeconds int     `json:"max_runtime_seconds"`
}
type Escalation struct {
	Trigger  string   `json:"trigger"`
	OwnerIDs []string `json:"owner_ids"`
	Action   string   `json:"action"`
}
type DeploymentBoundary struct {
	Environment      string   `json:"environment"`
	RepositoryAccess string   `json:"repository_access"`
	NetworkAccess    string   `json:"network_access"`
	DataClasses      []string `json:"data_classes"`
	ApprovalRequired bool     `json:"approval_required"`
}
type Revision struct {
	Version              int                  `json:"version"`
	Title                string               `json:"title"`
	Purpose              string               `json:"purpose"`
	OwnerIDs             []string             `json:"owner_ids"`
	Sources              []Source             `json:"sources"`
	Tools                []Tool               `json:"tools"`
	Models               []Model              `json:"models"`
	SupportedTasks       []string             `json:"supported_tasks"`
	ExpectedOutputs      []string             `json:"expected_outputs"`
	ProhibitedActions    []string             `json:"prohibited_actions"`
	MemoryPolicy         string               `json:"memory_policy"`
	DataUseTerms         string               `json:"data_use_terms"`
	Guarantees           []string             `json:"guarantees"`
	Budget               Budget               `json:"budget"`
	Escalations          []Escalation         `json:"escalations"`
	DeploymentBoundaries []DeploymentBoundary `json:"deployment_boundaries"`
	ChangeSummary        string               `json:"change_summary"`
	CreatedBy            string               `json:"created_by"`
	CreatedAt            time.Time            `json:"created_at"`
}
type Diagnostic struct {
	Kind         string `json:"kind"`
	Severity     string `json:"severity"`
	Message      string `json:"message"`
	SourceID     string `json:"source_id,omitempty"`
	AttributedTo string `json:"attributed_to"`
}
type EffectiveCapability struct {
	Tasks                   []string `json:"tasks"`
	Tools                   []string `json:"tools"`
	Stops                   []string `json:"stops"`
	HumanEscalationRequired bool     `json:"human_escalation_required"`
}
type Project struct {
	ID                  string              `json:"id"`
	RepositoryID        string              `json:"repository_id"`
	CurrentVersion      int                 `json:"current_version"`
	Revisions           []Revision          `json:"revisions"`
	Diagnostics         []Diagnostic        `json:"diagnostics"`
	EffectiveCapability EffectiveCapability `json:"effective_capability"`
	CreatedAt           time.Time           `json:"created_at"`
	UpdatedAt           time.Time           `json:"updated_at"`
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
func (s *Store) Create(repo, actor string, r Revision) (Project, error) {
	var out Project
	err := s.lock(func() error {
		if !valid(repo, r) {
			return ErrInvalid
		}
		now := s.now()
		stamp(&r, 1, actor, now)
		out = Project{ID: id(), RepositoryID: repo, CurrentVersion: 1, Revisions: []Revision{r}, CreatedAt: now, UpdatedAt: now}
		return s.write(out)
	})
	return project(out), err
}
func (s *Store) Revise(pid string, expected int, actor string, r Revision) (Project, error) {
	var out Project
	err := s.lock(func() error {
		p, e := s.read(pid)
		if e != nil {
			return e
		}
		if p.CurrentVersion != expected {
			return ErrConflict
		}
		if !valid(p.RepositoryID, r) {
			return ErrInvalid
		}
		stamp(&r, expected+1, actor, s.now())
		p.CurrentVersion = r.Version
		p.Revisions = append(p.Revisions, r)
		p.UpdatedAt = r.CreatedAt
		out = p
		return s.write(p)
	})
	return project(out), err
}
func (s *Store) Get(id string) (Project, error) {
	var out Project
	err := s.lock(func() error { var e error; out, e = s.read(id); return e })
	return project(out), err
}
func (s *Store) List(repo string) ([]Project, error) {
	out := []Project{}
	err := s.lock(func() error {
		es, e := os.ReadDir(s.root)
		if e != nil {
			return e
		}
		for _, x := range es {
			if x.IsDir() || !strings.HasSuffix(x.Name(), ".json") {
				continue
			}
			p, e := s.read(strings.TrimSuffix(x.Name(), ".json"))
			if e != nil {
				return e
			}
			if p.RepositoryID == repo {
				out = append(out, project(p))
			}
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, err
}
func stamp(r *Revision, v int, actor string, now time.Time) {
	r.Version = v
	r.CreatedBy = actor
	r.CreatedAt = now
}
func valid(repo string, r Revision) bool {
	if r.Title == "" || r.Purpose == "" || r.ChangeSummary == "" || len(r.Sources) == 0 || len(r.Models) == 0 || len(r.SupportedTasks) == 0 || len(r.ExpectedOutputs) == 0 || len(r.ProhibitedActions) == 0 || r.MemoryPolicy == "" || r.DataUseTerms == "" || r.Budget.MaxCostUSD < 0 || r.Budget.MaxTokens < 1 || r.Budget.MaxToolActions < 0 || r.Budget.MaxRuntimeSeconds < 1 {
		return false
	}
	seen := map[string]bool{}
	for _, x := range r.Sources {
		if x.ID == "" || seen[x.ID] || !one(x.Kind, "prompt", "instructions", "knowledge", "dependency") || x.RepositoryID == "" || !revision(x.Revision) || x.Path == "" || x.Purpose == "" {
			return false
		}
		seen[x.ID] = true
	}
	for _, x := range r.Tools {
		if x.Name == "" || x.Purpose == "" || x.Boundary == "" || len(x.Actions) == 0 {
			return false
		}
	}
	for _, x := range r.Models {
		if x.Provider == "" || x.Name == "" || x.Version == "" || x.Purpose == "" {
			return false
		}
	}
	for _, x := range r.Escalations {
		if x.Trigger == "" || x.Action == "" {
			return false
		}
	}
	for _, x := range r.DeploymentBoundaries {
		if x.Environment == "" || x.RepositoryAccess == "" || x.NetworkAccess == "" {
			return false
		}
	}
	return repo != ""
}
func project(p Project) Project {
	if len(p.Revisions) == 0 {
		return p
	}
	r := p.Revisions[len(p.Revisions)-1]
	d := []Diagnostic{}
	if len(r.OwnerIDs) == 0 {
		d = append(d, Diagnostic{"missing_owner", "blocking", "No accountable project owner is named.", "", r.CreatedBy})
	}
	for _, e := range r.Escalations {
		if len(e.OwnerIDs) == 0 {
			d = append(d, Diagnostic{"missing_owner", "blocking", "An escalation rule has no human owner.", "", r.CreatedBy})
		}
	}
	for _, g := range r.Guarantees {
		d = append(d, Diagnostic{"unsupported_guarantee", "warning", "Guarantee requires evaluation evidence: " + g, "", r.CreatedBy})
	}
	for _, a := range r.ProhibitedActions {
		for _, t := range r.Tools {
			for _, allowed := range t.Actions {
				if strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(allowed)) {
					d = append(d, Diagnostic{"conflicting_instruction", "blocking", "A tool action is also explicitly prohibited: " + a, "", r.CreatedBy})
				}
			}
		}
	}
	tools := []string{}
	for _, t := range r.Tools {
		tools = append(tools, t.Name+": "+strings.Join(t.Actions, ", "))
	}
	p.Diagnostics = d
	p.EffectiveCapability = EffectiveCapability{Tasks: append([]string(nil), r.SupportedTasks...), Tools: tools, Stops: append([]string(nil), r.ProhibitedActions...), HumanEscalationRequired: len(r.Escalations) > 0}
	return p
}
func one(v string, xs ...string) bool {
	for _, x := range xs {
		if v == x {
			return true
		}
	}
	return false
}
func revision(v string) bool {
	if len(v) != 40 {
		return false
	}
	_, e := hex.DecodeString(v)
	return e == nil
}
func id() string                      { b := make([]byte, 12); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func (s *Store) path(v string) string { return filepath.Join(s.root, v+".json") }
func (s *Store) read(v string) (Project, error) {
	if strings.ContainsAny(v, "/\\") {
		return Project{}, ErrNotFound
	}
	b, e := os.ReadFile(s.path(v))
	if os.IsNotExist(e) {
		return Project{}, ErrNotFound
	}
	if e != nil {
		return Project{}, e
	}
	var p Project
	if json.Unmarshal(b, &p) != nil || p.ID != v {
		return Project{}, ErrInvalid
	}
	return p, nil
}
func (s *Store) write(p Project) error {
	b, e := json.MarshalIndent(p, "", "  ")
	if e != nil {
		return e
	}
	tmp := s.path(p.ID) + ".tmp-" + id()
	f, e := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	remove := true
	defer func() {
		if remove {
			_ = os.Remove(tmp)
		}
	}()
	if _, e = f.Write(b); e != nil {
		_ = f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		_ = f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	if e = os.Rename(tmp, s.path(p.ID)); e != nil {
		return e
	}
	remove = false
	directory, e := os.Open(s.root)
	if e != nil {
		return e
	}
	if e = directory.Sync(); e != nil {
		_ = directory.Close()
		return e
	}
	if e = directory.Close(); e != nil {
		return e
	}
	return nil
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
