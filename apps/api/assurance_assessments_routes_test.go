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
	payload, _ = json.Marshal(map[string]any{"program_id": program.ID, "program_version": 1, "title": "Independent controls review", "assessor": map[string]any{"user_id": outside.User.ID, "kind": "external", "organization": "Independent LLP", "conflict_disclosure": "none"}, "scope": map[string]any{"control_ids": []string{"control"}, "system_ids": []string{"repo"}, "release_ids": []string{}, "period_starts_at": now.Add(-24 * time.Hour), "period_ends_at": now}, "evidence_package_ids": []string{}, "starts_at": now, "expires_at": now.Add(24 * time.Hour)})
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

	base := assuranceassessments.Assessment{RepositoryID: repo.ID, ProgramID: program.ID, ProgramVersion: 1, Title: "Window regression", OwnerID: owner.User.ID, Assessor: assuranceassessments.Assessor{UserID: outside.User.ID, Kind: "external", ConflictDisclosure: "none"}, Scope: assuranceassessments.Scope{ControlIDs: []string{"control"}, PeriodStartsAt: now.Add(-time.Hour), PeriodEndsAt: now}}
	future := base
	future.StartsAt, future.ExpiresAt = time.Now().UTC().Add(time.Hour), time.Now().UTC().Add(2*time.Hour)
	future, createErr := assessments.Create(future)
	if createErr != nil {
		t.Fatal(createErr)
	}
	authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repo.ID+"/assurance-assessments/"+future.ID, "", outside.Credential.Token, http.StatusForbidden).Body.Close()
	payload, _ = json.Marshal(map[string]any{"expected_version": 1, "kind": "question", "body": "too early"})
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/assurance-assessments/"+future.ID+"/events", string(payload), outside.Credential.Token, http.StatusForbidden).Body.Close()
	dualFuture := base
	dualFuture.OwnerID, dualFuture.StartsAt, dualFuture.ExpiresAt = outside.User.ID, time.Now().UTC().Add(time.Hour), time.Now().UTC().Add(2*time.Hour)
	dualFuture, createErr = assessments.Create(dualFuture)
	if createErr != nil {
		t.Fatal(createErr)
	}
	authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repo.ID+"/assurance-assessments/"+dualFuture.ID, "", outside.Credential.Token, http.StatusOK).Body.Close()
	expired := base
	expired.StartsAt, expired.ExpiresAt = time.Now().UTC(), time.Now().UTC().Add(100*time.Millisecond)
	expired, createErr = assessments.Create(expired)
	if createErr != nil {
		t.Fatal(createErr)
	}
	time.Sleep(150 * time.Millisecond)
	authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repo.ID+"/assurance-assessments/"+expired.ID, "", outside.Credential.Token, http.StatusForbidden).Body.Close()
	dualExpired := base
	dualExpired.OwnerID, dualExpired.StartsAt, dualExpired.ExpiresAt = outside.User.ID, time.Now().UTC(), time.Now().UTC().Add(100*time.Millisecond)
	dualExpired, createErr = assessments.Create(dualExpired)
	if createErr != nil {
		t.Fatal(createErr)
	}
	time.Sleep(150 * time.Millisecond)
	authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repo.ID+"/assurance-assessments/"+dualExpired.ID, "", outside.Credential.Token, http.StatusOK).Body.Close()
	response = authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repo.ID+"/assurance-assessments", "", outside.Credential.Token, http.StatusOK)
	var visible struct {
		Assessments []assuranceassessments.Assessment `json:"assessments"`
	}
	_ = json.NewDecoder(response.Body).Decode(&visible)
	response.Body.Close()
	seen := map[string]bool{}
	for _, item := range visible.Assessments {
		seen[item.ID] = true
	}
	if len(visible.Assessments) != 3 || !seen[assessment.ID] || !seen[dualFuture.ID] || !seen[dualExpired.ID] || seen[future.ID] || seen[expired.ID] {
		t.Fatalf("list did not preserve owner access while filtering assessor-only windows: %#v", visible.Assessments)
	}
}

func TestAssessorWindowIncludesStartAndExcludesExpiry(t *testing.T) {
	now := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	a := assuranceassessments.Assessment{StartsAt: now, ExpiresAt: now.Add(time.Hour)}
	if assessorWindowOpen(a, now.Add(-time.Nanosecond)) {
		t.Fatal("window opened early")
	}
	if !assessorWindowOpen(a, now) {
		t.Fatal("window closed at exact start")
	}
	if assessorWindowOpen(a, a.ExpiresAt) {
		t.Fatal("window remained open at expiry")
	}
}

func TestAssuranceStatementScopeContainsOnlyValidatedControls(t *testing.T) {
	assessed := assuranceassessments.Scope{ControlIDs: []string{"resolved", "unresolved"}, SystemIDs: []string{"system"}, ReleaseIDs: []string{"release"}}
	signed := assuranceStatementScope(assessed, []string{"resolved"})
	if len(signed.ControlIDs) != 1 || signed.ControlIDs[0] != "resolved" || len(signed.SystemIDs) != 1 || len(assessed.ControlIDs) != 2 {
		t.Fatalf("signed scope was not narrowed without mutating assessment: %#v %#v", signed, assessed)
	}
}
