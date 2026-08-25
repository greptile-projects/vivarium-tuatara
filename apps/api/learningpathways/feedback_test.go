package learningpathways

import "testing"

func TestFeedbackPreservesConsentPrivacyReviewAndRevalidation(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, actor := "11111111111111111111111111111111", "22222222222222222222222222222222"
	private, err := s.AddOutcome(Outcome{RequestID: "private-1", RepositoryID: repo, PathwaySlug: "contributor", PathwayVersion: 1, ActorID: actor, Kind: "recurring_question", State: "confused", Visibility: "private", Consent: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AddFinding(Finding{RequestID: "finding-private", RepositoryID: repo, PathwaySlug: "contributor", PathwayVersion: 1, Kind: "questions", Summary: "Private question", OutcomeIDs: []string{private.ID}, CreatedBy: "33333333333333333333333333333333"}); err != ErrInvalid {
		t.Fatalf("private outcome supported finding: %v", err)
	}
	shared, err := s.AddOutcome(Outcome{RequestID: "shared-1", RepositoryID: repo, PathwaySlug: "contributor", PathwayVersion: 1, ActorID: actor, Kind: "setup_failure", State: "failed", Visibility: "maintainers", Consent: true})
	if err != nil {
		t.Fatal(err)
	}
	finding, err := s.AddFinding(Finding{RequestID: "finding-1", RepositoryID: repo, PathwaySlug: "contributor", PathwayVersion: 1, Kind: "setup", Summary: "Setup repeatedly fails", OutcomeIDs: []string{shared.ID}, CreatedBy: "33333333333333333333333333333333"})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := s.AddProposal(UpdateProposal{RequestID: "proposal-1", RepositoryID: repo, PathwaySlug: "contributor", BaseVersion: 1, FindingID: finding.ID, TargetKind: "workspace", TargetID: "definition", Summary: "Correct setup image", MaterialRequirementChange: true, ProposedBy: "44444444444444444444444444444444"})
	if err != nil {
		t.Fatal(err)
	}
	reviewed, err := s.ReviewProposal(repo, "contributor", proposal.ID, "33333333333333333333333333333333", "accepted", "Supported by the cited setup outcome.")
	if err != nil || reviewed.Status != "accepted" || reviewed.ReviewedBy != "33333333333333333333333333333333" {
		t.Fatalf("review=%+v err=%v", reviewed, err)
	}
	items, err := s.Outcomes(repo, "contributor")
	if err != nil || len(items) != 2 || items[0].ID != private.ID {
		t.Fatalf("immutable outcomes=%+v err=%v", items, err)
	}
}

func TestFeedbackRequiresConsentAndStableLearnerRequest(t *testing.T) {
	s, _ := New(t.TempDir())
	base := Outcome{RequestID: "outcome-1", RepositoryID: "11111111111111111111111111111111", PathwaySlug: "path", PathwayVersion: 2, ActorID: "22222222222222222222222222222222", Kind: "retention", State: "returned", Visibility: "aggregate"}
	if _, err := s.AddOutcome(base); err != ErrInvalid {
		t.Fatalf("unconsented outcome: %v", err)
	}
	base.Consent = true
	first, err := s.AddOutcome(base)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := s.AddOutcome(base)
	if err != nil || retry.ID != first.ID {
		t.Fatalf("retry=%+v err=%v", retry, err)
	}
	base.State = "left"
	if _, err = s.AddOutcome(base); err != ErrRequestChanged {
		t.Fatalf("changed retry: %v", err)
	}
}
