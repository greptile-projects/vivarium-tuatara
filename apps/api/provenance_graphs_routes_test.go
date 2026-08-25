package main

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/provenancegraphs"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

func TestProvenanceAnalysisResolvesExactGitEvidenceAndRedactsRestrictedOrigin(t *testing.T) {
	git, _ := storage.New(t.TempDir())
	repo, _ := git.Create(strings.Repeat("a", 32))
	blob, _ := repo.WriteObject(storage.BlobObject, []byte("shipped\n"))
	tree := writeTestTree(t, repo, testTreeEntry{"100644", "output.txt", blob})
	commit := writeTestCommit(t, repo, tree, nil, 1700000000, "provenance")
	g := provenancegraphs.Graph{RequestID: "request", RepositoryID: strings.Repeat("a", 32), Revision: string(commit), CreatedBy: "owner", Nodes: []provenancegraphs.Node{{ID: "private", Kind: "dependency", Label: "embargoed-kit", License: "Private-1", Confidence: "verified", Audience: "restricted", AudienceIDs: []string{"owner"}, Citations: []provenancegraphs.Citation{{Kind: "attestation", ResourceID: "private-attestation"}}}, {ID: "output", Kind: "file", Label: "output.txt", Confidence: "verified", Audience: "public", Citations: []provenancegraphs.Citation{{Kind: "repository_file", Path: "output.txt", ResourceID: "forged", SHA256: "forged"}}}}, Edges: []provenancegraphs.Edge{{ID: "build", From: "private", To: "output", Transformation: "generated", Confidence: "verified", ToolNodeID: "private", Citation: provenancegraphs.Citation{Kind: "build", ResourceID: "private-command"}}}}
	if err := analyzeProvenanceGraph(&g, git); err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256([]byte("shipped\n"))
	citation := g.Nodes[1].Citations[0]
	if citation.ResourceID != string(blob) || citation.Revision != string(commit) || citation.SHA256 != hex.EncodeToString(want[:]) {
		t.Fatalf("server citation = %#v", citation)
	}
	g.Diagnostics = append(g.Diagnostics, provenancegraphs.Diagnostic{Kind: "private", NodeID: "private", Message: "embargoed-kit is missing", AttributedTo: "owner"})
	g.Diagnostics = append(g.Diagnostics, provenancegraphs.Diagnostic{Kind: "visible", NodeID: "output", Message: "Visible output needs review", AttributedTo: "owner"})
	reader := projectProvenanceGraph(g, "reader")
	if len(reader.Nodes) != 1 || reader.Nodes[0].ID != "output" || len(reader.Edges) != 0 || len(reader.Diagnostics) != 2 || reader.Diagnostics[0].Kind != "visible" || reader.Diagnostics[1].Kind != "restricted_provenance" {
		t.Fatalf("restricted evidence leaked: %#v", reader)
	}
	owner := projectProvenanceGraph(g, "owner")
	if owner.Nodes[0].Label != "embargoed-kit" || owner.Edges[0].Citation.ResourceID != "private-command" {
		t.Fatalf("audience lost evidence: %#v", owner)
	}
}

func TestProvenanceDiagnosticsUseExactMaterialIdentityForLicenseClaims(t *testing.T) {
	g := provenancegraphs.Graph{CreatedBy: "actor", Nodes: []provenancegraphs.Node{{ID: "output", Kind: "fragment", Label: "copied", Confidence: "unknown"}, {ID: "left", Kind: "license", Label: "terms", License: "MIT"}, {ID: "right", Kind: "license", Label: "terms", License: "GPL-3.0"}}, Edges: []provenancegraphs.Edge{}}
	ds := deriveProvenanceDiagnostics(g)
	kinds := map[string]bool{}
	for _, d := range ds {
		kinds[d.Kind] = true
	}
	for _, kind := range []string{"missing_origin", "unknown_origin"} {
		if !kinds[kind] {
			t.Fatalf("missing %s: %#v", kind, ds)
		}
	}
	if kinds["contradictory_license"] {
		t.Fatalf("distinct same-label materials conflicted: %#v", ds)
	}
	g.Edges = []provenancegraphs.Edge{{ID: "left-ordinary", From: "left", To: "output", Transformation: "packaged", Confidence: "verified"}, {ID: "right-ordinary", From: "right", To: "output", Transformation: "attested", Confidence: "verified"}}
	ds = deriveProvenanceDiagnostics(g)
	for _, d := range ds {
		if d.Kind == "contradictory_license" {
			t.Fatalf("ordinary transformations became claims: %#v", ds)
		}
	}
	g.Edges = []provenancegraphs.Edge{{ID: "left-claim", From: "left", To: "output", Transformation: "licensed_as", Confidence: "verified"}, {ID: "right-claim", From: "right", To: "output", Transformation: "licensed_as", Confidence: "verified"}}
	ds = deriveProvenanceDiagnostics(g)
	found := false
	for _, d := range ds {
		if d.Kind == "contradictory_license" && d.NodeID == "output" {
			found = true
		}
	}
	if !found {
		t.Fatalf("exact material conflict missing: %#v", ds)
	}
}
