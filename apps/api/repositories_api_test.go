package main

import (
	"encoding/json"
	"io"
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

func TestPublicApplicationContractSupportsAccountAndPagination(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	server := httptest.NewServer(newAuthenticatedAppHandler(gitStore, identities, credentials, catalog))
	defer server.Close()

	account := createTestAccount(t, server.URL, "contract-user")
	current := authenticatedRequest(t, http.MethodGet, server.URL+"/user", "", account.Credential.Token, http.StatusOK)
	var currentUser users.User
	if err := json.NewDecoder(current.Body).Decode(&currentUser); err != nil {
		t.Fatal(err)
	}
	current.Body.Close()
	if currentUser.ID != account.User.ID {
		t.Fatalf("current user = %#v", currentUser)
	}

	for _, name := range []string{"first", "second", "third"} {
		authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"`+name+`"}`, account.Credential.Token, http.StatusCreated).Body.Close()
	}
	first := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories?limit=2", "", account.Credential.Token, http.StatusOK)
	var firstPage struct {
		Repositories []repositories.Repository `json:"repositories"`
		NextCursor   *string                   `json:"next_cursor"`
	}
	if err := json.NewDecoder(first.Body).Decode(&firstPage); err != nil {
		t.Fatal(err)
	}
	first.Body.Close()
	if len(firstPage.Repositories) != 2 || firstPage.NextCursor == nil || *firstPage.NextCursor != firstPage.Repositories[1].ID {
		t.Fatalf("first page = %#v", firstPage)
	}
	second := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories?limit=2&after="+*firstPage.NextCursor, "", account.Credential.Token, http.StatusOK)
	var secondPage struct {
		Repositories []repositories.Repository `json:"repositories"`
		NextCursor   *string                   `json:"next_cursor"`
	}
	if err := json.NewDecoder(second.Body).Decode(&secondPage); err != nil {
		t.Fatal(err)
	}
	second.Body.Close()
	if len(secondPage.Repositories) != 1 || secondPage.NextCursor != nil || secondPage.Repositories[0].Name != "third" {
		t.Fatalf("second page = %#v", secondPage)
	}

	invalid := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories?limit=0", "", account.Credential.Token, http.StatusBadRequest)
	data, _ := io.ReadAll(invalid.Body)
	invalid.Body.Close()
	var failure struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &failure) != nil || failure.Error.Code != "invalid_pagination" || failure.Error.Message == "" {
		t.Fatalf("error response = %s", data)
	}
}

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

func TestRepositoryAuthorizationIsConsistentAcrossAPIAndGit(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	server := httptest.NewServer(newAuthenticatedAppHandler(gitStore, identities, credentials, catalog))
	defer server.Close()

	owner := createTestAccount(t, server.URL, "access-owner")
	other := createTestAccount(t, server.URL, "access-other")
	createdResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"protected"}`, owner.Credential.Token, http.StatusCreated)
	var repository repositories.Repository
	if err := json.NewDecoder(createdResponse.Body).Decode(&repository); err != nil {
		t.Fatal(err)
	}
	createdResponse.Body.Close()
	if repository.Visibility != repositories.Private {
		t.Fatalf("default visibility = %q", repository.Visibility)
	}

	ownerGit, _ := credentials.Issue(owner.User.ID, auth.Git, "owner", []string{"git:read", "git:write"}, time.Hour)
	otherGit, _ := credentials.Issue(other.User.ID, auth.Git, "other", []string{"git:read", "git:write"}, time.Hour)
	gitURL := server.URL + repository.GitRemote
	gitWithToken := func(token string) string {
		parsed, _ := url.Parse(gitURL)
		parsed.User = url.UserPassword("git", token)
		return parsed.String()
	}

	requestStatus(t, http.MethodGet, server.URL+"/repositories/"+repository.ID, "", http.StatusUnauthorized).Body.Close()
	authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repository.ID, "", other.Credential.Token, http.StatusNotFound).Body.Close()
	if err := execGit(gitURL, "ls-remote").Run(); err == nil {
		t.Fatal("anonymous private Git read succeeded")
	}
	if err := execGit(gitWithToken(otherGit.Token), "ls-remote").Run(); err == nil {
		t.Fatal("non-owner private Git read succeeded")
	}
	if output, err := execGit(gitWithToken(ownerGit.Token), "ls-remote").CombinedOutput(); err != nil {
		t.Fatalf("owner private Git read: %v\n%s", err, output)
	}

	patched := authenticatedRequest(t, http.MethodPatch, server.URL+"/repositories/"+repository.ID, `{"visibility":"public"}`, owner.Credential.Token, http.StatusOK)
	patched.Body.Close()
	requestStatus(t, http.MethodGet, server.URL+"/repositories/"+repository.ID, "", http.StatusOK).Body.Close()
	authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repository.ID, "", "malformed-token", http.StatusOK).Body.Close()
	if output, err := execGit(gitURL, "ls-remote").CombinedOutput(); err != nil {
		t.Fatalf("anonymous public Git read: %v\n%s", err, output)
	}
	writeOnlyGit, _ := credentials.Issue(other.User.ID, auth.Git, "write only", []string{"git:write"}, time.Hour)
	if output, err := execGit(gitWithToken(writeOnlyGit.Token), "ls-remote").CombinedOutput(); err != nil {
		t.Fatalf("public Git read with under-scoped credential: %v\n%s", err, output)
	}
	if output, err := execGit(gitWithToken("malformed-token"), "ls-remote").CombinedOutput(); err != nil {
		t.Fatalf("public Git read with malformed credential: %v\n%s", err, output)
	}
	assertGitDiscoveryStatus(t, gitURL+"/info/refs?service=git-receive-pack", "", http.StatusUnauthorized)
	assertGitDiscoveryStatus(t, gitURL+"/info/refs?service=git-receive-pack", otherGit.Token, http.StatusNotFound)
	assertGitDiscoveryStatus(t, gitURL+"/info/refs?service=git-receive-pack", ownerGit.Token, http.StatusOK)

	authenticatedRequest(t, http.MethodPatch, server.URL+"/repositories/"+repository.ID, `{"visibility":"private"}`, other.Credential.Token, http.StatusNotFound).Body.Close()
	authenticatedRequest(t, http.MethodDelete, server.URL+"/repositories/"+repository.ID, "", other.Credential.Token, http.StatusNotFound).Body.Close()
	requestStatus(t, http.MethodDelete, server.URL+"/repositories/"+repository.ID, "", http.StatusUnauthorized).Body.Close()
}

func assertGitDiscoveryStatus(t *testing.T, target, token string, want int) {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		request.SetBasicAuth("git", token)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != want {
		t.Fatalf("Git discovery status = %d, want %d", response.StatusCode, want)
	}
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
