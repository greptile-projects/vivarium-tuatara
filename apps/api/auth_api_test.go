package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

type accountResponse struct {
	User       users.User            `json:"user"`
	Credential auth.IssuedCredential `json:"credential"`
}

func TestAuthenticatedCredentialLifecycle(t *testing.T) {
	repositories, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	server := httptest.NewServer(newAuthenticatedAppHandler(repositories, identities, credentials))
	defer server.Close()
	response := requestStatus(t, http.MethodPost, server.URL+"/users", `{"handle":"octo","display_name":"Octo"}`, http.StatusCreated)
	var account accountResponse
	if err := json.NewDecoder(response.Body).Decode(&account); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if account.Credential.Token == "" || account.Credential.Kind != auth.Session {
		t.Fatalf("account = %#v", account)
	}

	requestStatus(t, http.MethodPatch, server.URL+"/users/"+account.User.ID, `{"display_name":"No auth"}`, http.StatusUnauthorized)
	created := authenticatedRequest(t, http.MethodPost, server.URL+"/auth/credentials", `{"kind":"git","name":"laptop clone","scopes":["git:read"],"expires_in":3600}`, account.Credential.Token, http.StatusCreated)
	var gitCredential auth.IssuedCredential
	if err := json.NewDecoder(created.Body).Decode(&gitCredential); err != nil {
		t.Fatal(err)
	}
	created.Body.Close()
	if gitCredential.Token == "" {
		t.Fatal("credential secret missing from creation response")
	}
	listed := authenticatedRequest(t, http.MethodGet, server.URL+"/auth/credentials", "", account.Credential.Token, http.StatusOK)
	listedData, _ := io.ReadAll(listed.Body)
	listed.Body.Close()
	if strings.Contains(string(listedData), "hash") || strings.Contains(string(listedData), gitCredential.Token) {
		t.Fatalf("credential inspection leaked secret material: %s", listedData)
	}
	var body map[string][]auth.Credential
	if err := json.Unmarshal(listedData, &body); err != nil {
		t.Fatal(err)
	}
	if len(body["credentials"]) != 2 {
		t.Fatalf("credentials = %#v", body)
	}
	authenticatedRequest(t, http.MethodDelete, server.URL+"/auth/credentials/"+gitCredential.ID, "", account.Credential.Token, http.StatusNoContent).Body.Close()
	if _, err := credentials.Authenticate(gitCredential.Token, "git:read"); err == nil {
		t.Fatal("revoked credential authenticated")
	}
}

func TestStockGitAuthenticatesWithScopedCredential(t *testing.T) {
	repositories, _ := storage.New(t.TempDir())
	repo, err := repositories.Create("private")
	if err != nil {
		t.Fatal(err)
	}
	identities, _ := users.New(t.TempDir())
	user, err := identities.Create("git-user", "Git User")
	if err != nil {
		t.Fatal(err)
	}
	credentials, _ := auth.New(t.TempDir())
	issued, err := credentials.Issue(user.ID, auth.Git, "stock git", []string{"git:read"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(newAuthenticatedAppHandler(repositories, identities, credentials))
	defer server.Close()

	command := execGit(server.URL+"/git/"+repo.ID()+".git", "ls-remote")
	if err := command.Run(); err == nil {
		t.Fatal("unauthenticated git ls-remote succeeded")
	}
	parsed, _ := url.Parse(server.URL)
	parsed.User = url.UserPassword("git", issued.Token)
	parsed.Path = "/git/" + repo.ID() + ".git"
	command = execGit(parsed.String(), "ls-remote")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("authenticated git ls-remote: %v\n%s", err, output)
	}
}

func execGit(remote, operation string) *exec.Cmd {
	command := exec.Command("git", "-c", "credential.helper=", operation, remote)
	command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	return command
}

func authenticatedRequest(t *testing.T, method, url, body, token string, status int) *http.Response {
	t.Helper()
	request, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != status {
		response.Body.Close()
		t.Fatalf("%s %s status = %d, want %d", method, url, response.StatusCode, status)
	}
	return response
}
