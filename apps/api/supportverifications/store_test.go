package supportverifications

import (
	"strings"
	"testing"
	"time"
)

func TestImmutableAttemptLifecycle(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	v, err := s.Create(Attempt{RepositoryID: "repo", ThreadID: "thread", AnswerID: "answer", AnswerRevisionID: "revision", WorkspaceID: strings.Repeat("w", 32), CommitID: strings.Repeat("a", 40), DefinitionSHA256: strings.Repeat("b", 64), SoftwareVersion: "2.4.1", Environment: Environment{Runtime: "Go 1.24", Dependencies: []string{"module@2.4.1"}}, InputSHA256: strings.Repeat("c", 64), Instructions: "go test ./...", InstructionsSHA256: strings.Repeat("d", 64), Commands: []Command{{Command: "go test ./...", OutcomeID: "outcome", ExitCode: 0, Output: "ok", StartedAt: now, CompletedAt: now.Add(time.Second)}}, Artifacts: []Artifact{{Name: "report.txt", MediaType: "text/plain", Size: 2, SHA256: strings.Repeat("e", 64), Data: "b2s="}}, Cost: Cost{ComputeSeconds: 1, CostUnits: .01}, Result: "passed", ActorID: "runner"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Get("repo", "thread", v.ID)
	if err != nil || got.Instructions != "go test ./..." || got.Artifacts[0].ID == "" {
		t.Fatalf("got %#v, %v", got, err)
	}
	all, err := s.List("repo", "thread")
	if err != nil || len(all) != 1 || all[0].ID != v.ID {
		t.Fatalf("list %#v, %v", all, err)
	}
}

func TestAttemptRejectsIncompleteEvidence(t *testing.T) {
	s, _ := New(t.TempDir())
	if _, err := s.Create(Attempt{RepositoryID: "repo", ThreadID: "thread", Result: "passed"}); err != ErrInvalid {
		t.Fatalf("err = %v", err)
	}
}
