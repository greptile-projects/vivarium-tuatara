package main

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/supportthreads"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestPublicSupportQuestionRetainsContextAndPrivateSuggestions(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	supportStore, _ := supportthreads.New(t.TempDir())
	issueStore, _ := issues.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, issueStore, supportStore))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "support-owner")
	developer := createTestAccount(t, server.URL, "support-developer")
	reader := createTestAccount(t, server.URL, "support-reader")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"client-kit"}`, owner.Credential.Token, http.StatusCreated)
	var repo repositories.Repository
	decodeResponse(t, response, &repo)
	authenticatedRequest(t, http.MethodPatch, server.URL+"/repositories/"+repo.ID, `{"visibility":"public"}`, owner.Credential.Token, http.StatusOK).Body.Close()
	body := `{"title":"Upload retry after timeout","body":"The upload API stops after a timeout.","target":{"kind":"api","label":"Upload API","version":"2.1"},"environment":{"operating_system":"Ubuntu 24.04","runtime":"Go 1.26"},"goal":"Resume without sending the file twice.","attempted_steps":["enabled retries","replayed the request"],"urgency":"high","audience":"public","contact_preferences":{"reply_in_thread":true,"email":"developer@example.test","allow_maintainer_contact":true},"attachments":[{"kind":"log","name":"client.log","media_type":"text/plain","data":"dGltZW91dA=="}]}`
	response = authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/support-threads", body, developer.Credential.Token, http.StatusCreated)
	var thread supportthreads.Thread
	decodeResponse(t, response, &thread)
	if thread.AuthorID != developer.User.ID || thread.Attachments[0].Size != 7 || len(thread.Diagnostics) != 0 || thread.Status != "open" {
		t.Fatalf("thread = %#v", thread)
	}
	boundaryData := base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", 1<<20)))
	boundaryBody := `{"title":"Boundary log","body":"A complete diagnostic log.","target":{"kind":"repository","label":"client-kit"},"urgency":"normal","audience":"public","contact_preferences":{"reply_in_thread":true},"attachments":[{"kind":"log","name":"complete.log","media_type":"text/plain","data":"` + boundaryData + `"}]}`
	response = authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/support-threads", boundaryBody, developer.Credential.Token, http.StatusCreated)
	var boundary supportthreads.Thread
	decodeResponse(t, response, &boundary)
	if boundary.Attachments[0].Size != 1<<20 {
		t.Fatalf("boundary attachment size = %d", boundary.Attachments[0].Size)
	}
	response = authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repo.ID+"/support-threads/"+thread.ID, "", reader.Credential.Token, http.StatusOK)
	var projected supportthreads.Thread
	decodeResponse(t, response, &projected)
	if projected.ContactPreferences.Email != "" {
		t.Fatal("public reader received private contact email")
	}
	authenticatedRequest(t, http.MethodPatch, server.URL+"/repositories/"+repo.ID+"/support-threads/"+thread.ID, `{"status":"answered","expected_version":`+strconv.Itoa(thread.Version)+`}`, developer.Credential.Token, http.StatusForbidden).Body.Close()
	response = authenticatedRequest(t, http.MethodPatch, server.URL+"/repositories/"+repo.ID+"/support-threads/"+thread.ID, `{"status":"answered","expected_version":`+strconv.Itoa(thread.Version)+`,"message":"Documented retry guidance applies."}`, owner.Credential.Token, http.StatusOK)
	decodeResponse(t, response, &thread)
	if thread.Status != "answered" || len(thread.History) != 2 {
		t.Fatalf("answered = %#v", thread)
	}
}
