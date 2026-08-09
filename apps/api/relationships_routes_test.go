package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

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
