// Package agentcandidates retains exact-pull agent candidates and evaluation evidence.
package agentcandidates

import (
	"crypto/rand"
	"crypto/sha256"
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

var ErrInvalid = errors.New("invalid agent candidate")
var ErrNotFound = errors.New("agent candidate not found")
var ErrConflict = errors.New("agent candidate conflict")

type SuiteSelection struct {
	SuiteID string `json:"suite_id"`
	Version int    `json:"version"`
	Digest  string `json:"digest"`
}
type ComponentDigest struct {
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Digest string `json:"digest"`
}
type Candidate struct {
	ID                  string            `json:"id"`
	RepositoryID        string            `json:"repository_id"`
	PullRequestID       string            `json:"pull_request_id"`
	PullRevision        string            `json:"pull_revision"`
	ProjectID           string            `json:"project_id"`
	ProjectVersion      int               `json:"project_version"`
	ContractDigest      string            `json:"contract_digest"`
	Components          []ComponentDigest `json:"components"`
	Suites              []SuiteSelection  `json:"suites"`
	BaselineCandidateID string            `json:"baseline_candidate_id,omitempty"`
	CreatedBy           string            `json:"created_by"`
	CreatedAt           time.Time         `json:"created_at"`
}
type ToolAction struct {
	Tool         string `json:"tool"`
	Action       string `json:"action"`
	InputDigest  string `json:"input_digest"`
	OutputDigest string `json:"output_digest"`
	DurationMS   int64  `json:"duration_ms"`
	Denied       bool   `json:"denied"`
}
type Artifact struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}
type ScenarioResult struct {
	ScenarioID        string       `json:"scenario_id"`
	Attempt           int          `json:"attempt"`
	TaskSuccess       bool         `json:"task_success"`
	PolicyAdherence   bool         `json:"policy_adherence"`
	HumanCorrections  int          `json:"human_corrections"`
	Uncertainty       float64      `json:"uncertainty"`
	LatencyMS         int64        `json:"latency_ms"`
	Cost              float64      `json:"cost"`
	TraceDigest       string       `json:"trace_digest"`
	OutputDigest      string       `json:"output_digest"`
	ToolActions       []ToolAction `json:"tool_actions"`
	Artifacts         []Artifact   `json:"artifacts"`
	EvaluatorDecision string       `json:"evaluator_decision"`
	EvaluatorID       string       `json:"evaluator_id"`
}
type StatisticalLimits struct {
	ConfidenceLevel float64 `json:"confidence_level"`
	MinimumSamples  int     `json:"minimum_samples"`
	MarginOfError   float64 `json:"margin_of_error"`
}
type Run struct {
	ID                   string            `json:"id"`
	CandidateID          string            `json:"candidate_id"`
	SuiteID              string            `json:"suite_id"`
	SuiteVersion         int               `json:"suite_version"`
	SuiteDigest          string            `json:"suite_digest"`
	Isolation            string            `json:"isolation"`
	Network              string            `json:"network"`
	PermittedServices    []string          `json:"permitted_services"`
	MaxToolActions       int               `json:"max_tool_actions"`
	MaxCost              float64           `json:"max_cost"`
	MaxLatencyMS         int64             `json:"max_latency_ms"`
	Results              []ScenarioResult  `json:"results"`
	Limits               StatisticalLimits `json:"statistical_limits"`
	Contaminated         bool              `json:"contaminated"`
	ContaminationReasons []string          `json:"contamination_reasons,omitempty"`
	Nondeterministic     bool              `json:"nondeterministic"`
	CreatedBy            string            `json:"created_by"`
	CreatedAt            time.Time         `json:"created_at"`
}
type Metrics struct {
	Samples             int     `json:"samples"`
	TaskSuccessRate     float64 `json:"task_success_rate"`
	PolicyAdherenceRate float64 `json:"policy_adherence_rate"`
	HumanCorrections    float64 `json:"human_corrections"`
	MeanUncertainty     float64 `json:"mean_uncertainty"`
	MeanLatencyMS       float64 `json:"mean_latency_ms"`
	MeanCost            float64 `json:"mean_cost"`
}
type Comparison struct {
	BaselineCandidateID string              `json:"baseline_candidate_id,omitempty"`
	ComparableSuites    []string            `json:"comparable_suites"`
	InvalidatedSuites   []string            `json:"invalidated_suites"`
	Candidate           Metrics             `json:"candidate"`
	Baseline            Metrics             `json:"baseline"`
	Delta               Metrics             `json:"delta"`
	Contaminated        bool                `json:"contaminated"`
	Nondeterministic    bool                `json:"nondeterministic"`
	StatisticalLimits   []StatisticalLimits `json:"statistical_limits"`
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
func uid() string { var b [16]byte; _, _ = rand.Read(b[:]); return hex.EncodeToString(b[:]) }
func Digest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func clean(v string, n int) bool {
	return strings.TrimSpace(v) != "" && len(v) <= n && !strings.ContainsRune(v, '\x00')
}
func (s *Store) path(kind, id string) string { return filepath.Join(s.root, kind+"-"+id+".json") }
func write(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	tmp := path + ".tmp"
	if e = os.WriteFile(tmp, b, 0600); e != nil {
		return e
	}
	return os.Rename(tmp, path)
}
func read[T any](path string) (T, error) {
	var v T
	b, e := os.ReadFile(path)
	if os.IsNotExist(e) {
		return v, ErrNotFound
	}
	if e != nil {
		return v, e
	}
	e = json.Unmarshal(b, &v)
	return v, e
}
func (s *Store) Create(c Candidate) (Candidate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !clean(c.RepositoryID, 64) || !clean(c.PullRequestID, 64) || len(c.PullRevision) != 40 || !clean(c.ProjectID, 64) || c.ProjectVersion < 1 || len(c.ContractDigest) != 64 || len(c.Components) == 0 || len(c.Suites) == 0 {
		return Candidate{}, ErrInvalid
	}
	for _, x := range c.Suites {
		if !clean(x.SuiteID, 64) || x.Version < 1 || len(x.Digest) != 64 {
			return Candidate{}, ErrInvalid
		}
	}
	c.ID = uid()
	c.CreatedAt = s.now()
	return c, write(s.path("candidate", c.ID), c)
}
func (s *Store) Get(id string) (Candidate, error) { return read[Candidate](s.path("candidate", id)) }
func (s *Store) List(repo, pull string) ([]Candidate, error) {
	es, e := os.ReadDir(s.root)
	if e != nil {
		return nil, e
	}
	out := []Candidate{}
	for _, x := range es {
		if !strings.HasPrefix(x.Name(), "candidate-") {
			continue
		}
		v, er := read[Candidate](filepath.Join(s.root, x.Name()))
		if er != nil {
			return nil, er
		}
		if v.RepositoryID == repo && v.PullRequestID == pull {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}
func (s *Store) CreateRun(r Run) (Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, e := read[Candidate](s.path("candidate", r.CandidateID))
	if e != nil {
		return Run{}, e
	}
	matched := false
	for _, x := range c.Suites {
		if x.SuiteID == r.SuiteID && x.Version == r.SuiteVersion && x.Digest == r.SuiteDigest {
			matched = true
		}
	}
	if !matched || r.Isolation != "ephemeral" || !slicesOne(r.Network, "none", "simulated", "permitted") || r.MaxToolActions < 1 || r.MaxCost < 0 || r.MaxLatencyMS < 1 || len(r.Results) == 0 || r.Limits.ConfidenceLevel <= 0 || r.Limits.ConfidenceLevel >= 1 || r.Limits.MinimumSamples < 1 || r.Limits.MarginOfError < 0 {
		return Run{}, ErrInvalid
	}
	if r.Network == "none" && len(r.PermittedServices) != 0 {
		return Run{}, ErrInvalid
	}
	for _, service := range r.PermittedServices {
		if !clean(service, 200) {
			return Run{}, ErrInvalid
		}
	}
	counts := map[string]int{}
	outputs := map[string]string{}
	r.Nondeterministic = false
	for _, x := range r.Results {
		if !clean(x.ScenarioID, 100) || x.Attempt < 1 || x.Uncertainty < 0 || x.Uncertainty > 1 || x.LatencyMS < 0 || x.Cost < 0 || len(x.TraceDigest) != 64 || len(x.OutputDigest) != 64 || !slicesOne(x.EvaluatorDecision, "passed", "failed", "needs_human") || !clean(x.EvaluatorID, 64) {
			return Run{}, ErrInvalid
		}
		counts[x.ScenarioID]++
		if prior, ok := outputs[x.ScenarioID]; ok && prior != x.OutputDigest {
			r.Nondeterministic = true
		}
		outputs[x.ScenarioID] = x.OutputDigest
		if len(x.ToolActions) > r.MaxToolActions || x.Cost > r.MaxCost || x.LatencyMS > r.MaxLatencyMS {
			return Run{}, ErrInvalid
		}
		for _, action := range x.ToolActions {
			if !clean(action.Tool, 100) || !clean(action.Action, 200) || len(action.InputDigest) != 64 || len(action.OutputDigest) != 64 || action.DurationMS < 0 {
				return Run{}, ErrInvalid
			}
		}
		for _, a := range x.Artifacts {
			if !clean(a.Name, 300) || len(a.SHA256) != 64 || a.Size < 0 {
				return Run{}, ErrInvalid
			}
		}
	}
	for _, n := range counts {
		if n < r.Limits.MinimumSamples {
			return Run{}, ErrInvalid
		}
	}
	r.Contaminated = len(r.ContaminationReasons) > 0
	r.ID = uid()
	r.CreatedAt = s.now()
	return r, write(s.path("run", r.ID), r)
}
func slicesOne(v string, x ...string) bool {
	for _, a := range x {
		if v == a {
			return true
		}
	}
	return false
}
func (s *Store) Runs(candidate string) ([]Run, error) {
	es, e := os.ReadDir(s.root)
	if e != nil {
		return nil, e
	}
	out := []Run{}
	for _, x := range es {
		if !strings.HasPrefix(x.Name(), "run-") {
			continue
		}
		v, er := read[Run](filepath.Join(s.root, x.Name()))
		if er != nil {
			return nil, er
		}
		if v.CandidateID == candidate {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}
func metrics(rs []Run, allowed map[string]bool) Metrics {
	var m Metrics
	for _, r := range rs {
		if !allowed[r.SuiteID] || r.Contaminated {
			continue
		}
		for _, x := range r.Results {
			m.Samples++
			if x.TaskSuccess {
				m.TaskSuccessRate++
			}
			if x.PolicyAdherence {
				m.PolicyAdherenceRate++
			}
			m.HumanCorrections += float64(x.HumanCorrections)
			m.MeanUncertainty += x.Uncertainty
			m.MeanLatencyMS += float64(x.LatencyMS)
			m.MeanCost += x.Cost
		}
	}
	if m.Samples > 0 {
		n := float64(m.Samples)
		m.TaskSuccessRate /= n
		m.PolicyAdherenceRate /= n
		m.HumanCorrections /= n
		m.MeanUncertainty /= n
		m.MeanLatencyMS /= n
		m.MeanCost /= n
	}
	return m
}
func (s *Store) Compare(c Candidate) (Comparison, error) {
	cr, e := s.Runs(c.ID)
	if e != nil {
		return Comparison{}, e
	}
	out := Comparison{BaselineCandidateID: c.BaselineCandidateID}
	allowed := map[string]bool{}
	for _, x := range c.Suites {
		allowed[x.SuiteID] = true
	}
	var br []Run
	if c.BaselineCandidateID != "" {
		b, e := s.Get(c.BaselineCandidateID)
		if e != nil || b.RepositoryID != c.RepositoryID {
			return Comparison{}, ErrInvalid
		}
		br, e = s.Runs(b.ID)
		if e != nil {
			return Comparison{}, e
		}
		base := map[string]string{}
		for _, x := range b.Suites {
			base[x.SuiteID] = x.Digest
		}
		for _, x := range c.Suites {
			if base[x.SuiteID] == x.Digest {
				out.ComparableSuites = append(out.ComparableSuites, x.SuiteID)
			} else {
				out.InvalidatedSuites = append(out.InvalidatedSuites, x.SuiteID)
				allowed[x.SuiteID] = false
			}
		}
	}
	out.Candidate = metrics(cr, allowed)
	out.Baseline = metrics(br, allowed)
	out.Delta = Metrics{Samples: out.Candidate.Samples - out.Baseline.Samples, TaskSuccessRate: out.Candidate.TaskSuccessRate - out.Baseline.TaskSuccessRate, PolicyAdherenceRate: out.Candidate.PolicyAdherenceRate - out.Baseline.PolicyAdherenceRate, HumanCorrections: out.Candidate.HumanCorrections - out.Baseline.HumanCorrections, MeanUncertainty: out.Candidate.MeanUncertainty - out.Baseline.MeanUncertainty, MeanLatencyMS: out.Candidate.MeanLatencyMS - out.Baseline.MeanLatencyMS, MeanCost: out.Candidate.MeanCost - out.Baseline.MeanCost}
	for _, r := range append(cr, br...) {
		out.Contaminated = out.Contaminated || r.Contaminated
		out.Nondeterministic = out.Nondeterministic || r.Nondeterministic
		out.StatisticalLimits = append(out.StatisticalLimits, r.Limits)
	}
	return out, nil
}
