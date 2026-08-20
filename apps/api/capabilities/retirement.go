package capabilities

import (
	"errors"
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
	Kind     string `json:"kind"`
	Message  string `json:"message"`
	OwnerID  string `json:"owner_id,omitempty"`
	Audience string `json:"audience,omitempty"`
}
type RetirementPlan struct {
	ID                string               `json:"id"`
	CapabilityVersion int                  `json:"capability_version"`
	Rationale         string               `json:"rationale"`
	Replacements      []Replacement        `json:"replacements"`
	Audiences         []Audience           `json:"audiences"`
	Stages            []CompatibilityStage `json:"stages"`
	Deadline          time.Time            `json:"deadline"`
	ApprovalDueAt     time.Time            `json:"approval_due_at"`
	SuccessCriteria   []string             `json:"success_criteria"`
	RollbackCriteria  []string             `json:"rollback_criteria"`
	Communication     CommunicationPolicy  `json:"communication"`
	RequiredOwnerIDs  []string             `json:"required_owner_ids"`
	Exceptions        []PlanException      `json:"exceptions,omitempty"`
	FrozenDiagnostics []Diagnostic         `json:"frozen_diagnostics,omitempty"`
	Events            []RetirementEvent    `json:"events"`
	Blockers          []RetirementBlocker  `json:"blockers"`
	Status            string               `json:"status"`
	CreatedBy         string               `json:"created_by"`
	CreatedAt         time.Time            `json:"created_at"`
	UpdatedAt         time.Time            `json:"updated_at"`
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
	add := func(k, m, o, a string) {
		b = append(b, RetirementBlocker{Kind: k, Message: m, OwnerID: o, Audience: a})
	}
	if p.CapabilityVersion != current {
		add("changed_usage", "The capability inventory changed after this plan was opened; reassess every affected audience.", "", "")
	}
	for _, d := range diagnostics {
		if d.Severity == "blocking" {
			add("inventory_"+d.Kind, d.Message, "", d.Consumer)
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
			add("policy_deferred", "An attributable bounded decision defers removal: "+e.Summary, e.ActorID, "")
		}
	}
	for _, id := range p.RequiredOwnerIDs {
		if !approved[id] {
			kind := "owner_approval_required"
			if now.After(p.ApprovalDueAt) {
				kind = "unresponsive_owner"
			}
			add(kind, "Required owner has not acknowledged the migration contract.", id, "")
		}
	}
	for _, a := range p.Audiences {
		if a.Commitment != "" {
			add("conflicting_commitment", "An affected audience has a compatibility commitment that must be reconciled.", "", a.Name)
		}
		if a.EmbargoedDependency {
			add("embargoed_dependency", "A restricted dependency remains represented without disclosing it.", "", a.Name)
		}
	}
	for _, x := range p.Exceptions {
		if x.ExpiresAt.After(now) {
			add("active_exception", "A bounded exception delays retirement: "+x.Rationale, x.GrantedBy, x.Audience)
		}
	}
	p.Blockers = b
	if deferred {
		p.Status = "deferred"
	} else if len(b) == 0 {
		p.Status = "acknowledged"
	} else {
		p.Status = "blocked"
	}
}
