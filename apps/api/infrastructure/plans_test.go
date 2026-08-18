package infrastructure

import (
	"testing"
)

func TestChangePlanDerivesOrderRisksAndInvalidatesAcknowledgements(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	base := Revision{Title: "Runtime", Summary: "Current", Revision: "1111111111111111111111111111111111111111", OwnerIDs: []string{"owner"}, Rationale: "reviewed", Resources: []Resource{
		{ID: "db", Kind: "data_store", Name: "Database", OwnerIDs: []string{"data-owner"}, Provider: "cloud", ProviderRef: "db/1", ProviderAccess: "participant", DependsOn: []string{}, Constraints: []Constraint{{Kind: "cost", Limit: 50, Unit: "USD/month", Note: "Current ceiling"}}, Commitments: Commitments{Security: []string{"encrypted"}, Privacy: []string{"regional records"}, Continuity: []string{"daily recovery"}}},
		{ID: "api", Kind: "service", Name: "API", OwnerIDs: []string{"service-owner"}, Provider: "cloud", ProviderRef: "service/1", ProviderAccess: "participant", DependsOn: []string{"db"}, Commitments: Commitments{Reliability: []string{"99.9%"}}},
	}}
	definition, err := s.Create("repo", "owner", base)
	if err != nil {
		t.Fatal(err)
	}
	candidate := base
	candidate.Revision = "2222222222222222222222222222222222222222"
	candidate.Resources = []Resource{
		{ID: "db", Kind: "data_store", Name: "Database", OwnerIDs: []string{"data-owner"}, Provider: "other-cloud", ProviderRef: "db/2", ProviderAccess: "participant", DependsOn: []string{}, Constraints: base.Resources[0].Constraints, Commitments: base.Resources[0].Commitments},
		{ID: "api", Kind: "service", Name: "API", OwnerIDs: []string{"service-owner"}, Provider: "cloud", ProviderRef: "service/1", ProviderAccess: "participant", DependsOn: []string{"db"}, Constraints: []Constraint{{Kind: "capacity", Limit: 1000, Unit: "requests/second"}}, Commitments: base.Resources[1].Commitments},
	}
	plan, err := s.CreatePlan("repo", "owner", PlanCreation{PullRequestID: "pull", Revision: candidate.Revision, Definition: definition, Candidate: candidate, Policies: []PolicyEffect{{Path: "infra/policy.json", Digest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Effects: []string{"encryption rule remains blocking"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Changes) != 2 || plan.Changes[0].ResourceID != "db" || plan.Changes[0].Action != "replace" || plan.Changes[1].DependencyIDs[0] != "db" {
		t.Fatalf("unexpected derived plan: %#v", plan.Changes)
	}
	foundData := false
	for _, risk := range plan.Changes[0].Risks {
		if risk.Kind == "data" && risk.Severity == "high" {
			foundData = true
		}
	}
	if !foundData {
		t.Fatal("replacement omitted persistent-data risk")
	}
	plan, err = s.AddPlanEvent(plan.ID, "agent-1", "agent", 0, PlanEvent{Kind: "impact", Body: "The database replacement affects write availability.", ResourceIDs: []string{"db"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AddPlanEvent(plan.ID, "agent-1", "agent", 1, PlanEvent{Kind: "owner_acknowledgement", Body: "ack", OwnerID: "data-owner"}); err == nil {
		t.Fatal("agent impersonated affected owner")
	}
	plan, err = s.AddPlanEvent(plan.ID, "data-owner", "human", 1, PlanEvent{Kind: "owner_acknowledgement", Body: "Recovery evidence is required before replacement.", OwnerID: "data-owner", ResourceIDs: []string{"db"}})
	if err != nil {
		t.Fatal(err)
	}
	current := s.ProjectPlan(plan, definition, candidate.Revision, func(PolicyEffect) bool { return true })
	if !current.Fresh || len(current.AcknowledgedOwnerIDs) != 1 {
		t.Fatalf("current acknowledgement missing: %#v", current)
	}
	definition.Observations = append(definition.Observations, Observation{ID: "new-provider-revision"})
	stale := s.ProjectPlan(plan, definition, candidate.Revision, func(PolicyEffect) bool { return true })
	if stale.Fresh || len(stale.AcknowledgedOwnerIDs) != 0 || !contains(stale.StaleReasons, "observed_state_changed") {
		t.Fatalf("stale plan reused approval: %#v", stale)
	}
	policyStale := s.ProjectPlan(plan, definition, candidate.Revision, func(PolicyEffect) bool { return false })
	if !contains(policyStale.StaleReasons, "policy_changed") {
		t.Fatalf("policy drift not projected: %#v", policyStale.StaleReasons)
	}
}
