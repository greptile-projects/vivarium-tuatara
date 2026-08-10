package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/impacts"
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

func hasImpactKind(v impacts.Assessment, kind string) bool {
	for _, item := range v.Items {
		if item.Kind == kind {
			return true
		}
	}
	return false
}
