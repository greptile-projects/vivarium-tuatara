package learningassessments

import "testing"

func TestAssessmentVersionsAttemptsAndReviewEvidence(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	d := Definition{RequestID: "publish-1", RepositoryID: "repo", Slug: "practical", PathwaySlug: "contributor", PathwayVersion: 2, ProjectRevision: "0123456789012345678901234567890123456789", Title: "Practical", Criteria: []Criterion{{ID: "debug", Label: "Debug", Description: "Diagnoses the project failure", Weight: 1, Required: true}}, ProtectedCases: []ProtectedCase{{ID: "novel", Description: "Unseen failure", Expected: "repair without fixture knowledge"}}, RequiredChecks: []string{"test"}, RetryPolicy: RetryPolicy{MaximumAttempts: 2}, PublishedBy: "owner"}
	published, err := s.Publish(d, 0)
	if err != nil || published.Version != 1 {
		t.Fatalf("publish = %#v, %v", published, err)
	}
	if _, err = s.Publish(d, 0); err != nil {
		t.Fatalf("retry = %v", err)
	}
	a, err := s.CreateAttempt(Attempt{RequestID: "attempt-1", RepositoryID: "repo", AssessmentSlug: "practical", AssessmentVersion: 1, WorkspaceID: "workspace-1", LearnerID: "learner", ProjectRevision: d.ProjectRevision, ReproducibilitySHA256: "digest", Evidence: Evidence{CheckpointIDs: []string{"cp"}, AuthorshipStatement: "my work"}}, 2, 0)
	if err != nil || a.AttemptNumber != 1 || a.Status != "submitted" {
		t.Fatalf("attempt = %#v, %v", a, err)
	}
	a, err = s.UpdateAttempt("repo", "practical", a.ID, func(a *Attempt) error {
		a.Status = "demonstrated"
		a.Reviews = append(a.Reviews, Review{ID: "review", ReviewerID: "owner", Decisions: []RubricDecision{{CriterionID: "debug", Decision: "met", Rationale: "evidence", Confidence: "high"}}, Feedback: "Ready", Outcome: "demonstrated"})
		return nil
	})
	if err != nil || a.Reviews[0].CreatedAt.IsZero() {
		t.Fatalf("review = %#v, %v", a, err)
	}
	if _, err = s.CreateAttempt(Attempt{RequestID: "attempt-2", RepositoryID: "repo", AssessmentSlug: "practical", AssessmentVersion: 1, WorkspaceID: "workspace-2", LearnerID: "learner"}, 2, 0); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateAttempt(Attempt{RequestID: "attempt-3", RepositoryID: "repo", AssessmentSlug: "practical", AssessmentVersion: 1, WorkspaceID: "workspace-3", LearnerID: "learner"}, 2, 0); err != ErrInvalid {
		t.Fatalf("retry limit = %v", err)
	}
}
