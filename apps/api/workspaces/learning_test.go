package workspaces

import (
	"testing"
	"time"
)

func TestLearningAttemptRetainsHintsCheckpointsCostAndReproduction(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	w, err := s.Create(Workspace{RepositoryID: "repo", CommitID: "revision", CreatorID: "learner", Definition: Definition{Resources: Resources{CPUs: 2, MemoryMB: 1024}}, LearningContext: &LearningContext{PathwaySlug: "api", PathwayVersion: 2, ModuleID: "routing", ExerciseID: "trace", Hints: []string{"Inspect the mux first"}, AcceptanceCriteria: []string{"Focused test passes"}, ReproducibilitySHA256: "digest"}}, []byte(`{"version":1}`))
	if err != nil {
		t.Fatal(err)
	}
	start := now
	now = now.Add(time.Minute)
	w, err = s.RecordCommand(w.ID, CommandOutcome{ActorID: "learner", StartedAt: start, CompletedAt: now})
	if err != nil || w.LearningContext.Cost <= 0 {
		t.Fatalf("command cost was not retained: %#v %v", w.LearningContext, err)
	}
	w, err = s.UseLearningHint(w.ID, "learner", 0)
	if err != nil || len(w.LearningContext.HintsUsed) != 1 {
		t.Fatalf("hint was not retained: %#v %v", w.LearningContext, err)
	}
	w, err = s.AddLearningCheckpoint(w.ID, "learner", "The focused behavior passes", []string{"Focused test passes"}, []string{w.Commands[0].ID})
	if err != nil || len(w.LearningContext.Checkpoints) != 1 || w.LearningContext.ReproducibilitySHA256 != "digest" {
		t.Fatalf("checkpoint was not retained: %#v %v", w.LearningContext, err)
	}
	if _, err = s.AddLearningCheckpoint(w.ID, "learner", "unsupported", []string{"Not a criterion"}, nil); err != ErrInvalid {
		t.Fatalf("uncited criterion accepted: %v", err)
	}
}

func TestLearningLaunchRequestReconcilesAtomically(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	input := Workspace{RepositoryID: "repo", CommitID: "revision", CreatorID: "learner", Source: Source{Kind: "learning_exercise", LearningRequestID: "launch-1", LearningPathwaySlug: "api", LearningPathwayVersion: 1, LearningModuleID: "routing", LearningExerciseID: "trace"}, LearningContext: &LearningContext{ReproducibilitySHA256: "digest"}}
	first, reused, err := s.CreateLearning(input, []byte("definition"))
	if err != nil || reused {
		t.Fatalf("first launch: reused=%v err=%v", reused, err)
	}
	second, reused, err := s.CreateLearning(input, []byte("definition"))
	if err != nil || !reused || second.ID != first.ID {
		t.Fatalf("retry was not reconciled: %#v %v %v", second, reused, err)
	}
	changed := input
	changed.CommitID = "other"
	if _, _, err = s.CreateLearning(changed, []byte("definition")); err != ErrRequestChanged {
		t.Fatalf("changed request accepted: %v", err)
	}
	all, _ := s.ListAll()
	if len(all) != 1 {
		t.Fatalf("retry created %d attempts", len(all))
	}
}

func TestLearningCheckpointCitesDurableCommandBeyondProjection(t *testing.T) {
	root := t.TempDir()
	s, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	w, err := s.Create(Workspace{RepositoryID: "repo", CommitID: "revision", CreatorID: "learner", LearningContext: &LearningContext{AcceptanceCriteria: []string{"passes"}}}, []byte("definition"))
	if err != nil {
		t.Fatal(err)
	}
	oldest := ""
	for i := 0; i < 101; i++ {
		w, err = s.RecordCommand(w.ID, CommandOutcome{ActorID: "learner"})
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			oldest = w.Commands[0].ID
		}
	}
	if len(w.Commands) != 100 {
		t.Fatalf("projection has %d commands", len(w.Commands))
	}
	reopened, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	w, err = reopened.AddLearningCheckpoint(w.ID, "learner", "old evidence remains valid", []string{"passes"}, []string{oldest})
	if err != nil || len(w.LearningContext.Checkpoints) != 1 {
		t.Fatalf("durable outcome rejected: %v", err)
	}
}
