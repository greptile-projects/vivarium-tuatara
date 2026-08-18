package durableschemas

import (
	"testing"
	"time"
)

func TestReviewedSchemasAndMigrationHistory(t *testing.T) {
	s, _ := New(t.TempDir())
	r := Revision{Name: "orders", StoreKind: "database", Description: "Order state", Definition: "orders(id uuid primary key)", DefinitionPath: "db/orders.sql", OwnerIDs: []string{"owner"}, Compatibility: []string{"additive changes remain backward compatible"}, Retention: "seven years", Privacy: []string{"customer identifiers encrypted"}, Links: []Link{{Kind: "service", ID: "checkout", Label: "Checkout"}}, PullRequestID: "42", ReviewedCommit: "abc", Rationale: "Reviewed baseline"}
	v, e := s.Create("repo", "owner", r)
	if e != nil || v.CurrentVersion != 1 {
		t.Fatalf("create = %#v, %v", v, e)
	}
	r.Definition += "\nstatus text"
	r.Rationale = "Add status"
	v, e = s.Revise("repo", v.ID, 1, "owner", r)
	if e != nil || v.CurrentVersion != 2 {
		t.Fatalf("revise = %#v, %v", v, e)
	}
	m := Migration{FromVersion: 1, ToVersion: 2, SourceKind: "pull_request", SourceID: "43", Summary: "Backfill status before enforcing writes", Operations: []Operation{{ID: "read-old", Kind: "read", Description: "Read legacy rows", OwnerIDs: []string{"owner"}, ConsumerIDs: []string{"checkout"}, RollbackLimit: "Safe until new writes"}, {ID: "fill", Kind: "backfill", Description: "Populate status", OwnerIDs: []string{"owner"}, ConsumerIDs: []string{"checkout"}, RollbackLimit: "Rows can be restored before cutover"}}, Steps: []Step{{ID: "observe", OperationIDs: []string{"read-old"}, Description: "Measure legacy reads", SuccessMeasures: []string{"zero unknown rows"}, RequiredApproverIDs: []string{"owner"}}, {ID: "backfill", OperationIDs: []string{"fill"}, Description: "Populate in batches", SuccessMeasures: []string{"100% populated"}, RequiredApproverIDs: []string{"owner"}}}, RollbackLimits: []string{"No rollback after old column removal"}}
	v, e = s.AddMigration("repo", v.ID, "owner", m)
	if e != nil || len(v.Migrations) != 1 {
		t.Fatalf("migration = %#v, %v", v, e)
	}
	if _, e = s.AddEvent("repo", v.ID, v.Migrations[0].ID, "reviewer", 1, Event{Kind: "approved", StepID: "observe", Summary: "Not authorized"}); e != ErrInvalid {
		t.Fatalf("non-approver event = %v", e)
	}
	v, e = s.AddEvent("repo", v.ID, v.Migrations[0].ID, "owner", 1, Event{Kind: "approved", StepID: "observe", Summary: "Storage owner approved sequence"})
	if e != nil || v.Migrations[0].Version != 2 || len(v.Migrations[0].Events) != 2 {
		t.Fatalf("event = %#v, %v", v, e)
	}
	if _, e = s.AddEvent("repo", v.ID, v.Migrations[0].ID, "reviewer", 1, Event{Kind: "approved", Summary: "retry"}); e != ErrConflict {
		t.Fatalf("stale event = %v", e)
	}
}

func TestMigrationRejectsUnsequencedOperation(t *testing.T) {
	schema := Schema{CurrentVersion: 2}
	m := Migration{FromVersion: 1, ToVersion: 2, SourceKind: "pull_request", SourceID: "1", Summary: "mixed", Operations: []Operation{{ID: "read", Kind: "read", Description: "read", OwnerIDs: []string{"o"}, ConsumerIDs: []string{"c"}, RollbackLimit: "safe"}, {ID: "write", Kind: "write", Description: "write", OwnerIDs: []string{"o"}, ConsumerIDs: []string{"c"}, RollbackLimit: "safe"}}, Steps: []Step{{ID: "read", OperationIDs: []string{"read"}, Description: "read", SuccessMeasures: []string{"done"}, RequiredApproverIDs: []string{"o"}}}, RollbackLimits: []string{"safe"}}
	if validateMigration(schema, m) != ErrInvalid {
		t.Fatal("unsequenced operation accepted")
	}
}

func TestMigrationRequiresExplicitIrreversibility(t *testing.T) {
	schema := Schema{CurrentVersion: 2}
	m := Migration{FromVersion: 1, ToVersion: 2, SourceKind: "pull_request", SourceID: "1", Summary: "drop", Operations: []Operation{{ID: "drop", Kind: "destructive", Description: "drop column", OwnerIDs: []string{"o"}, ConsumerIDs: []string{"c"}, RollbackLimit: "restore backup"}}, Steps: []Step{{ID: "drop", OperationIDs: []string{"drop"}, Description: "drop", SuccessMeasures: []string{"reads pass"}, RequiredApproverIDs: []string{"o"}}}, RollbackLimits: []string{"backup expires in one day"}}
	if validateMigration(schema, m) != ErrInvalid {
		t.Fatal("destructive operation without explicit flag accepted")
	}
}

func TestCriticalFailuresRejectGenericRetry(t *testing.T) {
	for _, kind := range []string{"service_regression", "capacity_exhaustion", "conflicting_writes"} {
		failure := FailureEvidence{Kind: kind}
		if recoveryValidForFailure(failure, "retry", "", "", "", "") {
			t.Fatalf("generic retry accepted for %s", kind)
		}
		if !recoveryValidForFailure(failure, "retry", "", "remediation attested", "", "") {
			t.Fatalf("attested retry rejected for %s", kind)
		}
	}
	for _, kind := range []string{"failed_invariant", "interrupted_backfill"} {
		if !recoveryValidForFailure(FailureEvidence{Kind: kind}, "retry", "", "", "", "") {
			t.Fatalf("idempotent retry rejected for %s", kind)
		}
	}
}

func TestBoundedRehearsalRetainsExactSanitizedEvidence(t *testing.T) {
	s, _ := New(t.TempDir())
	r := Revision{Name: "orders", StoreKind: "database", Description: "orders", Definition: "v1", DefinitionPath: "db.sql", OwnerIDs: []string{"owner"}, Compatibility: []string{"dual"}, Retention: "year", Privacy: []string{"tokenized"}, PullRequestID: "1", ReviewedCommit: "abc", Rationale: "baseline"}
	v, _ := s.Create("repo", "owner", r)
	r.Definition = "v2"
	r.Rationale = "candidate"
	v, _ = s.Revise("repo", v.ID, 1, "owner", r)
	m := Migration{FromVersion: 1, ToVersion: 2, SourceKind: "pull_request", SourceID: "2", Summary: "upgrade", Operations: []Operation{{ID: "upgrade", Kind: "write", Description: "upgrade", OwnerIDs: []string{"owner"}, ConsumerIDs: []string{"app"}, RollbackLimit: "before cutover"}}, Steps: []Step{{ID: "prove", OperationIDs: []string{"upgrade"}, Description: "rehearse", SuccessMeasures: []string{"meaning preserved"}, RequiredApproverIDs: []string{"owner"}}}, RollbackLimits: []string{"before cutover"}}
	v, _ = s.AddMigration("repo", v.ID, "owner", m)
	migration := v.Migrations[0]
	rehearsal := Rehearsal{Name: "realistic tokenized rehearsal", ApplicationRevision: "commit", Dataset: RehearsalDataset{Kind: "representative", Description: "tokenized distribution", PrivacyMethod: "irreversible tokenization", Digest: "data-digest", MaxBytes: 1024, RowCount: 10}, Dependencies: []RehearsalDependency{{Name: "driver", Revision: "1.2.3", Digest: "dep-digest"}}, Checks: []RehearsalCheck{{ID: "upgrade", Kind: "upgrade", Command: "./check upgrade", Invariant: "balances unchanged", InvariantCommand: "./check balances", RevisionInputs: []string{"application", "schema_from", "schema_to", "migration", "data_shape", "dependency:driver"}}, {ID: "rollback", Kind: "rollback", Command: "./check rollback", Invariant: "old reader succeeds", InvariantCommand: "./check old-reader", RevisionInputs: []string{"application", "migration"}}}}
	v, rehearsal, err := s.CreateRehearsal("repo", v.ID, migration.ID, "owner", migration.Version, rehearsal)
	if err != nil || len(v.Migrations[0].Rehearsals) != 1 {
		t.Fatalf("create rehearsal = %#v, %v", rehearsal, err)
	}
	run := RehearsalRun{WorkspaceID: "workspace", Result: "failed", Attestations: []string{"reviewer observed isolated execution"}, Outcomes: []RehearsalOutcome{{CheckID: "upgrade", Status: "passed", InvariantPassed: true, SanitizedLog: "10 rows upgraded", RowsBefore: 10, RowsAfter: 10, ArtifactDigests: []string{"artifact"}, CostUnits: 2}, {CheckID: "rollback", Status: "failed", ExitCode: 1, SanitizedLog: "old reader rejected new value", RowsBefore: 10, RowsAfter: 10}}}
	v, run, err = s.AddRehearsalRun("repo", v.ID, migration.ID, rehearsal.ID, "agent", run)
	if err != nil || run.ID == "" {
		t.Fatalf("run = %#v, %v", run, err)
	}
	_, note, err := s.AddRehearsalNote("repo", v.ID, migration.ID, rehearsal.ID, "owner", run.ID, "Rollback failed only for enum values; investigate decoder.")
	if err != nil || note.ActorID != "owner" {
		t.Fatalf("note = %#v, %v", note, err)
	}
	bad := run
	bad.ID = ""
	bad.Result = "passed"
	if _, _, err = s.AddRehearsalRun("repo", v.ID, migration.ID, rehearsal.ID, "agent", bad); err != ErrInvalid {
		t.Fatalf("dishonest passing summary accepted: %v", err)
	}
}

func TestProductionExecutionRequiresEvidenceAndKeepsAgentsDelegated(t *testing.T) {
	s, _ := New(t.TempDir())
	r := Revision{Name: "orders", StoreKind: "database", Description: "orders", Definition: "v1", DefinitionPath: "db.sql", OwnerIDs: []string{"owner"}, Compatibility: []string{"dual read"}, Retention: "year", Privacy: []string{"tokenized"}, PullRequestID: "1", ReviewedCommit: "abc", Rationale: "baseline"}
	v, _ := s.Create("repo", "owner", r)
	r.Definition, r.Rationale = "v2", "candidate"
	v, _ = s.Revise("repo", v.ID, 1, "owner", r)
	m := Migration{FromVersion: 1, ToVersion: 2, SourceKind: "pull_request", SourceID: "2", Summary: "expand then contract", Operations: []Operation{{ID: "change", Kind: "write", Description: "dual write", OwnerIDs: []string{"owner"}, ConsumerIDs: []string{"app"}, RollbackLimit: "before contract"}}, Steps: []Step{{ID: "change", OperationIDs: []string{"change"}, Description: "change", SuccessMeasures: []string{"healthy"}, RequiredApproverIDs: []string{"owner"}}}, RollbackLimits: []string{"before contract"}}
	v, _ = s.AddMigration("repo", v.ID, "owner", m)
	migration := v.Migrations[0]
	rehearsal := Rehearsal{Name: "proof", ApplicationRevision: "commit", Dataset: RehearsalDataset{Kind: "synthetic", Description: "shape", Digest: "digest", MaxBytes: 10}, Checks: []RehearsalCheck{{ID: "upgrade", Kind: "upgrade", Command: "./up", Invariant: "safe", InvariantCommand: "./verify", RevisionInputs: []string{"application"}}}}
	v, rehearsal, _ = s.CreateRehearsal("repo", v.ID, migration.ID, "owner", migration.Version, rehearsal)
	base := Execution{EnvironmentID: "prod", ReleaseID: "release", DeploymentID: "caller-forged-deployment", RehearsalID: rehearsal.ID, CompatibilityWindow: "old and new readers through contract", ObservationPeriodSeconds: 3600, PrivacyConstraints: []string{"aggregate metrics only"}, CostBudgetUnits: 100, AbortReversibleUntil: "before contract", Delegations: []ExecutionDelegation{{Phase: "backfill", AgentID: "agent", StepID: "change", Mandate: "report bounded batch progress"}}}
	if _, _, err := s.CreateExecution("repo", v.ID, migration.ID, "owner", v.Migrations[0].Version, base); err != ErrInvalid {
		t.Fatalf("execution without approvals or passing proof = %v", err)
	}
	run := RehearsalRun{WorkspaceID: "ws", Result: "passed", Attestations: []string{"exact outcomes"}, Outcomes: []RehearsalOutcome{{CheckID: "upgrade", Status: "passed", InvariantPassed: true}}}
	v, _, _ = s.AddRehearsalRun("repo", v.ID, migration.ID, rehearsal.ID, "owner", run)
	v, _ = s.AddEvent("repo", v.ID, migration.ID, "owner", v.Migrations[0].Version, Event{Kind: "approved", StepID: "change", Summary: "approved current evidence"})
	v, execution, err := s.CreateExecution("repo", v.ID, migration.ID, "operator", v.Migrations[0].Version, base)
	if err != nil || execution.Phases[0].Name != "expand" || execution.ControllerID != "operator" || execution.DeploymentID != "" {
		t.Fatalf("execution = %#v, %v", execution, err)
	}
	_, execution, _ = s.UpdateExecution("repo", v.ID, migration.ID, execution.ID, "operator", ExecutionUpdate{Action: "start", ExpectedVersion: execution.Version})
	if _, _, err = s.UpdateExecution("repo", v.ID, migration.ID, execution.ID, "operator", ExecutionUpdate{Action: "report", ExpectedVersion: execution.Version, Phase: "expand", ProgressPercent: 25, ServiceHealth: "healthy", Invariants: []string{"dual reads agree"}, AgentID: "agent"}); err != ErrInvalid {
		t.Fatalf("agent escaped phase delegation: %v", err)
	}
	_, execution, err = s.UpdateExecution("repo", v.ID, migration.ID, execution.ID, "operator", ExecutionUpdate{Action: "report", ExpectedVersion: execution.Version, Phase: "expand", ProgressPercent: 100, ServiceHealth: "healthy", Invariants: []string{"dual reads agree"}, NextActions: []string{"advance"}, CostUnits: 5})
	if err != nil {
		t.Fatal(err)
	}
	v, execution, err = s.UpdateExecution("repo", v.ID, migration.ID, execution.ID, "operator", ExecutionUpdate{Action: "advance", ExpectedVersion: execution.Version})
	if err != nil || execution.CurrentPhase != 1 || execution.Phases[0].State != "completed" {
		t.Fatalf("advance = %#v, %v", execution, err)
	}
	// Put the retained execution at its delegated phase without manufacturing
	// controller evidence for the intervening deployment phase.
	retained := &v.Migrations[0].Executions[0]
	retained.CurrentPhase, retained.Status = 2, "running"
	retained.Phases[2].State = "running"
	if err = s.write(v); err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.UpdateExecution("repo", v.ID, migration.ID, execution.ID, "operator", ExecutionUpdate{Action: "report", ExpectedVersion: retained.Version, Phase: "backfill", StepID: "other", ProgressPercent: 100, ServiceHealth: "healthy", Invariants: []string{"done"}, Summary: "wrong step", AgentID: "agent"}); err != ErrInvalid {
		t.Fatalf("agent escaped step delegation: %v", err)
	}
	_, execution, err = s.UpdateExecution("repo", v.ID, migration.ID, execution.ID, "operator", ExecutionUpdate{Action: "report", ExpectedVersion: retained.Version, Phase: "backfill", StepID: "change", ProgressPercent: 50, ServiceHealth: "degraded", Invariants: []string{"completed batches valid"}, Blockers: []string{"replica lag"}, Summary: "assigned step blocked", AgentID: "agent"})
	if err != nil || len(execution.StepReports) != 1 || execution.Phases[2].ProgressPercent != 0 || execution.Phases[2].ServiceHealth != "" {
		t.Fatalf("scoped agent report changed phase readiness: %#v, %v", execution, err)
	}
	_, execution, err = s.UpdateExecution("repo", v.ID, migration.ID, execution.ID, "operator", ExecutionUpdate{Action: "report", ExpectedVersion: execution.Version, Phase: "backfill", ProgressPercent: 100, ServiceHealth: "healthy", Invariants: []string{"aggregate rows match"}, NextActions: []string{"advance"}, Summary: "controller phase assessment"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.UpdateExecution("repo", v.ID, migration.ID, execution.ID, "operator", ExecutionUpdate{Action: "advance", ExpectedVersion: execution.Version}); err != ErrInvalid {
		t.Fatalf("blocked delegated step did not gate phase: %v", err)
	}
	_, execution, err = s.UpdateExecution("repo", v.ID, migration.ID, execution.ID, "operator", ExecutionUpdate{Action: "report", ExpectedVersion: execution.Version, Phase: "backfill", StepID: "change", ProgressPercent: 100, ServiceHealth: "healthy", Invariants: []string{"assigned batch valid"}, Summary: "assigned step complete", AgentID: "agent"})
	if err != nil {
		t.Fatal(err)
	}
	_, execution, err = s.UpdateExecution("repo", v.ID, migration.ID, execution.ID, "operator", ExecutionUpdate{Action: "advance", ExpectedVersion: execution.Version})
	if err != nil || execution.CurrentPhase != 3 {
		t.Fatalf("satisfied delegated step did not unblock phase: %#v, %v", execution, err)
	}
	_, execution, err = s.UpdateExecution("repo", v.ID, migration.ID, execution.ID, "operator", ExecutionUpdate{Action: "start", ExpectedVersion: execution.Version})
	if err != nil {
		t.Fatal(err)
	}
	_, execution, err = s.UpdateExecution("repo", v.ID, migration.ID, execution.ID, "operator", ExecutionUpdate{Action: "report", ExpectedVersion: execution.Version, Phase: "cutover", ProgressPercent: 55, ServiceHealth: "degraded", Blockers: []string{"write collision"}, Summary: "new and old writers diverged", FailureKind: "conflicting_writes", SafetyPoint: "stop before routing the next shard", FailureEvidence: []string{"aggregate mismatch digest"}})
	if err != nil || execution.Status != "paused" || len(execution.Failures) != 1 {
		t.Fatalf("failure did not pause safely: %#v, %v", execution, err)
	}
	failure := execution.Failures[0]
	if _, _, err = s.UpdateExecution("repo", v.ID, migration.ID, execution.ID, "operator", ExecutionUpdate{Action: "resume", ExpectedVersion: execution.Version}); err != ErrInvalid {
		t.Fatalf("failure pause resumed without recovery: %v", err)
	}
	if _, _, _, err = s.RecoverExecution("repo", v.ID, migration.ID, execution.ID, "operator", RecoveryRequest{ExpectedVersion: execution.Version, IdempotencyKey: "unsafe-retry", Kind: "retry", FailureID: failure.ID, Summary: "retry without remediation", Evidence: []string{"same conditions"}}); err != ErrInvalid {
		t.Fatalf("generic retry cleared conflicting writers: %v", err)
	}
	v, execution, recovery, err := s.RecoverExecution("repo", v.ID, migration.ID, execution.ID, "operator", RecoveryRequest{ExpectedVersion: execution.Version, IdempotencyKey: "retry-cutover-1", Kind: "retry", FailureID: failure.ID, Summary: "retry the idempotent shard after writer fencing", Evidence: []string{"writer fence confirmed"}, RecoveryAttestation: "writer lease proves the obsolete writer is fenced"})
	if err != nil || recovery.Kind != "retry" {
		t.Fatalf("recovery = %#v, %v", recovery, err)
	}
	_, _, same, err := s.RecoverExecution("repo", v.ID, migration.ID, execution.ID, "operator", RecoveryRequest{ExpectedVersion: 0, IdempotencyKey: "retry-cutover-1", Kind: "retry", FailureID: failure.ID, Summary: "retry the idempotent shard after writer fencing", Evidence: []string{"writer fence confirmed"}, RecoveryAttestation: "writer lease proves the obsolete writer is fenced"})
	if err != nil || same.ID != recovery.ID {
		t.Fatalf("idempotent recovery retry = %#v, %v", same, err)
	}
	v, execution, err = s.UpdateExecution("repo", v.ID, migration.ID, execution.ID, "operator", ExecutionUpdate{Action: "resume", ExpectedVersion: execution.Version})
	if err != nil || execution.Status != "running" {
		t.Fatalf("recovered execution did not resume: %#v, %v", execution, err)
	}
	v, execution, err = s.UpdateExecution("repo", v.ID, migration.ID, execution.ID, "operator", ExecutionUpdate{Action: "pause", ExpectedVersion: execution.Version, Summary: "hold for approval review"})
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.AddEvent("repo", v.ID, migration.ID, "owner", v.Migrations[0].Version, Event{Kind: "approval_revoked", StepID: "change", Summary: "service regression invalidated the approval"})
	if err != nil || len(v.Migrations[0].Executions[0].Failures) != 2 || v.Migrations[0].Executions[0].Failures[1].Kind != "revoked_approval" {
		t.Fatalf("revocation evidence = %#v, %v", v.Migrations[0].Executions[0].Failures, err)
	}
	retained = &v.Migrations[0].Executions[0]
	retained.Status = "completed"
	retained.CurrentPhase = len(retained.Phases) - 1
	retained.DeploymentID = "successful-prod-deployment"
	completedAt := time.Now().UTC().Add(-3 * time.Hour)
	retained.Phases[retained.CurrentPhase].State = "completed"
	retained.Phases[retained.CurrentPhase].CompletedAt = &completedAt
	if err = s.write(v); err != nil {
		t.Fatal(err)
	}
	v, err = s.ApproveRetirement("repo", v.ID, migration.ID, "owner", v.Migrations[0].Version, "observation stayed healthy; approve cleanup")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	completionInput := RetirementCompletion{ObservationStartedAt: now.Add(-2 * time.Hour), ObservationEndedAt: now.Add(-time.Hour), CompatibilityRemoved: []string{"dual writer"}, ObsoleteFields: []string{"orders.legacy_status"}, IrreversibleDecisions: []string{"legacy field physically deleted"}, Environments: []EnvironmentCompletion{{EnvironmentID: "prod", CurrentVersion: 2, RetainedData: []string{"10 order rows"}, ChangedData: []string{"10 status values normalized"}, VerifiedDeletion: []string{"legacy column absent digest"}, Exceptions: []string{"none"}, CostUnits: 12}}}
	predated := completionInput
	predated.ObservationStartedAt, predated.ObservationEndedAt = now.Add(-5*time.Hour), now.Add(-4*time.Hour)
	if _, _, err = s.CompleteRetirement("repo", v.ID, migration.ID, "owner", v.Migrations[0].Version, predated); err != ErrInvalid {
		t.Fatalf("pre-success observation accepted: %v", err)
	}
	equalityBoundary := completionInput
	equalityBoundary.ObservationStartedAt = completedAt
	equalityBoundary.ObservationEndedAt = completedAt.Add(time.Hour)
	if _, _, err = s.CompleteRetirement("repo", v.ID, migration.ID, "owner", v.Migrations[0].Version, equalityBoundary); err != ErrInvalid {
		t.Fatalf("observation starting exactly at success accepted: %v", err)
	}
	wrongEnvironment := completionInput
	wrongEnvironment.Environments = append([]EnvironmentCompletion{}, completionInput.Environments...)
	wrongEnvironment.Environments[0].EnvironmentID = "never-run"
	if _, _, err = s.CompleteRetirement("repo", v.ID, migration.ID, "owner", v.Migrations[0].Version, wrongEnvironment); err != ErrInvalid {
		t.Fatalf("unexecuted environment accepted: %v", err)
	}
	_, completion, err := s.CompleteRetirement("repo", v.ID, migration.ID, "owner", v.Migrations[0].Version, RetirementCompletion{ObservationStartedAt: now.Add(-2 * time.Hour), ObservationEndedAt: now.Add(-time.Hour), CompatibilityRemoved: []string{"dual writer"}, ObsoleteFields: []string{"orders.legacy_status"}, IrreversibleDecisions: []string{"legacy field physically deleted"}, Environments: []EnvironmentCompletion{{EnvironmentID: "prod", CurrentVersion: 2, RetainedData: []string{"10 order rows"}, ChangedData: []string{"10 status values normalized"}, VerifiedDeletion: []string{"legacy column absent digest"}, Exceptions: []string{"none"}, CostUnits: 12}}})
	if err != nil || len(completion.ApprovedBy) != 1 {
		t.Fatalf("completion = %#v, %v", completion, err)
	}
}
