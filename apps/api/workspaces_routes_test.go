package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
)

func TestProvisionWorkspaceEnforcesStorageAndRemovesFailedContainer(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker unavailable")
	}
	if err := exec.Command("docker", "image", "inspect", "alpine:3.22").Run(); err != nil {
		t.Skip("alpine:3.22 unavailable")
	}
	repository := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		command := exec.Command(args[0], args[1:]...)
		command.Dir = repository
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v: %s", args, err, output)
		}
	}
	run("git", "init", "-q")
	run("git", "config", "user.name", "Workspace Test")
	run("git", "config", "user.email", "workspace@example.com")
	if err := os.WriteFile(filepath.Join(repository, "README.md"), []byte("exact\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run("git", "add", "README.md")
	run("git", "commit", "-qm", "exact")
	commitBody, err := exec.Command("git", "-C", repository, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, id, command string
		timeout           int
	}{
		{"storage quota", "0123456789abcdef0123456789abcdef", "dd if=/dev/zero of=too-large bs=1M count=129", 20},
		{"setup timeout", "abcdef0123456789abcdef0123456789", "sleep 5", 1},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			steps, failed := provisionWorkspace(filepath.Join(repository, ".git"), t.TempDir(), test.id, strings.TrimSpace(string(commitBody)), workspaces.Definition{Image: "alpine:3.22", Setup: []string{test.command}, Resources: workspaces.Resources{CPUs: 1, MemoryMB: 256, StorageMB: 128, SetupSeconds: test.timeout}})
			if !failed || len(steps) != 1 || steps[0].State != "failed" {
				t.Fatalf("provision = %#v, failed %v", steps, failed)
			}
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				if err := exec.Command("docker", "inspect", "vivarium-workspace-"+test.id).Run(); err != nil {
					return
				}
				time.Sleep(50 * time.Millisecond)
			}
			t.Fatal("failed bounded container was not removed")
		})
	}
}
