package main

import (
	"strings"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/protectionplans"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/recoveryexercises"
)

func TestRecoveryExecutorAllowsOnlyDeclaredBoundedChecks(t *testing.T) {
	run := exerciseExecutor(protectionplans.RestoredSource{Manifest: []protectionplans.Entry{{Kind: "tree", Version: "tree"}, {Kind: "commit", Version: "commit", Dependencies: []string{"tree", "uncaptured-parent"}}}, Payload: []byte("production-secret")}, []string{"smoke"})
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
