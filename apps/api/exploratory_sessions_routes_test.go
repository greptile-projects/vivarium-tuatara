package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

func TestExploratoryEventAuthorizationRequiresTaskCredentialForAgents(t *testing.T) {
	git, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := repositories.New(t.TempDir(), git)
	if err != nil {
		t.Fatal(err)
	}
	credentials, err := auth.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ownerID := strings.Repeat("1", 32)
	repository, err := catalog.Create(ownerID, "exploratory-auth")
	if err != nil {
		t.Fatal(err)
	}
	agentID := strings.Repeat("2", 32)
	generalAgent, err := credentials.IssueOrganizationAgent(ownerID, "general API agent", strings.Repeat("3", 32), strings.Repeat("4", 32), agentID, repository.ID, []string{"repositories:read", "repositories:write"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	taskAgent, err := credentials.IssueTaskAgentBound(ownerID, "task explorer", agentID, []string{"git:read", "git:write"}, time.Hour, repository.ID, "refs/heads/agent/explore")
	if err != nil {
		t.Fatal(err)
	}
	human, err := credentials.Issue(ownerID, auth.API, "human API", []string{"repositories:write"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	request := func(token string) (*httptest.ResponseRecorder, auth.Credential, bool) {
		r := httptest.NewRequest(http.MethodPost, "/repositories/"+repository.ID+"/exploratory-sessions/session/events", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		actor, ok := authorizeExploratoryEvent(w, r, catalog, credentials, repository.ID)
		return w, actor, ok
	}

	denied, _, ok := request(generalAgent.Token)
	if ok || denied.Code != http.StatusForbidden {
		t.Fatalf("general API agent authorization = %d, %v", denied.Code, ok)
	}
	allowedTask, taskActor, ok := request(taskAgent.Token)
	if !ok || allowedTask.Code != http.StatusOK || taskActor.AgentID != agentID {
		t.Fatalf("task agent authorization = %d, %#v, %v", allowedTask.Code, taskActor, ok)
	}
	allowedHuman, humanActor, ok := request(human.Token)
	if !ok || allowedHuman.Code != http.StatusOK || humanActor.UserID != ownerID || humanActor.AgentID != "" {
		t.Fatalf("human authorization = %d, %#v, %v", allowedHuman.Code, humanActor, ok)
	}
}
