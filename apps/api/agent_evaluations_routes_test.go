package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/agentevaluations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestOwnerLinkedAgentCannotDecideProtectedEvaluationEvidence(t *testing.T) {
	git, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), git)
	orgs, _ := organizations.New(t.TempDir())
	evaluations, _ := agentevaluations.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(git, identities, credentials, catalog, nil, nil, nil, nil, nil, orgs, evaluations))
	defer server.Close()

	owner := createTestAccount(t, server.URL, "evaluation-human-owner")
	org, err := orgs.Create("Evaluation", "evaluation-human-owner", "", owner.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := catalog.Create(owner.User.ID, "evaluation-human-owner")
	if err != nil {
		t.Fatal(err)
	}
	revision := agentevaluations.Revision{
		RepositoryRevision: strings.Repeat("a", 40),
		Scenarios:          []agentevaluations.Scenario{{ID: "safe", Title: "Safe", SanitizedPrompt: "complete fixture", ExpectedOutcomes: []string{"done"}, Checks: []agentevaluations.Check{{Name: "public", Kind: "contains", Expected: "done"}}, HiddenChecks: []agentevaluations.Check{{Name: "protected", Kind: "contains", Expected: "done"}}}},
		Budget:             agentevaluations.Budget{MaxCost: 1, MaxLatencyMS: 1000, MaxToolActions: 3}, ProhibitedActions: []string{"publish"}, HumanReviewCriteria: []string{"inspect"}, ChangeSummary: "initial", CreatedBy: owner.User.ID,
	}
	suite, err := evaluations.Create(agentevaluations.Suite{OrganizationID: org.ID, RepositoryID: repo.ID, Name: "Protected"}, revision)
	if err != nil {
		t.Fatal(err)
	}
	run, err := evaluations.CreateRun(suite.ID, 1, agentevaluations.RunInput{AgentID: strings.Repeat("b", 32), AgentProfileVersion: 1, Outputs: map[string]string{"safe": "done"}}, owner.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	agent, err := credentials.IssueOrganizationAgent(owner.User.ID, "owner-linked evaluator", org.ID, strings.Repeat("c", 32), strings.Repeat("b", 32), repo.ID, []string{"repositories:read", "repositories:write"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+org.ID+"/agent-evaluation-runs/"+run.ID+"/decisions", `{"decision":"approved","rationale":"protected evidence passed"}`, agent.Token, http.StatusForbidden).Body.Close()
	unchanged, err := evaluations.GetEvaluatorRun(run.ID)
	if err != nil || len(unchanged.Decisions) != 0 {
		t.Fatalf("agent persisted protected decision: %#v, %v", unchanged.Decisions, err)
	}
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+org.ID+"/agent-evaluation-runs/"+run.ID+"/decisions", `{"decision":"approved","rationale":"human evaluator inspected protected evidence"}`, owner.Credential.Token, http.StatusCreated)
	response.Body.Close()
	approved, err := evaluations.GetEvaluatorRun(run.ID)
	if err != nil || len(approved.Decisions) != 1 || approved.Decisions[0].EvaluatorID != owner.User.ID {
		t.Fatalf("human decision = %#v, %v", approved.Decisions, err)
	}
}
