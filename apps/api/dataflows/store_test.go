package dataflows

import (
	"strings"
	"testing"
)

func declaration(revision string) Revision {
	ref := CommitmentRef{CommitmentID: "commitment", Version: 2, DataUseIDs: []string{"events"}}
	return Revision{CodeRevision: revision, Title: "Telemetry path", EntryPoints: []string{"click"}, CommitmentRefs: []CommitmentRef{ref}, Rationale: "Trace the retained copies", Nodes: []Node{{ID: "click", Kind: "interaction", Name: "Save", Accessible: true}, {ID: "api", Kind: "interface", Name: "POST /events", Accessible: true}, {ID: "vendor", Kind: "external_recipient", Name: "Processor", Accessible: false, Uncertainty: "subprocessor inventory unavailable"}}, Edges: []Edge{{ID: "submit", From: "click", To: "api", Operation: "submit", DataCategories: []string{"usage"}, Purpose: "reliability", CommitmentRefs: []CommitmentRef{ref}}, {ID: "forward", From: "api", To: "vendor", Operation: "forward", DataCategories: []string{"usage"}, Purpose: "reliability", RetainedCopy: true, CommitmentRefs: []CommitmentRef{ref}}}}
}

func TestDataFlowLifecycleProjectsPermissionAndStaleness(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rev := strings.Repeat("a", 40)
	flow, err := store.Create("repo", "human", declaration(rev))
	if err != nil {
		t.Fatal(err)
	}
	if len(flow.Diagnostics) != 2 {
		t.Fatalf("expected inaccessible and uncertainty diagnostics, got %#v", flow.Diagnostics)
	}
	analysis := Analysis{MapVersion: 1, CodeRevision: rev, Status: "completed", Bounds: []string{"apps/web/src"}, Findings: []Finding{{Kind: "undeclared_flow", Severity: "blocking", Summary: "An additional processor call is observed.", EdgeIDs: []string{"forward"}, Citations: []Citation{{Path: "apps/web/src/event.ts", StartLine: 10, EndLine: 12, Claim: "sends the usage event"}}, Uncertainty: "runtime flag not evaluated"}}}
	flow, err = store.AddAnalysis("repo", flow.ID, "agent", "readonly-agent", analysis)
	if err != nil {
		t.Fatal(err)
	}
	if flow.Analyses[0].Findings[0].AddedByType != "agent" {
		t.Fatal("agent attribution missing")
	}
	next := declaration(strings.Repeat("b", 40))
	next.Rationale = "Code moved"
	flow, err = store.Revise("repo", flow.ID, 1, "human", next)
	if err != nil {
		t.Fatal(err)
	}
	foundStale := false
	for _, d := range flow.Diagnostics {
		if d.Kind == "stale_analysis" {
			foundStale = true
		}
	}
	if !foundStale {
		t.Fatalf("successor did not mark earlier analysis stale: %#v", flow.Diagnostics)
	}
}

func TestDataFlowRejectsUnboundedOrPayloadLikeEvidence(t *testing.T) {
	store, _ := New(t.TempDir())
	revision := declaration(strings.Repeat("a", 40))
	invalidEdge := revision
	invalidEdge.Edges = append([]Edge(nil), revision.Edges...)
	invalidEdge.Edges[0].CommitmentRefs = []CommitmentRef{{CommitmentID: "commitment", Version: 2, DataUseIDs: []string{}}}
	if _, err := store.Create("repo", "human", invalidEdge); err != ErrInvalid {
		t.Fatalf("expected empty edge data-use binding to be invalid, got %v", err)
	}
	flow, _ := store.Create("repo", "human", revision)
	bad := Analysis{MapVersion: 1, CodeRevision: strings.Repeat("a", 40), Findings: []Finding{{Kind: "confirmed", Summary: "uncited"}}}
	if _, err := store.AddAnalysis("repo", flow.ID, "human", "human", bad); err != ErrInvalid {
		t.Fatalf("expected invalid bounded analysis, got %v", err)
	}
}
