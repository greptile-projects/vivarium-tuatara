package main

import (
	"testing"
	"time"

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

func TestLatestInterfaceCheckSupersedesFailedJourneyAttempt(t *testing.T) {
	revision := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	failed := interfacechecks.Check{ID: "1", Revision: revision, Journey: "checkout", Status: "failed", CreatedAt: time.Unix(1, 0)}
	passed := interfacechecks.Check{ID: "2", Revision: revision, Journey: "checkout", Status: "passed", CreatedAt: time.Unix(2, 0)}
	other := interfacechecks.Check{ID: "3", Revision: revision, Journey: "settings", Status: "passed", CreatedAt: time.Unix(1, 0)}
	latest := latestInterfaceChecks([]interfacechecks.Check{failed, passed, other}, revision)
	if len(latest) != 2 {
		t.Fatalf("latest = %#v", latest)
	}
	for _, check := range latest {
		if check.Journey == "checkout" && check.ID != passed.ID {
			t.Fatalf("failed attempt remained authoritative: %#v", check)
		}
	}
}

func TestLatestInterfaceChecksPreserveIndependentDefinitionsForOneJourney(t *testing.T) {
	revision := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	failed := interfacechecks.Check{ID: "1", Revision: revision, Journey: "checkout", DefinitionPath: ".vivarium/a.json", DefinitionDigest: "a", Status: "failed", CreatedAt: time.Unix(1, 0)}
	passed := interfacechecks.Check{ID: "2", Revision: revision, Journey: "checkout", DefinitionPath: ".vivarium/b.json", DefinitionDigest: "b", Status: "passed", CreatedAt: time.Unix(2, 0)}
	latest := latestInterfaceChecks([]interfacechecks.Check{failed, passed}, revision)
	if len(latest) != 2 {
		t.Fatalf("independent definition was discarded: %#v", latest)
	}
	statuses := map[string]string{}
	for _, check := range latest {
		statuses[check.DefinitionPath] = check.Status
	}
	if statuses[failed.DefinitionPath] != "failed" || statuses[passed.DefinitionPath] != "passed" {
		t.Fatalf("independent results = %#v", statuses)
	}
}

func TestInterfaceEvidenceRequirementsComeFromAcceptedDesign(t *testing.T) {
	design := designproposals.Proposal{CurrentVersion: 1, Revisions: []designproposals.Revision{{Journeys: []designproposals.Journey{{Name: "checkout"}}, AcceptanceCriteria: []string{"keyboard flow"}}}, Implementation: &designproposals.Implementation{DesignVersion: 1}}
	check := interfacechecks.Check{DesignVersion: 1, Journey: "checkout", AffectedRequirements: []string{"keyboard flow"}, Differences: []interfacechecks.Difference{{Requirement: "unaccepted requirement"}}}
	if interfaceCheckMatchesDesign(design, check) {
		t.Fatal("difference cited an unaccepted design requirement")
	}
}
