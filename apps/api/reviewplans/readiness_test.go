package reviewplans

import (
	"testing"
	"time"
)

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

func TestProjectReadinessRequiresDomainAndCompletePathScope(t *testing.T) {
	sha := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	area := Area{ID: "security", Title: "Security", Paths: []string{"auth.go", "token.go"}, OwnerIDs: []string{"owner"}, Evidence: []Evidence{{Kind: "security_evidence", Required: true}}}
	plan := Plan{ID: "plan", Version: 1, SourceRevision: sha, TargetRevision: sha, Areas: []Area{area}}
	assignment := []Assignment{{PlanID: "plan", AreaID: area.ID, Status: "accepted"}}
	decision := WorkEntry{PlanID: "plan", AreaID: area.ID, SourceRevision: sha, TargetRevision: sha, ActorType: "human", ActorID: "owner", Kind: "decision", Body: "reviewed"}
	for name, citation := range map[string]WorkCitation{
		"generic":       {Kind: "check", Value: "unrelated"},
		"wrong domain":  {Kind: "check", Value: "run", Domain: "privacy", CoveredPaths: area.Paths},
		"partial scope": {Kind: "check", Value: "run", Domain: "security", CoveredPaths: []string{"auth.go"}},
	} {
		t.Run(name, func(t *testing.T) {
			decision.Citations = []WorkCitation{citation}
			if got := ProjectReadiness(&plan, assignment, []WorkEntry{decision}, nil, sha, sha, nil); got.Complete {
				t.Fatalf("unscoped evidence completed matrix: %#v", got)
			}
		})
	}
	decision.Citations = []WorkCitation{{Kind: "check", Value: "run", Domain: "security", CoveredPaths: area.Paths}}
	if got := ProjectReadiness(&plan, assignment, []WorkEntry{decision}, nil, sha, sha, nil); !got.Complete {
		t.Fatalf("scoped evidence did not complete matrix: %#v", got)
	}
}

func TestProjectReadinessLaterDiscussionDoesNotEraseTerminalDisposition(t *testing.T) {
	sha := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	plan := Plan{ID: "plan", Version: 1, SourceRevision: sha, TargetRevision: sha, Areas: []Area{{ID: "code", Title: "Code", OwnerIDs: []string{"owner"}}}}
	work := []WorkEntry{{ID: "finding", PlanID: "plan", AreaID: "code", SourceRevision: sha, TargetRevision: sha, ActorType: "human", ActorID: "owner", Kind: "finding", Body: "issue"}, {ID: "decision", PlanID: "plan", AreaID: "code", SourceRevision: sha, TargetRevision: sha, ActorType: "human", ActorID: "owner", Kind: "decision", Body: "reviewed"}}
	now := time.Now()
	resolutions := []FindingResolution{{FindingID: "finding", CandidateRevision: sha, Action: "resolved", CreatedAt: now}, {FindingID: "finding", CandidateRevision: sha, Action: "discuss", CreatedAt: now.Add(time.Minute)}}
	got := ProjectReadiness(&plan, []Assignment{{PlanID: "plan", AreaID: "code", Status: "accepted"}}, work, resolutions, sha, sha, nil)
	if !got.Complete {
		t.Fatalf("discussion erased terminal disposition: %#v", got)
	}
}
