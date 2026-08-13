package main

import (
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
)

func TestSuccessfulExperimentChecksRequireExactCommitTerminalSuccess(t *testing.T) {
	commit := "0123456789012345678901234567890123456789"
	runs := []checkruns.Run{
		{CommitID: commit, State: "failed", Definition: checkruns.Definition{Name: "experiment/assignment"}},
		{CommitID: commit, State: "queued", Definition: checkruns.Definition{Name: "experiment/capture"}},
		{CommitID: commit, State: "running", Definition: checkruns.Definition{Name: "experiment/isolation"}},
		{CommitID: commit, State: "canceled", Definition: checkruns.Definition{Name: "experiment/fallback"}},
		{CommitID: "abcdefabcdefabcdefabcdefabcdefabcdefabcd", State: "succeeded", Definition: checkruns.Definition{Name: "experiment/stale"}},
		{CommitID: commit, State: "succeeded", Definition: checkruns.Definition{Name: "experiment/verified"}},
	}
	available := successfulExperimentChecks(runs, commit)
	if len(available) != 1 || !available["experiment/verified"] {
		t.Fatalf("successful checks = %#v", available)
	}
}
