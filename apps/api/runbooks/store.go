// Package runbooks persists immutable, reviewable operational procedures.
package runbooks

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

var ErrNotFound = errors.New("runbook not found")
var ErrInvalid = errors.New("invalid runbook")
var ErrConflict = errors.New("runbook conflict")

const maxRehearsalStepCostCents = 100_000_000

type Scope struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Revision   string `json:"revision,omitempty"`
	Name       string `json:"name"`
}
type Authority struct {
	RequiredAccess        []string `json:"required_access"`
	Inspects              []string `json:"inspects"`
	Changes               []string `json:"changes"`
	ProhibitedActions     []string `json:"prohibited_actions"`
	HumanApprovalRequired bool     `json:"human_approval_required"`
}
type Reference struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Revision   string `json:"revision"`
	Label      string `json:"label"`
	Reviewed   bool   `json:"reviewed"`
	Accessible bool   `json:"accessible"`
	Approved   bool   `json:"approved,omitempty"`
}
type Decision struct {
	Condition     string `json:"condition"`
	IfTrueStepID  string `json:"if_true_step_id"`
	IfFalseStepID string `json:"if_false_step_id"`
	HumanJudgment string `json:"human_judgment"`
}
type Step struct {
	ID               string      `json:"id"`
	Position         int         `json:"position"`
	Kind             string      `json:"kind"`
	Title            string      `json:"title"`
	Purpose          string      `json:"purpose"`
	Instructions     string      `json:"instructions"`
	Preconditions    []string    `json:"preconditions"`
	ExpectedEvidence []string    `json:"expected_evidence"`
	Decision         *Decision   `json:"decision,omitempty"`
	RollbackCriteria []string    `json:"rollback_criteria"`
	OwnerIDs         []string    `json:"owner_ids"`
	RequiredSkills   []string    `json:"required_skills"`
	References       []Reference `json:"references"`
	Authority        Authority   `json:"authority"`
	Assumptions      []string    `json:"assumptions"`
	PolicyRuleIDs    []string    `json:"policy_rule_ids"`
	AttributedTo     string      `json:"attributed_to,omitempty"`
}
type Escalation struct {
	Condition      string `json:"condition"`
	OwnerID        string `json:"owner_id"`
	Path           string `json:"path"`
	ExpectedAction string `json:"expected_action"`
}
type Revision struct {
	RequestID         string             `json:"request_id,omitempty"`
	Version           int                `json:"version,omitempty"`
	Title             string             `json:"title"`
	Purpose           string             `json:"purpose"`
	Scope             Scope              `json:"scope"`
	Preconditions     []string           `json:"preconditions"`
	Steps             []Step             `json:"steps"`
	RollbackCriteria  []string           `json:"rollback_criteria"`
	OutcomeCriteria   []OutcomeCriterion `json:"outcome_criteria"`
	OwnerIDs          []string           `json:"owner_ids"`
	RequiredSkills    []string           `json:"required_skills"`
	Escalations       []Escalation       `json:"escalations"`
	PolicyRevisionIDs []string           `json:"policy_revision_ids"`
	ChangeReason      string             `json:"change_reason"`
	CreatedBy         string             `json:"created_by,omitempty"`
	CreatedAt         time.Time          `json:"created_at,omitempty"`
}
type OutcomeCriterion struct {
	Kind      string `json:"kind"`
	Criterion string `json:"criterion"`
}
type RehearsalInput struct {
	Kind         string     `json:"kind"`
	ResourceID   string     `json:"resource_id"`
	Revision     string     `json:"revision"`
	EvidenceKind string     `json:"evidence_kind"`
	Digest       string     `json:"digest"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
}
type BranchDecision struct {
	StepID         string `json:"step_id"`
	Condition      string `json:"condition"`
	SelectedStepID string `json:"selected_step_id"`
	EvidenceStepID string `json:"evidence_step_id"`
	Rationale      string `json:"rationale"`
}
type ConditionAssertion struct {
	Condition      string `json:"condition"`
	Met            bool   `json:"met"`
	EvidenceDigest string `json:"evidence_digest"`
}
type Artifact struct {
	Name      string `json:"name"`
	Digest    string `json:"digest"`
	MediaType string `json:"media_type"`
}
type StepOutcome struct {
	StepID              string               `json:"step_id"`
	Command             string               `json:"command,omitempty"`
	Output              string               `json:"output"`
	StartedAt           time.Time            `json:"started_at"`
	FinishedAt          time.Time            `json:"finished_at"`
	Artifacts           []Artifact           `json:"artifacts"`
	CostCents           int                  `json:"cost_cents"`
	Permissions         []string             `json:"permissions"`
	Outcome             string               `json:"outcome"`
	ManualGaps          []string             `json:"manual_gaps"`
	DestructiveHandling string               `json:"destructive_handling"`
	Assertions          []ConditionAssertion `json:"assertions"`
}
type Scenario struct {
	ID              string           `json:"id"`
	Name            string           `json:"name"`
	Failure         string           `json:"failure"`
	Inputs          []RehearsalInput `json:"inputs"`
	Decisions       []BranchDecision `json:"decisions"`
	Steps           []StepOutcome    `json:"steps"`
	AchievedOutcome string           `json:"achieved_outcome"`
}
type Rehearsal struct {
	ID                     string     `json:"id"`
	RequestID              string     `json:"request_id"`
	RequestDigest          string     `json:"request_digest"`
	RunbookVersion         int        `json:"runbook_version"`
	EnvironmentKind        string     `json:"environment_kind"`
	EnvironmentID          string     `json:"environment_id"`
	PolicyApprovalRevision string     `json:"policy_approval_revision,omitempty"`
	Scenarios              []Scenario `json:"scenarios"`
	Status                 string     `json:"status"`
	ActorType              string     `json:"actor_type"`
	ActorID                string     `json:"actor_id"`
	CreatedAt              time.Time  `json:"created_at"`
	Stale                  bool       `json:"stale"`
	StaleReasons           []string   `json:"stale_reasons"`
}
type Diagnostic struct {
	Kind         string `json:"kind"`
	Severity     string `json:"severity"`
	Message      string `json:"message"`
	StepID       string `json:"step_id,omitempty"`
	ResourceID   string `json:"resource_id,omitempty"`
	AttributedTo string `json:"attributed_to"`
}
type ExecutionEvidence struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Revision   string `json:"revision"`
	Digest     string `json:"digest"`
	Summary    string `json:"summary"`
}
type ExecutionContext struct {
	OriginKind          string              `json:"origin_kind"`
	OriginID            string              `json:"origin_id"`
	OriginRevision      string              `json:"origin_revision"`
	Summary             string              `json:"summary"`
	AffectedResources   []string            `json:"affected_resources"`
	SignalClass         string              `json:"signal_class,omitempty"`
	WindowFrom          time.Time           `json:"window_from"`
	WindowTo            time.Time           `json:"window_to"`
	ReleaseIDs          []string            `json:"release_ids"`
	EnvironmentID       string              `json:"environment_id"`
	EnvironmentRevision string              `json:"environment_revision"`
	Evidence            []ExecutionEvidence `json:"evidence"`
	TimelineRefs        []string            `json:"timeline_refs"`
	AudienceIDs         []string            `json:"audience_ids"`
}
type Preconditions struct {
	Condition      string `json:"condition"`
	Status         string `json:"status"`
	EvidenceDigest string `json:"evidence_digest,omitempty"`
}
type ExecutionBlocker struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}
type Execution struct {
	ID                  string                 `json:"id"`
	RequestID           string                 `json:"request_id"`
	RequestDigest       string                 `json:"request_digest"`
	RunbookID           string                 `json:"runbook_id"`
	RunbookVersion      int                    `json:"runbook_version"`
	Context             ExecutionContext       `json:"context"`
	Preconditions       []Preconditions        `json:"preconditions"`
	CurrentAccess       []string               `json:"current_access"`
	Blockers            []ExecutionBlocker     `json:"blockers"`
	Status              string                 `json:"status"`
	StartedBy           string                 `json:"started_by"`
	CreatedAt           time.Time              `json:"created_at"`
	Version             int                    `json:"version"`
	ControllerType      string                 `json:"controller_type"`
	ControllerID        string                 `json:"controller_id"`
	CurrentStepID       string                 `json:"current_step_id,omitempty"`
	Participants        []ExecutionParticipant `json:"participants"`
	Delegations         []ExecutionDelegation  `json:"delegations"`
	Receipts            []ActionReceipt        `json:"receipts"`
	CompletedEvidence   []ExecutionEvidence    `json:"completed_evidence"`
	PendingDecisions    []string               `json:"pending_decisions"`
	ScopedCredentials   []string               `json:"scoped_credentials"`
	Health              string                 `json:"health"`
	CostCents           int                    `json:"cost_cents"`
	RollbackState       string                 `json:"rollback_state"`
	PredictedNextAction string                 `json:"predicted_next_action"`
	Assessment          *ExecutionAssessment   `json:"assessment,omitempty"`
}
type CriterionResult struct {
	Kind            string   `json:"kind"`
	Criterion       string   `json:"criterion"`
	Status          string   `json:"status"`
	EvidenceDigests []string `json:"evidence_digests"`
	Explanation     string   `json:"explanation"`
}
type ExecutionDeviation struct {
	Kind            string   `json:"kind"`
	StepID          string   `json:"step_id,omitempty"`
	Summary         string   `json:"summary"`
	EvidenceDigests []string `json:"evidence_digests"`
}
type ParticipantFeedback struct {
	ParticipantType string `json:"participant_type"`
	ParticipantID   string `json:"participant_id"`
	Summary         string `json:"summary"`
}
type ExecutionFinding struct {
	ID                     string                  `json:"id"`
	Kind                   string                  `json:"kind"`
	Summary                string                  `json:"summary"`
	EvidenceDigests        []string                `json:"evidence_digests"`
	ProposalID             string                  `json:"proposal_id,omitempty"`
	TaskID                 string                  `json:"task_id,omitempty"`
	ImprovementRequestID   string                  `json:"improvement_request_id,omitempty"`
	ImprovementActorID     string                  `json:"improvement_actor_id,omitempty"`
	ImprovementReservation *ImprovementReservation `json:"improvement_reservation,omitempty"`
}
type ImprovementReservation struct {
	RequestID string    `json:"request_id"`
	ActorID   string    `json:"actor_id"`
	Kind      string    `json:"kind"`
	CreatedAt time.Time `json:"created_at"`
}
type ExecutionAssessment struct {
	RequestID             string                `json:"request_id"`
	RequestDigest         string                `json:"request_digest"`
	Outcome               string                `json:"outcome"`
	Criteria              []CriterionResult     `json:"criteria"`
	Deviations            []ExecutionDeviation  `json:"deviations"`
	Findings              []ExecutionFinding    `json:"findings"`
	Feedback              []ParticipantFeedback `json:"feedback"`
	RequireFreshRehearsal bool                  `json:"require_fresh_rehearsal"`
	AssessedBy            string                `json:"assessed_by"`
	CreatedAt             time.Time             `json:"created_at"`
}
type AssessmentInput struct {
	RequestID              string                `json:"request_id"`
	ExpectedVersion        int                   `json:"expected_version"`
	Outcome                string                `json:"outcome"`
	Criteria               []CriterionResult     `json:"criteria"`
	Deviations             []ExecutionDeviation  `json:"deviations"`
	Findings               []ExecutionFinding    `json:"findings"`
	Feedback               []ParticipantFeedback `json:"feedback"`
	RequireFreshRehearsal  bool                  `json:"require_fresh_rehearsal"`
	SuspendCurrentUse      bool                  `json:"suspend_current_use"`
	FallbackRunbookID      string                `json:"fallback_runbook_id,omitempty"`
	FallbackRunbookVersion int                   `json:"fallback_runbook_version,omitempty"`
}
type ImprovementLink struct {
	RequestID  string `json:"request_id"`
	FindingID  string `json:"finding_id"`
	Kind       string `json:"kind"`
	ProposalID string `json:"proposal_id"`
	TaskID     string `json:"task_id"`
}
type ExecutionParticipant struct {
	ActorType string    `json:"actor_type"`
	ActorID   string    `json:"actor_id"`
	JoinedAt  time.Time `json:"joined_at"`
}
type ExecutionDelegation struct {
	StepID      string    `json:"step_id"`
	AgentID     string    `json:"agent_id"`
	Actions     []string  `json:"actions"`
	DelegatedBy string    `json:"delegated_by"`
	CreatedAt   time.Time `json:"created_at"`
}
type ActionReceipt struct {
	ID              string              `json:"id"`
	RequestID       string              `json:"request_id"`
	RequestDigest   string              `json:"request_digest"`
	ExpectedVersion int                 `json:"expected_version"`
	Action          string              `json:"action"`
	StepID          string              `json:"step_id,omitempty"`
	ActorType       string              `json:"actor_type"`
	ActorID         string              `json:"actor_id"`
	Message         string              `json:"message,omitempty"`
	Evidence        []ExecutionEvidence `json:"evidence"`
	CostCents       int                 `json:"cost_cents"`
	Health          string              `json:"health,omitempty"`
	CreatedAt       time.Time           `json:"created_at"`
}
type ExecutionAction struct {
	RequestID        string              `json:"request_id"`
	ExpectedVersion  int                 `json:"expected_version"`
	Action           string              `json:"action"`
	StepID           string              `json:"step_id,omitempty"`
	Message          string              `json:"message,omitempty"`
	Evidence         []ExecutionEvidence `json:"evidence"`
	CostCents        int                 `json:"cost_cents"`
	TargetID         string              `json:"target_id,omitempty"`
	DelegatedActions []string            `json:"delegated_actions,omitempty"`
	Decision         string              `json:"decision,omitempty"`
	Health           string              `json:"health,omitempty"`
}
type Recommendation struct {
	RunbookID      string             `json:"runbook_id"`
	RunbookVersion int                `json:"runbook_version"`
	Title          string             `json:"title"`
	Eligible       bool               `json:"eligible"`
	Score          int                `json:"score"`
	Reasons        []string           `json:"reasons"`
	Blockers       []ExecutionBlocker `json:"blockers"`
	ChoiceRequired bool               `json:"choice_required"`
}
type Runbook struct {
	ID                     string       `json:"id"`
	RepositoryID           string       `json:"repository_id"`
	RequestID              string       `json:"request_id"`
	RequestDigest          string       `json:"request_digest"`
	CurrentVersion         int          `json:"current_version"`
	Revisions              []Revision   `json:"revisions"`
	Diagnostics            []Diagnostic `json:"diagnostics"`
	CreatedAt              time.Time    `json:"created_at"`
	UpdatedAt              time.Time    `json:"updated_at"`
	Rehearsals             []Rehearsal  `json:"rehearsals"`
	Executions             []Execution  `json:"executions"`
	UseStatus              string       `json:"use_status"`
	SuspensionReason       string       `json:"suspension_reason,omitempty"`
	FallbackRunbookID      string       `json:"fallback_runbook_id,omitempty"`
	FallbackRunbookVersion int          `json:"fallback_runbook_version,omitempty"`
}

func (s *Store) Recommend(repo string, context ExecutionContext) ([]Recommendation, error) {
	if !validExecutionContext(context) {
		return nil, ErrInvalid
	}
	books, err := s.List(repo)
	if err != nil {
		return nil, err
	}
	out := []Recommendation{}
	for _, book := range books {
		r := book.Revisions[book.CurrentVersion-1]
		score := 0
		reasons := []string{}
		for _, resource := range context.AffectedResources {
			if resource == r.Scope.ResourceID {
				score += 100
				reasons = append(reasons, "affected resource matches runbook scope")
			}
		}
		if r.Scope.Kind == "signal" && (r.Scope.ResourceID == context.SignalClass || r.Scope.Name == context.SignalClass) {
			score += 80
			reasons = append(reasons, "signal class matches runbook scope")
		}
		if r.Scope.Kind == "environment" && r.Scope.ResourceID == context.EnvironmentID {
			score += 90
			reasons = append(reasons, "environment matches runbook scope")
		}
		if score == 0 {
			continue
		}
		blockers := executionSafetyBlockers(book, r, nil, nil)
		eligibleBlockers := blockers[:0]
		for _, blocker := range blockers {
			if blocker.Kind != "precondition_not_met" && blocker.Kind != "access_unavailable" {
				eligibleBlockers = append(eligibleBlockers, blocker)
			}
		}
		blockers = eligibleBlockers
		out = append(out, Recommendation{RunbookID: book.ID, RunbookVersion: book.CurrentVersion, Title: r.Title, Eligible: len(blockers) == 0, Score: score, Reasons: reasons, Blockers: blockers})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].Title < out[j].Title
		}
		return out[i].Score > out[j].Score
	})
	if len(out) > 1 && out[0].Score == out[1].Score {
		for i := range out {
			if out[i].Score == out[0].Score {
				out[i].ChoiceRequired = true
				out[i].Blockers = append(out[i].Blockers, ExecutionBlocker{Kind: "ambiguous_match", Message: "Multiple runbooks match equally; a responder must choose explicitly."})
			}
		}
	}
	return out, nil
}

func (s *Store) StartExecution(id, actor, request string, version int, context ExecutionContext, preconditions []Preconditions, access []string) (Execution, error) {
	var out Execution
	err := s.lock(func() error {
		book, err := s.read(id)
		if err != nil {
			return err
		}
		candidate := Execution{RunbookID: id, RunbookVersion: version, Context: context, Preconditions: preconditions, CurrentAccess: uniqueStrings(access)}
		d := executionDigest(candidate)
		for _, old := range book.Executions {
			if old.RequestID == request {
				if old.RequestDigest != d || old.StartedBy != actor {
					return ErrConflict
				}
				out = old
				return nil
			}
		}
		if request == "" || version < 1 || version > book.CurrentVersion || !validExecutionContext(context) {
			return ErrInvalid
		}
		for _, old := range book.Executions {
			if old.Status == "ready" && old.Context.OriginKind == context.OriginKind && old.Context.OriginID == context.OriginID && old.RunbookVersion == version {
				return ErrConflict
			}
		}
		r := book.Revisions[version-1]
		blockers := executionSafetyBlockers(project(book), r, preconditions, access)
		now := s.now()
		candidate.ID = stableID(id, actor, request)
		candidate.RequestID = request
		candidate.RequestDigest = d
		candidate.StartedBy = actor
		candidate.CreatedAt = now
		candidate.Version = 1
		candidate.ControllerType, candidate.ControllerID = "human", actor
		candidate.ScopedCredentials = uniqueStrings(access)
		candidate.Participants = []ExecutionParticipant{{ActorType: "human", ActorID: actor, JoinedAt: now}}
		candidate.Health, candidate.RollbackState = "unknown", "not_started"
		if len(r.Steps) > 0 {
			projectExecutionStep(&candidate, r.Steps[0])
		}
		candidate.Blockers = blockers
		candidate.Status = "ready"
		if len(blockers) > 0 {
			candidate.Status = "blocked"
		}
		book.Executions = append(book.Executions, candidate)
		book.UpdatedAt = now
		out = candidate
		return s.write(book)
	})
	return out, err
}

// Act appends an immutable, caller-stable receipt while atomically advancing the
// projected execution. Authority is supplied by the authenticated route and is
// still narrowed here by controller, approval, and exact agent delegation.
func (s *Store) Act(id, executionID, actorType, actorID string, in ExecutionAction) (Execution, error) {
	var out Execution
	err := s.lock(func() error {
		book, err := s.read(id)
		if err != nil {
			return err
		}
		i := -1
		for n := range book.Executions {
			if book.Executions[n].ID == executionID {
				i = n
				break
			}
		}
		if i < 0 {
			return ErrNotFound
		}
		e := &book.Executions[i]
		d := actionDigest(in)
		for _, old := range e.Receipts {
			if old.RequestID == in.RequestID {
				if old.RequestDigest != d || old.ActorType != actorType || old.ActorID != actorID {
					return ErrConflict
				}
				out = *e
				return nil
			}
		}
		if in.RequestID == "" || in.ExpectedVersion != e.Version || in.CostCents < 0 || in.CostCents > maxRehearsalStepCostCents || secretPattern.MatchString(in.Message) {
			return ErrConflict
		}
		for _, evidence := range in.Evidence {
			if evidence.Kind == "" || evidence.ResourceID == "" || evidence.Revision == "" || evidence.Digest == "" || evidence.Summary == "" || len(evidence.Summary) > 2000 || secretPattern.MatchString(evidence.Summary) {
				return ErrInvalid
			}
		}
		if in.Health != "" && in.Health != "unknown" && in.Health != "healthy" && in.Health != "degraded" && in.Health != "unhealthy" {
			return ErrInvalid
		}
		actions := map[string]bool{"join": true, "discuss": true, "approve": true, "decide": true, "perform": true, "analyze": true, "skip": true, "pause": true, "handoff": true, "abort": true, "delegate": true, "rollback": true}
		if !actions[in.Action] {
			return ErrInvalid
		}
		revision := book.Revisions[e.RunbookVersion-1]
		step, hasStep := executionStep(revision, in.StepID)
		controller := e.ControllerType == actorType && e.ControllerID == actorID
		if actorType == "agent" {
			delegated := false
			for _, x := range e.Delegations {
				if x.AgentID == actorID && x.StepID == in.StepID && containsString(x.Actions, in.Action) {
					delegated = true
				}
			}
			if !delegated || (in.Action != "analyze" && in.Action != "perform") {
				return ErrConflict
			}
		}
		if actorType == "human" && in.Action != "join" && in.Action != "discuss" && in.Action != "approve" && !controller {
			return ErrConflict
		}
		if e.Status == "blocked" && in.Action != "join" && in.Action != "discuss" && in.Action != "abort" {
			return ErrConflict
		}
		if e.Status != "ready" && (in.Action == "approve" || in.Action == "decide" || in.Action == "perform" || in.Action == "analyze" || in.Action == "skip" || in.Action == "delegate" || in.Action == "handoff" || in.Action == "pause") {
			return ErrConflict
		}
		if (in.Action == "perform" || in.Action == "analyze" || in.Action == "skip" || in.Action == "approve" || in.Action == "decide") && (!hasStep || in.StepID != e.CurrentStepID) {
			return ErrConflict
		}
		if in.Action == "skip" && (step.Kind != "communication" || len(step.Authority.Changes) > 0) {
			return ErrConflict
		}
		if in.Action == "perform" && step.Authority.HumanApprovalRequired {
			approved := false
			for _, r := range e.Receipts {
				if r.Action == "approve" && r.StepID == in.StepID && r.ActorID != actorID {
					approved = true
				}
			}
			if !approved {
				return ErrConflict
			}
		}
		now := s.now()
		receipt := ActionReceipt{ID: stableID(executionID, actorID, in.RequestID), RequestID: in.RequestID, RequestDigest: d, ExpectedVersion: in.ExpectedVersion, Action: in.Action, StepID: in.StepID, ActorType: actorType, ActorID: actorID, Message: in.Message, Evidence: in.Evidence, CostCents: in.CostCents, Health: in.Health, CreatedAt: now}
		switch in.Action {
		case "join":
			if !executionParticipant(e.Participants, actorType, actorID) {
				e.Participants = append(e.Participants, ExecutionParticipant{actorType, actorID, now})
			}
		case "handoff":
			if in.TargetID == "" || !executionParticipant(e.Participants, "human", in.TargetID) {
				return ErrInvalid
			}
			e.ControllerType, e.ControllerID = "human", in.TargetID
		case "delegate":
			if in.TargetID == "" || !hasStep || len(in.DelegatedActions) == 0 {
				return ErrInvalid
			}
			for _, a := range in.DelegatedActions {
				if a != "analyze" && a != "perform" {
					return ErrInvalid
				}
			}
			e.Delegations = append(e.Delegations, ExecutionDelegation{in.StepID, in.TargetID, uniqueStrings(in.DelegatedActions), actorID, now})
		case "pause":
			e.Status = "paused"
		case "decide":
			if !hasStep || step.Decision == nil || !containsString(e.PendingDecisions, step.Decision.Condition) || in.Decision == "" {
				return ErrInvalid
			}
			next := step.Decision.IfFalseStepID
			if in.Decision == "true" {
				next = step.Decision.IfTrueStepID
			} else if in.Decision != "false" {
				return ErrInvalid
			}
			e.PendingDecisions = nil
			if next == "" {
				e.CurrentStepID = ""
				e.Status = "completed"
				e.PredictedNextAction = "verify outcome"
			} else if n, ok := executionStep(revision, next); ok {
				projectExecutionStep(e, n)
			}
		case "abort":
			e.Status = "aborted"
			e.RollbackState = "required"
		case "rollback":
			e.Status = "rolling_back"
			e.RollbackState = "in_progress"
		case "perform", "skip":
			e.CompletedEvidence = append(e.CompletedEvidence, in.Evidence...)
			e.CostCents += in.CostCents
			if in.Health != "" {
				e.Health = in.Health
			}
			advanceExecution(e, revision)
		case "analyze":
			e.CompletedEvidence = append(e.CompletedEvidence, in.Evidence...)
			e.CostCents += in.CostCents
			if in.Health != "" {
				e.Health = in.Health
			}
		}
		e.Receipts = append(e.Receipts, receipt)
		e.Version++
		book.UpdatedAt = now
		out = *e
		return s.write(book)
	})
	return out, err
}

// Assess freezes the service outcome and procedure fitness without changing the
// revision used by the execution. Only a terminal execution can be assessed.
func (s *Store) Assess(id, executionID, actorID string, in AssessmentInput) (Execution, error) {
	var out Execution
	err := s.lock(func() error {
		book, err := s.read(id)
		if err != nil {
			return err
		}
		i := -1
		for n := range book.Executions {
			if book.Executions[n].ID == executionID {
				i = n
				break
			}
		}
		if i < 0 {
			return ErrNotFound
		}
		e := &book.Executions[i]
		d := assessmentDigest(in)
		if e.Assessment != nil {
			if e.Assessment.RequestID == in.RequestID && e.Assessment.RequestDigest == d && e.Assessment.AssessedBy == actorID {
				out = *e
				return nil
			}
			return ErrConflict
		}
		if in.RequestID == "" || in.ExpectedVersion != e.Version || (e.Status != "completed" && e.Status != "aborted") || (in.Outcome != "completed" && in.Outcome != "abandoned") {
			return ErrConflict
		}
		revision := book.Revisions[e.RunbookVersion-1]
		if !validAssessment(in, revision, *e) {
			return ErrInvalid
		}
		now := s.now()
		findings := append([]ExecutionFinding(nil), in.Findings...)
		for n := range findings {
			findings[n].ID = stableID(executionID, in.RequestID, findings[n].Kind, findings[n].Summary)
		}
		e.Assessment = &ExecutionAssessment{RequestID: in.RequestID, RequestDigest: d, Outcome: in.Outcome, Criteria: in.Criteria, Deviations: in.Deviations, Findings: findings, Feedback: in.Feedback, RequireFreshRehearsal: in.RequireFreshRehearsal, AssessedBy: actorID, CreatedAt: now}
		e.Status = "assessed"
		e.PredictedNextAction = "review findings and linked improvement work"
		e.Version++
		if in.SuspendCurrentUse {
			fallback, fallbackErr := s.read(in.FallbackRunbookID)
			if fallbackErr != nil || !validFallback(project(fallback), book, in) {
				return ErrInvalid
			}
			book.UseStatus, book.SuspensionReason = "suspended", "terminal execution "+executionID+" found repeated failure or unsafe drift"
			book.FallbackRunbookID, book.FallbackRunbookVersion = in.FallbackRunbookID, in.FallbackRunbookVersion
		}
		book.UpdatedAt = now
		out = *e
		return s.write(book)
	})
	return out, err
}

func validFallback(fallback, current Runbook, in AssessmentInput) bool {
	if fallback.ID == current.ID || fallback.RepositoryID != current.RepositoryID || fallback.CurrentVersion != in.FallbackRunbookVersion || fallback.UseStatus == "suspended" {
		return false
	}
	for _, rehearsal := range fallback.Rehearsals {
		if rehearsal.RunbookVersion == in.FallbackRunbookVersion && rehearsal.Status == "passed" && !rehearsal.Stale {
			return true
		}
	}
	return false
}

func validAssessment(in AssessmentInput, revision Revision, execution Execution) bool {
	if len(revision.OutcomeCriteria) == 0 || len(in.Criteria) != len(revision.OutcomeCriteria) || len(in.Findings) > 50 || len(in.Feedback) > 100 || secretPattern.MatchString(string(mustJSON(in))) {
		return false
	}
	declared := map[string]bool{}
	for _, c := range revision.OutcomeCriteria {
		declared[c.Kind+"\x00"+c.Criterion] = true
	}
	evidence := map[string]bool{}
	for _, e := range execution.CompletedEvidence {
		evidence[e.Digest] = true
	}
	for _, e := range execution.Context.Evidence {
		evidence[e.Digest] = true
	}
	seen := map[string]bool{}
	validKinds := map[string]bool{"health": true, "containment": true, "recovery": true, "communication": true, "rollback": true}
	for _, c := range in.Criteria {
		key := c.Kind + "\x00" + c.Criterion
		if !validKinds[c.Kind] || !declared[key] || seen[key] || (c.Status != "met" && c.Status != "unmet" && c.Status != "unknown") || c.Explanation == "" {
			return false
		}
		for _, d := range c.EvidenceDigests {
			if !evidence[d] {
				return false
			}
		}
		seen[key] = true
	}
	deviationKinds := map[string]bool{"deviation": true, "manual_work": true, "failed_step": true, "timing": true, "access_gap": true, "agent_correction": true, "cost": true}
	for _, d := range in.Deviations {
		if !deviationKinds[d.Kind] || d.Summary == "" {
			return false
		}
		if d.StepID != "" {
			if _, ok := executionStep(revision, d.StepID); !ok {
				return false
			}
		}
		for _, x := range d.EvidenceDigests {
			if !evidence[x] {
				return false
			}
		}
	}
	findingKinds := map[string]bool{"documentation": true, "workflow": true, "policy": true, "infrastructure": true, "code": true}
	for _, f := range in.Findings {
		if !findingKinds[f.Kind] || f.Summary == "" || len(f.EvidenceDigests) == 0 {
			return false
		}
		for _, x := range f.EvidenceDigests {
			if !evidence[x] {
				return false
			}
		}
	}
	for _, f := range in.Feedback {
		if f.Summary == "" || !executionParticipant(execution.Participants, f.ParticipantType, f.ParticipantID) {
			return false
		}
	}
	if in.SuspendCurrentUse && (in.FallbackRunbookID == "" || in.FallbackRunbookVersion < 1 || !in.RequireFreshRehearsal) {
		return false
	}
	return true
}
func assessmentDigest(in AssessmentInput) string {
	in.ExpectedVersion = 0
	b, _ := json.Marshal(in)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func (s *Store) LinkImprovement(id, executionID, actorID string, link ImprovementLink) (Execution, error) {
	var out Execution
	if actorID == "" || link.RequestID == "" || link.FindingID == "" || link.ProposalID == "" || link.TaskID == "" {
		return out, ErrInvalid
	}
	err := s.lock(func() error {
		book, err := s.read(id)
		if err != nil {
			return err
		}
		for i := range book.Executions {
			e := &book.Executions[i]
			if e.ID != executionID {
				continue
			}
			if e.Assessment == nil {
				return ErrConflict
			}
			for n := range e.Assessment.Findings {
				f := &e.Assessment.Findings[n]
				if f.ID != link.FindingID {
					continue
				}
				if f.ProposalID != "" {
					if f.ProposalID == link.ProposalID && f.TaskID == link.TaskID {
						out = *e
						return nil
					}
					return ErrConflict
				}
				if f.Kind != link.Kind || f.ImprovementReservation == nil || f.ImprovementReservation.RequestID != link.RequestID || f.ImprovementReservation.ActorID != actorID || f.ImprovementReservation.Kind != link.Kind {
					return ErrConflict
				}
				f.ProposalID, f.TaskID = link.ProposalID, link.TaskID
				f.ImprovementRequestID, f.ImprovementActorID = link.RequestID, actorID
				f.ImprovementReservation = nil
				e.Version++
				book.UpdatedAt = s.now()
				out = *e
				return s.write(book)
			}
			return ErrNotFound
		}
		return ErrNotFound
	})
	return out, err
}

// ReserveImprovement caller-stably claims one finding before ordinary work is
// created, preventing competing requests from leaving unlinked assigned work.
func (s *Store) ReserveImprovement(id, executionID, actorID, requestID, findingID, kind string) (Execution, error) {
	var out Execution
	if actorID == "" || requestID == "" || findingID == "" || kind == "" {
		return out, ErrInvalid
	}
	err := s.lock(func() error {
		book, err := s.read(id)
		if err != nil {
			return err
		}
		for i := range book.Executions {
			e := &book.Executions[i]
			if e.ID != executionID {
				continue
			}
			if e.Assessment == nil {
				return ErrConflict
			}
			for n := range e.Assessment.Findings {
				f := &e.Assessment.Findings[n]
				if f.ID != findingID {
					continue
				}
				if f.Kind != kind {
					return ErrConflict
				}
				if f.ProposalID != "" {
					if f.ImprovementRequestID == requestID && f.ImprovementActorID == actorID {
						out = *e
						return nil
					}
					return ErrConflict
				}
				if f.ImprovementReservation != nil {
					if f.ImprovementReservation.RequestID == requestID && f.ImprovementReservation.ActorID == actorID && f.ImprovementReservation.Kind == kind {
						out = *e
						return nil
					}
					return ErrConflict
				}
				f.ImprovementReservation = &ImprovementReservation{RequestID: requestID, ActorID: actorID, Kind: kind, CreatedAt: s.now()}
				e.Version++
				book.UpdatedAt = s.now()
				out = *e
				return s.write(book)
			}
			return ErrNotFound
		}
		return ErrNotFound
	})
	return out, err
}

func (s *Store) ReleaseImprovementReservation(id, executionID, actorID, requestID, findingID string) error {
	return s.lock(func() error {
		book, err := s.read(id)
		if err != nil {
			return err
		}
		for i := range book.Executions {
			e := &book.Executions[i]
			if e.ID != executionID || e.Assessment == nil {
				continue
			}
			for n := range e.Assessment.Findings {
				f := &e.Assessment.Findings[n]
				if f.ID != findingID {
					continue
				}
				if f.ProposalID != "" || f.ImprovementReservation == nil || f.ImprovementReservation.RequestID != requestID || f.ImprovementReservation.ActorID != actorID {
					return ErrConflict
				}
				f.ImprovementReservation = nil
				e.Version++
				book.UpdatedAt = s.now()
				return s.write(book)
			}
		}
		return ErrNotFound
	})
}
func executionStep(r Revision, id string) (Step, bool) {
	for _, s := range r.Steps {
		if s.ID == id {
			return s, true
		}
	}
	return Step{}, false
}
func executionParticipant(xs []ExecutionParticipant, t, id string) bool {
	for _, x := range xs {
		if x.ActorType == t && x.ActorID == id {
			return true
		}
	}
	return false
}
func advanceExecution(e *Execution, r Revision) {
	for i, s := range r.Steps {
		if s.ID == e.CurrentStepID {
			if s.Decision != nil {
				e.PendingDecisions = []string{s.Decision.Condition}
				e.PredictedNextAction = "record decision: " + s.Decision.Condition
				return
			}
			if i+1 < len(r.Steps) {
				projectExecutionStep(e, r.Steps[i+1])
			} else {
				e.CurrentStepID = ""
				e.Status = "completed"
				e.PredictedNextAction = "verify outcome"
			}
			return
		}
	}
}
func projectExecutionStep(e *Execution, step Step) {
	e.CurrentStepID = step.ID
	e.PendingDecisions = nil
	if step.Decision != nil {
		e.PendingDecisions = []string{step.Decision.Condition}
		e.PredictedNextAction = "record decision: " + step.Decision.Condition
		return
	}
	e.PredictedNextAction = "perform " + step.Title
}
func actionDigest(v ExecutionAction) string {
	v.RequestID = ""
	v.ExpectedVersion = 0
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func validExecutionContext(c ExecutionContext) bool {
	kinds := map[string]bool{"alert": true, "incident": true, "deployment": true, "failed_workflow": true, "service_objective": true, "support_thread": true, "manual_observation": true}
	if !kinds[c.OriginKind] || c.OriginID == "" || c.OriginRevision == "" || c.Summary == "" || len(c.Summary) > 4000 || len(c.AffectedResources) == 0 || c.WindowFrom.IsZero() || c.WindowTo.Before(c.WindowFrom) || len(c.Evidence) > 20 || secretPattern.MatchString(string(mustJSON(c))) {
		return false
	}
	for _, e := range c.Evidence {
		if e.Kind == "" || e.ResourceID == "" || e.Revision == "" || e.Digest == "" || e.Summary == "" || len(e.Summary) > 2000 {
			return false
		}
	}
	return true
}
func executionSafetyBlockers(book Runbook, r Revision, checks []Preconditions, access []string) []ExecutionBlocker {
	out := []ExecutionBlocker{}
	if book.UseStatus == "suspended" {
		message := "Current use is suspended: " + book.SuspensionReason
		if book.FallbackRunbookID != "" {
			message += "; use approved fallback " + book.FallbackRunbookID
		}
		out = append(out, ExecutionBlocker{Kind: "runbook_suspended", Message: message})
	}
	if r.Version != book.CurrentVersion {
		out = append(out, ExecutionBlocker{Kind: "stale_runbook", Message: "A newer runbook revision exists; choose whether to use the retained revision."})
	}
	for _, d := range book.Diagnostics {
		if d.Severity == "blocking" {
			out = append(out, ExecutionBlocker{Kind: d.Kind, Message: d.Message})
		}
	}
	passing := false
	for _, rehearsal := range book.Rehearsals {
		if rehearsal.RunbookVersion == r.Version && rehearsal.Status == "passed" && !rehearsal.Stale {
			passing = true
		}
	}
	if !passing {
		out = append(out, ExecutionBlocker{Kind: "unverified_procedure", Message: "This exact runbook revision has no current passing rehearsal."})
	}
	states := map[string]Preconditions{}
	for _, c := range checks {
		if c.Condition == "" || (c.Status != "met" && c.Status != "unmet" && c.Status != "unknown") {
			out = append(out, ExecutionBlocker{Kind: "invalid_precondition_decision", Message: "Precondition decisions must be met, unmet, or unknown."})
		}
		if _, exists := states[c.Condition]; exists {
			out = append(out, ExecutionBlocker{Kind: "invalid_precondition_decision", Message: "Each precondition must have one explicit current decision."})
		}
		states[c.Condition] = c
	}
	for _, required := range r.Preconditions {
		c, ok := states[required]
		if !ok || c.Status != "met" {
			out = append(out, ExecutionBlocker{Kind: "precondition_not_met", Message: "Precondition requires an explicit current decision: " + required})
		}
	}
	granted := map[string]bool{}
	for _, a := range access {
		granted[a] = true
	}
	for _, step := range r.Steps {
		for _, required := range step.Authority.RequiredAccess {
			if !granted[required] {
				out = append(out, ExecutionBlocker{Kind: "access_unavailable", Message: "Current access is unavailable for " + required + "."})
			}
		}
	}
	return uniqueBlockers(out)
}
func uniqueBlockers(xs []ExecutionBlocker) []ExecutionBlocker {
	seen := map[string]bool{}
	out := []ExecutionBlocker{}
	for _, x := range xs {
		k := x.Kind + "\x00" + x.Message
		if !seen[k] {
			seen[k] = true
			out = append(out, x)
		}
	}
	return out
}
func executionDigest(v Execution) string {
	v.ID = ""
	v.RequestID = ""
	v.RequestDigest = ""
	v.Blockers = nil
	v.Status = ""
	v.StartedBy = ""
	v.CreatedAt = time.Time{}
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func (s *Store) Rehearse(id, actorType, actorID, request string, version int, environmentKind, environmentID, policyApproval string, scenarios []Scenario) (Runbook, error) {
	var out Runbook
	err := s.lock(func() error {
		v, e := s.read(id)
		if e != nil {
			return e
		}
		candidate := Rehearsal{RunbookVersion: version, EnvironmentKind: environmentKind, EnvironmentID: environmentID, PolicyApprovalRevision: policyApproval, Scenarios: scenarios}
		d := rehearsalDigest(candidate)
		for _, old := range v.Rehearsals {
			if old.RequestID == request {
				if old.RequestDigest != d || old.ActorID != actorID {
					return ErrConflict
				}
				out = v
				return nil
			}
		}
		if request == "" || version < 1 || version > v.CurrentVersion || !validRehearsal(candidate, v.Revisions[version-1]) {
			return ErrInvalid
		}
		now := s.now()
		candidate.ID = stableID(id, actorType, actorID, request)
		candidate.RequestID = request
		candidate.RequestDigest = d
		candidate.ActorType = actorType
		candidate.ActorID = actorID
		candidate.CreatedAt = now
		candidate.Status = rehearsalStatus(scenarios)
		v.Rehearsals = append(v.Rehearsals, candidate)
		v.UpdatedAt = now
		out = v
		return s.write(v)
	})
	return project(out), err
}

func validRehearsal(v Rehearsal, revision Revision) bool {
	if (v.EnvironmentKind != "isolated" && v.EnvironmentKind != "policy_approved") || strings.TrimSpace(v.EnvironmentID) == "" || (v.EnvironmentKind == "policy_approved" && strings.TrimSpace(v.PolicyApprovalRevision) == "") || len(v.Scenarios) == 0 {
		return false
	}
	if v.EnvironmentKind == "policy_approved" && !containsString(revision.PolicyRevisionIDs, v.PolicyApprovalRevision) {
		return false
	}
	stepIDs := map[string]Step{}
	for _, s := range revision.Steps {
		stepIDs[s.ID] = s
	}
	seen := map[string]bool{}
	for _, scenario := range v.Scenarios {
		if scenario.ID == "" || seen[scenario.ID] || scenario.Name == "" || scenario.Failure == "" || len(scenario.Inputs) == 0 || len(scenario.Steps) == 0 || scenario.AchievedOutcome == "" {
			return false
		}
		seen[scenario.ID] = true
		for _, in := range scenario.Inputs {
			if in.ResourceID == "" || in.Revision == "" || in.Digest == "" || (in.EvidenceKind != "synthetic" && in.EvidenceKind != "permitted") {
				return false
			}
		}
		covered := map[string]bool{}
		outcomes := map[string]StepOutcome{}
		for _, result := range scenario.Steps {
			step, ok := stepIDs[result.StepID]
			if !ok || covered[result.StepID] || result.Output == "" || len(result.Output) > 32768 || secretPattern.MatchString(result.Output+result.Command) || result.FinishedAt.Before(result.StartedAt) || result.CostCents < 0 || result.CostCents > maxRehearsalStepCostCents || (result.Outcome != "passed" && result.Outcome != "failed" && result.Outcome != "manual_gap") {
				return false
			}
			covered[result.StepID] = true
			outcomes[result.StepID] = result
			for _, required := range step.Authority.RequiredAccess {
				if !containsString(result.Permissions, required) {
					return false
				}
			}
			if result.Command != "" {
				matched := false
				for _, ref := range step.References {
					if ref.Kind == "command" && ref.ResourceID == result.Command && ref.Reviewed && ref.Accessible {
						matched = true
					}
				}
				if !matched {
					return false
				}
			}
			if len(step.Authority.Changes) > 0 && result.DestructiveHandling != "simulated" && result.DestructiveHandling != "excluded" {
				return false
			}
			if result.DestructiveHandling != "" && result.DestructiveHandling != "not_applicable" && result.DestructiveHandling != "simulated" && result.DestructiveHandling != "excluded" {
				return false
			}
			for _, a := range result.Artifacts {
				if a.Name == "" || a.Digest == "" {
					return false
				}
			}
		}
		if len(covered) != len(stepIDs) {
			return false
		}
		decisions := map[string]bool{}
		for _, decision := range scenario.Decisions {
			step, ok := stepIDs[decision.StepID]
			evidence, evidenceOK := outcomes[decision.EvidenceStepID]
			if !ok || decisions[decision.StepID] || step.Decision == nil || decision.Condition != step.Decision.Condition || decision.SelectedStepID == "" || decision.Rationale == "" || !evidenceOK {
				return false
			}
			assertionOK := false
			assertionCount := 0
			for _, assertion := range evidence.Assertions {
				if assertion.Condition != decision.Condition {
					continue
				}
				assertionCount++
				artifactFound := false
				for _, artifact := range evidence.Artifacts {
					if artifact.Digest == assertion.EvidenceDigest {
						artifactFound = true
					}
				}
				if strings.TrimSpace(assertion.EvidenceDigest) == "" || !artifactFound {
					continue
				}
				expected := step.Decision.IfFalseStepID
				if assertion.Met {
					expected = step.Decision.IfTrueStepID
				}
				if expected != "" && decision.SelectedStepID == expected {
					assertionOK = true
				}
			}
			if assertionCount != 1 || !assertionOK {
				return false
			}
			decisions[decision.StepID] = true
		}
		for _, step := range revision.Steps {
			if step.Decision != nil && !decisions[step.ID] {
				return false
			}
		}
	}
	return true
}
func rehearsalStatus(scenarios []Scenario) string {
	for _, s := range scenarios {
		if s.AchievedOutcome != "achieved" {
			return "failed"
		}
		for _, x := range s.Steps {
			if x.Outcome != "passed" || len(x.ManualGaps) > 0 {
				return "failed"
			}
		}
	}
	return "passed"
}
func rehearsalDigest(v Rehearsal) string {
	v.ID = ""
	v.RequestID = ""
	v.RequestDigest = ""
	v.ActorType = ""
	v.ActorID = ""
	v.CreatedAt = time.Time{}
	v.Status = ""
	v.Stale = false
	v.StaleReasons = nil
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
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
func (s *Store) Create(repo, actor, request string, r Revision) (Runbook, error) {
	var out Runbook
	err := s.lock(func() error {
		if request == "" || validate(r) != nil {
			return ErrInvalid
		}
		digest := digest(r)
		id := stableID(repo, actor, request)
		if old, e := s.read(id); e == nil {
			if old.RequestDigest != digest {
				return ErrConflict
			}
			out = old
			return nil
		} else if !errors.Is(e, ErrNotFound) {
			return e
		}
		now := s.now()
		stamp(&r, actor, request, 1, now)
		out = Runbook{ID: id, RepositoryID: repo, RequestID: request, RequestDigest: digest, CurrentVersion: 1, Revisions: []Revision{r}, CreatedAt: now, UpdatedAt: now}
		return s.write(out)
	})
	return project(out), err
}
func (s *Store) Revise(id string, expected int, actor, request string, r Revision) (Runbook, error) {
	var out Runbook
	err := s.lock(func() error {
		v, e := s.read(id)
		if e != nil {
			return e
		}
		d := digest(r)
		for _, x := range v.Revisions {
			if x.RequestID == request {
				if digest(x) != d {
					return ErrConflict
				}
				out = v
				return nil
			}
		}
		if request == "" || v.CurrentVersion != expected {
			return ErrConflict
		}
		if validate(r) != nil {
			return ErrInvalid
		}
		now := s.now()
		stamp(&r, actor, request, expected+1, now)
		v.CurrentVersion++
		v.Revisions = append(v.Revisions, r)
		v.UpdatedAt = now
		out = v
		return s.write(v)
	})
	return project(out), err
}
func (s *Store) Get(id string) (Runbook, error) {
	var v Runbook
	e := s.lock(func() error { var x error; v, x = s.read(id); return x })
	return project(v), e
}
func (s *Store) List(repo string) ([]Runbook, error) {
	out := []Runbook{}
	e := s.lock(func() error {
		xs, e := os.ReadDir(s.root)
		if e != nil {
			return e
		}
		for _, x := range xs {
			if x.IsDir() || !strings.HasSuffix(x.Name(), ".json") {
				continue
			}
			v, e := s.read(strings.TrimSuffix(x.Name(), ".json"))
			if e != nil {
				return e
			}
			if v.RepositoryID == repo {
				out = append(out, project(v))
			}
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, e
}

func validate(r Revision) error {
	kinds := map[string]bool{"service": true, "environment": true, "dependency": true, "signal": true}
	stepKinds := map[string]bool{"diagnostic": true, "action": true, "decision": true, "communication": true}
	refKinds := map[string]bool{"command": true, "workflow_component": true, "documentation": true, "agent": true}
	if strings.TrimSpace(r.Title) == "" || strings.TrimSpace(r.Purpose) == "" || !kinds[r.Scope.Kind] || strings.TrimSpace(r.Scope.ResourceID) == "" || strings.TrimSpace(r.ChangeReason) == "" || len(r.Preconditions) == 0 || len(r.Steps) == 0 || len(r.RollbackCriteria) == 0 || len(r.RequiredSkills) == 0 {
		return ErrInvalid
	}
	ids := map[string]bool{}
	for _, s := range r.Steps {
		if s.ID == "" || ids[s.ID] || s.Position <= 0 || !stepKinds[s.Kind] || s.Title == "" || s.Purpose == "" || s.Instructions == "" || len(s.Preconditions) == 0 || len(s.ExpectedEvidence) == 0 || len(s.RequiredSkills) == 0 {
			return ErrInvalid
		}
		ids[s.ID] = true
		if s.Kind == "decision" && (s.Decision == nil || s.Decision.Condition == "" || s.Decision.HumanJudgment == "") {
			return ErrInvalid
		}
		for _, x := range s.References {
			if !refKinds[x.Kind] || x.ResourceID == "" || x.Revision == "" {
				return ErrInvalid
			}
		}
	}
	for _, s := range r.Steps {
		if s.Decision != nil {
			if s.Decision.IfTrueStepID != "" && !ids[s.Decision.IfTrueStepID] {
				return ErrInvalid
			}
			if s.Decision.IfFalseStepID != "" && !ids[s.Decision.IfFalseStepID] {
				return ErrInvalid
			}
		}
	}
	return nil
}

var secretPattern = regexp.MustCompile(`(?i)(bearer\s+[a-z0-9._~+/=-]{12,}|(?:password|secret|token|api[_-]?key)\s*[:=]\s*\S+|eyJ[a-zA-Z0-9_-]{8,}\.[a-zA-Z0-9_-]{8,}\.[a-zA-Z0-9_-]{8,})`)

func project(v Runbook) Runbook {
	if len(v.Revisions) == 0 {
		return v
	}
	r := v.Revisions[len(v.Revisions)-1]
	if v.UseStatus == "" {
		v.UseStatus = "active"
	}
	for i := range v.Rehearsals {
		v.Rehearsals[i].Stale = v.Rehearsals[i].RunbookVersion != v.CurrentVersion
		v.Rehearsals[i].StaleReasons = nil
		if v.Rehearsals[i].Stale {
			v.Rehearsals[i].StaleReasons = append(v.Rehearsals[i].StaleReasons, "runbook_steps_changed")
		}
		if v.Rehearsals[i].EnvironmentKind == "policy_approved" && !containsString(r.PolicyRevisionIDs, v.Rehearsals[i].PolicyApprovalRevision) {
			v.Rehearsals[i].Stale = true
			v.Rehearsals[i].StaleReasons = append(v.Rehearsals[i].StaleReasons, "policy_changed")
		}
		for _, scenario := range v.Rehearsals[i].Scenarios {
			for _, input := range scenario.Inputs {
				if input.Kind == "credential" && input.ExpiresAt != nil && !input.ExpiresAt.After(time.Now().UTC()) {
					v.Rehearsals[i].Stale = true
					v.Rehearsals[i].StaleReasons = append(v.Rehearsals[i].StaleReasons, "credential_expired")
				}
				if input.Kind == r.Scope.Kind && input.ResourceID == r.Scope.ResourceID && r.Scope.Revision != "" && input.Revision != r.Scope.Revision {
					v.Rehearsals[i].Stale = true
					v.Rehearsals[i].StaleReasons = append(v.Rehearsals[i].StaleReasons, input.Kind+"_changed")
				}
			}
		}
		v.Rehearsals[i].StaleReasons = uniqueStrings(v.Rehearsals[i].StaleReasons)
	}
	d := []Diagnostic{}
	add := func(kind, severity, msg, step, res, actor string) {
		d = append(d, Diagnostic{Kind: kind, Severity: severity, Message: msg, StepID: step, ResourceID: res, AttributedTo: actor})
	}
	if len(r.OwnerIDs) == 0 {
		add("missing_owner", "blocking", "The runbook has no accountable owner.", "", r.Scope.ResourceID, r.CreatedBy)
	}
	criteriaKinds := map[string]bool{}
	for _, criterion := range r.OutcomeCriteria {
		criteriaKinds[criterion.Kind] = strings.TrimSpace(criterion.Criterion) != ""
	}
	for _, kind := range []string{"health", "containment", "recovery", "communication", "rollback"} {
		if !criteriaKinds[kind] {
			add("missing_outcome_criterion", "blocking", "The runbook must declare a "+kind+" outcome criterion.", "", r.Scope.ResourceID, r.CreatedBy)
		}
	}
	if secretPattern.MatchString(string(mustJSON(r))) {
		add("secret_bearing_input", "blocking", "Potential credential material must be removed from the procedure.", "", r.Scope.ResourceID, r.CreatedBy)
	}
	skills := map[string]bool{}
	for _, x := range r.RequiredSkills {
		skills[x] = true
	}
	for _, s := range r.Steps {
		if len(s.OwnerIDs) == 0 {
			add("missing_owner", "blocking", "The step has no accountable owner.", s.ID, "", s.AttributedTo)
		}
		for _, x := range s.RequiredSkills {
			if !skills[x] {
				add("missing_skill", "blocking", "A step requires a skill not declared by the runbook.", s.ID, "", s.AttributedTo)
			}
		}
		if len(s.Assumptions) > 0 && len(s.Preconditions) == 0 {
			add("unsafe_assumption", "blocking", "Assumptions require an explicit precondition.", s.ID, "", s.AttributedTo)
		}
		if len(s.Authority.Changes) > 0 && !s.Authority.HumanApprovalRequired {
			add("unsafe_authority", "blocking", "A changing step lacks an explicit human approval boundary.", s.ID, "", s.AttributedTo)
		}
		if len(s.PolicyRuleIDs) > 0 && len(s.Authority.ProhibitedActions) == 0 {
			add("conflicting_policy", "blocking", "A policy-bound step does not retain prohibited actions.", s.ID, "", s.AttributedTo)
		}
		for _, x := range s.References {
			if !x.Accessible {
				add("inaccessible_resource", "blocking", "A referenced resource is not accessible.", s.ID, x.ResourceID, s.AttributedTo)
			}
			if (x.Kind == "command" || x.Kind == "workflow_component") && !x.Reviewed {
				add("unreviewed_execution", "blocking", "Executable material must reference a reviewed revision.", s.ID, x.ResourceID, s.AttributedTo)
			}
			if x.Kind == "agent" && !x.Approved {
				add("unapproved_agent", "blocking", "The referenced agent is not approved.", s.ID, x.ResourceID, s.AttributedTo)
			}
		}
	}
	v.Diagnostics = d
	return v
}
func containsString(xs []string, value string) bool {
	for _, x := range xs {
		if x == value {
			return true
		}
	}
	return false
}
func uniqueStrings(xs []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}
func stamp(r *Revision, actor, request string, version int, now time.Time) {
	r.RequestID = request
	r.Version = version
	r.CreatedBy = actor
	r.CreatedAt = now
	for i := range r.Steps {
		r.Steps[i].AttributedTo = actor
	}
}
func digest(r Revision) string {
	r.RequestID = ""
	r.Version = 0
	r.CreatedBy = ""
	r.CreatedAt = time.Time{}
	for i := range r.Steps {
		r.Steps[i].AttributedTo = ""
	}
	b, _ := json.Marshal(r)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func stableID(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "rb-" + hex.EncodeToString(h[:12])
}
func mustJSON(v any) []byte { b, _ := json.Marshal(v); return b }
func (s *Store) read(id string) (Runbook, error) {
	var v Runbook
	b, e := os.ReadFile(filepath.Join(s.root, id+".json"))
	if os.IsNotExist(e) {
		return v, ErrNotFound
	}
	if e != nil {
		return v, e
	}
	e = json.Unmarshal(b, &v)
	return v, e
}
func (s *Store) write(v Runbook) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(s.root, ".runbook-")
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
	if e == nil {
		e = os.Rename(name, filepath.Join(s.root, v.ID+".json"))
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
