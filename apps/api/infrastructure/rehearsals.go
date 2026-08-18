package infrastructure

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"time"
)

var rehearsalKinds = map[string]bool{"provisioning": true, "connectivity": true, "access": true, "policy": true, "service_journey": true, "failure": true, "cost": true, "teardown": true, "recovery": true}

type RehearsalCheck struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"`
	Command     string   `json:"command"`
	ResourceIDs []string `json:"resource_ids"`
	Expectation string   `json:"expectation"`
}

type RehearsalScope struct {
	EnvironmentKind       string    `json:"environment_kind"`
	EnvironmentID         string    `json:"environment_id"`
	PolicyApproval        string    `json:"policy_approval,omitempty"`
	CredentialResourceIDs []string  `json:"credential_resource_ids"`
	CredentialExpiresAt   time.Time `json:"credential_expires_at"`
	StateKind             string    `json:"state_kind"`
	StateDescription      string    `json:"state_description"`
}

type UnsupportedEffect struct {
	ResourceID string `json:"resource_id"`
	Effect     string `json:"effect"`
	Reason     string `json:"reason"`
}

type RehearsalOutcome struct {
	CheckID       string    `json:"check_id"`
	Kind          string    `json:"kind"`
	Status        string    `json:"status"`
	ExitCode      int       `json:"exit_code"`
	SanitizedLog  string    `json:"sanitized_log"`
	DurationMS    int64     `json:"duration_ms"`
	StartedAt     time.Time `json:"started_at"`
	CompletedAt   time.Time `json:"completed_at"`
	CommandDigest string    `json:"command_digest"`
	ActorID       string    `json:"actor_id"`
}

type RehearsalArtifact struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
	Size   int    `json:"size"`
}

type RehearsalRun struct {
	ID            string              `json:"id"`
	WorkspaceID   string              `json:"workspace_id"`
	Result        string              `json:"result"`
	Outcomes      []RehearsalOutcome  `json:"outcomes"`
	Artifacts     []RehearsalArtifact `json:"artifacts"`
	ResourceGraph []Resource          `json:"resource_graph"`
	Attestations  []string            `json:"attestations"`
	AgentActions  []string            `json:"agent_actions"`
	CreatedBy     string              `json:"created_by"`
	CreatedAt     time.Time           `json:"created_at"`
}

type Rehearsal struct {
	ID                 string              `json:"id"`
	Name               string              `json:"name"`
	PlanID             string              `json:"plan_id"`
	PlanSourceRevision string              `json:"plan_source_revision"`
	Scope              RehearsalScope      `json:"scope"`
	Checks             []RehearsalCheck    `json:"checks"`
	UnsupportedEffects []UnsupportedEffect `json:"unsupported_effects"`
	Runs               []RehearsalRun      `json:"runs"`
	CreatedBy          string              `json:"created_by"`
	CreatedAt          time.Time           `json:"created_at"`
}

func validateRehearsal(p ChangePlan, r Rehearsal, now time.Time) error {
	if strings.TrimSpace(r.Name) == "" || r.Scope.EnvironmentID == "" || (r.Scope.EnvironmentKind != "isolated" && r.Scope.EnvironmentKind != "policy_approved_ephemeral") || (r.Scope.EnvironmentKind == "policy_approved_ephemeral" && strings.TrimSpace(r.Scope.PolicyApproval) == "") || (r.Scope.StateKind != "synthetic" && r.Scope.StateKind != "permitted") || r.Scope.StateDescription == "" || len(r.Scope.CredentialResourceIDs) == 0 || !r.Scope.CredentialExpiresAt.After(now) || len(r.Checks) == 0 || unsafe(r.Name, r.Scope.EnvironmentID, r.Scope.PolicyApproval, r.Scope.StateDescription) {
		return ErrInvalid
	}
	changed := map[string]bool{}
	destructive := map[string]bool{}
	for _, c := range p.Changes {
		changed[c.ResourceID] = true
		destructive[c.ResourceID] = c.Action == "destroy" || c.Action == "replace"
	}
	for _, id := range r.Scope.CredentialResourceIDs {
		if !changed[id] {
			return ErrInvalid
		}
	}
	ids, covered, unsupported := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, c := range r.Checks {
		if c.ID == "" || ids[c.ID] || !rehearsalKinds[c.Kind] || strings.TrimSpace(c.Command) == "" || strings.TrimSpace(c.Expectation) == "" || len(c.ResourceIDs) == 0 || unsafe(c.ID, c.Command, c.Expectation) {
			return ErrInvalid
		}
		ids[c.ID], covered[c.Kind] = true, true
		for _, id := range c.ResourceIDs {
			if !changed[id] {
				return ErrInvalid
			}
		}
	}
	for _, u := range r.UnsupportedEffects {
		if !changed[u.ResourceID] || strings.TrimSpace(u.Effect) == "" || strings.TrimSpace(u.Reason) == "" || unsafe(u.ResourceID, u.Effect, u.Reason) {
			return ErrInvalid
		}
		unsupported[u.ResourceID] = true
	}
	for _, kind := range []string{"provisioning", "connectivity", "access", "policy", "service_journey", "failure", "cost", "teardown", "recovery"} {
		if !covered[kind] {
			return ErrInvalid
		}
	}
	for id, value := range destructive {
		if value && !unsupported[id] {
			return ErrInvalid
		}
	}
	return nil
}

func (s *Store) CreateRehearsal(planID, actor string, r Rehearsal) (ChangePlan, Rehearsal, error) {
	var plan ChangePlan
	err := s.lock(func() error {
		p, err := s.readPlan(planID)
		if err != nil {
			return err
		}
		if validateRehearsal(p, r, s.now()) != nil {
			return ErrInvalid
		}
		now := s.now()
		r.ID, r.PlanID, r.PlanSourceRevision, r.CreatedBy, r.CreatedAt = randomID(), p.ID, p.SourceRevision, actor, now
		r.Runs = []RehearsalRun{}
		p.Rehearsals = append(p.Rehearsals, r)
		plan = p
		return s.writePlan(p)
	})
	return plan, r, err
}

func (s *Store) CreateRehearsalCurrent(planID, actor string, r Rehearsal) (ChangePlan, Rehearsal, error) {
	var plan ChangePlan
	var rehearsal Rehearsal
	err := s.lock(func() error {
		p, err := s.readPlan(planID)
		if err != nil {
			return err
		}
		current, err := s.read(p.DefinitionID)
		if err != nil || current.CurrentVersion != p.DefinitionVersion || observationFingerprint(current) != p.ObservationFingerprint || (p.ObservationsValidUntil != nil && s.now().After(*p.ObservationsValidUntil)) {
			return ErrPlanStale
		}
		if validateRehearsal(p, r, s.now()) != nil {
			return ErrInvalid
		}
		now := s.now()
		r.ID, r.PlanID, r.PlanSourceRevision, r.CreatedBy, r.CreatedAt, r.Runs = randomID(), p.ID, p.SourceRevision, actor, now, []RehearsalRun{}
		p.Rehearsals = append(p.Rehearsals, r)
		plan, rehearsal = p, r
		return s.writePlan(p)
	})
	return plan, rehearsal, err
}

func (s *Store) AddRehearsalRun(planID, rehearsalID, actor string, run RehearsalRun) (ChangePlan, RehearsalRun, error) {
	var plan ChangePlan
	err := s.lock(func() error {
		p, err := s.readPlan(planID)
		if err != nil {
			return err
		}
		for i := range p.Rehearsals {
			if p.Rehearsals[i].ID != rehearsalID {
				continue
			}
			if run.WorkspaceID == "" || len(run.Outcomes) != len(p.Rehearsals[i].Checks) || (run.Result != "passed" && run.Result != "failed") {
				return ErrInvalid
			}
			run.ID, run.CreatedBy, run.CreatedAt = randomID(), actor, s.now()
			p.Rehearsals[i].Runs = append(p.Rehearsals[i].Runs, run)
			plan = p
			return s.writePlan(p)
		}
		return ErrPlanNotFound
	})
	return plan, run, err
}

func CommandDigest(command string) string {
	sum := sha256.Sum256([]byte(command))
	return hex.EncodeToString(sum[:])
}

func sortedActions(actions []string) []string { sort.Strings(actions); return actions }
