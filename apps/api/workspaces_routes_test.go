package main

import (
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/debugworkspaces"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
)

func TestDebuggingReproductionSourceResolvesScenarioAudienceAndRevision(t *testing.T) {
	store, err := debugworkspaces.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	repo, owner, reader := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32)
	commit := strings.Repeat("a", 40)
	debugging, err := store.Create(debugworkspaces.Workspace{RepositoryID: repo, Title: "production behavior", Summary: "bounded observation", Trigger: debugworkspaces.Reference{Kind: "trace", Label: "trace"}, Release: debugworkspaces.Reference{ResourceID: strings.Repeat("4", 32), Revision: commit}, Environment: debugworkspaces.Reference{ResourceID: strings.Repeat("5", 32)}, TimeStart: now.Add(-time.Hour), TimeEnd: now, UserJourney: "checkout", OwnerIDs: []string{owner}, Severity: "high", Audience: "restricted", AccessUserIDs: []string{reader}, Source: debugworkspaces.Reference{Revision: commit}}, owner)
	if err != nil {
		t.Fatal(err)
	}
	debugging, err = store.AddClaim(repo, debugging.ID, owner, []debugworkspaces.Citation{{Kind: "commit", Label: "affected commit", Revision: commit, Accessible: true}}, debugworkspaces.Claim{Kind: "finding", Statement: "bounded behavior", Uncertainty: "one condition unknown", Confidence: "medium"}, debugging.Version)
	if err != nil {
		t.Fatal(err)
	}
	debugging, scenario, err := store.CreateReplay(repo, debugging.ID, owner, debugworkspaces.ReplayScenario{Title: "synthetic replay", Objective: "demonstrate behavior", EvidenceCitationIDs: []string{debugging.Citations[0].ID}, Inputs: []debugworkspaces.ReplayInput{{Name: "shape", Kind: "synthetic", Schema: "bounded generated shape", SHA256: strings.Repeat("b", 64), Sanitization: "generated only"}}, Commands: []debugworkspaces.ReplayCommand{{Name: "replay", SHA256: strings.Repeat("c", 64), Purpose: "run replay"}}, Invariants: []debugworkspaces.ReplayInvariant{{Name: "observed", CommandName: "replay", ExpectedExitCode: 0, Description: "behavior observed"}}, ProductionDifferences: []string{"synthetic state"}}, debugging.Version)
	if err != nil {
		t.Fatal(err)
	}
	source := workspaces.Source{Kind: "debugging_reproduction", RepositoryID: repo, DebuggingWorkspaceID: debugging.ID, ReplayScenarioID: scenario.ID}
	if err = validateWorkspaceSource(source, commit, reader, nil, nil, nil, nil, nil, store); err != nil {
		t.Fatalf("valid source rejected: %v", err)
	}
	if err = validateWorkspaceSource(source, strings.Repeat("d", 40), reader, nil, nil, nil, nil, nil, store); err == nil {
		t.Fatal("changed revision accepted")
	}
	source.ReplayScenarioID = strings.Repeat("e", 32)
	if err = validateWorkspaceSource(source, commit, reader, nil, nil, nil, nil, nil, store); err == nil {
		t.Fatal("forged scenario accepted")
	}
	source.ReplayScenarioID = scenario.ID
	if err = validateWorkspaceSource(source, commit, strings.Repeat("6", 32), nil, nil, nil, nil, nil, store); err == nil {
		t.Fatal("excluded audience accepted")
	}
}

func TestPrivateReplayWorkspaceEvidenceRequiresWorkspaceAudience(t *testing.T) {
	workspace := workspaces.Workspace{CreatorID: "creator", Policy: workspaces.Policy{Sharing: "private"}}
	if canReadReplayWorkspace(workspace, "other-collaborator", "repository-owner") {
		t.Fatal("private workspace evidence crossed to another collaborator")
	}
	if !canReadReplayWorkspace(workspace, "creator", "repository-owner") || !canReadReplayWorkspace(workspace, "repository-owner", "repository-owner") {
		t.Fatal("private workspace owner access was rejected")
	}
	workspace.Policy.Sharing = "repository"
	if !canReadReplayWorkspace(workspace, "other-collaborator", "repository-owner") {
		t.Fatal("repository-shared evidence was rejected")
	}
}

func TestReplayOutputRequiresFrozenScenarioCommand(t *testing.T) {
	hash := strings.Repeat("a", 64)
	commands := []debugworkspaces.ReplayCommand{{Name: "replay", SHA256: hash}}
	declared := map[string]workspaces.ExperimentCommand{"replay": {Name: "replay", Command: hash}}
	if name, ok := replayCommandForOutcome(commands, declared, workspaces.CommandOutcome{CommandSHA256: hash}); !ok || name != "replay" {
		t.Fatal("matching frozen command was rejected")
	}
	if _, ok := replayCommandForOutcome(commands, declared, workspaces.CommandOutcome{CommandSHA256: strings.Repeat("b", 64)}); ok {
		t.Fatal("unmatched workspace output gained replay provenance")
	}
	declared["replay"] = workspaces.ExperimentCommand{Name: "replay", Command: strings.Repeat("c", 64)}
	if _, ok := replayCommandForOutcome(commands, declared, workspaces.CommandOutcome{CommandSHA256: hash}); ok {
		t.Fatal("command absent from the exact workspace definition was accepted")
	}
}

func TestReproductionSecretScreeningRejectsCredentialFormats(t *testing.T) {
	tests := []struct{ name, body string }{
		{"input.txt", "GITHUB_TOKEN=ghp_example"},
		{"input.txt", "CLIENT_SECRET=example"},
		{".pypirc", "[pypi]\npassword = pypi-example"},
		{"key.txt", "-----BEGIN OPENSSH PRIVATE KEY-----\nexample"},
		{"input.txt", "Authorization: Basic example"},
		{"input.txt", "APIKEY=example"},
		{"input.txt", "ACCESSKEY=example"},
	}
	for _, test := range tests {
		if !reproductionSecretLike(test.name, base64.StdEncoding.EncodeToString([]byte(test.body))) {
			t.Errorf("accepted credential %q", test.name)
		}
	}
	if reproductionSecretLike("sample.json", base64.StdEncoding.EncodeToString([]byte(`{"failure":"timeout"}`))) {
		t.Fatal("rejected ordinary reproduction input")
	}
}

func TestReproductionArtifactPathValidationIsComponentAware(t *testing.T) {
	for _, value := range []string{"coverage..json", "reports/coverage..json", "/workspace/reports/result.json"} {
		if _, ok := cleanReproductionArtifactPath(value); !ok {
			t.Errorf("rejected confined artifact %q", value)
		}
	}
	for _, value := range []string{"../secret", "reports/../secret", "/etc/passwd", `reports\..\secret`} {
		if clean, ok := cleanReproductionArtifactPath(value); ok {
			t.Errorf("accepted escaping artifact %q as %q", value, clean)
		}
	}
}

func TestReproductionInputNamesRemainUniqueAfterSanitizing(t *testing.T) {
	first := reproductionInputName("attachment-one", "my file.txt")
	second := reproductionInputName("attachment-two", "my_file.txt")
	if first == second {
		t.Fatalf("colliding destinations: %q", first)
	}
}

func TestReproductionArtifactReadRejectsSymlinkEscape(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker unavailable")
	}
	if err := exec.Command("docker", "image", "inspect", "alpine:3.22").Run(); err != nil {
		t.Skip("alpine:3.22 unavailable")
	}
	id := strings.ToLower(strings.ReplaceAll(t.Name(), "/", "-"))
	container := "vivarium-workspace-" + id
	command := exec.Command("docker", "run", "-d", "--name", container, "--read-only", "--tmpfs", "/workspace:rw,noexec,nosuid,nodev,size=16m", "--tmpfs", "/tmp:rw,noexec,nosuid,nodev,size=4m", "alpine:3.22", "sleep", "60")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("start container: %v: %s", err, output)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", "-v", container).Run() })
	setup := exec.Command("docker", "exec", container, "sh", "-c", "mkdir -p /workspace/reports && printf retained >/workspace/reports/result.txt && printf outside >/tmp/secret && ln -s /tmp/secret /workspace/escaped.txt && ln -s /tmp /workspace/escaped-dir")
	if output, err := setup.CombinedOutput(); err != nil {
		t.Fatalf("setup: %v: %s", err, output)
	}
	if output, err := readReproductionArtifact(id, "reports/result.txt"); err != nil || string(output) != "retained" {
		t.Fatalf("regular artifact = %q, %v", output, err)
	}
	for _, path := range []string{"escaped.txt", "escaped-dir/secret"} {
		if output, err := readReproductionArtifact(id, path); err == nil {
			t.Fatalf("symlink artifact %q returned %q", path, output)
		}
	}
	if output, err := exec.Command("docker", "exec", container, "sh", "-c", "printf safe >/workspace/raced.txt").CombinedOutput(); err != nil {
		t.Fatalf("race setup: %v: %s", err, output)
	}
	tracer := exec.Command("docker", "exec", container, "sh", "-c", "while :; do rm -f /workspace/raced.next; ln -s /tmp/secret /workspace/raced.next; mv -f /workspace/raced.next /workspace/raced.txt; rm -f /workspace/raced.txt; printf safe >/workspace/raced.txt; done")
	if err := tracer.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tracer.Process.Kill(); _, _ = tracer.Process.Wait() }()
	for range 100 {
		output, err := readReproductionArtifact(id, "raced.txt")
		if err == nil && string(output) != "safe" {
			t.Fatalf("raced artifact escaped: %q", output)
		}
	}
}

func TestProvisionWorkspaceEnforcesStorageAndRemovesFailedContainer(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker unavailable")
	}
	if err := exec.Command("docker", "image", "inspect", "alpine:3.22").Run(); err != nil {
		t.Skip("alpine:3.22 unavailable")
	}
	repository := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		command := exec.Command(args[0], args[1:]...)
		command.Dir = repository
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v: %s", args, err, output)
		}
	}
	run("git", "init", "-q")
	run("git", "config", "user.name", "Workspace Test")
	run("git", "config", "user.email", "workspace@example.com")
	if err := os.WriteFile(filepath.Join(repository, "README.md"), []byte("exact\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run("git", "add", "README.md")
	run("git", "commit", "-qm", "exact")
	commitBody, err := exec.Command("git", "-C", repository, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, id, command string
		timeout           int
	}{
		{"storage quota", "0123456789abcdef0123456789abcdef", "dd if=/dev/zero of=too-large bs=1M count=129", 20},
		{"setup timeout", "abcdef0123456789abcdef0123456789", "sleep 5", 1},
		{"scratch quota", "11111111111111111111111111111111", "dd if=/dev/zero of=/tmp/too-large bs=1M count=17", 20},
		{"read only root", "22222222222222222222222222222222", "touch /root/escape", 20},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			steps, failed := provisionWorkspace(filepath.Join(repository, ".git"), t.TempDir(), test.id, strings.TrimSpace(string(commitBody)), workspaces.Definition{Image: "alpine:3.22", Setup: []string{test.command}, Resources: workspaces.Resources{CPUs: 1, MemoryMB: 256, StorageMB: 128, SetupSeconds: test.timeout}})
			if !failed || len(steps) != 1 || steps[0].State != "failed" || steps[0].Command != test.command {
				t.Fatalf("provision = %#v, failed %v", steps, failed)
			}
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				if err := exec.Command("docker", "inspect", "vivarium-workspace-"+test.id).Run(); err != nil {
					return
				}
				time.Sleep(50 * time.Millisecond)
			}
			t.Fatal("failed bounded container was not removed")
		})
	}
}

func TestProvisionWorkspaceRejectsImageDeclaredVolumesBeforeContainerCreation(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker unavailable")
	}
	if err := exec.Command("docker", "image", "inspect", "alpine:3.22").Run(); err != nil {
		t.Skip("alpine:3.22 unavailable")
	}
	contextDirectory := t.TempDir()
	dockerfile := []byte("FROM alpine:3.22\nVOLUME /image-volume\n")
	if err := os.WriteFile(filepath.Join(contextDirectory, "Dockerfile"), dockerfile, 0600); err != nil {
		t.Fatal(err)
	}
	image := "vivarium-workspace-volume-test:" + strings.ToLower(strings.ReplaceAll(t.Name(), "/", "-"))
	command := exec.Command("docker", "build", "-q", "-t", image, contextDirectory)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build volume image: %v: %s", err, output)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "image", "rm", "-f", image).Run() })
	id := "33333333333333333333333333333333"
	steps, failed := provisionWorkspace("unused", t.TempDir(), id, "unused", workspaces.Definition{Image: image, Resources: workspaces.Resources{CPUs: 1, MemoryMB: 256, StorageMB: 128, SetupSeconds: 20}})
	if !failed || len(steps) != 1 || steps[0].Command != "validate workspace image volumes" || !strings.Contains(steps[0].Output, "must not declare writable volumes") {
		t.Fatalf("provision = %#v, failed %v", steps, failed)
	}
	if err := exec.Command("docker", "inspect", "vivarium-workspace-"+id).Run(); err == nil {
		t.Fatal("container was created for image-declared volume")
	}
}

func TestWorkspaceLifecycleDoesNotCommitExpiryWhenTeardownFails(t *testing.T) {
	bin := t.TempDir()
	docker := filepath.Join(bin, "docker")
	if err := os.WriteFile(docker, []byte("#!/bin/sh\necho teardown failed >&2\nexit 73\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	store, err := workspaces.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	item, err := store.Create(workspaces.Workspace{RepositoryID: "repo", CommitID: "commit", CreatorID: "owner", Policy: workspaces.DefaultPolicy()}, []byte("definition"))
	if err != nil {
		t.Fatal(err)
	}
	item, err = store.Complete(item.ID, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	expired := time.Now().UTC().Add(-time.Minute)
	item.ExpiresAt = &expired
	if _, err = reconcileWorkspaceLifecycle(store, item, "workspace-lifecycle", time.Now().UTC()); err == nil {
		t.Fatal("teardown failure was ignored")
	}
	retained, err := store.Get(item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if retained.State != "running" || retained.StoppedAt != nil {
		t.Fatalf("workspace terminalized before teardown: %#v", retained)
	}
}
