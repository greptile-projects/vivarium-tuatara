package securityadvisories

import "testing"

func TestDiagnosticEvidenceImpactAndBoundedInvestigation(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	actor := "0123456789abcdef0123456789abcdef"
	repository := "abcdef0123456789abcdef0123456789"
	v, err := store.Create(Advisory{Title: "Boundary bypass", Description: "A parser boundary may be bypassed.", Contact: "security@example.test", ReporterID: actor, AffectedRepositories: []AffectedRepository{{RepositoryID: repository, Versions: []string{"1.x"}}}, Evidence: []Evidence{{Label: "Reproduction", Description: "A bounded reproduction."}}})
	if err != nil {
		t.Fatal(err)
	}
	v, err = store.AddEvidence(v.ID, actor, Evidence{Kind: "dependency", RepositoryID: repository, Dependency: "parser@1.4.0", Label: "Resolved dependency", Description: "Lockfile resolution."})
	if err != nil {
		t.Fatal(err)
	}
	dependency := v.Evidence[1].ID
	v, err = store.AddFinding(v.ID, actor, "hypothesis", "The vulnerable parser is reachable.", "", []string{dependency})
	if err != nil {
		t.Fatal(err)
	}
	v, err = store.SetImpact(v.ID, actor, v.Version, Impact{RepositoryID: repository, VersionLine: "1.x", Environment: "production", State: "confirmed", EvidenceIDs: []string{dependency}, Rationale: "The production artifact contains this resolution."})
	if err != nil {
		t.Fatal(err)
	}
	credential := "11111111111111111111111111111111"
	v, investigation, err := store.StartInvestigation(v.ID, actor, credential, credential, "Determine exploitability from selected evidence.", []string{dependency})
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Findings) != 1 || len(v.ImpactMatrix) != 1 || len(investigation.Evidence) != 1 {
		t.Fatalf("diagnostic record incomplete: %#v", v)
	}
	if _, _, err = store.Investigation(v.ID, investigation.ID, actor); err != ErrNotFound {
		t.Fatalf("unbound credential accessed investigation: %v", err)
	}
}
