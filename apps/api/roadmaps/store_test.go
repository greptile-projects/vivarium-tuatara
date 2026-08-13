package roadmaps

import (
	"errors"
	"testing"
)

func revision() Revision {
	return Revision{Goals: []string{"Make review continuous"}, Capacity: "One team this quarter", Decisions: []OpportunityDecision{{OpportunityID: "opp-1", Version: 2, Outcome: "accepted", Reason: "High reach", GoalFit: "Direct", Capacity: "Fits"}, {OpportunityID: "opp-2", Version: 1, Outcome: "rejected", Reason: "Displaces reliability", GoalFit: "Weak", Capacity: "Unavailable"}}, Items: []Item{{ID: "item-1", OpportunityID: "opp-1", Title: "Review continuity", OwnerIDs: []string{"owner"}, TargetHorizon: "2026 Q4", SuccessMeasures: []string{"20% fewer abandoned reviews"}, Position: 1, Status: "planned"}}}
}
func TestRoadmapRequiresAttributedReplansAndSeparatesScenarios(t *testing.T) {
	s, _ := New(t.TempDir())
	v, e := s.Publish("repo", "owner", 0, revision())
	if e != nil || v.Version != 1 {
		t.Fatalf("publish=%#v %v", v, e)
	}
	if _, e = s.Publish("repo", "owner", 1, revision()); !errors.Is(e, ErrInvalid) {
		t.Fatalf("silent replan=%v", e)
	}
	scenario := revision()
	v, e = s.Propose("repo", "agent", "agent", 1, scenario, "Sequence dependencies first")
	if e != nil || len(v.Scenarios) != 1 || len(v.Revisions) != 1 {
		t.Fatalf("scenario=%#v %v", v, e)
	}
	r := revision()
	r.ChangeReason = "Owner became unavailable"
	r.ReplanTriggers = []string{"unavailable_owner"}
	r.Items[0].OwnerIDs = []string{"new-owner"}
	v, e = s.Publish("repo", "maintainer", 2, r)
	if e != nil || v.Revisions[1].CreatedBy != "maintainer" {
		t.Fatalf("replan=%#v %v", v, e)
	}
	v, e = s.Comment("repo", "reporter", "human", 3, "This horizon conflicts with the documented migration.")
	if e != nil || len(v.Comments) != 1 {
		t.Fatalf("comment=%#v %v", v, e)
	}
}

func TestImplementationRequiresMeasuredValueBeforeAchievementAndReopensOnDrift(t *testing.T) {
	s, _ := New(t.TempDir())
	v, err := s.Publish("repo", "owner", 0, revision())
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.LinkImplementation("repo", "owner", v.Version, 1, "item-1", "opp-1", "proposal-1", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", []string{"task-1"})
	if err != nil || v.Implementations[0].OutcomeState != "delivering" {
		t.Fatalf("link=%#v %v", v, err)
	}
	if _, err = s.ReportOutcome("repo", "owner", v.Version, "proposal-1", DeliveryEvidence{Kind: "measure_met", Summary: "wrong measure", ResourceKind: "experiment", ResourceID: "run-1", MeasureIndexes: []int{1}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid measure=%v", err)
	}
	v, err = s.ReportOutcome("repo", "owner", v.Version, "proposal-1", DeliveryEvidence{Kind: "delivery", Summary: "released", ResourceKind: "release", ResourceID: "release-1"})
	if err != nil || v.Implementations[0].OutcomeState != "delivering" {
		t.Fatalf("shipping claimed value=%#v %v", v, err)
	}
	v, err = s.ReportOutcome("repo", "researcher", v.Version, "proposal-1", DeliveryEvidence{Kind: "measure_met", Summary: "abandonment fell 23%", ResourceKind: "experiment", ResourceID: "run-1", MeasureIndexes: []int{0}})
	if err != nil || v.Implementations[0].OutcomeState != "achieved" {
		t.Fatalf("measure=%#v %v", v, err)
	}
	v, err = s.ReportOutcome("repo", "reporter", v.Version, "proposal-1", DeliveryEvidence{Kind: "need_unresolved", Summary: "keyboard reviewers still cannot finish", ResourceKind: "experiment", ResourceID: "run-1"})
	if err != nil || v.Implementations[0].OutcomeState != "revisit_required" || v.Implementations[0].RevisitReason == "" {
		t.Fatalf("revisit=%#v %v", v, err)
	}
	v, err = s.ReportOutcome("repo", "researcher", v.Version, "proposal-1", DeliveryEvidence{Kind: "measure_met", Summary: "later aggregate still clears the threshold", ResourceKind: "experiment", ResourceID: "run-2", MeasureIndexes: []int{0}})
	if err != nil || v.Implementations[0].OutcomeState != "revisit_required" {
		t.Fatalf("retained blocker overwritten=%#v %v", v, err)
	}
}
