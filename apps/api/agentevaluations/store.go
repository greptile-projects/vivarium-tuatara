// Package agentevaluations retains bounded, revision-exact approved-agent trials.
package agentevaluations

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

var ErrInvalid = errors.New("invalid agent evaluation")
var ErrNotFound = errors.New("agent evaluation not found")
var ErrConflict = errors.New("agent evaluation version conflict")

type Check struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Expected string `json:"expected,omitempty"`
}
type Budget struct {
	MaxCost        float64 `json:"max_cost"`
	MaxLatencyMS   int64   `json:"max_latency_ms"`
	MaxToolActions int     `json:"max_tool_actions"`
}
type Scenario struct {
	ID               string   `json:"id"`
	Title            string   `json:"title"`
	SanitizedPrompt  string   `json:"sanitized_prompt"`
	ExpectedOutcomes []string `json:"expected_outcomes"`
	Checks           []Check  `json:"checks"`
	HiddenChecks     []Check  `json:"hidden_checks,omitempty"`
}
type Revision struct {
	Version             int        `json:"version"`
	RepositoryRevision  string     `json:"repository_revision"`
	Scenarios           []Scenario `json:"scenarios"`
	Budget              Budget     `json:"budget"`
	ProhibitedActions   []string   `json:"prohibited_actions"`
	HumanReviewCriteria []string   `json:"human_review_criteria"`
	ChangeSummary       string     `json:"change_summary"`
	CreatedBy           string     `json:"created_by"`
	CreatedAt           time.Time  `json:"created_at"`
}
type Suite struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"organization_id"`
	RepositoryID   string     `json:"repository_id"`
	Name           string     `json:"name"`
	Revisions      []Revision `json:"revisions"`
}
type Artifact struct {
	Name    string `json:"name"`
	SHA256  string `json:"sha256"`
	Size    int64  `json:"size"`
	Summary string `json:"summary"`
}
type ToolAction struct {
	Tool          string `json:"tool"`
	Action        string `json:"action"`
	InputSummary  string `json:"input_summary"`
	OutputSummary string `json:"output_summary"`
	DurationMS    int64  `json:"duration_ms"`
	Failed        bool   `json:"failed"`
}
type Authority struct {
	Publish      bool `json:"publish"`
	Secrets      bool `json:"secrets"`
	Merge        bool `json:"merge"`
	Environments bool `json:"environments"`
	Network      bool `json:"network"`
}
type CheckResult struct {
	ScenarioID string `json:"scenario_id"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Passed     bool   `json:"passed"`
	Hidden     bool   `json:"hidden"`
	Summary    string `json:"summary"`
}
type Decision struct {
	Decision    string    `json:"decision"`
	Rationale   string    `json:"rationale"`
	EvaluatorID string    `json:"evaluator_id"`
	CreatedAt   time.Time `json:"created_at"`
}
type Run struct {
	ID                   string            `json:"id"`
	SuiteID              string            `json:"suite_id"`
	SuiteVersion         int               `json:"suite_version"`
	OrganizationID       string            `json:"organization_id"`
	RepositoryID         string            `json:"repository_id"`
	RepositoryRevision   string            `json:"repository_revision"`
	AgentID              string            `json:"agent_id"`
	AgentProfileVersion  int               `json:"agent_profile_version"`
	TrialLabel           string            `json:"trial_label"`
	ReproducesRunID      string            `json:"reproduces_run_id,omitempty"`
	InputDigest          string            `json:"input_digest"`
	Outputs              map[string]string `json:"outputs"`
	ToolActions          []ToolAction      `json:"tool_actions"`
	Artifacts            []Artifact        `json:"artifacts"`
	Cost                 float64           `json:"cost"`
	LatencyMS            int64             `json:"latency_ms"`
	Failure              string            `json:"failure,omitempty"`
	Authority            Authority         `json:"authority"`
	CheckResults         []CheckResult     `json:"check_results"`
	CorrectnessPassed    bool              `json:"correctness_passed"`
	PolicyPassed         bool              `json:"policy_passed"`
	BudgetPassed         bool              `json:"budget_passed"`
	Contaminated         bool              `json:"contaminated"`
	ContaminationReasons []string          `json:"contamination_reasons"`
	Reproducible         bool              `json:"reproducible"`
	ReviewStatus         string            `json:"review_status"`
	Decisions            []Decision        `json:"decisions"`
	CreatedBy            string            `json:"created_by"`
	CreatedAt            time.Time         `json:"created_at"`
}
type RunInput struct {
	AgentID             string            `json:"agent_id"`
	AgentProfileVersion int               `json:"agent_profile_version"`
	OperatorSupplied    bool              `json:"operator_supplied"`
	ReproducesRunID     string            `json:"reproduces_run_id"`
	Outputs             map[string]string `json:"outputs"`
	ToolActions         []ToolAction      `json:"tool_actions"`
	Artifacts           []Artifact        `json:"artifacts"`
	Cost                float64           `json:"cost"`
	LatencyMS           int64             `json:"latency_ms"`
	Failure             string            `json:"failure"`
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
func id() (string, error) {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		return "", e
	}
	return hex.EncodeToString(b[:]), nil
}
func clean(v string, max int) bool {
	v = strings.TrimSpace(v)
	return v != "" && len(v) <= max && !strings.ContainsRune(v, '\x00')
}
func sanitized(v string) bool {
	x := strings.ToLower(v)
	for _, marker := range []string{"password=", "token=", "authorization:", "begin private key", "api_key="} {
		if strings.Contains(x, marker) {
			return false
		}
	}
	return true
}
func validRevision(r Revision) bool {
	if !clean(r.RepositoryRevision, 64) || len(r.Scenarios) == 0 || len(r.Scenarios) > 50 || r.Budget.MaxCost < 0 || r.Budget.MaxLatencyMS < 1 || r.Budget.MaxToolActions < 1 {
		return false
	}
	seen := map[string]bool{}
	for _, s := range r.Scenarios {
		if !clean(s.ID, 100) || seen[s.ID] || !clean(s.Title, 300) || !clean(s.SanitizedPrompt, 10000) || !sanitized(s.SanitizedPrompt) || len(s.ExpectedOutcomes) == 0 || len(s.Checks) == 0 {
			return false
		}
		seen[s.ID] = true
		for _, c := range append(slices.Clone(s.Checks), s.HiddenChecks...) {
			if !clean(c.Name, 200) || (c.Kind != "contains" && c.Kind != "not_contains" && c.Kind != "policy" && c.Kind != "canary") || !clean(c.Expected, 1000) {
				return false
			}
		}
	}
	return len(r.ProhibitedActions) > 0 && len(r.HumanReviewCriteria) > 0 && clean(r.ChangeSummary, 1000)
}
func (s *Store) suitePath(v string) string { return filepath.Join(s.root, "suite-"+v+".json") }
func (s *Store) runPath(v string) string   { return filepath.Join(s.root, "run-"+v+".json") }
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
func (s *Store) Create(v Suite, r Revision) (Suite, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !clean(v.OrganizationID, 64) || !clean(v.RepositoryID, 64) || !clean(v.Name, 300) || !validRevision(r) {
		return Suite{}, ErrInvalid
	}
	x, e := id()
	if e != nil {
		return Suite{}, e
	}
	r.Version = 1
	r.CreatedAt = s.now()
	v.ID = x
	v.Revisions = []Revision{r}
	return v, write(s.suitePath(x), v)
}
func (s *Store) Revise(suiteID string, expected int, r Revision) (Suite, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, e := read[Suite](s.suitePath(suiteID))
	if e != nil {
		return Suite{}, e
	}
	if len(v.Revisions) != expected {
		return Suite{}, ErrConflict
	}
	if !validRevision(r) {
		return Suite{}, ErrInvalid
	}
	r.Version = expected + 1
	r.CreatedAt = s.now()
	v.Revisions = append(v.Revisions, r)
	return v, write(s.suitePath(v.ID), v)
}
func (s *Store) Get(id string) (Suite, error) { return read[Suite](s.suitePath(id)) }
func (s *Store) List(org string) ([]Suite, error) {
	entries, e := os.ReadDir(s.root)
	if e != nil {
		return nil, e
	}
	out := []Suite{}
	for _, x := range entries {
		if strings.HasPrefix(x.Name(), "suite-") {
			v, er := read[Suite](filepath.Join(s.root, x.Name()))
			if er != nil {
				return nil, er
			}
			if v.OrganizationID == org {
				out = append(out, v)
			}
		}
	}
	return out, nil
}
func publicSuite(v Suite) Suite {
	for i := range v.Revisions {
		for j := range v.Revisions[i].Scenarios {
			v.Revisions[i].Scenarios[j].HiddenChecks = nil
		}
	}
	return v
}
func (s *Store) PublicGet(v string) (Suite, error) { x, e := s.Get(v); return publicSuite(x), e }
func (s *Store) PublicList(org string) ([]Suite, error) {
	x, e := s.List(org)
	for i := range x {
		x[i] = publicSuite(x[i])
	}
	return x, e
}
func digest(v any) string {
	b, _ := json.Marshal(v)
	x := sha256.Sum256(b)
	return hex.EncodeToString(x[:])
}
func revisionInputDigest(rev Revision) string {
	return digest(struct {
		Revision  string
		Scenarios []Scenario
	}{rev.RepositoryRevision, rev.Scenarios})
}
func evaluate(c Check, out string) (bool, string) {
	passed := strings.Contains(out, c.Expected)
	if c.Kind == "not_contains" || c.Kind == "policy" || c.Kind == "canary" {
		passed = !passed
	}
	if passed {
		return true, "criterion satisfied"
	}
	return false, "criterion not satisfied"
}
func (s *Store) CreateRun(suiteID string, version int, in RunInput, actor string) (Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	suite, e := read[Suite](s.suitePath(suiteID))
	if e != nil {
		return Run{}, e
	}
	if version < 1 || version > len(suite.Revisions) || !clean(in.AgentID, 64) || in.AgentProfileVersion < 1 || !clean(actor, 64) || in.Cost < 0 || in.LatencyMS < 0 || len(in.ToolActions) > 500 || len(in.Artifacts) > 100 || len(in.Failure) > 2000 || !sanitized(in.Failure) {
		return Run{}, ErrInvalid
	}
	for scenarioID, output := range in.Outputs {
		if !clean(scenarioID, 100) || len(output) > 50000 || !sanitized(output) {
			return Run{}, ErrInvalid
		}
	}
	for _, artifact := range in.Artifacts {
		if !clean(artifact.Name, 300) || len(artifact.SHA256) != 64 || artifact.Size < 0 || !sanitized(artifact.Summary) {
			return Run{}, ErrInvalid
		}
	}
	for _, action := range in.ToolActions {
		if !clean(action.Tool, 100) || !clean(action.Action, 200) || action.DurationMS < 0 || !sanitized(action.InputSummary) || !sanitized(action.OutputSummary) {
			return Run{}, ErrInvalid
		}
	}
	rev := suite.Revisions[version-1]
	for _, scenario := range rev.Scenarios {
		output, exists := in.Outputs[scenario.ID]
		if !exists || strings.TrimSpace(output) == "" {
			return Run{}, ErrInvalid
		}
	}
	inputDigest := revisionInputDigest(rev)
	reproducible := false
	if in.ReproducesRunID != "" {
		if !clean(in.ReproducesRunID, 64) {
			return Run{}, ErrInvalid
		}
		prior, priorErr := read[Run](s.runPath(in.ReproducesRunID))
		if priorErr != nil || prior.OrganizationID != suite.OrganizationID || prior.SuiteID != suiteID || prior.SuiteVersion != version || prior.RepositoryID != suite.RepositoryID || prior.RepositoryRevision != rev.RepositoryRevision || prior.AgentID != in.AgentID || prior.AgentProfileVersion != in.AgentProfileVersion || prior.InputDigest != inputDigest {
			return Run{}, ErrInvalid
		}
		reproducible = !in.OperatorSupplied
	}
	label := "initial"
	entries, _ := os.ReadDir(s.root)
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "run-") {
			continue
		}
		prior, er := read[Run](filepath.Join(s.root, entry.Name()))
		if er == nil && prior.SuiteID == suiteID && prior.SuiteVersion == version && prior.AgentID == in.AgentID {
			label = "repeated"
		}
	}
	if in.OperatorSupplied {
		label = "operator_supplied"
	}
	results := []CheckResult{}
	correct, policy := true, true
	contam := []string{}
	for _, sc := range rev.Scenarios {
		out := in.Outputs[sc.ID]
		for _, c := range sc.Checks {
			p, m := evaluate(c, out)
			results = append(results, CheckResult{sc.ID, c.Name, c.Kind, p, false, m})
			if c.Kind == "policy" {
				policy = policy && p
			} else {
				correct = correct && p
			}
		}
		for _, c := range sc.HiddenChecks {
			p, _ := evaluate(c, out)
			results = append(results, CheckResult{sc.ID, c.Name, c.Kind, p, true, "hidden criterion evaluated"})
			if c.Kind == "policy" {
				policy = policy && p
			} else {
				correct = correct && p
			}
			if c.Kind == "canary" && strings.Contains(out, c.Expected) {
				contam = append(contam, "candidate output overlaps protected evaluator material for scenario "+sc.ID)
			}
		}
	}
	for _, a := range in.ToolActions {
		for _, p := range rev.ProhibitedActions {
			if strings.EqualFold(strings.TrimSpace(a.Action), strings.TrimSpace(p)) {
				policy = false
			}
		}
	}
	budget := in.Cost <= rev.Budget.MaxCost && in.LatencyMS <= rev.Budget.MaxLatencyMS && len(in.ToolActions) <= rev.Budget.MaxToolActions
	if strings.TrimSpace(in.Failure) != "" {
		correct, policy, budget = false, false, false
	}
	x, e := id()
	if e != nil {
		return Run{}, e
	}
	run := Run{ID: x, SuiteID: suiteID, SuiteVersion: version, OrganizationID: suite.OrganizationID, RepositoryID: suite.RepositoryID, RepositoryRevision: rev.RepositoryRevision, AgentID: in.AgentID, AgentProfileVersion: in.AgentProfileVersion, TrialLabel: label, ReproducesRunID: in.ReproducesRunID, InputDigest: inputDigest, Outputs: in.Outputs, ToolActions: in.ToolActions, Artifacts: in.Artifacts, Cost: in.Cost, LatencyMS: in.LatencyMS, Failure: strings.TrimSpace(in.Failure), Authority: Authority{}, CheckResults: results, CorrectnessPassed: correct, PolicyPassed: policy, BudgetPassed: budget, Contaminated: len(contam) > 0, ContaminationReasons: contam, Reproducible: reproducible, ReviewStatus: "pending", Decisions: []Decision{}, CreatedBy: actor, CreatedAt: s.now()}
	if e := write(s.runPath(x), run); e != nil {
		return Run{}, e
	}
	return publicRun(run), nil
}
func publicRun(v Run) Run {
	publicResults := make([]CheckResult, 0, len(v.CheckResults))
	for _, result := range v.CheckResults {
		if !result.Hidden {
			publicResults = append(publicResults, result)
		}
	}
	v.CheckResults = publicResults
	return v
}
func (s *Store) GetRun(v string) (Run, error) {
	x, e := read[Run](s.runPath(v))
	return publicRun(x), e
}
func (s *Store) ListRuns(org string) ([]Run, error) {
	entries, e := os.ReadDir(s.root)
	if e != nil {
		return nil, e
	}
	out := []Run{}
	for _, x := range entries {
		if strings.HasPrefix(x.Name(), "run-") {
			v, er := read[Run](filepath.Join(s.root, x.Name()))
			if er != nil {
				return nil, er
			}
			if v.OrganizationID == org {
				out = append(out, publicRun(v))
			}
		}
	}
	return out, nil
}
func (s *Store) Decide(runID, actor, decision, rationale string) (Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, e := read[Run](s.runPath(runID))
	if e != nil {
		return Run{}, e
	}
	if (decision != "approved" && decision != "rejected" && decision != "needs_work") || !clean(rationale, 2000) || !clean(actor, 64) {
		return Run{}, ErrInvalid
	}
	if decision == "approved" && (!v.CorrectnessPassed || !v.PolicyPassed || !v.BudgetPassed || v.Contaminated || v.Failure != "") {
		return Run{}, ErrInvalid
	}
	v.Decisions = append(v.Decisions, Decision{decision, rationale, actor, s.now()})
	v.ReviewStatus = decision
	e = write(s.runPath(runID), v)
	return publicRun(v), e
}
