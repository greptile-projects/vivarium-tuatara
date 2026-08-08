package deployments

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
)

func TestExecutorFailsBeforeCommandWhenArtifactChecksumChanged(t *testing.T) {
	deploymentStore, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	buildRoot := t.TempDir()
	buildStore, err := checkruns.New(buildRoot)
	if err != nil {
		t.Fatal(err)
	}
	repo, release, actor := id('1'), id('2'), id('3')
	environment, err := deploymentStore.PutEnvironment(Environment{RepositoryID: repo, Name: "production", Position: 1, Image: "alpine:3.22", Command: "exit 99", TimeoutSeconds: 30, RequiredApprovals: 0, Concurrency: 1, UpdatedBy: actor})
	if err != nil {
		t.Fatal(err)
	}
	runs, err := buildStore.CreateRequested(repo, release, id('4'), []checkruns.Definition{{Name: "package", Image: "alpine:3.22", Command: "true", WorkingDirectory: ".", TimeoutSeconds: 30}}, actor)
	if err != nil {
		t.Fatal(err)
	}
	run := runs[0]
	artifactID := id('5')
	expected := sha256.Sum256([]byte("expected"))
	run.State = "succeeded"
	run.Artifacts = []checkruns.Artifact{{ID: artifactID, SHA256: hex.EncodeToString(expected[:])}}
	if err = buildStore.Update(run); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(buildRoot, repo, release, "artifacts", run.ID)
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, artifactID), []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	promotion, err := deploymentStore.CreatePromotion(Promotion{RepositoryID: repo, EnvironmentID: environment.ID, ReleaseID: release, BuildID: run.ID, ArtifactID: artifactID, ArtifactSHA256: hex.EncodeToString(expected[:]), InitiatedBy: actor})
	if err != nil {
		t.Fatal(err)
	}
	if err = NewExecutor(deploymentStore, buildStore).Execute(repo, promotion.ID); err != nil {
		t.Fatal(err)
	}
	promotion, err = deploymentStore.GetPromotion(repo, promotion.ID)
	if err != nil {
		t.Fatal(err)
	}
	if promotion.State != "failed" {
		t.Fatalf("state = %q", promotion.State)
	}
}

func TestRecoveryFailsInterruptedRunningPromotion(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, actor := id('6'), id('7')
	environment, err := store.PutEnvironment(Environment{RepositoryID: repo, Name: "staging", Position: 1, Image: "alpine:3.22", Command: "true", TimeoutSeconds: 30, RequiredApprovals: 0, Concurrency: 1, UpdatedBy: actor})
	if err != nil {
		t.Fatal(err)
	}
	promotion, err := store.CreatePromotion(Promotion{RepositoryID: repo, EnvironmentID: environment.ID, ReleaseID: id('8'), BuildID: id('9'), ArtifactID: id('a'), ArtifactSHA256: string(make([]byte, 64)), InitiatedBy: actor})
	if err != nil {
		t.Fatal(err)
	}
	promotion, err = store.Transition(repo, promotion.ID, "running", "claimed")
	if err != nil {
		t.Fatal(err)
	}
	builds, err := checkruns.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = NewExecutor(store, builds).Recover(); err != nil {
		t.Fatal(err)
	}
	promotion, err = store.GetPromotion(repo, promotion.ID)
	if err != nil {
		t.Fatal(err)
	}
	if promotion.State != "failed" {
		t.Fatalf("state = %q", promotion.State)
	}
}

func TestRecoveryPreservesLiveLeasedPromotion(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, actor := id('b'), id('c')
	environment, err := store.PutEnvironment(Environment{RepositoryID: repo, Name: "live", Position: 1, Image: "alpine:3.22", Command: "true", TimeoutSeconds: 30, RequiredApprovals: 0, Concurrency: 1, UpdatedBy: actor})
	if err != nil {
		t.Fatal(err)
	}
	promotion, err := store.CreatePromotion(Promotion{RepositoryID: repo, EnvironmentID: environment.ID, ReleaseID: id('d'), BuildID: id('e'), ArtifactID: id('f'), ArtifactSHA256: string(make([]byte, 64)), InitiatedBy: actor})
	if err != nil {
		t.Fatal(err)
	}
	owner := id('a')
	promotion, err = store.Claim(repo, promotion.ID, owner, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	builds, err := checkruns.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = NewExecutor(store, builds).Recover(); err != nil {
		t.Fatal(err)
	}
	promotion, err = store.GetPromotion(repo, promotion.ID)
	if err != nil {
		t.Fatal(err)
	}
	if promotion.State != "running" {
		t.Fatalf("state = %q", promotion.State)
	}
	if _, err = store.Complete(repo, promotion.ID, owner, "succeeded", "done"); err != nil {
		t.Fatalf("live completion = %v", err)
	}
}

func TestHeartbeatRenewsLiveExecutionLease(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, actor := id('1'), id('2')
	environment, err := store.PutEnvironment(Environment{RepositoryID: repo, Name: "live", Position: 1, Image: "alpine:3.22", Command: "true", TimeoutSeconds: 30, RequiredApprovals: 0, Concurrency: 1, UpdatedBy: actor})
	if err != nil {
		t.Fatal(err)
	}
	promotion, err := store.CreatePromotion(Promotion{RepositoryID: repo, EnvironmentID: environment.ID, ReleaseID: id('3'), BuildID: id('4'), ArtifactID: id('5'), ArtifactSHA256: string(make([]byte, 64)), InitiatedBy: actor})
	if err != nil {
		t.Fatal(err)
	}
	builds, err := checkruns.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	executor := NewExecutor(store, builds)
	executor.heartbeatInterval = time.Millisecond
	initial := time.Now().Add(time.Second)
	promotion, err = store.Claim(repo, promotion.ID, executor.owner, initial)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go executor.heartbeat(ctx, cancel, promotion, time.Minute, done)
	time.Sleep(10 * time.Millisecond)
	close(done)
	promotion, err = store.GetPromotion(repo, promotion.ID)
	if err != nil {
		t.Fatal(err)
	}
	if promotion.LeaseExpiresAt == nil || !promotion.LeaseExpiresAt.After(initial) {
		t.Fatalf("lease = %v, initial = %v", promotion.LeaseExpiresAt, initial)
	}
}

func TestPausedObservationRetainsExecutorUntilResume(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, actor, controller := id('d'), id('e'), id('f')
	environment, err := store.PutEnvironment(Environment{RepositoryID: repo, Name: "canary", Position: 1, Image: "alpine:3.22", Command: "true", TimeoutSeconds: 1, RequiredApprovals: 0, Concurrency: 1, UpdatedBy: actor})
	if err != nil {
		t.Fatal(err)
	}
	promotion, err := store.CreatePromotion(Promotion{RepositoryID: repo, EnvironmentID: environment.ID, ReleaseID: id('1'), BuildID: id('2'), ArtifactID: id('3'), ArtifactSHA256: string(make([]byte, 64)), InitiatedBy: actor})
	if err != nil {
		t.Fatal(err)
	}
	executor := NewExecutor(store, nil)
	promotion, err = store.Claim(repo, promotion.ID, executor.owner, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Control(repo, promotion.ID, controller, "pause", "running", "observe longer"); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- executor.waitAvailable(context.Background(), promotion, 10*time.Millisecond) }()
	select {
	case err := <-done:
		t.Fatalf("paused observation returned early: %v", err)
	case <-time.After(1100 * time.Millisecond):
	}
	if _, err = store.Control(repo, promotion.ID, controller, "resume", "paused", "continue"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("observation remained stranded after resume")
	}
	current, err := store.GetPromotion(repo, promotion.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.State != "running" || current.ExecutionOwner != executor.owner {
		t.Fatalf("resumed promotion = %#v", current)
	}
}

func TestUnavailableEnvironmentRejectsQueuedPromotion(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, actor := id('6'), id('7')
	environment, err := store.PutEnvironment(Environment{RepositoryID: repo, Name: "gone", Position: 1, Image: "alpine:3.22", Command: "true", TimeoutSeconds: 30, RequiredApprovals: 0, Concurrency: 1, UpdatedBy: actor})
	if err != nil {
		t.Fatal(err)
	}
	promotion, err := store.CreatePromotion(Promotion{RepositoryID: repo, EnvironmentID: environment.ID, ReleaseID: id('8'), BuildID: id('9'), ArtifactID: id('a'), ArtifactSHA256: string(make([]byte, 64)), InitiatedBy: actor})
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(filepath.Join(store.root, repo, "environments", environment.ID+".json")); err != nil {
		t.Fatal(err)
	}
	builds, err := checkruns.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = NewExecutor(store, builds).Execute(repo, promotion.ID); err != nil {
		t.Fatal(err)
	}
	promotion, err = store.GetPromotion(repo, promotion.ID)
	if err != nil {
		t.Fatal(err)
	}
	if promotion.State != "failed" {
		t.Fatalf("state = %q", promotion.State)
	}
}
