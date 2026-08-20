package capabilities

import (
	"testing"
	"time"
)

func readyRemoval(t *testing.T) (*Store, Capability, RetirementPlan, MigrationCandidate) {
	t.Helper()
	s, _ := New(t.TempDir())
	now := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	r := retirementFixture(now)
	r.Consumers[0].EvidenceState = "current"
	r.Consumers[0].RepositoryID = "consumer"
	r.Consumers[0].Revision = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	r.Consumers[0].EvidenceReference = "usage:zero"
	r.Consumers[0].LastObservedAt = &now
	v, _ := s.Create("repo", "provider", r)
	p := planFixture(now)
	p.Audiences[0].Commitment, p.Audiences[0].EmbargoedDependency = "", false
	v, _ = s.OpenRetirement("repo", v.ID, "provider", p)
	v, _ = s.AppendRetirementEvent("repo", v.ID, v.RetirementPlans[0].ID, "mobile-owner", "human", 0, RetirementEvent{Type: "approval", Summary: "approved", OwnerID: "mobile-owner", Decision: "approved"})
	checks := []CandidateCheck{}
	for i, stage := range []string{"old_only", "dual_support", "replacement", "rollback", "journey"} {
		checks = append(checks, CandidateCheck{ID: stage, Stage: stage, Journey: map[bool]string{true: "checkout"}[stage == "journey"], RepositoryID: "repo", Revision: r.CommitID, Command: "test " + stage, Paths: []string{"api.go"}, Expectation: "pass", Evidence: []CandidateEvidence{{ID: stage, OutcomeID: "outcome-" + stage, Status: "passed", CreatedAt: now.Add(time.Duration(i) * time.Second)}}})
	}
	c := MigrationCandidate{ID: randomID(), CapabilityVersion: 1, ProviderRevision: r.CommitID, Checks: checks, Usage: []UsageObservation{{ID: randomID(), ConsumerIndex: 0, RepositoryID: "consumer", Revision: r.Consumers[0].Revision, State: "measured", TotalUses: 10, OldBehaviorUses: 0, Summary: "zero use", OwnerID: "mobile-owner", Acknowledged: true}}}
	raw, _ := s.read("repo", v.ID)
	raw.RetirementPlans[0].Candidates = []MigrationCandidate{c}
	if err := s.write(raw); err != nil {
		t.Fatal(err)
	}
	v, _ = s.Get("repo", v.ID)
	return s, v, v.RetirementPlans[0], v.RetirementPlans[0].Candidates[0]
}

func TestRemovalPausesOnUnexpectedUseAndRestoresCompatibility(t *testing.T) {
	s, v, p, c := readyRemoval(t)
	_, execution, err := s.StartRemoval("repo", v.ID, p.ID, c.ID, "provider")
	if err != nil {
		t.Fatal(err)
	}
	report := StageReport{StageIndex: 0, Stage: p.Stages[0].Name, Action: "advance", RemainingUse: 1, Health: "degraded", Control: "owner", RollbackBoundary: "before schema contract", NextAction: "restore", UnexpectedConsumers: []string{"batch client"}, Delivery: []DeliveryReference{{Kind: "deployment", ResourceID: "deployment-1", Revision: c.ProviderRevision, Status: "succeeded"}}}
	v, err = s.ReportRemovalStage("repo", v.ID, p.ID, execution.ID, "provider", 1, report)
	if err != nil || v.RetirementPlans[0].Executions[0].Status != "paused" {
		t.Fatalf("pause = %#v, %v", v.RetirementPlans[0].Executions, err)
	}
	report.Action, report.CompatibilityRestored, report.NextAction = "restore", true, "reassess consumer"
	v, err = s.ReportRemovalStage("repo", v.ID, p.ID, execution.ID, "provider", 2, report)
	if err != nil || v.RetirementPlans[0].Executions[0].Status != "restored" {
		t.Fatalf("restore = %#v, %v", v.RetirementPlans[0].Executions, err)
	}
}

func TestRemovalCompletionAccountsForEveryObsoleteSurface(t *testing.T) {
	s, v, p, c := readyRemoval(t)
	_, execution, err := s.StartRemoval("repo", v.ID, p.ID, c.ID, "provider")
	if err != nil {
		t.Fatal(err)
	}
	version := 1
	for i, stage := range execution.StageNames {
		report := StageReport{StageIndex: i, Stage: stage, Action: "advance", Health: "healthy", Control: "provider owner", RollbackBoundary: "release before contract", NextAction: "continue", Delivery: []DeliveryReference{{Kind: "merge_queue", ResourceID: "pull-1", Revision: c.ProviderRevision, Status: "succeeded"}}}
		v, err = s.ReportRemovalStage("repo", v.ID, p.ID, execution.ID, "provider", version, report)
		if err != nil {
			t.Fatal(err)
		}
		version++
	}
	if v.RetirementPlans[0].Executions[0].Status != "awaiting_verification" {
		t.Fatal("final stage did not await verification")
	}
	verify := func(proof CleanupProof) bool {
		return proof.RepositoryID == "repo" && proof.Revision == c.ProviderRevision
	}
	if _, err = s.CompleteRemoval("repo", v.ID, p.ID, execution.ID, "provider", version, []CleanupProof{{Kind: "code"}}, verify); err != ErrConflict {
		t.Fatalf("partial cleanup = %v", err)
	}
	proofs := []CleanupProof{}
	for _, kind := range []string{"code", "flags", "data", "credentials", "telemetry", "documentation", "policy_exceptions"} {
		proofs = append(proofs, CleanupProof{Kind: kind, RepositoryID: "repo", Revision: c.ProviderRevision, Paths: []string{"evidence/" + kind}, Digest: "sha256:" + kind, Summary: kind + " removed"})
	}
	unrelated := append([]CleanupProof(nil), proofs...)
	for i := range unrelated {
		unrelated[i].Revision = "cccccccccccccccccccccccccccccccccccccccc"
	}
	if _, err = s.CompleteRemoval("repo", v.ID, p.ID, execution.ID, "provider", version, unrelated, func(CleanupProof) bool { return true }); err != ErrConflict {
		t.Fatalf("unrelated cleanup revision = %v", err)
	}
	v, err = s.CompleteRemoval("repo", v.ID, p.ID, execution.ID, "provider", version, proofs, verify)
	if err != nil || v.RetirementPlans[0].Executions[0].Status != "completed" {
		t.Fatalf("completion = %#v, %v", v.RetirementPlans[0].Executions, err)
	}
}

func TestRemovalControllerTransferPreservesRecoveryAfterOwnershipChange(t *testing.T) {
	s, v, p, c := readyRemoval(t)
	_, execution, err := s.StartRemoval("repo", v.ID, p.ID, c.ID, "provider")
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.TransferRemovalController("repo", v.ID, p.ID, execution.ID, "new-owner", "new-owner", "provider ownership changed", 1)
	if err != nil {
		t.Fatal(err)
	}
	transferred := v.RetirementPlans[0].Executions[0]
	if transferred.ControllerID != "new-owner" || transferred.Version != 2 || len(transferred.Transfers) != 1 || transferred.Transfers[0].PreviousController != "provider" {
		t.Fatalf("transfer = %#v", transferred)
	}
	report := StageReport{StageIndex: 0, Stage: p.Stages[0].Name, Action: "pause", Health: "healthy", Control: "successor owner", RollbackBoundary: "before contract", NextAction: "inspect", Delivery: []DeliveryReference{{Kind: "deployment", ResourceID: "deployment-1", Revision: c.ProviderRevision, Status: "pending"}}}
	if _, err = s.ReportRemovalStage("repo", v.ID, p.ID, execution.ID, "provider", 2, report); err != ErrConflict {
		t.Fatalf("predecessor report = %v", err)
	}
	if _, err = s.ReportRemovalStage("repo", v.ID, p.ID, execution.ID, "new-owner", 2, report); err != nil {
		t.Fatalf("successor report = %v", err)
	}
}
