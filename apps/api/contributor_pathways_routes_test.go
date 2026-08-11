package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/contributorpathways"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestMaintainerPublishesPublicVersionedContributorPathwayAndNewcomerAcknowledges(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	pathways, _ := contributorpathways.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, pathways))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "pathway-owner")
	newcomer := createTestAccount(t, server.URL, "pathway-newcomer")
	created := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"welcoming"}`, owner.Credential.Token, http.StatusCreated)
	var repository repositories.Repository
	decodeResponse(t, created, &repository)
	authenticatedRequest(t, http.MethodPatch, server.URL+"/repositories/"+repository.ID, `{"visibility":"public"}`, owner.Credential.Token, http.StatusOK).Body.Close()
	body := `{"expected_version":0,"pathway":{"goals":"Build a welcoming collaboration platform.","prerequisites":["Know basic Git"],"conduct":"Be kind and specific.","security":"Report vulnerabilities privately.","setup":{"summary":"Use Bun and Go.","verification_commands":["bun run lint","go test ./..."]},"communication":"Ask in the linked issue before starting.","review_policy":"A maintainer reviews every change.","work_categories":[{"name":"Documentation","description":"Improve newcomer-facing guidance.","audience":"human_or_agent"}],"requirements":[{"kind":"ownership","label":"Current maintainers"}]}}`
	published := authenticatedRequest(t, http.MethodPut, server.URL+"/repositories/"+repository.ID+"/contributor-pathway", body, owner.Credential.Token, http.StatusCreated)
	var revision contributorpathways.Revision
	decodeResponse(t, published, &revision)
	if revision.Version != 1 || revision.PublishedBy != owner.User.ID || revision.Requirements[0].Status != "current" {
		t.Fatalf("revision = %#v", revision)
	}

	public, err := http.Get(server.URL + "/repositories/" + repository.ID + "/contributor-pathway")
	if err != nil {
		t.Fatal(err)
	}
	var projection struct {
		Pathway contributorpathways.Revision   `json:"pathway"`
		History []contributorpathways.Revision `json:"history"`
	}
	decodeResponse(t, public, &projection)
	if projection.Pathway.Version != 1 || len(projection.History) != 1 {
		t.Fatalf("projection = %#v", projection)
	}

	authenticatedRequest(t, http.MethodPut, server.URL+"/repositories/"+repository.ID+"/contributor-pathway", body, newcomer.Credential.Token, http.StatusNotFound).Body.Close()
	ack := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/contributor-pathway/acknowledgements", `{"version":1}`, newcomer.Credential.Token, http.StatusCreated)
	var acknowledgement contributorpathways.Acknowledgement
	decodeResponse(t, ack, &acknowledgement)
	if acknowledgement.ActorID != newcomer.User.ID || acknowledgement.Version != 1 {
		t.Fatalf("acknowledgement = %#v", acknowledgement)
	}
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/contributor-pathway/acknowledgements", `{"version":1}`, newcomer.Credential.Token, http.StatusConflict).Body.Close()

	secondBody := strings.Replace(body, `"expected_version":0`, `"expected_version":1`, 1)
	second := authenticatedRequest(t, http.MethodPut, server.URL+"/repositories/"+repository.ID+"/contributor-pathway", secondBody, owner.Credential.Token, http.StatusCreated)
	decodeResponse(t, second, &revision)
	if revision.Version != 2 {
		t.Fatalf("second revision = %#v", revision)
	}
}

func TestContributorPathwayRejectsStalePublicationVersion(t *testing.T) {
	store, _ := contributorpathways.New(t.TempDir())
	repositoryID, actorID := "0123456789abcdef0123456789abcdef", "abcdef0123456789abcdef0123456789"
	input := contributorpathways.Revision{RepositoryID: repositoryID, PublishedBy: actorID, Goals: "A clear goal", Prerequisites: []string{"Git"}, Conduct: "Be kind", Security: "Report privately", Setup: contributorpathways.Setup{Summary: "Run setup"}, Communication: "Use issues", ReviewPolicy: "Owner review", WorkCategories: []contributorpathways.WorkCategory{{Name: "Docs", Description: "Clarify docs", Audience: "human"}}}
	if _, err := store.Publish(input, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Publish(input, 0); err != contributorpathways.ErrConflict {
		t.Fatalf("stale publish error = %v", err)
	}
	items, err := store.List(repositoryID)
	if err != nil || len(items) != 1 {
		t.Fatalf("history = %#v, %v", items, err)
	}
	data, _ := json.Marshal(items[0])
	if len(data) == 0 {
		t.Fatal("revision did not marshal")
	}
}
