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
	reader := projectProvenanceGraph(g, "reader")
	if reader.Nodes[0].Label != "Restricted provenance source" || reader.Nodes[0].License != "" || len(reader.Nodes[0].Citations) != 0 || reader.Edges[0].Citation.ResourceID != "" {
		t.Fatalf("restricted evidence leaked: %#v %#v", reader.Nodes[0], reader.Edges[0])
	}
	owner := projectProvenanceGraph(g, "owner")
	if owner.Nodes[0].Label != "embargoed-kit" || owner.Edges[0].Citation.ResourceID != "private-command" {
		t.Fatalf("audience lost evidence: %#v", owner)
	}
}

func TestProvenanceDiagnosticsKeepMissingAndContradictoryOriginExplicit(t *testing.T) {
	g := provenancegraphs.Graph{CreatedBy: "actor", Nodes: []provenancegraphs.Node{{ID: "output", Kind: "fragment", Label: "copied", Confidence: "unknown"}, {ID: "left", Kind: "license", Label: "terms", License: "MIT"}, {ID: "right", Kind: "license", Label: "terms", License: "GPL-3.0"}}, Edges: []provenancegraphs.Edge{}}
	ds := deriveProvenanceDiagnostics(g)
	kinds := map[string]bool{}
	for _, d := range ds {
		kinds[d.Kind] = true
	}
	for _, kind := range []string{"missing_origin", "unknown_origin", "contradictory_license"} {
		if !kinds[kind] {
			t.Fatalf("missing %s: %#v", kind, ds)
		}
	}
}
