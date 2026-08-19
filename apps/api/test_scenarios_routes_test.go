package main

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/qualityplans"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/testscenarios"
)

func TestScenarioProvenanceResolvesExactBranchAssetsAndQualityIntent(t *testing.T) {
	git, _ := storage.New(t.TempDir())
	repo, _ := git.Create("repo")
	testBody, fixtureBody, secretBody, rationale := []byte("test checkout"), []byte(`{"shopper":"synthetic"}`), []byte(`{"api_key":"sk-sensitivefixture273"}`), []byte("expected behavior")
	testBlob, _ := repo.WriteObject(storage.BlobObject, testBody)
	fixtureBlob, _ := repo.WriteObject(storage.BlobObject, fixtureBody)
	secretBlob, _ := repo.WriteObject(storage.BlobObject, secretBody)
	rationaleBlob, _ := repo.WriteObject(storage.BlobObject, rationale)
	tree := writeTestTree(t, repo,
		testTreeEntry{mode: "40000", name: "docs", id: writeTestTree(t, repo, testTreeEntry{mode: "100644", name: "journey.md", id: rationaleBlob})},
		testTreeEntry{mode: "40000", name: "fixtures", id: writeTestTree(t, repo, testTreeEntry{mode: "100644", name: "secret.json", id: secretBlob}, testTreeEntry{mode: "100644", name: "shopper.json", id: fixtureBlob})},
		testTreeEntry{mode: "40000", name: "tests", id: writeTestTree(t, repo, testTreeEntry{mode: "100644", name: "checkout.test.ts", id: testBlob})},
	)
	commit := writeTestCommit(t, repo, tree, nil, 1, "scenario")
	if err := repo.CreateReference(storage.Reference{Name: "refs/heads/scenario/decline", Target: string(commit)}); err != nil {
		t.Fatal(err)
	}
	plans, _ := qualityplans.New(t.TempDir())
	planRevision := qualityAPIRevision("owner")
	planRevision.Scopes[0] = qualityplans.Scope{Kind: "journey", ResourceID: "checkout", Name: "Checkout"}
	plan, err := plans.Create("repo", "owner", planRevision)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(fixtureBody)
	v := testscenarios.Scenario{Title: "Decline", Purpose: "Prove safe recovery", QualityPlanID: plan.ID, QualityPlanVersion: 1, RequirementIDs: []string{"pay"}, Sources: []testscenarios.Source{{Kind: "user_journey", ResourceID: "checkout", Revision: string(commit), Path: "docs/journey.md", Summary: "Reviewed journey"}}, Parameters: []testscenarios.Parameter{{Name: "decline", Description: "Decline response", Type: "boolean"}}, Preconditions: []testscenarios.Step{{ID: "ready", Description: "Service ready", Operation: "health check"}}, Actions: []testscenarios.Step{{ID: "submit", Description: "Submit", Operation: "request", Parameters: []string{"decline"}}}, Assertions: []testscenarios.Assertion{{ID: "safe", Description: "No order", Matcher: "equals", Expected: "0"}}, Fixtures: []testscenarios.Fixture{{ID: "shopper", Kind: "synthetic", Description: "Synthetic shopper", Path: "fixtures/shopper.json", SHA256: hex.EncodeToString(sum[:]), DataClass: "synthetic", Assumptions: []string{"No identity"}, SourceIDs: []string{"checkout"}}}, Environments: []testscenarios.Environment{{ID: "local", Description: "Local", Runtime: "bun", Requirements: []string{"loopback"}}}, Cases: []testscenarios.Case{{ID: "decline", Name: "Decline", Values: map[string]string{"decline": "true"}, Assumptions: []string{"deterministic"}, ExpectedOutcome: "No order"}}, Implementation: testscenarios.Implementation{AuthoredByType: "human", Branch: "scenario/decline", CommitID: string(commit), TestPaths: []string{"tests/checkout.test.ts"}, Command: "bun test", Framework: "bun:test", Assumptions: []string{"local"}, Provenance: []string{"journey"}}}
	if !testscenarios.Valid(v) {
		t.Fatal("scenario shape invalid")
	}
	if ref, e := repo.ReadReference("refs/heads/scenario/decline"); e != nil || ref.Target != string(commit) {
		t.Fatalf("ref=%#v %v", ref, e)
	}
	for _, path := range []string{"tests/checkout.test.ts", "fixtures/shopper.json", "docs/journey.md"} {
		if _, _, ok := infrastructureCommitBlob(git, "repo", string(commit), path); !ok {
			t.Fatalf("blob %s missing", path)
		}
	}
	if !testScenarioProvenance(git, "repo", v, plans, nil, nil, testScenarioSources{}) {
		t.Fatal("valid exact-revision scenario was rejected")
	}
	v.Fixtures[0].SHA256 = hex.EncodeToString(sha256.New().Sum(nil))
	if testScenarioProvenance(git, "repo", v, plans, nil, nil, testScenarioSources{}) {
		t.Fatal("mismatched fixture digest was accepted")
	}
	v.Fixtures[0].Path = "fixtures/secret.json"
	secretSum := sha256.Sum256(secretBody)
	v.Fixtures[0].SHA256 = hex.EncodeToString(secretSum[:])
	if testScenarioProvenance(git, "repo", v, plans, nil, nil, testScenarioSources{}) {
		t.Fatal("secret-shaped fixture content was accepted")
	}
	v.Fixtures[0].Path, v.Fixtures[0].SHA256 = "fixtures/shopper.json", hex.EncodeToString(sum[:])
	v.Sources[0].ResourceID = "nonexistent-unrelated-resource-273"
	if testScenarioProvenance(git, "repo", v, plans, nil, nil, testScenarioSources{}) {
		t.Fatal("nonexistent rationale resource was accepted")
	}
}
