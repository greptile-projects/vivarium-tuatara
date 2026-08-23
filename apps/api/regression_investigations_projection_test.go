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
