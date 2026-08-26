// Package capacityrollouts retains governed production capacity execution and evidence.
package capacityrollouts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

var ErrNotFound = errors.New("capacity rollout not found")
var ErrInvalid = errors.New("invalid capacity rollout")
var ErrConflict = errors.New("capacity rollout conflict")

type Phase struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	ControllerType      string   `json:"controller_type"`
	ControllerID        string   `json:"controller_id"`
	EnvironmentID       string   `json:"environment_id"`
	DeploymentIDs       []string `json:"deployment_ids"`
	DeployedRevisions   []string `json:"deployed_revisions"`
	State               string   `json:"state"`
	ThrottlePercent     float64  `json:"throttle_percent"`
	Blockers            []string `json:"blockers"`
	PredictedNextAction string   `json:"predicted_next_action"`
}
type RegionEvidence struct {
	Region            string  `json:"region"`
	AllocatedCapacity float64 `json:"allocated_capacity"`
	UsableCapacity    float64 `json:"usable_capacity"`
	Load              float64 `json:"load"`
	Unit              string  `json:"unit"`
}
type Evidence struct {
	ID                 string             `json:"id"`
	PhaseID            string             `json:"phase_id"`
	Source             string             `json:"source"`
	ObservationStart   time.Time          `json:"observation_start"`
	ObservationEnd     time.Time          `json:"observation_end"`
	DeploymentIDs      []string           `json:"deployment_ids"`
	DeployedRevisions  []string           `json:"deployed_revisions"`
	Regions            []RegionEvidence   `json:"regions"`
	AllocatedCapacity  float64            `json:"allocated_capacity"`
	UsableCapacity     float64            `json:"usable_capacity"`
	Load               float64            `json:"load"`
	Headroom           float64            `json:"headroom"`
	Unit               string             `json:"unit"`
	ServiceLevels      map[string]float64 `json:"service_levels"`
	DependencyHealth   map[string]string  `json:"dependency_health"`
	Cost               float64            `json:"cost"`
	Currency           string             `json:"currency"`
	ForecastDemand     float64            `json:"forecast_demand"`
	ObjectiveSatisfied bool               `json:"objective_satisfied"`
	ForecastValidated  bool               `json:"forecast_validated"`
	FailureKinds       []string           `json:"failure_kinds"`
	CreatedBy          string             `json:"created_by"`
	CreatedAt          time.Time          `json:"created_at"`
}
type Event struct {
	RequestID       string    `json:"request_id"`
	Sequence        int       `json:"sequence"`
	Kind            string    `json:"kind"`
	PhaseID         string    `json:"phase_id"`
	ActorType       string    `json:"actor_type"`
	ActorID         string    `json:"actor_id"`
	Reason          string    `json:"reason"`
	ThrottlePercent float64   `json:"throttle_percent,omitempty"`
	DecisionID      string    `json:"decision_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}
type Rollout struct {
	RequestID      string     `json:"request_id"`
	ID             string     `json:"id"`
	RepositoryID   string     `json:"repository_id"`
	PlanID         string     `json:"plan_id"`
	EnvironmentIDs []string   `json:"environment_ids"`
	Version        int        `json:"version"`
	Status         string     `json:"status"`
	Phases         []Phase    `json:"phases"`
	Evidence       []Evidence `json:"evidence"`
	Events         []Event    `json:"events"`
	CreatedBy      string     `json:"created_by"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
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
func (s *Store) Create(repo, actor, request string, r Rollout) (Rollout, error) {
	var out Rollout
	e := s.lock(func() error {
		if request == "" || r.PlanID == "" || len(r.Phases) == 0 {
			return ErrInvalid
		}
		r.RequestID = request
		r.ID = stable(repo, actor, request)
		r.RepositoryID = repo
		r.CreatedBy = actor
		if old, x := s.read(r.ID); x == nil {
			if createDigest(old) != createDigest(r) {
				return ErrConflict
			}
			out = old
			return nil
		}
		r.Version = 1
		r.Status = "staged"
		now := s.now()
		r.CreatedAt = now
		r.UpdatedAt = now
		r.Evidence = []Evidence{}
		r.Events = []Event{{RequestID: request + ":stage", Sequence: 1, Kind: "stage", ActorType: "human", ActorID: actor, Reason: "protected capacity rollout staged", CreatedAt: now}}
		for i := range r.Phases {
			if r.Phases[i].ID == "" || r.Phases[i].ControllerID == "" || r.Phases[i].EnvironmentID == "" || len(r.Phases[i].DeploymentIDs) == 0 || len(r.Phases[i].DeploymentIDs) != len(r.Phases[i].DeployedRevisions) {
				return ErrInvalid
			}
			r.Phases[i].State = "staged"
			r.Phases[i].ThrottlePercent = 100
			r.Phases[i].Blockers = []string{}
			r.Phases[i].PredictedNextAction = "resume when protected-environment approval and current production evidence are available"
		}
		out = r
		return s.write(r)
	})
	return out, e
}
func (s *Store) Get(repo, id string) (Rollout, error) {
	var out Rollout
	e := s.lock(func() error {
		var x error
		out, x = s.read(id)
		if x == nil && out.RepositoryID != repo {
			return ErrNotFound
		}
		return x
	})
	return out, e
}
func (s *Store) List(repo string) ([]Rollout, error) {
	out := []Rollout{}
	e := s.lock(func() error {
		fs, x := os.ReadDir(s.root)
		if x != nil {
			return x
		}
		for _, f := range fs {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
				continue
			}
			var r Rollout
			if x = s.decode(f.Name(), &r); x != nil {
				return x
			}
			if r.RepositoryID == repo {
				out = append(out, r)
			}
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, e
}
func (s *Store) Mutate(repo, id string, expected int, ev Event, evidence *Evidence) (Rollout, error) {
	var out Rollout
	e := s.lock(func() error {
		r, x := s.read(id)
		if x != nil || r.RepositoryID != repo {
			return ErrNotFound
		}
		for _, old := range r.Events {
			if old.RequestID == ev.RequestID {
				if digest(old) != digestWithSequence(ev, old.Sequence, old.CreatedAt) {
					return ErrConflict
				}
				if ev.Kind == "observe" {
					if evidence == nil {
						return ErrConflict
					}
					matched := false
					for _, retained := range r.Evidence {
						if retained.ID == stable(r.ID, ev.RequestID) {
							candidate := normalizedEvidence(*evidence, retained.ID, retained.PhaseID, retained.CreatedBy, retained.CreatedAt)
							matched = digest(candidate) == digest(retained)
							break
						}
					}
					if !matched {
						return ErrConflict
					}
				}
				out = r
				return nil
			}
		}
		if expected != r.Version || ev.RequestID == "" || ev.ActorID == "" {
			return ErrConflict
		}
		idx := -1
		for i := range r.Phases {
			if r.Phases[i].ID == ev.PhaseID {
				idx = i
			}
		}
		if idx < 0 {
			return ErrInvalid
		}
		p := &r.Phases[idx]
		switch ev.Kind {
		case "pause":
			p.State = "paused"
			p.PredictedNextAction = "resume after blockers clear and evidence is current"
		case "resume":
			if p.State != "paused" && p.State != "staged" {
				return ErrInvalid
			}
			p.State = "running"
			p.PredictedNextAction = "observe production load, headroom, reliability, dependencies, and cost"
		case "throttle":
			if ev.ThrottlePercent < 0 || ev.ThrottlePercent > 100 {
				return ErrInvalid
			}
			p.State = "throttled"
			p.ThrottlePercent = ev.ThrottlePercent
			p.PredictedNextAction = "re-evaluate containment at the throttled load"
		case "rollback":
			p.State = "rolled_back"
			p.PredictedNextAction = "verify restored capacity and reliability"
		case "replan":
			if strings.TrimSpace(ev.DecisionID) == "" {
				return ErrInvalid
			}
			p.State = "replan_required"
			p.PredictedNextAction = "revisit connected decision " + ev.DecisionID
		case "observe":
			if evidence == nil || !validEvidence(*evidence) {
				return ErrInvalid
			}
			retained := normalizedEvidence(*evidence, stable(r.ID, ev.RequestID), p.ID, ev.ActorID, s.now())
			r.Evidence = append(r.Evidence, retained)
			p.DeploymentIDs = append([]string{}, evidence.DeploymentIDs...)
			p.DeployedRevisions = append([]string{}, evidence.DeployedRevisions...)
			p.Blockers = append([]string{}, retained.FailureKinds...)
			if !retained.ObjectiveSatisfied {
				p.State = "contained"
				p.ThrottlePercent = 0
				p.PredictedNextAction = containmentAction(retained.FailureKinds, ev.DecisionID)
			} else {
				p.State = "verified"
				p.PredictedNextAction = "advance to the next plan phase while evidence remains current"
			}
		default:
			return ErrInvalid
		}
		ev.Sequence = len(r.Events) + 1
		ev.CreatedAt = s.now()
		r.Events = append(r.Events, ev)
		r.Version++
		r.UpdatedAt = ev.CreatedAt
		r.Status = rolloutStatus(r.Phases)
		out = r
		return s.write(r)
	})
	return out, e
}
func validEvidence(e Evidence) bool {
	if e.Source != "production" || e.ObservationStart.IsZero() || !e.ObservationEnd.After(e.ObservationStart) || len(e.DeploymentIDs) == 0 || len(e.DeploymentIDs) != len(e.DeployedRevisions) || e.Unit == "" || e.Currency == "" {
		return false
	}
	vals := []float64{e.AllocatedCapacity, e.UsableCapacity, e.Load, e.Cost, e.ForecastDemand}
	for _, v := range vals {
		if v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return e.UsableCapacity <= e.AllocatedCapacity
}
func deriveEvidence(e *Evidence) {
	e.Headroom = e.UsableCapacity - e.Load
	if e.Headroom < 0 && !contains(e.FailureKinds, "insufficient_headroom") {
		e.FailureKinds = append(e.FailureKinds, "insufficient_headroom")
	}
	e.ObjectiveSatisfied = e.Headroom >= 0 && len(e.FailureKinds) == 0
	e.ForecastValidated = e.Load >= e.ForecastDemand && e.ObjectiveSatisfied
	known := map[string]bool{"demand_shift": true, "quota_denial": true, "regional_imbalance": true, "scaling_lag": true, "correctness_regression": true, "reliability_regression": true, "unused_reservation": true, "budget_breach": true}
	for _, k := range e.FailureKinds {
		if !known[k] {
			e.ObjectiveSatisfied = false
			e.ForecastValidated = false
		}
	}
}
func normalizedEvidence(e Evidence, id, phaseID, actor string, createdAt time.Time) Evidence {
	e.ID, e.PhaseID, e.CreatedBy, e.CreatedAt = id, phaseID, actor, createdAt
	deriveEvidence(&e)
	return e
}
func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
func containmentAction(kinds []string, decision string) string {
	revisit := false
	for _, k := range kinds {
		if k == "demand_shift" || k == "quota_denial" || k == "unused_reservation" || k == "budget_breach" {
			revisit = true
		}
	}
	if revisit && decision != "" {
		return "pause scaling and revisit connected decision " + decision
	}
	return "pause and throttle scaling; roll back if correctness or reliability does not recover"
}
func rolloutStatus(ps []Phase) string {
	all := true
	for _, p := range ps {
		if p.State == "contained" || p.State == "replan_required" || p.State == "rolled_back" {
			return p.State
		}
		all = all && p.State == "verified"
	}
	if all {
		return "verified"
	}
	return "in_progress"
}
func stable(v ...string) string {
	h := sha256.Sum256([]byte(strings.Join(v, "\x00")))
	return hex.EncodeToString(h[:16])
}
func digest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func createDigest(r Rollout) string {
	r.Version, r.Status, r.Evidence, r.Events = 0, "", nil, nil
	r.CreatedAt, r.UpdatedAt = time.Time{}, time.Time{}
	for i := range r.Phases {
		r.Phases[i].State, r.Phases[i].ThrottlePercent = "", 0
		r.Phases[i].Blockers, r.Phases[i].PredictedNextAction = nil, ""
	}
	return digest(r)
}
func digestWithSequence(e Event, n int, t time.Time) string {
	e.Sequence = n
	e.CreatedAt = t
	return digest(e)
}
func (s *Store) read(id string) (Rollout, error) {
	var r Rollout
	e := s.decode(id+".json", &r)
	if os.IsNotExist(e) {
		e = ErrNotFound
	}
	return r, e
}
func (s *Store) decode(n string, v any) error {
	b, e := os.ReadFile(filepath.Join(s.root, n))
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
	tmp, e := os.CreateTemp(s.root, ".capacity-rollout-")
	if e != nil {
		return e
	}
	n := tmp.Name()
	defer os.Remove(n)
	if e = tmp.Chmod(0600); e == nil {
		_, e = tmp.Write(b)
	}
	if e == nil {
		e = tmp.Sync()
	}
	if x := tmp.Close(); e == nil {
		e = x
	}
	if e == nil {
		e = os.Rename(n, filepath.Join(s.root, r.ID+".json"))
	}
	return e
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
