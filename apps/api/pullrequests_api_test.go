package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestContributorOpensPullRequestWithExactBranchState(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	proposalRoot := t.TempDir()
	proposalStore, _ := proposals.New(proposalRoot)
	pullRequestRoot := t.TempDir()
	pullRequestStore, _ := pullrequests.New(pullRequestRoot, gitStore)
	server := httptest.NewServer(newPlatformHandler(gitStore, identities, credentials, catalog, proposalStore, pullRequestStore))
	defer server.Close()

	owner := createTestAccount(t, server.URL, "pull-owner")
	contributor := createTestAccount(t, server.URL, "pull-contributor")
	outsider := createTestAccount(t, server.URL, "pull-outsider")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"reviewable"}`, owner.Credential.Token, http.StatusCreated)
	var repository repositories.Repository
	if err := json.NewDecoder(response.Body).Decode(&repository); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/collaborators", `{"user_id":"`+contributor.User.ID+`"}`, owner.Credential.Token, http.StatusCreated).Body.Close()

	gitRepository, err := gitStore.Open(repository.ID)
	if err != nil {
		t.Fatal(err)
	}
	tree := writeTestTree(t, gitRepository)
	base := writeTestCommit(t, gitRepository, tree, nil, 1700000000, "base")
	head := writeTestCommit(t, gitRepository, tree, []storage.ObjectID{base}, 1700000001, "candidate")
	if err := gitRepository.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(base)}); err != nil {
		t.Fatal(err)
	}
	if err := gitRepository.CreateReference(storage.Reference{Name: "refs/heads/feature", Target: string(head)}); err != nil {
		t.Fatal(err)
	}

	proposalResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/proposals", `{"title":"Plan feature","body":"Agree on the behavior."}`, contributor.Credential.Token, http.StatusCreated)
	var proposal proposals.Proposal
	if err := json.NewDecoder(proposalResponse.Body).Decode(&proposal); err != nil {
		t.Fatal(err)
	}
	proposalResponse.Body.Close()
	body := `{"title":"Add the feature","body":"Implements the agreed behavior.","source_branch":"feature","target_branch":"main","proposal_id":"` + proposal.ID + `"}`
	created := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/pulls", body, contributor.Credential.Token, http.StatusCreated)
	var pullRequest pullrequests.PullRequest
	if err := json.NewDecoder(created.Body).Decode(&pullRequest); err != nil {
		t.Fatal(err)
	}
	created.Body.Close()
	if pullRequest.AuthorID != contributor.User.ID || pullRequest.SourceCommitID != string(head) || pullRequest.TargetCommitID != string(base) || pullRequest.Status != pullrequests.Open || pullRequest.ProposalID == nil || *pullRequest.ProposalID != proposal.ID {
		t.Fatalf("pull request = %#v", pullRequest)
	}
	if got := created.Header.Get("Location"); got != "/repositories/"+repository.ID+"/pulls/"+pullRequest.ID {
		t.Fatalf("Location = %q", got)
	}

	advanced := writeTestCommit(t, gitRepository, tree, []storage.ObjectID{head}, 1700000002, "more work")
	if err := gitRepository.UpdateReference(storage.Reference{Name: "refs/heads/feature", Target: string(advanced)}); err != nil {
		t.Fatal(err)
	}
	inspected := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repository.ID+"/pulls/"+pullRequest.ID, "", owner.Credential.Token, http.StatusOK)
	var persisted pullrequests.PullRequest
	if err := json.NewDecoder(inspected.Body).Decode(&persisted); err != nil {
		t.Fatal(err)
	}
	inspected.Body.Close()
	if persisted.SourceCommitID != string(head) {
		t.Fatalf("source commit moved to %s, want creation state %s", persisted.SourceCommitID, head)
	}

	authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repository.ID+"/pulls", "", contributor.Credential.Token, http.StatusOK).Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/pulls", body, outsider.Credential.Token, http.StatusNotFound).Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/pulls", `{"title":"Bad","body":"","source_branch":"missing","target_branch":"main"}`, contributor.Credential.Token, http.StatusBadRequest).Body.Close()

	objectPath := filepath.Join(gitRepository.Path(), "objects", string(advanced)[:2], string(advanced)[2:])
	if err := os.Remove(objectPath); err != nil {
		t.Fatal(err)
	}
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/pulls", `{"title":"Unavailable branch","body":"","source_branch":"feature","target_branch":"main"}`, contributor.Credential.Token, http.StatusInternalServerError).Body.Close()
	if err := os.WriteFile(filepath.Join(proposalRoot, proposal.ID+".json"), []byte("not json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/pulls", body, contributor.Credential.Token, http.StatusInternalServerError).Body.Close()
	authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repository.ID+"/pulls/00000000000000000000000000000000", "", owner.Credential.Token, http.StatusNotFound).Body.Close()
	if err := os.WriteFile(filepath.Join(pullRequestRoot, repository.ID, pullRequest.ID+".json"), []byte("not json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repository.ID+"/pulls/"+pullRequest.ID, "", owner.Credential.Token, http.StatusInternalServerError).Body.Close()
}
