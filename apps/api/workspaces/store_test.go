package workspaces

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	collaborator := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	suspended, err := store.Transition(created.ID, collaborator, created.DefinitionSHA256, "suspended")
	if err != nil || suspended.CommitID != created.CommitID {
		t.Fatalf("suspend = %#v, %v", suspended, err)
	}
	if actor := suspended.Events[len(suspended.Events)-1].ActorID; actor != collaborator {
		t.Fatalf("suspend actor = %q, want collaborator", actor)
	}
	if _, err = store.Transition(created.ID, created.CreatorID, "different", "running"); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed foundation error = %v", err)
	}
	if _, err = store.Transition(created.ID, created.CreatorID, "", "running"); !errors.Is(err, ErrConflict) {
		t.Fatalf("missing foundation error = %v", err)
	}
	resumed, err := store.Transition(created.ID, created.CreatorID, created.DefinitionSHA256, "running")
	if err != nil || resumed.DefinitionSHA256 != created.DefinitionSHA256 || resumed.CommitID != created.CommitID {
		t.Fatalf("resume = %#v, %v", resumed, err)
	}
}

func TestWorkspaceCommandAndChangeEvidenceIsBoundedAndContentFree(t *testing.T) {
	store, _ := New(t.TempDir())
	created, err := store.Create(Workspace{RepositoryID: "repository", CommitID: "commit", CreatorID: "actor"}, []byte("definition"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for index := 0; index < 105; index++ {
		if _, err = store.RecordCommand(created.ID, CommandOutcome{Command: "go test ./...", ActorID: "actor", StartedAt: now, CompletedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	secret := []byte("not-retained-source-secret")
	digest := sha256.Sum256(secret)
	updated, err := store.RecordChange(created.ID, Change{Path: "config.txt", SHA256: hex.EncodeToString(digest[:]), Size: len(secret), ActorID: "actor", CreatedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Commands) != 100 || len(updated.Changes) != 1 || updated.Changes[0].SHA256 == "" {
		t.Fatalf("unexpected evidence: %#v", updated)
	}
	body, err := os.ReadFile(filepath.Join(store.root, created.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), string(secret)) {
		t.Fatal("changed file content leaked into durable workspace evidence")
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
