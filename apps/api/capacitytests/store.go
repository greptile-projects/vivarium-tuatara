// Package capacitytests retains revision-exact, bounded capacity experiments and their evidence.
package capacitytests

import (
	"crypto/sha256"
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

var ErrNotFound = errors.New("capacity test not found")
var ErrInvalid = errors.New("invalid capacity test")
var ErrConflict = errors.New("capacity test conflict")

type Component struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Revision   string `json:"revision"`
}
type Candidate struct {
	ID           string      `json:"id"`
	Name         string      `json:"name"`
	Strategy     string      `json:"strategy"`
	Components   []Component `json:"components"`
	ExpectedCost float64     `json:"expected_cost"`
	Currency     string      `json:"currency"`
}
type Workload struct {
	Kind                   string `json:"kind"`
	SourcePath             string `json:"source_path"`
	Sanitization           string `json:"sanitization"`
	ContainsProductionData bool   `json:"contains_production_data"`
}
type Limits struct {
	MaxDurationSeconds      int     `json:"max_duration_seconds"`
	MaxRequests             int     `json:"max_requests"`
	MaxConcurrency          int     `json:"max_concurrency"`
	MaxCost                 float64 `json:"max_cost"`
	CoordinatedLoadKey      string  `json:"coordinated_load_key"`
	ProductionImpactAllowed bool    `json:"production_impact_allowed"`
}
type Scenario struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Kind                string   `json:"kind"`
	Command             []string `json:"command"`
	Workload            Workload `json:"workload"`
	Limits              Limits   `json:"limits"`
	CorrectnessCriteria []string `json:"correctness_criteria"`
}
type Plan struct {
	RequestID        string      `json:"request_id"`
	ID               string      `json:"id"`
	RepositoryID     string      `json:"repository_id"`
	ObjectiveID      string      `json:"objective_id"`
	ObjectiveVersion int         `json:"objective_version"`
	ModelID          string      `json:"model_id,omitempty"`
	ModelVersion     int         `json:"model_version,omitempty"`
	Title            string      `json:"title"`
	EnvironmentKind  string      `json:"environment_kind"`
	EnvironmentID    string      `json:"environment_id,omitempty"`
	Candidates       []Candidate `json:"candidates"`
	Scenarios        []Scenario  `json:"scenarios"`
	CreatedBy        string      `json:"created_by"`
	CreatedAt        time.Time   `json:"created_at"`
}
type Metrics struct {
	Throughput      float64  `json:"throughput"`
	ThroughputUnit  string   `json:"throughput_unit"`
	LatencyP50MS    float64  `json:"latency_p50_ms"`
	LatencyP95MS    float64  `json:"latency_p95_ms"`
	LatencyP99MS    float64  `json:"latency_p99_ms"`
	ErrorRate       float64  `json:"error_rate"`
	Saturation      float64  `json:"saturation"`
	RecoverySeconds float64  `json:"recovery_seconds"`
	ResourceAmount  float64  `json:"resource_amount"`
	ResourceUnit    string   `json:"resource_unit"`
	CarbonGrams     *float64 `json:"carbon_grams,omitempty"`
	Cost            float64  `json:"cost"`
	Currency        string   `json:"currency"`
}
type Run struct {
	RequestID         string    `json:"request_id"`
	ID                string    `json:"id"`
	PlanID            string    `json:"plan_id"`
	CandidateID       string    `json:"candidate_id"`
	ScenarioID        string    `json:"scenario_id"`
	CandidateDigest   string    `json:"candidate_digest"`
	Status            string    `json:"status"`
	Repetitions       int       `json:"repetitions"`
	NoiseRatio        float64   `json:"noise_ratio"`
	Comparable        bool      `json:"comparable"`
	CorrectnessPassed bool      `json:"correctness_passed"`
	LimitBreaches     []string  `json:"limit_breaches"`
	Metrics           Metrics   `json:"metrics"`
	LogsDigest        string    `json:"logs_digest"`
	ActorType         string    `json:"actor_type"`
	ActorID           string    `json:"actor_id"`
	Quality           string    `json:"quality"`
	CreatedAt         time.Time `json:"created_at"`
}
type Comparison struct {
	PlanID             string   `json:"plan_id"`
	Runs               []Run    `json:"runs"`
	ProvenCandidateIDs []string `json:"proven_candidate_ids"`
	Diagnostics        []string `json:"diagnostics"`
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
func (s *Store) Create(repositoryID, actor, request string, p Plan) (Plan, error) {
	var out Plan
	e := s.lock(func() error {
		if request == "" || !validPlan(p) {
			return ErrInvalid
		}
		id := stable(repositoryID, actor, request)
		p.RequestID = request
		p.ID = id
		p.RepositoryID = repositoryID
		p.CreatedBy = actor
		if old, x := s.readPlan(id); x == nil {
			if digestPlan(old) != digestPlan(p) {
				return ErrConflict
			}
			out = old
			return nil
		}
		p.CreatedAt = s.now()
		out = p
		return s.write("plan-"+id, p)
	})
	return out, e
}
func (s *Store) AddRun(repositoryID, actorType, actor, planID, request string, r Run) (Run, error) {
	var out Run
	e := s.lock(func() error {
		p, x := s.readPlan(planID)
		if x != nil || p.RepositoryID != repositoryID {
			return ErrNotFound
		}
		if request == "" {
			return ErrInvalid
		}
		id := stable(planID, actor, request)
		r.RequestID = request
		r.ID = id
		r.PlanID = planID
		r.ActorType = actorType
		r.ActorID = actor
		for _, c := range p.Candidates {
			if c.ID == r.CandidateID {
				r.CandidateDigest = digest(c)
			}
		}
		found := false
		for _, q := range p.Scenarios {
			if q.ID == r.ScenarioID {
				found = true
				if r.Metrics.Cost > q.Limits.MaxCost {
					r.LimitBreaches = appendUnique(r.LimitBreaches, "max_cost")
				}
			}
		}
		for _, c := range p.Candidates {
			if c.ID == r.CandidateID && c.Currency != r.Metrics.Currency {
				r.Comparable = false
			}
		}
		if r.CandidateDigest == "" || !found || !validRun(r) {
			return ErrInvalid
		}
		if old, x := s.readRun(id); x == nil {
			if digestRun(old) != digestRun(r) {
				return ErrConflict
			}
			out = old
			return nil
		}
		r.Quality = quality(r)
		r.CreatedAt = s.now()
		out = r
		return s.write("run-"+id, r)
	})
	return out, e
}
func (s *Store) Get(repositoryID, id string) (Plan, error) {
	var p Plan
	e := s.lock(func() error {
		var x error
		p, x = s.readPlan(id)
		if x == nil && p.RepositoryID != repositoryID {
			return ErrNotFound
		}
		return x
	})
	return p, e
}
func (s *Store) List(repositoryID string) ([]Plan, error) {
	xs := []Plan{}
	e := s.lock(func() error {
		fs, x := os.ReadDir(s.root)
		if x != nil {
			return x
		}
		for _, f := range fs {
			if !strings.HasPrefix(f.Name(), "plan-") || !strings.HasSuffix(f.Name(), ".json") {
				continue
			}
			var p Plan
			if x = s.decode(f.Name(), &p); x != nil {
				return x
			}
			if p.RepositoryID == repositoryID {
				xs = append(xs, p)
			}
		}
		return nil
	})
	sort.Slice(xs, func(i, j int) bool { return xs[i].CreatedAt.After(xs[j].CreatedAt) })
	return xs, e
}
func (s *Store) Compare(repositoryID, planID string) (Comparison, error) {
	p, e := s.Get(repositoryID, planID)
	if e != nil {
		return Comparison{}, e
	}
	runs := []Run{}
	_ = s.lock(func() error {
		fs, _ := os.ReadDir(s.root)
		for _, f := range fs {
			if strings.HasPrefix(f.Name(), "run-") {
				var r Run
				if s.decode(f.Name(), &r) == nil && r.PlanID == planID {
					runs = append(runs, r)
				}
			}
		}
		return nil
	})
	out := Comparison{PlanID: planID, Runs: runs}
	coverage := map[string]map[string]bool{}
	for _, r := range runs {
		if r.Quality == "proof" {
			if coverage[r.CandidateID] == nil {
				coverage[r.CandidateID] = map[string]bool{}
			}
			coverage[r.CandidateID][r.ScenarioID] = true
		}
		if r.Quality != "proof" {
			out.Diagnostics = append(out.Diagnostics, r.CandidateID+"/"+r.ScenarioID+": "+r.Quality)
		}
	}
	if len(runs) == 0 {
		out.Diagnostics = []string{"no retained executions"}
	}
	for _, candidate := range p.Candidates {
		complete := true
		for _, scenario := range p.Scenarios {
			if !coverage[candidate.ID][scenario.ID] {
				complete = false
				out.Diagnostics = append(out.Diagnostics, candidate.ID+"/"+scenario.ID+": missing proof")
			}
		}
		if complete {
			out.ProvenCandidateIDs = append(out.ProvenCandidateIDs, candidate.ID)
		}
	}
	return out, nil
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
func validPlan(p Plan) bool {
	if p.ObjectiveID == "" || p.ObjectiveVersion < 1 || p.Title == "" || (p.EnvironmentKind != "isolated" && p.EnvironmentKind != "policy_approved") || len(p.Candidates) < 2 || len(p.Scenarios) == 0 {
		return false
	}
	strategies := map[string]bool{"vertical": true, "horizontal": true, "architectural": true, "caching": true, "queueing": true, "demand_shaping": true}
	ids := map[string]bool{}
	for _, c := range p.Candidates {
		if c.ID == "" || ids[c.ID] || !strategies[c.Strategy] || len(c.Components) == 0 || c.ExpectedCost < 0 {
			return false
		}
		ids[c.ID] = true
		for _, x := range c.Components {
			if x.ResourceID == "" || x.Revision == "" || !(x.Kind == "release" || x.Kind == "infrastructure" || x.Kind == "schema" || x.Kind == "dependency_configuration") {
				return false
			}
		}
	}
	ids = map[string]bool{}
	for _, q := range p.Scenarios {
		if q.ID == "" || ids[q.ID] || len(q.Command) == 0 || len(q.CorrectnessCriteria) == 0 || q.Workload.ContainsProductionData || (q.Workload.Kind != "synthetic" && q.Workload.Kind != "privacy_preserving") || q.Workload.Sanitization == "" || q.Limits.MaxDurationSeconds < 1 || q.Limits.MaxDurationSeconds > 3600 || q.Limits.MaxRequests < 1 || q.Limits.MaxConcurrency < 1 || q.Limits.MaxCost < 0 || q.Limits.ProductionImpactAllowed || q.Limits.CoordinatedLoadKey == "" {
			return false
		}
		ids[q.ID] = true
	}
	return true
}
func validRun(r Run) bool {
	return (r.Status == "succeeded" || r.Status == "failed" || r.Status == "untestable" || r.Status == "canceled") && r.Repetitions >= 0 && r.Repetitions <= 20 && r.NoiseRatio >= 0 && r.ErrorRateOK() && r.LogsDigest != ""
}
func (r Run) ErrorRateOK() bool {
	return r.Metrics.ErrorRate >= 0 && r.Metrics.ErrorRate <= 1 && r.Metrics.LatencyP50MS >= 0 && r.Metrics.LatencyP95MS >= r.Metrics.LatencyP50MS && r.Metrics.LatencyP99MS >= r.Metrics.LatencyP95MS
}
func quality(r Run) string {
	if r.Status == "untestable" || r.Status == "canceled" {
		return "untestable"
	}
	if len(r.LimitBreaches) > 0 {
		return "limit_breached"
	}
	if r.Status != "succeeded" || !r.CorrectnessPassed {
		return "failed"
	}
	if !r.Comparable {
		return "incomparable"
	}
	if r.Repetitions < 3 || r.NoiseRatio > .15 {
		return "noisy"
	}
	return "proof"
}
func stable(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(h[:16])
}
func digest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func digestPlan(p Plan) string {
	p.ID = ""
	p.RepositoryID = ""
	p.RequestID = ""
	p.CreatedBy = ""
	p.CreatedAt = time.Time{}
	return digest(p)
}
func digestRun(r Run) string {
	r.ID = ""
	r.PlanID = ""
	r.RequestID = ""
	r.ActorID = ""
	r.ActorType = ""
	r.CreatedAt = time.Time{}
	r.Quality = ""
	return digest(r)
}
func (s *Store) readPlan(id string) (Plan, error) {
	var p Plan
	e := s.decode("plan-"+id+".json", &p)
	return p, e
}
func (s *Store) readRun(id string) (Run, error) {
	var r Run
	e := s.decode("run-"+id+".json", &r)
	return r, e
}
func (s *Store) decode(name string, v any) error {
	b, e := os.ReadFile(filepath.Join(s.root, name))
	if os.IsNotExist(e) {
		return ErrNotFound
	}
	if e != nil {
		return e
	}
	return json.Unmarshal(b, v)
}
func (s *Store) write(prefix string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(s.root, ".capacity-")
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
	if x := tmp.Close(); e == nil {
		e = x
	}
	if e != nil {
		return e
	}
	return os.Rename(name, filepath.Join(s.root, prefix+".json"))
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
