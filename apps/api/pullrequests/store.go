// Package pullrequests stores repository-scoped requests to merge one branch into another.
package pullrequests

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/acceptance"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/accessibilityassessments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/accessibilitydelivery"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/changesessions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/durableschemas"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/localization"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/performanceevidence"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/previews"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/privacychecks"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/serviceobjectives"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

var (
	ErrNotFound            = errors.New("pull request not found")
	ErrInvalid             = errors.New("invalid pull request")
	ErrBranchNotFound      = errors.New("pull request branch not found")
	ErrSourceChanged       = errors.New("pull request source branch must be synchronized")
	ErrDurabilityUncertain = errors.New("pull request is visible but durability is uncertain")
	ErrNotReady            = errors.New("pull request is not ready to merge")
)

const (
	Open   = "open"
	Closed = "closed"
	Merged = "merged"
)

const (
	Approved         = "approved"
	ChangesRequested = "changes_requested"
	Withdrawn        = "withdrawn"
)

type PullRequest struct {
	ID                       string                  `json:"id"`
	RepositoryID             string                  `json:"repository_id"`
	SourceRepositoryID       string                  `json:"source_repository_id"`
	AuthorID                 string                  `json:"author_id"`
	FederatedAuthor          string                  `json:"federated_author,omitempty"`
	FederatedContributionID  string                  `json:"federated_contribution_id,omitempty"`
	Title                    string                  `json:"title"`
	Body                     string                  `json:"body"`
	SourceBranch             string                  `json:"source_branch"`
	TargetBranch             string                  `json:"target_branch"`
	SourceCommitID           string                  `json:"source_commit_id"`
	TargetCommitID           string                  `json:"target_commit_id"`
	ProposalID               *string                 `json:"proposal_id"`
	TaskID                   *string                 `json:"task_id,omitempty"`
	TaskSessionID            *string                 `json:"task_session_id,omitempty"`
	TaskRunID                *string                 `json:"task_run_id,omitempty"`
	TaskCommitIDs            []string                `json:"task_commit_ids,omitempty"`
	TaskEvidence             *TaskReviewEvidence     `json:"task_evidence,omitempty"`
	DurableMigration         *DurableMigrationReview `json:"durable_migration,omitempty"`
	WorkspaceID              string                  `json:"workspace_id,omitempty"`
	WorkspaceCheckpointID    string                  `json:"workspace_checkpoint_id,omitempty"`
	WorkspaceContributorIDs  []string                `json:"workspace_contributor_ids,omitempty"`
	WorkspaceCommandIDs      []string                `json:"workspace_command_ids,omitempty"`
	ContributionEvidence     *ContributionEvidence   `json:"contribution_evidence,omitempty"`
	DeliveryTeamID           string                  `json:"delivery_team_id,omitempty"`
	DeliveryIntegrationID    string                  `json:"delivery_integration_id,omitempty"`
	DeliveryStreamID         string                  `json:"delivery_stream_id,omitempty"`
	DeliveryIntegrationOrder int                     `json:"delivery_integration_order,omitempty"`
	TaskStatePending         string                  `json:"task_state_pending,omitempty"`
	Status                   string                  `json:"status"`
	MaintainerEditsAllowed   bool                    `json:"maintainer_edits_allowed"`
	CreatedAt                time.Time               `json:"created_at"`
	UpdatedAt                time.Time               `json:"updated_at"`
	ClosedAt                 *time.Time              `json:"closed_at"`
	ClosedBy                 *string                 `json:"closed_by"`
	MergedAt                 *time.Time              `json:"merged_at"`
	MergedBy                 *string                 `json:"merged_by"`
	MergeCommitID            *string                 `json:"merge_commit_id"`
	QueuedAt                 *time.Time              `json:"queued_at,omitempty"`
	QueuedBy                 *string                 `json:"queued_by,omitempty"`
	QueueRank                string                  `json:"queue_rank,omitempty"`
	QueuePaused              bool                    `json:"queue_paused,omitempty"`
	QueueActions             []QueueAction           `json:"queue_actions,omitempty"`
	QueueFinalizationPending bool                    `json:"queue_finalization_pending,omitempty"`
	QueueFinalizedAt         *time.Time              `json:"queue_finalized_at,omitempty"`
	IntegrationCandidates    []IntegrationCandidate  `json:"integration_candidates,omitempty"`
	mergeIntent              *mergeIntent
}

// DurableMigrationReview exposes the exact non-sensitive coexistence contract
// on an ordinary linked pull without making it a source of review or merge authority.
type DurableMigrationReview struct {
	SchemaID      string                      `json:"schema_id"`
	MigrationID   string                      `json:"migration_id"`
	WorkID        string                      `json:"work_id"`
	StepID        string                      `json:"step_id"`
	Kind          string                      `json:"kind"`
	DependencyIDs []string                    `json:"dependency_ids"`
	Contract      durableschemas.WorkContract `json:"contract"`
}

// ContributionEvidence keeps the intent and support behind guided newcomer
// work attached to the ordinary pull. It is review context, never authority.
type ContributionEvidence struct {
	OpportunityID       string                `json:"opportunity_id"`
	OpportunityVersion  int                   `json:"opportunity_version"`
	PathwayVersion      int                   `json:"pathway_version"`
	UpstreamRevision    string                `json:"upstream_revision"`
	SetupEvidence       []ContributionSetup   `json:"setup_evidence"`
	MentorGuidanceIDs   []string              `json:"mentor_guidance_ids"`
	AgentAssistanceIDs  []string              `json:"agent_assistance_ids"`
	AcceptanceCriteria  []string              `json:"acceptance_criteria"`
	SatisfiedCriteria   []string              `json:"satisfied_criteria"`
	ProjectRequirements []ContributionFinding `json:"project_requirements"`
	CoachingNeeds       []ContributionFinding `json:"coaching_needs"`
}

type ContributionSetup struct {
	Command  string `json:"command"`
	State    string `json:"state"`
	ExitCode int    `json:"exit_code"`
}

type ContributionFinding struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Fix     string `json:"fix"`
}

type GuidedContributionCreation struct {
	WorkspaceID  string
	CheckpointID string
	Contributors []string
	CommandIDs   []string
	Evidence     ContributionEvidence
}

// LinkDeliveryIntegration retains a pull's place in a distributed outcome
// without changing any ordinary review, check, queue, or merge rule.
func (s *Store) LinkDeliveryIntegration(repositoryID, pullID, teamID, integrationID, streamID string, order int) (PullRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return PullRequest{}, err
	}
	defer unlock()
	p, err := s.read(repositoryID, pullID)
	if err != nil {
		return p, err
	}
	if !validID(teamID) || !validID(integrationID) || strings.TrimSpace(streamID) == "" || order < 1 {
		return p, ErrInvalid
	}
	if p.Status != Open {
		return p, ErrNotReady
	}
	if p.DeliveryTeamID != "" && (p.DeliveryTeamID != teamID || p.DeliveryIntegrationID != integrationID || p.DeliveryStreamID != streamID || p.DeliveryIntegrationOrder != order) {
		return p, ErrInvalid
	}
	p.DeliveryTeamID, p.DeliveryIntegrationID, p.DeliveryStreamID, p.DeliveryIntegrationOrder = teamID, integrationID, streamID, order
	_, err = s.write(p)
	return p, err
}

// TaskReviewEvidence freezes the execution and intent offered to reviewers;
// it conveys provenance, never extra review or merge authority.
type TaskReviewEvidence struct {
	BaseRevision       string                 `json:"base_revision"`
	AssignmentID       string                 `json:"assignment_id"`
	AgentID            string                 `json:"agent_id"`
	InitiatorID        string                 `json:"initiator_id"`
	Mandate            string                 `json:"mandate"`
	OrganizationID     string                 `json:"organization_id,omitempty"`
	MandateID          string                 `json:"mandate_id,omitempty"`
	OpportunityID      string                 `json:"opportunity_id,omitempty"`
	EvidenceRevision   string                 `json:"evidence_revision,omitempty"`
	Reasoning          []ReviewReasoningItem  `json:"reasoning"`
	CompletionCriteria string                 `json:"completion_criteria"`
	Outcome            changesessions.Outcome `json:"outcome"`
}

type ReviewReasoningItem struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Summary string `json:"summary"`
	Status  string `json:"status"`
}

type recoveryIdentity struct {
	DeliveryTeamID        string
	DeliveryIntegrationID string
	DeliveryStreamID      string
	DeliveryOrder         int
}

// LinkWorkspace records collaboration provenance without changing ordinary
// pull revision, review, check, or integration behavior.
func (s *Store) LinkWorkspace(repositoryID, pullID, workspaceID, checkpointID string, contributors, commands []string) (PullRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return PullRequest{}, err
	}
	defer unlock()
	p, err := s.read(repositoryID, pullID)
	if err != nil {
		return p, err
	}
	if !validID(workspaceID) || !validID(checkpointID) {
		return p, ErrInvalid
	}
	for _, id := range contributors {
		if !validID(id) {
			return p, ErrInvalid
		}
	}
	for _, id := range commands {
		if strings.TrimSpace(id) == "" || len(id) > 128 {
			return p, ErrInvalid
		}
	}
	p.WorkspaceID, p.WorkspaceCheckpointID = workspaceID, checkpointID
	p.WorkspaceContributorIDs = append([]string(nil), contributors...)
	p.WorkspaceCommandIDs = append([]string(nil), commands...)
	_, err = s.write(p)
	return p, err
}

func (s *Store) LinkContributionEvidence(repositoryID, pullID string, evidence ContributionEvidence) (PullRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return PullRequest{}, err
	}
	defer unlock()
	p, err := s.read(repositoryID, pullID)
	if err != nil {
		return p, err
	}
	if !validID(evidence.OpportunityID) || evidence.OpportunityVersion < 1 || evidence.PathwayVersion < 1 || !validCommitID(evidence.UpstreamRevision) || len(evidence.AcceptanceCriteria) == 0 {
		return p, ErrInvalid
	}
	p.ContributionEvidence = &evidence
	_, err = s.write(p)
	return p, err
}

// QueueAction is the durable, attributable operating history for one entry.
type QueueAction struct {
	Action    string    `json:"action"`
	ActorID   string    `json:"actor_id"`
	CreatedAt time.Time `json:"created_at"`
}

// IntegrationQueueEntry is the branch-level shared coordination projection.
type IntegrationQueueEntry struct {
	Position    int                       `json:"position"`
	PullRequest PullRequest               `json:"pull_request"`
	Candidate   *IntegrationCandidateView `json:"candidate,omitempty"`
	State       string                    `json:"state"`
	Blockers    []ReadinessBlocker        `json:"blockers"`
	NextAction  string                    `json:"next_action"`
}

type IntegrationQueueView struct {
	Branch  string                  `json:"branch"`
	Entries []IntegrationQueueEntry `json:"entries"`
}

// IntegrationCandidate is an immutable prospective merge. CommitID names a
// synthetic two-parent commit whose first parent is BaseCommitID and whose
// second parent is SourceCommitID. Its lifecycle is derived from check runs.
type IntegrationCandidate struct {
	ID               string                 `json:"id"`
	SourceCommitID   string                 `json:"source_commit_id"`
	BaseCommitID     string                 `json:"base_commit_id"`
	CommitID         string                 `json:"commit_id"`
	RequiredChecks   []string               `json:"required_checks"`
	CheckDefinitions []checkruns.Definition `json:"check_definitions"`
	CreatedAt        time.Time              `json:"created_at"`
	SupersededAt     *time.Time             `json:"superseded_at,omitempty"`
	SupersededReason string                 `json:"superseded_reason,omitempty"`
}

type IntegrationCandidateView struct {
	IntegrationCandidate
	State  string          `json:"state"`
	Checks []checkruns.Run `json:"checks"`
}

// UpdatePolicy changes the source owner's explicit grant allowing target
// repository participants to help update an independently owned branch.
func (s *Store) UpdatePolicy(repositoryID, id string, allowed bool) (PullRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return PullRequest{}, err
	}
	defer unlock()
	p, err := s.read(repositoryID, id)
	if err != nil {
		return PullRequest{}, err
	}
	if p.Status != Open || p.SourceRepositoryID == p.RepositoryID {
		return PullRequest{}, ErrNotReady
	}
	p.MaintainerEditsAllowed = allowed
	p.UpdatedAt = s.now().Truncate(time.Microsecond)
	if committed, err := s.write(p); err != nil {
		if committed {
			return p, fmt.Errorf("%w: %v", ErrDurabilityUncertain, err)
		}
		return PullRequest{}, err
	}
	return p, nil
}

// Close preserves the review record while stopping synchronization, review,
// checks, and merge mutations. Authorization belongs to the HTTP boundary.
func (s *Store) Close(repositoryID, id, actorID string) (PullRequest, error) {
	if !validID(actorID) {
		return PullRequest{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return PullRequest{}, err
	}
	defer unlock()
	p, err := s.read(repositoryID, id)
	if err != nil {
		return PullRequest{}, err
	}
	if p.Status != Open || p.mergeIntent != nil {
		return PullRequest{}, ErrNotReady
	}
	now := s.now().Truncate(time.Microsecond)
	p.Status, p.ClosedAt, p.ClosedBy, p.UpdatedAt = Closed, &now, &actorID, now
	p.MaintainerEditsAllowed = false
	p.QueuedAt, p.QueueRank = nil, ""
	p.QueuedBy = nil
	if p.TaskID != nil {
		p.TaskStatePending = "closed"
	}
	if committed, err := s.write(p); err != nil {
		if committed {
			return p, fmt.Errorf("%w: %v", ErrDurabilityUncertain, err)
		}
		return PullRequest{}, err
	}
	return p, nil
}

// ConfirmTaskState clears cross-store repair intent only after the linked task
// reflects this pull's durable lifecycle state.
func (s *Store) ConfirmTaskState(repositoryID, id, expected string) (PullRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return PullRequest{}, err
	}
	defer unlock()
	p, err := s.read(repositoryID, id)
	if err != nil {
		return PullRequest{}, err
	}
	if expected == "" || p.TaskStatePending != expected {
		return PullRequest{}, ErrSourceChanged
	}
	p.TaskStatePending = ""
	if committed, err := s.write(p); err != nil {
		if committed {
			return p, fmt.Errorf("%w: %v", ErrDurabilityUncertain, err)
		}
		return PullRequest{}, err
	}
	return p, nil
}

type mergeIntent struct {
	CommitID string    `json:"commit_id"`
	MergerID string    `json:"merger_id"`
	MergedAt time.Time `json:"merged_at"`
}

type pullRequestRecord struct {
	PullRequest
	MergeIntent *mergeIntent `json:"merge_intent,omitempty"`
}

type Commit struct {
	ID      string         `json:"id"`
	TreeID  string         `json:"tree_id"`
	Parents []string       `json:"parent_ids"`
	Headers []CommitHeader `json:"headers"`
	Message string         `json:"message"`
}

type CommitHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type FileChange struct {
	Path    string  `json:"path"`
	Status  string  `json:"status"`
	OldID   *string `json:"old_id"`
	NewID   *string `json:"new_id"`
	OldMode *string `json:"old_mode"`
	NewMode *string `json:"new_mode"`
}

type Comment struct {
	ID            string    `json:"id"`
	PullRequestID string    `json:"pull_request_id"`
	AuthorID      string    `json:"author_id"`
	Body          string    `json:"body"`
	Revision      string    `json:"revision"`
	CreatedAt     time.Time `json:"created_at"`
}

// Review is one participant's current decision. ReviewedCommitID identifies
// the live source-branch tip the participant evaluated; Stale is derived when
// the record is read and is never trusted as durable state.
type Review struct {
	ID               string    `json:"id"`
	PullRequestID    string    `json:"pull_request_id"`
	ReviewerID       string    `json:"reviewer_id"`
	Decision         string    `json:"decision"`
	ReviewedCommitID string    `json:"reviewed_commit_id"`
	Stale            bool      `json:"stale"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type BranchState struct {
	Branch           string  `json:"branch"`
	SnapshotCommitID string  `json:"snapshot_commit_id"`
	CurrentCommitID  *string `json:"current_commit_id"`
	State            string  `json:"state"`
}

type ReadinessBlocker struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type CheckRequirement struct {
	Name     string  `json:"name"`
	Status   string  `json:"status"`
	CommitID *string `json:"commit_id,omitempty"`
	RunID    *string `json:"run_id,omitempty"`
}

// MergeReadiness is derived entirely from durable pull-request, review, and
// Git state. It is never persisted and computing it does not modify the
// repository.
type MergeReadiness struct {
	Mergeable               bool                                   `json:"mergeable"`
	CanMerge                bool                                   `json:"can_merge"`
	RequiredApprovals       int                                    `json:"required_approvals"`
	Approvals               int                                    `json:"approvals"`
	EvaluatedCommitID       string                                 `json:"evaluated_commit_id"`
	RequiredChecks          []CheckRequirement                     `json:"required_checks"`
	Source                  BranchState                            `json:"source"`
	Target                  BranchState                            `json:"target"`
	HasConflicts            bool                                   `json:"has_conflicts"`
	Blockers                []ReadinessBlocker                     `json:"blockers"`
	IntegrationQueue        *repositories.IntegrationQueuePolicy   `json:"integration_queue,omitempty"`
	CanEnqueue              bool                                   `json:"can_enqueue"`
	PreviewAcceptance       *acceptance.Evaluation                 `json:"preview_acceptance,omitempty"`
	PerformanceRequirements []performanceevidence.MergeRequirement `json:"performance_requirements"`
	AccessibilityReadiness  *accessibilitydelivery.Readiness       `json:"accessibility_readiness,omitempty"`
	PrivacyReadiness        *privacychecks.Readiness               `json:"privacy_readiness,omitempty"`
	LocalizationReadiness   *localization.LocaleReadiness          `json:"localization_readiness,omitempty"`
	ReliabilityReadiness    []serviceobjectives.DeliveryEvaluation `json:"reliability_readiness"`
	DesignReadiness         any                                    `json:"design_readiness,omitempty"`
	QualityConfidence       any                                    `json:"quality_confidence,omitempty"`
	SecurityConfidence      any                                    `json:"security_confidence,omitempty"`
	AssuranceImpact         any                                    `json:"assurance_impact,omitempty"`
}

type commentRecord struct {
	Comments []Comment `json:"comments"`
}

type reviewRecord struct {
	Reviews []Review `json:"reviews"`
}

type Store struct {
	root           string
	git            *storage.Store
	mu             sync.Mutex
	now            func() time.Time
	directorySync  func(string) error
	rootSync       func(string) error
	checkRuns      *checkruns.Store
	queueFinalizer func(PullRequest) error
	requirements   interface {
		RequiredChecks(string, string) ([]string, error)
		LockRequiredChecks() (func(), error)
		IntegrationQueuePolicy(string, string) (repositories.IntegrationQueuePolicy, error)
	}
	acceptance *acceptance.Store
	previews   interface {
		List(string, string, string) ([]previews.Preview, error)
		WithAudienceAdmission(func() error) error
	}
	performance              *performanceevidence.Store
	accessibilityDelivery    *accessibilitydelivery.Store
	accessibilityAssessments *accessibilityassessments.Store
	privacyChecks            *privacychecks.Store
	localization             *localization.Store
	reliability              *serviceobjectives.Store
	designReadiness          func(PullRequest, []FileChange) (any, []ReadinessBlocker, error)
	qualityConfidence        func(PullRequest, []FileChange) (any, []ReadinessBlocker, error)
	securityConfidence       func(PullRequest, []FileChange) (any, []ReadinessBlocker, error)
	assuranceImpact          func(PullRequest, []FileChange) (any, []ReadinessBlocker, error)
}

// ConfigureDesignReadiness adds an evidence-only policy projection to ordinary
// readiness. Its blockers remain subject to every existing review and check.
func (s *Store) ConfigureDesignReadiness(fn func(PullRequest, []FileChange) (any, []ReadinessBlocker, error)) {
	s.designReadiness = fn
}

// ConfigureQualityConfidence makes ordinary merge and queue readiness consume
// the same revision-exact quality matrix exposed by the quality API.
func (s *Store) ConfigureQualityConfidence(fn func(PullRequest, []FileChange) (any, []ReadinessBlocker, error)) {
	s.qualityConfidence = fn
}

// ConfigureSecurityConfidence makes merge and integration-queue readiness
// consume the same revision-exact security matrix exposed by delivery APIs.
func (s *Store) ConfigureSecurityConfidence(fn func(PullRequest, []FileChange) (any, []ReadinessBlocker, error)) {
	s.securityConfidence = fn
}
func (s *Store) ConfigureAssuranceImpact(fn func(PullRequest, []FileChange) (any, []ReadinessBlocker, error)) {
	s.assuranceImpact = fn
}

func (s *Store) ConfigurePerformanceEvidence(store *performanceevidence.Store) { s.performance = store }
func (s *Store) ConfigureAccessibilityDelivery(delivery *accessibilitydelivery.Store, assessments *accessibilityassessments.Store) {
	s.accessibilityDelivery, s.accessibilityAssessments = delivery, assessments
}
func (s *Store) ConfigurePrivacyChecks(store *privacychecks.Store)   { s.privacyChecks = store }
func (s *Store) ConfigureLocalization(store *localization.Store)     { s.localization = store }
func (s *Store) ConfigureReliability(store *serviceobjectives.Store) { s.reliability = store }

func (s *Store) ConfigurePreviewAcceptance(a *acceptance.Store, p *previews.Store) {
	s.acceptance = a
	if p != nil {
		s.previews = p
	}
}

func (s *Store) ConfigureRequiredChecks(requirements interface {
	RequiredChecks(string, string) ([]string, error)
	LockRequiredChecks() (func(), error)
	IntegrationQueuePolicy(string, string) (repositories.IntegrationQueuePolicy, error)
}, runs *checkruns.Store) {
	s.requirements, s.checkRuns = requirements, runs
}

// ConfigureQueueFinalizer connects an automatically completed pull request to
// application-level collaboration side effects without moving those stores
// into the pull-request persistence package.
func (s *Store) ConfigureQueueFinalizer(finalize func(PullRequest) error) {
	s.queueFinalizer = finalize
}

func New(root string, git *storage.Store) (*Store, error) {
	if root == "" || git == nil {
		return nil, errors.New("pull request storage requires metadata and Git storage")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, fmt.Errorf("create pull request store: %w", err)
	}
	return &Store{root: abs, git: git, now: func() time.Time { return time.Now().UTC() }, directorySync: syncDirectory, rootSync: syncDirectory}, nil
}

func (s *Store) Create(repositoryID, authorID, title, body, sourceBranch, targetBranch string, proposalID *string) (PullRequest, error) {
	return s.CreateFrom(repositoryID, repositoryID, authorID, title, body, sourceBranch, targetBranch, proposalID)
}

// CreateFrom snapshots a source branch that may live in a distinct repository.
// Its reachable objects are imported into the target without publishing a ref,
// keeping every later review and merge operation pinned to the adopted commit.
func (s *Store) CreateFrom(repositoryID, sourceRepositoryID, authorID, title, body, sourceBranch, targetBranch string, proposalID *string) (PullRequest, error) {
	return s.createFrom(repositoryID, sourceRepositoryID, authorID, title, body, sourceBranch, targetBranch, "", nil, proposalID, nil, nil, nil, nil, nil, nil)
}

// CreateFederated imports one verified exact revision into ordinary review.
// The synthetic local author is an attribution anchor, never a local login.
func (s *Store) CreateFederated(repositoryID, authorID, federatedAuthor, contributionID, title, body, sourceBranch, targetBranch, sourceCommit string, source *storage.Repository) (PullRequest, error) {
	if source == nil || !validCommitID(sourceCommit) || federatedAuthor == "" || contributionID == "" {
		return PullRequest{}, ErrInvalid
	}
	repository, err := s.git.Open(repositoryID)
	if err != nil {
		return PullRequest{}, err
	}
	targetCommit, err := branchCommit(repository, targetBranch)
	if err != nil {
		return PullRequest{}, err
	}
	if _, err = source.ReadCommit(storage.ObjectID(sourceCommit)); err != nil {
		return PullRequest{}, ErrInvalid
	}
	if err = repository.ImportCommit(source, storage.ObjectID(sourceCommit)); err != nil {
		return PullRequest{}, err
	}
	title, body, err = validatePurpose(title, body)
	if err != nil {
		return PullRequest{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return PullRequest{}, err
	}
	defer unlock()
	existing, _ := s.List(repositoryID)
	for _, p := range existing {
		if p.FederatedContributionID == contributionID {
			return p, nil
		}
	}
	id, err := newID()
	if err != nil {
		return PullRequest{}, err
	}
	now := s.now().Truncate(time.Microsecond)
	p := PullRequest{ID: id, RepositoryID: repositoryID, SourceRepositoryID: repositoryID, AuthorID: authorID, FederatedAuthor: federatedAuthor, FederatedContributionID: contributionID, Title: title, Body: body, SourceBranch: sourceBranch, TargetBranch: targetBranch, SourceCommitID: sourceCommit, TargetCommitID: targetCommit, Status: Open, CreatedAt: now, UpdatedAt: now}
	if err = s.ensureRepositoryDirectory(repositoryID); err != nil {
		return PullRequest{}, err
	}
	if committed, err := s.write(p); err != nil {
		if committed {
			return p, ErrDurabilityUncertain
		}
		return PullRequest{}, err
	}
	return p, nil
}

// AdoptFederatedRevision imports an exact peer-provided descendant and moves
// only the review snapshot. It never creates a local source ref or authority.
func (s *Store) AdoptFederatedRevision(repositoryID, contributionID, prior, next string, source *storage.Repository) (PullRequest, error) {
	if source == nil || !validCommitID(prior) || !validCommitID(next) {
		return PullRequest{}, ErrInvalid
	}
	target, err := s.git.Open(repositoryID)
	if err != nil {
		return PullRequest{}, err
	}
	ancestry, err := source.ListCommitAncestry(storage.ObjectID(next))
	if err != nil {
		return PullRequest{}, ErrInvalid
	}
	found := false
	for _, commit := range ancestry {
		if string(commit.ID) == prior {
			found = true
			break
		}
	}
	if !found {
		return PullRequest{}, ErrInvalid
	}
	if err = target.ImportCommit(source, storage.ObjectID(next)); err != nil {
		return PullRequest{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return PullRequest{}, err
	}
	defer unlock()
	all, err := s.List(repositoryID)
	if err != nil {
		return PullRequest{}, err
	}
	for _, p := range all {
		if p.FederatedContributionID != contributionID {
			continue
		}
		if p.Status != Open || (p.SourceCommitID != prior && p.SourceCommitID != next) {
			return PullRequest{}, ErrSourceChanged
		}
		if p.SourceCommitID == next {
			return p, nil
		}
		p.SourceCommitID, p.UpdatedAt = next, s.now().Truncate(time.Microsecond)
		committed, writeErr := s.write(p)
		if writeErr != nil && committed {
			return p, ErrDurabilityUncertain
		}
		return p, writeErr
	}
	return PullRequest{}, ErrNotFound
}

func (s *Store) CreateGuidedContributionFrom(repositoryID, sourceRepositoryID, authorID, title, body, sourceBranch, targetBranch string, guided GuidedContributionCreation) (PullRequest, error) {
	return s.createFrom(repositoryID, sourceRepositoryID, authorID, title, body, sourceBranch, targetBranch, "", nil, nil, nil, nil, nil, nil, nil, &guided)
}

// FindOrCreateRecovery enforces one review boundary for a deterministic
// repository/source-branch pair under the cross-process pull-store lock.
func (s *Store) FindOrCreateRecovery(repositoryID, authorID, title, body, sourceBranch, targetBranch string) (PullRequest, error) {
	return s.createFrom(repositoryID, repositoryID, authorID, title, body, sourceBranch, targetBranch, "", nil, nil, nil, nil, nil, &recoveryIdentity{}, nil, nil)
}

// FindOrCreateDeliveryIntegration recovers only the exact delivery review. An
// unrelated pull on the same branches remains an independent review boundary.
func (s *Store) FindOrCreateDeliveryIntegration(repositoryID, authorID, title, body, sourceBranch, targetBranch, teamID, integrationID, streamID string, order int) (PullRequest, error) {
	if !validID(teamID) || !validID(integrationID) || strings.TrimSpace(streamID) == "" || order < 1 {
		return PullRequest{}, ErrInvalid
	}
	recovery := &recoveryIdentity{DeliveryTeamID: teamID, DeliveryIntegrationID: integrationID, DeliveryStreamID: streamID, DeliveryOrder: order}
	return s.createFrom(repositoryID, repositoryID, authorID, title, body, sourceBranch, targetBranch, "", nil, nil, nil, nil, nil, recovery, nil, nil)
}

// CreateTaskContribution publishes task-scoped work into ordinary review while
// retaining stable links to the agreed intent and optional execution evidence.
func (s *Store) CreateTaskContribution(repositoryID, authorID, title, body, sourceBranch, targetBranch, expectedSourceCommit string, commitIDs []string, proposalID, taskID *string, sessionID, runID *string) (PullRequest, error) {
	return s.CreateTaskContributionFrom(repositoryID, repositoryID, authorID, title, body, sourceBranch, targetBranch, expectedSourceCommit, commitIDs, proposalID, taskID, sessionID, runID)
}

func (s *Store) CreateTaskContributionFrom(repositoryID, sourceRepositoryID, authorID, title, body, sourceBranch, targetBranch, expectedSourceCommit string, commitIDs []string, proposalID, taskID *string, sessionID, runID *string) (PullRequest, error) {
	return s.CreateTaskContributionFromWithEvidence(repositoryID, sourceRepositoryID, authorID, title, body, sourceBranch, targetBranch, expectedSourceCommit, commitIDs, proposalID, taskID, sessionID, runID, nil)
}

func (s *Store) CreateTaskContributionFromWithEvidence(repositoryID, sourceRepositoryID, authorID, title, body, sourceBranch, targetBranch, expectedSourceCommit string, commitIDs []string, proposalID, taskID *string, sessionID, runID *string, reviewEvidence *TaskReviewEvidence) (PullRequest, error) {
	return s.createFrom(repositoryID, sourceRepositoryID, authorID, title, body, sourceBranch, targetBranch, expectedSourceCommit, commitIDs, proposalID, taskID, sessionID, runID, nil, reviewEvidence, nil)
}

func (s *Store) createFrom(repositoryID, sourceRepositoryID, authorID, title, body, sourceBranch, targetBranch string, expectedSourceCommit string, commitIDs []string, proposalID, taskID, sessionID, runID *string, recovery *recoveryIdentity, reviewEvidence *TaskReviewEvidence, guided *GuidedContributionCreation) (PullRequest, error) {
	if !validID(repositoryID) || !validID(sourceRepositoryID) || !validID(authorID) {
		return PullRequest{}, ErrInvalid
	}
	title, body, err := validatePurpose(title, body)
	if err != nil {
		return PullRequest{}, err
	}
	sourceBranch, targetBranch = strings.TrimSpace(sourceBranch), strings.TrimSpace(targetBranch)
	if sourceBranch == "" || targetBranch == "" || (sourceRepositoryID == repositoryID && sourceBranch == targetBranch) || strings.HasPrefix(sourceBranch, "refs/") || strings.HasPrefix(targetBranch, "refs/") {
		return PullRequest{}, ErrInvalid
	}
	if proposalID != nil && !validID(*proposalID) {
		return PullRequest{}, ErrInvalid
	}
	if (taskID != nil && !validID(*taskID)) || (sessionID != nil && !validID(*sessionID)) || (runID != nil && !validID(*runID)) || (taskID == nil && (sessionID != nil || runID != nil)) {
		return PullRequest{}, ErrInvalid
	}
	for _, commitID := range commitIDs {
		if !validCommitID(commitID) {
			return PullRequest{}, ErrInvalid
		}
	}
	if guided != nil && !validGuidedContribution(*guided) {
		return PullRequest{}, ErrInvalid
	}
	repository, err := s.git.Open(repositoryID)
	if err != nil {
		return PullRequest{}, fmt.Errorf("open Git repository: %w", err)
	}
	sourceRepository, err := s.git.Open(sourceRepositoryID)
	if err != nil {
		return PullRequest{}, fmt.Errorf("open source Git repository: %w", err)
	}
	sourceCommit, err := branchCommit(sourceRepository, sourceBranch)
	if err != nil {
		return PullRequest{}, err
	}
	if expectedSourceCommit != "" && sourceCommit != expectedSourceCommit {
		return PullRequest{}, ErrSourceChanged
	}
	targetCommit, err := branchCommit(repository, targetBranch)
	if err != nil {
		return PullRequest{}, err
	}
	id, err := newID()
	if err != nil {
		return PullRequest{}, err
	}
	now := s.now().Truncate(time.Microsecond)
	if sourceRepositoryID != repositoryID {
		if err := repository.ImportCommit(sourceRepository, storage.ObjectID(sourceCommit)); err != nil {
			return PullRequest{}, fmt.Errorf("import source commit: %w", err)
		}
	}
	if taskID != nil && len(commitIDs) == 0 {
		commitIDs = []string{sourceCommit}
	}
	p := PullRequest{ID: id, RepositoryID: repositoryID, SourceRepositoryID: sourceRepositoryID, AuthorID: authorID, Title: title, Body: body, SourceBranch: sourceBranch, TargetBranch: targetBranch, SourceCommitID: sourceCommit, TargetCommitID: targetCommit, ProposalID: proposalID, TaskID: taskID, TaskSessionID: sessionID, TaskRunID: runID, TaskCommitIDs: append([]string(nil), commitIDs...), TaskEvidence: reviewEvidence, Status: Open, CreatedAt: now, UpdatedAt: now}
	if guided != nil {
		p.WorkspaceID, p.WorkspaceCheckpointID = guided.WorkspaceID, guided.CheckpointID
		p.WorkspaceContributorIDs = append([]string(nil), guided.Contributors...)
		p.WorkspaceCommandIDs = append([]string(nil), guided.CommandIDs...)
		evidence := guided.Evidence
		p.ContributionEvidence = &evidence
	}
	if recovery != nil && recovery.DeliveryTeamID != "" {
		p.DeliveryTeamID, p.DeliveryIntegrationID, p.DeliveryStreamID, p.DeliveryIntegrationOrder = recovery.DeliveryTeamID, recovery.DeliveryIntegrationID, recovery.DeliveryStreamID, recovery.DeliveryOrder
	}
	if taskID != nil {
		p.TaskStatePending = "review"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return PullRequest{}, err
	}
	defer unlock()
	if recovery != nil {
		existing, listErr := s.List(repositoryID)
		if listErr != nil {
			return PullRequest{}, listErr
		}
		for _, candidate := range existing {
			matchesDelivery := recovery.DeliveryTeamID == "" || candidate.DeliveryTeamID == recovery.DeliveryTeamID && candidate.DeliveryIntegrationID == recovery.DeliveryIntegrationID && candidate.DeliveryStreamID == recovery.DeliveryStreamID && candidate.DeliveryIntegrationOrder == recovery.DeliveryOrder
			if candidate.Status == Open && candidate.SourceRepositoryID == sourceRepositoryID && candidate.SourceBranch == sourceBranch && candidate.TargetBranch == targetBranch && matchesDelivery {
				return candidate, nil
			}
		}
	}
	if err := s.ensureRepositoryDirectory(repositoryID); err != nil {
		return PullRequest{}, err
	}
	if committed, err := s.write(p); err != nil {
		if committed {
			return p, fmt.Errorf("%w: %v", ErrDurabilityUncertain, err)
		}
		return PullRequest{}, err
	}
	return p, nil
}

func validGuidedContribution(g GuidedContributionCreation) bool {
	if !validID(g.WorkspaceID) || !validID(g.CheckpointID) || !validID(g.Evidence.OpportunityID) || g.Evidence.OpportunityVersion < 1 || g.Evidence.PathwayVersion < 1 || !validCommitID(g.Evidence.UpstreamRevision) || len(g.Evidence.AcceptanceCriteria) == 0 {
		return false
	}
	for _, id := range g.Contributors {
		if !validID(id) {
			return false
		}
	}
	for _, id := range g.CommandIDs {
		if strings.TrimSpace(id) == "" || len(id) > 128 {
			return false
		}
	}
	return true
}

func branchCommit(repository *storage.Repository, branch string) (string, error) {
	ref, err := repository.ReadReference("refs/heads/" + branch)
	if errors.Is(err, storage.ErrReferenceNotFound) || ref.Symbolic {
		return "", ErrBranchNotFound
	}
	if err != nil {
		return "", fmt.Errorf("read branch %q: %w", branch, err)
	}
	object, err := repository.ReadObject(storage.ObjectID(ref.Target))
	if err != nil {
		return "", fmt.Errorf("read branch %q target: %w", branch, err)
	}
	if object.Type != storage.CommitObject {
		return "", ErrBranchNotFound
	}
	return ref.Target, nil
}

func (s *Store) Get(repositoryID, id string) (PullRequest, error) {
	if !validID(repositoryID) {
		return PullRequest{}, ErrNotFound
	}
	p, err := s.read(repositoryID, id)
	if err != nil {
		return PullRequest{}, err
	}
	if p.RepositoryID != repositoryID {
		return PullRequest{}, ErrNotFound
	}
	return p, nil
}

// SynchronizeSource adopts the source branch's current commit as the next
// reviewable revision of an open pull request. Existing reviews retain the
// commit they evaluated and therefore become stale when the branch advanced.
func (s *Store) SynchronizeSource(repositoryID, id string) (PullRequest, error) {
	return s.SynchronizeSourceAfter(repositoryID, id, nil)
}

// WithSourceRevision runs fn while holding the pull-request mutation lock,
// provided the request is still open at the expected adopted source revision.
// Cross-store revision-pinned publications use this boundary so they cannot
// race source synchronization.
func (s *Store) WithSourceRevision(repositoryID, id, expected string, fn func(PullRequest) error) error {
	if !validID(repositoryID) || !validID(id) {
		return ErrNotFound
	}
	if !validCommitID(expected) || fn == nil {
		return ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()
	p, err := s.read(repositoryID, id)
	if err != nil {
		return err
	}
	if p.Status != Open || p.mergeIntent != nil {
		return ErrNotReady
	}
	if p.SourceCommitID != expected {
		return ErrSourceChanged
	}
	return fn(p)
}

// SynchronizeSourceAfter checks synchronization eligibility and the live
// source tip under the pull-request lock, then invokes before immediately
// before publishing the new snapshot. Callers can durably prepare a related
// mutation without terminalizing it when a merge intent already blocks sync.
func (s *Store) SynchronizeSourceAfter(repositoryID, id string, before func() error) (PullRequest, error) {
	if !validID(repositoryID) || !validID(id) {
		return PullRequest{}, ErrNotFound
	}
	repository, err := s.git.Open(repositoryID)
	if err != nil {
		return PullRequest{}, fmt.Errorf("open Git repository: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return PullRequest{}, err
	}
	defer unlock()
	p, err := s.read(repositoryID, id)
	if err != nil {
		return PullRequest{}, err
	}
	if p.Status != Open || p.mergeIntent != nil {
		return PullRequest{}, ErrNotReady
	}
	sourceRepository, err := s.git.Open(p.SourceRepositoryID)
	if err != nil {
		return PullRequest{}, fmt.Errorf("open source Git repository: %w", err)
	}
	commitID, err := branchCommit(sourceRepository, p.SourceBranch)
	if err != nil {
		return PullRequest{}, err
	}
	if before != nil {
		if err := before(); err != nil {
			return PullRequest{}, err
		}
	}
	if commitID == p.SourceCommitID {
		return p, nil
	}
	if p.SourceRepositoryID != repositoryID {
		if err := repository.ImportCommit(sourceRepository, storage.ObjectID(commitID)); err != nil {
			return PullRequest{}, fmt.Errorf("import source commit: %w", err)
		}
	}
	p.SourceCommitID = commitID
	p.QueuedAt, p.QueuedBy, p.QueueRank = nil, nil, ""
	p.UpdatedAt = s.now().Truncate(time.Microsecond)
	if committed, err := s.write(p); err != nil {
		if committed {
			return p, fmt.Errorf("%w: %v", ErrDurabilityUncertain, err)
		}
		return PullRequest{}, err
	}
	return p, nil
}

func (s *Store) List(repositoryID string) ([]PullRequest, error) {
	if !validID(repositoryID) {
		return nil, ErrNotFound
	}
	entries, err := os.ReadDir(s.repositoryPath(repositoryID))
	if errors.Is(err, os.ErrNotExist) {
		return []PullRequest{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := []PullRequest{}
	for _, entry := range entries {
		id, ok := strings.CutSuffix(entry.Name(), ".json")
		if entry.IsDir() || !ok || !validID(id) {
			continue
		}
		p, err := s.read(repositoryID, id)
		if err != nil {
			return nil, err
		}
		if p.RepositoryID == repositoryID {
			result = append(result, p)
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

// AllowsMaintainerEdit validates a branch-scoped cross-repository grant. The
// callback deliberately rechecks current target participation on every Git
// request, so revocation closes access without changing source ownership.
func (s *Store) AllowsMaintainerEdit(sourceRepositoryID, branch, pullRequestID, actorID string, participant func(string, string) bool) bool {
	if !validID(pullRequestID) {
		return false
	}
	source, err := s.git.Open(sourceRepositoryID)
	if err != nil {
		return false
	}
	branchName, ok := strings.CutPrefix(branch, "refs/heads/")
	if !ok {
		return false
	}
	if _, err := branchCommit(source, branchName); err != nil {
		return false
	}
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() || !validID(entry.Name()) {
			continue
		}
		pulls, err := s.List(entry.Name())
		if err != nil {
			continue
		}
		for _, p := range pulls {
			if p.ID == pullRequestID && p.Status == Open && p.MaintainerEditsAllowed && p.SourceRepositoryID == sourceRepositoryID && "refs/heads/"+p.SourceBranch == branch && participant(p.RepositoryID, actorID) {
				return true
			}
		}
	}
	return false
}

// Commits returns the source commits that are not reachable from the target
// snapshot, in depth-first parent order from the snapshotted source tip.
func (s *Store) Commits(repositoryID, id string) ([]Commit, error) {
	p, err := s.Get(repositoryID, id)
	if err != nil {
		return nil, err
	}
	repository, err := s.git.Open(repositoryID)
	if err != nil {
		return nil, err
	}
	target, err := repository.ListCommitAncestry(storage.ObjectID(p.TargetCommitID))
	if err != nil {
		return nil, err
	}
	excluded := make(map[storage.ObjectID]bool, len(target))
	for _, commit := range target {
		excluded[commit.ID] = true
	}
	source, err := repository.ListCommitAncestry(storage.ObjectID(p.SourceCommitID))
	if err != nil {
		return nil, err
	}
	result := []Commit{}
	for _, commit := range source {
		if excluded[commit.ID] {
			continue
		}
		parents := make([]string, len(commit.Parents))
		for i, parent := range commit.Parents {
			parents[i] = string(parent)
		}
		headers := make([]CommitHeader, len(commit.Headers))
		for i, header := range commit.Headers {
			headers[i] = CommitHeader{Name: header.Name, Value: header.Value}
		}
		result = append(result, Commit{ID: string(commit.ID), TreeID: string(commit.Tree), Parents: parents, Headers: headers, Message: string(commit.Message)})
	}
	return result, nil
}

// Changes compares the complete target and source snapshots by path. Tree
// container entries are omitted; files, symlinks, and gitlinks are included.
func (s *Store) Changes(repositoryID, id string) ([]FileChange, error) {
	p, err := s.Get(repositoryID, id)
	if err != nil {
		return nil, err
	}
	repository, err := s.git.Open(repositoryID)
	if err != nil {
		return nil, err
	}
	oldCommit, err := repository.ReadCommit(storage.ObjectID(p.TargetCommitID))
	if err != nil {
		return nil, err
	}
	newCommit, err := repository.ReadCommit(storage.ObjectID(p.SourceCommitID))
	if err != nil {
		return nil, err
	}
	oldPaths, err := repository.WalkTree(oldCommit.Tree)
	if err != nil {
		return nil, err
	}
	newPaths, err := repository.WalkTree(newCommit.Tree)
	if err != nil {
		return nil, err
	}
	oldFiles, newFiles := map[string]storage.TreeEntry{}, map[string]storage.TreeEntry{}
	for _, entry := range oldPaths {
		if entry.Type != storage.TreeObject {
			oldFiles[entry.Path] = entry.TreeEntry
		}
	}
	for _, entry := range newPaths {
		if entry.Type != storage.TreeObject {
			newFiles[entry.Path] = entry.TreeEntry
		}
	}
	paths := make([]string, 0, len(oldFiles)+len(newFiles))
	seen := map[string]bool{}
	for path := range oldFiles {
		paths = append(paths, path)
		seen[path] = true
	}
	for path := range newFiles {
		if !seen[path] {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	result := []FileChange{}
	for _, path := range paths {
		oldEntry, oldOK := oldFiles[path]
		newEntry, newOK := newFiles[path]
		if oldOK && newOK && oldEntry.ID == newEntry.ID && oldEntry.Mode == newEntry.Mode {
			continue
		}
		change := FileChange{Path: path}
		if oldOK {
			value, mode := string(oldEntry.ID), oldEntry.Mode
			change.OldID, change.OldMode = &value, &mode
		}
		if newOK {
			value, mode := string(newEntry.ID), newEntry.Mode
			change.NewID, change.NewMode = &value, &mode
		}
		switch {
		case !oldOK:
			change.Status = "added"
		case !newOK:
			change.Status = "deleted"
		default:
			change.Status = "modified"
		}
		result = append(result, change)
	}
	return result, nil
}

// CompareCommits returns the path-ordered file delta between two exact commit
// snapshots. Change-session publication uses this to describe only the work
// produced by a run, independently of the pull request's older target.
func (s *Store) CompareCommits(repositoryID, oldCommitID, newCommitID string) ([]FileChange, error) {
	repository, err := s.git.Open(repositoryID)
	if err != nil {
		return nil, err
	}
	oldCommit, err := repository.ReadCommit(storage.ObjectID(oldCommitID))
	if err != nil {
		return nil, err
	}
	newCommit, err := repository.ReadCommit(storage.ObjectID(newCommitID))
	if err != nil {
		return nil, err
	}
	return compareTrees(repository, oldCommit.Tree, newCommit.Tree)
}

func compareTrees(repository *storage.Repository, oldTree, newTree storage.ObjectID) ([]FileChange, error) {
	oldPaths, err := repository.WalkTree(oldTree)
	if err != nil {
		return nil, err
	}
	newPaths, err := repository.WalkTree(newTree)
	if err != nil {
		return nil, err
	}
	return compareTreeEntries(oldPaths, newPaths), nil
}

func compareTreeEntries(oldPaths, newPaths []storage.TreePath) []FileChange {
	oldFiles, newFiles := map[string]storage.TreeEntry{}, map[string]storage.TreeEntry{}
	for _, entry := range oldPaths {
		if entry.Type != storage.TreeObject {
			oldFiles[entry.Path] = entry.TreeEntry
		}
	}
	for _, entry := range newPaths {
		if entry.Type != storage.TreeObject {
			newFiles[entry.Path] = entry.TreeEntry
		}
	}
	paths, seen := make([]string, 0, len(oldFiles)+len(newFiles)), map[string]bool{}
	for path := range oldFiles {
		paths = append(paths, path)
		seen[path] = true
	}
	for path := range newFiles {
		if !seen[path] {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	result := []FileChange{}
	for _, path := range paths {
		oldEntry, oldOK := oldFiles[path]
		newEntry, newOK := newFiles[path]
		if oldOK && newOK && oldEntry.ID == newEntry.ID && oldEntry.Mode == newEntry.Mode {
			continue
		}
		change := FileChange{Path: path}
		if oldOK {
			value, mode := string(oldEntry.ID), oldEntry.Mode
			change.OldID, change.OldMode = &value, &mode
		}
		if newOK {
			value, mode := string(newEntry.ID), newEntry.Mode
			change.NewID, change.NewMode = &value, &mode
		}
		switch {
		case !oldOK:
			change.Status = "added"
		case !newOK:
			change.Status = "deleted"
		default:
			change.Status = "modified"
		}
		result = append(result, change)
	}
	return result
}

func (s *Store) AddComment(repositoryID, pullRequestID, authorID, body string) (Comment, error) {
	if !validID(authorID) {
		return Comment{}, ErrInvalid
	}
	body = strings.TrimSpace(body)
	if body == "" || len([]rune(body)) > 10000 {
		return Comment{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Comment{}, err
	}
	defer unlock()
	pull, err := s.Get(repositoryID, pullRequestID)
	if err != nil {
		return Comment{}, err
	}
	commentID, err := newID()
	if err != nil {
		return Comment{}, err
	}
	comment := Comment{ID: commentID, PullRequestID: pullRequestID, AuthorID: authorID, Body: body, Revision: pull.SourceCommitID, CreatedAt: s.now().Truncate(time.Microsecond)}
	record, err := s.readComments(repositoryID, pullRequestID)
	if err != nil {
		return Comment{}, err
	}
	record.Comments = append(record.Comments, comment)
	if committed, err := s.writeComments(repositoryID, pullRequestID, record); err != nil {
		if committed {
			return comment, fmt.Errorf("%w: %v", ErrDurabilityUncertain, err)
		}
		return Comment{}, err
	}
	return comment, nil
}

func (s *Store) ListComments(repositoryID, pullRequestID string) ([]Comment, error) {
	if _, err := s.Get(repositoryID, pullRequestID); err != nil {
		return nil, err
	}
	record, err := s.readComments(repositoryID, pullRequestID)
	if err != nil {
		return nil, err
	}
	return append([]Comment(nil), record.Comments...), nil
}

// SetReview creates or replaces the actor's decision against the recorded
// source revision. A reviewer has exactly one durable current review.
func (s *Store) SetReview(repositoryID, pullRequestID, reviewerID, decision string) (Review, error) {
	if !validID(reviewerID) || (decision != Approved && decision != ChangesRequested) {
		return Review{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Review{}, err
	}
	defer unlock()
	p, err := s.read(repositoryID, pullRequestID)
	if err != nil {
		return Review{}, err
	}
	if p.Status != Open {
		return Review{}, ErrNotReady
	}
	commitID, err := s.reviewSourceCommit(p)
	if err != nil {
		return Review{}, err
	}
	if commitID != p.SourceCommitID {
		return Review{}, ErrSourceChanged
	}
	record, err := s.readReviews(repositoryID, pullRequestID)
	if err != nil {
		return Review{}, err
	}
	now := s.now().Truncate(time.Microsecond)
	var review Review
	for i := range record.Reviews {
		if record.Reviews[i].ReviewerID == reviewerID {
			review = record.Reviews[i]
			review.Decision = decision
			review.ReviewedCommitID = commitID
			review.UpdatedAt = now
			record.Reviews[i] = review
			break
		}
	}
	if review.ID == "" {
		id, err := newID()
		if err != nil {
			return Review{}, err
		}
		review = Review{ID: id, PullRequestID: pullRequestID, ReviewerID: reviewerID, Decision: decision, ReviewedCommitID: commitID, CreatedAt: now, UpdatedAt: now}
		record.Reviews = append(record.Reviews, review)
	}
	if committed, err := s.writeReviews(repositoryID, pullRequestID, record); err != nil {
		if committed {
			return review, fmt.Errorf("%w: %v", ErrDurabilityUncertain, err)
		}
		return Review{}, err
	}
	return review, nil
}

// WithdrawReview retains who evaluated which commit while making clear that
// the participant no longer has an active approval or change request.
func (s *Store) WithdrawReview(repositoryID, pullRequestID, reviewID, reviewerID string) (Review, error) {
	if !validID(reviewID) || !validID(reviewerID) {
		return Review{}, ErrNotFound
	}
	if _, err := s.Get(repositoryID, pullRequestID); err != nil {
		return Review{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Review{}, err
	}
	defer unlock()
	record, err := s.readReviews(repositoryID, pullRequestID)
	if err != nil {
		return Review{}, err
	}
	var review Review
	for i := range record.Reviews {
		if record.Reviews[i].ID == reviewID && record.Reviews[i].ReviewerID == reviewerID {
			review = record.Reviews[i]
			review.Decision = Withdrawn
			review.UpdatedAt = s.now().Truncate(time.Microsecond)
			record.Reviews[i] = review
			break
		}
	}
	if review.ID == "" {
		return Review{}, ErrNotFound
	}
	if committed, err := s.writeReviews(repositoryID, pullRequestID, record); err != nil {
		if committed {
			return review, fmt.Errorf("%w: %v", ErrDurabilityUncertain, err)
		}
		return Review{}, err
	}
	return review, nil
}

func (s *Store) ListReviews(repositoryID, pullRequestID string) ([]Review, error) {
	p, err := s.Get(repositoryID, pullRequestID)
	if err != nil {
		return nil, err
	}
	currentCommitID, err := s.reviewSourceCommit(p)
	if errors.Is(err, ErrBranchNotFound) {
		currentCommitID = ""
		err = nil
	}
	if err != nil {
		return nil, err
	}
	return s.reviewsAtCommit(repositoryID, pullRequestID, currentCommitID)
}

func (s *Store) reviewsAtCommit(repositoryID, pullRequestID, currentCommitID string) ([]Review, error) {
	record, err := s.readReviews(repositoryID, pullRequestID)
	if err != nil {
		return nil, err
	}
	result := append([]Review(nil), record.Reviews...)
	for i := range result {
		result[i].Stale = currentCommitID == "" || result[i].ReviewedCommitID != currentCommitID
	}
	return result, nil
}

// Readiness recomputes every repository-level merge condition against live
// branch state. The caller supplies whether the inspecting actor has merge
// authority so the report can distinguish mergeability from permission.
func (s *Store) Readiness(repositoryID, pullRequestID string, actorCanMerge bool) (MergeReadiness, error) {
	p, err := s.Get(repositoryID, pullRequestID)
	if err != nil {
		return MergeReadiness{}, err
	}
	repository, err := s.git.Open(repositoryID)
	if err != nil {
		return MergeReadiness{}, err
	}
	report := MergeReadiness{
		RequiredApprovals: 1,
		EvaluatedCommitID: p.SourceCommitID,
		RequiredChecks:    []CheckRequirement{},
		Source:            BranchState{Branch: p.SourceBranch, SnapshotCommitID: p.SourceCommitID},
		Target:            BranchState{Branch: p.TargetBranch, SnapshotCommitID: p.TargetCommitID},
		Blockers:          []ReadinessBlocker{},
	}
	addBlocker := func(code, message string) {
		report.Blockers = append(report.Blockers, ReadinessBlocker{Code: code, Message: message})
	}
	if p.Status != Open {
		addBlocker("pull_request_not_open", "pull request must be open")
	}
	if p.TaskStatePending != "" {
		addBlocker("task_state_pending", "linked task state must be reconciled")
	}

	sourceID, sourceState, err := s.liveReviewSourceState(p, repository)
	if err != nil {
		return MergeReadiness{}, err
	}
	report.Source.State, report.Source.CurrentCommitID = sourceState, sourceID
	if sourceID == nil {
		addBlocker("source_branch_missing", "source branch must identify a commit")
	} else if *sourceID != p.SourceCommitID {
		addBlocker("source_branch_changed", "source branch no longer matches the pull request snapshot")
	}
	targetID, targetState, err := liveBranchState(repository, p.TargetBranch, p.TargetCommitID)
	if err != nil {
		return MergeReadiness{}, err
	}
	report.Target.State, report.Target.CurrentCommitID = targetState, targetID
	if targetID == nil {
		addBlocker("target_branch_missing", "target branch must identify a commit")
	}

	currentSourceID := ""
	if sourceID != nil {
		currentSourceID = *sourceID
	}
	reviews, err := s.reviewsAtCommit(repositoryID, pullRequestID, currentSourceID)
	if err != nil {
		return MergeReadiness{}, err
	}
	changesRequested := false
	for _, review := range reviews {
		if review.Stale || review.Decision == Withdrawn {
			continue
		}
		if review.Decision == Approved {
			report.Approvals++
		} else if review.Decision == ChangesRequested {
			changesRequested = true
		}
	}
	if report.Approvals < report.RequiredApprovals {
		addBlocker("approval_required", "at least one current approval is required")
	}
	if changesRequested {
		addBlocker("changes_requested", "a current review requests changes")
	}
	if s.acceptance != nil {
		policy, evaluationErr := s.acceptance.Policy(repositoryID, p.TargetBranch)
		if evaluationErr != nil {
			return MergeReadiness{}, evaluationErr
		}
		decisions, evaluationErr := s.acceptance.Decisions(repositoryID, pullRequestID)
		if evaluationErr != nil {
			return MergeReadiness{}, evaluationErr
		}
		changes, evaluationErr := s.Changes(repositoryID, pullRequestID)
		if evaluationErr != nil {
			return MergeReadiness{}, evaluationErr
		}
		paths := make([]string, 0, len(changes))
		for _, change := range changes {
			paths = append(paths, change.Path)
		}
		findings := []acceptance.Finding{}
		activeStakeholders := map[string]bool{}
		if s.previews != nil {
			attempts, listErr := s.previews.List(repositoryID, pullRequestID, p.SourceCommitID)
			if listErr != nil {
				return MergeReadiness{}, listErr
			}
			for _, attempt := range attempts {
				if attempt.Revision == p.SourceCommitID {
					for _, invitation := range attempt.Invitations {
						if invitation.Role == "feedback" && invitation.RevokedAt == nil && invitation.ExpiresAt.After(s.now()) {
							activeStakeholders[invitation.UserID] = true
						}
					}
				}
				for _, finding := range attempt.Findings {
					findings = append(findings, acceptance.Finding{ID: finding.ID, PreviewID: attempt.ID, Revision: finding.Revision, Title: finding.Title, Severity: finding.Severity, Status: finding.Status, AuthorID: finding.AuthorID})
				}
			}
		}
		risks := []string{}
		for i := range decisions {
			if decisions[i].Role == "stakeholder" && !activeStakeholders[decisions[i].ActorID] {
				// Preserve the decision as stale evidence while making it ineligible
				// to satisfy the live gate after invitation expiry or revocation.
				decisions[i].PolicyVersion = 0
				continue
			}
			if decisions[i].Revision == p.SourceCommitID {
				risks = append(risks, decisions[i].RiskClasses...)
			}
		}
		evaluation := acceptance.Evaluate(policy, p.SourceCommitID, paths, risks, decisions, findings)
		report.PreviewAcceptance = &evaluation
		for _, missing := range evaluation.Missing {
			addBlocker("preview_acceptance_required", fmt.Sprintf("preview scenario %q requires current %s acceptance", missing.Scenario, missing.Role))
		}
		for _, finding := range evaluation.Findings {
			if finding.Revision == p.SourceCommitID && finding.Severity == "blocking" && finding.Status != "resolved" {
				addBlocker("preview_finding_blocking", fmt.Sprintf("blocking preview finding %q is unresolved", finding.Title))
			}
		}
	}
	if s.requirements != nil {
		names, requirementErr := s.requirements.RequiredChecks(repositoryID, p.TargetBranch)
		if requirementErr != nil {
			return MergeReadiness{}, requirementErr
		}
		runs := []checkruns.Run{}
		if s.checkRuns != nil {
			runs, requirementErr = s.checkRuns.List(repositoryID, pullRequestID)
		}
		if requirementErr != nil {
			return MergeReadiness{}, requirementErr
		}
		for _, name := range names {
			requirement := CheckRequirement{Name: name, Status: "missing"}
			var stale *checkruns.Run
			for i := len(runs) - 1; i >= 0; i-- {
				run := runs[i]
				if run.Definition.Name != name {
					continue
				}
				if run.CommitID != p.SourceCommitID {
					if stale == nil {
						candidate := run
						stale = &candidate
					}
					continue
				}
				commit, runID := run.CommitID, run.ID
				requirement.CommitID, requirement.RunID = &commit, &runID
				switch run.State {
				case "succeeded":
					requirement.Status = "passed"
				case "failed":
					requirement.Status = "failed"
				case "canceled":
					requirement.Status = "cancelled"
				default:
					requirement.Status = "pending"
				}
				break
			}
			if requirement.Status == "missing" && stale != nil {
				commit, runID := stale.CommitID, stale.ID
				requirement.Status, requirement.CommitID, requirement.RunID = "stale", &commit, &runID
			}
			report.RequiredChecks = append(report.RequiredChecks, requirement)
			if requirement.Status != "passed" {
				addBlocker("required_check_"+requirement.Status, fmt.Sprintf("required check %q is %s for revision %s", name, requirement.Status, p.SourceCommitID))
			}
		}
	}
	if s.performance != nil {
		changes, changeErr := s.Changes(repositoryID, pullRequestID)
		if changeErr != nil {
			return MergeReadiness{}, changeErr
		}
		paths := make([]string, 0, len(changes))
		for _, change := range changes {
			paths = append(paths, change.Path)
		}
		risks := []string{}
		if report.PreviewAcceptance != nil {
			for _, decision := range report.PreviewAcceptance.Decisions {
				risks = append(risks, decision.RiskClasses...)
			}
		}
		requirements, performanceErr := s.performance.EvaluateMerge(repositoryID, pullRequestID, p.SourceCommitID, p.TargetBranch, paths, risks)
		if performanceErr != nil {
			return MergeReadiness{}, performanceErr
		}
		report.PerformanceRequirements = requirements
		for _, requirement := range requirements {
			if requirement.Status != "passed" {
				addBlocker("performance_"+requirement.Status, fmt.Sprintf("performance goal %q: %s", requirement.GoalID, requirement.Message))
			}
		}
	}
	if s.reliability != nil {
		risks := []string{}
		if report.PreviewAcceptance != nil {
			for _, decision := range report.PreviewAcceptance.Decisions {
				risks = append(risks, decision.RiskClasses...)
			}
		}
		evaluations, evaluationErr := s.reliability.EvaluateReliability(repositoryID, "pull_request", p.ID, p.SourceCommitID, p.TargetBranch, "", "", nil, risks)
		if evaluationErr != nil {
			return MergeReadiness{}, evaluationErr
		}
		report.ReliabilityReadiness = evaluations
		for _, evaluation := range evaluations {
			if evaluation.Effect == "block" || evaluation.Effect == "pause" || evaluation.Effect == "rollback" {
				addBlocker("reliability_"+evaluation.Effect, strings.Join(evaluation.Blockers, "; "))
			}
		}
	}
	if s.accessibilityDelivery != nil && s.accessibilityAssessments != nil {
		changes, changeErr := s.Changes(repositoryID, pullRequestID)
		if changeErr != nil {
			return MergeReadiness{}, changeErr
		}
		paths := make([]string, 0, len(changes))
		for _, change := range changes {
			paths = append(paths, change.Path)
		}
		statuses := map[string]string{}
		if s.checkRuns != nil {
			runs, runErr := s.checkRuns.List(repositoryID, pullRequestID)
			if runErr != nil {
				return MergeReadiness{}, runErr
			}
			for _, run := range runs {
				if run.CommitID != p.SourceCommitID {
					if _, ok := statuses[run.Definition.Name]; !ok {
						statuses[run.Definition.Name] = "stale"
					}
					continue
				}
				status := "pending"
				if run.State == "succeeded" {
					status = "passed"
				} else if run.State == "failed" {
					status = "failed"
				}
				statuses[run.Definition.Name] = status
			}
		}
		evidence, evidenceErr := s.accessibilityAssessments.List(repositoryID, "", pullRequestID)
		if evidenceErr != nil {
			return MergeReadiness{}, evidenceErr
		}
		journeys, risks := []string{}, []string{}
		for _, assessment := range evidence {
			for _, check := range assessment.Checks {
				if check.JourneyID != "" {
					journeys = append(journeys, check.JourneyID)
				}
			}
			for _, finding := range assessment.Findings {
				risks = append(risks, finding.Severity)
				journeys = append(journeys, finding.JourneyIDs...)
			}
		}
		if report.PreviewAcceptance != nil {
			for _, decision := range report.PreviewAcceptance.Decisions {
				risks = append(risks, decision.RiskClasses...)
			}
		}
		accessibility, evaluationErr := s.accessibilityDelivery.Evaluate(repositoryID, p.SourceCommitID, p.TargetBranch, pullRequestID, "", paths, journeys, risks, statuses, evidence)
		if evaluationErr != nil {
			return MergeReadiness{}, evaluationErr
		}
		report.AccessibilityReadiness = &accessibility
		for _, requirement := range accessibility.Requirements {
			if requirement.Status != "passed" {
				overridden := false
				for _, exception := range accessibility.ActiveExceptions {
					if exception.PolicyID == requirement.PolicyID {
						overridden = true
					}
				}
				if !overridden {
					addBlocker("accessibility_"+requirement.Kind+"_"+requirement.Status, requirement.Message+": "+requirement.Name)
				}
			}
		}
	}
	if s.privacyChecks != nil {
		changes, changeErr := s.Changes(repositoryID, pullRequestID)
		if changeErr != nil {
			return MergeReadiness{}, changeErr
		}
		paths := make([]string, 0, len(changes))
		for _, change := range changes {
			paths = append(paths, change.Path)
		}
		privacy, evaluationErr := s.privacyChecks.Evaluate(repositoryID, p.SourceCommitID, p.TargetBranch, pullRequestID, paths)
		if evaluationErr != nil {
			return MergeReadiness{}, evaluationErr
		}
		report.PrivacyReadiness = &privacy
		for _, requirement := range privacy.Requirements {
			if requirement.Status == "passed" {
				continue
			}
			overridden := false
			for _, exception := range privacy.ActiveExceptions {
				if exception.PolicyID == requirement.PolicyID && requirement.Kind == "runtime_rule" && containsPrivacyRule(exception.Rules, requirement.Name) {
					overridden = true
				}
			}
			if !overridden {
				addBlocker("privacy_"+requirement.Kind+"_"+requirement.Status, requirement.Message+": "+requirement.Name)
			}
		}
	}
	if s.localization != nil {
		statuses := map[string]string{}
		risks := acceptedLocalizationRisks(report.PreviewAcceptance)
		if s.checkRuns != nil {
			runs, runErr := s.checkRuns.List(repositoryID, pullRequestID)
			if runErr != nil {
				return MergeReadiness{}, runErr
			}
			for _, run := range runs {
				status := "pending"
				if run.CommitID != p.SourceCommitID {
					status = "stale"
				} else if run.State == "succeeded" {
					status = "passed"
				} else if run.State == "failed" {
					status = "failed"
				}
				statuses[run.Definition.Name] = status
			}
		}
		localeReadiness, evaluationErr := s.localization.EvaluateDelivery(repositoryID, pullRequestID, "", p.SourceCommitID, p.TargetBranch, nil, risks, statuses)
		if evaluationErr != nil {
			return MergeReadiness{}, evaluationErr
		}
		report.LocalizationReadiness = &localeReadiness
		for _, requirement := range localeReadiness.Requirements {
			if requirement.Status != "passed" {
				addBlocker("localization_"+requirement.Kind+"_"+requirement.Status, requirement.Locale+": "+requirement.Name)
			}
		}
	}

	if sourceID != nil && targetID != nil && *sourceID == p.SourceCommitID {
		merged, err := commitReachable(repository, *sourceID, *targetID)
		if err != nil {
			return MergeReadiness{}, err
		}
		if merged {
			addBlocker("already_merged", "source commit is already reachable from the target branch")
		} else {
			report.HasConflicts, err = mergeConflicts(repository, *targetID, *sourceID)
			if err != nil {
				return MergeReadiness{}, err
			}
			if report.HasConflicts {
				addBlocker("merge_conflict", "source and target branches have merge conflicts")
			}
		}
	}
	if s.designReadiness != nil {
		changes, changeErr := s.Changes(repositoryID, pullRequestID)
		if changeErr != nil {
			return MergeReadiness{}, changeErr
		}
		projection, blockers, designErr := s.designReadiness(p, changes)
		if designErr != nil {
			return MergeReadiness{}, designErr
		}
		report.DesignReadiness = projection
		report.Blockers = append(report.Blockers, blockers...)
	}
	if s.qualityConfidence != nil {
		changes, changeErr := s.Changes(repositoryID, pullRequestID)
		if changeErr != nil {
			return MergeReadiness{}, changeErr
		}
		projection, blockers, qualityErr := s.qualityConfidence(p, changes)
		if qualityErr != nil {
			return MergeReadiness{}, qualityErr
		}
		report.QualityConfidence = projection
		report.Blockers = append(report.Blockers, blockers...)
	}
	if s.securityConfidence != nil {
		changes, changeErr := s.Changes(repositoryID, pullRequestID)
		if changeErr != nil {
			return MergeReadiness{}, changeErr
		}
		projection, blockers, securityErr := s.securityConfidence(p, changes)
		if securityErr != nil {
			return MergeReadiness{}, securityErr
		}
		report.SecurityConfidence = projection
		report.Blockers = append(report.Blockers, blockers...)
	}
	if s.assuranceImpact != nil {
		changes, changeErr := s.Changes(repositoryID, pullRequestID)
		if changeErr != nil {
			return MergeReadiness{}, changeErr
		}
		projection, blockers, impactErr := s.assuranceImpact(p, changes)
		if impactErr != nil {
			return MergeReadiness{}, impactErr
		}
		report.AssuranceImpact = projection
		report.Blockers = append(report.Blockers, blockers...)
	}
	report.Mergeable = len(report.Blockers) == 0
	report.CanMerge = report.Mergeable && actorCanMerge
	if s.requirements != nil {
		policy, policyErr := s.requirements.IntegrationQueuePolicy(repositoryID, p.TargetBranch)
		if policyErr != nil {
			return MergeReadiness{}, policyErr
		}
		report.IntegrationQueue = &policy
		if policy.Enabled {
			report.CanEnqueue = report.Mergeable && actorCanMerge && p.QueuedAt == nil
			report.CanMerge = false
		}
	}
	return report, nil
}

func acceptedLocalizationRisks(evaluation *acceptance.Evaluation) []string {
	if evaluation == nil {
		return nil
	}
	risks := []string{}
	for _, decision := range evaluation.Decisions {
		if decision.Outcome == "accepted" {
			risks = append(risks, decision.RiskClasses...)
		}
	}
	return risks
}

func containsPrivacyRule(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

// Enqueue admits a currently mergeable pull request and freezes the exact
// prospective merge that verification must evaluate.
func (s *Store) Enqueue(repositoryID, pullRequestID, actorID string) (PullRequest, error) {
	if !validID(actorID) {
		return PullRequest{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return PullRequest{}, err
	}
	defer unlock()
	p, err := s.Get(repositoryID, pullRequestID)
	if err != nil {
		return PullRequest{}, err
	}
	if p.QueuedAt != nil {
		return p, nil
	}
	if s.requirements != nil {
		unlockRequirements, lockErr := s.requirements.LockRequiredChecks()
		if lockErr != nil {
			return PullRequest{}, lockErr
		}
		defer unlockRequirements()
	}
	report, err := s.Readiness(repositoryID, pullRequestID, true)
	if err != nil || !report.CanEnqueue || report.Target.CurrentCommitID == nil {
		return PullRequest{}, ErrNotReady
	}
	repository, err := s.git.Open(repositoryID)
	if err != nil {
		return PullRequest{}, err
	}
	now := s.now().Truncate(time.Microsecond)
	requiredChecks := make([]string, len(report.RequiredChecks))
	for i, requirement := range report.RequiredChecks {
		requiredChecks[i] = requirement.Name
	}
	definitions, err := requiredDefinitions(repository, *report.Target.CurrentCommitID, requiredChecks)
	if err != nil {
		return PullRequest{}, ErrNotReady
	}
	tree, err := mergeTree(repository, *report.Target.CurrentCommitID, p.SourceCommitID)
	if err != nil {
		return PullRequest{}, ErrNotReady
	}
	candidateID, err := newID()
	if err != nil {
		return PullRequest{}, err
	}
	stamp := fmt.Sprintf("%d +0000", now.Unix())
	content := fmt.Sprintf("tree %s\nparent %s\nparent %s\nauthor Vivarium Integration Queue <queue@vivarium> %s\ncommitter Vivarium Integration Queue <queue@vivarium> %s\n\nVerify pull request %s against %s\n", tree, *report.Target.CurrentCommitID, p.SourceCommitID, stamp, stamp, p.ID, *report.Target.CurrentCommitID)
	commit, err := repository.WriteObject(storage.CommitObject, []byte(content))
	if err != nil {
		return PullRequest{}, fmt.Errorf("write integration candidate: %w", err)
	}
	p.IntegrationCandidates = append(p.IntegrationCandidates, IntegrationCandidate{ID: candidateID, SourceCommitID: p.SourceCommitID, BaseCommitID: *report.Target.CurrentCommitID, CommitID: string(commit), RequiredChecks: requiredChecks, CheckDefinitions: definitions, CreatedAt: now})
	p.QueuedAt, p.QueuedBy, p.UpdatedAt = &now, &actorID, now
	p.QueueRank = new(big.Int).SetInt64(now.UnixNano()).String()
	p.QueuePaused = false
	p.QueueActions = append(p.QueueActions, QueueAction{Action: "enqueued", ActorID: actorID, CreatedAt: now})
	if committed, writeErr := s.write(p); writeErr != nil {
		if committed {
			return p, fmt.Errorf("%w: %v", ErrDurabilityUncertain, writeErr)
		}
		return PullRequest{}, writeErr
	}
	return p, nil
}

// OperateQueue applies an owner-authorized intervention while retaining its
// attribution on the pull request. Position is one-based for reprioritization.
func (s *Store) OperateQueue(repositoryID, pullRequestID, actorID, action string, position int) (PullRequest, error) {
	if !validID(actorID) || (action != "pause" && action != "resume" && action != "retry" && action != "remove" && action != "reprioritize") {
		return PullRequest{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return PullRequest{}, err
	}
	defer unlock()
	p, err := s.read(repositoryID, pullRequestID)
	if err != nil {
		return PullRequest{}, err
	}
	if p.Status != Open || p.QueuedAt == nil {
		return PullRequest{}, ErrNotReady
	}
	now := s.now().Truncate(time.Microsecond)
	switch action {
	case "pause":
		p.QueuePaused = true
	case "resume":
		p.QueuePaused = false
	case "retry":
		p.QueuePaused = false
		if candidate := activeCandidate(p); candidate != nil {
			for i := range p.IntegrationCandidates {
				if p.IntegrationCandidates[i].ID == candidate.ID {
					p.IntegrationCandidates[i].SupersededAt, p.IntegrationCandidates[i].SupersededReason = &now, "retried"
				}
			}
		}
	case "remove":
		p.QueuePaused, p.QueuedAt, p.QueuedBy, p.QueueRank = false, nil, nil, ""
	case "reprioritize":
		pulls, listErr := s.List(repositoryID)
		if listErr != nil {
			return PullRequest{}, listErr
		}
		queued := make([]PullRequest, 0)
		for _, candidate := range pulls {
			if candidate.Status == Open && candidate.TargetBranch == p.TargetBranch && candidate.QueuedAt != nil && candidate.ID != p.ID {
				queued = append(queued, candidate)
			}
		}
		sort.SliceStable(queued, func(i, j int) bool { return queueLess(queued[i], queued[j]) })
		if position < 1 || position > len(queued)+1 {
			return PullRequest{}, ErrInvalid
		}
		queued = append(queued, PullRequest{})
		copy(queued[position:], queued[position-1:])
		queued[position-1] = p
		var rank *big.Rat
		switch {
		case len(queued) == 1:
			rank = queueRank(p)
		case position == 1:
			rank = new(big.Rat).Sub(queueRank(queued[1]), big.NewRat(1, 1))
		case position == len(queued):
			rank = new(big.Rat).Add(queueRank(queued[len(queued)-2]), big.NewRat(1, 1))
		default:
			rank = new(big.Rat).Add(queueRank(queued[position-2]), queueRank(queued[position]))
			rank.Quo(rank, big.NewRat(2, 1))
		}
		p.QueueRank = rank.RatString()
	}
	p.UpdatedAt = now
	p.QueueActions = append(p.QueueActions, QueueAction{Action: action, ActorID: actorID, CreatedAt: now})
	if committed, writeErr := s.write(p); writeErr != nil {
		if committed {
			return p, fmt.Errorf("%w: %v", ErrDurabilityUncertain, writeErr)
		}
		return PullRequest{}, writeErr
	}
	return p, nil
}

// IntegrationQueue returns ordered entries with current evidence, blockers,
// and a concise prediction of the next automatic or human action.
func (s *Store) IntegrationQueue(repositoryID, branch string) (IntegrationQueueView, error) {
	pulls, err := s.List(repositoryID)
	if err != nil {
		return IntegrationQueueView{}, err
	}
	sort.SliceStable(pulls, func(i, j int) bool {
		if pulls[i].QueuedAt == nil {
			return false
		}
		if pulls[j].QueuedAt == nil {
			return true
		}
		return queueLess(pulls[i], pulls[j])
	})
	view := IntegrationQueueView{Branch: branch, Entries: []IntegrationQueueEntry{}}
	for _, p := range pulls {
		if p.Status != Open || p.TargetBranch != branch || p.QueuedAt == nil {
			continue
		}
		entry := IntegrationQueueEntry{Position: len(view.Entries) + 1, PullRequest: p, Blockers: []ReadinessBlocker{}}
		candidates, candidateErr := s.Candidates(repositoryID, p.ID)
		if candidateErr != nil {
			return IntegrationQueueView{}, candidateErr
		}
		if len(candidates) > 0 {
			entry.Candidate = &candidates[len(candidates)-1]
			entry.State = entry.Candidate.State
		}
		if entry.State == "" {
			entry.State = "waiting"
		}
		switch {
		case p.QueuePaused:
			entry.State, entry.NextAction = "paused", "Resume, retry, reprioritize, or remove this entry"
		case entry.State == "failed":
			entry.Blockers = append(entry.Blockers, ReadinessBlocker{Code: "candidate_failed", Message: "The current integration candidate did not pass verification"})
			entry.NextAction = "Retry or remove the entry"
		case entry.Position > 1:
			entry.NextAction = "Wait for earlier entries, or reprioritize"
		case entry.State == "passed":
			entry.NextAction = "Merge automatically when the reconciler advances"
		default:
			entry.NextAction = "Wait for candidate verification"
		}
		view.Entries = append(view.Entries, entry)
	}
	return view, nil
}

func queueRank(p PullRequest) *big.Rat {
	if p.QueueRank != "" {
		if rank, ok := new(big.Rat).SetString(p.QueueRank); ok {
			return rank
		}
	}
	if p.QueuedAt != nil {
		return new(big.Rat).SetInt64(p.QueuedAt.UnixNano())
	}
	return new(big.Rat)
}

func queueLess(left, right PullRequest) bool {
	comparison := queueRank(left).Cmp(queueRank(right))
	if comparison == 0 {
		return left.ID < right.ID
	}
	return comparison < 0
}

func requiredDefinitions(repository *storage.Repository, commitID string, required []string) ([]checkruns.Definition, error) {
	if len(required) == 0 {
		return []checkruns.Definition{}, nil
	}
	command := exec.Command("git", "--git-dir="+repository.Path(), "show", commitID+":"+checkruns.ConfigPath)
	data, err := command.Output()
	if err != nil {
		return nil, err
	}
	config, err := checkruns.ParseConfig(data)
	if err != nil {
		return nil, err
	}
	byName := map[string]checkruns.Definition{}
	for _, definition := range config.Checks {
		byName[definition.Name] = definition
	}
	definitions := make([]checkruns.Definition, 0, len(required))
	for _, name := range required {
		definition, exists := byName[name]
		if !exists {
			return nil, ErrNotReady
		}
		definitions = append(definitions, definition)
	}
	return definitions, nil
}

// Candidates returns immutable candidate identity together with its derived
// verification lifecycle and evidence handles.
func (s *Store) Candidates(repositoryID, pullRequestID string) ([]IntegrationCandidateView, error) {
	p, err := s.Get(repositoryID, pullRequestID)
	if err != nil {
		return nil, err
	}
	result := make([]IntegrationCandidateView, len(p.IntegrationCandidates))
	for i, candidate := range p.IntegrationCandidates {
		result[i].IntegrationCandidate = candidate
		result[i].Checks = []checkruns.Run{}
	}
	runs := []checkruns.Run{}
	if s.checkRuns != nil {
		runs, err = s.checkRuns.List(repositoryID, pullRequestID)
		if err != nil {
			return nil, err
		}
	}
	for i := range result {
		for _, run := range runs {
			if run.CommitID == result[i].CommitID {
				result[i].Checks = append(result[i].Checks, run)
			}
		}
		if result[i].SupersededAt != nil {
			result[i].State = "superseded"
		} else {
			result[i].State = candidateState(result[i].Checks, result[i].RequiredChecks)
		}
	}
	return result, nil
}

// AdvanceIntegrationQueues serializes automatic queue progress with every
// pull-request mutation. A candidate may update its target only when its
// frozen base is still the live target; every other candidate is superseded
// and rebuilt before its evidence can be considered.
func (s *Store) AdvanceIntegrationQueues() error {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return err
	}
	var failures []error
	for _, entry := range entries {
		if !entry.IsDir() || !validID(entry.Name()) {
			continue
		}
		pulls, listErr := s.List(entry.Name())
		if listErr != nil {
			failures = append(failures, fmt.Errorf("scan integration queues for repository %s: %w", entry.Name(), listErr))
			continue
		}
		for _, p := range pulls {
			if p.Status == Merged && p.QueueFinalizationPending {
				if err := s.finalizeQueueCompletion(p.RepositoryID, p.ID); err != nil {
					failures = append(failures, fmt.Errorf("finalize queued merge %s/%s: %w", p.RepositoryID, p.ID, err))
				}
			}
		}
		branches := map[string]bool{}
		for _, p := range pulls {
			if p.Status == Open && p.QueuedAt != nil {
				branches[p.TargetBranch] = true
			}
		}
		for branch := range branches {
			if err := s.advanceIntegrationQueue(entry.Name(), branch); err != nil {
				failures = append(failures, fmt.Errorf("advance integration queue %s/%s: %w", entry.Name(), branch, err))
			}
		}
	}
	return errors.Join(failures...)
}

func (s *Store) advanceIntegrationQueue(repositoryID, branch string) error {
	if s.previews != nil {
		return s.previews.WithAudienceAdmission(func() error {
			return s.advanceIntegrationQueueAdmitted(repositoryID, branch)
		})
	}
	return s.advanceIntegrationQueueAdmitted(repositoryID, branch)
}

func (s *Store) advanceIntegrationQueueAdmitted(repositoryID, branch string) error {
	var finalizationFailures []error
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()
	if s.requirements == nil {
		return nil
	}
	unReq, err := s.requirements.LockRequiredChecks()
	if err != nil {
		return err
	}
	defer unReq()
	policy, err := s.requirements.IntegrationQueuePolicy(repositoryID, branch)
	if err != nil || !policy.Enabled {
		return err
	}
	repository, err := s.git.Open(repositoryID)
	if err != nil {
		return err
	}
	pulls, err := s.List(repositoryID)
	if err != nil {
		return err
	}
	sort.SliceStable(pulls, func(i, j int) bool {
		if pulls[i].QueuedAt == nil {
			return false
		}
		if pulls[j].QueuedAt == nil {
			return true
		}
		return queueLess(pulls[i], pulls[j])
	})
	queued := make([]PullRequest, 0, len(pulls))
	for _, p := range pulls {
		if p.Status == Open && p.TargetBranch == branch && p.QueuedAt != nil {
			if p.mergeIntent != nil {
				reconciled, found, reconcileErr := s.reconcileMerged(repository, p)
				if reconcileErr != nil {
					return reconcileErr
				}
				if found {
					if finalizeErr := s.finalizeQueueCompletionLocked(reconciled); finalizeErr != nil {
						finalizationFailures = append(finalizationFailures, finalizeErr)
					}
					continue
				}
				p, err = s.read(repositoryID, p.ID)
				if err != nil {
					return err
				}
			}
			queued = append(queued, p)
		}
	}
	for len(queued) > 0 {
		target, targetErr := branchCommit(repository, branch)
		if targetErr != nil {
			return targetErr
		}
		limit := policy.Concurrency
		if limit > len(queued) {
			limit = len(queued)
		}
		for i := 0; i < limit; i++ {
			p := &queued[i]
			if p.QueuePaused {
				continue
			}
			candidate := activeCandidate(*p)
			if candidate != nil && candidate.SourceCommitID == p.SourceCommitID && candidate.BaseCommitID == target {
				s.launchCandidate(*p, *candidate)
				continue
			}
			if candidate != nil {
				now := s.now().Truncate(time.Microsecond)
				for n := range p.IntegrationCandidates {
					if p.IntegrationCandidates[n].ID == candidate.ID {
						p.IntegrationCandidates[n].SupersededAt, p.IntegrationCandidates[n].SupersededReason = &now, "target_changed"
					}
				}
			}
			created, createErr := s.newIntegrationCandidate(repository, *p, target, policy.RequiredChecks)
			if createErr != nil {
				if policy.FailureBehavior == repositories.QueueFailurePause {
					if _, e := s.write(*p); e != nil {
						return e
					}
					return errors.Join(finalizationFailures...)
				}
				p.QueuedAt, p.QueuedBy, p.QueueRank = nil, nil, ""
				p.UpdatedAt = s.now().Truncate(time.Microsecond)
				if _, e := s.write(*p); e != nil {
					return e
				}
				queued = append(queued[:i], queued[i+1:]...)
				limit--
				i--
				continue
			}
			p.IntegrationCandidates = append(p.IntegrationCandidates, created)
			p.UpdatedAt = created.CreatedAt
			if _, e := s.write(*p); e != nil {
				return e
			}
			s.launchCandidate(*p, created)
		}
		if len(queued) == 0 {
			break
		}
		head := &queued[0]
		if head.QueuePaused {
			return errors.Join(finalizationFailures...)
		}
		candidate := activeCandidate(*head)
		if candidate == nil || candidate.BaseCommitID != target {
			return errors.Join(finalizationFailures...)
		}
		state := s.integrationCandidateState(*head, *candidate)
		if state == "pending" || state == "verifying" {
			return errors.Join(finalizationFailures...)
		}
		if state == "failed" {
			if policy.FailureBehavior == repositories.QueueFailurePause {
				return errors.Join(finalizationFailures...)
			}
			head.QueuedAt, head.QueuedBy, head.QueueRank = nil, nil, ""
			head.UpdatedAt = s.now().Truncate(time.Microsecond)
			if _, err := s.write(*head); err != nil {
				return err
			}
			queued = queued[1:]
			continue
		}
		// Admission freezes a candidate, not future authority. Re-evaluate every
		// live review, check, acceptance, finding, and branch gate immediately
		// before landing so policy or evidence changes pause the durable entry.
		readiness, readinessErr := s.Readiness(repositoryID, head.ID, true)
		if readinessErr != nil {
			return readinessErr
		}
		if !readiness.Mergeable {
			head.QueuePaused = true
			head.UpdatedAt = s.now().Truncate(time.Microsecond)
			if _, err := s.write(*head); err != nil {
				return err
			}
			return errors.Join(finalizationFailures...)
		}
		merger := ""
		if head.QueuedBy != nil {
			merger = *head.QueuedBy
		}
		if !validID(merger) {
			return ErrInvalid
		}
		now := s.now().Truncate(time.Microsecond)
		head.mergeIntent = &mergeIntent{CommitID: candidate.CommitID, MergerID: merger, MergedAt: now}
		if _, err := s.write(*head); err != nil {
			return err
		}
		if err := repository.UpdateReferenceIfTarget(storage.Reference{Name: "refs/heads/" + branch, Target: candidate.CommitID}, candidate.BaseCommitID); err != nil {
			head.mergeIntent = nil
			_, _ = s.write(*head)
			continue
		}
		commit, mergedBy := candidate.CommitID, merger
		head.Status, head.UpdatedAt, head.MergedAt, head.MergedBy, head.MergeCommitID = Merged, now, &now, &mergedBy, &commit
		if head.TaskID != nil {
			head.TaskStatePending = "merged"
		}
		head.QueueFinalizationPending = true
		head.QueuedAt, head.QueuedBy, head.QueueRank, head.mergeIntent = nil, nil, "", nil
		if _, err := s.write(*head); err != nil {
			return err
		}
		if finalizeErr := s.finalizeQueueCompletionLocked(*head); finalizeErr != nil {
			finalizationFailures = append(finalizationFailures, finalizeErr)
		}
		queued = queued[1:]
	}
	return errors.Join(finalizationFailures...)
}

func (s *Store) finalizeQueueCompletion(repositoryID, pullRequestID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()
	p, err := s.read(repositoryID, pullRequestID)
	if err != nil {
		return err
	}
	return s.finalizeQueueCompletionLocked(p)
}

func (s *Store) finalizeQueueCompletionLocked(p PullRequest) error {
	if !p.QueueFinalizationPending || p.Status != Merged || s.queueFinalizer == nil {
		return nil
	}
	if err := s.queueFinalizer(p); err != nil {
		return err
	}
	now := s.now().Truncate(time.Microsecond)
	p.QueueFinalizationPending, p.QueueFinalizedAt, p.UpdatedAt = false, &now, now
	if committed, err := s.write(p); err != nil {
		if committed {
			return fmt.Errorf("%w: %v", ErrDurabilityUncertain, err)
		}
		return err
	}
	return nil
}

func activeCandidate(p PullRequest) *IntegrationCandidate {
	for i := len(p.IntegrationCandidates) - 1; i >= 0; i-- {
		if p.IntegrationCandidates[i].SupersededAt == nil {
			c := p.IntegrationCandidates[i]
			return &c
		}
	}
	return nil
}

func (s *Store) integrationCandidateState(p PullRequest, candidate IntegrationCandidate) string {
	runs := []checkruns.Run{}
	if s.checkRuns != nil {
		runs, _ = s.checkRuns.List(p.RepositoryID, p.ID)
	}
	matching := []checkruns.Run{}
	for _, run := range runs {
		if run.CommitID == candidate.CommitID {
			matching = append(matching, run)
		}
	}
	return candidateState(matching, candidate.RequiredChecks)
}

func (s *Store) newIntegrationCandidate(repository *storage.Repository, p PullRequest, base string, required []string) (IntegrationCandidate, error) {
	definitions, err := requiredDefinitions(repository, base, required)
	if err != nil {
		return IntegrationCandidate{}, err
	}
	tree, err := mergeTree(repository, base, p.SourceCommitID)
	if err != nil {
		return IntegrationCandidate{}, err
	}
	id, err := newID()
	if err != nil {
		return IntegrationCandidate{}, err
	}
	now := s.now().Truncate(time.Microsecond)
	stamp := fmt.Sprintf("%d +0000", now.Unix())
	content := fmt.Sprintf("tree %s\nparent %s\nparent %s\nauthor Vivarium Integration Queue <queue@vivarium> %s\ncommitter Vivarium Integration Queue <queue@vivarium> %s\n\nVerify pull request %s against %s\n", tree, base, p.SourceCommitID, stamp, stamp, p.ID, base)
	commit, err := repository.WriteObject(storage.CommitObject, []byte(content))
	if err != nil {
		return IntegrationCandidate{}, err
	}
	requiredCopy := make([]string, len(required))
	copy(requiredCopy, required)
	return IntegrationCandidate{ID: id, SourceCommitID: p.SourceCommitID, BaseCommitID: base, CommitID: string(commit), RequiredChecks: requiredCopy, CheckDefinitions: definitions, CreatedAt: now}, nil
}

func (s *Store) launchCandidate(p PullRequest, candidate IntegrationCandidate) {
	if s.checkRuns == nil || len(candidate.CheckDefinitions) == 0 {
		return
	}
	existing, err := s.checkRuns.List(p.RepositoryID, p.ID)
	if err != nil {
		return
	}
	present := map[string]bool{}
	for _, run := range existing {
		if run.CommitID == candidate.CommitID {
			present[run.Definition.Name] = true
		}
	}
	complete := true
	for _, definition := range candidate.CheckDefinitions {
		complete = complete && present[definition.Name]
	}
	if complete {
		return
	}
	runs, err := s.checkRuns.Create(p.RepositoryID, p.ID, candidate.CommitID, candidate.CheckDefinitions)
	if err != nil {
		return
	}
	repository, err := s.git.Open(p.RepositoryID)
	if err != nil {
		return
	}
	for _, run := range runs {
		go s.checkRuns.Execute(run, repository.Path())
	}
}

func candidateState(runs []checkruns.Run, required []string) string {
	if len(required) == 0 {
		return "passed"
	}
	byName := map[string]checkruns.Run{}
	for _, run := range runs {
		byName[run.Definition.Name] = run
	}
	if len(runs) == 0 {
		return "pending"
	}
	state := "passed"
	for _, name := range required {
		run, exists := byName[name]
		if !exists {
			state = "pending"
			continue
		}
		if run.State == "failed" || run.State == "canceled" {
			return "failed"
		}
		if run.State != "succeeded" {
			state = "verifying"
		}
	}
	return state
}

// Merge revalidates readiness while holding the cross-process mutation lock,
// writes an attributable two-parent commit, advances the target with compare-
// and-swap semantics, and closes the pull request.
func (s *Store) Merge(repositoryID, pullRequestID, mergerID string) (PullRequest, error) {
	var merged PullRequest
	merge := func() error {
		var err error
		merged, err = s.merge(repositoryID, pullRequestID, mergerID)
		return err
	}
	if s.previews != nil {
		if err := s.previews.WithAudienceAdmission(merge); err != nil {
			return PullRequest{}, err
		}
		return merged, nil
	}
	return merged, merge()
}

// RecordStackMerge closes an ordinary pull after an exact stack candidate has
// already advanced the target. The expected source prevents another revision
// from borrowing the retained candidate's delivery identity.
func (s *Store) RecordStackMerge(repositoryID, pullRequestID, expectedSource, commitID, mergerID string) (PullRequest, error) {
	if !validID(mergerID) || !validCommitID(expectedSource) || !validCommitID(commitID) {
		return PullRequest{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return PullRequest{}, err
	}
	defer unlock()
	p, err := s.Get(repositoryID, pullRequestID)
	if err != nil {
		return PullRequest{}, err
	}
	if p.Status == Merged {
		if p.MergeCommitID != nil && *p.MergeCommitID == commitID {
			return p, nil
		}
		return PullRequest{}, ErrInvalid
	}
	if p.Status != Open || p.SourceCommitID != expectedSource {
		return PullRequest{}, ErrNotReady
	}
	now := s.now().Truncate(time.Microsecond)
	mergedBy, mergedCommit := mergerID, commitID
	p.Status, p.UpdatedAt, p.MergedAt, p.MergedBy, p.MergeCommitID = Merged, now, &now, &mergedBy, &mergedCommit
	if p.TaskID != nil {
		p.TaskStatePending = "merged"
	}
	_, err = s.write(p)
	return p, err
}

func (s *Store) merge(repositoryID, pullRequestID, mergerID string) (PullRequest, error) {
	if !validID(mergerID) {
		return PullRequest{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return PullRequest{}, err
	}
	defer unlock()

	p, err := s.Get(repositoryID, pullRequestID)
	if err != nil {
		return PullRequest{}, err
	}
	// A merge retry is idempotent. This also lets the application complete a
	// linked-proposal close after an earlier cross-store failure.
	if p.Status == Merged {
		return p, nil
	}
	repository, err := s.git.Open(repositoryID)
	if err != nil {
		return PullRequest{}, err
	}
	if reconciled, found, reconcileErr := s.reconcileMerged(repository, p); reconcileErr != nil {
		return PullRequest{}, reconcileErr
	} else if found {
		return reconciled, nil
	}
	if s.requirements != nil {
		unlockRequirements, lockErr := s.requirements.LockRequiredChecks()
		if lockErr != nil {
			return PullRequest{}, lockErr
		}
		defer unlockRequirements()
	}
	report, err := s.Readiness(repositoryID, pullRequestID, true)
	if err != nil {
		return PullRequest{}, err
	}
	if !report.CanMerge || report.Source.CurrentCommitID == nil || report.Target.CurrentCommitID == nil {
		return PullRequest{}, ErrNotReady
	}
	tree, err := mergeTree(repository, *report.Target.CurrentCommitID, *report.Source.CurrentCommitID)
	if err != nil {
		return PullRequest{}, err
	}
	now := s.now().Truncate(time.Microsecond)
	stamp := fmt.Sprintf("%d +0000", now.Unix())
	message := p.Title + "\n"
	if p.Body != "" {
		message += "\n" + p.Body + "\n"
	}
	message += fmt.Sprintf("\nPull-Request: %s\nSource-Repository: %s\nSource-Branch: %s\nSource-Commit: %s\nAuthored-by: %s\nMerged-by: %s\n", p.ID, p.SourceRepositoryID, p.SourceBranch, p.SourceCommitID, p.AuthorID, mergerID)
	if p.ProposalID != nil {
		message += "Proposal: " + *p.ProposalID + "\n"
	}
	content := fmt.Sprintf("tree %s\nparent %s\nparent %s\nauthor Vivarium Author <%s@users.vivarium> %s\ncommitter Vivarium Maintainer <%s@users.vivarium> %s\n\n%s", tree, *report.Target.CurrentCommitID, *report.Source.CurrentCommitID, p.AuthorID, stamp, mergerID, stamp, message)
	commit, err := repository.WriteObject(storage.CommitObject, []byte(content))
	if err != nil {
		return PullRequest{}, err
	}
	p.mergeIntent = &mergeIntent{CommitID: string(commit), MergerID: mergerID, MergedAt: now}
	intentUncertain := false
	if committed, intentErr := s.write(p); intentErr != nil {
		if !committed {
			return PullRequest{}, intentErr
		}
		intentUncertain = true
	}
	if err := repository.UpdateReferenceIfTarget(storage.Reference{Name: "refs/heads/" + p.TargetBranch, Target: string(commit)}, *report.Target.CurrentCommitID); err != nil {
		p.mergeIntent = nil
		_, _ = s.write(p)
		return PullRequest{}, ErrNotReady
	}
	mergedBy, commitID := mergerID, string(commit)
	p.Status, p.UpdatedAt, p.MergedAt, p.MergedBy, p.MergeCommitID, p.mergeIntent = Merged, now, &now, &mergedBy, &commitID, nil
	if p.TaskID != nil {
		p.TaskStatePending = "merged"
	}
	if committed, err := s.write(p); err != nil {
		if committed {
			return p, fmt.Errorf("%w: %v", ErrDurabilityUncertain, err)
		}
		return PullRequest{}, err
	}
	if intentUncertain {
		return p, ErrDurabilityUncertain
	}
	return p, nil
}

// reconcileMerged repairs metadata when the target publication succeeded but
// a later metadata write and compensating reference update both failed. The
// private durable intent identifies the exact server-generated commit even
// after later target commits have descended from it. Git metadata alone is
// never authorization provenance because contributors can forge it.
func (s *Store) reconcileMerged(repository *storage.Repository, p PullRequest) (PullRequest, bool, error) {
	if p.mergeIntent == nil || !validCommitID(p.mergeIntent.CommitID) || !validID(p.mergeIntent.MergerID) || p.mergeIntent.MergedAt.IsZero() {
		return PullRequest{}, false, nil
	}
	target, err := branchCommit(repository, p.TargetBranch)
	if errors.Is(err, ErrBranchNotFound) {
		return PullRequest{}, false, nil
	}
	if err != nil {
		return PullRequest{}, false, err
	}
	ancestry, err := repository.ListCommitAncestry(storage.ObjectID(target))
	if err != nil {
		return PullRequest{}, false, err
	}
	for _, commit := range ancestry {
		if string(commit.ID) != p.mergeIntent.CommitID {
			continue
		}
		merger, mergedAt := p.mergeIntent.MergerID, p.mergeIntent.MergedAt
		commitID := string(commit.ID)
		p.Status, p.UpdatedAt, p.MergedAt, p.MergedBy, p.MergeCommitID, p.mergeIntent = Merged, mergedAt, &mergedAt, &merger, &commitID, nil
		if p.TaskID != nil {
			p.TaskStatePending = "merged"
		}
		if p.QueuedAt != nil {
			p.QueuedAt, p.QueuedBy, p.QueueRank, p.QueueFinalizationPending = nil, nil, "", true
		}
		if committed, writeErr := s.write(p); writeErr != nil {
			if committed {
				return p, true, fmt.Errorf("%w: %v", ErrDurabilityUncertain, writeErr)
			}
			return PullRequest{}, false, writeErr
		}
		return p, true, nil
	}
	// The intent never reached the target (for example, its CAS lost). Remove
	// it before evaluating a fresh merge attempt.
	p.mergeIntent = nil
	if committed, writeErr := s.write(p); writeErr != nil && !committed {
		return PullRequest{}, false, writeErr
	}
	return PullRequest{}, false, nil
}

func mergeTree(repository *storage.Repository, target, source string) (storage.ObjectID, error) {
	temporary, err := os.MkdirTemp("", "vivarium-merge-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(temporary)
	objects := filepath.Join(temporary, "objects")
	if err := os.Mkdir(objects, 0o700); err != nil {
		return "", err
	}
	env := append(os.Environ(), "GIT_OBJECT_DIRECTORY="+objects, "GIT_ALTERNATE_OBJECT_DIRECTORIES="+filepath.Join(repository.Path(), "objects"))
	command := exec.Command("git", "-C", repository.Path(), "merge-tree", "--write-tree", target, source)
	command.Env = env
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("calculate merge tree: %w: %s", err, strings.TrimSpace(string(output)))
	}
	id := storage.ObjectID(strings.TrimSpace(string(output)))
	seen := map[storage.ObjectID]bool{}
	var importTree func(storage.ObjectID) error
	importTree = func(tree storage.ObjectID) error {
		if seen[tree] {
			return nil
		}
		seen[tree] = true
		cat := exec.Command("git", "-C", repository.Path(), "cat-file", "tree", string(tree))
		cat.Env = env
		content, err := cat.Output()
		if err != nil {
			return err
		}
		written, err := repository.WriteObject(storage.TreeObject, content)
		if err != nil || written != tree {
			return fmt.Errorf("import merge tree %s: %v", tree, err)
		}
		list := exec.Command("git", "-C", repository.Path(), "ls-tree", "-z", string(tree))
		list.Env = env
		listed, err := list.Output()
		if err != nil {
			return err
		}
		for _, record := range strings.Split(string(listed), "\x00") {
			fields := strings.Fields(record)
			if len(fields) >= 3 && fields[1] == "tree" {
				if err := importTree(storage.ObjectID(fields[2])); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := importTree(id); err != nil {
		return "", err
	}
	return id, nil
}

func liveBranchState(repository *storage.Repository, branch, snapshot string) (*string, string, error) {
	current, err := branchCommit(repository, branch)
	if errors.Is(err, ErrBranchNotFound) {
		return nil, "missing", nil
	}
	if err != nil {
		return nil, "", err
	}
	state := "current"
	if current != snapshot {
		advanced, err := commitReachable(repository, snapshot, current)
		if err != nil {
			return nil, "", err
		}
		if advanced {
			state = "advanced"
		} else {
			state = "rewritten"
		}
	}
	return &current, state, nil
}

// reviewSourceCommit follows a live source branch when it exists. If an
// independently owned source repository has been deleted, the exact adopted
// commit remains a valid review boundary because CreateFrom imported and
// verified its reachable objects in the target repository. Synchronization is
// intentionally stricter and never uses this fallback.
func (s *Store) reviewSourceCommit(p PullRequest) (string, error) {
	source, err := s.git.Open(p.SourceRepositoryID)
	if err == nil {
		return branchCommit(source, p.SourceBranch)
	}
	if p.SourceRepositoryID == p.RepositoryID || !errors.Is(err, storage.ErrRepositoryNotFound) {
		return "", err
	}
	target, targetErr := s.git.Open(p.RepositoryID)
	if targetErr != nil {
		return "", targetErr
	}
	if _, targetErr = target.ReadCommit(storage.ObjectID(p.SourceCommitID)); targetErr != nil {
		return "", targetErr
	}
	return p.SourceCommitID, nil
}

func (s *Store) liveReviewSourceState(p PullRequest, target *storage.Repository) (*string, string, error) {
	source, err := s.git.Open(p.SourceRepositoryID)
	if err == nil {
		return liveBranchState(source, p.SourceBranch, p.SourceCommitID)
	}
	if p.SourceRepositoryID == p.RepositoryID || !errors.Is(err, storage.ErrRepositoryNotFound) {
		return nil, "", err
	}
	if _, err := target.ReadCommit(storage.ObjectID(p.SourceCommitID)); err != nil {
		return nil, "", err
	}
	commit := p.SourceCommitID
	return &commit, "unavailable", nil
}

func commitReachable(repository *storage.Repository, ancestor, descendant string) (bool, error) {
	commits, err := repository.ListCommitAncestry(storage.ObjectID(descendant))
	if err != nil {
		return false, err
	}
	for _, commit := range commits {
		if string(commit.ID) == ancestor {
			return true, nil
		}
	}
	return false, nil
}

// mergeConflicts asks stock Git to perform its merge calculation while
// redirecting every object write into a disposable object directory. The bare
// repository remains byte-for-byte read-only.
func mergeConflicts(repository *storage.Repository, target, source string) (bool, error) {
	temporary, err := os.MkdirTemp("", "vivarium-merge-readiness-")
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(temporary)
	objects := filepath.Join(temporary, "objects")
	if err := os.Mkdir(objects, 0o700); err != nil {
		return false, err
	}
	command := exec.Command("git", "-C", repository.Path(), "merge-tree", "--write-tree", target, source)
	command.Env = append(os.Environ(), "GIT_OBJECT_DIRECTORY="+objects, "GIT_ALTERNATE_OBJECT_DIRECTORIES="+filepath.Join(repository.Path(), "objects"))
	if output, err := command.CombinedOutput(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return true, nil
		}
		return false, fmt.Errorf("calculate merge: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return false, nil
}

func (s *Store) commentsPath(repositoryID, pullRequestID string) string {
	return filepath.Join(s.repositoryPath(repositoryID), pullRequestID+".comments.json")
}

func (s *Store) reviewsPath(repositoryID, pullRequestID string) string {
	return filepath.Join(s.repositoryPath(repositoryID), pullRequestID+".reviews.json")
}

func (s *Store) readReviews(repositoryID, pullRequestID string) (reviewRecord, error) {
	data, err := os.ReadFile(s.reviewsPath(repositoryID, pullRequestID))
	if errors.Is(err, os.ErrNotExist) {
		return reviewRecord{Reviews: []Review{}}, nil
	}
	if err != nil {
		return reviewRecord{}, err
	}
	var record reviewRecord
	if json.Unmarshal(data, &record) != nil {
		return reviewRecord{}, fmt.Errorf("corrupt pull request reviews %s", pullRequestID)
	}
	seenIDs, seenReviewers := map[string]bool{}, map[string]bool{}
	for _, review := range record.Reviews {
		validDecision := review.Decision == Approved || review.Decision == ChangesRequested || review.Decision == Withdrawn
		if !validID(review.ID) || review.PullRequestID != pullRequestID || !validID(review.ReviewerID) || !validDecision || !validCommitID(review.ReviewedCommitID) || review.CreatedAt.IsZero() || review.UpdatedAt.IsZero() || review.UpdatedAt.Before(review.CreatedAt) || seenIDs[review.ID] || seenReviewers[review.ReviewerID] {
			return reviewRecord{}, fmt.Errorf("corrupt pull request reviews %s", pullRequestID)
		}
		seenIDs[review.ID], seenReviewers[review.ReviewerID] = true, true
	}
	sort.Slice(record.Reviews, func(i, j int) bool {
		if record.Reviews[i].CreatedAt.Equal(record.Reviews[j].CreatedAt) {
			return record.Reviews[i].ID < record.Reviews[j].ID
		}
		return record.Reviews[i].CreatedAt.Before(record.Reviews[j].CreatedAt)
	})
	return record, nil
}

func (s *Store) writeReviews(repositoryID, pullRequestID string, record reviewRecord) (bool, error) {
	data, err := json.Marshal(record)
	if err != nil {
		return false, err
	}
	directory := s.repositoryPath(repositoryID)
	temp, err := os.CreateTemp(directory, ".writing-reviews-")
	if err != nil {
		return false, err
	}
	path := temp.Name()
	defer os.Remove(path)
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
	if err := os.Rename(path, s.reviewsPath(repositoryID, pullRequestID)); err != nil {
		return false, err
	}
	return true, s.directorySync(directory)
}

func (s *Store) readComments(repositoryID, pullRequestID string) (commentRecord, error) {
	data, err := os.ReadFile(s.commentsPath(repositoryID, pullRequestID))
	if errors.Is(err, os.ErrNotExist) {
		return commentRecord{Comments: []Comment{}}, nil
	}
	if err != nil {
		return commentRecord{}, err
	}
	var record commentRecord
	if json.Unmarshal(data, &record) != nil {
		return commentRecord{}, fmt.Errorf("corrupt pull request comments %s", pullRequestID)
	}
	seen := map[string]bool{}
	for _, comment := range record.Comments {
		if !validID(comment.ID) || comment.PullRequestID != pullRequestID || !validID(comment.AuthorID) || strings.TrimSpace(comment.Body) == "" || len([]rune(comment.Body)) > 10000 || comment.CreatedAt.IsZero() || seen[comment.ID] {
			return commentRecord{}, fmt.Errorf("corrupt pull request comments %s", pullRequestID)
		}
		seen[comment.ID] = true
	}
	return record, nil
}

func (s *Store) writeComments(repositoryID, pullRequestID string, record commentRecord) (bool, error) {
	data, err := json.Marshal(record)
	if err != nil {
		return false, err
	}
	directory := s.repositoryPath(repositoryID)
	temp, err := os.CreateTemp(directory, ".writing-comments-")
	if err != nil {
		return false, err
	}
	path := temp.Name()
	defer os.Remove(path)
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
	if err := os.Rename(path, s.commentsPath(repositoryID, pullRequestID)); err != nil {
		return false, err
	}
	return true, s.directorySync(directory)
}

func validatePurpose(title, body string) (string, string, error) {
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
func (s *Store) repositoryPath(repositoryID string) string {
	return filepath.Join(s.root, repositoryID)
}

func (s *Store) path(repositoryID, id string) string {
	return filepath.Join(s.repositoryPath(repositoryID), id+".json")
}

func (s *Store) read(repositoryID, id string) (PullRequest, error) {
	if !validID(repositoryID) || !validID(id) {
		return PullRequest{}, ErrNotFound
	}
	data, err := os.ReadFile(s.path(repositoryID, id))
	if errors.Is(err, os.ErrNotExist) {
		return PullRequest{}, ErrNotFound
	}
	if err != nil {
		return PullRequest{}, err
	}
	var record pullRequestRecord
	if json.Unmarshal(data, &record) != nil {
		return PullRequest{}, fmt.Errorf("corrupt pull request %s", id)
	}
	p := record.PullRequest
	// Records created before cross-repository contributions were introduced
	// implicitly sourced their branch from the target repository.
	if p.SourceRepositoryID == "" {
		p.SourceRepositoryID = p.RepositoryID
	}
	p.mergeIntent = record.MergeIntent
	if p.IntegrationCandidates == nil {
		p.IntegrationCandidates = []IntegrationCandidate{}
	}
	validOutcome := (p.Status == Open && p.ClosedAt == nil && p.ClosedBy == nil && p.MergedAt == nil && p.MergedBy == nil && p.MergeCommitID == nil) ||
		(p.Status == Closed && p.ClosedAt != nil && !p.ClosedAt.IsZero() && p.ClosedBy != nil && validID(*p.ClosedBy) && p.MergedAt == nil && p.MergedBy == nil && p.MergeCommitID == nil) ||
		(p.Status == Merged && p.ClosedAt == nil && p.ClosedBy == nil && p.MergedAt != nil && p.MergedBy != nil && validID(*p.MergedBy) && p.MergeCommitID != nil && validCommitID(*p.MergeCommitID))
	validIntent := p.mergeIntent == nil || (p.Status == Open && validCommitID(p.mergeIntent.CommitID) && validID(p.mergeIntent.MergerID) && !p.mergeIntent.MergedAt.IsZero())
	_, validRank := new(big.Rat).SetString(p.QueueRank)
	validQueue := (p.QueuedAt == nil && p.QueuedBy == nil && p.QueueRank == "") || (p.Status == Open && p.QueuedAt != nil && !p.QueuedAt.IsZero() && p.QueuedBy != nil && validID(*p.QueuedBy) && (p.QueueRank == "" || validRank))
	validFinalization := (!p.QueueFinalizationPending || (p.Status == Merged && p.QueueFinalizedAt == nil)) && (p.QueueFinalizedAt == nil || (p.Status == Merged && !p.QueueFinalizationPending && !p.QueueFinalizedAt.IsZero()))
	if p.ID != id || !validID(p.RepositoryID) || !validID(p.SourceRepositoryID) || !validID(p.AuthorID) || !validOutcome || !validIntent || !validQueue || !validFinalization || !validCommitID(p.SourceCommitID) || !validCommitID(p.TargetCommitID) || (p.SourceRepositoryID == p.RepositoryID && p.SourceBranch == p.TargetBranch) || p.SourceBranch == "" || p.TargetBranch == "" || strings.HasPrefix(p.SourceBranch, "refs/") || strings.HasPrefix(p.TargetBranch, "refs/") || p.CreatedAt.IsZero() || p.UpdatedAt.IsZero() || (p.ProposalID != nil && !validID(*p.ProposalID)) || (p.TaskID != nil && (!validID(*p.TaskID) || len(p.TaskCommitIDs) == 0)) || (p.TaskSessionID != nil && !validID(*p.TaskSessionID)) || (p.TaskRunID != nil && !validID(*p.TaskRunID)) || (p.TaskID == nil && (p.TaskSessionID != nil || p.TaskRunID != nil || p.TaskStatePending != "" || len(p.TaskCommitIDs) != 0)) || (p.TaskStatePending != "" && p.TaskStatePending != "review" && p.TaskStatePending != "closed" && p.TaskStatePending != "merged") {
		return PullRequest{}, fmt.Errorf("corrupt pull request %s", id)
	}
	for _, commitID := range p.TaskCommitIDs {
		if !validCommitID(commitID) {
			return PullRequest{}, fmt.Errorf("corrupt pull request %s", id)
		}
	}
	if _, _, err := validatePurpose(p.Title, p.Body); err != nil {
		return PullRequest{}, fmt.Errorf("corrupt pull request %s", id)
	}
	for _, candidate := range p.IntegrationCandidates {
		validSupersession := (candidate.SupersededAt == nil && candidate.SupersededReason == "") || (candidate.SupersededAt != nil && !candidate.SupersededAt.IsZero() && (candidate.SupersededReason == "target_changed" || candidate.SupersededReason == "retried"))
		if !validID(candidate.ID) || !validCommitID(candidate.SourceCommitID) || !validCommitID(candidate.BaseCommitID) || !validCommitID(candidate.CommitID) || candidate.RequiredChecks == nil || candidate.CheckDefinitions == nil || len(candidate.RequiredChecks) != len(candidate.CheckDefinitions) || candidate.CreatedAt.IsZero() || !validSupersession {
			return PullRequest{}, fmt.Errorf("corrupt pull request %s", id)
		}
		for i, definition := range candidate.CheckDefinitions {
			if definition.Name != candidate.RequiredChecks[i] {
				return PullRequest{}, fmt.Errorf("corrupt pull request %s", id)
			}
		}
	}
	for _, action := range p.QueueActions {
		if !validID(action.ActorID) || action.CreatedAt.IsZero() || (action.Action != "enqueued" && action.Action != "pause" && action.Action != "resume" && action.Action != "retry" && action.Action != "remove" && action.Action != "reprioritize") {
			return PullRequest{}, fmt.Errorf("corrupt pull request %s", p.ID)
		}
	}
	return p, nil
}

func validCommitID(id string) bool {
	if len(id) != 40 || id != strings.ToLower(id) {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

func (s *Store) write(p PullRequest) (bool, error) {
	data, err := json.Marshal(pullRequestRecord{PullRequest: p, MergeIntent: p.mergeIntent})
	if err != nil {
		return false, err
	}
	repositoryPath := s.repositoryPath(p.RepositoryID)
	temp, err := os.CreateTemp(repositoryPath, ".writing-")
	if err != nil {
		return false, err
	}
	path := temp.Name()
	defer os.Remove(path)
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
	if err := os.Rename(path, s.path(p.RepositoryID, p.ID)); err != nil {
		return false, err
	}
	return true, s.directorySync(repositoryPath)
}

func (s *Store) ensureRepositoryDirectory(repositoryID string) error {
	path := s.repositoryPath(repositoryID)
	if err := os.Mkdir(path, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("create pull request repository directory: %w", err)
	}
	// Sync the root even for an existing directory. A retry after an uncertain
	// directory publication must not acknowledge writes beneath an unsynced entry.
	if err := s.rootSync(s.root); err != nil {
		return fmt.Errorf("sync pull request storage root: %w", err)
	}
	return nil
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
		_ = f.Close()
		return nil, err
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}
