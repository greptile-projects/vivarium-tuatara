package main

import (
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/dataflows"
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
