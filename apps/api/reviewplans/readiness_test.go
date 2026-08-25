package reviewplans

import "testing"

func TestProjectReadinessRequiresCompleteCurrentAreaCoverage(t *testing.T) {
	sha := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	plan := Plan{ID: "plan", Version: 1, SourceRevision: sha, TargetRevision: sha, Areas: []Area{{ID: "security", Title: "Security", OwnerIDs: []string{"owner"}, Evidence: []Evidence{{Kind: "check", Required: true}}}}}
	matrix := ProjectReadiness(&plan, []Assignment{{PlanID: "plan", AreaID: "security", Status: "accepted"}}, nil, nil, sha, sha, nil)
	if matrix.Complete || len(matrix.Areas[0].UnresolvedGaps) != 3 {
		t.Fatalf("incomplete matrix = %#v", matrix)
	}
	work := []WorkEntry{{ID: "decision", PlanID: "plan", AreaID: "security", SourceRevision: sha, TargetRevision: sha, ActorType: "human", ActorID: "owner", Kind: "decision", Body: "approved", Citations: []WorkCitation{{Kind: "check", Value: "run"}}}}
	matrix = ProjectReadiness(&plan, []Assignment{{PlanID: "plan", AreaID: "security", Status: "accepted"}}, work, nil, sha, sha, nil)
	if !matrix.Complete || !matrix.Areas[0].Complete {
		t.Fatalf("complete matrix = %#v", matrix)
	}
	matrix = ProjectReadiness(&plan, nil, work, nil, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", sha, nil)
	if matrix.Complete || matrix.Current {
		t.Fatalf("moved source matrix = %#v", matrix)
	}
}
