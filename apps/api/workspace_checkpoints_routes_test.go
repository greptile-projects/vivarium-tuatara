package main

import (
	"os"
	"path/filepath"
	"testing"
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
}

func TestMissingCheckpointDependencies(t *testing.T) {
	got := missingDependencies([]string{"go", "bun"}, []string{"go"})
	if len(got) != 1 || got[0] != "bun" {
		t.Fatalf("missing = %v", got)
	}
}
