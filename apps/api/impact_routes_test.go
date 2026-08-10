package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/explanations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/impacts"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestImpactAssessmentFreezesEvidenceAndRequiresExplicitParticipation(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	impactStore, _ := impacts.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, impactStore))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "impact-owner")
	collaborator := createTestAccount(t, server.URL, "impact-reviewer")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"impact-source"}`, owner.Credential.Token, http.StatusCreated)
	var repository repositories.Repository
	json.NewDecoder(response.Body).Decode(&repository)
	response.Body.Close()
	repo, _ := gitStore.Open(repository.ID)
	source, _ := repo.WriteObject(storage.BlobObject, []byte("package behavior\nfunc Authorize() bool { return true }\n"))
	testSource, _ := repo.WriteObject(storage.BlobObject, []byte("package behavior\nfunc TestAuthorize(t *testing.T) {}\n"))
	tree := writeTestTree(t, repo, testTreeEntry{"100644", "authorize.go", source}, testTreeEntry{"100644", "authorize_test.go", testSource})
	commit := writeTestCommit(t, repo, tree, nil, 1700000000, "authorization")
	if err := repo.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(commit)}); err != nil {
		t.Fatal(err)
	}
	created := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/impact-assessments", `{"title":"Change authorization","ref":"main","query":"Authorize","source":{"kind":"selected_code","path":"authorize.go","start_line":2,"end_line":2}}`, owner.Credential.Token, http.StatusCreated)
	var assessment impacts.Assessment
	json.NewDecoder(created.Body).Decode(&assessment)
	created.Body.Close()
	if assessment.Revision != string(commit) || assessment.Version != 1 || !hasImpactKind(assessment, "reference") || !hasImpactKind(assessment, "test") || !hasImpactKind(assessment, "owner") {
		t.Fatalf("derived assessment = %#v", assessment)
	}
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/collaborators", `{"user_id":"`+collaborator.User.ID+`"}`, owner.Credential.Token, http.StatusCreated).Body.Close()
	list := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repository.ID+"/impact-assessments", "", collaborator.Credential.Token, http.StatusOK)
	var before struct {
		Assessments []impacts.Assessment `json:"assessments"`
	}
	json.NewDecoder(list.Body).Decode(&before)
	list.Body.Close()
	if len(before.Assessments) != 0 {
		t.Fatal("repository collaborator inherited a private assessment")
	}
	invited := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/impact-assessments/"+assessment.ID+"/participants", `{"user_id":"`+collaborator.User.ID+`","version":1}`, owner.Credential.Token, http.StatusOK)
	invited.Body.Close()
	read := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repository.ID+"/impact-assessments/"+assessment.ID, "", collaborator.Credential.Token, http.StatusOK)
	var visible impacts.Assessment
	json.NewDecoder(read.Body).Decode(&visible)
	read.Body.Close()
	if len(visible.Participants) != 2 || visible.Version != 2 {
		t.Fatalf("invited assessment = %#v", visible)
	}
}

func TestImpactImplementationRetainsReasoningInOwnedTasks(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	impactStore, _ := impacts.New(t.TempDir())
	proposalStore, _ := proposals.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, proposalStore, nil, nil, nil, nil, impactStore))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "implementation-owner")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"reasoned-change"}`, owner.Credential.Token, http.StatusCreated)
	var repository repositories.Repository
	json.NewDecoder(response.Body).Decode(&repository)
	response.Body.Close()
	repo, _ := gitStore.Open(repository.ID)
	blob, _ := repo.WriteObject(storage.BlobObject, []byte("func Authorize() bool { return true }\n"))
	tree := writeTestTree(t, repo, testTreeEntry{"100644", "authorize.go", blob})
	commit := writeTestCommit(t, repo, tree, nil, 1700000000, "base")
	if err := repo.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(commit)}); err != nil {
		t.Fatal(err)
	}
	created := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/impact-assessments", `{"title":"Authorization risk","ref":"main","query":"Authorize","source":{"kind":"selected_code","path":"authorize.go","start_line":1,"end_line":1}}`, owner.Credential.Token, http.StatusCreated)
	var assessment impacts.Assessment
	json.NewDecoder(created.Body).Decode(&assessment)
	created.Body.Close()
	body := fmt.Sprintf(`{"version":%d,"title":"Implement authorization decision","body":"Preserve the validated risk and verification context.","item_ids":[%q],"tasks":[{"title":"Change authorization","outcome":"Update behavior and verify the cited call site.","assignee_type":"human","assignee_id":%q},{"title":"Verify consumers","outcome":"Run the frozen verification requirements.","assignee_type":"agent","depends_on_previous":true}]}`, assessment.Version, assessment.Items[0].ID, owner.User.ID)
	implemented := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/impact-assessments/"+assessment.ID+"/implementation", body, owner.Credential.Token, http.StatusCreated)
	var result struct {
		Assessment impacts.Assessment `json:"assessment"`
		Proposal   proposals.Proposal `json:"proposal"`
		Tasks      []proposals.Task   `json:"tasks"`
	}
	json.NewDecoder(implemented.Body).Decode(&result)
	implemented.Body.Close()
	if result.Assessment.Implementation == nil || len(result.Tasks) != 2 || result.Tasks[1].DependencyIDs[0] != result.Tasks[0].ID || result.Tasks[0].Reasoning == nil || result.Tasks[0].Reasoning.Revision != string(commit) || result.Proposal.Reasoning == nil {
		t.Fatalf("implementation provenance = %#v", result)
	}
	if result.Tasks[0].Assignment.AssigneeType != "human" || result.Tasks[1].Assignment.AssigneeType != "agent" {
		t.Fatalf("task ownership = %#v", result.Tasks)
	}
	retry := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/impact-assessments/"+assessment.ID+"/implementation", body, owner.Credential.Token, http.StatusOK)
	var recovered struct {
		Proposal  proposals.Proposal `json:"proposal"`
		Tasks     []proposals.Task   `json:"tasks"`
		Recovered bool               `json:"recovered"`
	}
	json.NewDecoder(retry.Body).Decode(&recovered)
	retry.Body.Close()
	if !recovered.Recovered || recovered.Proposal.ID != result.Proposal.ID || len(recovered.Tasks) != 2 || recovered.Tasks[0].ID != result.Tasks[0].ID {
		t.Fatalf("exact implementation retry = %#v", recovered)
	}
	newCommit := writeTestCommit(t, repo, tree, []storage.ObjectID{commit}, 1700000001, "later")
	if err := repo.UpdateReferenceIfTarget(storage.Reference{Name: "refs/heads/main", Target: string(newCommit)}, string(commit)); err != nil {
		t.Fatal(err)
	}
	read := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repository.ID+"/impact-assessments/"+assessment.ID, "", owner.Credential.Token, http.StatusOK)
	var changed impacts.Assessment
	json.NewDecoder(read.Body).Decode(&changed)
	read.Body.Close()
	if changed.ContextState != "changed" {
		t.Fatalf("context state = %q", changed.ContextState)
	}
}

func hasImpactKind(v impacts.Assessment, kind string) bool {
	for _, item := range v.Items {
		if item.Kind == kind {
			return true
		}
	}
	return false
}

func TestImpactAnalysisReportsScannerOmissions(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	repoRecord, err := catalog.Create("owner", "scanner-impact")
	if err != nil {
		t.Fatal(err)
	}
	repo, _ := gitStore.Open(repoRecord.ID)
	longLine := strings.Repeat("x", 70*1024) + "\nAuthorize()\n"
	blob, _ := repo.WriteObject(storage.BlobObject, []byte(longLine))
	tree := writeTestTree(t, repo, testTreeEntry{"100644", "scanner.go", blob})
	commit := writeTestCommit(t, repo, tree, nil, 1700000000, "scanner fixture")
	items, status, reason := deriveImpact(repo.Path(), repoRecord.ID, string(commit), "Authorize", catalog, nil, nil, nil, nil, "owner")
	if status != "incomplete" || !strings.Contains(reason, "scanner limit") {
		t.Fatalf("analysis = %q, %q, %#v", status, reason, items)
	}
}

func TestImpactConclusionMustMatchRequestedRevision(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	impactStore, _ := impacts.New(t.TempDir())
	explanationStore, _ := explanations.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, impactStore, explanationStore))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "impact-conclusion-owner")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"conclusion-impact"}`, owner.Credential.Token, http.StatusCreated)
	var repository repositories.Repository
	json.NewDecoder(response.Body).Decode(&repository)
	response.Body.Close()
	repo, _ := gitStore.Open(repository.ID)
	blobA, _ := repo.WriteObject(storage.BlobObject, []byte("func Authorize() bool { return true }\n"))
	treeA := writeTestTree(t, repo, testTreeEntry{"100644", "authorize.go", blobA})
	commitA := writeTestCommit(t, repo, treeA, nil, 1700000000, "revision A")
	if err := repo.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(commitA)}); err != nil {
		t.Fatal(err)
	}
	conversation, err := explanationStore.Create(explanations.Conversation{RepositoryID: repository.ID, Revision: string(commitA), Context: explanations.Context{Kind: "repository"}, Question: "How does authorization work?", AskedBy: owner.User.ID, Claims: []explanations.Claim{{Text: "Authorization is centralized", Basis: "evidence", Confidence: "high"}}})
	if err != nil {
		t.Fatal(err)
	}
	conversation, err = explanationStore.AddEntry(conversation.ID, explanations.Entry{Kind: "conclusion", Body: "Authorize is the behavior boundary", ActorID: owner.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	conclusion := conversation.Entries[len(conversation.Entries)-1]
	blobB, _ := repo.WriteObject(storage.BlobObject, []byte("func Permit() bool { return true }\n"))
	treeB := writeTestTree(t, repo, testTreeEntry{"100644", "authorize.go", blobB})
	commitB := writeTestCommit(t, repo, treeB, []storage.ObjectID{commitA}, 1700000001, "revision B")
	if err := repo.UpdateReferenceIfTarget(storage.Reference{Name: "refs/heads/main", Target: string(commitB)}, string(commitA)); err != nil {
		t.Fatal(err)
	}
	body := `{"title":"Assess conclusion","ref":"main","source":{"kind":"investigation_conclusion","explanation_id":"` + conversation.ID + `","entry_id":"` + conclusion.ID + `"}}`
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/impact-assessments", body, owner.Credential.Token, http.StatusConflict).Body.Close()
	values, _ := impactStore.List(repository.ID)
	if len(values) != 0 {
		t.Fatalf("mismatched conclusion persisted: %#v", values)
	}
}

func TestImpactAcknowledgementRejectsUnrelatedTargetsAndUnauthorizedOwners(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	impactStore, _ := impacts.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, impactStore))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "private-impact-owner")
	other := createTestAccount(t, server.URL, "unrelated-owner")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"private-impact"}`, owner.Credential.Token, http.StatusCreated)
	var source repositories.Repository
	json.NewDecoder(response.Body).Decode(&source)
	response.Body.Close()
	repo, _ := gitStore.Open(source.ID)
	blob, _ := repo.WriteObject(storage.BlobObject, []byte("func Change() {}\n"))
	tree := writeTestTree(t, repo, testTreeEntry{"100644", "change.go", blob})
	commit := writeTestCommit(t, repo, tree, nil, 1700000000, "source")
	repo.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(commit)})
	targetResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"unrelated"}`, other.Credential.Token, http.StatusCreated)
	var target repositories.Repository
	json.NewDecoder(targetResponse.Body).Decode(&target)
	targetResponse.Body.Close()
	authenticatedRequest(t, http.MethodPatch, server.URL+"/repositories/"+target.ID, `{"visibility":"public"}`, other.Credential.Token, http.StatusOK).Body.Close()
	created := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+source.ID+"/impact-assessments", `{"title":"Private change","ref":"main","query":"Change","source":{"kind":"proposed_diff","diff":"+Change()"}}`, owner.Credential.Token, http.StatusCreated)
	var assessment impacts.Assessment
	json.NewDecoder(created.Body).Decode(&assessment)
	created.Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+source.ID+"/impact-assessments/"+assessment.ID+"/acknowledgement-requests", `{"repository_id":"`+target.ID+`","version":1}`, owner.Credential.Token, http.StatusNotFound).Body.Close()
	// Seed a legacy/malicious request directly: the acknowledgement route must
	// still deny an owner who cannot read the private source assessment.
	seeded, err := impactStore.Request(assessment.ID, assessment.Version, impacts.AcknowledgementRequest{RepositoryID: target.ID, OwnerID: other.User.ID, RequestedBy: owner.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+source.ID+"/impact-assessments/"+assessment.ID+"/acknowledgement-requests/"+seeded.AcknowledgementRequests[0].ID, `{"version":2}`, other.Credential.Token, http.StatusNotFound).Body.Close()
	reopened, _ := impactStore.Get(assessment.ID)
	if reopened.AcknowledgementRequests[0].AcknowledgedBy != "" {
		t.Fatal("unauthorized owner acknowledged private assessment")
	}
}
