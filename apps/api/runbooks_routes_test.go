package main

import (
	"encoding/json"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/responsealerts"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/responsepolicies"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/runbooks"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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
	revision.OutcomeCriteria = []runbooks.OutcomeCriterion{{Kind: "health", Criterion: "stable"}, {Kind: "containment", Criterion: "contained"}, {Kind: "recovery", Criterion: "recovered"}, {Kind: "communication", Criterion: "updated"}, {Kind: "rollback", Criterion: "safe"}}
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
	now := time.Now().UTC()
	context := runbooks.ExecutionContext{OriginKind: "manual_observation", OriginID: "observation-1", OriginRevision: "v1", Summary: "checkout unavailable", AffectedResources: []string{"checkout"}, WindowFrom: now, WindowTo: now, Evidence: []runbooks.ExecutionEvidence{{Kind: "observation", ResourceID: "checkout", Revision: "v1", Digest: "sha256:evidence", Summary: "bounded evidence"}}}
	launch, _ := json.Marshal(map[string]any{"request_id": "client-assertions", "runbook_version": 1, "context": context, "preconditions": []runbooks.Preconditions{{Condition: "signal confirmed", Status: "met", EvidenceDigest: "sha256:evidence"}}, "current_access": []string{"service:read"}})
	launchResp := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/runbooks/"+created.ID+"/executions", string(launch), owner.Credential.Token, http.StatusCreated)
	var execution runbooks.Execution
	_ = json.NewDecoder(launchResp.Body).Decode(&execution)
	launchResp.Body.Close()
	if execution.Status != "blocked" || len(execution.CurrentAccess) != 0 || len(execution.Preconditions) != 0 {
		t.Fatalf("client assertions established readiness: %+v", execution)
	}
	kinds := map[string]bool{}
	for _, blocker := range execution.Blockers {
		kinds[blocker.Kind] = true
	}
	if !kinds["precondition_not_met"] || !kinds["access_unavailable"] {
		t.Fatalf("blockers=%+v", execution.Blockers)
	}
}

func TestVerifyAlertRunbookLaunchRequiresCompleteResourceBinding(t *testing.T) {
	now := time.Now().UTC()
	alerts, err := responsealerts.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	policy := responsepolicies.Policy{ID: "policy", RepositoryID: "repo", Revisions: []responsepolicies.Revision{{Version: 1, Rules: []responsepolicies.Rule{{ID: "rule", ResourceIDs: []string{"checkout"}, SignalClass: "reliability", Severity: "high", AccountableTeamID: "ops", AcknowledgeSeconds: 60, ResolveSeconds: 600}}}}}
	signal := responsealerts.Signal{SignalClass: "reliability", Severity: "high", ResourceIDs: []string{"checkout"}, Summary: "degraded", Uncertainty: "sampled", OccurredAt: now, SourceRevision: "0123456789012345678901234567890123456789", Evidence: []responsealerts.Evidence{{Kind: "metric", ResourceID: "checkout", Revision: "sample", Digest: "sha256:alert", Summary: "bounded", Available: true}}}
	alert, err := alerts.Create("repo", "source", "request", signal, policy, []string{"operator"})
	if err != nil {
		t.Fatal(err)
	}
	book := runbooks.Runbook{RepositoryID: "repo", Revisions: []runbooks.Revision{{Preconditions: []string{"Exact alert remains active"}}}}
	context := runbooks.ExecutionContext{OriginKind: "alert", OriginID: alert.ID, OriginRevision: signal.SourceRevision, AffectedResources: []string{"checkout"}, WindowFrom: now.Add(-time.Minute), WindowTo: now.Add(time.Minute)}
	preconditions, access := verifyAlertRunbookLaunch(alerts, book, 1, context)
	if len(preconditions) != 1 || len(access) != 1 {
		t.Fatalf("exact alert binding was not verified: preconditions=%+v access=%+v", preconditions, access)
	}
	context.AffectedResources = append(context.AffectedResources, "billing")
	preconditions, access = verifyAlertRunbookLaunch(alerts, book, 1, context)
	if len(preconditions) != 0 || len(access) != 0 {
		t.Fatalf("mixed-resource context inherited alert readiness: preconditions=%+v access=%+v", preconditions, access)
	}
}

func TestRunbookAgentGrantRequiresCurrentUndeniedRepositoryAuthority(t *testing.T) {
	now := time.Now().UTC()
	resource := organizations.ResourceScope{Kind: "repository", ID: "repo"}
	base := organizations.AccessGrant{PrincipalType: "agent", PrincipalID: "agent", Resources: []organizations.ResourceScope{resource}}
	if !runbookAgentGrantCurrent(base, "agent", resource, now) {
		t.Fatal("current grant rejected")
	}
	expired := base
	expiry := now
	expired.ExpiresAt = &expiry
	if runbookAgentGrantCurrent(expired, "agent", resource, now) {
		t.Fatal("expired grant approved")
	}
	denied := base
	denied.Exceptions = []organizations.AccessException{{Resource: resource, Reason: "denied"}}
	if runbookAgentGrantCurrent(denied, "agent", resource, now) {
		t.Fatal("repository-denied grant approved")
	}
}
