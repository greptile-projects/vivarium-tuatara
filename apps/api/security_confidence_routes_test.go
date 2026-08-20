package main

import (
	"strings"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/securityadvisories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/securityconfidence"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/securityscenarios"
)

func TestSecurityScenarioCurrentRequiresExactRerunWithoutPathMapping(t *testing.T) {
	oldRevision, newRevision := strings.Repeat("a", 40), strings.Repeat("b", 40)
	scenario := securityscenarios.Scenario{CommitID: oldRevision, DependencyIDs: []string{"identity"}, CheckPath: "security/login_test.go", Review: &securityscenarios.Review{Decision: "approved"}}
	q := securityconfidence.Requirement{Selector: securityconfidence.Selector{}}
	if securityScenarioCurrent(nil, "", q, scenario, newRevision, []string{"apps/api/auth/login.go"}) {
		t.Fatal("semantic dependency ID was treated as an unaffected path")
	}
	q.Selector.Paths = []string{"apps/api/auth"}
	if securityScenarioCurrent(nil, "", q, scenario, newRevision, []string{"apps/api/auth/login.go"}) {
		t.Fatal("affected explicit path retained stale scenario proof")
	}
	if !securityScenarioCurrent(nil, "", q, scenario, newRevision, []string{"docs/readme.md"}) {
		t.Fatal("unaffected explicit path invalidated scenario proof")
	}
}

func TestSecurityAdvisoryLinkRequiresOrdinaryVisibility(t *testing.T) {
	v := securityadvisories.Advisory{ReporterID: "reporter", ResponseTeam: []string{"responder"}}
	if !securityAdvisoryVisible(nil, "reporter", v) || !securityAdvisoryVisible(nil, "responder", v) {
		t.Fatal("advisory principals lost visibility")
	}
	if securityAdvisoryVisible(nil, "collaborator", v) {
		t.Fatal("uninvited collaborator gained embargoed advisory visibility")
	}
}
