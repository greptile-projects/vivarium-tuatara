package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
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

func TestRecoveryExecutorRunsDeclaredContainerApplicationJourney(t *testing.T) {
	output := []byte("healthy")
	outputSum := sha256.Sum256(output)
	entrypoint := []byte("#!/bin/sh\nprintf healthy")
	contract, err := json.Marshal(recoverySmokeContract{Version: "v1", Image: "alpine@sha256:" + strings.Repeat("a", 64), Entrypoint: "bin/smoke", TimeoutSeconds: 10, ExpectedExitCode: 0, ExpectedStdoutSHA256: hex.EncodeToString(outputSum[:])})
	if err != nil {
		t.Fatal(err)
	}
	contractSum := sha256.Sum256(contract)
	payload, err := json.Marshal(map[string]any{"source": map[string]any{"objects": map[string]storage.Object{
		"contract": {ID: "contract", Type: storage.BlobObject, Size: int64(len(contract)), Content: contract},
		"entry":    {ID: "entry", Type: storage.BlobObject, Size: int64(len(entrypoint)), Content: entrypoint},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	source := protectionplans.RestoredSource{Manifest: []protectionplans.Entry{
		{Path: ".vivarium/recovery-smoke.json", Kind: "blob", Version: "contract", SHA256: hex.EncodeToString(contractSum[:]), Size: int64(len(contract))},
		{Path: "bin/smoke", Kind: "blob", Version: "entry", SHA256: hex.EncodeToString(sha256Bytes(entrypoint)), Size: int64(len(entrypoint))},
	}, Payload: payload}
	priorRunner := executeRecoveryApplication
	executeRecoveryApplication = func(root string, got recoverySmokeContract) (int, []byte, error) {
		body, readErr := os.ReadFile(filepath.Join(root, got.Entrypoint))
		if readErr != nil || string(body) != string(entrypoint) {
			t.Fatalf("restored entrypoint = %q, %v", body, readErr)
		}
		return 0, output, nil
	}
	defer func() { executeRecoveryApplication = priorRunner }()
	run, cleanup := newExerciseExecutor(source, []string{"smoke"})
	defer cleanup()
	if status, _, _, _ := run(recoveryexercises.Step{Kind: "restore", Command: "restore:protected-manifest"}); status != "passed" {
		t.Fatalf("restore = %q", status)
	}
	if status, log, _, _ := run(recoveryexercises.Step{Kind: "journey", Command: "journey:smoke"}); status != "passed" || !strings.Contains(log, "application") {
		t.Fatalf("journey = %q %q", status, log)
	}
}

func sha256Bytes(value []byte) []byte { sum := sha256.Sum256(value); return sum[:] }

func TestRecoveryApplicationContainerIsBounded(t *testing.T) {
	contract := recoverySmokeContract{Image: "app@sha256:" + strings.Repeat("a", 64), Entrypoint: "bin/smoke", Arguments: []string{"--check"}}
	args := strings.Join(recoveryContainerArguments("/isolated", "exercise", contract), " ")
	for _, required := range []string{"--pull=never", "--network=none", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--pids-limit=64", "--memory=128m", "src=/isolated,dst=/workspace,readonly", "app@sha256:", "./bin/smoke --check"} {
		if !strings.Contains(args, required) {
			t.Fatalf("missing %q in %s", required, args)
		}
	}
}
