package runbooks

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func validRevision(owner string) Revision {
	return Revision{Title: "Checkout recovery", Purpose: "Diagnose before changing state", Scope: Scope{Kind: "service", ResourceID: "checkout", Name: "Checkout"}, Preconditions: []string{"Confirm signal"}, RollbackCriteria: []string{"Stop on increased impact"}, OwnerIDs: []string{owner}, RequiredSkills: []string{"operations"}, Escalations: []Escalation{{Condition: "blocked", OwnerID: owner, Path: "owner", ExpectedAction: "decide"}}, ChangeReason: "initial", Steps: []Step{{ID: "inspect", Position: 1, Kind: "diagnostic", Title: "Inspect", Purpose: "Test hypothesis", Instructions: "Use reviewed workflow", Preconditions: []string{"read access"}, ExpectedEvidence: []string{"health digest"}, OwnerIDs: []string{owner}, RequiredSkills: []string{"operations"}, References: []Reference{{Kind: "command", ResourceID: "health-check", Revision: "abc", Reviewed: true, Accessible: true}}, Authority: Authority{RequiredAccess: []string{"service:read"}, Inspects: []string{"health"}, ProhibitedActions: []string{"deploy"}}}}}
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
