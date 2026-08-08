package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/changesessions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestUnhealthyDeploymentOpensEvidencePinnedRepairReview(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	pullRoot := t.TempDir()
	pulls, _ := pullrequests.New(pullRoot, gitStore)
	sessionRoot := t.TempDir()
	sessions, _ := changesessions.New(sessionRoot)
	checks, _ := checkruns.New(t.TempDir())
	releaseStore, _ := releases.New(t.TempDir())
	deploymentStore, _ := deployments.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, pulls, nil, sessions, checks, releaseStore, deploymentStore))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "recovery-owner")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"service"}`, owner.Credential.Token, http.StatusCreated)
	var repository repositories.Repository
	decodeResponse(t, response, &repository)
	gitRepository, _ := gitStore.Open(repository.ID)
	readme, _ := gitRepository.WriteObject(storage.BlobObject, []byte("healthy source\n"))
	tree := writeTestTree(t, gitRepository, testTreeEntry{mode: "100644", name: "README.md", id: readme})
	commit := writeTestCommit(t, gitRepository, tree, nil, 1700000000, "release source")
	if err := gitRepository.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(commit)}); err != nil {
		t.Fatal(err)
	}
	release, err := releaseStore.Create(releases.Candidate{RepositoryID: repository.ID, Version: "v1.2.3", Notes: "Repair the observed canary regression.", CommitID: string(commit), CreatedBy: owner.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	environment, err := deploymentStore.PutEnvironment(deployments.Environment{RepositoryID: repository.ID, Name: "production", Position: 1, Image: "alpine:3.22", Command: "true", TimeoutSeconds: 30, RequiredApprovals: 0, Concurrency: 1, UpdatedBy: owner.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	promotion, err := deploymentStore.CreatePromotion(deployments.Promotion{RepositoryID: repository.ID, EnvironmentID: environment.ID, ReleaseID: release.ID, BuildID: strings.Repeat("b", 32), ArtifactID: strings.Repeat("a", 32), ArtifactSHA256: strings.Repeat("c", 64), CommitID: string(commit), Rollout: deployments.RolloutDefinition{Version: 1, Stages: []deployments.RolloutStage{{Name: "canary", Signals: []deployments.HealthSignal{{Name: "errors", Command: "false"}}}}}, InitiatedBy: owner.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	promotion, err = deploymentStore.Reject(repository.ID, promotion.ID, "customer errors increased")
	if err != nil {
		t.Fatal(err)
	}
	newReadme, _ := gitRepository.WriteObject(storage.BlobObject, []byte("new default-branch work\n"))
	newTree := writeTestTree(t, gitRepository, testTreeEntry{mode: "100644", name: "README.md", id: newReadme})
	current := writeTestCommit(t, gitRepository, newTree, []storage.ObjectID{commit}, 1700000100, "intervening integrated work")
	if err := gitRepository.UpdateReferenceIfTarget(storage.Reference{Name: "refs/heads/main", Target: string(current)}, string(commit)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(pullRoot, 0500); err != nil {
		t.Fatal(err)
	}
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/deployments/"+promotion.ID+"/recoveries", `{"action":"repair","expected_state":"failed"}`, owner.Credential.Token, http.StatusInternalServerError).Body.Close()
	if err := os.Chmod(pullRoot, 0700); err != nil {
		t.Fatal(err)
	}
	latestReadme, _ := gitRepository.WriteObject(storage.BlobObject, []byte("latest default-branch work\n"))
	latestTree := writeTestTree(t, gitRepository, testTreeEntry{mode: "100644", name: "README.md", id: latestReadme})
	latest := writeTestCommit(t, gitRepository, latestTree, []storage.ObjectID{current}, 1700000200, "work after interrupted repair")
	if err := gitRepository.UpdateReferenceIfTarget(storage.Reference{Name: "refs/heads/main", Target: string(latest)}, string(current)); err != nil {
		t.Fatal(err)
	}
	divergentReadme, _ := gitRepository.WriteObject(storage.BlobObject, []byte("unexpected repair work\n"))
	divergentTree := writeTestTree(t, gitRepository, testTreeEntry{mode: "100644", name: "README.md", id: divergentReadme})
	divergent := writeTestCommit(t, gitRepository, divergentTree, []storage.ObjectID{current}, 1700000300, "divergent unpublished repair")
	recoveryRef := "refs/heads/agent/recovery/" + promotion.ID
	if err := gitRepository.UpdateReferenceIfTarget(storage.Reference{Name: recoveryRef, Target: string(divergent)}, string(current)); err != nil {
		t.Fatal(err)
	}
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/deployments/"+promotion.ID+"/recoveries", `{"action":"repair","expected_state":"failed"}`, owner.Credential.Token, http.StatusConflict).Body.Close()
	if err := gitRepository.UpdateReferenceIfTarget(storage.Reference{Name: recoveryRef, Target: string(current)}, string(divergent)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sessionRoot, 0500); err != nil {
		t.Fatal(err)
	}
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/deployments/"+promotion.ID+"/recoveries", `{"action":"repair","expected_state":"failed"}`, owner.Credential.Token, http.StatusInternalServerError).Body.Close()
	if err := os.Chmod(sessionRoot, 0700); err != nil {
		t.Fatal(err)
	}
	recoveryURL := server.URL + "/repositories/" + repository.ID + "/deployments/" + promotion.ID + "/recoveries"
	var wait sync.WaitGroup
	concurrentErrors := make(chan error, 24)
	for range 24 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			request, requestErr := http.NewRequest(http.MethodPost, recoveryURL, strings.NewReader(`{"action":"repair","expected_state":"failed"}`))
			if requestErr != nil {
				concurrentErrors <- requestErr
				return
			}
			request.Header.Set("Authorization", "Bearer "+owner.Credential.Token)
			request.Header.Set("Content-Type", "application/json")
			response, requestErr := http.DefaultClient.Do(request)
			if requestErr != nil {
				concurrentErrors <- requestErr
				return
			}
			response.Body.Close()
			if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusCreated {
				concurrentErrors <- fmt.Errorf("concurrent recovery status = %d", response.StatusCode)
			}
		}()
	}
	wait.Wait()
	close(concurrentErrors)
	for concurrentErr := range concurrentErrors {
		t.Error(concurrentErr)
	}
	repairResponse := authenticatedRequest(t, http.MethodPost, recoveryURL, `{"action":"repair","expected_state":"failed"}`, owner.Credential.Token, http.StatusOK)
	var repair struct {
		PullRequest pullrequests.PullRequest `json:"pull_request"`
		Session     changesessions.Session   `json:"session"`
	}
	if err := json.NewDecoder(repairResponse.Body).Decode(&repair); err != nil {
		t.Fatal(err)
	}
	repairResponse.Body.Close()
	if repair.PullRequest.SourceBranch != "agent/recovery/"+promotion.ID || repair.PullRequest.SourceCommitID != string(latest) || repair.PullRequest.TargetBranch != "main" || repair.Session.SourceCommitID != string(latest) || repair.Session.DeploymentEvidence == nil {
		t.Fatalf("repair = %#v", repair)
	}
	evidence := repair.Session.DeploymentEvidence
	if evidence.ReleaseVersion != "v1.2.3" || evidence.ReleaseNotes != release.Notes || evidence.CommitID != string(commit) || evidence.State != "failed" {
		t.Fatalf("evidence = %#v", evidence)
	}
	if repairResponse.Header.Get("Location") != "/repositories/"+repository.ID+"/pulls/"+repair.PullRequest.ID+"/sessions/"+repair.Session.ID {
		t.Fatalf("Location = %q", repairResponse.Header.Get("Location"))
	}
	allPulls, err := pulls.List(repository.ID)
	if err != nil || len(allPulls) != 1 || allPulls[0].ID != repair.PullRequest.ID {
		t.Fatalf("idempotent repair pulls = %#v, %v", allPulls, err)
	}
	allSessions, err := sessions.List(repository.ID, repair.PullRequest.ID)
	if err != nil || len(allSessions) != 1 || allSessions[0].ID != repair.Session.ID {
		t.Fatalf("idempotent repair sessions = %#v, %v", allSessions, err)
	}
	reconnectResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/deployments/"+promotion.ID+"/recoveries", `{"action":"repair","expected_state":"failed"}`, owner.Credential.Token, http.StatusOK)
	var reconnected struct {
		PullRequest pullrequests.PullRequest `json:"pull_request"`
		Session     changesessions.Session   `json:"session"`
	}
	decodeResponse(t, reconnectResponse, &reconnected)
	if reconnected.PullRequest.ID != repair.PullRequest.ID || reconnected.Session.ID != repair.Session.ID {
		t.Fatalf("reconnected repair = %#v", reconnected)
	}
}
