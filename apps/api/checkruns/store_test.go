package checkruns

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestParseConfigValidatesExecutionContext(t *testing.T) {
	config, err := ParseConfig([]byte(`{"version":1,"checks":[{"name":"test","image":"alpine:3.22","command":"test \"$MODE\" = ci","working_directory":"app","environment":{"MODE":"ci"},"timeout_seconds":30}]}`))
	if err != nil || len(config.Checks) != 1 || config.Checks[0].WorkingDirectory != "app" {
		t.Fatalf("ParseConfig() = %#v, %v", config, err)
	}
	for _, body := range []string{
		`{"version":2,"checks":[{"name":"test","image":"alpine:3.22","command":"true"}]}`,
		`{"version":1,"checks":[{"name":"test","image":"alpine:3.22","command":"true","working_directory":"../secret"}]}`,
		`{"version":1,"checks":[{"name":"test","image":"alpine:3.22","command":"true"},{"name":"test","image":"alpine:3.22","command":"false"}]}`,
	} {
		if _, err := ParseConfig([]byte(body)); err == nil {
			t.Fatalf("ParseConfig(%s) unexpectedly succeeded", body)
		}
	}
}

func TestEvidenceSequenceHandlesEscapedMaximumLogChunk(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runs, err := store.Create("0123456789abcdef0123456789abcdef", "abcdef0123456789abcdef0123456789", strings.Repeat("a", 40), []Definition{{Name: "logs", Image: "alpine:3.22", Command: "true"}})
	if err != nil {
		t.Fatal(err)
	}
	body := bytes.Repeat([]byte{0x00, 0xff}, 16*1024)
	writer := &evidenceWriter{store: store, run: runs[0], attempt: 1, stream: "stdout"}
	if written, err := writer.Write(body); err != nil || written != len(body) {
		t.Fatalf("Write() = %d, %v", written, err)
	}
	if err := store.appendEvent(runs[0], Event{Attempt: 1, Kind: "status", Timestamp: time.Now().UTC(), State: "succeeded"}); err != nil {
		t.Fatal(err)
	}
	events, err := store.Events(runs[0].RepositoryID, runs[0].PullRequestID, runs[0].ID, 0)
	if err != nil || len(events) != 3 || events[2].Sequence != 3 || events[2].State != "succeeded" {
		t.Fatalf("Events() = %#v, %v", events, err)
	}
}

func TestCollaboratorControlsPreserveAttemptsAndAttribution(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runs, err := store.Create("0123456789abcdef0123456789abcdef", "abcdef0123456789abcdef0123456789", strings.Repeat("d", 40), []Definition{{Name: "test", Image: "alpine:3.22", Command: "true"}})
	if err != nil {
		t.Fatal(err)
	}
	run := runs[0]
	now := time.Now().UTC()
	run.State, run.CompletedAt = "succeeded", &now
	run.Attempts = []Attempt{{Number: 1, State: "succeeded", StartedAt: now, CompletedAt: &now}}
	if err := store.Update(run); err != nil {
		t.Fatal(err)
	}
	actor := "11111111111111111111111111111111"
	queued, err := store.Rerun(run.RepositoryID, run.PullRequestID, run.ID, actor)
	if err != nil || queued.State != "queued" || queued.RequestedBy != actor || len(queued.Attempts) != 1 {
		t.Fatalf("Rerun() = %#v, %v", queued, err)
	}
	canceled, err := store.Cancel(run.RepositoryID, run.PullRequestID, run.ID, actor)
	if err != nil || canceled.State != "canceled" || len(canceled.Attempts) != 1 {
		t.Fatalf("Cancel() = %#v, %v", canceled, err)
	}
	events, err := store.Events(run.RepositoryID, run.PullRequestID, run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 || events[1].Kind != "control" || events[1].ActorID != actor || events[3].Message != "cancel" || events[3].ActorID != actor {
		t.Fatalf("events = %#v", events)
	}
	if _, err := store.Rerun(run.RepositoryID, run.PullRequestID, run.ID, actor); err != nil {
		t.Fatal(err)
	}
}

func TestRerunSurvivesEvidenceAppendFailureAndRepairsAttribution(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runs, err := store.Create("0123456789abcdef0123456789abcdef", "abcdef0123456789abcdef0123456789", strings.Repeat("e", 40), []Definition{{Name: "test", Image: "alpine:3.22", Command: "true"}})
	if err != nil {
		t.Fatal(err)
	}
	run, now := runs[0], time.Now().UTC()
	run.State, run.CompletedAt = "succeeded", &now
	if err := store.Update(run); err != nil {
		t.Fatal(err)
	}
	evidencePath := filepath.Join(store.root, run.RepositoryID, run.PullRequestID, run.ID+".events")
	if err := os.Remove(evidencePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(evidencePath, 0o700); err != nil {
		t.Fatal(err)
	}
	actor := "11111111111111111111111111111111"
	queued, err := store.Rerun(run.RepositoryID, run.PullRequestID, run.ID, actor)
	if err != nil || queued.State != "queued" || len(queued.Controls) != 1 {
		t.Fatalf("Rerun() = %#v, %v", queued, err)
	}
	if err := os.Remove(evidencePath); err != nil {
		t.Fatal(err)
	}
	events, err := store.Events(run.RepositoryID, run.PullRequestID, run.ID, 0)
	if err != nil || len(events) != 1 || events[0].Kind != "control" || events[0].ActorID != actor {
		t.Fatalf("Events() = %#v, %v", events, err)
	}
}

func TestCancellationIntentWinsExecutorFailureRace(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runs, err := store.Create("0123456789abcdef0123456789abcdef", "abcdef0123456789abcdef0123456789", strings.Repeat("f", 40), []Definition{{Name: "test", Image: "alpine:3.22", Command: "true"}})
	if err != nil {
		t.Fatal(err)
	}
	run, now := runs[0], time.Now().UTC()
	run.State, run.StartedAt, run.Attempts = "running", &now, []Attempt{{Number: 1, State: "running", StartedAt: now}}
	if err := store.Update(run); err != nil {
		t.Fatal(err)
	}
	lock, err := os.OpenFile(filepath.Join(store.root, run.RepositoryID, run.PullRequestID, run.ID+".execution.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	actor := "22222222222222222222222222222222"
	go func() {
		_, cancelErr := store.Cancel(run.RepositoryID, run.PullRequestID, run.ID, actor)
		result <- cancelErr
	}()
	intentPath := filepath.Join(store.root, run.RepositoryID, run.PullRequestID, run.ID+".cancel")
	deadline := time.Now().Add(time.Second)
	for {
		if _, statErr := os.Stat(intentPath); statErr == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("cancel intent was not published")
		}
		time.Sleep(time.Millisecond)
	}
	failed := run
	failed.State, failed.CompletedAt, failed.Failure = "failed", &now, "exit status 137"
	if err := store.Update(failed); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	lock.Close()
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	persisted, err := store.Get(run.RepositoryID, run.PullRequestID, run.ID)
	if err != nil || persisted.State != "canceled" || len(persisted.Controls) != 1 || persisted.Controls[0].ActorID != actor {
		t.Fatalf("run = %#v, %v", persisted, err)
	}
}

func TestCancellationIntentSurvivesUncertainTerminalPublication(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runs, err := store.Create("0123456789abcdef0123456789abcdef", "abcdef0123456789abcdef0123456789", strings.Repeat("1", 40), []Definition{{Name: "test", Image: "alpine:3.22", Command: "true"}})
	if err != nil {
		t.Fatal(err)
	}
	run := runs[0]
	control, err := newControl("cancel", "33333333333333333333333333333333", 0, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.writeCancelIntent(run, control); err != nil {
		t.Fatal(err)
	}
	store.syncDirectory = func(*os.File) error { return errors.New("injected post-rename sync failure") }
	if _, err := store.finalizeCancellation(run, control); err != nil {
		t.Fatal(err)
	}
	intent := filepath.Join(store.root, run.RepositoryID, run.PullRequestID, run.ID+".cancel")
	if _, err := os.Stat(intent); err != nil {
		t.Fatalf("cancel intent was removed after uncertain publication: %v", err)
	}
	recoverable, err := store.Nonterminal()
	if err != nil || len(recoverable) != 1 || recoverable[0].ID != run.ID {
		t.Fatalf("Nonterminal() = %#v, %v", recoverable, err)
	}
}

func TestConcurrentEventsRepairProjectsControlOnce(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runs, err := store.Create("0123456789abcdef0123456789abcdef", "abcdef0123456789abcdef0123456789", strings.Repeat("2", 40), []Definition{{Name: "test", Image: "alpine:3.22", Command: "true"}})
	if err != nil {
		t.Fatal(err)
	}
	run := runs[0]
	control, err := newControl("rerun", "44444444444444444444444444444444", 1, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	run.Controls = []Control{control}
	if err := store.Update(run); err != nil {
		t.Fatal(err)
	}
	evidence := filepath.Join(store.root, run.RepositoryID, run.PullRequestID, run.ID+".events")
	if err := os.Remove(evidence); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	errs := make(chan error, 16)
	for range 16 {
		go func() {
			<-start
			events, eventErr := store.Events(run.RepositoryID, run.PullRequestID, run.ID, 0)
			if eventErr == nil && len(events) != 1 {
				eventErr = fmt.Errorf("events = %#v", events)
			}
			errs <- eventErr
		}()
	}
	close(start)
	for range 16 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	events, err := store.readEvents(run)
	if err != nil || len(events) != 1 || events[0].ControlID != control.ID {
		t.Fatalf("persisted events = %#v, %v", events, err)
	}
}

func TestRecoveryDoesNotPublishInterruptedFailureBeforeRunUpdate(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runs, err := store.Create("0123456789abcdef0123456789abcdef", "abcdef0123456789abcdef0123456789", strings.Repeat("b", 40), []Definition{{Name: "recovery", Image: "alpine:3.22", Command: "true"}})
	if err != nil {
		t.Fatal(err)
	}
	run := runs[0]
	started := time.Now().UTC()
	run.State, run.StartedAt, run.Attempts = "running", &started, []Attempt{{Number: 1, State: "running", StartedAt: started}}
	if err := store.Update(run); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(store.root, run.RepositoryID, run.PullRequestID)
	if err := os.WriteFile(filepath.Join(directory, run.ID+".execution.lock"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(directory, 0o500); err != nil {
		t.Fatal(err)
	}
	store.Execute(run, t.TempDir())
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	events, err := store.Events(run.RepositoryID, run.PullRequestID, run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.Attempt == 1 && event.State == "failed" {
			t.Fatalf("published failure before run update: %#v", events)
		}
	}
	persisted, err := store.Get(run.RepositoryID, run.PullRequestID, run.ID)
	if err != nil || persisted.Attempts[0].State != "running" {
		t.Fatalf("run = %#v, %v", persisted, err)
	}
}

func TestRecoveryPublishesLifecycleAfterPostRenameDurabilityFailure(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runs, err := store.Create("0123456789abcdef0123456789abcdef", "abcdef0123456789abcdef0123456789", strings.Repeat("c", 40), []Definition{{Name: "recovery", Image: "alpine:3.22", Command: "true"}})
	if err != nil {
		t.Fatal(err)
	}
	run := runs[0]
	started := time.Now().UTC()
	run.State, run.StartedAt, run.Attempts = "running", &started, []Attempt{{Number: 1, State: "running", StartedAt: started}}
	if err := store.Update(run); err != nil {
		t.Fatal(err)
	}
	store.syncDirectory = func(*os.File) error { return errors.New("injected post-rename sync failure") }
	store.Execute(run, t.TempDir())
	events, err := store.Events(run.RepositoryID, run.PullRequestID, run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	var interrupted, replacement bool
	for _, event := range events {
		interrupted = interrupted || event.Attempt == 1 && event.Kind == "status" && event.State == "failed"
		replacement = replacement || event.Attempt == 2 && event.Kind == "status" && event.State == "running"
	}
	if !interrupted || !replacement {
		t.Fatalf("recovery events = %#v", events)
	}
	persisted, err := store.Get(run.RepositoryID, run.PullRequestID, run.ID)
	if err != nil || len(persisted.Attempts) != 2 || persisted.Attempts[0].State != "failed" {
		t.Fatalf("run = %#v, %v", persisted, err)
	}
}

func TestTerminalEvidenceWaitsForDefinitiveRunPublication(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runs, err := store.Create("0123456789abcdef0123456789abcdef", "abcdef0123456789abcdef0123456789", strings.Repeat("d", 40), []Definition{{Name: "terminal", Image: "alpine:3.22", Command: "false"}})
	if err != nil {
		t.Fatal(err)
	}
	run := runs[0]
	started := time.Now().UTC()
	run.State, run.StartedAt, run.Attempts = "running", &started, []Attempt{{Number: 1, State: "running", StartedAt: started}}
	if err := store.Update(run); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(store.root, run.RepositoryID, run.PullRequestID)
	if err := os.Chmod(directory, 0o500); err != nil {
		t.Fatal(err)
	}
	done, code := time.Now().UTC(), 1
	run.State, run.CompletedAt, run.ExitCode, run.Failure = "failed", &done, &code, "exit status 1"
	run.Attempts[0].State, run.Attempts[0].CompletedAt, run.Attempts[0].ExitCode, run.Attempts[0].Failure = "failed", &done, &code, run.Failure
	if store.publishTerminal(run, 1, done) {
		t.Fatal("terminal publication unexpectedly succeeded")
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	persisted, err := store.Get(run.RepositoryID, run.PullRequestID, run.ID)
	if err != nil || persisted.State != "running" || persisted.Attempts[0].State != "running" {
		t.Fatalf("run = %#v, %v", persisted, err)
	}
	events, err := store.Events(run.RepositoryID, run.PullRequestID, run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.Kind == "command" || event.State == "failed" {
			t.Fatalf("terminal evidence preceded durable state: %#v", events)
		}
	}
}

func TestExecuteUsesExactDisposableSnapshotAndPersistsLifecycle(t *testing.T) {
	repository := filepath.Join(t.TempDir(), "repository.git")
	runGit(t, "init", "--bare", repository)
	work := t.TempDir()
	runGit(t, "-C", work, "init")
	runGit(t, "-C", work, "config", "user.email", "test@example.com")
	runGit(t, "-C", work, "config", "user.name", "Test")
	if err := os.Mkdir(filepath.Join(work, "app"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "app", "value"), []byte("candidate\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit(t, "-C", work, "add", ".")
	runGit(t, "-C", work, "commit", "-m", "candidate")
	commit := gitOutput(t, "-C", work, "rev-parse", "HEAD")
	runGit(t, "-C", work, "push", repository, "HEAD:main")
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	hostSecret := filepath.Join(t.TempDir(), "host-secret")
	if err := os.WriteFile(hostSecret, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	command := fmt.Sprintf(`test "$(cat value)" = candidate && test "$TOKEN" = bounded && test ! -d .git && test ! -e %q && ! touch ./forbidden && touch "$VIVARIUM_OUTPUT/result"; sleep 30 &`, hostSecret)
	definitions := []Definition{{Name: "snapshot", Image: "alpine:3.22", Command: command, WorkingDirectory: "app", Environment: map[string]string{"TOKEN": "bounded"}, TimeoutSeconds: 10}}
	runs, err := store.Create("0123456789abcdef0123456789abcdef", "abcdef0123456789abcdef0123456789", commit, definitions)
	if err != nil {
		t.Fatal(err)
	}
	store.Execute(runs[0], repository)
	got, err := store.List(runs[0].RepositoryID, runs[0].PullRequestID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].State != "succeeded" || got[0].StartedAt == nil || got[0].CompletedAt == nil || len(got[0].Attempts) != 1 || len(got[0].Artifacts) != 1 {
		t.Fatalf("run = %#v", got)
	}
	events, err := store.Events(got[0].RepositoryID, got[0].PullRequestID, got[0].ID, 0)
	if err != nil || len(events) < 5 || events[0].State != "queued" || events[1].State != "running" || events[len(events)-1].State != "succeeded" {
		t.Fatalf("events = %#v, %v", events, err)
	}
	remaining, err := store.Events(got[0].RepositoryID, got[0].PullRequestID, got[0].ID, events[len(events)-1].Sequence)
	if err != nil || len(remaining) != 0 {
		t.Fatalf("reconnected events = %#v, %v", remaining, err)
	}
	artifact, metadata, err := store.OpenArtifact(got[0].RepositoryID, got[0].PullRequestID, got[0].ID, got[0].Artifacts[0].ID)
	if err != nil || metadata.Path != "result" {
		t.Fatalf("artifact = %#v, %v", metadata, err)
	}
	artifact.Close()
	if got[0].CompletedAt.Before(got[0].CreatedAt) || got[0].CompletedAt.After(time.Now().Add(time.Second)) {
		t.Fatalf("invalid lifecycle times: %#v", got[0])
	}
	if output, err := exec.Command("docker", "ps", "--quiet", "--filter", "name=vivarium-check-"+got[0].ID).Output(); err != nil || len(output) != 0 {
		t.Fatalf("check descendants retained: %q, %v", output, err)
	}

	// A process restart releases the execution lock; durable nonterminal work is
	// discovered and can be safely relaunched to a terminal result.
	abandoned := got[0]
	abandoned.State = "running"
	abandoned.CompletedAt = nil
	abandoned.ExitCode = nil
	if err := store.Update(abandoned); err != nil {
		t.Fatal(err)
	}
	resumable, err := store.Create(abandoned.RepositoryID, abandoned.PullRequestID, abandoned.CommitID, definitions)
	if err != nil || len(resumable) != 1 || resumable[0].ID != abandoned.ID {
		t.Fatalf("deduplicated resumable runs = %#v, %v", resumable, err)
	}
	reopened, err := New(store.root)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := reopened.Nonterminal()
	if err != nil || len(pending) != 1 {
		t.Fatalf("Nonterminal() = %#v, %v", pending, err)
	}
	reopened.Execute(pending[0], repository)
	final, err := reopened.List(abandoned.RepositoryID, abandoned.PullRequestID)
	if err != nil || len(final) != 1 || final[0].State != "succeeded" || len(final[0].Attempts) != 2 {
		t.Fatalf("recovered runs = %#v, %v", final, err)
	}
	cleanupPending := final[0]
	cleanupPending.State = "cleanup_pending"
	cleanupPending.CompletedAt = nil
	if err := reopened.Update(cleanupPending); err != nil {
		t.Fatal(err)
	}
	reopened.Execute(cleanupPending, repository)
	final, err = reopened.List(abandoned.RepositoryID, abandoned.PullRequestID)
	if err != nil || final[0].State != "succeeded" || final[0].CompletedAt == nil {
		t.Fatalf("cleanup recovery = %#v, %v", final, err)
	}

	// A failed forced removal must remain durable and nonterminal until a later
	// cleanup confirms that the named container is absent.
	fakeBin := t.TempDir()
	fakeDocker := filepath.Join(fakeBin, "docker")
	marker := filepath.Join(t.TempDir(), "container")
	failingDocker := `#!/bin/sh
case "$1" in
run) touch "$FAKE_CONTAINER"; exit 0 ;;
rm) exit 1 ;;
ps) test -e "$FAKE_CONTAINER" && echo retained; exit 0 ;;
esac
`
	if err := os.WriteFile(fakeDocker, []byte(failingDocker), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_CONTAINER", marker)
	t.Setenv("PATH", fakeBin+":"+os.Getenv("PATH"))
	retry := final[0]
	retry.State = "queued"
	retry.CompletedAt = nil
	retry.ExitCode = nil
	if err := reopened.Update(retry); err != nil {
		t.Fatal(err)
	}
	reopened.Execute(retry, repository)
	pendingCleanup, err := reopened.List(retry.RepositoryID, retry.PullRequestID)
	if err != nil || pendingCleanup[0].State != "cleanup_pending" || pendingCleanup[0].CleanupFailure == "" {
		t.Fatalf("failed cleanup = %#v, %v", pendingCleanup, err)
	}
	successfulDocker := `#!/bin/sh
case "$1" in
rm) rm -f "$FAKE_CONTAINER"; exit 0 ;;
ps) exit 0 ;;
esac
exit 0
`
	if err := os.WriteFile(fakeDocker, []byte(successfulDocker), 0o700); err != nil {
		t.Fatal(err)
	}
	reopened.Execute(pendingCleanup[0], repository)
	cleaned, err := reopened.List(retry.RepositoryID, retry.PullRequestID)
	if err != nil || cleaned[0].State != "succeeded" || cleaned[0].CleanupFailure != "" {
		t.Fatalf("retried cleanup = %#v, %v", cleaned, err)
	}
}

func runGit(t *testing.T, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}
func gitOutput(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(out[:len(out)-1])
}
