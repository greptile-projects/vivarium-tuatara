package workspaces

import (
	"errors"
	"testing"
)

func TestSuspendResumePreservesFrozenFoundation(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.Create(Workspace{RepositoryID: "0123456789abcdef0123456789abcdef", CommitID: "0123456789012345678901234567890123456789", CreatorID: "abcdef0123456789abcdef0123456789", Source: Source{Kind: "repository"}, Definition: Definition{Version: 1, Image: "alpine:3.22"}}, []byte(`{"version":1}`))
	if err != nil {
		t.Fatal(err)
	}
	running, err := store.Complete(created.ID, []SetupStep{}, false)
	if err != nil || running.State != "running" {
		t.Fatalf("complete = %#v, %v", running, err)
	}
	suspended, err := store.Transition(created.ID, created.CreatorID, created.DefinitionSHA256, "suspended")
	if err != nil || suspended.CommitID != created.CommitID {
		t.Fatalf("suspend = %#v, %v", suspended, err)
	}
	if _, err = store.Transition(created.ID, created.CreatorID, "different", "running"); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed foundation error = %v", err)
	}
	resumed, err := store.Transition(created.ID, created.CreatorID, created.DefinitionSHA256, "running")
	if err != nil || resumed.DefinitionSHA256 != created.DefinitionSHA256 || resumed.CommitID != created.CommitID {
		t.Fatalf("resume = %#v, %v", resumed, err)
	}
}

func TestDefinitionSnapshotIsIndependentOfCaller(t *testing.T) {
	store, _ := New(t.TempDir())
	definition := Definition{Version: 1, Image: "alpine", Tools: []Tool{{Name: "go", Version: "1.25"}}}
	created, err := store.Create(Workspace{RepositoryID: "0123456789abcdef0123456789abcdef", CommitID: "0123456789012345678901234567890123456789", CreatorID: "abcdef0123456789abcdef0123456789", Definition: definition}, []byte("definition"))
	if err != nil {
		t.Fatal(err)
	}
	definition.Tools[0].Version = "changed"
	loaded, err := store.Get(created.ID)
	if err != nil || loaded.Definition.Tools[0].Version != "1.25" {
		t.Fatalf("loaded = %#v, %v", loaded, err)
	}
}
