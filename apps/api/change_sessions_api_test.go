package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
	baseTree := writeTestTree(t, gitRepository)
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
	authenticatedRequest(t, http.MethodGet, reconnectBase, "", outsider.Credential.Token, http.StatusNotFound).Body.Close()
}
