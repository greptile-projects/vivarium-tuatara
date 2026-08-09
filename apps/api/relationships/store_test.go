package relationships

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCompatibilityConstraints(t *testing.T) {
	for _, test := range []struct {
		version, constraint string
		want                bool
	}{
		{"v1.4.2", ">=v1.0.0 <v2.0.0", true},
		{"v2.0.0", ">=v1.0.0 <v2.0.0", false},
		{"v3.2.1", "=v3.2.1", true},
		{"v3.2.1", "*", true},
	} {
		if got := Satisfies(test.version, test.constraint); got != test.want {
			t.Fatalf("Satisfies(%q, %q) = %v", test.version, test.constraint, got)
		}
	}
}

func TestEvolutionDecisionRetainsImpactAcknowledgementAndAgentUncertainty(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	provider := "0123456789abcdef0123456789abcdef"
	actor := "abcdef0123456789abcdef0123456789"
	consumer := "11111111111111111111111111111111"
	predecessor := Interface{ID: "22222222222222222222222222222222", RepositoryID: provider, Name: "events", Version: "v1.0.0", ReleaseID: "33333333333333333333333333333333", CommitID: strings.Repeat("a", 40), PublishedBy: actor, PublishedAt: time.Now()}
	plan, err := store.CreateEvolution(Evolution{RepositoryID: provider, InterfaceName: "events", Predecessor: predecessor, SourceKind: "proposal", SourceID: "44444444444444444444444444444444", CandidateDescription: "remove the legacy event field", Changes: []CompatibilityChange{{Kind: "field removal", Summary: "old consumers still read it", Classification: "breaking"}}, Impacts: []ConsumerImpact{{RepositoryID: consumer, OwnerID: actor, DependencyID: "55555555555555555555555555555555", CommitID: strings.Repeat("b", 40), Constraint: "<v2.0.0", State: "affected"}}, Strategy: "dual publish", Sequencing: "consumers first", CreatedBy: actor})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.UpdateEvolution(provider, plan.ID, actor, 2, "bad", "bad", ""); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale update = %v", err)
	}
	plan, err = store.AcknowledgeEvolution(provider, plan.ID, actor, consumer, "migration accepted")
	if err != nil {
		t.Fatal(err)
	}
	plan, analysis, err := store.StartEvolutionAnalysis(provider, plan.ID, actor, "66666666666666666666666666666666", "inspect call sites", []string{consumer})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = store.EvolutionAnalysis(provider, plan.ID, analysis.ID, "77777777777777777777777777777777"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("wrong credential = %v", err)
	}
	plan, err = store.AddEvolutionFinding(provider, plan.ID, analysis.AgentID, []string{consumer}, "two callers require ordering", "generated clients were not available")
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := store.GetEvolution(provider, plan.ID)
	if err != nil || len(reopened.Acknowledgements) != 1 || len(reopened.Findings) != 1 || reopened.Findings[0].Uncertainty == "" {
		t.Fatalf("reopened = %#v, %v", reopened, err)
	}
}

func TestStoreRetainsImmutableEvidence(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	created, err := store.CreateInterface(Interface{RepositoryID: "11111111111111111111111111111111", Name: " events ", Version: "v1.2.3", ReleaseID: "22222222222222222222222222222222", CommitID: "3333333333333333333333333333333333333333", PublishedBy: "44444444444444444444444444444444"})
	if err != nil {
		t.Fatal(err)
	}
	values, err := store.ListInterfaces(created.RepositoryID)
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.PublishedAt.IsZero() || len(values) != 1 || values[0].ID != created.ID || values[0].Name != "events" {
		t.Fatalf("created/listed = %#v / %#v", created, values)
	}
}
