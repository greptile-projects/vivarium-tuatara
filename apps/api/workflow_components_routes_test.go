package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadGitBlobBoundedRejectsBeforeBufferingCompleteBlob(t *testing.T) {
	repository := t.TempDir()
	git := func(args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = repository
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.test", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.test")
		body, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, body)
		}
		return strings.TrimSpace(string(body))
	}
	git("init", "-q")
	if err := os.WriteFile(filepath.Join(repository, "component.json"), []byte(strings.Repeat("x", 512*1024)), 0600); err != nil {
		t.Fatal(err)
	}
	git("add", "component.json")
	git("commit", "-qm", "oversized component")
	revision := git("rev-parse", "HEAD")
	if body, err := readGitBlobBounded(filepath.Join(repository, ".git"), revision, "component.json", 256*1024); err == nil || body != nil {
		t.Fatalf("oversized blob returned %d bytes, %v", len(body), err)
	}
	if body, err := readGitBlobBounded(filepath.Join(repository, ".git"), revision, "component.json", 600*1024); err != nil || len(body) != 512*1024 {
		t.Fatalf("bounded valid read returned %d bytes, %v", len(body), err)
	}
}
