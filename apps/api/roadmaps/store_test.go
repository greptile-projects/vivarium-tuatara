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
