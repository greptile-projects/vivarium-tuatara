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
