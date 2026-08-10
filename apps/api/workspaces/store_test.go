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
		if _, err = store.RecordCommand(created.ID, CommandOutcome{CommandSHA256: strings.Repeat("a", 64), ActorID: "actor", StartedAt: now, CompletedAt: now}); err != nil {
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

func TestSharedPresenceDiscussionAndVersionedControlSurviveRestart(t *testing.T) {
	root := t.TempDir()
	store, _ := New(root)
	creator := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	peer := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	created, err := store.Create(Workspace{RepositoryID: "repository", CommitID: "commit", CreatorID: creator}, []byte("definition"))
	if err != nil {
		t.Fatal(err)
	}
	joined, err := store.Join(created.ID, peer, "file", "main.go")
	if err != nil || len(joined.Presence) != 1 || joined.Presence[0].Path != "main.go" {
		t.Fatalf("join = %#v, %v", joined, err)
	}
	controlled, err := store.SetControl(created.ID, peer, "human", peer, "edit", []string{"files"}, 1, 300)
	if err != nil || !controlled.CanControl(peer, "files", time.Now()) || controlled.CanControl(creator, "files", time.Now()) {
		t.Fatalf("control = %#v, %v", controlled.Control, err)
	}
	if _, err = store.SetControl(created.ID, creator, "human", creator, "execute", []string{"commands"}, 1, 300); !errors.Is(err, ErrControl) {
		t.Fatalf("stale control = %v", err)
	}
	if _, err = store.AddMessage(created.ID, creator, "Please keep the test focused."); err != nil {
		t.Fatal(err)
	}
	reopened, _ := New(root)
	durable, err := reopened.Get(created.ID)
	if err != nil || len(durable.Messages) != 1 || len(durable.Presence) != 1 || durable.Control.Version != 2 {
		t.Fatalf("durable = %#v, %v", durable, err)
	}
	roles := map[string]bool{}
	for _, event := range durable.Events {
		roles[event.Role] = true
	}
	if !roles["observation"] || !roles["instruction"] {
		t.Fatalf("activity roles = %#v", roles)
	}
}

func TestControlTransferWaitsForAdmittedMutationAndRejectsStaleActor(t *testing.T) {
	store, _ := New(t.TempDir())
	creator := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	peer := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	created, err := store.Create(Workspace{RepositoryID: "repository", CommitID: "commit", CreatorID: creator}, []byte("definition"))
	if err != nil {
		t.Fatal(err)
	}
	started, finish := make(chan struct{}), make(chan struct{})
	mutationDone := make(chan error, 1)
	go func() {
		mutationDone <- store.WithControl(created.ID, creator, "commands", func(Workspace) error {
			close(started)
			<-finish
			return nil
		})
	}()
	<-started
	transferDone := make(chan error, 1)
	go func() {
		_, transferErr := store.SetControl(created.ID, peer, "human", peer, "execute", []string{"commands"}, 1, 300)
		transferDone <- transferErr
	}()
	select {
	case err := <-transferDone:
		t.Fatalf("transfer completed during admitted mutation: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(finish)
	if err := <-mutationDone; err != nil {
		t.Fatal(err)
	}
	if err := <-transferDone; err != nil {
		t.Fatal(err)
	}
	if err := store.WithControl(created.ID, creator, "commands", func(Workspace) error { return nil }); !errors.Is(err, ErrControl) {
		t.Fatalf("former controller mutation = %v", err)
	}
}

func TestControlCanBeExplicitlyReleased(t *testing.T) {
	store, _ := New(t.TempDir())
	created, _ := store.Create(Workspace{RepositoryID: "repository", CommitID: "commit", CreatorID: "actor"}, []byte("definition"))
	released, err := store.SetControl(created.ID, "actor", "", "", "observe", nil, 1, 0)
	if err != nil || released.Control.PrincipalID != "" || len(released.Control.Scopes) != 0 || released.Control.Version != 2 {
		t.Fatalf("release = %#v, %v", released.Control, err)
	}
}

func TestCommandEvidenceDoesNotRetainPrivateInput(t *testing.T) {
	store, _ := New(t.TempDir())
	created, _ := store.Create(Workspace{RepositoryID: "repository", CommitID: "commit", CreatorID: "actor"}, []byte("definition"))
	secret := "export PRIVATE_TOKEN=do-not-share"
	digest := sha256.Sum256([]byte(secret))
	if _, err := store.RecordCommand(created.ID, CommandOutcome{CommandSHA256: hex.EncodeToString(digest[:]), ActorID: "actor", StartedAt: time.Now(), CompletedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(filepath.Join(store.root, created.ID+".json"))
	if strings.Contains(string(body), secret) {
		t.Fatal("private terminal input entered the durable record")
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
