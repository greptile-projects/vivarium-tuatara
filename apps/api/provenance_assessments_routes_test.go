package main

import (
	"github.com/greptile-projects/vivarium-tuatara/apps/api/provenancegraphs"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/provenancepolicies"
	"testing"
)

func TestProvenanceAssessmentDerivesLicenseGenerationAndDistributionObligations(t *testing.T) {
	g := provenancegraphs.Graph{Nodes: []provenancegraphs.Node{{ID: "tool", Kind: "tool", Label: "generator", Revision: "tool-v1"}, {ID: "output", Kind: "artifact", Label: "generated.js", License: "GPL-3.0", Obligations: []string{"offer corresponding source"}}}, Edges: []provenancegraphs.Edge{{From: "tool", To: "output", Transformation: "generated"}}, Diagnostics: []provenancegraphs.Diagnostic{{Kind: "missing_origin", Severity: "blocking", NodeID: "output", Message: "origin missing"}}}
	p := provenancepolicies.Revision{OwnerIDs: []string{"owner"}, Rules: []provenancepolicies.MaterialRule{{Kind: "generated_code", PermittedLicenses: []string{"MIT"}, RequiredAttribution: []string{"include copyright notice"}, ContributorAttestations: []string{"origin attestation"}, ReviewOwnerIDs: []string{"reviewer"}, DistributionContexts: []string{"public package"}}}}
	findings := deriveProvenanceFindings(g, p, []string{"public package", "desktop binary"})
	kinds := map[string]bool{}
	for _, f := range findings {
		kinds[f.Kind] = true
		if len(f.DistributionTargets) != 2 {
			t.Fatalf("distribution targets = %#v", f)
		}
	}
	for _, kind := range []string{"incompatible_license", "generated_output", "source_offer", "required_notice", "contributor_attestation", "missing_origin"} {
		if !kinds[kind] {
			t.Fatalf("missing %s in %#v", kind, findings)
		}
	}
}

func TestProvenanceAssessmentRejectsClaimOnlyEdgesAsOrigins(t *testing.T) {
	policy := provenancepolicies.Revision{Rules: []provenancepolicies.MaterialRule{{Kind: "source", PermittedLicenses: []string{"MIT"}, ReviewOwnerIDs: []string{"reviewer"}}}}
	for _, transformation := range []string{"licensed_as", "attested"} {
		graph := provenancegraphs.Graph{Nodes: []provenancegraphs.Node{{ID: "claim", Kind: "license", Label: "MIT", License: "MIT"}, {ID: "material", Kind: "file", Label: "main.go", License: "MIT"}}, Edges: []provenancegraphs.Edge{{From: "claim", To: "material", Transformation: transformation}}}
		findings := deriveProvenanceFindings(graph, policy, nil)
		found := false
		for _, finding := range findings {
			if finding.Kind == "unattributed_material" && finding.NodeID == "material" && finding.Severity == "blocking" {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s claim satisfied production origin: %#v", transformation, findings)
		}
	}
}
