package workspaces

import (
	"errors"
	"testing"
	"time"
)

func TestPolicyConstrainAndUpdateMarksWorkspaceForRebuild(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p := DefaultPolicy()
	p.MaxCPUs = 4
	p.Sharing = "organization"
	org := DefaultPolicy()
	org.MaxCPUs = 2
	org.Sharing = "repository"
	org.AgentExecution = false
	effective := Constrain(org, p)
	if effective.MaxCPUs != 2 || effective.Sharing != "repository" || effective.AgentExecution {
		t.Fatalf("effective = %#v", effective)
	}
	w, err := store.Create(Workspace{RepositoryID: "repo-1", CommitID: "commit", CreatorID: "owner", Definition: Definition{Resources: Resources{CPUs: 1, MemoryMB: 256, StorageMB: 128}}, Policy: DefaultPolicy()}, []byte("definition"))
	if err != nil {
		t.Fatal(err)
	}
	update := DefaultPolicy()
	update.IdleMinutes = 30
	if _, err = store.PutPolicy("repository", "repo-1", "owner", update, 1); err != nil {
		t.Fatal(err)
	}
	w, err = store.Get(w.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !w.RebuildRequired || len(w.RebuildReasons) != 1 {
		t.Fatalf("workspace = %#v", w)
	}
}

func TestStopRetainsWorkspaceAndConsumptionEvidence(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return clock }
	w, err := store.Create(Workspace{RepositoryID: "repo", CommitID: "commit", CreatorID: "owner", Definition: Definition{Resources: Resources{CPUs: 2, MemoryMB: 256, StorageMB: 128}}, Policy: DefaultPolicy()}, []byte("definition"))
	if err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(2 * time.Hour)
	w, err = store.Stop(w.ID, "owner", "budget", "stopped")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Get(w.ID); err != nil {
		t.Fatal("workspace evidence was deleted", err)
	}
	usage := Usage(w, clock.Add(time.Hour))
	if usage.CPUSeconds != 14400 || usage.MemoryMBHours != 512 {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestOrganizationPolicyUpdateMarksOnlyItsWorkspacesForRebuild(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first, _ := store.Create(Workspace{RepositoryID: "repo-1", OrganizationID: "org-1", CommitID: "commit", CreatorID: "owner", Policy: DefaultPolicy()}, []byte("one"))
	second, _ := store.Create(Workspace{RepositoryID: "repo-2", OrganizationID: "org-2", CommitID: "commit", CreatorID: "owner", Policy: DefaultPolicy()}, []byte("two"))
	update := DefaultPolicy()
	update.MaxCPUs = 2
	if _, err = store.PutPolicy("organization", "org-1", "owner", update, 1); err != nil {
		t.Fatal(err)
	}
	first, _ = store.Get(first.ID)
	second, _ = store.Get(second.ID)
	if !first.RebuildRequired || second.RebuildRequired {
		t.Fatalf("first=%#v second=%#v", first, second)
	}
}

func TestActivityMutationsRenewIdleDeadline(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return clock }
	w, err := store.Create(Workspace{RepositoryID: "repo", CommitID: "commit", CreatorID: "owner", Policy: DefaultPolicy()}, []byte("definition"))
	if err != nil {
		t.Fatal(err)
	}
	clock = clock.Add(time.Minute)
	w, err = store.Join(w.ID, "owner", "workspace", "")
	if err != nil || !w.LastActivityAt.Equal(clock) {
		t.Fatal("presence did not renew activity", err)
	}
	clock = clock.Add(time.Minute)
	w, err = store.AddMessage(w.ID, "owner", "still here")
	if err != nil || !w.LastActivityAt.Equal(clock) {
		t.Fatal("message did not renew activity", err)
	}
	clock = clock.Add(time.Minute)
	w, err = store.RecordCommand(w.ID, CommandOutcome{ActorID: "owner"})
	if err != nil || !w.LastActivityAt.Equal(clock) {
		t.Fatal("command did not renew activity", err)
	}
	clock = clock.Add(time.Minute)
	w, err = store.RecordChange(w.ID, Change{ActorID: "owner", Path: "README.md"})
	if err != nil || !w.LastActivityAt.Equal(clock) {
		t.Fatal("edit did not renew activity", err)
	}
}

func TestStopControlledSerializesRuntimeTeardownWithResume(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	w, err := store.Create(Workspace{RepositoryID: "repo", CommitID: "commit", CreatorID: "owner", Policy: DefaultPolicy()}, []byte("definition"))
	if err != nil {
		t.Fatal(err)
	}
	w, err = store.Complete(w.ID, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	w, err = store.TransitionControlled(w.ID, "owner", w.DefinitionSHA256, "suspended")
	if err != nil {
		t.Fatal(err)
	}
	started, release := make(chan struct{}), make(chan struct{})
	stopped := make(chan error, 1)
	go func() {
		_, stopErr := store.StopControlled(w.ID, "workspace-lifecycle", "expired", "expired", func() error { close(started); <-release; return nil })
		stopped <- stopErr
	}()
	<-started
	resumed := make(chan error, 1)
	go func() {
		_, resumeErr := store.TransitionControlled(w.ID, "owner", w.DefinitionSHA256, "running")
		resumed <- resumeErr
	}()
	select {
	case err := <-resumed:
		t.Fatalf("resume escaped lifecycle serialization: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	if err = <-stopped; err != nil {
		t.Fatal(err)
	}
	if err = <-resumed; !errors.Is(err, ErrConflict) {
		t.Fatalf("resume error = %v", err)
	}
}
