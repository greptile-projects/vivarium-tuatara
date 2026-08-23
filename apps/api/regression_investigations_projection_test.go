package main

import (
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/regressioninvestigations"
)

func TestRegressionEvidenceProjectionRecoversAuthoritativeSource(t *testing.T) {
	retained := regressioninvestigations.Evidence{Kind: "failed_check", ResourceID: "pull/run", Label: "failed run", Visibility: "repository", Available: true}
	stale := projectRegressionEvidence(retained, false)
	if stale.Available || !stale.Stale || stale.Diagnostic == "" {
		t.Fatalf("moved source was not downgraded: %#v", stale)
	}
	recovered := projectRegressionEvidence(stale, true)
	if !recovered.Available || recovered.Stale || recovered.Diagnostic != "" {
		t.Fatalf("recovered source retained a stale gap: %#v", recovered)
	}
}

func TestRegressionEvidenceProjectionKeepsUnresolvedSourceUnavailable(t *testing.T) {
	retained := regressioninvestigations.Evidence{Kind: "unsupported", ResourceID: "outside", Label: "unresolved", Visibility: "repository", Available: false, Diagnostic: "evidence does not resolve in this repository"}
	projected := projectRegressionEvidence(retained, false)
	if projected.Available || projected.Stale || projected.Diagnostic != retained.Diagnostic {
		t.Fatalf("unresolved evidence changed projection: %#v", projected)
	}
}

func TestRegressionSetupFailureUsesRetainedDockerStderr(t *testing.T) {
	if !regressionSetupFailure("exit status 125", "docker: Error response from daemon: No such image: unavailable:never") {
		t.Fatal("unavailable image was treated as behavioral evidence")
	}
	if regressionSetupFailure("exit status 1", "expected behavior differed") {
		t.Fatal("behavioral failure was treated as setup failure")
	}
	if regressionSetupFailure("exit status 1", "working directory fixture was retained; executable file not found in expected output") {
		t.Fatal("arbitrary behavioral logs were treated as setup failure")
	}
	if regressionSetupFailure("exit status 125", "comparison intentionally returned 125") {
		t.Fatal("unstructured exit 125 was treated as setup failure")
	}
}

func TestRegressionActiveRetryDoesNotBecomeFailure(t *testing.T) {
	for _, state := range []string{"queued", "running", "cleanup_pending"} {
		if !regressionRunActive(state) {
			t.Fatalf("%s run was considered terminal", state)
		}
	}
	for _, state := range []string{"succeeded", "failed", "cancelled"} {
		if regressionRunActive(state) {
			t.Fatalf("%s run was considered active", state)
		}
	}
}
