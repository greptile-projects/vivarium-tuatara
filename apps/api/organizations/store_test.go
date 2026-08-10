package organizations

import (
	"errors"
	"testing"
	"time"
)

func TestStewardshipMandateRequiresCurrentOperatorAcceptanceAfterEveryRevision(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	owner := "0123456789abcdef0123456789abcdef"
	operator := "abcdef0123456789abcdef0123456789"
	repository := "11111111111111111111111111111111"
	v, _ := store.Create("Runtime", "runtime", "", owner)
	v, _ = store.Invite(v.ID, owner, operator)
	v, _ = store.AcceptInvitation(v.ID, v.Invitations[0].ID, operator)
	v, err = store.RegisterAgent(v.ID, owner, "Caretaker", "caretaker", "", "organization", []string{"inspect"}, []string{operator}, nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Add(time.Minute)
	revision := MandateRevision{DesiredOutcomes: []string{"Keep checks green"}, Repositories: []MandateRepository{{RepositoryID: repository, Branches: []string{"main"}}}, TrustedSignals: []string{"required checks"}, Exclusions: []string{"No source writes"}, Budget: MandateBudget{MaxAgentMinutes: 120, MaxActions: 20}, StartsAt: now, ExpiresAt: now.Add(24 * time.Hour), AgentID: v.Agents[0].ID, AllowedActions: []string{"inspect_checks", "summarize"}, RequiredHumanDecisions: []string{"Any Git write or merge"}}
	v, mandate, err := store.CreateStewardshipMandate(v.ID, owner, "Keep runtime healthy", revision)
	if err != nil || mandate.Status != "pending_acceptance" {
		t.Fatalf("create = %#v, %v", mandate, err)
	}
	if _, _, err = store.AcceptStewardshipMandate(v.ID, mandate.ID, owner, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("non-operator acceptance = %v", err)
	}
	v, mandate, err = store.AcceptStewardshipMandate(v.ID, mandate.ID, operator, 1)
	if err != nil || mandate.Status != "active" || mandate.Acceptance.OperatorID != operator {
		t.Fatalf("accept = %#v, %v", mandate, err)
	}
	revision.DesiredOutcomes = []string{"Keep checks green", "Report regressions"}
	v, mandate, err = store.ReviseStewardshipMandate(v.ID, mandate.ID, owner, 1, revision)
	if err != nil || mandate.Version != 2 || mandate.Acceptance != nil || mandate.Status != "pending_acceptance" || len(mandate.Revisions) != 2 {
		t.Fatalf("revise = %#v, %v", mandate, err)
	}
	_, mandate, err = store.AcceptStewardshipMandate(v.ID, mandate.ID, operator, 2)
	if err != nil {
		t.Fatal(err)
	}
	_, mandate, err = store.ChangeStewardshipMandateState(v.ID, mandate.ID, owner, "pause", 2)
	if err != nil || mandate.Status != "paused" {
		t.Fatalf("pause = %#v, %v", mandate, err)
	}
	_, mandate, err = store.ChangeStewardshipMandateState(v.ID, mandate.ID, owner, "revoke", 2)
	if err != nil || mandate.Status != "revoked" || mandate.RevokedAt == nil {
		t.Fatalf("revoke = %#v, %v", mandate, err)
	}
}

func TestRemovingOperatorInvalidatesAcceptedStewardshipMandate(t *testing.T) {
	store, _ := New(t.TempDir())
	owner := "0123456789abcdef0123456789abcdef"
	operator := "abcdef0123456789abcdef0123456789"
	v, _ := store.Create("Runtime", "runtime", "", owner)
	v, _ = store.Invite(v.ID, owner, operator)
	v, _ = store.AcceptInvitation(v.ID, v.Invitations[0].ID, operator)
	v, _ = store.RegisterAgent(v.ID, owner, "Caretaker", "caretaker", "", "organization", []string{"inspect"}, []string{operator}, nil)
	start := time.Now().UTC().Add(time.Minute)
	revision := MandateRevision{DesiredOutcomes: []string{"Keep checks green"}, Repositories: []MandateRepository{{RepositoryID: "11111111111111111111111111111111", Branches: []string{"main"}}}, TrustedSignals: []string{"checks"}, Exclusions: []string{"writes"}, Budget: MandateBudget{MaxAgentMinutes: 60, MaxActions: 10}, StartsAt: start, ExpiresAt: start.Add(time.Hour), AgentID: v.Agents[0].ID, AllowedActions: []string{"inspect"}, RequiredHumanDecisions: []string{"merge"}}
	v, mandate, _ := store.CreateStewardshipMandate(v.ID, owner, "Runtime health", revision)
	v, mandate, _ = store.AcceptStewardshipMandate(v.ID, mandate.ID, operator, 1)
	v, err := store.RemoveMember(v.ID, owner, operator, func(Organization) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	mandate = v.StewardshipMandates[0]
	if mandate.Status != "pending_acceptance" || mandate.Acceptance != nil {
		t.Fatalf("removed operator retained acceptance: %#v", mandate)
	}
	if _, _, err = store.AcceptStewardshipMandate(v.ID, mandate.ID, owner, mandate.Version); !errors.Is(err, ErrNotFound) {
		t.Fatalf("accept with removed agent = %v", err)
	}
	if _, _, err = store.ChangeStewardshipMandateState(v.ID, mandate.ID, owner, "resume", mandate.Version); !errors.Is(err, ErrConflict) {
		t.Fatalf("resume after removal = %v", err)
	}
}

func TestStewardshipOpportunitiesDeduplicateRetainStaleEvidenceAndAcceptChallenges(t *testing.T) {
	store, _ := New(t.TempDir())
	base := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return base }
	owner := "0123456789abcdef0123456789abcdef"
	operator := "abcdef0123456789abcdef0123456789"
	repository := "11111111111111111111111111111111"
	v, _ := store.Create("Runtime", "runtime", "", owner)
	v, _ = store.Invite(v.ID, owner, operator)
	v, _ = store.AcceptInvitation(v.ID, v.Invitations[0].ID, operator)
	v, _ = store.RegisterAgent(v.ID, owner, "Caretaker", "caretaker", "", "organization", []string{"inspect"}, []string{operator}, nil)
	revision := MandateRevision{DesiredOutcomes: []string{"Keep checks green"}, Repositories: []MandateRepository{{RepositoryID: repository, Branches: []string{"main"}}}, TrustedSignals: []string{"required checks"}, Exclusions: []string{"writes"}, Budget: MandateBudget{MaxAgentMinutes: 60, MaxActions: 10}, StartsAt: base.Add(time.Minute), ExpiresAt: base.Add(time.Hour), AgentID: v.Agents[0].ID, AllowedActions: []string{"summarize"}, RequiredHumanDecisions: []string{"merge"}}
	v, mandate, _ := store.CreateStewardshipMandate(v.ID, owner, "Runtime health", revision)
	v, mandate, _ = store.AcceptStewardshipMandate(v.ID, mandate.ID, operator, 1)
	store.now = func() time.Time { return base.Add(2 * time.Minute) }
	finding := OpportunityFinding{RepositoryID: repository, Signal: "required checks", EvidenceType: "check", EvidenceID: "required-ci", EvidenceRevision: "run-1", Title: "Required checks are failing", Summary: "The default branch cannot ship.", Severity: "high", ExpectedValue: "Restore the release path.", Confidence: .9, AffectedOwnerIDs: []string{owner}, AffectedRevisions: []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, InScopeReason: "The mandate requires green checks.", Citations: []OpportunityCitation{{Kind: "check", ResourceID: "required-ci", Revision: "run-1", Label: "Failed run"}}}
	v, items, err := store.PublishStewardshipOpportunities(v.ID, mandate.ID, operator, []OpportunityFinding{finding})
	if err != nil || len(items) != 1 || len(v.StewardshipMandates[0].Opportunities) != 1 {
		t.Fatalf("publish = %#v, %v", items, err)
	}
	finding.EvidenceRevision = "run-2"
	finding.Citations = []OpportunityCitation{{Kind: "check", ResourceID: "required-ci", Revision: "run-2", Label: "Latest failed run"}}
	v, items, err = store.PublishStewardshipOpportunities(v.ID, mandate.ID, operator, []OpportunityFinding{finding})
	if err != nil || len(v.StewardshipMandates[0].Opportunities) != 1 || len(items[0].Citations) != 2 || !items[0].Citations[0].Stale || items[0].Citations[1].Stale {
		t.Fatalf("dedupe/stale = %#v, %v", items, err)
	}
	_, item, err := store.DecideStewardshipOpportunity(v.ID, mandate.ID, items[0].ID, owner, OpportunityDecision{ExpectedVersion: items[0].Version, Action: "incorrect", Reason: "The run belongs to an obsolete branch."})
	if err != nil || item.Status != "incorrect" || item.DecisionReason == "" {
		t.Fatalf("challenge = %#v, %v", item, err)
	}
	second := finding
	second.EvidenceID, second.EvidenceRevision, second.Title = "dependency-audit", "scan-1", "Dependency support is ending"
	second.Citations = []OpportunityCitation{{Kind: "dependency", ResourceID: "dependency-audit", Revision: "scan-1", Label: "Support policy"}}
	_, added, err := store.PublishStewardshipOpportunities(v.ID, mandate.ID, operator, []OpportunityFinding{second})
	if err != nil || added[0].Rank != 2 {
		t.Fatalf("second publish = %#v, %v", added, err)
	}
	_, moved, err := store.DecideStewardshipOpportunity(v.ID, mandate.ID, item.ID, owner, OpportunityDecision{ExpectedVersion: item.Version, Action: "rank", Rank: 2})
	if err != nil || moved.Rank != 2 {
		t.Fatalf("rank move = %#v, %v", moved, err)
	}
	persisted, err := store.Get(v.ID)
	if err != nil {
		t.Fatal(err)
	}
	queue := persisted.StewardshipMandates[0].Opportunities
	if len(queue) != 2 || queue[0].Rank != 2 || queue[1].Rank != 1 || queue[0].Rank == queue[1].Rank || queue[1].Version != added[0].Version+1 {
		t.Fatalf("rank move did not persist a unique, versioned order: %#v", queue)
	}
}

func TestRemoveMemberCleanupFailurePreventsMembershipCommit(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	owner := "0123456789abcdef0123456789abcdef"
	member := "abcdef0123456789abcdef0123456789"
	organization, err := store.Create("Runtime", "runtime", "", owner)
	if err != nil {
		t.Fatal(err)
	}
	organization, err = store.Invite(organization.ID, owner, member)
	if err != nil {
		t.Fatal(err)
	}
	organization, err = store.AcceptInvitation(organization.ID, organization.Invitations[0].ID, member)
	if err != nil {
		t.Fatal(err)
	}
	cleanupFailure := errors.New("credential store unavailable")
	if _, err = store.RemoveMember(organization.ID, owner, member, func(Organization) error { return cleanupFailure }); !errors.Is(err, cleanupFailure) {
		t.Fatalf("RemoveMember error = %v", err)
	}
	stored, err := store.Get(organization.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !HasRole(stored, member, "member") {
		t.Fatal("cleanup failure committed member removal")
	}
	for _, event := range stored.Events {
		if event.Action == "member.removed" {
			t.Fatal("cleanup failure published removal audit")
		}
	}
}

func TestPolicyPreviewActivationAndExpiringException(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	owner := "0123456789abcdef0123456789abcdef"
	maintainer := "abcdef0123456789abcdef0123456789"
	repository := "11111111111111111111111111111111"
	v, err := store.Create("Runtime", "runtime", "", owner)
	if err != nil {
		t.Fatal(err)
	}
	v, _ = store.Invite(v.ID, owner, maintainer)
	v, _ = store.AcceptInvitation(v.ID, v.Invitations[0].ID, maintainer)
	v, err = store.CreateTeam(v.ID, owner, "Runtime", "runtime-team", "", "", "organization")
	if err != nil {
		t.Fatal(err)
	}
	teamID := v.Teams[0].ID
	v, err = store.AddTeamMember(v.ID, teamID, owner, maintainer, "maintainer", 1)
	if err != nil {
		t.Fatal(err)
	}
	v, err = store.AddResponsibility(v.ID, teamID, owner, repository, "runtime", "", 2, func(write func() error) error { return write() })
	if err != nil {
		t.Fatal(err)
	}
	v, err = store.CreatePolicy(v.ID, owner, "Shared bar", "", []PolicyTarget{{Kind: "organization"}}, PolicyRules{MinimumReviews: 2, RequiredChecks: []string{"test"}, Integration: "queue", ReleaseProvenance: "attested", DependencyUse: "active-only", PromotionApprovals: 1, AgentAuthority: "explicit-grants"})
	if err != nil {
		t.Fatal(err)
	}
	policy := v.Policies[0]
	otherRepository := "22222222222222222222222222222222"
	v, err = store.CreatePolicy(v.ID, owner, "Other repository", "", []PolicyTarget{{Kind: "repository", ID: otherRepository}}, PolicyRules{Integration: "queue"})
	if err != nil {
		t.Fatal(err)
	}
	v, err = store.ActivatePolicy(v.ID, v.Policies[1].ID, owner, 1)
	if err != nil {
		t.Fatal(err)
	}
	preview := EffectivePolicies(v, repository, []string{teamID}, true, time.Now())
	if len(preview.Policies) != 1 || preview.Rules.MinimumReviews != 2 || preview.Rules.Integration != "queue" {
		t.Fatalf("unexpected preview: %#v", preview)
	}
	v, err = store.ActivatePolicy(v.ID, policy.ID, owner, 1)
	if err != nil {
		t.Fatal(err)
	}
	if v.Policies[0].Version != 2 || !v.Policies[0].AppliesToNewWork {
		t.Fatalf("activation did not retain version/new-work boundary: %#v", v.Policies[0])
	}
	expires := time.Now().Add(time.Hour)
	if _, err = store.RequestPolicyException(v.ID, maintainer, v.Policies[1].ID, repository, "minimum_reviews", "0", "unrelated", expires); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unrelated policy exception error = %v", err)
	}
	if _, err = store.RequestPolicyException(v.ID, maintainer, policy.ID, repository, "repository_visibility", "public", "missing rule", expires); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing-rule exception error = %v", err)
	}
	v, err = store.RequestPolicyException(v.ID, maintainer, policy.ID, repository, "minimum_reviews", "1", "legacy integration", expires)
	if err != nil {
		t.Fatal(err)
	}
	exceptionID := v.PolicyExceptions[0].ID
	v, err = store.DecidePolicyException(v.ID, exceptionID, owner, "approve")
	if err != nil {
		t.Fatal(err)
	}
	effective := EffectivePolicies(v, repository, []string{teamID}, false, time.Now())
	if effective.BaselineRules.MinimumReviews != 2 || effective.Rules.MinimumReviews != 1 || len(effective.Exceptions) != 1 || effective.Exceptions[0].Status != "approved" {
		t.Fatalf("exception silently weakened baseline or was not explained: %#v", effective)
	}
}

func TestPolicyExceptionCannotWeakenStricterSiblingPolicy(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	owner := "0123456789abcdef0123456789abcdef"
	repository := "11111111111111111111111111111111"
	v, err := store.Create("Runtime", "runtime", "", owner)
	if err != nil {
		t.Fatal(err)
	}
	v, err = store.CreatePolicy(v.ID, owner, "Local baseline", "", []PolicyTarget{{Kind: "organization"}}, PolicyRules{MinimumReviews: 1})
	if err != nil {
		t.Fatal(err)
	}
	localID := v.Policies[0].ID
	v, err = store.ActivatePolicy(v.ID, localID, owner, 1)
	if err != nil {
		t.Fatal(err)
	}
	v, err = store.CreatePolicy(v.ID, owner, "Security baseline", "", []PolicyTarget{{Kind: "repository", ID: repository}}, PolicyRules{MinimumReviews: 3})
	if err != nil {
		t.Fatal(err)
	}
	v, err = store.ActivatePolicy(v.ID, v.Policies[1].ID, owner, 1)
	if err != nil {
		t.Fatal(err)
	}
	v, err = store.RequestPolicyException(v.ID, owner, localID, repository, "minimum_reviews", "0", "temporary local constraint", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	v, err = store.DecidePolicyException(v.ID, v.PolicyExceptions[0].ID, owner, "approve")
	if err != nil {
		t.Fatal(err)
	}
	effective := EffectivePolicies(v, repository, nil, false, time.Now())
	if effective.BaselineRules.MinimumReviews != 3 || effective.Rules.MinimumReviews != 3 || len(effective.Exceptions) != 1 {
		t.Fatalf("sibling policy was weakened: %#v", effective)
	}
}

func TestInitiativeRetainsOrderedCrossRepositoryOwnershipAndCASUpdates(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	owner := "0123456789abcdef0123456789abcdef"
	member := "11111111111111111111111111111111"
	repositoryA := "22222222222222222222222222222222"
	repositoryB := "33333333333333333333333333333333"
	sourceID := "44444444444444444444444444444444"
	firstID := "55555555555555555555555555555555"
	secondID := "66666666666666666666666666666666"
	v, err := store.Create("Runtime", "runtime", "", owner)
	if err != nil {
		t.Fatal(err)
	}
	v, err = store.Invite(v.ID, owner, member)
	if err != nil {
		t.Fatal(err)
	}
	v, err = store.AcceptInvitation(v.ID, v.Invitations[0].ID, member)
	if err != nil {
		t.Fatal(err)
	}
	items := []InitiativeWorkItem{
		{ID: firstID, Title: "Publish provider", RepositoryID: repositoryA, Owner: InitiativeOwner{Type: "human", ID: owner}, Status: "in_progress"},
		{ID: secondID, Title: "Adopt consumer", RepositoryID: repositoryB, Owner: InitiativeOwner{Type: "human", ID: member}, DependencyIDs: []string{firstID}, Status: "todo", Contribution: &InitiativeSource{Kind: "proposal", RepositoryID: repositoryB, ID: sourceID}},
	}
	v, initiative, err := store.CreateInitiative(v.ID, owner, "Runtime v2", "Coordinate the rollout", InitiativeSource{Kind: "evolution", RepositoryID: repositoryA, ID: sourceID}, items, nil)
	if err != nil {
		t.Fatal(err)
	}
	if initiative.Version != 1 || len(initiative.WorkItems) != 2 || initiative.WorkItems[1].Position != 2 || initiative.WorkItems[1].DependencyIDs[0] != firstID {
		t.Fatalf("initiative graph was not retained: %#v", initiative)
	}
	v, err = store.UpdateInitiativeItem(v.ID, initiative.ID, firstID, member, InitiativeOwner{Type: "human", ID: member}, "completed", 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if v.Initiatives[0].Version != 2 || v.Initiatives[0].WorkItems[0].Status != "completed" || v.Events[len(v.Events)-1].Action != "initiative.item.updated" {
		t.Fatalf("initiative update was not attributable: %#v", v.Initiatives[0])
	}
	if _, err = store.UpdateInitiativeItem(v.ID, initiative.ID, firstID, owner, InitiativeOwner{Type: "human", ID: owner}, "todo", 1, nil); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale initiative update error = %v", err)
	}
}

func TestInitiativeRejectsDependencyCycles(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	owner := "0123456789abcdef0123456789abcdef"
	repository := "11111111111111111111111111111111"
	sourceID := "22222222222222222222222222222222"
	firstID := "33333333333333333333333333333333"
	secondID := "44444444444444444444444444444444"
	v, err := store.Create("Runtime", "runtime", "", owner)
	if err != nil {
		t.Fatal(err)
	}
	items := []InitiativeWorkItem{
		{ID: firstID, Title: "Provider", RepositoryID: repository, Owner: InitiativeOwner{Type: "human", ID: owner}, DependencyIDs: []string{secondID}, Status: "todo"},
		{ID: secondID, Title: "Consumer", RepositoryID: repository, Owner: InitiativeOwner{Type: "human", ID: owner}, DependencyIDs: []string{firstID}, Status: "todo"},
	}
	if _, _, err = store.CreateInitiative(v.ID, owner, "Cyclic rollout", "", InitiativeSource{Kind: "proposal", RepositoryID: repository, ID: sourceID}, items, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("cyclic initiative error = %v", err)
	}
	stored, err := store.Get(v.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.Initiatives) != 0 {
		t.Fatalf("cyclic initiative persisted: %#v", stored.Initiatives)
	}
}
