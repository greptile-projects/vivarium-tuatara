package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/designgovernance"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/interfacechecks"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/interfacesystems"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestDesignAcceptanceRequiresCurrentRepositoryParticipation(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	pullStore, _ := pullrequests.New(t.TempDir(), gitStore)
	organizationStore, _ := organizations.New(t.TempDir())
	governanceStore, _ := designgovernance.New(t.TempDir())
	releaseStore, _ := releases.New(t.TempDir())
	checkStore, _ := interfacechecks.New(t.TempDir())
	systemStore, _ := interfacesystems.New(t.TempDir())
	proposalStore, _ := proposals.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, proposalStore, pullStore, nil, nil, nil, organizationStore, releaseStore, checkStore, systemStore, governanceStore))
	defer server.Close()

	owner := createTestAccount(t, server.URL, "design-acceptance-owner")
	outsider := createTestAccount(t, server.URL, "design-acceptance-outsider")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"private-design"}`, owner.Credential.Token, http.StatusCreated)
	var repository repositories.Repository
	decodeResponse(t, response, &repository)
	bare, _ := gitStore.Open(repository.ID)
	base := writeCommit(t, bare, 1700000000, "base")
	candidate := writeCommit(t, bare, 1700000001, "candidate")
	if err := bare.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(base)}); err != nil {
		t.Fatal(err)
	}
	if err := bare.CreateReference(storage.Reference{Name: "refs/heads/design", Target: string(candidate)}); err != nil {
		t.Fatal(err)
	}
	pull, err := pullStore.Create(repository.ID, owner.User.ID, "Private design", "Exact candidate", "design", "main", nil)
	if err != nil {
		t.Fatal(err)
	}
	policyResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/design-acceptance-policies", `{"name":"Named design owner","selectors":[{"kind":"path","value":"README.md"}],"requirements":[{"role":"design_owner","approver_ids":["`+outsider.User.ID+`"]}],"exception_max_hours":24}`, owner.Credential.Token, http.StatusCreated)
	var policy designgovernance.Policy
	decodeResponse(t, policyResponse, &policy)
	acceptanceBody := `{"policy_id":"` + policy.ID + `","policy_version":1,"role":"design_owner","decision":"accepted","rationale":"Named approver decision"}`
	acceptanceURL := server.URL + "/repositories/" + repository.ID + "/pulls/" + pull.ID + "/design-acceptances"
	authenticatedRequest(t, http.MethodPost, acceptanceURL, acceptanceBody, outsider.Credential.Token, http.StatusNotFound).Body.Close()

	listed := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repository.ID+"/pulls/"+pull.ID+"/design-readiness", "", owner.Credential.Token, http.StatusOK)
	var before designgovernance.Readiness
	decodeResponse(t, listed, &before)
	if len(before.Acceptances) != 0 {
		t.Fatalf("outsider acceptance persisted: %#v", before.Acceptances)
	}
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/collaborators", `{"user_id":"`+outsider.User.ID+`"}`, owner.Credential.Token, http.StatusCreated).Body.Close()
	accepted := authenticatedRequest(t, http.MethodPost, acceptanceURL, acceptanceBody, outsider.Credential.Token, http.StatusCreated)
	var acceptance designgovernance.Acceptance
	if err := json.NewDecoder(accepted.Body).Decode(&acceptance); err != nil {
		t.Fatal(err)
	}
	accepted.Body.Close()
	if acceptance.ActorID != outsider.User.ID || acceptance.PullRequestID != pull.ID || acceptance.Revision != strings.ToLower(string(candidate)) {
		t.Fatalf("authorized acceptance = %#v", acceptance)
	}
}
