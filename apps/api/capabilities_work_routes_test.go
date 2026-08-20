package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/capabilities"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestRetirementWorkStaysConsumerOwnedAndReportsExactNewUse(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	proposalStore, _ := proposals.New(t.TempDir())
	releaseStore, _ := releases.New(t.TempDir())
	inventory, _ := capabilities.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, proposalStore, nil, nil, nil, nil, releaseStore, inventory))
	defer server.Close()
	provider := createTestAccount(t, server.URL, "retirement-provider")
	consumer := createTestAccount(t, server.URL, "retirement-consumer")
	newConsumer := createTestAccount(t, server.URL, "retirement-new-consumer")
	createRepo := func(name, token string) (repositories.Repository, storage.ObjectID) {
		response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"`+name+`"}`, token, http.StatusCreated)
		var repo repositories.Repository
		decodeResponse(t, response, &repo)
		gitRepo, _ := gitStore.Open(repo.ID)
		blob, err := gitRepo.WriteObject(storage.BlobObject, []byte(name))
		if err != nil {
			t.Fatal(err)
		}
		tree := writeTestTree(t, gitRepo, testTreeEntry{mode: "100644", name: "README.md", id: blob})
		return repo, writeTestCommit(t, gitRepo, tree, nil, 1700000000, name)
	}
	providerRepo, providerCommit := createRepo("retiring-provider", provider.Credential.Token)
	consumerRepo, consumerCommit := createRepo("affected-consumer", consumer.Credential.Token)
	newRepo, newCommit := createRepo("newly-found-consumer", newConsumer.Credential.Token)
	if _, err := catalog.SetVisibility(provider.User.ID, providerRepo.ID, repositories.Public); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	revision := capabilities.Revision{Name: "legacy widgets", Summary: "v1 widget contract", CommitID: string(providerCommit), ReleaseID: strings.Repeat("1", 32), OwnerIDs: []string{provider.User.ID}, Items: []capabilities.Item{{Kind: "interface", Name: "v1", Path: "README.md", Revision: string(providerCommit)}}, Consumers: []capabilities.Consumer{{Name: "affected app", RepositoryID: consumerRepo.ID, OwnerIDs: []string{consumer.User.ID}, Environment: "production", Revision: string(consumerCommit), Discovery: "declared", EvidenceState: "unknown", CompatibilityPromise: "migrate during notice"}}}
	capability, err := inventory.Create(providerRepo.ID, provider.User.ID, revision)
	if err != nil {
		t.Fatal(err)
	}
	plan := capabilities.RetirementPlan{Rationale: "retire v1", Replacements: []capabilities.Replacement{{Name: "v2", Reference: "capability:v2", MigrationGuide: "docs/v2.md", Supported: true}}, Audiences: []capabilities.Audience{{Name: "affected app", OwnerIDs: []string{consumer.User.ID}, Impact: "v1 stops"}}, Stages: []capabilities.CompatibilityStage{{Name: "adopt", StartsAt: now.Add(time.Hour), Behavior: "both", ExitCriteria: []string{"consumer merged"}}}, Deadline: now.Add(72 * time.Hour), ApprovalDueAt: now.Add(24 * time.Hour), SuccessCriteria: []string{"v2 passes"}, RollbackCriteria: []string{"consumer fails"}, Communication: capabilities.CommunicationPolicy{Channels: []string{"inbox"}, NoticeDays: 1, Updates: "daily", Escalation: "owner"}, RequiredOwnerIDs: []string{consumer.User.ID}}
	capability, err = inventory.OpenRetirement(providerRepo.ID, capability.ID, provider.User.ID, plan)
	if err != nil {
		t.Fatal(err)
	}
	plan = capability.RetirementPlans[0]
	candidateURL := fmt.Sprintf("%s/repositories/%s/capabilities/%s/retirement-plans/%s/candidates", server.URL, providerRepo.ID, capability.ID, plan.ID)
	candidateChecks := []capabilities.CandidateCheck{}
	for _, stage := range []string{"old_only", "dual_support", "replacement", "rollback", "journey"} {
		candidateChecks = append(candidateChecks, capabilities.CandidateCheck{ID: stage, Stage: stage, Journey: map[bool]string{true: "checkout"}[stage == "journey"], RepositoryID: consumerRepo.ID, Revision: string(consumerCommit), Command: "test " + stage, Paths: []string{"README.md"}, Expectation: "supported behavior"})
	}
	requestCandidate := func(checks []capabilities.CandidateCheck) {
		body, marshalErr := json.Marshal(capabilities.MigrationCandidate{Environment: "isolated", Checks: checks})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		authenticatedRequest(t, http.MethodPost, candidateURL, string(body), provider.Credential.Token, http.StatusNotFound).Body.Close()
	}
	requestCandidate(candidateChecks)
	missingPath := append([]capabilities.CandidateCheck(nil), candidateChecks...)
	missingPath[0].Paths = []string{"private-missing-path"}
	requestCandidate(missingPath)
	missingRevision := append([]capabilities.CandidateCheck(nil), candidateChecks...)
	missingRevision[0].Revision = strings.Repeat("f", 40)
	requestCandidate(missingRevision)
	workURL := fmt.Sprintf("%s/repositories/%s/capabilities/%s/retirement-plans/%s/work", server.URL, providerRepo.ID, capability.ID, plan.ID)
	body := fmt.Sprintf(`{"expected_version":0,"repository_id":"%s","title":"Adopt widget v2","completion_criteria":"Consumer tests and docs pass","assignee_type":"human","assignee_id":"%s","mandate":"Change only the affected consumer paths","base_revision":"%s","work":{"audience_index":0,"old_contract":"GET /v1 returns legacy widgets","replacement_contract":"GET /v2 returns supported widgets","acceptance_criteria":["v2 contract test passes"],"documentation_changes":["replace v1 example"],"rollout_stage":"adopt"}}`, consumerRepo.ID, consumer.User.ID, consumerCommit)
	authenticatedRequest(t, http.MethodPost, workURL, body, provider.Credential.Token, http.StatusForbidden).Body.Close()
	response := authenticatedRequest(t, http.MethodPost, workURL, body, consumer.Credential.Token, http.StatusCreated)
	var result struct {
		Capability capabilities.Capability `json:"capability"`
		Task       proposals.Task          `json:"task"`
	}
	decodeResponse(t, response, &result)
	work := result.Capability.RetirementPlans[0].Work[0]
	if result.Task.Assignment == nil || result.Task.Assignment.AssigneeID != consumer.User.ID || work.OldContract == "" || work.ReplacementContract == "" {
		t.Fatalf("consumer-owned work = %#v / %#v", work, result.Task)
	}
	proposal, err := proposalStore.Get(consumerRepo.ID, work.ProposalID)
	if err != nil || !containsAll(proposal.Body, "Old contract", "Documentation changes", "grants the retiring provider no repository") {
		t.Fatalf("proposal context = %q, %v", proposal.Body, err)
	}
	discoveryURL := fmt.Sprintf("%s/repositories/%s/capabilities/%s/retirement-plans/%s/consumer-discoveries", server.URL, providerRepo.ID, capability.ID, plan.ID)
	discovery := fmt.Sprintf(`{"expected_version":1,"discovery":{"repository_id":"%s","revision":"%s","paths":["README.md"],"evidence":["symbol:legacyClient"],"impact":"still invokes v1"}}`, newRepo.ID, newCommit)
	response = authenticatedRequest(t, http.MethodPost, discoveryURL, discovery, newConsumer.Credential.Token, http.StatusCreated)
	var updated capabilities.Capability
	if err = json.NewDecoder(response.Body).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	if len(updated.RetirementPlans[0].DiscoveredConsumers) != 1 || updated.RetirementPlans[0].DiscoveredConsumers[0].ReportedBy != newConsumer.User.ID {
		t.Fatalf("discovery = %#v", updated.RetirementPlans[0].DiscoveredConsumers)
	}
}
