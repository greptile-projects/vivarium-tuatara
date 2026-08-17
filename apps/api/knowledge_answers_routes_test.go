package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/knowledgeanswers"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/packages"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/supportthreads"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func knowledgeRouteTestServer(t *testing.T) (*httptest.Server, *auth.Store, *knowledgeanswers.Store) {
	t.Helper()
	git, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), git)
	answers, _ := knowledgeanswers.New(t.TempDir())
	support, _ := supportthreads.New(t.TempDir())
	issueStore, _ := issues.New(t.TempDir())
	releaseStore, _ := releases.New(t.TempDir())
	packageStore, _ := packages.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(git, identities, credentials, catalog, nil, nil, nil, nil, nil, releaseStore, packageStore, issueStore, support, answers))
	t.Cleanup(server.Close)
	return server, credentials, answers
}

func routeKnowledgeRevision(actor string) knowledgeanswers.Revision {
	return knowledgeanswers.Revision{Summary: "Supported setup", Body: "Use the documented setup.", AuthorID: actor, AuthorType: "human", Claims: []knowledgeanswers.Claim{{Text: "The setup applies to 2.x.", Confidence: "high", Citations: []knowledgeanswers.Citation{{Kind: "documentation", Revision: "0123456789012345678901234567890123456789", Path: "README.md", Label: "setup guide", ApplicableVersions: []string{"2.x"}}}}}}
}

func TestKnowledgeRoutesRejectCrossRepositoryAnswerTraversal(t *testing.T) {
	server, _, answers := knowledgeRouteTestServer(t)
	ownerA := createTestAccount(t, server.URL, "knowledge-owner-a")
	ownerB := createTestAccount(t, server.URL, "knowledge-owner-b")
	var repoA, repoB repositories.Repository
	decodeResponse(t, authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"knowledge-a"}`, ownerA.Credential.Token, http.StatusCreated), &repoA)
	decodeResponse(t, authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"knowledge-b"}`, ownerB.Credential.Token, http.StatusCreated), &repoB)
	private, err := answers.Create(knowledgeanswers.Answer{RepositoryID: repoB.ID, Question: "Private guidance?", Audience: "participants"}, routeKnowledgeRevision(ownerB.User.ID))
	if err != nil {
		t.Fatal(err)
	}
	escaped := url.PathEscape("../" + repoB.ID + "/" + private.ID)
	authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repoA.ID+"/knowledge-answers/"+escaped, "", ownerA.Credential.Token, http.StatusNotFound).Body.Close()
	authenticatedRequest(t, http.MethodPatch, server.URL+"/repositories/"+repoA.ID+"/knowledge-answers/"+escaped, `{"expected_version":1,"status":"verified"}`, ownerA.Credential.Token, http.StatusNotFound).Body.Close()
	unchanged, err := answers.Get(repoB.ID, private.ID)
	if err != nil || unchanged.Status != "proposed" {
		t.Fatalf("private answer changed: %#v, %v", unchanged, err)
	}
}

func TestKnowledgeRoutesRejectOwnerLinkedAgentHumanDecisions(t *testing.T) {
	server, credentials, answers := knowledgeRouteTestServer(t)
	owner := createTestAccount(t, server.URL, "knowledge-agent-owner")
	var repo repositories.Repository
	decodeResponse(t, authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"knowledge-agent"}`, owner.Credential.Token, http.StatusCreated), &repo)
	answer, err := answers.Create(knowledgeanswers.Answer{RepositoryID: repo.ID, Question: "Can this be verified?", Audience: "participants"}, routeKnowledgeRevision(owner.User.ID))
	if err != nil {
		t.Fatal(err)
	}
	agent, err := credentials.IssueOrganizationAgent(owner.User.ID, "review agent", "11111111111111111111111111111111", "22222222222222222222222222222222", "33333333333333333333333333333333", repo.ID, []string{"repositories:read"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	responseBody := `{"expected_version":1,"revision_id":"` + answer.CurrentRevisionID + `","kind":"endorsement","body":"Approved."}`
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/knowledge-answers/"+answer.ID+"/responses", responseBody, agent.Token, http.StatusForbidden).Body.Close()
	authenticatedRequest(t, http.MethodPatch, server.URL+"/repositories/"+repo.ID+"/knowledge-answers/"+answer.ID, `{"expected_version":1,"status":"verified"}`, agent.Token, http.StatusForbidden).Body.Close()
	unchanged, err := answers.Get(repo.ID, answer.ID)
	if err != nil || unchanged.Status != "proposed" || len(unchanged.Responses) != 0 || unchanged.Version != 1 {
		t.Fatalf("agent mutated human decisions: %#v, %v", unchanged, err)
	}
}
