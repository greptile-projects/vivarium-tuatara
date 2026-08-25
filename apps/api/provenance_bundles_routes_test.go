package main

import (
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/packages"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/provenancebundles"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/provenancegraphs"
)

func TestRestrictedBundleAndNodeAudienceRequireExactReaderCoverage(t *testing.T) {
	bundle := provenancebundles.Bundle{Claim: provenancebundles.Claim{Audience: "restricted", AudienceIDs: []string{"alice", "bob"}}}
	if bundleReadableBy(bundle, "mallory") || !bundleReadableBy(bundle, "alice") {
		t.Fatal("bundle audience predicate did not enforce named readers")
	}
	node := provenancegraphs.Node{Audience: "restricted", Restricted: true, AudienceIDs: []string{"alice"}}
	if nodeVisibleToBundleAudience(node, "restricted", bundle.Claim.AudienceIDs) {
		t.Fatal("node visible to only one reader leaked into a two-reader bundle")
	}
	node.AudienceIDs = []string{"alice", "bob"}
	if !nodeVisibleToBundleAudience(node, "restricted", bundle.Claim.AudienceIDs) {
		t.Fatal("node visible to every bundle reader was omitted")
	}
}

func TestPackageProvenanceRequiresExactSignedArtifact(t *testing.T) {
	bundle := provenancebundles.Bundle{Claim: provenancebundles.Claim{Artifacts: []provenancebundles.Artifact{{Name: "alpha", Version: "1.0.0", SHA256: "alpha-digest"}}}}
	if !bundleCoversPackage(bundle, packages.Version{Name: "alpha", Version: "1.0.0", SHA256: "alpha-digest"}) {
		t.Fatal("exact signed artifact was not covered")
	}
	for _, candidate := range []packages.Version{{Name: "beta", Version: "1.0.0", SHA256: "alpha-digest"}, {Name: "alpha", Version: "2.0.0", SHA256: "alpha-digest"}, {Name: "alpha", Version: "1.0.0", SHA256: "other-digest"}} {
		if bundleCoversPackage(bundle, candidate) {
			t.Fatalf("uncovered package matched: %#v", candidate)
		}
	}
}
