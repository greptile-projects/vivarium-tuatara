package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/learningassessments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/learningpathways"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
)

func TestContributionLearningEvidenceIsCurrentLearnerContextNotAuthority(t *testing.T) {
	root := t.TempDir()
	pathways, _ := learningpathways.New(filepath.Join(root, "pathways"))
	assessments, _ := learningassessments.New(filepath.Join(root, "assessments"))
	workspaceStore, _ := workspaces.New(filepath.Join(root, "workspaces"))
	repo, learner, owner, revision := "11111111111111111111111111111111", "22222222222222222222222222222222", "33333333333333333333333333333333", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	pathway, err := pathways.Publish(learningpathways.Revision{
		RequestID: "pathway-request", RepositoryID: repo, Slug: "onboarding", Role: "contributor", Outcome: "ship a reviewed change", PublishedBy: owner,
		Objectives: []string{"Go APIs"}, SupportedRevisions: []string{revision}, ExpectedMinutes: 30, Locales: []string{"en"}, CompletionEvidence: []string{"reviewed exercise"},
		Modules: []learningpathways.Module{{ID: "api", Title: "API work", WhyItMatters: "safe collaboration", Objectives: []string{"Go APIs"}, EstimatedMinutes: 30, Exercises: []learningpathways.Exercise{{ID: "fix", Title: "Fix route", Kind: "change", Instructions: "make a bounded change", Revision: revision, AcceptanceCriteria: []string{"tests pass"}, Evidence: []string{"checkpoint"}}}}},
	}, 0)
	if err != nil {
		t.Fatalf("publish pathway: %v", err)
	}
	definition, err := assessments.Publish(learningassessments.Definition{RequestID: "assessment-request", RepositoryID: repo, Slug: "readiness", PathwaySlug: pathway.Slug, PathwayVersion: pathway.Version, ProjectRevision: revision, Title: "Readiness", Criteria: []learningassessments.Criterion{{ID: "go-api", Label: "Go APIs", Description: "implements bounded APIs", Weight: 1, Required: true}}, RetryPolicy: learningassessments.RetryPolicy{MaximumAttempts: 2}, PublishedBy: owner}, 0)
	if err != nil {
		t.Fatalf("publish assessment: %v", err)
	}
	workspace, err := workspaceStore.Create(workspaces.Workspace{RepositoryID: repo, CommitID: revision, CreatorID: learner, Source: workspaces.Source{Kind: "learning"}, Definition: workspaces.Definition{Version: 1, Image: "alpine"}, LearningContext: &workspaces.LearningContext{PathwaySlug: pathway.Slug, PathwayVersion: pathway.Version, ModuleID: "api", ExerciseID: "fix"}}, []byte(`{}`))
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	attempt, err := assessments.CreateAttempt(learningassessments.Attempt{RequestID: "attempt-request", RepositoryID: repo, AssessmentSlug: definition.Slug, AssessmentVersion: definition.Version, WorkspaceID: workspace.ID, LearnerID: learner, ProjectRevision: revision, Evidence: learningassessments.Evidence{CheckpointIDs: []string{"checkpoint"}, AuthorshipStatement: "I authored the solution"}}, 2, 0)
	if err != nil {
		t.Fatalf("create attempt: %v", err)
	}
	attempt, err = assessments.UpdateAttempt(repo, definition.Slug, attempt.ID, func(value *learningassessments.Attempt) error { value.Status = "demonstrated"; return nil })
	if err != nil {
		t.Fatalf("demonstrate attempt: %v", err)
	}
	evidence, skills, err := resolveContributionLearningEvidence(repo, learner, attempt.ID, assessments, pathways, workspaceStore)
	if err != nil || evidence.ModuleID != "api" || evidence.ExerciseID != "fix" || evidence.AuthorshipStatement == "" || !learningAssessmentContains(skills, "Go APIs") {
		t.Fatalf("evidence=%#v skills=%#v err=%v", evidence, skills, err)
	}
	if _, _, err = resolveContributionLearningEvidence(repo, "someone-else", attempt.ID, assessments, pathways, workspaceStore); err == nil {
		t.Fatal("another actor reused private learning evidence")
	}
	if _, err = pathways.Publish(learningpathways.Revision{RequestID: "next-request", RepositoryID: repo, Slug: "onboarding", Role: "contributor", Outcome: "ship revised work", PublishedBy: owner, Objectives: []string{"Go APIs"}, SupportedRevisions: []string{revision}, ExpectedMinutes: 30, Locales: []string{"en"}, CompletionEvidence: []string{"reviewed exercise"}, Modules: pathway.Modules}, 1); err != nil {
		t.Fatalf("publish successor: %v", err)
	}
	if _, _, err = resolveContributionLearningEvidence(repo, learner, attempt.ID, assessments, pathways, workspaceStore); err == nil {
		t.Fatal("stale pathway evidence remained eligible")
	}
}

func TestContributionPullBodyDisclosesLearningAuthorshipAndOrdinaryBoundary(t *testing.T) {
	body := contributionPullBody(workspaces.Workspace{ID: "workspace"}, workspaces.Checkpoint{ID: "checkpoint"}, pullrequests.ContributionEvidence{
		OpportunityID: "opportunity", PathwayVersion: 2, UpstreamRevision: strings.Repeat("a", 40), AcceptanceCriteria: []string{"tests pass"},
		LearningEvidence: &pullrequests.ContributionLearningEvidence{PathwaySlug: "onboarding", PathwayVersion: 3, ModuleID: "api", ExerciseID: "fix", AssessmentSlug: "readiness", AssessmentVersion: 1, AttemptID: "attempt", AuthorshipStatement: "I authored the solution", AgentAssistanceDeclared: true},
	})
	for _, retained := range []string{"Learning pathway: onboarding revision 3", "Completed exercise: api/fix", "Authorship: I authored the solution", "Learning agent assistance: declared", "ordinary discussion, review, reproduction, checks"} {
		if !strings.Contains(body, retained) {
			t.Fatalf("pull body omitted %q: %s", retained, body)
		}
	}
}
