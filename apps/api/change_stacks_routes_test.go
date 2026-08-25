package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/changestacks"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
)

func TestStackIntegrationCheckStatusUsesPersistedTerminalStates(t *testing.T) {
	revision := strings.Repeat("1", 40)
	candidate := changestacks.IntegrationCandidate{CandidateRevision: revision, RequiredChecks: []string{"required"}}
	if got := stackIntegrationCheckStatus(candidate, []checkruns.Run{{CommitID: revision, Definition: checkruns.Definition{Name: "required"}, State: "succeeded"}}); got != "passed" {
		t.Fatalf("succeeded check status = %q, want passed", got)
	}
	if got := stackIntegrationCheckStatus(candidate, []checkruns.Run{{CommitID: revision, Definition: checkruns.Definition{Name: "required"}, State: "canceled"}}); got != "failed" {
		t.Fatalf("canceled check status = %q, want failed", got)
	}
	if got := stackIntegrationCheckStatus(candidate, []checkruns.Run{{CommitID: revision, Definition: checkruns.Definition{Name: "required"}, State: "running"}}); got != "verifying" {
		t.Fatalf("running check status = %q, want verifying", got)
	}
}

func TestChangeStackAgentMemberScopeRejectsAnotherBranch(t *testing.T) {
	repositoryID := strings.Repeat("a", 32)
	agent := auth.Credential{AgentID: strings.Repeat("b", 32), RepositoryID: repositoryID, GitWriteBranch: "refs/heads/authorized"}
	if changeStackAgentMemberScope(agent, repositoryID, "different") {
		t.Fatal("branch-bound agent could publish timeline work for another member branch")
	}
	if !changeStackAgentMemberScope(agent, repositoryID, "authorized") {
		t.Fatal("canonical authorized member branch was rejected")
	}
	if changeStackAgentMemberScope(agent, strings.Repeat("c", 32), "authorized") {
		t.Fatal("repository-bound agent crossed source repositories")
	}
}

func TestRestackRewritePreservesAuthorAndAppliesPatchOntoNewParent(t *testing.T) {
	dir := t.TempDir()
	run := func(input []byte, args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Layer Author", "GIT_AUTHOR_EMAIL=layer@example.test", "GIT_COMMITTER_NAME=Layer Author", "GIT_COMMITTER_EMAIL=layer@example.test")
		cmd.Stdin = bytes.NewReader(input)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	run(nil, "init", "-q", dir)
	if err := os.WriteFile(filepath.Join(dir, "base.txt"), []byte("base\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run(nil, "-C", dir, "add", ".")
	run(nil, "-C", dir, "commit", "-qm", "base")
	base := run(nil, "-C", dir, "rev-parse", "HEAD")
	if err := os.WriteFile(filepath.Join(dir, "layer.txt"), []byte("layer\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run(nil, "-C", dir, "add", ".")
	run(nil, "-C", dir, "commit", "-qm", "layer")
	layer := run(nil, "-C", dir, "rev-parse", "HEAD")
	run(nil, "-C", dir, "checkout", "-q", "--detach", base)
	if err := os.WriteFile(filepath.Join(dir, "upstream.txt"), []byte("feedback\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run(nil, "-C", dir, "add", ".")
	run(nil, "-C", dir, "commit", "-qm", "feedback")
	upstream := run(nil, "-C", dir, "rev-parse", "HEAD")
	candidate, mapping, err := rewriteStackCommits(filepath.Join(dir, ".git"), upstream, []string{layer}, time.Unix(1_700_000_000, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if mapping[layer] != candidate || run(nil, "--git-dir="+filepath.Join(dir, ".git"), "show", "-s", "--format=%P", candidate) != upstream {
		t.Fatalf("candidate %s mapping %#v", candidate, mapping)
	}
	if author := run(nil, "--git-dir="+filepath.Join(dir, ".git"), "show", "-s", "--format=%an <%ae>", candidate); author != "Layer Author <layer@example.test>" {
		t.Fatalf("author = %q", author)
	}
	if got := run(nil, "--git-dir="+filepath.Join(dir, ".git"), "show", candidate+":layer.txt"); got != "layer" {
		t.Fatalf("layer contents = %q", got)
	}
}

func TestRestackBranchIdentityNormalizesDefaultSourceRepository(t *testing.T) {
	repositoryID := strings.Repeat("a", 32)
	implicit := normalizedStackBranchKey(repositoryID, changestacks.Member{SourceBranch: "topic"})
	explicit := normalizedStackBranchKey(repositoryID, changestacks.Member{SourceRepositoryID: repositoryID, SourceBranch: "refs/heads/topic"})
	if implicit != explicit {
		t.Fatalf("implicit key %q != explicit key %q", implicit, explicit)
	}
	if canonicalStackBranch("refs/heads/topic") != canonicalStackBranch("topic") {
		t.Fatal("pull and member branch spellings did not canonicalize equally")
	}
}

func TestChangeStackCycleRemainsExplicit(t *testing.T) {
	graph := map[string][]string{"one": {"two"}, "two": {"one"}}
	if !stackCycle("one", graph, map[string]bool{}, map[string]bool{}) {
		t.Fatal("cycle was hidden")
	}
	if stackCycle("one", map[string][]string{"one": nil}, map[string]bool{}, map[string]bool{}) {
		t.Fatal("acyclic member was blocked")
	}
}

func TestStackOwnerEvidenceSnapshotDetectsUpstreamMovement(t *testing.T) {
	refs := []stackRevisionRef{{MemberID: "one", Revision: strings.Repeat("1", 40), Current: true}}
	if !sameUpstreamSnapshot(map[string]string{"one": strings.Repeat("1", 40)}, refs) {
		t.Fatal("exact upstream snapshot was stale")
	}
	refs[0].Revision = strings.Repeat("2", 40)
	if sameUpstreamSnapshot(map[string]string{"one": strings.Repeat("1", 40)}, refs) {
		t.Fatal("moved upstream revision retained owner evidence")
	}
}

func TestChangeStackRangeViewJoinsDistinctObjectStores(t *testing.T) {
	makeRepo := func(name, contents string) (string, string) {
		dir := filepath.Join(t.TempDir(), name)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		run := func(args ...string) string {
			command := exec.Command("git", args...)
			command.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Stack Author", "GIT_AUTHOR_EMAIL=stack@example.test", "GIT_COMMITTER_NAME=Stack Author", "GIT_COMMITTER_EMAIL=stack@example.test")
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("git %v: %v: %s", args, err, output)
			}
			return strings.TrimSpace(string(output))
		}
		run("init", "-q", dir)
		if err := os.WriteFile(filepath.Join(dir, "layer.txt"), []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
		run("-C", dir, "add", "layer.txt")
		run("-C", dir, "commit", "-qm", name)
		bare := filepath.Join(t.TempDir(), name+".git")
		run("clone", "-q", "--bare", dir, bare)
		return bare, run("-C", dir, "rev-parse", "HEAD")
	}
	baseRepo, base := makeRepo("base", "base\n")
	headRepo, originalHead := makeRepo("head", "head\n")
	raw, err := exec.Command("git", "--git-dir="+headRepo, "cat-file", "commit", originalHead).Output()
	if err != nil {
		t.Fatal(err)
	}
	raw = bytes.Replace(raw, []byte("\n\n"), []byte("\nparent "+base+"\n\n"), 1)
	write := exec.Command("git", "--git-dir="+headRepo, "hash-object", "-w", "-t", "commit", "--stdin")
	write.Stdin = bytes.NewReader(raw)
	written, err := write.Output()
	if err != nil {
		t.Fatal(err)
	}
	head := strings.TrimSpace(string(written))
	view, cleanup, err := stackRangeView(headRepo, baseRepo, base, head)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if !commitExists(view, base) || !commitExists(view, head) {
		t.Fatal("range view did not import both exact commits")
	}
	if scope := stackScope(view, base, head); len(scope.Files) != 1 || scope.Files[0] != "layer.txt" {
		t.Fatalf("joined scope = %#v", scope)
	}
	secondRepo, originalSecond := makeRepo("second", "second\n")
	secondRaw, err := exec.Command("git", "--git-dir="+secondRepo, "cat-file", "commit", originalSecond).Output()
	if err != nil {
		t.Fatal(err)
	}
	secondRaw = bytes.Replace(secondRaw, []byte("\n\n"), []byte("\nparent "+head+"\n\n"), 1)
	writeSecond := exec.Command("git", "--git-dir="+secondRepo, "hash-object", "-w", "-t", "commit", "--stdin")
	writeSecond.Stdin = bytes.NewReader(secondRaw)
	secondOutput, err := writeSecond.Output()
	if err != nil {
		t.Fatal(err)
	}
	second := strings.TrimSpace(string(secondOutput))
	threeStoreView, threeStoreCleanup, err := stackRangeView(secondRepo, headRepo, head, second, baseRepo)
	if err != nil {
		t.Fatal(err)
	}
	defer threeStoreCleanup()
	for _, revision := range []string{base, head, second} {
		if !commitExists(threeStoreView, revision) {
			t.Fatalf("three-store view omitted %s", revision)
		}
	}
	if scope := stackScope(threeStoreView, head, second); len(scope.Files) != 1 || scope.Files[0] != "layer.txt" {
		t.Fatalf("three-store scope = %#v", scope)
	}
	if authors := stackAuthors(threeStoreView, head, second); len(authors) != 1 {
		t.Fatalf("three-store authors = %#v", authors)
	}
}
