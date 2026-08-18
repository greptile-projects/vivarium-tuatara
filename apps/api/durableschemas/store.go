// Package durableschemas persists reviewed persistent-state contracts and migration plans.
package durableschemas

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

var ErrNotFound = errors.New("durable schema not found")
var ErrInvalid = errors.New("invalid durable schema")
var ErrConflict = errors.New("durable schema version conflict")

type Link struct {
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	Label string `json:"label"`
}
type Revision struct {
	Version        int       `json:"version"`
	Name           string    `json:"name"`
	StoreKind      string    `json:"store_kind"`
	Description    string    `json:"description"`
	Definition     string    `json:"definition"`
	DefinitionPath string    `json:"definition_path"`
	OwnerIDs       []string  `json:"owner_ids"`
	Compatibility  []string  `json:"compatibility"`
	Retention      string    `json:"retention"`
	Privacy        []string  `json:"privacy"`
	Links          []Link    `json:"links"`
	PullRequestID  string    `json:"pull_request_id"`
	ReviewedCommit string    `json:"reviewed_commit"`
	Rationale      string    `json:"rationale"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
}
type Operation struct {
	ID            string   `json:"id"`
	Kind          string   `json:"kind"`
	Description   string   `json:"description"`
	OwnerIDs      []string `json:"owner_ids"`
	ConsumerIDs   []string `json:"consumer_ids"`
	Destructive   bool     `json:"destructive"`
	RollbackLimit string   `json:"rollback_limit"`
}
type Step struct {
	ID                  string   `json:"id"`
	OperationIDs        []string `json:"operation_ids"`
	Description         string   `json:"description"`
	SuccessMeasures     []string `json:"success_measures"`
	RequiredApproverIDs []string `json:"required_approver_ids"`
}

// WorkContract is the exact, deliberately non-sensitive coexistence agreement
// carried into repository work and review. Schema definitions, privacy terms,
// and data samples remain at the schema's own visibility boundary.
type WorkContract struct {
	OldReaders          []string `json:"old_readers"`
	NewReaders          []string `json:"new_readers"`
	OldWriters          []string `json:"old_writers"`
	NewWriters          []string `json:"new_writers"`
	RolloutFlags        []string `json:"rollout_flags"`
	Idempotency         string   `json:"idempotency"`
	Transformations     []string `json:"transformations"`
	Ownership           []string `json:"ownership"`
	RollbackAssumptions []string `json:"rollback_assumptions"`
}

// MigrationWork links coordination to ordinary repository-owned proposal
// work. Those repositories remain authoritative for assignment, sessions,
// workspaces, review, checks, and merge.
type MigrationWork struct {
	ID                 string       `json:"id"`
	Kind               string       `json:"kind"`
	StepID             string       `json:"step_id"`
	RepositoryID       string       `json:"repository_id"`
	ProposalID         string       `json:"proposal_id"`
	TaskID             string       `json:"task_id"`
	DependencyIDs      []string     `json:"dependency_ids"`
	Contract           WorkContract `json:"contract"`
	CreatedBy          string       `json:"created_by"`
	CreatedAt          time.Time    `json:"created_at"`
	Status             string       `json:"status,omitempty"`
	Ready              bool         `json:"ready"`
	AssignmentID       string       `json:"assignment_id,omitempty"`
	AssigneeType       string       `json:"assignee_type,omitempty"`
	AssigneeID         string       `json:"assignee_id,omitempty"`
	BaseRevision       string       `json:"base_revision,omitempty"`
	SessionID          string       `json:"session_id,omitempty"`
	WorkspaceID        string       `json:"workspace_id,omitempty"`
	PullRequestID      string       `json:"pull_request_id,omitempty"`
	ContributionStatus string       `json:"contribution_status,omitempty"`
}

// Rehearsal is a bounded, non-authoritative proof plan. It names only
// synthetic or explicitly privacy-preserving inputs and exact revisions; the
// retained runs are evidence for review and never confer data-store access.
type Rehearsal struct {
	ID                  string                `json:"id"`
	Name                string                `json:"name"`
	ApplicationRevision string                `json:"application_revision"`
	MigrationVersion    int                   `json:"migration_version"`
	Dataset             RehearsalDataset      `json:"dataset"`
	Dependencies        []RehearsalDependency `json:"dependencies"`
	Checks              []RehearsalCheck      `json:"checks"`
	Runs                []RehearsalRun        `json:"runs"`
	Notes               []RehearsalNote       `json:"notes"`
	CreatedBy           string                `json:"created_by"`
	CreatedAt           time.Time             `json:"created_at"`
}
type RehearsalDataset struct {
	Kind          string `json:"kind"`
	Description   string `json:"description"`
	PrivacyMethod string `json:"privacy_method"`
	Digest        string `json:"digest"`
	MaxBytes      int64  `json:"max_bytes"`
	RowCount      int64  `json:"row_count"`
	ObjectCount   int64  `json:"object_count"`
}
type RehearsalDependency struct {
	Name     string `json:"name"`
	Revision string `json:"revision"`
	Digest   string `json:"digest"`
}
type RehearsalCheck struct {
	ID               string   `json:"id"`
	Kind             string   `json:"kind"`
	Command          string   `json:"command"`
	Invariant        string   `json:"invariant"`
	InvariantCommand string   `json:"invariant_command"`
	RevisionInputs   []string `json:"revision_inputs"`
}
type RehearsalOutcome struct {
	CheckID         string   `json:"check_id"`
	Status          string   `json:"status"`
	ExitCode        int      `json:"exit_code"`
	DurationMS      int64    `json:"duration_ms"`
	SanitizedLog    string   `json:"sanitized_log"`
	RowsBefore      int64    `json:"rows_before"`
	RowsAfter       int64    `json:"rows_after"`
	ObjectsBefore   int64    `json:"objects_before"`
	ObjectsAfter    int64    `json:"objects_after"`
	InvariantPassed bool     `json:"invariant_passed"`
	ArtifactDigests []string `json:"artifact_digests"`
	CostUnits       int64    `json:"cost_units"`
}
type RehearsalRun struct {
	ID           string             `json:"id"`
	WorkspaceID  string             `json:"workspace_id"`
	Result       string             `json:"result"`
	Outcomes     []RehearsalOutcome `json:"outcomes"`
	Attestations []string           `json:"attestations"`
	CreatedBy    string             `json:"created_by"`
	CreatedAt    time.Time          `json:"created_at"`
}
type RehearsalNote struct {
	ID        string    `json:"id"`
	RunID     string    `json:"run_id"`
	Body      string    `json:"body"`
	ActorID   string    `json:"actor_id"`
	CreatedAt time.Time `json:"created_at"`
}
type Event struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	StepID    string    `json:"step_id,omitempty"`
	Summary   string    `json:"summary"`
	ActorID   string    `json:"actor_id"`
	CreatedAt time.Time `json:"created_at"`
}
type ExecutionPhase struct {
	Name            string     `json:"name"`
	State           string     `json:"state"`
	ProgressPercent int        `json:"progress_percent"`
	LagSeconds      int64      `json:"lag_seconds"`
	Invariants      []string   `json:"invariants"`
	ServiceHealth   string     `json:"service_health"`
	Blockers        []string   `json:"blockers"`
	NextActions     []string   `json:"next_actions"`
	CostUnits       int64      `json:"cost_units"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
}
type ExecutionDelegation struct {
	Phase   string `json:"phase"`
	AgentID string `json:"agent_id"`
	StepID  string `json:"step_id"`
	Mandate string `json:"mandate"`
}
type ExecutionEvent struct {
	Kind      string    `json:"kind"`
	Phase     string    `json:"phase,omitempty"`
	StepID    string    `json:"step_id,omitempty"`
	Summary   string    `json:"summary"`
	ActorID   string    `json:"actor_id"`
	AgentID   string    `json:"agent_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
type ExecutionStepReport struct {
	Phase           string    `json:"phase"`
	StepID          string    `json:"step_id"`
	AgentID         string    `json:"agent_id"`
	ProgressPercent int       `json:"progress_percent"`
	LagSeconds      int64     `json:"lag_seconds"`
	Invariants      []string  `json:"invariants"`
	ServiceHealth   string    `json:"service_health"`
	Blockers        []string  `json:"blockers"`
	NextActions     []string  `json:"next_actions"`
	CostUnits       int64     `json:"cost_units"`
	Summary         string    `json:"summary"`
	CreatedAt       time.Time `json:"created_at"`
}
type FailureEvidence struct {
	ID              string    `json:"id"`
	Kind            string    `json:"kind"`
	Phase           string    `json:"phase"`
	SafetyPoint     string    `json:"safety_point"`
	Summary         string    `json:"summary"`
	Evidence        []string  `json:"evidence"`
	RecoveryActions []string  `json:"recovery_actions"`
	ActorID         string    `json:"actor_id"`
	CreatedAt       time.Time `json:"created_at"`
}
type RecoveryAction struct {
	ID                  string    `json:"id"`
	IdempotencyKey      string    `json:"idempotency_key"`
	Kind                string    `json:"kind"`
	FailureID           string    `json:"failure_id"`
	Summary             string    `json:"summary"`
	Evidence            []string  `json:"evidence"`
	RecoveryPoint       string    `json:"recovery_point,omitempty"`
	RecoveryAttestation string    `json:"recovery_attestation,omitempty"`
	RollbackReleaseID   string    `json:"rollback_release_id,omitempty"`
	RepairWorkID        string    `json:"repair_work_id,omitempty"`
	ActorID             string    `json:"actor_id"`
	CreatedAt           time.Time `json:"created_at"`
}
type RetirementApproval struct {
	OwnerID   string    `json:"owner_id"`
	Summary   string    `json:"summary"`
	CreatedAt time.Time `json:"created_at"`
}
type EnvironmentCompletion struct {
	EnvironmentID    string   `json:"environment_id"`
	CurrentVersion   int      `json:"current_version"`
	RetainedData     []string `json:"retained_data"`
	ChangedData      []string `json:"changed_data"`
	VerifiedDeletion []string `json:"verified_deletion"`
	Exceptions       []string `json:"exceptions"`
	CostUnits        int64    `json:"cost_units"`
}
type RetirementCompletion struct {
	ObservationStartedAt  time.Time               `json:"observation_started_at"`
	ObservationEndedAt    time.Time               `json:"observation_ended_at"`
	CompatibilityRemoved  []string                `json:"compatibility_removed"`
	ObsoleteFields        []string                `json:"obsolete_fields"`
	IrreversibleDecisions []string                `json:"irreversible_decisions"`
	Environments          []EnvironmentCompletion `json:"environments"`
	ApprovedBy            []string                `json:"approved_by"`
	CompletedBy           string                  `json:"completed_by"`
	CompletedAt           time.Time               `json:"completed_at"`
}

// Execution is a collaboration record over authoritative work. It references
// established release resources but deliberately carries no credentials or
// executable commands.
type Execution struct {
	ID                       string                `json:"id"`
	Version                  int                   `json:"version"`
	MigrationVersion         int                   `json:"migration_version"`
	ActiveRevision           int                   `json:"active_revision"`
	EnvironmentID            string                `json:"environment_id"`
	ReleaseID                string                `json:"release_id"`
	DeploymentID             string                `json:"deployment_id,omitempty"`
	RehearsalID              string                `json:"rehearsal_id"`
	ControllerID             string                `json:"controller_id"`
	Status                   string                `json:"status"`
	CurrentPhase             int                   `json:"current_phase"`
	CompatibilityWindow      string                `json:"compatibility_window"`
	ObservationPeriodSeconds int64                 `json:"observation_period_seconds"`
	PrivacyConstraints       []string              `json:"privacy_constraints"`
	CostBudgetUnits          int64                 `json:"cost_budget_units"`
	ThrottlePercent          int                   `json:"throttle_percent"`
	AbortReversibleUntil     string                `json:"abort_reversible_until"`
	Phases                   []ExecutionPhase      `json:"phases"`
	Delegations              []ExecutionDelegation `json:"delegations"`
	StepReports              []ExecutionStepReport `json:"step_reports"`
	Failures                 []FailureEvidence     `json:"failures"`
	Recoveries               []RecoveryAction      `json:"recoveries"`
	Events                   []ExecutionEvent      `json:"events"`
	CreatedAt                time.Time             `json:"created_at"`
	UpdatedAt                time.Time             `json:"updated_at"`
}
type Migration struct {
	ID                  string                `json:"id"`
	FromVersion         int                   `json:"from_version"`
	ToVersion           int                   `json:"to_version"`
	SourceKind          string                `json:"source_kind"`
	SourceID            string                `json:"source_id"`
	Summary             string                `json:"summary"`
	Operations          []Operation           `json:"operations"`
	Steps               []Step                `json:"steps"`
	RollbackLimits      []string              `json:"rollback_limits"`
	Version             int                   `json:"version"`
	Events              []Event               `json:"events"`
	Work                []MigrationWork       `json:"work"`
	Rehearsals          []Rehearsal           `json:"rehearsals"`
	Executions          []Execution           `json:"executions"`
	RetirementApprovals []RetirementApproval  `json:"retirement_approvals"`
	Completion          *RetirementCompletion `json:"completion,omitempty"`
	CreatedBy           string                `json:"created_by"`
	CreatedAt           time.Time             `json:"created_at"`
	UpdatedAt           time.Time             `json:"updated_at"`
}

func (s *Store) CreateExecution(repo, schema, migration, actor string, expected int, in Execution) (Schema, Execution, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.read(repo, schema)
	if err != nil {
		return Schema{}, Execution{}, err
	}
	for mi := range v.Migrations {
		m := &v.Migrations[mi]
		if m.ID != migration {
			continue
		}
		if m.Version != expected {
			return v, Execution{}, ErrConflict
		}
		if len(m.Executions) > 0 && m.Executions[len(m.Executions)-1].Status != "completed" && m.Executions[len(m.Executions)-1].Status != "aborted" {
			return v, Execution{}, ErrConflict
		}
		passed := false
		var passedAt time.Time
		for _, rehearsal := range m.Rehearsals {
			if rehearsal.ID == in.RehearsalID {
				for _, run := range rehearsal.Runs {
					if run.Result == "passed" && run.CreatedAt.After(passedAt) {
						passed, passedAt = true, run.CreatedAt
					}
				}
			}
		}
		approved := map[string]map[string]bool{}
		for _, e := range m.Events {
			if e.Kind == "approval_revoked" {
				if approved[e.StepID] != nil {
					delete(approved[e.StepID], e.ActorID)
				}
			} else if e.Kind == "approved" && !e.CreatedAt.Before(passedAt) {
				if approved[e.StepID] == nil {
					approved[e.StepID] = map[string]bool{}
				}
				approved[e.StepID][e.ActorID] = true
			}
		}
		for _, step := range m.Steps {
			for _, owner := range step.RequiredApproverIDs {
				if !approved[step.ID][owner] {
					return v, Execution{}, ErrInvalid
				}
			}
		}
		if !passed || in.EnvironmentID == "" || in.ReleaseID == "" || strings.TrimSpace(in.CompatibilityWindow) == "" || in.ObservationPeriodSeconds < 1 || in.ObservationPeriodSeconds > int64((365*24*time.Hour)/time.Second) || len(in.PrivacyConstraints) == 0 || in.CostBudgetUnits < 0 || in.CostBudgetUnits > 1_000_000_000 || strings.TrimSpace(in.AbortReversibleUntil) == "" {
			return v, Execution{}, ErrInvalid
		}
		steps := map[string]bool{}
		for _, step := range m.Steps {
			steps[step.ID] = true
		}
		delegations := map[string]bool{}
		for _, d := range in.Delegations {
			key := d.AgentID + ":" + d.Phase + ":" + d.StepID
			if d.AgentID == "" || !steps[d.StepID] || strings.TrimSpace(d.Mandate) == "" || !executionPhase(d.Phase) || delegations[key] {
				return v, Execution{}, ErrInvalid
			}
			delegations[key] = true
		}
		now := s.now()
		phases := make([]ExecutionPhase, 5)
		for i, name := range []string{"expand", "deploy", "backfill", "cutover", "contract"} {
			phases[i] = ExecutionPhase{Name: name, State: "pending", Invariants: []string{}, Blockers: []string{}, NextActions: []string{"begin " + name}}
		}
		phases[0].State = "ready"
		in.ID = id()
		in.Version = 1
		in.MigrationVersion = m.Version
		in.ActiveRevision = m.ToVersion
		in.ControllerID = actor
		in.Status = "ready"
		in.CurrentPhase = 0
		in.ThrottlePercent = 100
		// Deployment evidence is attached only through the separately verified
		// deploy-phase report boundary; execution creation cannot self-attest it.
		in.DeploymentID = ""
		in.Phases = phases
		in.StepReports = []ExecutionStepReport{}
		in.Failures = []FailureEvidence{}
		in.Recoveries = []RecoveryAction{}
		in.Events = []ExecutionEvent{{Kind: "created", Phase: "expand", Summary: "production migration execution opened after approvals and rehearsal evidence", ActorID: actor, CreatedAt: now}}
		in.CreatedAt = now
		in.UpdatedAt = now
		m.Executions = append(m.Executions, in)
		m.Version++
		m.UpdatedAt = now
		v.UpdatedAt = now
		return v, in, s.write(v)
	}
	return Schema{}, Execution{}, ErrNotFound
}

type ExecutionUpdate struct {
	Action          string
	ExpectedVersion int
	Phase           string
	ProgressPercent int
	LagSeconds      int64
	Invariants      []string
	ServiceHealth   string
	Blockers        []string
	NextActions     []string
	CostUnits       int64
	ThrottlePercent int
	Summary         string
	AgentID         string
	StepID          string
	DeploymentID    string
	FailureKind     string
	SafetyPoint     string
	FailureEvidence []string
}

func (s *Store) UpdateExecution(repo, schema, migration, execution, actor string, in ExecutionUpdate) (Schema, Execution, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.read(repo, schema)
	if err != nil {
		return Schema{}, Execution{}, err
	}
	for mi := range v.Migrations {
		m := &v.Migrations[mi]
		if m.ID != migration {
			continue
		}
		for ei := range m.Executions {
			x := &m.Executions[ei]
			if x.ID != execution {
				continue
			}
			if x.Version != in.ExpectedVersion {
				return v, *x, ErrConflict
			}
			now := s.now()
			phase := &x.Phases[x.CurrentPhase]
			if in.AgentID != "" {
				delegated := false
				for _, d := range x.Delegations {
					delegated = delegated || d.AgentID == in.AgentID && d.Phase == phase.Name && d.StepID == in.StepID
				}
				if !delegated || in.Action != "report" || in.Phase != phase.Name || in.ProgressPercent < 0 || in.ProgressPercent > 100 || in.LagSeconds < 0 || in.CostUnits < 0 || in.DeploymentID != "" {
					return v, *x, ErrInvalid
				}
				summary := strings.TrimSpace(in.Summary)
				if summary == "" {
					return v, *x, ErrInvalid
				}
				x.StepReports = append(x.StepReports, ExecutionStepReport{Phase: phase.Name, StepID: in.StepID, AgentID: in.AgentID, ProgressPercent: in.ProgressPercent, LagSeconds: in.LagSeconds, Invariants: append([]string{}, in.Invariants...), ServiceHealth: strings.TrimSpace(in.ServiceHealth), Blockers: append([]string{}, in.Blockers...), NextActions: append([]string{}, in.NextActions...), CostUnits: in.CostUnits, Summary: summary, CreatedAt: now})
				x.Version++
				x.UpdatedAt = now
				x.Events = append(x.Events, ExecutionEvent{Kind: "step_report", Phase: phase.Name, StepID: in.StepID, Summary: summary, ActorID: actor, AgentID: in.AgentID, CreatedAt: now})
				m.UpdatedAt, v.UpdatedAt = now, now
				return v, *x, s.write(v)
			}
			switch in.Action {
			case "start":
				if x.Status != "ready" {
					return v, *x, ErrInvalid
				}
				x.Status = "running"
				phase.State = "running"
				if phase.StartedAt == nil {
					phase.StartedAt = &now
				}
			case "pause":
				if x.Status != "running" {
					return v, *x, ErrInvalid
				}
				x.Status = "paused"
				phase.State = "paused"
			case "resume":
				if x.Status != "paused" || !migrationApprovalsCurrent(*m) || !latestFailureRecovered(*x) {
					return v, *x, ErrInvalid
				}
				x.Status = "running"
				phase.State = "running"
			case "throttle":
				if x.Status != "running" || in.ThrottlePercent < 1 || in.ThrottlePercent > 100 {
					return v, *x, ErrInvalid
				}
				x.ThrottlePercent = in.ThrottlePercent
			case "abort":
				if x.Status == "completed" || phase.Name == "contract" || in.Summary == "" {
					return v, *x, ErrInvalid
				}
				x.Status = "aborted"
				phase.State = "aborted"
			case "report":
				if x.Status != "running" || in.Phase != phase.Name || in.ProgressPercent < 0 || in.ProgressPercent > 100 || in.LagSeconds < 0 || in.CostUnits < 0 {
					return v, *x, ErrInvalid
				}
				if in.DeploymentID != "" && phase.Name != "deploy" {
					return v, *x, ErrInvalid
				}
				total := int64(0)
				for _, p := range x.Phases {
					total += p.CostUnits
				}
				total += in.CostUnits - phase.CostUnits
				overCapacity := x.CostBudgetUnits > 0 && total > x.CostBudgetUnits
				phase.ProgressPercent = in.ProgressPercent
				phase.LagSeconds = in.LagSeconds
				phase.Invariants = append([]string{}, in.Invariants...)
				phase.ServiceHealth = strings.TrimSpace(in.ServiceHealth)
				phase.Blockers = append([]string{}, in.Blockers...)
				phase.NextActions = append([]string{}, in.NextActions...)
				phase.CostUnits = in.CostUnits
				if in.DeploymentID != "" {
					x.DeploymentID = in.DeploymentID
				}
				failureKind := in.FailureKind
				failureSummary := strings.TrimSpace(in.Summary)
				safetyPoint := strings.TrimSpace(in.SafetyPoint)
				failureEvidence := append([]string{}, in.FailureEvidence...)
				if overCapacity {
					failureKind = "capacity_exhaustion"
					if failureSummary == "" {
						failureSummary = "declared migration cost capacity exhausted"
					}
					if safetyPoint == "" {
						safetyPoint = phase.Name + " cost-budget boundary"
					}
					if len(failureEvidence) == 0 {
						failureEvidence = []string{"cost budget units exceeded by retained phase report"}
					}
				}
				if failureKind != "" {
					if !executionFailureKind(failureKind) || safetyPoint == "" || len(safetyPoint) > 500 || failureSummary == "" || len(failureSummary) > 2000 || !boundedEvidence(failureEvidence) {
						return v, *x, ErrInvalid
					}
					x.Status, phase.State = "paused", "paused"
					x.Failures = append(x.Failures, FailureEvidence{ID: id(), Kind: failureKind, Phase: phase.Name, SafetyPoint: safetyPoint, Summary: failureSummary, Evidence: failureEvidence, RecoveryActions: []string{"retry", "restore", "traffic_rollback", "repair"}, ActorID: actor, CreatedAt: now})
				}
			case "advance":
				if x.Status != "running" || phase.ProgressPercent != 100 || len(phase.Blockers) > 0 || phase.ServiceHealth != "healthy" || len(phase.Invariants) == 0 || !delegatedPhaseReady(*x, phase.Name) || (phase.Name == "deploy" && x.DeploymentID == "") {
					return v, *x, ErrInvalid
				}
				phase.State = "completed"
				phase.CompletedAt = &now
				if x.CurrentPhase == len(x.Phases)-1 {
					x.Status = "completed"
				} else {
					x.CurrentPhase++
					x.Status = "ready"
					x.Phases[x.CurrentPhase].State = "ready"
				}
			default:
				return v, *x, ErrInvalid
			}
			x.Version++
			x.UpdatedAt = now
			summary := strings.TrimSpace(in.Summary)
			if summary == "" {
				summary = in.Action
			}
			x.Events = append(x.Events, ExecutionEvent{Kind: in.Action, Phase: phase.Name, Summary: summary, ActorID: actor, AgentID: in.AgentID, CreatedAt: now})
			m.UpdatedAt = now
			v.UpdatedAt = now
			return v, *x, s.write(v)
		}
	}
	return Schema{}, Execution{}, ErrNotFound
}
func migrationApprovalsCurrent(m Migration) bool {
	current := map[string]map[string]bool{}
	for _, event := range m.Events {
		if event.Kind != "approved" && event.Kind != "approval_revoked" {
			continue
		}
		if current[event.StepID] == nil {
			current[event.StepID] = map[string]bool{}
		}
		current[event.StepID][event.ActorID] = event.Kind == "approved"
	}
	for _, step := range m.Steps {
		for _, owner := range step.RequiredApproverIDs {
			if !current[step.ID][owner] {
				return false
			}
		}
	}
	return true
}
func latestFailureRecovered(x Execution) bool {
	if len(x.Failures) == 0 {
		return true
	}
	latest := x.Failures[len(x.Failures)-1]
	for _, recovery := range x.Recoveries {
		if recovery.FailureID == latest.ID && recovery.Kind != "repair" && !recovery.CreatedAt.Before(latest.CreatedAt) {
			return true
		}
	}
	return false
}
func executionFailureKind(v string) bool {
	return v == "failed_invariant" || v == "service_regression" || v == "conflicting_writes" || v == "capacity_exhaustion" || v == "interrupted_backfill"
}

func boundedEvidence(values []string) bool {
	if len(values) == 0 || len(values) > 20 {
		return false
	}
	for _, value := range values {
		if strings.TrimSpace(value) == "" || len(value) > 2000 {
			return false
		}
	}
	return true
}

type RecoveryRequest struct {
	ExpectedVersion     int
	IdempotencyKey      string
	Kind                string
	FailureID           string
	Summary             string
	Evidence            []string
	RecoveryPoint       string
	RecoveryAttestation string
	RollbackReleaseID   string
	RepairWorkID        string
}

func (s *Store) RecoverExecution(repo, schema, migration, execution, actor string, in RecoveryRequest) (Schema, Execution, RecoveryAction, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.read(repo, schema)
	if err != nil {
		return Schema{}, Execution{}, RecoveryAction{}, err
	}
	for mi := range v.Migrations {
		m := &v.Migrations[mi]
		if m.ID != migration {
			continue
		}
		for ei := range m.Executions {
			x := &m.Executions[ei]
			if x.ID != execution {
				continue
			}
			for _, prior := range x.Recoveries {
				if prior.IdempotencyKey == in.IdempotencyKey {
					if prior.Kind != in.Kind || prior.FailureID != in.FailureID || prior.Summary != strings.TrimSpace(in.Summary) || !slices.Equal(prior.Evidence, in.Evidence) || prior.RecoveryPoint != in.RecoveryPoint || prior.RecoveryAttestation != in.RecoveryAttestation || prior.RollbackReleaseID != in.RollbackReleaseID || prior.RepairWorkID != in.RepairWorkID {
						return v, *x, prior, ErrConflict
					}
					return v, *x, prior, nil
				}
			}
			if x.Version != in.ExpectedVersion || x.Status != "paused" || in.IdempotencyKey == "" || len(in.IdempotencyKey) > 200 || strings.TrimSpace(in.Summary) == "" || len(in.Summary) > 2000 || !boundedEvidence(in.Evidence) || len(in.RecoveryPoint) > 1000 || len(in.RecoveryAttestation) > 2000 {
				return v, *x, RecoveryAction{}, ErrInvalid
			}
			failureFound := false
			for _, f := range x.Failures {
				failureFound = failureFound || f.ID == in.FailureID
			}
			if !failureFound {
				return v, *x, RecoveryAction{}, ErrInvalid
			}
			valid := in.Kind == "retry" || (in.Kind == "restore" && in.RecoveryPoint != "" && in.RecoveryAttestation != "") || (in.Kind == "traffic_rollback" && in.RollbackReleaseID != "" && x.CurrentPhase < 4) || (in.Kind == "repair" && in.RepairWorkID != "")
			if in.Kind == "repair" {
				found := false
				for _, work := range m.Work {
					found = found || work.ID == in.RepairWorkID
				}
				valid = valid && found
			}
			if !valid {
				return v, *x, RecoveryAction{}, ErrInvalid
			}
			now := s.now()
			a := RecoveryAction{ID: id(), IdempotencyKey: in.IdempotencyKey, Kind: in.Kind, FailureID: in.FailureID, Summary: strings.TrimSpace(in.Summary), Evidence: append([]string{}, in.Evidence...), RecoveryPoint: in.RecoveryPoint, RecoveryAttestation: in.RecoveryAttestation, RollbackReleaseID: in.RollbackReleaseID, RepairWorkID: in.RepairWorkID, ActorID: actor, CreatedAt: now}
			x.Recoveries = append(x.Recoveries, a)
			x.Version++
			x.UpdatedAt = now
			x.Events = append(x.Events, ExecutionEvent{Kind: "recovery_" + in.Kind, Phase: x.Phases[x.CurrentPhase].Name, Summary: a.Summary, ActorID: actor, CreatedAt: now})
			m.UpdatedAt, v.UpdatedAt = now, now
			return v, *x, a, s.write(v)
		}
	}
	return Schema{}, Execution{}, RecoveryAction{}, ErrNotFound
}
func executionPhase(v string) bool {
	return v == "expand" || v == "deploy" || v == "backfill" || v == "cutover" || v == "contract"
}

func delegatedPhaseReady(execution Execution, phase string) bool {
	latest := map[string]ExecutionStepReport{}
	for _, report := range execution.StepReports {
		if report.Phase == phase {
			latest[report.AgentID+":"+report.StepID] = report
		}
	}
	for _, delegation := range execution.Delegations {
		if delegation.Phase != phase {
			continue
		}
		report, ok := latest[delegation.AgentID+":"+delegation.StepID]
		if !ok || report.ProgressPercent != 100 || report.ServiceHealth != "healthy" || len(report.Invariants) == 0 || len(report.Blockers) > 0 {
			return false
		}
	}
	return true
}

type Schema struct {
	ID             string      `json:"id"`
	RepositoryID   string      `json:"repository_id"`
	CurrentVersion int         `json:"current_version"`
	Revisions      []Revision  `json:"revisions"`
	Migrations     []Migration `json:"migrations"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
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
func (s *Store) Create(repo, actor string, r Revision) (Schema, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if validateRevision(r) != nil {
		return Schema{}, ErrInvalid
	}
	now := s.now()
	r.Version = 1
	r.CreatedBy = actor
	r.CreatedAt = now
	v := Schema{ID: id(), RepositoryID: repo, CurrentVersion: 1, Revisions: []Revision{r}, Migrations: []Migration{}, CreatedAt: now, UpdatedAt: now}
	return v, s.write(v)
}
func (s *Store) Revise(repo, schema string, expected int, actor string, r Revision) (Schema, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, e := s.read(repo, schema)
	if e != nil {
		return Schema{}, e
	}
	if v.CurrentVersion != expected {
		return Schema{}, ErrConflict
	}
	if validateRevision(r) != nil {
		return Schema{}, ErrInvalid
	}
	r.Version = expected + 1
	r.CreatedBy = actor
	r.CreatedAt = s.now()
	v.CurrentVersion = r.Version
	v.Revisions = append(v.Revisions, r)
	v.UpdatedAt = r.CreatedAt
	return v, s.write(v)
}
func (s *Store) AddMigration(repo, schema, actor string, m Migration) (Schema, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, e := s.read(repo, schema)
	if e != nil {
		return Schema{}, e
	}
	if validateMigration(v, m) != nil {
		return Schema{}, ErrInvalid
	}
	now := s.now()
	m.ID = id()
	m.Version = 1
	m.CreatedBy = actor
	m.CreatedAt = now
	m.UpdatedAt = now
	m.Events = []Event{{ID: id(), Kind: "created", Summary: m.Summary, ActorID: actor, CreatedAt: now}}
	v.Migrations = append(v.Migrations, m)
	v.UpdatedAt = now
	return v, s.write(v)
}
func (s *Store) AddEvent(repo, schema, migration, actor string, expected int, e Event) (Schema, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, x := s.read(repo, schema)
	if x != nil {
		return Schema{}, x
	}
	for i := range v.Migrations {
		m := &v.Migrations[i]
		if m.ID != migration {
			continue
		}
		if m.Version != expected {
			return Schema{}, ErrConflict
		}
		if strings.TrimSpace(e.Kind) == "" || strings.TrimSpace(e.Summary) == "" {
			return Schema{}, ErrInvalid
		}
		if e.Kind == "approved" {
			authorized := false
			for _, step := range m.Steps {
				if step.ID != e.StepID {
					continue
				}
				for _, approver := range step.RequiredApproverIDs {
					if approver == actor {
						authorized = true
						break
					}
				}
			}
			if !authorized {
				return Schema{}, ErrInvalid
			}
		}
		if e.Kind == "approval_revoked" {
			authorized := false
			for _, step := range m.Steps {
				if step.ID == e.StepID {
					for _, approver := range step.RequiredApproverIDs {
						authorized = authorized || approver == actor
					}
				}
			}
			if !authorized {
				return Schema{}, ErrInvalid
			}
			for xi := range m.Executions {
				x := &m.Executions[xi]
				if x.Status != "completed" && x.Status != "aborted" {
					now := s.now()
					x.Status = "paused"
					x.Phases[x.CurrentPhase].State = "paused"
					x.Failures = append(x.Failures, FailureEvidence{ID: id(), Kind: "revoked_approval", Phase: x.Phases[x.CurrentPhase].Name, SafetyPoint: "approval revocation boundary", Summary: e.Summary, Evidence: []string{"step:" + e.StepID, "approver:" + actor}, RecoveryActions: []string{"retry", "restore", "traffic_rollback", "repair"}, ActorID: actor, CreatedAt: now})
					x.Version++
					x.UpdatedAt = now
				}
			}
		}
		e.ID = id()
		e.ActorID = actor
		e.CreatedAt = s.now()
		m.Version++
		m.Events = append(m.Events, e)
		m.UpdatedAt = e.CreatedAt
		v.UpdatedAt = e.CreatedAt
		return v, s.write(v)
	}
	return Schema{}, ErrNotFound
}

func (s *Store) ApproveRetirement(repo, schema, migration, actor string, expected int, summary string) (Schema, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.read(repo, schema)
	if err != nil {
		return Schema{}, err
	}
	for mi := range v.Migrations {
		m := &v.Migrations[mi]
		if m.ID != migration {
			continue
		}
		if m.Version != expected || m.Completion != nil || strings.TrimSpace(summary) == "" {
			return v, ErrConflict
		}
		owners := map[string]bool{}
		for _, rev := range v.Revisions {
			if rev.Version == m.ToVersion {
				for _, owner := range rev.OwnerIDs {
					owners[owner] = true
				}
			}
		}
		if !owners[actor] {
			return v, ErrInvalid
		}
		for _, approval := range m.RetirementApprovals {
			if approval.OwnerID == actor {
				return v, ErrConflict
			}
		}
		now := s.now()
		m.RetirementApprovals = append(m.RetirementApprovals, RetirementApproval{OwnerID: actor, Summary: summary, CreatedAt: now})
		m.Version++
		m.UpdatedAt, v.UpdatedAt = now, now
		return v, s.write(v)
	}
	return Schema{}, ErrNotFound
}

func (s *Store) CompleteRetirement(repo, schema, migration, actor string, expected int, c RetirementCompletion) (Schema, RetirementCompletion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.read(repo, schema)
	if err != nil {
		return Schema{}, RetirementCompletion{}, err
	}
	for mi := range v.Migrations {
		m := &v.Migrations[mi]
		if m.ID != migration {
			continue
		}
		if m.Version != expected || m.Completion != nil {
			return v, RetirementCompletion{}, ErrConflict
		}
		owners, approved := map[string]bool{}, map[string]bool{}
		for _, rev := range v.Revisions {
			if rev.Version == m.ToVersion {
				for _, owner := range rev.OwnerIDs {
					owners[owner] = true
				}
			}
		}
		for _, a := range m.RetirementApprovals {
			approved[a.OwnerID] = true
		}
		allOwners := len(owners) > 0
		for owner := range owners {
			allOwners = allOwners && approved[owner]
		}
		completedByEnvironment := map[string]Execution{}
		for _, x := range m.Executions {
			if x.Status == "completed" && x.DeploymentID != "" && x.CurrentPhase == len(x.Phases)-1 && len(x.Phases) > 0 && x.Phases[len(x.Phases)-1].CompletedAt != nil {
				prior, exists := completedByEnvironment[x.EnvironmentID]
				if !exists || x.Phases[len(x.Phases)-1].CompletedAt.After(*prior.Phases[len(prior.Phases)-1].CompletedAt) {
					completedByEnvironment[x.EnvironmentID] = x
				}
			}
		}
		validEnvironments := len(c.Environments) > 0
		observationSeconds := int64(0)
		var latestSuccess time.Time
		seen := map[string]bool{}
		for _, env := range c.Environments {
			execution, executed := completedByEnvironment[env.EnvironmentID]
			if env.EnvironmentID == "" || seen[env.EnvironmentID] || !executed || env.CurrentVersion != m.ToVersion || len(env.RetainedData) == 0 || len(env.ChangedData) == 0 || len(env.VerifiedDeletion) == 0 || env.CostUnits < 0 {
				validEnvironments = false
			} else {
				completedAt := *execution.Phases[len(execution.Phases)-1].CompletedAt
				if completedAt.After(latestSuccess) {
					latestSuccess = completedAt
				}
				if execution.ObservationPeriodSeconds > observationSeconds {
					observationSeconds = execution.ObservationPeriodSeconds
				}
			}
			seen[env.EnvironmentID] = true
		}
		if !owners[actor] || !allOwners || latestSuccess.IsZero() || c.ObservationStartedAt.Before(latestSuccess) || !c.ObservationEndedAt.After(c.ObservationStartedAt) || c.ObservationEndedAt.Sub(c.ObservationStartedAt) < time.Duration(observationSeconds)*time.Second || c.ObservationEndedAt.After(s.now()) || len(c.CompatibilityRemoved) == 0 || len(c.ObsoleteFields) == 0 || len(c.IrreversibleDecisions) == 0 || !validEnvironments {
			return v, RetirementCompletion{}, ErrInvalid
		}
		c.ApprovedBy = make([]string, 0, len(approved))
		for owner := range approved {
			c.ApprovedBy = append(c.ApprovedBy, owner)
		}
		sort.Strings(c.ApprovedBy)
		now := s.now()
		c.CompletedBy, c.CompletedAt = actor, now
		m.Completion = &c
		m.Version++
		m.UpdatedAt, v.UpdatedAt = now, now
		m.Events = append(m.Events, Event{ID: id(), Kind: "retirement_completed", Summary: "compatibility machinery retired after observation", ActorID: actor, CreatedAt: now})
		return v, c, s.write(v)
	}
	return Schema{}, RetirementCompletion{}, ErrNotFound
}

// CreateMigrationWork holds the schema-plan CAS boundary while an ordinary
// repository task is published, preventing unlinked work and stale ordering.
func (s *Store) CreateMigrationWork(repo, schema, migration, actor string, expected int, work MigrationWork, publish func() (string, string, error)) (Schema, MigrationWork, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.read(repo, schema)
	if err != nil {
		return Schema{}, MigrationWork{}, err
	}
	for i := range v.Migrations {
		m := &v.Migrations[i]
		if m.ID != migration {
			continue
		}
		if m.Version != expected {
			return v, MigrationWork{}, ErrConflict
		}
		if validateWork(*m, work) != nil || publish == nil {
			return v, MigrationWork{}, ErrInvalid
		}
		known := map[string]bool{}
		for _, existing := range m.Work {
			known[existing.ID] = true
		}
		seenDependencies := map[string]bool{}
		for _, dependency := range work.DependencyIDs {
			if !known[dependency] || seenDependencies[dependency] {
				return v, MigrationWork{}, ErrInvalid
			}
			seenDependencies[dependency] = true
		}
		proposalID, taskID, publishErr := publish()
		if publishErr != nil {
			return v, MigrationWork{}, publishErr
		}
		if proposalID == "" || taskID == "" {
			return v, MigrationWork{}, ErrInvalid
		}
		now := s.now()
		work.ID, work.ProposalID, work.TaskID = id(), proposalID, taskID
		work.CreatedBy, work.CreatedAt = actor, now
		m.Work = append(m.Work, work)
		m.Version++
		m.Events = append(m.Events, Event{ID: id(), Kind: "work_created", StepID: work.StepID, Summary: work.Kind + " work created in repository " + work.RepositoryID, ActorID: actor, CreatedAt: now})
		m.UpdatedAt, v.UpdatedAt = now, now
		return v, work, s.write(v)
	}
	return Schema{}, MigrationWork{}, ErrNotFound
}

func (s *Store) CreateRehearsal(repo, schema, migration, actor string, expected int, rehearsal Rehearsal) (Schema, Rehearsal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.read(repo, schema)
	if err != nil {
		return Schema{}, Rehearsal{}, err
	}
	for i := range v.Migrations {
		m := &v.Migrations[i]
		if m.ID != migration {
			continue
		}
		if m.Version != expected {
			return v, Rehearsal{}, ErrConflict
		}
		rehearsal.MigrationVersion = m.Version
		if validateRehearsal(rehearsal) != nil {
			return v, Rehearsal{}, ErrInvalid
		}
		now := s.now()
		rehearsal.ID, rehearsal.CreatedBy, rehearsal.CreatedAt = id(), actor, now
		rehearsal.Runs, rehearsal.Notes = []RehearsalRun{}, []RehearsalNote{}
		m.Rehearsals = append(m.Rehearsals, rehearsal)
		m.Version++
		m.UpdatedAt, v.UpdatedAt = now, now
		m.Events = append(m.Events, Event{ID: id(), Kind: "rehearsal_created", Summary: rehearsal.Name, ActorID: actor, CreatedAt: now})
		return v, rehearsal, s.write(v)
	}
	return Schema{}, Rehearsal{}, ErrNotFound
}

func (s *Store) AddRehearsalRun(repo, schema, migration, rehearsal, actor string, run RehearsalRun) (Schema, RehearsalRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.read(repo, schema)
	if err != nil {
		return Schema{}, RehearsalRun{}, err
	}
	for mi := range v.Migrations {
		m := &v.Migrations[mi]
		if m.ID != migration {
			continue
		}
		for ri := range m.Rehearsals {
			x := &m.Rehearsals[ri]
			if x.ID != rehearsal {
				continue
			}
			if validateRun(*x, run) != nil {
				return v, RehearsalRun{}, ErrInvalid
			}
			for _, prior := range x.Runs {
				if prior.WorkspaceID == run.WorkspaceID {
					return v, RehearsalRun{}, ErrConflict
				}
			}
			now := s.now()
			run.ID, run.CreatedBy, run.CreatedAt = id(), actor, now
			x.Runs = append(x.Runs, run)
			v.UpdatedAt = now
			return v, run, s.write(v)
		}
	}
	return Schema{}, RehearsalRun{}, ErrNotFound
}

func (s *Store) AddRehearsalNote(repo, schema, migration, rehearsal, actor, runID, body string) (Schema, RehearsalNote, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.read(repo, schema)
	if err != nil {
		return Schema{}, RehearsalNote{}, err
	}
	if strings.TrimSpace(body) == "" || len([]rune(body)) > 4000 {
		return v, RehearsalNote{}, ErrInvalid
	}
	for mi := range v.Migrations {
		m := &v.Migrations[mi]
		if m.ID != migration {
			continue
		}
		for ri := range m.Rehearsals {
			x := &m.Rehearsals[ri]
			if x.ID != rehearsal {
				continue
			}
			found := false
			for _, run := range x.Runs {
				if run.ID == runID {
					found = true
				}
			}
			if !found {
				return v, RehearsalNote{}, ErrInvalid
			}
			now := s.now()
			n := RehearsalNote{ID: id(), RunID: runID, Body: body, ActorID: actor, CreatedAt: now}
			x.Notes = append(x.Notes, n)
			v.UpdatedAt = now
			return v, n, s.write(v)
		}
	}
	return Schema{}, RehearsalNote{}, ErrNotFound
}

func validateRehearsal(r Rehearsal) error {
	if strings.TrimSpace(r.Name) == "" || r.ApplicationRevision == "" || r.MigrationVersion < 1 || len(r.Checks) == 0 || len(r.Checks) > 30 || len(r.Dependencies) > 30 {
		return ErrInvalid
	}
	if (r.Dataset.Kind != "synthetic" && r.Dataset.Kind != "representative") || r.Dataset.Description == "" || r.Dataset.Digest == "" || r.Dataset.MaxBytes <= 0 || r.Dataset.MaxBytes > 1<<30 || (r.Dataset.Kind == "representative" && r.Dataset.PrivacyMethod == "") || r.Dataset.RowCount < 0 || r.Dataset.ObjectCount < 0 {
		return ErrInvalid
	}
	deps := map[string]bool{"application": true, "schema_from": true, "schema_to": true, "migration": true, "data_shape": true}
	for _, d := range r.Dependencies {
		if d.Name == "" || d.Revision == "" || d.Digest == "" {
			return ErrInvalid
		}
		deps["dependency:"+d.Name] = true
	}
	ids := map[string]bool{}
	commands := map[string]bool{}
	kinds := map[string]bool{"upgrade": true, "dual_read": true, "dual_write": true, "backfill": true, "validation": true, "rollback": true, "failure_injection": true}
	for _, c := range r.Checks {
		if c.ID == "" || ids[c.ID] || commands[c.Command] || commands[c.InvariantCommand] || c.Command == c.InvariantCommand || !kinds[c.Kind] || strings.TrimSpace(c.Command) == "" || strings.TrimSpace(c.InvariantCommand) == "" || len(c.Command) > 2000 || len(c.InvariantCommand) > 2000 || c.Invariant == "" || len(c.RevisionInputs) == 0 {
			return ErrInvalid
		}
		ids[c.ID] = true
		commands[c.Command] = true
		commands[c.InvariantCommand] = true
		for _, input := range c.RevisionInputs {
			if !deps[input] {
				return ErrInvalid
			}
		}
	}
	return nil
}
func validateRun(r Rehearsal, run RehearsalRun) error {
	if run.WorkspaceID == "" || (run.Result != "passed" && run.Result != "failed" && run.Result != "inconclusive") || len(run.Outcomes) != len(r.Checks) || len(run.Attestations) == 0 {
		return ErrInvalid
	}
	checks := map[string]bool{}
	for _, c := range r.Checks {
		checks[c.ID] = true
	}
	seen := map[string]bool{}
	allPassed := true
	for _, o := range run.Outcomes {
		if !checks[o.CheckID] || seen[o.CheckID] || (o.Status != "passed" && o.Status != "failed" && o.Status != "skipped") || len(o.SanitizedLog) > 65536 || o.DurationMS < 0 || o.CostUnits < 0 || o.RowsBefore < 0 || o.RowsAfter < 0 || o.ObjectsBefore < 0 || o.ObjectsAfter < 0 || len(o.ArtifactDigests) > 20 {
			return ErrInvalid
		}
		seen[o.CheckID] = true
		allPassed = allPassed && o.Status == "passed" && o.ExitCode == 0 && o.InvariantPassed
	}
	if (run.Result == "passed") != allPassed {
		return ErrInvalid
	}
	return nil
}

func (s *Store) FindMigrationWork(repositoryID, taskID string) (Schema, Migration, MigrationWork, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return Schema{}, Migration{}, MigrationWork{}, err
	}
	for _, dir := range entries {
		if !dir.IsDir() {
			continue
		}
		files, readErr := os.ReadDir(filepath.Join(s.root, dir.Name()))
		if readErr != nil {
			return Schema{}, Migration{}, MigrationWork{}, readErr
		}
		for _, file := range files {
			if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
				continue
			}
			body, readErr := os.ReadFile(filepath.Join(s.root, dir.Name(), file.Name()))
			if readErr != nil {
				return Schema{}, Migration{}, MigrationWork{}, readErr
			}
			var schema Schema
			if json.Unmarshal(body, &schema) != nil {
				return Schema{}, Migration{}, MigrationWork{}, ErrInvalid
			}
			for _, migration := range schema.Migrations {
				for _, work := range migration.Work {
					if work.RepositoryID == repositoryID && work.TaskID == taskID {
						return schema, migration, work, nil
					}
				}
			}
		}
	}
	return Schema{}, Migration{}, MigrationWork{}, ErrNotFound
}

func validateWork(m Migration, w MigrationWork) error {
	kinds := map[string]bool{"schema_change": true, "compatibility": true, "backfill": true, "verification": true, "cleanup": true}
	if !kinds[w.Kind] || w.RepositoryID == "" || w.StepID == "" || len(w.DependencyIDs) > 20 {
		return ErrInvalid
	}
	stepFound := false
	for _, step := range m.Steps {
		if step.ID == w.StepID {
			stepFound = true
		}
	}
	if !stepFound {
		return ErrInvalid
	}
	c := w.Contract
	if len(c.OldReaders) == 0 || len(c.NewReaders) == 0 || len(c.OldWriters) == 0 || len(c.NewWriters) == 0 || len(c.RolloutFlags) == 0 || strings.TrimSpace(c.Idempotency) == "" || len(c.Transformations) == 0 || len(c.Ownership) == 0 || len(c.RollbackAssumptions) == 0 {
		return ErrInvalid
	}
	return nil
}
func (s *Store) Get(repo, schema string) (Schema, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read(repo, schema)
}
func (s *Store) List(repo string) ([]Schema, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dir := s.repo(repo)
	entries, e := os.ReadDir(dir)
	if os.IsNotExist(e) {
		return []Schema{}, nil
	}
	if e != nil {
		return nil, e
	}
	out := []Schema{}
	for _, x := range entries {
		if x.IsDir() || !strings.HasSuffix(x.Name(), ".json") {
			continue
		}
		b, e := os.ReadFile(filepath.Join(dir, x.Name()))
		if e != nil {
			return nil, e
		}
		var v Schema
		if json.Unmarshal(b, &v) != nil {
			return nil, ErrInvalid
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}
func validateRevision(r Revision) error {
	k := map[string]bool{"database": true, "queue": true, "index": true, "object_store": true, "event_log": true, "cache": true, "other": true}
	if r.Name == "" || !k[r.StoreKind] || r.Description == "" || r.Definition == "" || r.DefinitionPath == "" || len(r.OwnerIDs) == 0 || len(r.Compatibility) == 0 || r.Retention == "" || len(r.Privacy) == 0 || r.PullRequestID == "" || r.ReviewedCommit == "" || r.Rationale == "" {
		return ErrInvalid
	}
	for _, l := range r.Links {
		if (l.Kind != "service" && l.Kind != "environment") || l.ID == "" || l.Label == "" {
			return ErrInvalid
		}
	}
	return nil
}
func validateMigration(s Schema, m Migration) error {
	if m.FromVersion < 1 || m.ToVersion < 1 || m.FromVersion >= m.ToVersion || m.ToVersion > s.CurrentVersion || (m.SourceKind != "pull_request" && m.SourceKind != "decision") || m.SourceID == "" || m.Summary == "" || len(m.Operations) == 0 || len(m.Steps) == 0 || len(m.RollbackLimits) == 0 {
		return ErrInvalid
	}
	ops := map[string]bool{}
	k := map[string]bool{"read": true, "write": true, "backfill": true, "destructive": true}
	for _, o := range m.Operations {
		if o.ID == "" || ops[o.ID] || !k[o.Kind] || o.Description == "" || len(o.OwnerIDs) == 0 || len(o.ConsumerIDs) == 0 || o.RollbackLimit == "" || (o.Kind == "destructive" && !o.Destructive) {
			return ErrInvalid
		}
		ops[o.ID] = true
	}
	steps := map[string]bool{}
	coveredOperations := map[string]bool{}
	for _, st := range m.Steps {
		if st.ID == "" || steps[st.ID] || st.Description == "" || len(st.OperationIDs) == 0 || len(st.SuccessMeasures) == 0 || len(st.RequiredApproverIDs) == 0 {
			return ErrInvalid
		}
		steps[st.ID] = true
		for _, o := range st.OperationIDs {
			if !ops[o] {
				return ErrInvalid
			}
			coveredOperations[o] = true
		}
	}
	if len(coveredOperations) != len(ops) {
		return ErrInvalid
	}
	return nil
}
func (s *Store) repo(repo string) string {
	return filepath.Join(s.root, "repo-"+hex.EncodeToString([]byte(repo)))
}
func (s *Store) read(repo, schema string) (Schema, error) {
	b, e := os.ReadFile(filepath.Join(s.repo(repo), schema+".json"))
	if os.IsNotExist(e) {
		return Schema{}, ErrNotFound
	}
	if e != nil {
		return Schema{}, e
	}
	var v Schema
	if json.Unmarshal(b, &v) != nil || v.RepositoryID != repo || v.ID != schema {
		return Schema{}, ErrInvalid
	}
	return v, nil
}
func (s *Store) write(v Schema) error {
	d := s.repo(v.RepositoryID)
	if e := os.MkdirAll(d, 0700); e != nil {
		return e
	}
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(d, ".schema-")
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
	return os.Rename(name, filepath.Join(d, v.ID+".json"))
}
func id() string { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
