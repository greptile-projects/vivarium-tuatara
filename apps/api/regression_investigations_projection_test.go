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
	if !regressionSetupFailure("setup") {
		t.Fatal("structured setup failure was treated as behavioral evidence")
	}
	for _, kind := range []string{"", "command", "exit status 125: error response from daemon: no such image:"} {
		if regressionSetupFailure(kind) {
			t.Fatalf("command-controlled value %q was treated as setup failure", kind)
		}
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

func TestCulpritRangesKeepFlakyAndMergeAmbiguityExplicit(t *testing.T) {
	candidates := []regressioninvestigations.SearchCandidate{{Revision: "good", Classification: "working"}, {Revision: "flaky", Classification: "flaky"}, {Revision: "merge", Classification: "regressed", Merge: true}}
	ranges := deriveCulpritRanges(candidates)
	if len(ranges) != 1 || ranges[0].Ambiguity == "" || ranges[0].Confidence >= 1 {
		t.Fatalf("ambiguous range collapsed: %#v", ranges)
	}
}
