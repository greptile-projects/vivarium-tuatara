package securityconfidence

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func requirement(kind string) Requirement {
	return Requirement{ID: kind, Title: "Protect authentication", Kind: kind, ThreatModelID: "model", ThreatModelVersion: 2, AbusePathID: "credential-replay", ScenarioID: map[bool]string{true: "scenario"}[kind == "security_scenario"], OwnerIDs: []string{"security-owner"}, Selector: Selector{Branches: []string{"main"}, Paths: []string{"auth"}}}
}

func TestMatrixRetainsExactSecurityGapsAndAffectedOnlyInvalidation(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	reqs := []Requirement{requirement("threat_coverage"), requirement("security_scenario"), requirement("control_acknowledgement"), requirement("resolved_findings")}
	p, err := s.Publish("repository", "repo", "repo", "owner", 0, reqs)
	if err != nil {
		t.Fatal(err)
	}
	evidence := map[string]Evidence{
		"threat_coverage":         {ThreatCurrent: true, ThreatRevision: 2, ResidualRisk: "bounded replay window"},
		"security_scenario":       {ScenarioAttemptID: "attempt", ScenarioResult: "passed"},
		"control_acknowledgement": {AcknowledgedOwnerIDs: []string{"security-owner"}},
		"resolved_findings":       {OpenFindingIDs: []string{"restricted"}},
	}
	m, err := s.Evaluate(p, "pull", "pull", strings.Repeat("a", 40), "main", []string{"auth/session.go"}, evidence)
	if err != nil || m.Ready || m.Requirements[3].State != "failed" || m.Requirements[0].Evidence.ResidualRisk == "" {
		t.Fatalf("matrix = %#v, %v", m, err)
	}
	// An unrelated docs-only change selects no auth requirement and therefore
	// does not invalidate or block the retained security proof.
	m, err = s.Evaluate(p, "pull", "pull-2", strings.Repeat("b", 40), "main", []string{"docs/readme.md"}, evidence)
	if err != nil || !m.Ready || len(m.Requirements) != 0 {
		t.Fatalf("unaffected matrix = %#v, %v", m, err)
	}
}

func TestExceptionIsExactExpiringAttributedAndFollowedUp(t *testing.T) {
	s, _ := New(t.TempDir())
	p, _ := s.Publish("repository", "repo", "repo", "owner", 0, []Requirement{requirement("security_scenario")})
	revision := strings.Repeat("a", 40)
	x, err := s.AddException("repo", "security-owner", Exception{RequirementID: "security_scenario", Revision: revision, Rationale: "Temporary isolated runner outage", Scope: Selector{Branches: []string{"main"}}, ExpiresAt: time.Now().Add(time.Hour), FollowUpKind: "issue", FollowUpID: "issue"})
	if err != nil {
		t.Fatal(err)
	}
	m, _ := s.Evaluate(p, "release", "release", revision, "main", []string{"auth/a.go"}, map[string]Evidence{})
	if !m.Ready || m.Requirements[0].State != "overridden" || m.Requirements[0].Exception.OwnerID != "security-owner" || x.FollowUpID != "issue" {
		t.Fatalf("matrix = %#v", m)
	}
	m, _ = s.Evaluate(p, "release", "release", strings.Repeat("b", 40), "main", []string{"auth/a.go"}, map[string]Evidence{})
	if m.Ready {
		t.Fatal("exception transferred to another revision")
	}
	_, err = s.AddException("repo", "security-owner", Exception{RequirementID: "security_scenario", Revision: revision, Rationale: "Too long", Scope: Selector{Branches: []string{"main"}}, ExpiresAt: time.Now().Add(31 * 24 * time.Hour), FollowUpKind: "issue", FollowUpID: "issue"})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("long exception = %v", err)
	}
	for _, scope := range []Selector{{Components: []string{"identity"}}, {Assets: []string{"credentials"}}, {RiskClasses: []string{"critical"}}} {
		_, err = s.AddException("repo", "security-owner", Exception{RequirementID: "security_scenario", Revision: revision, Rationale: "Cannot evaluate this dimension safely", Scope: scope, ExpiresAt: time.Now().Add(time.Hour), FollowUpKind: "issue", FollowUpID: "issue"})
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("unevaluable exception scope %#v = %v", scope, err)
		}
	}
}

func TestProductionSignalRequiresExactSanitizedResponseLink(t *testing.T) {
	s, _ := New(t.TempDir())
	in := Signal{ReleaseID: "release", DeploymentID: "deployment", Revision: strings.Repeat("a", 40), EnvironmentID: "production", RequirementID: "control", Kind: "control_failed", State: "confirmed", Summary: "Synthetic canary bypassed the expected control", ArtifactSHA256: strings.Repeat("b", 64), ControlIDs: []string{"rate-limit"}, ResponseKind: "private_incident", ResponseID: "incident"}
	out, err := s.RecordSignal("repo", "reporter", in)
	if err != nil || out.ReportedBy != "reporter" || out.ResponseID != "incident" {
		t.Fatalf("signal = %#v, %v", out, err)
	}
	in.ArtifactSHA256 = "raw production log"
	if _, err = s.RecordSignal("repo", "reporter", in); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unsafe signal = %v", err)
	}
}

func TestRequirementGovernanceDimensionsRequirePathMapping(t *testing.T) {
	for _, selector := range []Selector{{Components: []string{"identity"}}, {Assets: []string{"credentials"}}, {RiskClasses: []string{"critical"}}} {
		s, _ := New(t.TempDir())
		r := requirement("resolved_findings")
		r.Selector = selector
		if _, err := s.Publish("repository", "repo", "repo", "owner", 0, []Requirement{r}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("selector-only requirement %#v = %v", selector, err)
		}
	}
	s, _ := New(t.TempDir())
	r := requirement("resolved_findings")
	r.Selector = Selector{Components: []string{"identity"}, Assets: []string{"credentials"}, RiskClasses: []string{"critical"}, Paths: []string{"apps/api/auth"}}
	p, err := s.Publish("repository", "repo", "repo", "owner", 0, []Requirement{r})
	if err != nil {
		t.Fatal(err)
	}
	m, err := s.Evaluate(p, "pull", "pull", strings.Repeat("a", 40), "main", []string{"docs/readme.md"}, map[string]Evidence{})
	if err != nil || !m.Ready || len(m.Requirements) != 0 {
		t.Fatalf("unrelated mapped requirement = %#v, %v", m, err)
	}
}
