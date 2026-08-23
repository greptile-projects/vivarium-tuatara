package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestConflictDependencyManifestDistinguishesAbsenceFromReadFailure(t *testing.T) {
	repository := t.TempDir()
	run := func(arguments ...string) string {
		command := exec.Command("git", append([]string{"-C", repository}, arguments...)...)
		output, err := command.Output()
		if err != nil {
			t.Fatalf("git %v: %v", arguments, err)
		}
		return string(output)
	}
	run("init", "-q")
	run("config", "user.name", "checkpoint-test")
	run("config", "user.email", "checkpoint-test@invalid")
	run("commit", "--allow-empty", "-qm", "without inventory")
	absentCommit := run("rev-parse", "HEAD")[:40]
	body, err := readConflictDependencyManifest(filepath.Join(repository, ".git"), absentCommit)
	if err != nil || string(body) != "absent" {
		t.Fatalf("absence body=%q err=%v", body, err)
	}

	manifestPath := filepath.Join(repository, ".vivarium", "packages.json")
	if err = os.MkdirAll(filepath.Dir(manifestPath), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(manifestPath, []byte(`{"version":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	run("add", ".vivarium/packages.json")
	run("commit", "-qm", "with inventory")
	presentCommit := run("rev-parse", "HEAD")[:40]
	body, err = readConflictDependencyManifest(filepath.Join(repository, ".git"), presentCommit)
	if err != nil || string(body) != `{"version":1}` {
		t.Fatalf("present body=%q err=%v", body, err)
	}

	if _, err = readConflictDependencyManifest(filepath.Join(repository, "unavailable.git"), presentCommit); err == nil {
		t.Fatal("operational repository failure was treated as manifest absence")
	}
}
