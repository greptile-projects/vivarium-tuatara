package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/relationships"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestRepositoryRelationshipGraphResolvesExactEvidence(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	releaseStore, _ := releases.New(t.TempDir())
	deploymentStore, _ := deployments.New(t.TempDir())
	relationshipStore, _ := relationships.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, releaseStore, deploymentStore, relationshipStore))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "graph-owner")
	createRepo := func(name string) (repositories.Repository, storage.ObjectID) {
		response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"`+name+`"}`, owner.Credential.Token, http.StatusCreated)
		var repo repositories.Repository
		decodeResponse(t, response, &repo)
		gitRepo, _ := gitStore.Open(repo.ID)
		blob, _ := gitRepo.WriteObject(storage.BlobObject, []byte(name))
		tree := writeTestTree(t, gitRepo, testTreeEntry{mode: "100644", name: "README.md", id: blob})
		commit := writeTestCommit(t, gitRepo, tree, nil, 1700000000, name)
		return repo, commit
	}
	provider, providerCommit := createRepo("provider")
	consumer, consumerCommit := createRepo("consumer")
	providerRelease, _ := releaseStore.Create(releases.Candidate{RepositoryID: provider.ID, Version: "v1.2.0", Notes: "Contract", CommitID: string(providerCommit), CreatedBy: owner.User.ID})
	consumerRelease, _ := releaseStore.Create(releases.Candidate{RepositoryID: consumer.ID, Version: "v3.0.0", Notes: "Consumer", CommitID: string(consumerCommit), CreatedBy: owner.User.ID})
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+provider.ID+"/interfaces", `{"name":"events","release_id":"`+providerRelease.ID+`"}`, owner.Credential.Token, http.StatusCreated).Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+consumer.ID+"/dependencies", `{"commit_id":"`+string(consumerCommit)+`","release_id":"`+consumerRelease.ID+`","provider_repository_id":"`+provider.ID+`","interface_name":"events","constraint":">=v1.0.0 <v2.0.0"}`, owner.Credential.Token, http.StatusCreated).Body.Close()
	response := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+consumer.ID+"/relationships", "", owner.Credential.Token, http.StatusOK)
	var graph struct {
		Interfaces   []interfaceNode          `json:"interfaces"`
		Dependencies []dependencyEdge         `json:"dependencies"`
		Repositories []relationshipRepository `json:"repositories"`
	}
	decodeResponse(t, response, &graph)
	if len(graph.Repositories) != 2 || len(graph.Interfaces) != 1 || len(graph.Dependencies) != 1 || graph.Dependencies[0].State != "resolved" || graph.Dependencies[0].ResolvedVersion != "v1.2.0" {
		t.Fatalf("graph = %#v", graph)
	}
}

func TestAnonymousGraphDoesNotExposePrivateProviderRelationship(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	releaseStore, _ := releases.New(t.TempDir())
	deploymentStore, _ := deployments.New(t.TempDir())
	relationshipStore, _ := relationships.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, releaseStore, deploymentStore, relationshipStore))
	defer server.Close()
	consumerOwner := createTestAccount(t, server.URL, "public-consumer-owner")
	providerOwner := createTestAccount(t, server.URL, "private-provider-owner")
	create := func(name, token string) repositories.Repository {
		response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"`+name+`"}`, token, http.StatusCreated)
		var repository repositories.Repository
		decodeResponse(t, response, &repository)
		return repository
	}
	consumer := create("public-consumer", consumerOwner.Credential.Token)
	provider := create("private-provider", providerOwner.Credential.Token)
	if _, err := catalog.SetVisibility(consumerOwner.User.ID, consumer.ID, repositories.Public); err != nil {
		t.Fatal(err)
	}
	_, err := relationshipStore.CreateDependency(relationships.Dependency{RepositoryID: consumer.ID, CommitID: strings.Repeat("a", 40), ProviderRepositoryID: provider.ID, InterfaceName: "private-contract", Constraint: "*", DeclaredBy: consumerOwner.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	response := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+consumer.ID+"/relationships", "", "", http.StatusOK)
	var graph struct {
		Dependencies []dependencyEdge         `json:"dependencies"`
		Repositories []relationshipRepository `json:"repositories"`
	}
	decodeResponse(t, response, &graph)
	if len(graph.Dependencies) != 0 || len(graph.Repositories) != 1 || graph.Repositories[0].ID != consumer.ID {
		t.Fatalf("private relationship escaped authorization: %#v", graph)
	}
}

func TestRelationshipGraphUsesLatestEnvironmentDeployment(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	releaseStore, _ := releases.New(t.TempDir())
	deploymentStore, _ := deployments.New(t.TempDir())
	relationshipStore, _ := relationships.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, releaseStore, deploymentStore, relationshipStore))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "deployment-graph-owner")
	createRepo := func(name string) (repositories.Repository, storage.ObjectID, storage.ObjectID) {
		response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"`+name+`"}`, owner.Credential.Token, http.StatusCreated)
		var repo repositories.Repository
		decodeResponse(t, response, &repo)
		gitRepo, _ := gitStore.Open(repo.ID)
		blobA, _ := gitRepo.WriteObject(storage.BlobObject, []byte("A"))
		treeA := writeTestTree(t, gitRepo, testTreeEntry{mode: "100644", name: "value", id: blobA})
		commitA := writeTestCommit(t, gitRepo, treeA, nil, 1700000000, "A")
		blobB, _ := gitRepo.WriteObject(storage.BlobObject, []byte("B"))
		treeB := writeTestTree(t, gitRepo, testTreeEntry{mode: "100644", name: "value", id: blobB})
		commitB := writeTestCommit(t, gitRepo, treeB, []storage.ObjectID{commitA}, 1700000100, "B")
		return repo, commitA, commitB
	}
	provider, providerCommit, _ := createRepo("deployment-provider")
	consumer, commitA, commitB := createRepo("deployment-consumer")
	providerRelease, _ := releaseStore.Create(releases.Candidate{RepositoryID: provider.ID, Version: "v1.0.0", Notes: "Provider", CommitID: string(providerCommit), CreatedBy: owner.User.ID})
	consumerReleaseA, _ := releaseStore.Create(releases.Candidate{RepositoryID: consumer.ID, Version: "v1.0.0", Notes: "A", CommitID: string(commitA), CreatedBy: owner.User.ID})
	consumerReleaseB, _ := releaseStore.Create(releases.Candidate{RepositoryID: consumer.ID, Version: "v2.0.0", Notes: "B", CommitID: string(commitB), CreatedBy: owner.User.ID})
	_, _ = relationshipStore.CreateInterface(relationships.Interface{RepositoryID: provider.ID, Name: "events", Version: providerRelease.Version, ReleaseID: providerRelease.ID, CommitID: providerRelease.CommitID, PublishedBy: owner.User.ID})
	environment, err := deploymentStore.PutEnvironment(deployments.Environment{RepositoryID: consumer.ID, Name: "production", Position: 1, Image: "alpine:3.22", Command: "true", TimeoutSeconds: 30, RequiredApprovals: 0, Concurrency: 1, UpdatedBy: owner.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	definition := deployments.RolloutDefinition{Version: 1, Stages: []deployments.RolloutStage{{Name: "all", Signals: []deployments.HealthSignal{{Name: "health", Command: "true"}}}}}
	deploy := func(release releases.Candidate, commit storage.ObjectID, marker string) {
		promotion, createErr := deploymentStore.CreatePromotion(deployments.Promotion{RepositoryID: consumer.ID, EnvironmentID: environment.ID, ReleaseID: release.ID, BuildID: strings.Repeat(marker, 32), ArtifactID: strings.Repeat(marker, 32), ArtifactSHA256: strings.Repeat(marker, 64), CommitID: string(commit), Rollout: definition, InitiatedBy: owner.User.ID})
		if createErr != nil {
			t.Fatal(createErr)
		}
		claimed, claimErr := deploymentStore.Claim(consumer.ID, promotion.ID, owner.User.ID, time.Now().Add(time.Minute))
		if claimErr != nil {
			t.Fatal(claimErr)
		}
		if _, completeErr := deploymentStore.Complete(consumer.ID, claimed.ID, owner.User.ID, "succeeded", "done"); completeErr != nil {
			t.Fatal(completeErr)
		}
	}
	deploy(consumerReleaseA, commitA, "a")
	_, err = relationshipStore.CreateDependency(relationships.Dependency{RepositoryID: consumer.ID, CommitID: string(commitA), ReleaseID: consumerReleaseA.ID, EnvironmentID: environment.ID, ProviderRepositoryID: provider.ID, InterfaceName: "events", Constraint: "*", DeclaredBy: owner.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	deploy(consumerReleaseB, commitB, "b")
	response := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+consumer.ID+"/relationships", "", owner.Credential.Token, http.StatusOK)
	var graph struct {
		Dependencies []dependencyEdge `json:"dependencies"`
	}
	decodeResponse(t, response, &graph)
	if len(graph.Dependencies) != 1 || graph.Dependencies[0].State != "stale" {
		t.Fatalf("superseded deployment graph = %#v", graph)
	}
}
