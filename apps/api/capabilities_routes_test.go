package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/capabilities"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

func TestRetirementProjectionUsesFrozenRevisionConsumerAccess(t *testing.T) {
	gitStore, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := repositories.New(t.TempDir(), gitStore)
	if err != nil {
		t.Fatal(err)
	}
	restrictedRepositoryOwner := strings.Repeat("a", 32)
	publicRepositoryOwner := strings.Repeat("b", 32)
	restricted, err := catalog.Create(restrictedRepositoryOwner, "secret-consumer")
	if err != nil {
		t.Fatal(err)
	}
	public, err := catalog.Create(publicRepositoryOwner, "public-consumer")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = catalog.SetVisibility(publicRepositoryOwner, public.ID, repositories.Public); err != nil {
		t.Fatal(err)
	}
	secretConsumer := capabilities.Consumer{Name: "SECRET_AUDIENCE", RepositoryID: restricted.ID, OwnerIDs: []string{"SECRET_OWNER"}, Environment: "SECRET_ENVIRONMENT", Discovery: "declared", EvidenceState: "unknown", CompatibilityPromise: "SECRET_COMMITMENT"}
	publicConsumer := capabilities.Consumer{Name: "public audience", RepositoryID: public.ID, OwnerIDs: []string{"public-owner"}, Environment: "public", Discovery: "declared", EvidenceState: "unknown", CompatibilityPromise: "public promise"}
	frozenConsumerIndex := 0
	currentConsumerIndex := 1
	values := []capabilities.Capability{{
		CurrentVersion: 2,
		Revisions: []capabilities.Revision{
			{Version: 1, Consumers: []capabilities.Consumer{secretConsumer, publicConsumer}},
			{Version: 2, Consumers: []capabilities.Consumer{publicConsumer, secretConsumer}},
		},
		RetirementPlans: []capabilities.RetirementPlan{{
			CapabilityVersion: 1,
			Audiences: []capabilities.Audience{
				{Name: "SECRET_AUDIENCE", OwnerIDs: []string{"SECRET_OWNER"}, Impact: "SECRET_IMPACT", Commitment: "SECRET_COMMITMENT"},
				{Name: "public audience", OwnerIDs: []string{"public-owner"}, Impact: "public impact"},
			},
			FrozenDiagnostics: []capabilities.Diagnostic{{Kind: "unknown_evidence", Consumer: "SECRET_AUDIENCE", ConsumerIndex: &frozenConsumerIndex}},
			RequiredOwnerIDs:  []string{"SECRET_OWNER", "public-owner"},
			Events: []capabilities.RetirementEvent{{
				Version: 1, Type: "approval", ActorID: "SECRET_OWNER", ActorType: "human", OwnerID: "SECRET_OWNER", Decision: "approved", Summary: "SECRET_EVENT_SUMMARY", Evidence: []string{"SECRET_EVENT_EVIDENCE"},
			}},
			Exceptions: []capabilities.PlanException{{Audience: "SECRET_AUDIENCE"}},
			Blockers: []capabilities.RetirementBlocker{
				{Kind: "conflicting_commitment", Audience: "SECRET_AUDIENCE", OwnerID: "SECRET_OWNER"},
				{Kind: "inventory_unknown_evidence", Audience: "LATEST_RESTRICTED_NAME", ConsumerIndex: &currentConsumerIndex},
			},
		}},
	}}

	projected := projectCapabilitiesForReader(catalog, "unrelated-reader", values)
	body, err := json.Marshal(projected)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"SECRET_AUDIENCE", "SECRET_OWNER", "SECRET_ENVIRONMENT", "SECRET_COMMITMENT", "SECRET_IMPACT", "LATEST_RESTRICTED_NAME", "SECRET_EVENT_SUMMARY", "SECRET_EVENT_EVIDENCE"} {
		if strings.Contains(string(body), secret) {
			t.Fatalf("frozen revision detail %q leaked after consumer reorder: %s", secret, body)
		}
	}
	plan := projected[0].RetirementPlans[0]
	if plan.Audiences[0].Name != "restricted" || plan.Audiences[1].Name != "public audience" {
		t.Fatalf("plan audiences projected from wrong revision: %#v", plan.Audiences)
	}
	if plan.Events[0].ActorID != "restricted" || plan.Events[0].OwnerID != "restricted" || plan.Events[0].Summary != "restricted owner response" || len(plan.Events[0].Evidence) != 0 {
		t.Fatalf("restricted owner event was not projected safely: %#v", plan.Events[0])
	}
}

func TestRetirementReadinessUsesHiddenMergedDependenciesWithoutDisclosingThem(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	hiddenOwner, publicOwner, reader := strings.Repeat("a", 32), strings.Repeat("b", 32), strings.Repeat("c", 32)
	hidden, _ := catalog.Create(hiddenOwner, "hidden-consumer")
	visible, _ := catalog.Create(publicOwner, "visible-consumer")
	_, _ = catalog.SetVisibility(publicOwner, visible.ID, repositories.Public)
	proposalStore, _ := proposals.New(t.TempDir())
	makeTask := func(repo, owner, title string) (proposals.Proposal, proposals.Task) {
		proposal, err := proposalStore.Create(repo, owner, title, "retirement work")
		if err != nil {
			t.Fatal(err)
		}
		task, err := proposalStore.CreateTask(repo, proposal.ID, owner, title, "complete", nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		return proposal, task
	}
	hiddenProposal, hiddenTask := makeTask(hidden.ID, hiddenOwner, "hidden predecessor")
	pullID := strings.Repeat("d", 32)
	linked, err := proposalStore.LinkTaskContribution(hidden.ID, hiddenProposal.ID, hiddenTask.ID, hiddenOwner, proposals.TaskContribution{PullRequestID: pullID, SourceCommitID: strings.Repeat("e", 40), CommitIDs: []string{strings.Repeat("e", 40)}, Status: "review"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = proposalStore.UpdateTaskContribution(hidden.ID, hiddenProposal.ID, hiddenTask.ID, hiddenOwner, pullID, "merged"); err != nil {
		t.Fatal(err)
	}
	visibleProposal, visibleTask := makeTask(visible.ID, publicOwner, "visible dependent")
	values := []capabilities.Capability{{CurrentVersion: 1, Revisions: []capabilities.Revision{{Version: 1, Consumers: []capabilities.Consumer{{Name: "hidden", RepositoryID: hidden.ID}, {Name: "visible", RepositoryID: visible.ID}}}}, RetirementPlans: []capabilities.RetirementPlan{{CapabilityVersion: 1, Audiences: []capabilities.Audience{{Name: "hidden"}, {Name: "visible"}}, Work: []capabilities.RetirementWork{{ID: "hidden-work", AudienceIndex: 0, RepositoryID: hidden.ID, ProposalID: hiddenProposal.ID, TaskID: linked.ID}, {ID: "visible-work", AudienceIndex: 1, RepositoryID: visible.ID, ProposalID: visibleProposal.ID, TaskID: visibleTask.ID, DependencyIDs: []string{"hidden-work"}}}}}}}
	projected := projectCapabilitiesForReader(catalog, reader, projectCapabilityWork(values, reader, proposalStore, nil, nil, nil))
	work := projected[0].RetirementPlans[0].Work
	if len(work) != 1 || work[0].ID != "visible-work" || !work[0].Ready {
		t.Fatalf("visible readiness leaked or lost hidden completion: %#v", work)
	}
}
