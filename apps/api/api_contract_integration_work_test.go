package main

import (
	"strings"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/apicontracts"
)

func TestIntegrationEvidenceRejectsCredentialsAndPayloadArtifacts(t *testing.T) {
	valid := apicontracts.IntegrationEvidence{Scenario: "consumer", Side: "consumer", Status: "passed", Logs: []string{"sanitized"}, Artifacts: []apicontracts.EvidenceArtifact{{Name: "report.json", SHA256: strings.Repeat("a", 64), Size: 10}}}
	if unsafeIntegrationEvidence(valid) {
		t.Fatal("bounded metadata was rejected")
	}
	valid.Logs = []string{"Authorization: Bearer vva_exposed"}
	if !unsafeIntegrationEvidence(valid) {
		t.Fatal("credential-shaped evidence was accepted")
	}
	valid.Logs = []string{"sanitized"}
	valid.Artifacts[0].SHA256 = "not-a-digest"
	if !unsafeIntegrationEvidence(valid) {
		t.Fatal("artifact content substitute was accepted")
	}
}
