package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestHumanTaskContributionLinksReviewAndDoesNotCompleteBeforeMerge(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	proposalStore, _ := proposals.New(t.TempDir())
	pullStore, _ := pullrequests.New(t.TempDir(), gitStore)
	server := httptest.NewServer(newPlatformHandler(gitStore, identities, credentials, catalog, proposalStore, pullStore, nil))
	defer server.Close()

	owner := createTestAccount(t, server.URL, "task-publisher")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"connected-work"}`, owner.Credential.Token, http.StatusCreated)
	var repository repositories.Repository
	json.NewDecoder(response.Body).Decode(&repository)
	response.Body.Close()
	gitRepository, _ := gitStore.Open(repository.ID)
	base := writeCommit(t, gitRepository, 1700000000, "base")
	baseCommit, _ := gitRepository.ReadCommit(base)
	head := writeTestCommit(t, gitRepository, baseCommit.Tree, []storage.ObjectID{base}, 1700000001, "candidate")
	if err := gitRepository.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(base)}); err != nil {
		t.Fatal(err)
	}
	if err := gitRepository.CreateReference(storage.Reference{Name: "refs/heads/task", Target: string(head)}); err != nil {
		t.Fatal(err)
	}

	proposalResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/proposals", `{"title":"Connected intent","body":"Keep review traceable."}`, owner.Credential.Token, http.StatusCreated)
	var proposal proposals.Proposal
	json.NewDecoder(proposalResponse.Body).Decode(&proposal)
	proposalResponse.Body.Close()
	baseURL := server.URL + "/repositories/" + repository.ID + "/proposals/" + proposal.ID
	taskResponse := authenticatedRequest(t, http.MethodPost, baseURL+"/tasks", `{"title":"Publish candidate","outcome":"Review exact work"}`, owner.Credential.Token, http.StatusCreated)
	var task proposals.Task
	json.NewDecoder(taskResponse.Body).Decode(&task)
	taskResponse.Body.Close()
	authenticatedRequest(t, http.MethodPut, baseURL+"/tasks/"+task.ID+"/assignment", `{"assignee_type":"human","assignee_id":"`+owner.User.ID+`","mandate":"Publish it","repository_id":"`+repository.ID+`","base_revision":"`+string(base)+`"}`, owner.Credential.Token, http.StatusOK).Body.Close()

	createdResponse := authenticatedRequest(t, http.MethodPost, baseURL+"/tasks/"+task.ID+"/contributions", `{"title":"Connected candidate","body":"Carries task intent.","source_branch":"task","target_branch":"main"}`, owner.Credential.Token, http.StatusCreated)
	var pull pullrequests.PullRequest
	json.NewDecoder(createdResponse.Body).Decode(&pull)
	createdResponse.Body.Close()
	if pull.TaskID == nil || *pull.TaskID != task.ID || pull.ProposalID == nil || *pull.ProposalID != proposal.ID || pull.SourceCommitID != string(head) {
		t.Fatalf("pull = %#v", pull)
	}
	linked, _ := proposalStore.GetTask(repository.ID, proposal.ID, task.ID)
	if linked.Status != proposals.TaskInProgress || linked.Contribution == nil || linked.Contribution.Status != "review" || linked.Contribution.PullRequestID != pull.ID {
		t.Fatalf("linked task = %#v", linked)
	}

	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/pulls/"+pull.ID+"/close", `{}`, owner.Credential.Token, http.StatusOK).Body.Close()
	closed, _ := proposalStore.GetTask(repository.ID, proposal.ID, task.ID)
	if closed.Status == proposals.TaskCompleted || closed.Contribution == nil || closed.Contribution.Status != "closed" {
		t.Fatalf("closed task = %#v", closed)
	}
}
