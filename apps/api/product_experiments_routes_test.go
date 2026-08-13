package main

import (
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
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

func TestExperimentDeploymentEvidenceContainsReviewedPull(t *testing.T) {
	promotion := deployments.Promotion{ReleaseID: "release-1", CommitID: "0123456789012345678901234567890123456789"}
	release := releases.Candidate{ID: "release-1", CommitID: promotion.CommitID, Inclusions: releases.Inclusion{PullRequestIDs: []string{"pull-1"}}}
	if !experimentDeploymentContainsPull(release, promotion, "pull-1") {
		t.Fatal("exact release provenance did not satisfy deployment evidence")
	}
	unrelated := release
	unrelated.Inclusions.PullRequestIDs = []string{"other-pull"}
	if experimentDeploymentContainsPull(unrelated, promotion, "pull-1") {
		t.Fatal("unrelated deployment satisfied reviewed pull evidence")
	}
	moved := promotion
	moved.CommitID = "abcdefabcdefabcdefabcdefabcdefabcdefabcd"
	if experimentDeploymentContainsPull(release, moved, "pull-1") {
		t.Fatal("release/commit mismatch satisfied deployment evidence")
	}
}
