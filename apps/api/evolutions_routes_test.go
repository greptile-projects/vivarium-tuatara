package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/relationships"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestEvolutionPlanFreezesImpactAndScopesAgentFindings(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	proposalStore, _ := proposals.New(t.TempDir())
	pullStore, _ := pullrequests.New(t.TempDir(), gitStore)
	releaseStore, _ := releases.New(t.TempDir())
	deploymentStore, _ := deployments.New(t.TempDir())
	relationStore, _ := relationships.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, proposalStore, pullStore, nil, nil, nil, releaseStore, deploymentStore, relationStore))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "evolution-owner")
	createRepo := func(name string) (repositories.Repository, storage.ObjectID) {
		response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"`+name+`"}`, owner.Credential.Token, http.StatusCreated)
		var repo repositories.Repository
		decodeResponse(t, response, &repo)
		r, _ := gitStore.Open(repo.ID)
		blob, _ := r.WriteObject(storage.BlobObject, []byte(name))
		tree := writeTestTree(t, r, testTreeEntry{mode: "100644", name: "README.md", id: blob})
		return repo, writeTestCommit(t, r, tree, nil, 1700000000, name)
	}
	provider, providerCommit := createRepo("evolution-provider")
	consumer, consumerCommit := createRepo("evolution-consumer")
	release, _ := releaseStore.Create(releases.Candidate{RepositoryID: provider.ID, Version: "v1.0.0", Notes: "contract", CommitID: string(providerCommit), CreatedBy: owner.User.ID})
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+provider.ID+"/interfaces", `{"name":"events","release_id":"`+release.ID+`"}`, owner.Credential.Token, http.StatusCreated)
	var predecessor relationships.Interface
	decodeResponse(t, response, &predecessor)
	_, _ = relationStore.CreateDependency(relationships.Dependency{RepositoryID: consumer.ID, CommitID: string(consumerCommit), ProviderRepositoryID: provider.ID, InterfaceName: "events", Constraint: "<v2.0.0", DeclaredBy: owner.User.ID})
	_, _ = relationStore.CreateDependency(relationships.Dependency{RepositoryID: consumer.ID, CommitID: string(consumerCommit), ReleaseID: "99999999999999999999999999999999", ProviderRepositoryID: provider.ID, InterfaceName: "events", Constraint: "<v2.0.0", DeclaredBy: owner.User.ID})
	proposal, err := proposalStore.Create(provider.ID, owner.User.ID, "Events v2", "Remove legacy field")
	if err != nil {
		t.Fatal(err)
	}
	body := `{"interface_name":"events","predecessor_interface_id":"` + predecessor.ID + `","source_kind":"proposal","source_id":"` + proposal.ID + `","candidate_description":"remove legacy field","changes":[{"kind":"field removal","summary":"old clients read it","classification":"breaking"}],"strategy":"dual publish","sequencing":"consumer first"}`
	response = authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+provider.ID+"/evolutions", body, owner.Credential.Token, http.StatusCreated)
	var plan relationships.Evolution
	decodeResponse(t, response, &plan)
	if len(plan.Impacts) != 1 || plan.Impacts[0].OwnerID != owner.User.ID {
		t.Fatalf("impact = %#v", plan.Impacts)
	}
	plan, err = relationStore.AcknowledgeEvolution(provider.ID, plan.ID, owner.User.ID, consumer.ID, "consumer migration accepted")
	if err != nil {
		t.Fatal(err)
	}
	analysisBody := `{"mandate":"inspect consumer call sites","repository_ids":["` + consumer.ID + `"]}`
	response = authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+provider.ID+"/evolutions/"+plan.ID+"/analyses", analysisBody, owner.Credential.Token, http.StatusCreated)
	var delegated struct {
		Analysis   relationships.EvolutionAnalysis `json:"analysis"`
		Credential auth.IssuedCredential           `json:"credential"`
	}
	decodeResponse(t, response, &delegated)
	findingBody := `{"repository_ids":["` + consumer.ID + `"],"finding":"two callers require a staged migration","uncertainty":"generated clients were not inspected"}`
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+provider.ID+"/evolutions/"+plan.ID+"/analyses/"+delegated.Analysis.ID+"/findings", findingBody, delegated.Credential.Token, http.StatusCreated).Body.Close()
	authenticatedRequest(t, http.MethodPatch, server.URL+"/repositories/"+provider.ID, `{"name":"forbidden"}`, delegated.Credential.Token, http.StatusUnauthorized).Body.Close()
	response = authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+provider.ID+"/evolutions/"+plan.ID, "", owner.Credential.Token, http.StatusOK)
	decodeResponse(t, response, &plan)
	if len(plan.Findings) != 1 || !strings.Contains(plan.Findings[0].Uncertainty, "generated") {
		t.Fatalf("findings = %#v", plan.Findings)
	}
	if _, err = catalog.SetVisibility(owner.User.ID, provider.ID, repositories.Public); err != nil {
		t.Fatal(err)
	}
	response = authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+provider.ID+"/evolutions/"+plan.ID, "", "", http.StatusOK)
	var anonymous relationships.Evolution
	decodeResponse(t, response, &anonymous)
	if len(anonymous.Impacts) != 0 || len(anonymous.Findings) != 0 || len(anonymous.Acknowledgements) != 0 {
		t.Fatalf("anonymous plan leaked private evidence: %#v", anonymous)
	}
	response = authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+provider.ID+"/evolutions", "", "", http.StatusOK)
	var anonymousCollection struct {
		Evolutions []relationships.Evolution `json:"evolutions"`
	}
	decodeResponse(t, response, &anonymousCollection)
	if len(anonymousCollection.Evolutions) != 1 || len(anonymousCollection.Evolutions[0].Acknowledgements) != 0 {
		t.Fatalf("anonymous collection leaked private acknowledgements: %#v", anonymousCollection)
	}
	response = authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+provider.ID+"/evolutions/"+plan.ID, "", owner.Credential.Token, http.StatusOK)
	decodeResponse(t, response, &plan)
	if len(plan.Impacts) != 1 || len(plan.Findings) != 1 || len(plan.Acknowledgements) != 1 {
		t.Fatalf("authenticated public-provider view = %#v", plan)
	}
}

func TestEvolutionAcknowledgementResponseFiltersOtherPrivateConsumers(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	releaseStore, _ := releases.New(t.TempDir())
	deploymentStore, _ := deployments.New(t.TempDir())
	relationStore, _ := relationships.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, releaseStore, deploymentStore, relationStore))
	defer server.Close()
	providerOwner := createTestAccount(t, server.URL, "evolution-provider-owner")
	consumerAOwner := createTestAccount(t, server.URL, "evolution-consumer-a-owner")
	createRepo := func(name, token string) repositories.Repository {
		response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"`+name+`"}`, token, http.StatusCreated)
		var repo repositories.Repository
		decodeResponse(t, response, &repo)
		return repo
	}
	provider := createRepo("ack-provider", providerOwner.Credential.Token)
	consumerA := createRepo("ack-consumer-a", consumerAOwner.Credential.Token)
	consumerB := createRepo("ack-consumer-b", providerOwner.Credential.Token)
	if _, err := catalog.SetVisibility(providerOwner.User.ID, provider.ID, repositories.Public); err != nil {
		t.Fatal(err)
	}
	predecessor := relationships.Interface{ID: "11111111111111111111111111111111", RepositoryID: provider.ID, Name: "events", Version: "v1.0.0", ReleaseID: "22222222222222222222222222222222", CommitID: strings.Repeat("a", 40), PublishedBy: providerOwner.User.ID}
	plan, err := relationStore.CreateEvolution(relationships.Evolution{
		RepositoryID: provider.ID, InterfaceName: "events", Predecessor: predecessor,
		SourceKind: "proposal", SourceID: "33333333333333333333333333333333", CandidateDescription: "replace legacy events",
		Changes: []relationships.CompatibilityChange{{Kind: "replacement", Summary: "consumers must migrate", Classification: "breaking"}},
		Impacts: []relationships.ConsumerImpact{
			{RepositoryID: consumerA.ID, OwnerID: consumerAOwner.User.ID, DependencyID: "44444444444444444444444444444444", CommitID: strings.Repeat("b", 40), Constraint: "<v2.0.0", State: "affected"},
			{RepositoryID: consumerB.ID, OwnerID: providerOwner.User.ID, DependencyID: "55555555555555555555555555555555", CommitID: strings.Repeat("c", 40), Constraint: "<v2.0.0", State: "affected"},
		},
		Strategy: "dual publish", Sequencing: "consumers first", CreatedBy: providerOwner.User.ID,
		Findings: []relationships.EvolutionFinding{{ID: "66666666666666666666666666666666", ActorID: "77777777777777777777777777777777", RepositoryIDs: []string{consumerB.ID}, Finding: "consumer-b-only finding", Uncertainty: "consumer-b-only uncertainty"}},
		Analyses: []relationships.EvolutionAnalysis{{ID: "88888888888888888888888888888888", AgentID: "99999999999999999999999999999999", InitiatorID: providerOwner.User.ID, StoredCredentialID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Mandate: "consumer-b-only mandate", RepositoryIDs: []string{consumerB.ID}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	plan, err = relationStore.AcknowledgeEvolution(provider.ID, plan.ID, providerOwner.User.ID, consumerB.ID, "consumer-b-only acknowledgement")
	if err != nil {
		t.Fatal(err)
	}
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+provider.ID+"/evolutions/"+plan.ID+"/acknowledgements", `{"repository_id":"`+consumerA.ID+`","note":"consumer A accepts"}`, consumerAOwner.Credential.Token, http.StatusCreated)
	var visible relationships.Evolution
	decodeResponse(t, response, &visible)
	if len(visible.Impacts) != 1 || visible.Impacts[0].RepositoryID != consumerA.ID || len(visible.Acknowledgements) != 1 || visible.Acknowledgements[0].RepositoryID != consumerA.ID || len(visible.Findings) != 0 || len(visible.Analyses) != 0 {
		t.Fatalf("acknowledgement response leaked another consumer: %#v", visible)
	}
}
