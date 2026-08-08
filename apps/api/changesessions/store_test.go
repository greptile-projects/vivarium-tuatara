package changesessions

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCreateReopenAndListTimeline(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Date(2026, 8, 7, 17, 0, 0, 123456789, time.UTC)
	store.now = func() time.Time { return clock }
	repositoryID := "11111111111111111111111111111111"
	pullID := "22222222222222222222222222222222"
	actorID := "33333333333333333333333333333333"
	revision := "4444444444444444444444444444444444444444"

	created, err := store.Create(repositoryID, pullID, actorID, revision)
	if err != nil {
		t.Fatal(err)
	}
	if created.State != Open || created.SourceCommitID != revision || created.InitiatorID != actorID {
		t.Fatalf("unexpected session: %+v", created)
	}

	reopened, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reopened.Get(repositoryID, pullID, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got != created {
		t.Fatalf("reopened session mismatch: %+v != %+v", got, created)
	}
	sessions, err := reopened.List(repositoryID, pullID)
	if err != nil || len(sessions) != 1 || sessions[0].ID != created.ID {
		t.Fatalf("sessions = %+v, %v", sessions, err)
	}
	events, err := reopened.ListEvents(repositoryID, pullID, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Kind != "session.opened" || events[0].ActorID != actorID || events[0].State != Open {
		t.Fatalf("events = %+v", events)
	}
}

func TestRecoverySessionFreezesDeploymentEvidence(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	repositoryID := "11111111111111111111111111111111"
	pullID := "22222222222222222222222222222222"
	actorID := "33333333333333333333333333333333"
	revision := "4444444444444444444444444444444444444444"
	evidence := &DeploymentEvidence{DeploymentID: "55555555555555555555555555555555", ReleaseID: "66666666666666666666666666666666", ReleaseVersion: "v1.2.3", ReleaseNotes: "Known regression context.", EnvironmentID: "77777777777777777777777777777777", ArtifactID: "88888888888888888888888888888888", ArtifactSHA256: strings.Repeat("a", 64), CommitID: revision, State: "failed", CurrentStage: 1, Evidence: []DeploymentSignal{{Stage: "canary", Signal: "errors", State: "failed", Message: "threshold exceeded"}}, Events: []DeploymentEvent{{Sequence: 4, Kind: "health.signal", State: "failed", Message: "threshold exceeded"}}}
	created, err := store.CreateWithRecoveryEvidence(repositoryID, pullID, actorID, revision, nil, evidence)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := reopened.Get(repositoryID, pullID, created.ID)
	if err != nil || loaded.DeploymentEvidence == nil || loaded.DeploymentEvidence.DeploymentID != evidence.DeploymentID || loaded.DeploymentEvidence.Events[0].Message != "threshold exceeded" {
		t.Fatalf("recovery evidence = %#v, %v", loaded.DeploymentEvidence, err)
	}
}

func TestCreateWithEvidencePersistsFailureContext(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	evidence := &CheckEvidence{
		RunID:      strings.Repeat("5", 32),
		Definition: CheckDefinition{Name: "api", Image: "vivarium/go:1.26", Command: "go test ./...", WorkingDirectory: "apps/api"},
		Events:     []CheckEvent{{Sequence: 4, Attempt: 1, Kind: "log", Stream: "stderr", Message: "FAIL\n"}},
		Artifacts:  []CheckArtifact{{ID: strings.Repeat("6", 32), Attempt: 1, Path: "report.xml", Size: 12, SHA256: strings.Repeat("a", 64), ContentType: "application/xml"}},
	}
	session, err := store.CreateWithEvidence(strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32), strings.Repeat("4", 40), evidence)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := store.Get(session.RepositoryID, session.PullRequestID, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.CheckEvidence == nil || reopened.CheckEvidence.RunID != evidence.RunID || reopened.CheckEvidence.Definition.Command != "go test ./..." || len(reopened.CheckEvidence.Events) != 1 || len(reopened.CheckEvidence.Artifacts) != 1 {
		t.Fatalf("failure evidence was not preserved: %#v", reopened.CheckEvidence)
	}
}

func TestCreateTaskSessionAndInitialRunAreOneRecord(t *testing.T) {
	store, _ := New(t.TempDir())
	repositoryID := strings.Repeat("1", 32)
	proposalID := strings.Repeat("2", 32)
	taskID := strings.Repeat("3", 32)
	actorID := strings.Repeat("4", 32)
	agentID := strings.Repeat("5", 32)
	revision := strings.Repeat("6", 40)
	credentialID := strings.Repeat("7", 32)
	context := TaskContext{RepositoryName: "repo", ProposalTitle: "Plan", ProposalBody: "Context", TaskTitle: "Build", TaskOutcome: "Works", Mandate: "Implement it"}
	session, run, err := store.CreateForTaskWithRun(repositoryID, proposalID, taskID, actorID, agentID, revision, context, []string{"README.md"}, "agent/tasks/work", credentialID, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if run.SessionID != session.ID || run.AgentID != agentID {
		t.Fatalf("session/run = %+v / %+v", session, run)
	}
	reopened, _ := New(store.root)
	runs, err := reopened.ListRuns(repositoryID, taskID, session.ID)
	if err != nil || len(runs) != 1 || runs[0].ID != run.ID {
		t.Fatalf("runs = %+v, %v", runs, err)
	}
	events, err := reopened.ListEvents(repositoryID, taskID, session.ID)
	if err != nil || len(events) != 2 || events[0].Kind != "session.opened" || events[1].Kind != "run.launched" {
		t.Fatalf("events = %+v, %v", events, err)
	}
}

func TestCreateReportsVisibleRecordWhenDirectorySyncFails(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	store.directorySync = func(string) error { return errors.New("injected sync failure") }
	session, err := store.Create("11111111111111111111111111111111", "22222222222222222222222222222222", "33333333333333333333333333333333", "4444444444444444444444444444444444444444")
	if !errors.Is(err, ErrDurabilityUncertain) || session.ID == "" {
		t.Fatalf("session = %+v, err = %v", session, err)
	}
	if _, statErr := filepath.Glob(filepath.Join(store.root, session.RepositoryID, session.PullRequestID, session.ID+".json")); statErr != nil {
		t.Fatal(statErr)
	}
	if _, getErr := store.Get(session.RepositoryID, session.PullRequestID, session.ID); !errors.Is(getErr, ErrDurabilityUncertain) {
		t.Fatalf("inspection error = %v, want durability uncertainty", getErr)
	}
	if _, eventsErr := store.ListEvents(session.RepositoryID, session.PullRequestID, session.ID); !errors.Is(eventsErr, ErrDurabilityUncertain) {
		t.Fatalf("timeline error = %v, want durability uncertainty", eventsErr)
	}
	store.directorySync = syncDirectory
	if _, getErr := store.Get(session.RepositoryID, session.PullRequestID, session.ID); getErr != nil {
		t.Fatalf("inspection did not reconcile durability: %v", getErr)
	}
}

func TestIsolationAndValidation(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("bad", "bad", "bad", "bad"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("err = %v", err)
	}
	sessions, err := store.List("11111111111111111111111111111111", "22222222222222222222222222222222")
	if err != nil || len(sessions) != 0 {
		t.Fatalf("sessions = %+v, err = %v", sessions, err)
	}
}

func TestCompletedRunCredentialIsDeniedByDurableState(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repositoryID := "11111111111111111111111111111111"
	pullID := "22222222222222222222222222222222"
	actorID := "33333333333333333333333333333333"
	base := "4444444444444444444444444444444444444444"
	head := "5555555555555555555555555555555555555555"
	credentialID := "66666666666666666666666666666666"
	session, err := store.Create(repositoryID, pullID, actorID, base)
	if err != nil {
		t.Fatal(err)
	}
	run, err := store.LaunchRun(repositoryID, pullID, session.ID, actorID, "Make the reviewable change.", base, []string{"README.md"}, "feature", credentialID, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	allowed, err := store.AllowsGitWrite(repositoryID, credentialID)
	if err != nil || !allowed {
		t.Fatalf("active run allowed = %v, %v", allowed, err)
	}
	completed, _, err := store.CompleteRun(repositoryID, pullID, session.ID, run.ID, credentialID, "Published reviewable work.", head, []string{head}, nil, []Check{{Name: "go test ./...", Status: "passed"}}, nil)
	if err != nil || completed.State != Completed {
		t.Fatalf("completion = %+v, %v", completed, err)
	}
	allowed, err = store.AllowsGitWrite(repositoryID, credentialID)
	if err != nil || allowed {
		t.Fatalf("completed run allowed = %v, %v", allowed, err)
	}
}
