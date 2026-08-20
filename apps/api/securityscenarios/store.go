// Package securityscenarios retains reviewed abuse/defense specifications and exact-run evidence.
package securityscenarios

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
	"time"
)

var ErrNotFound = errors.New("security scenario not found")
var ErrInvalid = errors.New("invalid security scenario")
var ErrConflict = errors.New("security scenario conflict")

type Fixture struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Path        string `json:"path"`
	SHA256      string `json:"sha256"`
	DataClass   string `json:"data_class"`
	Generator   string `json:"generator"`
}
type Criterion struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Observable  string `json:"observable"`
	Expected    string `json:"expected"`
}
type Scenario struct {
	ID                    string      `json:"id"`
	RepositoryID          string      `json:"repository_id"`
	ThreatModelID         string      `json:"threat_model_id"`
	ThreatModelVersion    int         `json:"threat_model_version"`
	AbusePathID           string      `json:"abuse_path_id"`
	Title                 string      `json:"title"`
	AttackerPreconditions []string    `json:"attacker_preconditions"`
	BoundedCapabilities   []string    `json:"bounded_capabilities"`
	Fixtures              []Fixture   `json:"fixtures"`
	Actions               []string    `json:"actions"`
	Containment           []Criterion `json:"containment"`
	Detection             []Criterion `json:"detection"`
	Recovery              []Criterion `json:"recovery"`
	MitigationIDs         []string    `json:"mitigation_ids"`
	DependencyIDs         []string    `json:"dependency_ids"`
	CommitID              string      `json:"commit_id"`
	CheckPath             string      `json:"check_path"`
	CheckSHA256           string      `json:"check_sha256"`
	Command               string      `json:"command"`
	Isolation             string      `json:"isolation"`
	MaxCostUnits          float64     `json:"max_cost_units"`
	CreatedBy             string      `json:"created_by"`
	CreatedByType         string      `json:"created_by_type"`
	CreatedAt             time.Time   `json:"created_at"`
	Review                *Review     `json:"review,omitempty"`
	Attempts              []Attempt   `json:"attempts"`
}
type Review struct {
	ReviewerID string    `json:"reviewer_id"`
	Decision   string    `json:"decision"`
	Note       string    `json:"note"`
	CreatedAt  time.Time `json:"created_at"`
}
type Command struct {
	OutcomeID   string    `json:"outcome_id"`
	SHA256      string    `json:"sha256"`
	Directory   string    `json:"directory"`
	ExitCode    int       `json:"exit_code"`
	Log         string    `json:"log"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
}
type Artifact struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	SHA256    string `json:"sha256"`
	Size      int64  `json:"size"`
	Sanitized bool   `json:"sanitized"`
}
type Coverage struct {
	AbuseAttempted bool     `json:"abuse_attempted"`
	ContainmentIDs []string `json:"containment_ids"`
	DetectionIDs   []string `json:"detection_ids"`
	RecoveryIDs    []string `json:"recovery_ids"`
	Gaps           []string `json:"gaps"`
}
type Attempt struct {
	ID                     string     `json:"id"`
	Revision               string     `json:"revision"`
	ExecutionKind          string     `json:"execution_kind"`
	WorkspaceID            string     `json:"workspace_id,omitempty"`
	PreviewID              string     `json:"preview_id,omitempty"`
	Commands               []Command  `json:"commands"`
	Artifacts              []Artifact `json:"artifacts"`
	Coverage               Coverage   `json:"coverage"`
	CostUnits              float64    `json:"cost_units"`
	Result                 string     `json:"result"`
	UnsafeReasons          []string   `json:"unsafe_reasons"`
	NonReproducibleReasons []string   `json:"non_reproducible_reasons"`
	Provenance             []string   `json:"provenance"`
	ActorID                string     `json:"actor_id"`
	CreatedAt              time.Time  `json:"created_at"`
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
func (s *Store) Create(repo, actor, actorType string, v Scenario) (Scenario, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !Valid(v) {
		return Scenario{}, ErrInvalid
	}
	v.ID = id()
	v.RepositoryID = repo
	v.CreatedBy = actor
	v.CreatedByType = actorType
	v.CreatedAt = s.now()
	v.Review = nil
	v.Attempts = []Attempt{}
	return v, s.write(v)
}
func (s *Store) Get(repo, idv string) (Scenario, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(idv) != 32 || !hexText(idv) {
		return Scenario{}, ErrNotFound
	}
	v, e := s.read(idv)
	if e == nil && v.RepositoryID != repo {
		return Scenario{}, ErrNotFound
	}
	return v, e
}
func (s *Store) List(repo string) ([]Scenario, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	es, e := os.ReadDir(s.root)
	if e != nil {
		return nil, e
	}
	out := []Scenario{}
	for _, x := range es {
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
func (s *Store) Review(repo, idv, reviewer, decision, note string) (Scenario, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, e := s.read(idv)
	if e != nil || v.RepositoryID != repo {
		return Scenario{}, ErrNotFound
	}
	if v.Review != nil {
		return Scenario{}, ErrConflict
	}
	if decision != "approved" && decision != "changes_requested" || strings.TrimSpace(note) == "" {
		return Scenario{}, ErrInvalid
	}
	v.Review = &Review{reviewer, decision, note, s.now()}
	return v, s.write(v)
}
func (s *Store) AddAttempt(repo, idv, actor string, a Attempt) (Scenario, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, e := s.read(idv)
	if e != nil || v.RepositoryID != repo {
		return Scenario{}, ErrNotFound
	}
	if v.Review == nil || v.Review.Decision != "approved" || !validAttempt(v, a) {
		return Scenario{}, ErrInvalid
	}
	a.ID = id()
	a.ActorID = actor
	a.CreatedAt = s.now()
	v.Attempts = append(v.Attempts, a)
	return v, s.write(v)
}
func Valid(v Scenario) bool {
	return text(v.ThreatModelID, v.AbusePathID, v.Title, v.CommitID, v.CheckPath, v.CheckSHA256, v.Command) && v.ThreatModelVersion > 0 && len(v.CommitID) == 40 && hexText(v.CommitID) && len(v.CheckSHA256) == 64 && hexText(v.CheckSHA256) && one(v.Isolation, "workspace", "preview") && v.MaxCostUnits > 0 && len(v.AttackerPreconditions) > 0 && len(v.BoundedCapabilities) > 0 && len(v.Actions) > 0 && len(v.Containment) > 0 && len(v.Detection) > 0 && len(v.Recovery) > 0 && len(v.MitigationIDs) > 0 && safeFixtures(v.Fixtures) && criteria(v.Containment) && criteria(v.Detection) && criteria(v.Recovery)
}
func validAttempt(s Scenario, a Attempt) bool {
	if a.Revision != s.CommitID || !one(a.ExecutionKind, "workspace", "preview") || !one(a.Result, "passed", "failed", "unsafe", "not_reproducible") || a.CostUnits < 0 || a.CostUnits > s.MaxCostUnits || len(a.Provenance) == 0 {
		return false
	}
	for _, x := range a.Artifacts {
		if !text(x.Kind, x.Name, x.SHA256) || len(x.SHA256) != 64 || !hexText(x.SHA256) || x.Size < 0 || !x.Sanitized || !one(x.Kind, "log", "trace", "recording", "report", "coverage") {
			return false
		}
	}
	if !criterionIDs(a.Coverage.ContainmentIDs, s.Containment) || !criterionIDs(a.Coverage.DetectionIDs, s.Detection) || !criterionIDs(a.Coverage.RecoveryIDs, s.Recovery) {
		return false
	}
	if a.Result == "unsafe" {
		return len(a.UnsafeReasons) > 0
	}
	if a.Result == "not_reproducible" {
		return len(a.NonReproducibleReasons) > 0
	}
	return len(a.Commands) > 0 && a.Coverage.AbuseAttempted
}
func criterionIDs(ids []string, criteria []Criterion) bool {
	for _, id := range ids {
		found := false
		for _, c := range criteria {
			found = found || c.ID == id
		}
		if !found {
			return false
		}
	}
	return true
}
func safeFixtures(v []Fixture) bool {
	if len(v) == 0 {
		return false
	}
	for _, x := range v {
		if !text(x.ID, x.Description, x.Path, x.SHA256, x.DataClass, x.Generator) || len(x.SHA256) != 64 || !hexText(x.SHA256) || !one(x.DataClass, "synthetic", "anonymized", "public") {
			return false
		}
	}
	return true
}
func criteria(v []Criterion) bool {
	seen := map[string]bool{}
	for _, x := range v {
		if !text(x.ID, x.Description, x.Observable, x.Expected) || seen[x.ID] {
			return false
		}
		seen[x.ID] = true
	}
	return true
}
func text(v ...string) bool {
	for _, x := range v {
		if strings.TrimSpace(x) == "" || len(x) > 4096 {
			return false
		}
	}
	return true
}
func one(v string, x ...string) bool {
	for _, q := range x {
		if v == q {
			return true
		}
	}
	return false
}
func hexText(v string) bool { _, e := hex.DecodeString(v); return e == nil }
func (s *Store) read(i string) (Scenario, error) {
	var v Scenario
	if len(i) != 32 || !hexText(i) {
		return v, ErrNotFound
	}
	b, e := os.ReadFile(filepath.Join(s.root, i+".json"))
	if os.IsNotExist(e) {
		return v, ErrNotFound
	}
	if e != nil {
		return v, e
	}
	if json.Unmarshal(b, &v) != nil || v.ID != i {
		return v, ErrInvalid
	}
	return v, nil
}
func (s *Store) write(v Scenario) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(s.root, ".security-*")
	if e != nil {
		return e
	}
	name := tmp.Name()
	defer os.Remove(name)
	_ = tmp.Chmod(0600)
	if _, e = tmp.Write(b); e == nil {
		e = tmp.Sync()
	}
	if c := tmp.Close(); e == nil {
		e = c
	}
	if e == nil {
		e = os.Rename(name, filepath.Join(s.root, v.ID+".json"))
	}
	return e
}
func id() string { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
