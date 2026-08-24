package propagationcampaigns

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func validCampaign() Campaign {
	return Campaign{RequestID: "request-1", RepositoryID: "repo-1", Title: "Carry the parser repair", Intent: "Every maintained line rejects the malformed input.", AcceptanceCriteria: []string{"The shared reproduction passes."}, Source: Source{Kind: "merged_pull", ResourceID: "pull-1", RepositoryID: "repo-1", Commits: []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, Label: "reviewed repair"}, Targets: []Target{{ID: "target-1", Kind: "repository", RepositoryID: "repo-1", ReleaseLine: "release/1.x", OwnerIDs: []string{"owner-1"}, Deadline: time.Now().UTC().Add(24 * time.Hour)}}, CompletionPolicy: CompletionPolicy{Mode: "all", RequireAcceptance: true}}
}

func TestCreateReconcilesStableRequestAndRetainsBoundary(t *testing.T) {
	s, e := New(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	v, e := s.Create(validCampaign(), "owner-1", "digest-1")
	if e != nil {
		t.Fatal(e)
	}
	again, e := s.Create(validCampaign(), "owner-1", "digest-1")
	if e != nil {
		t.Fatal(e)
	}
	if v.ID != again.ID || v.CreatedBy != "owner-1" || len(v.Targets) != 1 {
		t.Fatalf("campaign was not retained: %#v", again)
	}
	changed := validCampaign()
	changed.Title = "changed"
	if _, e = s.Create(changed, "owner-1", "digest-2"); !errors.Is(e, ErrConflict) {
		t.Fatalf("want conflict, got %v", e)
	}
}

func TestValidationRejectsInvalidSequencingAndCompletion(t *testing.T) {
	s, _ := New(t.TempDir())
	v := validCampaign()
	v.Targets[0].DependsOn = []string{"target-1"}
	if _, e := s.Create(v, "owner-1", "one"); !errors.Is(e, ErrInvalid) {
		t.Fatalf("want invalid self dependency, got %v", e)
	}
	v = validCampaign()
	v.Targets = append(v.Targets, Target{ID: "target-2", Kind: "repository", RepositoryID: "repo-2", ReleaseLine: "release/2.x", OwnerIDs: []string{"owner-2"}, Deadline: time.Now().UTC().Add(48 * time.Hour), DependsOn: []string{"target-1"}})
	v.Targets[0].DependsOn = []string{"target-2"}
	if _, e := s.Create(v, "owner-1", "cycle"); !errors.Is(e, ErrInvalid) {
		t.Fatalf("want invalid dependency cycle, got %v", e)
	}
	v = validCampaign()
	v.CompletionPolicy = CompletionPolicy{Mode: "minimum", MinimumTargets: 2}
	if _, e := s.Create(v, "owner-1", "two"); !errors.Is(e, ErrInvalid) {
		t.Fatalf("want invalid minimum, got %v", e)
	}
}

func TestListIsRepositoryScoped(t *testing.T) {
	s, _ := New(t.TempDir())
	a := validCampaign()
	a.RequestID = "a"
	if _, e := s.Create(a, "owner-1", "a"); e != nil {
		t.Fatal(e)
	}
	b := validCampaign()
	b.RepositoryID = "repo-2"
	b.Source.RepositoryID = "repo-2"
	b.RequestID = "b"
	if _, e := s.Create(b, "owner-2", "b"); e != nil {
		t.Fatal(e)
	}
	values, e := s.List("repo-1")
	if e != nil || len(values) != 1 || values[0].RepositoryID != "repo-1" {
		t.Fatalf("unexpected list: %#v %v", values, e)
	}
}

func TestAssessmentLedgerIsRevisionBoundAndCASVersioned(t *testing.T) {
	s, _ := New(t.TempDir())
	v, e := s.Create(validCampaign(), "owner-1", "campaign")
	if e != nil {
		t.Fatal(e)
	}
	comparisons := make([]Comparison, 7)
	for i, kind := range []string{"histories", "symbols", "dependencies", "interfaces", "schemas", "prior_fixes", "release_commitments"} {
		comparisons[i] = Comparison{Kind: kind, Status: "review_required", Summary: "bounded comparison"}
	}
	v, assessment, e := s.CreateAssessment(v.RepositoryID, v.ID, "owner-1", Assessment{TargetID: "target-1", Classification: "adaptation_required", TargetRevision: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", SourceRevision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Comparisons: comparisons})
	if e != nil || assessment.Version != 1 || len(v.Assessments) != 1 {
		t.Fatalf("assessment not retained: %#v %v", assessment, e)
	}
	entry := AssessmentEntry{Kind: "risk", Body: "The older parser exposes a different callback.", Citations: []Citation{{Kind: "commit", Reference: "target tip", Revision: assessment.TargetRevision}}}
	_, assessment, e = s.AddAssessmentEntry(v.RepositoryID, v.ID, assessment.ID, "agent-1", "read_only_agent", 1, entry)
	if e != nil || assessment.Version != 2 || assessment.Entries[0].ActorKind != "read_only_agent" {
		t.Fatalf("entry not retained: %#v %v", assessment, e)
	}
	if _, _, e = s.AddAssessmentEntry(v.RepositoryID, v.ID, assessment.ID, "owner-1", "human", 1, entry); !errors.Is(e, ErrVersion) {
		t.Fatalf("want CAS conflict, got %v", e)
	}
}

func TestContributionBindsCurrentAssessmentAndReconcilesTarget(t *testing.T) {
	s, _ := New(t.TempDir())
	v, _ := s.Create(validCampaign(), "owner-1", "campaign")
	comparisons := make([]Comparison, 7)
	for i, kind := range []string{"histories", "symbols", "dependencies", "interfaces", "schemas", "prior_fixes", "release_commitments"} {
		comparisons[i] = Comparison{Kind: kind, Status: "review_required", Summary: "bounded comparison"}
	}
	v, assessment, err := s.CreateAssessment(v.RepositoryID, v.ID, "owner-1", Assessment{TargetID: "target-1", Classification: "adaptation_required", TargetRevision: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", SourceRevision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Comparisons: comparisons})
	if err != nil {
		t.Fatal(err)
	}
	in := Contribution{TargetID: "target-1", AssessmentID: assessment.ID, AssessmentVersion: assessment.Version, TargetRevision: assessment.TargetRevision, Application: "adapted", Deviation: "Use the release-line callback.", Topology: "fork", Constraints: []string{"dependency unavailable"}, ProposalID: "cccccccccccccccccccccccccccccccc", TaskIDs: []string{"dddddddddddddddddddddddddddddddd"}}
	updated, created, err := s.LinkContribution(v.RepositoryID, v.ID, "owner-1", in)
	if err != nil || len(updated.Contributions) != 1 || created.PublishedBy != "owner-1" || created.Authority == "" {
		t.Fatalf("contribution not retained: %#v %v", created, err)
	}
	_, again, err := s.LinkContribution(v.RepositoryID, v.ID, "owner-1", in)
	if err != nil || again.ID != created.ID {
		t.Fatalf("retry did not reconcile: %#v %v", again, err)
	}
	changed := in
	changed.ProposalID = "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	if _, _, err = s.LinkContribution(v.RepositoryID, v.ID, "owner-1", changed); !errors.Is(err, ErrConflict) {
		t.Fatalf("want conflict, got %v", err)
	}
	stale := in
	stale.TargetID = "target-2"
	if _, _, err = s.LinkContribution(v.RepositoryID, v.ID, "owner-1", stale); !errors.Is(err, ErrInvalid) {
		t.Fatalf("want invalid stale link, got %v", err)
	}
}

func TestEquivalenceProofRequiresSubstitutesAndOwnerDecisionIsCASVersioned(t *testing.T) {
	s, _ := New(t.TempDir())
	v, _ := s.Create(validCampaign(), "owner-1", "campaign")
	proof := EquivalenceProof{RequestID: "proof-1", TargetID: "target-1", TargetRevision: strings.Repeat("b", 40), SourceRevision: strings.Repeat("a", 40), SourceAssumptionsSHA256: strings.Repeat("c", 64), DependencySHA256: strings.Repeat("d", 64), EvidenceRequirements: []string{"malformed input is rejected"}, Scenarios: []EquivalenceScenario{{Name: "regression", SourceCommand: "go test ./...", Coverage: []string{"malformed input"}, State: "unsupported"}}, OrdinaryChecks: []OrdinaryCheck{{Name: "unit", Command: "go test ./...", State: "succeeded", CheckRunID: "run-1"}}, State: "residual_differences"}
	if _, _, err := s.CreateEquivalenceProof(v.RepositoryID, v.ID, "owner-1", "digest", proof); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unsupported source scenario without substitute evidence must fail: %v", err)
	}
	proof.Scenarios[0].SubstituteEvidence = []Citation{{Kind: "target_check", Reference: "compatibility suite", Revision: proof.TargetRevision}}
	updated, created, err := s.CreateEquivalenceProof(v.RepositoryID, v.ID, "owner-1", "digest", proof)
	if err != nil || len(updated.EquivalenceProofs) != 1 || created.Version != 1 || created.Authority == "" {
		t.Fatalf("proof not retained: %#v %v", created, err)
	}
	_, decided, err := s.DecideEquivalenceProof(v.RepositoryID, v.ID, created.ID, "owner-1", "accepted", "The substitute covers the unsupported runtime.", 1)
	if err != nil || decided.Version != 2 || decided.State != "residual_differences" || len(decided.OwnerDecisions) != 1 {
		t.Fatalf("decision not retained without erasing residual state: %#v %v", decided, err)
	}
	if _, _, err = s.DecideEquivalenceProof(v.RepositoryID, v.ID, created.ID, "owner-1", "rejected", "stale view", 1); !errors.Is(err, ErrProofVersion) {
		t.Fatalf("want proof CAS conflict, got %v", err)
	}
}
