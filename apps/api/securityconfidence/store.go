// Package securityconfidence retains scoped delivery policy, exceptions, and
// production signals. It evaluates caller-resolved security ledgers but grants
// no authority over those ledgers or delivery targets.
package securityconfidence

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

var ErrNotFound = errors.New("security confidence policy not found")
var ErrInvalid = errors.New("invalid security confidence record")
var ErrConflict = errors.New("security confidence version conflict")

type Selector struct {
	Branches    []string `json:"branches,omitempty"`
	Components  []string `json:"components,omitempty"`
	Assets      []string `json:"assets,omitempty"`
	RiskClasses []string `json:"risk_classes,omitempty"`
	Paths       []string `json:"paths,omitempty"`
}
type Requirement struct {
	ID                 string   `json:"id"`
	Title              string   `json:"title"`
	Kind               string   `json:"kind"`
	ThreatModelID      string   `json:"threat_model_id,omitempty"`
	ThreatModelVersion int      `json:"threat_model_version,omitempty"`
	AbusePathID        string   `json:"abuse_path_id,omitempty"`
	ScenarioID         string   `json:"scenario_id,omitempty"`
	OwnerIDs           []string `json:"owner_ids,omitempty"`
	FindingSeverities  []string `json:"finding_severities,omitempty"`
	Selector           Selector `json:"selector"`
}
type Policy struct {
	ID           string        `json:"id"`
	ScopeKind    string        `json:"scope_kind"`
	ScopeID      string        `json:"scope_id"`
	RepositoryID string        `json:"repository_id"`
	CreatedBy    string        `json:"created_by"`
	Version      int           `json:"version"`
	Requirements []Requirement `json:"requirements"`
	CreatedAt    time.Time     `json:"created_at"`
}
type Exception struct {
	ID            string    `json:"id"`
	RepositoryID  string    `json:"repository_id"`
	RequirementID string    `json:"requirement_id"`
	Revision      string    `json:"revision"`
	Rationale     string    `json:"rationale"`
	OwnerID       string    `json:"owner_id"`
	FollowUpKind  string    `json:"follow_up_kind"`
	FollowUpID    string    `json:"follow_up_id"`
	Scope         Selector  `json:"scope"`
	ExpiresAt     time.Time `json:"expires_at"`
	CreatedAt     time.Time `json:"created_at"`
}
type Evidence struct {
	ThreatCurrent        bool     `json:"threat_current"`
	ThreatRevision       int      `json:"threat_model_version,omitempty"`
	ScenarioAttemptID    string   `json:"scenario_attempt_id,omitempty"`
	ScenarioResult       string   `json:"scenario_result,omitempty"`
	AcknowledgedOwnerIDs []string `json:"acknowledged_owner_ids,omitempty"`
	OpenFindingIDs       []string `json:"open_finding_ids,omitempty"`
	ResidualRisk         string   `json:"residual_risk,omitempty"`
	AffectedPaths        []string `json:"affected_paths,omitempty"`
	DependencyPaths      []string `json:"dependency_paths,omitempty"`
}
type Cell struct {
	Requirement Requirement `json:"requirement"`
	State       string      `json:"state"`
	Gaps        []string    `json:"gaps"`
	Evidence    Evidence    `json:"evidence"`
	Exception   *Exception  `json:"exception,omitempty"`
}
type Matrix struct {
	TargetKind    string `json:"target_kind"`
	TargetID      string `json:"target_id"`
	Revision      string `json:"revision"`
	PolicyID      string `json:"policy_id"`
	PolicyVersion int    `json:"policy_version"`
	Ready         bool   `json:"ready"`
	Requirements  []Cell `json:"requirements"`
	Authority     string `json:"authority"`
}
type Signal struct {
	ID             string    `json:"id"`
	RepositoryID   string    `json:"repository_id"`
	ReleaseID      string    `json:"release_id"`
	DeploymentID   string    `json:"deployment_id"`
	Revision       string    `json:"revision"`
	EnvironmentID  string    `json:"environment_id"`
	RequirementID  string    `json:"requirement_id"`
	Kind           string    `json:"kind"`
	State          string    `json:"state"`
	Summary        string    `json:"summary"`
	ArtifactSHA256 string    `json:"artifact_sha256"`
	ReportedBy     string    `json:"reported_by"`
	ResponseKind   string    `json:"response_kind,omitempty"`
	ResponseID     string    `json:"response_id,omitempty"`
	AssumptionIDs  []string  `json:"assumption_ids,omitempty"`
	ControlIDs     []string  `json:"control_ids,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
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
func (s *Store) Publish(scopeKind, scopeID, repo, actor string, expected int, reqs []Requirement) (Policy, error) {
	var out Policy
	err := s.lock(func() error {
		all, e := read[Policy](filepath.Join(s.root, scopeKind+"-"+scopeID+"-policies.jsonl"))
		if e != nil {
			return e
		}
		version, id := 1, newID()
		if len(all) > 0 {
			p := all[len(all)-1]
			if p.Version != expected {
				return ErrConflict
			}
			version, id = p.Version+1, p.ID
		} else if expected != 0 {
			return ErrConflict
		}
		if !one(scopeKind, "repository", "organization") || scopeID == "" || repo == "" || actor == "" || !validRequirements(reqs) {
			return ErrInvalid
		}
		out = Policy{id, scopeKind, scopeID, repo, actor, version, reqs, s.now()}
		return appendLine(filepath.Join(s.root, scopeKind+"-"+scopeID+"-policies.jsonl"), out)
	})
	return out, err
}
func (s *Store) Current(scopeKind, scopeID string) (Policy, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	xs, e := read[Policy](filepath.Join(s.root, scopeKind+"-"+scopeID+"-policies.jsonl"))
	if e != nil {
		return Policy{}, e
	}
	if len(xs) == 0 {
		return Policy{}, ErrNotFound
	}
	return xs[len(xs)-1], nil
}
func (s *Store) AddException(repo, actor string, x Exception) (Exception, error) {
	var out Exception
	err := s.lock(func() error {
		if x.RequirementID == "" || x.Revision == "" || len(strings.TrimSpace(x.Rationale)) < 10 || x.ExpiresAt.After(s.now().Add(30*24*time.Hour)) || !x.ExpiresAt.After(s.now()) || x.FollowUpID == "" || !one(x.FollowUpKind, "issue", "proposal") {
			return ErrInvalid
		}
		x.ID = newID()
		x.RepositoryID = repo
		x.OwnerID = actor
		x.CreatedAt = s.now()
		out = x
		return appendLine(filepath.Join(s.root, repo+"-exceptions.jsonl"), x)
	})
	return out, err
}
func (s *Store) Evaluate(p Policy, targetKind, targetID, revision, branch string, paths []string, evidence map[string]Evidence) (Matrix, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	exceptions, err := read[Exception](filepath.Join(s.root, p.RepositoryID+"-exceptions.jsonl"))
	if err != nil {
		return Matrix{}, err
	}
	m := Matrix{targetKind, targetID, revision, p.ID, p.Version, true, []Cell{}, "Security confidence is evidence only and grants no Git, review, merge, queue, release, deployment, disclosure, environment, incident, or agent authority."}
	for _, r := range p.Requirements {
		if !matches(r.Selector, branch, paths) {
			continue
		}
		ev := evidence[r.ID]
		c := Cell{Requirement: r, State: "gap", Gaps: []string{}, Evidence: ev}
		switch r.Kind {
		case "threat_coverage":
			if ev.ThreatCurrent && ev.ThreatRevision >= r.ThreatModelVersion {
				c.State = "passed"
			} else {
				c.Gaps = append(c.Gaps, "current threat coverage is missing")
			}
		case "security_scenario":
			if ev.ScenarioResult == "passed" && ev.ScenarioAttemptID != "" {
				c.State = "passed"
			} else {
				c.Gaps = append(c.Gaps, "a current passing reviewed security scenario is missing")
			}
		case "control_acknowledgement":
			if containsAll(ev.AcknowledgedOwnerIDs, r.OwnerIDs) {
				c.State = "passed"
			} else {
				c.Gaps = append(c.Gaps, "required control owners have not acknowledged the current model")
			}
		case "resolved_findings":
			if len(ev.OpenFindingIDs) == 0 {
				c.State = "passed"
			} else {
				c.State = "failed"
				c.Gaps = append(c.Gaps, "scoped security findings remain unresolved")
			}
		}
		for i := len(exceptions) - 1; i >= 0; i-- {
			x := exceptions[i]
			if x.RequirementID == r.ID && x.Revision == revision && x.ExpiresAt.After(s.now()) && matches(x.Scope, branch, paths) {
				y := x
				c.Exception = &y
				c.State = "overridden"
				break
			}
		}
		if c.State != "passed" && c.State != "overridden" {
			m.Ready = false
		}
		m.Requirements = append(m.Requirements, c)
	}
	return m, nil
}
func (s *Store) RecordSignal(repo, actor string, v Signal) (Signal, error) {
	var out Signal
	err := s.lock(func() error {
		if v.ReleaseID == "" || v.DeploymentID == "" || len(v.Revision) != 40 || v.EnvironmentID == "" || v.RequirementID == "" || !one(v.Kind, "assumption_violated", "control_failed") || !one(v.State, "observed", "confirmed") || strings.TrimSpace(v.Summary) == "" || len(v.ArtifactSHA256) != 64 {
			return ErrInvalid
		}
		if (v.ResponseKind == "") != (v.ResponseID == "") || v.ResponseKind != "" && !one(v.ResponseKind, "private_incident", "security_advisory", "repair") {
			return ErrInvalid
		}
		v.ID = newID()
		v.RepositoryID = repo
		v.ReportedBy = actor
		v.CreatedAt = s.now()
		out = v
		return appendLine(filepath.Join(s.root, repo+"-signals.jsonl"), v)
	})
	return out, err
}
func validRequirements(rs []Requirement) bool {
	if len(rs) == 0 || len(rs) > 100 {
		return false
	}
	seen := map[string]bool{}
	for _, r := range rs {
		if !bounded(r.ID, 200) || !bounded(r.Title, 500) || seen[r.ID] || !one(r.Kind, "threat_coverage", "security_scenario", "control_acknowledgement", "resolved_findings") || len(r.OwnerIDs) == 0 || !boundedList(r.OwnerIDs, 50, 200) || !boundedList(r.Selector.Branches, 50, 500) || !boundedList(r.Selector.Components, 50, 500) || !boundedList(r.Selector.Assets, 50, 500) || !boundedList(r.Selector.RiskClasses, 50, 500) || !boundedList(r.Selector.Paths, 100, 1000) {
			return false
		}
		for _, severity := range r.FindingSeverities {
			if !one(severity, "low", "medium", "high", "critical") {
				return false
			}
		}
		seen[r.ID] = true
		if r.Kind == "security_scenario" && r.ScenarioID == "" {
			return false
		}
		if r.Kind != "resolved_findings" && (r.ThreatModelID == "" || r.ThreatModelVersion < 1) {
			return false
		}
	}
	return true
}
func bounded(v string, max int) bool { return strings.TrimSpace(v) != "" && len(v) <= max }
func boundedList(xs []string, maxItems, maxLength int) bool {
	if len(xs) > maxItems {
		return false
	}
	for _, x := range xs {
		if !bounded(x, maxLength) {
			return false
		}
	}
	return true
}
func matches(s Selector, branch string, paths []string) bool {
	if len(s.Branches) > 0 && !contains(s.Branches, branch) {
		return false
	}
	if len(s.Paths) == 0 {
		return true
	}
	for _, a := range s.Paths {
		for _, b := range paths {
			if a == b || strings.HasPrefix(a, strings.TrimSuffix(b, "/")+"/") || strings.HasPrefix(b, strings.TrimSuffix(a, "/")+"/") {
				return true
			}
		}
	}
	return false
}
func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
func containsAll(xs, ys []string) bool {
	for _, y := range ys {
		if !contains(xs, y) {
			return false
		}
	}
	return true
}
func one(v string, xs ...string) bool { return contains(xs, v) }
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
func appendLine(path string, v any) error {
	if e := os.MkdirAll(filepath.Dir(path), 0700); e != nil {
		return e
	}
	f, e := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	b, _ := json.Marshal(v)
	if _, e = f.Write(append(b, '\n')); e == nil {
		e = f.Sync()
	}
	return e
}
func read[T any](path string) ([]T, error) {
	b, e := os.ReadFile(path)
	if errors.Is(e, os.ErrNotExist) {
		return []T{}, nil
	}
	if e != nil {
		return nil, e
	}
	out := []T{}
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if line == "" {
			continue
		}
		var v T
		if json.Unmarshal([]byte(line), &v) != nil {
			return nil, ErrInvalid
		}
		out = append(out, v)
	}
	return out, nil
}
func newID() string { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
