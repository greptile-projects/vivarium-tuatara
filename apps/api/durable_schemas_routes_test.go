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
	"github.com/greptile-projects/vivarium-tuatara/apps/api/changesessions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/durableschemas"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
)

func TestBindRehearsalRunDerivesEvidenceFromExactCommand(t *testing.T) {
	command := "./scripts/rehearse upgrade"
	invariantCommand := "./scripts/rehearse verify-invariant"
	digest, invariantDigest := sha256.Sum256([]byte(command)), sha256.Sum256([]byte(invariantCommand))
	started := time.Now().UTC()
	ws := workspaces.Workspace{ID: "workspace", Commands: []workspaces.CommandOutcome{{ID: "command", CommandSHA256: hex.EncodeToString(digest[:]), ExitCode: 0, Output: "upgrade completed", StartedAt: started, CompletedAt: started.Add(25 * time.Millisecond)}, {ID: "invariant", CommandSHA256: hex.EncodeToString(invariantDigest[:]), ExitCode: 9, Output: "balances changed", StartedAt: started, CompletedAt: started.Add(10 * time.Millisecond)}}}
	rehearsal := durableschemas.Rehearsal{CreatedAt: started.Add(-time.Second), Checks: []durableschemas.RehearsalCheck{{ID: "upgrade", Command: command, Invariant: "balances unchanged", InvariantCommand: invariantCommand}}}
	forged := durableschemas.RehearsalRun{Result: "passed", Outcomes: []durableschemas.RehearsalOutcome{{CheckID: "upgrade", Status: "passed", InvariantPassed: true}}}
	bound, ok := bindRehearsalRun(ws, rehearsal, forged)
	if !ok || bound.Result != "failed" || bound.Outcomes[0].Status != "failed" || bound.Outcomes[0].ExitCode != 0 || bound.Outcomes[0].InvariantPassed || !strings.Contains(bound.Outcomes[0].SanitizedLog, "balances changed") || bound.Outcomes[0].DurationMS != 35 || len(bound.Attestations) != 2 || !strings.Contains(bound.Attestations[1], "invariant_outcome:invariant") {
		t.Fatalf("retained failure was not canonical: %#v, %v", bound, ok)
	}
	ws.Commands[1].ExitCode = 0
	ws.Commands[1].StartedAt, ws.Commands[1].CompletedAt = rehearsal.CreatedAt.Add(-time.Minute), rehearsal.CreatedAt.Add(-time.Minute+time.Second)
	if _, ok = bindRehearsalRun(ws, rehearsal, forged); ok {
		t.Fatal("pre-rehearsal invariant evidence was accepted")
	}
	ws.Commands[1].StartedAt, ws.Commands[1].CompletedAt = started, started.Add(10*time.Millisecond)
	ws.Commands = append(ws.Commands, ws.Commands[0])
	if _, ok = bindRehearsalRun(ws, rehearsal, forged); ok {
		t.Fatal("ambiguous repeated execution was accepted")
	}
	forged.Outcomes[0].RowsAfter = 1_000_000
	forged.Outcomes[0].ArtifactDigests = []string{"fabricated"}
	forged.Outcomes[0].CostUnits = 1
	forged.Attestations = []string{"fabricated"}
	if safeRehearsalRunRequest(forged) {
		t.Fatal("caller-supplied operational evidence was accepted")
	}
}

func TestDurableSchemaDefinitionResolvesExactReviewedBlob(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	repository, _ := gitStore.Create("schema-paths")
	definition, _ := repository.WriteObject(storage.BlobObject, []byte("create table orders(id uuid primary key);\n"))
	tree := writeTestTree(t, repository, testTreeEntry{mode: "100644", name: "orders.sql", id: definition})
	commit := writeTestCommit(t, repository, tree, nil, 1, "review schema")
	revision := durableschemas.Revision{ReviewedCommit: string(commit), DefinitionPath: "orders.sql", Definition: "create table orders(id uuid primary key);\n"}
	if !durableSchemaDefinitionResolves(gitStore, "schema-paths", revision) {
		t.Fatal("exact reviewed definition rejected")
	}
	revision.Definition = "drop table orders;"
	if durableSchemaDefinitionResolves(gitStore, "schema-paths", revision) {
		t.Fatal("caller definition that differs from reviewed blob accepted")
	}
	revision.Definition = "create table orders(id uuid primary key);\n"
	revision.DefinitionPath = "../orders.sql"
	if durableSchemaDefinitionResolves(gitStore, "schema-paths", revision) {
		t.Fatal("traversal definition path accepted")
	}
}

func TestDurableMigrationWorkCreatesScopedOrdinaryTaskWithExactContract(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	proposalStore, _ := proposals.New(t.TempDir())
	pullStore, _ := pullrequests.New(t.TempDir(), gitStore)
	sessionStore, _ := changesessions.New(t.TempDir())
	durableStore, _ := durableschemas.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, proposalStore, pullStore, nil, sessionStore, nil, durableStore))
	defer server.Close()
	provider := createTestAccount(t, server.URL, "durable-provider")
	consumer := createTestAccount(t, server.URL, "durable-consumer")
	observer := createTestAccount(t, server.URL, "durable-observer")
	createRepo := func(name, token string) (repositories.Repository, storage.ObjectID) {
		response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"`+name+`"}`, token, http.StatusCreated)
		var repo repositories.Repository
		decodeResponse(t, response, &repo)
		gitRepo, _ := gitStore.Open(repo.ID)
		commit := writeCommit(t, gitRepo, 1700000000, name)
		return repo, commit
	}
	source, _ := createRepo("durable-source", provider.Credential.Token)
	target, targetCommit := createRepo("durable-target", consumer.Credential.Token)
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+source.ID+"/collaborators", `{"user_id":"`+observer.User.ID+`"}`, provider.Credential.Token, http.StatusCreated).Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+target.ID+"/collaborators", `{"user_id":"`+provider.User.ID+`"}`, consumer.Credential.Token, http.StatusCreated).Body.Close()
	if _, err := catalog.SetVisibility(provider.User.ID, source.ID, repositories.Public); err != nil {
		t.Fatal(err)
	}
	revision := durableschemas.Revision{Name: "orders", StoreKind: "database", Description: "orders", Definition: "v1", DefinitionPath: "orders.sql", OwnerIDs: []string{provider.User.ID}, Compatibility: []string{"expand-contract"}, Retention: "seven years", Privacy: []string{"restricted"}, PullRequestID: "pull", ReviewedCommit: string(targetCommit), Rationale: "test"}
	schema, err := durableStore.Create(source.ID, provider.User.ID, revision)
	if err != nil {
		t.Fatal(err)
	}
	revision.Definition = "v2"
	schema, err = durableStore.Revise(source.ID, schema.ID, 1, provider.User.ID, revision)
	if err != nil {
		t.Fatal(err)
	}
	schema, err = durableStore.AddMigration(source.ID, schema.ID, provider.User.ID, durableschemas.Migration{FromVersion: 1, ToVersion: 2, SourceKind: "pull_request", SourceID: "pull", Summary: "expand and contract", Operations: []durableschemas.Operation{{ID: "write", Kind: "write", Description: "dual write", OwnerIDs: []string{provider.User.ID}, ConsumerIDs: []string{consumer.User.ID}, RollbackLimit: "before cleanup"}}, Steps: []durableschemas.Step{{ID: "compat", OperationIDs: []string{"write"}, Description: "support both", SuccessMeasures: []string{"both readers pass"}, RequiredApproverIDs: []string{provider.User.ID}}}, RollbackLimits: []string{"retain old column"}})
	if err != nil {
		t.Fatal(err)
	}
	migration := schema.Migrations[0]
	contract := `"contract":{"old_readers":["read v1"],"new_readers":["read v1 and v2"],"old_writers":["write v1"],"new_writers":["dual write v1 and v2"],"rollout_flags":["orders_v2 defaults off"],"idempotency":"backfill uses order id compare-and-swap","transformations":["copy cents without rounding"],"ownership":["consumer owns compatibility"],"rollback_assumptions":["old column remains authoritative"]}`
	body := fmt.Sprintf(`{"expected_version":%d,"kind":"compatibility","step_id":"compat","repository_id":"%s","title":"Add compatible order reader","completion_criteria":"Both schema versions pass","assignee_type":"human","assignee_id":"%s","mandate":"Implement only the compatibility reader","base_revision":"%s",%s}`, migration.Version, target.ID, consumer.User.ID, targetCommit, contract)
	workURL := server.URL + "/repositories/" + source.ID + "/durable-schemas/" + schema.ID + "/migrations/" + migration.ID + "/work"
	authenticatedRequest(t, http.MethodPost, workURL, body, consumer.Credential.Token, http.StatusNotFound).Body.Close()
	unchanged, err := durableStore.Get(source.ID, schema.ID)
	if err != nil || len(unchanged.Migrations[0].Work) != 0 || unchanged.Migrations[0].Version != migration.Version {
		t.Fatalf("source-read-only request mutated plan: %#v, %v", unchanged.Migrations[0], err)
	}
	response := authenticatedRequest(t, http.MethodPost, workURL, body, provider.Credential.Token, http.StatusCreated)
	var result struct {
		Schema durableschemas.Schema `json:"schema"`
		Task   proposals.Task        `json:"task"`
	}
	decodeResponse(t, response, &result)
	if len(result.Schema.Migrations[0].Work) != 1 || result.Task.Assignment == nil || result.Task.Assignment.AssigneeID != consumer.User.ID {
		t.Fatalf("work/task = %#v / %#v", result.Schema.Migrations[0].Work, result.Task)
	}
	work := result.Schema.Migrations[0].Work[0]
	if work.Contract.Idempotency != "backfill uses order id compare-and-swap" || work.Contract.RolloutFlags[0] != "orders_v2 defaults off" {
		t.Fatalf("contract = %#v", work.Contract)
	}
	pull := pullrequests.PullRequest{RepositoryID: target.ID, TaskID: &work.TaskID}
	projectPullDurableMigration(&pull, durableStore)
	if pull.DurableMigration == nil || pull.DurableMigration.WorkID != work.ID || pull.DurableMigration.Contract.NewWriters[0] != "dual write v1 and v2" {
		t.Fatalf("pull migration review = %#v", pull.DurableMigration)
	}
	proposal, err := proposalStore.Get(target.ID, work.ProposalID)
	if err != nil || !containsAll(proposal.Body, "Old readers: read v1", "Rollback assumptions: old column remains authoritative", "grants no repository") {
		t.Fatalf("proposal body = %q, %v", proposal.Body, err)
	}
	read := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+source.ID+"/durable-schemas/"+schema.ID, "", observer.Credential.Token, http.StatusOK)
	var projected durableschemas.Schema
	if err = json.NewDecoder(read.Body).Decode(&projected); err != nil {
		t.Fatal(err)
	}
	read.Body.Close()
	if len(projected.Migrations[0].Work) != 0 {
		t.Fatalf("private target work leaked: %#v", projected.Migrations[0].Work)
	}
}

func containsAll(value string, values ...string) bool {
	for _, candidate := range values {
		if !strings.Contains(value, candidate) {
			return false
		}
	}
	return true
}
