package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/datacommitments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/dataflows"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestDataFlowAPIRequiresExactCommitmentAndRetainsCitedAnalysis(t *testing.T) {
	git, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), git)
	commitments, _ := datacommitments.New(t.TempDir())
	flows, _ := dataflows.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(git, identities, credentials, catalog, nil, nil, nil, nil, nil, commitments, flows))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "flow-owner")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"traced-data"}`, owner.Credential.Token, http.StatusCreated)
	var repo repositories.Repository
	json.NewDecoder(response.Body).Decode(&repo)
	response.Body.Close()
	repository, _ := git.Open(repo.ID)
	blob, _ := repository.WriteObject(storage.BlobObject, []byte("one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\nten\neleven\ntwelve\n"))
	tree := writeTestTree(t, repository, testTreeEntry{mode: "100644", name: "event.ts", id: blob})
	revision := writeTestCommit(t, repository, tree, nil, 1700000000, "trace data")
	if err := repository.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(revision)}); err != nil {
		t.Fatal(err)
	}
	commitment, err := commitments.Create(repo.ID, owner.User.ID, datacommitments.Revision{Title: "Usage", Scopes: []datacommitments.Scope{{Kind: "repository", Name: "Project"}}, OwnerIDs: []string{owner.User.ID}, DataUses: []datacommitments.DataUse{{ID: "events", Category: "usage", Subjects: []string{"users"}, Purposes: []string{"reliability"}, Collection: "interaction", Processing: []string{"aggregate"}, Retention: "30 days", Deletion: "on expiry", Consent: "notice", Supported: true}}, Links: []datacommitments.Link{{Kind: "policy", URL: "https://example.test/policy", Label: "Policy"}, {Kind: "notice", URL: "https://example.test/notice", Label: "Notice"}}})
	if err != nil {
		t.Fatal(err)
	}
	ref := map[string]any{"commitment_id": commitment.ID, "version": 1, "data_use_ids": []string{"events"}}
	declaration := map[string]any{"code_revision": string(revision), "title": "Save flow", "entry_points": []string{"save"}, "commitment_refs": []any{ref}, "rationale": "Current implementation", "nodes": []any{map[string]any{"id": "save", "kind": "interaction", "name": "Save", "accessible": true}, map[string]any{"id": "db", "kind": "store", "name": "Events", "accessible": true}}, "edges": []any{map[string]any{"id": "write", "from": "save", "to": "db", "operation": "write", "data_categories": []string{"usage"}, "purpose": "reliability", "retained_copy": true, "commitment_refs": []any{ref}}}}
	payload, _ := json.Marshal(map[string]any{"revision": declaration})
	createdResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/data-flows", string(payload), owner.Credential.Token, http.StatusCreated)
	var created dataflows.Map
	json.NewDecoder(createdResponse.Body).Decode(&created)
	createdResponse.Body.Close()
	bad := mapsClone(declaration)
	bad["code_revision"] = strings.Repeat("f", 40)
	badPayload, _ := json.Marshal(map[string]any{"revision": bad})
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/data-flows", string(badPayload), owner.Credential.Token, http.StatusUnprocessableEntity).Body.Close()
	analysis := map[string]any{"map_version": 1, "code_revision": string(revision), "status": "completed", "bounds": []string{"event.ts"}, "findings": []any{map[string]any{"kind": "declared_observed_difference", "severity": "warning", "summary": "The observed retention condition differs from the declaration.", "edge_ids": []string{"write"}, "citations": []any{map[string]any{"path": "event.ts", "start_line": 8, "end_line": 12, "claim": "writes without the declared expiry option"}}, "uncertainty": "runtime defaults were not executed"}}}
	analysisPayload, _ := json.Marshal(analysis)
	result := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/data-flows/"+created.ID+"/analyses", string(analysisPayload), owner.Credential.Token, http.StatusCreated)
	var analyzed dataflows.Map
	json.NewDecoder(result.Body).Decode(&analyzed)
	result.Body.Close()
	if analyzed.Analyses[0].Findings[0].AddedBy != owner.User.ID || analyzed.Diagnostics[len(analyzed.Diagnostics)-1].Kind != "declared_observed_difference" {
		t.Fatalf("analysis attribution/diagnostic missing: %#v", analyzed)
	}
}
