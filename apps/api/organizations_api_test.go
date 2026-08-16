package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/incidents"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/securityadvisories"
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

func TestOrganizationStewardshipMandatePublicLifecycleAndNoImplicitAuthority(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	groups, _ := organizations.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, groups))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "mandate-owner")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/organizations", `{"name":"Caretakers","slug":"caretakers"}`, owner.Credential.Token, http.StatusCreated)
	var group organizations.Organization
	if err := json.NewDecoder(response.Body).Decode(&group); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	response = authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/repositories", `{"name":"runtime"}`, owner.Credential.Token, http.StatusCreated)
	var repository repositories.Repository
	if err := json.NewDecoder(response.Body).Decode(&repository); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	response = authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/agents", `{"name":"Caretaker","slug":"caretaker","capabilities":["inspect checks"],"operator_ids":["`+owner.User.ID+`"],"team_ids":[]}`, owner.Credential.Token, http.StatusCreated)
	if err := json.NewDecoder(response.Body).Decode(&group); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	starts, expires := time.Now().UTC().Add(time.Minute).Format(time.RFC3339), time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	body := `{"title":"Keep runtime healthy","desired_outcomes":["required checks stay green"],"repositories":[{"repository_id":"` + repository.ID + `","branches":["main"]}],"trusted_signals":["required checks"],"exclusions":["no source writes"],"budget":{"max_agent_minutes":30,"max_actions":10},"starts_at":"` + starts + `","expires_at":"` + expires + `","agent_id":"` + group.Agents[0].ID + `","allowed_actions":["inspect_checks","summarize"],"required_human_decisions":["merge or release"]}`
	response = authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/stewardship-mandates", body, owner.Credential.Token, http.StatusCreated)
	var mandate organizations.StewardshipMandate
	if err := json.NewDecoder(response.Body).Decode(&mandate); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	response = authenticatedRequest(t, http.MethodGet, server.URL+"/organizations/"+group.ID+"/stewardship-mandates/"+mandate.ID+"/preview", "", owner.Credential.Token, http.StatusOK)
	var preview struct {
		AccessGrants      []organizations.AccessGrant `json:"access_grants"`
		ImplicitAuthority []string                    `json:"implicit_authority"`
	}
	if err := json.NewDecoder(response.Body).Decode(&preview); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if len(preview.AccessGrants) != 0 || len(preview.ImplicitAuthority) != 0 {
		t.Fatalf("mandate conferred authority: %#v", preview)
	}
	authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/stewardship-mandates/"+mandate.ID+"/accept", `{"expected_version":1}`, owner.Credential.Token, http.StatusOK).Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/stewardship-mandates/"+mandate.ID+"/pause", `{"expected_version":1}`, owner.Credential.Token, http.StatusOK).Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/stewardship-mandates/"+mandate.ID+"/revoke", `{"expected_version":1}`, owner.Credential.Token, http.StatusOK).Body.Close()
}

func TestOrganizationInitiativeProjectsDependenciesAndRejectsUnknownSources(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	groups, _ := organizations.New(t.TempDir())
	proposalStore, _ := proposals.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, proposalStore, nil, nil, nil, nil, groups))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "initiative-owner")
	created := authenticatedRequest(t, http.MethodPost, server.URL+"/organizations", `{"name":"Delivery","slug":"delivery"}`, owner.Credential.Token, http.StatusCreated)
	var group organizations.Organization
	if err := json.NewDecoder(created.Body).Decode(&group); err != nil {
		t.Fatal(err)
	}
	created.Body.Close()
	repositoryResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/repositories", `{"name":"provider"}`, owner.Credential.Token, http.StatusCreated)
	var repository repositories.Repository
	if err := json.NewDecoder(repositoryResponse.Body).Decode(&repository); err != nil {
		t.Fatal(err)
	}
	repositoryResponse.Body.Close()
	proposalResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/proposals", `{"title":"Ship runtime","body":"Coordinate delivery."}`, owner.Credential.Token, http.StatusCreated)
	var proposal proposals.Proposal
	if err := json.NewDecoder(proposalResponse.Body).Decode(&proposal); err != nil {
		t.Fatal(err)
	}
	proposalResponse.Body.Close()
	if _, err := proposalStore.Get(repository.ID, proposal.ID); err != nil {
		t.Fatalf("proposal source unavailable: %v", err)
	}
	if items, err := catalog.ListOrganization(group.ID); err != nil || len(items) != 1 {
		t.Fatalf("organization portfolio unavailable: %#v %v", items, err)
	}
	portfolioItems, _ := catalog.ListOrganization(group.ID)
	if !initiativeSourceExists(organizations.InitiativeSource{Kind: "proposal", RepositoryID: repository.ID, ID: proposal.ID}, owner.User.ID, portfolioItems, catalog, proposalStore, nil, nil, nil) {
		t.Fatal("initiative source validation failed")
	}
	if err := initiativeValidator(group, portfolioItems)(organizations.InitiativeWorkItem{RepositoryID: repository.ID, Owner: organizations.InitiativeOwner{Type: "human", ID: owner.User.ID}}); err != nil {
		t.Fatalf("initiative owner validation failed: %v", err)
	}
	unknown := `{"title":"Unknown","source":{"kind":"proposal","repository_id":"` + repository.ID + `","id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},"work_items":[{"id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","title":"Work","repository_id":"` + repository.ID + `","owner":{"type":"human","id":"` + owner.User.ID + `"},"dependency_ids":[],"status":"todo"}]}`
	authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/initiatives", unknown, owner.Credential.Token, http.StatusBadRequest).Body.Close()
	externalRepositoryResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"private-external"}`, owner.Credential.Token, http.StatusCreated)
	var externalRepository repositories.Repository
	if err := json.NewDecoder(externalRepositoryResponse.Body).Decode(&externalRepository); err != nil {
		t.Fatal(err)
	}
	externalRepositoryResponse.Body.Close()
	externalProposalResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+externalRepository.ID+"/proposals", `{"title":"Private plan","body":"Outside the organization portfolio."}`, owner.Credential.Token, http.StatusCreated)
	var externalProposal proposals.Proposal
	if err := json.NewDecoder(externalProposalResponse.Body).Decode(&externalProposal); err != nil {
		t.Fatal(err)
	}
	externalProposalResponse.Body.Close()
	externalContribution := `{"title":"Leaky contribution","source":{"kind":"proposal","repository_id":"` + repository.ID + `","id":"` + proposal.ID + `"},"work_items":[{"id":"dddddddddddddddddddddddddddddddd","title":"Private dependency","repository_id":"` + repository.ID + `","contribution":{"kind":"proposal","repository_id":"` + externalRepository.ID + `","id":"` + externalProposal.ID + `"},"owner":{"type":"human","id":"` + owner.User.ID + `"},"dependency_ids":[],"status":"todo"}]}`
	authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/initiatives", externalContribution, owner.Credential.Token, http.StatusBadRequest).Body.Close()
	body := `{"title":"Runtime rollout","source":{"kind":"proposal","repository_id":"` + repository.ID + `","id":"` + proposal.ID + `"},"work_items":[{"id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","title":"Publish provider","repository_id":"` + repository.ID + `","owner":{"type":"human","id":"` + owner.User.ID + `"},"dependency_ids":[],"status":"in_progress"},{"id":"cccccccccccccccccccccccccccccccc","title":"Verify adoption","repository_id":"` + repository.ID + `","owner":{"type":"human","id":"` + owner.User.ID + `"},"dependency_ids":["bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"],"status":"todo"}]}`
	initiativeResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/initiatives", body, owner.Credential.Token, http.StatusCreated)
	var initiative organizations.Initiative
	if err := json.NewDecoder(initiativeResponse.Body).Decode(&initiative); err != nil {
		t.Fatal(err)
	}
	initiativeResponse.Body.Close()
	portfolioResponse := authenticatedRequest(t, http.MethodGet, server.URL+"/organizations/"+group.ID+"/portfolio", "", owner.Credential.Token, http.StatusOK)
	var portfolio struct {
		Initiatives []struct {
			ID        string `json:"id"`
			WorkItems []struct {
				Blocked        bool     `json:"blocked"`
				BlockerIDs     []string `json:"blocker_ids"`
				OwnershipState string   `json:"ownership_state"`
			} `json:"work_items"`
		} `json:"initiatives"`
	}
	if err := json.NewDecoder(portfolioResponse.Body).Decode(&portfolio); err != nil {
		t.Fatal(err)
	}
	portfolioResponse.Body.Close()
	if len(portfolio.Initiatives) != 1 || portfolio.Initiatives[0].ID != initiative.ID || !portfolio.Initiatives[0].WorkItems[1].Blocked || len(portfolio.Initiatives[0].WorkItems[1].BlockerIDs) != 1 || portfolio.Initiatives[0].WorkItems[0].OwnershipState != "accountable" {
		t.Fatalf("initiative projection = %#v", portfolio.Initiatives)
	}
}

func TestSecurityInitiativeAuthorizationChecksEveryAffectedRepository(t *testing.T) {
	securityStore, err := securityadvisories.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	reporter := "0123456789abcdef0123456789abcdef"
	firstOwner := "11111111111111111111111111111111"
	secondOwner := "22222222222222222222222222222222"
	firstRepository := "33333333333333333333333333333333"
	secondRepository := "44444444444444444444444444444444"
	advisory, err := securityStore.Create(securityadvisories.Advisory{
		Title: "Shared runtime issue", Description: "Affects both services.", Contact: "security@example.test", ReporterID: reporter,
		AffectedRepositories: []securityadvisories.AffectedRepository{{RepositoryID: firstRepository, Versions: []string{"1.x"}}, {RepositoryID: secondRepository, Versions: []string{"2.x"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	portfolio := []repositories.Repository{{ID: firstRepository, OwnerID: firstOwner}, {ID: secondRepository, OwnerID: secondOwner}}
	if !initiativeSourceExists(organizations.InitiativeSource{Kind: "security", ID: advisory.ID}, secondOwner, portfolio, nil, nil, nil, nil, securityStore) {
		t.Fatal("owner of the later affected repository was denied")
	}
	if initiativeSourceExists(organizations.InitiativeSource{Kind: "security", RepositoryID: firstRepository, ID: advisory.ID}, secondOwner, portfolio, nil, nil, nil, nil, securityStore) {
		t.Fatal("repository-filtered source authorized an unrelated affected owner")
	}
}

func TestIncidentInitiativeRequiresExactAuthorizedPortfolioRepository(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	incidentStore, _ := incidents.New(t.TempDir())
	owner := "0123456789abcdef0123456789abcdef"
	actor := "11111111111111111111111111111111"
	portfolioRepository, err := catalog.Create(owner, "portfolio")
	if err != nil {
		t.Fatal(err)
	}
	privateRepository, err := catalog.Create(owner, "private-incident")
	if err != nil {
		t.Fatal(err)
	}
	incident, err := incidentStore.Create(incidents.Incident{Title: "Private outage", Summary: "Restricted response.", Severity: "sev2", Status: "investigating", Scopes: []incidents.Scope{{RepositoryID: privateRepository.ID, EnvironmentIDs: []string{}}}, Roles: []incidents.Role{}, DeclaredBy: owner})
	if err != nil {
		t.Fatal(err)
	}
	portfolio := []repositories.Repository{portfolioRepository}
	if initiativeSourceExists(organizations.InitiativeSource{Kind: "incident", ID: incident.ID}, actor, portfolio, catalog, nil, nil, incidentStore, nil) {
		t.Fatal("unscoped private incident reference was authorized")
	}
	if initiativeSourceExists(organizations.InitiativeSource{Kind: "incident", RepositoryID: privateRepository.ID, ID: incident.ID}, actor, portfolio, catalog, nil, nil, incidentStore, nil) {
		t.Fatal("incident outside the organization portfolio was authorized")
	}
	if _, err := catalog.AddCollaborator(owner, privateRepository.ID, actor); err != nil {
		t.Fatal(err)
	}
	portfolio = append(portfolio, privateRepository)
	if !initiativeSourceExists(organizations.InitiativeSource{Kind: "incident", RepositoryID: privateRepository.ID, ID: incident.ID}, actor, portfolio, catalog, nil, nil, incidentStore, nil) {
		t.Fatal("exact incident scope denied a current repository collaborator")
	}
}

func TestOrganizationTeamDirectoryExplainsEffectivePeopleAgentsAndResponsibility(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	groups, _ := organizations.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, groups))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "directory-owner")
	member := createTestAccount(t, server.URL, "directory-member")
	pending := createTestAccount(t, server.URL, "directory-pending")
	created := authenticatedRequest(t, http.MethodPost, server.URL+"/organizations", `{"name":"Platform","slug":"platform"}`, owner.Credential.Token, http.StatusCreated)
	var group organizations.Organization
	json.NewDecoder(created.Body).Decode(&group)
	created.Body.Close()
	invited := authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/invitations", `{"user_id":"`+member.User.ID+`"}`, owner.Credential.Token, http.StatusCreated)
	json.NewDecoder(invited.Body).Decode(&group)
	invited.Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/invitations/"+group.Invitations[0].ID+"/accept", "", member.Credential.Token, http.StatusOK).Body.Close()
	repoResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/repositories", `{"name":"runtime"}`, owner.Credential.Token, http.StatusCreated)
	var repo repositories.Repository
	json.NewDecoder(repoResponse.Body).Decode(&repo)
	repoResponse.Body.Close()
	authenticatedRequest(t, http.MethodPatch, server.URL+"/repositories/"+repo.ID, `{"visibility":"public"}`, owner.Credential.Token, http.StatusOK).Body.Close()
	teamResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/teams", `{"name":"Platform","slug":"platform","visibility":"public"}`, owner.Credential.Token, http.StatusCreated)
	json.NewDecoder(teamResponse.Body).Decode(&group)
	teamResponse.Body.Close()
	parent := group.Teams[0]
	childResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/teams", `{"name":"Runtime","slug":"runtime","parent_id":"`+parent.ID+`","visibility":"public"}`, owner.Credential.Token, http.StatusCreated)
	json.NewDecoder(childResponse.Body).Decode(&group)
	childResponse.Body.Close()
	child := group.Teams[1]
	memberResponse := authenticatedRequest(t, http.MethodPut, server.URL+"/organizations/"+group.ID+"/teams/"+child.ID+"/members", `{"user_id":"`+member.User.ID+`","role":"maintainer","expected_version":1}`, owner.Credential.Token, http.StatusOK)
	json.NewDecoder(memberResponse.Body).Decode(&group)
	memberResponse.Body.Close()
	child = group.Teams[1]
	authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/teams/"+child.ID+"/responsibilities", `{"repository_id":"`+repo.ID+`","area":"release runtime","description":"Owns runtime release health.","expected_version":2}`, owner.Credential.Token, http.StatusCreated).Body.Close()
	authenticatedRequest(t, http.MethodPut, server.URL+"/organizations/"+group.ID+"/teams/"+child.ID+"/members", `{"user_id":"`+member.User.ID+`","role":"member","expected_version":1}`, owner.Credential.Token, http.StatusConflict).Body.Close()
	agentResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/agents", `{"name":"Release Scout","slug":"release-scout","visibility":"public","capabilities":["inspect checks","summarize failures"],"operator_ids":["`+owner.User.ID+`"],"team_ids":["`+child.ID+`"]}`, owner.Credential.Token, http.StatusCreated)
	json.NewDecoder(agentResponse.Body).Decode(&group)
	agentResponse.Body.Close()
	agentID := group.Agents[0].ID
	profile := `{"expected_version":0,"profile":{"summary":"Reviews release failures","supported_tasks":["review checks"],"tools":["git"],"model_provenance":"Operator-selected model recorded per run","execution_provenance":"Operator-managed remote container","data_use":"Repository context is used for the requested review","retention":"Deleted after 24 hours","pricing":"One compute credit per run","resource_requirements":["read-only checkout"],"requested_capabilities":["repository:read"],"availability":"Weekdays","support":"support@example.test","subprocessors":["Compute Co for inference"],"remote_execution_boundaries":["Context crosses to Compute Co"],"change_summary":"Initial profile","verified_evidence":[{"kind":"forged","statement":"trust me"}]}}`
	authenticatedRequest(t, http.MethodPut, server.URL+"/organizations/"+group.ID+"/agents/"+agentID+"/profile", profile, owner.Credential.Token, http.StatusOK).Body.Close()
	authenticatedRequest(t, http.MethodPut, server.URL+"/organizations/"+group.ID+"/agents/"+agentID+"/profile", profile, owner.Credential.Token, http.StatusConflict).Body.Close()
	hiddenResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/teams", `{"name":"Internal","slug":"internal","visibility":"organization"}`, owner.Credential.Token, http.StatusCreated)
	json.NewDecoder(hiddenResponse.Body).Decode(&group)
	hiddenResponse.Body.Close()
	hidden := group.Teams[2]
	authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/teams", `{"name":"Community","slug":"community","parent_id":"`+hidden.ID+`","visibility":"public"}`, owner.Credential.Token, http.StatusCreated).Body.Close()
	pendingResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/invitations", `{"user_id":"`+pending.User.ID+`"}`, owner.Credential.Token, http.StatusCreated)
	pendingResponse.Body.Close()
	pendingDetail := authenticatedRequest(t, http.MethodGet, server.URL+"/organizations/"+group.ID, "", pending.Credential.Token, http.StatusOK)
	var pendingGroup organizations.Organization
	json.NewDecoder(pendingDetail.Body).Decode(&pendingGroup)
	pendingDetail.Body.Close()
	if len(pendingGroup.Teams) != 0 || len(pendingGroup.Agents) != 0 || len(pendingGroup.Events) != 0 || len(pendingGroup.Members) != 0 || len(pendingGroup.Invitations) != 1 {
		t.Fatalf("pending invitation exposed organization data: %#v", pendingGroup)
	}
	public := authenticatedRequest(t, http.MethodGet, server.URL+"/organizations/"+group.ID+"/directory", "", "", http.StatusOK)
	var directory organizations.Directory
	if err := json.NewDecoder(public.Body).Decode(&directory); err != nil {
		t.Fatal(err)
	}
	public.Body.Close()
	if len(directory.Teams) != 3 || len(directory.Agents) != 1 || len(directory.Teams[0].EffectiveMembers) != 1 || directory.Teams[0].EffectiveMembers[0].Reason != "nested team Runtime" || len(directory.Teams[1].Team.Responsibilities) != 1 || directory.Teams[2].Team.ParentID != "" {
		t.Fatalf("directory did not explain effective responsibility: %#v", directory)
	}
	if len(directory.Events) != 0 || directory.Agents[0].OperatorIDs[0] != owner.User.ID {
		t.Fatalf("public projection leaked audit or hid operator: %#v", directory)
	}
	if got := directory.Agents[0].Profiles; len(got) != 1 || got[0].Version != 1 || len(got[0].VerifiedEvidence) != 2 || got[0].VerifiedEvidence[0].Kind == "forged" {
		t.Fatalf("public profile did not separate claims and verification: %#v", got)
	}
	internal := authenticatedRequest(t, http.MethodGet, server.URL+"/organizations/"+group.ID+"/directory", "", member.Credential.Token, http.StatusOK)
	json.NewDecoder(internal.Body).Decode(&directory)
	internal.Body.Close()
	if len(directory.Events) < 7 {
		t.Fatalf("attribution events missing: %#v", directory.Events)
	}
}

func TestOrganizationScopedAgentAccessRequestCredentialAndRevocation(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	groups, _ := organizations.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, groups))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "access-owner")
	member := createTestAccount(t, server.URL, "access-member")
	created := authenticatedRequest(t, http.MethodPost, server.URL+"/organizations", `{"name":"Access Guild","slug":"access-guild"}`, owner.Credential.Token, http.StatusCreated)
	var group organizations.Organization
	json.NewDecoder(created.Body).Decode(&group)
	created.Body.Close()
	invited := authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/invitations", `{"user_id":"`+member.User.ID+`"}`, owner.Credential.Token, http.StatusCreated)
	json.NewDecoder(invited.Body).Decode(&group)
	invited.Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/invitations/"+group.Invitations[0].ID+"/accept", "", member.Credential.Token, http.StatusOK).Body.Close()
	repoResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/repositories", `{"name":"runtime"}`, owner.Credential.Token, http.StatusCreated)
	var repo repositories.Repository
	json.NewDecoder(repoResponse.Body).Decode(&repo)
	repoResponse.Body.Close()
	agentResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/agents", `{"name":"Release Agent","slug":"release-agent","capabilities":["prepare changes"],"operator_ids":["`+owner.User.ID+`"],"team_ids":[]}`, owner.Credential.Token, http.StatusCreated)
	json.NewDecoder(agentResponse.Body).Decode(&group)
	agentResponse.Body.Close()
	agentID := group.Agents[0].ID
	expires := time.Now().UTC().Add(time.Hour).Format(time.RFC3339)
	requestResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/access-requests", `{"principal_type":"agent","principal_id":"`+agentID+`","role":"contributor","resources":[{"kind":"repository","id":"`+repo.ID+`"}],"exceptions":[],"reason":"prepare coordinated runtime changes","expires_at":"`+expires+`"}`, owner.Credential.Token, http.StatusCreated)
	json.NewDecoder(requestResponse.Body).Decode(&group)
	requestResponse.Body.Close()
	requestID := group.AccessRequests[0].ID
	decision := authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/access-requests/"+requestID+"/decision", `{"decision":"approve"}`, owner.Credential.Token, http.StatusOK)
	json.NewDecoder(decision.Body).Decode(&group)
	decision.Body.Close()
	if len(group.AccessGrants) != 1 || group.AccessGrants[0].GrantedBy != owner.User.ID || group.AccessGrants[0].Role != "contributor" {
		t.Fatalf("grant = %#v", group.AccessGrants)
	}
	grant := group.AccessGrants[0]
	issuedResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/access-grants/"+grant.ID+"/credentials", `{"agent_id":"`+agentID+`","repository_id":"`+repo.ID+`","expires_in":600}`, owner.Credential.Token, http.StatusCreated)
	var issued auth.IssuedCredential
	json.NewDecoder(issuedResponse.Body).Decode(&issued)
	issuedResponse.Body.Close()
	if issued.OrganizationID != group.ID || issued.AccessGrantID != grant.ID || issued.RepositoryID != repo.ID {
		t.Fatalf("credential bounds = %#v", issued)
	}
	policyResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/policies", `{"name":"No agents","targets":[{"kind":"organization"}],"rules":{"agent_authority":"disabled"}}`, owner.Credential.Token, http.StatusCreated)
	var policy organizations.Policy
	json.NewDecoder(policyResponse.Body).Decode(&policy)
	policyResponse.Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/policies/"+policy.ID+"/activate", `{"expected_version":1}`, owner.Credential.Token, http.StatusOK).Body.Close()
	// Activation governs new authority without invalidating the already-issued credential.
	authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/access-grants/"+grant.ID+"/credentials", `{"agent_id":"`+agentID+`","repository_id":"`+repo.ID+`","expires_in":600}`, owner.Credential.Token, http.StatusConflict).Body.Close()
	blockedRequest := authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/access-requests", `{"principal_type":"agent","principal_id":"`+agentID+`","role":"viewer","resources":[{"kind":"repository","id":"`+repo.ID+`"}],"exceptions":[],"reason":"request after policy activation"}`, owner.Credential.Token, http.StatusCreated)
	json.NewDecoder(blockedRequest.Body).Decode(&group)
	blockedRequest.Body.Close()
	blockedRequestID := group.AccessRequests[len(group.AccessRequests)-1].ID
	authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/access-requests/"+blockedRequestID+"/decision", `{"decision":"approve"}`, owner.Credential.Token, http.StatusConflict).Body.Close()
	authenticatedRequest(t, http.MethodDelete, server.URL+"/organizations/"+group.ID+"/members/"+owner.User.ID, "", member.Credential.Token, http.StatusNotFound).Body.Close()
	if _, err := credentials.Authenticate(issued.Token, "git:read"); err != nil {
		t.Fatalf("rejected removal revoked derived credential: %v", err)
	}
	unrelated, err := credentials.Issue(owner.User.ID, auth.API, "unrelated", []string{"repositories:read"}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	authenticatedRequest(t, http.MethodDelete, server.URL+"/organizations/"+group.ID+"/access-grants/"+grant.ID, `{"expected_version":1}`, owner.Credential.Token, http.StatusNoContent).Body.Close()
	if _, err := credentials.Authenticate(issued.Token, "git:read"); !errors.Is(err, auth.ErrNotFound) {
		t.Fatalf("derived credential remained active: %v", err)
	}
	if _, err := credentials.Authenticate(unrelated.Token, "repositories:read"); err != nil {
		t.Fatalf("unrelated credential was disturbed: %v", err)
	}
	staleRepoResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/repositories", `{"name":"stale-target"}`, owner.Credential.Token, http.StatusCreated)
	var staleRepo repositories.Repository
	json.NewDecoder(staleRepoResponse.Body).Decode(&staleRepo)
	staleRepoResponse.Body.Close()
	staleRequestResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/access-requests", `{"principal_type":"agent","principal_id":"`+agentID+`","role":"viewer","resources":[{"kind":"repository","id":"`+staleRepo.ID+`"}],"exceptions":[],"reason":"inspect a temporary repository"}`, owner.Credential.Token, http.StatusCreated)
	json.NewDecoder(staleRequestResponse.Body).Decode(&group)
	staleRequestResponse.Body.Close()
	staleRequestID := group.AccessRequests[len(group.AccessRequests)-1].ID
	authenticatedRequest(t, http.MethodDelete, server.URL+"/repositories/"+staleRepo.ID, "", owner.Credential.Token, http.StatusNoContent).Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/access-requests/"+staleRequestID+"/decision", `{"decision":"approve"}`, owner.Credential.Token, http.StatusConflict).Body.Close()
	stored, _ := groups.Get(group.ID)
	if len(stored.AccessGrants) != 1 {
		t.Fatalf("stale repository approval created grant: %#v", stored.AccessGrants)
	}
	removedAgentResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/agents", `{"name":"Temporary Agent","slug":"temporary-agent","capabilities":["inspect"],"operator_ids":["`+member.User.ID+`"],"team_ids":[]}`, owner.Credential.Token, http.StatusCreated)
	json.NewDecoder(removedAgentResponse.Body).Decode(&group)
	removedAgentResponse.Body.Close()
	removedAgentID := group.Agents[len(group.Agents)-1].ID
	removedAgentRequest := authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/access-requests", `{"principal_type":"agent","principal_id":"`+removedAgentID+`","role":"viewer","resources":[{"kind":"repository","id":"`+repo.ID+`"}],"exceptions":[],"reason":"temporary inspection"}`, member.Credential.Token, http.StatusCreated)
	json.NewDecoder(removedAgentRequest.Body).Decode(&group)
	removedAgentRequest.Body.Close()
	removedAgentRequestID := group.AccessRequests[len(group.AccessRequests)-1].ID
	authenticatedRequest(t, http.MethodDelete, server.URL+"/organizations/"+group.ID+"/members/"+member.User.ID, "", owner.Credential.Token, http.StatusNoContent).Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/organizations/"+group.ID+"/access-requests/"+removedAgentRequestID+"/decision", `{"decision":"approve"}`, owner.Credential.Token, http.StatusConflict).Body.Close()
	stored, _ = groups.Get(group.ID)
	if len(stored.AccessGrants) != 1 {
		t.Fatalf("removed agent approval created grant: %#v", stored.AccessGrants)
	}
}
