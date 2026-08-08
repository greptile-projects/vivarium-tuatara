// Package incidents persists the shared operating picture for service incidents.
package incidents

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
)

var (
	ErrNotFound = errors.New("incident not found")
	ErrInvalid  = errors.New("invalid incident")
	ErrConflict = errors.New("incident changed")
)

type Scope struct {
	RepositoryID   string   `json:"repository_id"`
	EnvironmentIDs []string `json:"environment_ids"`
}
type Source struct {
	RepositoryID string `json:"repository_id"`
	DeploymentID string `json:"deployment_id"`
	Stage        string `json:"stage,omitempty"`
	Signal       string `json:"signal,omitempty"`
}
type Role struct {
	Name   string `json:"name"`
	UserID string `json:"user_id"`
}
type Entry struct {
	ID              string     `json:"id"`
	Kind            string     `json:"kind"`
	ActorID         string     `json:"actor_id"`
	Message         string     `json:"message"`
	Audience        string     `json:"audience"`
	CreatedAt       time.Time  `json:"created_at"`
	AcknowledgedBy  []string   `json:"acknowledged_by,omitempty"`
	Evidence        []Evidence `json:"evidence,omitempty"`
	InvestigationID string     `json:"investigation_id,omitempty"`
}
type Revision struct {
	RepositoryID string `json:"repository_id"`
	CommitID     string `json:"commit_id"`
	Label        string `json:"label"`
}
type EvidenceContext struct {
	Selection Evidence        `json:"selection"`
	Resource  json.RawMessage `json:"resource"`
}
type Investigation struct {
	ID                 string            `json:"id"`
	AgentID            string            `json:"agent_id"`
	InitiatorID        string            `json:"initiator_id"`
	CredentialID       string            `json:"credential_id,omitempty"`
	Mandate            string            `json:"mandate"`
	State              string            `json:"state"`
	Evidence           []Evidence        `json:"evidence"`
	Revisions          []Revision        `json:"revisions"`
	OperationalContext []EvidenceContext `json:"operational_context"`
	Access             []string          `json:"access"`
	CreatedAt          time.Time         `json:"created_at"`
	UpdatedAt          time.Time         `json:"updated_at"`
}
type HealthCriterion struct {
	Stage  string `json:"stage"`
	Signal string `json:"signal"`
}
type ActionAttempt struct {
	ID         string    `json:"id"`
	ActorID    string    `json:"actor_id"`
	Outcome    string    `json:"outcome"`
	ResourceID string    `json:"resource_id,omitempty"`
	Message    string    `json:"message"`
	CreatedAt  time.Time `json:"created_at"`
}
type ActionDecision struct {
	ActorID   string    `json:"actor_id"`
	Decision  string    `json:"decision"`
	Message   string    `json:"message"`
	Override  bool      `json:"override,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
type Action struct {
	ID             string            `json:"id"`
	OperationID    string            `json:"operation_id"`
	Kind           string            `json:"kind"`
	RepositoryID   string            `json:"repository_id"`
	DeploymentID   string            `json:"deployment_id"`
	Rationale      string            `json:"rationale"`
	Status         string            `json:"status"`
	ProposedBy     string            `json:"proposed_by"`
	Evidence       []Evidence        `json:"evidence"`
	HealthCriteria []HealthCriterion `json:"health_criteria"`
	Decisions      []ActionDecision  `json:"decisions"`
	Attempts       []ActionAttempt   `json:"attempts"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}
type Evidence struct {
	Kind         string     `json:"kind"`
	RepositoryID string     `json:"repository_id,omitempty"`
	ResourceID   string     `json:"resource_id"`
	Label        string     `json:"label"`
	Query        string     `json:"query,omitempty"`
	WindowStart  *time.Time `json:"window_start,omitempty"`
	WindowEnd    *time.Time `json:"window_end,omitempty"`
	CapturedAt   time.Time  `json:"captured_at"`
}
type Review struct {
	Impact              string    `json:"impact"`
	Timeline            string    `json:"timeline"`
	ContributingFactors []string  `json:"contributing_factors"`
	Conclusions         string    `json:"conclusions"`
	PublishedBy         string    `json:"published_by"`
	PublishedAt         time.Time `json:"published_at"`
}
type CommitmentProgress struct {
	State         string   `json:"state"`
	PullRequestID string   `json:"pull_request_id,omitempty"`
	CheckStates   []string `json:"check_states,omitempty"`
	ReleaseIDs    []string `json:"release_ids,omitempty"`
	DeploymentIDs []string `json:"deployment_ids,omitempty"`
}
type Commitment struct {
	ID           string             `json:"id"`
	OperationID  string             `json:"operation_id"`
	RepositoryID string             `json:"repository_id"`
	ProposalID   string             `json:"proposal_id"`
	TaskID       string             `json:"task_id"`
	AssigneeID   string             `json:"assignee_id"`
	DueAt        time.Time          `json:"due_at"`
	CreatedBy    string             `json:"created_by"`
	CreatedAt    time.Time          `json:"created_at"`
	Progress     CommitmentProgress `json:"progress"`
}
type Incident struct {
	ID             string          `json:"id"`
	Title          string          `json:"title"`
	Summary        string          `json:"summary"`
	Severity       string          `json:"severity"`
	Status         string          `json:"status"`
	Scopes         []Scope         `json:"scopes"`
	Roles          []Role          `json:"roles"`
	Source         *Source         `json:"source,omitempty"`
	DeclaredBy     string          `json:"declared_by"`
	Timeline       []Entry         `json:"timeline"`
	Investigations []Investigation `json:"investigations"`
	Actions        []Action        `json:"actions"`
	Review         *Review         `json:"review,omitempty"`
	Commitments    []Commitment    `json:"commitments"`
	Version        int             `json:"version"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	ResolvedAt     *time.Time      `json:"resolved_at,omitempty"`
}

func (s *Store) Resolve(id, actor string, expected int, impact, timeline string, factors []string, conclusions string) (Incident, error) {
	var v Incident
	err := s.mutate(func() error {
		if err := s.read(id, &v); err != nil {
			return err
		}
		impact, timeline, conclusions = strings.TrimSpace(impact), strings.TrimSpace(timeline), strings.TrimSpace(conclusions)
		if v.Version != expected {
			return ErrConflict
		}
		if !validID(actor) || impact == "" || timeline == "" || conclusions == "" || len(impact) > 10000 || len(timeline) > 20000 || len(conclusions) > 10000 || len(factors) == 0 || len(factors) > 20 {
			return ErrInvalid
		}
		clean := make([]string, len(factors))
		for i, factor := range factors {
			clean[i] = strings.TrimSpace(factor)
			if clean[i] == "" || len(clean[i]) > 1000 {
				return ErrInvalid
			}
		}
		now := s.now()
		v.Review = &Review{Impact: impact, Timeline: timeline, ContributingFactors: clean, Conclusions: conclusions, PublishedBy: actor, PublishedAt: now}
		v.Status, v.ResolvedAt, v.UpdatedAt = "resolved", &now, now
		v.Version++
		v.Timeline = append(v.Timeline, Entry{ID: mustID(), Kind: "incident_resolved", ActorID: actor, Message: conclusions, Audience: "participants", CreatedAt: now})
		return s.write(v)
	})
	return v, err
}

func (s *Store) LinkCommitment(id, operationID, actor, repositoryID, proposalID, taskID, assigneeID string, dueAt time.Time) (Incident, Commitment, error) {
	var v Incident
	var out Commitment
	err := s.mutate(func() error {
		if err := s.read(id, &v); err != nil {
			return err
		}
		if v.Status != "resolved" || v.Review == nil || !validID(operationID) || !validID(actor) || !validID(repositoryID) || !validID(proposalID) || !validID(taskID) || !validID(assigneeID) || dueAt.IsZero() {
			return ErrInvalid
		}
		for _, existing := range v.Commitments {
			if existing.OperationID == operationID {
				if existing.CreatedBy != actor || existing.RepositoryID != repositoryID || existing.ProposalID != proposalID || existing.TaskID != taskID || existing.AssigneeID != assigneeID || !existing.DueAt.Equal(dueAt) {
					return ErrConflict
				}
				out = existing
				return nil
			}
		}
		now := s.now()
		out = Commitment{ID: mustID(), OperationID: operationID, RepositoryID: repositoryID, ProposalID: proposalID, TaskID: taskID, AssigneeID: assigneeID, DueAt: dueAt.UTC(), CreatedBy: actor, CreatedAt: now, Progress: CommitmentProgress{State: "committed"}}
		v.Commitments = append(v.Commitments, out)
		v.Version++
		v.UpdatedAt = now
		v.Timeline = append(v.Timeline, Entry{ID: mustID(), Kind: "corrective_work_committed", ActorID: actor, Message: "Corrective work assigned through proposal " + proposalID + " and task " + taskID + ".", Audience: "participants", CreatedAt: now})
		return s.write(v)
	})
	return v, out, err
}

func (s *Store) ProposeAction(id, operationID, actor, kind, repositoryID, deploymentID, rationale string, evidence []Evidence, criteria []HealthCriterion) (Incident, Action, error) {
	var v Incident
	var out Action
	e := s.mutate(func() error {
		if e := s.read(id, &v); e != nil {
			return e
		}
		rationale = strings.TrimSpace(rationale)
		if !validID(operationID) || !validID(actor) || !validID(repositoryID) || !validID(deploymentID) || !validActionKind(kind) || rationale == "" || len(rationale) > 10000 || len(evidence) == 0 || len(evidence) > 20 || len(criteria) == 0 || len(criteria) > 20 {
			return ErrInvalid
		}
		for _, x := range evidence {
			if !validEvidence(x) {
				return ErrInvalid
			}
		}
		for _, x := range criteria {
			if strings.TrimSpace(x.Stage) == "" || strings.TrimSpace(x.Signal) == "" || len(x.Stage) > 100 || len(x.Signal) > 100 {
				return ErrInvalid
			}
		}
		for _, existing := range v.Actions {
			if existing.OperationID != operationID {
				continue
			}
			if existing.ProposedBy != actor || existing.Kind != kind || existing.RepositoryID != repositoryID || existing.DeploymentID != deploymentID || existing.Rationale != rationale || !sameEvidence(existing.Evidence, evidence) || !sameCriteria(existing.HealthCriteria, criteria) {
				return ErrConflict
			}
			out = existing
			return nil
		}
		now := s.now()
		out = Action{ID: mustID(), OperationID: operationID, Kind: kind, RepositoryID: repositoryID, DeploymentID: deploymentID, Rationale: rationale, Status: "proposed", ProposedBy: actor, Evidence: evidence, HealthCriteria: criteria, Decisions: []ActionDecision{}, Attempts: []ActionAttempt{}, CreatedAt: now, UpdatedAt: now}
		v.Actions = append(v.Actions, out)
		v.Version++
		v.UpdatedAt = now
		v.Timeline = append(v.Timeline, Entry{ID: mustID(), Kind: "mitigation_proposed", ActorID: actor, Message: rationale, Audience: "participants", Evidence: evidence, CreatedAt: now})
		return s.write(v)
	})
	return v, out, e
}

func (s *Store) DecideAction(id, actionID, actor, decision, message string, override bool) (Incident, Action, error) {
	var v Incident
	var out Action
	e := s.mutate(func() error {
		if e := s.read(id, &v); e != nil {
			return e
		}
		var x *Action
		for i := range v.Actions {
			if v.Actions[i].ID == actionID {
				x = &v.Actions[i]
			}
		}
		message = strings.TrimSpace(message)
		if x == nil {
			return ErrNotFound
		}
		if !validID(actor) || (decision != "approve" && decision != "reject") || len(message) > 10000 || x.Status != "proposed" {
			return ErrConflict
		}
		if decision == "approve" && actor == x.ProposedBy && !override {
			return ErrConflict
		}
		now := s.now()
		x.Status = map[bool]string{true: "approved", false: "rejected"}[decision == "approve"]
		x.UpdatedAt = now
		x.Decisions = append(x.Decisions, ActionDecision{ActorID: actor, Decision: decision, Message: message, Override: override, CreatedAt: now})
		v.Version++
		v.UpdatedAt = now
		out = *x
		v.Timeline = append(v.Timeline, Entry{ID: mustID(), Kind: "mitigation_" + decision, ActorID: actor, Message: message, Audience: "participants", Evidence: x.Evidence, CreatedAt: now})
		return s.write(v)
	})
	return v, out, e
}

func (s *Store) RecordActionAttempt(id, actionID, operationID, actor, outcome, resourceID, message string) (Incident, Action, error) {
	var v Incident
	var out Action
	e := s.mutate(func() error {
		if e := s.read(id, &v); e != nil {
			return e
		}
		var x *Action
		for i := range v.Actions {
			if v.Actions[i].ID == actionID {
				x = &v.Actions[i]
			}
		}
		message = strings.TrimSpace(message)
		resourceID = strings.TrimSpace(resourceID)
		if x == nil {
			return ErrNotFound
		}
		if !validID(operationID) || !validID(actor) || (outcome != "pending" && outcome != "started" && outcome != "failed" && outcome != "recovered") || message == "" || len(message) > 10000 || x.Status == "proposed" || x.Status == "rejected" {
			return ErrConflict
		}
		for i := range x.Attempts {
			existing := &x.Attempts[i]
			if existing.ID != operationID {
				continue
			}
			if existing.ActorID != actor {
				return ErrConflict
			}
			if existing.Outcome == outcome && existing.ResourceID == resourceID && existing.Message == message {
				out = *x
				return nil
			}
			if existing.Outcome != "pending" || (outcome != "started" && outcome != "failed") {
				return ErrConflict
			}
			now := s.now()
			existing.Outcome, existing.ResourceID, existing.Message = outcome, resourceID, message
			x.Status = map[string]string{"started": "executing", "failed": "failed"}[outcome]
			x.UpdatedAt = now
			v.Version++
			v.UpdatedAt = now
			out = *x
			v.Timeline = append(v.Timeline, Entry{ID: mustID(), Kind: "mitigation_" + outcome, ActorID: actor, Message: message, Audience: "participants", Evidence: x.Evidence, CreatedAt: now})
			return s.write(v)
		}
		now := s.now()
		x.Status = map[string]string{"pending": "executing", "started": "executing", "failed": "failed", "recovered": "recovered"}[outcome]
		x.UpdatedAt = now
		x.Attempts = append(x.Attempts, ActionAttempt{ID: operationID, ActorID: actor, Outcome: outcome, ResourceID: resourceID, Message: message, CreatedAt: now})
		v.Version++
		v.UpdatedAt = now
		out = *x
		v.Timeline = append(v.Timeline, Entry{ID: mustID(), Kind: "mitigation_" + outcome, ActorID: actor, Message: message, Audience: "participants", Evidence: x.Evidence, CreatedAt: now})
		return s.write(v)
	})
	return v, out, e
}

func validActionKind(v string) bool {
	return v == "pause_rollout" || v == "restore_release" || v == "emergency_repair"
}
func sameCriteria(a, b []HealthCriterion) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (s *Store) StartInvestigation(id, actor, agent, credential, mandate string, evidence []Evidence, revisions []Revision, context []EvidenceContext) (Incident, Investigation, error) {
	var v Incident
	var investigation Investigation
	e := s.mutate(func() error {
		if e := s.read(id, &v); e != nil {
			return e
		}
		mandate = strings.TrimSpace(mandate)
		if !validID(actor) || !validID(agent) || !validID(credential) || mandate == "" || len(mandate) > 10000 || len(evidence) > 20 || len(revisions) == 0 || len(revisions) > 20 || len(context) != len(evidence) {
			return ErrInvalid
		}
		for _, x := range evidence {
			if !validEvidence(x) {
				return ErrInvalid
			}
		}
		for _, x := range revisions {
			if !validID(x.RepositoryID) || len(x.CommitID) != 40 || strings.TrimSpace(x.Label) == "" {
				return ErrInvalid
			}
		}
		for i, x := range context {
			if len(x.Resource) == 0 || !json.Valid(x.Resource) || !sameEvidence([]Evidence{x.Selection}, []Evidence{evidence[i]}) {
				return ErrInvalid
			}
		}
		now := s.now()
		investigation = Investigation{ID: mustID(), AgentID: agent, InitiatorID: actor, CredentialID: credential, Mandate: mandate, State: "running", Evidence: evidence, Revisions: revisions, OperationalContext: context, Access: []string{"selected incident evidence", "selected repository revisions", "incident investigation timeline:write"}, CreatedAt: now, UpdatedAt: now}
		v.Investigations = append(v.Investigations, investigation)
		v.Version++
		v.UpdatedAt = now
		v.Timeline = append(v.Timeline, Entry{ID: mustID(), Kind: "investigation_delegated", ActorID: actor, Message: mandate, Audience: "participants", Evidence: evidence, InvestigationID: investigation.ID, CreatedAt: now})
		return s.write(v)
	})
	return v, investigation, e
}

func (s *Store) Investigation(id, investigationID string) (Incident, Investigation, error) {
	v, e := s.Get(id)
	if e != nil {
		return v, Investigation{}, e
	}
	for _, x := range v.Investigations {
		if x.ID == investigationID {
			return v, x, nil
		}
	}
	return v, Investigation{}, ErrNotFound
}

func (s *Store) AddInvestigationEvent(id, investigationID, credentialID, kind, message, tool string) (Incident, error) {
	var v Incident
	e := s.mutate(func() error {
		if e := s.read(id, &v); e != nil {
			return e
		}
		var x *Investigation
		for i := range v.Investigations {
			if v.Investigations[i].ID == investigationID {
				x = &v.Investigations[i]
			}
		}
		message = strings.TrimSpace(message)
		kind = strings.TrimSpace(kind)
		tool = strings.TrimSpace(tool)
		if x == nil {
			return ErrNotFound
		}
		if x.CredentialID != credentialID || x.State != "running" {
			return ErrConflict
		}
		if (kind != "finding" && kind != "tool_action" && kind != "question" && kind != "uncertainty") || message == "" || len(message) > 10000 || len(tool) > 200 {
			return ErrInvalid
		}
		if kind == "tool_action" && tool == "" {
			return ErrInvalid
		}
		now := s.now()
		x.UpdatedAt = now
		v.Version++
		v.UpdatedAt = now
		text := message
		if tool != "" {
			text = tool + ": " + message
		}
		v.Timeline = append(v.Timeline, Entry{ID: mustID(), Kind: "agent_" + kind, ActorID: x.AgentID, Message: text, Audience: "participants", InvestigationID: x.ID, CreatedAt: now})
		return s.write(v)
	})
	return v, e
}

func (s *Store) ControlInvestigation(id, investigationID, actor, action, message string) (Incident, Investigation, error) {
	var v Incident
	var out Investigation
	e := s.mutate(func() error {
		if e := s.read(id, &v); e != nil {
			return e
		}
		var x *Investigation
		for i := range v.Investigations {
			if v.Investigations[i].ID == investigationID {
				x = &v.Investigations[i]
			}
		}
		if x == nil {
			return ErrNotFound
		}
		if !validID(actor) {
			return ErrInvalid
		}
		switch action {
		case "guide":
			if strings.TrimSpace(message) == "" || (x.State != "running" && x.State != "paused") {
				return ErrConflict
			}
		case "pause":
			if x.State != "running" {
				return ErrConflict
			}
			x.State = "paused"
		case "resume":
			if x.State != "paused" {
				return ErrConflict
			}
			x.State = "running"
		case "cancel":
			if x.State == "cancelled" {
				out = *x
				return nil
			}
			x.State = "cancelled"
		default:
			return ErrInvalid
		}
		now := s.now()
		x.UpdatedAt = now
		v.Version++
		v.UpdatedAt = now
		v.Timeline = append(v.Timeline, Entry{ID: mustID(), Kind: "investigation_" + action, ActorID: actor, Message: strings.TrimSpace(message), Audience: "participants", InvestigationID: x.ID, CreatedAt: now})
		out = *x
		return s.write(v)
	})
	return v, out, e
}

type Store struct {
	root          string
	mu            sync.Mutex
	now           func() time.Time
	directorySync func(string) error
}

func New(root string) (*Store, error) {
	if root == "" {
		return nil, errors.New("incident root required")
	}
	abs, e := filepath.Abs(root)
	if e == nil {
		e = os.MkdirAll(abs, 0700)
	}
	if e != nil {
		return nil, e
	}
	return &Store{root: abs, now: func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }, directorySync: syncDirectory}, nil
}

func (s *Store) Create(v Incident) (Incident, error) {
	if !validIncident(v) {
		return Incident{}, ErrInvalid
	}
	id, e := newID()
	if e != nil {
		return Incident{}, e
	}
	now := s.now()
	v.ID = id
	v.Title = strings.TrimSpace(v.Title)
	v.Summary = strings.TrimSpace(v.Summary)
	v.Version = 1
	v.CreatedAt = now
	v.UpdatedAt = now
	v.Timeline = []Entry{{ID: mustID(), Kind: "declared", ActorID: v.DeclaredBy, Message: v.Summary, Audience: "participants", CreatedAt: now}}
	e = s.mutate(func() error { return s.write(v) })
	return v, e
}
func (s *Store) Get(id string) (Incident, error) {
	if !validID(id) {
		return Incident{}, ErrNotFound
	}
	var v Incident
	e := s.read(id, &v)
	return v, e
}
func (s *Store) List() ([]Incident, error) {
	es, e := os.ReadDir(s.root)
	if e != nil {
		return nil, e
	}
	out := []Incident{}
	for _, x := range es {
		if x.IsDir() || !strings.HasSuffix(x.Name(), ".json") {
			continue
		}
		var v Incident
		if e = s.read(strings.TrimSuffix(x.Name(), ".json"), &v); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

func (s *Store) Update(id, actor string, expected int, severity, status string, roles []Role, message string) (Incident, error) {
	var v Incident
	e := s.mutate(func() error {
		var x error
		if x = s.read(id, &v); x != nil {
			return x
		}
		if v.Version != expected {
			return ErrConflict
		}
		if !validID(actor) || !validSeverity(severity) || !validStatus(status) || !validRoles(roles) || len(message) > 10000 {
			return ErrInvalid
		}
		now := s.now()
		changes := []string{}
		if severity != v.Severity {
			changes = append(changes, "severity "+v.Severity+" → "+severity)
		}
		if status != v.Status {
			changes = append(changes, "status "+v.Status+" → "+status)
		}
		v.Severity = severity
		v.Status = status
		v.Roles = roles
		v.Version++
		v.UpdatedAt = now
		if status == "resolved" && v.ResolvedAt == nil {
			v.ResolvedAt = &now
		}
		if status != "resolved" {
			v.ResolvedAt = nil
		}
		text := strings.TrimSpace(message)
		if text == "" {
			text = strings.Join(changes, ", ")
		}
		v.Timeline = append(v.Timeline, Entry{ID: mustID(), Kind: "state_changed", ActorID: actor, Message: text, Audience: "participants", CreatedAt: now})
		return s.write(v)
	})
	return v, e
}
func (s *Store) AddUpdate(id, operationID, actor, message, audience string) (Incident, error) {
	var v Incident
	e := s.mutate(func() error {
		var x error
		if x = s.read(id, &v); x != nil {
			return x
		}
		message = strings.TrimSpace(message)
		if !validID(operationID) || !validID(actor) || message == "" || len(message) > 10000 || (audience != "participants" && audience != "public") {
			return ErrInvalid
		}
		for _, entry := range v.Timeline {
			if entry.ID == operationID {
				if entry.Kind != "update" || entry.ActorID != actor || entry.Message != message || entry.Audience != audience {
					return ErrConflict
				}
				return nil
			}
		}
		now := s.now()
		v.Version++
		v.UpdatedAt = now
		v.Timeline = append(v.Timeline, Entry{ID: operationID, Kind: "update", ActorID: actor, Message: message, Audience: audience, CreatedAt: now})
		return s.write(v)
	})
	return v, e
}
func (s *Store) AddFinding(id, operationID, actor, kind, message, audience string, evidence []Evidence) (Incident, error) {
	var v Incident
	e := s.mutate(func() error {
		var x error
		if x = s.read(id, &v); x != nil {
			return x
		}
		message = strings.TrimSpace(message)
		if !validID(operationID) || !validID(actor) || !validFindingKind(kind) || message == "" || len(message) > 10000 || (audience != "participants" && audience != "public") || len(evidence) == 0 || len(evidence) > 20 {
			return ErrInvalid
		}
		for _, source := range evidence {
			if !validEvidence(source) {
				return ErrInvalid
			}
		}
		for _, entry := range v.Timeline {
			if entry.ID == operationID {
				if entry.Kind != kind || entry.ActorID != actor || entry.Message != message || entry.Audience != audience || !sameEvidence(entry.Evidence, evidence) {
					return ErrConflict
				}
				return nil
			}
		}
		now := s.now()
		for i := range evidence {
			evidence[i].CapturedAt = now
		}
		v.Version++
		v.UpdatedAt = now
		v.Timeline = append(v.Timeline, Entry{ID: operationID, Kind: kind, ActorID: actor, Message: message, Audience: audience, CreatedAt: now, Evidence: evidence})
		return s.write(v)
	})
	return v, e
}

func validFindingKind(v string) bool {
	return v == "observation" || v == "hypothesis" || v == "query" || v == "conclusion"
}
func validEvidence(v Evidence) bool {
	validKind := v.Kind == "log" || v.Kind == "health_signal" || v.Kind == "deployment" || v.Kind == "release" || v.Kind == "commit" || v.Kind == "pull_request" || v.Kind == "incident"
	if !validKind || !validID(v.RepositoryID) || strings.TrimSpace(v.ResourceID) == "" || len(v.ResourceID) > 200 || strings.TrimSpace(v.Label) == "" || len(v.Label) > 300 || len(v.Query) > 2000 {
		return false
	}
	if (v.WindowStart == nil) != (v.WindowEnd == nil) || (v.WindowStart != nil && !v.WindowStart.Before(*v.WindowEnd)) {
		return false
	}
	return (v.Kind != "log" && v.Kind != "health_signal") || v.WindowStart != nil
}
func sameEvidence(a, b []Evidence) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		x, y := a[i], b[i]
		x.CapturedAt = time.Time{}
		y.CapturedAt = time.Time{}
		// Label is a server-derived historical snapshot, not part of the caller's
		// operation identity. A live resource may change between a committed
		// request and its lost-response retry.
		if x.Kind != y.Kind || x.RepositoryID != y.RepositoryID || x.ResourceID != y.ResourceID || x.Query != y.Query || !sameTime(x.WindowStart, y.WindowStart) || !sameTime(x.WindowEnd, y.WindowEnd) {
			return false
		}
	}
	return true
}
func sameTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}
func (s *Store) Acknowledge(id, entryID, actor string) (Incident, error) {
	var v Incident
	e := s.mutate(func() error {
		var x error
		if x = s.read(id, &v); x != nil {
			return x
		}
		if !validID(actor) || !validID(entryID) {
			return ErrInvalid
		}
		found := false
		for i := range v.Timeline {
			if v.Timeline[i].ID == entryID {
				found = true
				for _, a := range v.Timeline[i].AcknowledgedBy {
					if a == actor {
						return nil
					}
				}
				v.Timeline[i].AcknowledgedBy = append(v.Timeline[i].AcknowledgedBy, actor)
			}
		}
		if !found {
			return ErrNotFound
		}
		v.Version++
		v.UpdatedAt = s.now()
		return s.write(v)
	})
	return v, e
}

func validIncident(v Incident) bool {
	return strings.TrimSpace(v.Title) != "" && len(v.Title) <= 200 && strings.TrimSpace(v.Summary) != "" && len(v.Summary) <= 10000 && validID(v.DeclaredBy) && validSeverity(v.Severity) && validStatus(v.Status) && len(v.Scopes) > 0 && len(v.Scopes) <= 25 && validRoles(v.Roles) && validScopes(v.Scopes)
}
func validInvestigation(v Investigation) bool {
	if !validID(v.ID) || !validID(v.AgentID) || !validID(v.InitiatorID) || !validID(v.CredentialID) || strings.TrimSpace(v.Mandate) == "" || (v.State != "running" && v.State != "paused" && v.State != "cancelled") || len(v.Revisions) == 0 || v.CreatedAt.IsZero() || v.UpdatedAt.IsZero() {
		return false
	}
	for _, x := range v.Evidence {
		if !validEvidence(x) {
			return false
		}
	}
	for _, x := range v.Revisions {
		if !validID(x.RepositoryID) || len(x.CommitID) != 40 || strings.TrimSpace(x.Label) == "" {
			return false
		}
	}
	if len(v.OperationalContext) != len(v.Evidence) {
		return false
	}
	for i, x := range v.OperationalContext {
		if len(x.Resource) == 0 || !json.Valid(x.Resource) || !sameEvidence([]Evidence{x.Selection}, []Evidence{v.Evidence[i]}) {
			return false
		}
	}
	return true
}
func validAction(v Action) bool {
	if !validID(v.ID) || !validID(v.OperationID) || !validActionKind(v.Kind) || !validID(v.RepositoryID) || !validID(v.DeploymentID) || !validID(v.ProposedBy) || strings.TrimSpace(v.Rationale) == "" || (v.Status != "proposed" && v.Status != "approved" && v.Status != "rejected" && v.Status != "executing" && v.Status != "failed" && v.Status != "recovered") || len(v.Evidence) == 0 || len(v.HealthCriteria) == 0 || v.CreatedAt.IsZero() || v.UpdatedAt.IsZero() {
		return false
	}
	for _, x := range v.Evidence {
		if !validEvidence(x) {
			return false
		}
	}
	for _, x := range v.HealthCriteria {
		if strings.TrimSpace(x.Stage) == "" || strings.TrimSpace(x.Signal) == "" || len(x.Stage) > 100 || len(x.Signal) > 100 {
			return false
		}
	}
	for _, x := range v.Decisions {
		if !validID(x.ActorID) || (x.Decision != "approve" && x.Decision != "reject") || x.CreatedAt.IsZero() {
			return false
		}
	}
	for _, x := range v.Attempts {
		if !validID(x.ID) || !validID(x.ActorID) || (x.Outcome != "pending" && x.Outcome != "started" && x.Outcome != "failed" && x.Outcome != "recovered") || strings.TrimSpace(x.Message) == "" || x.CreatedAt.IsZero() {
			return false
		}
	}
	return true
}
func validScopes(v []Scope) bool {
	seen := map[string]bool{}
	for _, s := range v {
		if !validID(s.RepositoryID) || seen[s.RepositoryID] || len(s.EnvironmentIDs) > 25 {
			return false
		}
		seen[s.RepositoryID] = true
		for _, id := range s.EnvironmentIDs {
			if !validID(id) {
				return false
			}
		}
	}
	return true
}
func validRoles(v []Role) bool {
	if len(v) > 20 {
		return false
	}
	seen := map[string]bool{}
	for _, r := range v {
		r.Name = strings.TrimSpace(r.Name)
		if r.Name == "" || len(r.Name) > 50 || !validID(r.UserID) || seen[strings.ToLower(r.Name)] {
			return false
		}
		seen[strings.ToLower(r.Name)] = true
	}
	return true
}
func validSeverity(v string) bool { return v == "sev1" || v == "sev2" || v == "sev3" || v == "sev4" }
func validStatus(v string) bool {
	return v == "investigating" || v == "identified" || v == "monitoring" || v == "resolved"
}
func validID(v string) bool {
	if len(v) != 32 {
		return false
	}
	_, e := hex.DecodeString(v)
	return e == nil
}
func newID() (string, error) {
	b := make([]byte, 16)
	_, e := rand.Read(b)
	return hex.EncodeToString(b), e
}
func mustID() string {
	id, e := newID()
	if e != nil {
		panic(e)
	}
	return id
}
func (s *Store) read(id string, v *Incident) error {
	b, e := os.ReadFile(filepath.Join(s.root, id+".json"))
	if errors.Is(e, os.ErrNotExist) {
		return ErrNotFound
	}
	if e != nil {
		return e
	}
	if json.Unmarshal(b, v) != nil || v.ID != id {
		return errors.New("corrupt incident")
	}
	for _, investigation := range v.Investigations {
		if !validInvestigation(investigation) {
			return errors.New("corrupt incident investigation")
		}
	}
	for _, action := range v.Actions {
		if !validAction(action) {
			return errors.New("corrupt incident action")
		}
	}
	if v.Review != nil && (!validID(v.Review.PublishedBy) || strings.TrimSpace(v.Review.Impact) == "" || strings.TrimSpace(v.Review.Timeline) == "" || len(v.Review.ContributingFactors) == 0 || strings.TrimSpace(v.Review.Conclusions) == "" || v.Review.PublishedAt.IsZero()) {
		return errors.New("corrupt incident review")
	}
	for _, commitment := range v.Commitments {
		if !validID(commitment.ID) || !validID(commitment.OperationID) || !validID(commitment.RepositoryID) || !validID(commitment.ProposalID) || !validID(commitment.TaskID) || !validID(commitment.AssigneeID) || !validID(commitment.CreatedBy) || commitment.DueAt.IsZero() || commitment.CreatedAt.IsZero() {
			return errors.New("corrupt incident commitment")
		}
	}
	return nil
}
func (s *Store) write(v Incident) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(s.root, ".incident-*")
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
	ce := tmp.Close()
	if e == nil {
		e = ce
	}
	if e == nil {
		e = os.Rename(name, filepath.Join(s.root, v.ID+".json"))
	}
	if e == nil {
		e = s.directorySync(s.root)
	}
	return e
}

func syncDirectory(path string) error {
	d, err := os.Open(path)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
func (s *Store) mutate(fn func() error) error {
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
