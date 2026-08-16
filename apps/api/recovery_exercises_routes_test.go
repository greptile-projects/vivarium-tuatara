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

func TestRecoveryExecutorRestoresFilesBeforeRunningSmokeJourney(t *testing.T) {
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
	if status, log, _, _ := run(recoveryexercises.Step{Kind: "journey", Command: "journey:smoke"}); status != "passed" || !strings.Contains(log, "executed") {
		t.Fatalf("journey = %q %q", status, log)
	}
}
