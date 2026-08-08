package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestProposalLifecycleDiscussionAndAuthorization(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	proposalStore, _ := proposals.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandler(gitStore, identities, credentials, catalog, proposalStore, nil, nil))
	defer server.Close()

	owner := createTestAccount(t, server.URL, "proposal-owner")
	contributor := createTestAccount(t, server.URL, "proposal-contributor")
	outsider := createTestAccount(t, server.URL, "proposal-outsider")
	createdRepository := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"ideas"}`, owner.Credential.Token, http.StatusCreated)
	var repository repositories.Repository
	if err := json.NewDecoder(createdRepository.Body).Decode(&repository); err != nil {
		t.Fatal(err)
	}
	createdRepository.Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/collaborators", `{"user_id":"`+contributor.User.ID+`"}`, owner.Credential.Token, http.StatusCreated).Body.Close()

	created := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/proposals", `{"title":"Improve onboarding","body":"Newcomers need a path."}`, contributor.Credential.Token, http.StatusCreated)
	var proposal proposals.Proposal
	if err := json.NewDecoder(created.Body).Decode(&proposal); err != nil {
		t.Fatal(err)
	}
	created.Body.Close()
	if proposal.AuthorID != contributor.User.ID || proposal.Status != proposals.Open {
		t.Fatalf("proposal = %#v", proposal)
	}
	base := server.URL + "/repositories/" + repository.ID + "/proposals/" + proposal.ID
	authenticatedRequest(t, http.MethodPost, base+"/comments", `{"body":"Let us document the first step."}`, owner.Credential.Token, http.StatusCreated).Body.Close()
	comments := authenticatedRequest(t, http.MethodGet, base+"/comments", "", contributor.Credential.Token, http.StatusOK)
	var conversation struct {
		Comments []proposals.Comment `json:"comments"`
	}
	if err := json.NewDecoder(comments.Body).Decode(&conversation); err != nil {
		t.Fatal(err)
	}
	comments.Body.Close()
	if len(conversation.Comments) != 1 || conversation.Comments[0].AuthorID != owner.User.ID {
		t.Fatalf("conversation = %#v", conversation)
	}
	firstResponse := authenticatedRequest(t, http.MethodPost, base+"/tasks", `{"title":"Define the path","outcome":"The API contract is agreed","discussion_comment_ids":["`+conversation.Comments[0].ID+`"]}`, contributor.Credential.Token, http.StatusCreated)
	var firstTask proposals.Task
	if err := json.NewDecoder(firstResponse.Body).Decode(&firstTask); err != nil {
		t.Fatal(err)
	}
	firstResponse.Body.Close()
	secondResponse := authenticatedRequest(t, http.MethodPost, base+"/tasks", `{"title":"Build the path","outcome":"A contributor can follow it","dependency_ids":["`+firstTask.ID+`"]}`, owner.Credential.Token, http.StatusCreated)
	var secondTask proposals.Task
	if err := json.NewDecoder(secondResponse.Body).Decode(&secondTask); err != nil {
		t.Fatal(err)
	}
	secondResponse.Body.Close()
	if !firstTask.Ready || secondTask.Ready || len(secondTask.BlockedBy) != 1 {
		t.Fatalf("tasks = %#v, %#v", firstTask, secondTask)
	}
	authenticatedRequest(t, http.MethodPatch, base+"/tasks/"+firstTask.ID, `{"status":"completed"}`, owner.Credential.Token, http.StatusOK).Body.Close()
	planResponse := authenticatedRequest(t, http.MethodGet, base+"/tasks", "", contributor.Credential.Token, http.StatusOK)
	var plan struct {
		Tasks []proposals.Task `json:"tasks"`
	}
	if err := json.NewDecoder(planResponse.Body).Decode(&plan); err != nil {
		t.Fatal(err)
	}
	planResponse.Body.Close()
	if len(plan.Tasks) != 2 || !plan.Tasks[1].Ready {
		t.Fatalf("plan = %#v", plan)
	}
	historyResponse := authenticatedRequest(t, http.MethodGet, base+"/tasks/"+firstTask.ID+"/history", "", contributor.Credential.Token, http.StatusOK)
	var history struct {
		History []proposals.TaskChange `json:"history"`
	}
	if err := json.NewDecoder(historyResponse.Body).Decode(&history); err != nil {
		t.Fatal(err)
	}
	historyResponse.Body.Close()
	if len(history.History) != 2 || history.History[0].ActorID != contributor.User.ID || history.History[1].ActorID != owner.User.ID {
		t.Fatalf("history = %#v", history)
	}
	authenticatedRequest(t, http.MethodPost, base+"/tasks", `{"title":"No access","outcome":"Never"}`, outsider.Credential.Token, http.StatusNotFound).Body.Close()
	authenticatedRequest(t, http.MethodPatch, base, `{"title":"Improve newcomer onboarding"}`, contributor.Credential.Token, http.StatusOK).Body.Close()
	authenticatedRequest(t, http.MethodPatch, base, `{"body":"owner rewrite"}`, owner.Credential.Token, http.StatusNotFound).Body.Close()
	authenticatedRequest(t, http.MethodPatch, base, `{"status":"closed"}`, owner.Credential.Token, http.StatusOK).Body.Close()
	authenticatedRequest(t, http.MethodPatch, base, `{"body":"hijacked"}`, outsider.Credential.Token, http.StatusNotFound).Body.Close()
	authenticatedRequest(t, http.MethodPost, base+"/comments", `{"body":"uninvited"}`, outsider.Credential.Token, http.StatusNotFound).Body.Close()

	authenticatedRequest(t, http.MethodPatch, server.URL+"/repositories/"+repository.ID, `{"visibility":"public"}`, owner.Credential.Token, http.StatusOK).Body.Close()
	requestStatus(t, http.MethodGet, base, "", http.StatusOK).Body.Close()
	requestStatus(t, http.MethodGet, base+"/comments", "", http.StatusOK).Body.Close()
	requestStatus(t, http.MethodGet, base+"/tasks", "", http.StatusOK).Body.Close()
	requestStatus(t, http.MethodGet, base+"/tasks/"+firstTask.ID+"/history", "", http.StatusOK).Body.Close()
}

func TestUncertainProposalMutationPreservesResourceForClient(t *testing.T) {
	recorder := httptest.NewRecorder()
	resource := proposals.Comment{ID: "0123456789abcdef0123456789abcdef"}
	writeUncertainMutation(recorder, resource)
	if recorder.Code != http.StatusAccepted || recorder.Header().Get("Vivarium-Durability") != "uncertain" {
		t.Fatalf("response = %d, durability %q", recorder.Code, recorder.Header().Get("Vivarium-Durability"))
	}
	var got proposals.Comment
	if err := json.NewDecoder(recorder.Body).Decode(&got); err != nil || got.ID != resource.ID {
		t.Fatalf("resource = %#v, %v", got, err)
	}
}
