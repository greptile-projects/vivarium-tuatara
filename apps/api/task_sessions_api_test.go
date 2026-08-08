package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/changesessions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestAgentAssignedTaskStartsIsolatedObservableSession(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	plans, _ := proposals.New(t.TempDir())
	sessions, _ := changesessions.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandler(gitStore, identities, credentials, catalog, plans, nil, nil, sessions))
	defer server.Close()

	owner := createTestAccount(t, server.URL, "task-session-owner")
	created := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"planned-agent-work"}`, owner.Credential.Token, http.StatusCreated)
	var repository repositories.Repository
	if err := json.NewDecoder(created.Body).Decode(&repository); err != nil {
		t.Fatal(err)
	}
	created.Body.Close()
	gitRepository, _ := gitStore.Open(repository.ID)
	base := writeCommit(t, gitRepository, 1700000000, "planned base")
	if err := gitRepository.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(base)}); err != nil {
		t.Fatal(err)
	}

	proposalResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/proposals", `{"title":"Automate setup","body":"Keep the setup path reproducible."}`, owner.Credential.Token, http.StatusCreated)
	var proposal proposals.Proposal
	if err := json.NewDecoder(proposalResponse.Body).Decode(&proposal); err != nil {
		t.Fatal(err)
	}
	proposalResponse.Body.Close()
	proposalBase := server.URL + "/repositories/" + repository.ID + "/proposals/" + proposal.ID
	commentResponse := authenticatedRequest(t, http.MethodPost, proposalBase+"/comments", `{"body":"Preserve the clean-base constraint."}`, owner.Credential.Token, http.StatusCreated)
	var comment proposals.Comment
	if err := json.NewDecoder(commentResponse.Body).Decode(&comment); err != nil {
		t.Fatal(err)
	}
	commentResponse.Body.Close()
	dependencyResponse := authenticatedRequest(t, http.MethodPost, proposalBase+"/tasks", `{"title":"Agree contract","outcome":"The contract is fixed"}`, owner.Credential.Token, http.StatusCreated)
	var dependency proposals.Task
	if err := json.NewDecoder(dependencyResponse.Body).Decode(&dependency); err != nil {
		t.Fatal(err)
	}
	dependencyResponse.Body.Close()
	authenticatedRequest(t, http.MethodPatch, proposalBase+"/tasks/"+dependency.ID, `{"status":"completed"}`, owner.Credential.Token, http.StatusOK).Body.Close()
	taskResponse := authenticatedRequest(t, http.MethodPost, proposalBase+"/tasks", `{"title":"Implement setup","outcome":"Setup works from a clean clone","dependency_ids":["`+dependency.ID+`"],"discussion_comment_ids":["`+comment.ID+`"]}`, owner.Credential.Token, http.StatusCreated)
	var task proposals.Task
	if err := json.NewDecoder(taskResponse.Body).Decode(&task); err != nil {
		t.Fatal(err)
	}
	taskResponse.Body.Close()
	assignmentResponse := authenticatedRequest(t, http.MethodPut, proposalBase+"/tasks/"+task.ID+"/assignment", `{"assignee_type":"agent","mandate":"Implement and report the setup path.","repository_id":"`+repository.ID+`","base_revision":"`+string(base)+`"}`, owner.Credential.Token, http.StatusOK)
	if err := json.NewDecoder(assignmentResponse.Body).Decode(&task); err != nil {
		t.Fatal(err)
	}
	assignmentResponse.Body.Close()

	sessionBase := proposalBase + "/tasks/" + task.ID + "/sessions"
	start := authenticatedRequest(t, http.MethodPost, sessionBase, `{"expected_assignment_id":"`+task.Assignment.ID+`","context_paths":[]}`, owner.Credential.Token, http.StatusCreated)
	var launched struct {
		Session    changesessions.Session `json:"session"`
		Run        changesessions.Run     `json:"run"`
		Credential auth.IssuedCredential  `json:"credential"`
	}
	if err := json.NewDecoder(start.Body).Decode(&launched); err != nil {
		t.Fatal(err)
	}
	start.Body.Close()
	if launched.Session.TaskContext == nil || launched.Session.TaskContext.ProposalTitle != proposal.Title || len(launched.Session.TaskContext.Dependencies) != 1 || len(launched.Session.TaskContext.Discussion) != 1 || launched.Run.AgentID != task.Assignment.AssigneeID || launched.Run.WorkingBranch == "" || launched.Credential.Token == "" {
		t.Fatalf("start response = %+v", launched)
	}
	branch, err := gitRepository.ReadReference("refs/heads/" + launched.Run.WorkingBranch)
	if err != nil || branch.Target != string(base) {
		t.Fatalf("isolated branch = %+v, %v", branch, err)
	}
	authenticatedRequest(t, http.MethodPost, sessionBase, `{"expected_assignment_id":"`+task.Assignment.ID+`"}`, owner.Credential.Token, http.StatusConflict).Body.Close()

	detail := sessionBase + "/" + launched.Session.ID
	authenticatedRequest(t, http.MethodGet, detail, "", owner.Credential.Token, http.StatusOK).Body.Close()
	authenticatedRequest(t, http.MethodPost, detail+"/runs/"+launched.Run.ID+"/interventions", `{"kind":"run.guidance","message":"Keep the setup deterministic."}`, owner.Credential.Token, http.StatusCreated).Body.Close()
	authenticatedRequest(t, http.MethodPost, detail+"/runs/"+launched.Run.ID+"/interventions", `{"kind":"run.paused"}`, owner.Credential.Token, http.StatusCreated).Body.Close()
	authenticatedRequest(t, http.MethodPost, detail+"/runs/"+launched.Run.ID+"/events", `{"kind":"agent.message","state":"working","message":"still working"}`, launched.Credential.Token, http.StatusConflict).Body.Close()
	authenticatedRequest(t, http.MethodPost, detail+"/runs/"+launched.Run.ID+"/interventions", `{"kind":"run.resumed"}`, owner.Credential.Token, http.StatusCreated).Body.Close()
	authenticatedRequest(t, http.MethodGet, detail+"/runs/"+launched.Run.ID+"/control", "", launched.Credential.Token, http.StatusOK).Body.Close()
	baseCommit, err := gitRepository.ReadCommit(base)
	if err != nil {
		t.Fatal(err)
	}
	completedTip := writeTestCommit(t, gitRepository, baseCommit.Tree, []storage.ObjectID{base}, 1700000001, "completed task work")
	if err := gitRepository.UpdateReference(storage.Reference{Name: "refs/heads/" + launched.Run.WorkingBranch, Target: string(completedTip)}); err != nil {
		t.Fatal(err)
	}
	authenticatedRequest(t, http.MethodPost, detail+"/runs/"+launched.Run.ID+"/completion", `{"summary":"Completed valid task work.","commit_id":"`+string(completedTip)+`","checks":[],"unresolved_concerns":[]}`, launched.Credential.Token, http.StatusInternalServerError).Body.Close()
	authenticatedRequest(t, http.MethodPost, detail+"/runs/"+launched.Run.ID+"/interventions", `{"kind":"run.canceled"}`, owner.Credential.Token, http.StatusCreated).Body.Close()
	authenticatedRequest(t, http.MethodPost, detail+"/runs/"+launched.Run.ID+"/events", `{"kind":"agent.message","state":"working","message":"late"}`, launched.Credential.Token, http.StatusUnauthorized).Body.Close()
	eventsResponse := authenticatedRequest(t, http.MethodGet, detail+"/events", "", owner.Credential.Token, http.StatusOK)
	var timeline struct {
		Events []changesessions.Event `json:"events"`
	}
	if err := json.NewDecoder(eventsResponse.Body).Decode(&timeline); err != nil {
		t.Fatal(err)
	}
	eventsResponse.Body.Close()
	if len(timeline.Events) != 6 || timeline.Events[0].Kind != "session.opened" || timeline.Events[5].Kind != "run.canceled" {
		t.Fatalf("timeline = %+v", timeline.Events)
	}
}
