package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/decisions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
)

func TestHistoricalExperimentInvalidatesOnlyAfterLaunchBaselineMoves(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	ownerID := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	meta, err := catalog.Create(ownerID, "historical-experiment")
	if err != nil {
		t.Fatal(err)
	}
	repo, _ := gitStore.Open(meta.ID)
	definition := []byte(`{"version":1,"image":"alpine","tools":[],"dependencies":[],"setup":[],"experiments":[{"name":"bench","command":"true"}],"resources":{"cpus":1,"memory_mb":64,"storage_mb":32,"setup_seconds":30}}`)
	blob, _ := repo.WriteObject(storage.BlobObject, definition)
	definitionTree := writeTestTree(t, repo, testTreeEntry{mode: "100644", name: "workspace.json", id: blob})
	root := writeTestTree(t, repo, testTreeEntry{mode: "40000", name: ".vivarium", id: definitionTree})
	old := writeTestCommit(t, repo, root, nil, 1, "old")
	baseline := writeTestCommit(t, repo, root, []storage.ObjectID{old}, 2, "baseline")
	if err = repo.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(baseline)}); err != nil {
		t.Fatal(err)
	}
	workspaceStore, _ := workspaces.New(t.TempDir())
	workspace, _ := workspaceStore.Create(workspaces.Workspace{RepositoryID: meta.ID, CommitID: string(old), CreatorID: ownerID, Source: workspaces.Source{Kind: "decision_experiment", DecisionID: "decision", AlternativeID: "alternative"}, Definition: workspaces.Definition{Version: 1, Image: "alpine"}}, definition)
	workspace, _ = workspaceStore.Complete(workspace.ID, nil, false)
	digest := sha256.Sum256(definition)
	if _, _, err = currentDecisionBaseline(meta.ID, gitStore, catalog); err != nil {
		t.Fatalf("baseline: %v", err)
	}
	decision := decisions.Decision{RepositoryID: meta.ID, Experiments: []decisions.Experiment{{ID: "experiment", WorkspaceID: workspace.ID, Revision: string(old), DefinitionSHA256: workspace.DefinitionSHA256, DefaultBranchRevision: string(baseline), DefaultDefinitionSHA256: hex.EncodeToString(digest[:])}}}
	projected := projectDecisionExperiments(decision, gitStore, catalog, workspaceStore)
	if projected.Experiments[0].Invalidated {
		t.Fatalf("historical pin invalidated at launch = %#v", projected.Experiments[0].InvalidationReasons)
	}
	later := writeTestCommit(t, repo, root, []storage.ObjectID{baseline}, 3, "later")
	if err = repo.UpdateReferenceIfTarget(storage.Reference{Name: "refs/heads/main", Target: string(later)}, string(baseline)); err != nil {
		t.Fatal(err)
	}
	projected = projectDecisionExperiments(decision, gitStore, catalog, workspaceStore)
	if !projected.Experiments[0].Invalidated {
		t.Fatal("post-launch default branch movement was not reported")
	}
}

func TestDecisionAPIKeepsPendingContextCollaborativeAndVersioned(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	decisionStore, _ := decisions.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, decisionStore))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "decision-owner")
	peer := createTestAccount(t, server.URL, "decision-peer")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"decision-source"}`, owner.Credential.Token, http.StatusCreated)
	var repository repositories.Repository
	json.NewDecoder(response.Body).Decode(&repository)
	response.Body.Close()
	unknown := "ffffffffffffffffffffffffffffffff"
	invalid := fmt.Sprintf(`{"source":{"kind":"repository","resource_id":%q},"scope":{"question":"Who owns this?","constraints":["No downtime"],"success_measures":["An answer"],"deadline":"2026-09-01T00:00:00Z","affected_resources":[{"kind":"repository","repository_id":%q,"label":"Repository"}],"participants":[{"user_id":%q}],"owner_id":%q}}`, repository.ID, repository.ID, unknown, unknown)
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/decisions", invalid, owner.Credential.Token, http.StatusBadRequest).Body.Close()
	body := fmt.Sprintf(`{"source":{"kind":"repository","resource_id":%q},"scope":{"question":"How should requests be queued?","constraints":["No downtime"],"success_measures":["p95 below 100ms"],"deadline":"2026-09-01T00:00:00Z","affected_resources":[{"kind":"service","repository_id":%q,"label":"API"}],"participants":[{"user_id":%q},{"user_id":%q}],"owner_id":%q}}`, repository.ID, repository.ID, owner.User.ID, peer.User.ID, owner.User.ID)
	created := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/decisions", body, owner.Credential.Token, http.StatusCreated)
	var decision decisions.Decision
	json.NewDecoder(created.Body).Decode(&decision)
	created.Body.Close()
	if decision.Status != "pending" || decision.Source.Kind != "repository" || decision.Version != 1 {
		t.Fatalf("decision = %#v", decision)
	}
	authenticatedRequest(t, http.MethodGet, server.URL+"/decisions/"+decision.ID, "", peer.Credential.Token, http.StatusNotFound).Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/collaborators", `{"user_id":"`+peer.User.ID+`"}`, owner.Credential.Token, http.StatusCreated).Body.Close()
	discussed := authenticatedRequest(t, http.MethodPost, server.URL+"/decisions/"+decision.ID+"/discussion", `{"body":"We should compare backpressure behavior."}`, peer.Credential.Token, http.StatusCreated)
	json.NewDecoder(discussed.Body).Decode(&decision)
	discussed.Body.Close()
	if len(decision.History) != 2 || decision.History[1].ActorID != peer.User.ID {
		t.Fatalf("discussion = %#v", decision.History)
	}
	update := fmt.Sprintf(`{"expected_version":1,"summary":"Added an operability constraint","scope":{"question":"How should requests be queued?","constraints":["No downtime","Operators can drain queues"],"success_measures":["p95 below 100ms"],"deadline":"2026-09-01T00:00:00Z","affected_resources":[{"kind":"service","repository_id":%q,"label":"API"}],"participants":[{"user_id":%q},{"user_id":%q}],"owner_id":%q}}`, repository.ID, owner.User.ID, peer.User.ID, owner.User.ID)
	invalidUpdate := strings.Replace(update, peer.User.ID, unknown, 1)
	authenticatedRequest(t, http.MethodPut, server.URL+"/decisions/"+decision.ID, invalidUpdate, peer.Credential.Token, http.StatusBadRequest).Body.Close()
	changed := authenticatedRequest(t, http.MethodPut, server.URL+"/decisions/"+decision.ID, update, peer.Credential.Token, http.StatusOK)
	json.NewDecoder(changed.Body).Decode(&decision)
	changed.Body.Close()
	if decision.Version != 2 || len(decision.History) != 3 {
		t.Fatalf("changed = %#v", decision)
	}
	authenticatedRequest(t, http.MethodPut, server.URL+"/decisions/"+decision.ID, update, peer.Credential.Token, http.StatusConflict).Body.Close()
	listed := authenticatedRequest(t, http.MethodGet, server.URL+"/decisions?source_kind=repository&source_id="+repository.ID, "", peer.Credential.Token, http.StatusOK)
	var result struct {
		Decisions []decisions.Decision `json:"decisions"`
	}
	json.NewDecoder(listed.Body).Decode(&result)
	listed.Body.Close()
	if len(result.Decisions) != 1 {
		t.Fatalf("listed = %#v", result)
	}
}

func TestDecisionResearchCredentialIsReadOnlyAndDecisionBound(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	decisionStore, _ := decisions.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, decisionStore))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "research-owner")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"research-source"}`, owner.Credential.Token, http.StatusCreated)
	var repository repositories.Repository
	json.NewDecoder(response.Body).Decode(&repository)
	response.Body.Close()
	deadline := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	scope := decisions.Scope{Question: "Which queue?", Constraints: []string{"No downtime"}, SuccessMeasures: []string{"p95 under 100ms"}, Deadline: &deadline, AffectedResources: []decisions.Resource{{Kind: "repository", RepositoryID: repository.ID, Label: "API"}}, Participants: []decisions.Participant{{UserID: owner.User.ID}}, OwnerID: owner.User.ID}
	decision, err := decisionStore.Create(repository.ID, decisions.Source{Kind: "repository", ResourceID: repository.ID}, scope, owner.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	evidence := []decisions.Evidence{{Kind: "usage", ResourceID: "latency", Revision: "window:2026-08-10", Label: "p95"}}
	alternative := decisions.Alternative{Title: "FIFO", Summary: "Bound it", Assumptions: []string{"Bursty load"}, Tradeoffs: []string{"Reject overload"}, Risks: []string{"Retries"}, CompatibilityImpact: "None", Cost: "Two days", ExpectedOutcomes: []string{"Stable latency"}, Evidence: evidence, Criteria: []decisions.CriterionAssessment{{Criterion: "p95 under 100ms", Outcome: "82ms", Evidence: evidence}}}
	decision, err = decisionStore.AddAlternative(decision.ID, owner.User.ID, 1, alternative)
	if err != nil {
		t.Fatal(err)
	}
	issuedResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/decisions/"+decision.ID+"/research-credentials", fmt.Sprintf(`{"expires_in":600,"alternative_id":%q}`, decision.Alternatives[0].ID), owner.Credential.Token, http.StatusCreated)
	var issued auth.IssuedCredential
	json.NewDecoder(issuedResponse.Body).Decode(&issued)
	issuedResponse.Body.Close()
	authenticatedRequest(t, http.MethodGet, server.URL+"/decisions/"+decision.ID, "", issued.Token, http.StatusOK).Body.Close()
	finding := fmt.Sprintf(`{"alternative_id":%q,"body":"Retry evidence weakens this option.","position":"oppose","uncertainty":"One region only.","citations":[{"kind":"usage","resource_id":"latency","revision":"window:2026-08-10","label":"p95"}]}`, decision.Alternatives[0].ID)
	result := authenticatedRequest(t, http.MethodPost, server.URL+"/decisions/"+decision.ID+"/findings", finding, issued.Token, http.StatusCreated)
	json.NewDecoder(result.Body).Decode(&decision)
	result.Body.Close()
	if len(decision.Findings) != 1 || decision.Findings[0].Position != "oppose" {
		t.Fatalf("finding = %#v", decision.Findings)
	}
	authenticatedRequest(t, http.MethodPost, server.URL+"/decisions/"+decision.ID+"/discussion", `{"body":"cannot mutate"}`, issued.Token, http.StatusUnauthorized).Body.Close()
}
