package organizations

import (
	"strings"
	"testing"
	"time"
)

func TestMatchAgentsExplainsEligibilityWithoutOpaqueRanking(t *testing.T) {
	now := time.Now().UTC()
	repo := strings.Repeat("a", 32)
	profile := AgentProfile{SupportedTasks: []string{"incident response"}, ExecutionProvenance: "platform container", Pricing: "$2/run", Availability: "now", PublishedAt: now, VerifiedEvidence: []AgentVerifiedEvidence{{Kind: "evaluation", Statement: "passed incident fixture", VerifiedAt: now}}}
	v := Organization{Agents: []Agent{{ID: strings.Repeat("b", 32), Name: "Responder", Capabilities: []string{"triage"}, Profiles: []AgentProfile{profile}}, {ID: strings.Repeat("c", 32), Name: "Generic"}}, AccessGrants: []AccessGrant{{PrincipalType: "agent", PrincipalID: strings.Repeat("b", 32), Resources: []ResourceScope{{Kind: "repository", ID: repo}}}}}
	got, err := MatchAgents(v, AgentMatchRequest{SourceKind: "incident", SourceID: "incident-7", RepositoryID: repo, Workflow: "incident response", RequiredPermissions: []ResourceScope{{Kind: "repository", ID: repo}}, DeploymentBoundary: "platform"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Matches) != 2 || !got.Matches[0].Eligible || got.Matches[0].Name != "Responder" || got.Matches[0].Score == 0 || len(got.Matches[0].Reasons) == 0 || len(got.Matches[0].VerifiedEvaluations) != 1 {
		t.Fatalf("unexpected explained match: %#v", got)
	}
	if got.Matches[1].Eligible || len(got.Matches[1].MissingEvidence) == 0 {
		t.Fatalf("ineligible agent lacks evidence gaps: %#v", got.Matches[1])
	}
}

func TestMatchAgentsRejectsUnboundedSources(t *testing.T) {
	if _, err := MatchAgents(Organization{}, AgentMatchRequest{SourceKind: "secret", SourceID: "x", Workflow: "review"}, time.Now()); err == nil {
		t.Fatal("expected invalid source kind")
	}
}
