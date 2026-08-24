package main

import (
	"strings"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/historyremediations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

func TestHistoryRemediationReconcileVisibility(t *testing.T) {
	v := historyremediations.Remediation{CreatedBy: "creator", AudienceIDs: []string{"audience"}, OwnerIDs: []string{"owner"}, RequiredApprovals: []historyremediations.Approval{{ApproverIDs: []string{"approver"}}}}
	for _, actor := range []string{"creator", "audience", "owner", "approver"} {
		if !historyRemediationCanSee(v, actor) {
			t.Fatalf("%s cannot see retained record", actor)
		}
	}
	if historyRemediationCanSee(v, "other-participant") {
		t.Fatal("non-audience participant can receive reconciled record")
	}
}

func TestHistoryRemediationGitRefMustMatchClaimedObject(t *testing.T) {
	gitStore, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := repositories.New(t.TempDir(), gitStore)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := catalog.Create(strings.Repeat("1", 32), "history-scope")
	if err != nil {
		t.Fatal(err)
	}
	repo, err := gitStore.Open(metadata.ID)
	if err != nil {
		t.Fatal(err)
	}
	first, err := repo.WriteObject(storage.BlobObject, []byte("first"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := repo.WriteObject(storage.BlobObject, []byte("second"))
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.CreateReference(storage.Reference{Name: "refs/tags/affected", Target: string(second)}); err != nil {
		t.Fatal(err)
	}
	scope := historyremediations.Scope{RepositoryID: metadata.ID, Kind: "git_object", ObjectID: string(first), Ref: "refs/tags/affected"}
	if historyRemediationScopeExists(scope, gitStore, catalog, nil, nil, nil, nil) {
		t.Fatal("ref targeting another object was accepted")
	}
	scope.ObjectID = string(second)
	if !historyRemediationScopeExists(scope, gitStore, catalog, nil, nil, nil, nil) {
		t.Fatal("matching ref and object were rejected")
	}
	scope.Revision = strings.Repeat("a", 40)
	if historyRemediationScopeExists(scope, gitStore, catalog, nil, nil, nil, nil) {
		t.Fatal("mismatched revision was accepted")
	}
}
