package checkruns

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	command := fmt.Sprintf(`test "$(cat value)" = candidate && test "$TOKEN" = bounded && test ! -d .git && test ! -e %q; sleep 30 &`, hostSecret)
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
	if len(got) != 1 || got[0].State != "succeeded" || got[0].StartedAt == nil || got[0].CompletedAt == nil {
		t.Fatalf("run = %#v", got)
	}
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
	if err != nil || len(final) != 1 || final[0].State != "succeeded" {
		t.Fatalf("recovered runs = %#v, %v", final, err)
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
