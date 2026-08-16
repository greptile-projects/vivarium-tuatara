package recoveryexercises

import (
	"strings"
	"testing"
)

func TestRunRetainsOrderedEvidenceGapsAndAttribution(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	in := Exercise{Name: "Region loss drill", Scenario: "primary unavailable", PlanID: "plan", PlanVersion: 2, CommitmentID: "contract", CommitmentVersion: 3, CaptureID: "capture", SourceRevision: "source", Steps: []Step{
		{ID: "restore", Kind: "restore", Name: "Restore", Command: "restore:protected-manifest", Objective: "state restored"},
		{ID: "journey", Kind: "journey", Name: "Smoke", DependsOn: []string{"restore"}, Command: "journey:smoke", Objective: "system usable"},
	}}
	got, err := store.Run("repo", "owner", in, func(step Step) (string, string, string, bool) {
		if step.ID == "restore" {
			return "passed", "restored", "manifest:2", false
		}
		return "failed", "missing queue state", "", false
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "gaps_found" || got.StartedBy != "owner" || got.EnvironmentID == "" || len(got.Results) != 2 || len(got.Gaps) != 1 || len(got.AchievedObjectives) != 1 {
		t.Fatalf("exercise = %#v", got)
	}
	listed, err := store.List("repo")
	if err != nil || len(listed) != 1 || listed[0].Results[1].Log != "missing queue state" {
		t.Fatalf("list = %#v, %v", listed, err)
	}
}

func TestGapInvestigationImprovementAndFreshVerification(t *testing.T) {
	store, _ := New(t.TempDir())
	run := func(version int, source, status string) Exercise {
		in := Exercise{Name: "Regional drill", Scenario: "loss", PlanID: "plan", PlanVersion: version, CommitmentID: "contract", CommitmentVersion: 1, CaptureID: "capture", SourceRevision: source, Steps: []Step{{ID: "journey", Kind: "journey", Name: "Smoke", Command: "journey:smoke", Objective: "usable"}}}
		got, err := store.Run("repo", "owner", in, func(Step) (string, string, string, bool) { return status, "bounded evidence", "", false })
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	failed := run(1, strings.Repeat("a", 40), "failed")
	opened, err := store.OpenInvestigation("repo", failed.ID, "reader", "agent", Investigation{Title: "Queue state missing", Evidence: []Evidence{{Kind: "exercise_result", ResourceID: "journey", Summary: "Bounded journey failed"}}})
	if err != nil || opened.Investigations[0].ActorType != "agent" {
		t.Fatalf("investigation = %#v, %v", opened, err)
	}
	inv := opened.Investigations[0]
	found, err := store.AddFinding("repo", failed.ID, inv.ID, "reader", "agent", inv.Version, Finding{Statement: "Queue restore is absent.", Uncertainty: "Release configuration was not inspected.", Confidence: "medium", CitationIDs: []string{inv.Evidence[0].ID}})
	if err != nil || found.Investigations[0].Findings[0].CreatedBy != "reader" {
		t.Fatalf("finding = %#v, %v", found, err)
	}
	finding := found.Investigations[0].Findings[0]
	linked, improvement, err := store.LinkImprovement("repo", failed.ID, "owner", Improvement{InvestigationID: inv.ID, FindingID: finding.ID, ProposalID: "proposal", TaskIDs: []string{"task"}, BaseRevision: strings.Repeat("b", 40), Criteria: []string{"fresh exercise passes"}})
	if err != nil || improvement.Status != "work_open" || linked.Improvements[0].ProposalID != "proposal" {
		t.Fatalf("improvement = %#v, %v", improvement, err)
	}
	unchanged := run(1, strings.Repeat("a", 40), "passed")
	if _, err = store.VerifyImprovement("repo", failed.ID, improvement.ID, unchanged.ID); err != ErrInvalid {
		t.Fatalf("unchanged verification = %v", err)
	}
	fresh := run(2, strings.Repeat("c", 40), "passed")
	verified, err := store.VerifyImprovement("repo", failed.ID, improvement.ID, fresh.ID)
	if err != nil || verified.Improvements[0].Status != "verified" || verified.Improvements[0].FollowUpID != fresh.ID {
		t.Fatalf("verified = %#v, %v", verified, err)
	}
}

func TestInvestigationRejectsUncitedOrAgentOutcomeAuthority(t *testing.T) {
	store, _ := New(t.TempDir())
	in := Exercise{Name: "Drill", Scenario: "loss", PlanID: "plan", PlanVersion: 1, CommitmentID: "contract", CommitmentVersion: 1, CaptureID: "capture", SourceRevision: "source", Steps: []Step{{ID: "restore", Kind: "restore", Name: "Restore", Command: "restore:protected-manifest", Objective: "restored"}}}
	failed, _ := store.Run("repo", "owner", in, func(Step) (string, string, string, bool) { return "failed", "gap", "", false })
	if _, err := store.OpenInvestigation("repo", failed.ID, "agent", "agent", Investigation{Title: "Uncited"}); err != ErrInvalid {
		t.Fatalf("uncited = %v", err)
	}
	opened, _ := store.OpenInvestigation("repo", failed.ID, "agent", "agent", Investigation{Title: "Cited", Evidence: []Evidence{{Kind: "exercise_result", ResourceID: "restore", Summary: "restore failed"}}})
	inv := opened.Investigations[0]
	if _, err := store.AddFinding("repo", failed.ID, inv.ID, "agent", "agent", inv.Version, Finding{Statement: "Claim", Uncertainty: "Unknown", Confidence: "high", CitationIDs: []string{"invented"}}); err != ErrInvalid {
		t.Fatalf("invented citation = %v", err)
	}
}

func TestRunRejectsOutOfOrderOrUnboundedSteps(t *testing.T) {
	store, _ := New(t.TempDir())
	base := Exercise{Name: "Drill", Scenario: "loss", PlanID: "plan", PlanVersion: 1, CommitmentID: "contract", CommitmentVersion: 1, CaptureID: "capture", SourceRevision: "source"}
	base.Steps = []Step{{ID: "journey", Kind: "shell", Name: "Escape", Command: "rm", Objective: "bad"}}
	if _, err := store.Run("repo", "owner", base, nil); err != ErrInvalid {
		t.Fatalf("unbounded step error = %v", err)
	}
	base.Steps = []Step{{ID: "journey", Kind: "journey", Name: "Smoke", DependsOn: []string{"restore"}, Command: "journey:smoke", Objective: "usable"}}
	if _, err := store.Run("repo", "owner", base, nil); err != ErrInvalid {
		t.Fatalf("out-of-order error = %v", err)
	}
}
