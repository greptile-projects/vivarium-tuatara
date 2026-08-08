package deployments

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGovernedPromotionRetainsSecretAndExactArtifactHistory(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, owner, initiator, approver := id('1'), id('2'), id('3'), id('4')
	env, err := s.PutEnvironment(Environment{RepositoryID: repo, Name: "production", Position: 1, Image: "alpine:3.22", Command: "test -f \"$VIVARIUM_ARTIFACT\"", TimeoutSeconds: 30, Configuration: map[string]string{"REGION": "east"}, Credentials: map[string]string{"DEPLOY_TOKEN": "secret-value"}, RequiredApprovals: 1, Concurrency: 1, UpdatedBy: owner})
	if err != nil {
		t.Fatal(err)
	}
	if env.Credentials != nil || len(env.CredentialNames) != 1 || env.CredentialNames[0] != "DEPLOY_TOKEN" {
		t.Fatalf("public environment = %#v", env)
	}
	body, err := os.ReadFile(filepath.Join(s.root, repo, "environments", env.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) == "" || contains(string(body), "secret-value") {
		t.Fatal("credential leaked into environment record")
	}
	p, err := s.CreatePromotion(Promotion{RepositoryID: repo, EnvironmentID: env.ID, ReleaseID: id('5'), BuildID: id('6'), ArtifactID: id('7'), ArtifactSHA256: string(make([]byte, 64)), InitiatedBy: initiator})
	if err != nil || p.State != "pending_approval" {
		t.Fatalf("promotion = %#v, %v", p, err)
	}
	if _, err = s.Approve(repo, p.ID, initiator); err != ErrBlocked {
		t.Fatalf("self approval = %v", err)
	}
	p, err = s.Approve(repo, p.ID, approver)
	if err != nil || p.State != "queued" || len(p.Approvals) != 1 {
		t.Fatalf("approval = %#v, %v", p, err)
	}
	p, err = s.Transition(repo, p.ID, "running", "provisioned")
	if err != nil || p.StartedAt == nil {
		t.Fatal(err)
	}
	p, err = s.Transition(repo, p.ID, "succeeded", "deployed")
	if err != nil || p.CompletedAt == nil || len(p.Events) != 5 {
		t.Fatalf("completed = %#v, %v", p, err)
	}
}

func TestPromotionAdmissionHonorsEnvironmentConcurrency(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, owner := id('a'), id('b')
	env, err := s.PutEnvironment(Environment{RepositoryID: repo, Name: "staging", Position: 1, Image: "alpine:3.22", Command: "true", TimeoutSeconds: 30, RequiredApprovals: 0, Concurrency: 2, UpdatedBy: owner})
	if err != nil {
		t.Fatal(err)
	}
	makePromotion := func(release, build, artifact, actor byte) error {
		_, err := s.CreatePromotion(Promotion{RepositoryID: repo, EnvironmentID: env.ID, ReleaseID: id(release), BuildID: id(build), ArtifactID: id(artifact), ArtifactSHA256: string(make([]byte, 64)), InitiatedBy: id(actor)})
		return err
	}
	if err = makePromotion('1', '2', '3', '4'); err != nil {
		t.Fatal(err)
	}
	if err = makePromotion('5', '6', '7', '8'); err != nil {
		t.Fatalf("second promotion = %v", err)
	}
	if err = makePromotion('9', 'c', 'd', 'e'); err != ErrBlocked {
		t.Fatalf("third promotion = %v", err)
	}
}

func TestRollbackTargetSelectsNewestEarlierSuccessfulArtifact(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, actor := id('1'), id('2')
	environment, err := store.PutEnvironment(Environment{RepositoryID: repo, Name: "production", Position: 1, Image: "alpine:3.22", Command: "true", TimeoutSeconds: 30, RequiredApprovals: 0, Concurrency: 1, UpdatedBy: actor})
	if err != nil {
		t.Fatal(err)
	}
	create := func(release byte) Promotion {
		value, createErr := store.CreatePromotion(Promotion{RepositoryID: repo, EnvironmentID: environment.ID, ReleaseID: id(release), BuildID: id(release + 1), ArtifactID: id(release + 2), ArtifactSHA256: strings.Repeat(string(release), 64), InitiatedBy: actor})
		if createErr != nil {
			t.Fatal(createErr)
		}
		return value
	}
	first := create('3')
	if _, err = store.Transition(repo, first.ID, "running", "deploying"); err != nil {
		t.Fatal(err)
	}
	if first, err = store.Transition(repo, first.ID, "succeeded", "healthy"); err != nil {
		t.Fatal(err)
	}
	second := create('6')
	if _, err = store.Transition(repo, second.ID, "running", "deploying"); err != nil {
		t.Fatal(err)
	}
	if second, err = store.Transition(repo, second.ID, "failed", "unhealthy"); err != nil {
		t.Fatal(err)
	}
	unhealthy, target, err := store.RollbackTarget(repo, second.ID)
	if err != nil || unhealthy.ID != second.ID || target.ID != first.ID || target.ArtifactID != first.ArtifactID {
		t.Fatalf("rollback target = %#v, %#v, %v", unhealthy, target, err)
	}
	rollback, err := store.CreatePromotion(Promotion{RepositoryID: repo, EnvironmentID: target.EnvironmentID, ReleaseID: target.ReleaseID, BuildID: target.BuildID, ArtifactID: target.ArtifactID, ArtifactSHA256: target.ArtifactSHA256, InitiatedBy: actor, RecoveryOf: second.ID, RecoveryKind: "rollback", RestoresDeploymentID: first.ID})
	if err != nil || rollback.RecoveryOf != second.ID || rollback.ArtifactSHA256 != first.ArtifactSHA256 {
		t.Fatalf("rollback = %#v, %v", rollback, err)
	}
}

func TestRolloutControlsRetainAttributedDecisionsAndHealthEvidence(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, envID, actor, other := id('1'), id('2'), id('3'), id('4')
	_, err = store.PutEnvironment(Environment{RepositoryID: repo, Name: "production", Position: 1, Image: "alpine:3.22", Command: "true", TimeoutSeconds: 60, RequiredApprovals: 0, Concurrency: 1, UpdatedBy: actor})
	if err != nil {
		t.Fatal(err)
	}
	envs, _ := store.ListEnvironments(repo)
	envID = envs[0].ID
	definition, err := ParseRolloutDefinition([]byte(`{"version":1,"stages":[{"name":"canary","observation_seconds":0,"signals":[{"name":"errors","command":"test -f /vivarium/artifact"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	p, err := store.CreatePromotion(Promotion{RepositoryID: repo, EnvironmentID: envID, ReleaseID: id('5'), BuildID: id('6'), ArtifactID: id('7'), ArtifactSHA256: strings.Repeat("a", 64), CommitID: strings.Repeat("b", 40), Rollout: definition, InitiatedBy: actor})
	if err != nil {
		t.Fatal(err)
	}
	p, err = store.Claim(repo, p.ID, id('8'), time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	p, err = store.RecordStage(repo, p.ID, p.ExecutionOwner, 0, SignalEvidence{Stage: "canary", Signal: "errors", State: "passed", Message: "healthy"})
	if err != nil {
		t.Fatal(err)
	}
	p, err = store.Control(repo, p.ID, other, "pause", "running", "investigating elevated latency")
	if err != nil {
		t.Fatal(err)
	}
	if p.State != "paused" || len(p.Evidence) != 1 || p.Events[len(p.Events)-1].ActorID != other {
		t.Fatalf("paused promotion = %#v", p)
	}
	p, err = store.RecordStage(repo, p.ID, p.ExecutionOwner, 0, SignalEvidence{Stage: "canary", Signal: "latency", State: "passed", Message: "completed while paused"})
	if err != nil {
		t.Fatal(err)
	}
	if p.State != "paused" || len(p.Evidence) != 2 {
		t.Fatalf("paused signal evidence = %#v", p)
	}
	p, err = store.Control(repo, p.ID, other, "resume", "paused", "signal recovered")
	if err != nil {
		t.Fatal(err)
	}
	if p.State != "running" {
		t.Fatalf("state = %s", p.State)
	}
	p, err = store.Control(repo, p.ID, actor, "mark_unsuccessful", "running", "customer impact")
	if err != nil {
		t.Fatal(err)
	}
	if p.State != "failed" || p.CompletedAt == nil || p.Events[len(p.Events)-1].Message != "customer impact" {
		t.Fatalf("failed promotion = %#v", p)
	}
}

func TestOwnedFailedSignalTerminalizesPausedRollout(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, actor, controller, owner := id('a'), id('b'), id('c'), id('d')
	environment, err := store.PutEnvironment(Environment{RepositoryID: repo, Name: "production", Position: 1, Image: "alpine:3.22", Command: "true", TimeoutSeconds: 60, RequiredApprovals: 0, Concurrency: 1, UpdatedBy: actor})
	if err != nil {
		t.Fatal(err)
	}
	definition, err := ParseRolloutDefinition([]byte(`{"version":1,"stages":[{"name":"canary","observation_seconds":0,"signals":[{"name":"errors","command":"false"}]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	promotion, err := store.CreatePromotion(Promotion{RepositoryID: repo, EnvironmentID: environment.ID, ReleaseID: id('1'), BuildID: id('2'), ArtifactID: id('3'), ArtifactSHA256: strings.Repeat("a", 64), CommitID: strings.Repeat("b", 40), Rollout: definition, InitiatedBy: actor})
	if err != nil {
		t.Fatal(err)
	}
	promotion, err = store.Claim(repo, promotion.ID, owner, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Control(repo, promotion.ID, controller, "pause", "running", "investigate"); err != nil {
		t.Fatal(err)
	}
	promotion, err = store.RecordStage(repo, promotion.ID, owner, 0, SignalEvidence{Stage: "canary", Signal: "errors", State: "failed", Message: "threshold exceeded"})
	if err != nil {
		t.Fatal(err)
	}
	promotion, err = store.Complete(repo, promotion.ID, owner, "failed", "health signal failed")
	if err != nil {
		t.Fatal(err)
	}
	if promotion.State != "failed" || promotion.CompletedAt == nil || len(promotion.Evidence) != 1 {
		t.Fatalf("failed promotion = %#v", promotion)
	}
	if _, err = store.Complete(repo, promotion.ID, id('e'), "failed", "forged"); err != ErrBlocked {
		t.Fatalf("replacement completion = %v", err)
	}
}

func id(r byte) string {
	b := make([]byte, 32)
	for i := range b {
		b[i] = r
	}
	return string(b)
}
func contains(value, part string) bool {
	for i := 0; i+len(part) <= len(value); i++ {
		if value[i:i+len(part)] == part {
			return true
		}
	}
	return false
}
