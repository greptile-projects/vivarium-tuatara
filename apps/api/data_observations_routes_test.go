package main

import (
	"errors"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/dataflows"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/extensions"
)

func TestDataFlowRevisionReferencesExactObservationUse(t *testing.T) {
	revision := dataflows.Revision{CommitmentRefs: []dataflows.CommitmentRef{{CommitmentID: "commitment-a", Version: 2, DataUseIDs: []string{"account", "profile"}}}}
	for _, test := range []struct {
		name, commitment string
		version          int
		use              string
		want             bool
	}{
		{"exact reference", "commitment-a", 2, "account", true},
		{"unreferenced commitment", "commitment-b", 2, "account", false},
		{"unreferenced version", "commitment-a", 1, "account", false},
		{"unreferenced use", "commitment-a", 2, "payments", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := dataFlowRevisionReferencesUse(revision, test.commitment, test.version, test.use); got != test.want {
				t.Fatalf("reference match = %v, want %v", got, test.want)
			}
		})
	}
}

func TestDataObservationExtensionScopeRequiresLiveInstallationAccess(t *testing.T) {
	repo := "repository"
	for _, test := range []struct {
		name         string
		installation extensions.Installation
		err          error
		want         bool
	}{
		{"active repository installation", extensions.Installation{Status: "active", RepositoryIDs: []string{repo}}, nil, true},
		{"removed installation", extensions.Installation{Status: "removed", RepositoryIDs: []string{repo}}, nil, false},
		{"suspended installation", extensions.Installation{Status: "suspended", RepositoryIDs: []string{repo}}, nil, false},
		{"different repository", extensions.Installation{Status: "active", RepositoryIDs: []string{"other"}}, nil, false},
		{"unreadable installation", extensions.Installation{}, errors.New("unavailable"), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := dataObservationInstallationActive(repo, test.installation, test.err); got != test.want {
				t.Fatalf("active = %v, want %v", got, test.want)
			}
		})
	}
}
