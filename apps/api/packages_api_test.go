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
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	packages "github.com/greptile-projects/vivarium-tuatara/apps/api/packages"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
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

func TestVersionMatchingPreservesPrereleaseBoundaries(t *testing.T) {
	for _, constraint := range []string{"1.2.3", "=1.2.3", "^1.2.0", "~1.2.3", ">=1.2.3", "*"} {
		if versionMatches("1.2.3-rc.1", constraint) {
			t.Fatalf("prerelease matched stable constraint %q", constraint)
		}
	}
	if !versionMatches("1.2.3-rc.1", "=1.2.3-rc.1") || compareVersion("1.2.3-rc.1", "1.2.3") >= 0 || compareVersion("1.2.3-rc.2", "1.2.3-rc.1") <= 0 {
		t.Fatal("prerelease ordering or explicit matching is incorrect")
	}
}

func TestPackageUpdateScanOpensAttributableOrdinaryWork(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	proposalStore, _ := proposals.New(t.TempDir())
	buildStore, _ := checkruns.New(t.TempDir())
	releaseStore, _ := releases.New(t.TempDir())
	packageStore, _ := packages.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, proposalStore, nil, nil, nil, buildStore, releaseStore, packageStore))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "update-owner")
	var consumer, publisher repositories.Repository
	decodeResponse(t, authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"consumer"}`, owner.Credential.Token, http.StatusCreated), &consumer)
	decodeResponse(t, authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"publisher"}`, owner.Credential.Token, http.StatusCreated), &publisher)
	repo, _ := gitStore.Open(consumer.ID)
	manifest, _ := repo.WriteObject(storage.BlobObject, []byte(`{"version":1,"dependencies":[{"name":"core-kit","constraint":"^1.0.0"}],"lock":[{"name":"core-kit","version":"1.0.0"}]}`))
	configTree := writeTestTree(t, repo, testTreeEntry{mode: "100644", name: "packages.json", id: manifest})
	root := writeTestTree(t, repo, testTreeEntry{mode: "40000", name: ".vivarium", id: configTree})
	commit := writeTestCommit(t, repo, root, nil, 1700000000, "locked dependency")
	if err := repo.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(commit)}); err != nil {
		t.Fatal(err)
	}
	for versionIndex, version := range []string{"1.0.0", "1.2.0", "2.0.0"} {
		marker := string(rune('1' + versionIndex))
		release, err := releaseStore.Create(releases.Candidate{RepositoryID: publisher.ID, Version: "v" + version, Notes: "Notes for " + version, CommitID: strings.Repeat(marker, 40), CreatedBy: owner.User.ID, Inclusions: releases.Inclusion{}})
		if err != nil {
			t.Fatal(err)
		}
		body := []byte(version)
		sum := sha256.Sum256(body)
		_, err = packageStore.Publish(packages.Version{Name: "core-kit", Version: version, RepositoryID: publisher.ID, ReleaseID: release.ID, SourceCommit: release.CommitID, BuildID: strings.Repeat(marker, 32), BuildAttestation: packages.BuildAttestation{Step: "compatibility", Image: "alpine:3.22", Command: "go test ./...", Attempt: 1, State: "succeeded"}, ArtifactID: strings.Repeat(string(rune('5'+versionIndex)), 32), ArtifactPath: "core.tgz", Size: int64(len(body)), SHA256: hex.EncodeToString(sum[:]), PublisherID: owner.User.ID, Visibility: "public"}, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
	}
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+consumer.ID+"/dependency-inventories", `{"commit_id":"`+string(commit)+`"}`, owner.Credential.Token, http.StatusCreated).Body.Close()
	authenticatedRequest(t, http.MethodPut, server.URL+"/repositories/"+consumer.ID+"/package-update-policies/core-kit", `{"strategy":"minor","action":"proposal"}`, owner.Credential.Token, http.StatusOK).Body.Close()
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+consumer.ID+"/package-updates/scan", `{}`, owner.Credential.Token, http.StatusOK)
	var result struct {
		Updates []packages.Update `json:"updates"`
	}
	decodeResponse(t, response, &result)
	if len(result.Updates) != 1 || result.Updates[0].FromVersion != "1.0.0" || result.Updates[0].ToVersion != "1.2.0" || result.Updates[0].Manifest.Lock[0].Version != "1.2.0" || result.Updates[0].CreatedBy != owner.User.ID {
		t.Fatalf("updates = %#v", result.Updates)
	}
	proposal, err := proposalStore.Get(consumer.ID, result.Updates[0].ProposalID)
	if err != nil || !strings.Contains(proposal.Body, "Affected dependency paths") {
		t.Fatalf("proposal = %#v, err = %v", proposal, err)
	}
	tasks, _ := proposalStore.ListTasks(consumer.ID, proposal.ID)
	if len(tasks) != 1 || tasks[0].ID != result.Updates[0].TaskID {
		t.Fatalf("tasks = %#v", tasks)
	}
	response = authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+consumer.ID+"/package-updates/scan", `{}`, owner.Credential.Token, http.StatusOK)
	decodeResponse(t, response, &result)
	if len(result.Updates) != 1 {
		t.Fatalf("retry updates = %#v", result.Updates)
	}
}

func TestPackageUpdateListingRechecksPrivatePackageVisibility(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	buildStore, _ := checkruns.New(t.TempDir())
	releaseStore, _ := releases.New(t.TempDir())
	packageStore, _ := packages.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, buildStore, releaseStore, packageStore))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "private-update-owner")
	reader := createTestAccount(t, server.URL, "private-update-reader")
	var consumer, publisher repositories.Repository
	decodeResponse(t, authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"private-consumer"}`, owner.Credential.Token, http.StatusCreated), &consumer)
	decodeResponse(t, authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"private-publisher"}`, owner.Credential.Token, http.StatusCreated), &publisher)
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+consumer.ID+"/collaborators", `{"user_id":"`+reader.User.ID+`"}`, owner.Credential.Token, http.StatusCreated).Body.Close()
	body := []byte("private")
	sum := sha256.Sum256(body)
	published, err := packageStore.Publish(packages.Version{Name: "private-kit", Version: "1.1.0", RepositoryID: publisher.ID, ReleaseID: strings.Repeat("1", 32), SourceCommit: strings.Repeat("2", 40), BuildID: strings.Repeat("3", 32), BuildAttestation: packages.BuildAttestation{Step: "secret-check", Image: "private-image", Command: "verify", Attempt: 1, State: "succeeded"}, ArtifactID: strings.Repeat("4", 32), ArtifactPath: "private.tgz", Size: int64(len(body)), SHA256: hex.EncodeToString(sum[:]), PublisherID: owner.User.ID, Visibility: "private"}, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	_, err = packageStore.RecordUpdate(packages.Update{RepositoryID: consumer.ID, PackageName: published.Name, FromVersion: "1.0.0", ToVersion: published.Version, BaseCommit: strings.Repeat("5", 40), ProposalID: strings.Repeat("6", 32), TaskID: strings.Repeat("7", 32), ReleaseNotes: "private notes", Compatibility: published.BuildAttestation, CreatedBy: owner.User.ID})
	if err != nil {
		t.Fatal(err)
	}
	var ownerView, readerView struct {
		Updates []packages.Update `json:"updates"`
	}
	decodeResponse(t, authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+consumer.ID+"/package-updates", "", owner.Credential.Token, http.StatusOK), &ownerView)
	decodeResponse(t, authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+consumer.ID+"/package-updates", "", reader.Credential.Token, http.StatusOK), &readerView)
	if len(ownerView.Updates) != 1 || len(readerView.Updates) != 0 {
		t.Fatalf("owner = %#v, reader = %#v", ownerView.Updates, readerView.Updates)
	}
}

func TestPrivatePackageUpdateProposalRedactsPublisherEvidence(t *testing.T) {
	version := packages.Version{Name: "secret-kit", Version: "1.1.0", ReleaseID: strings.Repeat("a", 32), Visibility: "private", BuildAttestation: packages.BuildAttestation{Step: "private-step", Image: "private-image", Attempt: 2}}
	body := packageUpdateProposalBody(version, packages.InventoryEntry{Version: "1.0.0", Paths: []string{"app > secret-kit"}}, "private release notes")
	for _, secret := range []string{"private release notes", "private-step", "private-image", version.ReleaseID} {
		if strings.Contains(body, secret) {
			t.Fatalf("proposal leaked %q: %s", secret, body)
		}
	}
	if !strings.Contains(body, "app > secret-kit") || !strings.Contains(body, "package-authorized update record") {
		t.Fatalf("proposal lost safe adoption context: %s", body)
	}
}

func TestRecordedDependencyInventoryDerivesExactVisibleConsumer(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	buildStore, _ := checkruns.New(t.TempDir())
	releaseStore, _ := releases.New(t.TempDir())
	packageStore, _ := packages.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, buildStore, releaseStore, packageStore))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "inventory-owner")
	var consumer, publisher repositories.Repository
	decodeResponse(t, authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"consumer"}`, owner.Credential.Token, http.StatusCreated), &consumer)
	decodeResponse(t, authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"publisher"}`, owner.Credential.Token, http.StatusCreated), &publisher)
	authenticatedRequest(t, http.MethodPatch, server.URL+"/repositories/"+consumer.ID, `{"visibility":"public"}`, owner.Credential.Token, http.StatusOK).Body.Close()
	body := []byte("package")
	sum := sha256.Sum256(body)
	published, err := packageStore.Publish(packages.Version{Name: "core-kit", Version: "2.1.0", RepositoryID: publisher.ID, ReleaseID: strings.Repeat("1", 32), SourceCommit: strings.Repeat("2", 40), BuildID: strings.Repeat("3", 32), BuildAttestation: packages.BuildAttestation{Step: "package", Image: "alpine:3.22", Command: "make", Attempt: 1, State: "succeeded"}, ArtifactID: strings.Repeat("4", 32), ArtifactPath: "core.tgz", Size: int64(len(body)), SHA256: hex.EncodeToString(sum[:]), License: "MIT", Support: "https://support.example.test", PublisherID: owner.User.ID, Visibility: "public"}, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	publishParent := func(name, constraint, marker string) {
		content := []byte(name)
		digest := sha256.Sum256(content)
		_, publishErr := packageStore.Publish(packages.Version{Name: name, Version: "1.0.0", RepositoryID: publisher.ID, ReleaseID: strings.Repeat(marker, 32), SourceCommit: strings.Repeat(marker, 40), BuildID: strings.Repeat(marker, 32), BuildAttestation: packages.BuildAttestation{Step: "package", Image: "alpine:3.22", Command: "make", Attempt: 1, State: "succeeded"}, ArtifactID: strings.Repeat(marker, 32), ArtifactPath: name + ".tgz", Size: int64(len(content)), SHA256: hex.EncodeToString(digest[:]), Dependencies: []packages.Dependency{{Name: "core-kit", Constraint: constraint}}, PublisherID: owner.User.ID, Visibility: "public"}, bytes.NewReader(content))
		if publishErr != nil {
			t.Fatal(publishErr)
		}
	}
	publishParent("legacy-app", "^3.0.0", "5")
	publishParent("current-app", "^2.0.0", "6")
	repo, _ := gitStore.Open(consumer.ID)
	manifest, _ := repo.WriteObject(storage.BlobObject, []byte(`{"version":1,"dependencies":[{"name":"legacy-app","constraint":"^1.0.0"},{"name":"current-app","constraint":"^1.0.0"}],"lock":[{"name":"legacy-app","version":"1.0.0"},{"name":"current-app","version":"1.0.0"},{"name":"core-kit","version":"2.1.0"}]}`))
	configTree := writeTestTree(t, repo, testTreeEntry{mode: "100644", name: "packages.json", id: manifest})
	root := writeTestTree(t, repo, testTreeEntry{mode: "40000", name: ".vivarium", id: configTree})
	commit := writeTestCommit(t, repo, root, nil, 1700000000, "locked dependencies")
	if err = repo.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(commit)}); err != nil {
		t.Fatal(err)
	}
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+consumer.ID+"/dependency-inventories", `{"commit_id":"`+string(commit)+`"}`, owner.Credential.Token, http.StatusCreated)
	var projection struct {
		Inventory packages.Inventory `json:"inventory"`
		Current   bool               `json:"current"`
	}
	decodeResponse(t, response, &projection)
	var core packages.InventoryEntry
	for _, entry := range projection.Inventory.Entries {
		if entry.Name == "core-kit" {
			core = entry
		}
	}
	if !projection.Current || len(projection.Inventory.Entries) != 3 || core.PackageID != published.ID || core.State != "stale" || len(core.Paths) != 2 {
		t.Fatalf("projection = %#v", projection)
	}
	consumerResponse := authenticatedRequest(t, http.MethodGet, server.URL+"/packages/core-kit/versions/2.1.0/consumers", "", "", http.StatusOK)
	var consumers struct {
		Consumers []json.RawMessage `json:"consumers"`
	}
	decodeResponse(t, consumerResponse, &consumers)
	if len(consumers.Consumers) != 1 {
		t.Fatalf("consumers = %#v", consumers)
	}
}

func TestPackageDeploymentProjectionMarksOnlyLatestSuccessfulEnvironmentCurrent(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	promotions := []deployments.Promotion{
		{ID: strings.Repeat("a", 32), ReleaseID: strings.Repeat("1", 32), EnvironmentID: strings.Repeat("e", 32), State: "succeeded", CreationSequence: 1, CreatedAt: now},
		{ID: strings.Repeat("b", 32), ReleaseID: strings.Repeat("2", 32), EnvironmentID: strings.Repeat("e", 32), State: "failed", CreationSequence: 2, CreatedAt: now.Add(time.Minute)},
		{ID: strings.Repeat("c", 32), ReleaseID: strings.Repeat("2", 32), EnvironmentID: strings.Repeat("e", 32), State: "succeeded", CreationSequence: 3, CreatedAt: now.Add(2 * time.Minute)},
	}
	old := projectPackageDeployments(promotions, map[string]bool{strings.Repeat("1", 32): true})
	latest := projectPackageDeployments(promotions, map[string]bool{strings.Repeat("2", 32): true})
	if len(old) != 1 || old[0].Current || len(latest) != 2 || latest[0].Current || !latest[1].Current {
		t.Fatalf("old = %#v, latest = %#v", old, latest)
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
	lifecycle := authenticatedRequest(t, http.MethodPatch, server.URL+"/repositories/"+repository.ID+"/packages/library-kit/versions/1.2.3", `{"lifecycle":"deprecated","warning":"Use library-kit 2.x before the next release.","reason":"This line no longer receives security fixes."}`, owner.Credential.Token, http.StatusOK)
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
	authenticatedRequest(t, http.MethodGet, server.URL+"/packages/library-kit/versions/1.2.3/artifact", "", registryCredential.Token, http.StatusConflict).Body.Close()
	authenticatedRequest(t, http.MethodGet, server.URL+"/packages/library-kit/resolve?constraint=%5E1.0.0&os=linux&architecture=amd64", "", "", http.StatusNotFound).Body.Close()
}
