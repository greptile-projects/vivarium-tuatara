package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/incubators"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestIncubatorPublicAPIRequiresInviteeConsentBeforeContribution(t *testing.T) {
	git, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), git)
	orgs, _ := organizations.New(t.TempDir())
	ledger, _ := incubators.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(git, identities, credentials, catalog, nil, nil, nil, nil, nil, orgs, ledger))
	defer server.Close()
	creator := createTestAccount(t, server.URL, "incubator-creator")
	invitee := createTestAccount(t, server.URL, "incubator-invitee")
	body := `{"title":"A project before code","audience":"Developer teams","problem":"Repository choices arrive before shared intent","desired_outcome":"Agree on purpose first","constraints":["No repository"],"success_measures":["Participant consent"],"sponsor_ids":["` + creator.User.ID + `"],"decision_rights":[{"kind":"scope_change","decision":"Change scope","principal_ids":["` + creator.User.ID + `"],"rule":"owner"}],"visibility":"participants","source":{"kind":"new_idea","label":"New collaborative idea"},"invitations":[{"principal_type":"human","principal_id":"` + invitee.User.ID + `","role":"co-designer"}]}`
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/incubators", body, creator.Credential.Token, http.StatusCreated)
	var created incubators.Incubator
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if created.Source.Resolution != "resolved" || len(created.Invitations) != 1 {
		t.Fatalf("created = %#v", created)
	}
	authenticatedRequest(t, http.MethodPost, server.URL+"/incubators/"+created.ID+"/events", `{"expected_version":1,"kind":"discussion","body":"premature","visibility":"participants"}`, invitee.Credential.Token, http.StatusNotFound).Body.Close()
	consent := authenticatedRequest(t, http.MethodPost, server.URL+"/incubators/"+created.ID+"/invitations/"+created.Invitations[0].ID+"/consent", `{"expected_version":1,"decision":"accepted"}`, invitee.Credential.Token, http.StatusOK)
	var accepted incubators.Incubator
	_ = json.NewDecoder(consent.Body).Decode(&accepted)
	consent.Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/incubators/"+created.ID+"/events", `{"expected_version":2,"kind":"evidence","body":"Three teams reported the same gap","visibility":"participants"}`, invitee.Credential.Token, http.StatusOK).Body.Close()
}
