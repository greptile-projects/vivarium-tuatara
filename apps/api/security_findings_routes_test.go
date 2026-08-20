package main

import (
	"strings"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/securityfindings"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/securityscenarios"
)

func TestSecurityFindingAudienceAlwaysIncludesOwner(t *testing.T) {
	if securityFindingAudienceIncludesOwner([]string{"reporter"}, "owner") {
		t.Fatal("reporter-only audience admitted owner governance")
	}
	if !securityFindingAudienceIncludesOwner([]string{"reporter", "owner"}, "owner") {
		t.Fatal("owner audience rejected")
	}
}

func TestSecurityFindingProtectionRequiresExactThreatModelRevision(t *testing.T) {
	base := strings.Repeat("a", 40)
	repair := strings.Repeat("b", 40)
	finding := securityfindings.Finding{ThreatModelID: "model", ThreatModelVersion: 1, AbusePathID: "path", CandidateCommitID: base}
	failed := securityscenarios.Attempt{Result: "failed", Coverage: securityscenarios.Coverage{AbuseAttempted: true}}
	passed := securityscenarios.Attempt{Result: "passed", Coverage: securityscenarios.Coverage{AbuseAttempted: true, ContainmentIDs: []string{"contained"}}}
	baseScenario := securityscenarios.Scenario{ThreatModelID: "model", ThreatModelVersion: 1, AbusePathID: "path", CommitID: base, Attempts: []securityscenarios.Attempt{failed}}
	repairScenario := securityscenarios.Scenario{ThreatModelID: "model", ThreatModelVersion: 1, AbusePathID: "path", CommitID: repair, Review: &securityscenarios.Review{Decision: "approved"}, Attempts: []securityscenarios.Attempt{passed}}
	if !securityScenarioProvesFindingBase(baseScenario, finding) || !securityScenarioProtectsFinding(repairScenario, finding, repair) {
		t.Fatal("exact frozen base and repair evidence rejected")
	}
	baseScenario.ThreatModelVersion = 2
	repairScenario.ThreatModelVersion = 2
	if securityScenarioProvesFindingBase(baseScenario, finding) || securityScenarioProtectsFinding(repairScenario, finding, repair) {
		t.Fatal("different threat-model revision admitted as protection")
	}
}
