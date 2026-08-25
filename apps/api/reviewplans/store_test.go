package reviewplans

import (
	"errors"
	"os"
	"testing"
)

func TestPlansRetainVersionsAndProjectMovementAsAttributedGap(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	base := Plan{RequestID: "request-one", RepositoryID: "repo", PullRequestID: "pull", SourceRevision: "1111111111111111111111111111111111111111", TargetRevision: "2222222222222222222222222222222222222222", Intent: "preserve behavior", ChangedPaths: []string{"api/auth.go"}, Areas: []Area{{ID: "security", Title: "Security", Questions: []string{"safe?"}, Evidence: []Evidence{{Kind: "test", Description: "proof", Required: true}}, CompletionRule: "question answered"}}, CompletionRule: "all areas complete", CreatedBy: "owner"}
	first, err := s.Create(base)
	if err != nil {
		t.Fatal(err)
	}
	base.RequestID = "request-two"
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

func TestCreateDoesNotAcknowledgeFailedFileSync(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.syncFile = func(*os.File) error { return errors.New("disk unavailable") }
	if _, err = s.Create(validTestPlan()); err == nil {
		t.Fatal("create acknowledged an unsynced file")
	}
	values, err := s.List("repo", "pull", validTestPlan().SourceRevision, validTestPlan().TargetRevision)
	if err != nil || len(values) != 0 {
		t.Fatalf("failed publication became visible: %#v, %v", values, err)
	}
}

func TestCreateDoesNotAcknowledgeFailedDirectorySync(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.syncDir = func(*os.File) error { return errors.New("directory unavailable") }
	plan := validTestPlan()
	if _, err = s.Create(plan); err == nil {
		t.Fatal("create acknowledged an unsynced directory entry")
	}
	s.syncDir = func(directory *os.File) error { return directory.Sync() }
	reconciled, err := s.Create(plan)
	if err != nil || reconciled.Version != 1 {
		t.Fatalf("ambiguous publication did not reconcile: %#v, %v", reconciled, err)
	}
	values, err := s.List("repo", "pull", plan.SourceRevision, plan.TargetRevision)
	if err != nil || len(values) != 1 {
		t.Fatalf("retry duplicated the plan: %#v, %v", values, err)
	}
}

func TestRequestIdentityRejectsChangedReuse(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	plan := validTestPlan()
	if _, err = s.Create(plan); err != nil {
		t.Fatal(err)
	}
	plan.Intent = "different intent"
	if _, err = s.Create(plan); !errors.Is(err, ErrInvalid) {
		t.Fatalf("changed request identity was reused: %v", err)
	}
}

func validTestPlan() Plan {
	return Plan{RequestID: "stable-request", RepositoryID: "repo", PullRequestID: "pull", SourceRevision: "1111111111111111111111111111111111111111", TargetRevision: "2222222222222222222222222222222222222222", Intent: "preserve behavior", Areas: []Area{{ID: "code", Title: "Code", Questions: []string{"safe?"}, Evidence: []Evidence{{Kind: "test", Description: "proof", Required: true}}, CompletionRule: "answered"}}, CompletionRule: "complete", CreatedBy: "owner"}
}
