package collaborationworkflows

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

var ErrGovernanceBlocked = errors.New("workflow governance blocked")

var consequentialActions = map[string]bool{
	"merge": true, "release": true, "change_infrastructure": true,
	"access_protected_evidence": true, "spend_funds": true,
}

type GovernancePolicy struct {
	RepositoryID             string    `json:"repository_id"`
	Version                  int       `json:"version"`
	RequiredReviews          int       `json:"required_reviews"`
	RequiredScenarioIDs      []string  `json:"required_scenario_ids"`
	RequireOwnerAcknowledged bool      `json:"require_owner_acknowledged"`
	RequireSeparation        bool      `json:"require_separation_of_duties"`
	ApprovalTTLSeconds       int       `json:"approval_ttl_seconds"`
	ProtectedActionClasses   []string  `json:"protected_action_classes"`
	ResourceOwnerIDs         []string  `json:"resource_owner_ids"`
	UpdatedBy                string    `json:"updated_by"`
	UpdatedAt                time.Time `json:"updated_at"`
}

type SimulationCase struct {
	ID              string   `json:"id"`
	Event           string   `json:"event"`
	ExpectedEffects []string `json:"expected_effects"`
	CostActions     int      `json:"cost_actions"`
}

type CandidateDecision struct {
	Kind       string     `json:"kind"`
	ActorID    string     `json:"actor_id"`
	OwnerID    string     `json:"owner_id,omitempty"`
	ScenarioID string     `json:"scenario_id,omitempty"`
	Reason     string     `json:"reason,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

type GovernanceCandidate struct {
	ID                      string              `json:"id"`
	RepositoryID            string              `json:"repository_id"`
	WorkflowID              string              `json:"workflow_id,omitempty"`
	ExpectedWorkflowVersion int                 `json:"expected_workflow_version"`
	Source                  Source              `json:"source"`
	DefinitionOwnerIDs      []string            `json:"definition_owner_ids"`
	PolicyVersion           int                 `json:"policy_version"`
	SimulationCases         []SimulationCase    `json:"simulation_cases"`
	PermissionAdded         []string            `json:"permission_added"`
	PermissionRemoved       []string            `json:"permission_removed"`
	EstimatedActionCost     int                 `json:"estimated_action_cost"`
	PolicyConflicts         []Diagnostic        `json:"policy_conflicts"`
	Decisions               []CandidateDecision `json:"decisions"`
	Ready                   bool                `json:"ready"`
	Blockers                []string            `json:"blockers"`
	CreatedBy               string              `json:"created_by"`
	CreatedAt               time.Time           `json:"created_at"`
}

func (s *Store) SetGovernancePolicy(repo, actor string, expectedVersion int, p GovernancePolicy) (GovernancePolicy, error) {
	var out GovernancePolicy
	err := s.lock(func() error {
		current, err := s.readGovernancePolicy(repo)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return err
		}
		if errors.Is(err, ErrNotFound) {
			current.Version = 0
		}
		if current.Version != expectedVersion || repo == "" || actor == "" || p.RequiredReviews < 0 || p.RequiredReviews > 20 || p.ApprovalTTLSeconds < 60 || p.ApprovalTTLSeconds > 30*86400 || len(p.ResourceOwnerIDs) == 0 {
			if current.Version != expectedVersion {
				return ErrConflict
			}
			return ErrInvalid
		}
		seen := map[string]bool{}
		for _, class := range p.ProtectedActionClasses {
			if !consequentialActions[class] || seen[class] {
				return ErrInvalid
			}
			seen[class] = true
		}
		p.RepositoryID, p.Version, p.UpdatedBy, p.UpdatedAt = repo, current.Version+1, actor, s.now()
		p.ProtectedActionClasses = unique(p.ProtectedActionClasses)
		p.ResourceOwnerIDs = unique(p.ResourceOwnerIDs)
		p.RequiredScenarioIDs = unique(p.RequiredScenarioIDs)
		out = p
		return writeJSONAtomic(filepath.Join(s.root, "governance-"+repo+".json"), p)
	})
	return out, err
}

func (s *Store) GetGovernancePolicy(repo string) (GovernancePolicy, error) {
	return s.readGovernancePolicy(repo)
}

func (s *Store) readGovernancePolicy(repo string) (GovernancePolicy, error) {
	var p GovernancePolicy
	b, err := os.ReadFile(filepath.Join(s.root, "governance-"+repo+".json"))
	if os.IsNotExist(err) {
		return p, ErrNotFound
	}
	if err != nil {
		return p, err
	}
	if json.Unmarshal(b, &p) != nil || p.RepositoryID != repo || p.Version < 1 {
		return p, ErrInvalid
	}
	return p, nil
}

func (s *Store) EvaluateCandidate(repo, workflowID, actor string, expectedVersion int, preview Preview) (GovernanceCandidate, error) {
	p, err := s.readGovernancePolicy(repo)
	if err != nil {
		return GovernanceCandidate{}, err
	}
	if !preview.Activatable || preview.Source.SHA256 == "" {
		return GovernanceCandidate{}, ErrInvalid
	}
	c := GovernanceCandidate{RepositoryID: repo, WorkflowID: workflowID, ExpectedWorkflowVersion: expectedVersion, Source: preview.Source, DefinitionOwnerIDs: unique(preview.Definition.OwnerIDs), PolicyVersion: p.Version, CreatedBy: actor, CreatedAt: s.now(), Decisions: []CandidateDecision{}, PolicyConflicts: preview.Diagnostics}
	grants := []string{}
	for _, st := range preview.Definition.Steps {
		c.DefinitionOwnerIDs = append(c.DefinitionOwnerIDs, st.OwnerIDs...)
		if contains(p.ProtectedActionClasses, st.Invocation.Action) && st.Approval != st.Invocation.Action {
			c.PolicyConflicts = append(c.PolicyConflicts, Diagnostic{Kind: "missing_action_approval", Message: "protected action requires a matching time-bounded approval", AttributedTo: "workflow_governance", StepID: st.ID, ResourceID: st.Invocation.Action})
		}
	}
	c.DefinitionOwnerIDs = unique(c.DefinitionOwnerIDs)
	for _, a := range preview.EffectiveAuthority {
		grants = append(grants, a.Grants...)
	}
	currentGrants := []string{}
	if workflowID != "" {
		w, e := s.Get(workflowID)
		if e != nil || w.RepositoryID != repo || w.CurrentVersion != expectedVersion {
			return GovernanceCandidate{}, ErrConflict
		}
		for _, a := range w.Revisions[w.CurrentVersion-1].EffectiveAuthority {
			currentGrants = append(currentGrants, a.Grants...)
		}
	}
	c.PermissionAdded, c.PermissionRemoved = difference(unique(grants), unique(currentGrants)), difference(unique(currentGrants), unique(grants))
	for _, tr := range preview.Definition.Triggers {
		effects, cost := []string{}, 0
		for _, st := range preview.Definition.Steps {
			effects = append(effects, st.Invocation.Action)
			cost += st.BudgetActions
		}
		c.SimulationCases = append(c.SimulationCases, SimulationCase{ID: tr.ID, Event: tr.Kind + ":" + tr.Event, ExpectedEffects: nonempty(effects), CostActions: cost})
	}
	c.EstimatedActionCost = preview.Definition.BudgetActions
	h := sha256.Sum256([]byte(repo + "\x00" + workflowID + "\x00" + preview.Source.SHA256 + "\x00" + strconv.Itoa(p.Version)))
	c.ID = hex.EncodeToString(h[:16])
	sort.Slice(c.SimulationCases, func(i, j int) bool { return c.SimulationCases[i].ID < c.SimulationCases[j].ID })
	c = projectCandidate(c, p, s.now())
	if err := s.lock(func() error {
		path := filepath.Join(s.root, "candidate-"+c.ID+".json")
		if existing, readErr := s.readCandidate(c.ID); readErr == nil {
			if existing.RepositoryID == c.RepositoryID && existing.WorkflowID == c.WorkflowID && existing.Source.SHA256 == c.Source.SHA256 && existing.PolicyVersion == c.PolicyVersion {
				c = projectCandidate(existing, p, s.now())
				return nil
			}
			return ErrConflict
		} else if !errors.Is(readErr, ErrNotFound) {
			return readErr
		}
		return writeJSONAtomic(path, c)
	}); err != nil {
		return GovernanceCandidate{}, err
	}
	return c, nil
}

func (s *Store) DecideCandidate(id, actor, kind, ownerID, scenarioID, reason string, expiresAt *time.Time) (GovernanceCandidate, error) {
	var out GovernanceCandidate
	err := s.lock(func() error {
		c, err := s.readCandidate(id)
		if err != nil {
			return err
		}
		p, err := s.readGovernancePolicy(c.RepositoryID)
		if err != nil || p.Version != c.PolicyVersion {
			return ErrGovernanceBlocked
		}
		if !oneOf(kind, "review", "owner_acknowledgement", "scenario_pass", "exception") || actor == "" {
			return ErrInvalid
		}
		if kind == "owner_acknowledgement" && (actor != ownerID || !contains(p.ResourceOwnerIDs, ownerID)) {
			return ErrGovernanceBlocked
		}
		if kind == "scenario_pass" && !contains(p.RequiredScenarioIDs, scenarioID) {
			return ErrInvalid
		}
		if kind == "exception" && (expiresAt == nil || !expiresAt.After(s.now()) || expiresAt.After(s.now().Add(30*24*time.Hour)) || !contains(p.ResourceOwnerIDs, actor) || strings.TrimSpace(reason) == "") {
			return ErrGovernanceBlocked
		}
		for _, d := range c.Decisions {
			if d.Kind == kind && d.ActorID == actor && d.OwnerID == ownerID && d.ScenarioID == scenarioID {
				return ErrConflict
			}
		}
		c.Decisions = append(c.Decisions, CandidateDecision{Kind: kind, ActorID: actor, OwnerID: ownerID, ScenarioID: scenarioID, Reason: reason, ExpiresAt: expiresAt, CreatedAt: s.now()})
		out = projectCandidate(c, p, s.now())
		return writeJSONAtomic(filepath.Join(s.root, "candidate-"+id+".json"), out)
	})
	return out, err
}

func (s *Store) GetCandidate(id string) (GovernanceCandidate, error) {
	c, err := s.readCandidate(id)
	if err != nil {
		return c, err
	}
	p, err := s.readGovernancePolicy(c.RepositoryID)
	if err != nil {
		return c, err
	}
	return projectCandidate(c, p, s.now()), nil
}

func (s *Store) RequireApprovedCandidate(repo, workflowID, sourceSHA string, expectedVersion int) error {
	return s.requireApprovedCandidateUnlocked(repo, workflowID, sourceSHA, expectedVersion)
}

func (s *Store) requireApprovedCandidateUnlocked(repo, workflowID, sourceSHA string, expectedVersion int) error {
	if _, err := s.readGovernancePolicy(repo); errors.Is(err, ErrNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	entries, _ := filepath.Glob(filepath.Join(s.root, "candidate-*.json"))
	for _, path := range entries {
		var c GovernanceCandidate
		b, e := os.ReadFile(path)
		if e == nil && json.Unmarshal(b, &c) == nil && c.RepositoryID == repo && c.WorkflowID == workflowID && c.Source.SHA256 == sourceSHA && c.ExpectedWorkflowVersion == expectedVersion {
			projected, e := s.GetCandidate(c.ID)
			if e == nil && projected.Ready {
				return nil
			}
		}
	}
	return ErrGovernanceBlocked
}

func (s *Store) readCandidate(id string) (GovernanceCandidate, error) {
	var c GovernanceCandidate
	b, e := os.ReadFile(filepath.Join(s.root, "candidate-"+id+".json"))
	if os.IsNotExist(e) {
		return c, ErrNotFound
	}
	if e != nil {
		return c, e
	}
	if json.Unmarshal(b, &c) != nil || c.ID != id {
		return c, ErrInvalid
	}
	return c, nil
}

func projectCandidate(c GovernanceCandidate, p GovernancePolicy, now time.Time) GovernanceCandidate {
	blockers := []string{}
	reviewers, owners, scenarios := map[string]bool{}, map[string]bool{}, map[string]bool{}
	validException := false
	for _, d := range c.Decisions {
		switch d.Kind {
		case "review":
			reviewers[d.ActorID] = true
		case "owner_acknowledgement":
			owners[d.OwnerID] = true
		case "scenario_pass":
			scenarios[d.ScenarioID] = true
		case "exception":
			validException = c.PolicyVersion == p.Version && contains(p.ResourceOwnerIDs, d.ActorID) && d.ExpiresAt != nil && d.ExpiresAt.After(now)
		}
	}
	if len(reviewers) < p.RequiredReviews {
		blockers = append(blockers, "required_reviews")
	}
	for _, id := range p.RequiredScenarioIDs {
		if !scenarios[id] {
			blockers = append(blockers, "scenario:"+id)
		}
	}
	if p.RequireOwnerAcknowledged {
		for _, id := range p.ResourceOwnerIDs {
			if !owners[id] {
				blockers = append(blockers, "owner_acknowledgement:"+id)
			}
		}
	}
	if p.RequireSeparation {
		for actor := range reviewers {
			if contains(c.DefinitionOwnerIDs, actor) || owners[actor] {
				blockers = append(blockers, "separation_of_duties")
				break
			}
		}
	}
	if c.PolicyVersion != p.Version {
		blockers = append(blockers, "policy_changed")
	}
	if len(c.PolicyConflicts) > 0 {
		blockers = append(blockers, "policy_conflict")
	}
	if validException {
		blockers = []string{}
	}
	c.Blockers, c.Ready = unique(blockers), len(blockers) == 0
	return c
}

func difference(a, b []string) []string {
	out := []string{}
	for _, v := range a {
		if !contains(b, v) {
			out = append(out, v)
		}
	}
	return out
}
func nonempty(v []string) []string {
	out := []string{}
	for _, x := range v {
		if x != "" {
			out = append(out, x)
		}
	}
	return out
}
func writeJSONAtomic(path string, value any) error {
	b, e := json.MarshalIndent(value, "", "  ")
	if e != nil {
		return e
	}
	tmp := path + ".tmp"
	if e = os.WriteFile(tmp, b, 0600); e != nil {
		return e
	}
	return os.Rename(tmp, path)
}
