package recoveryoperations

import (
	"strings"
	"testing"
	"time"
)

var testDigest = strings.Repeat("a", 64)

func criterion(id string) ValidationCriterion {
	return ValidationCriterion{ID: id, Description: id, EvidenceKind: "protection_capture"}
}
func result(id string) ValidationResult {
	return ValidationResult{Criterion: id, Status: "passed", Evidence: EvidenceReference{Kind: "protection_capture", ResourceID: "capture", SHA256: testDigest}}
}

func TestRecoveryRequiresIndependentApprovalAndDependencyValidation(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.ConfigureValidationResolver(func(_ Operation, _ Step, results []ValidationResult) bool {
		for _, item := range results {
			if item.Evidence.ResourceID != "capture" || item.Evidence.SHA256 != testDigest {
				return false
			}
		}
		return true
	})
	now := time.Now().UTC()
	v, err := s.Create("incident", "repository", "commander", RecoveryPoint{PlanID: "plan", PlanVersion: 2, CaptureID: "capture", SourceRevision: "revision", CapturedAt: now, EstimatedLossMinutes: 7, ManifestSHA256: testDigest}, Revision{Objective: "Restore coherent collaboration state", RequiredApprovals: 1, ApproverIDs: []string{"approver"}, RollbackOption: "Return traffic to the isolated old environment", Steps: []Step{{ID: "data", Name: "Restore state", Kind: "restore", ResourceID: "database", AssigneeType: "human", AssigneeID: "responder", ValidationCriteria: []ValidationCriterion{criterion("manifest")}, Status: "pending"}, {ID: "service", Name: "Restore service", Kind: "cutover", ResourceID: "production", DependsOn: []string{"data"}, AssigneeType: "agent", AssigneeID: "agent", Delegation: "Execute only the bounded service bootstrap", Destructive: true, ValidationCriteria: []ValidationCriterion{criterion("health")}, Status: "pending"}}})
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
	if _, err = s.UpdateStep(v.ID, "data", "responder", "validated", "unsupported assertion", []ValidationResult{result("unrelated")}, v.CurrentVersion); err != ErrConflict {
		t.Fatalf("unsupported validation = %v", err)
	}
	forged := result("manifest")
	forged.Evidence.SHA256 = strings.Repeat("b", 64)
	if _, err = s.UpdateStep(v.ID, "data", "responder", "validated", "forged reference", []ValidationResult{forged}, v.CurrentVersion); err != ErrConflict {
		t.Fatalf("forged validation = %v", err)
	}
	v, err = s.UpdateStep(v.ID, "data", "responder", "validated", "manifest matches", []ValidationResult{result("manifest")}, v.CurrentVersion)
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
	v, err = s.Control(v.ID, "commander", "resume", "cutover fault contained; retry approved", v.CurrentVersion)
	if err != nil || v.Status != "ready" || current(&v).Steps[1].Status != "paused" {
		t.Fatalf("safe retry = %#v, %v", v, err)
	}
	v, err = s.UpdateStep(v.ID, "service", "agent", "running", "retry delegated bootstrap", nil, v.CurrentVersion)
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.UpdateStep(v.ID, "service", "agent", "validated", "health journey passed", []ValidationResult{result("health")}, v.CurrentVersion)
	if err != nil || v.Status != "validated" {
		t.Fatalf("retry validation = %#v, %v", v, err)
	}
	v, err = s.Control(v.ID, "commander", "complete", "service returned", v.CurrentVersion)
	if err != nil || v.Status != "completed" {
		t.Fatalf("completed recovery = %#v, %v", v, err)
	}
}

func TestRejectedRecoveryCannotResume(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Now().UTC()
	v, err := s.Create("incident", "repository", "commander", RecoveryPoint{PlanID: "plan", PlanVersion: 1, CaptureID: "capture", SourceRevision: "revision", CapturedAt: now, ManifestSHA256: testDigest}, Revision{Objective: "restore", RequiredApprovals: 1, ApproverIDs: []string{"approver"}, RollbackOption: "rollback", Steps: []Step{{ID: "step", Name: "restore", Kind: "restore", ResourceID: "resource", AssigneeType: "human", AssigneeID: "responder", ValidationCriteria: []ValidationCriterion{criterion("valid")}, Status: "pending"}}})
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
	_, err := s.Create("incident", "repository", "commander", RecoveryPoint{PlanID: "plan", PlanVersion: 1, CaptureID: "capture", SourceRevision: "revision", CapturedAt: now, ManifestSHA256: testDigest}, Revision{Objective: "restore", RequiredApprovals: 1, ApproverIDs: []string{"approver"}, RollbackOption: "rollback", Steps: []Step{{ID: "step", Name: "agent work", Kind: "restore", ResourceID: "resource", AssigneeType: "agent", AssigneeID: "agent", ValidationCriteria: []ValidationCriterion{criterion("valid")}, Status: "pending"}}})
	if err != ErrInvalid {
		t.Fatalf("undelegated agent = %v", err)
	}
}

func TestRecoveryCreatorCannotApproveOwnPlan(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Now().UTC()
	revision := Revision{Objective: "restore", RequiredApprovals: 1, ApproverIDs: []string{"commander"}, RollbackOption: "rollback", Steps: []Step{{ID: "step", Name: "restore", Kind: "restore", ResourceID: "resource", AssigneeType: "human", AssigneeID: "responder", ValidationCriteria: []ValidationCriterion{criterion("valid")}, Status: "pending"}}}
	if _, err := s.Create("incident", "repository", "commander", RecoveryPoint{PlanID: "plan", PlanVersion: 1, CaptureID: "capture", SourceRevision: "revision", CapturedAt: now, ManifestSHA256: testDigest}, revision); err != ErrInvalid {
		t.Fatalf("self-approved plan = %v", err)
	}
	revision.ApproverIDs = []string{"approver"}
	v, err := s.Create("incident", "repository", "commander", RecoveryPoint{PlanID: "plan", PlanVersion: 1, CaptureID: "capture", SourceRevision: "revision", CapturedAt: now, ManifestSHA256: testDigest}, revision)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := s.read(v.ID)
	if err != nil {
		t.Fatal(err)
	}
	legacy.Revisions[0].CreatedBy = "approver"
	if err = s.write(legacy); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Approve(v.ID, "approver", "approve", "self vote", v.CurrentVersion); err != ErrConflict {
		t.Fatalf("defensive self approval = %v", err)
	}
}
