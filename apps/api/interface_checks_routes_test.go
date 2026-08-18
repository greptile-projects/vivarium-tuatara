package main

import (
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/designproposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/interfacechecks"
)

func TestInterfaceEvidenceRequiresImplementationForExactDesignRevision(t *testing.T) {
	design := designproposals.Proposal{CurrentVersion: 2, Revisions: []designproposals.Revision{{Journeys: []designproposals.Journey{{Name: "checkout"}}, AcceptanceCriteria: []string{"version one"}}, {Journeys: []designproposals.Journey{{Name: "checkout"}}, AcceptanceCriteria: []string{"version two"}}}, Implementation: &designproposals.Implementation{DesignVersion: 1}}
	check := interfacechecks.Check{DesignVersion: 2, Journey: "checkout", AffectedRequirements: []string{"version two"}}
	if interfaceCheckMatchesDesign(design, check) {
		t.Fatal("new design revision reused an older implementation")
	}
	design.Implementation.DesignVersion = 2
	if !interfaceCheckMatchesDesign(design, check) {
		t.Fatal("exact implemented design revision was rejected")
	}
}

func TestInterfaceEvidenceRequirementsComeFromAcceptedDesign(t *testing.T) {
	design := designproposals.Proposal{CurrentVersion: 1, Revisions: []designproposals.Revision{{Journeys: []designproposals.Journey{{Name: "checkout"}}, AcceptanceCriteria: []string{"keyboard flow"}}}, Implementation: &designproposals.Implementation{DesignVersion: 1}}
	check := interfacechecks.Check{DesignVersion: 1, Journey: "checkout", AffectedRequirements: []string{"keyboard flow"}, Differences: []interfacechecks.Difference{{Requirement: "unaccepted requirement"}}}
	if interfaceCheckMatchesDesign(design, check) {
		t.Fatal("difference cited an unaccepted design requirement")
	}
}
