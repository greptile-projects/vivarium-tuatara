package main

import (
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

func TestReviewPlanDerivesSpecialistAreasOverlapAndExactRevisions(t *testing.T) {
	p := pullrequests.PullRequest{ID: "pull", RepositoryID: "repo", SourceCommitID: "1111111111111111111111111111111111111111", TargetCommitID: "2222222222222222222222222222222222222222", Body: "Protect sign in and keyboard navigation"}
	repo := repositories.Repository{ID: "repo", OwnerID: "owner"}
	plan := deriveReviewPlan(p, repo, "author", []string{"apps/web/security-accessibility.tsx", "README.md"}, reviewPlanInput{})
	if plan.SourceRevision != p.SourceCommitID || plan.TargetRevision != p.TargetCommitID || plan.Intent != p.Body {
		t.Fatalf("exact candidate context lost: %#v", plan)
	}
	if len(plan.Areas) != 3 || len(plan.AffectedCommitments) != 2 {
		t.Fatalf("specialist review was not derived: %#v", plan.Areas)
	}
	foundOverlap := false
	for _, d := range plan.Diagnostics {
		if d.Code == "overlapping_scope" && d.AttributedTo == "author" {
			foundOverlap = true
		}
	}
	if !foundOverlap {
		t.Fatalf("overlapping scope was hidden: %#v", plan.Diagnostics)
	}
}
