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
	candidates := []regressioninvestigations.SearchCandidate{{Kind: "commit", Revision: "good", Classification: "working"}, {Kind: "commit", Revision: "flaky", Parents: []string{"good"}, Classification: "flaky"}, {Kind: "commit", Revision: "merge", Parents: []string{"flaky"}, Classification: "regressed", Merge: true}}
	ranges := deriveCulpritRanges(candidates)
	if len(ranges) != 1 || ranges[0].Ambiguity == "" || ranges[0].Confidence >= 1 {
		t.Fatalf("ambiguous range collapsed: %#v", ranges)
	}
}

func TestCulpritRangesRejectTopoOrderedMergeSiblings(t *testing.T) {
	candidates := []regressioninvestigations.SearchCandidate{
		{Kind: "commit", Revision: "base", Classification: "working"},
		{Kind: "commit", Revision: "main", Parents: []string{"base"}, Classification: "working"},
		{Kind: "commit", Revision: "feature", Parents: []string{"base"}, Classification: "regressed"},
		{Kind: "commit", Revision: "merge", Parents: []string{"main", "feature"}, Merge: true},
	}
	for _, culprit := range deriveCulpritRanges(candidates) {
		if culprit.WorkingRevision == "main" && culprit.RegressedRevision == "feature" {
			t.Fatalf("non-ancestral siblings formed a range: %#v", culprit)
		}
	}
}

func TestSearchAttemptProjectionIsScopedToFrozenScenario(t *testing.T) {
	candidate := regressioninvestigations.SearchCandidate{Kind: "commit", Revision: "revision"}
	attempts := []regressioninvestigations.Attempt{
		{ID: "other", ScenarioID: "scenario-a", Revision: "revision", State: "completed", Classification: "failed"},
		{ID: "selected", ScenarioID: "scenario-b", Revision: "revision", State: "completed", Classification: "passed"},
	}
	projectRegressionCandidateAttempts(&candidate, "scenario-b", attempts)
	if candidate.Classification != "working" || len(candidate.AttemptIDs) != 1 || candidate.AttemptIDs[0] != "selected" {
		t.Fatalf("cross-scenario attempt projected: %#v", candidate)
	}
}

func TestSearchAttemptProjectionIncludesExactDependencyCandidate(t *testing.T) {
	attempt := regressioninvestigations.Attempt{ID: "attempt", ScenarioID: "scenario", Revision: "primary", State: "completed", Classification: "failed", Dependencies: []regressioninvestigations.Dependency{{RepositoryID: "dependency-repo", Revision: "dependency-revision"}}}
	wrongRepository := regressioninvestigations.SearchCandidate{Kind: "dependency", RepositoryID: "other-repo", Revision: "dependency-revision"}
	projectRegressionCandidateAttempts(&wrongRepository, "scenario", []regressioninvestigations.Attempt{attempt})
	if len(wrongRepository.AttemptIDs) != 0 || wrongRepository.Classification != "" {
		t.Fatalf("cross-repository dependency attempt projected: %#v", wrongRepository)
	}
	candidate := regressioninvestigations.SearchCandidate{Kind: "dependency", RepositoryID: "dependency-repo", Revision: "dependency-revision"}
	projectRegressionCandidateAttempts(&candidate, "scenario", []regressioninvestigations.Attempt{attempt})
	if candidate.Classification != "regressed" || len(candidate.AttemptIDs) != 1 || candidate.AttemptIDs[0] != "attempt" {
		t.Fatalf("exact dependency attempt omitted: %#v", candidate)
	}
}

func TestSearchAttemptProjectionKeepsConflictingEvidenceFlakyRegardlessOfOrder(t *testing.T) {
	passed := regressioninvestigations.Attempt{ID: "passed", ScenarioID: "scenario", Revision: "revision", State: "completed", Classification: "passed"}
	failed := regressioninvestigations.Attempt{ID: "failed", ScenarioID: "scenario", Revision: "revision", State: "completed", Classification: "failed"}
	for _, attempts := range [][]regressioninvestigations.Attempt{{passed, failed}, {failed, passed}} {
		candidate := regressioninvestigations.SearchCandidate{Kind: "commit", Revision: "revision"}
		projectRegressionCandidateAttempts(&candidate, "scenario", attempts)
		if candidate.Classification != "flaky" || len(candidate.AttemptIDs) != 2 {
			t.Fatalf("conflicting evidence collapsed by order: %#v", candidate)
		}
	}
	boundary := regressioninvestigations.SearchCandidate{Kind: "commit", Revision: "revision", Classification: "working"}
	projectRegressionCandidateAttempts(&boundary, "scenario", []regressioninvestigations.Attempt{failed})
	if boundary.Classification != "flaky" {
		t.Fatalf("attempt contradiction erased retained guidance: %#v", boundary)
	}
}
