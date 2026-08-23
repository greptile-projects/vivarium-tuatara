package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestConflictHistoryStreamAndVerification(t *testing.T) {
	if err := exec.Command("docker", "image", "inspect", "alpine/git:latest").Run(); err != nil {
		t.Skip("alpine/git image is not available")
	}
	root := t.TempDir()
	work := filepath.Join(root, "work")
	bare := filepath.Join(root, "repo.git")
	run := func(args ...string) string {
		out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %s", err, out)
		}
		return strings.TrimSpace(string(out))
	}
	run("git", "init", work)
	run("git", "-C", work, "config", "user.name", "test")
	run("git", "-C", work, "config", "user.email", "test@example.com")
	run("git", "-C", work, "commit", "--allow-empty", "-m", "base")
	base := run("git", "-C", work, "rev-parse", "HEAD")
	run("git", "-C", work, "commit", "--allow-empty", "-m", "target")
	target := run("git", "-C", work, "rev-parse", "HEAD")
	run("git", "-C", work, "checkout", "-b", "source", base)
	run("git", "-C", work, "commit", "--allow-empty", "-m", "source")
	source := run("git", "-C", work, "rev-parse", "HEAD")
	run("git", "clone", "--bare", work, bare)
	name := "vivarium-workspace-manualstage"
	defer exec.Command("docker", "rm", "-f", name).Run()
	run("docker", "create", "--name", name, "--entrypoint", "sh", "alpine/git:latest", "-c", "while :; do sleep 3600; done")
	run("docker", "start", name)
	if err := stageConflictHistories(bare, "manualstage", source, target); err != nil {
		t.Fatal(err)
	}
	if got := run("docker", "exec", "--workdir", "/workspace", name, "git", "rev-parse", "refs/remotes/conflict/source"); got != source {
		t.Fatalf("source=%s", got)
	}
	if got := run("docker", "exec", "--workdir", "/workspace", name, "git", "rev-parse", "HEAD"); got != target {
		t.Fatalf("target=%s", got)
	}
}
