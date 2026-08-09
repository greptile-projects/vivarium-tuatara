package organizations

import (
	"errors"
	"testing"
	"time"
)

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
