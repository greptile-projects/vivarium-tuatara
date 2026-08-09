// Package securityadvisories persists private vulnerability coordination records.
package securityadvisories

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
	"syscall"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
)

var (
	ErrNotFound = errors.New("security advisory not found")
	ErrInvalid  = errors.New("invalid security advisory")
	ErrConflict = errors.New("security advisory changed")
)

type AffectedRepository struct {
	RepositoryID string   `json:"repository_id"`
	Versions     []string `json:"versions"`
}

type Evidence struct {
	ID           string    `json:"id,omitempty"`
	Kind         string    `json:"kind,omitempty"`
	RepositoryID string    `json:"repository_id,omitempty"`
	CommitID     string    `json:"commit_id,omitempty"`
	ReleaseID    string    `json:"release_id,omitempty"`
	BuildID      string    `json:"build_id,omitempty"`
	ArtifactID   string    `json:"artifact_id,omitempty"`
	DeploymentID string    `json:"deployment_id,omitempty"`
	Dependency   string    `json:"dependency,omitempty"`
	Label        string    `json:"label"`
	Description  string    `json:"description"`
	CapturedAt   time.Time `json:"captured_at,omitempty"`
}

type Finding struct {
	ID              string    `json:"id"`
	Kind            string    `json:"kind"`
	ActorID         string    `json:"actor_id"`
	Statement       string    `json:"statement"`
	EvidenceIDs     []string  `json:"evidence_ids"`
	InvestigationID string    `json:"investigation_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}
type Impact struct {
	RepositoryID string    `json:"repository_id"`
	VersionLine  string    `json:"version_line"`
	Environment  string    `json:"environment"`
	State        string    `json:"state"`
	EvidenceIDs  []string  `json:"evidence_ids"`
	Rationale    string    `json:"rationale"`
	ActorID      string    `json:"actor_id"`
	UpdatedAt    time.Time `json:"updated_at"`
}
type Investigation struct {
	ID           string     `json:"id"`
	AgentID      string     `json:"agent_id"`
	InitiatorID  string     `json:"initiator_id"`
	CredentialID string     `json:"credential_id,omitempty"`
	Mandate      string     `json:"mandate"`
	State        string     `json:"state"`
	Evidence     []Evidence `json:"evidence"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type RepairTask struct {
	ID                string    `json:"id"`
	RepositoryID      string    `json:"repository_id"`
	VersionLine       string    `json:"version_line"`
	Title             string    `json:"title"`
	Mandate           string    `json:"mandate"`
	BaseCommitID      string    `json:"base_commit_id"`
	AssigneeID        string    `json:"assignee_id"`
	AssigneeKind      string    `json:"assignee_kind"`
	DependencyTaskIDs []string  `json:"dependency_task_ids"`
	Status            string    `json:"status"`
	CreatedBy         string    `json:"created_by"`
	CreatedAt         time.Time `json:"created_at"`
}

type RepairComment struct {
	ID        string    `json:"id"`
	ActorID   string    `json:"actor_id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}
type RepairReview struct {
	ID        string    `json:"id"`
	ActorID   string    `json:"actor_id"`
	Decision  string    `json:"decision"`
	Body      string    `json:"body"`
	CommitID  string    `json:"commit_id"`
	CreatedAt time.Time `json:"created_at"`
}
type RepairSession struct {
	ID           string          `json:"id"`
	TaskID       string          `json:"task_id"`
	RepositoryID string          `json:"repository_id"`
	InitiatorID  string          `json:"initiator_id"`
	WorkerID     string          `json:"worker_id"`
	CredentialID string          `json:"credential_id,omitempty"`
	Branch       string          `json:"branch"`
	BaseCommitID string          `json:"base_commit_id"`
	CommitID     string          `json:"commit_id,omitempty"`
	State        string          `json:"state"`
	Comments     []RepairComment `json:"comments"`
	Reviews      []RepairReview  `json:"reviews"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// SecurityReproduction is an embargoed, repository-owner-defined check. The
// command never enters ordinary pull/check APIs and is returned only through
// advisory-authorized reads.
type SecurityReproduction struct {
	ID           string               `json:"id"`
	RepositoryID string               `json:"repository_id"`
	VersionLine  string               `json:"version_line"`
	Definition   checkruns.Definition `json:"definition"`
	CreatedBy    string               `json:"created_by"`
	CreatedAt    time.Time            `json:"created_at"`
}

type RepairVerification struct {
	ID                 string           `json:"id"`
	TaskID             string           `json:"task_id"`
	SessionID          string           `json:"session_id"`
	RepositoryID       string           `json:"repository_id"`
	VersionLine        string           `json:"version_line"`
	CandidateCommitID  string           `json:"candidate_commit_id"`
	RequiredRunIDs     []string         `json:"required_run_ids"`
	ReproductionRunIDs []string         `json:"reproduction_run_ids"`
	RequestedBy        string           `json:"requested_by"`
	Approvals          []RepairApproval `json:"approvals"`
	CreatedAt          time.Time        `json:"created_at"`
}

type RepairApproval struct {
	ID        string    `json:"id"`
	ActorID   string    `json:"actor_id"`
	CreatedAt time.Time `json:"created_at"`
}

type ReleaseAttestation struct {
	ID              string    `json:"id"`
	VerificationID  string    `json:"verification_id"`
	RepositoryID    string    `json:"repository_id"`
	VersionLine     string    `json:"version_line"`
	ReleaseID       string    `json:"release_id"`
	ReleaseCommitID string    `json:"release_commit_id"`
	ArtifactIDs     []string  `json:"artifact_ids"`
	ArtifactSHA256  []string  `json:"artifact_sha256"`
	ActorID         string    `json:"actor_id"`
	CreatedAt       time.Time `json:"created_at"`
}

type Message struct {
	ID        string    `json:"id"`
	ActorID   string    `json:"actor_id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

type AccessEvent struct {
	ID        string    `json:"id"`
	ActorID   string    `json:"actor_id"`
	Action    string    `json:"action"`
	Detail    string    `json:"detail,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type Advisory struct {
	ID                    string                 `json:"id"`
	Title                 string                 `json:"title"`
	Description           string                 `json:"description"`
	AffectedRepositories  []AffectedRepository   `json:"affected_repositories"`
	Evidence              []Evidence             `json:"evidence"`
	Contact               string                 `json:"contact"`
	ReporterID            string                 `json:"reporter_id"`
	ResponseTeam          []string               `json:"response_team"`
	Severity              string                 `json:"severity"`
	EmbargoState          string                 `json:"embargo_state"`
	Messages              []Message              `json:"messages"`
	AccessLog             []AccessEvent          `json:"access_log"`
	Findings              []Finding              `json:"findings"`
	ImpactMatrix          []Impact               `json:"impact_matrix"`
	Investigations        []Investigation        `json:"investigations"`
	RepairTasks           []RepairTask           `json:"repair_tasks"`
	RepairSessions        []RepairSession        `json:"repair_sessions"`
	SecurityReproductions []SecurityReproduction `json:"security_reproductions"`
	RepairVerifications   []RepairVerification   `json:"repair_verifications"`
	ReleaseAttestations   []ReleaseAttestation   `json:"release_attestations"`
	Version               int                    `json:"version"`
	CreatedAt             time.Time              `json:"created_at"`
	UpdatedAt             time.Time              `json:"updated_at"`
}

type Store struct {
	root string
	mu   sync.Mutex
	now  func() time.Time
}

func New(root string) (*Store, error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	if err := os.Chmod(root, 0700); err != nil {
		return nil, err
	}
	return &Store{root: root, now: func() time.Time { return time.Now().UTC() }}, nil
}

func (s *Store) Create(v Advisory) (Advisory, error) {
	if err := validateCreate(v); err != nil {
		return Advisory{}, err
	}
	v.Title, v.Description, v.Contact = strings.TrimSpace(v.Title), strings.TrimSpace(v.Description), strings.TrimSpace(v.Contact)
	for i := range v.AffectedRepositories {
		for j := range v.AffectedRepositories[i].Versions {
			v.AffectedRepositories[i].Versions[j] = strings.TrimSpace(v.AffectedRepositories[i].Versions[j])
		}
	}
	for i := range v.Evidence {
		v.Evidence[i].Label, v.Evidence[i].Description = strings.TrimSpace(v.Evidence[i].Label), strings.TrimSpace(v.Evidence[i].Description)
	}
	now := s.now()
	v.ID, v.Severity, v.EmbargoState, v.Version = mustID(), "untriaged", "reported", 1
	v.CreatedAt, v.UpdatedAt = now, now
	v.ResponseTeam, v.Messages = []string{}, []Message{}
	v.Findings, v.ImpactMatrix, v.Investigations = []Finding{}, []Impact{}, []Investigation{}
	v.RepairTasks, v.RepairSessions = []RepairTask{}, []RepairSession{}
	v.SecurityReproductions, v.RepairVerifications, v.ReleaseAttestations = []SecurityReproduction{}, []RepairVerification{}, []ReleaseAttestation{}
	for i := range v.Evidence {
		v.Evidence[i].ID, v.Evidence[i].CapturedAt = mustID(), now
	}
	v.AccessLog = []AccessEvent{{ID: mustID(), ActorID: v.ReporterID, Action: "reported", CreatedAt: now}}
	err := s.mutate(func() error { return s.write(v) })
	return v, err
}

func (s *Store) AddSecurityReproduction(id, actor string, reproduction SecurityReproduction) (Advisory, SecurityReproduction, error) {
	var out SecurityReproduction
	v, err := s.update(id, func(v *Advisory) error {
		reproduction.VersionLine = strings.TrimSpace(reproduction.VersionLine)
		definition := reproduction.Definition
		body, marshalErr := json.Marshal(checkruns.Config{Version: 1, Checks: []checkruns.Definition{definition}})
		validated, parseErr := checkruns.ParseConfig(body)
		if marshalErr != nil || parseErr != nil || !validID(actor) || !validID(reproduction.RepositoryID) || !affectedVersion(v, reproduction.RepositoryID, reproduction.VersionLine) {
			return ErrInvalid
		}
		for _, existing := range v.SecurityReproductions {
			if existing.RepositoryID == reproduction.RepositoryID && existing.VersionLine == reproduction.VersionLine && existing.Definition.Name == validated.Checks[0].Name {
				return ErrConflict
			}
		}
		now := s.now()
		reproduction.ID, reproduction.Definition, reproduction.CreatedBy, reproduction.CreatedAt = mustID(), validated.Checks[0], actor, now
		out = reproduction
		v.SecurityReproductions = append(v.SecurityReproductions, reproduction)
		v.Version++
		v.AccessLog = append(v.AccessLog, AccessEvent{ID: mustID(), ActorID: actor, Action: "security_reproduction_defined", Detail: reproduction.ID, CreatedAt: now})
		return nil
	})
	return v, out, err
}

func affectedVersion(v *Advisory, repositoryID, versionLine string) bool {
	for _, affected := range v.AffectedRepositories {
		if affected.RepositoryID == repositoryID && slicesContains(affected.Versions, versionLine) {
			return true
		}
	}
	return false
}

func slicesContains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func (s *Store) StartRepairVerification(id, actor, taskID, sessionID string, requiredRunIDs, reproductionRunIDs []string) (Advisory, RepairVerification, error) {
	var out RepairVerification
	v, err := s.update(id, func(v *Advisory) error {
		var task *RepairTask
		var session *RepairSession
		for i := range v.RepairTasks {
			if v.RepairTasks[i].ID == taskID {
				task = &v.RepairTasks[i]
			}
		}
		for i := range v.RepairSessions {
			if v.RepairSessions[i].ID == sessionID {
				session = &v.RepairSessions[i]
			}
		}
		if task == nil || session == nil || session.TaskID != task.ID || session.State != "completed" || session.CommitID == "" || len(requiredRunIDs)+len(reproductionRunIDs) == 0 || !validID(actor) {
			return ErrInvalid
		}
		reviewed := false
		for _, review := range session.Reviews {
			if review.Decision == "approve" && review.CommitID == session.CommitID && review.ActorID != session.WorkerID {
				reviewed = true
			}
		}
		if !reviewed {
			return ErrInvalid
		}
		for _, existing := range v.RepairVerifications {
			if existing.SessionID == sessionID && existing.CandidateCommitID == session.CommitID {
				return ErrConflict
			}
		}
		now := s.now()
		out = RepairVerification{ID: mustID(), TaskID: task.ID, SessionID: session.ID, RepositoryID: task.RepositoryID, VersionLine: task.VersionLine, CandidateCommitID: session.CommitID, RequiredRunIDs: append([]string{}, requiredRunIDs...), ReproductionRunIDs: append([]string{}, reproductionRunIDs...), RequestedBy: actor, Approvals: []RepairApproval{}, CreatedAt: now}
		v.RepairVerifications = append(v.RepairVerifications, out)
		v.Version++
		v.AccessLog = append(v.AccessLog, AccessEvent{ID: mustID(), ActorID: actor, Action: "repair_verification_started", Detail: out.ID, CreatedAt: now})
		return nil
	})
	return v, out, err
}

func (s *Store) ApproveRepairVerification(id, actor, verificationID string) (Advisory, RepairVerification, error) {
	var out RepairVerification
	v, err := s.update(id, func(v *Advisory) error {
		for i := range v.RepairVerifications {
			x := &v.RepairVerifications[i]
			if x.ID != verificationID {
				continue
			}
			workerID := ""
			for _, session := range v.RepairSessions {
				if session.ID == x.SessionID {
					workerID = session.WorkerID
				}
			}
			if !validID(actor) || actor == workerID {
				return ErrInvalid
			}
			for _, approval := range x.Approvals {
				if approval.ActorID == actor {
					out = *x
					return nil
				}
			}
			now := s.now()
			x.Approvals = append(x.Approvals, RepairApproval{ID: mustID(), ActorID: actor, CreatedAt: now})
			out = *x
			v.Version++
			v.AccessLog = append(v.AccessLog, AccessEvent{ID: mustID(), ActorID: actor, Action: "repair_verification_approved", Detail: x.ID, CreatedAt: now})
			return nil
		}
		return ErrNotFound
	})
	return v, out, err
}

func (s *Store) AddReleaseAttestation(id, actor string, attestation ReleaseAttestation) (Advisory, ReleaseAttestation, error) {
	var out ReleaseAttestation
	v, err := s.update(id, func(v *Advisory) error {
		found := false
		for _, verification := range v.RepairVerifications {
			if verification.ID == attestation.VerificationID && verification.RepositoryID == attestation.RepositoryID && verification.VersionLine == attestation.VersionLine && len(verification.Approvals) > 0 {
				found = true
			}
		}
		if !found || !validID(actor) || !validID(attestation.ReleaseID) || len(attestation.ReleaseCommitID) != 40 || len(attestation.ArtifactIDs) == 0 || len(attestation.ArtifactIDs) != len(attestation.ArtifactSHA256) {
			return ErrInvalid
		}
		for _, existing := range v.ReleaseAttestations {
			if existing.VerificationID == attestation.VerificationID && existing.ReleaseID == attestation.ReleaseID {
				out = existing
				return nil
			}
		}
		now := s.now()
		attestation.ID, attestation.ActorID, attestation.CreatedAt = mustID(), actor, now
		out = attestation
		v.ReleaseAttestations = append(v.ReleaseAttestations, attestation)
		v.Version++
		v.AccessLog = append(v.AccessLog, AccessEvent{ID: mustID(), ActorID: actor, Action: "fixed_release_attested", Detail: attestation.ID, CreatedAt: now})
		return nil
	})
	return v, out, err
}

func (s *Store) AddRepairTask(id, actor string, task RepairTask) (Advisory, RepairTask, error) {
	var out RepairTask
	v, err := s.update(id, func(v *Advisory) error {
		task.Title, task.Mandate, task.VersionLine = strings.TrimSpace(task.Title), strings.TrimSpace(task.Mandate), strings.TrimSpace(task.VersionLine)
		if !validID(actor) || !validID(task.RepositoryID) || !validID(task.AssigneeID) || len(task.BaseCommitID) != 40 || task.Title == "" || len(task.Title) > 200 || task.Mandate == "" || len(task.Mandate) > 10000 || task.VersionLine == "" || len(task.VersionLine) > 200 || !oneOf(task.AssigneeKind, "human", "agent") || len(task.DependencyTaskIDs) > 20 {
			return ErrInvalid
		}
		affected := false
		for _, x := range v.AffectedRepositories {
			if x.RepositoryID == task.RepositoryID {
				for _, line := range x.Versions {
					if line == task.VersionLine {
						affected = true
					}
				}
			}
		}
		if !affected {
			return ErrInvalid
		}
		seen := map[string]bool{}
		for _, dependency := range task.DependencyTaskIDs {
			if seen[dependency] {
				return ErrInvalid
			}
			seen[dependency] = true
			found := false
			for _, existing := range v.RepairTasks {
				if existing.ID == dependency {
					found = true
				}
			}
			if !found {
				return ErrInvalid
			}
		}
		now := s.now()
		task.ID, task.Status, task.CreatedBy, task.CreatedAt = mustID(), "open", actor, now
		out = task
		v.RepairTasks = append(v.RepairTasks, task)
		v.Version++
		v.AccessLog = append(v.AccessLog, AccessEvent{ID: mustID(), ActorID: actor, Action: "repair_task_created", Detail: task.ID, CreatedAt: now})
		return nil
	})
	return v, out, err
}

func (s *Store) StartRepairSession(id, actor, taskID, credentialID, branch string) (Advisory, RepairSession, error) {
	var out RepairSession
	v, err := s.update(id, func(v *Advisory) error {
		var task *RepairTask
		for i := range v.RepairTasks {
			if v.RepairTasks[i].ID == taskID {
				task = &v.RepairTasks[i]
			}
		}
		if task == nil || task.Status != "open" || !validID(actor) || !validID(credentialID) || branch == "" {
			return ErrInvalid
		}
		for _, x := range v.RepairSessions {
			if x.TaskID == taskID && x.State == "active" {
				return ErrConflict
			}
		}
		now := s.now()
		out = RepairSession{ID: mustID(), TaskID: taskID, RepositoryID: task.RepositoryID, InitiatorID: actor, WorkerID: task.AssigneeID, CredentialID: credentialID, Branch: branch, BaseCommitID: task.BaseCommitID, State: "active", Comments: []RepairComment{}, Reviews: []RepairReview{}, CreatedAt: now, UpdatedAt: now}
		v.RepairSessions = append(v.RepairSessions, out)
		v.Version++
		v.AccessLog = append(v.AccessLog, AccessEvent{ID: mustID(), ActorID: actor, Action: "repair_session_started", Detail: out.ID, CreatedAt: now})
		return nil
	})
	return v, out, err
}

func (s *Store) UpdateRepairSession(id, actor, sessionID, action, body, decision, commitID string) (Advisory, RepairSession, error) {
	var out RepairSession
	v, err := s.update(id, func(v *Advisory) error {
		var session *RepairSession
		for i := range v.RepairSessions {
			if v.RepairSessions[i].ID == sessionID {
				session = &v.RepairSessions[i]
			}
		}
		if session == nil || !validID(actor) {
			return ErrNotFound
		}
		now := s.now()
		body = strings.TrimSpace(body)
		switch action {
		case "comment":
			if body == "" || len(body) > 20000 {
				return ErrInvalid
			}
			session.Comments = append(session.Comments, RepairComment{ID: mustID(), ActorID: actor, Body: body, CreatedAt: now})
		case "review":
			if !oneOf(decision, "approve", "request_changes") || len(commitID) != 40 || len(body) > 10000 {
				return ErrInvalid
			}
			session.Reviews = append(session.Reviews, RepairReview{ID: mustID(), ActorID: actor, Decision: decision, Body: body, CommitID: commitID, CreatedAt: now})
		case "complete":
			if session.State != "active" || len(commitID) != 40 {
				return ErrInvalid
			}
			session.State, session.CommitID = "completed", commitID
			for i := range v.RepairTasks {
				if v.RepairTasks[i].ID == session.TaskID {
					v.RepairTasks[i].Status = "review"
				}
			}
		case "revoke":
			if session.State != "active" {
				return ErrInvalid
			}
			session.State = "revoked"
		default:
			return ErrInvalid
		}
		session.UpdatedAt = now
		out = *session
		v.Version++
		v.AccessLog = append(v.AccessLog, AccessEvent{ID: mustID(), ActorID: actor, Action: "repair_" + action, Detail: sessionID, CreatedAt: now})
		return nil
	})
	return v, out, err
}

func (s *Store) AddEvidence(id, actor string, evidence Evidence) (Advisory, error) {
	return s.update(id, func(v *Advisory) error {
		evidence.Label, evidence.Description, evidence.Dependency = strings.TrimSpace(evidence.Label), strings.TrimSpace(evidence.Description), strings.TrimSpace(evidence.Dependency)
		if !validID(actor) || !oneOf(evidence.Kind, "commit", "dependency", "build", "artifact", "release", "deployment") || evidence.Label == "" || len(evidence.Label) > 200 || len(evidence.Description) > 10000 {
			return ErrInvalid
		}
		evidence.ID, evidence.CapturedAt = mustID(), s.now()
		v.Evidence = append(v.Evidence, evidence)
		v.Version++
		v.AccessLog = append(v.AccessLog, AccessEvent{ID: mustID(), ActorID: actor, Action: "evidence_connected", Detail: evidence.Kind + " / " + evidence.Label, CreatedAt: evidence.CapturedAt})
		return nil
	})
}

func evidenceSelected(v *Advisory, ids []string) bool {
	for _, id := range ids {
		found := false
		for _, e := range v.Evidence {
			if e.ID == id {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
func (s *Store) AddFinding(id, actor, kind, statement, investigationID string, evidenceIDs []string) (Advisory, error) {
	return s.update(id, func(v *Advisory) error {
		statement = strings.TrimSpace(statement)
		if !validID(actor) || !oneOf(kind, "hypothesis", "conclusion", "uncertainty") || statement == "" || len(statement) > 10000 || len(evidenceIDs) > 50 || !evidenceSelected(v, evidenceIDs) {
			return ErrInvalid
		}
		now := s.now()
		v.Findings = append(v.Findings, Finding{ID: mustID(), Kind: kind, ActorID: actor, Statement: statement, EvidenceIDs: evidenceIDs, InvestigationID: investigationID, CreatedAt: now})
		v.Version++
		v.AccessLog = append(v.AccessLog, AccessEvent{ID: mustID(), ActorID: actor, Action: kind + "_recorded", CreatedAt: now})
		return nil
	})
}
func (s *Store) SetImpact(id, actor string, expected int, impact Impact) (Advisory, error) {
	return s.update(id, func(v *Advisory) error {
		impact.VersionLine, impact.Environment, impact.Rationale = strings.TrimSpace(impact.VersionLine), strings.TrimSpace(impact.Environment), strings.TrimSpace(impact.Rationale)
		if v.Version != expected {
			return ErrConflict
		}
		if !validID(actor) || !validID(impact.RepositoryID) || impact.VersionLine == "" || impact.Environment == "" || len(impact.VersionLine) > 200 || len(impact.Environment) > 200 || len(impact.Rationale) > 10000 || !oneOf(impact.State, "confirmed", "suspected", "unaffected", "fixed") || !evidenceSelected(v, impact.EvidenceIDs) {
			return ErrInvalid
		}
		now := s.now()
		impact.ActorID, impact.UpdatedAt = actor, now
		replaced := false
		for i := range v.ImpactMatrix {
			if v.ImpactMatrix[i].RepositoryID == impact.RepositoryID && v.ImpactMatrix[i].VersionLine == impact.VersionLine && v.ImpactMatrix[i].Environment == impact.Environment {
				v.ImpactMatrix[i] = impact
				replaced = true
			}
		}
		if !replaced {
			v.ImpactMatrix = append(v.ImpactMatrix, impact)
		}
		v.Version++
		v.AccessLog = append(v.AccessLog, AccessEvent{ID: mustID(), ActorID: actor, Action: "impact_updated", Detail: impact.VersionLine + " / " + impact.Environment + " / " + impact.State, CreatedAt: now})
		return nil
	})
}
func (s *Store) StartInvestigation(id, actor, agent, credential, mandate string, evidenceIDs []string) (Advisory, Investigation, error) {
	var out Investigation
	v, e := s.update(id, func(v *Advisory) error {
		mandate = strings.TrimSpace(mandate)
		if !validID(actor) || !validID(agent) || !validID(credential) || mandate == "" || len(mandate) > 10000 || len(evidenceIDs) == 0 || !evidenceSelected(v, evidenceIDs) {
			return ErrInvalid
		}
		selected := []Evidence{}
		for _, eid := range evidenceIDs {
			for _, x := range v.Evidence {
				if x.ID == eid {
					selected = append(selected, x)
				}
			}
		}
		now := s.now()
		out = Investigation{ID: mustID(), AgentID: agent, InitiatorID: actor, CredentialID: credential, Mandate: mandate, State: "running", Evidence: selected, CreatedAt: now, UpdatedAt: now}
		v.Investigations = append(v.Investigations, out)
		v.Version++
		v.AccessLog = append(v.AccessLog, AccessEvent{ID: mustID(), ActorID: actor, Action: "investigation_delegated", Detail: out.ID, CreatedAt: now})
		return nil
	})
	return v, out, e
}
func (s *Store) Investigation(id, investigationID, credentialID string) (Advisory, Investigation, error) {
	v, e := s.Get(id)
	if e != nil {
		return v, Investigation{}, e
	}
	for _, x := range v.Investigations {
		if x.ID == investigationID && x.CredentialID == credentialID && x.State == "running" {
			return v, x, nil
		}
	}
	return v, Investigation{}, ErrNotFound
}

func (s *Store) Get(id string) (Advisory, error) {
	var v Advisory
	if !validID(id) {
		return v, ErrNotFound
	}
	if err := s.read(id, &v); err != nil {
		return Advisory{}, ErrNotFound
	}
	return v, nil
}

func (s *Store) List() ([]Advisory, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	out := []Advisory{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		var v Advisory
		if err := s.read(strings.TrimSuffix(entry.Name(), ".json"), &v); err != nil {
			continue
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *Store) RecordAccess(id, actor string) (Advisory, error) {
	return s.update(id, func(v *Advisory) error {
		if !validID(actor) {
			return ErrInvalid
		}
		v.AccessLog = append(v.AccessLog, AccessEvent{ID: mustID(), ActorID: actor, Action: "viewed", CreatedAt: s.now()})
		return nil
	})
}

func (s *Store) Triage(id, actor string, expected int, severity, embargo string) (Advisory, error) {
	return s.update(id, func(v *Advisory) error {
		if v.Version != expected {
			return ErrConflict
		}
		if !validID(actor) || !oneOf(severity, "low", "moderate", "high", "critical") || !oneOf(embargo, "reported", "triaging", "embargoed", "coordinating") {
			return ErrInvalid
		}
		v.Severity, v.EmbargoState, v.Version = severity, embargo, v.Version+1
		v.AccessLog = append(v.AccessLog, AccessEvent{ID: mustID(), ActorID: actor, Action: "triage_updated", Detail: severity + " / " + embargo, CreatedAt: s.now()})
		return nil
	})
}

func (s *Store) Invite(id, actor, userID string) (Advisory, error) {
	return s.update(id, func(v *Advisory) error {
		if !validID(actor) || !validID(userID) {
			return ErrInvalid
		}
		for _, id := range v.ResponseTeam {
			if id == userID {
				return nil
			}
		}
		if userID == v.ReporterID {
			return nil
		}
		if len(v.ResponseTeam) >= 20 {
			return ErrInvalid
		}
		v.ResponseTeam = append(v.ResponseTeam, userID)
		v.Version++
		v.AccessLog = append(v.AccessLog, AccessEvent{ID: mustID(), ActorID: actor, Action: "responder_invited", Detail: userID, CreatedAt: s.now()})
		return nil
	})
}

func (s *Store) AddMessage(id, actor, body string) (Advisory, error) {
	return s.update(id, func(v *Advisory) error {
		body = strings.TrimSpace(body)
		if !validID(actor) || body == "" || len(body) > 20000 {
			return ErrInvalid
		}
		now := s.now()
		v.Messages = append(v.Messages, Message{ID: mustID(), ActorID: actor, Body: body, CreatedAt: now})
		v.Version++
		v.AccessLog = append(v.AccessLog, AccessEvent{ID: mustID(), ActorID: actor, Action: "message_added", CreatedAt: now})
		return nil
	})
}

func (s *Store) update(id string, fn func(*Advisory) error) (Advisory, error) {
	var v Advisory
	err := s.mutate(func() error {
		if err := s.read(id, &v); err != nil {
			return ErrNotFound
		}
		if err := fn(&v); err != nil {
			return err
		}
		v.UpdatedAt = s.now()
		return s.write(v)
	})
	return v, err
}

func validateCreate(v Advisory) error {
	v.Title, v.Description, v.Contact = strings.TrimSpace(v.Title), strings.TrimSpace(v.Description), strings.TrimSpace(v.Contact)
	if !validID(v.ReporterID) || v.Title == "" || len(v.Title) > 200 || v.Description == "" || len(v.Description) > 20000 || v.Contact == "" || len(v.Contact) > 500 || len(v.AffectedRepositories) == 0 || len(v.AffectedRepositories) > 20 || len(v.Evidence) > 20 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, scope := range v.AffectedRepositories {
		if !validID(scope.RepositoryID) || seen[scope.RepositoryID] || len(scope.Versions) == 0 || len(scope.Versions) > 50 {
			return ErrInvalid
		}
		seen[scope.RepositoryID] = true
		for _, version := range scope.Versions {
			if strings.TrimSpace(version) == "" || len(version) > 200 {
				return ErrInvalid
			}
		}
	}
	for _, evidence := range v.Evidence {
		if strings.TrimSpace(evidence.Label) == "" || len(evidence.Label) > 200 || strings.TrimSpace(evidence.Description) == "" || len(evidence.Description) > 10000 {
			return ErrInvalid
		}
	}
	return nil
}

func oneOf(v string, choices ...string) bool {
	for _, choice := range choices {
		if v == choice {
			return true
		}
	}
	return false
}
func validID(v string) bool {
	if len(v) != 32 {
		return false
	}
	_, err := hex.DecodeString(v)
	return err == nil
}
func mustID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func (s *Store) path(id string) string { return filepath.Join(s.root, id+".json") }
func (s *Store) read(id string, out *Advisory) error {
	data, err := os.ReadFile(s.path(id))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return err
	}
	if out.SecurityReproductions == nil {
		out.SecurityReproductions = []SecurityReproduction{}
	}
	if out.RepairVerifications == nil {
		out.RepairVerifications = []RepairVerification{}
	}
	if out.ReleaseAttestations == nil {
		out.ReleaseAttestations = []ReleaseAttestation{}
	}
	return nil
}
func (s *Store) write(v Advisory) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.root, ".advisory-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(data)
	}
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(name, s.path(v.ID))
	}
	return err
}
func (s *Store) mutate(fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	return fn()
}
