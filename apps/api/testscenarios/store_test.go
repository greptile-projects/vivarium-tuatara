package testscenarios

import (
	"errors"
	"strings"
	"testing"
)

func TestScenarioRetainsReadableParameterizedIntent(t *testing.T) {
	s, _ := New(t.TempDir())
	v := completeScenario()
	created, err := s.Create("repo", "author", v)
	if err != nil || created.CreatedBy != "author" || len(created.Cases) != 1 || created.Cases[0].Values["decline"] != "true" {
		t.Fatalf("created = %#v, %v", created, err)
	}
	listed, err := s.List("repo")
	if err != nil || len(listed) != 1 || listed[0].ID != created.ID {
		t.Fatalf("listed = %#v, %v", listed, err)
	}
}

func TestScenarioRejectsUnsafeOrOpaqueFixtureContracts(t *testing.T) {
	s, _ := New(t.TempDir())
	for name, mutate := range map[string]func(*Scenario){
		"production data":    func(v *Scenario) { v.Fixtures[0].DataClass = "production_personal_data" },
		"unknown provenance": func(v *Scenario) { v.Fixtures[0].SourceIDs = []string{"hidden"} },
		"opaque case":        func(v *Scenario) { v.Cases[0].Assumptions = nil },
		"unknown parameter":  func(v *Scenario) { v.Cases[0].Values["hidden"] = "value" },
	} {
		t.Run(name, func(t *testing.T) {
			v := completeScenario()
			mutate(&v)
			if _, err := s.Create("repo", "author", v); !errors.Is(err, ErrInvalid) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func completeScenario() Scenario {
	revision := strings.Repeat("a", 40)
	digest := strings.Repeat("b", 64)
	return Scenario{Title: "Declined checkout is recoverable", Purpose: "Prove a synthetic decline creates no order and can be retried", QualityPlanID: "plan", QualityPlanVersion: 1, RequirementIDs: []string{"pay"}, Sources: []Source{{Kind: "issue", ResourceID: "issue-1", Revision: revision, Summary: "Duplicate charge report"}}, Parameters: []Parameter{{Name: "decline", Description: "Whether the synthetic gateway declines", Type: "boolean", Required: true, Example: "true"}}, Preconditions: []Step{{ID: "account", Description: "Create a synthetic shopper", Operation: "seed shopper"}}, Actions: []Step{{ID: "submit", Description: "Submit checkout", Operation: "POST /checkout", Parameters: []string{"decline"}}}, Assertions: []Assertion{{ID: "no-order", Description: "No order is persisted", Matcher: "count_equals", Expected: "0"}}, Fixtures: []Fixture{{ID: "shopper", Kind: "generated", Description: "Synthetic shopper", Path: "fixtures/shopper.json", SHA256: digest, DataClass: "synthetic", Generator: "bun generate-fixture", Assumptions: []string{"No external identity"}, SourceIDs: []string{"issue-1"}}}, Environments: []Environment{{ID: "local", Description: "Local API", Runtime: "bun and Go", Requirements: []string{"loopback database"}}}, Cases: []Case{{ID: "decline", Name: "Gateway decline", Values: map[string]string{"decline": "true"}, Assumptions: []string{"Gateway stub is deterministic"}, ExpectedOutcome: "A retry prompt appears and no order exists"}}, Implementation: Implementation{AuthoredByType: "agent", Branch: "scenario/decline", CommitID: revision, TestPaths: []string{"tests/checkout.test.ts"}, Command: "bun test tests/checkout.test.ts", Framework: "bun:test", Generated: true, Assumptions: []string{"API runs on loopback"}, Provenance: []string{"issue-1", "plan@1/pay"}}}
}
