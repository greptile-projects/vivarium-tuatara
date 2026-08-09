package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestOrganizationMembershipAndAcceptedRepositoryStewardship(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	groups, _ := organizations.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, groups))
	defer server.Close()

	owner := createTestAccount(t, server.URL, "group-owner")
	member := createTestAccount(t, server.URL, "group-member")
	veteran := createTestAccount(t, server.URL, "existing-collaborator")
	individual := createTestAccount(t, server.URL, "individual-owner")

	created := authenticatedRequest(t, http.MethodPost, server.URL+"/organizations", `{"name":"Runtime Guild","slug":"runtime-guild","description":"Stewards shared runtime work."}`, owner.Credential.Token, http.StatusCreated)
	var group organizations.Organization
	if err := json.NewDecoder(created.Body).Decode(&group); err != nil {
		t.Fatal(err)
	}
	created.Body.Close()
	readOnly, err := credentials.Issue(owner.User.ID, auth.API, "organization reader", []string{"repositories:read"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/repositories", `{"name":"scope-bypass"}`, readOnly.Token, http.StatusUnauthorized).Body.Close()
	if items, err := catalog.ListOrganization(group.ID); err != nil || len(items) != 0 {
		t.Fatalf("read-only mutation persisted repositories: items=%#v err=%v", items, err)
	}

	invited := authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/invitations", `{"user_id":"`+member.User.ID+`"}`, owner.Credential.Token, http.StatusCreated)
	if err := json.NewDecoder(invited.Body).Decode(&group); err != nil {
		t.Fatal(err)
	}
	invited.Body.Close()
	if len(group.Invitations) != 1 {
		t.Fatalf("invitations = %#v", group.Invitations)
	}
	authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/invitations/"+group.Invitations[0].ID+"/accept", "", member.Credential.Token, http.StatusOK).Body.Close()
	invited = authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/invitations", `{"user_id":"`+veteran.User.ID+`"}`, owner.Credential.Token, http.StatusCreated)
	if err := json.NewDecoder(invited.Body).Decode(&group); err != nil {
		t.Fatal(err)
	}
	invited.Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/invitations/"+group.Invitations[0].ID+"/accept", "", veteran.Credential.Token, http.StatusOK).Body.Close()

	ownedResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"preserved-history"}`, individual.Credential.Token, http.StatusCreated)
	var repository repositories.Repository
	if err := json.NewDecoder(ownedResponse.Body).Decode(&repository); err != nil {
		t.Fatal(err)
	}
	ownedResponse.Body.Close()
	beforeID, beforeRemote, beforeCreated := repository.ID, repository.GitRemote, repository.CreatedAt
	if _, err := catalog.AddCollaborator(individual.User.ID, repository.ID, veteran.User.ID); err != nil {
		t.Fatal(err)
	}

	requested := authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/repository-transfers", `{"repository_id":"`+repository.ID+`"}`, individual.Credential.Token, http.StatusAccepted)
	var transferResponse struct {
		organizations.Transfer
		Members json.RawMessage `json:"members"`
	}
	if err := json.NewDecoder(requested.Body).Decode(&transferResponse); err != nil {
		t.Fatal(err)
	}
	requested.Body.Close()
	if transferResponse.ID == "" || transferResponse.Status != "pending" || transferResponse.Members != nil {
		t.Fatalf("transfer response exposed organization data or omitted transfer = %#v", transferResponse)
	}
	preAcceptance, _ := catalog.GetByID(repository.ID)
	if preAcceptance.OrganizationID != "" {
		t.Fatal("request changed stewardship before acceptance")
	}

	authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/repository-transfers/"+transferResponse.ID+"/accept", "", owner.Credential.Token, http.StatusOK).Body.Close()
	after, _ := catalog.GetByID(repository.ID)
	if after.ID != beforeID || after.GitRemote != beforeRemote || !after.CreatedAt.Equal(beforeCreated) || after.OwnerID != individual.User.ID || after.OrganizationID != group.ID {
		t.Fatalf("repository identity changed across stewardship: before=%#v after=%#v", repository, after)
	}
	if collaborator, _ := catalog.HasCollaborator(member.User.ID, after.ID); !collaborator {
		t.Fatal("accepted member did not receive repository collaboration")
	}

	portfolio := authenticatedRequest(t, http.MethodGet, server.URL+"/organizations/"+group.ID+"/portfolio", "", member.Credential.Token, http.StatusOK)
	var view struct {
		Repositories []repositories.Repository `json:"repositories"`
	}
	if err := json.NewDecoder(portfolio.Body).Decode(&view); err != nil {
		t.Fatal(err)
	}
	portfolio.Body.Close()
	if len(view.Repositories) != 1 || view.Repositories[0].ID != repository.ID {
		t.Fatalf("portfolio = %#v", view)
	}

	authenticatedRequest(t, http.MethodDelete, server.URL+"/organizations/"+group.ID+"/members/"+member.User.ID, "", owner.Credential.Token, http.StatusNoContent).Body.Close()
	if collaborator, _ := catalog.HasCollaborator(member.User.ID, after.ID); collaborator {
		t.Fatal("removed group member retained projected repository access")
	}
	authenticatedRequest(t, http.MethodDelete, server.URL+"/organizations/"+group.ID+"/members/"+veteran.User.ID, "", owner.Credential.Token, http.StatusNoContent).Body.Close()
	if collaborator, _ := catalog.HasCollaborator(veteran.User.ID, after.ID); !collaborator {
		t.Fatal("removal erased a collaborator grant that predated organization stewardship")
	}
}
