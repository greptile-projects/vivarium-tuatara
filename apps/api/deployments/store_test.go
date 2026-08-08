package deployments

import (
	"os"
	"path/filepath"
	"testing"
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
