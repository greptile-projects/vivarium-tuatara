package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestCollaboratorDefinesAndInspectsExactReleaseCandidate(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	releaseStore, _ := releases.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, releaseStore))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "release-owner")
	collaborator := createTestAccount(t, server.URL, "release-collaborator")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"ship"}`, owner.Credential.Token, http.StatusCreated)
	var repository repositories.Repository
	decodeResponse(t, response, &repository)
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/collaborators", `{"user_id":"`+collaborator.User.ID+`"}`, owner.Credential.Token, http.StatusCreated).Body.Close()
	gitRepository, _ := gitStore.Open(repository.ID)
	tree := writeTestTree(t, gitRepository)
	commit := writeTestCommit(t, gitRepository, tree, nil, 1700000000, "release state")
	createdResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/releases", `{"version":"v1.0.0","notes":"First exact delivery.","commit_id":"`+string(commit)+`"}`, collaborator.Credential.Token, http.StatusCreated)
	var created releases.Candidate
	if err := json.NewDecoder(createdResponse.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	createdResponse.Body.Close()
	if created.CommitID != string(commit) || created.CreatedBy != collaborator.User.ID || created.Status != "candidate" || created.Inclusions.PullRequestIDs == nil {
		t.Fatalf("created = %#v", created)
	}
	if createdResponse.Header.Get("Location") != "/repositories/"+repository.ID+"/releases/"+created.ID {
		t.Fatalf("Location = %q", createdResponse.Header.Get("Location"))
	}
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/releases", `{"version":"v1.0.0","notes":"duplicate","commit_id":"`+string(commit)+`"}`, owner.Credential.Token, http.StatusConflict).Body.Close()
	listResponse := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repository.ID+"/releases", "", collaborator.Credential.Token, http.StatusOK)
	var listed struct {
		Releases []releases.Candidate `json:"releases"`
	}
	decodeResponse(t, listResponse, &listed)
	if len(listed.Releases) != 1 || listed.Releases[0].ID != created.ID {
		t.Fatalf("listed = %#v", listed)
	}
	authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repository.ID+"/releases/"+created.ID, "", collaborator.Credential.Token, http.StatusOK).Body.Close()
}
