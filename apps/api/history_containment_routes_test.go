package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestHistoryQuarantineRequiresObjectsToBeUnreachableFromAllRefs(t *testing.T) {
	dir := t.TempDir()
	run := func(input string, args ...string) string {
		cmd := exec.Command("git", append([]string{"--git-dir=" + dir}, args...)...)
		cmd.Stdin = strings.NewReader(input)
		cmd.Env = append(cmd.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
		return strings.TrimSpace(string(out))
	}
	run("", "init", "--bare")
	blob := run("affected", "hash-object", "-w", "--stdin")
	tree := run("100644 blob "+blob+"\tsecret.txt\n", "mktree")
	commit := run("replacement boundary", "commit-tree", tree)
	run("", "update-ref", "refs/heads/main", commit)
	if found, err := historyQuarantinedObjectsReachable(dir, []string{blob}); err != nil || len(found) != 1 {
		t.Fatalf("reachable affected object = %v, %v", found, err)
	}
	run("", "update-ref", "-d", "refs/heads/main", commit)
	if found, err := historyQuarantinedObjectsReachable(dir, []string{blob}); err != nil || len(found) != 0 {
		t.Fatalf("unreferenced affected object = %v, %v", found, err)
	}
}
