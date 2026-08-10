package workspaces

import (
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
