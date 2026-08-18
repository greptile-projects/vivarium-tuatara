package infrastructure

import (
	"testing"
	"time"
)

func TestInfrastructureExecutionIsDependencyOrderedSteerableAndAgentBounded(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	passed := Rehearsal{ID: "rehearsal", Runs: []RehearsalRun{{Result: "passed"}}}
	plan := ChangePlan{ID: "plan", RepositoryID: "repo", PullRequestID: "pull", SourceRevision: "reviewed", CandidateDigest: "digest", DefinitionID: "definition", DefinitionVersion: 2, AffectedOwnerIDs: []string{"owner"}, AcknowledgedOwnerIDs: []string{"owner"}, Rehearsals: []Rehearsal{passed}, Changes: []PlanChange{
		{ResourceID: "network", Action: "create", Order: 1},
		{ResourceID: "service", Action: "change", DependencyIDs: []string{"network"}, Order: 2},
	}}
	x, err := s.CreateExecution(plan, "operator", ExecutionCreation{MergeCommitID: "merged", EnvironmentID: "prod", EnvironmentPolicy: "production policy v3", RehearsalID: "rehearsal", BudgetUnits: 10, CredentialExpiry: now.Add(time.Hour), Delegations: []ExecutionDelegation{{StepID: "step-service", AgentID: "agent", Mandate: "report the service update"}}})
	if err != nil || x.Status != "running" || len(x.Credential.Actions) == 0 {
		t.Fatalf("create = %#v, %v", x, err)
	}
	if _, err = s.ReportExecution(x.ID, "agent", "agent", "step-service", 1, StepReport{Status: "succeeded", ProviderResponse: "updated", Health: "healthy", NextAction: "done", SafetyPoint: true}); err != ErrExecutionBlocked {
		t.Fatalf("agent bypassed dependency: %v", err)
	}
	x, err = s.ReportExecution(x.ID, "operator", "human", "step-network", 1, StepReport{Status: "succeeded", ProviderResponse: "provider accepted network", Health: "healthy", CostUnits: 3, NextAction: "apply service", SafetyPoint: true})
	if err != nil {
		t.Fatal(err)
	}
	x, err = s.ReportExecution(x.ID, "agent", "agent", "step-service", 2, StepReport{Status: "succeeded", ProviderResponse: "provider accepted service", Health: "healthy", CostUnits: 4, NextAction: "observe health", SafetyPoint: true})
	if err != nil || x.Status != "succeeded" || x.CostUnits != 7 {
		t.Fatalf("completion = %#v, %v", x, err)
	}
}

func TestInfrastructureExecutionPausesOnBlockerAndHonorsSafetyAndBudget(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	plan := ChangePlan{ID: "plan", RepositoryID: "repo", AffectedOwnerIDs: []string{"owner"}, AcknowledgedOwnerIDs: []string{"owner"}, Rehearsals: []Rehearsal{{ID: "r", Runs: []RehearsalRun{{Result: "passed"}}}}, Changes: []PlanChange{{ResourceID: "db", Action: "replace", Order: 1}}}
	if _, err := s.CreateExecution(plan, "operator", ExecutionCreation{MergeCommitID: "merged", EnvironmentID: "prod", EnvironmentPolicy: "approved", RehearsalID: "r", BudgetUnits: 5, CredentialExpiry: now.Add(time.Hour), Delegations: []ExecutionDelegation{{StepID: "step-db", AgentID: "agent", Mandate: "replace database"}}}); err != ErrInvalid {
		t.Fatalf("destructive agent delegation = %v", err)
	}
	x, err := s.CreateExecution(plan, "operator", ExecutionCreation{MergeCommitID: "merged", EnvironmentID: "prod", EnvironmentPolicy: "approved", RehearsalID: "r", BudgetUnits: 5, CredentialExpiry: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ReportExecution(x.ID, "operator", "human", "step-db", 1, StepReport{Status: "running", ProviderResponse: "replacement began", Health: "healthy", CostUnits: 6, NextAction: "continue", SafetyPoint: false}); err != ErrExecutionBlocked {
		t.Fatalf("budget bypass = %v", err)
	}
	x, err = s.ReportExecution(x.ID, "operator", "human", "step-db", 1, StepReport{Status: "failed", ProviderResponse: "provider capacity unavailable", Health: "degraded", CostUnits: 1, Blockers: []string{"capacity unavailable"}, NextAction: "remediate capacity", SafetyPoint: true})
	if err != nil || x.Status != "paused" {
		t.Fatalf("pause = %#v, %v", x, err)
	}
	if _, err = s.ControlExecution(x.ID, "operator", "resume", "retry", x.Version); err != ErrExecutionBlocked {
		t.Fatalf("resumed through blocker = %v", err)
	}
	x, err = s.ReportExecution(x.ID, "operator", "human", "step-db", x.Version, StepReport{Status: "running", ProviderResponse: "capacity remediation accepted", Health: "healthy", CostUnits: 2, NextAction: "continue replacement", SafetyPoint: true})
	if err != nil || x.Status != "paused" || len(x.Blockers) != 0 {
		t.Fatalf("remediation = %#v, %v", x, err)
	}
	x, err = s.ControlExecution(x.ID, "operator", "resume", "capacity evidence is healthy", x.Version)
	if err != nil || x.Status != "running" {
		t.Fatalf("resume after remediation = %#v, %v", x, err)
	}
	if _, err = s.ControlExecution(x.ID, "operator", "cancel", "stop safely", x.Version); err != nil {
		t.Fatalf("cancel = %v", err)
	}
}
