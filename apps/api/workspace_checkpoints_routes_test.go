package main

import (
	"bytes"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/changesessions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
)

func TestCompareCheckpointTreesCapturesDiffWithoutCredentials(t *testing.T) {
	base, runtime := filepath.Join(t.TempDir(), "base"), filepath.Join(t.TempDir(), "runtime")
	os.MkdirAll(base, 0700)
	os.MkdirAll(runtime, 0700)
	os.WriteFile(filepath.Join(base, "changed.txt"), []byte("old\n"), 0644)
	os.WriteFile(filepath.Join(base, "deleted.txt"), []byte("gone\n"), 0644)
	os.WriteFile(filepath.Join(runtime, "changed.txt"), []byte("new\n"), 0644)
	os.WriteFile(filepath.Join(runtime, "added.bin"), []byte{0, 1, 2}, 0600)
	files, err := compareCheckpointTrees(base, runtime)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatalf("files = %#v", files)
	}
	if files[0].Path != "added.bin" || files[0].ContentB64 == "" || files[1].Patch == "" || files[2].Operation != "delete" {
		t.Fatalf("files = %#v", files)
	}
	os.WriteFile(filepath.Join(runtime, ".env"), []byte("TOKEN=secret"), 0600)
	if _, err = compareCheckpointTrees(base, runtime); err == nil {
		t.Fatal("credential path was captured")
	}
	os.Remove(filepath.Join(runtime, ".env"))
	os.WriteFile(filepath.Join(runtime, "config.txt"), []byte("GITHUB_TOKEN=secret"), 0600)
	if _, err = compareCheckpointTrees(base, runtime); err == nil {
		t.Fatal("credential content was captured")
	}
	os.Remove(filepath.Join(runtime, "config.txt"))
	oversized := append([]byte("AWS_SECRET_ACCESS_KEY=secret\n"), bytes.Repeat([]byte("x"), 2<<20)...)
	os.WriteFile(filepath.Join(runtime, "large.txt"), oversized, 0600)
	if _, err = compareCheckpointTrees(base, runtime); err == nil {
		t.Fatal("oversized credential content was captured")
	}
	os.Remove(filepath.Join(runtime, "large.txt"))
	os.WriteFile(filepath.Join(runtime, ".npmrc"), []byte("//registry.npmjs.org/:_authToken=npm_secret"), 0600)
	if _, err = compareCheckpointTrees(base, runtime); err == nil {
		t.Fatal("npm credential file was captured")
	}
	os.Remove(filepath.Join(runtime, ".npmrc"))
	os.WriteFile(filepath.Join(runtime, "registry.txt"), []byte("//registry.npmjs.org/:_authToken=npm_secret"), 0600)
	if _, err = compareCheckpointTrees(base, runtime); err == nil {
		t.Fatal("disguised npm auth directive was captured")
	}
}

func TestCheckpointSessionMustBelongToWorkspaceTask(t *testing.T) {
	store, err := changesessions.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repository, proposalA, taskA, proposalB, taskB, user := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32), strings.Repeat("4", 32), strings.Repeat("5", 32), strings.Repeat("6", 32)
	context := changesessions.TaskContext{RepositoryName: "repo", ProposalTitle: "proposal", TaskTitle: "task", TaskOutcome: "outcome", Mandate: "mandate"}
	sessionA, err := store.CreateForTask(repository, proposalA, taskA, user, strings.Repeat("a", 40), context)
	if err != nil {
		t.Fatal(err)
	}
	sessionB, err := store.CreateForTask(repository, proposalB, taskB, user, strings.Repeat("a", 40), context)
	if err != nil {
		t.Fatal(err)
	}
	workspace := workspaces.Workspace{RepositoryID: repository, Source: workspaces.Source{Kind: "proposal_task", RepositoryID: repository, ProposalID: proposalA, TaskID: taskA}}
	if err = validateCheckpointSession(store, workspace, sessionA.ID); err != nil {
		t.Fatalf("matching session: %v", err)
	}
	if err = validateCheckpointSession(store, workspace, sessionB.ID); err == nil {
		t.Fatal("cross-task session accepted")
	}
}

func TestCommitCheckpointPublishesOnlyManifestAtExactBase(t *testing.T) {
	root, work := filepath.Join(t.TempDir(), "repo.git"), filepath.Join(t.TempDir(), "work")
	run := func(args ...string) string {
		t.Helper()
		out, err := exec.Command(args[0], args[1:]...).CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	run("git", "init", "--bare", "-q", root)
	run("git", "init", "-q", work)
	run("git", "-C", work, "config", "user.name", "Test")
	run("git", "-C", work, "config", "user.email", "test@example.com")
	os.WriteFile(filepath.Join(work, "kept.txt"), []byte("before\n"), 0600)
	os.WriteFile(filepath.Join(work, "deleted.txt"), []byte("remove\n"), 0600)
	run("git", "-C", work, "add", ".")
	run("git", "-C", work, "commit", "-qm", "base")
	base := run("git", "-C", work, "rev-parse", "HEAD")
	run("git", "-C", work, "push", "-q", root, "HEAD:main")
	checkpoint := workspaces.Checkpoint{ID: strings.Repeat("1", 32), WorkspaceID: strings.Repeat("2", 32), BaseCommitID: base, Title: "Reviewed files", Files: []workspaces.CheckpointFile{
		{Path: "kept.txt", Operation: "modify", Mode: 0600, ContentB64: base64.StdEncoding.EncodeToString([]byte("after\n"))},
		{Path: "added.txt", Operation: "add", Mode: 0600, ContentB64: base64.StdEncoding.EncodeToString([]byte("published\n"))},
		{Path: "deleted.txt", Operation: "delete"},
	}}
	commit, err := commitCheckpoint(root, checkpoint, strings.Repeat("a", 32))
	if err != nil {
		t.Fatal(err)
	}
	if got := run("git", "--git-dir="+root, "show", commit+":kept.txt"); got != "after" {
		t.Fatalf("kept = %q", got)
	}
	if got := run("git", "--git-dir="+root, "show", commit+":added.txt"); got != "published" {
		t.Fatalf("added = %q", got)
	}
	if err := exec.Command("git", "--git-dir="+root, "cat-file", "-e", commit+":deleted.txt").Run(); err == nil {
		t.Fatal("deleted file remained")
	}
	if got := run("git", "--git-dir="+root, "rev-parse", commit+"^"); got != base {
		t.Fatalf("parent = %s", got)
	}
}

func TestMissingCheckpointDependencies(t *testing.T) {
	got := missingDependencies([]string{"go", "bun"}, []string{"go"})
	if len(got) != 1 || got[0] != "bun" {
		t.Fatalf("missing = %v", got)
	}
}

func TestCheckpointRestoreRollsBackEveryEarlierMutation(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "first.txt"), []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "blocked"), []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	checkpoint := workspaces.Checkpoint{Files: []workspaces.CheckpointFile{
		{Path: "first.txt", Operation: "modify", Mode: 0600, ContentB64: base64.StdEncoding.EncodeToString([]byte("after"))},
		{Path: "created/parents/new.txt", Operation: "add", Mode: 0600, ContentB64: base64.StdEncoding.EncodeToString([]byte("temporary"))},
		{Path: "blocked/second.txt", Operation: "add", Mode: 0600, ContentB64: base64.StdEncoding.EncodeToString([]byte("fails"))},
	}}
	archive, err := checkpointRestoreArchive(checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("sh", "-c", checkpointRestoreScript, "sh", root)
	cmd.Stdin = bytes.NewReader(archive)
	trace, runErr := cmd.CombinedOutput()
	if err = runErr; err == nil {
		t.Fatal("restore unexpectedly succeeded")
	}
	got, err := os.ReadFile(filepath.Join(root, "first.txt"))
	if err != nil || string(got) != "before" {
		t.Fatalf("first file after rollback = %q, %v", got, err)
	}
	blocked, err := os.ReadFile(filepath.Join(root, "blocked"))
	if err != nil || string(blocked) != "not a directory" {
		t.Fatalf("blocker after rollback = %q, %v", blocked, err)
	}
	if _, err = os.Stat(filepath.Join(root, "created")); !os.IsNotExist(err) {
		t.Fatalf("restore left created parent directories: %v\n%s", err, trace)
	}
}
