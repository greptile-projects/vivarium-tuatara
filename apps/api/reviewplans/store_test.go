package reviewplans

import "testing"

func TestPlansRetainVersionsAndProjectMovementAsAttributedGap(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	base := Plan{RepositoryID: "repo", PullRequestID: "pull", SourceRevision: "1111111111111111111111111111111111111111", TargetRevision: "2222222222222222222222222222222222222222", Intent: "preserve behavior", ChangedPaths: []string{"api/auth.go"}, Areas: []Area{{ID: "security", Title: "Security", Questions: []string{"safe?"}, Evidence: []Evidence{{Kind: "test", Description: "proof", Required: true}}, CompletionRule: "question answered"}}, CompletionRule: "all areas complete", CreatedBy: "owner"}
	first, err := s.Create(base)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Create(base)
	if err != nil {
		t.Fatal(err)
	}
	if first.Version != 1 || second.Version != 2 || first.ID == second.ID {
		t.Fatalf("unexpected versions: %#v %#v", first, second)
	}
	values, err := s.List("repo", "pull", "3333333333333333333333333333333333333333", base.TargetRevision)
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 2 || !values[0].Stale || values[0].Diagnostics[len(values[0].Diagnostics)-1].Code != "stale_analysis" || values[0].Diagnostics[len(values[0].Diagnostics)-1].AttributedTo != "owner" {
		t.Fatalf("movement was hidden: %#v", values)
	}
}

func TestNormalizeMakesDerivedScopeStable(t *testing.T) {
	got := Normalize([]string{" b ", "a", "b", ""})
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("unexpected normalization: %#v", got)
	}
}
