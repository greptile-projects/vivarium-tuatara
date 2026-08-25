package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/contributorpathways"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/learningpathways"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestCollaboratorPublishesVersionedLearningPathwayWithExplicitHealth(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	pathways, _ := learningpathways.New(t.TempDir())
	guidance, _ := contributorpathways.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, pathways, guidance))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "learning-owner")
	created := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"learnable"}`, owner.Credential.Token, http.StatusCreated)
	var repo repositories.Repository
	decodeResponse(t, created, &repo)
	authenticatedRequest(t, http.MethodPatch, server.URL+"/repositories/"+repo.ID, `{"visibility":"public"}`, owner.Credential.Token, http.StatusOK).Body.Close()
	revision := strings.Repeat("a", 40)
	body := `{"expected_version":0,"pathway":{"role":"API contributor","outcome":"Ship a reviewed endpoint safely","prerequisites":["Basic Go"],"objectives":["Trace a request to storage"],"supported_revisions":["` + revision + `"],"expected_minutes":120,"accessibility_needs":["Keyboard-only workflow"],"locales":["en-US"],"completion_evidence":["Passing focused test"],"mentors":[{"user_id":"` + owner.User.ID + `","responsibility":"API review"}],"environments":[{"name":"Linux","requirements":["Go 1.25"],"supported":true},{"name":"Windows","requirements":["WSL"],"supported":false}],"modules":[{"id":"request-boundary","title":"Request boundary","why_it_matters":"Authorization protects collaborators","objectives":["Explain repository reads"],"estimated_minutes":120,"exercises":[{"title":"Trace a request","instructions":"Follow one request from route to store.","completion_evidence":["Annotated trace"]}],"materials":[{"kind":"documentation","label":"API guide","path":"docs/api.md","revision":"` + revision + `","owner_id":"` + owner.User.ID + `"},{"kind":"api","label":"Learning route","path":"apps/api/main.go","revision":"` + revision + `"}]}]}}`
	response := authenticatedRequest(t, http.MethodPut, server.URL+"/repositories/"+repo.ID+"/learning-pathways/api-contributor", body, owner.Credential.Token, http.StatusCreated)
	var published learningpathways.Revision
	decodeResponse(t, response, &published)
	if published.Version != 1 || published.Modules[0].Materials[0].Status != "inaccessible" || published.Modules[0].Materials[1].Status != "missing_owner" || published.Environments[0].Status != "missing_owner" || published.Environments[1].Status != "unsupported" {
		t.Fatalf("projection = %#v", published)
	}
	public, e := http.Get(server.URL + "/repositories/" + repo.ID + "/learning-pathways/api-contributor")
	if e != nil {
		t.Fatal(e)
	}
	var projection struct {
		Pathway learningpathways.Revision   `json:"pathway"`
		History []learningpathways.Revision `json:"history"`
	}
	decodeResponse(t, public, &projection)
	if projection.Pathway.Outcome != "Ship a reviewed endpoint safely" || len(projection.History) != 1 {
		t.Fatalf("read = %#v", projection)
	}
	authenticatedRequest(t, http.MethodPut, server.URL+"/repositories/"+repo.ID+"/learning-pathways/api-contributor", body, owner.Credential.Token, http.StatusConflict).Body.Close()
}
