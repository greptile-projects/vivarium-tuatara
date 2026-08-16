package recoveryexercises

import "testing"

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
