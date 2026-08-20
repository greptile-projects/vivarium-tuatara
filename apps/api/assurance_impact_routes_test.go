package main

import (
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/assuranceprograms"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
)

func TestDeriveAssuranceImpactsSelectsOnlyMappedChangedControls(t *testing.T) {
	revision := assuranceprograms.Revision{
		Scopes: []assuranceprograms.Scope{{ID: "policy", Kind: "policy", Path: "policies/export.rego"}, {ID: "docs", Kind: "policy", Path: "docs/privacy.md"}},
		Controls: []assuranceprograms.Control{
			{ID: "export", Title: "Export authorization", OwnerIDs: []string{"security"}, Mappings: []assuranceprograms.Mapping{{ScopeID: "policy"}}, EvidenceCriteria: []assuranceprograms.EvidenceCriterion{{ID: "scenario"}}},
			{ID: "notice", Title: "Privacy notice", OwnerIDs: []string{"privacy"}, Mappings: []assuranceprograms.Mapping{{ScopeID: "docs"}}, EvidenceCriteria: []assuranceprograms.EvidenceCriterion{{ID: "review"}}},
		},
	}
	changes := []pullrequests.FileChange{{Path: "policies/export.rego"}}
	impacts := deriveAssuranceImpacts(revision, changes, map[string]assuranceImpactInputDecision{"export": {Applicability: "affected", Rationale: "export role broadened"}})
	if len(impacts) != 1 || impacts[0].ControlID != "export" || len(impacts[0].AffectedPaths) != 1 || impacts[0].ChangedEvidenceIDs[0] != "scenario" {
		t.Fatalf("impacts = %#v", impacts)
	}
	current := assuranceImpactCurrent("commit", revision, changes)
	if len(current.ControlDigests) != 1 || current.ControlDigests["export"] == "" {
		t.Fatalf("current = %#v", current)
	}
}
