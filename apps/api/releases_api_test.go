package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestCollaboratorDefinesAndInspectsExactReleaseCandidate(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	releaseStore, _ := releases.New(t.TempDir())
	buildStore, _ := checkruns.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, buildStore, releaseStore))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "release-owner")
	collaborator := createTestAccount(t, server.URL, "release-collaborator")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"ship"}`, owner.Credential.Token, http.StatusCreated)
	var repository repositories.Repository
	decodeResponse(t, response, &repository)
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/collaborators", `{"user_id":"`+collaborator.User.ID+`"}`, owner.Credential.Token, http.StatusCreated).Body.Close()
	gitRepository, _ := gitStore.Open(repository.ID)
	definition, _ := gitRepository.WriteObject(storage.BlobObject, []byte(`{"version":1,"steps":[{"name":"package","image":"alpine:3.22","command":"printf artifact > \"$VIVARIUM_OUTPUT/package.txt\""}]}`))
	readme, _ := gitRepository.WriteObject(storage.BlobObject, []byte("release documentation"))
	baseTree := writeTestTree(t, gitRepository, testTreeEntry{mode: "100644", name: "README.md", id: readme})
	baseCommit := writeTestCommit(t, gitRepository, baseTree, nil, 1699999999, "base state")
	definitionTree := writeTestTree(t, gitRepository, testTreeEntry{mode: "100644", name: "release.json", id: definition})
	tree := writeTestTree(t, gitRepository, testTreeEntry{mode: "40000", name: ".vivarium", id: definitionTree}, testTreeEntry{mode: "100644", name: "README.md", id: readme})
	commit := writeTestCommit(t, gitRepository, tree, []storage.ObjectID{baseCommit}, 1700000000, "release state")
	createdResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/releases", `{"version":"v1.0.0","notes":"First exact delivery.","commit_id":"`+string(commit)+`"}`, collaborator.Credential.Token, http.StatusCreated)
	var created releases.Candidate
	if err := json.NewDecoder(createdResponse.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	createdResponse.Body.Close()
	if created.CommitID != string(commit) || created.CreatedBy != collaborator.User.ID || created.Status != "candidate" || created.Inclusions.PullRequestIDs == nil || created.TargetBranch != "main" || len(created.ChangedPaths) != 2 || created.ChangedPaths[0] != ".vivarium/release.json" || created.ChangedPaths[1] != "README.md" {
		t.Fatalf("created = %#v", created)
	}
	if createdResponse.Header.Get("Location") != "/repositories/"+repository.ID+"/releases/"+created.ID {
		t.Fatalf("Location = %q", createdResponse.Header.Get("Location"))
	}
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/releases", `{"version":"v1.0.0","notes":"duplicate","commit_id":"`+string(commit)+`"}`, owner.Credential.Token, http.StatusConflict).Body.Close()
	listResponse := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repository.ID+"/releases", "", collaborator.Credential.Token, http.StatusOK)
	var listed struct {
		Releases []releases.Candidate `json:"releases"`
	}
	decodeResponse(t, listResponse, &listed)
	if len(listed.Releases) != 1 || listed.Releases[0].ID != created.ID {
		t.Fatalf("listed = %#v", listed)
	}
	authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repository.ID+"/releases/"+created.ID, "", collaborator.Credential.Token, http.StatusOK).Body.Close()
	buildResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/releases/"+created.ID+"/builds", "", owner.Credential.Token, http.StatusAccepted)
	var buildSet struct {
		Builds []checkruns.Run `json:"builds"`
	}
	decodeResponse(t, buildResponse, &buildSet)
	if len(buildSet.Builds) != 1 || buildSet.Builds[0].CommitID != string(commit) || buildSet.Builds[0].RequestedBy != owner.User.ID {
		t.Fatalf("builds = %#v", buildSet.Builds)
	}
	attestationResponse := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repository.ID+"/releases/"+created.ID+"/builds/"+buildSet.Builds[0].ID+"/attestation", "", collaborator.Credential.Token, http.StatusOK)
	var attestation struct {
		SourceCommit string   `json:"source_commit"`
		Command      string   `json:"command"`
		ActorID      string   `json:"actor_id"`
		Dependencies []string `json:"dependencies"`
		Verification struct {
			State string `json:"state"`
		} `json:"verification"`
	}
	decodeResponse(t, attestationResponse, &attestation)
	if attestation.SourceCommit != string(commit) || attestation.ActorID != owner.User.ID || attestation.Command == "" || len(attestation.Dependencies) != 1 || attestation.Verification.State == "" {
		t.Fatalf("attestation = %#v", attestation)
	}
	releaseAttestation := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repository.ID+"/releases/"+created.ID+"/attestation", "", collaborator.Credential.Token, http.StatusOK)
	var aggregate struct {
		SourceCommit string          `json:"source_commit"`
		State        string          `json:"state"`
		Builds       []checkruns.Run `json:"builds"`
	}
	decodeResponse(t, releaseAttestation, &aggregate)
	if aggregate.SourceCommit != string(commit) || aggregate.State == "unbuilt" || len(aggregate.Builds) != 1 {
		t.Fatalf("release attestation = %#v", aggregate)
	}
	provenanceRuns, err := buildStore.CreateRequested(repository.ID, created.ID, string(commit), []checkruns.Definition{{Name: "provenance", Image: "alpine:3.22", Command: "true", WorkingDirectory: ".", TimeoutSeconds: 30}}, owner.User.ID)
	if err != nil {
		t.Fatalf("create provenance build = %#v, %v", provenanceRuns, err)
	}
	var provenance checkruns.Run
	for _, run := range provenanceRuns {
		if run.Definition.Name == "provenance" {
			provenance = run
		}
	}
	if provenance.ID == "" {
		t.Fatalf("provenance build missing from %#v", provenanceRuns)
	}
	provenance.State = "succeeded"
	provenance.RequestedBy = ""
	provenance.Attempts = []checkruns.Attempt{{Number: 1, State: "failed", ActorID: owner.User.ID}, {Number: 2, State: "succeeded", ActorID: collaborator.User.ID}}
	if err := buildStore.Update(provenance); err != nil {
		t.Fatal(err)
	}
	latestResponse := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repository.ID+"/releases/"+created.ID+"/builds/"+provenance.ID+"/attestation", "", owner.Credential.Token, http.StatusOK)
	var latest struct {
		ActorID string `json:"actor_id"`
	}
	decodeResponse(t, latestResponse, &latest)
	if latest.ActorID != collaborator.User.ID {
		t.Fatalf("rerun attestation actor = %q, want %q", latest.ActorID, collaborator.User.ID)
	}
}
