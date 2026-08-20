// Package proposals stores repository-scoped ideas and their attributable discussion.
package proposals

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	ErrNotFound               = errors.New("proposal not found")
	ErrInvalid                = errors.New("invalid proposal")
	ErrDurabilityUncertain    = errors.New("proposal mutation is visible but durability is uncertain")
	ErrTaskAssignmentConflict = errors.New("task assignment changed")
	ErrCorrectiveConflict     = errors.New("corrective work operation changed")
	ErrImplementationConflict = errors.New("implementation plan changed")
)

const (
	Open           = "open"
	Closed         = "closed"
	TaskTodo       = "todo"
	TaskInProgress = "in_progress"
	TaskCompleted  = "completed"
	TaskCancelled  = "cancelled"
)

type Proposal struct {
	ID           string           `json:"id"`
	RepositoryID string           `json:"repository_id"`
	AuthorID     string           `json:"author_id"`
	Title        string           `json:"title"`
	Body         string           `json:"body"`
	Status       string           `json:"status"`
	CreatedAt    time.Time        `json:"created_at"`
	UpdatedAt    time.Time        `json:"updated_at"`
	ClosedAt     *time.Time       `json:"closed_at"`
	Reasoning    *ReasoningOrigin `json:"reasoning,omitempty"`
}

// ReasoningOrigin is an immutable, revision-exact handoff from collaborative
// investigation and impact analysis into implementation and review.
type ReasoningOrigin struct {
	SecurityFindingID              string                     `json:"security_finding_id,omitempty"`
	SecurityFindingVersion         int                        `json:"security_finding_version,omitempty"`
	ThreatModelID                  string                     `json:"threat_model_id,omitempty"`
	ThreatModelVersion             int                        `json:"threat_model_version,omitempty"`
	ExploratorySessionID           string                     `json:"exploratory_session_id,omitempty"`
	ExploratoryFindingID           string                     `json:"exploratory_finding_id,omitempty"`
	ExploratoryRepairID            string                     `json:"exploratory_repair_id,omitempty"`
	DebuggingWorkspaceID           string                     `json:"debugging_workspace_id,omitempty"`
	DebuggingRepairWorkID          string                     `json:"debugging_repair_work_id,omitempty"`
	DebuggingScenarioID            string                     `json:"debugging_scenario_id,omitempty"`
	DebuggingCauseClaimID          string                     `json:"debugging_cause_claim_id,omitempty"`
	SupportThreadID                string                     `json:"support_thread_id,omitempty"`
	SupportThreadVersion           int                        `json:"support_thread_version,omitempty"`
	DesignProposalID               string                     `json:"design_proposal_id,omitempty"`
	DesignProposalVersion          int                        `json:"design_proposal_version,omitempty"`
	RecoveryExerciseID             string                     `json:"recovery_exercise_id,omitempty"`
	RecoveryInvestigationID        string                     `json:"recovery_investigation_id,omitempty"`
	RecoveryFindingID              string                     `json:"recovery_finding_id,omitempty"`
	ReliabilityContractID          string                     `json:"reliability_contract_id,omitempty"`
	ReliabilityInvestigationID     string                     `json:"reliability_investigation_id,omitempty"`
	ReliabilityFindingID           string                     `json:"reliability_finding_id,omitempty"`
	ReliabilityImpactID            string                     `json:"reliability_impact_id,omitempty"`
	DataObservationID              string                     `json:"data_observation_id,omitempty"`
	DataObservationVersion         int                        `json:"data_observation_version,omitempty"`
	GovernanceProposalID           string                     `json:"governance_proposal_id,omitempty"`
	GovernanceReceiptID            string                     `json:"governance_receipt_id,omitempty"`
	IssueID                        string                     `json:"issue_id,omitempty"`
	IssueVersion                   int                        `json:"issue_version,omitempty"`
	ReproductionID                 string                     `json:"reproduction_attempt_id,omitempty"`
	DecisionID                     string                     `json:"decision_id,omitempty"`
	CommitmentVersion              int                        `json:"commitment_version,omitempty"`
	AssessmentID                   string                     `json:"assessment_id"`
	AssessmentVersion              int                        `json:"assessment_version"`
	AccessibilityFindingID         string                     `json:"accessibility_finding_id,omitempty"`
	AccessibilityCommitmentID      string                     `json:"accessibility_commitment_id,omitempty"`
	AccessibilityCommitmentVersion int                        `json:"accessibility_commitment_version,omitempty"`
	Revision                       string                     `json:"revision"`
	ExplanationID                  string                     `json:"explanation_id,omitempty"`
	ConclusionEntryID              string                     `json:"conclusion_entry_id,omitempty"`
	SelectedItemIDs                []string                   `json:"selected_item_ids"`
	Items                          []ReasoningItem            `json:"items"`
	Acknowledgements               []ReasoningAcknowledgement `json:"acknowledgements,omitempty"`
	AnalysisStatus                 string                     `json:"analysis_status"`
	OrganizationID                 string                     `json:"organization_id,omitempty"`
	MandateID                      string                     `json:"mandate_id,omitempty"`
	OpportunityID                  string                     `json:"opportunity_id,omitempty"`
	RoadmapItemID                  string                     `json:"roadmap_item_id,omitempty"`
	RoadmapVersion                 int                        `json:"roadmap_version,omitempty"`
}

type ReasoningItem struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Summary string `json:"summary"`
	Status  string `json:"status"`
}
type ReasoningAcknowledgement struct {
	RequestID      string `json:"request_id"`
	RepositoryID   string `json:"repository_id"`
	OwnerID        string `json:"owner_id"`
	AcknowledgedBy string `json:"acknowledged_by"`
	Note           string `json:"note,omitempty"`
}

type Comment struct {
	ID         string    `json:"id"`
	ProposalID string    `json:"proposal_id"`
	AuthorID   string    `json:"author_id"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
}

type Task struct {
	ID                   string             `json:"id"`
	ProposalID           string             `json:"proposal_id"`
	Title                string             `json:"title"`
	Outcome              string             `json:"outcome"`
	Risk                 string             `json:"risk,omitempty"`
	VerificationPlan     string             `json:"verification_plan,omitempty"`
	Status               string             `json:"status"`
	Position             int                `json:"position"`
	DependencyIDs        []string           `json:"dependency_ids"`
	DiscussionCommentIDs []string           `json:"discussion_comment_ids"`
	Ready                bool               `json:"ready"`
	BlockedBy            []string           `json:"blocked_by"`
	ContextRevision      int                `json:"context_revision"`
	ContextState         string             `json:"context_state"`
	CreatedBy            string             `json:"created_by"`
	UpdatedBy            string             `json:"updated_by"`
	CreatedAt            time.Time          `json:"created_at"`
	UpdatedAt            time.Time          `json:"updated_at"`
	Assignment           *TaskAssignment    `json:"assignment,omitempty"`
	Contribution         *TaskContribution  `json:"contribution,omitempty"`
	Contributions        []TaskContribution `json:"contributions,omitempty"`
	Reasoning            *ReasoningOrigin   `json:"reasoning,omitempty"`
}

type ImplementationTaskInput struct {
	Title, Outcome, Risk, VerificationPlan, AssigneeType, AssigneeID string
	DependsOnPrevious                                                bool
}

type ImplementationInput struct {
	RepositoryID, ActorID, Title, Body string
	Origin                             ReasoningOrigin
	Tasks                              []ImplementationTaskInput
}

// TaskContribution is the bidirectional review handoff. Status follows the
// candidate rather than declaring the planned outcome complete before merge.
type TaskContribution struct {
	PullRequestID   string   `json:"pull_request_id"`
	SessionID       string   `json:"session_id,omitempty"`
	RunID           string   `json:"run_id,omitempty"`
	SourceCommitID  string   `json:"source_commit_id"`
	CommitIDs       []string `json:"commit_ids"`
	Status          string   `json:"status"`
	ContextRevision int      `json:"context_revision"`
}

type TaskAccess struct {
	RepositoryID string   `json:"repository_id"`
	BaseRevision string   `json:"base_revision"`
	Scopes       []string `json:"scopes"`
	Branch       string   `json:"branch"`
}

type TaskAssignment struct {
	ID              string     `json:"id"`
	AssigneeType    string     `json:"assignee_type"`
	AssigneeID      string     `json:"assignee_id"`
	Mandate         string     `json:"mandate"`
	Access          TaskAccess `json:"access"`
	AssignedBy      string     `json:"assigned_by"`
	AssignedAt      time.Time  `json:"assigned_at"`
	ContextRevision int        `json:"context_revision"`
}

type TaskChange struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id"`
	ActorID   string    `json:"actor_id"`
	Action    string    `json:"action"`
	Task      Task      `json:"task"`
	CreatedAt time.Time `json:"created_at"`
}

type TaskPatch struct {
	Title                *string
	Outcome              *string
	Status               *string
	Position             *int
	DependencyIDs        *[]string
	DiscussionCommentIDs *[]string
}

type TaskAssignmentInput struct {
	AssigneeType         string
	AssigneeID           string
	Mandate              string
	RepositoryID         string
	BaseRevision         string
	ExpectedAssignmentID string
}

type TaskRebaseInput struct {
	BaseRevision         string
	ExpectedAssignmentID string
}

type CorrectiveWorkInput struct {
	IncidentID    string
	OperationID   string
	RepositoryID  string
	ActorID       string
	ProposalTitle string
	ProposalBody  string
	TaskTitle     string
	Outcome       string
	AssigneeID    string
	BaseRevision  string
	DueAt         time.Time
}

type CorrectiveOrigin struct {
	IncidentID   string    `json:"incident_id"`
	OperationID  string    `json:"operation_id"`
	ActorID      string    `json:"actor_id"`
	AssigneeID   string    `json:"assignee_id"`
	BaseRevision string    `json:"base_revision"`
	DueAt        time.Time `json:"due_at"`
}

// WithStartableAgentTask serializes task-session publication with proposal and
// task mutations. The callback receives snapshots from the same locked record
// that proved the exact assignment is still startable.
func (s *Store) WithStartableAgentTask(repositoryID, proposalID, taskID, assignmentID string, fn func(Proposal, Task, []Task, []Comment) error) error {
	if !validID(repositoryID) || !validID(proposalID) || !validID(taskID) || !validID(assignmentID) || fn == nil {
		return ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()
	r, err := s.read(proposalID)
	if err != nil || r.Proposal.RepositoryID != repositoryID {
		return ErrNotFound
	}
	deriveTasks(r.Tasks)
	for _, task := range r.Tasks {
		if task.ID != taskID {
			continue
		}
		if r.Proposal.Status != Open || task.Status != TaskTodo || !task.Ready || task.Assignment == nil || task.Assignment.AssigneeType != "agent" || task.Assignment.ID != assignmentID || effectiveContextRevision(task.Assignment.ContextRevision) != effectiveContextRevision(task.ContextRevision) {
			return ErrTaskAssignmentConflict
		}
		return fn(r.Proposal, task, append([]Task(nil), r.Tasks...), append([]Comment(nil), r.Comments...))
	}
	return ErrNotFound
}

type Patch struct {
	Title  *string
	Body   *string
	Status *string
}

type record struct {
	Proposal    Proposal          `json:"proposal"`
	Comments    []Comment         `json:"comments,omitempty"`
	Tasks       []Task            `json:"tasks,omitempty"`
	TaskChanges []TaskChange      `json:"task_changes,omitempty"`
	Corrective  *CorrectiveOrigin `json:"corrective_origin,omitempty"`
}

// CreateCorrectiveWork atomically publishes a proposal, its first task, and
// human assignment. Incident/operation identity makes a retry return the same
// resources even when a later incident-store link could not be acknowledged.
func (s *Store) CreateCorrectiveWork(input CorrectiveWorkInput) (Proposal, Task, error) {
	title, body, err := validateContent(input.ProposalTitle, input.ProposalBody)
	if err != nil {
		return Proposal{}, Task{}, err
	}
	taskTitle, outcome, err := validateTaskContent(input.TaskTitle, input.Outcome)
	if err != nil {
		return Proposal{}, Task{}, err
	}
	input.BaseRevision = strings.ToLower(strings.TrimSpace(input.BaseRevision))
	if !validID(input.IncidentID) || !validID(input.OperationID) || !validID(input.RepositoryID) || !validID(input.ActorID) || !validID(input.AssigneeID) || len(input.BaseRevision) != 40 || input.DueAt.IsZero() {
		return Proposal{}, Task{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Proposal{}, Task{}, err
	}
	defer unlock()
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return Proposal{}, Task{}, err
	}
	for _, entry := range entries {
		id, ok := strings.CutSuffix(entry.Name(), ".json")
		if entry.IsDir() || !ok || !validID(id) {
			continue
		}
		r, readErr := s.read(id)
		if readErr != nil {
			return Proposal{}, Task{}, readErr
		}
		if r.Corrective == nil || r.Corrective.IncidentID != input.IncidentID || r.Corrective.OperationID != input.OperationID {
			continue
		}
		origin := r.Corrective
		if r.Proposal.RepositoryID != input.RepositoryID || r.Proposal.AuthorID != input.ActorID || r.Proposal.Title != title || r.Proposal.Body != body || len(r.Tasks) != 1 || r.Tasks[0].Title != taskTitle || r.Tasks[0].Outcome != outcome || origin.ActorID != input.ActorID || origin.AssigneeID != input.AssigneeID || origin.BaseRevision != input.BaseRevision || !origin.DueAt.Equal(input.DueAt) {
			return Proposal{}, Task{}, ErrCorrectiveConflict
		}
		return r.Proposal, r.Tasks[0], nil
	}
	proposalID, err := newID()
	if err != nil {
		return Proposal{}, Task{}, err
	}
	taskID, err := newID()
	if err != nil {
		return Proposal{}, Task{}, err
	}
	assignmentID, err := newID()
	if err != nil {
		return Proposal{}, Task{}, err
	}
	now := s.now().Truncate(time.Microsecond)
	p := Proposal{ID: proposalID, RepositoryID: input.RepositoryID, AuthorID: input.ActorID, Title: title, Body: body, Status: Open, CreatedAt: now, UpdatedAt: now}
	task := Task{ID: taskID, ProposalID: proposalID, Title: taskTitle, Outcome: outcome, Status: TaskTodo, Position: 0, ContextRevision: 1, ContextState: "current", Ready: true, CreatedBy: input.ActorID, UpdatedBy: input.ActorID, CreatedAt: now, UpdatedAt: now}
	task.Assignment = &TaskAssignment{ID: assignmentID, AssigneeType: "human", AssigneeID: input.AssigneeID, Mandate: outcome, Access: TaskAccess{RepositoryID: input.RepositoryID, BaseRevision: input.BaseRevision, Scopes: []string{}, Branch: "no new access; existing collaborator authority only"}, AssignedBy: input.ActorID, AssignedAt: now, ContextRevision: 1}
	created, err := newTaskChange(Task{ID: task.ID, ProposalID: task.ProposalID, Title: task.Title, Outcome: task.Outcome, Status: task.Status, Position: task.Position, ContextRevision: 1, ContextState: "current", Ready: true, CreatedBy: task.CreatedBy, UpdatedBy: task.UpdatedBy, CreatedAt: now, UpdatedAt: now}, input.ActorID, "created", now)
	if err != nil {
		return Proposal{}, Task{}, err
	}
	assigned, err := newTaskChange(task, input.ActorID, "assigned", now)
	if err != nil {
		return Proposal{}, Task{}, err
	}
	r := record{Proposal: p, Tasks: []Task{task}, TaskChanges: []TaskChange{created, assigned}, Corrective: &CorrectiveOrigin{IncidentID: input.IncidentID, OperationID: input.OperationID, ActorID: input.ActorID, AssigneeID: input.AssigneeID, BaseRevision: input.BaseRevision, DueAt: input.DueAt}}
	if committed, writeErr := s.write(r); writeErr != nil {
		if committed {
			return p, task, fmt.Errorf("%w: %v", ErrDurabilityUncertain, writeErr)
		}
		return Proposal{}, Task{}, writeErr
	}
	return p, task, nil
}

type Store struct {
	root          string
	mu            sync.Mutex
	now           func() time.Time
	directorySync func(string) error
	readFile      func(string) ([]byte, error)
}

func New(root string) (*Store, error) {
	if root == "" {
		return nil, errors.New("proposal storage root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, fmt.Errorf("create proposal store: %w", err)
	}
	return &Store{root: abs, now: func() time.Time { return time.Now().UTC() }, directorySync: syncDirectory, readFile: os.ReadFile}, nil
}

// CreateImplementation atomically creates an ordered, owned plan from one
// frozen reasoning snapshot. The source-specific recovery identity makes
// retries converge.
func (s *Store) CreateImplementation(input ImplementationInput) (Proposal, []Task, error) {
	isAccessibility := validID(input.Origin.AssessmentID) && input.Origin.AssessmentVersion > 0 && validID(input.Origin.AccessibilityFindingID) && validID(input.Origin.AccessibilityCommitmentID) && input.Origin.AccessibilityCommitmentVersion > 0
	isAssessment := validID(input.Origin.AssessmentID) && input.Origin.AssessmentVersion > 0 && !isAccessibility
	isDecision := validID(input.Origin.DecisionID) && input.Origin.CommitmentVersion > 0
	isIssue := validID(input.Origin.IssueID) && input.Origin.IssueVersion > 0 && validID(input.Origin.ReproductionID)
	isGovernance := validID(input.Origin.GovernanceProposalID) && validID(input.Origin.GovernanceReceiptID)
	isRoadmap := strings.TrimSpace(input.Origin.RoadmapItemID) != "" && input.Origin.RoadmapVersion > 0 && strings.TrimSpace(input.Origin.OpportunityID) != ""
	isDataObservation := validID(input.Origin.DataObservationID) && input.Origin.DataObservationVersion > 0
	isReliability := validReliabilityReference(input.Origin.ReliabilityContractID) && (validReliabilityReference(input.Origin.ReliabilityFindingID) != validReliabilityReference(input.Origin.ReliabilityImpactID))
	isRecovery := validID(input.Origin.RecoveryExerciseID) && validID(input.Origin.RecoveryInvestigationID) && validID(input.Origin.RecoveryFindingID)
	isSupport := validID(input.Origin.SupportThreadID) && input.Origin.SupportThreadVersion > 0
	isDesign := validID(input.Origin.DesignProposalID) && input.Origin.DesignProposalVersion > 0
	isDebugging := validID(input.Origin.DebuggingWorkspaceID) && validID(input.Origin.DebuggingRepairWorkID) && validID(input.Origin.DebuggingScenarioID) && validID(input.Origin.DebuggingCauseClaimID)
	isExploratory := validID(input.Origin.ExploratorySessionID) && strings.TrimSpace(input.Origin.ExploratoryFindingID) != "" && validID(input.Origin.ExploratoryRepairID)
	isSecurityFinding := validID(input.Origin.SecurityFindingID) && input.Origin.SecurityFindingVersion > 0 && validThreatModelReference(input.Origin.ThreatModelID) && input.Origin.ThreatModelVersion > 0
	originCount := 0
	for _, present := range []bool{isAssessment, isAccessibility, isDecision, isIssue, isGovernance, isRoadmap, isDataObservation, isReliability, isRecovery, isSupport, isDebugging, isDesign, isExploratory, isSecurityFinding} {
		if present {
			originCount++
		}
	}
	if !validID(input.RepositoryID) || !validID(input.ActorID) || originCount != 1 || len(input.Origin.Revision) != 40 || len(input.Tasks) == 0 || len(input.Tasks) > 20 || len(input.Origin.Items) == 0 {
		return Proposal{}, nil, ErrInvalid
	}
	title, body, err := validateContent(input.Title, input.Body)
	if err != nil {
		return Proposal{}, nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Proposal{}, nil, err
	}
	defer unlock()
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return Proposal{}, nil, err
	}
	for _, entry := range entries {
		id, ok := strings.CutSuffix(entry.Name(), ".json")
		if entry.IsDir() || !ok || !validID(id) {
			continue
		}
		r, readErr := s.read(id)
		if readErr != nil {
			return Proposal{}, nil, readErr
		}
		if isDebugging && r.Proposal.RepositoryID == input.RepositoryID && r.Proposal.Reasoning != nil && r.Proposal.Reasoning.DebuggingRepairWorkID == input.Origin.DebuggingRepairWorkID {
			if !reflect.DeepEqual(*r.Proposal.Reasoning, input.Origin) || r.Proposal.Title != title || r.Proposal.Body != body || len(r.Tasks) != len(input.Tasks) {
				return Proposal{}, nil, ErrImplementationConflict
			}
			return r.Proposal, append([]Task(nil), r.Tasks...), nil
		}
		if isExploratory && r.Proposal.RepositoryID == input.RepositoryID && r.Proposal.Reasoning != nil && r.Proposal.Reasoning.ExploratoryRepairID == input.Origin.ExploratoryRepairID {
			if !reflect.DeepEqual(*r.Proposal.Reasoning, input.Origin) || r.Proposal.Title != title || r.Proposal.Body != body || len(r.Tasks) != len(input.Tasks) {
				return Proposal{}, nil, ErrImplementationConflict
			}
			return r.Proposal, append([]Task(nil), r.Tasks...), nil
		}
		if isSecurityFinding && r.Proposal.RepositoryID == input.RepositoryID && r.Proposal.Reasoning != nil && r.Proposal.Reasoning.SecurityFindingID == input.Origin.SecurityFindingID {
			if !reflect.DeepEqual(*r.Proposal.Reasoning, input.Origin) || r.Proposal.Title != title || r.Proposal.Body != body || len(r.Tasks) != len(input.Tasks) {
				return Proposal{}, nil, ErrImplementationConflict
			}
			for i, task := range r.Tasks {
				value := input.Tasks[i]
				if task.Title != strings.TrimSpace(value.Title) || task.Outcome != strings.TrimSpace(value.Outcome) || task.Risk != strings.TrimSpace(value.Risk) || task.VerificationPlan != strings.TrimSpace(value.VerificationPlan) || task.Assignment == nil || task.Assignment.AssigneeType != value.AssigneeType || (value.AssigneeID != "" && task.Assignment.AssigneeID != value.AssigneeID) || (i > 0 && value.DependsOnPrevious != (len(task.DependencyIDs) == 1 && task.DependencyIDs[0] == r.Tasks[i-1].ID)) {
					return Proposal{}, nil, ErrImplementationConflict
				}
			}
			return r.Proposal, append([]Task(nil), r.Tasks...), nil
		}
		if r.Proposal.RepositoryID == input.RepositoryID && r.Proposal.Reasoning != nil && ((isAccessibility && r.Proposal.Reasoning.AssessmentID == input.Origin.AssessmentID && r.Proposal.Reasoning.AccessibilityFindingID == input.Origin.AccessibilityFindingID) || (isAssessment && r.Proposal.Reasoning.AssessmentID == input.Origin.AssessmentID) || (isDecision && r.Proposal.Reasoning.DecisionID == input.Origin.DecisionID && r.Proposal.Reasoning.CommitmentVersion == input.Origin.CommitmentVersion) || (isIssue && r.Proposal.Reasoning.IssueID == input.Origin.IssueID && r.Proposal.Reasoning.ReproductionID == input.Origin.ReproductionID) || (isGovernance && r.Proposal.Reasoning.GovernanceProposalID == input.Origin.GovernanceProposalID) || (isRoadmap && r.Proposal.Reasoning.RoadmapItemID == input.Origin.RoadmapItemID && r.Proposal.Reasoning.RoadmapVersion == input.Origin.RoadmapVersion) || (isDataObservation && r.Proposal.Reasoning.DataObservationID == input.Origin.DataObservationID) || (isReliability && r.Proposal.Reasoning.ReliabilityContractID == input.Origin.ReliabilityContractID && r.Proposal.Reasoning.ReliabilityFindingID == input.Origin.ReliabilityFindingID && r.Proposal.Reasoning.ReliabilityImpactID == input.Origin.ReliabilityImpactID) || (isRecovery && r.Proposal.Reasoning.RecoveryExerciseID == input.Origin.RecoveryExerciseID && r.Proposal.Reasoning.RecoveryFindingID == input.Origin.RecoveryFindingID) || (isSupport && r.Proposal.Reasoning.SupportThreadID == input.Origin.SupportThreadID) || (isDesign && r.Proposal.Reasoning.DesignProposalID == input.Origin.DesignProposalID)) {
			if ((isAccessibility || isIssue || isGovernance || isRoadmap || isReliability || isRecovery || isSupport || isDesign) && !reflect.DeepEqual(*r.Proposal.Reasoning, input.Origin)) || r.Proposal.Title != title || r.Proposal.Body != body || len(r.Tasks) != len(input.Tasks) {
				return Proposal{}, nil, ErrImplementationConflict
			}
			for i, task := range r.Tasks {
				value := input.Tasks[i]
				if task.Title != strings.TrimSpace(value.Title) || task.Outcome != strings.TrimSpace(value.Outcome) || task.Risk != strings.TrimSpace(value.Risk) || task.VerificationPlan != strings.TrimSpace(value.VerificationPlan) || task.Assignment == nil || task.Assignment.AssigneeType != value.AssigneeType || (value.AssigneeID != "" && task.Assignment.AssigneeID != value.AssigneeID) || (i > 0 && value.DependsOnPrevious != (len(task.DependencyIDs) == 1 && task.DependencyIDs[0] == r.Tasks[i-1].ID)) {
					return Proposal{}, nil, ErrImplementationConflict
				}
			}
			return r.Proposal, append([]Task(nil), r.Tasks...), nil
		}
	}
	proposalID, err := newID()
	if err != nil {
		return Proposal{}, nil, err
	}
	now := s.now().Truncate(time.Microsecond)
	origin := input.Origin
	origin.SelectedItemIDs = cloneStrings(origin.SelectedItemIDs)
	origin.Items = append([]ReasoningItem(nil), origin.Items...)
	origin.Acknowledgements = append([]ReasoningAcknowledgement(nil), origin.Acknowledgements...)
	p := Proposal{ID: proposalID, RepositoryID: input.RepositoryID, AuthorID: input.ActorID, Title: title, Body: body, Status: Open, CreatedAt: now, UpdatedAt: now, Reasoning: &origin}
	tasks := make([]Task, 0, len(input.Tasks))
	changes := make([]TaskChange, 0, len(input.Tasks)*2)
	for index, value := range input.Tasks {
		taskTitle, outcome, validationErr := validateTaskContent(value.Title, value.Outcome)
		if validationErr != nil || len(value.Risk) > 4000 || len(value.VerificationPlan) > 4000 || (value.AssigneeType != "human" && value.AssigneeType != "agent") {
			return Proposal{}, nil, ErrInvalid
		}
		assignee := value.AssigneeID
		if value.AssigneeType == "agent" && assignee == "" {
			assignee, err = newID()
		}
		if err != nil || !validID(assignee) {
			return Proposal{}, nil, ErrInvalid
		}
		taskID, idErr := newID()
		if idErr != nil {
			return Proposal{}, nil, idErr
		}
		assignmentID, idErr := newID()
		if idErr != nil {
			return Proposal{}, nil, idErr
		}
		dependencies := []string{}
		if value.DependsOnPrevious && index > 0 {
			dependencies = []string{tasks[index-1].ID}
		}
		scopes, branch := []string{}, "no new access; existing collaborator authority only"
		if value.AssigneeType == "agent" {
			scopes, branch = []string{"git:read", "git:write"}, "task-scoped branch (created when work starts)"
		}
		taskOrigin := origin
		task := Task{ID: taskID, ProposalID: proposalID, Title: taskTitle, Outcome: outcome, Risk: strings.TrimSpace(value.Risk), VerificationPlan: strings.TrimSpace(value.VerificationPlan), Status: TaskTodo, Position: index, DependencyIDs: dependencies, ContextRevision: 1, ContextState: "current", CreatedBy: input.ActorID, UpdatedBy: input.ActorID, CreatedAt: now, UpdatedAt: now, Reasoning: &taskOrigin}
		task.Assignment = &TaskAssignment{ID: assignmentID, AssigneeType: value.AssigneeType, AssigneeID: assignee, Mandate: outcome, Access: TaskAccess{RepositoryID: input.RepositoryID, BaseRevision: origin.Revision, Scopes: scopes, Branch: branch}, AssignedBy: input.ActorID, AssignedAt: now, ContextRevision: 1}
		tasks = append(tasks, task)
		created, _ := newTaskChange(Task{ID: task.ID, ProposalID: proposalID, Title: task.Title, Outcome: task.Outcome, Risk: task.Risk, VerificationPlan: task.VerificationPlan, Status: task.Status, Position: index, DependencyIDs: dependencies, ContextRevision: 1, ContextState: "current", Ready: index == 0, CreatedBy: input.ActorID, UpdatedBy: input.ActorID, CreatedAt: now, UpdatedAt: now, Reasoning: &taskOrigin}, input.ActorID, "created", now)
		assigned, _ := newTaskChange(task, input.ActorID, "assigned", now)
		changes = append(changes, created, assigned)
	}
	deriveTasks(tasks)
	r := record{Proposal: p, Tasks: tasks, TaskChanges: changes}
	if committed, writeErr := s.write(r); writeErr != nil {
		if committed {
			return p, tasks, fmt.Errorf("%w: %v", ErrDurabilityUncertain, writeErr)
		}
		return Proposal{}, nil, writeErr
	}
	return p, tasks, nil
}

// Reliability records use 12-byte opaque IDs rather than proposal-sized IDs.
// Admit only their canonical lowercase hexadecimal representation so retries
// cannot create a second origin by padding or otherwise re-encoding an ID.
func validReliabilityReference(value string) bool {
	if len(value) != 24 || value != strings.ToLower(value) {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 12
}

// Threat models use the same canonical 12-byte opaque identity shape.
func validThreatModelReference(value string) bool {
	return validReliabilityReference(value)
}

func (s *Store) Create(repositoryID, authorID, title, body string) (Proposal, error) {
	if !validID(repositoryID) || !validID(authorID) {
		return Proposal{}, ErrInvalid
	}
	title, body, err := validateContent(title, body)
	if err != nil {
		return Proposal{}, err
	}
	id, err := newID()
	if err != nil {
		return Proposal{}, err
	}
	now := s.now().Truncate(time.Microsecond)
	p := Proposal{ID: id, RepositoryID: repositoryID, AuthorID: authorID, Title: title, Body: body, Status: Open, CreatedAt: now, UpdatedAt: now}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Proposal{}, err
	}
	defer unlock()
	desired := record{Proposal: p}
	if committed, err := s.write(desired); err != nil {
		if committed {
			return p, fmt.Errorf("%w: %v", ErrDurabilityUncertain, err)
		}
		return Proposal{}, err
	}
	return p, nil
}

// DeleteMigrationWork compensates a failed cross-store evolution publication.
// Empty task/assignment IDs describe the exact earlier publication stages. It
// refuses once any additional discussion, task, assignment, or contribution
// could be lost.
func (s *Store) DeleteMigrationWork(repositoryID, proposalID, taskID, assignmentID string) error {
	if !validID(repositoryID) || !validID(proposalID) || (taskID != "" && !validID(taskID)) || (assignmentID != "" && !validID(assignmentID)) || (taskID == "" && assignmentID != "") {
		return ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()
	r, err := s.read(proposalID)
	if err != nil {
		return err
	}
	if r.Proposal.RepositoryID != repositoryID || len(r.Comments) != 0 {
		return ErrInvalid
	}
	if taskID == "" {
		if len(r.Tasks) != 0 || len(r.TaskChanges) != 0 {
			return ErrInvalid
		}
	} else {
		if len(r.Tasks) != 1 || r.Tasks[0].ID != taskID || r.Tasks[0].Contribution != nil || len(r.TaskChanges) == 0 || r.TaskChanges[0].Action != "created" {
			return ErrInvalid
		}
		if assignmentID == "" {
			if r.Tasks[0].Assignment != nil || len(r.TaskChanges) != 1 {
				return ErrInvalid
			}
		} else if r.Tasks[0].Assignment == nil || r.Tasks[0].Assignment.ID != assignmentID || len(r.TaskChanges) != 2 || r.TaskChanges[1].Action != "assigned" {
			return ErrInvalid
		}
	}
	if err = os.Remove(s.path(proposalID)); err != nil {
		return err
	}
	return s.directorySync(s.root)
}

func (s *Store) Get(repositoryID, id string) (Proposal, error) {
	r, err := s.read(id)
	if err != nil {
		return Proposal{}, err
	}
	if r.Proposal.RepositoryID != repositoryID {
		return Proposal{}, ErrNotFound
	}
	return r.Proposal, nil
}

func (s *Store) List(repositoryID string) ([]Proposal, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	result := []Proposal{}
	for _, entry := range entries {
		id, ok := strings.CutSuffix(entry.Name(), ".json")
		if entry.IsDir() || !ok || !validID(id) {
			continue
		}
		r, err := s.read(id)
		if err != nil {
			return nil, err
		}
		if r.Proposal.RepositoryID == repositoryID {
			result = append(result, r.Proposal)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result, nil
}

func (s *Store) Update(repositoryID, id string, patch Patch) (Proposal, error) {
	if patch.Title == nil && patch.Body == nil && patch.Status == nil {
		return Proposal{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Proposal{}, err
	}
	defer unlock()
	r, err := s.read(id)
	if err != nil || r.Proposal.RepositoryID != repositoryID {
		return Proposal{}, ErrNotFound
	}
	p := r.Proposal
	if patch.Title != nil {
		title, _, e := validateContent(*patch.Title, p.Body)
		if e != nil {
			return Proposal{}, e
		}
		p.Title = title
	}
	if patch.Body != nil {
		_, body, e := validateContent(p.Title, *patch.Body)
		if e != nil {
			return Proposal{}, e
		}
		p.Body = body
	}
	if patch.Status != nil {
		if *patch.Status != Closed || p.Status == Closed {
			return Proposal{}, ErrInvalid
		}
		closed := s.now().Truncate(time.Microsecond)
		p.Status, p.ClosedAt = Closed, &closed
	}
	p.UpdatedAt = s.now().Truncate(time.Microsecond)
	r.Proposal = p
	if committed, err := s.write(r); err != nil {
		if committed {
			return p, fmt.Errorf("%w: %v", ErrDurabilityUncertain, err)
		}
		return Proposal{}, err
	}
	return p, nil
}

func (s *Store) AddComment(repositoryID, proposalID, authorID, body string) (Comment, error) {
	if !validID(repositoryID) || !validID(authorID) {
		return Comment{}, ErrInvalid
	}
	body = strings.TrimSpace(body)
	if body == "" || len([]rune(body)) > 10000 {
		return Comment{}, ErrInvalid
	}
	id, err := newID()
	if err != nil {
		return Comment{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Comment{}, err
	}
	defer unlock()
	r, err := s.read(proposalID)
	if err != nil || r.Proposal.RepositoryID != repositoryID {
		return Comment{}, ErrNotFound
	}
	c := Comment{ID: id, ProposalID: proposalID, AuthorID: authorID, Body: body, CreatedAt: s.now().Truncate(time.Microsecond)}
	r.Comments = append(r.Comments, c)
	if committed, err := s.write(r); err != nil {
		if committed {
			return c, fmt.Errorf("%w: %v", ErrDurabilityUncertain, err)
		}
		return Comment{}, err
	}
	return c, nil
}

func (s *Store) ListComments(repositoryID, proposalID string) ([]Comment, error) {
	r, err := s.read(proposalID)
	if err != nil || r.Proposal.RepositoryID != repositoryID {
		return nil, ErrNotFound
	}
	return append([]Comment(nil), r.Comments...), nil
}

func (s *Store) CreateTask(repositoryID, proposalID, actorID, title, outcome string, dependencyIDs, commentIDs []string) (Task, error) {
	if !validID(actorID) {
		return Task{}, ErrInvalid
	}
	title, outcome, err := validateTaskContent(title, outcome)
	if err != nil {
		return Task{}, err
	}
	id, err := newID()
	if err != nil {
		return Task{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Task{}, err
	}
	defer unlock()
	r, err := s.read(proposalID)
	if err != nil || r.Proposal.RepositoryID != repositoryID {
		return Task{}, ErrNotFound
	}
	if r.Proposal.Status != Open {
		return Task{}, ErrInvalid
	}
	if err := validateTaskLinks(r, id, dependencyIDs, commentIDs); err != nil {
		return Task{}, err
	}
	now := s.now().Truncate(time.Microsecond)
	task := Task{ID: id, ProposalID: proposalID, Title: title, Outcome: outcome, Status: TaskTodo, Position: len(r.Tasks), DependencyIDs: cloneStrings(dependencyIDs), DiscussionCommentIDs: cloneStrings(commentIDs), ContextRevision: 1, CreatedBy: actorID, UpdatedBy: actorID, CreatedAt: now, UpdatedAt: now}
	r.Tasks = append(r.Tasks, task)
	deriveTasks(r.Tasks)
	task = r.Tasks[len(r.Tasks)-1]
	change, err := newTaskChange(task, actorID, "created", now)
	if err != nil {
		return Task{}, err
	}
	r.TaskChanges = append(r.TaskChanges, change)
	if committed, err := s.write(r); err != nil {
		if committed {
			return task, fmt.Errorf("%w: %v", ErrDurabilityUncertain, err)
		}
		return Task{}, err
	}
	return task, nil
}

func (s *Store) ListTasks(repositoryID, proposalID string) ([]Task, error) {
	r, err := s.read(proposalID)
	if err != nil || r.Proposal.RepositoryID != repositoryID {
		return nil, ErrNotFound
	}
	tasks := append([]Task(nil), r.Tasks...)
	deriveTasks(tasks)
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].Position < tasks[j].Position })
	return tasks, nil
}

func (s *Store) GetTask(repositoryID, proposalID, taskID string) (Task, error) {
	tasks, err := s.ListTasks(repositoryID, proposalID)
	if err != nil {
		return Task{}, err
	}
	for _, task := range tasks {
		if task.ID == taskID {
			return task, nil
		}
	}
	return Task{}, ErrNotFound
}

// LinkTaskContribution records the exact review candidate on the task. A
// subsequent candidate supersedes the previous attempt without completing it.
func (s *Store) LinkTaskContribution(repositoryID, proposalID, taskID, actorID string, contribution TaskContribution) (Task, error) {
	if !validID(actorID) || !validID(contribution.PullRequestID) || len(contribution.SourceCommitID) != 40 || contribution.Status != "review" || (contribution.SessionID != "" && !validID(contribution.SessionID)) || (contribution.RunID != "" && !validID(contribution.RunID)) || len(contribution.CommitIDs) == 0 {
		return Task{}, ErrInvalid
	}
	return s.mutateContribution(repositoryID, proposalID, taskID, actorID, func(task *Task) error {
		if task.Contribution != nil && task.Contribution.PullRequestID == contribution.PullRequestID && task.Contribution.Status == "review" {
			return nil
		}
		if task.Status != TaskTodo {
			return ErrInvalid
		}
		if task.Contribution != nil {
			task.Contribution.Status = "superseded"
			if len(task.Contributions) > 0 {
				task.Contributions[len(task.Contributions)-1].Status = "superseded"
			}
		}
		contribution.ContextRevision = effectiveContextRevision(task.ContextRevision)
		task.Contribution = &contribution
		task.Contributions = append(task.Contributions, contribution)
		task.Status = TaskInProgress
		return nil
	}, "contribution_published")
}

func (s *Store) UpdateTaskContribution(repositoryID, proposalID, taskID, actorID, pullRequestID, status string) (Task, error) {
	if !validID(actorID) || !validID(pullRequestID) || (status != "merged" && status != "closed" && status != "superseded") {
		return Task{}, ErrInvalid
	}
	return s.mutateContribution(repositoryID, proposalID, taskID, actorID, func(task *Task) error {
		if task.Contribution == nil || task.Contribution.PullRequestID != pullRequestID {
			return ErrNotFound
		}
		task.Contribution.Status = status
		if len(task.Contributions) > 0 && task.Contributions[len(task.Contributions)-1].PullRequestID == pullRequestID {
			task.Contributions[len(task.Contributions)-1].Status = status
		}
		if status == "merged" {
			task.Status = TaskCompleted
		} else {
			task.Status = TaskTodo
		}
		return nil
	}, "contribution_"+status)
}

func (s *Store) mutateContribution(repositoryID, proposalID, taskID, actorID string, mutate func(*Task) error, action string) (Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Task{}, err
	}
	defer unlock()
	r, err := s.read(proposalID)
	if err != nil || r.Proposal.RepositoryID != repositoryID {
		return Task{}, ErrNotFound
	}
	if action == "contribution_published" && r.Proposal.Status != Open {
		return Task{}, ErrInvalid
	}
	for i := range r.Tasks {
		if r.Tasks[i].ID == taskID {
			task := r.Tasks[i]
			if err := mutate(&task); err != nil {
				return Task{}, err
			}
			now := s.now().Truncate(time.Microsecond)
			task.UpdatedAt, task.UpdatedBy = now, actorID
			r.Tasks[i] = task
			deriveTasks(r.Tasks)
			change, err := newTaskChange(task, actorID, action, now)
			if err != nil {
				return Task{}, err
			}
			r.TaskChanges = append(r.TaskChanges, change)
			if committed, err := s.write(r); err != nil {
				if committed {
					return task, fmt.Errorf("%w: %v", ErrDurabilityUncertain, err)
				}
				return Task{}, err
			}
			return task, nil
		}
	}
	return Task{}, ErrNotFound
}

func (s *Store) UpdateTask(repositoryID, proposalID, taskID, actorID string, patch TaskPatch) (Task, error) {
	if !validID(actorID) || (patch.Title == nil && patch.Outcome == nil && patch.Status == nil && patch.Position == nil && patch.DependencyIDs == nil && patch.DiscussionCommentIDs == nil) {
		return Task{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Task{}, err
	}
	defer unlock()
	r, err := s.read(proposalID)
	if err != nil || r.Proposal.RepositoryID != repositoryID {
		return Task{}, ErrNotFound
	}
	if r.Proposal.Status != Open {
		return Task{}, ErrInvalid
	}
	index := -1
	for i := range r.Tasks {
		if r.Tasks[i].ID == taskID {
			index = i
			break
		}
	}
	if index < 0 {
		return Task{}, ErrNotFound
	}
	task := r.Tasks[index]
	original := task
	if patch.Title != nil {
		task.Title, _, err = validateTaskContent(*patch.Title, task.Outcome)
	}
	if err == nil && patch.Outcome != nil {
		_, task.Outcome, err = validateTaskContent(task.Title, *patch.Outcome)
	}
	if err != nil {
		return Task{}, err
	}
	if patch.Status != nil {
		if !validTaskStatus(*patch.Status) {
			return Task{}, ErrInvalid
		}
		task.Status = *patch.Status
	}
	if patch.DependencyIDs != nil {
		task.DependencyIDs = cloneStrings(*patch.DependencyIDs)
	}
	if patch.DiscussionCommentIDs != nil {
		task.DiscussionCommentIDs = cloneStrings(*patch.DiscussionCommentIDs)
	}
	if err := validateTaskLinks(r, task.ID, task.DependencyIDs, task.DiscussionCommentIDs); err != nil {
		return Task{}, err
	}
	definitionChanged := task.Title != original.Title || task.Outcome != original.Outcome || !slices.Equal(task.DependencyIDs, original.DependencyIDs) || !slices.Equal(task.DiscussionCommentIDs, original.DiscussionCommentIDs)
	if definitionChanged {
		task.ContextRevision = effectiveContextRevision(task.ContextRevision) + 1
	}
	oldPosition := index
	newPosition := oldPosition
	if patch.Position != nil {
		newPosition = *patch.Position
		if newPosition < 0 || newPosition >= len(r.Tasks) {
			return Task{}, ErrInvalid
		}
	}
	now := s.now().Truncate(time.Microsecond)
	task.UpdatedAt, task.UpdatedBy = now, actorID
	r.Tasks[index] = task
	if newPosition != oldPosition {
		moved := r.Tasks[index]
		r.Tasks = append(r.Tasks[:index], r.Tasks[index+1:]...)
		r.Tasks = append(r.Tasks, Task{})
		copy(r.Tasks[newPosition+1:], r.Tasks[newPosition:])
		r.Tasks[newPosition] = moved
	}
	for i := range r.Tasks {
		r.Tasks[i].Position = i
	}
	if hasDependencyCycle(r.Tasks) {
		return Task{}, ErrInvalid
	}
	deriveTasks(r.Tasks)
	for i := range r.Tasks {
		if r.Tasks[i].ID == taskID {
			task = r.Tasks[i]
			break
		}
	}
	action := "updated"
	if newPosition != oldPosition {
		action = "reordered"
	}
	if patch.Status != nil {
		action = "status_changed"
	}
	change, err := newTaskChange(task, actorID, action, now)
	if err != nil {
		return Task{}, err
	}
	r.TaskChanges = append(r.TaskChanges, change)
	if committed, err := s.write(r); err != nil {
		if committed {
			return task, fmt.Errorf("%w: %v", ErrDurabilityUncertain, err)
		}
		return Task{}, err
	}
	return task, nil
}

func (s *Store) ListTaskChanges(repositoryID, proposalID, taskID string) ([]TaskChange, error) {
	r, err := s.read(proposalID)
	if err != nil || r.Proposal.RepositoryID != repositoryID {
		return nil, ErrNotFound
	}
	found := false
	for _, task := range r.Tasks {
		if task.ID == taskID {
			found = true
			break
		}
	}
	if !found {
		return nil, ErrNotFound
	}
	result := []TaskChange{}
	for _, change := range r.TaskChanges {
		if change.TaskID == taskID {
			result = append(result, change)
		}
	}
	return result, nil
}

// AssignTask atomically establishes one accountable owner. ExpectedAssignmentID
// is empty for a claim and must match for reassignment, preventing stale clients
// from silently replacing another collaborator's claim.
func (s *Store) AssignTask(repositoryID, proposalID, taskID, actorID string, input TaskAssignmentInput) (Task, error) {
	mandate := strings.TrimSpace(input.Mandate)
	if input.AssigneeType == "agent" && input.AssigneeID == "" {
		var err error
		input.AssigneeID, err = newID()
		if err != nil {
			return Task{}, err
		}
	}
	if !validID(actorID) || !validID(input.AssigneeID) || !validID(input.RepositoryID) || len(input.BaseRevision) != 40 ||
		(input.AssigneeType != "human" && input.AssigneeType != "agent") || mandate == "" || len([]rune(mandate)) > 4000 {
		return Task{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Task{}, err
	}
	defer unlock()
	r, err := s.read(proposalID)
	if err != nil || r.Proposal.RepositoryID != repositoryID {
		return Task{}, ErrNotFound
	}
	if r.Proposal.Status != Open {
		return Task{}, ErrInvalid
	}
	index := -1
	for i := range r.Tasks {
		if r.Tasks[i].ID == taskID {
			index = i
			break
		}
	}
	if index < 0 {
		return Task{}, ErrNotFound
	}
	deriveTasks(r.Tasks)
	task := r.Tasks[index]
	if !task.Ready || task.Status != TaskTodo {
		return Task{}, ErrInvalid
	}
	if task.Assignment == nil {
		if input.ExpectedAssignmentID != "" {
			return Task{}, ErrTaskAssignmentConflict
		}
	} else if input.ExpectedAssignmentID == "" || task.Assignment.ID != input.ExpectedAssignmentID {
		return Task{}, ErrTaskAssignmentConflict
	}
	id, err := newID()
	if err != nil {
		return Task{}, err
	}
	now := s.now().Truncate(time.Microsecond)
	scopes, branch := []string{"git:read", "git:write"}, "task-scoped branch (created when work starts)"
	if input.AssigneeType == "human" {
		scopes, branch = []string{}, "no new access; existing collaborator authority only"
	}
	task.Assignment = &TaskAssignment{ID: id, AssigneeType: input.AssigneeType, AssigneeID: input.AssigneeID, Mandate: mandate,
		Access: TaskAccess{RepositoryID: input.RepositoryID, BaseRevision: strings.ToLower(input.BaseRevision), Scopes: scopes, Branch: branch}, AssignedBy: actorID, AssignedAt: now, ContextRevision: effectiveContextRevision(task.ContextRevision)}
	task.UpdatedBy, task.UpdatedAt = actorID, now
	r.Tasks[index] = task
	change, err := newTaskChange(task, actorID, map[bool]string{true: "reassigned", false: "assigned"}[input.ExpectedAssignmentID != ""], now)
	if err != nil {
		return Task{}, err
	}
	r.TaskChanges = append(r.TaskChanges, change)
	if committed, err := s.write(r); err != nil {
		if committed {
			return task, fmt.Errorf("%w: %v", ErrDurabilityUncertain, err)
		}
		return Task{}, err
	}
	return task, nil
}

// RebaseTaskAssignment deliberately replaces the starting boundary while
// retaining the accountable owner and mandate. The new assignment ID is a CAS
// boundary: sessions and pull requests created from the prior assignment stay
// attributable, but can no longer be mistaken for work on the current plan.
func (s *Store) RebaseTaskAssignment(repositoryID, proposalID, taskID, actorID string, input TaskRebaseInput) (Task, error) {
	if !validID(actorID) || !validID(input.ExpectedAssignmentID) || len(input.BaseRevision) != 40 {
		return Task{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Task{}, err
	}
	defer unlock()
	r, err := s.read(proposalID)
	if err != nil || r.Proposal.RepositoryID != repositoryID {
		return Task{}, ErrNotFound
	}
	if r.Proposal.Status != Open {
		return Task{}, ErrInvalid
	}
	for i := range r.Tasks {
		if r.Tasks[i].ID != taskID {
			continue
		}
		task := r.Tasks[i]
		if task.Status == TaskCompleted || task.Status == TaskCancelled || task.Assignment == nil || task.Assignment.ID != input.ExpectedAssignmentID {
			return Task{}, ErrTaskAssignmentConflict
		}
		id, err := newID()
		if err != nil {
			return Task{}, err
		}
		now := s.now().Truncate(time.Microsecond)
		assignment := *task.Assignment
		assignment.ID, assignment.AssignedBy, assignment.AssignedAt = id, actorID, now
		assignment.Access.BaseRevision = strings.ToLower(input.BaseRevision)
		assignment.ContextRevision = effectiveContextRevision(task.ContextRevision)
		task.Assignment, task.UpdatedBy, task.UpdatedAt = &assignment, actorID, now
		if task.Status == TaskInProgress && task.Contribution != nil && effectiveContextRevision(task.Contribution.ContextRevision) != effectiveContextRevision(task.ContextRevision) {
			task.Status = TaskTodo
		}
		r.Tasks[i] = task
		deriveTasks(r.Tasks)
		task = r.Tasks[i]
		change, err := newTaskChange(task, actorID, "rebased", now)
		if err != nil {
			return Task{}, err
		}
		r.TaskChanges = append(r.TaskChanges, change)
		if committed, err := s.write(r); err != nil {
			if committed {
				return task, fmt.Errorf("%w: %v", ErrDurabilityUncertain, err)
			}
			return Task{}, err
		}
		return task, nil
	}
	return Task{}, ErrNotFound
}

func (s *Store) RevokeTaskAssignment(repositoryID, proposalID, taskID, actorID, expectedID string) (Task, error) {
	if !validID(actorID) || !validID(expectedID) {
		return Task{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Task{}, err
	}
	defer unlock()
	r, err := s.read(proposalID)
	if err != nil || r.Proposal.RepositoryID != repositoryID {
		return Task{}, ErrNotFound
	}
	if r.Proposal.Status != Open {
		return Task{}, ErrInvalid
	}
	index := -1
	for i := range r.Tasks {
		if r.Tasks[i].ID == taskID {
			index = i
			break
		}
	}
	if index < 0 {
		return Task{}, ErrNotFound
	}
	task := r.Tasks[index]
	if task.Assignment == nil || task.Assignment.ID != expectedID {
		return Task{}, ErrTaskAssignmentConflict
	}
	now := s.now().Truncate(time.Microsecond)
	task.Assignment = nil
	task.UpdatedBy, task.UpdatedAt = actorID, now
	r.Tasks[index] = task
	change, err := newTaskChange(task, actorID, "assignment_revoked", now)
	if err != nil {
		return Task{}, err
	}
	r.TaskChanges = append(r.TaskChanges, change)
	if committed, err := s.write(r); err != nil {
		if committed {
			return task, fmt.Errorf("%w: %v", ErrDurabilityUncertain, err)
		}
		return Task{}, err
	}
	return task, nil
}

func validateTaskContent(title, outcome string) (string, string, error) {
	title, outcome = strings.TrimSpace(title), strings.TrimSpace(outcome)
	if title == "" || outcome == "" || strings.ContainsAny(title, "\r\n") || len([]rune(title)) > 200 || len([]rune(outcome)) > 2000 {
		return "", "", ErrInvalid
	}
	return title, outcome, nil
}

func validateTaskLinks(r record, taskID string, dependencies, comments []string) error {
	seen := map[string]bool{}
	for _, id := range dependencies {
		if !validID(id) || id == taskID || seen[id] {
			return ErrInvalid
		}
		seen[id] = true
		found := false
		for _, task := range r.Tasks {
			if task.ID == id {
				found = true
			}
		}
		if !found {
			return ErrInvalid
		}
	}
	seen = map[string]bool{}
	for _, id := range comments {
		if !validID(id) || seen[id] {
			return ErrInvalid
		}
		seen[id] = true
		found := false
		for _, comment := range r.Comments {
			if comment.ID == id {
				found = true
			}
		}
		if !found {
			return ErrInvalid
		}
	}
	return nil
}
func validTaskStatus(status string) bool {
	return status == TaskTodo || status == TaskInProgress || status == TaskCompleted || status == TaskCancelled
}
func cloneStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return append([]string(nil), values...)
}
func deriveTasks(tasks []Task) {
	satisfied := map[string]bool{}
	for _, task := range tasks {
		currentContribution := task.Contribution == nil || (task.Contribution.Status == "merged" && effectiveContextRevision(task.Contribution.ContextRevision) == effectiveContextRevision(task.ContextRevision))
		satisfied[task.ID] = task.Status == TaskCompleted && currentContribution
	}
	for i := range tasks {
		blocked := []string{}
		for _, id := range tasks[i].DependencyIDs {
			if !satisfied[id] {
				blocked = append(blocked, id)
			}
		}
		tasks[i].BlockedBy = blocked
		tasks[i].Ready = tasks[i].Status == TaskTodo && len(blocked) == 0
		tasks[i].ContextRevision = effectiveContextRevision(tasks[i].ContextRevision)
		if tasks[i].Assignment != nil {
			tasks[i].Assignment.ContextRevision = effectiveContextRevision(tasks[i].Assignment.ContextRevision)
		}
		if tasks[i].Contribution != nil {
			tasks[i].Contribution.ContextRevision = effectiveContextRevision(tasks[i].Contribution.ContextRevision)
		}
		tasks[i].ContextState = "current"
		if tasks[i].Contribution != nil && effectiveContextRevision(tasks[i].Contribution.ContextRevision) != tasks[i].ContextRevision {
			tasks[i].ContextState = "obsolete"
		} else if tasks[i].Assignment != nil && effectiveContextRevision(tasks[i].Assignment.ContextRevision) != tasks[i].ContextRevision {
			tasks[i].ContextState = "changed"
		}
		tasks[i].DependencyIDs = cloneStrings(tasks[i].DependencyIDs)
		tasks[i].DiscussionCommentIDs = cloneStrings(tasks[i].DiscussionCommentIDs)
	}
}
func effectiveContextRevision(revision int) int {
	if revision < 1 {
		return 1
	}
	return revision
}
func hasDependencyCycle(tasks []Task) bool {
	edges := map[string][]string{}
	for _, task := range tasks {
		edges[task.ID] = task.DependencyIDs
	}
	visiting, done := map[string]bool{}, map[string]bool{}
	var visit func(string) bool
	visit = func(id string) bool {
		if visiting[id] {
			return true
		}
		if done[id] {
			return false
		}
		visiting[id] = true
		for _, next := range edges[id] {
			if visit(next) {
				return true
			}
		}
		visiting[id] = false
		done[id] = true
		return false
	}
	for id := range edges {
		if visit(id) {
			return true
		}
	}
	return false
}
func newTaskChange(task Task, actorID, action string, now time.Time) (TaskChange, error) {
	id, err := newID()
	if err != nil {
		return TaskChange{}, err
	}
	return TaskChange{ID: id, TaskID: task.ID, ActorID: actorID, Action: action, Task: task, CreatedAt: now}, nil
}

func validateContent(title, body string) (string, string, error) {
	title, body = strings.TrimSpace(title), strings.TrimSpace(body)
	if title == "" || len([]rune(title)) > 200 || strings.ContainsAny(title, "\r\n") || len([]rune(body)) > 10000 {
		return "", "", ErrInvalid
	}
	return title, body, nil
}
func newID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
func validID(id string) bool {
	if len(id) != 32 || id != strings.ToLower(id) {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}
func (s *Store) path(id string) string { return filepath.Join(s.root, id+".json") }

func (s *Store) read(id string) (record, error) {
	if !validID(id) {
		return record{}, ErrNotFound
	}
	data, err := s.readFile(s.path(id))
	if errors.Is(err, os.ErrNotExist) {
		return record{}, ErrNotFound
	}
	if err != nil {
		return record{}, err
	}
	var r record
	if json.Unmarshal(data, &r) != nil || r.Proposal.ID != id || !validID(r.Proposal.RepositoryID) || !validID(r.Proposal.AuthorID) || (r.Proposal.Status != Open && r.Proposal.Status != Closed) || (r.Proposal.Status == Open && r.Proposal.ClosedAt != nil) || (r.Proposal.Status == Closed && r.Proposal.ClosedAt == nil) {
		return record{}, fmt.Errorf("corrupt proposal %s", id)
	}
	if _, _, err := validateContent(r.Proposal.Title, r.Proposal.Body); err != nil {
		return record{}, fmt.Errorf("corrupt proposal %s", id)
	}
	if r.Corrective != nil && (!validID(r.Corrective.IncidentID) || !validID(r.Corrective.OperationID) || !validID(r.Corrective.ActorID) || !validID(r.Corrective.AssigneeID) || len(r.Corrective.BaseRevision) != 40 || r.Corrective.DueAt.IsZero()) {
		return record{}, fmt.Errorf("corrupt proposal %s", id)
	}
	seen := map[string]bool{}
	for _, c := range r.Comments {
		if !validID(c.ID) || c.ProposalID != id || !validID(c.AuthorID) || strings.TrimSpace(c.Body) == "" || len([]rune(c.Body)) > 10000 || seen[c.ID] {
			return record{}, fmt.Errorf("corrupt proposal %s", id)
		}
		seen[c.ID] = true
	}
	positions := make([]bool, len(r.Tasks))
	seenTasks := map[string]bool{}
	for _, task := range r.Tasks {
		if !validStoredTask(task, id) || seenTasks[task.ID] || task.Position < 0 || task.Position >= len(r.Tasks) || positions[task.Position] {
			return record{}, fmt.Errorf("corrupt proposal %s", id)
		}
		seenTasks[task.ID], positions[task.Position] = true, true
		if err := validateTaskLinks(r, task.ID, task.DependencyIDs, task.DiscussionCommentIDs); err != nil {
			return record{}, fmt.Errorf("corrupt proposal %s", id)
		}
	}
	if hasDependencyCycle(r.Tasks) {
		return record{}, fmt.Errorf("corrupt proposal %s", id)
	}
	seenChanges := map[string]bool{}
	for _, change := range r.TaskChanges {
		if !validID(change.ID) || !seenTasks[change.TaskID] || !validID(change.ActorID) || seenChanges[change.ID] || (change.Action != "created" && change.Action != "updated" && change.Action != "status_changed" && change.Action != "reordered" && change.Action != "assigned" && change.Action != "reassigned" && change.Action != "rebased" && change.Action != "assignment_revoked" && change.Action != "contribution_published" && change.Action != "contribution_merged" && change.Action != "contribution_closed" && change.Action != "contribution_superseded") || !validStoredTask(change.Task, id) || change.Task.ID != change.TaskID {
			return record{}, fmt.Errorf("corrupt proposal %s", id)
		}
		seenChanges[change.ID] = true
	}
	return r, nil
}

func validStoredTask(task Task, proposalID string) bool {
	if !validID(task.ID) || task.ProposalID != proposalID || !validID(task.CreatedBy) || !validID(task.UpdatedBy) || !validTaskStatus(task.Status) || task.CreatedAt.IsZero() || task.UpdatedAt.Before(task.CreatedAt) || (task.Contribution != nil && (!validID(task.Contribution.PullRequestID) || len(task.Contribution.SourceCommitID) != 40 || len(task.Contribution.CommitIDs) == 0 || (task.Contribution.Status != "review" && task.Contribution.Status != "merged" && task.Contribution.Status != "closed" && task.Contribution.Status != "superseded"))) {
		return false
	}
	if task.Assignment != nil {
		a := task.Assignment
		if !validID(a.ID) || !validID(a.AssigneeID) || !validID(a.AssignedBy) || !validID(a.Access.RepositoryID) || len(a.Access.BaseRevision) != 40 || a.AssignedAt.IsZero() || strings.TrimSpace(a.Mandate) == "" || len([]rune(a.Mandate)) > 4000 || (a.AssigneeType != "human" && a.AssigneeType != "agent") {
			return false
		}
	}
	seen := map[string]bool{}
	for _, dependencyID := range task.DependencyIDs {
		if !validID(dependencyID) || dependencyID == task.ID || seen[dependencyID] {
			return false
		}
		seen[dependencyID] = true
	}
	seen = map[string]bool{}
	for _, commentID := range task.DiscussionCommentIDs {
		if !validID(commentID) || seen[commentID] {
			return false
		}
		seen[commentID] = true
	}
	_, _, err := validateTaskContent(task.Title, task.Outcome)
	return err == nil
}

// write reports whether the atomic rename made the requested state visible.
// Once committed, callers must preserve the resource result even if syncing
// the parent directory cannot confirm crash durability; reporting an ordinary
// failure would discard generated IDs and make client retries duplicate work.
func (s *Store) write(r record) (bool, error) {
	data, err := json.Marshal(r)
	if err != nil {
		return false, err
	}
	temp, err := os.CreateTemp(s.root, ".writing-")
	if err != nil {
		return false, err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err = temp.Chmod(0o600); err == nil {
		_, err = temp.Write(append(data, '\n'))
	}
	if err == nil {
		err = temp.Sync()
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return false, err
	}
	if err := os.Rename(tempPath, s.path(r.Proposal.ID)); err != nil {
		return false, err
	}
	return true, s.directorySync(s.root)
}

func syncDirectory(path string) error {
	d, err := os.Open(path)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
func (s *Store) lock() (func(), error) {
	f, err := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}
