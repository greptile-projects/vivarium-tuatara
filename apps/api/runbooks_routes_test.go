package main

import (
	"encoding/json"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/runbooks"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRunbookAPI(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	bookStore, _ := runbooks.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, bookStore))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "runbook-owner")
	resp := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"procedures"}`, owner.Credential.Token, http.StatusCreated)
	var repo repositories.Repository
	_ = json.NewDecoder(resp.Body).Decode(&repo)
	resp.Body.Close()
	revision := runbooks.Revision{Title: "Recovery", Purpose: "Safe recovery", Scope: runbooks.Scope{Kind: "service", ResourceID: "checkout", Name: "Checkout"}, Preconditions: []string{"signal confirmed"}, RollbackCriteria: []string{"impact rises"}, OwnerIDs: []string{owner.User.ID}, RequiredSkills: []string{"operations"}, ChangeReason: "initial", Steps: []runbooks.Step{{ID: "inspect", Position: 1, Kind: "diagnostic", Title: "Inspect", Purpose: "verify", Instructions: "read health", Preconditions: []string{"access"}, ExpectedEvidence: []string{"digest"}, OwnerIDs: []string{owner.User.ID}, RequiredSkills: []string{"operations"}, References: []runbooks.Reference{{Kind: "documentation", ResourceID: "ops.md", Revision: "abc", Reviewed: true, Accessible: true}, {Kind: "command", ResourceID: "missing-command.sh", Revision: "abc", Reviewed: true, Accessible: true}, {Kind: "agent", ResourceID: "missing-agent", Revision: "1", Approved: true, Accessible: true}}, Authority: runbooks.Authority{RequiredAccess: []string{"service:read"}, Inspects: []string{"health"}, ProhibitedActions: []string{"deploy"}}}}}
	payload, _ := json.Marshal(map[string]any{"request_id": "stable-create", "revision": revision})
	createdResp := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/runbooks", string(payload), owner.Credential.Token, http.StatusCreated)
	var created runbooks.Runbook
	_ = json.NewDecoder(createdResp.Body).Decode(&created)
	createdResp.Body.Close()
	if created.CurrentVersion != 1 || len(created.Diagnostics) != 5 {
		t.Fatalf("created=%+v", created)
	}
	for _, ref := range created.Revisions[0].Steps[0].References {
		if ref.Accessible || ref.Reviewed || ref.Approved {
			t.Fatalf("caller reference status survived: %+v", ref)
		}
	}
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/runbooks", string(payload), owner.Credential.Token, http.StatusCreated).Body.Close()
	listed := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repo.ID+"/runbooks", "", owner.Credential.Token, http.StatusOK)
	var result struct {
		Runbooks []runbooks.Runbook `json:"runbooks"`
	}
	_ = json.NewDecoder(listed.Body).Decode(&result)
	listed.Body.Close()
	if len(result.Runbooks) != 1 {
		t.Fatalf("list=%+v", result)
	}
}
