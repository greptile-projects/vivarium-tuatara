package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/federation"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/restructuringplans"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

func TestRestructuringCandidatePreservesSelectedHistoryWithoutMovingSource(t *testing.T) {
	gitStore, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source, err := gitStore.Create("source")
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	run := func(args ...string) string {
		command := exec.Command("git", args...)
		command.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Source Author", "GIT_AUTHOR_EMAIL=source@example.test", "GIT_COMMITTER_NAME=Source Author", "GIT_COMMITTER_EMAIL=source@example.test")
		b, e := command.CombinedOutput()
		if e != nil {
			t.Fatalf("git %v: %v: %s", args, e, b)
		}
		return strings.TrimSpace(string(b))
	}
	run("init", "-q", work)
	if err = os.MkdirAll(filepath.Join(work, "packages", "core"), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(work, "packages", "core", "core.txt"), []byte("one\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(work, "LICENSE"), []byte("test license\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run("-C", work, "add", ".")
	run("-C", work, "commit", "-q", "-m", "initial core")
	if err = os.WriteFile(filepath.Join(work, "packages", "core", "core.txt"), []byte("two\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run("-C", work, "commit", "-qam", "advance core")
	revision := run("-C", work, "rev-parse", "HEAD")
	run("--git-dir="+source.Path(), "fetch", work, "HEAD:refs/heads/main")
	planStore, err := restructuringplans.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	plan := restructuringplans.Plan{ID: "plan", RepositoryID: "source", Sources: []restructuringplans.SourceRepository{{RepositoryID: "source", Revision: revision}}, Destinations: []restructuringplans.Destination{{ID: "core", Name: "core", DefaultBranch: "main"}}, Mappings: []restructuringplans.ContentMapping{{ID: "core-history", SourceRepositoryID: "source", SourcePath: "packages/core", DestinationID: "core", DestinationPath: "src", Disposition: "move", HistoryMode: "full"}}}
	candidate, err := assembleRestructuringCandidate(gitStore, planStore, plan, "candidate", "digest", restructuringCandidateInput{RequestID: "request"})
	if err != nil {
		t.Fatal(err)
	}
	if len(candidate.Repositories) != 1 || candidate.Repositories[0].ObjectCount < 3 {
		t.Fatalf("candidate = %#v", candidate)
	}
	path := planStore.CandidatePath("source", "plan", "candidate", "core")
	if got := run("--git-dir="+path, "show", "main:src/core.txt"); got != "two" {
		t.Fatalf("candidate content = %q", got)
	}
	if got := run("--git-dir="+source.Path(), "rev-parse", "refs/heads/main"); got != revision {
		t.Fatalf("source moved from %s to %s", revision, got)
	}
	if got := run("--git-dir="+path, "log", "--format=%an <%ae>", "--all"); !strings.Contains(got, "Source Author <source@example.test>") {
		t.Fatalf("authorship missing: %s", got)
	}
}

func TestRestructuringRehearsalRequiresBoundedPerDestinationMatrix(t *testing.T) {
	candidate := restructuringplans.CandidateSet{Repositories: []restructuringplans.CandidateRepository{{ID: "core", DestinationID: "core"}, {ID: "docs", DestinationID: "docs"}}}
	scenarios := make([]restructuringplans.Scenario, 0, len(restructuringScenarioKinds))
	for i, kind := range restructuringScenarioKinds {
		scenarios = append(scenarios, restructuringplans.Scenario{ID: fmt.Sprintf("scenario-%d", i), Kind: kind, DestinationID: "core", TimeoutSeconds: 1})
	}
	if _, err := runRestructuringRehearsal(nil, restructuringplans.Plan{}, candidate, restructuringplans.Rehearsal{RequestID: "missing-docs", Scenarios: scenarios}); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Fatalf("missing destination matrix = %v", err)
	}
	candidate.Repositories = candidate.Repositories[:1]
	scenarios = append(scenarios, restructuringplans.Scenario{ID: "duplicate", Kind: "repository_integrity", DestinationID: "core", TimeoutSeconds: 1})
	if _, err := runRestructuringRehearsal(nil, restructuringplans.Plan{}, candidate, restructuringplans.Rehearsal{RequestID: "duplicate", Scenarios: scenarios}); err == nil {
		t.Fatal("duplicate scenario was accepted")
	}
	for i := range scenarios[:len(restructuringScenarioKinds)] {
		scenarios[i].TimeoutSeconds = 100
	}
	if _, err := runRestructuringRehearsal(nil, restructuringplans.Plan{}, candidate, restructuringplans.Rehearsal{RequestID: "over-budget", Scenarios: scenarios[:len(restructuringScenarioKinds)]}); err == nil || !strings.Contains(err.Error(), "aggregate") {
		t.Fatalf("over-budget matrix = %v", err)
	}
}

func TestRestructuringRehearsalSetupHonorsScenarioDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := restructuringRehearsalSetup(ctx, "sh", "-c", "sleep 2")
	if err == nil || time.Since(started) > time.Second {
		t.Fatalf("setup cancellation = %v after %s", err, time.Since(started))
	}
}

func TestRestructuringResolvedIssueMustExistInAuthoritativeStore(t *testing.T) {
	gitStore, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repositoryID := "0123456789abcdef0123456789abcdef"
	repo, err := gitStore.Create(repositoryID)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := repo.WriteObject(storage.TreeObject, nil)
	if err != nil {
		t.Fatal(err)
	}
	commit, err := repo.WriteObject(storage.CommitObject, []byte("tree "+string(tree)+"\nauthor Test <test@example.com> 0 +0000\ncommitter Test <test@example.com> 0 +0000\n\nsource\n"))
	if err != nil {
		t.Fatal(err)
	}
	issueStore, err := issues.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	item := restructuringplans.InventoryItem{Kind: "issue", RepositoryID: repositoryID, ResourceID: "fabricated-issue", Revision: string(commit), State: "resolved"}
	if restructuringInventoryCitationResolves(gitStore, item, nil, issueStore, nil, nil, nil, nil, nil, nil, nil, nil, nil) {
		t.Fatal("fabricated resolved issue was accepted")
	}
	created, err := issueStore.Create(issues.Issue{RepositoryID: repositoryID, ReporterID: strings.Repeat("a", 32), Title: "Affected issue", ExpectedBehavior: "works", ObservedBehavior: "fails", Environment: "test", Severity: "medium", Visibility: "repository", ReproductionSteps: []string{"run"}})
	if err != nil {
		t.Fatal(err)
	}
	item.ResourceID = created.ID
	if restructuringInventoryCitationResolves(gitStore, item, nil, issueStore, nil, nil, nil, nil, nil, nil, nil, nil, nil) {
		t.Fatal("issue without exact revision provenance was accepted")
	}
	item.State = "inaccessible"
	if !restructuringInventoryCitationResolves(gitStore, item, nil, issueStore, nil, nil, nil, nil, nil, nil, nil, nil, nil) {
		t.Fatal("explicit inaccessible inventory gap was rejected")
	}
}

func TestRestructuringFederatedRelationshipBindsRepositoryAndRevision(t *testing.T) {
	gitStore, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repositoryID := strings.Repeat("1", 32)
	repo, err := gitStore.Create(repositoryID)
	if err != nil {
		t.Fatal(err)
	}
	tree, _ := repo.WriteObject(storage.TreeObject, nil)
	commit, _ := repo.WriteObject(storage.CommitObject, []byte("tree "+string(tree)+"\nauthor Test <test@example.com> 0 +0000\ncommitter Test <test@example.com> 0 +0000\n\nsource\n"))
	peers, err := federation.New(t.TempDir(), "local", "https://local.example", []string{"owner"})
	if err != nil {
		t.Fatal(err)
	}
	if err = peers.BindContribution("relationship", strings.Repeat("a", 32)); err != nil {
		t.Fatal(err)
	}
	if err = peers.BindContributionSource("relationship", strings.Repeat("2", 32), "main", string(commit)); err != nil {
		t.Fatal(err)
	}
	item := restructuringplans.InventoryItem{Kind: "federated_relationship", RepositoryID: repositoryID, ResourceID: "relationship", Revision: string(commit), State: "resolved"}
	if restructuringInventoryCitationResolves(gitStore, item, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, peers) {
		t.Fatal("relationship for another repository was accepted")
	}
	// The existing binding cannot be reassigned; create an exact relationship.
	if err = peers.BindContribution("exact-relationship", strings.Repeat("a", 32)); err != nil {
		t.Fatal(err)
	}
	if err = peers.BindContributionSource("exact-relationship", repositoryID, "main", string(commit)); err != nil {
		t.Fatal(err)
	}
	item.ResourceID = "exact-relationship"
	if !restructuringInventoryCitationResolves(gitStore, item, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, peers) {
		t.Fatal("exact repository/revision relationship was rejected")
	}
	targetID := strings.Repeat("3", 32)
	target, err := gitStore.Create(targetID)
	if err != nil {
		t.Fatal(err)
	}
	targetTree, _ := target.WriteObject(storage.TreeObject, nil)
	targetCommit, _ := target.WriteObject(storage.CommitObject, []byte("tree "+string(targetTree)+"\nauthor Test <test@example.com> 1 +0000\ncommitter Test <test@example.com> 1 +0000\n\ntarget\n"))
	if err = peers.BindContributionTarget("exact-relationship", targetID, "pull", string(targetCommit)); err != nil {
		t.Fatal(err)
	}
	item.RepositoryID = targetID
	item.Revision = string(targetCommit)
	if !restructuringInventoryCitationResolves(gitStore, item, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, peers) {
		t.Fatal("exact target repository/revision relationship was rejected")
	}
}
