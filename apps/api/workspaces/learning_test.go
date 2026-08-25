package workspaces

import (
	"sync"
	"sync/atomic"
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

func TestLearningLaunchReconcilesAcrossStoreInstances(t *testing.T) {
	root := t.TempDir()
	firstStore, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	secondStore, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	input := Workspace{RepositoryID: "repo", CommitID: "revision", CreatorID: "learner", Source: Source{Kind: "learning_exercise", LearningRequestID: "shared-launch", LearningPathwaySlug: "api", LearningPathwayVersion: 1, LearningModuleID: "routing", LearningExerciseID: "trace"}, LearningContext: &LearningContext{ReproducibilitySHA256: "digest"}}
	type result struct {
		workspace Workspace
		reused    bool
		err       error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	launch := func(store *Store) {
		ready.Done()
		<-start
		w, reused, err := store.CreateLearning(input, []byte("definition"))
		results <- result{w, reused, err}
	}
	go launch(firstStore)
	go launch(secondStore)
	ready.Wait()
	close(start)
	a, b := <-results, <-results
	if a.err != nil || b.err != nil || a.workspace.ID != b.workspace.ID || a.reused == b.reused {
		t.Fatalf("cross-store launch was not reconciled: a=%#v b=%#v", a, b)
	}
	all, err := firstStore.ListAll()
	if err != nil || len(all) != 1 {
		t.Fatalf("cross-store launch retained %d attempts: %v", len(all), err)
	}
}

func TestLearningProvisioningReconcilesAcrossStoreInstances(t *testing.T) {
	root := t.TempDir()
	firstStore, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	secondStore, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	input := Workspace{RepositoryID: "repo", CommitID: "revision", CreatorID: "learner", Source: Source{Kind: "learning_exercise", LearningRequestID: "shared-provision", LearningPathwaySlug: "api", LearningPathwayVersion: 1, LearningModuleID: "routing", LearningExerciseID: "trace"}, LearningContext: &LearningContext{ReproducibilitySHA256: "digest"}}
	w, _, err := firstStore.CreateLearning(input, []byte("definition"))
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	var runs atomic.Int32
	provision := func(store *Store) {
		ready.Done()
		<-start
		_, _, err := store.ReconcileLearningProvisioning(w.ID, func() ([]SetupStep, bool) {
			runs.Add(1)
			time.Sleep(10 * time.Millisecond)
			return []SetupStep{{Command: "setup", State: "passed"}}, false
		})
		results <- err
	}
	go provision(firstStore)
	go provision(secondStore)
	ready.Wait()
	close(start)
	if err = <-results; err != nil {
		t.Fatal(err)
	}
	if err = <-results; err != nil {
		t.Fatal(err)
	}
	got, err := firstStore.Get(w.ID)
	if err != nil || runs.Load() != 1 || got.State != "running" || len(got.Setup) != 1 {
		t.Fatalf("cross-store provisioning duplicated: runs=%d workspace=%#v err=%v", runs.Load(), got, err)
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
	runs := 0
	second, ran, err := s.ReconcileLearningProvisioning(second.ID, func() ([]SetupStep, bool) {
		runs++
		return []SetupStep{{Command: "recover setup", State: "passed"}}, false
	})
	if err != nil || !ran || second.State != "running" || len(second.Setup) != 1 {
		t.Fatalf("provisioning retry was not recovered: %#v ran=%v err=%v", second, ran, err)
	}
	second, ran, err = s.ReconcileLearningProvisioning(second.ID, func() ([]SetupStep, bool) {
		runs++
		return nil, true
	})
	if err != nil || ran || second.State != "running" || runs != 1 {
		t.Fatalf("completed attempt provisioned again: %#v ran=%v runs=%d err=%v", second, ran, runs, err)
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

func TestLearningGuidanceRetainsSharedStateAndLearnerAgentControl(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	w, err := s.Create(Workspace{RepositoryID: "repo", CommitID: "revision", CreatorID: "learner", LearningContext: &LearningContext{AcceptanceCriteria: []string{"understands routing"}, Guidance: LearningGuidance{Version: 1}}}, []byte("definition"))
	if err != nil {
		t.Fatal(err)
	}
	w, err = s.RecordCommand(w.ID, CommandOutcome{ActorID: "learner"})
	if err != nil {
		t.Fatal(err)
	}
	w, err = s.AddLearningCheckpoint(w.ID, "learner", "I traced the route", []string{"understands routing"}, []string{w.Commands[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	w, err = s.AddLearningGuidance(w.ID, "learner", "learner", "", "question", "Why does this branch win?", nil, []string{w.LearningContext.Checkpoints[0].ID}, []string{w.Commands[0].ID}, 1)
	if err != nil || !w.LearningContext.Guidance.Entries[0].LearnerControlled {
		t.Fatalf("question = %#v, %v", w.LearningContext.Guidance, err)
	}
	w, err = s.AddLearningGuidance(w.ID, "mentor", "mentor", "", "hint", "Inspect registration order.", []LearningEvidenceCitation{{Path: "routes.go", Revision: "revision"}}, nil, nil, 2)
	if err != nil || w.LearningContext.Guidance.Entries[1].ActorKind != "mentor" {
		t.Fatalf("mentor help = %#v, %v", w.LearningContext.Guidance, err)
	}
	w, err = s.SetLearningAgent(w.ID, "learner", "approved-agent", "active", "Only ask questions about dispatch.", 3)
	if err != nil || w.LearningContext.Guidance.AgentState != "active" {
		t.Fatalf("agent activation = %#v, %v", w.LearningContext.Guidance, err)
	}
	w, err = s.SetLearningAgent(w.ID, "learner", "approved-agent", "paused", "", 4)
	if err != nil || w.LearningContext.Guidance.AgentState != "paused" {
		t.Fatalf("agent pause = %#v, %v", w.LearningContext.Guidance, err)
	}
	if _, err = s.SetLearningAgent(w.ID, "mentor", "approved-agent", "revoked", "", 5); err != ErrConflict {
		t.Fatalf("mentor controlled learner agent: %v", err)
	}
}

func TestLearningGuidanceRejectsUnsharedExerciseState(t *testing.T) {
	s, _ := New(t.TempDir())
	w, _ := s.Create(Workspace{RepositoryID: "repo", CommitID: "revision", CreatorID: "learner", LearningContext: &LearningContext{Guidance: LearningGuidance{Version: 1}}}, []byte("definition"))
	if _, err := s.AddLearningGuidance(w.ID, "learner", "learner", "", "question", "Here is unrelated state", nil, []string{"unknown"}, nil, 1); err != ErrInvalid {
		t.Fatalf("unknown checkpoint accepted: %v", err)
	}
}

func TestLearningAgentGuidanceRechecksGuideLeaseUnderMutationLock(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 25, 11, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	w, err := s.Create(Workspace{RepositoryID: "repo", CommitID: "revision", CreatorID: "learner", LearningContext: &LearningContext{Guidance: LearningGuidance{Version: 1}}}, []byte("definition"))
	if err != nil {
		t.Fatal(err)
	}
	w, err = s.SetLearningAgent(w.ID, "learner", "agent", "active", "Only provide routing hints.", 1)
	if err != nil {
		t.Fatal(err)
	}
	w, err = s.SetControl(w.ID, "learner", "approved_agent", "agent", "guide", []string{"files"}, w.Control.Version, 60)
	if err != nil {
		t.Fatal(err)
	}
	// The route may have authorized this request against the live snapshot above;
	// expiry does not advance the guidance version, so persistence must recheck it.
	now = now.Add(time.Minute)
	_, err = s.AddLearningGuidance(w.ID, "operator", "agent", "agent", "hint", "Inspect the router.", []LearningEvidenceCitation{{Path: "routes.go", Revision: "revision"}}, nil, nil, 2)
	if err != ErrControl {
		t.Fatalf("expired guide write = %v", err)
	}
	retained, err := s.Get(w.ID)
	if err != nil || len(retained.LearningContext.Guidance.Entries) != 0 {
		t.Fatalf("expired hint persisted: %#v, %v", retained.LearningContext.Guidance, err)
	}
}
