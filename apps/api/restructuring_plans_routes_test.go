package main

import (
	"strings"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/federation"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/restructuringplans"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

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
}
