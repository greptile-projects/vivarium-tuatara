package main

import (
	"strings"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

func TestContributionCompletionRequiresExactGuidedDeliveryAndCredit(t *testing.T) {
	merge := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	pull := pullrequests.PullRequest{ID: "11111111111111111111111111111111", AuthorID: "22222222222222222222222222222222", Status: pullrequests.Merged, MergeCommitID: &merge, ContributionEvidence: &pullrequests.ContributionEvidence{OpportunityID: "33333333333333333333333333333333", OpportunityVersion: 4}}
	release := releases.Candidate{CommitID: merge, Inclusions: releases.Inclusion{PullRequestIDs: []string{pull.ID}, ContributorIDs: []string{pull.AuthorID}}}
	if !validContributionDelivery(pull.ContributionEvidence.OpportunityID, 4, pull, release, true) {
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
			if validContributionDelivery(pull.ContributionEvidence.OpportunityID, 4, candidatePull, candidateRelease, true) {
				t.Fatal("invalid completion accepted")
			}
		})
	}
	if validContributionDelivery(pull.ContributionEvidence.OpportunityID, 4, pull, release, false) {
		t.Fatal("release outside merge ancestry accepted")
	}
}

func TestReleaseContainsGuidedMergeByAncestry(t *testing.T) {
	gitStore, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repositoryID := strings.Repeat("a", 32)
	repository, err := gitStore.Create(repositoryID)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := repository.WriteObject(storage.TreeObject, nil)
	if err != nil {
		t.Fatal(err)
	}
	base := writeTestCommit(t, repository, tree, nil, 1700000000, "base")
	merge := writeTestCommit(t, repository, tree, []storage.ObjectID{base}, 1700000001, "guided merge")
	release := writeTestCommit(t, repository, tree, []storage.ObjectID{merge}, 1700000002, "bundled release")
	unrelated := writeTestCommit(t, repository, tree, nil, 1700000003, "unrelated")
	if !releaseContainsCommit(gitStore, repositoryID, string(release), string(merge)) {
		t.Fatal("descendant release did not retain guided merge")
	}
	if releaseContainsCommit(gitStore, repositoryID, string(unrelated), string(merge)) {
		t.Fatal("unrelated release accepted guided merge")
	}
}
