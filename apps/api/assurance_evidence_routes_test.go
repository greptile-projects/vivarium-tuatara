package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/assuranceevidence"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/assuranceprograms"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestAssuranceEvidenceBindsExactControlAndProjectsAudience(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	programs, _ := assuranceprograms.New(t.TempDir())
	evidence, _ := assuranceevidence.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, programs, evidence))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "evidence-owner")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"controlled"}`, owner.Credential.Token, http.StatusCreated)
	var repo repositories.Repository
	_ = json.NewDecoder(response.Body).Decode(&repo)
	response.Body.Close()
	authenticatedRequest(t, http.MethodPatch, server.URL+"/repositories/"+repo.ID, `{"visibility":"public"}`, owner.Credential.Token, http.StatusOK).Body.Close()
	revision := completeAssuranceRevision(owner.User.ID, repo.ID)
	payload, _ := json.Marshal(map[string]any{"revision": revision})
	response = authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/assurance-programs", string(payload), owner.Credential.Token, http.StatusCreated)
	var program assuranceprograms.Program
	_ = json.NewDecoder(response.Body).Decode(&program)
	response.Body.Close()
	now := time.Now().UTC()
	definition := assuranceevidence.Definition{ProgramID: program.ID, ProgramVersion: 1, ControlID: "control", Title: "Storage operation", PeriodStartsAt: now.Add(-time.Hour), PeriodEndsAt: now.Add(time.Hour), Schedule: "daily", Audience: []string{owner.User.ID}, Queries: []assuranceevidence.Query{{ID: "check", Kind: "check", Required: true, MaxAgeHours: 24}}}
	body, _ := json.Marshal(definition)
	response = authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/assurance-evidence/definitions", string(body), owner.Credential.Token, http.StatusCreated)
	var created assuranceevidence.Definition
	_ = json.NewDecoder(response.Body).Decode(&created)
	response.Body.Close()
	if created.ProgramVersion != 1 || created.OwnerID != owner.User.ID {
		t.Fatalf("definition = %#v", created)
	}
	sources, _ := json.Marshal(map[string]any{"sources": []assuranceevidence.Source{{QueryID: "check", Kind: "check", ResourceID: "run-1", Revision: "commit", OccurredAt: now, Provenance: "retained check run", Summary: "passed", Accessible: true}}})
	response = authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/assurance-evidence/definitions/"+created.ID+"/packages", string(sources), owner.Credential.Token, http.StatusCreated)
	var pkg assuranceevidence.Package
	_ = json.NewDecoder(response.Body).Decode(&pkg)
	response.Body.Close()
	if pkg.Coverage != 100 || pkg.ManifestHash == "" {
		t.Fatalf("package = %#v", pkg)
	}
	outsider := createTestAccount(t, server.URL, "evidence-outsider")
	listed := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repo.ID+"/assurance-evidence", "", outsider.Credential.Token, http.StatusOK)
	var projection struct {
		Definitions []assuranceevidence.Definition `json:"definitions"`
		Packages    []assuranceevidence.Package    `json:"packages"`
	}
	_ = json.NewDecoder(listed.Body).Decode(&projection)
	listed.Body.Close()
	if len(projection.Definitions) != 0 || len(projection.Packages) != 0 {
		t.Fatalf("private evidence leaked: %#v", projection)
	}
}
