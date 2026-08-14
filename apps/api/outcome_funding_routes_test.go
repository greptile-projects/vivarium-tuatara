package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"crypto/ed25519"
	"crypto/rand"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/projectfunds"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestFormerPublicBackerCanWithdrawWithoutReadingRestrictedOutcome(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	publicKey, _, _ := ed25519.GenerateKey(rand.Reader)
	funds, _ := projectfunds.New(t.TempDir(), map[string]string{"card": base64.StdEncoding.EncodeToString(publicKey)})
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, funds))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "funding-owner")
	backer := createTestAccount(t, server.URL, "former-public-backer")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"funding-visibility"}`, owner.Credential.Token, http.StatusCreated)
	var repository repositories.Repository
	if err := json.NewDecoder(response.Body).Decode(&repository); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	authenticatedRequest(t, http.MethodPatch, server.URL+"/repositories/"+repository.ID, `{"visibility":"public"}`, owner.Credential.Token, http.StatusOK).Body.Close()
	fund, err := funds.Create(repository.ID, owner.User.ID, projectfunds.Terms{Name: "Fund", Purpose: "Outcome backing", Stewards: []string{owner.User.ID}, FundingSources: []string{"card"}, Unit: "USD", Precision: 2, SpendingLimits: []projectfunds.Limit{{Period: "monthly", Amount: 1000}}, ApprovalRules: []projectfunds.ApprovalRule{{RequiredApprovals: 1, EligibleApprovers: []string{owner.User.ID}}}, EligibleRecipients: []string{"contributors"}, RefundPolicy: "Return withdrawn backing", LedgerVisibility: "public"})
	if err != nil {
		t.Fatal(err)
	}
	terms := projectfunds.OutcomeTerms{Title: "Public repair", Source: projectfunds.OutcomeSource{Kind: "issue", ID: "issue-1", Revision: "1", Visibility: "public"}, Scope: "Public scope", AcceptanceCriteria: []string{"verified"}, EvidenceRequirements: []string{"test"}, Budget: 100, Deadline: time.Now().UTC().Add(time.Hour), ContributorEligibility: []string{"participants"}, AllocationMethod: "first_accepted", CancellationTerms: "Backing may be withdrawn"}
	outcome, err := funds.CreateOutcome(repository.ID, fund.ID, owner.User.ID, terms)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err = funds.PledgeOutcome(outcome.ID, backer.User.ID, "", 50, "public-pledge", "", outcome.Version)
	if err != nil {
		t.Fatal(err)
	}
	pledgeID := outcome.Pledges[0].ID
	terms.Source.Visibility, terms.Scope = "participants", "CONFIDENTIAL-CURRENT-SCOPE-DO-NOT-DISCLOSE"
	outcome, err = funds.ReviseOutcome(outcome.ID, owner.User.ID, outcome.Version, terms, "Restrict current work")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = funds.GetOutcome(outcome.ID); err != nil {
		t.Fatalf("stored outcome before action: %v", err)
	}
	actionURL := server.URL + "/repositories/" + repository.ID + "/funded-outcomes/" + outcome.ID + "/pledges/" + pledgeID
	authenticatedRequest(t, http.MethodPost, actionURL, `{"expected_version":`+strconv.Itoa(outcome.Version)+`,"action":"reconfirm","reason":"accept"}`, backer.Credential.Token, http.StatusForbidden).Body.Close()
	response = authenticatedRequest(t, http.MethodPost, actionURL, `{"expected_version":`+strconv.Itoa(outcome.Version)+`,"action":"withdraw","reason":"leave restricted work"}`, backer.Credential.Token, http.StatusOK)
	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if _, disclosed := body["revisions"]; disclosed {
		t.Fatalf("restricted outcome disclosed: %+v", body)
	}
	pledge := body["pledge"].(map[string]any)
	if pledge["status"] != "withdrawn" || len(pledge) != 3 {
		t.Fatalf("minimal withdrawal = %+v", body)
	}
}

func TestHumanDeliveryProposalIsSelfSubmittedAndAcceptanceGated(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	publicKey, _, _ := ed25519.GenerateKey(rand.Reader)
	funds, _ := projectfunds.New(t.TempDir(), map[string]string{"card": base64.StdEncoding.EncodeToString(publicKey)})
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, funds))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "delivery-owner")
	other := createTestAccount(t, server.URL, "delivery-other")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"delivery-selection"}`, owner.Credential.Token, http.StatusCreated)
	var repository repositories.Repository
	if err := json.NewDecoder(response.Body).Decode(&repository); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	fund, err := funds.Create(repository.ID, owner.User.ID, projectfunds.Terms{Name: "Fund", Purpose: "Delivery", Stewards: []string{owner.User.ID}, FundingSources: []string{"card"}, Unit: "USD", Precision: 2, SpendingLimits: []projectfunds.Limit{{Period: "monthly", Amount: 1000}}, ApprovalRules: []projectfunds.ApprovalRule{{RequiredApprovals: 1, EligibleApprovers: []string{owner.User.ID}}}, EligibleRecipients: []string{"contributors"}, RefundPolicy: "Return unallocated backing", LedgerVisibility: "public"})
	if err != nil {
		t.Fatal(err)
	}
	terms := projectfunds.OutcomeTerms{Title: "Commission repair", Source: projectfunds.OutcomeSource{Kind: "issue", ID: "issue-1", Revision: "1", Visibility: "participants"}, Scope: "Repair the endpoint", AcceptanceCriteria: []string{"checks pass"}, EvidenceRequirements: []string{"reviewed pull"}, Budget: 100, Deadline: time.Now().UTC().Add(time.Hour), ContributorEligibility: []string{"repository participants"}, AllocationMethod: "maintainer_selection", CancellationTerms: "Return unallocated value"}
	outcome, err := funds.CreateOutcome(repository.ID, fund.ID, owner.User.ID, terms)
	if err != nil {
		t.Fatal(err)
	}
	endpoint := server.URL + "/repositories/" + repository.ID + "/funded-outcomes/" + outcome.ID + "/delivery-proposals"
	body := `{"applicant":{"kind":"human","id":"` + other.User.ID + `"},"terms":{"approach":"Repair and verify.","milestones":["deliver"],"cost":90,"dependencies":[],"availability":"This week","required_access":["task branch"],"relevant_work":[{"kind":"pull","id":"prior","note":"Related repair"}]}}`
	authenticatedRequest(t, http.MethodPost, endpoint, body, owner.Credential.Token, http.StatusForbidden).Body.Close()
	body = strings.Replace(body, other.User.ID, owner.User.ID, 1)
	response = authenticatedRequest(t, http.MethodPost, endpoint, body, owner.Credential.Token, http.StatusCreated)
	if err := json.NewDecoder(response.Body).Decode(&outcome); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	proposal := outcome.DeliveryProposals[0]
	acceptURL := endpoint + "/" + proposal.ID + "/accept"
	authenticatedRequest(t, http.MethodPost, acceptURL, `{"expected_version":`+strconv.Itoa(outcome.Version)+`}`, other.Credential.Token, http.StatusNotFound).Body.Close()
	response = authenticatedRequest(t, http.MethodPost, acceptURL, `{"expected_version":`+strconv.Itoa(outcome.Version)+`}`, owner.Credential.Token, http.StatusOK)
	if err := json.NewDecoder(response.Body).Decode(&outcome); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if outcome.DeliveryProposals[0].Status != "accepted" || outcome.DeliveryProposals[0].AcceptedBy != owner.User.ID {
		t.Fatalf("accepted proposal = %+v", outcome.DeliveryProposals[0])
	}
}

func TestFormerParticipantStewardCannotSelectDeliveryRecipients(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	publicKey, _, _ := ed25519.GenerateKey(rand.Reader)
	funds, _ := projectfunds.New(t.TempDir(), map[string]string{"card": base64.StdEncoding.EncodeToString(publicKey)})
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, funds))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "former-steward-owner")
	steward := createTestAccount(t, server.URL, "former-steward")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"former-steward-selection"}`, owner.Credential.Token, http.StatusCreated)
	var repository repositories.Repository
	if err := json.NewDecoder(response.Body).Decode(&repository); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if _, err := catalog.AddCollaborator(owner.User.ID, repository.ID, steward.User.ID); err != nil {
		t.Fatal(err)
	}
	authenticatedRequest(t, http.MethodPatch, server.URL+"/repositories/"+repository.ID, `{"visibility":"public"}`, owner.Credential.Token, http.StatusOK).Body.Close()
	fund, err := funds.Create(repository.ID, owner.User.ID, projectfunds.Terms{Name: "Fund", Purpose: "Delivery", Stewards: []string{steward.User.ID}, FundingSources: []string{"card"}, Unit: "USD", Precision: 2, SpendingLimits: []projectfunds.Limit{{Period: "monthly", Amount: 1000}}, ApprovalRules: []projectfunds.ApprovalRule{{RequiredApprovals: 1, EligibleApprovers: []string{steward.User.ID}}}, EligibleRecipients: []string{"contributors"}, RefundPolicy: "Return unallocated backing", LedgerVisibility: "public"})
	if err != nil {
		t.Fatal(err)
	}
	terms := projectfunds.OutcomeTerms{Title: "Commission repair", Source: projectfunds.OutcomeSource{Kind: "issue", ID: "issue-1", Revision: "1", Visibility: "public"}, Scope: "Repair the endpoint", AcceptanceCriteria: []string{"checks pass"}, EvidenceRequirements: []string{"reviewed pull"}, Budget: 100, Deadline: time.Now().UTC().Add(time.Hour), ContributorEligibility: []string{"repository participants"}, AllocationMethod: "maintainer_selection", CancellationTerms: "Return unallocated value"}
	outcome, err := funds.CreateOutcome(repository.ID, fund.ID, owner.User.ID, terms)
	if err != nil {
		t.Fatal(err)
	}
	proposalTerms := projectfunds.DeliveryProposalTerms{Approach: "Repair and verify.", Milestones: []string{"deliver"}, Cost: 90, Availability: "This week", RelevantWork: []projectfunds.AttributedWork{{Kind: "pull", ID: "prior", Note: "Related repair"}}}
	outcome, err = funds.SubmitDeliveryProposal(outcome.ID, projectfunds.DeliveryApplicant{Kind: "human", ID: owner.User.ID, SubmittedBy: owner.User.ID}, proposalTerms)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err = funds.AcceptDeliveryProposal(outcome.ID, outcome.DeliveryProposals[0].ID, owner.User.ID, outcome.Version)
	if err != nil {
		t.Fatal(err)
	}
	if err := catalog.RemoveCollaborator(owner.User.ID, repository.ID, steward.User.ID); err != nil {
		t.Fatal(err)
	}
	selectionURL := server.URL + "/repositories/" + repository.ID + "/funded-outcomes/" + outcome.ID + "/delivery-selections"
	body := `{"expected_version":` + strconv.Itoa(outcome.Version) + `,"proposal_ids":["` + outcome.DeliveryProposals[0].ID + `"],"conflict_disclosure":"None.","rationale":"Best fit."}`
	authenticatedRequest(t, http.MethodPost, selectionURL, body, steward.Credential.Token, http.StatusForbidden).Body.Close()
	fund, err = funds.Get(fund.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fund.Balances.Reserved != 0 {
		t.Fatalf("former steward reserved funds: %+v", fund.Balances)
	}
}
