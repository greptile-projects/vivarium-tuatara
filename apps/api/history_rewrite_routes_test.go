package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/historyremediations"
)

func historyGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Author", "GIT_AUTHOR_EMAIL=author@example.test", "GIT_COMMITTER_NAME=Committer", "GIT_COMMITTER_EMAIL=committer@example.test")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestAssembleAndRehearseHistoryCandidateWithoutMovingRef(t *testing.T) {
	repo := t.TempDir()
	historyGit(t, repo, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(repo, "safe.txt"), []byte("safe\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "affected.txt"), []byte("unsafe-category\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	historyGit(t, repo, "add", ".")
	historyGit(t, repo, "commit", "-m", "original")
	oldTip := historyGit(t, repo, "rev-parse", "HEAD")
	affected := historyGit(t, repo, "rev-parse", "HEAD:affected.txt")
	cmd := exec.Command("git", "-C", repo, "hash-object", "-w", "--stdin")
	cmd.Stdin = strings.NewReader("sanitized\n")
	replacementBytes, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	replacement := strings.TrimSpace(string(replacementBytes))
	v := historyremediations.Remediation{Scopes: []historyremediations.Scope{{Kind: "git_object", ObjectID: affected}}, ExposureMap: []historyremediations.ExposureFinding{{CopyKind: "active_clone", ResourceID: "clone-private", IndependentlyControlled: true}}}
	in := historyremediations.RewriteCandidate{RequestID: "candidate-1", Rules: []historyremediations.RewriteRule{{ID: "replace", AffectedObjectID: affected, Action: "replace", ReplacementObjectID: replacement, Reason: "Replace the affected category"}}, SelectedRefs: []historyremediations.RewriteRef{{Name: "refs/heads/main", ExpectedTip: oldTip}}, RollbackLimit: "Old clones can restore the lineage.", CollaboratorActions: []string{"replace local branches"}}
	candidate, err := assembleHistoryCandidate(filepath.Join(repo, ".git"), v, in)
	if err != nil {
		t.Fatal(err)
	}
	if historyGit(t, repo, "rev-parse", "refs/heads/main") != oldTip {
		t.Fatal("candidate assembly moved the authoritative ref")
	}
	if len(candidate.CandidateRefs) != 1 || candidate.CandidateRefs[0].NewTip == oldTip || len(candidate.CommitMap) != 1 || !candidate.CommitMap[0].Changed {
		t.Fatalf("candidate = %#v", candidate)
	}
	if got := historyGit(t, repo, "show", candidate.CandidateRefs[0].NewTip+":affected.txt"); got != "sanitized" {
		t.Fatalf("affected content = %q", got)
	}
	if got := historyGit(t, repo, "show", candidate.CandidateRefs[0].NewTip+":safe.txt"); got != "safe" {
		t.Fatalf("safe content = %q", got)
	}
	if len(candidate.Unrewritable) != 1 {
		t.Fatalf("unrewritable = %v", candidate.Unrewritable)
	}
	kinds := []string{"repository_integrity", "build", "check", "release", "dependency", "clone", "fetch"}
	rehearsal := historyremediations.Rehearsal{RequestID: "run-1"}
	for i, kind := range kinds {
		command := "test \"$(cat affected.txt)\" = sanitized"
		rehearsal.Scenarios = append(rehearsal.Scenarios, historyremediations.RehearsalScenario{ID: kind, Kind: kind, Command: command, Expectation: "candidate usable", TimeoutSeconds: 10})
		_ = i
	}
	run, err := runHistoryRehearsal(filepath.Join(repo, ".git"), candidate, rehearsal)
	if err != nil {
		t.Fatal(err)
	}
	for _, outcome := range run.Outcomes {
		if outcome.State != "passed" {
			t.Fatalf("outcome = %#v", outcome)
		}
	}
}
