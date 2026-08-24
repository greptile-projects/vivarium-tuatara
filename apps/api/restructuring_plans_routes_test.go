package main

import (
	"testing"

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
