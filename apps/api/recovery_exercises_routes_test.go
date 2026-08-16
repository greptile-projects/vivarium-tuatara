package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/protectionplans"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/recoveryexercises"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

func TestRecoveryExecutorAllowsOnlyDeclaredBoundedChecks(t *testing.T) {
	run, cleanup := newExerciseExecutor(protectionplans.RestoredSource{Manifest: []protectionplans.Entry{{Kind: "tree", Version: "tree"}, {Kind: "commit", Version: "commit", Dependencies: []string{"tree", "uncaptured-parent"}}}, Payload: []byte("production-secret")}, []string{"smoke"})
	defer cleanup()
	status, log, artifact, _ := run(recoveryexercises.Step{Kind: "integrity", Command: "verify:dependencies"})
	if status != "passed" || strings.Contains(log, "production-secret") || !strings.HasPrefix(artifact, "sha256:") {
		t.Fatalf("integrity result = %q %q %q", status, log, artifact)
	}
	status, _, _, _ = run(recoveryexercises.Step{Kind: "journey", Command: "journey:undeclared"})
	if status != "failed" {
		t.Fatalf("undeclared journey = %q", status)
	}
	status, _, _, _ = run(recoveryexercises.Step{Kind: "restore", Command: "shell:cat /etc/passwd"})
	if status != "failed" {
		t.Fatalf("unbounded command = %q", status)
	}
}

func TestRecoveryExecutorRejectsReadmeOnlySmokeJourney(t *testing.T) {
	content := []byte("usable system")
	sum := sha256.Sum256(content)
	payload, err := json.Marshal(map[string]any{"source": map[string]any{"objects": map[string]storage.Object{"blob": {ID: "blob", Type: storage.BlobObject, Size: int64(len(content)), Content: content}}}})
	if err != nil {
		t.Fatal(err)
	}
	run, cleanup := newExerciseExecutor(protectionplans.RestoredSource{Manifest: []protectionplans.Entry{{Path: "README.md", Kind: "blob", Version: "blob", SHA256: hex.EncodeToString(sum[:]), Size: int64(len(content))}}, Payload: payload}, []string{"smoke"})
	defer cleanup()
	if status, _, _, _ := run(recoveryexercises.Step{Kind: "journey", Command: "journey:smoke"}); status != "failed" {
		t.Fatalf("journey before restore = %q", status)
	}
	if status, _, artifact, _ := run(recoveryexercises.Step{Kind: "restore", Command: "restore:protected-manifest"}); status != "passed" || artifact != "restored artifacts: 1" {
		t.Fatalf("restore = %q %q", status, artifact)
	}
	if status, _, _, _ := run(recoveryexercises.Step{Kind: "journey", Command: "journey:smoke"}); status != "failed" {
		t.Fatalf("README-only journey = %q", status)
	}
}

func TestRecoveryExecutorRunsDeclaredHTTPApplicationJourney(t *testing.T) {
	health := []byte(`{"status":"healthy"}`)
	healthSum := sha256.Sum256(health)
	contract, err := json.Marshal(recoverySmokeContract{Version: "v1", Entrypoint: "public", RequestPath: "/health.json", ExpectedStatus: 200, ExpectedSHA256: hex.EncodeToString(healthSum[:])})
	if err != nil {
		t.Fatal(err)
	}
	contractSum := sha256.Sum256(contract)
	payload, err := json.Marshal(map[string]any{"source": map[string]any{"objects": map[string]storage.Object{
		"contract": {ID: "contract", Type: storage.BlobObject, Size: int64(len(contract)), Content: contract},
		"health":   {ID: "health", Type: storage.BlobObject, Size: int64(len(health)), Content: health},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	source := protectionplans.RestoredSource{Manifest: []protectionplans.Entry{
		{Path: ".vivarium/recovery-smoke.json", Kind: "blob", Version: "contract", SHA256: hex.EncodeToString(contractSum[:]), Size: int64(len(contract))},
		{Path: "public/health.json", Kind: "blob", Version: "health", SHA256: hex.EncodeToString(healthSum[:]), Size: int64(len(health))},
	}, Payload: payload}
	run, cleanup := newExerciseExecutor(source, []string{"smoke"})
	defer cleanup()
	if status, _, _, _ := run(recoveryexercises.Step{Kind: "restore", Command: "restore:protected-manifest"}); status != "passed" {
		t.Fatalf("restore = %q", status)
	}
	if status, log, _, _ := run(recoveryexercises.Step{Kind: "journey", Command: "journey:smoke"}); status != "passed" || !strings.Contains(log, "HTTP") {
		t.Fatalf("journey = %q %q", status, log)
	}
}
