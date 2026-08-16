// Package recoveryoperations persists the authoritative shared workspace used
// to coordinate restoration after an incident or confirmed loss event.
package recoveryoperations

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

var ErrNotFound = errors.New("recovery operation not found")
var ErrInvalid = errors.New("invalid recovery operation")
var ErrConflict = errors.New("recovery operation changed")

type RecoveryPoint struct {
	PlanID               string    `json:"plan_id"`
	PlanVersion          int       `json:"plan_version"`
	CaptureID            string    `json:"capture_id"`
	SourceRevision       string    `json:"source_revision"`
	CapturedAt           time.Time `json:"captured_at"`
	EstimatedLossMinutes int       `json:"estimated_loss_minutes"`
	ManifestSHA256       string    `json:"manifest_sha256"`
}
type Step struct {
	ID                 string             `json:"id"`
	Name               string             `json:"name"`
	Kind               string             `json:"kind"`
	ResourceID         string             `json:"resource_id"`
	EnvironmentID      string             `json:"environment_id,omitempty"`
	DependsOn          []string           `json:"depends_on,omitempty"`
	AssigneeType       string             `json:"assignee_type"`
	AssigneeID         string             `json:"assignee_id"`
	Delegation         string             `json:"delegation,omitempty"`
	Destructive        bool               `json:"destructive"`
	ValidationCriteria []string           `json:"validation_criteria"`
	ValidationResults  []ValidationResult `json:"validation_results,omitempty"`
	Status             string             `json:"status"`
	Message            string             `json:"message,omitempty"`
	UpdatedBy          string             `json:"updated_by,omitempty"`
	UpdatedAt          *time.Time         `json:"updated_at,omitempty"`
}
type ValidationResult struct {
	Criterion string `json:"criterion"`
	Status    string `json:"status"`
	Evidence  string `json:"evidence"`
}
type Approval struct {
	ActorID   string    `json:"actor_id"`
	Decision  string    `json:"decision"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}
type Communication struct {
	ID        string    `json:"id"`
	ActorID   string    `json:"actor_id"`
	Audience  string    `json:"audience"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}
type Event struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	ActorID   string    `json:"actor_id"`
	StepID    string    `json:"step_id,omitempty"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}
type Revision struct {
	Version           int       `json:"version"`
	Objective         string    `json:"objective"`
	RequiredApprovals int       `json:"required_approvals"`
	ApproverIDs       []string  `json:"approver_ids"`
	RollbackOption    string    `json:"rollback_option"`
	Steps             []Step    `json:"steps"`
	CreatedBy         string    `json:"created_by"`
	CreatedAt         time.Time `json:"created_at"`
}
type Operation struct {
	ID             string          `json:"id"`
	IncidentID     string          `json:"incident_id"`
	RepositoryID   string          `json:"repository_id"`
	Status         string          `json:"status"`
	Control        string          `json:"control"`
	RecoveryPoint  RecoveryPoint   `json:"recovery_point"`
	CurrentVersion int             `json:"current_version"`
	Revisions      []Revision      `json:"revisions"`
	Approvals      []Approval      `json:"approvals"`
	Communications []Communication `json:"communications"`
	Events         []Event         `json:"events"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
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

func (s *Store) Create(incidentID, repositoryID, actor string, point RecoveryPoint, revision Revision) (Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	revision.Version = 1
	revision.CreatedBy = actor
	revision.CreatedAt = now
	if !validPoint(point) || !validRevision(revision) || blank(incidentID, repositoryID, actor) {
		return Operation{}, ErrInvalid
	}
	v := Operation{ID: id(), IncidentID: incidentID, RepositoryID: repositoryID, Status: "awaiting_approval", Control: "recovery_control_active", RecoveryPoint: point, CurrentVersion: 1, Revisions: []Revision{revision}, Approvals: []Approval{}, Communications: []Communication{}, Events: []Event{{ID: id(), Kind: "recovery_activated", ActorID: actor, Message: "Recovery control activated at a verified recovery point.", CreatedAt: now}}, CreatedAt: now, UpdatedAt: now}
	return v, s.write(v)
}
func (s *Store) Get(idv string) (Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read(idv)
}
func (s *Store) ListIncident(incidentID string) ([]Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	files, e := filepath.Glob(filepath.Join(s.root, "*.json"))
	if e != nil {
		return nil, e
	}
	out := []Operation{}
	for _, f := range files {
		var v Operation
		b, e := os.ReadFile(f)
		if e != nil || json.Unmarshal(b, &v) != nil {
			return nil, ErrInvalid
		}
		if v.IncidentID == incidentID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}
func (s *Store) Approve(idv, actor, decision, message string, expected int) (Operation, error) {
	return s.change(idv, expected, func(v *Operation, now time.Time) error {
		r := current(v)
		if v.Status != "awaiting_approval" || !contains(r.ApproverIDs, actor) || (decision != "approve" && decision != "reject") {
			return ErrConflict
		}
		for _, a := range v.Approvals {
			if a.ActorID == actor {
				return ErrConflict
			}
		}
		v.Approvals = append(v.Approvals, Approval{actor, decision, strings.TrimSpace(message), now})
		if decision == "reject" {
			v.Status = "paused"
			v.Control = "approval_rejected"
		} else {
			n := 0
			for _, a := range v.Approvals {
				if a.Decision == "approve" {
					n++
				}
			}
			if n >= r.RequiredApprovals {
				v.Status = "ready"
				v.Control = "approved_recovery_control"
			}
		}
		v.Events = append(v.Events, Event{ID: id(), Kind: "recovery_" + decision, ActorID: actor, Message: strings.TrimSpace(message), CreatedAt: now})
		return nil
	})
}
func (s *Store) UpdateStep(idv, stepID, actor, status, message string, results []ValidationResult, expected int) (Operation, error) {
	return s.change(idv, expected, func(v *Operation, now time.Time) error {
		if v.Status != "ready" && v.Status != "restoring" {
			return ErrConflict
		}
		r := current(v)
		var step *Step
		for i := range r.Steps {
			if r.Steps[i].ID == stepID {
				step = &r.Steps[i]
			}
		}
		if step == nil {
			return ErrNotFound
		}
		if step.AssigneeID != actor {
			return ErrConflict
		}
		if step.AssigneeType == "agent" && strings.TrimSpace(step.Delegation) == "" {
			return ErrConflict
		}
		if status == "running" {
			for _, dep := range step.DependsOn {
				d := findStep(r.Steps, dep)
				if d == nil || d.Status != "validated" {
					return ErrConflict
				}
			}
			if step.Status != "pending" && step.Status != "paused" {
				return ErrConflict
			}
			v.Status = "restoring"
		} else if status == "validated" {
			if step.Status != "running" || !validResults(step.ValidationCriteria, results) {
				return ErrConflict
			}
			step.ValidationResults = append([]ValidationResult(nil), results...)
		} else if status == "failed" || status == "blocked" {
			if step.Status != "running" {
				return ErrConflict
			}
			v.Status = "paused"
			v.Control = map[string]string{"failed": "validation_failed", "blocked": "restoration_blocked"}[status]
		} else {
			return ErrInvalid
		}
		step.Status = status
		step.Message = strings.TrimSpace(message)
		step.UpdatedBy = actor
		step.UpdatedAt = &now
		v.Revisions[len(v.Revisions)-1] = r
		kind := "recovery_step_" + status
		v.Events = append(v.Events, Event{ID: id(), Kind: kind, ActorID: actor, StepID: stepID, Message: step.Message, CreatedAt: now})
		if allValidated(r.Steps) {
			v.Status = "validated"
			v.Control = "return_to_service_ready"
		}
		return nil
	})
}
func (s *Store) Communicate(idv, actor, audience, message string, expected int) (Operation, error) {
	return s.change(idv, expected, func(v *Operation, now time.Time) error {
		message = strings.TrimSpace(message)
		if message == "" || len(message) > 10000 || (audience != "responders" && audience != "participants" && audience != "public") {
			return ErrInvalid
		}
		v.Communications = append(v.Communications, Communication{ID: id(), ActorID: actor, Audience: audience, Message: message, CreatedAt: now})
		v.Events = append(v.Events, Event{ID: id(), Kind: "recovery_communication", ActorID: actor, Message: message, CreatedAt: now})
		return nil
	})
}
func (s *Store) Control(idv, actor, action, message string, expected int) (Operation, error) {
	return s.change(idv, expected, func(v *Operation, now time.Time) error {
		switch action {
		case "pause":
			if v.Status == "completed" || v.Status == "rolled_back" {
				return ErrConflict
			}
			v.Status = "paused"
			v.Control = "manually_paused"
		case "resume":
			if v.Status != "paused" || v.Control == "approval_rejected" || !approvalThresholdSatisfied(v) {
				return ErrConflict
			}
			v.Status = "ready"
			v.Control = "approved_recovery_control"
		case "rollback":
			if v.Status != "restoring" && v.Status != "paused" && v.Status != "validated" {
				return ErrConflict
			}
			v.Status = "rolled_back"
			v.Control = "rollback_in_progress"
		case "complete":
			if v.Status != "validated" {
				return ErrConflict
			}
			v.Status = "completed"
			v.Control = "service_return_authorized"
		default:
			return ErrInvalid
		}
		v.Events = append(v.Events, Event{ID: id(), Kind: "recovery_" + action, ActorID: actor, Message: strings.TrimSpace(message), CreatedAt: now})
		return nil
	})
}
func (s *Store) change(idv string, expected int, fn func(*Operation, time.Time) error) (Operation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, e := s.read(idv)
	if e != nil {
		return v, e
	}
	if v.CurrentVersion != expected {
		return v, ErrConflict
	}
	now := s.now()
	if e = fn(&v, now); e != nil {
		return v, e
	}
	v.CurrentVersion++
	v.UpdatedAt = now
	return v, s.write(v)
}
func current(v *Operation) Revision { return v.Revisions[len(v.Revisions)-1] }
func findStep(v []Step, id string) *Step {
	for i := range v {
		if v[i].ID == id {
			return &v[i]
		}
	}
	return nil
}
func allValidated(v []Step) bool {
	for _, x := range v {
		if x.Status != "validated" {
			return false
		}
	}
	return true
}
func approvalThresholdSatisfied(v *Operation) bool {
	revision := current(v)
	approved := 0
	for _, approval := range v.Approvals {
		if approval.Decision == "reject" {
			return false
		}
		if approval.Decision == "approve" {
			approved++
		}
	}
	return approved >= revision.RequiredApprovals
}
func validResults(criteria []string, results []ValidationResult) bool {
	if len(criteria) != len(results) {
		return false
	}
	required := map[string]bool{}
	for _, criterion := range criteria {
		required[criterion] = true
	}
	seen := map[string]bool{}
	for _, result := range results {
		if !required[result.Criterion] || seen[result.Criterion] || result.Status != "passed" || strings.TrimSpace(result.Evidence) == "" || len(result.Evidence) > 10000 {
			return false
		}
		seen[result.Criterion] = true
	}
	return len(seen) == len(required)
}
func validPoint(v RecoveryPoint) bool {
	return !blank(v.PlanID, v.CaptureID, v.SourceRevision, v.ManifestSHA256) && v.PlanVersion > 0 && v.EstimatedLossMinutes >= 0 && !v.CapturedAt.IsZero()
}
func validRevision(v Revision) bool {
	if strings.TrimSpace(v.Objective) == "" || v.RequiredApprovals < 1 || v.RequiredApprovals > len(v.ApproverIDs) || len(v.Steps) == 0 || strings.TrimSpace(v.RollbackOption) == "" {
		return false
	}
	seen := map[string]bool{}
	for _, a := range v.ApproverIDs {
		if strings.TrimSpace(a) == "" || seen[a] {
			return false
		}
		seen[a] = true
	}
	steps := map[string]bool{}
	for _, x := range v.Steps {
		if blank(x.ID, x.Name, x.Kind, x.ResourceID, x.AssigneeType, x.AssigneeID) || len(x.ValidationCriteria) == 0 || (x.AssigneeType != "human" && x.AssigneeType != "agent") || x.Status != "pending" {
			return false
		}
		criteria := map[string]bool{}
		for _, criterion := range x.ValidationCriteria {
			if strings.TrimSpace(criterion) == "" || criteria[criterion] {
				return false
			}
			criteria[criterion] = true
		}
		steps[x.ID] = true
		if x.AssigneeType == "agent" && strings.TrimSpace(x.Delegation) == "" {
			return false
		}
	}
	for _, x := range v.Steps {
		for _, d := range x.DependsOn {
			if !steps[d] || d == x.ID {
				return false
			}
		}
	}
	visiting, visited := map[string]bool{}, map[string]bool{}
	var cycle func(string) bool
	cycle = func(stepID string) bool {
		if visiting[stepID] {
			return true
		}
		if visited[stepID] {
			return false
		}
		visiting[stepID] = true
		step := findStep(v.Steps, stepID)
		for _, dependency := range step.DependsOn {
			if cycle(dependency) {
				return true
			}
		}
		delete(visiting, stepID)
		visited[stepID] = true
		return false
	}
	for stepID := range steps {
		if cycle(stepID) {
			return false
		}
	}
	return true
}
func blank(v ...string) bool {
	for _, x := range v {
		if strings.TrimSpace(x) == "" {
			return true
		}
	}
	return false
}
func contains(v []string, x string) bool {
	for _, y := range v {
		if y == x {
			return true
		}
	}
	return false
}
func id() string { b := make([]byte, 12); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func (s *Store) read(idv string) (Operation, error) {
	var v Operation
	b, e := os.ReadFile(filepath.Join(s.root, idv+".json"))
	if errors.Is(e, os.ErrNotExist) {
		return v, ErrNotFound
	}
	if e != nil {
		return v, e
	}
	if json.Unmarshal(b, &v) != nil {
		return v, ErrInvalid
	}
	return v, nil
}
func (s *Store) write(v Operation) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	tmp := filepath.Join(s.root, "."+v.ID+".tmp")
	if e = os.WriteFile(tmp, b, 0600); e != nil {
		return e
	}
	return os.Rename(tmp, filepath.Join(s.root, v.ID+".json"))
}
