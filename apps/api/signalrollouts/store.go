// Package signalrollouts retains progressive telemetry collection and its immutable evidence.
package signalrollouts

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
	"time"
)

var ErrNotFound = errors.New("signal rollout not found")
var ErrInvalid = errors.New("invalid signal rollout")
var ErrConflict = errors.New("signal rollout conflict")

type Scope struct {
	Service        string  `json:"service"`
	Audience       string  `json:"audience"`
	Region         string  `json:"region"`
	TrafficPercent float64 `json:"traffic_percent"`
}
type Budget struct {
	StorageBytes   int64 `json:"storage_bytes"`
	QueryCostCents int64 `json:"query_cost_cents"`
	Cardinality    int64 `json:"cardinality"`
}
type Quality struct {
	SignalHealth            string   `json:"signal_health"`
	Coverage                float64  `json:"coverage"`
	LatencyMS               float64  `json:"latency_ms"`
	Missingness             float64  `json:"missingness"`
	SamplingBias            float64  `json:"sampling_bias"`
	Cardinality             int64    `json:"cardinality"`
	StorageBytes            int64    `json:"storage_bytes"`
	QueryCostCents          int64    `json:"query_cost_cents"`
	PipelineLoss            float64  `json:"pipeline_loss"`
	PrivacyControls         []string `json:"privacy_controls"`
	MalformedPayloads       int64    `json:"malformed_payloads"`
	UnexpectedSensitiveData bool     `json:"unexpected_sensitive_data"`
	CollectorAvailable      bool     `json:"collector_available"`
	ServiceRegression       bool     `json:"service_regression"`
}
type Observation struct {
	ID           string    `json:"id"`
	Scope        Scope     `json:"scope"`
	Quality      Quality   `json:"quality"`
	DeploymentID string    `json:"deployment_id,omitempty"`
	ReleaseID    string    `json:"release_id,omitempty"`
	CommitID     string    `json:"commit_id,omitempty"`
	StartedAt    time.Time `json:"started_at"`
	EndedAt      time.Time `json:"ended_at"`
	Digest       string    `json:"digest"`
	CreatedBy    string    `json:"created_by"`
	CreatedAt    time.Time `json:"created_at"`
}
type Event struct {
	RequestID   string       `json:"request_id"`
	Sequence    int          `json:"sequence"`
	Kind        string       `json:"kind"`
	ActorID     string       `json:"actor_id"`
	Reason      string       `json:"reason"`
	Scope       *Scope       `json:"scope,omitempty"`
	Observation *Observation `json:"observation,omitempty"`
	CreatedAt   time.Time    `json:"created_at"`
}
type Rollout struct {
	RequestID               string        `json:"request_id"`
	ID                      string        `json:"id"`
	RepositoryID            string        `json:"repository_id"`
	ContractID              string        `json:"contract_id"`
	ContractVersion         int           `json:"contract_version"`
	InstrumentationRevision string        `json:"instrumentation_revision"`
	DeploymentID            string        `json:"deployment_id"`
	EnvironmentID           string        `json:"environment_id"`
	ControllerID            string        `json:"controller_id"`
	Scope                   Scope         `json:"scope"`
	Budget                  Budget        `json:"budget"`
	Status                  string        `json:"status"`
	ContainmentReasons      []string      `json:"containment_reasons"`
	Version                 int           `json:"version"`
	Observations            []Observation `json:"observations"`
	Events                  []Event       `json:"events"`
	CreatedBy               string        `json:"created_by"`
	CreatedAt               time.Time     `json:"created_at"`
	UpdatedAt               time.Time     `json:"updated_at"`
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
func (s *Store) Create(repo, actor, request string, r Rollout) (Rollout, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if request == "" || r.ContractID == "" || r.ContractVersion < 1 || r.InstrumentationRevision == "" || r.DeploymentID == "" || r.EnvironmentID == "" || r.ControllerID == "" || !validScope(r.Scope) || r.Budget.StorageBytes < 0 || r.Budget.QueryCostCents < 0 || r.Budget.Cardinality < 0 {
		return Rollout{}, ErrInvalid
	}
	r.ID = stable(repo, actor, request)
	r.RequestID = request
	r.RepositoryID = repo
	r.CreatedBy = actor
	if old, e := s.read(r.ID); e == nil {
		if digestCreate(old) != digestCreate(r) {
			return Rollout{}, ErrConflict
		}
		return old, nil
	}
	now := s.now()
	r.Version = 1
	r.Status = "paused"
	r.ContainmentReasons = []string{}
	r.Observations = []Observation{}
	r.CreatedAt = now
	r.UpdatedAt = now
	r.Events = []Event{{RequestID: request + ":stage", Sequence: 1, Kind: "stage", ActorID: actor, Reason: "reviewed instrumentation staged in protected environment", CreatedAt: now}}
	return r, s.write(r)
}
func (s *Store) Get(repo, id string) (Rollout, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, e := s.read(id)
	if e != nil || r.RepositoryID != repo {
		return Rollout{}, ErrNotFound
	}
	return r, nil
}
func (s *Store) List(repo string) ([]Rollout, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Rollout{}
	fs, e := os.ReadDir(s.root)
	if e != nil {
		return nil, e
	}
	for _, f := range fs {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
			continue
		}
		var r Rollout
		if e = s.decode(f.Name(), &r); e != nil {
			return nil, e
		}
		if r.RepositoryID == repo {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}
func (s *Store) Mutate(repo, id string, expected int, ev Event) (Rollout, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, e := s.read(id)
	if e != nil || r.RepositoryID != repo {
		return Rollout{}, ErrNotFound
	}
	for _, x := range r.Events {
		if x.RequestID == ev.RequestID {
			ev.Sequence = x.Sequence
			ev.CreatedAt = x.CreatedAt
			if ev.Observation != nil && x.Observation != nil {
				o := *ev.Observation
				o.ID = x.Observation.ID
				o.CreatedBy = x.Observation.CreatedBy
				o.CreatedAt = x.Observation.CreatedAt
				o.Digest = digest(o.Quality)
				ev.Observation = &o
			}
			if digest(x) != digest(ev) {
				return Rollout{}, ErrConflict
			}
			return r, nil
		}
	}
	if expected != r.Version || ev.RequestID == "" || ev.ActorID == "" || strings.TrimSpace(ev.Reason) == "" {
		return Rollout{}, ErrConflict
	}
	switch ev.Kind {
	case "pause":
		if r.Status != "staged" && r.Status != "running" && r.Status != "narrowed" && r.Status != "paused" {
			return Rollout{}, ErrInvalid
		}
		r.Status = "paused"
	case "resume":
		if len(r.ContainmentReasons) > 0 || r.Status != "paused" && r.Status != "staged" {
			return Rollout{}, ErrInvalid
		}
		r.Status = "running"
	case "narrow":
		if len(r.ContainmentReasons) > 0 || r.Status == "rolled_back" || ev.Scope == nil || !validScope(*ev.Scope) || ev.Scope.Service != r.Scope.Service || ev.Scope.Audience != r.Scope.Audience || ev.Scope.Region != r.Scope.Region || ev.Scope.TrafficPercent > r.Scope.TrafficPercent {
			return Rollout{}, ErrInvalid
		}
		r.Scope = *ev.Scope
		r.Status = "narrowed"
	case "rollback":
		if r.Status == "rolled_back" {
			return Rollout{}, ErrInvalid
		}
		r.Status = "rolled_back"
	case "verify_stopped":
		if r.Status != "rolled_back" || ev.Observation == nil || !validObservation(*ev.Observation) || !sameScope(ev.Observation.Scope, r.Scope) || ev.Observation.Quality.CollectorAvailable {
			return Rollout{}, ErrInvalid
		}
		o := *ev.Observation
		o.ID = stable(r.ID, ev.RequestID)
		o.CreatedBy = ev.ActorID
		o.CreatedAt = s.now()
		o.Digest = digest(o.Quality)
		r.Observations = append(r.Observations, o)
		ev.Observation = &o
	case "observe":
		if r.Status == "rolled_back" {
			return Rollout{}, ErrInvalid
		}
		if ev.Observation == nil || !validObservation(*ev.Observation) || !sameScope(ev.Observation.Scope, r.Scope) {
			return Rollout{}, ErrInvalid
		}
		o := *ev.Observation
		o.ID = stable(r.ID, ev.RequestID)
		o.CreatedBy = ev.ActorID
		o.CreatedAt = s.now()
		o.Digest = digest(o.Quality)
		r.Observations = append(r.Observations, o)
		reasons := contain(o.Quality, r.Budget)
		if len(r.ContainmentReasons) > 0 {
			reasons = mergeReasons(r.ContainmentReasons, reasons)
		}
		r.ContainmentReasons = reasons
		if len(reasons) > 0 {
			r.Status = "contained"
		}
		ev.Observation = &o
	case "resolve":
		if r.Status != "contained" || len(r.ContainmentReasons) == 0 || ev.Observation == nil || !validObservation(*ev.Observation) || !sameScope(ev.Observation.Scope, r.Scope) || len(contain(ev.Observation.Quality, r.Budget)) > 0 {
			return Rollout{}, ErrInvalid
		}
		o := *ev.Observation
		o.ID = stable(r.ID, ev.RequestID)
		o.CreatedBy = ev.ActorID
		o.CreatedAt = s.now()
		o.Digest = digest(o.Quality)
		r.Observations = append(r.Observations, o)
		r.ContainmentReasons = []string{}
		r.Status = "paused"
		ev.Observation = &o
	default:
		return Rollout{}, ErrInvalid
	}
	r.Version++
	r.UpdatedAt = s.now()
	ev.Sequence = len(r.Events) + 1
	ev.CreatedAt = r.UpdatedAt
	r.Events = append(r.Events, ev)
	return r, s.write(r)
}

func mergeReasons(existing, observed []string) []string {
	out := append([]string{}, existing...)
	seen := map[string]bool{}
	for _, reason := range out {
		seen[reason] = true
	}
	for _, reason := range observed {
		if !seen[reason] {
			out = append(out, reason)
			seen[reason] = true
		}
	}
	return out
}
func contain(q Quality, b Budget) []string {
	v := []string{}
	if q.StorageBytes > b.StorageBytes || q.QueryCostCents > b.QueryCostCents || q.Cardinality > b.Cardinality {
		v = append(v, "budget_breach")
	}
	if q.MalformedPayloads > 0 {
		v = append(v, "malformed_payload")
	}
	if q.UnexpectedSensitiveData {
		v = append(v, "unexpected_sensitive_data")
	}
	if !q.CollectorAvailable {
		v = append(v, "collector_outage")
	}
	if q.SamplingBias > 0.1 {
		v = append(v, "sampling_skew")
	}
	if q.ServiceRegression {
		v = append(v, "service_regression")
	}
	return v
}

// ContainmentReasons exposes the deterministic quality/budget boundary to
// revision-exact consumers without granting collection authority.
func ContainmentReasons(q Quality, b Budget) []string { return contain(q, b) }
func validScope(x Scope) bool {
	return x.Service != "" && x.Audience != "" && x.Region != "" && x.TrafficPercent > 0 && x.TrafficPercent <= 100
}
func sameScope(a, b Scope) bool {
	return a.Service == b.Service && a.Audience == b.Audience && a.Region == b.Region && a.TrafficPercent == b.TrafficPercent
}
func validObservation(x Observation) bool {
	return !x.StartedAt.IsZero() && !x.EndedAt.Before(x.StartedAt) && x.Quality.SignalHealth != "" && x.Quality.Coverage >= 0 && x.Quality.Coverage <= 1 && x.Quality.Missingness >= 0 && x.Quality.Missingness <= 1 && x.Quality.PipelineLoss >= 0 && x.Quality.PipelineLoss <= 1 && len(x.Quality.PrivacyControls) > 0 && validScope(x.Scope)
}
func stable(v ...string) string {
	h := sha256.Sum256([]byte(strings.Join(v, "\x00")))
	return "signal-rollout-" + hex.EncodeToString(h[:12])
}
func digest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func digestCreate(r Rollout) string {
	r.Version = 0
	r.Status = ""
	r.Events = nil
	r.Observations = nil
	r.CreatedAt = time.Time{}
	r.UpdatedAt = time.Time{}
	r.ContainmentReasons = nil
	return digest(r)
}
func (s *Store) path(id string) string { return filepath.Join(s.root, id+".json") }
func (s *Store) read(id string) (Rollout, error) {
	var r Rollout
	e := s.decode(id+".json", &r)
	if os.IsNotExist(e) {
		return r, ErrNotFound
	}
	return r, e
}
func (s *Store) decode(name string, v any) error {
	b, e := os.ReadFile(filepath.Join(s.root, name))
	if e != nil {
		return e
	}
	return json.Unmarshal(b, v)
}
func (s *Store) write(r Rollout) error {
	b, e := json.MarshalIndent(r, "", "  ")
	if e != nil {
		return e
	}
	tmp := s.path(r.ID) + ".tmp"
	if e = os.WriteFile(tmp, b, 0600); e != nil {
		return e
	}
	return os.Rename(tmp, s.path(r.ID))
}
