package storage

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestVerifyPackExpansionBudgetRejectsCompressedExpansion(t *testing.T) {
	repository := filepath.Join(t.TempDir(), "repository.git")
	if output, err := exec.Command("git", "init", "--bare", repository).CombinedOutput(); err != nil {
		t.Fatalf("init repository: %v: %s", err, output)
	}
	content := make([]byte, 2<<20)
	command := exec.Command("git", "--git-dir="+repository, "hash-object", "-w", "--stdin")
	command.Stdin = bytes.NewReader(content)
	objectID, err := command.Output()
	if err != nil {
		t.Fatalf("write compressible object: %v", err)
	}
	if output, err := exec.Command("git", "--git-dir="+repository, "update-ref", "refs/tags/blob", string(bytes.TrimSpace(objectID))).CombinedOutput(); err != nil {
		t.Fatalf("write compressible object: %v: %s", err, output)
	}
	if output, err := exec.Command("git", "--git-dir="+repository, "repack", "-a", "-d").CombinedOutput(); err != nil {
		t.Fatalf("pack object: %v: %s", err, output)
	}
	if err := verifyPackExpansionBudget(repository, 1<<20); err == nil {
		t.Fatal("compressed object expansion exceeded budget without error")
	}
	if err := verifyPackExpansionBudget(repository, 3<<20); err != nil {
		t.Fatalf("bounded expansion rejected: %v", err)
	}
}
