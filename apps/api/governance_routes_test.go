package main

import (
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/governance"
)

func TestSecretGovernanceProjectionExposesOnlyActorsOwnBallot(t *testing.T) {
	p := governance.Proposal{Rule: governance.Rule{SecretBallot: true}, Ballots: []governance.Ballot{
		{ID: "one", ActorID: "alice", Choice: "yes", Reason: "support", Receipt: "alice-receipt", CastAt: time.Now()},
		{ID: "two", ActorID: "bob", Choice: "no", Reason: "dissent", Receipt: "bob-receipt", CastAt: time.Now()},
	}, Events: []governance.Event{
		{ID: "open", Kind: "proposal.opened", ActorID: "alice"},
		{ID: "cast-a", Kind: "ballot.cast", ActorID: "alice"},
		{ID: "cast-b", Kind: "ballot.cast", ActorID: "bob"},
	}}

	got := governedProposalProjection(p, "alice")
	if len(got.Ballots) != 1 || got.Ballots[0].ActorID != "alice" || got.Ballots[0].Receipt != "alice-receipt" {
		t.Fatalf("ballots = %#v", got.Ballots)
	}
	if len(got.Events) != 2 || got.Events[1].ActorID != "alice" {
		t.Fatalf("events = %#v", got.Events)
	}
}
