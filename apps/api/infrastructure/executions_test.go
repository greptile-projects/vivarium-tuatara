package infrastructure

import (
	"testing"
	"time"
)

func TestInfrastructureExecutionIsDependencyOrderedSteerableAndAgentBounded(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	passed := Rehearsal{ID: "rehearsal", Scope: RehearsalScope{EnvironmentID: "prod"}, Runs: []RehearsalRun{{Result: "passed"}}}
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
	plan := ChangePlan{ID: "plan", RepositoryID: "repo", AffectedOwnerIDs: []string{"owner"}, AcknowledgedOwnerIDs: []string{"owner"}, Rehearsals: []Rehearsal{{ID: "r", Scope: RehearsalScope{EnvironmentID: "prod"}, Runs: []RehearsalRun{{Result: "passed"}}}}, Changes: []PlanChange{{ResourceID: "db", Action: "replace", Order: 1}}}
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

func TestInfrastructureExecutionDerivesPersistedAcknowledgementsAndLocksEnvironment(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	plan := ChangePlan{ID: "plan-one", RepositoryID: "repo", AffectedOwnerIDs: []string{"owner"}, Events: []PlanEvent{{Kind: "owner_acknowledgement", ActorID: "owner", ActorType: "human", OwnerID: "owner"}}, Rehearsals: []Rehearsal{{ID: "r", Scope: RehearsalScope{EnvironmentID: "prod"}, Runs: []RehearsalRun{{Result: "passed"}}}}, Changes: []PlanChange{{ResourceID: "service", Action: "change", Order: 1}}}
	first, err := s.CreateExecution(plan, "operator", ExecutionCreation{MergeCommitID: "merged", EnvironmentID: "prod", EnvironmentPolicy: "policy", RehearsalID: "r", BudgetUnits: 5, CredentialExpiry: now.Add(time.Hour)})
	if err != nil || first.Status != "running" {
		t.Fatalf("persisted acknowledgement admission = %#v, %v", first, err)
	}
	plan.ID = "plan-two"
	if _, err = s.CreateExecution(plan, "other-operator", ExecutionCreation{MergeCommitID: "merged-two", EnvironmentID: "prod", EnvironmentPolicy: "policy", RehearsalID: "r", BudgetUnits: 5, CredentialExpiry: now.Add(time.Hour)}); err != ErrExecutionBlocked {
		t.Fatalf("second environment controller = %v", err)
	}
}

func TestInfrastructureExecutionRejectsMissingAndCyclicChangedDependencies(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	base := ChangePlan{ID: "dependencies", RepositoryID: "repo", AffectedOwnerIDs: []string{"owner"}, AcknowledgedOwnerIDs: []string{"owner"}, Rehearsals: []Rehearsal{{ID: "r", Scope: RehearsalScope{EnvironmentID: "prod"}, Runs: []RehearsalRun{{Result: "passed"}}}}}
	in := ExecutionCreation{MergeCommitID: "merged", EnvironmentID: "prod", EnvironmentPolicy: "policy", RehearsalID: "r", BudgetUnits: 5, CredentialExpiry: now.Add(time.Hour)}
	missing := base
	missing.Changes = []PlanChange{{ResourceID: "service", Action: "change", Order: 1, DependencyIDs: []string{"absent-network"}}}
	if _, err := s.CreateExecution(missing, "operator", in); err != ErrInvalid {
		t.Fatalf("missing dependency = %v", err)
	}
	cyclic := base
	cyclic.ID = "cycle"
	cyclic.Changes = []PlanChange{{ResourceID: "alpha", Action: "change", Order: 1, DependencyIDs: []string{"beta"}}, {ResourceID: "beta", Action: "change", Order: 2, DependencyIDs: []string{"alpha"}}}
	if _, err := s.CreateExecution(cyclic, "operator", in); err != ErrInvalid {
		t.Fatalf("cyclic dependency = %v", err)
	}
}

func TestPausedInfrastructureExecutionCannotCompleteBeforeResume(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	plan := ChangePlan{ID: "pause", RepositoryID: "repo", AffectedOwnerIDs: []string{"owner"}, AcknowledgedOwnerIDs: []string{"owner"}, Rehearsals: []Rehearsal{{ID: "r", Scope: RehearsalScope{EnvironmentID: "prod"}, Runs: []RehearsalRun{{Result: "passed"}}}}, Changes: []PlanChange{{ResourceID: "service", Action: "change", Order: 1}}}
	x, err := s.CreateExecution(plan, "operator", ExecutionCreation{MergeCommitID: "merged", EnvironmentID: "prod", EnvironmentPolicy: "policy", RehearsalID: "r", BudgetUnits: 5, CredentialExpiry: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	x, err = s.ReportExecution(x.ID, "operator", "human", "step-service", x.Version, StepReport{Status: "failed", ProviderResponse: "health failed", Health: "degraded", Blockers: []string{"health"}, NextAction: "remediate", SafetyPoint: true})
	if err != nil || x.Status != "paused" {
		t.Fatalf("pause = %#v, %v", x, err)
	}
	if _, err = s.ReportExecution(x.ID, "operator", "human", "step-service", x.Version, StepReport{Status: "succeeded", ProviderResponse: "health recovered", Health: "healthy", NextAction: "complete", SafetyPoint: true}); err != ErrExecutionBlocked {
		t.Fatalf("completed while paused = %v", err)
	}
}

func TestInfrastructureExecutionRequiresEnvironmentBoundRehearsalEvidence(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	plan := ChangePlan{ID: "environment-proof", RepositoryID: "repo", AffectedOwnerIDs: []string{"owner"}, AcknowledgedOwnerIDs: []string{"owner"}, Rehearsals: []Rehearsal{{ID: "r", Scope: RehearsalScope{EnvironmentKind: "isolated", EnvironmentID: "preview-isolated-42"}, Runs: []RehearsalRun{{Result: "passed"}}}}, Changes: []PlanChange{{ResourceID: "service", Action: "change", Order: 1}}}
	in := ExecutionCreation{MergeCommitID: "merged", EnvironmentID: "production", EnvironmentPolicy: "production-policy-v2", RehearsalID: "r", BudgetUnits: 5, CredentialExpiry: now.Add(time.Hour)}
	if _, err := s.CreateExecution(plan, "operator", in); err != ErrExecutionBlocked {
		t.Fatalf("isolated cross-environment rehearsal = %v", err)
	}
	plan.Rehearsals[0].Scope.EnvironmentKind = "policy_approved_ephemeral"
	plan.Rehearsals[0].Scope.PolicyApproval = in.EnvironmentPolicy
	x, err := s.CreateExecution(plan, "operator", in)
	if err != nil || x.Status != "running" {
		t.Fatalf("policy-authorized equivalence = %#v, %v", x, err)
	}
}

func TestExecutionAssessmentNeverClaimsConvergenceForPartialOrFailedEvidence(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	resource := Resource{ID: "service", EnvironmentID: "prod", Commitments: Commitments{Security: []string{"least privilege"}, Privacy: []string{"regional data"}, Reliability: []string{"99.9 availability"}, Continuity: []string{"restore tested"}}, Constraints: []Constraint{{Kind: "cost", Limit: 10, Unit: "usd"}}}
	plan := ChangePlan{ID: "plan", RepositoryID: "repo", DefinitionID: "definition", DefinitionVersion: 2, AffectedOwnerIDs: []string{"owner"}, AcknowledgedOwnerIDs: []string{"owner"}, Rehearsals: []Rehearsal{{ID: "r", Scope: RehearsalScope{EnvironmentID: "prod"}, Runs: []RehearsalRun{{Result: "passed"}}}}, Changes: []PlanChange{{ResourceID: "service", Action: "change", After: &resource, Order: 1}}}
	x, err := s.CreateExecution(plan, "owner", ExecutionCreation{MergeCommitID: "merge", EnvironmentID: "prod", EnvironmentPolicy: "policy", RehearsalID: "r", BudgetUnits: 10, CredentialExpiry: now.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	x, err = s.ReportExecution(x.ID, "owner", "human", "step-service", x.Version, StepReport{Status: "failed", ProviderResponse: "apply partially completed", Health: "degraded", NextAction: "repair", SafetyPoint: true})
	if err != nil {
		t.Fatal(err)
	}
	x, err = s.AssessExecution(x.ID, "owner", x.Version, []ResourceOutcome{{ResourceID: "service", Present: true, ProviderRevision: "provider-2", Service: "failed", Security: "unknown", Privacy: "unknown", Cost: "passed", Continuity: "failed", MeasuresPassed: []string{"cost", "cost:10 usd"}, Summary: "partial provider state retained"}}, []string{"orphan-network"}, []string{"old-service"})
	if err != nil || x.Assessments[len(x.Assessments)-1].Converged {
		t.Fatalf("assessment = %#v, %v", x, err)
	}
}

func TestExecutionConvergenceDriftMonitoringAndGovernedResponse(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	resource := Resource{ID: "service", EnvironmentID: "prod"}
	plan := ChangePlan{ID: "plan", RepositoryID: "repo", DefinitionID: "definition", DefinitionVersion: 3, AffectedOwnerIDs: []string{"owner"}, AcknowledgedOwnerIDs: []string{"owner"}, Rehearsals: []Rehearsal{{ID: "r", Scope: RehearsalScope{EnvironmentID: "prod"}, Runs: []RehearsalRun{{Result: "passed"}}}}, Changes: []PlanChange{{ResourceID: "service", Action: "change", After: &resource, Order: 1}}}
	x, _ := s.CreateExecution(plan, "owner", ExecutionCreation{MergeCommitID: "merge", EnvironmentID: "prod", EnvironmentPolicy: "policy", RehearsalID: "r", BudgetUnits: 0, CredentialExpiry: now.Add(time.Hour)})
	x, _ = s.ReportExecution(x.ID, "owner", "human", "step-service", x.Version, StepReport{Status: "succeeded", ProviderResponse: "applied", Health: "healthy", NextAction: "verify", SafetyPoint: true})
	x, err := s.AssessExecution(x.ID, "owner", x.Version, []ResourceOutcome{{ResourceID: "service", Present: true, ProviderRevision: "provider-3", Service: "passed", Security: "passed", Privacy: "passed", Cost: "passed", Continuity: "passed", MeasuresPassed: []string{"service", "security", "privacy", "cost", "continuity"}, Summary: "all declared outcomes verified"}}, nil, nil)
	if err != nil || !x.Assessments[0].Converged {
		t.Fatalf("convergence = %#v, %v", x.Assessments, err)
	}
	x, err = s.MonitorExecution(x.ID, "owner", "partial", "available", []DriftFinding{{Kind: "configuration_drift", ResourceID: "service", Severity: "high", Summary: "provider configuration changed", Cause: "attributed external maintenance"}})
	if err != nil {
		t.Fatal(err)
	}
	finding := x.MonitorRuns[0].Findings[0]
	x, err = s.RespondToDrift(x.ID, "owner", x.Version, DriftResponse{FindingID: finding.ID, Kind: "adopt", OwnerID: "owner", ResourceKind: "pull_request", ResourceID: "pull-42", Summary: "review the legitimate provider change through ordinary policy"})
	if err != nil || len(x.DriftResponses) != 1 {
		t.Fatalf("response = %#v, %v", x, err)
	}
	if _, err = s.MonitorExecution(x.ID, "owner", "denied", "unknown", []DriftFinding{{Kind: "provider_loss", Severity: "critical", Summary: "invented finding"}}); err != ErrInvalid {
		t.Fatalf("permission bypass = %v", err)
	}
}
