package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/accessibilityreports"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestAccessibilityReportAPIProtectsReporterAndRetainsExactAttempt(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	reports, _ := accessibilityreports.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, reports))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "barrier-owner")
	reporter := createTestAccount(t, server.URL, "barrier-reporter")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"accessible"}`, owner.Credential.Token, http.StatusCreated)
	var repo repositories.Repository
	json.NewDecoder(response.Body).Decode(&repo)
	response.Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/collaborators", `{"user_id":"`+reporter.User.ID+`"}`, owner.Credential.Token, http.StatusCreated).Body.Close()
	payload := `{"target":{"kind":"page","resource_id":"settings","revision":"0123456789abcdef"},"access_needs":["keyboard only"],"expected_outcome":"focus reaches Save","steps":["press Tab"],"reporter_environment":{"browser":"Firefox","browser_version":"128","device":"personal switch","operating_system":"Linux","assistive_technology":"Orca","assistive_technology_version":"47"},"evidence":[{"kind":"screenshot","description":"redacted focus image","content_ref":"artifact://focus","redacted":true}],"consent":{"share_identity":false,"share_device_details":false}}`
	createdResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/accessibility-reports", payload, reporter.Credential.Token, http.StatusCreated)
	var created accessibilityreports.Report
	json.NewDecoder(createdResponse.Body).Decode(&created)
	createdResponse.Body.Close()
	listed := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repo.ID+"/accessibility-reports", "", owner.Credential.Token, http.StatusOK)
	var collection struct {
		Reports []accessibilityreports.Report `json:"reports"`
	}
	json.NewDecoder(listed.Body).Decode(&collection)
	listed.Body.Close()
	if len(collection.Reports) != 1 || collection.Reports[0].ReporterID != "" || collection.Reports[0].ReporterEnvironment.Device != "" {
		t.Fatalf("projection = %+v", collection.Reports)
	}
	attempt := `{"boundary":"workspace","environment":{"browser":"Firefox","device":"desktop","operating_system":"Linux","assistive_technology":"Orca"},"outcome":"reproducible","notes":"focus skips Save","evidence":[]}`
	result := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/accessibility-reports/"+created.ID+"/attempts", attempt, owner.Credential.Token, http.StatusCreated)
	var updated accessibilityreports.Report
	json.NewDecoder(result.Body).Decode(&updated)
	result.Body.Close()
	if len(updated.Attempts) != 1 || updated.Attempts[0].Revision != "0123456789abcdef" {
		t.Fatalf("attempt = %+v", updated.Attempts)
	}
}
