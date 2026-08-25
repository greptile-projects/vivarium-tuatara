package main

import (
	"encoding/base64"
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

func TestBundleVerificationRejectsTamperedOrMalformedEvidence(t *testing.T) {
	store, err := provenancebundles.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := store.Create(provenancebundles.Bundle{RequestID: "verify", Claim: provenancebundles.Claim{Schema: "https://vivarium.dev/provenance-bundle/v1", RepositoryID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ReleaseID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", ReleaseVersion: "v1", Revision: "cccccccccccccccccccccccccccccccccccccccc", GraphID: "graph", GraphDigest: "digest", AssessmentID: "assessment", AssessmentVersion: 1, PolicyID: "policy", PolicyVersion: 1, Audience: "public", Artifacts: []provenancebundles.Artifact{{ID: "artifact", Name: "sdk", Version: "1.0.0", SHA256: "abcd"}}, Verification: []string{"verify"}, PublishedBy: "owner"}})
	if err != nil {
		t.Fatal(err)
	}
	if !bundleVerificationValid(bundle) {
		t.Fatal("fresh signed bundle did not verify")
	}
	tests := map[string]func(*provenancebundles.Bundle){
		"payload": func(b *provenancebundles.Bundle) {
			b.Payload = base64.RawURLEncoding.EncodeToString([]byte("tampered"))
		},
		"claim":     func(b *provenancebundles.Bundle) { b.Claim.ReleaseVersion = "substituted" },
		"digest":    func(b *provenancebundles.Bundle) { b.PayloadSHA256 = "00" },
		"signature": func(b *provenancebundles.Bundle) { b.Signature = "not-base64!" },
		"key":       func(b *provenancebundles.Bundle) { b.PublicKey = base64.RawURLEncoding.EncodeToString([]byte("short")) },
		"algorithm": func(b *provenancebundles.Bundle) { b.Algorithm = "unknown" },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			candidate := bundle
			mutate(&candidate)
			if bundleVerificationValid(candidate) {
				t.Fatal("tampered bundle reported valid")
			}
		})
	}
}
