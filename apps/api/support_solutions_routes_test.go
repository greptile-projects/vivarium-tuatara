package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/knowledgeanswers"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/supportsolutions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/supportthreads"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/supportverifications"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
)

func TestSupportSolutionRequiresCurrentPassingRevisionAndPreservesSearchableScope(t *testing.T) {
	git, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), git)
	threads, _ := supportthreads.New(t.TempDir())
	answers, _ := knowledgeanswers.New(t.TempDir())
	attempts, _ := supportverifications.New(t.TempDir())
	workspaceStore, _ := workspaces.New(t.TempDir())
	solutions, _ := supportsolutions.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(git, identities, credentials, catalog, nil, nil, nil, nil, nil, threads, attempts, solutions, answers, workspaceStore))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "solution-owner")
	var repo repositories.Repository
	decodeResponse(t, authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"support-solutions"}`, owner.Credential.Token, http.StatusCreated), &repo)
	authenticatedRequest(t, http.MethodPatch, server.URL+"/repositories/"+repo.ID, `{"visibility":"public"}`, owner.Credential.Token, http.StatusOK).Body.Close()
	thread, err := threads.Create(supportthreads.Thread{RepositoryID: repo.ID, AuthorID: owner.User.ID, Title: "How do retries work?", Body: "Uploads time out.", Target: supportthreads.Target{Kind: "api", Label: "uploads", Version: "2.1"}, Environment: supportthreads.Environment{Runtime: "Go 1.26"}, Urgency: "normal", Audience: "public", ContactPreferences: supportthreads.ContactPreferences{ReplyInThread: true}})
	if err != nil {
		t.Fatal(err)
	}
	answer, err := answers.Create(knowledgeanswers.Answer{RepositoryID: repo.ID, Question: thread.Title, Audience: "public"}, knowledgeanswers.Revision{Summary: "Use idempotency keys", Body: "Reuse the same idempotency key when retrying.", AuthorID: owner.User.ID, AuthorType: "human", Claims: []knowledgeanswers.Claim{{Text: "Retries are safe.", Confidence: "high", Citations: []knowledgeanswers.Citation{{Kind: "support_thread", ResourceID: thread.ID, Label: "question", ApplicableVersions: []string{"2.1"}}}}}})
	if err != nil {
		t.Fatal(err)
	}
	ws, err := workspaceStore.Create(workspaces.Workspace{RepositoryID: repo.ID, CommitID: "0123456789012345678901234567890123456789", CreatorID: owner.User.ID, Source: workspaces.Source{Kind: "support_verification", SupportThreadID: thread.ID, AnswerID: answer.ID, AnswerRevisionID: answer.CurrentRevisionID}}, []byte("definition"))
	if err != nil {
		t.Fatal(err)
	}
	proof, err := attempts.Create(supportverifications.Attempt{RepositoryID: repo.ID, ThreadID: thread.ID, AnswerID: answer.ID, AnswerRevisionID: answer.CurrentRevisionID, WorkspaceID: ws.ID, CommitID: ws.CommitID, DefinitionSHA256: ws.DefinitionSHA256, SoftwareVersion: "2.1", Environment: supportverifications.Environment{Runtime: "Go 1.26"}, InputSHA256: sha256Text("inputs"), Instructions: "Reuse the same idempotency key when retrying.", InstructionsSHA256: sha256Text("instructions"), Commands: []supportverifications.Command{{Command: "go test ./...", OutcomeID: "outcome"}}, Result: "passed", ActorID: owner.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"answer_id": answer.ID, "answer_revision_id": answer.CurrentRevisionID, "verification_attempt_id": proof.ID, "title": "Safe upload retries", "summary": "Retry without duplicate bytes.", "audience": "public", "applicable_versions": []string{"2.1"}, "limitations": []string{"Not for 1.x"}, "links": []map[string]string{{"kind": "search", "label": "upload retry"}}})
	var published supportsolutions.Solution
	decodeResponse(t, authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/support-threads/"+thread.ID+"/solutions", string(body), owner.Credential.Token, http.StatusCreated), &published)
	if published.AnswerRevisionID != answer.CurrentRevisionID || published.Instructions == "" || len(published.Credits) == 0 || len(published.Notifications) == 0 {
		t.Fatalf("published = %#v", published)
	}
	var retry supportsolutions.Solution
	decodeResponse(t, authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/support-threads/"+thread.ID+"/solutions", string(body), owner.Credential.Token, http.StatusCreated), &retry)
	if retry.ID != published.ID {
		t.Fatalf("retry created duplicate %s after %s", retry.ID, published.ID)
	}
	var search struct {
		Solutions []supportsolutions.Solution `json:"solutions"`
	}
	decodeResponse(t, authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repo.ID+"/support-solutions?q=duplicate", "", owner.Credential.Token, http.StatusOK), &search)
	if len(search.Solutions) != 1 || search.Solutions[0].ID != published.ID {
		t.Fatalf("search = %#v", search)
	}
}
