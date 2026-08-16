package recoveryoperations

import (
	"testing"
	"time"
)

func TestRecoveryRequiresIndependentApprovalAndDependencyValidation(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	v, err := s.Create("incident", "repository", "commander", RecoveryPoint{PlanID: "plan", PlanVersion: 2, CaptureID: "capture", SourceRevision: "revision", CapturedAt: now, EstimatedLossMinutes: 7, ManifestSHA256: "digest"}, Revision{Objective: "Restore coherent collaboration state", RequiredApprovals: 1, ApproverIDs: []string{"approver"}, RollbackOption: "Return traffic to the isolated old environment", Steps: []Step{{ID: "data", Name: "Restore state", Kind: "restore", ResourceID: "database", AssigneeType: "human", AssigneeID: "responder", ValidationCriteria: []string{"manifest matches"}, Status: "pending"}, {ID: "service", Name: "Restore service", Kind: "cutover", ResourceID: "production", DependsOn: []string{"data"}, AssigneeType: "agent", AssigneeID: "agent", Delegation: "Execute only the bounded service bootstrap", Destructive: true, ValidationCriteria: []string{"health journey passes"}, Status: "pending"}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.UpdateStep(v.ID, "data", "responder", "running", "starting", nil, v.CurrentVersion); err != ErrConflict {
		t.Fatalf("step before approval = %v", err)
	}
	v, err = s.Approve(v.ID, "approver", "approve", "capture and rollback reviewed", v.CurrentVersion)
	if err != nil || v.Status != "ready" {
		t.Fatalf("approval = %#v, %v", v, err)
	}
	if _, err = s.UpdateStep(v.ID, "service", "agent", "running", "starting", nil, v.CurrentVersion); err != ErrConflict {
		t.Fatalf("dependency bypass = %v", err)
	}
	v, err = s.UpdateStep(v.ID, "data", "responder", "running", "isolated restore", nil, v.CurrentVersion)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.UpdateStep(v.ID, "data", "responder", "validated", "unsupported assertion", []ValidationResult{{Criterion: "unrelated", Status: "passed", Evidence: "claim"}}, v.CurrentVersion); err != ErrConflict {
		t.Fatalf("unsupported validation = %v", err)
	}
	v, err = s.UpdateStep(v.ID, "data", "responder", "validated", "manifest matches", []ValidationResult{{Criterion: "manifest matches", Status: "passed", Evidence: "sha256 digest matched the frozen manifest"}}, v.CurrentVersion)
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.UpdateStep(v.ID, "service", "agent", "running", "delegated bootstrap", nil, v.CurrentVersion)
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.UpdateStep(v.ID, "service", "agent", "failed", "health journey failed", nil, v.CurrentVersion)
	if err != nil || v.Status != "paused" || v.Control != "validation_failed" {
		t.Fatalf("failed safe = %#v, %v", v, err)
	}
	if _, err = s.Control(v.ID, "commander", "complete", "unsafe return", v.CurrentVersion); err != ErrConflict {
		t.Fatalf("unsafe completion = %v", err)
	}
}

func TestRejectedRecoveryCannotResume(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Now().UTC()
	v, err := s.Create("incident", "repository", "commander", RecoveryPoint{PlanID: "plan", PlanVersion: 1, CaptureID: "capture", SourceRevision: "revision", CapturedAt: now, ManifestSHA256: "digest"}, Revision{Objective: "restore", RequiredApprovals: 1, ApproverIDs: []string{"approver"}, RollbackOption: "rollback", Steps: []Step{{ID: "step", Name: "restore", Kind: "restore", ResourceID: "resource", AssigneeType: "human", AssigneeID: "responder", ValidationCriteria: []string{"valid"}, Status: "pending"}}})
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Approve(v.ID, "approver", "reject", "unsafe point", v.CurrentVersion)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Control(v.ID, "commander", "resume", "bypass", v.CurrentVersion); err != ErrConflict {
		t.Fatalf("rejected resume = %v", err)
	}
}

func TestRecoveryRejectsUndelegatedAgentAndStaleWrites(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Now().UTC()
	_, err := s.Create("incident", "repository", "commander", RecoveryPoint{PlanID: "plan", PlanVersion: 1, CaptureID: "capture", SourceRevision: "revision", CapturedAt: now, ManifestSHA256: "digest"}, Revision{Objective: "restore", RequiredApprovals: 1, ApproverIDs: []string{"approver"}, RollbackOption: "rollback", Steps: []Step{{ID: "step", Name: "agent work", Kind: "restore", ResourceID: "resource", AssigneeType: "agent", AssigneeID: "agent", ValidationCriteria: []string{"valid"}, Status: "pending"}}})
	if err != ErrInvalid {
		t.Fatalf("undelegated agent = %v", err)
	}
}
