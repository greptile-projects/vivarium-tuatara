package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestRepositoryBrowsingPreservesBranchAndCommitRevision(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	server := httptest.NewServer(newAuthenticatedAppHandler(gitStore, identities, credentials, catalog))
	defer server.Close()

	account := createTestAccount(t, server.URL, "browser")
	created := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"project"}`, account.Credential.Token, http.StatusCreated)
	var repository repositories.Repository
	if err := json.NewDecoder(created.Body).Decode(&repository); err != nil {
		t.Fatal(err)
	}
	created.Body.Close()
	repo, _ := gitStore.Open(repository.ID)
	readme, _ := repo.WriteObject(storage.BlobObject, []byte("# project\n"))
	source, _ := repo.WriteObject(storage.BlobObject, []byte("package main\n"))
	srcTree := writeTestTree(t, repo, testTreeEntry{"100644", "main.go", source})
	rootTree := writeTestTree(t, repo, testTreeEntry{"100644", "README.md", readme}, testTreeEntry{"40000", "src", srcTree})
	commit := writeTestCommit(t, repo, rootTree, nil, 1700000000, "initial browser state")
	if err := repo.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(commit)}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateReference(storage.Reference{Name: "refs/heads/feature", Target: string(commit)}); err != nil {
		t.Fatal(err)
	}

	for _, endpoint := range []string{
		"/branches",
		"/commits?ref=feature",
		"/tree?ref=feature",
		"/tree?ref=" + string(commit) + "&path=src",
		"/blob?ref=feature&path=README.md",
	} {
		response := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repository.ID+endpoint, "", account.Credential.Token, http.StatusOK)
		var body map[string]any
		if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if endpoint != "/branches" && body["revision"] != string(commit) {
			t.Fatalf("%s revision = %#v", endpoint, body["revision"])
		}
	}

	missing := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repository.ID+"/tree?ref=missing", "", account.Credential.Token, http.StatusNotFound)
	missing.Body.Close()
}

func TestPublicRepositoryBrowsingIsAnonymousAndPrivateBrowsingIsHidden(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	server := httptest.NewServer(newAuthenticatedAppHandler(gitStore, identities, credentials, catalog))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "browse-owner")
	created := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"visible"}`, owner.Credential.Token, http.StatusCreated)
	var repository repositories.Repository
	json.NewDecoder(created.Body).Decode(&repository)
	created.Body.Close()

	requestStatus(t, http.MethodGet, server.URL+"/repositories/"+repository.ID+"/branches", "", http.StatusUnauthorized).Body.Close()
	authenticatedRequest(t, http.MethodPatch, server.URL+"/repositories/"+repository.ID, `{"visibility":"public"}`, owner.Credential.Token, http.StatusOK).Body.Close()
	requestStatus(t, http.MethodGet, server.URL+"/repositories/"+repository.ID+"/branches", "", http.StatusOK).Body.Close()
}
