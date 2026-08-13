package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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
	for _, path := range []string{
		"/repositories?limit=",
		"/repositories?after=",
		"/auth/credentials?limit=",
		"/auth/credentials?after=",
	} {
		response := authenticatedRequest(t, http.MethodGet, server.URL+path, "", account.Credential.Token, http.StatusBadRequest)
		response.Body.Close()
	}
}

func TestRepositoryBoundAPIReadCannotCrossRepository(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	userID := "0123456789abcdef0123456789abcdef"
	first, _ := catalog.Create(userID, "first-bound")
	second, _ := catalog.Create(userID, "second-bound")
	issued, err := credentials.IssueOrganizationAgent(userID, "bounded read", "11111111111111111111111111111111", "22222222222222222222222222222222", "33333333333333333333333333333333", first.ID, []string{"repositories:read"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repositories/{id}/bounded-read", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); ok {
			w.WriteHeader(http.StatusNoContent)
		}
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+first.ID+"/bounded-read", "", issued.Token, http.StatusNoContent).Body.Close()
	authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+second.ID+"/bounded-read", "", issued.Token, http.StatusNotFound).Body.Close()
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

func TestReadableRepositoryCanBeForkedAndSynchronizedWithoutSourceAuthority(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	server := httptest.NewServer(newAuthenticatedAppHandler(gitStore, identities, credentials, catalog))
	defer server.Close()

	maintainer := createTestAccount(t, server.URL, "fork-maintainer")
	newcomer := createTestAccount(t, server.URL, "fork-newcomer")
	created := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"upstream"}`, maintainer.Credential.Token, http.StatusCreated)
	var upstream repositories.Repository
	if err := json.NewDecoder(created.Body).Decode(&upstream); err != nil {
		t.Fatal(err)
	}
	created.Body.Close()
	authenticatedRequest(t, http.MethodPatch, server.URL+"/repositories/"+upstream.ID, `{"visibility":"public"}`, maintainer.Credential.Token, http.StatusOK).Body.Close()

	maintainerGit, _ := credentials.Issue(maintainer.User.ID, auth.Git, "upstream", []string{"git:read", "git:write"}, time.Hour)
	newcomerGit, _ := credentials.Issue(newcomer.User.ID, auth.Git, "fork", []string{"git:read", "git:write"}, time.Hour)
	remoteWith := func(repository repositories.Repository, token string) string {
		parsed, _ := url.Parse(server.URL + repository.GitRemote)
		parsed.User = url.UserPassword("git", token)
		return parsed.String()
	}
	upstreamCopy := filepath.Join(t.TempDir(), "upstream")
	gitCommand(t, "", "clone", remoteWith(upstream, maintainerGit.Token), upstreamCopy)
	gitCommand(t, upstreamCopy, "config", "user.name", "Maintainer")
	gitCommand(t, upstreamCopy, "config", "user.email", "maintainer@example.com")
	if err := os.WriteFile(filepath.Join(upstreamCopy, "README.md"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, upstreamCopy, "add", "README.md")
	gitCommand(t, upstreamCopy, "commit", "-m", "initial upstream")
	initial := strings.TrimSpace(gitCommand(t, upstreamCopy, "rev-parse", "HEAD"))
	gitCommand(t, upstreamCopy, "push", "origin", "main")

	forkResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+upstream.ID+"/forks", `{"name":"independent"}`, newcomer.Credential.Token, http.StatusCreated)
	var fork repositories.Repository
	if err := json.NewDecoder(forkResponse.Body).Decode(&fork); err != nil {
		t.Fatal(err)
	}
	forkResponse.Body.Close()
	if fork.OwnerID != newcomer.User.ID || fork.UpstreamRepositoryID != upstream.ID || fork.Visibility != repositories.Private {
		t.Fatalf("fork = %#v", fork)
	}
	if _, err := catalog.HasCollaborator(newcomer.User.ID, upstream.ID); err != nil {
		t.Fatal(err)
	}
	if collaborator, _ := catalog.HasCollaborator(newcomer.User.ID, upstream.ID); collaborator {
		t.Fatal("fork owner gained upstream contributor authority")
	}

	forkCopy := filepath.Join(t.TempDir(), "fork")
	gitCommand(t, "", "clone", remoteWith(fork, newcomerGit.Token), forkCopy)
	if got := strings.TrimSpace(gitCommand(t, forkCopy, "rev-parse", "HEAD")); got != initial {
		t.Fatalf("fork head = %s, want %s", got, initial)
	}
	gitCommand(t, forkCopy, "config", "user.name", "Newcomer")
	gitCommand(t, forkCopy, "config", "user.email", "newcomer@example.com")
	gitCommand(t, forkCopy, "switch", "-c", "experiment")
	if err := os.WriteFile(filepath.Join(forkCopy, "idea.txt"), []byte("independent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, forkCopy, "add", "idea.txt")
	gitCommand(t, forkCopy, "commit", "-m", "independent work")
	gitCommand(t, forkCopy, "push", "origin", "experiment")

	if err := os.WriteFile(filepath.Join(upstreamCopy, "README.md"), []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, upstreamCopy, "add", "README.md")
	gitCommand(t, upstreamCopy, "commit", "-m", "new upstream history")
	updated := strings.TrimSpace(gitCommand(t, upstreamCopy, "rev-parse", "HEAD"))
	gitCommand(t, upstreamCopy, "push", "origin", "main")
	syncResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+fork.ID+"/synchronizations", `{"branch":"main"}`, newcomer.Credential.Token, http.StatusOK)
	var synchronized repositories.ForkSynchronization
	if err := json.NewDecoder(syncResponse.Body).Decode(&synchronized); err != nil {
		t.Fatal(err)
	}
	syncResponse.Body.Close()
	if synchronized.PreviousCommitID != initial || synchronized.CommitID != updated || synchronized.UpstreamRepositoryID != upstream.ID {
		t.Fatalf("synchronization = %#v", synchronized)
	}
	gitCommand(t, forkCopy, "fetch", "origin", "main")
	if got := strings.TrimSpace(gitCommand(t, forkCopy, "rev-parse", "origin/main")); got != updated {
		t.Fatalf("synchronized fork head = %s, want %s", got, updated)
	}
	if got := strings.TrimSpace(gitCommand(t, forkCopy, "rev-parse", "origin/experiment")); got == "" {
		t.Fatal("independent fork branch disappeared")
	}

	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+fork.ID+"/synchronizations", `{"branch":"main"}`, maintainer.Credential.Token, http.StatusNotFound).Body.Close()
	authenticatedRequest(t, http.MethodDelete, server.URL+"/repositories/"+upstream.ID, "", maintainer.Credential.Token, http.StatusNoContent).Body.Close()
	independentClone := filepath.Join(t.TempDir(), "independent-after-upstream-delete")
	gitCommand(t, "", "clone", remoteWith(fork, newcomerGit.Token), independentClone)
	if got := strings.TrimSpace(gitCommand(t, independentClone, "rev-parse", "HEAD")); got != updated {
		t.Fatalf("fork after upstream deletion = %s, want %s", got, updated)
	}
	gitCommand(t, independentClone, "fsck", "--full")
}

func TestForkSynchronizationRechecksPrivateUpstreamAccess(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	server := httptest.NewServer(newAuthenticatedAppHandler(gitStore, identities, credentials, catalog))
	defer server.Close()

	maintainer := createTestAccount(t, server.URL, "private-upstream-owner")
	reader := createTestAccount(t, server.URL, "private-upstream-reader")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"private-upstream"}`, maintainer.Credential.Token, http.StatusCreated)
	var upstream repositories.Repository
	if err := json.NewDecoder(response.Body).Decode(&upstream); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+upstream.ID+"/collaborators", `{"user_id":"`+reader.User.ID+`"}`, maintainer.Credential.Token, http.StatusCreated).Body.Close()

	upstreamGit, _ := gitStore.Open(upstream.ID)
	tree, err := upstreamGit.WriteObject(storage.TreeObject, nil)
	if err != nil {
		t.Fatal(err)
	}
	initial := writeTestCommit(t, upstreamGit, tree, nil, 1700000000, "initial private history")
	if err := upstreamGit.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(initial)}); err != nil {
		t.Fatal(err)
	}
	forkResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+upstream.ID+"/forks", `{"name":"private-fork"}`, reader.Credential.Token, http.StatusCreated)
	var fork repositories.Repository
	if err := json.NewDecoder(forkResponse.Body).Decode(&fork); err != nil {
		t.Fatal(err)
	}
	forkResponse.Body.Close()

	authenticatedRequest(t, http.MethodDelete, server.URL+"/repositories/"+upstream.ID+"/collaborators/"+reader.User.ID, "", maintainer.Credential.Token, http.StatusNoContent).Body.Close()
	later := writeTestCommit(t, upstreamGit, tree, []storage.ObjectID{initial}, 1700000100, "later private history")
	if err := upstreamGit.UpdateReference(storage.Reference{Name: "refs/heads/main", Target: string(later)}); err != nil {
		t.Fatal(err)
	}
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+fork.ID+"/synchronizations", `{"branch":"main"}`, reader.Credential.Token, http.StatusNotFound).Body.Close()
	forkGit, _ := gitStore.Open(fork.ID)
	main, err := forkGit.ReadReference("refs/heads/main")
	if err != nil || main.Target != string(initial) {
		t.Fatalf("fork main after revoked synchronization = %#v, %v", main, err)
	}
	if _, err := forkGit.ReadObject(later); !errors.Is(err, storage.ErrObjectNotFound) {
		t.Fatalf("private upstream object imported after revocation: %v", err)
	}
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

func TestOwnerCanGrantAndRevokeContributorCandidateAccess(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	server := httptest.NewServer(newAuthenticatedAppHandler(gitStore, identities, credentials, catalog))
	defer server.Close()

	owner := createTestAccount(t, server.URL, "grant-owner")
	contributor := createTestAccount(t, server.URL, "grant-contributor")
	createdResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"shared"}`, owner.Credential.Token, http.StatusCreated)
	var repository repositories.Repository
	if err := json.NewDecoder(createdResponse.Body).Decode(&repository); err != nil {
		t.Fatal(err)
	}
	createdResponse.Body.Close()

	grant := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/collaborators", `{"user_id":"`+contributor.User.ID+`"}`, owner.Credential.Token, http.StatusCreated)
	var collaborator repositories.Collaborator
	if err := json.NewDecoder(grant.Body).Decode(&collaborator); err != nil {
		t.Fatal(err)
	}
	grant.Body.Close()
	if collaborator.UserID != contributor.User.ID || collaborator.Role != repositories.Contributor {
		t.Fatalf("collaborator = %#v", collaborator)
	}
	authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repository.ID, "", contributor.Credential.Token, http.StatusOK).Body.Close()
	discovery := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories", "", contributor.Credential.Token, http.StatusOK)
	var discovered struct {
		Repositories []repositories.Repository `json:"repositories"`
	}
	if err := json.NewDecoder(discovery.Body).Decode(&discovered); err != nil {
		t.Fatal(err)
	}
	discovery.Body.Close()
	if len(discovered.Repositories) != 1 || discovered.Repositories[0] != repository {
		t.Fatalf("collaborator repository discovery = %#v", discovered.Repositories)
	}
	authenticatedRequest(t, http.MethodPatch, server.URL+"/repositories/"+repository.ID, `{"visibility":"public"}`, contributor.Credential.Token, http.StatusNotFound).Body.Close()
	authenticatedRequest(t, http.MethodDelete, server.URL+"/repositories/"+repository.ID, "", contributor.Credential.Token, http.StatusNotFound).Body.Close()

	ownerGit, _ := credentials.Issue(owner.User.ID, auth.Git, "owner", []string{"git:read", "git:write"}, time.Hour)
	contributorGit, _ := credentials.Issue(contributor.User.ID, auth.Git, "contributor", []string{"git:read", "git:write"}, time.Hour)
	remoteWith := func(token string) string {
		parsed, _ := url.Parse(server.URL + repository.GitRemote)
		parsed.User = url.UserPassword("git", token)
		return parsed.String()
	}
	ownerCopy := filepath.Join(t.TempDir(), "owner")
	gitCommand(t, "", "clone", remoteWith(ownerGit.Token), ownerCopy)
	gitCommand(t, ownerCopy, "config", "user.name", "Owner")
	gitCommand(t, ownerCopy, "config", "user.email", "owner@example.com")
	if err := os.WriteFile(filepath.Join(ownerCopy, "README.md"), []byte("maintained\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, ownerCopy, "add", "README.md")
	gitCommand(t, ownerCopy, "commit", "-m", "initial")
	gitCommand(t, ownerCopy, "push", "origin", "main")

	contributorCopy := filepath.Join(t.TempDir(), "contributor")
	gitCommand(t, "", "clone", remoteWith(contributorGit.Token), contributorCopy)
	gitCommand(t, contributorCopy, "config", "user.name", "Contributor")
	gitCommand(t, contributorCopy, "config", "user.email", "contributor@example.com")
	gitCommand(t, contributorCopy, "switch", "-c", "candidate/contributor")
	if err := os.WriteFile(filepath.Join(contributorCopy, "candidate.txt"), []byte("proposal\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, contributorCopy, "add", "candidate.txt")
	gitCommand(t, contributorCopy, "commit", "-m", "candidate work")
	gitCommand(t, contributorCopy, "push", "origin", "candidate/contributor")
	gitCommandFails(t, contributorCopy, "push", "origin", "HEAD:main")

	authenticatedRequest(t, http.MethodDelete, server.URL+"/repositories/"+repository.ID+"/collaborators/"+contributor.User.ID, "", owner.Credential.Token, http.StatusNoContent).Body.Close()
	authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repository.ID, "", contributor.Credential.Token, http.StatusNotFound).Body.Close()
	if output, err := execGit(remoteWith(contributorGit.Token), "ls-remote").CombinedOutput(); err == nil {
		t.Fatalf("revoked contributor retained Git read: %s", output)
	}
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
