package learningpathways

import (
	"os"
	"path/filepath"
	"testing"
)

func seedPathwayRevision(t *testing.T, s *Store, repo, slug string, version int) {
	t.Helper()
	dir := filepath.Join(s.root, repo, slug)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(dir, "revision-000000001.json"), Revision{RepositoryID: repo, Slug: slug, Version: version}); err != nil {
		t.Fatal(err)
	}
}

func TestFeedbackPreservesConsentPrivacyReviewAndRevalidation(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, actor := "11111111111111111111111111111111", "22222222222222222222222222222222"
	seedPathwayRevision(t, s, repo, "contributor", 1)
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

func TestAggregateFindingsRequireThreeDistinctLearners(t *testing.T) {
	s, _ := New(t.TempDir())
	repo, slug := "11111111111111111111111111111111", "path"
	seedPathwayRevision(t, s, repo, slug, 1)
	actors := []string{"22222222222222222222222222222222", "33333333333333333333333333333333", "44444444444444444444444444444444"}
	first := Outcome{}
	for i := 0; i < 3; i++ {
		x, err := s.AddOutcome(Outcome{RequestID: "same-actor-" + string(rune('a'+i)), RepositoryID: repo, PathwaySlug: slug, PathwayVersion: 1, ActorID: actors[0], Kind: "setup_failure", State: "failed", Visibility: "aggregate", Consent: true})
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first = x
		}
	}
	base := Finding{RequestID: "finding-one-actor", RepositoryID: repo, PathwaySlug: slug, PathwayVersion: 1, Kind: "setup", Summary: "Setup fails", OutcomeIDs: []string{first.ID}, CreatedBy: "55555555555555555555555555555555"}
	if _, err := s.AddFinding(base); err != ErrInvalid {
		t.Fatalf("one learner met cohort threshold: %v", err)
	}
	for i := 1; i < 3; i++ {
		if _, err := s.AddOutcome(Outcome{RequestID: "distinct-" + string(rune('a'+i)), RepositoryID: repo, PathwaySlug: slug, PathwayVersion: 1, ActorID: actors[i], Kind: "setup_failure", State: "failed", Visibility: "aggregate", Consent: true}); err != nil {
			t.Fatal(err)
		}
	}
	base.RequestID = "finding-three-actors"
	if _, err := s.AddFinding(base); err != nil {
		t.Fatalf("three learners did not meet threshold: %v", err)
	}
}

func TestFindingsRequireExistingMatchingPathwayRevision(t *testing.T) {
	s, _ := New(t.TempDir())
	repo, slug := "11111111111111111111111111111111", "path"
	seedPathwayRevision(t, s, repo, slug, 1)
	outcome, err := s.AddOutcome(Outcome{RequestID: "outcome-revision", RepositoryID: repo, PathwaySlug: slug, PathwayVersion: 1, ActorID: "22222222222222222222222222222222", Kind: "setup_failure", State: "failed", Visibility: "maintainers", Consent: true})
	if err != nil {
		t.Fatal(err)
	}
	base := Finding{RequestID: "wrong-revision", RepositoryID: repo, PathwaySlug: slug, PathwayVersion: 2, Kind: "setup", Summary: "Setup fails", OutcomeIDs: []string{outcome.ID}, CreatedBy: "33333333333333333333333333333333"}
	if _, err = s.AddFinding(base); err != ErrInvalid {
		t.Fatalf("cross-revision finding: %v", err)
	}
	base.RequestID, base.PathwayVersion = "missing-revision", 999
	if _, err = s.AddFinding(base); err != ErrInvalid {
		t.Fatalf("missing-revision finding: %v", err)
	}
}

func TestFeedbackRetriesRejectEveryChangedSemanticPayload(t *testing.T) {
	s, _ := New(t.TempDir())
	repo, slug := "11111111111111111111111111111111", "path"
	seedPathwayRevision(t, s, repo, slug, 1)
	o := Outcome{RequestID: "outcome-retry", RepositoryID: repo, PathwaySlug: slug, PathwayVersion: 1, ModuleID: "intro", ActorID: "22222222222222222222222222222222", Kind: "setup_failure", State: "failed", Detail: "Image missing", Visibility: "maintainers", Consent: true}
	stored, err := s.AddOutcome(o)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Outcome){"visibility": func(x *Outcome) { x.Visibility = "private" }, "detail": func(x *Outcome) { x.Detail = "Other detail" }, "module": func(x *Outcome) { x.ModuleID = "advanced" }} {
		changed := o
		mutate(&changed)
		if _, err = s.AddOutcome(changed); err != ErrRequestChanged {
			t.Errorf("changed %s retry: %v", name, err)
		}
	}
	f := Finding{RequestID: "finding-retry", RepositoryID: repo, PathwaySlug: slug, PathwayVersion: 1, Kind: "setup", Summary: "Setup fails", OutcomeIDs: []string{stored.ID}, CreatedBy: "33333333333333333333333333333333"}
	finding, err := s.AddFinding(f)
	if err != nil {
		t.Fatal(err)
	}
	changedFinding := f
	changedFinding.Summary = "Different finding"
	if _, err = s.AddFinding(changedFinding); err != ErrRequestChanged {
		t.Fatalf("changed finding retry: %v", err)
	}
	p := UpdateProposal{RequestID: "proposal-retry", RepositoryID: repo, PathwaySlug: slug, BaseVersion: 1, FindingID: finding.ID, TargetKind: "documentation", TargetID: "guide", Summary: "Correct guide", ProposedBy: "44444444444444444444444444444444"}
	if _, err = s.AddProposal(p); err != nil {
		t.Fatal(err)
	}
	p.TargetID = "other-guide"
	if _, err = s.AddProposal(p); err != ErrRequestChanged {
		t.Fatalf("changed proposal retry: %v", err)
	}
}
