package main

import (
	"errors"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/learningassessments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/learningpathways"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
)

var errLearningEvidenceIneligible = errors.New("learning evidence is not eligible")

// resolveContributionLearningEvidence turns a reviewed learning result into
// portable context. It deliberately returns no capability: launch, fork,
// agent, pull, review, and merge authorization continue through their ordinary
// stores and routes.
func resolveContributionLearningEvidence(repositoryID, learnerID, attemptID string, assessments *learningassessments.Store, pathways *learningpathways.Store, workspaceStore *workspaces.Store) (workspaces.ContributionLearningEvidence, []string, error) {
	if assessments == nil || pathways == nil || workspaceStore == nil || learnerID == "" {
		return workspaces.ContributionLearningEvidence{}, nil, errLearningEvidenceIneligible
	}
	attempt, err := assessments.FindAttempt(repositoryID, attemptID)
	if err != nil || attempt.LearnerID != learnerID || attempt.Status != "demonstrated" || len(attempt.Blockers) != 0 {
		return workspaces.ContributionLearningEvidence{}, nil, errLearningEvidenceIneligible
	}
	authorship := strings.TrimSpace(attempt.Evidence.AuthorshipStatement)
	if authorship == "" || len(authorship) > 2000 || len(attempt.Evidence.CheckpointIDs) > 100 || len(attempt.Evidence.CommandOutcomeIDs) > 100 {
		return workspaces.ContributionLearningEvidence{}, nil, errLearningEvidenceIneligible
	}
	definitions, err := assessments.List(repositoryID, attempt.AssessmentSlug)
	if err != nil || attempt.AssessmentVersion < 1 || attempt.AssessmentVersion != len(definitions) {
		return workspaces.ContributionLearningEvidence{}, nil, errLearningEvidenceIneligible
	}
	definition := definitions[attempt.AssessmentVersion-1]
	pathway, err := pathways.Current(repositoryID, definition.PathwaySlug)
	if err != nil || pathway.Version != definition.PathwayVersion || attempt.ProjectRevision != definition.ProjectRevision {
		return workspaces.ContributionLearningEvidence{}, nil, errLearningEvidenceIneligible
	}
	workspace, err := workspaceStore.Get(attempt.WorkspaceID)
	if err != nil || workspace.CreatorID != learnerID || workspace.LearningContext == nil || workspace.LearningContext.PathwaySlug != pathway.Slug || workspace.LearningContext.PathwayVersion != pathway.Version {
		return workspaces.ContributionLearningEvidence{}, nil, errLearningEvidenceIneligible
	}
	var module *learningpathways.Module
	for i := range pathway.Modules {
		if pathway.Modules[i].ID == workspace.LearningContext.ModuleID {
			module = &pathway.Modules[i]
			break
		}
	}
	if module == nil {
		return workspaces.ContributionLearningEvidence{}, nil, errLearningEvidenceIneligible
	}
	exerciseFound := false
	for _, exercise := range module.Exercises {
		if exercise.ID == workspace.LearningContext.ExerciseID {
			exerciseFound = true
			break
		}
	}
	if !exerciseFound {
		return workspaces.ContributionLearningEvidence{}, nil, errLearningEvidenceIneligible
	}
	evidence := workspaces.ContributionLearningEvidence{
		AssessmentSlug: attempt.AssessmentSlug, AssessmentVersion: attempt.AssessmentVersion, AttemptID: attempt.ID,
		PathwaySlug: pathway.Slug, PathwayVersion: pathway.Version, ModuleID: module.ID, ExerciseID: workspace.LearningContext.ExerciseID,
		CheckpointIDs: append([]string(nil), attempt.Evidence.CheckpointIDs...), CommandOutcomeIDs: append([]string(nil), attempt.Evidence.CommandOutcomeIDs...),
		AuthorshipStatement: authorship, AgentAssistanceDeclared: attempt.Evidence.AgentAssistanceDeclared,
	}
	skills := append([]string(nil), module.Objectives...)
	for _, criterion := range definition.Criteria {
		skills = append(skills, criterion.ID, criterion.Label)
	}
	return evidence, cleanContributionText(skills), nil
}
