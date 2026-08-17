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

// Participation turns an approved trial into a proposed collaboration boundary.
// It is deliberately separate from the eventual organization access grant so
// evidence, agreement, denial, expiry, and revocation remain inspectable.
type ParticipationBudget struct {
	MaxCost         float64 `json:"max_cost"`
	MaxAgentMinutes int     `json:"max_agent_minutes"`
	MaxActions      int     `json:"max_actions"`
}
type ParticipationResource struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}
type ParticipationAgreement struct {
	Kind      string    `json:"kind"`
	ActorID   string    `json:"actor_id"`
	Statement string    `json:"statement"`
	CreatedAt time.Time `json:"created_at"`
}
type ParticipationEvent struct {
	Kind      string    `json:"kind"`
	ActorID   string    `json:"actor_id"`
	Summary   string    `json:"summary"`
	CreatedAt time.Time `json:"created_at"`
}
type Participation struct {
	ID                   string                   `json:"id"`
	OrganizationID       string                   `json:"organization_id"`
	AgentID              string                   `json:"agent_id"`
	AgentProfileVersion  int                      `json:"agent_profile_version"`
	EvaluationRunID      string                   `json:"evaluation_run_id"`
	EvaluationDecisionAt time.Time                `json:"evaluation_decision_at"`
	Version              int                      `json:"version"`
	Status               string                   `json:"status"`
	Role                 string                   `json:"role"`
	Resources            []ParticipationResource  `json:"resources"`
	Actions              []string                 `json:"actions"`
	Budget               ParticipationBudget      `json:"budget"`
	StartsAt             time.Time                `json:"starts_at"`
	ExpiresAt            time.Time                `json:"expires_at"`
	DataBoundaries       []string                 `json:"data_boundaries"`
	PolicyExceptionIDs   []string                 `json:"policy_exception_ids"`
	AgreementRequirement string                   `json:"agreement_requirement"`
	SponsorID            string                   `json:"sponsor_id,omitempty"`
	Agreements           []ParticipationAgreement `json:"agreements"`
	AuthorityIdentity    string                   `json:"authority_identity,omitempty"`
	AccessGrantID        string                   `json:"access_grant_id,omitempty"`
	CreatedBy            string                   `json:"created_by"`
	CreatedAt            time.Time                `json:"created_at"`
	DeniedBy             string                   `json:"denied_by,omitempty"`
	RevokedBy            string                   `json:"revoked_by,omitempty"`
	Events               []ParticipationEvent     `json:"events"`
}
type ParticipationInput struct {
	AgentID              string                  `json:"agent_id"`
	AgentProfileVersion  int                     `json:"agent_profile_version"`
	EvaluationRunID      string                  `json:"evaluation_run_id"`
	Role                 string                  `json:"role"`
	Resources            []ParticipationResource `json:"resources"`
	Actions              []string                `json:"actions"`
	Budget               ParticipationBudget     `json:"budget"`
	StartsAt             time.Time               `json:"starts_at"`
	ExpiresAt            time.Time               `json:"expires_at"`
	DataBoundaries       []string                `json:"data_boundaries"`
	PolicyExceptionIDs   []string                `json:"policy_exception_ids"`
	AgreementRequirement string                  `json:"agreement_requirement"`
	SponsorID            string                  `json:"sponsor_id"`
}

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
func (s *Store) participationPath(v string) string {
	return filepath.Join(s.root, "participation-"+v+".json")
}
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
		if priorErr != nil || prior.TrialLabel == "operator_supplied" || prior.OrganizationID != suite.OrganizationID || prior.SuiteID != suiteID || prior.SuiteVersion != version || prior.RepositoryID != suite.RepositoryID || prior.RepositoryRevision != rev.RepositoryRevision || prior.AgentID != in.AgentID || prior.AgentProfileVersion != in.AgentProfileVersion || prior.InputDigest != inputDigest {
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
	return projectedRun(run, rev, false), nil
}
func projectedRun(v Run, rev Revision, evaluator bool) Run {
	publicResults := make([]CheckResult, 0, len(v.CheckResults))
	for _, result := range v.CheckResults {
		if !result.Hidden {
			publicResults = append(publicResults, result)
		}
	}
	v.CheckResults = publicResults
	if !evaluator {
		correct, policy := true, true
		for _, result := range publicResults {
			if result.Kind == "policy" {
				policy = policy && result.Passed
			} else {
				correct = correct && result.Passed
			}
		}
		for _, action := range v.ToolActions {
			for _, prohibited := range rev.ProhibitedActions {
				if strings.EqualFold(strings.TrimSpace(action.Action), strings.TrimSpace(prohibited)) {
					policy = false
				}
			}
		}
		if v.Failure != "" {
			correct, policy = false, false
		}
		v.CorrectnessPassed, v.PolicyPassed = correct, policy
		v.Contaminated = false
		v.ContaminationReasons = nil
	}
	return v
}
func (s *Store) GetRun(v string) (Run, error) {
	x, e := read[Run](s.runPath(v))
	if e != nil {
		return Run{}, e
	}
	suite, e := read[Suite](s.suitePath(x.SuiteID))
	if e != nil || x.SuiteVersion < 1 || x.SuiteVersion > len(suite.Revisions) {
		return Run{}, ErrInvalid
	}
	return projectedRun(x, suite.Revisions[x.SuiteVersion-1], false), nil
}
func (s *Store) GetEvaluatorRun(v string) (Run, error) {
	x, e := read[Run](s.runPath(v))
	if e != nil {
		return Run{}, e
	}
	suite, e := read[Suite](s.suitePath(x.SuiteID))
	if e != nil || x.SuiteVersion < 1 || x.SuiteVersion > len(suite.Revisions) {
		return Run{}, ErrInvalid
	}
	return projectedRun(x, suite.Revisions[x.SuiteVersion-1], true), nil
}
func (s *Store) listRuns(org string, evaluator bool) ([]Run, error) {
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
				suite, suiteErr := read[Suite](s.suitePath(v.SuiteID))
				if suiteErr != nil || v.SuiteVersion < 1 || v.SuiteVersion > len(suite.Revisions) {
					return nil, ErrInvalid
				}
				out = append(out, projectedRun(v, suite.Revisions[v.SuiteVersion-1], evaluator))
			}
		}
	}
	return out, nil
}
func (s *Store) ListRuns(org string) ([]Run, error)          { return s.listRuns(org, false) }
func (s *Store) ListEvaluatorRuns(org string) ([]Run, error) { return s.listRuns(org, true) }
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
	if e != nil {
		return Run{}, e
	}
	suite, e := read[Suite](s.suitePath(v.SuiteID))
	if e != nil || v.SuiteVersion < 1 || v.SuiteVersion > len(suite.Revisions) {
		return Run{}, ErrInvalid
	}
	return projectedRun(v, suite.Revisions[v.SuiteVersion-1], true), nil
}

func validParticipation(in ParticipationInput, now time.Time) bool {
	if !clean(in.AgentID, 64) || in.AgentProfileVersion < 1 || !clean(in.EvaluationRunID, 64) || !slices.Contains([]string{"viewer", "contributor", "maintainer", "operator"}, in.Role) || len(in.Resources) == 0 || len(in.Resources) > 100 || len(in.Actions) == 0 || len(in.Actions) > 100 || len(in.DataBoundaries) == 0 || len(in.DataBoundaries) > 100 || in.Budget.MaxCost < 0 || in.Budget.MaxAgentMinutes < 1 || in.Budget.MaxActions < 1 || in.StartsAt.Before(now.Add(-time.Minute)) || !in.ExpiresAt.After(in.StartsAt) || !slices.Contains([]string{"operator", "sponsor"}, in.AgreementRequirement) {
		return false
	}
	if in.AgreementRequirement == "sponsor" && !clean(in.SponsorID, 64) {
		return false
	}
	seen := map[string]bool{}
	for _, r := range in.Resources {
		if !slices.Contains([]string{"repository", "package", "environment", "collaboration"}, r.Kind) || !clean(r.ID, 64) || seen[r.Kind+":"+r.ID] {
			return false
		}
		seen[r.Kind+":"+r.ID] = true
	}
	for _, values := range [][]string{in.Actions, in.DataBoundaries, in.PolicyExceptionIDs} {
		for _, v := range values {
			if !clean(v, 300) || !sanitized(v) {
				return false
			}
		}
	}
	return true
}

func (s *Store) CreateParticipation(org, actor string, in ParticipationInput) (Participation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if !clean(org, 64) || !clean(actor, 64) || !validParticipation(in, now) {
		return Participation{}, ErrInvalid
	}
	run, e := read[Run](s.runPath(in.EvaluationRunID))
	if e != nil || run.OrganizationID != org || run.AgentID != in.AgentID || run.AgentProfileVersion != in.AgentProfileVersion || run.ReviewStatus != "approved" {
		return Participation{}, ErrInvalid
	}
	var approvedAt time.Time
	for i := len(run.Decisions) - 1; i >= 0; i-- {
		if run.Decisions[i].Decision == "approved" {
			approvedAt = run.Decisions[i].CreatedAt
			break
		}
	}
	if approvedAt.IsZero() {
		return Participation{}, ErrInvalid
	}
	x, e := id()
	if e != nil {
		return Participation{}, e
	}
	p := Participation{ID: x, OrganizationID: org, AgentID: in.AgentID, AgentProfileVersion: in.AgentProfileVersion, EvaluationRunID: in.EvaluationRunID, EvaluationDecisionAt: approvedAt, Version: 1, Status: "pending_agreement", Role: in.Role, Resources: in.Resources, Actions: in.Actions, Budget: in.Budget, StartsAt: in.StartsAt, ExpiresAt: in.ExpiresAt, DataBoundaries: in.DataBoundaries, PolicyExceptionIDs: in.PolicyExceptionIDs, AgreementRequirement: in.AgreementRequirement, SponsorID: in.SponsorID, Agreements: []ParticipationAgreement{}, CreatedBy: actor, CreatedAt: now, Events: []ParticipationEvent{{Kind: "participation.proposed", ActorID: actor, Summary: "Approved trial proposed for bounded project participation.", CreatedAt: now}}}
	return p, write(s.participationPath(x), p)
}
func (s *Store) GetParticipation(v string) (Participation, error) {
	return read[Participation](s.participationPath(v))
}
func (s *Store) ListParticipations(org string) ([]Participation, error) {
	entries, e := os.ReadDir(s.root)
	if e != nil {
		return nil, e
	}
	out := []Participation{}
	for _, x := range entries {
		if strings.HasPrefix(x.Name(), "participation-") {
			v, er := read[Participation](filepath.Join(s.root, x.Name()))
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
func (s *Store) AgreeParticipation(v, actor, kind, statement string) (Participation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, e := read[Participation](s.participationPath(v))
	if e != nil {
		return p, e
	}
	if p.Status != "pending_agreement" || kind != p.AgreementRequirement || !clean(actor, 64) || !clean(statement, 1000) {
		return p, ErrConflict
	}
	now := s.now()
	p.Agreements = append(p.Agreements, ParticipationAgreement{kind, actor, strings.TrimSpace(statement), now})
	p.Status = "ready"
	p.Version++
	p.Events = append(p.Events, ParticipationEvent{"participation.agreed", actor, "Required human boundary accepted.", now})
	return p, write(s.participationPath(v), p)
}
func (s *Store) ActivateParticipation(v, actor, identity, grant string, expected int) (Participation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, e := read[Participation](s.participationPath(v))
	if e != nil {
		return p, e
	}
	now := s.now()
	if p.Version != expected || p.Status != "ready" || now.Before(p.StartsAt) || !p.ExpiresAt.After(now) || !clean(identity, 200) || !clean(grant, 64) {
		return p, ErrConflict
	}
	p.Status = "active"
	p.Version++
	p.AuthorityIdentity = identity
	p.AccessGrantID = grant
	p.Events = append(p.Events, ParticipationEvent{"participation.activated", actor, "Scoped identity linked to an ordinary revocable access grant.", now})
	return p, write(s.participationPath(v), p)
}
func (s *Store) DecideParticipation(v, actor, decision string, expected int) (Participation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, e := read[Participation](s.participationPath(v))
	if e != nil {
		return p, e
	}
	if p.Version != expected || !slices.Contains([]string{"deny", "revoke"}, decision) {
		return p, ErrConflict
	}
	now := s.now()
	if decision == "deny" {
		if p.Status == "active" {
			return p, ErrConflict
		}
		p.Status = "denied"
		p.DeniedBy = actor
	} else {
		if p.Status != "active" {
			return p, ErrConflict
		}
		p.Status = "revoked"
		p.RevokedBy = actor
	}
	p.Version++
	p.Events = append(p.Events, ParticipationEvent{"participation." + decision + "d", actor, "Participation authority was explicitly " + decision + "d.", now})
	return p, write(s.participationPath(v), p)
}
