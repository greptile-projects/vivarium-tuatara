package main

import (
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/charters"
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

func TestGovernanceEligibilityRequiresActiveStanding(t *testing.T) {
	now := time.Now()
	roles := map[string][]string{"owner": {"maintainer"}, "active": {"maintainer"}, "suspended": {"maintainer"}}
	record := charters.Record{Standings: []charters.Standing{
		{PrincipalType: "human", PrincipalID: "active", Role: "maintainer", CharterVersion: 2, Status: "active", ExpiresAt: now.Add(time.Hour)},
		{PrincipalType: "human", PrincipalID: "suspended", Role: "maintainer", CharterVersion: 2, Status: "suspended", ExpiresAt: now.Add(time.Hour)},
	}}

	got := activeGovernanceStandingRoles(record, 2, roles, now)
	if len(got) != 1 || len(got["active"]) != 1 || got["active"][0] != "maintainer" {
		t.Fatalf("eligible roles = %#v", got)
	}
}
