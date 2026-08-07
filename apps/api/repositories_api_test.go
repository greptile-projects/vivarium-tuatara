package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestOwnedRepositoryLifecycleProvidesUsableGitRemote(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	server := httptest.NewServer(newAuthenticatedAppHandler(gitStore, identities, credentials, catalog))
	defer server.Close()

	account := createTestAccount(t, server.URL, "owner")
	createdResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"collaboration"}`, account.Credential.Token, http.StatusCreated)
	var created repositories.Repository
	if err := json.NewDecoder(createdResponse.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	createdResponse.Body.Close()
	if created.OwnerID != account.User.ID || created.GitRemote != "/git/"+created.ID+".git" || created.DefaultBranch != "main" {
		t.Fatalf("created repository = %#v", created)
	}

	inspectedResponse := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+created.ID, "", account.Credential.Token, http.StatusOK)
	var inspected repositories.Repository
	if err := json.NewDecoder(inspectedResponse.Body).Decode(&inspected); err != nil {
		t.Fatal(err)
	}
	inspectedResponse.Body.Close()
	if inspected != created {
		t.Fatalf("inspected = %#v, created = %#v", inspected, created)
	}

	listedResponse := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories", "", account.Credential.Token, http.StatusOK)
	var listed map[string][]repositories.Repository
	if err := json.NewDecoder(listedResponse.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	listedResponse.Body.Close()
	if len(listed["repositories"]) != 1 || listed["repositories"][0] != created {
		t.Fatalf("listed = %#v", listed)
	}

	gitCredential, err := credentials.Issue(account.User.ID, auth.Git, "repository remote", []string{"git:read", "git:write"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	remote, _ := url.Parse(server.URL + created.GitRemote)
	remote.User = url.UserPassword("git", gitCredential.Token)
	if output, err := execGit(remote.String(), "ls-remote").CombinedOutput(); err != nil {
		t.Fatalf("use advertised Git remote: %v\n%s", err, output)
	}

	other := createTestAccount(t, server.URL, "other")
	authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+created.ID, "", other.Credential.Token, http.StatusNotFound).Body.Close()
	authenticatedRequest(t, http.MethodDelete, server.URL+"/repositories/"+created.ID, "", other.Credential.Token, http.StatusNotFound).Body.Close()

	authenticatedRequest(t, http.MethodDelete, server.URL+"/repositories/"+created.ID, "", account.Credential.Token, http.StatusNoContent).Body.Close()
	authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+created.ID, "", account.Credential.Token, http.StatusNotFound).Body.Close()
	if output, err := execGit(remote.String(), "ls-remote").CombinedOutput(); err == nil {
		t.Fatalf("deleted Git remote remained available: %s", output)
	}
}

func TestRepositoryNamesAreUniquePerOwner(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	server := httptest.NewServer(newAuthenticatedAppHandler(gitStore, identities, credentials, catalog))
	defer server.Close()
	first := createTestAccount(t, server.URL, "first")
	second := createTestAccount(t, server.URL, "second")
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"Project"}`, first.Credential.Token, http.StatusCreated).Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"project"}`, first.Credential.Token, http.StatusConflict).Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"Project"}`, second.Credential.Token, http.StatusCreated).Body.Close()
}

func createTestAccount(t *testing.T, baseURL, handle string) accountResponse {
	t.Helper()
	response := requestStatus(t, http.MethodPost, baseURL+"/users", `{"handle":"`+handle+`","display_name":"Test User"}`, http.StatusCreated)
	defer response.Body.Close()
	var account accountResponse
	if err := json.NewDecoder(response.Body).Decode(&account); err != nil {
		t.Fatal(err)
	}
	return account
}
