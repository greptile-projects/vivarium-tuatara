package organizations

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func TestMatchAgentsExplainsEligibilityWithoutOpaqueRanking(t *testing.T) {
	now := time.Now().UTC()
	repo := strings.Repeat("a", 32)
	profile := AgentProfile{SupportedTasks: []string{"incident response"}, ExecutionProvenance: "platform container", Pricing: "$2/run", Availability: "now", PublishedAt: now, VerifiedEvidence: []AgentVerifiedEvidence{{Kind: "evaluation", Statement: "passed incident fixture", VerifiedAt: now}}}
	profile.DeploymentBoundaries = []string{"platform"}
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
	if _, err := MatchAgents(Organization{}, AgentMatchRequest{SourceKind: "task", SourceID: "x", Workflow: "review", DeploymentBoundary: "free-form platform"}, time.Now()); err == nil {
		t.Fatal("expected invalid deployment boundary")
	}
}

func TestMatchAgentsMakesDeploymentBoundaryConflictsIneligible(t *testing.T) {
	now := time.Now().UTC()
	v := Organization{Agents: []Agent{{
		ID:   strings.Repeat("d", 32),
		Name: "Internal agent",
		Profiles: []AgentProfile{{
			SupportedTasks:       []string{"incident response"},
			ExecutionProvenance:  "internal platform container",
			DeploymentBoundaries: []string{"operator_managed"},
			Pricing:              "$2/run",
			Availability:         "now",
			PublishedAt:          now,
		}},
	}}}
	got, err := MatchAgents(v, AgentMatchRequest{SourceKind: "incident", SourceID: "incident-8", Workflow: "incident response", DeploymentBoundary: "external_service"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Matches) != 1 || got.Matches[0].Eligible || len(got.Matches[0].Conflicts) != 1 {
		t.Fatalf("deployment conflict remained eligible: %#v", got.Matches)
	}
}

func TestMatchAgentsNeverInfersBoundaryFromProse(t *testing.T) {
	now := time.Now().UTC()
	v := Organization{Agents: []Agent{{
		ID: strings.Repeat("e", 32), Name: "Remote agent",
		Profiles: []AgentProfile{{SupportedTasks: []string{"incident response"}, ExecutionProvenance: "inference leaves the platform", DeploymentBoundaries: []string{"external_service"}, Pricing: "$2/run", Availability: "now", PublishedAt: now}},
	}}}
	got, err := MatchAgents(v, AgentMatchRequest{SourceKind: "incident", SourceID: "incident-9", Workflow: "incident response", DeploymentBoundary: "platform"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if got.Matches[0].Eligible || len(got.Matches[0].Conflicts) != 1 {
		t.Fatalf("prose produced a boundary match: %#v", got.Matches[0])
	}
}

func TestMatchAgentsTreatsUndisclosedBoundaryAsMissingEvidence(t *testing.T) {
	now := time.Now().UTC()
	v := Organization{Agents: []Agent{{
		ID: strings.Repeat("f", 32), Name: "Legacy agent",
		Profiles: []AgentProfile{{SupportedTasks: []string{"incident response"}, ExecutionProvenance: "operator container", Pricing: "$2/run", Availability: "now", PublishedAt: now}},
	}}}
	got, err := MatchAgents(v, AgentMatchRequest{SourceKind: "incident", SourceID: "incident-10", Workflow: "incident response", DeploymentBoundary: "platform"}, now)
	if err != nil {
		t.Fatal(err)
	}
	match := got.Matches[0]
	if match.Eligible || len(match.Conflicts) != 0 || !slices.Contains(match.MissingEvidence, "No structured deployment boundary is disclosed for the requested boundary.") {
		t.Fatalf("undisclosed boundary was not classified as missing evidence: %#v", match)
	}
}
