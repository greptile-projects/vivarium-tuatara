package main

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/docscollections"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
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

func TestCollaboratorEscalatesRestrictedSupportIntoOrderedGovernedWork(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	supportStore, _ := supportthreads.New(t.TempDir())
	issueStore, _ := issues.New(t.TempDir())
	proposalStore, _ := proposals.New(t.TempDir())
	documentationStore, _ := docscollections.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, proposalStore, nil, nil, nil, nil, issueStore, supportStore, documentationStore))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "support-escalation-owner")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"sdk"}`, owner.Credential.Token, http.StatusCreated)
	var repo repositories.Repository
	decodeResponse(t, response, &repo)
	bare, _ := gitStore.Open(repo.ID)
	base := writeCommit(t, bare, 1700000000, "support escalation base")
	if err := bare.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(base)}); err != nil {
		t.Fatal(err)
	}
	response = authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/support-threads", `{"title":"SDK retries lose uploads","body":"A private customer identifier is present only in the attached log.","target":{"kind":"package","label":"sdk","version":"3.2"},"environment":{"runtime":"Go 1.26"},"goal":"Retries preserve one upload.","attempted_steps":["run the retry sample","observe two uploads"],"urgency":"high","audience":"maintainers","contact_preferences":{"reply_in_thread":true},"attachments":[{"kind":"log","name":"private.log","media_type":"text/plain","data":"c2Vuc2l0aXZl"}]}`, owner.Credential.Token, http.StatusCreated)
	var thread supportthreads.Thread
	decodeResponse(t, response, &thread)
	body := `{"classification":"compatibility_problem","resource_kind":"ordered_work","expected_version":1,"acceptance_criteria":["one upload is retained","the retry journey is documented"],"tasks":[{"title":"Fix retry idempotency","outcome":"one upload is retained","risk":"duplicate writes","verification_plan":"run the bounded reproduction","assignee_type":"agent"},{"title":"Publish retry example","outcome":"the retry journey is documented","risk":"stale guidance","verification_plan":"review the documentation preview","assignee_type":"human","assignee_id":"` + owner.User.ID + `"}]}`
	response = authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/support-threads/"+thread.ID+"/escalations", body, owner.Credential.Token, http.StatusCreated)
	decodeResponse(t, response, &thread)
	if len(thread.Escalations) != 1 || thread.Escalations[0].AffectedVersion != "3.2" || len(thread.Escalations[0].Reproduction) != 2 {
		t.Fatalf("escalation = %#v", thread.Escalations)
	}
	proposal, err := proposalStore.Get(repo.ID, thread.Escalations[0].ResourceID)
	if err != nil || proposal.Reasoning == nil || proposal.Reasoning.SupportThreadID != thread.ID {
		t.Fatalf("proposal = %#v, err = %v", proposal, err)
	}
	tasks, _ := proposalStore.ListTasks(repo.ID, proposal.ID)
	if len(tasks) != 2 || len(tasks[1].DependencyIDs) != 1 || tasks[1].DependencyIDs[0] != tasks[0].ID || len(tasks[0].Assignment.Access.Scopes) != 2 || len(tasks[1].Assignment.Access.Scopes) != 0 {
		t.Fatalf("tasks = %#v", tasks)
	}
	if strings.Contains(proposal.Body, "sensitive") || strings.Contains(proposal.Body, "private.log") {
		t.Fatalf("restricted attachment leaked into proposal: %s", proposal.Body)
	}
}
