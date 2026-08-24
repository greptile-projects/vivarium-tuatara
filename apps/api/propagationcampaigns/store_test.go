package propagationcampaigns

import (
	"errors"
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
