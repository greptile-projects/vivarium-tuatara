// Package performanceevidence retains sanitized, exact-revision performance trials.
package performanceevidence

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
	"time"
)

var ErrInvalid = errors.New("invalid performance trial")
var ErrNotFound = errors.New("performance trial not found")

type Source struct {
	Kind      string `json:"kind"`
	Revision  string `json:"revision"`
	ReleaseID string `json:"release_id,omitempty"`
}
type Environment struct {
	Name           string `json:"name"`
	OS             string `json:"os"`
	Architecture   string `json:"architecture"`
	Runtime        string `json:"runtime"`
	Hardware       string `json:"hardware,omitempty"`
	ContainerImage string `json:"container_image,omitempty"`
}
type Sampling struct {
	Warmup  int    `json:"warmup"`
	Samples int    `json:"samples"`
	Method  string `json:"method"`
}
type Timing struct {
	Metric   string    `json:"metric"`
	Unit     string    `json:"unit"`
	Values   []float64 `json:"values"`
	Mean     float64   `json:"mean"`
	Minimum  float64   `json:"minimum"`
	Maximum  float64   `json:"maximum"`
	Variance float64   `json:"variance"`
}
type ResourceProfile struct {
	CPUSeconds   float64 `json:"cpu_seconds"`
	PeakMemoryMB float64 `json:"peak_memory_mb"`
	ReadBytes    int64   `json:"read_bytes"`
	WriteBytes   int64   `json:"write_bytes"`
}
type Artifact struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}
type Cost struct {
	Amount float64 `json:"amount"`
	Unit   string  `json:"unit"`
}
type Trial struct {
	ID           string          `json:"id"`
	RepositoryID string          `json:"repository_id"`
	GoalID       string          `json:"goal_id,omitempty"`
	ContextKind  string          `json:"context_kind"`
	ContextID    string          `json:"context_id"`
	Mode         string          `json:"mode"`
	Source       Source          `json:"source"`
	Workload     string          `json:"workload"`
	Inputs       string          `json:"inputs"`
	Sanitization []string        `json:"sanitization"`
	Environment  Environment     `json:"environment"`
	Sampling     Sampling        `json:"sampling"`
	Timings      []Timing        `json:"timings"`
	Resources    ResourceProfile `json:"resources"`
	Traces       []Artifact      `json:"traces"`
	Logs         []string        `json:"logs"`
	Artifacts    []Artifact      `json:"artifacts"`
	Cost         Cost            `json:"cost"`
	CreatedBy    string          `json:"created_by"`
	CreatedAt    time.Time       `json:"created_at"`
}
type Comparison struct {
	Metric        string  `json:"metric"`
	Unit          string  `json:"unit"`
	BaselineMean  float64 `json:"baseline_mean"`
	CurrentMean   float64 `json:"current_mean"`
	ChangePercent float64 `json:"change_percent"`
	Comparable    bool    `json:"comparable"`
	Reason        string  `json:"reason,omitempty"`
}
type CorrectnessCheck struct {
	Name    string `json:"name"`
	Command string `json:"command"`
	Passed  bool   `json:"passed"`
	Summary string `json:"summary"`
}
type Evaluation struct {
	ID                string             `json:"id"`
	RepositoryID      string             `json:"repository_id"`
	PullRequestID     string             `json:"pull_request_id"`
	Revision          string             `json:"revision"`
	GoalID            string             `json:"goal_id"`
	InvestigationID   string             `json:"investigation_id"`
	BaselineTrialID   string             `json:"baseline_trial_id"`
	CandidateTrialID  string             `json:"candidate_trial_id"`
	AffectedScenarios []string           `json:"affected_scenarios"`
	Commands          []string           `json:"commands"`
	CorrectnessChecks []CorrectnessCheck `json:"correctness_checks"`
	ResidualRisks     []string           `json:"residual_risks"`
	CreatedBy         string             `json:"created_by"`
	CreatedAt         time.Time          `json:"created_at"`
	Comparisons       []Comparison       `json:"comparisons"`
	Confidence        *float64           `json:"confidence"`
	ResourceChanges   map[string]float64 `json:"resource_changes"`
	CostChangePercent *float64           `json:"cost_change_percent"`
	CorrectnessPassed bool               `json:"correctness_passed"`
	Stale             bool               `json:"stale"`
}
type Reference struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Revision string `json:"revision,omitempty"`
	Symbol   string `json:"symbol,omitempty"`
	Path     string `json:"path,omitempty"`
	Label    string `json:"label"`
}
type FlameFrame struct {
	Name string `json:"name"`
	File string `json:"file,omitempty"`
	Line int    `json:"line,omitempty"`
}
type FlameStack struct {
	Frames []FlameFrame `json:"frames"`
	Value  float64      `json:"value"`
	Unit   string       `json:"unit"`
}
type Finding struct {
	ID            string         `json:"id"`
	Kind          string         `json:"kind"`
	Body          string         `json:"body"`
	CitationIDs   []string       `json:"citation_ids"`
	Flamegraph    []FlameStack   `json:"flamegraph,omitempty"`
	Confidence    string         `json:"confidence"`
	CreatedBy     string         `json:"created_by"`
	CreatedAt     time.Time      `json:"created_at"`
	Challenges    []Challenge    `json:"challenges"`
	Confirmations []Confirmation `json:"confirmations"`
	Stale         bool           `json:"stale"`
	StaleReasons  []string       `json:"stale_reasons,omitempty"`
}
type Challenge struct {
	ID        string    `json:"id"`
	Body      string    `json:"body"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}
type Confirmation struct {
	ID        string    `json:"id"`
	Body      string    `json:"body"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}
type Investigation struct {
	ID           string      `json:"id"`
	RepositoryID string      `json:"repository_id"`
	Title        string      `json:"title"`
	TrialIDs     []string    `json:"trial_ids"`
	References   []Reference `json:"references"`
	InviteeIDs   []string    `json:"invitee_ids"`
	CredentialID string      `json:"credential_id,omitempty"`
	CreatedBy    string      `json:"created_by"`
	CreatedAt    time.Time   `json:"created_at"`
	Findings     []Finding   `json:"findings"`
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
func newID() (string, error) {
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		return "", e
	}
	return hex.EncodeToString(b[:]), nil
}
func (s *Store) CreateInvestigation(v Investigation, resolveReference func(Reference) bool) (Investigation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v.RepositoryID == "" || strings.TrimSpace(v.Title) == "" || v.CreatedBy == "" || len(v.TrialIDs) == 0 || len(v.TrialIDs) > 20 || len(v.References) > 100 || len(v.InviteeIDs) > 50 {
		return Investigation{}, ErrInvalid
	}
	seen := map[string]bool{}
	for _, id := range v.TrialIDs {
		t, e := s.read(id)
		if e != nil || t.RepositoryID != v.RepositoryID || seen[id] {
			return Investigation{}, ErrInvalid
		}
		seen[id] = true
	}
	for _, ref := range v.References {
		if ref.Kind == "" || ref.ID == "" || ref.Label == "" || resolveReference == nil || !resolveReference(ref) {
			return Investigation{}, ErrInvalid
		}
	}
	id, e := newID()
	if e != nil {
		return Investigation{}, e
	}
	v.ID = id
	v.Title = strings.TrimSpace(v.Title)
	v.CreatedAt = s.now()
	v.Findings = []Finding{}
	return v, s.writeInvestigation(v)
}
func (s *Store) GetInvestigation(id string) (Investigation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.readInvestigation(id)
}
func (s *Store) ListInvestigations(repositoryID string) ([]Investigation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := filepath.Join(s.root, "investigations")
	es, e := os.ReadDir(dir)
	if os.IsNotExist(e) {
		return []Investigation{}, nil
	}
	if e != nil {
		return nil, e
	}
	out := []Investigation{}
	for _, x := range es {
		v, er := s.readInvestigation(strings.TrimSuffix(x.Name(), ".json"))
		if er != nil {
			return nil, er
		}
		if v.RepositoryID == repositoryID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}
func (s *Store) AddFinding(investigationID, actor string, v Finding) (Investigation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	x, e := s.readInvestigation(investigationID)
	if e != nil {
		return x, e
	}
	if actor == "" || (v.Kind != "hypothesis" && v.Kind != "comparison" && v.Kind != "uncertainty" && v.Kind != "conclusion") || strings.TrimSpace(v.Body) == "" || len(v.CitationIDs) == 0 || len(v.CitationIDs) > 50 || (v.Confidence != "low" && v.Confidence != "medium" && v.Confidence != "high") {
		return x, ErrInvalid
	}
	allowed := map[string]bool{}
	for _, id := range x.TrialIDs {
		allowed[id] = true
	}
	for _, r := range x.References {
		allowed[r.ID] = true
	}
	for _, id := range v.CitationIDs {
		if !allowed[id] {
			return x, ErrInvalid
		}
	}
	for _, stack := range v.Flamegraph {
		if len(stack.Frames) == 0 || stack.Value < 0 || stack.Unit == "" || len(stack.Frames) > 256 {
			return x, ErrInvalid
		}
	}
	v.ID, _ = newID()
	v.Body = strings.TrimSpace(v.Body)
	v.CreatedBy = actor
	v.CreatedAt = s.now()
	v.Challenges = []Challenge{}
	v.Confirmations = []Confirmation{}
	x.Findings = append(x.Findings, v)
	return x, s.writeInvestigation(x)
}
func (s *Store) BindCredential(investigationID, credentialID string) (Investigation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, e := s.readInvestigation(investigationID)
	if e != nil {
		return v, e
	}
	if credentialID == "" {
		return v, ErrInvalid
	}
	v.CredentialID = credentialID
	return v, s.writeInvestigation(v)
}
func (s *Store) Respond(investigationID, findingID, actor, body string, confirm bool) (Investigation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	x, e := s.readInvestigation(investigationID)
	if e != nil {
		return x, e
	}
	if actor == "" || strings.TrimSpace(body) == "" {
		return x, ErrInvalid
	}
	for i := range x.Findings {
		if x.Findings[i].ID == findingID {
			id, _ := newID()
			if confirm {
				x.Findings[i].Confirmations = append(x.Findings[i].Confirmations, Confirmation{ID: id, Body: strings.TrimSpace(body), CreatedBy: actor, CreatedAt: s.now()})
			} else {
				x.Findings[i].Challenges = append(x.Findings[i].Challenges, Challenge{ID: id, Body: strings.TrimSpace(body), CreatedBy: actor, CreatedAt: s.now()})
			}
			return x, s.writeInvestigation(x)
		}
	}
	return x, ErrNotFound
}
func (s *Store) ProjectStaleness(v Investigation) Investigation {
	// Credential bindings are persistence-only authority and never part of a
	// repository or delegated evidence projection.
	v.CredentialID = ""
	trials := []Trial{}
	for _, id := range v.TrialIDs {
		if t, e := s.Get(id); e == nil {
			trials = append(trials, t)
		}
	}
	all, _ := s.List(v.RepositoryID)
	for fi := range v.Findings {
		for _, old := range trials {
			for _, now := range all {
				if now.ContextKind == old.ContextKind && now.ContextID == old.ContextID && now.CreatedAt.After(old.CreatedAt) {
					if now.Source.Revision != old.Source.Revision {
						v.Findings[fi].StaleReasons = append(v.Findings[fi].StaleReasons, "revision changed")
					}
					if now.Workload != old.Workload {
						v.Findings[fi].StaleReasons = append(v.Findings[fi].StaleReasons, "workload changed")
					}
					if now.Environment != old.Environment {
						v.Findings[fi].StaleReasons = append(v.Findings[fi].StaleReasons, "environment changed")
					}
				}
			}
		}
		v.Findings[fi].Stale = len(v.Findings[fi].StaleReasons) > 0
	}
	return v
}
func (s *Store) writeInvestigation(v Investigation) error {
	dir := filepath.Join(s.root, "investigations")
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	body, _ := json.Marshal(v)
	tmp, e := os.CreateTemp(dir, ".investigation-*")
	if e != nil {
		return e
	}
	name := tmp.Name()
	defer os.Remove(name)
	_ = tmp.Chmod(0600)
	_, e = tmp.Write(body)
	if e == nil {
		e = tmp.Sync()
	}
	if ce := tmp.Close(); e == nil {
		e = ce
	}
	if e == nil {
		e = os.Rename(name, filepath.Join(dir, v.ID+".json"))
	}
	return e
}
func (s *Store) readInvestigation(id string) (Investigation, error) {
	if len(id) != 32 {
		return Investigation{}, ErrNotFound
	}
	body, e := os.ReadFile(filepath.Join(s.root, "investigations", id+".json"))
	if e != nil {
		return Investigation{}, ErrNotFound
	}
	var v Investigation
	if json.Unmarshal(body, &v) != nil || v.ID != id {
		return Investigation{}, ErrNotFound
	}
	return v, nil
}
func (s *Store) Create(v Trial) (Trial, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !valid(v) {
		return Trial{}, ErrInvalid
	}
	var b [16]byte
	if _, e := rand.Read(b[:]); e != nil {
		return Trial{}, e
	}
	v.ID = hex.EncodeToString(b[:])
	v.CreatedAt = s.now()
	// Production captures retain the declared recipe and sanitization policy, not
	// producer-supplied operational input. A declaration alone cannot prove that
	// an arbitrary value contains no private user data.
	if v.Mode == "production_capture" {
		v.Inputs = "[sanitized production-derived workload]"
		v.Logs = sanitizedProductionLogs(v.Logs)
	}
	for i := range v.Timings {
		summarize(&v.Timings[i])
	}
	body, _ := json.Marshal(v)
	tmp, e := os.CreateTemp(s.root, ".trial-*")
	if e != nil {
		return Trial{}, e
	}
	name := tmp.Name()
	defer os.Remove(name)
	_ = tmp.Chmod(0600)
	_, e = tmp.Write(body)
	if e == nil {
		e = tmp.Sync()
	}
	if ce := tmp.Close(); e == nil {
		e = ce
	}
	if e == nil {
		e = os.Rename(name, filepath.Join(s.root, v.ID+".json"))
	}
	return v, e
}
func (s *Store) Get(id string) (Trial, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.read(id) }
func (s *Store) List(repositoryID string) ([]Trial, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	es, e := os.ReadDir(s.root)
	if e != nil {
		return nil, e
	}
	out := []Trial{}
	for _, x := range es {
		if x.IsDir() || !strings.HasSuffix(x.Name(), ".json") {
			continue
		}
		v, er := s.read(strings.TrimSuffix(x.Name(), ".json"))
		if er != nil {
			return nil, er
		}
		if v.RepositoryID == repositoryID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}
func (s *Store) Compare(a, b Trial) []Comparison {
	out := []Comparison{}
	old := map[string]Timing{}
	for _, t := range a.Timings {
		old[t.Metric+"\x00"+t.Unit] = t
	}
	for _, t := range b.Timings {
		x := Comparison{Metric: t.Metric, Unit: t.Unit, CurrentMean: t.Mean}
		o, ok := old[t.Metric+"\x00"+t.Unit]
		x.Comparable = ok && a.Workload == b.Workload && a.Environment == b.Environment && a.Sampling == b.Sampling
		if !x.Comparable {
			x.Reason = "workload, complete environment, warmup, sampling method/count, metric, and unit must match"
		} else {
			x.BaselineMean = o.Mean
			if o.Mean != 0 {
				x.ChangePercent = (t.Mean - o.Mean) / o.Mean * 100
			}
		}
		out = append(out, x)
	}
	return out
}

func (s *Store) CreateEvaluation(v Evaluation) (Evaluation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	baseline, e1 := s.read(v.BaselineTrialID)
	candidate, e2 := s.read(v.CandidateTrialID)
	inv, e3 := s.readInvestigation(v.InvestigationID)
	if e1 != nil || e2 != nil || e3 != nil || v.RepositoryID == "" || v.PullRequestID == "" || len(v.Revision) != 40 || v.CreatedBy == "" || v.GoalID == "" || baseline.RepositoryID != v.RepositoryID || candidate.RepositoryID != v.RepositoryID || inv.RepositoryID != v.RepositoryID || candidate.Source.Revision != v.Revision || baseline.GoalID != v.GoalID || candidate.GoalID != v.GoalID || !contains(inv.TrialIDs, baseline.ID) || len(v.AffectedScenarios) == 0 || len(v.AffectedScenarios) > 20 || len(v.Commands) == 0 || len(v.Commands) > 20 || len(v.CorrectnessChecks) == 0 || len(v.CorrectnessChecks) > 50 || len(v.ResidualRisks) > 50 {
		return Evaluation{}, ErrInvalid
	}
	for _, x := range append(append([]string{}, v.AffectedScenarios...), append(v.Commands, v.ResidualRisks...)...) {
		if strings.TrimSpace(x) == "" || len(x) > 2000 {
			return Evaluation{}, ErrInvalid
		}
	}
	v.CorrectnessPassed = true
	for _, check := range v.CorrectnessChecks {
		if strings.TrimSpace(check.Name) == "" || strings.TrimSpace(check.Command) == "" || strings.TrimSpace(check.Summary) == "" {
			return Evaluation{}, ErrInvalid
		}
		v.CorrectnessPassed = v.CorrectnessPassed && check.Passed
	}
	v.ID, _ = newID()
	v.CreatedAt = s.now()
	v.Comparisons = s.Compare(baseline, candidate)
	v.Confidence = confidence(baseline, candidate)
	v.ResourceChanges = map[string]float64{"cpu_seconds_percent": percent(baseline.Resources.CPUSeconds, candidate.Resources.CPUSeconds), "peak_memory_mb_percent": percent(baseline.Resources.PeakMemoryMB, candidate.Resources.PeakMemoryMB), "read_bytes_percent": percent(float64(baseline.Resources.ReadBytes), float64(candidate.Resources.ReadBytes)), "write_bytes_percent": percent(float64(baseline.Resources.WriteBytes), float64(candidate.Resources.WriteBytes))}
	if baseline.Cost.Unit == candidate.Cost.Unit && baseline.Cost.Amount != 0 {
		change := percent(baseline.Cost.Amount, candidate.Cost.Amount)
		v.CostChangePercent = &change
	}
	dir := filepath.Join(s.root, "evaluations", v.RepositoryID, v.PullRequestID)
	if e := os.MkdirAll(dir, 0700); e != nil {
		return Evaluation{}, e
	}
	body, _ := json.Marshal(v)
	if e := os.WriteFile(filepath.Join(dir, v.ID+".json"), body, 0600); e != nil {
		return Evaluation{}, e
	}
	return v, nil
}
func (s *Store) ListEvaluations(repositoryID, pullID, revision string) ([]Evaluation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := filepath.Join(s.root, "evaluations", repositoryID, pullID)
	entries, e := os.ReadDir(dir)
	if os.IsNotExist(e) {
		return []Evaluation{}, nil
	}
	if e != nil {
		return nil, e
	}
	out := []Evaluation{}
	for _, entry := range entries {
		body, er := os.ReadFile(filepath.Join(dir, entry.Name()))
		var v Evaluation
		if er != nil || json.Unmarshal(body, &v) != nil {
			return nil, ErrNotFound
		}
		v.Stale = v.Revision != revision
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}
func contains(xs []string, wanted string) bool {
	for _, x := range xs {
		if x == wanted {
			return true
		}
	}
	return false
}
func percent(old, current float64) float64 {
	if old == 0 {
		return 0
	}
	return (current - old) / old * 100
}
func confidence(a, b Trial) *float64 {
	if a.Workload != b.Workload || a.Environment != b.Environment || a.Sampling != b.Sampling {
		return nil
	}
	var x, y *Timing
	for i := range a.Timings {
		for j := range b.Timings {
			if a.Timings[i].Metric == b.Timings[j].Metric && a.Timings[i].Unit == b.Timings[j].Unit {
				x, y = &a.Timings[i], &b.Timings[j]
				break
			}
		}
		if x != nil {
			break
		}
	}
	if x == nil || len(x.Values) < 2 || len(y.Values) < 2 {
		return nil
	}
	se := math.Sqrt(x.Variance/float64(len(x.Values)) + y.Variance/float64(len(y.Values)))
	if se == 0 {
		return nil
	}
	z := math.Abs(y.Mean-x.Mean) / se
	value := math.Erf(z / math.Sqrt2)
	return &value
}
func (s *Store) read(id string) (Trial, error) {
	if len(id) != 32 {
		return Trial{}, ErrNotFound
	}
	body, e := os.ReadFile(filepath.Join(s.root, id+".json"))
	if os.IsNotExist(e) {
		return Trial{}, ErrNotFound
	}
	var v Trial
	if e != nil || json.Unmarshal(body, &v) != nil || v.ID != id {
		return Trial{}, ErrNotFound
	}
	if v.Mode == "production_capture" {
		v.Inputs = "[sanitized production-derived workload]"
		v.Logs = sanitizedProductionLogs(v.Logs)
	}
	return v, nil
}

func sanitizedProductionLogs(logs []string) []string {
	if len(logs) == 0 {
		return logs
	}
	result := make([]string, len(logs))
	for i := range result {
		result[i] = "[sanitized production log entry]"
	}
	return result
}
func valid(v Trial) bool {
	if v.RepositoryID == "" || v.CreatedBy == "" || len(v.Source.Revision) != 40 || (v.Source.Kind != "revision" && v.Source.Kind != "release") || (v.Mode != "benchmark" && v.Mode != "production_capture") || v.Workload == "" || v.Inputs == "" || v.Environment.Name == "" || v.Sampling.Samples < 1 || v.Sampling.Samples > 10000 || v.Sampling.Warmup < 0 || v.Sampling.Method == "" || len(v.Timings) == 0 || len(v.Logs) > 200 {
		return false
	}
	if v.Mode == "production_capture" && len(v.Sanitization) == 0 {
		return false
	}
	for _, line := range v.Logs {
		if len(line) > 4000 || containsCredential(line) {
			return false
		}
	}
	for _, t := range v.Timings {
		if t.Metric == "" || t.Unit == "" || len(t.Values) != v.Sampling.Samples {
			return false
		}
		for _, n := range t.Values {
			if math.IsNaN(n) || math.IsInf(n, 0) || n < 0 {
				return false
			}
		}
	}
	for _, a := range append(v.Traces, v.Artifacts...) {
		if a.Name == "" || len(a.SHA256) != 64 || a.Size < 0 {
			return false
		}
	}
	return true
}
func containsCredential(v string) bool {
	x := strings.ToLower(v)
	for _, p := range []string{"authorization:", "bearer ", "password=", "password:", "token=", "token:", "secret=", "secret:", "cookie:", "x-api-key", "api-key", "api_key", "apikey"} {
		if strings.Contains(x, p) {
			return true
		}
	}
	var structured any
	if json.Unmarshal([]byte(v), &structured) == nil && structuredCredential(structured) {
		return true
	}
	return false
}

func structuredCredential(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalized := strings.NewReplacer("_", "", "-", "", " ", "").Replace(strings.ToLower(key))
			if normalized == "token" || normalized == "accesstoken" || normalized == "refreshtoken" || normalized == "password" || normalized == "secret" || normalized == "apikey" || normalized == "authorization" || normalized == "cookie" {
				return true
			}
			if structuredCredential(child) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if structuredCredential(child) {
				return true
			}
		}
	}
	return false
}
func summarize(t *Timing) {
	t.Minimum, t.Maximum = t.Values[0], t.Values[0]
	var sum float64
	for _, v := range t.Values {
		sum += v
		if v < t.Minimum {
			t.Minimum = v
		}
		if v > t.Maximum {
			t.Maximum = v
		}
	}
	t.Mean = sum / float64(len(t.Values))
	for _, v := range t.Values {
		d := v - t.Mean
		t.Variance += d * d
	}
	t.Variance /= float64(len(t.Values))
}
