package capabilities

import (
	"encoding/base64"
	"errors"
	"os"
	"strings"
	"time"
)

var ErrPlanNotFound = errors.New("retirement plan not found")

type Replacement struct {
	Name           string `json:"name"`
	Reference      string `json:"reference"`
	MigrationGuide string `json:"migration_guide"`
	Supported      bool   `json:"supported"`
}
type Audience struct {
	Name                string   `json:"name"`
	OwnerIDs            []string `json:"owner_ids"`
	Impact              string   `json:"impact"`
	Commitment          string   `json:"commitment,omitempty"`
	EmbargoedDependency bool     `json:"embargoed_dependency"`
}
type CompatibilityStage struct {
	Name         string    `json:"name"`
	StartsAt     time.Time `json:"starts_at"`
	Behavior     string    `json:"behavior"`
	ExitCriteria []string  `json:"exit_criteria"`
}
type CommunicationPolicy struct {
	Channels   []string `json:"channels"`
	NoticeDays int      `json:"notice_days"`
	Updates    string   `json:"updates"`
	Escalation string   `json:"escalation"`
}
type PlanException struct {
	Audience  string    `json:"audience"`
	Rationale string    `json:"rationale"`
	Decision  string    `json:"decision"`
	GrantedBy string    `json:"granted_by"`
	ExpiresAt time.Time `json:"expires_at"`
	FollowUp  string    `json:"follow_up"`
}
type RetirementEvent struct {
	Version   int        `json:"version"`
	Type      string     `json:"type"`
	ActorID   string     `json:"actor_id"`
	ActorType string     `json:"actor_type"`
	Summary   string     `json:"summary"`
	Evidence  []string   `json:"evidence,omitempty"`
	OwnerID   string     `json:"owner_id,omitempty"`
	Decision  string     `json:"decision,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	FollowUp  string     `json:"follow_up,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}
type RetirementBlocker struct {
	Kind          string `json:"kind"`
	Message       string `json:"message"`
	OwnerID       string `json:"owner_id,omitempty"`
	Audience      string `json:"audience,omitempty"`
	ConsumerIndex *int   `json:"consumer_index,omitempty"`
}

// RetirementWork connects the shared retirement reason to ordinary work owned
// by the repository that must change. The target proposal remains authoritative
// for assignment, sessions, workspaces, forks, review, and merge.
type RetirementWork struct {
	ID                   string    `json:"id"`
	AudienceIndex        int       `json:"audience_index"`
	RepositoryID         string    `json:"repository_id"`
	ProposalID           string    `json:"proposal_id"`
	TaskID               string    `json:"task_id"`
	DependencyIDs        []string  `json:"dependency_ids"`
	OldContract          string    `json:"old_contract"`
	ReplacementContract  string    `json:"replacement_contract"`
	AcceptanceCriteria   []string  `json:"acceptance_criteria"`
	DocumentationChanges []string  `json:"documentation_changes"`
	RolloutStage         string    `json:"rollout_stage"`
	CreatedBy            string    `json:"created_by"`
	CreatedAt            time.Time `json:"created_at"`
	Status               string    `json:"status,omitempty"`
	Ready                bool      `json:"ready"`
	AssignmentID         string    `json:"assignment_id,omitempty"`
	AssigneeType         string    `json:"assignee_type,omitempty"`
	AssigneeID           string    `json:"assignee_id,omitempty"`
	BaseRevision         string    `json:"base_revision,omitempty"`
	SessionID            string    `json:"session_id,omitempty"`
	WorkspaceID          string    `json:"workspace_id,omitempty"`
	ForkRepositoryID     string    `json:"fork_repository_id,omitempty"`
	PullRequestID        string    `json:"pull_request_id,omitempty"`
	ContributionStatus   string    `json:"contribution_status,omitempty"`
}

// ConsumerDiscovery reports newly found use without adding that repository to
// the retiring provider's authority or rewriting the frozen inventory.
type ConsumerDiscovery struct {
	ID           string    `json:"id"`
	RepositoryID string    `json:"repository_id"`
	Revision     string    `json:"revision"`
	Paths        []string  `json:"paths"`
	Evidence     []string  `json:"evidence"`
	Impact       string    `json:"impact"`
	ReportedBy   string    `json:"reported_by"`
	CreatedAt    time.Time `json:"created_at"`
}
type RetirementPlan struct {
	ID                  string               `json:"id"`
	CapabilityVersion   int                  `json:"capability_version"`
	Rationale           string               `json:"rationale"`
	Replacements        []Replacement        `json:"replacements"`
	Audiences           []Audience           `json:"audiences"`
	Stages              []CompatibilityStage `json:"stages"`
	Deadline            time.Time            `json:"deadline"`
	ApprovalDueAt       time.Time            `json:"approval_due_at"`
	SuccessCriteria     []string             `json:"success_criteria"`
	RollbackCriteria    []string             `json:"rollback_criteria"`
	Communication       CommunicationPolicy  `json:"communication"`
	RequiredOwnerIDs    []string             `json:"required_owner_ids"`
	Exceptions          []PlanException      `json:"exceptions,omitempty"`
	FrozenDiagnostics   []Diagnostic         `json:"frozen_diagnostics,omitempty"`
	Events              []RetirementEvent    `json:"events"`
	WorkVersion         int                  `json:"work_version"`
	Work                []RetirementWork     `json:"work,omitempty"`
	DiscoveredConsumers []ConsumerDiscovery  `json:"discovered_consumers,omitempty"`
	Candidates          []MigrationCandidate `json:"candidates,omitempty"`
	Executions          []RemovalExecution   `json:"executions,omitempty"`
	Blockers            []RetirementBlocker  `json:"blockers"`
	Status              string               `json:"status"`
	CreatedBy           string               `json:"created_by"`
	CreatedAt           time.Time            `json:"created_at"`
	UpdatedAt           time.Time            `json:"updated_at"`
}

func validWorkText(v string) bool { return v != "" && len([]rune(v)) <= 4000 }

// CreateRetirementWork holds the plan CAS boundary while target-owned work is
// published, so stale ordering cannot leave an unlinked proposal behind.
func (s *Store) CreateRetirementWork(repo, capabilityID, planID, actor string, expected int, work RetirementWork, publish func() (string, string, error)) (Capability, RetirementWork, error) {
	var out Capability
	var link RetirementWork
	err := s.lock(func() error {
		v, err := s.read(repo, capabilityID)
		if err != nil {
			return err
		}
		var p *RetirementPlan
		for i := range v.RetirementPlans {
			if v.RetirementPlans[i].ID == planID {
				p = &v.RetirementPlans[i]
			}
		}
		if p == nil {
			return ErrPlanNotFound
		}
		if p.WorkVersion != expected {
			return ErrConflict
		}
		providerWork := work.RepositoryID == repo && work.AudienceIndex == -1
		consumerWork := work.AudienceIndex >= 0 && work.AudienceIndex < len(p.Audiences)
		if actor == "" || publish == nil || (!providerWork && !consumerWork) || !validWorkText(work.OldContract) || !validWorkText(work.ReplacementContract) || len(work.AcceptanceCriteria) == 0 || len(work.AcceptanceCriteria) > 50 || len(work.DocumentationChanges) == 0 || len(work.DocumentationChanges) > 50 || work.RolloutStage == "" || len(work.DependencyIDs) > 50 {
			return ErrInvalid
		}
		if p.CapabilityVersion < 1 || p.CapabilityVersion > len(v.Revisions) {
			return ErrInvalid
		}
		if consumerWork {
			consumer := v.Revisions[p.CapabilityVersion-1].Consumers[work.AudienceIndex]
			if consumer.RepositoryID == "" || consumer.RepositoryID != work.RepositoryID {
				return ErrInvalid
			}
		}
		known, seen := map[string]bool{}, map[string]bool{}
		for _, x := range p.Work {
			known[x.ID] = true
		}
		for _, id := range work.DependencyIDs {
			if !known[id] || seen[id] {
				return ErrInvalid
			}
			seen[id] = true
		}
		for _, x := range append(append([]string{}, work.AcceptanceCriteria...), work.DocumentationChanges...) {
			if !validWorkText(x) {
				return ErrInvalid
			}
		}
		proposalID, taskID, err := publish()
		if err != nil {
			return err
		}
		if proposalID == "" || taskID == "" {
			return ErrInvalid
		}
		now := s.now()
		work.ID, work.ProposalID, work.TaskID, work.CreatedBy, work.CreatedAt = randomID(), proposalID, taskID, actor, now
		p.Work = append(p.Work, work)
		p.WorkVersion++
		p.UpdatedAt = now
		v.UpdatedAt, out, link = now, v, work
		return s.write(v)
	})
	return s.project(out), link, err
}

func (s *Store) ReportRetirementConsumer(repo, capabilityID, planID, actor string, expected int, report ConsumerDiscovery) (Capability, error) {
	var out Capability
	err := s.lock(func() error {
		v, err := s.read(repo, capabilityID)
		if err != nil {
			return err
		}
		var p *RetirementPlan
		for i := range v.RetirementPlans {
			if v.RetirementPlans[i].ID == planID {
				p = &v.RetirementPlans[i]
			}
		}
		if p == nil {
			return ErrPlanNotFound
		}
		if p.WorkVersion != expected {
			return ErrConflict
		}
		if actor == "" || report.RepositoryID == "" || len(report.Revision) != 40 || len(report.Paths) == 0 || len(report.Paths) > 50 || len(report.Evidence) == 0 || len(report.Evidence) > 50 || !validWorkText(report.Impact) {
			return ErrInvalid
		}
		for _, x := range append(append([]string{}, report.Paths...), report.Evidence...) {
			if !validWorkText(x) {
				return ErrInvalid
			}
		}
		now := s.now()
		report.ID, report.ReportedBy, report.CreatedAt = randomID(), actor, now
		p.DiscoveredConsumers = append(p.DiscoveredConsumers, report)
		p.WorkVersion++
		p.UpdatedAt = now
		v.UpdatedAt, out = now, v
		return s.write(v)
	})
	return s.project(out), err
}

// FindRetirementWork lets the ordinary task launcher enforce the plan's
// dependency order without giving the provider any task mutation capability.
func (s *Store) FindRetirementWork(repositoryID, taskID string) (Capability, RetirementPlan, RetirementWork, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return Capability{}, RetirementPlan{}, RetirementWork{}, err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "repo-") {
			continue
		}
		repoBytes, decodeErr := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(entry.Name(), "repo-"))
		if decodeErr != nil {
			continue
		}
		values, listErr := s.List(string(repoBytes))
		if listErr != nil {
			return Capability{}, RetirementPlan{}, RetirementWork{}, listErr
		}
		for _, capability := range values {
			for _, plan := range capability.RetirementPlans {
				for _, work := range plan.Work {
					if work.RepositoryID == repositoryID && work.TaskID == taskID {
						return capability, plan, work, nil
					}
				}
			}
		}
	}
	return Capability{}, RetirementPlan{}, RetirementWork{}, ErrPlanNotFound
}

func validatePlan(p RetirementPlan, r Revision, now time.Time) error {
	if p.Rationale == "" || len(p.Replacements) == 0 || len(p.Audiences) == 0 || len(p.Stages) == 0 || len(p.SuccessCriteria) == 0 || len(p.RollbackCriteria) == 0 || len(p.RequiredOwnerIDs) == 0 || p.Deadline.Before(now) || p.ApprovalDueAt.Before(now) || p.ApprovalDueAt.After(p.Deadline) || len(p.Communication.Channels) == 0 || p.Communication.NoticeDays < 1 || p.Communication.Updates == "" || p.Communication.Escalation == "" {
		return ErrInvalid
	}
	for _, x := range p.Replacements {
		if x.Name == "" || x.Reference == "" || x.MigrationGuide == "" || !x.Supported {
			return ErrInvalid
		}
	}
	for _, x := range p.Audiences {
		if x.Name == "" || len(x.OwnerIDs) == 0 || x.Impact == "" {
			return ErrInvalid
		}
	}
	if len(p.Audiences) != len(r.Consumers) {
		return ErrInvalid
	}
	for i, audience := range p.Audiences {
		consumer := r.Consumers[i]
		if audience.Name != consumer.Name || len(audience.OwnerIDs) != len(consumer.OwnerIDs) {
			return ErrInvalid
		}
		for _, owner := range consumer.OwnerIDs {
			if !contains(audience.OwnerIDs, owner) {
				return ErrInvalid
			}
		}
	}
	for i, x := range p.Stages {
		if x.Name == "" || x.Behavior == "" || len(x.ExitCriteria) == 0 || (i > 0 && !x.StartsAt.After(p.Stages[i-1].StartsAt)) {
			return ErrInvalid
		}
	}
	owners := map[string]bool{}
	for _, id := range p.RequiredOwnerIDs {
		if id == "" || owners[id] {
			return ErrInvalid
		}
		owners[id] = true
	}
	for _, c := range r.Consumers {
		for _, id := range c.OwnerIDs {
			if !owners[id] {
				return ErrInvalid
			}
		}
	}
	for _, x := range p.Exceptions {
		if x.Audience == "" || x.Rationale == "" || x.Decision != "defer" || x.GrantedBy == "" || x.FollowUp == "" || !x.ExpiresAt.After(now) || x.ExpiresAt.After(now.Add(30*24*time.Hour)) {
			return ErrInvalid
		}
	}
	return nil
}

func (s *Store) OpenRetirement(repo, capabilityID, actor string, p RetirementPlan) (Capability, error) {
	var out Capability
	err := s.lock(func() error {
		v, err := s.read(repo, capabilityID)
		if err != nil {
			return err
		}
		if actor == "" || len(v.Revisions) == 0 {
			return ErrInvalid
		}
		now := s.now()
		current := v.Revisions[len(v.Revisions)-1]
		for i := range p.Exceptions {
			p.Exceptions[i].GrantedBy = actor
		}
		if validatePlan(p, current, now) != nil {
			return ErrInvalid
		}
		p.ID = randomID()
		p.CapabilityVersion = v.CurrentVersion
		p.FrozenDiagnostics = append([]Diagnostic(nil), s.project(v).Diagnostics...)
		p.Status = "proposed"
		p.CreatedBy = actor
		p.CreatedAt = now
		p.UpdatedAt = now
		v.RetirementPlans = append(v.RetirementPlans, p)
		v.UpdatedAt = now
		out = v
		return s.write(v)
	})
	return s.project(out), err
}

func (s *Store) AppendRetirementEvent(repo, capabilityID, planID, actor, actorType string, expected int, event RetirementEvent) (Capability, error) {
	var out Capability
	err := s.lock(func() error {
		v, e := s.read(repo, capabilityID)
		if e != nil {
			return e
		}
		var p *RetirementPlan
		for i := range v.RetirementPlans {
			if v.RetirementPlans[i].ID == planID {
				p = &v.RetirementPlans[i]
			}
		}
		if p == nil {
			return ErrPlanNotFound
		}
		if len(p.Events) != expected {
			return ErrConflict
		}
		if actor == "" || (actorType != "human" && actorType != "read_only_agent") || event.Summary == "" || len(event.Summary) > 4000 || len(event.Evidence) > 50 {
			return ErrInvalid
		}
		allowed := map[string]bool{"assessment": true, "challenge": true, "approval": true, "policy_decision": true}
		if !allowed[event.Type] || ((event.Type == "assessment" || event.Type == "challenge") && len(event.Evidence) == 0) {
			return ErrInvalid
		}
		for _, citation := range event.Evidence {
			if citation == "" || len(citation) > 500 {
				return ErrInvalid
			}
		}
		if actorType == "read_only_agent" && (event.Type == "approval" || event.Type == "policy_decision") {
			return ErrInvalid
		}
		if event.Type == "approval" {
			if event.OwnerID != actor || event.Decision != "approved" || !contains(p.RequiredOwnerIDs, actor) {
				return ErrInvalid
			}
		}
		if event.Type == "policy_decision" && (event.Decision != "defer" || event.ExpiresAt == nil || !event.ExpiresAt.After(s.now()) || event.ExpiresAt.After(s.now().Add(30*24*time.Hour)) || event.FollowUp == "") {
			return ErrInvalid
		}
		now := s.now()
		event.Version = len(p.Events) + 1
		event.ActorID = actor
		event.ActorType = actorType
		event.CreatedAt = now
		p.Events = append(p.Events, event)
		p.UpdatedAt = now
		v.UpdatedAt = now
		out = v
		return s.write(v)
	})
	return s.project(out), err
}

func contains(v []string, s string) bool {
	for _, x := range v {
		if x == s {
			return true
		}
	}
	return false
}
func projectRetirement(p *RetirementPlan, current int, diagnostics []Diagnostic, now time.Time) {
	b := []RetirementBlocker{}
	add := func(k, m, o, a string, consumerIndex *int) {
		b = append(b, RetirementBlocker{Kind: k, Message: m, OwnerID: o, Audience: a, ConsumerIndex: consumerIndex})
	}
	readyCandidate := false
	for i := range p.Candidates {
		ProjectCandidate(&p.Candidates[i], *p)
		readyCandidate = readyCandidate || p.Candidates[i].RemovalReady
	}
	if !readyCandidate {
		add("migration_evidence_required", "No immutable candidate currently proves coexistence, rollback, journeys, and measured zero residual use.", "", "", nil)
	}
	if p.CapabilityVersion != current {
		add("changed_usage", "The capability inventory changed after this plan was opened; reassess every affected audience.", "", "", nil)
	}
	for _, d := range diagnostics {
		if d.Severity == "blocking" {
			add("inventory_"+d.Kind, d.Message, "", d.Consumer, d.ConsumerIndex)
		}
	}
	approved := map[string]bool{}
	deferred := false
	for _, e := range p.Events {
		if e.Type == "approval" && e.Decision == "approved" {
			approved[e.OwnerID] = true
		}
		if e.Type == "policy_decision" && e.Decision == "defer" && e.ExpiresAt != nil && e.ExpiresAt.After(now) {
			deferred = true
			add("policy_deferred", "An attributable bounded decision defers removal: "+e.Summary, e.ActorID, "", nil)
		}
	}
	for _, id := range p.RequiredOwnerIDs {
		if !approved[id] {
			kind := "owner_approval_required"
			if now.After(p.ApprovalDueAt) {
				kind = "unresponsive_owner"
			}
			add(kind, "Required owner has not acknowledged the migration contract.", id, "", nil)
		}
	}
	for _, a := range p.Audiences {
		if a.Commitment != "" {
			add("conflicting_commitment", "An affected audience has a compatibility commitment that must be reconciled.", "", a.Name, nil)
		}
		if a.EmbargoedDependency {
			add("embargoed_dependency", "A restricted dependency remains represented without disclosing it.", "", a.Name, nil)
		}
	}
	for _, x := range p.Exceptions {
		if x.ExpiresAt.After(now) {
			add("active_exception", "A bounded exception delays retirement: "+x.Rationale, x.GrantedBy, x.Audience, nil)
		}
	}
	for range p.DiscoveredConsumers {
		add("new_consumer_discovered", "A consumer reported exact usage after the inventory was frozen; reassess its impact without assuming repository authority.", "", "", nil)
	}
	p.Blockers = b
	if deferred {
		p.Status = "deferred"
	} else if len(b) == 0 {
		p.Status = "acknowledged"
	} else {
		p.Status = "blocked"
	}
	if len(p.Executions) > 0 {
		execution := p.Executions[len(p.Executions)-1]
		if execution.Status == "running" || execution.Status == "paused" || execution.Status == "restored" || execution.Status == "awaiting_verification" || execution.Status == "completed" {
			p.Status = execution.Status
		}
	}
}
