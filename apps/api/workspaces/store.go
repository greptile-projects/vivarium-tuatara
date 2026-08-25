// Package workspaces stores reproducible, revision-pinned development environments.
package workspaces

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
)

var (
	ErrNotFound       = errors.New("workspace not found")
	ErrInvalid        = errors.New("invalid workspace")
	ErrConflict       = errors.New("workspace foundation changed")
	ErrControl        = errors.New("workspace control changed")
	ErrRequestChanged = errors.New("workspace request identity changed")
)

const DefinitionPath = ".vivarium/workspace.json"

type Resources struct {
	CPUs         float64 `json:"cpus"`
	MemoryMB     int     `json:"memory_mb"`
	StorageMB    int     `json:"storage_mb"`
	SetupSeconds int     `json:"setup_seconds"`
}
type Tool struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}
type Definition struct {
	Version      int                 `json:"version"`
	Image        string              `json:"image"`
	Tools        []Tool              `json:"tools"`
	Dependencies []string            `json:"dependencies"`
	Setup        []string            `json:"setup"`
	Experiments  []ExperimentCommand `json:"experiments,omitempty"`
	Resources    Resources           `json:"resources"`
}
type ExperimentCommand struct {
	Name    string `json:"name"`
	Command string `json:"command"`
}
type Source struct {
	Kind                    string `json:"kind"`
	RepositoryID            string `json:"repository_id"`
	ProposalID              string `json:"proposal_id,omitempty"`
	TaskID                  string `json:"task_id,omitempty"`
	PullRequestID           string `json:"pull_request_id,omitempty"`
	IncidentID              string `json:"incident_id,omitempty"`
	RepairID                string `json:"repair_id,omitempty"`
	DecisionID              string `json:"decision_id,omitempty"`
	AlternativeID           string `json:"alternative_id,omitempty"`
	IssueID                 string `json:"issue_id,omitempty"`
	ReleaseID               string `json:"release_id,omitempty"`
	DefaultBranchRevision   string `json:"default_branch_revision,omitempty"`
	DefaultDefinitionSHA256 string `json:"default_definition_sha256,omitempty"`
	UpstreamRepositoryID    string `json:"upstream_repository_id,omitempty"`
	OpportunityID           string `json:"opportunity_id,omitempty"`
	SupportThreadID         string `json:"support_thread_id,omitempty"`
	AnswerID                string `json:"answer_id,omitempty"`
	AnswerRevisionID        string `json:"answer_revision_id,omitempty"`
	DebuggingWorkspaceID    string `json:"debugging_workspace_id,omitempty"`
	ReplayScenarioID        string `json:"replay_scenario_id,omitempty"`
	ConflictLaunchID        string `json:"conflict_launch_id,omitempty"`
	LearningPathwaySlug     string `json:"learning_pathway_slug,omitempty"`
	LearningPathwayVersion  int    `json:"learning_pathway_version,omitempty"`
	LearningModuleID        string `json:"learning_module_id,omitempty"`
	LearningExerciseID      string `json:"learning_exercise_id,omitempty"`
	LearningRequestID       string `json:"learning_request_id,omitempty"`
}

type LearningCheckpoint struct {
	ID                string    `json:"id"`
	Summary           string    `json:"summary"`
	CriterionIDs      []string  `json:"criterion_ids"`
	CommandOutcomeIDs []string  `json:"command_outcome_ids"`
	CreatedAt         time.Time `json:"created_at"`
}
type LearningHintUse struct {
	Index  int       `json:"index"`
	Hint   string    `json:"hint"`
	UsedAt time.Time `json:"used_at"`
}
type LearningEvidenceCitation struct {
	Path     string `json:"path"`
	Revision string `json:"revision"`
}
type LearningGuidanceEntry struct {
	ID                string                     `json:"id"`
	Kind              string                     `json:"kind"`
	Body              string                     `json:"body"`
	ActorID           string                     `json:"actor_id"`
	ActorKind         string                     `json:"actor_kind"`
	AgentID           string                     `json:"agent_id,omitempty"`
	Citations         []LearningEvidenceCitation `json:"citations,omitempty"`
	CheckpointIDs     []string                   `json:"checkpoint_ids,omitempty"`
	CommandOutcomeIDs []string                   `json:"command_outcome_ids,omitempty"`
	LearnerControlled bool                       `json:"learner_controlled"`
	CreatedAt         time.Time                  `json:"created_at"`
}
type LearningGuidance struct {
	Version       int                     `json:"version"`
	Entries       []LearningGuidanceEntry `json:"entries"`
	AgentID       string                  `json:"agent_id,omitempty"`
	AgentState    string                  `json:"agent_state,omitempty"`
	AgentGuidedBy string                  `json:"agent_guided_by,omitempty"`
}
type LearningContext struct {
	PathwaySlug           string               `json:"pathway_slug"`
	PathwayVersion        int                  `json:"pathway_version"`
	ModuleID              string               `json:"module_id"`
	ExerciseID            string               `json:"exercise_id"`
	Kind                  string               `json:"kind"`
	Instructions          string               `json:"instructions"`
	StarterCommands       []string             `json:"starter_commands"`
	AcceptanceCriteria    []string             `json:"acceptance_criteria"`
	Data                  []string             `json:"data"`
	Hints                 []string             `json:"hints"`
	HintsUsed             []LearningHintUse    `json:"hints_used"`
	Checkpoints           []LearningCheckpoint `json:"checkpoints"`
	ReproducibilitySHA256 string               `json:"reproducibility_sha256"`
	Cost                  float64              `json:"cost"`
	Guidance              LearningGuidance     `json:"guidance"`
}

// ConflictContext freezes the two histories and the evidence that was visible
// when a reconciliation workspace was launched. It is context, not authority:
// publication still passes through the named repository's normal controls.
type ConflictContext struct {
	Version           int                         `json:"version"`
	PullRequestID     string                      `json:"pull_request_id"`
	CandidateID       string                      `json:"candidate_id,omitempty"`
	BaseCommitID      string                      `json:"base_commit_id"`
	Source            ConflictRevision            `json:"source"`
	Target            ConflictRevision            `json:"target"`
	Files             []ConflictFileEvidence      `json:"files"`
	AffectedChecks    []string                    `json:"affected_checks"`
	RequiredChecks    []ConflictRequiredCheck     `json:"required_checks"`
	Incomplete        []string                    `json:"incomplete"`
	PublicationTarget []ConflictPublication       `json:"publication_targets"`
	Questions         []ConflictQuestion          `json:"questions"`
	Resolutions       []ConflictResolution        `json:"resolutions"`
	Checkpoints       []ConflictCheckpoint        `json:"checkpoints"`
	Publications      []ConflictPublicationRecord `json:"publications"`
}

// ConflictPublicationRecord is the durable bridge from workspace evidence to
// ordinary contribution governance. It records what was published, but grants
// no branch, pull-request, review, check, queue, or merge authority.
type ConflictPublicationRecord struct {
	ID                string             `json:"id"`
	CheckpointID      string             `json:"checkpoint_id"`
	Mode              string             `json:"mode"`
	RepositoryID      string             `json:"repository_id"`
	Branch            string             `json:"branch"`
	ExpectedBranchTip string             `json:"expected_branch_tip,omitempty"`
	PublishedCommitID string             `json:"published_commit_id,omitempty"`
	PullRequestID     string             `json:"pull_request_id,omitempty"`
	Status            string             `json:"status"`
	ActionRequired    string             `json:"action_required,omitempty"`
	PublishedBy       ConflictAuthorship `json:"published_by"`
	CreatedAt         time.Time          `json:"created_at"`
	UpdatedAt         time.Time          `json:"updated_at"`
}
type ConflictRequiredCheck struct {
	Name       string               `json:"name"`
	Definition checkruns.Definition `json:"definition"`
}

// ConflictCheckpoint is an immutable verification of one assembled result.
// Later checkpoints can mark only evidence whose named inputs moved stale;
// the original commands, output, artifacts, cost, and decision remain intact.
type ConflictCheckpoint struct {
	ID                 string                       `json:"id"`
	CandidateCommitID  string                       `json:"candidate_commit_id"`
	CandidateTreeID    string                       `json:"candidate_tree_id"`
	SourceRevision     string                       `json:"source_revision"`
	TargetRevision     string                       `json:"target_revision"`
	DependencyRevision string                       `json:"dependency_revision,omitempty"`
	PolicyRevision     string                       `json:"policy_revision,omitempty"`
	Criteria           []ConflictCriterion          `json:"criteria"`
	Decisions          []ConflictCheckpointDecision `json:"decisions"`
	CreatedBy          ConflictAuthorship           `json:"created_by"`
	CreatedAt          time.Time                    `json:"created_at"`
}
type ConflictCriterion struct {
	ID              string                       `json:"id"`
	Kind            string                       `json:"kind"`
	Name            string                       `json:"name"`
	Origin          string                       `json:"origin"`
	Command         string                       `json:"command"`
	ExactCriteria   []string                     `json:"exact_criteria"`
	Coverage        []string                     `json:"coverage"`
	OwnerIDs        []string                     `json:"owner_ids"`
	State           string                       `json:"state"`
	ExitCode        int                          `json:"exit_code"`
	Logs            string                       `json:"logs,omitempty"`
	Artifacts       []ConflictCheckpointArtifact `json:"artifacts"`
	Cost            float64                      `json:"cost"`
	InvalidatedBy   []string                     `json:"invalidated_by,omitempty"`
	CheckRunID      string                       `json:"check_run_id,omitempty"`
	CheckRunScopeID string                       `json:"check_run_scope_id,omitempty"`
}
type ConflictCheckpointArtifact struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}
type ConflictCheckpointDecision struct {
	CriterionID string    `json:"criterion_id"`
	OwnerID     string    `json:"owner_id"`
	Decision    string    `json:"decision"`
	Rationale   string    `json:"rationale"`
	CreatedAt   time.Time `json:"created_at"`
}

type ConflictCitation struct {
	Side       string `json:"side"`
	Revision   string `json:"revision"`
	Path       string `json:"path"`
	EvidenceID string `json:"evidence_id,omitempty"`
}
type ConflictAuthorship struct {
	ActorID string `json:"actor_id"`
	AgentID string `json:"agent_id,omitempty"`
}
type ConflictAnswer struct {
	Body        string             `json:"body"`
	Uncertainty string             `json:"uncertainty"`
	Citations   []ConflictCitation `json:"citations"`
	Authorship  ConflictAuthorship `json:"authorship"`
	CreatedAt   time.Time          `json:"created_at"`
}
type ConflictQuestion struct {
	ID          string             `json:"id"`
	Body        string             `json:"body"`
	Uncertainty string             `json:"uncertainty"`
	Citations   []ConflictCitation `json:"citations"`
	Authorship  ConflictAuthorship `json:"authorship"`
	Answer      *ConflictAnswer    `json:"answer,omitempty"`
	CreatedAt   time.Time          `json:"created_at"`
}
type ConflictPreservation struct {
	Kind        string             `json:"kind"`
	Reference   string             `json:"reference"`
	Disposition string             `json:"disposition"`
	Rationale   string             `json:"rationale"`
	Citations   []ConflictCitation `json:"citations"`
}
type ConflictResolution struct {
	ID              string                 `json:"id"`
	Path            string                 `json:"path"`
	Summary         string                 `json:"summary"`
	ProposedContent string                 `json:"proposed_content"`
	PreviousContent string                 `json:"previous_content,omitempty"`
	ExpectedSHA256  string                 `json:"expected_sha256"`
	AppliedSHA256   string                 `json:"applied_sha256,omitempty"`
	State           string                 `json:"state"`
	Uncertainty     string                 `json:"uncertainty"`
	Preservation    []ConflictPreservation `json:"preservation"`
	Authorship      ConflictAuthorship     `json:"authorship"`
	AppliedBy       *ConflictAuthorship    `json:"applied_by,omitempty"`
	CreatedAt       time.Time              `json:"created_at"`
	AppliedAt       *time.Time             `json:"applied_at,omitempty"`
	UndoneAt        *time.Time             `json:"undone_at,omitempty"`
	PendingAction   *ConflictPendingAction `json:"pending_action,omitempty"`
}
type ConflictPendingAction struct {
	Kind           string             `json:"kind"`
	BeforeContent  string             `json:"before_content"`
	BeforeSHA256   string             `json:"before_sha256"`
	IntendedSHA256 string             `json:"intended_sha256"`
	Authorship     ConflictAuthorship `json:"authorship"`
	StartedAt      time.Time          `json:"started_at"`
	PrincipalID    string             `json:"principal_id"`
	ControlVersion int                `json:"control_version"`
}
type ConflictRevision struct {
	Branch   string   `json:"branch"`
	CommitID string   `json:"commit_id"`
	OwnerIDs []string `json:"owner_ids"`
}
type ConflictFileEvidence struct {
	Path         string   `json:"path"`
	Kinds        []string `json:"kinds"`
	Symbols      []string `json:"symbols"`
	SourceChange string   `json:"source_change"`
	TargetChange string   `json:"target_change"`
}
type ConflictPublication struct {
	RepositoryID string `json:"repository_id"`
	Branch       string `json:"branch"`
	Revision     string `json:"revision"`
	Authority    string `json:"authority"`
}
type WorkspaceParticipant struct {
	PrincipalKind string     `json:"principal_kind"`
	PrincipalID   string     `json:"principal_id"`
	Role          string     `json:"role"`
	Status        string     `json:"status"`
	InvitedBy     string     `json:"invited_by"`
	InvitedAt     time.Time  `json:"invited_at"`
	RespondedAt   *time.Time `json:"responded_at,omitempty"`
}

type ContributorContext struct {
	OpportunityID        string           `json:"opportunity_id"`
	OpportunityVersion   int              `json:"opportunity_version"`
	UpstreamRepositoryID string           `json:"upstream_repository_id"`
	PathwayVersion       int              `json:"pathway_version"`
	Guidance             string           `json:"guidance"`
	Prerequisites        []string         `json:"prerequisites"`
	AcceptanceCriteria   []string         `json:"acceptance_criteria"`
	EvidenceKind         string           `json:"evidence_kind"`
	EvidenceID           string           `json:"evidence_id"`
	EvidenceParentID     string           `json:"evidence_parent_id,omitempty"`
	SampleAttachmentIDs  []string         `json:"sample_attachment_ids,omitempty"`
	Diagnostics          []string         `json:"diagnostics"`
	MentorIDs            []string         `json:"mentor_ids,omitempty"`
	AgentAssistance      bool             `json:"agent_assistance"`
	Help                 ContributionHelp `json:"help"`
}
type ContributionHelp struct {
	Version      int                     `json:"version"`
	State        string                  `json:"state"`
	StateReason  string                  `json:"state_reason,omitempty"`
	Entries      []ContributionHelpEntry `json:"entries"`
	Availability []MentorAvailability    `json:"mentor_availability"`
}
type ContributionHelpEntry struct {
	ID            string     `json:"id"`
	Kind          string     `json:"kind"`
	ActorID       string     `json:"actor_id"`
	AgentID       string     `json:"agent_id,omitempty"`
	Action        string     `json:"action,omitempty"`
	Body          string     `json:"body"`
	ReplyTo       string     `json:"reply_to,omitempty"`
	Status        string     `json:"status"`
	DecisionOwner string     `json:"decision_owner"`
	RequestedBy   string     `json:"requested_by,omitempty"`
	DueAt         *time.Time `json:"due_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	ResolvedAt    *time.Time `json:"resolved_at,omitempty"`
}
type MentorAvailability struct {
	MentorID      string    `json:"mentor_id"`
	Status        string    `json:"status"`
	ResponseHours int       `json:"response_hours,omitempty"`
	Note          string    `json:"note,omitempty"`
	UpdatedAt     time.Time `json:"updated_at"`
}
type Access struct {
	Role   string   `json:"role"`
	Scopes []string `json:"scopes"`
}
type SetupStep struct {
	Command     string    `json:"command"`
	State       string    `json:"state"`
	ExitCode    int       `json:"exit_code"`
	Output      string    `json:"output,omitempty"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
}
type Event struct {
	ID        string    `json:"id,omitempty"`
	Kind      string    `json:"kind"`
	ActorID   string    `json:"actor_id"`
	Role      string    `json:"role,omitempty"`
	Detail    string    `json:"detail,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
type CommandOutcome struct {
	ID            string    `json:"id"`
	CommandSHA256 string    `json:"command_sha256"`
	Directory     string    `json:"directory"`
	ExitCode      int       `json:"exit_code"`
	Output        string    `json:"output,omitempty"`
	ActorID       string    `json:"actor_id"`
	StartedAt     time.Time `json:"started_at"`
	CompletedAt   time.Time `json:"completed_at"`
}
type Presence struct {
	ActorID  string    `json:"actor_id"`
	Focus    string    `json:"focus"`
	Path     string    `json:"path,omitempty"`
	JoinedAt time.Time `json:"joined_at"`
	SeenAt   time.Time `json:"seen_at"`
}
type Control struct {
	Version       int       `json:"version"`
	PrincipalKind string    `json:"principal_kind"`
	PrincipalID   string    `json:"principal_id"`
	Mode          string    `json:"mode"`
	Scopes        []string  `json:"scopes"`
	GrantedBy     string    `json:"granted_by"`
	GrantedAt     time.Time `json:"granted_at"`
	ExpiresAt     time.Time `json:"expires_at"`
}
type Message struct {
	ID        string    `json:"id"`
	ActorID   string    `json:"actor_id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}
type Change struct {
	Path               string    `json:"path"`
	SHA256             string    `json:"sha256"`
	Size               int       `json:"size"`
	ActorID            string    `json:"actor_id"`
	CreatedAt          time.Time `json:"created_at"`
	ResolutionActionID string    `json:"resolution_action_id,omitempty"`
}
type Workspace struct {
	ID                             string                 `json:"id"`
	RepositoryID                   string                 `json:"repository_id"`
	OrganizationID                 string                 `json:"organization_id,omitempty"`
	CommitID                       string                 `json:"commit_id"`
	Definition                     Definition             `json:"definition"`
	DefinitionSHA256               string                 `json:"definition_sha256"`
	Source                         Source                 `json:"source"`
	CreatorID                      string                 `json:"creator_id"`
	Access                         Access                 `json:"effective_access"`
	State                          string                 `json:"state"`
	Setup                          []SetupStep            `json:"setup_evidence"`
	Events                         []Event                `json:"events"`
	Commands                       []CommandOutcome       `json:"command_outcomes"`
	Changes                        []Change               `json:"changes"`
	Presence                       []Presence             `json:"presence"`
	Control                        Control                `json:"control"`
	Messages                       []Message              `json:"messages"`
	HeadCheckpointID               string                 `json:"head_checkpoint_id,omitempty"`
	CreatedAt                      time.Time              `json:"created_at"`
	UpdatedAt                      time.Time              `json:"updated_at"`
	SuspendedAt                    *time.Time             `json:"suspended_at,omitempty"`
	ResumedAt                      *time.Time             `json:"resumed_at,omitempty"`
	Policy                         Policy                 `json:"policy"`
	PolicyScope                    string                 `json:"policy_scope"`
	PolicyVersion                  int                    `json:"policy_version"`
	LastActivityAt                 time.Time              `json:"last_activity_at"`
	ExpiresAt                      *time.Time             `json:"expires_at,omitempty"`
	ExpiryAnnouncedAt              *time.Time             `json:"expiry_announced_at,omitempty"`
	StoppedAt                      *time.Time             `json:"stopped_at,omitempty"`
	StoppedBy                      string                 `json:"stopped_by,omitempty"`
	StopReason                     string                 `json:"stop_reason,omitempty"`
	RebuildRequired                bool                   `json:"rebuild_required"`
	RebuildReasons                 []string               `json:"rebuild_reasons"`
	Reasoning                      *ReasoningContext      `json:"reasoning,omitempty"`
	ReproductionInputAttachmentIDs []string               `json:"reproduction_input_attachment_ids,omitempty"`
	ContributorContext             *ContributorContext    `json:"contributor_context,omitempty"`
	ConflictContext                *ConflictContext       `json:"conflict_context,omitempty"`
	LearningContext                *LearningContext       `json:"learning_context,omitempty"`
	Participants                   []WorkspaceParticipant `json:"participants,omitempty"`
}

func (s *Store) UseLearningHint(id, actor string, index int) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.read(id)
	if err != nil {
		return Workspace{}, err
	}
	if w.LearningContext == nil || actor != w.CreatorID || index < 0 || index >= len(w.LearningContext.Hints) {
		return Workspace{}, ErrInvalid
	}
	for _, used := range w.LearningContext.HintsUsed {
		if used.Index == index {
			return w, nil
		}
	}
	now := s.now()
	hint := LearningHintUse{Index: index, Hint: w.LearningContext.Hints[index], UsedAt: now}
	w.LearningContext.HintsUsed = append(w.LearningContext.HintsUsed, hint)
	w.UpdatedAt = now
	w.Events = append(w.Events, Event{Kind: "learning.hint.used", ActorID: actor, Role: "instruction", Detail: fmt.Sprint(index), CreatedAt: now})
	return w, s.write(w)
}

func (s *Store) AddLearningCheckpoint(id, actor, summary string, criteria, outcomes []string) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.read(id)
	if err != nil {
		return Workspace{}, err
	}
	if w.LearningContext == nil || actor != w.CreatorID || strings.TrimSpace(summary) == "" || len(summary) > 1000 || len(criteria) == 0 {
		return Workspace{}, ErrInvalid
	}
	validCriteria := map[string]bool{}
	for _, c := range w.LearningContext.AcceptanceCriteria {
		validCriteria[c] = true
	}
	validOutcomes := map[string]bool{}
	provenance, err := s.readOrSeedProvenance(id, w)
	if err != nil {
		return Workspace{}, err
	}
	for _, o := range provenance.Commands {
		validOutcomes[o.ID] = true
	}
	for _, c := range criteria {
		if !validCriteria[c] {
			return Workspace{}, ErrInvalid
		}
	}
	for _, o := range outcomes {
		if !validOutcomes[o] {
			return Workspace{}, ErrInvalid
		}
	}
	idv, err := randomID(12)
	if err != nil {
		return Workspace{}, err
	}
	now := s.now()
	cp := LearningCheckpoint{ID: idv, Summary: summary, CriterionIDs: append([]string(nil), criteria...), CommandOutcomeIDs: append([]string(nil), outcomes...), CreatedAt: now}
	w.LearningContext.Checkpoints = append(w.LearningContext.Checkpoints, cp)
	w.UpdatedAt = now
	w.Events = append(w.Events, Event{ID: idv, Kind: "learning.checkpoint.created", ActorID: actor, Role: "verification", Detail: summary, CreatedAt: now})
	return w, s.write(w)
}

func (s *Store) AddLearningGuidance(id, actor, actorKind, agentID, kind, body string, citations []LearningEvidenceCitation, checkpoints, outcomes []string, expected int) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.read(id)
	if err != nil {
		return Workspace{}, err
	}
	if w.LearningContext == nil || w.LearningContext.Guidance.Version != expected {
		return Workspace{}, ErrConflict
	}
	validRole := (actorKind == "learner" && actor == w.CreatorID && agentID == "" && kind == "question") || (actorKind == "mentor" && agentID == "" && (kind == "explanation" || kind == "hint" || kind == "demonstration" || kind == "direct_action")) || (actorKind == "agent" && agentID != "" && kind == "hint")
	if !validRole || strings.TrimSpace(body) == "" || len(body) > 4000 || (actorKind != "learner" && len(citations) == 0) || (actorKind != "learner" && (len(checkpoints) > 0 || len(outcomes) > 0)) {
		return Workspace{}, ErrInvalid
	}
	if actorKind == "agent" && (w.LearningContext.Guidance.AgentID != agentID || w.LearningContext.Guidance.AgentState != "active" || w.Control.PrincipalKind != "approved_agent" || w.Control.PrincipalID != agentID || w.Control.Mode != "guide" || !w.Control.ExpiresAt.After(s.now())) {
		return Workspace{}, ErrControl
	}
	validCP := map[string]bool{}
	for _, cp := range w.LearningContext.Checkpoints {
		validCP[cp.ID] = true
	}
	provenance, err := s.readOrSeedProvenance(id, w)
	if err != nil {
		return Workspace{}, err
	}
	validOutcome := map[string]bool{}
	for _, o := range provenance.Commands {
		validOutcome[o.ID] = true
	}
	for _, x := range checkpoints {
		if !validCP[x] {
			return Workspace{}, ErrInvalid
		}
	}
	for _, x := range outcomes {
		if !validOutcome[x] {
			return Workspace{}, ErrInvalid
		}
	}
	entryID, err := randomID(12)
	if err != nil {
		return Workspace{}, err
	}
	now := s.now()
	e := LearningGuidanceEntry{ID: entryID, Kind: kind, Body: strings.TrimSpace(body), ActorID: actor, ActorKind: actorKind, AgentID: agentID, Citations: append([]LearningEvidenceCitation(nil), citations...), CheckpointIDs: append([]string(nil), checkpoints...), CommandOutcomeIDs: append([]string(nil), outcomes...), LearnerControlled: actor == w.CreatorID, CreatedAt: now}
	w.LearningContext.Guidance.Version++
	w.LearningContext.Guidance.Entries = append(w.LearningContext.Guidance.Entries, e)
	w.UpdatedAt = now
	w.Events = append(w.Events, Event{ID: entryID, Kind: "learning.guidance." + kind, ActorID: actor, Role: actorKind, Detail: entryID, CreatedAt: now})
	return w, s.write(w)
}

func (s *Store) SetLearningAgent(id, learner, agentID, state, guidedBy string, expected int) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.read(id)
	if err != nil {
		return Workspace{}, err
	}
	if w.LearningContext == nil || learner != w.CreatorID || w.LearningContext.Guidance.Version != expected {
		return Workspace{}, ErrConflict
	}
	if state == "active" && agentID == "" {
		return Workspace{}, ErrInvalid
	}
	if state != "active" && state != "paused" && state != "revoked" {
		return Workspace{}, ErrInvalid
	}
	if state != "active" && (w.LearningContext.Guidance.AgentID == "" || agentID != w.LearningContext.Guidance.AgentID) {
		return Workspace{}, ErrInvalid
	}
	now := s.now()
	w.LearningContext.Guidance.Version++
	w.LearningContext.Guidance.AgentID = agentID
	w.LearningContext.Guidance.AgentState = state
	w.LearningContext.Guidance.AgentGuidedBy = guidedBy
	w.UpdatedAt = now
	w.Events = append(w.Events, Event{Kind: "learning.agent." + state, ActorID: learner, Role: "learner", Detail: agentID, CreatedAt: now})
	return w, s.write(w)
}

// PublishLearningGuidanceToAgent makes authorization and response publication
// one linearizable read operation. A concurrent pause, revoke, or control
// change either completes first and denies this read, or waits until the
// already-authorized response has been published.
func (s *Store) PublishLearningGuidanceToAgent(id, agentID string, publish func(LearningGuidance) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.read(id)
	if err != nil {
		return err
	}
	if w.LearningContext == nil || w.LearningContext.Guidance.AgentID != agentID || w.LearningContext.Guidance.AgentState != "active" || w.Control.PrincipalKind != "approved_agent" || w.Control.PrincipalID != agentID || w.Control.Mode != "guide" || !w.Control.ExpiresAt.After(s.now()) {
		return ErrControl
	}
	return publish(w.LearningContext.Guidance)
}

type ReasoningContext struct {
	AssessmentID      string                     `json:"assessment_id"`
	AssessmentVersion int                        `json:"assessment_version"`
	DesignProposalID  string                     `json:"design_proposal_id,omitempty"`
	DesignVersion     int                        `json:"design_version,omitempty"`
	Revision          string                     `json:"revision"`
	ExplanationID     string                     `json:"explanation_id,omitempty"`
	ConclusionEntryID string                     `json:"conclusion_entry_id,omitempty"`
	Items             []ReasoningItem            `json:"items"`
	Acknowledgements  []ReasoningAcknowledgement `json:"acknowledgements,omitempty"`
}
type ReasoningItem struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Summary string `json:"summary"`
	Status  string `json:"status"`
}
type ReasoningAcknowledgement struct {
	RepositoryID   string `json:"repository_id"`
	OwnerID        string `json:"owner_id"`
	AcknowledgedBy string `json:"acknowledged_by"`
	Note           string `json:"note,omitempty"`
}

func randomID(size int) (string, error) {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *Store) Join(id, actor, focus, path string) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.read(id)
	if err != nil {
		return Workspace{}, err
	}
	now := s.now()
	found := false
	for i := range w.Presence {
		if w.Presence[i].ActorID == actor {
			w.Presence[i].Focus, w.Presence[i].Path, w.Presence[i].SeenAt = focus, path, now
			found = true
		}
	}
	if !found {
		w.Presence = append(w.Presence, Presence{ActorID: actor, Focus: focus, Path: path, JoinedAt: now, SeenAt: now})
		w.Events = append(w.Events, Event{Kind: "presence.joined", ActorID: actor, Role: "observation", CreatedAt: now})
	}
	w.UpdatedAt = now
	w.LastActivityAt = now
	return w, s.write(w)
}

func (s *Store) Leave(id, actor string) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.read(id)
	if err != nil {
		return Workspace{}, err
	}
	next := w.Presence[:0]
	found := false
	for _, p := range w.Presence {
		if p.ActorID == actor {
			found = true
		} else {
			next = append(next, p)
		}
	}
	w.Presence = next
	if found {
		now := s.now()
		w.UpdatedAt = now
		w.Events = append(w.Events, Event{Kind: "presence.left", ActorID: actor, Role: "observation", CreatedAt: now})
		err = s.write(w)
	}
	return w, err
}

func (s *Store) SetControl(id, actor, principalKind, principalID, mode string, scopes []string, expectedVersion, seconds int) (Workspace, error) {
	control := s.controlLock(id)
	control.Lock()
	defer control.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.read(id)
	if err != nil {
		return Workspace{}, err
	}
	if expectedVersion != w.Control.Version {
		return Workspace{}, ErrControl
	}
	now := s.now()
	if principalID == "" || seconds < 30 || seconds > 3600 {
		return Workspace{}, ErrInvalid
	}
	w.Control = Control{Version: expectedVersion + 1, PrincipalKind: principalKind, PrincipalID: principalID, Mode: mode, Scopes: append([]string(nil), scopes...), GrantedBy: actor, GrantedAt: now, ExpiresAt: now.Add(time.Duration(seconds) * time.Second)}
	w.UpdatedAt = now
	w.Events = append(w.Events, Event{Kind: "control.changed", ActorID: actor, Role: "instruction", Detail: principalKind + ":" + principalID, CreatedAt: now})
	return w, s.write(w)
}

func (s *Store) ReleaseControl(id, actor string, expectedVersion int) (Workspace, error) {
	return s.ReleaseControlAs(id, actor, actor, expectedVersion)
}

func (s *Store) ReleaseControlAs(id, principal, actor string, expectedVersion int) (Workspace, error) {
	control := s.controlLock(id)
	control.Lock()
	defer control.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.read(id)
	if err != nil {
		return Workspace{}, err
	}
	now := s.now()
	if expectedVersion != w.Control.Version || w.Control.PrincipalID != principal || !w.Control.ExpiresAt.After(now) {
		return Workspace{}, ErrControl
	}
	w.Control = Control{Version: expectedVersion + 1, Mode: "observe", Scopes: []string{}, GrantedBy: actor, GrantedAt: now, ExpiresAt: now}
	w.UpdatedAt = now
	w.Events = append(w.Events, Event{Kind: "control.changed", ActorID: actor, Role: "instruction", CreatedAt: now})
	return w, s.write(w)
}

func (s *Store) AddMessage(id, actor, body string) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.read(id)
	if err != nil {
		return Workspace{}, err
	}
	mid, err := randomID(12)
	if err != nil {
		return Workspace{}, err
	}
	now := s.now()
	w.Messages = append(w.Messages, Message{ID: mid, ActorID: actor, Body: body, CreatedAt: now})
	if len(w.Messages) > 200 {
		w.Messages = w.Messages[len(w.Messages)-200:]
	}
	w.UpdatedAt = now
	w.LastActivityAt = now
	w.Events = append(w.Events, Event{ID: mid, Kind: "discussion.message", ActorID: actor, Role: "instruction", Detail: mid, CreatedAt: now})
	return w, s.write(w)
}

func (s *Store) AddContributionHelp(id, actor string, entry ContributionHelpEntry, expected int) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.read(id)
	if err != nil {
		return Workspace{}, err
	}
	if w.ContributorContext == nil || w.ContributorContext.Help.Version != expected {
		return Workspace{}, ErrConflict
	}
	if entry.Body == "" || entry.DecisionOwner == "" || (entry.Kind != "agent_action" && (entry.AgentID != "" || entry.Action != "")) {
		return Workspace{}, ErrInvalid
	}
	entry.ID, err = randomID(12)
	if err != nil {
		return Workspace{}, err
	}
	now := s.now()
	entry.ActorID, entry.CreatedAt = actor, now
	if entry.Status == "" {
		entry.Status = "open"
	}
	w.ContributorContext.Help.Entries = append(w.ContributorContext.Help.Entries, entry)
	w.ContributorContext.Help.Version++
	w.UpdatedAt, w.LastActivityAt = now, now
	detail := entry.ID
	if entry.Action != "" {
		detail += ":" + entry.Action
	}
	w.Events = append(w.Events, Event{ID: entry.ID, Kind: "contribution.help." + entry.Kind, ActorID: actor, Role: map[string]string{"agent_action": "execution", "advice": "instruction"}[entry.Kind], Detail: detail, CreatedAt: now})
	return w, s.write(w)
}

func (s *Store) ResolveContributionHelp(id, actor, entryID, status string, expected int) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.read(id)
	if err != nil {
		return Workspace{}, err
	}
	if w.ContributorContext == nil || w.ContributorContext.Help.Version != expected {
		return Workspace{}, ErrConflict
	}
	now := s.now()
	found := false
	for i := range w.ContributorContext.Help.Entries {
		if w.ContributorContext.Help.Entries[i].ID == entryID && w.ContributorContext.Help.Entries[i].Status == "open" {
			w.ContributorContext.Help.Entries[i].Status = status
			w.ContributorContext.Help.Entries[i].ResolvedAt = &now
			found = true
		}
	}
	if !found {
		return Workspace{}, ErrInvalid
	}
	w.ContributorContext.Help.Version++
	w.UpdatedAt = now
	w.Events = append(w.Events, Event{Kind: "contribution.help.resolved", ActorID: actor, Role: "authorship", Detail: entryID + ":" + status, CreatedAt: now})
	return w, s.write(w)
}

func (s *Store) SetMentorAvailability(id, actor, status, note string, hours, expected int) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.read(id)
	if err != nil {
		return Workspace{}, err
	}
	if w.ContributorContext == nil || w.ContributorContext.Help.Version != expected {
		return Workspace{}, ErrConflict
	}
	now := s.now()
	next := MentorAvailability{MentorID: actor, Status: status, ResponseHours: hours, Note: note, UpdatedAt: now}
	found := false
	for i := range w.ContributorContext.Help.Availability {
		if w.ContributorContext.Help.Availability[i].MentorID == actor {
			w.ContributorContext.Help.Availability[i] = next
			found = true
		}
	}
	if !found {
		w.ContributorContext.Help.Availability = append(w.ContributorContext.Help.Availability, next)
	}
	w.ContributorContext.Help.Version++
	w.UpdatedAt = now
	w.Events = append(w.Events, Event{Kind: "contribution.mentor.availability", ActorID: actor, Role: "authorship", Detail: status, CreatedAt: now})
	return w, s.write(w)
}

func (s *Store) SetContributionState(id, actor, state, reason string, expected int) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.read(id)
	if err != nil {
		return Workspace{}, err
	}
	if w.ContributorContext == nil || w.ContributorContext.Help.Version != expected {
		return Workspace{}, ErrConflict
	}
	now := s.now()
	w.ContributorContext.Help.State = state
	w.ContributorContext.Help.StateReason = reason
	w.ContributorContext.Help.Version++
	w.UpdatedAt = now
	w.Events = append(w.Events, Event{Kind: "contribution." + state, ActorID: actor, Role: "authorship", Detail: reason, CreatedAt: now})
	return w, s.write(w)
}

func (w Workspace) CanControl(actor, scope string, now time.Time) bool {
	if (w.Control.PrincipalKind != "human" && w.Control.PrincipalKind != "approved_agent") || w.Control.PrincipalID != actor || !w.Control.ExpiresAt.After(now) {
		return false
	}
	if w.Control.Mode != "execute" && !(w.Control.Mode == "edit" && scope == "files") {
		return false
	}
	for _, v := range w.Control.Scopes {
		if v == scope {
			return true
		}
	}
	return false
}

// WithControl serializes the final live-lease check and mutation execution with
// control transfer. Once admitted, a mutation finishes before a takeover can
// publish; a request that lost control before admission fails closed.
func (s *Store) WithControl(id, actor, scope string, operation func(Workspace) error) error {
	control := s.controlLock(id)
	control.Lock()
	defer control.Unlock()
	s.mu.Lock()
	w, err := s.read(id)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	if !w.CanControl(actor, scope, s.now()) {
		return ErrControl
	}
	return operation(w)
}

func (s *Store) RecordCommand(id string, outcome CommandOutcome) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.read(id)
	if err != nil {
		return Workspace{}, err
	}
	outcomeID, err := randomID(12)
	if err != nil {
		return Workspace{}, err
	}
	outcome.ID = outcomeID
	provenance, err := s.readOrSeedProvenance(id, w)
	if err != nil {
		return Workspace{}, err
	}
	provenance.Commands = append(provenance.Commands, outcome)
	if err = s.writeProvenance(id, provenance); err != nil {
		return Workspace{}, err
	}
	w.Commands = append(w.Commands, outcome)
	if w.LearningContext != nil {
		hours := outcome.CompletedAt.Sub(outcome.StartedAt).Hours()
		if hours > 0 {
			w.LearningContext.Cost += hours * (w.Definition.Resources.CPUs*0.04 + float64(w.Definition.Resources.MemoryMB)/1024*0.01)
		}
	}
	if len(w.Commands) > 100 {
		w.Commands = w.Commands[len(w.Commands)-100:]
	}
	w.UpdatedAt = s.now()
	w.LastActivityAt = w.UpdatedAt
	w.Events = append(w.Events, Event{Kind: "command.completed", ActorID: outcome.ActorID, Role: "execution", Detail: outcome.ID, CreatedAt: w.UpdatedAt})
	err = s.write(w)
	return w, err
}
func (s *Store) RecordChange(id string, change Change) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.read(id)
	if err != nil {
		return Workspace{}, err
	}
	provenance, err := s.readOrSeedProvenance(id, w)
	if err != nil {
		return Workspace{}, err
	}
	provenance.Changes = append(provenance.Changes, change)
	if err = s.writeProvenance(id, provenance); err != nil {
		return Workspace{}, err
	}
	w.Changes = append(w.Changes, change)
	if len(w.Changes) > 200 {
		w.Changes = w.Changes[len(w.Changes)-200:]
	}
	w.UpdatedAt = s.now()
	w.LastActivityAt = w.UpdatedAt
	w.Events = append(w.Events, Event{Kind: "file.changed", ActorID: change.ActorID, Role: "authorship", Detail: change.Path, CreatedAt: w.UpdatedAt})
	err = s.write(w)
	return w, err
}

type Store struct {
	root       string
	mu         sync.Mutex
	controlsMu sync.Mutex
	controls   map[string]*sync.Mutex
	now        func() time.Time
}

type provenanceRecord struct {
	Changes  []Change         `json:"changes"`
	Commands []CommandOutcome `json:"commands"`
}

func (s *Store) provenancePath(id string) string {
	return filepath.Join(s.root, "provenance", id+".json")
}
func (s *Store) readProvenance(id string) (provenanceRecord, error) {
	b, err := os.ReadFile(s.provenancePath(id))
	if errors.Is(err, os.ErrNotExist) {
		return provenanceRecord{}, nil
	}
	if err != nil {
		return provenanceRecord{}, err
	}
	var value provenanceRecord
	if json.Unmarshal(b, &value) != nil {
		return provenanceRecord{}, ErrInvalid
	}
	return value, nil
}
func (s *Store) readOrSeedProvenance(id string, w Workspace) (provenanceRecord, error) {
	_, statErr := os.Stat(s.provenancePath(id))
	value, err := s.readProvenance(id)
	if err != nil {
		return value, err
	}
	if errors.Is(statErr, os.ErrNotExist) {
		value.Changes = append([]Change(nil), w.Changes...)
		value.Commands = append([]CommandOutcome(nil), w.Commands...)
	}
	return value, nil
}
func (s *Store) writeProvenance(id string, value provenanceRecord) error {
	dir := filepath.Dir(s.provenancePath(id))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".provenance-")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(b)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err = os.Rename(name, s.provenancePath(id)); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

func New(root string) (*Store, error) {
	if root == "" {
		return nil, ErrInvalid
	}
	if err := os.MkdirAll(filepath.Join(root, "runtime"), 0700); err != nil {
		return nil, err
	}
	return &Store{root: root, controls: map[string]*sync.Mutex{}, now: func() time.Time { return time.Now().UTC() }}, nil
}
func (s *Store) controlLock(id string) *sync.Mutex {
	s.controlsMu.Lock()
	defer s.controlsMu.Unlock()
	if s.controls[id] == nil {
		s.controls[id] = &sync.Mutex{}
	}
	return s.controls[id]
}
func (s *Store) RuntimePath(id string) string { return filepath.Join(s.root, "runtime", id) }

// ClaimDecisionExperiment serializes identical workspace launches across
// processes. The caller holds the returned release through provisioning so an
// exact retry either creates the one workspace or reuses its running result.
func (s *Store) ClaimDecisionExperiment(repositoryID, commitID, creatorID, decisionID, alternativeID string) (Workspace, bool, func(), error) {
	claim, err := os.OpenFile(filepath.Join(s.root, ".decision-experiment-launch.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return Workspace{}, false, nil, err
	}
	if err = syscall.Flock(int(claim.Fd()), syscall.LOCK_EX); err != nil {
		claim.Close()
		return Workspace{}, false, nil, err
	}
	release := func() { _ = syscall.Flock(int(claim.Fd()), syscall.LOCK_UN); _ = claim.Close() }
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.root)
	if err != nil {
		release()
		return Workspace{}, false, nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		workspace, readErr := s.readName(entry.Name())
		if readErr != nil {
			release()
			return Workspace{}, false, nil, readErr
		}
		if workspace.RepositoryID == repositoryID && workspace.CommitID == commitID && workspace.CreatorID == creatorID && workspace.Source.Kind == "decision_experiment" && workspace.Source.DecisionID == decisionID && workspace.Source.AlternativeID == alternativeID && workspace.State == "running" {
			release()
			return workspace, true, func() {}, nil
		}
	}
	return Workspace{}, false, release, nil
}

// ClaimConflictLaunch gives a caller-stable launch exactly one durable
// workspace, including across an ambiguous provisioning response.
func (s *Store) ClaimConflictLaunch(repositoryID, pullID, launchID string) (Workspace, bool, func(), error) {
	claim, err := os.OpenFile(filepath.Join(s.root, ".conflict-launch.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return Workspace{}, false, nil, err
	}
	if err = syscall.Flock(int(claim.Fd()), syscall.LOCK_EX); err != nil {
		_ = claim.Close()
		return Workspace{}, false, nil, err
	}
	release := func() { _ = syscall.Flock(int(claim.Fd()), syscall.LOCK_UN); _ = claim.Close() }
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.root)
	if err != nil {
		release()
		return Workspace{}, false, nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		item, readErr := s.readName(entry.Name())
		if readErr != nil {
			release()
			return Workspace{}, false, nil, readErr
		}
		if item.RepositoryID == repositoryID && item.Source.PullRequestID == pullID && item.Source.ConflictLaunchID == launchID {
			return item, true, release, nil
		}
	}
	return Workspace{}, false, release, nil
}

func (w Workspace) HasParticipant(actor string) bool {
	if actor == w.CreatorID {
		return true
	}
	for _, participant := range w.Participants {
		if participant.PrincipalID == actor && participant.Status == "accepted" {
			return true
		}
	}
	return false
}

func (s *Store) Invite(id, actor, principalKind, principalID, role string) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.read(id)
	if err != nil {
		return Workspace{}, err
	}
	if w.CreatorID != actor || w.ConflictContext == nil || principalID == "" {
		return Workspace{}, ErrInvalid
	}
	for _, p := range w.Participants {
		if p.PrincipalKind == principalKind && p.PrincipalID == principalID {
			return Workspace{}, ErrConflict
		}
	}
	status := "pending"
	if principalKind == "approved_agent" {
		status = "accepted"
	}
	w.Participants = append(w.Participants, WorkspaceParticipant{PrincipalKind: principalKind, PrincipalID: principalID, Role: role, Status: status, InvitedBy: actor, InvitedAt: s.now()})
	w.UpdatedAt = s.now()
	if err = s.write(w); err != nil {
		return Workspace{}, err
	}
	return w, nil
}

func (s *Store) RespondInvitation(id, actor, status string) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.read(id)
	if err != nil {
		return Workspace{}, err
	}
	if status != "accepted" && status != "declined" {
		return Workspace{}, ErrInvalid
	}
	for i := range w.Participants {
		if w.Participants[i].PrincipalKind == "human" && w.Participants[i].PrincipalID == actor && w.Participants[i].Status == "pending" {
			now := s.now()
			w.Participants[i].Status, w.Participants[i].RespondedAt = status, &now
			w.UpdatedAt = now
			if err = s.write(w); err != nil {
				return Workspace{}, err
			}
			return w, nil
		}
	}
	return Workspace{}, ErrInvalid
}

func (s *Store) AddConflictQuestion(id string, expected int, question ConflictQuestion) (Workspace, error) {
	control := s.controlLock(id)
	control.Lock()
	defer control.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.read(id)
	if err != nil {
		return Workspace{}, err
	}
	if w.ConflictContext == nil || w.ConflictContext.Version != expected {
		return Workspace{}, ErrConflict
	}
	question.ID, err = randomID(12)
	if err != nil {
		return Workspace{}, err
	}
	question.CreatedAt = s.now()
	w.ConflictContext.Questions = append(w.ConflictContext.Questions, question)
	w.ConflictContext.Version++
	w.UpdatedAt = question.CreatedAt
	w.Events = append(w.Events, Event{ID: question.ID, Kind: "conflict.question", ActorID: question.Authorship.ActorID, Role: "instruction", Detail: question.ID, CreatedAt: question.CreatedAt})
	return w, s.write(w)
}

func (s *Store) AnswerConflictQuestion(id, questionID string, expected int, answer ConflictAnswer) (Workspace, error) {
	control := s.controlLock(id)
	control.Lock()
	defer control.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.read(id)
	if err != nil {
		return Workspace{}, err
	}
	if w.ConflictContext == nil || w.ConflictContext.Version != expected {
		return Workspace{}, ErrConflict
	}
	now, found := s.now(), false
	for i := range w.ConflictContext.Questions {
		if w.ConflictContext.Questions[i].ID == questionID && w.ConflictContext.Questions[i].Answer == nil {
			answer.CreatedAt = now
			w.ConflictContext.Questions[i].Answer = &answer
			found = true
		}
	}
	if !found {
		return Workspace{}, ErrInvalid
	}
	w.ConflictContext.Version++
	w.UpdatedAt = now
	w.Events = append(w.Events, Event{Kind: "conflict.question.answered", ActorID: answer.Authorship.ActorID, Role: "authorship", Detail: questionID, CreatedAt: now})
	return w, s.write(w)
}

func (s *Store) AddConflictResolution(id string, expected int, resolution ConflictResolution) (Workspace, error) {
	control := s.controlLock(id)
	control.Lock()
	defer control.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.read(id)
	if err != nil {
		return Workspace{}, err
	}
	if w.ConflictContext == nil || w.ConflictContext.Version != expected {
		return Workspace{}, ErrConflict
	}
	resolution.ID, err = randomID(12)
	if err != nil {
		return Workspace{}, err
	}
	resolution.CreatedAt, resolution.State = s.now(), "proposed"
	w.ConflictContext.Resolutions = append(w.ConflictContext.Resolutions, resolution)
	w.ConflictContext.Version++
	w.UpdatedAt = resolution.CreatedAt
	w.Events = append(w.Events, Event{ID: resolution.ID, Kind: "conflict.resolution.proposed", ActorID: resolution.Authorship.ActorID, Role: "authorship", Detail: resolution.ID, CreatedAt: resolution.CreatedAt})
	return w, s.write(w)
}

func (s *Store) AddConflictCheckpoint(id string, expected int, principal string, controlVersion int, checkpoint ConflictCheckpoint) (Workspace, error) {
	control := s.controlLock(id)
	control.Lock()
	defer control.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.read(id)
	if err != nil {
		return Workspace{}, err
	}
	if w.ConflictContext == nil || w.ConflictContext.Version != expected {
		return Workspace{}, ErrConflict
	}
	if w.State != "running" || w.Control.Version != controlVersion || !w.CanControl(principal, "commands", s.now()) {
		return Workspace{}, ErrControl
	}
	checkpoint.ID, err = randomID(12)
	if err != nil {
		return Workspace{}, err
	}
	checkpoint.CreatedAt = s.now()
	for i := range checkpoint.Criteria {
		checkpoint.Criteria[i].ID, err = randomID(12)
		if err != nil {
			return Workspace{}, err
		}
	}
	// Staleness is append-only metadata on the old proof. The proof and its
	// historical decisions are never removed; consumers can see exactly which
	// changed input stopped each criterion from being current.
	for i := range w.ConflictContext.Checkpoints {
		old := &w.ConflictContext.Checkpoints[i]
		for j := range old.Criteria {
			criterion := &old.Criteria[j]
			if old.SourceRevision != checkpoint.SourceRevision && (criterion.Origin == "source" || criterion.Origin == "both") {
				criterion.InvalidatedBy = appendUnique(criterion.InvalidatedBy, "source_revision")
			}
			if old.TargetRevision != checkpoint.TargetRevision && (criterion.Origin == "target" || criterion.Origin == "both") {
				criterion.InvalidatedBy = appendUnique(criterion.InvalidatedBy, "target_revision")
			}
			if old.DependencyRevision != checkpoint.DependencyRevision {
				criterion.InvalidatedBy = appendUnique(criterion.InvalidatedBy, "dependency_revision")
			}
			if old.PolicyRevision != checkpoint.PolicyRevision {
				criterion.InvalidatedBy = appendUnique(criterion.InvalidatedBy, "policy_revision")
			}
		}
	}
	w.ConflictContext.Checkpoints = append(w.ConflictContext.Checkpoints, checkpoint)
	w.ConflictContext.Version++
	w.UpdatedAt = checkpoint.CreatedAt
	w.Events = append(w.Events, Event{ID: checkpoint.ID, Kind: "conflict.checkpoint.created", ActorID: checkpoint.CreatedBy.ActorID, Role: "verification", Detail: checkpoint.CandidateCommitID, CreatedAt: checkpoint.CreatedAt})
	return w, s.write(w)
}

func appendUnique(values []string, value string) []string {
	if slices.Contains(values, value) {
		return values
	}
	return append(values, value)
}

func (s *Store) DecideConflictCheckpoint(id, checkpointID, criterionID string, expected int, decision ConflictCheckpointDecision) (Workspace, error) {
	control := s.controlLock(id)
	control.Lock()
	defer control.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.read(id)
	if err != nil {
		return Workspace{}, err
	}
	if w.ConflictContext == nil || w.ConflictContext.Version != expected {
		return Workspace{}, ErrConflict
	}
	now, found := s.now(), false
	for i := range w.ConflictContext.Checkpoints {
		checkpoint := &w.ConflictContext.Checkpoints[i]
		if checkpoint.ID != checkpointID {
			continue
		}
		for _, criterion := range checkpoint.Criteria {
			if criterion.ID == criterionID && slices.Contains(criterion.OwnerIDs, decision.OwnerID) && len(criterion.InvalidatedBy) == 0 {
				found = true
			}
		}
		if found {
			for i := len(checkpoint.Decisions) - 1; i >= 0; i-- {
				old := checkpoint.Decisions[i]
				if old.CriterionID == criterionID && old.OwnerID == decision.OwnerID {
					if old.Decision == decision.Decision {
						return Workspace{}, ErrConflict
					}
					break
				}
			}
			decision.CriterionID, decision.CreatedAt = criterionID, now
			checkpoint.Decisions = append(checkpoint.Decisions, decision)
		}
	}
	if !found {
		return Workspace{}, ErrInvalid
	}
	w.ConflictContext.Version++
	w.UpdatedAt = now
	w.Events = append(w.Events, Event{Kind: "conflict.checkpoint." + decision.Decision, ActorID: decision.OwnerID, Role: "decision", Detail: criterionID, CreatedAt: now})
	return w, s.write(w)
}

// ReserveConflictPublication freezes an idempotent publication request before
// Git or pull-request state is changed. Identical retries reconcile the same
// record; changed reuse fails closed.
func (s *Store) ReserveConflictPublication(id string, expected int, publication ConflictPublicationRecord) (Workspace, ConflictPublicationRecord, error) {
	control := s.controlLock(id)
	control.Lock()
	defer control.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.read(id)
	if err != nil {
		return Workspace{}, ConflictPublicationRecord{}, err
	}
	if w.ConflictContext == nil {
		return Workspace{}, ConflictPublicationRecord{}, ErrInvalid
	}
	for _, existing := range w.ConflictContext.Publications {
		if existing.ID != publication.ID {
			continue
		}
		if existing.CheckpointID != publication.CheckpointID || existing.Mode != publication.Mode || existing.RepositoryID != publication.RepositoryID || existing.Branch != publication.Branch || existing.ExpectedBranchTip != publication.ExpectedBranchTip || existing.PublishedBy != publication.PublishedBy {
			return Workspace{}, ConflictPublicationRecord{}, ErrConflict
		}
		return w, existing, nil
	}
	if w.ConflictContext.Version != expected {
		return Workspace{}, ConflictPublicationRecord{}, ErrConflict
	}
	found := false
	for _, checkpoint := range w.ConflictContext.Checkpoints {
		if checkpoint.ID != publication.CheckpointID {
			continue
		}
		found = true
		for _, criterion := range checkpoint.Criteria {
			if criterion.State != "passed" || len(criterion.InvalidatedBy) != 0 {
				return Workspace{}, ConflictPublicationRecord{}, ErrInvalid
			}
			for _, owner := range criterion.OwnerIDs {
				accepted := false
				for _, decision := range checkpoint.Decisions {
					if decision.CriterionID == criterion.ID && decision.OwnerID == owner {
						accepted = decision.Decision == "accepted"
					}
				}
				if !accepted {
					return Workspace{}, ConflictPublicationRecord{}, ErrInvalid
				}
			}
		}
	}
	if !found {
		return Workspace{}, ConflictPublicationRecord{}, ErrInvalid
	}
	now := s.now()
	publication.Status, publication.CreatedAt, publication.UpdatedAt = "publishing", now, now
	w.ConflictContext.Publications = append(w.ConflictContext.Publications, publication)
	w.ConflictContext.Version++
	w.UpdatedAt = now
	w.Events = append(w.Events, Event{ID: publication.ID, Kind: "conflict.publication.reserved", ActorID: publication.PublishedBy.ActorID, Role: "publication", Detail: publication.Branch, CreatedAt: now})
	return w, publication, s.write(w)
}

func (s *Store) CompleteConflictPublication(id, publicationID, commitID, pullID, status, action string) (Workspace, error) {
	control := s.controlLock(id)
	control.Lock()
	defer control.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.read(id)
	if err != nil {
		return Workspace{}, err
	}
	for i := range w.ConflictContext.Publications {
		p := &w.ConflictContext.Publications[i]
		if p.ID != publicationID {
			continue
		}
		if p.Status == "published" && p.PublishedCommitID == commitID && p.PullRequestID == pullID {
			return w, nil
		}
		if p.Status != "publishing" && p.Status != "branch_published" {
			return Workspace{}, ErrConflict
		}
		p.PublishedCommitID, p.PullRequestID, p.Status, p.ActionRequired, p.UpdatedAt = commitID, pullID, status, action, s.now()
		w.ConflictContext.Version++
		w.UpdatedAt = p.UpdatedAt
		w.Events = append(w.Events, Event{ID: p.ID, Kind: "conflict.publication." + status, ActorID: p.PublishedBy.ActorID, Role: "publication", Detail: commitID, CreatedAt: p.UpdatedAt})
		return w, s.write(w)
	}
	return Workspace{}, ErrNotFound
}

// PublishConflictBranch serializes the final approval recheck with the Git ref
// compare-and-swap supplied by the route. Owners cannot withdraw while that
// exact accepted publication is crossing the repository boundary.
func (s *Store) PublishConflictBranch(id, publicationID, commitID string, publish func() error) (Workspace, error) {
	control := s.controlLock(id)
	control.Lock()
	defer control.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.read(id)
	if err != nil {
		return Workspace{}, err
	}
	var publication *ConflictPublicationRecord
	for i := range w.ConflictContext.Publications {
		if w.ConflictContext.Publications[i].ID == publicationID {
			publication = &w.ConflictContext.Publications[i]
			break
		}
	}
	if publication == nil {
		return Workspace{}, ErrNotFound
	}
	if publication.Status == "branch_published" && publication.PublishedCommitID == commitID {
		return w, nil
	}
	if publication.Status != "publishing" {
		return Workspace{}, ErrConflict
	}
	for _, checkpoint := range w.ConflictContext.Checkpoints {
		if checkpoint.ID != publication.CheckpointID {
			continue
		}
		for _, criterion := range checkpoint.Criteria {
			if criterion.State != "passed" || len(criterion.InvalidatedBy) != 0 {
				return Workspace{}, ErrInvalid
			}
			for _, owner := range criterion.OwnerIDs {
				accepted := false
				for _, decision := range checkpoint.Decisions {
					if decision.CriterionID == criterion.ID && decision.OwnerID == owner {
						accepted = decision.Decision == "accepted"
					}
				}
				if !accepted {
					return Workspace{}, ErrInvalid
				}
			}
		}
	}
	if err = publish(); err != nil {
		return Workspace{}, err
	}
	now := s.now()
	publication.Status, publication.PublishedCommitID, publication.UpdatedAt = "branch_published", commitID, now
	w.ConflictContext.Version++
	w.UpdatedAt = now
	w.Events = append(w.Events, Event{ID: publication.ID, Kind: "conflict.publication.branch_published", ActorID: publication.PublishedBy.ActorID, Role: "publication", Detail: commitID, CreatedAt: now})
	return w, s.write(w)
}

// ActConflictResolution persists a recoverable intent before touching runtime
// content. An identical retry inspects that intent and either performs the edit
// or finalizes it, so a failure after the external mutation cannot make the
// durable record claim that no action happened.
func (s *Store) ActConflictResolution(id, resolutionID string, expected int, applying bool, principal string, authorship ConflictAuthorship, inspect func(Workspace, ConflictResolution) (string, string, error), operation func(Workspace, ConflictResolution, string, string) error) (Workspace, error) {
	control := s.controlLock(id)
	control.Lock()
	defer control.Unlock()
	s.mu.Lock()
	w, err := s.read(id)
	if err != nil {
		s.mu.Unlock()
		return Workspace{}, err
	}
	if w.ConflictContext == nil || w.State != "running" || !w.CanControl(principal, "files", s.now()) {
		s.mu.Unlock()
		return Workspace{}, ErrControl
	}
	var resolution *ConflictResolution
	for i := range w.ConflictContext.Resolutions {
		if w.ConflictContext.Resolutions[i].ID == resolutionID {
			resolution = &w.ConflictContext.Resolutions[i]
		}
	}
	if resolution == nil {
		s.mu.Unlock()
		return Workspace{}, ErrInvalid
	}
	if (applying && resolution.State == "applied") || (!applying && resolution.State == "undone") {
		s.mu.Unlock()
		return w, nil
	}
	pendingState := "applying"
	if !applying {
		pendingState = "undoing"
	}
	if resolution.State != pendingState {
		if w.ConflictContext.Version != expected || (applying && resolution.State != "proposed") || (!applying && resolution.State != "applied") {
			s.mu.Unlock()
			return Workspace{}, ErrConflict
		}
		snapshot := *resolution
		s.mu.Unlock()
		beforeContent, beforeDigest, inspectErr := inspect(w, snapshot)
		if inspectErr != nil {
			return Workspace{}, inspectErr
		}
		expectedBefore, intendedDigest := snapshot.ExpectedSHA256, digestContent(snapshot.ProposedContent)
		if !applying {
			expectedBefore, intendedDigest = snapshot.AppliedSHA256, digestContent(snapshot.PreviousContent)
		}
		if beforeDigest != expectedBefore {
			return Workspace{}, ErrConflict
		}
		s.mu.Lock()
		w, err = s.read(id)
		if err != nil {
			s.mu.Unlock()
			return Workspace{}, err
		}
		for i := range w.ConflictContext.Resolutions {
			if w.ConflictContext.Resolutions[i].ID == resolutionID {
				r := &w.ConflictContext.Resolutions[i]
				r.State = pendingState
				r.PendingAction = &ConflictPendingAction{Kind: map[bool]string{true: "apply", false: "undo"}[applying], BeforeContent: beforeContent, BeforeSHA256: beforeDigest, IntendedSHA256: intendedDigest, Authorship: authorship, StartedAt: s.now(), PrincipalID: principal, ControlVersion: w.Control.Version}
			}
		}
		w.ConflictContext.Version++
		w.UpdatedAt = s.now()
		w.Events = append(w.Events, Event{Kind: "conflict.resolution." + pendingState, ActorID: authorship.ActorID, Role: "instruction", Detail: resolutionID, CreatedAt: w.UpdatedAt})
		if err = s.write(w); err != nil {
			s.mu.Unlock()
			return Workspace{}, err
		}
	}
	// From this point the intent is durable. Reload it even on the first request
	// so retries and uninterrupted execution share exactly the same path.
	w, err = s.read(id)
	if err != nil {
		s.mu.Unlock()
		return Workspace{}, err
	}
	for i := range w.ConflictContext.Resolutions {
		if w.ConflictContext.Resolutions[i].ID == resolutionID {
			resolution = &w.ConflictContext.Resolutions[i]
		}
	}
	snapshot := *resolution
	s.mu.Unlock()
	currentContent, currentDigest, err := inspect(w, snapshot)
	if err != nil {
		return Workspace{}, err
	}
	if currentDigest == snapshot.PendingAction.BeforeSHA256 {
		content := snapshot.ProposedContent
		if !applying {
			content = snapshot.PreviousContent
		}
		if err = operation(w, snapshot, content, snapshot.PendingAction.BeforeSHA256); err != nil {
			return Workspace{}, err
		}
		currentContent, currentDigest, err = inspect(w, snapshot)
		if err != nil {
			return Workspace{}, err
		}
	}
	if currentDigest != snapshot.PendingAction.IntendedSHA256 {
		return Workspace{}, ErrConflict
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err = s.read(id)
	if err != nil {
		return Workspace{}, err
	}
	if w.State != "running" || !w.CanControl(snapshot.PendingAction.PrincipalID, "files", s.now()) || w.Control.Version != snapshot.PendingAction.ControlVersion {
		return Workspace{}, ErrControl
	}
	now := s.now()
	for i := range w.ConflictContext.Resolutions {
		r := &w.ConflictContext.Resolutions[i]
		if r.ID != resolutionID {
			continue
		}
		if applying {
			r.State, r.PreviousContent, r.AppliedSHA256, r.AppliedBy, r.AppliedAt = "applied", r.PendingAction.BeforeContent, currentDigest, &r.PendingAction.Authorship, &now
		} else {
			r.State, r.UndoneAt = "undone", &now
		}
		r.PendingAction = nil
	}
	w.ConflictContext.Version++
	w.UpdatedAt = now
	actionID := resolutionID + ":" + map[bool]string{true: "apply", false: "undo"}[applying]
	change := Change{Path: snapshot.Path, SHA256: currentDigest, Size: len(currentContent), ActorID: snapshot.PendingAction.Authorship.ActorID, CreatedAt: snapshot.PendingAction.StartedAt, ResolutionActionID: actionID}
	provenance, provenanceErr := s.readOrSeedProvenance(id, w)
	if provenanceErr != nil {
		return Workspace{}, provenanceErr
	}
	if !hasResolutionChange(provenance.Changes, actionID) {
		provenance.Changes = append(provenance.Changes, change)
	}
	if provenanceErr = s.writeProvenance(id, provenance); provenanceErr != nil {
		return Workspace{}, provenanceErr
	}
	if !hasResolutionChange(w.Changes, actionID) {
		w.Changes = append(w.Changes, change)
	}
	if len(w.Changes) > 200 {
		w.Changes = w.Changes[len(w.Changes)-200:]
	}
	kind := "conflict.resolution.applied"
	if !applying {
		kind = "conflict.resolution.undone"
	}
	w.Events = append(w.Events, Event{Kind: kind, ActorID: snapshot.PendingAction.Authorship.ActorID, Role: "authorship", Detail: resolutionID, CreatedAt: now})
	return w, s.write(w)
}

func digestContent(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}
func hasResolutionChange(changes []Change, id string) bool {
	for _, change := range changes {
		if change.ResolutionActionID == id {
			return true
		}
	}
	return false
}

func (s *Store) Create(w Workspace, definitionBytes []byte) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.create(w, definitionBytes)
}

func (s *Store) CreateLearning(w Workspace, definitionBytes []byte) (Workspace, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	release, err := s.learningFileLock("launch")
	if err != nil {
		return Workspace{}, false, err
	}
	defer release()
	if w.Source.LearningRequestID == "" || len(w.Source.LearningRequestID) > 200 {
		return Workspace{}, false, ErrInvalid
	}
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return Workspace{}, false, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		prior, readErr := s.readName(entry.Name())
		if readErr != nil || prior.CreatorID != w.CreatorID || prior.Source.Kind != "learning_exercise" || prior.Source.LearningRequestID != w.Source.LearningRequestID {
			continue
		}
		if prior.RepositoryID != w.RepositoryID || prior.CommitID != w.CommitID || prior.Source.LearningPathwaySlug != w.Source.LearningPathwaySlug || prior.Source.LearningPathwayVersion != w.Source.LearningPathwayVersion || prior.Source.LearningModuleID != w.Source.LearningModuleID || prior.Source.LearningExerciseID != w.Source.LearningExerciseID || prior.LearningContext == nil || w.LearningContext == nil || prior.LearningContext.ReproducibilitySHA256 != w.LearningContext.ReproducibilitySHA256 {
			return Workspace{}, false, ErrRequestChanged
		}
		return prior, true, nil
	}
	created, err := s.create(w, definitionBytes)
	return created, false, err
}

// ReconcileLearningProvisioning serializes setup for a retained attempt. A
// concurrent retry waits for the active launch and observes its completed
// state; after interruption or restart, the retry performs the missing setup.
func (s *Store) ReconcileLearningProvisioning(id string, operation func() ([]SetupStep, bool)) (Workspace, bool, error) {
	control := s.controlLock(id)
	control.Lock()
	defer control.Unlock()
	release, err := s.learningFileLock("provision-" + id)
	if err != nil {
		return Workspace{}, false, err
	}
	defer release()
	s.mu.Lock()
	w, err := s.read(id)
	s.mu.Unlock()
	if err != nil {
		return Workspace{}, false, err
	}
	if w.Source.Kind != "learning_exercise" || w.LearningContext == nil {
		return Workspace{}, false, ErrInvalid
	}
	if w.State != "provisioning" {
		return w, false, nil
	}
	steps, failure := operation()
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err = s.read(id)
	if err != nil {
		return Workspace{}, true, err
	}
	if w.State != "provisioning" {
		return w, false, nil
	}
	w.Setup = steps
	if w.LearningContext != nil {
		for _, step := range steps {
			hours := step.CompletedAt.Sub(step.StartedAt).Hours()
			if hours > 0 {
				w.LearningContext.Cost += hours * (w.Definition.Resources.CPUs*0.04 + float64(w.Definition.Resources.MemoryMB)/1024*0.01)
			}
		}
	}
	if failure {
		w.State = "failed"
	} else {
		w.State = "running"
	}
	w.UpdatedAt = s.now()
	w.Events = append(w.Events, Event{Kind: "setup_completed", ActorID: w.CreatorID, CreatedAt: w.UpdatedAt})
	return w, true, s.write(w)
}

func (s *Store) learningFileLock(name string) (func(), error) {
	dir := filepath.Join(s.root, ".learning-locks")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(dir, name+".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		_ = file.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
	}, nil
}

func (s *Store) create(w Workspace, definitionBytes []byte) (Workspace, error) {
	if w.RepositoryID == "" || w.CommitID == "" || w.CreatorID == "" {
		return Workspace{}, ErrInvalid
	}
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		return Workspace{}, e
	}
	w.ID = hex.EncodeToString(b)
	sum := sha256.Sum256(definitionBytes)
	w.DefinitionSHA256 = hex.EncodeToString(sum[:])
	now := s.now()
	w.CreatedAt, w.UpdatedAt = now, now
	w.LastActivityAt = now
	if w.Policy.MaxRuntimeHours > 0 {
		expires := now.Add(time.Duration(w.Policy.MaxRuntimeHours) * time.Hour)
		w.ExpiresAt = &expires
	}
	w.State = "provisioning"
	w.Control = Control{Version: 1, PrincipalKind: "human", PrincipalID: w.CreatorID, Mode: "execute", Scopes: []string{"files", "commands", "lifecycle"}, GrantedBy: w.CreatorID, GrantedAt: now, ExpiresAt: now.Add(time.Hour)}
	w.Presence = []Presence{}
	w.Messages = []Message{}
	w.Events = []Event{{Kind: "created", ActorID: w.CreatorID, Role: "authorship", CreatedAt: now}}
	if w.ConflictContext != nil && w.ConflictContext.Version == 0 {
		w.ConflictContext.Version = 1
		w.ConflictContext.Questions = []ConflictQuestion{}
		w.ConflictContext.Resolutions = []ConflictResolution{}
		w.ConflictContext.Checkpoints = []ConflictCheckpoint{}
	}
	if err := os.MkdirAll(s.RuntimePath(w.ID), 0700); err != nil {
		return Workspace{}, err
	}
	if err := s.writeProvenance(w.ID, provenanceRecord{}); err != nil {
		return Workspace{}, err
	}
	if err := s.write(w); err != nil {
		return Workspace{}, err
	}
	return w, nil
}
func (s *Store) Complete(id string, steps []SetupStep, failure bool) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, e := s.read(id)
	if e != nil {
		return Workspace{}, e
	}
	w.Setup = steps
	if w.LearningContext != nil {
		for _, step := range steps {
			hours := step.CompletedAt.Sub(step.StartedAt).Hours()
			if hours > 0 {
				w.LearningContext.Cost += hours * (w.Definition.Resources.CPUs*0.04 + float64(w.Definition.Resources.MemoryMB)/1024*0.01)
			}
		}
	}
	if failure {
		w.State = "failed"
	} else {
		w.State = "running"
	}
	w.UpdatedAt = s.now()
	w.Events = append(w.Events, Event{Kind: "setup_completed", ActorID: w.CreatorID, CreatedAt: w.UpdatedAt})
	e = s.write(w)
	return w, e
}

// Stop removes compute authority while retaining the workspace record,
// checkpoints, provenance ledger, and any already-published Git objects.
func (s *Store) Stop(id, actor, reason, state string) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopLocked(id, actor, reason, state)
}

func (s *Store) stopLocked(id, actor, reason, state string) (Workspace, error) {
	w, err := s.read(id)
	if err != nil {
		return Workspace{}, err
	}
	if w.State == "stopped" || w.State == "expired" {
		return Workspace{}, ErrConflict
	}
	if state != "stopped" && state != "expired" {
		return Workspace{}, ErrInvalid
	}
	now := s.now()
	w.State, w.StoppedAt, w.StoppedBy, w.StopReason = state, &now, actor, reason
	w.Control = Control{Version: w.Control.Version + 1, Mode: "observe", GrantedBy: actor, GrantedAt: now, ExpiresAt: now}
	w.Presence = []Presence{}
	w.UpdatedAt = now
	w.Events = append(w.Events, Event{Kind: state, ActorID: actor, Role: "instruction", Detail: reason, CreatedAt: now})
	return w, s.write(w)
}

// StopControlled serializes external compute teardown and terminal state
// publication with suspend/resume. The global store lock is deliberately not
// held while teardown runs, so unrelated workspaces remain available.
func (s *Store) StopControlled(id, actor, reason, state string, teardown func() error) (Workspace, error) {
	control := s.controlLock(id)
	control.Lock()
	defer control.Unlock()
	s.mu.Lock()
	w, err := s.read(id)
	s.mu.Unlock()
	if err != nil {
		return Workspace{}, err
	}
	if w.State == "stopped" || w.State == "expired" {
		return Workspace{}, ErrConflict
	}
	if err = teardown(); err != nil {
		return Workspace{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stopLocked(id, actor, reason, state)
}

func (s *Store) AnnounceExpiry(id, actor string, at time.Time, reason string) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.read(id)
	if err != nil {
		return Workspace{}, err
	}
	if !at.After(s.now()) {
		return Workspace{}, ErrInvalid
	}
	now := s.now()
	w.ExpiresAt, w.ExpiryAnnouncedAt, w.UpdatedAt = &at, &now, now
	w.Events = append(w.Events, Event{Kind: "expiry.announced", ActorID: actor, Role: "instruction", Detail: reason, CreatedAt: now})
	return w, s.write(w)
}

func (s *Store) MarkRebuild(id string, reasons []string) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, err := s.read(id)
	if err != nil {
		return Workspace{}, err
	}
	w.RebuildRequired, w.RebuildReasons, w.UpdatedAt = len(reasons) > 0, append([]string(nil), reasons...), s.now()
	return w, s.write(w)
}
func (s *Store) Get(id string) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read(id)
}
func (s *Store) List(actor string) ([]Workspace, error) {
	items, err := s.ListAll()
	if err != nil {
		return nil, err
	}
	out := []Workspace{}
	for _, w := range items {
		if w.CreatorID == actor {
			out = append(out, w)
		}
	}
	return out, nil
}
func (s *Store) ListAll() ([]Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, e := os.ReadDir(s.root)
	if e != nil {
		return nil, e
	}
	out := []Workspace{}
	for _, x := range entries {
		if x.IsDir() {
			continue
		}
		w, e := s.readName(x.Name())
		if e == nil {
			out = append(out, w)
		}
	}
	return out, nil
}
func (s *Store) Transition(id, actor, expectedFoundation, target string) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.transition(id, actor, actor, expectedFoundation, target, false)
}

func (s *Store) TransitionControlled(id, actor, expectedFoundation, target string) (Workspace, error) {
	control := s.controlLock(id)
	control.Lock()
	defer control.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.transition(id, actor, actor, expectedFoundation, target, true)
}

func (s *Store) TransitionControlledAs(id, principal, actor, expectedFoundation, target string) (Workspace, error) {
	control := s.controlLock(id)
	control.Lock()
	defer control.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.transition(id, principal, actor, expectedFoundation, target, true)
}

func (s *Store) transition(id, principal, actor, expectedFoundation, target string, requireControl bool) (Workspace, error) {
	w, e := s.read(id)
	if e != nil {
		return Workspace{}, e
	}
	if expectedFoundation == "" || expectedFoundation != w.DefinitionSHA256 {
		return Workspace{}, ErrConflict
	}
	if (target == "suspended" && w.State != "running") || (target == "running" && w.State != "suspended") {
		return Workspace{}, ErrConflict
	}
	if requireControl && !w.CanControl(principal, "lifecycle", s.now()) {
		return Workspace{}, ErrControl
	}
	now := s.now()
	switch target {
	case "suspended":
		w.SuspendedAt = &now
	case "running":
		if _, e = os.Stat(s.RuntimePath(id)); e != nil {
			return Workspace{}, ErrConflict
		}
		w.ResumedAt = &now
	default:
		return Workspace{}, ErrInvalid
	}
	w.State = target
	w.UpdatedAt = now
	w.Events = append(w.Events, Event{Kind: target, ActorID: actor, CreatedAt: now})
	e = s.write(w)
	return w, e
}
func (s *Store) read(id string) (Workspace, error) {
	if len(id) != 32 {
		return Workspace{}, ErrNotFound
	}
	return s.readName(id + ".json")
}
func (s *Store) readName(name string) (Workspace, error) {
	body, e := os.ReadFile(filepath.Join(s.root, name))
	if os.IsNotExist(e) {
		return Workspace{}, ErrNotFound
	}
	if e != nil {
		return Workspace{}, e
	}
	var w Workspace
	if json.Unmarshal(body, &w) != nil {
		return Workspace{}, ErrNotFound
	}
	if w.Policy.Version == 0 {
		w.Policy = DefaultPolicy()
		w.PolicyVersion = w.Policy.Version
		w.PolicyScope = "platform-default"
	}
	if w.LastActivityAt.IsZero() {
		w.LastActivityAt = w.UpdatedAt
	}
	// Presence is a renewable observation, not authority. A disconnected client
	// disappears deterministically even when it cannot publish an explicit leave.
	cutoff := s.now().Add(-20 * time.Second)
	active := w.Presence[:0]
	for _, presence := range w.Presence {
		if presence.SeenAt.After(cutoff) {
			active = append(active, presence)
		}
	}
	w.Presence = active
	return w, nil
}
func (s *Store) write(w Workspace) error {
	body, e := json.MarshalIndent(w, "", "  ")
	if e != nil {
		return e
	}
	tmp := filepath.Join(s.root, "."+w.ID+".tmp")
	if e = os.WriteFile(tmp, body, 0600); e != nil {
		return e
	}
	if e = os.Rename(tmp, filepath.Join(s.root, w.ID+".json")); e != nil {
		return e
	}
	d, e := os.Open(s.root)
	if e != nil {
		return e
	}
	defer d.Close()
	if e = d.Sync(); e != nil {
		return fmt.Errorf("sync workspace store: %w", e)
	}
	return nil
}
