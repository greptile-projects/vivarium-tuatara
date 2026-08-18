package infrastructure

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var ErrExecutionNotFound = errors.New("infrastructure execution not found")
var ErrExecutionBlocked = errors.New("infrastructure execution transition blocked")

type ExecutionDelegation struct {
	StepID  string `json:"step_id"`
	AgentID string `json:"agent_id"`
	Mandate string `json:"mandate"`
}

type ExecutionCredential struct {
	PrincipalID string    `json:"principal_id"`
	StepIDs     []string  `json:"step_ids"`
	ResourceIDs []string  `json:"resource_ids"`
	Actions     []string  `json:"actions"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type ExecutionStep struct {
	ID               string    `json:"id"`
	Order            int       `json:"order"`
	ResourceID       string    `json:"resource_id"`
	Action           string    `json:"action"`
	DependencyIDs    []string  `json:"dependency_ids"`
	Status           string    `json:"status"`
	ControllerID     string    `json:"controller_id,omitempty"`
	ProviderResponse string    `json:"provider_response,omitempty"`
	Health           string    `json:"health"`
	CostUnits        float64   `json:"cost_units"`
	Blockers         []string  `json:"blockers"`
	NextAction       string    `json:"next_action"`
	SafetyPoint      bool      `json:"safety_point"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type ExecutionEvent struct {
	Sequence  int       `json:"sequence"`
	Kind      string    `json:"kind"`
	ActorID   string    `json:"actor_id"`
	ActorType string    `json:"actor_type"`
	StepID    string    `json:"step_id,omitempty"`
	Summary   string    `json:"summary"`
	CreatedAt time.Time `json:"created_at"`
}

type Execution struct {
	ID                 string                `json:"id"`
	RepositoryID       string                `json:"repository_id"`
	PlanID             string                `json:"plan_id"`
	PullRequestID      string                `json:"pull_request_id"`
	ReviewedRevision   string                `json:"reviewed_revision"`
	MergeCommitID      string                `json:"merge_commit_id"`
	CandidateDigest    string                `json:"candidate_digest"`
	DefinitionID       string                `json:"definition_id"`
	DefinitionVersion  int                   `json:"definition_version"`
	EnvironmentID      string                `json:"environment_id"`
	EnvironmentPolicy  string                `json:"environment_policy"`
	RehearsalID        string                `json:"rehearsal_id"`
	BudgetUnits        float64               `json:"budget_units"`
	CostUnits          float64               `json:"cost_units"`
	Status             string                `json:"status"`
	ActiveControllerID string                `json:"active_controller_id"`
	Version            int                   `json:"version"`
	Steps              []ExecutionStep       `json:"steps"`
	Delegations        []ExecutionDelegation `json:"delegations"`
	Credential         ExecutionCredential   `json:"credential"`
	Events             []ExecutionEvent      `json:"events"`
	Blockers           []string              `json:"blockers"`
	NextActions        []string              `json:"next_actions"`
	CreatedBy          string                `json:"created_by"`
	CreatedAt          time.Time             `json:"created_at"`
	UpdatedAt          time.Time             `json:"updated_at"`
}

type ExecutionCreation struct {
	MergeCommitID     string
	EnvironmentID     string
	EnvironmentPolicy string
	RehearsalID       string
	BudgetUnits       float64
	CredentialExpiry  time.Time
	Delegations       []ExecutionDelegation
}

type StepReport struct {
	Status           string   `json:"status"`
	ProviderResponse string   `json:"provider_response"`
	Health           string   `json:"health"`
	CostUnits        float64  `json:"cost_units"`
	Blockers         []string `json:"blockers"`
	NextAction       string   `json:"next_action"`
	SafetyPoint      bool     `json:"safety_point"`
}

func (s *Store) CreateExecution(plan ChangePlan, actor string, in ExecutionCreation) (Execution, error) {
	var out Execution
	err := s.lock(func() error {
		if plan.ID == "" || in.MergeCommitID == "" || in.EnvironmentID == "" || strings.TrimSpace(in.EnvironmentPolicy) == "" || in.BudgetUnits < 0 || !in.CredentialExpiry.After(s.now()) || in.CredentialExpiry.After(s.now().Add(time.Hour)) || !planOwnersAcknowledged(plan) {
			return ErrExecutionBlocked
		}
		entries, err := os.ReadDir(s.executionDir(plan.RepositoryID))
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			prior, readErr := s.readExecutionFile(filepath.Join(s.executionDir(plan.RepositoryID), entry.Name()))
			if readErr != nil {
				return readErr
			}
			if prior.EnvironmentID == in.EnvironmentID && (prior.Status == "running" || prior.Status == "paused") {
				return ErrExecutionBlocked
			}
		}
		passed := false
		for _, rehearsal := range plan.Rehearsals {
			environmentBound := rehearsal.Scope.EnvironmentID == in.EnvironmentID || (rehearsal.Scope.EnvironmentKind == "policy_approved_ephemeral" && strings.TrimSpace(rehearsal.Scope.PolicyApproval) != "" && rehearsal.Scope.PolicyApproval == strings.TrimSpace(in.EnvironmentPolicy))
			if rehearsal.ID == in.RehearsalID && environmentBound && len(rehearsal.Runs) > 0 && rehearsal.Runs[len(rehearsal.Runs)-1].Result == "passed" {
				passed = true
			}
		}
		if !passed {
			return ErrExecutionBlocked
		}
		changed := map[string]bool{}
		for _, change := range plan.Changes {
			if change.ResourceID == "" || changed[change.ResourceID] {
				return ErrInvalid
			}
			changed[change.ResourceID] = true
		}
		candidateResources := map[string]bool{}
		for _, resource := range plan.Candidate.Resources {
			candidateResources[resource.ID] = true
		}
		steps, resources := make([]ExecutionStep, 0, len(plan.Changes)), []string{}
		stepIDs := map[string]bool{}
		for _, change := range plan.Changes {
			id := "step-" + change.ResourceID
			stepIDs[id] = true
			resources = append(resources, change.ResourceID)
			dependencies := []string{}
			for _, dependencyID := range change.DependencyIDs {
				if !changed[dependencyID] && !candidateResources[dependencyID] {
					return ErrInvalid
				}
				if changed[dependencyID] {
					dependencies = append(dependencies, dependencyID)
				}
			}
			steps = append(steps, ExecutionStep{ID: id, Order: change.Order, ResourceID: change.ResourceID, Action: change.Action, DependencyIDs: dependencies, Status: "pending", Health: "unknown", Blockers: []string{}, NextAction: "wait for dependencies, then apply the frozen change", SafetyPoint: true})
		}
		if cyclicExecutionSteps(steps) {
			return ErrInvalid
		}
		seenAgents := map[string]bool{}
		for _, d := range in.Delegations {
			if !stepIDs[d.StepID] || d.AgentID == "" || strings.TrimSpace(d.Mandate) == "" || unsafe(d.Mandate) || seenAgents[d.StepID+":"+d.AgentID] {
				return ErrInvalid
			}
			for _, step := range steps {
				if step.ID == d.StepID && (step.Action == "destroy" || step.Action == "replace") {
					return ErrInvalid
				}
			}
			seenAgents[d.StepID+":"+d.AgentID] = true
		}
		now := s.now()
		out = Execution{ID: randomID(), RepositoryID: plan.RepositoryID, PlanID: plan.ID, PullRequestID: plan.PullRequestID, ReviewedRevision: plan.SourceRevision, MergeCommitID: in.MergeCommitID, CandidateDigest: plan.CandidateDigest, DefinitionID: plan.DefinitionID, DefinitionVersion: plan.DefinitionVersion, EnvironmentID: in.EnvironmentID, EnvironmentPolicy: strings.TrimSpace(in.EnvironmentPolicy), RehearsalID: in.RehearsalID, BudgetUnits: in.BudgetUnits, Status: "running", ActiveControllerID: actor, Version: 1, Steps: steps, Delegations: append([]ExecutionDelegation{}, in.Delegations...), Credential: ExecutionCredential{PrincipalID: actor, StepIDs: keys(stepIDs), ResourceIDs: resources, Actions: []string{"report", "pause", "resume", "cancel"}, ExpiresAt: in.CredentialExpiry}, Events: []ExecutionEvent{{Sequence: 1, Kind: "started", ActorID: actor, ActorType: "human", Summary: "exact merged infrastructure execution started", CreatedAt: now}}, Blockers: []string{}, NextActions: []string{"report the first dependency-ready step"}, CreatedBy: actor, CreatedAt: now, UpdatedAt: now}
		return s.writeExecution(out)
	})
	return out, err
}

func (s *Store) ReportExecution(id, actor, actorType, stepID string, expected int, report StepReport) (Execution, error) {
	var out Execution
	err := s.lock(func() error {
		x, err := s.readExecution(id)
		if err != nil {
			return err
		}
		if x.Version != expected || (x.Status != "running" && x.Status != "paused") || s.now().After(x.Credential.ExpiresAt) {
			return ErrExecutionBlocked
		}
		idx := -1
		for i := range x.Steps {
			if x.Steps[i].ID == stepID {
				idx = i
			}
		}
		if idx < 0 || (report.Status != "running" && report.Status != "succeeded" && report.Status != "failed") || (report.Health != "healthy" && report.Health != "degraded" && report.Health != "unknown") || report.CostUnits < 0 || strings.TrimSpace(report.ProviderResponse) == "" || strings.TrimSpace(report.NextAction) == "" || unsafe(report.ProviderResponse, report.NextAction, strings.Join(report.Blockers, " ")) {
			return ErrInvalid
		}
		if actorType == "agent" {
			allowed := false
			for _, d := range x.Delegations {
				if d.AgentID == actor && d.StepID == stepID {
					allowed = true
				}
			}
			if !allowed || x.Steps[idx].Action == "destroy" || x.Steps[idx].Action == "replace" {
				return ErrExecutionBlocked
			}
		} else if actor != x.ActiveControllerID {
			return ErrExecutionBlocked
		}
		for _, dep := range x.Steps[idx].DependencyIDs {
			matches := 0
			for _, prior := range x.Steps {
				if prior.ResourceID == dep {
					matches++
					if prior.Status != "succeeded" {
						return ErrExecutionBlocked
					}
				}
			}
			if matches != 1 {
				return ErrExecutionBlocked
			}
		}
		priorCost := x.Steps[idx].CostUnits
		if x.CostUnits-priorCost+report.CostUnits > x.BudgetUnits {
			return ErrExecutionBlocked
		}
		wasPaused := x.Status == "paused"
		if wasPaused && report.Status == "succeeded" {
			return ErrExecutionBlocked
		}
		stepStatus := report.Status
		if wasPaused {
			stepStatus = x.Steps[idx].Status
		}
		x.Steps[idx].Status, x.Steps[idx].ControllerID, x.Steps[idx].ProviderResponse, x.Steps[idx].Health, x.Steps[idx].CostUnits, x.Steps[idx].Blockers, x.Steps[idx].NextAction, x.Steps[idx].SafetyPoint, x.Steps[idx].UpdatedAt = stepStatus, actor, report.ProviderResponse, report.Health, report.CostUnits, append([]string{}, report.Blockers...), report.NextAction, report.SafetyPoint, s.now()
		x.CostUnits = x.CostUnits - priorCost + report.CostUnits
		kind := "step_reported"
		if report.Status == "failed" || report.Health == "degraded" || len(report.Blockers) > 0 {
			x.Status = "paused"
			kind = "safety_pause"
		}
		complete := true
		x.Blockers = []string{}
		x.NextActions = []string{}
		for _, step := range x.Steps {
			if step.Status != "succeeded" {
				complete = false
				x.NextActions = append(x.NextActions, step.NextAction)
			}
			x.Blockers = append(x.Blockers, step.Blockers...)
		}
		if complete {
			x.Status = "succeeded"
			kind = "completed"
			x.NextActions = []string{}
		} else if wasPaused {
			x.Status = "paused"
		}
		x.Version++
		x.UpdatedAt = s.now()
		x.Events = append(x.Events, ExecutionEvent{Sequence: len(x.Events) + 1, Kind: kind, ActorID: actor, ActorType: actorType, StepID: stepID, Summary: report.ProviderResponse, CreatedAt: s.now()})
		out = x
		return s.writeExecution(x)
	})
	return out, err
}

func (s *Store) ControlExecution(id, actor, action, summary string, expected int) (Execution, error) {
	var out Execution
	err := s.lock(func() error {
		x, err := s.readExecution(id)
		if err != nil {
			return err
		}
		if x.Version != expected || actor != x.ActiveControllerID || unsafe(summary) {
			return ErrExecutionBlocked
		}
		atSafety := true
		for _, step := range x.Steps {
			if step.Status == "running" && !step.SafetyPoint {
				atSafety = false
			}
		}
		if !atSafety {
			return ErrExecutionBlocked
		}
		switch action {
		case "pause":
			if x.Status != "running" {
				return ErrExecutionBlocked
			}
			x.Status = "paused"
		case "resume":
			if x.Status != "paused" || len(x.Blockers) > 0 || s.now().After(x.Credential.ExpiresAt) {
				return ErrExecutionBlocked
			}
			x.Status = "running"
		case "cancel":
			if x.Status != "running" && x.Status != "paused" {
				return ErrExecutionBlocked
			}
			x.Status = "cancelled"
		default:
			return ErrInvalid
		}
		x.Version++
		x.UpdatedAt = s.now()
		x.Events = append(x.Events, ExecutionEvent{Sequence: len(x.Events) + 1, Kind: action + "d", ActorID: actor, ActorType: "human", Summary: summary, CreatedAt: s.now()})
		out = x
		return s.writeExecution(x)
	})
	return out, err
}

func (s *Store) GetExecution(id string) (Execution, error) {
	var out Execution
	err := s.lock(func() error { var e error; out, e = s.readExecution(id); return e })
	return out, err
}
func (s *Store) ListExecutions(repo string) ([]Execution, error) {
	out := []Execution{}
	err := s.lock(func() error {
		entries, e := os.ReadDir(s.executionDir(repo))
		if os.IsNotExist(e) {
			return nil
		}
		if e != nil {
			return e
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			x, e := s.readExecutionFile(filepath.Join(s.executionDir(repo), entry.Name()))
			if e != nil {
				return e
			}
			out = append(out, x)
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, err
}
func (s *Store) executionDir(repo string) string { return filepath.Join(s.root, "executions", repo) }
func (s *Store) writeExecution(x Execution) error {
	dir := s.executionDir(x.RepositoryID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	body, err := json.MarshalIndent(x, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".execution-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(body)
	}
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, filepath.Join(dir, x.ID+".json"))
}
func (s *Store) readExecution(id string) (Execution, error) {
	matches, _ := filepath.Glob(filepath.Join(s.root, "executions", "*", id+".json"))
	if len(matches) != 1 {
		return Execution{}, ErrExecutionNotFound
	}
	return s.readExecutionFile(matches[0])
}
func (s *Store) readExecutionFile(path string) (Execution, error) {
	var x Execution
	body, err := os.ReadFile(path)
	if err != nil {
		return x, err
	}
	if json.Unmarshal(body, &x) != nil {
		return x, ErrExecutionNotFound
	}
	return x, nil
}
func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for x := range m {
		out = append(out, x)
	}
	sort.Strings(out)
	return out
}

func allAcknowledged(owners, acknowledgements []string) bool {
	seen := map[string]bool{}
	for _, id := range acknowledgements {
		seen[id] = true
	}
	for _, id := range owners {
		if !seen[id] {
			return false
		}
	}
	return len(owners) > 0
}

func planOwnersAcknowledged(plan ChangePlan) bool {
	acknowledgements := append([]string{}, plan.AcknowledgedOwnerIDs...)
	for _, event := range plan.Events {
		if event.Kind == "owner_acknowledgement" && event.ActorType == "human" && event.ActorID == event.OwnerID {
			acknowledgements = append(acknowledgements, event.OwnerID)
		}
	}
	return allAcknowledged(plan.AffectedOwnerIDs, acknowledgements)
}

func cyclicExecutionSteps(steps []ExecutionStep) bool {
	dependencies := map[string][]string{}
	for _, step := range steps {
		dependencies[step.ResourceID] = append([]string{}, step.DependencyIDs...)
	}
	visiting, visited := map[string]bool{}, map[string]bool{}
	var visit func(string) bool
	visit = func(id string) bool {
		if visiting[id] {
			return true
		}
		if visited[id] {
			return false
		}
		visiting[id] = true
		for _, dependencyID := range dependencies[id] {
			if visit(dependencyID) {
				return true
			}
		}
		delete(visiting, id)
		visited[id] = true
		return false
	}
	for id := range dependencies {
		if visit(id) {
			return true
		}
	}
	return false
}
