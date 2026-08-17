// Package issues persists structured, repository-scoped unexpected-behavior reports.
package issues

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

var (
	ErrNotFound            = errors.New("issue not found")
	ErrInvalid             = errors.New("invalid issue")
	ErrConflict            = errors.New("issue changed")
	ErrForbidden           = errors.New("issue status transition forbidden")
	ErrDurabilityUncertain = errors.New("issue mutation is visible but durability is uncertain")
)

type Attachment struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Name      string    `json:"name"`
	MediaType string    `json:"media_type"`
	Size      int       `json:"size"`
	Data      string    `json:"data,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type Comment struct {
	ID        string    `json:"id"`
	AuthorID  string    `json:"author_id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

type HistoryEntry struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	ActorID   string    `json:"actor_id"`
	From      string    `json:"from,omitempty"`
	To        string    `json:"to,omitempty"`
	Message   string    `json:"message,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type ReproductionInput struct {
	AttachmentID string `json:"attachment_id"`
	Name         string `json:"name"`
	SHA256       string `json:"sha256"`
	Size         int    `json:"size"`
}
type ReproductionCommand struct {
	Name          string    `json:"name"`
	OutcomeID     string    `json:"outcome_id"`
	CommandSHA256 string    `json:"command_sha256"`
	ExitCode      int       `json:"exit_code"`
	Log           string    `json:"log,omitempty"`
	StartedAt     time.Time `json:"started_at"`
	CompletedAt   time.Time `json:"completed_at"`
}
type ReproductionArtifact struct {
	Name      string `json:"name"`
	MediaType string `json:"media_type"`
	SHA256    string `json:"sha256"`
	Size      int    `json:"size"`
	Data      string `json:"data,omitempty"`
}
type ReproductionAttempt struct {
	ID                    string                 `json:"id"`
	WorkspaceID           string                 `json:"workspace_id"`
	CommitID              string                 `json:"commit_id"`
	ReleaseID             string                 `json:"release_id,omitempty"`
	DefinitionSHA256      string                 `json:"definition_sha256"`
	EnvironmentDefinition json.RawMessage        `json:"environment_definition"`
	Inputs                []ReproductionInput    `json:"inputs"`
	Commands              []ReproductionCommand  `json:"commands"`
	Artifacts             []ReproductionArtifact `json:"artifacts"`
	ObservedResult        string                 `json:"observed_result"`
	Result                string                 `json:"result"`
	ReproducedBy          string                 `json:"reproduced_by"`
	CreatedAt             time.Time              `json:"created_at"`
}

type Link struct {
	ID           string    `json:"id"`
	Kind         string    `json:"kind"`
	RepositoryID string    `json:"repository_id,omitempty"`
	ResourceID   string    `json:"resource_id"`
	Revision     string    `json:"revision,omitempty"`
	Label        string    `json:"label"`
	AddedBy      string    `json:"added_by"`
	CreatedAt    time.Time `json:"created_at"`
}
type Triage struct {
	Classification    string    `json:"classification,omitempty"`
	Priority          string    `json:"priority,omitempty"`
	AssigneeID        string    `json:"assignee_id,omitempty"`
	SuspectedRevision string    `json:"suspected_revision,omitempty"`
	SuspectedOwnerIDs []string  `json:"suspected_owner_ids,omitempty"`
	UpdatedBy         string    `json:"updated_by,omitempty"`
	UpdatedAt         time.Time `json:"updated_at,omitempty"`
}
type EvidenceRequest struct {
	ID            string    `json:"id"`
	Body          string    `json:"body"`
	RequestedFrom string    `json:"requested_from"`
	RequestedBy   string    `json:"requested_by"`
	State         string    `json:"state"`
	Response      string    `json:"response,omitempty"`
	RespondedBy   string    `json:"responded_by,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
type Finding struct {
	ID              string      `json:"id"`
	Kind            string      `json:"kind"`
	Statement       string      `json:"statement"`
	ActorID         string      `json:"actor_id"`
	InvestigationID string      `json:"investigation_id,omitempty"`
	CitationIDs     []string    `json:"citation_ids"`
	Challenges      []Challenge `json:"challenges,omitempty"`
	SupersedesID    string      `json:"supersedes_id,omitempty"`
	CreatedAt       time.Time   `json:"created_at"`
}
type Challenge struct {
	ID        string    `json:"id"`
	ActorID   string    `json:"actor_id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}
type Investigation struct {
	ID                    string    `json:"id"`
	AgentID               string    `json:"agent_id"`
	InitiatorID           string    `json:"initiator_id"`
	CredentialID          string    `json:"credential_id,omitempty"`
	Mandate               string    `json:"mandate"`
	ReproductionAttemptID string    `json:"reproduction_attempt_id"`
	LinkIDs               []string  `json:"link_ids"`
	State                 string    `json:"state"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

// Implementation freezes the evidence handoff into ordinary proposal/task
// governance. Live pull/check/review state is projected from those stores.
type Implementation struct {
	ProposalID            string    `json:"proposal_id"`
	TaskID                string    `json:"task_id"`
	ReproductionAttemptID string    `json:"reproduction_attempt_id"`
	FindingIDs            []string  `json:"finding_ids"`
	AffectedRevision      string    `json:"affected_revision"`
	AcceptanceCriteria    []string  `json:"acceptance_criteria"`
	CreatedBy             string    `json:"created_by"`
	CreatedAt             time.Time `json:"created_at"`
}

// RepairVerification freezes issue-specific proof for one exact pull revision.
// Decisions are append-only so a maintainer override never erases the
// reporter's dissent.
type RepairVerification struct {
	ID                    string               `json:"id"`
	PullRequestID         string               `json:"pull_request_id"`
	CandidateCommitID     string               `json:"candidate_commit_id"`
	ReproductionAttemptID string               `json:"reproduction_attempt_id"`
	DefinitionSHA256      string               `json:"definition_sha256"`
	InputSHA256s          []string             `json:"input_sha256s"`
	AcceptanceCriteria    []string             `json:"acceptance_criteria"`
	RequiredRunIDs        []string             `json:"required_run_ids"`
	ReproductionRunIDs    []string             `json:"reproduction_run_ids"`
	RequestedBy           string               `json:"requested_by"`
	Decisions             []ResolutionDecision `json:"decisions"`
	PreviewAllowed        bool                 `json:"preview_allowed"`
	PreviewWorkspaceID    string               `json:"preview_workspace_id,omitempty"`
	CreatedAt             time.Time            `json:"created_at"`
}

type ResolutionDecision struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	ActorID   string    `json:"actor_id"`
	CommitID  string    `json:"commit_id"`
	Rationale string    `json:"rationale"`
	CreatedAt time.Time `json:"created_at"`
}

// DeliveryResolution closes the loop from exact repair proof to an immutable
// released and successfully promoted commit. The original proof remains the
// reproducible contract; release/deployment stores remain authoritative.
type DeliveryResolution struct {
	ID                   string    `json:"id"`
	RepairVerificationID string    `json:"repair_verification_id"`
	ReleaseID            string    `json:"release_id"`
	ReleaseVersion       string    `json:"release_version"`
	ReleaseCommitID      string    `json:"release_commit_id"`
	DeploymentID         string    `json:"deployment_id"`
	EnvironmentID        string    `json:"environment_id"`
	ArtifactSHA256       string    `json:"artifact_sha256"`
	ReporterDecisionID   string    `json:"reporter_decision_id"`
	RecordedBy           string    `json:"recorded_by"`
	CreatedAt            time.Time `json:"created_at"`
}

type Issue struct {
	ID                   string                `json:"id"`
	RepositoryID         string                `json:"repository_id"`
	ReleaseID            string                `json:"release_id,omitempty"`
	AffectedVersion      string                `json:"affected_version,omitempty"`
	Title                string                `json:"title"`
	ExpectedBehavior     string                `json:"expected_behavior"`
	ObservedBehavior     string                `json:"observed_behavior"`
	Severity             string                `json:"severity"`
	Environment          string                `json:"environment"`
	ReproductionSteps    []string              `json:"reproduction_steps"`
	Visibility           string                `json:"visibility"`
	Status               string                `json:"status"`
	ReporterID           string                `json:"reporter_id"`
	Attachments          []Attachment          `json:"attachments"`
	Discussion           []Comment             `json:"discussion"`
	History              []HistoryEntry        `json:"history"`
	ReproductionAttempts []ReproductionAttempt `json:"reproduction_attempts"`
	Triage               Triage                `json:"triage"`
	Links                []Link                `json:"links"`
	EvidenceRequests     []EvidenceRequest     `json:"evidence_requests"`
	Findings             []Finding             `json:"findings"`
	Investigations       []Investigation       `json:"investigations"`
	Implementation       *Implementation       `json:"implementation,omitempty"`
	RepairVerifications  []RepairVerification  `json:"repair_verifications,omitempty"`
	DeliveryResolution   *DeliveryResolution   `json:"delivery_resolution,omitempty"`
	DuplicateOf          string                `json:"duplicate_of,omitempty"`
	Version              int                   `json:"version"`
	CreatedAt            time.Time             `json:"created_at"`
	UpdatedAt            time.Time             `json:"updated_at"`
}

func (s *Store) Mutate(repositoryID, id, actor string, expected int, fn func(*Issue) error) (Issue, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.read(repositoryID, id)
	if err != nil {
		return Issue{}, err
	}
	if expected != 0 && expected != v.Version {
		return Issue{}, ErrConflict
	}
	if err = fn(&v); err != nil {
		return Issue{}, err
	}
	now := time.Now().UTC()
	v.Version++
	v.UpdatedAt = now
	committed, err := s.write(v)
	if err != nil {
		if committed {
			return v, errors.Join(ErrDurabilityUncertain, err)
		}
		return Issue{}, err
	}
	return v, nil
}

func AddHistory(v *Issue, kind, actor, message string) {
	v.History = append(v.History, HistoryEntry{ID: newID(), Kind: kind, ActorID: actor, Message: message, CreatedAt: time.Now().UTC()})
}

// NewEvidenceID allocates an opaque identifier for cross-store evidence that
// is persisted through Mutate.
func NewEvidenceID() string { return newID() }
func NewID() string         { return newID() }

func (s *Store) AddReproductionAttempt(repositoryID, id, actor string, attempt ReproductionAttempt) (Issue, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.read(repositoryID, id)
	if err != nil {
		return Issue{}, err
	}
	if strings.TrimSpace(attempt.WorkspaceID) == "" || len(attempt.CommitID) != 40 || len(attempt.DefinitionSHA256) != 64 || len(attempt.Commands) > 20 || len(attempt.ObservedResult) == 0 || len(attempt.ObservedResult) > 10000 || (attempt.Result != "reproduced" && attempt.Result != "not_reproduced" && attempt.Result != "inconclusive") || len(attempt.Commands) == 0 && attempt.Result != "inconclusive" {
		return Issue{}, ErrInvalid
	}
	attempt.ID = newID()
	attempt.ReproducedBy = actor
	attempt.CreatedAt = time.Now().UTC()
	v.ReproductionAttempts = append(v.ReproductionAttempts, attempt)
	v.History = append(v.History, HistoryEntry{ID: newID(), Kind: "reproduction_attempted", ActorID: actor, Message: attempt.Result, CreatedAt: attempt.CreatedAt})
	v.Version++
	v.UpdatedAt = attempt.CreatedAt
	committed, err := s.write(v)
	if err != nil {
		if committed {
			return v, errors.Join(ErrDurabilityUncertain, err)
		}
		return Issue{}, err
	}
	return v, nil
}

type Store struct {
	root          string
	mu            sync.Mutex
	directorySync func(string) error
}

func New(root string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("issue root required")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	return &Store{root: root, directorySync: syncDirectory}, nil
}

func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func (s *Store) Create(v Issue) (Issue, error) {
	return s.create(v, "")
}

// CreateEscalated makes the support escalation identity the issue creation
// idempotency key, so reconciliation cannot publish a second issue.
func (s *Store) CreateEscalated(v Issue, escalationID string) (Issue, error) {
	return s.create(v, escalationID)
}

func (s *Store) create(v Issue, requestedID string) (Issue, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v.Title, v.ExpectedBehavior, v.ObservedBehavior, v.Environment = strings.TrimSpace(v.Title), strings.TrimSpace(v.ExpectedBehavior), strings.TrimSpace(v.ObservedBehavior), strings.TrimSpace(v.Environment)
	if v.RepositoryID == "" || v.ReporterID == "" || v.Title == "" || v.ExpectedBehavior == "" || v.ObservedBehavior == "" || v.Environment == "" || !validSeverity(v.Severity) || !validVisibility(v.Visibility) || len(v.ReproductionSteps) == 0 {
		return Issue{}, ErrInvalid
	}
	for i := range v.ReproductionSteps {
		v.ReproductionSteps[i] = strings.TrimSpace(v.ReproductionSteps[i])
		if v.ReproductionSteps[i] == "" {
			return Issue{}, ErrInvalid
		}
	}
	if err := validateAttachments(v.Attachments); err != nil {
		return Issue{}, err
	}
	if requestedID != "" {
		if len(requestedID) != 32 {
			return Issue{}, ErrInvalid
		}
		if existing, err := s.read(v.RepositoryID, requestedID); err == nil {
			return existing, nil
		} else if !errors.Is(err, ErrNotFound) {
			return Issue{}, err
		}
	}
	now := time.Now().UTC()
	if requestedID == "" {
		requestedID = newID()
	}
	v.ID, v.Status, v.Version, v.CreatedAt, v.UpdatedAt = requestedID, "open", 1, now, now
	for i := range v.Attachments {
		v.Attachments[i].ID, v.Attachments[i].CreatedAt = newID(), now
	}
	v.History = []HistoryEntry{{ID: newID(), Kind: "opened", ActorID: v.ReporterID, To: "open", CreatedAt: now}}
	committed, err := s.write(v)
	if err != nil {
		if committed {
			return v, errors.Join(ErrDurabilityUncertain, err)
		}
		return Issue{}, err
	}
	return v, nil
}

func (s *Store) Get(repositoryID, id string) (Issue, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read(repositoryID, id)
}

func (s *Store) List(repositoryID string) ([]Issue, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	out := []Issue{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, e := os.ReadFile(filepath.Join(s.root, entry.Name()))
		if e != nil {
			return nil, e
		}
		var v Issue
		if json.Unmarshal(data, &v) != nil {
			return nil, errors.New("corrupt issue")
		}
		if v.RepositoryID == repositoryID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) AddComment(repositoryID, id, actor, body string) (Issue, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.read(repositoryID, id)
	body = strings.TrimSpace(body)
	if err != nil || body == "" {
		if err != nil {
			return Issue{}, err
		}
		return Issue{}, ErrInvalid
	}
	now := time.Now().UTC()
	v.Discussion = append(v.Discussion, Comment{ID: newID(), AuthorID: actor, Body: body, CreatedAt: now})
	v.History = append(v.History, HistoryEntry{ID: newID(), Kind: "commented", ActorID: actor, Message: body, CreatedAt: now})
	v.Version++
	v.UpdatedAt = now
	committed, err := s.write(v)
	if err != nil {
		if committed {
			return v, errors.Join(ErrDurabilityUncertain, err)
		}
		return Issue{}, err
	}
	return v, nil
}

func (s *Store) UpdateStatus(repositoryID, id, actor, status string, expected int, message string, owner bool) (Issue, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.read(repositoryID, id)
	if err != nil {
		return Issue{}, err
	}
	if expected != v.Version {
		return Issue{}, ErrConflict
	}
	if !validStatus(status) {
		return Issue{}, ErrInvalid
	}
	if !owner && (terminalStatus(v.Status) || terminalStatus(status)) {
		return Issue{}, ErrForbidden
	}
	if status == v.Status {
		return Issue{}, ErrInvalid
	}
	now := time.Now().UTC()
	from := v.Status
	v.Status = status
	v.Version++
	v.UpdatedAt = now
	v.History = append(v.History, HistoryEntry{ID: newID(), Kind: "status_changed", ActorID: actor, From: from, To: status, Message: strings.TrimSpace(message), CreatedAt: now})
	committed, err := s.write(v)
	if err != nil {
		if committed {
			return v, errors.Join(ErrDurabilityUncertain, err)
		}
		return Issue{}, err
	}
	return v, nil
}

func (s *Store) read(repo, id string) (Issue, error) {
	data, err := os.ReadFile(filepath.Join(s.root, id+".json"))
	if os.IsNotExist(err) {
		return Issue{}, ErrNotFound
	}
	if err != nil {
		return Issue{}, err
	}
	var v Issue
	if json.Unmarshal(data, &v) != nil || v.ID != id || v.RepositoryID != repo {
		return Issue{}, ErrNotFound
	}
	return v, nil
}
func (s *Store) write(v Issue) (bool, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return false, err
	}
	f, err := os.CreateTemp(s.root, ".issue-*")
	if err != nil {
		return false, err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0o600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return false, err
	}
	if err = os.Rename(name, filepath.Join(s.root, v.ID+".json")); err != nil {
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
func validSeverity(v string) bool {
	return v == "low" || v == "medium" || v == "high" || v == "critical"
}
func validVisibility(v string) bool { return v == "public" || v == "repository" }
func validStatus(v string) bool {
	return v == "open" || v == "triaged" || v == "in_progress" || v == "resolved" || v == "closed"
}
func terminalStatus(v string) bool { return v == "resolved" || v == "closed" }
func validateAttachments(items []Attachment) error {
	if len(items) > 10 {
		return ErrInvalid
	}
	for i := range items {
		a := &items[i]
		a.Name = strings.TrimSpace(filepath.Base(a.Name))
		if a.Name == "" || a.Size < 0 || a.Size > 1<<20 || len(a.Data) > 1400000 {
			return ErrInvalid
		}
		permitted := a.Kind == "log" && a.MediaType == "text/plain" ||
			a.Kind == "trace" && (a.MediaType == "application/json" || a.MediaType == "application/octet-stream") ||
			a.Kind == "sample" && (a.MediaType == "application/json" || a.MediaType == "text/plain" || a.MediaType == "application/octet-stream") ||
			a.Kind == "screenshot" && (a.MediaType == "image/png" || a.MediaType == "image/jpeg" || a.MediaType == "image/webp")
		if !permitted {
			return ErrInvalid
		}
	}
	return nil
}
