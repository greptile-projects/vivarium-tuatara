package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	packages "github.com/greptile-projects/vivarium-tuatara/apps/api/packages"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestOwnerPublishesVerifiedPackageAndContributorCannot(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	releaseStore, _ := releases.New(t.TempDir())
	checkRoot := t.TempDir()
	buildStore, _ := checkruns.New(checkRoot)
	packageStore, _ := packages.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, buildStore, releaseStore, packageStore))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "package-owner")
	contributor := createTestAccount(t, server.URL, "package-contributor")
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"library"}`, owner.Credential.Token, http.StatusCreated)
	var repository repositories.Repository
	decodeResponse(t, response, &repository)
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/collaborators", `{"user_id":"`+contributor.User.ID+`"}`, owner.Credential.Token, http.StatusCreated).Body.Close()
	repo, _ := gitStore.Open(repository.ID)
	tree := writeTestTree(t, repo)
	commit := writeTestCommit(t, repo, tree, nil, 1700000000, "reviewed package source")
	release, err := releaseStore.Create(releases.Candidate{RepositoryID: repository.ID, Version: "v1.2.3", Notes: "Verified package release", CommitID: string(commit), CreatedBy: owner.User.ID, Inclusions: releases.Inclusion{PullRequestIDs: []string{}, ProposalIDs: []string{}, TaskIDs: []string{}, ContributorIDs: []string{}}})
	if err != nil {
		t.Fatal(err)
	}
	runs, err := buildStore.CreateRequested(repository.ID, release.ID, string(commit), []checkruns.Definition{{Name: "package", Image: "alpine:3.22", Command: "true", WorkingDirectory: ".", TimeoutSeconds: 30}}, owner.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte("immutable package")
	sum := sha256.Sum256(body)
	artifactID := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	run := runs[0]
	run.State = "succeeded"
	run.Attempts = []checkruns.Attempt{{Number: 1, State: "succeeded", ActorID: owner.User.ID}}
	run.Artifacts = []checkruns.Artifact{{ID: artifactID, Attempt: 1, Path: "dist/library.tgz", Size: int64(len(body)), SHA256: hex.EncodeToString(sum[:]), ContentType: "application/gzip"}}
	if err = buildStore.Update(run); err != nil {
		t.Fatal(err)
	}
	artifactDir := filepath.Join(checkRoot, repository.ID, release.ID, "artifacts", run.ID)
	if err = os.MkdirAll(artifactDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(artifactDir, artifactID), body, 0600); err != nil {
		t.Fatal(err)
	}
	payload := `{"name":"library-kit","version":"1.2.3","build_id":"` + run.ID + `","artifact_id":"` + artifactID + `","platform":{"os":"linux","architecture":"amd64"},"dependencies":[{"name":"core-kit","constraint":"^2.0.0"}],"visibility":"public"}`
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/releases/"+release.ID+"/packages", payload, contributor.Credential.Token, http.StatusForbidden).Body.Close()
	publishedResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/releases/"+release.ID+"/packages", payload, owner.Credential.Token, http.StatusCreated)
	var published packages.Version
	if err = json.NewDecoder(publishedResponse.Body).Decode(&published); err != nil {
		t.Fatal(err)
	}
	publishedResponse.Body.Close()
	if published.PublisherID != owner.User.ID || published.SourceCommit != string(commit) || published.SHA256 != hex.EncodeToString(sum[:]) || published.Lifecycle != "active" {
		t.Fatalf("published = %#v", published)
	}
	public := authenticatedRequest(t, http.MethodGet, server.URL+"/packages/library-kit/versions/1.2.3", "", "", http.StatusOK)
	var visible packages.Version
	decodeResponse(t, public, &visible)
	if visible.ID != published.ID {
		t.Fatalf("visible = %#v", visible)
	}
	artifact := authenticatedRequest(t, http.MethodGet, server.URL+"/packages/library-kit/versions/1.2.3/artifact", "", "", http.StatusOK)
	defer artifact.Body.Close()
	if artifact.Header.Get("X-Checksum-Sha256") != published.SHA256 {
		t.Fatalf("checksum header = %q", artifact.Header.Get("X-Checksum-Sha256"))
	}
	retryResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/releases/"+release.ID+"/packages", payload, owner.Credential.Token, http.StatusOK)
	var retried packages.Version
	decodeResponse(t, retryResponse, &retried)
	if retried.ID != published.ID {
		t.Fatalf("retry = %#v", retried)
	}
}
