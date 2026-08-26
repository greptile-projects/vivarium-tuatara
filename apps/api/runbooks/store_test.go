package runbooks

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func validRevision(owner string) Revision {
	return Revision{Title: "Checkout recovery", Purpose: "Diagnose before changing state", Scope: Scope{Kind: "service", ResourceID: "checkout", Name: "Checkout"}, Preconditions: []string{"Confirm signal"}, RollbackCriteria: []string{"Stop on increased impact"}, OutcomeCriteria: []OutcomeCriterion{{Kind: "health", Criterion: "Health is stable"}, {Kind: "containment", Criterion: "Impact is contained"}, {Kind: "recovery", Criterion: "Service recovered"}, {Kind: "communication", Criterion: "Audience updated"}, {Kind: "rollback", Criterion: "Rollback is complete or unnecessary"}}, OwnerIDs: []string{owner}, RequiredSkills: []string{"operations"}, Escalations: []Escalation{{Condition: "blocked", OwnerID: owner, Path: "owner", ExpectedAction: "decide"}}, ChangeReason: "initial", Steps: []Step{{ID: "inspect", Position: 1, Kind: "diagnostic", Title: "Inspect", Purpose: "Test hypothesis", Instructions: "Use reviewed workflow", Preconditions: []string{"read access"}, ExpectedEvidence: []string{"health digest"}, OwnerIDs: []string{owner}, RequiredSkills: []string{"operations"}, References: []Reference{{Kind: "command", ResourceID: "health-check", Revision: "abc", Reviewed: true, Accessible: true}}, Authority: Authority{RequiredAccess: []string{"service:read"}, Inspects: []string{"health"}, ProhibitedActions: []string{"deploy"}}}}}
}

func TestRehearsalRetainsBoundedEvidenceAndBecomesStale(t *testing.T) {
	s, _ := New(t.TempDir())
	r := validRevision("owner")
	r.Scope.Revision = "service-v1"
	created, err := s.Create("repo", "owner", "create", r)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	scenario := Scenario{ID: "latency", Name: "Elevated latency", Failure: "health check is slow", Inputs: []RehearsalInput{{Kind: "service", ResourceID: "checkout", Revision: "service-v1", EvidenceKind: "synthetic", Digest: "sha256:input"}}, Steps: []StepOutcome{{StepID: "inspect", Output: "healthy synthetic response", StartedAt: now, FinishedAt: now.Add(time.Second), Artifacts: []Artifact{{Name: "health.json", Digest: "sha256:artifact", MediaType: "application/json"}}, CostCents: 2, Permissions: []string{"service:read"}, Outcome: "passed", DestructiveHandling: "not_applicable"}}, AchievedOutcome: "achieved"}
	rehearsed, err := s.Rehearse(created.ID, "human", "owner", "rehearse", 1, "isolated", "sandbox-1", "", []Scenario{scenario})
	if err != nil || len(rehearsed.Rehearsals) != 1 || rehearsed.Rehearsals[0].Status != "passed" || rehearsed.Rehearsals[0].Stale {
		t.Fatalf("rehearsed=%+v err=%v", rehearsed, err)
	}
	retry, err := s.Rehearse(created.ID, "human", "owner", "rehearse", 1, "isolated", "sandbox-1", "", []Scenario{scenario})
	if err != nil || retry.Rehearsals[0].ID != rehearsed.Rehearsals[0].ID {
		t.Fatalf("retry=%+v err=%v", retry, err)
	}
	r.ChangeReason = "clarify recovery"
	revised, err := s.Revise(created.ID, 1, "owner", "revise", r)
	if err != nil || !revised.Rehearsals[0].Stale || revised.Rehearsals[0].StaleReasons[0] != "runbook_steps_changed" {
		t.Fatalf("revised=%+v err=%v", revised, err)
	}
}

func TestRecommendAndStartExecutionRetainExactContextAndSafetyChoices(t *testing.T) {
	s, _ := New(t.TempDir())
	created, err := s.Create("repo", "owner", "create", validRevision("owner"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	scenario := Scenario{ID: "live-shape", Name: "Degraded checkout", Failure: "latency", Inputs: []RehearsalInput{{Kind: "service", ResourceID: "checkout", Revision: "v1", EvidenceKind: "synthetic", Digest: "sha256:input"}}, Steps: []StepOutcome{{StepID: "inspect", Output: "bounded output", StartedAt: now, FinishedAt: now, Artifacts: []Artifact{}, CostCents: 0, Permissions: []string{"service:read"}, Outcome: "passed", DestructiveHandling: "not_applicable"}}, AchievedOutcome: "achieved"}
	if _, err = s.Rehearse(created.ID, "human", "owner", "rehearse", 1, "isolated", "sandbox", "", []Scenario{scenario}); err != nil {
		t.Fatal(err)
	}
	context := ExecutionContext{OriginKind: "alert", OriginID: "alert-1", OriginRevision: "alert-v3", Summary: "checkout latency", AffectedResources: []string{"checkout"}, SignalClass: "reliability", WindowFrom: now.Add(-time.Minute), WindowTo: now, ReleaseIDs: []string{"release-7"}, EnvironmentID: "production", EnvironmentRevision: "snapshot-2", Evidence: []ExecutionEvidence{{Kind: "metric", ResourceID: "checkout", Revision: "metric-v1", Digest: "sha256:evidence", Summary: "bounded latency evidence"}}, TimelineRefs: []string{"alert-1:event-3"}, AudienceIDs: []string{"owner"}}
	recommendations, err := s.Recommend("repo", context)
	if err != nil || len(recommendations) != 1 || !recommendations[0].Eligible || recommendations[0].Score != 100 {
		t.Fatalf("recommendations=%+v err=%v", recommendations, err)
	}
	execution, err := s.StartExecution(created.ID, "owner", "launch", 1, context, []Preconditions{{Condition: "Confirm signal", Status: "met", EvidenceDigest: "sha256:evidence"}}, []string{"service:read"})
	if err != nil || execution.Status != "ready" || execution.Context.OriginRevision != "alert-v3" || len(execution.Blockers) != 0 {
		t.Fatalf("execution=%+v err=%v", execution, err)
	}
	retry, err := s.StartExecution(created.ID, "owner", "launch", 1, context, []Preconditions{{Condition: "Confirm signal", Status: "met", EvidenceDigest: "sha256:evidence"}}, []string{"service:read"})
	if err != nil || retry.ID != execution.ID {
		t.Fatalf("retry=%+v err=%v", retry, err)
	}
	if _, err = s.StartExecution(created.ID, "owner", "duplicate", 1, context, []Preconditions{{Condition: "Confirm signal", Status: "met"}}, []string{"service:read"}); err != ErrConflict {
		t.Fatalf("duplicate err=%v", err)
	}
}

func TestExecutionRetainsExplicitBlockersInsteadOfStartingUnsafeWork(t *testing.T) {
	s, _ := New(t.TempDir())
	created, _ := s.Create("repo", "owner", "create", validRevision("owner"))
	now := time.Now().UTC()
	context := ExecutionContext{OriginKind: "manual_observation", OriginID: "observation-1", OriginRevision: "v1", Summary: "checkout failure", AffectedResources: []string{"checkout"}, WindowFrom: now, WindowTo: now, Evidence: []ExecutionEvidence{{Kind: "observation", ResourceID: "checkout", Revision: "v1", Digest: "sha256:x", Summary: "sanitized"}}}
	execution, err := s.StartExecution(created.ID, "owner", "launch", 1, context, []Preconditions{{Condition: "Confirm signal", Status: "unknown"}}, nil)
	if err != nil || execution.Status != "blocked" {
		t.Fatalf("execution=%+v err=%v", execution, err)
	}
	kinds := map[string]bool{}
	for _, blocker := range execution.Blockers {
		kinds[blocker.Kind] = true
	}
	if !kinds["unverified_procedure"] || !kinds["precondition_not_met"] || !kinds["access_unavailable"] {
		t.Fatalf("blockers=%+v", execution.Blockers)
	}
	// A new caller-stable attempt may preserve the blocked audit record and
	// re-evaluate corrected conditions instead of treating the block as active.
	scenario := Scenario{ID: "corrected", Name: "Corrected", Failure: "checkout failure", Inputs: []RehearsalInput{{Kind: "service", ResourceID: "checkout", Revision: "v1", EvidenceKind: "synthetic", Digest: "sha256:x"}}, Steps: []StepOutcome{{StepID: "inspect", Output: "bounded", StartedAt: now, FinishedAt: now, Permissions: []string{"service:read"}, Outcome: "passed", DestructiveHandling: "not_applicable"}}, AchievedOutcome: "achieved"}
	if _, err = s.Rehearse(created.ID, "human", "owner", "rehearse", 1, "isolated", "sandbox", "", []Scenario{scenario}); err != nil {
		t.Fatal(err)
	}
	corrected, err := s.StartExecution(created.ID, "owner", "corrected-launch", 1, context, []Preconditions{{Condition: "Confirm signal", Status: "met", EvidenceDigest: "sha256:x"}}, []string{"service:read"})
	if err != nil || corrected.Status != "ready" || corrected.ID == execution.ID {
		t.Fatalf("corrected=%+v err=%v", corrected, err)
	}
	retained, _ := s.Get(created.ID)
	if len(retained.Executions) != 2 || retained.Executions[0].Status != "blocked" {
		t.Fatalf("retained executions=%+v", retained.Executions)
	}
}

func TestExecutionActionsAreVersionBoundDelegatedAndReceipted(t *testing.T) {
	s, _ := New(t.TempDir())
	r := validRevision("owner")
	r.Steps[0].Authority.HumanApprovalRequired = true
	created, _ := s.Create("repo", "owner", "create", r)
	now := time.Now().UTC()
	context := ExecutionContext{OriginKind: "alert", OriginID: "alert-actions", OriginRevision: "v1", Summary: "checkout failure", AffectedResources: []string{"checkout"}, WindowFrom: now, WindowTo: now, Evidence: []ExecutionEvidence{{Kind: "metric", ResourceID: "checkout", Revision: "v1", Digest: "sha256:x", Summary: "sanitized"}}}
	execution, err := s.StartExecution(created.ID, "owner", "launch", 1, context, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Blocked launch context cannot be converted into operational authority.
	if _, err = s.Act(created.ID, execution.ID, "human", "owner", ExecutionAction{RequestID: "unsafe", ExpectedVersion: execution.Version, Action: "perform", StepID: "inspect"}); err != ErrConflict {
		t.Fatalf("blocked perform err=%v", err)
	}
	joined, err := s.Act(created.ID, execution.ID, "human", "reviewer", ExecutionAction{RequestID: "join", ExpectedVersion: execution.Version, Action: "join"})
	if err != nil || len(joined.Receipts) != 1 || joined.Version != 2 {
		t.Fatalf("joined=%+v err=%v", joined, err)
	}
	retry, err := s.Act(created.ID, execution.ID, "human", "reviewer", ExecutionAction{RequestID: "join", ExpectedVersion: execution.Version, Action: "join"})
	if err != nil || retry.Version != joined.Version || len(retry.Receipts) != 1 {
		t.Fatalf("retry=%+v err=%v", retry, err)
	}
	if _, err = s.Act(created.ID, execution.ID, "human", "reviewer", ExecutionAction{RequestID: "stale", ExpectedVersion: 1, Action: "discuss", Message: "observation"}); err != ErrConflict {
		t.Fatalf("stale err=%v", err)
	}
}

func TestReadyExecutionRequiresSeparateApprovalAndExactAgentDelegation(t *testing.T) {
	s, _ := New(t.TempDir())
	r := validRevision("owner")
	r.Steps[0].Authority.HumanApprovalRequired = true
	created, _ := s.Create("repo", "owner", "create", r)
	now := time.Now().UTC()
	scenario := Scenario{ID: "case", Name: "case", Failure: "failure", Inputs: []RehearsalInput{{Kind: "service", ResourceID: "checkout", Revision: "v1", EvidenceKind: "synthetic", Digest: "sha256:i"}}, Steps: []StepOutcome{{StepID: "inspect", Output: "ok", StartedAt: now, FinishedAt: now, Permissions: []string{"service:read"}, Outcome: "passed", DestructiveHandling: "not_applicable"}}, AchievedOutcome: "achieved"}
	if _, err := s.Rehearse(created.ID, "human", "owner", "rehearse", 1, "isolated", "box", "", []Scenario{scenario}); err != nil {
		t.Fatal(err)
	}
	context := ExecutionContext{OriginKind: "alert", OriginID: "alert-ready", OriginRevision: "v1", Summary: "failure", AffectedResources: []string{"checkout"}, WindowFrom: now, WindowTo: now, Evidence: []ExecutionEvidence{{Kind: "metric", ResourceID: "checkout", Revision: "v1", Digest: "sha256:x", Summary: "safe"}}}
	execution, err := s.StartExecution(created.ID, "owner", "launch", 1, context, []Preconditions{{Condition: "Confirm signal", Status: "met"}}, []string{"service:read"})
	if err != nil || execution.Status != "ready" {
		t.Fatalf("execution=%+v err=%v", execution, err)
	}
	delegated, err := s.Act(created.ID, execution.ID, "human", "owner", ExecutionAction{RequestID: "delegate", ExpectedVersion: 1, Action: "delegate", StepID: "inspect", TargetID: "agent-1", DelegatedActions: []string{"analyze", "perform"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Act(created.ID, execution.ID, "agent", "agent-2", ExecutionAction{RequestID: "wrong-agent", ExpectedVersion: 2, Action: "analyze", StepID: "inspect"}); err != ErrConflict {
		t.Fatalf("wrong agent err=%v", err)
	}
	approved, err := s.Act(created.ID, execution.ID, "human", "reviewer", ExecutionAction{RequestID: "approve", ExpectedVersion: delegated.Version, Action: "approve", StepID: "inspect", Message: "evidence supports bounded action"})
	if err != nil {
		t.Fatal(err)
	}
	completed, err := s.Act(created.ID, execution.ID, "agent", "agent-1", ExecutionAction{RequestID: "perform", ExpectedVersion: approved.Version, Action: "perform", StepID: "inspect", Evidence: []ExecutionEvidence{{Kind: "result", ResourceID: "checkout", Revision: "v1", Digest: "sha256:result", Summary: "healthy"}}, CostCents: 7})
	if err != nil || completed.Status != "completed" || completed.CostCents != 7 || len(completed.Receipts) != 3 {
		t.Fatalf("completed=%+v err=%v", completed, err)
	}
}

func TestExecutionRejectsInactiveDecisionBypass(t *testing.T) {
	s, _ := New(t.TempDir())
	r := validRevision("owner")
	r.Steps[0].Authority.HumanApprovalRequired = true
	r.Steps = append(r.Steps,
		Step{ID: "choose", Position: 2, Kind: "decision", Title: "Choose", Purpose: "Choose the bounded branch", Instructions: "Evaluate retained evidence", Preconditions: []string{"inspection complete"}, ExpectedEvidence: []string{"decision evidence"}, OwnerIDs: []string{"owner"}, RequiredSkills: []string{"operations"}, Decision: &Decision{Condition: "health restored", IfTrueStepID: "after", IfFalseStepID: "after", HumanJudgment: "Confirm health"}, Authority: Authority{}},
		Step{ID: "after", Position: 3, Kind: "communication", Title: "Communicate", Purpose: "Share status", Instructions: "Send the bounded update", Preconditions: []string{"decision complete"}, ExpectedEvidence: []string{"timeline update"}, OwnerIDs: []string{"owner"}, RequiredSkills: []string{"operations"}, Authority: Authority{}},
	)
	created, err := s.Create("repo", "owner", "create", r)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	context := ExecutionContext{OriginKind: "alert", OriginID: "alert-decision", OriginRevision: "v1", Summary: "checkout failure", AffectedResources: []string{"checkout"}, WindowFrom: now, WindowTo: now, Evidence: []ExecutionEvidence{{Kind: "metric", ResourceID: "checkout", Revision: "v1", Digest: "sha256:x", Summary: "sanitized"}}}
	execution, err := s.StartExecution(created.ID, "owner", "launch", 1, context, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Force only the unrelated launch-readiness concerns out of this state-machine
	// regression; the current cursor remains on the approval-gated inspect step.
	stored, _ := s.read(created.ID)
	stored.Executions[0].Status = "ready"
	stored.Executions[0].Blockers = nil
	if err = s.write(stored); err != nil {
		t.Fatal(err)
	}
	_, err = s.Act(created.ID, execution.ID, "human", "owner", ExecutionAction{RequestID: "inactive-decision", ExpectedVersion: 1, Action: "decide", StepID: "choose", Decision: "true"})
	if err != ErrConflict {
		t.Fatalf("inactive decision err=%v", err)
	}
	retained, _ := s.Get(created.ID)
	got := retained.Executions[0]
	if got.CurrentStepID != "inspect" || len(got.Receipts) != 0 {
		t.Fatalf("inactive decision mutated execution: %+v", got)
	}
}

func TestExecutionInitializesConsecutiveDecisionTarget(t *testing.T) {
	s, _ := New(t.TempDir())
	r := validRevision("owner")
	r.Steps = []Step{
		{ID: "first", Position: 1, Kind: "decision", Title: "First decision", Purpose: "Choose first branch", Instructions: "Evaluate first condition", Preconditions: []string{"signal confirmed"}, ExpectedEvidence: []string{"first decision"}, OwnerIDs: []string{"owner"}, RequiredSkills: []string{"operations"}, Decision: &Decision{Condition: "first condition", IfTrueStepID: "second", IfFalseStepID: "second", HumanJudgment: "Judge first condition"}, Authority: Authority{}},
		{ID: "second", Position: 2, Kind: "decision", Title: "Second decision", Purpose: "Choose second branch", Instructions: "Evaluate second condition", Preconditions: []string{"first decision complete"}, ExpectedEvidence: []string{"second decision"}, OwnerIDs: []string{"owner"}, RequiredSkills: []string{"operations"}, Decision: &Decision{Condition: "second condition", IfTrueStepID: "after", IfFalseStepID: "after", HumanJudgment: "Judge second condition"}, Authority: Authority{}},
		{ID: "after", Position: 3, Kind: "communication", Title: "Communicate", Purpose: "Share status", Instructions: "Send update", Preconditions: []string{"decisions complete"}, ExpectedEvidence: []string{"timeline update"}, OwnerIDs: []string{"owner"}, RequiredSkills: []string{"operations"}, Authority: Authority{}},
	}
	created, err := s.Create("repo", "owner", "create", r)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	context := ExecutionContext{OriginKind: "alert", OriginID: "alert-consecutive", OriginRevision: "v1", Summary: "checkout failure", AffectedResources: []string{"checkout"}, WindowFrom: now, WindowTo: now, Evidence: []ExecutionEvidence{{Kind: "metric", ResourceID: "checkout", Revision: "v1", Digest: "sha256:x", Summary: "sanitized"}}}
	execution, err := s.StartExecution(created.ID, "owner", "launch", 1, context, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	stored, _ := s.read(created.ID)
	stored.Executions[0].Status = "ready"
	stored.Executions[0].Blockers = nil
	if err = s.write(stored); err != nil {
		t.Fatal(err)
	}
	second, err := s.Act(created.ID, execution.ID, "human", "owner", ExecutionAction{RequestID: "first", ExpectedVersion: 1, Action: "decide", StepID: "first", Decision: "true"})
	if err != nil || second.CurrentStepID != "second" || len(second.PendingDecisions) != 1 || second.PendingDecisions[0] != "second condition" || second.PredictedNextAction != "record decision: second condition" {
		t.Fatalf("second=%+v err=%v", second, err)
	}
	after, err := s.Act(created.ID, execution.ID, "human", "owner", ExecutionAction{RequestID: "second", ExpectedVersion: 2, Action: "decide", StepID: "second", Decision: "false"})
	if err != nil || after.CurrentStepID != "after" || len(after.PendingDecisions) != 0 || after.PredictedNextAction != "perform Communicate" {
		t.Fatalf("after=%+v err=%v", after, err)
	}
}

func TestRehearsalRejectsUnsafeChangingStepAndSecretOutput(t *testing.T) {
	s, _ := New(t.TempDir())
	r := validRevision("owner")
	r.Steps[0].Authority.Changes = []string{"traffic"}
	r.Steps[0].Authority.HumanApprovalRequired = true
	created, _ := s.Create("repo", "owner", "create", r)
	now := time.Now().UTC()
	base := Scenario{ID: "failure", Name: "Failure", Failure: "unavailable", Inputs: []RehearsalInput{{Kind: "service", ResourceID: "checkout", Revision: "v1", EvidenceKind: "permitted", Digest: "sha256:x"}}, Steps: []StepOutcome{{StepID: "inspect", Output: "ok", StartedAt: now, FinishedAt: now, Permissions: []string{"service:read"}, Outcome: "passed"}}, AchievedOutcome: "achieved"}
	if _, err := s.Rehearse(created.ID, "agent", "agent", "unsafe", 1, "isolated", "box", "", []Scenario{base}); err != ErrInvalid {
		t.Fatalf("unsafe error=%v", err)
	}
	base.Steps[0].DestructiveHandling = "simulated"
	base.Steps[0].Output = "token=abcdefghijklmnop"
	if _, err := s.Rehearse(created.ID, "agent", "agent", "secret", 1, "isolated", "box", "", []Scenario{base}); err != ErrInvalid {
		t.Fatalf("secret error=%v", err)
	}
	base.Steps[0].Output = "ok"
	base.Steps[0].CostCents = -1
	if _, err := s.Rehearse(created.ID, "agent", "agent", "negative-cost", 1, "isolated", "box", "", []Scenario{base}); err != ErrInvalid {
		t.Fatalf("negative cost error=%v", err)
	}
	base.Steps[0].CostCents = maxRehearsalStepCostCents + 1
	if _, err := s.Rehearse(created.ID, "agent", "agent", "excessive-cost", 1, "isolated", "box", "", []Scenario{base}); err != ErrInvalid {
		t.Fatalf("excessive cost error=%v", err)
	}
	base.Steps[0].CostCents = maxRehearsalStepCostCents
	if _, err := s.Rehearse(created.ID, "agent", "agent", "maximum-cost", 1, "isolated", "box", "", []Scenario{base}); err != nil {
		t.Fatalf("maximum cost error=%v", err)
	}
}

func TestRehearsalRequiresExactlyOneDecisionRecord(t *testing.T) {
	s, _ := New(t.TempDir())
	r := validRevision("owner")
	r.Steps = append(r.Steps, Step{ID: "choose", Position: 2, Kind: "decision", Title: "Choose", Purpose: "Select recovery", Instructions: "Evaluate evidence", Preconditions: []string{"health known"}, ExpectedEvidence: []string{"selection"}, Decision: &Decision{Condition: "health failed", IfTrueStepID: "inspect", IfFalseStepID: "", HumanJudgment: "interpret health"}, OwnerIDs: []string{"owner"}, RequiredSkills: []string{"operations"}, Authority: Authority{ProhibitedActions: []string{"deploy"}}})
	created, err := s.Create("repo", "owner", "create", r)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	scenario := Scenario{ID: "failure", Name: "Failure", Failure: "unavailable", Inputs: []RehearsalInput{{Kind: "service", ResourceID: "checkout", Revision: "v1", EvidenceKind: "synthetic", Digest: "sha256:x"}}, Steps: []StepOutcome{{StepID: "inspect", Output: "failed", StartedAt: now, FinishedAt: now, Permissions: []string{"service:read"}, Outcome: "passed", DestructiveHandling: "not_applicable"}, {StepID: "choose", Output: "selected inspect", StartedAt: now, FinishedAt: now, Outcome: "passed", DestructiveHandling: "not_applicable"}}, AchievedOutcome: "achieved"}
	if _, err := s.Rehearse(created.ID, "human", "owner", "missing", 1, "isolated", "box", "", []Scenario{scenario}); err != ErrInvalid {
		t.Fatalf("missing decision error=%v", err)
	}
	scenario.Steps[0].Artifacts = []Artifact{{Name: "health.json", Digest: "sha256:health-failed", MediaType: "application/json"}}
	scenario.Steps[0].Assertions = []ConditionAssertion{{Condition: "health failed", Met: true, EvidenceDigest: "sha256:health-failed"}}
	scenario.Decisions = []BranchDecision{{StepID: "choose", Condition: "health failed", SelectedStepID: "inspect", EvidenceStepID: "inspect", Rationale: "synthetic health failed"}}
	if _, err := s.Rehearse(created.ID, "human", "owner", "valid", 1, "isolated", "box", "", []Scenario{scenario}); err != nil {
		t.Fatalf("valid decision error=%v", err)
	}
	scenario.Decisions = append(scenario.Decisions, scenario.Decisions[0])
	if _, err := s.Rehearse(created.ID, "human", "owner", "duplicate", 1, "isolated", "box", "", []Scenario{scenario}); err != ErrInvalid {
		t.Fatalf("duplicate decision error=%v", err)
	}
	scenario.Decisions = scenario.Decisions[:1]
	scenario.Decisions[0].Condition = "operator preference"
	if _, err := s.Rehearse(created.ID, "human", "owner", "arbitrary", 1, "isolated", "box", "", []Scenario{scenario}); err != ErrInvalid {
		t.Fatalf("arbitrary condition error=%v", err)
	}
	scenario.Decisions[0].Condition = "health failed"
	scenario.Steps[0].Assertions[0].Met = false
	if _, err := s.Rehearse(created.ID, "human", "owner", "mismatch", 1, "isolated", "box", "", []Scenario{scenario}); err != ErrInvalid {
		t.Fatalf("evidence branch mismatch error=%v", err)
	}
}
func TestVersioningAndAuthorityDiagnostics(t *testing.T) {
	s, _ := New(t.TempDir())
	r := validRevision("owner")
	created, e := s.Create("repo", "owner", "create", r)
	if e != nil || created.CurrentVersion != 1 || len(created.Diagnostics) != 0 {
		t.Fatalf("created=%+v err=%v", created, e)
	}
	r.Steps[0].References[0].Accessible = false
	r.Steps[0].Authority.Changes = []string{"traffic"}
	revised, e := s.Revise(created.ID, 1, "owner", "revise", r)
	if e != nil || revised.CurrentVersion != 2 || len(revised.Revisions) != 2 {
		t.Fatalf("revised=%+v err=%v", revised, e)
	}
	kinds := map[string]bool{}
	for _, d := range revised.Diagnostics {
		kinds[d.Kind] = true
	}
	if !kinds["inaccessible_resource"] || !kinds["unsafe_authority"] {
		t.Fatalf("diagnostics=%+v", revised.Diagnostics)
	}
	if _, e = s.Revise(created.ID, 1, "owner", "other", r); e != ErrConflict {
		t.Fatalf("expected conflict, got %v", e)
	}
}
func TestSecretBearingInputRemainsExplicit(t *testing.T) {
	s, _ := New(t.TempDir())
	r := validRevision("owner")
	r.Steps[0].Instructions = "Authorization: Bearer abcdefghijklmnopqrstuvwxyz"
	v, e := s.Create("repo", "owner", "secret", r)
	if e != nil {
		t.Fatal(e)
	}
	if len(v.Diagnostics) != 1 || v.Diagnostics[0].Kind != "secret_bearing_input" {
		t.Fatalf("diagnostics=%+v", v.Diagnostics)
	}
}

func TestTerminalExecutionAssessmentPreservesProcedureAndLinksSupportedFinding(t *testing.T) {
	s, _ := New(t.TempDir())
	created, err := s.Create("repo", "owner", "create-assessment", validRevision("owner"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	context := ExecutionContext{OriginKind: "alert", OriginID: "alert-assessment", OriginRevision: "v1", Summary: "checkout failure", AffectedResources: []string{"checkout"}, WindowFrom: now, WindowTo: now, Evidence: []ExecutionEvidence{{Kind: "metric", ResourceID: "checkout", Revision: "v1", Digest: "sha256:signal", Summary: "sanitized signal"}}}
	execution, err := s.StartExecution(created.ID, "owner", "launch-assessment", 1, context, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	stored, _ := s.read(created.ID)
	stored.Executions[0].Status = "ready"
	stored.Executions[0].Blockers = nil
	if err = s.write(stored); err != nil {
		t.Fatal(err)
	}
	terminal, err := s.Act(created.ID, execution.ID, "human", "owner", ExecutionAction{RequestID: "perform-assessment", ExpectedVersion: 1, Action: "perform", StepID: "inspect", Evidence: []ExecutionEvidence{{Kind: "result", ResourceID: "checkout", Revision: "v1", Digest: "sha256:result", Summary: "recovered with manual correction"}}, CostCents: 9, Health: "healthy"})
	if err != nil || terminal.Status != "completed" {
		t.Fatalf("terminal=%+v err=%v", terminal, err)
	}
	criteria := []CriterionResult{}
	for _, c := range created.Revisions[0].OutcomeCriteria {
		criteria = append(criteria, CriterionResult{Kind: c.Kind, Criterion: c.Criterion, Status: "met", EvidenceDigests: []string{"sha256:result"}, Explanation: "Exact retained evidence supports the result."})
	}
	assessed, err := s.Assess(created.ID, execution.ID, "owner", AssessmentInput{RequestID: "assess", ExpectedVersion: terminal.Version, Outcome: "completed", Criteria: criteria, Deviations: []ExecutionDeviation{{Kind: "manual_work", StepID: "inspect", Summary: "Operator corrected an undocumented flag.", EvidenceDigests: []string{"sha256:result"}}, {Kind: "cost", Summary: "Execution consumed nine cents.", EvidenceDigests: []string{"sha256:result"}}}, Findings: []ExecutionFinding{{Kind: "documentation", Summary: "Document the required flag correction.", EvidenceDigests: []string{"sha256:result"}}}, Feedback: []ParticipantFeedback{{ParticipantType: "human", ParticipantID: "owner", Summary: "The recovery step needs a reviewed flag example."}}, RequireFreshRehearsal: true})
	if err != nil || assessed.Status != "assessed" || assessed.RunbookVersion != 1 || assessed.Assessment == nil || len(assessed.Assessment.Findings) != 1 {
		t.Fatalf("assessed=%+v err=%v", assessed, err)
	}
	finding := assessed.Assessment.Findings[0]
	if _, err = s.ReserveImprovement(created.ID, execution.ID, "owner", "work", finding.ID, finding.Kind); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ReserveImprovement(created.ID, execution.ID, "other-owner", "competing-work", finding.ID, finding.Kind); err != ErrConflict {
		t.Fatalf("competing reservation err=%v", err)
	}
	linked, err := s.LinkImprovement(created.ID, execution.ID, "owner", ImprovementLink{RequestID: "work", FindingID: finding.ID, Kind: finding.Kind, ProposalID: "proposal-1", TaskID: "task-1"})
	if err != nil || linked.Assessment.Findings[0].ProposalID != "proposal-1" {
		t.Fatalf("linked=%+v err=%v", linked, err)
	}
	if retried, retryErr := s.ReserveImprovement(created.ID, execution.ID, "owner", "work", finding.ID, finding.Kind); retryErr != nil || retried.Assessment.Findings[0].ProposalID != "proposal-1" {
		t.Fatalf("retry=%+v err=%v", retried, retryErr)
	}
	retained, _ := s.Get(created.ID)
	if retained.CurrentVersion != 1 || retained.Executions[0].RunbookVersion != 1 {
		t.Fatalf("assessment rewrote procedure: %+v", retained)
	}
}

func TestAssessmentRechecksFallbackUnderStoreLock(t *testing.T) {
	s, _ := New(t.TempDir())
	primary, _ := s.Create("repo", "owner", "primary", validRevision("owner"))
	fallback, _ := s.Create("repo", "owner", "fallback", validRevision("owner"))
	now := time.Now().UTC()
	scenario := Scenario{ID: "fallback", Name: "Fallback", Failure: "primary unavailable", Inputs: []RehearsalInput{{Kind: "service", ResourceID: "checkout", Revision: "v1", EvidenceKind: "synthetic", Digest: "sha256:input"}}, Steps: []StepOutcome{{StepID: "inspect", Output: "healthy", StartedAt: now, FinishedAt: now, Permissions: []string{"service:read"}, Outcome: "passed", DestructiveHandling: "not_applicable"}}, AchievedOutcome: "achieved"}
	if _, err := s.Rehearse(fallback.ID, "human", "owner", "fallback-proof", 1, "isolated", "box", "", []Scenario{scenario}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Revise(fallback.ID, 1, "owner", "fallback-moved", validRevision("owner")); err != nil {
		t.Fatal(err)
	}
	stored, _ := s.read(primary.ID)
	stored.Executions = []Execution{{ID: "execution", RunbookID: primary.ID, RunbookVersion: 1, Status: "completed", Version: 1, Participants: []ExecutionParticipant{{ActorType: "human", ActorID: "owner", JoinedAt: now}}, CompletedEvidence: []ExecutionEvidence{{Kind: "result", ResourceID: "checkout", Revision: "v1", Digest: "sha256:result", Summary: "recovered"}}}}
	if err := s.write(stored); err != nil {
		t.Fatal(err)
	}
	criteria := []CriterionResult{}
	for _, c := range primary.Revisions[0].OutcomeCriteria {
		criteria = append(criteria, CriterionResult{Kind: c.Kind, Criterion: c.Criterion, Status: "met", EvidenceDigests: []string{"sha256:result"}, Explanation: "retained evidence"})
	}
	_, err := s.Assess(primary.ID, "execution", "owner", AssessmentInput{RequestID: "unsafe-suspension", ExpectedVersion: 1, Outcome: "completed", Criteria: criteria, RequireFreshRehearsal: true, SuspendCurrentUse: true, FallbackRunbookID: fallback.ID, FallbackRunbookVersion: 1})
	if err != ErrInvalid {
		t.Fatalf("stale fallback assessment err=%v", err)
	}
	retained, _ := s.Get(primary.ID)
	if retained.UseStatus != "active" || retained.Executions[0].Assessment != nil {
		t.Fatalf("stale fallback mutated primary: %+v", retained)
	}
}

func TestCreatePreservesUnreadableRetainedRecord(t *testing.T) {
	root := t.TempDir()
	s, _ := New(root)
	path := filepath.Join(root, stableID("repo", "owner", "create")+".json")
	original := []byte(`{"truncated"`)
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create("repo", "owner", "create", validRevision("owner")); err == nil {
		t.Fatal("expected retained-record read error")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) {
		t.Fatalf("retained record changed: %q", after)
	}
}
