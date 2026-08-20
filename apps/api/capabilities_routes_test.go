package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/capabilities"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

func TestRetirementProjectionUsesFrozenRevisionConsumerAccess(t *testing.T) {
	gitStore, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := repositories.New(t.TempDir(), gitStore)
	if err != nil {
		t.Fatal(err)
	}
	restrictedRepositoryOwner := strings.Repeat("a", 32)
	publicRepositoryOwner := strings.Repeat("b", 32)
	restricted, err := catalog.Create(restrictedRepositoryOwner, "secret-consumer")
	if err != nil {
		t.Fatal(err)
	}
	public, err := catalog.Create(publicRepositoryOwner, "public-consumer")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = catalog.SetVisibility(publicRepositoryOwner, public.ID, repositories.Public); err != nil {
		t.Fatal(err)
	}
	secretConsumer := capabilities.Consumer{Name: "SECRET_AUDIENCE", RepositoryID: restricted.ID, OwnerIDs: []string{"SECRET_OWNER"}, Environment: "SECRET_ENVIRONMENT", Discovery: "declared", EvidenceState: "unknown", CompatibilityPromise: "SECRET_COMMITMENT"}
	publicConsumer := capabilities.Consumer{Name: "public audience", RepositoryID: public.ID, OwnerIDs: []string{"public-owner"}, Environment: "public", Discovery: "declared", EvidenceState: "unknown", CompatibilityPromise: "public promise"}
	consumerIndex := 0
	values := []capabilities.Capability{{
		CurrentVersion: 2,
		Revisions: []capabilities.Revision{
			{Version: 1, Consumers: []capabilities.Consumer{secretConsumer, publicConsumer}},
			{Version: 2, Consumers: []capabilities.Consumer{publicConsumer, secretConsumer}},
		},
		RetirementPlans: []capabilities.RetirementPlan{{
			CapabilityVersion: 1,
			Audiences: []capabilities.Audience{
				{Name: "SECRET_AUDIENCE", OwnerIDs: []string{"SECRET_OWNER"}, Impact: "SECRET_IMPACT", Commitment: "SECRET_COMMITMENT"},
				{Name: "public audience", OwnerIDs: []string{"public-owner"}, Impact: "public impact"},
			},
			FrozenDiagnostics: []capabilities.Diagnostic{{Kind: "unknown_evidence", Consumer: "SECRET_AUDIENCE", ConsumerIndex: &consumerIndex}},
			RequiredOwnerIDs:  []string{"SECRET_OWNER", "public-owner"},
			Exceptions:        []capabilities.PlanException{{Audience: "SECRET_AUDIENCE"}},
			Blockers:          []capabilities.RetirementBlocker{{Kind: "conflicting_commitment", Audience: "SECRET_AUDIENCE", OwnerID: "SECRET_OWNER"}},
		}},
	}}

	projected := projectCapabilitiesForReader(catalog, "unrelated-reader", values)
	body, err := json.Marshal(projected)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"SECRET_AUDIENCE", "SECRET_OWNER", "SECRET_ENVIRONMENT", "SECRET_COMMITMENT", "SECRET_IMPACT"} {
		if strings.Contains(string(body), secret) {
			t.Fatalf("frozen revision detail %q leaked after consumer reorder: %s", secret, body)
		}
	}
	plan := projected[0].RetirementPlans[0]
	if plan.Audiences[0].Name != "restricted" || plan.Audiences[1].Name != "public audience" {
		t.Fatalf("plan audiences projected from wrong revision: %#v", plan.Audiences)
	}
}
