package main

import (
	"bytes"
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

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
