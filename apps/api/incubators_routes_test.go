package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/governance"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/incubators"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestArchiveResolutionRequiresAcceptedExactArtifactDecision(t *testing.T) {
	artifact := incubators.LaunchArtifact{Kind: "environment", RepositoryID: "repo-a", ResourceID: "production", Revision: ""}
	proposal := governance.Proposal{Status: "closed", ScopeType: "repository", ScopeID: "repo-a", Tally: &governance.Tally{Status: "not_accepted", Result: "reject"}, AffectedResources: []governance.Reference{{Kind: "environment", ResourceID: "production"}}}
	if proposalResolvesLaunchArtifact(proposal, artifact) {
		t.Fatal("rejected proposal resolved artifact")
	}
	proposal.Tally.Status = "accepted"
	proposal.Tally.Contested = true
	if proposalResolvesLaunchArtifact(proposal, artifact) {
		t.Fatal("contested proposal resolved artifact")
	}
	proposal.Tally.Contested = false
	proposal.AffectedResources[0].ResourceID = "unrelated"
	if proposalResolvesLaunchArtifact(proposal, artifact) {
		t.Fatal("unrelated proposal resolved artifact")
	}
	proposal.AffectedResources[0].ResourceID = artifact.ResourceID
	if !proposalResolvesLaunchArtifact(proposal, artifact) {
		t.Fatal("accepted exact proposal did not resolve artifact")
	}
	proposal.AffectedResources[0].Revision = strings.Repeat("a", 40)
	if proposalResolvesLaunchArtifact(proposal, artifact) {
		t.Fatal("mismatched revision resolved artifact")
	}
}

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

func TestIncubatorResearchAPIResolvesEvidenceAndBoundsExperimentAuthority(t *testing.T) {
	git, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), git)
	orgs, _ := organizations.New(t.TempDir())
	ledger, _ := incubators.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(git, identities, credentials, catalog, nil, nil, nil, nil, nil, orgs, ledger))
	defer server.Close()
	creator := createTestAccount(t, server.URL, "research-creator")
	createBody := `{"title":"Research before architecture","audience":"Developer teams","problem":"Prototypes become permanent by accident","desired_outcome":"Compare foundations with evidence","constraints":["No authoritative infrastructure"],"success_measures":["A reproducible comparison"],"sponsor_ids":["` + creator.User.ID + `"],"decision_rights":[{"kind":"scope_change","decision":"Change scope","principal_ids":["` + creator.User.ID + `"],"rule":"owner"}],"visibility":"participants","source":{"kind":"new_idea","label":"Research need"},"invitations":[]}`
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/incubators", createBody, creator.Credential.Token, http.StatusCreated)
	var x incubators.Incubator
	_ = json.NewDecoder(response.Body).Decode(&x)
	response.Body.Close()
	alternative := `{"expected_version":1,"sources":[{"kind":"public","label":"Benchmark","url":"https://example.test/benchmark"},{"kind":"public","label":"Broken","url":"http://example.test/private"}],"alternative":{"name":"Adopt storage","product_boundary":"Own workflow","architecture":"Stateless service","interfaces":["HTTP"],"dependencies":["vendor"],"licenses":["Apache-2.0"],"operating_costs":["$20/month"],"security_risks":["supply chain"],"data_risks":["residency"],"build_or_adopt":"hybrid","unknowns":["latency"]}}`
	response = authenticatedRequest(t, http.MethodPost, server.URL+"/incubators/"+x.ID+"/alternatives", alternative, creator.Credential.Token, http.StatusCreated)
	_ = json.NewDecoder(response.Body).Decode(&x)
	response.Body.Close()
	if x.ResearchSources[0].Resolution != "resolved" || x.ResearchSources[1].Resolution != "missing" {
		t.Fatalf("research projection = %#v", x.ResearchSources)
	}
	experiment := `{"expected_version":2,"experiment":{"alternative_id":"` + x.Alternatives[0].ID + `","question":"Is it fast enough?","environment":"ephemeral container","commands":["bench --synthetic"],"inputs":["synthetic v1"],"expected_measures":["p95 ms"],"safety_limits":["no writes"],"source_ids":["` + x.ResearchSources[0].ID + `"]}}`
	response = authenticatedRequest(t, http.MethodPost, server.URL+"/incubators/"+x.ID+"/experiments", experiment, creator.Credential.Token, http.StatusCreated)
	_ = json.NewDecoder(response.Body).Decode(&x)
	response.Body.Close()
	if x.Experiments[0].Authority != "research_only_no_code_or_infrastructure_authority" || len(x.Experiments[0].DefinitionSHA256) != 64 {
		t.Fatalf("experiment = %#v", x.Experiments[0])
	}
}

func TestBootstrapActivationRevalidatesConnectedRepositoryOwnership(t *testing.T) {
	git, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), git)
	orgs, _ := organizations.New(t.TempDir())
	ledger, _ := incubators.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(git, identities, credentials, catalog, nil, nil, nil, nil, nil, orgs, ledger))
	defer server.Close()
	creator := createTestAccount(t, server.URL, "bootstrap-authority-owner")
	invitee := createTestAccount(t, server.URL, "bootstrap-non-owner")
	x, err := ledger.Create(incubators.Incubator{Title: "Governed boundary", Audience: "Contributors", Problem: "Disconnected settings", DesiredOutcome: "One safe boundary", Constraints: []string{"No orphaned names"}, SuccessMeasures: []string{"Current authority"}, SponsorIDs: []string{creator.User.ID}, DecisionRights: []incubators.DecisionRight{{Kind: "project_update", Decision: "Activate boundary", PrincipalIDs: []string{creator.User.ID}, Rule: "owner"}}, Visibility: "participants", Source: incubators.Source{Kind: "new_idea", Label: "Project", Resolution: "resolved"}}, creator.User.ID, []incubators.Invitation{{PrincipalType: "human", PrincipalID: invitee.User.ID, Role: "participant"}})
	if err != nil {
		t.Fatal(err)
	}
	x, err = ledger.Consent(x.ID, x.Invitations[0].ID, invitee.User.ID, "accepted", 1)
	if err != nil {
		t.Fatal(err)
	}
	x, err = ledger.AddAlternative(x.ID, "human", creator.User.ID, 2, nil, incubators.Alternative{Name: "Accepted", ProductBoundary: "Service", Architecture: "API", Interfaces: []string{"HTTP"}, Dependencies: []string{"runtime"}, Licenses: []string{"MIT"}, OperatingCosts: []string{"estimated"}, SecurityRisks: []string{"access"}, DataRisks: []string{"retention"}, BuildOrAdopt: "build", Unknowns: []string{"scale"}})
	if err != nil {
		t.Fatal(err)
	}
	repo, err := catalog.Create(creator.User.ID, "connected-bootstrap")
	if err != nil {
		t.Fatal(err)
	}
	kinds := []string{"organization", "repository", "team", "package", "agent_role", "contributor_pathway", "documentation", "environment", "review_policy", "security_policy", "privacy_policy", "quality_policy", "release_policy"}
	resources := []incubators.BootstrapResource{}
	for _, kind := range kinds {
		resource := incubators.BootstrapResource{Kind: kind, Mode: "create", Name: "boundary-" + kind, OwnerIDs: []string{creator.User.ID}, MonthlyCostEstimateCents: 10}
		if kind == "repository" {
			resource.Mode = "connect"
			resource.ResourceID = repo.ID
		}
		resources = append(resources, resource)
	}
	fabricated := append([]incubators.BootstrapResource{}, resources...)
	for i := range fabricated {
		if fabricated[i].Kind == "repository" {
			fabricated[i].OwnerIDs = []string{creator.User.ID, invitee.User.ID}
		}
	}
	body, _ := json.Marshal(map[string]any{"expected_version": 3, "alternative_id": x.Alternatives[0].ID, "resources": fabricated})
	authenticatedRequest(t, http.MethodPost, server.URL+"/incubators/"+x.ID+"/bootstrap-previews", string(body), creator.Credential.Token, http.StatusUnprocessableEntity).Body.Close()
	body, _ = json.Marshal(map[string]any{"expected_version": 3, "alternative_id": x.Alternatives[0].ID, "resources": resources})
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/incubators/"+x.ID+"/bootstrap-previews", string(body), creator.Credential.Token, http.StatusCreated)
	_ = json.NewDecoder(response.Body).Decode(&x)
	response.Body.Close()
	plan := x.BootstrapPlans[0]
	response = authenticatedRequest(t, http.MethodPost, server.URL+"/incubators/"+x.ID+"/bootstrap-plans/"+plan.ID+"/decisions", `{"expected_version":4,"plan_version":1,"decision":"approved"}`, creator.Credential.Token, http.StatusOK)
	_ = json.NewDecoder(response.Body).Decode(&x)
	response.Body.Close()
	if err = catalog.Delete(creator.User.ID, repo.ID); err != nil {
		t.Fatal(err)
	}
	action := `{"expected_version":5,"plan_version":2,"action":"activate"}`
	authenticatedRequest(t, http.MethodPost, server.URL+"/incubators/"+x.ID+"/bootstrap-plans/"+plan.ID+"/actions", action, creator.Credential.Token, http.StatusConflict).Body.Close()
	retained, err := ledger.Get(x.ID, creator.User.ID)
	if err != nil || retained.BootstrapPlans[0].Status != "approved" {
		t.Fatalf("deleted connection activated: %#v, %v", retained.BootstrapPlans, err)
	}
}

func TestIncubatorCodeEvidenceRequiresVisibleCommitAndExactBlob(t *testing.T) {
	git, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), git)
	orgs, _ := organizations.New(t.TempDir())
	ledger, _ := incubators.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(git, identities, credentials, catalog, nil, nil, nil, nil, nil, orgs, ledger))
	defer server.Close()
	creator := createTestAccount(t, server.URL, "code-research-creator")
	record, err := catalog.Create(creator.User.ID, "research-code")
	if err != nil {
		t.Fatal(err)
	}
	repository, _ := git.Open(record.ID)
	visible := agentProjectCommit(t, git, record.ID, "main", "evidence.md", "")
	hidden := agentProjectCommit(t, git, record.ID, "vivarium-security/embargo", "secret.md", visible)
	blob, _ := repository.WriteObject(storage.BlobObject, []byte("not a commit"))
	createBody := `{"title":"Exact code evidence","audience":"Developer teams","problem":"Claims need exact code","desired_outcome":"Only visible files resolve","constraints":["No hidden history"],"success_measures":["Exact file"],"sponsor_ids":["` + creator.User.ID + `"],"decision_rights":[{"kind":"scope_change","decision":"Change scope","principal_ids":["` + creator.User.ID + `"],"rule":"owner"}],"visibility":"participants","source":{"kind":"new_idea","label":"Code research"},"invitations":[]}`
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/incubators", createBody, creator.Credential.Token, http.StatusCreated)
	var x incubators.Incubator
	_ = json.NewDecoder(response.Body).Decode(&x)
	response.Body.Close()
	add := func(revision, file string, version int) incubators.Incubator {
		body := `{"expected_version":` + strconv.Itoa(version) + `,"sources":[{"kind":"code","label":"Exact file","repository_id":"` + record.ID + `","resource_id":"code","revision":"` + revision + `","path":"` + file + `"}],"alternative":{"name":"Code candidate","product_boundary":"Own workflow","architecture":"Service","interfaces":["HTTP"],"dependencies":["library"],"licenses":["MIT"],"operating_costs":["bounded"],"security_risks":["dependency"],"data_risks":["retention"],"build_or_adopt":"build","unknowns":["scale"]}}`
		res := authenticatedRequest(t, http.MethodPost, server.URL+"/incubators/"+x.ID+"/alternatives", body, creator.Credential.Token, http.StatusCreated)
		var out incubators.Incubator
		_ = json.NewDecoder(res.Body).Decode(&out)
		res.Body.Close()
		return out
	}
	cases := []struct{ revision, path, resolution string }{{strings.Repeat("f", 40), "evidence.md", "missing"}, {string(blob), "evidence.md", "missing"}, {string(hidden), "secret.md", "inaccessible"}, {string(visible), "missing.md", "missing"}, {string(visible), "evidence.md", "resolved"}}
	for index, tc := range cases {
		x = add(tc.revision, tc.path, index+1)
		got := x.ResearchSources[index]
		if got.Resolution != tc.resolution {
			t.Fatalf("case %d = %#v", index, got)
		}
		if tc.resolution != "resolved" && (got.RepositoryID != "" || got.Revision != "" || got.Path != "") {
			t.Fatalf("failed code reference leaked: %#v", got)
		}
	}
}
