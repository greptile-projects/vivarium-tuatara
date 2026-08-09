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

func TestContractCandidatesSupersedeOnlyChangedCombinations(t *testing.T) {
	store, _ := New(t.TempDir())
	provider, consumer, actor := "11111111111111111111111111111111", "22222222222222222222222222222222", "33333333333333333333333333333333"
	plan, err := store.CreateEvolution(Evolution{RepositoryID: provider, InterfaceName: "events", Predecessor: Interface{ID: "44444444444444444444444444444444", RepositoryID: provider, Name: "events", Version: "v1.0.0", ReleaseID: "55555555555555555555555555555555", CommitID: strings.Repeat("a", 40), PublishedBy: actor}, SourceKind: "pull_request", SourceID: "66666666666666666666666666666666", CandidateCommitID: strings.Repeat("b", 40), CandidateDescription: "events v2", Changes: []CompatibilityChange{{Kind: "field", Summary: "rename", Classification: "breaking"}}, Strategy: "test together", Sequencing: "consumer first", CreatedBy: actor})
	if err != nil {
		t.Fatal(err)
	}
	revisions := []ContractCandidateRevision{{Role: "provider", RepositoryID: provider, PullRequestID: "77777777777777777777777777777777", SourceRepositoryID: provider, CommitID: strings.Repeat("b", 40)}, {Role: "consumer", RepositoryID: consumer, PullRequestID: "88888888888888888888888888888888", SourceRepositoryID: consumer, CommitID: strings.Repeat("c", 40)}}
	plan, first, err := store.AddContractCandidate(provider, plan.ID, actor, strings.Repeat("d", 40), strings.Repeat("1", 64), revisions, []string{"99999999999999999999999999999999"})
	if err != nil {
		t.Fatal(err)
	}
	revisions[1].CommitID = strings.Repeat("e", 40)
	plan, second, err := store.AddContractCandidate(provider, plan.ID, actor, strings.Repeat("f", 40), strings.Repeat("2", 64), revisions, []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.ContractCandidates[0].SupersededBy != second.ID || plan.ContractCandidates[0].SupersededAt == nil || plan.ContractCandidates[1].SupersededAt != nil || first.ID == second.ID {
		t.Fatalf("candidates = %#v", plan.ContractCandidates)
	}
}

func TestEvolutionRolloutFreezesPhasesAndRequiresRepositoryApprovals(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	provider, consumer, actor := strings.Repeat("1", 32), strings.Repeat("2", 32), strings.Repeat("3", 32)
	plan, err := store.CreateEvolution(Evolution{RepositoryID: provider, InterfaceName: "api", Predecessor: Interface{ID: strings.Repeat("4", 32)}, SourceKind: "proposal", SourceID: strings.Repeat("5", 32), CandidateDescription: "breaking candidate", Strategy: "expand then contract", Sequencing: "consumer before provider", Changes: []CompatibilityChange{{Kind: "schema", Summary: "remove field", Classification: "breaking"}}, Impacts: []ConsumerImpact{{RepositoryID: consumer}}, CreatedBy: actor})
	if err != nil {
		t.Fatal(err)
	}
	revisions := []ContractCandidateRevision{{Role: "provider", RepositoryID: provider, PullRequestID: strings.Repeat("6", 32), SourceRepositoryID: provider, CommitID: strings.Repeat("a", 40)}, {Role: "consumer", RepositoryID: consumer, PullRequestID: strings.Repeat("7", 32), SourceRepositoryID: consumer, CommitID: strings.Repeat("b", 40)}}
	plan, candidate, err := store.AddContractCandidate(provider, plan.ID, actor, strings.Repeat("c", 40), strings.Repeat("d", 64), revisions, []string{strings.Repeat("8", 32)})
	if err != nil {
		t.Fatal(err)
	}
	plan, err = store.ConfigureEvolutionRollout(provider, plan.ID, actor, candidate.ID, []EvolutionRolloutPhase{{Name: "consumers", RepositoryIDs: []string{consumer}}, {Name: "provider", RepositoryIDs: []string{provider}}}, plan.Version)
	if err != nil || plan.Rollout == nil || len(plan.Rollout.Phases) != 2 {
		t.Fatalf("rollout = %#v, %v", plan.Rollout, err)
	}
	if _, err = store.ConfigureEvolutionRollout(provider, plan.ID, actor, candidate.ID, []EvolutionRolloutPhase{{Name: "duplicate", RepositoryIDs: []string{consumer, consumer}}}, plan.Version); !errors.Is(err, ErrInvalid) {
		t.Fatalf("duplicate repository = %v", err)
	}
	plan, err = store.ApproveEvolutionRollout(provider, plan.ID, consumer, consumer, plan.Version)
	if err != nil || len(plan.Rollout.Approvals) != 1 || plan.Rollout.Approvals[0].RepositoryID != consumer {
		t.Fatalf("approval = %#v, %v", plan.Rollout, err)
	}
	if _, err = store.ApproveEvolutionRollout(provider, plan.ID, consumer, consumer, plan.Version); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate approval = %v", err)
	}
}
