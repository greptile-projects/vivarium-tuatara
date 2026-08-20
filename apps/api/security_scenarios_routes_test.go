package main

import (
	"strings"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/threatmodels"
)

func TestSecurityAttemptCommandsMustMatchReviewedCommand(t *testing.T) {
	reviewed := "go test ./security/expected"
	if securityCommandDigestMatches(securityDigest("go test ./unrelated"), reviewed) {
		t.Fatal("unrelated workspace outcome matched reviewed security command")
	}
	if !securityCommandDigestMatches(securityDigest(reviewed), reviewed) {
		t.Fatal("exact workspace outcome did not match reviewed security command")
	}
	if securityCommandMatches("echo preview-build-only", reviewed) {
		t.Fatal("ordinary preview build matched reviewed security command")
	}
	if !securityCommandMatches(reviewed, reviewed) {
		t.Fatal("exact preview command did not match reviewed security command")
	}
}

func TestSecurityScenarioRepairCandidateMustDescendFromModeledCommit(t *testing.T) {
	git, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repository, err := git.Create(strings.Repeat("1", 32))
	if err != nil {
		t.Fatal(err)
	}
	vulnerableBlob, err := repository.WriteObject(storage.BlobObject, []byte("vulnerable\n"))
	if err != nil {
		t.Fatal(err)
	}
	tree := writeTestTree(t, repository, testTreeEntry{mode: "100644", name: "security.txt", id: vulnerableBlob})
	modeled := writeTestCommit(t, repository, tree, nil, 1, "modeled candidate")
	protectedBlob, err := repository.WriteObject(storage.BlobObject, []byte("protected\n"))
	if err != nil {
		t.Fatal(err)
	}
	repairTree := writeTestTree(t, repository, testTreeEntry{mode: "100644", name: "security.txt", id: protectedBlob})
	repair := writeTestCommit(t, repository, repairTree, []storage.ObjectID{modeled}, 2, "repair candidate")
	unrelated := writeTestCommit(t, repository, repairTree, nil, 3, "unrelated candidate")

	if !securityScenarioCandidateDescendsFrom(git, repository.ID(), string(modeled), string(modeled)) {
		t.Fatal("modeled candidate rejected")
	}
	if !securityScenarioCandidateDescendsFrom(git, repository.ID(), string(modeled), string(repair)) {
		t.Fatal("descendant repair rejected")
	}
	if securityScenarioCandidateDescendsFrom(git, repository.ID(), string(modeled), string(unrelated)) {
		t.Fatal("unrelated candidate admitted")
	}
}

func TestSecurityScenarioCanResolveRetainedThreatModelRevision(t *testing.T) {
	model := threatmodels.Model{CurrentVersion: 2, Revisions: []threatmodels.Revision{{Version: 1}, {Version: 2}}}
	revision, ok := retainedThreatModelRevision(model, 1)
	if !ok || revision.Version != 1 {
		t.Fatalf("retained revision = %+v, %v", revision, ok)
	}
	if _, ok = retainedThreatModelRevision(model, 3); ok {
		t.Fatal("missing future revision resolved")
	}
}
