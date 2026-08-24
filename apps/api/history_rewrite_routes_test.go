package main

import (
	"context"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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
	historyGit(t, repo, "branch", "release")
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
	in := historyremediations.RewriteCandidate{RequestID: "candidate-1", Rules: []historyremediations.RewriteRule{{ID: "replace", AffectedObjectID: affected, Action: "replace", ReplacementObjectID: replacement, Reason: "Replace the affected category"}}, SelectedRefs: []historyremediations.RewriteRef{{Name: "refs/heads/main", ExpectedTip: oldTip}, {Name: "refs/heads/release", ExpectedTip: oldTip}}, ObjectMap: []historyremediations.ObjectMapping{{OldObjectID: "forged", Action: "remove"}}, BrokenLinks: []string{"forged-link"}, Unrewritable: []string{"forged-resource"}, RollbackLimit: "Old clones can restore the lineage.", CollaboratorActions: []string{"replace local branches"}}
	candidate, err := assembleHistoryCandidate(filepath.Join(repo, ".git"), v, in)
	if err != nil {
		t.Fatal(err)
	}
	if historyGit(t, repo, "rev-parse", "refs/heads/main") != oldTip {
		t.Fatal("candidate assembly moved the authoritative ref")
	}
	if len(candidate.CandidateRefs) != 2 || candidate.CandidateRefs[0].NewTip == oldTip || len(candidate.CommitMap) != 1 || !candidate.CommitMap[0].Changed {
		t.Fatalf("candidate = %#v", candidate)
	}
	if len(candidate.ObjectMap) != 1 || candidate.ObjectMap[0].OldObjectID == "forged" || slices.Contains(candidate.BrokenLinks, "forged-link") || slices.Contains(candidate.Unrewritable, "forged-resource") {
		t.Fatalf("client-derived audit fields survived: %#v", candidate)
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
	for _, kind := range kinds {
		rehearsal.Scenarios = append(rehearsal.Scenarios, historyremediations.RehearsalScenario{ID: kind, Kind: kind, Expectation: "candidate usable", TimeoutSeconds: 10})
	}
	run, err := runHistoryRehearsal(filepath.Join(repo, ".git"), candidate, rehearsal)
	if err != nil {
		t.Fatal(err)
	}
	if len(run.Outcomes) != len(kinds)*2 {
		t.Fatalf("only part of the ref matrix ran: %#v", run.Outcomes)
	}
	covered := map[string]int{}
	for _, outcome := range run.Outcomes {
		covered[outcome.RefName]++
		if map[string]bool{"repository_integrity": true, "clone": true, "fetch": true}[outcome.Kind] && outcome.State != "passed" {
			t.Fatalf("outcome = %#v", outcome)
		}
	}
	if covered["refs/heads/main"] != 7 || covered["refs/heads/release"] != 7 {
		t.Fatalf("coverage = %v", covered)
	}
}

func TestBoundedRehearsalCommandHasNoHostOrNetworkAuthority(t *testing.T) {
	cmd := boundedRehearsalCommand(context.Background(), t.TempDir(), "bounded-test", historyremediations.RehearsalScenario{Image: "example.invalid/preinstalled:test", Command: "touch /host-marker"})
	joined := strings.Join(cmd.Args, " ")
	for _, required := range []string{"--network=none", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "readonly"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("missing %q in %s", required, joined)
		}
	}
	if strings.HasPrefix(strings.Join(cmd.Args[:3], " "), "sh -c") {
		t.Fatalf("host shell selected: %v", cmd.Args)
	}
}

func TestHistoryRewriteCandidateInputRejectsDerivedAuditFields(t *testing.T) {
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"expected_version":1,"candidate":{"request_id":"request","rules":[],"selected_refs":[],"rollback_limit":"limit","collaborator_actions":[],"object_map":[{"old_object_id":"forged"}]}}`))
	var input struct {
		ExpectedVersion int                          `json:"expected_version"`
		Candidate       historyRewriteCandidateInput `json:"candidate"`
	}
	if err := decodeJSON(r, &input); err == nil {
		t.Fatal("client-supplied derived audit projection was accepted")
	}
}

func TestPublishHistoryRefsIsAtomicAndRetryStable(t *testing.T) {
	repo := t.TempDir()
	historyGit(t, repo, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(repo, "value"), []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	historyGit(t, repo, "add", "value")
	historyGit(t, repo, "commit", "-m", "old")
	old := historyGit(t, repo, "rev-parse", "HEAD")
	historyGit(t, repo, "branch", "release")
	if err := os.WriteFile(filepath.Join(repo, "value"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	historyGit(t, repo, "commit", "-am", "new")
	newTip := historyGit(t, repo, "rev-parse", "HEAD")
	historyGit(t, repo, "reset", "--hard", old)
	refs := []historyremediations.CandidateRef{{Name: "refs/heads/main", OldTip: old, NewTip: newTip}, {Name: "refs/heads/release", OldTip: strings.Repeat("a", 40), NewTip: newTip}}
	if err := publishHistoryRefs(filepath.Join(repo, ".git"), refs); err == nil {
		t.Fatal("transaction with one stale ref succeeded")
	}
	if got := historyGit(t, repo, "rev-parse", "refs/heads/main"); got != old {
		t.Fatalf("partial ref publication: %s", got)
	}
	refs[1].OldTip = old
	if err := publishHistoryRefs(filepath.Join(repo, ".git"), refs); err != nil {
		t.Fatal(err)
	}
	if err := publishHistoryRefs(filepath.Join(repo, ".git"), refs); err != nil {
		t.Fatalf("exact retry: %v", err)
	}
	for _, name := range []string{"refs/heads/main", "refs/heads/release"} {
		if got := historyGit(t, repo, "rev-parse", name); got != newTip {
			t.Fatalf("%s = %s", name, got)
		}
	}
}

func TestHistoryRewritePublicationRequiresNamedOwner(t *testing.T) {
	v := historyremediations.Remediation{CreatedBy: "creator", OwnerIDs: []string{"owner"}}
	if historyRemediationOwner(v, "creator") {
		t.Fatal("non-owner creator received publication authority")
	}
	if !historyRemediationOwner(v, "owner") {
		t.Fatal("named owner lacks publication authority")
	}
}
