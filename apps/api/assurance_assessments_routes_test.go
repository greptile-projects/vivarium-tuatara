package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/assuranceassessments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/assuranceevidence"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/assuranceprograms"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestIndependentAssessorHasOnlyBoundedAssessmentAccess(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	programs, _ := assuranceprograms.New(t.TempDir())
	evidence, _ := assuranceevidence.New(t.TempDir())
	assessments, _ := assuranceassessments.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, programs, evidence, assessments))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "assessment-owner")
	outside := createTestAccount(t, server.URL, "independent-assessor")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"assessed"}`, owner.Credential.Token, http.StatusCreated)
	var repo repositories.Repository
	_ = json.NewDecoder(response.Body).Decode(&repo)
	response.Body.Close()
	revision := completeAssuranceRevision(owner.User.ID, repo.ID)
	payload, _ := json.Marshal(map[string]any{"revision": revision})
	response = authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/assurance-programs", string(payload), owner.Credential.Token, http.StatusCreated)
	var program assuranceprograms.Program
	_ = json.NewDecoder(response.Body).Decode(&program)
	response.Body.Close()
	now := time.Now().UTC()
	payload, _ = json.Marshal(map[string]any{"program_id": program.ID, "program_version": 1, "title": "Independent controls review", "assessor": map[string]any{"user_id": outside.User.ID, "kind": "external", "organization": "Independent LLP", "conflict_disclosure": "none"}, "scope": map[string]any{"control_ids": []string{"control"}, "system_ids": []string{"repo"}, "release_ids": []string{}, "period_starts_at": now.Add(-24 * time.Hour), "period_ends_at": now}, "evidence_package_ids": []string{}, "starts_at": now.Add(time.Second), "expires_at": now.Add(24 * time.Hour)})
	response = authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/assurance-assessments", string(payload), owner.Credential.Token, http.StatusCreated)
	var assessment assuranceassessments.Assessment
	_ = json.NewDecoder(response.Body).Decode(&assessment)
	response.Body.Close()
	// A private repository remains unavailable, while the explicit assessment view is readable.
	authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repo.ID, "", outside.Credential.Token, http.StatusNotFound).Body.Close()
	response = authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repo.ID+"/assurance-assessments/"+assessment.ID, "", outside.Credential.Token, http.StatusOK)
	response.Body.Close()
	payload, _ = json.Marshal(map[string]any{"expected_version": assessment.Version, "kind": "question", "body": "How was the encryption claim sampled?", "control_id": "control"})
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/assurance-assessments/"+assessment.ID+"/events", string(payload), outside.Credential.Token, http.StatusCreated).Body.Close()
	payload, _ = json.Marshal(map[string]any{"expected_version": assessment.Version + 1, "kind": "response", "body": "attempt project-side response"})
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/assurance-assessments/"+assessment.ID+"/events", string(payload), outside.Credential.Token, http.StatusForbidden).Body.Close()
}
