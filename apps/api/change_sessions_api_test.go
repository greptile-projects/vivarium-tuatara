package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/changesessions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestCollaboratorOpensAndReconnectsToChangeSession(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	pulls, _ := pullrequests.New(t.TempDir(), gitStore)
	sessionRoot := t.TempDir()
	changeSessions, _ := changesessions.New(sessionRoot)
	server := httptest.NewServer(newPlatformHandler(gitStore, identities, credentials, catalog, nil, pulls, nil, changeSessions))
	defer server.Close()

	owner := createTestAccount(t, server.URL, "session-owner")
	contributor := createTestAccount(t, server.URL, "session-contributor")
	outsider := createTestAccount(t, server.URL, "session-outsider")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"agent-work"}`, owner.Credential.Token, http.StatusCreated)
	var repository repositories.Repository
	if err := json.NewDecoder(response.Body).Decode(&repository); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/collaborators", `{"user_id":"`+contributor.User.ID+`"}`, owner.Credential.Token, http.StatusCreated).Body.Close()

	gitRepository, _ := gitStore.Open(repository.ID)
	readme, err := gitRepository.WriteObject(storage.BlobObject, []byte("agent context\n"))
	if err != nil {
		t.Fatal(err)
	}
	baseTree := writeTestTree(t, gitRepository, testTreeEntry{mode: "100644", name: "README.md", id: readme})
	base := writeTestCommit(t, gitRepository, baseTree, nil, 1700000000, "base")
	head := writeTestCommit(t, gitRepository, baseTree, []storage.ObjectID{base}, 1700000001, "candidate")
	if err := gitRepository.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(base)}); err != nil {
		t.Fatal(err)
	}
	if err := gitRepository.CreateReference(storage.Reference{Name: "refs/heads/feature", Target: string(head)}); err != nil {
		t.Fatal(err)
	}
	pullResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/pulls", `{"title":"Agent-ready change","body":"Continue this work.","source_branch":"feature","target_branch":"main"}`, contributor.Credential.Token, http.StatusCreated)
	var pull pullrequests.PullRequest
	if err := json.NewDecoder(pullResponse.Body).Decode(&pull); err != nil {
		t.Fatal(err)
	}
	pullResponse.Body.Close()

	baseURL := server.URL + "/repositories/" + repository.ID + "/pulls/" + pull.ID + "/sessions"
	createdResponse := authenticatedRequest(t, http.MethodPost, baseURL, "", owner.Credential.Token, http.StatusCreated)
	var session changesessions.Session
	if err := json.NewDecoder(createdResponse.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	createdResponse.Body.Close()
	if session.InitiatorID != owner.User.ID || session.SourceCommitID != pull.SourceCommitID || session.State != changesessions.Open {
		t.Fatalf("session = %+v", session)
	}
	if createdResponse.Header.Get("Location") != "/repositories/"+repository.ID+"/pulls/"+pull.ID+"/sessions/"+session.ID {
		t.Fatalf("Location = %q", createdResponse.Header.Get("Location"))
	}

	// Reopen the durable store to prove the public reconnect path has no process-local dependency.
	reopened, err := changesessions.New(sessionRoot)
	if err != nil {
		t.Fatal(err)
	}
	reconnectedServer := httptest.NewServer(newPlatformHandler(gitStore, identities, credentials, catalog, nil, pulls, nil, reopened))
	defer reconnectedServer.Close()
	reconnectBase := reconnectedServer.URL + "/repositories/" + repository.ID + "/pulls/" + pull.ID + "/sessions/" + session.ID
	inspected := authenticatedRequest(t, http.MethodGet, reconnectBase, "", contributor.Credential.Token, http.StatusOK)
	var persisted changesessions.Session
	if err := json.NewDecoder(inspected.Body).Decode(&persisted); err != nil {
		t.Fatal(err)
	}
	inspected.Body.Close()
	if persisted.ID != session.ID {
		t.Fatalf("persisted = %+v", persisted)
	}
	timeline := authenticatedRequest(t, http.MethodGet, reconnectBase+"/events", "", contributor.Credential.Token, http.StatusOK)
	var eventPage struct {
		Events []changesessions.Event `json:"events"`
		Next   *string                `json:"next_cursor"`
	}
	if err := json.NewDecoder(timeline.Body).Decode(&eventPage); err != nil {
		t.Fatal(err)
	}
	timeline.Body.Close()
	if len(eventPage.Events) != 1 || eventPage.Events[0].Kind != "session.opened" || eventPage.Events[0].ActorID != owner.User.ID || eventPage.Next != nil {
		t.Fatalf("events = %+v", eventPage)
	}

	runResponse := authenticatedRequest(t, http.MethodPost, reconnectBase+"/runs", `{"instructions":"Add focused regression coverage.","source_commit_id":"`+pull.SourceCommitID+`","context_paths":["README.md"],"working_branch":"feature","expires_in":3600}`, contributor.Credential.Token, http.StatusCreated)
	var launched struct {
		Run        changesessions.Run    `json:"run"`
		Credential auth.IssuedCredential `json:"credential"`
	}
	if err := json.NewDecoder(runResponse.Body).Decode(&launched); err != nil {
		t.Fatal(err)
	}
	runResponse.Body.Close()
	if launched.Run.InitiatorID != contributor.User.ID || launched.Run.SourceCommitID != pull.SourceCommitID || launched.Run.WorkingBranch != "feature" || launched.Credential.Token == "" || launched.Credential.RepositoryID != repository.ID || launched.Credential.GitWriteBranch != "refs/heads/feature" {
		t.Fatalf("launched = %+v", launched)
	}
	if _, err := credentials.Authenticate(launched.Credential.Token, "git:write"); err != nil {
		t.Fatalf("agent credential: %v", err)
	}
	otherResponse := authenticatedRequest(t, http.MethodPost, reconnectedServer.URL+"/repositories", `{"name":"unrelated-agent-work"}`, contributor.Credential.Token, http.StatusCreated)
	var otherRepository repositories.Repository
	if err := json.NewDecoder(otherResponse.Body).Decode(&otherRepository); err != nil {
		t.Fatal(err)
	}
	otherResponse.Body.Close()
	assertGitDiscoveryStatus(t, reconnectedServer.URL+otherRepository.GitRemote+"/info/refs?service=git-receive-pack", launched.Credential.Token, http.StatusNotFound)
	remote, _ := url.Parse(reconnectedServer.URL + repository.GitRemote)
	remote.User = url.UserPassword("git", launched.Credential.Token)
	workingCopy := t.TempDir()
	gitCommand(t, "", "clone", remote.String(), workingCopy)
	gitCommand(t, workingCopy, "config", "user.name", "Agent")
	gitCommand(t, workingCopy, "config", "user.email", "agent@example.com")
	gitCommand(t, workingCopy, "switch", "-c", "agent-test", "origin/feature")
	gitCommand(t, workingCopy, "commit", "--allow-empty", "-m", "agent work")
	gitCommand(t, workingCopy, "push", "origin", "HEAD:refs/heads/feature")
	gitCommandFails(t, workingCopy, "push", "origin", "HEAD:refs/heads/agent/other")
	ordinary, err := credentials.Issue(contributor.User.ID, auth.Git, "ordinary collaborator", []string{"git:read", "git:write"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	ordinaryRemote, _ := url.Parse(reconnectedServer.URL + repository.GitRemote)
	ordinaryRemote.User = url.UserPassword("git", ordinary.Token)
	marker := filepath.Join(t.TempDir(), "header-executed")
	gitCommand(t, workingCopy, "commit", "--allow-empty", "-m", "ordinary work")
	gitCommand(t, workingCopy, "-c", "http.extraHeader=Vivarium-Git-Write-Branch: $(touch "+marker+")", "push", ordinaryRemote.String(), "HEAD:refs/heads/feature")
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("client branch header executed: %v", err)
	}
	runsResponse := authenticatedRequest(t, http.MethodGet, reconnectBase+"/runs", "", owner.Credential.Token, http.StatusOK)
	var runPage struct {
		Runs []changesessions.Run `json:"runs"`
	}
	if err := json.NewDecoder(runsResponse.Body).Decode(&runPage); err != nil {
		t.Fatal(err)
	}
	runsResponse.Body.Close()
	if len(runPage.Runs) != 1 || runPage.Runs[0].Instructions != "Add focused regression coverage." {
		t.Fatalf("runs = %+v", runPage.Runs)
	}
	timeline = authenticatedRequest(t, http.MethodGet, reconnectBase+"/events", "", contributor.Credential.Token, http.StatusOK)
	if err := json.NewDecoder(timeline.Body).Decode(&eventPage); err != nil {
		t.Fatal(err)
	}
	timeline.Body.Close()
	if len(eventPage.Events) != 2 || eventPage.Events[1].Kind != "run.launched" || eventPage.Events[1].RunID != launched.Run.ID || eventPage.Events[1].ActorID != contributor.User.ID {
		t.Fatalf("events after launch = %+v", eventPage.Events)
	}
	authenticatedRequest(t, http.MethodPost, reconnectBase+"/runs", `{"instructions":"Use unknown context.","source_commit_id":"`+pull.SourceCommitID+`","context_paths":["missing.txt"],"working_branch":"agent/invalid"}`, contributor.Credential.Token, http.StatusBadRequest).Body.Close()
	revokedResponse := authenticatedRequest(t, http.MethodDelete, reconnectBase+"/runs/"+launched.Run.ID+"/credential", "", owner.Credential.Token, http.StatusOK)
	var revoked changesessions.Run
	if err := json.NewDecoder(revokedResponse.Body).Decode(&revoked); err != nil {
		t.Fatal(err)
	}
	revokedResponse.Body.Close()
	if revoked.AccessRevokedAt == nil {
		t.Fatalf("revoked run = %+v", revoked)
	}
	if _, err := credentials.Authenticate(launched.Credential.Token, "git:read"); !errors.Is(err, auth.ErrNotFound) {
		t.Fatalf("revoked credential error = %v", err)
	}
	authenticatedRequest(t, http.MethodGet, reconnectBase, "", outsider.Credential.Token, http.StatusNotFound).Body.Close()
}
