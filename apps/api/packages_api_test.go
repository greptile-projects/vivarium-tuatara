package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	packages "github.com/greptile-projects/vivarium-tuatara/apps/api/packages"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestRepositoryPackageCredentialCannotReadUnrelatedPrivatePackage(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	buildStore, _ := checkruns.New(t.TempDir())
	releaseStore, _ := releases.New(t.TempDir())
	packageStore, _ := packages.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, buildStore, releaseStore, packageStore))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "private-package-owner")
	var consumer, publisher repositories.Repository
	decodeResponse(t, authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"consumer"}`, owner.Credential.Token, http.StatusCreated), &consumer)
	decodeResponse(t, authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"publisher"}`, owner.Credential.Token, http.StatusCreated), &publisher)
	for index, name := range []string{"allowed-kit", "unrelated-kit"} {
		body := []byte(name)
		sum := sha256.Sum256(body)
		item, err := packageStore.Publish(packages.Version{Name: name, Version: "1.0." + string(rune('0'+index)), RepositoryID: publisher.ID, ReleaseID: strings.Repeat("1", 32), SourceCommit: strings.Repeat("2", 40), BuildID: strings.Repeat("3", 32), BuildAttestation: packages.BuildAttestation{Step: "package", Image: "alpine:3.22", Command: "make", Attempt: 1, State: "succeeded"}, ArtifactID: strings.Repeat(string(rune('4'+index)), 32), ArtifactPath: name + ".tgz", ContentType: "application/gzip", Size: int64(len(body)), SHA256: hex.EncodeToString(sum[:]), PublisherID: owner.User.ID, Visibility: "private"}, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		_ = item
	}
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+consumer.ID+"/package-credentials", `{"name":"consumer build","package_names":["allowed-kit"],"expires_in":600}`, owner.Credential.Token, http.StatusCreated)
	var issued auth.IssuedCredential
	decodeResponse(t, response, &issued)
	authenticatedRequest(t, http.MethodGet, server.URL+"/packages/allowed-kit/versions/1.0.0", "", issued.Token, http.StatusOK).Body.Close()
	authenticatedRequest(t, http.MethodGet, server.URL+"/packages/unrelated-kit/versions/1.0.1", "", issued.Token, http.StatusNotFound).Body.Close()
	response = authenticatedRequest(t, http.MethodGet, server.URL+"/packages", "", issued.Token, http.StatusOK)
	var visible struct {
		Packages []packages.Version `json:"packages"`
	}
	decodeResponse(t, response, &visible)
	if len(visible.Packages) != 1 || visible.Packages[0].Name != "allowed-kit" {
		t.Fatalf("visible = %#v", visible.Packages)
	}
}

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
	payload := `{"name":"library-kit","version":"1.2.3","build_id":"` + run.ID + `","artifact_id":"` + artifactID + `","platform":{"os":"linux","architecture":"amd64"},"dependencies":[{"name":"core-kit","constraint":"^2.0.0"}],"summary":"A reviewed library","documentation":"Install with your package client and verify the published SHA-256 digest.","license":"MIT","visibility":"public"}`
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
	resolved := authenticatedRequest(t, http.MethodGet, server.URL+"/packages/library-kit/resolve?constraint=%5E1.0.0&os=linux&architecture=amd64", "", "", http.StatusOK)
	var selected packages.Version
	decodeResponse(t, resolved, &selected)
	if selected.ID != published.ID || selected.Documentation == "" || selected.License != "MIT" {
		t.Fatalf("resolved = %#v", selected)
	}
	search := authenticatedRequest(t, http.MethodGet, server.URL+"/packages?q=reviewed", "", "", http.StatusOK)
	var catalogResult struct {
		Packages []packages.Version `json:"packages"`
	}
	decodeResponse(t, search, &catalogResult)
	if len(catalogResult.Packages) != 1 || catalogResult.Packages[0].ID != published.ID {
		t.Fatalf("catalog = %#v", catalogResult)
	}
	lifecycle := authenticatedRequest(t, http.MethodPatch, server.URL+"/repositories/"+repository.ID+"/packages/library-kit/versions/1.2.3", `{"lifecycle":"deprecated","warning":"Use library-kit 2.x before the next release."}`, owner.Credential.Token, http.StatusOK)
	decodeResponse(t, lifecycle, &selected)
	if selected.Lifecycle != "deprecated" || selected.LifecycleWarning == "" {
		t.Fatalf("lifecycle = %#v", selected)
	}
	credentialResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/package-credentials", `{"name":"isolated build","package_names":["library-kit"],"expires_in":600}`, contributor.Credential.Token, http.StatusCreated)
	var registryCredential auth.IssuedCredential
	decodeResponse(t, credentialResponse, &registryCredential)
	if registryCredential.RepositoryID != repository.ID || len(registryCredential.PackageNames) != 1 || registryCredential.PackageNames[0] != "library-kit" {
		t.Fatalf("registry credential = %#v", registryCredential)
	}
	authenticatedRequest(t, http.MethodGet, server.URL+"/packages/library-kit/versions/1.2.3/artifact", "", registryCredential.Token, http.StatusOK).Body.Close()
}
