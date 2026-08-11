package main

import (
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
)

func TestContributionCompletionRequiresExactGuidedDeliveryAndCredit(t *testing.T) {
	merge := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	pull := pullrequests.PullRequest{ID: "11111111111111111111111111111111", AuthorID: "22222222222222222222222222222222", Status: pullrequests.Merged, MergeCommitID: &merge, ContributionEvidence: &pullrequests.ContributionEvidence{OpportunityID: "33333333333333333333333333333333", OpportunityVersion: 4}}
	release := releases.Candidate{CommitID: merge, Inclusions: releases.Inclusion{PullRequestIDs: []string{pull.ID}, ContributorIDs: []string{pull.AuthorID}}}
	if !validContributionDelivery(pull.ContributionEvidence.OpportunityID, 4, pull, release) {
		t.Fatal("exact delivery rejected")
	}
	for name, mutate := range map[string]func(*pullrequests.PullRequest, *releases.Candidate){
		"open pull":                  func(p *pullrequests.PullRequest, _ *releases.Candidate) { p.Status = pullrequests.Open },
		"wrong opportunity version":  func(p *pullrequests.PullRequest, _ *releases.Candidate) { p.ContributionEvidence.OpportunityVersion++ },
		"missing pull inclusion":     func(_ *pullrequests.PullRequest, r *releases.Candidate) { r.Inclusions.PullRequestIDs = nil },
		"missing contributor credit": func(_ *pullrequests.PullRequest, r *releases.Candidate) { r.Inclusions.ContributorIDs = nil },
	} {
		t.Run(name, func(t *testing.T) {
			candidatePull, candidateRelease := pull, release
			evidence := *pull.ContributionEvidence
			candidatePull.ContributionEvidence = &evidence
			mutate(&candidatePull, &candidateRelease)
			if validContributionDelivery(pull.ContributionEvidence.OpportunityID, 4, candidatePull, candidateRelease) {
				t.Fatal("invalid completion accepted")
			}
		})
	}
}
