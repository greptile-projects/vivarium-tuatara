package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/incidents"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/infrastructure"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
)

func TestInfrastructureDriftTargetsRejectNonexistentGovernedWork(t *testing.T) {
	issueStore, _ := issues.New(t.TempDir())
	proposalStore, _ := proposals.New(t.TempDir())
	incidentStore, _ := incidents.New(t.TempDir())
	for _, response := range []infrastructure.DriftResponse{
		{ResourceKind: "issue", ResourceID: "nonexistent"},
		{ResourceKind: "proposal", ResourceID: "nonexistent"},
		{ResourceKind: "task", ParentID: "nonexistent-proposal", ResourceID: "nonexistent"},
		{ResourceKind: "incident", ResourceID: "nonexistent"},
		{ResourceKind: "pull_request", ResourceID: "nonexistent"},
		{ResourceKind: "exception", ResourceID: "nonexistent"},
	} {
		if infrastructureDriftTargetExists("repo", response, nil, issueStore, proposalStore, incidentStore) {
			t.Fatalf("accepted nonexistent %s target", response.ResourceKind)
		}
	}
}

func TestInfrastructureCommitBlobBindsCandidateToExactPullTree(t *testing.T) {
	git, _ := storage.New(t.TempDir())
	repo, _ := git.Create("candidate-binding")
	body := []byte(`{"revision":"candidate","resources":[]}`)
	blob, _ := repo.WriteObject(storage.BlobObject, body)
	tree := writeTestTree(t, repo, testTreeEntry{mode: "100644", name: "candidate.json", id: blob})
	commit, _ := repo.WriteObject(storage.CommitObject, []byte("tree "+string(tree)+"\nauthor Test <test@example.com> 0 +0000\ncommitter Test <test@example.com> 0 +0000\n\ncandidate\n"))
	got, digest, ok := infrastructureCommitBlob(git, "candidate-binding", string(commit), "candidate.json")
	want := sha256.Sum256(body)
	if !ok || string(got) != string(body) || digest != hex.EncodeToString(want[:]) {
		t.Fatalf("exact candidate blob = %q, %q, %v", got, digest, ok)
	}
	if _, _, ok = infrastructureCommitBlob(git, "candidate-binding", string(commit), "invented.json"); ok {
		t.Fatal("resource declaration absent from pull commit resolved")
	}
}

func TestInfrastructureRehearsalBindingUsesLatestCommandRetry(t *testing.T) {
	created := time.Now().UTC()
	command := "./verify provisioning"
	digest := infrastructure.CommandDigest(command)
	first := workspaces.CommandOutcome{ID: "first", CommandSHA256: digest, ExitCode: 1, Output: "first attempt failed", ActorID: "owner", StartedAt: created.Add(time.Second), CompletedAt: created.Add(2 * time.Second)}
	latest := workspaces.CommandOutcome{ID: "latest", CommandSHA256: digest, ExitCode: 0, Output: "retry passed", ActorID: "owner", StartedAt: created.Add(3 * time.Second), CompletedAt: created.Add(4 * time.Second)}
	ws := workspaces.Workspace{ID: "workspace", CreatorID: "owner", Commands: []workspaces.CommandOutcome{first, latest}}
	rehearsal := infrastructure.Rehearsal{CreatedAt: created, Checks: []infrastructure.RehearsalCheck{{ID: "provisioning", Kind: "provisioning", Command: command}}}
	run, ok := bindInfrastructureRehearsal(ws, infrastructure.ChangePlan{}, rehearsal, []string{"provisioning"})
	if !ok || run.Result != "passed" || len(run.Outcomes) != 1 || run.Outcomes[0].SanitizedLog != "retry passed" || run.Outcomes[0].ExitCode != 0 {
		t.Fatalf("latest retry binding = %#v, %v", run, ok)
	}
	stale := workspaces.CommandOutcome{ID: "stale", CommandSHA256: digest, ExitCode: 0, Output: "stale retry", ActorID: "owner", StartedAt: created.Add(-2 * time.Second), CompletedAt: created.Add(-time.Second)}
	secret := workspaces.CommandOutcome{ID: "secret", CommandSHA256: digest, ExitCode: 0, Output: "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789", ActorID: "owner", StartedAt: created.Add(5 * time.Second), CompletedAt: created.Add(6 * time.Second)}
	ws.Commands = append(ws.Commands, stale, secret)
	run, ok = bindInfrastructureRehearsal(ws, infrastructure.ChangePlan{}, rehearsal, []string{"provisioning"})
	if !ok || run.Result != "passed" || len(run.Outcomes) != 1 || run.Outcomes[0].SanitizedLog != "retry passed" || run.Outcomes[0].CommandDigest != digest {
		t.Fatalf("newest eligible retry binding = %#v, %v", run, ok)
	}
}

func TestInfrastructureAPIKeepsExactIntentPublicAndSensitiveEvidenceBounded(t *testing.T) {
	git, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), git)
	definitions, _ := infrastructure.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(git, identities, credentials, catalog, nil, nil, nil, nil, nil, definitions))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "infra-owner")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"runtime"}`, owner.Credential.Token, http.StatusCreated)
	var repo repositories.Repository
	json.NewDecoder(response.Body).Decode(&repo)
	response.Body.Close()
	authenticatedRequest(t, http.MethodPatch, server.URL+"/repositories/"+repo.ID, `{"visibility":"public"}`, owner.Credential.Token, http.StatusOK).Body.Close()
	gr, _ := git.Open(repo.ID)
	tree, _ := gr.WriteObject(storage.TreeObject, []byte{})
	commit, _ := gr.WriteObject(storage.CommitObject, []byte("tree "+string(tree)+"\nauthor Test <test@example.com> 0 +0000\ncommitter Test <test@example.com> 0 +0000\n\ninfrastructure\n"))
	revision := map[string]any{"title": "Runtime", "summary": "Production runtime", "revision": string(commit), "owner_ids": []string{owner.User.ID}, "rationale": "Reviewed inventory", "resources": []any{map[string]any{"id": "api", "kind": "service", "name": "API", "description": "Public API", "owner_ids": []string{owner.User.ID}, "provider": "cloud", "provider_ref": "service/api", "provider_access": "participant", "depends_on": []string{}, "configuration": []any{map[string]any{"name": "DATABASE_URL", "source": "secret", "sensitivity": "secret_backed", "required": true}}, "constraints": []any{map[string]any{"kind": "cost", "limit": 100, "unit": "USD/month"}, map[string]any{"kind": "capacity", "limit": 500, "unit": "requests/second"}}, "commitments": map[string]any{"security": []string{"least privilege"}, "privacy": []string{"regional"}, "reliability": []string{"99.9%"}, "continuity": []string{"multi-zone"}, "regions": []string{"eu-west"}}}}}
	payload, _ := json.Marshal(map[string]any{"revision": revision})
	createdResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/infrastructure", string(payload), owner.Credential.Token, http.StatusCreated)
	var created infrastructure.Definition
	json.NewDecoder(createdResponse.Body).Decode(&created)
	createdResponse.Body.Close()
	observation, _ := json.Marshal(map[string]any{"definition_version": 1, "resource_id": "api", "provider_resource": "service/api", "observed_revision": "generation-4", "status": "healthy", "summary": "Within declared bounds.", "visibility": "participant", "managed": true, "observed_at": time.Now().UTC()})
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/infrastructure/"+created.ID+"/observations", string(observation), owner.Credential.Token, http.StatusCreated).Body.Close()
	for _, credential := range []string{"ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789", "AKIAABCDEFGHIJKLMNOP", "github_pat_11ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"} {
		unsafeObservation, _ := json.Marshal(map[string]any{"definition_version": 1, "resource_id": "api", "provider_resource": "service/api", "observed_revision": "generation-4", "status": "healthy", "summary": "deployment credential " + credential, "visibility": "public", "managed": true, "observed_at": time.Now().UTC()})
		authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/infrastructure/"+created.ID+"/observations", string(unsafeObservation), owner.Credential.Token, http.StatusBadRequest).Body.Close()
	}
	gitLabToken := "glpat-XXXXXXXXXXXXXXXXXXXX"
	for _, field := range []string{"provider_resource", "observed_revision", "summary"} {
		unsafe := map[string]any{"definition_version": 1, "resource_id": "api", "provider_resource": "service/api", "observed_revision": "generation-4", "status": "healthy", "summary": "sanitized", "visibility": "public", "managed": true, "observed_at": time.Now().UTC()}
		unsafe[field] = gitLabToken
		unsafeObservation, _ := json.Marshal(unsafe)
		authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/infrastructure/"+created.ID+"/observations", string(unsafeObservation), owner.Credential.Token, http.StatusBadRequest).Body.Close()
	}
	publicResponse, err := http.Get(server.URL + "/repositories/" + repo.ID + "/infrastructure/" + created.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer publicResponse.Body.Close()
	if publicResponse.StatusCode != http.StatusOK {
		t.Fatalf("public status = %d", publicResponse.StatusCode)
	}
	var public infrastructure.Definition
	json.NewDecoder(publicResponse.Body).Decode(&public)
	publicJSON, _ := json.Marshal(public)
	if strings.Contains(string(publicJSON), gitLabToken) {
		t.Fatalf("public projection contains rejected GitLab credential: %s", publicJSON)
	}
	if public.Revisions[0].Resources[0].ProviderRef != "restricted" || public.Observations[0].ObservedRevision != "restricted" || public.Observations[0].Summary != "Participant-only observation" {
		t.Fatalf("public projection leaked sensitive evidence: %#v %#v", public.Revisions[0].Resources[0], public.Observations[0])
	}
	bad := revision
	bad["revision"] = "ffffffffffffffffffffffffffffffffffffffff"
	badPayload, _ := json.Marshal(map[string]any{"revision": bad})
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repo.ID+"/infrastructure", string(badPayload), owner.Credential.Token, http.StatusBadRequest).Body.Close()
}
