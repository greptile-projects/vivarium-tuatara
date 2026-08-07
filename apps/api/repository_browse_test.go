package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestRepositoryBrowsingPreservesBranchAndCommitRevision(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	server := httptest.NewServer(newAuthenticatedAppHandler(gitStore, identities, credentials, catalog))
	defer server.Close()

	account := createTestAccount(t, server.URL, "browser")
	created := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"project"}`, account.Credential.Token, http.StatusCreated)
	var repository repositories.Repository
	if err := json.NewDecoder(created.Body).Decode(&repository); err != nil {
		t.Fatal(err)
	}
	created.Body.Close()
	repo, _ := gitStore.Open(repository.ID)
	readme, _ := repo.WriteObject(storage.BlobObject, []byte("# project\n"))
	large, _ := repo.WriteObject(storage.BlobObject, bytes.Repeat([]byte("a"), maxBlobPreviewBytes+1024))
	source, _ := repo.WriteObject(storage.BlobObject, []byte("package main\n"))
	srcTree := writeTestTree(t, repo, testTreeEntry{"100644", "main.go", source})
	rootTree := writeTestTree(t, repo, testTreeEntry{"100644", "README.md", readme}, testTreeEntry{"100644", "large.txt", large}, testTreeEntry{"40000", "src", srcTree})
	base := writeTestCommit(t, repo, rootTree, nil, 1699999999, "base browser state")
	commit := writeTestCommit(t, repo, rootTree, []storage.ObjectID{base}, 1700000000, "initial browser state")
	if err := repo.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(commit)}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateReference(storage.Reference{Name: "refs/heads/feature", Target: string(commit)}); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("git", "--git-dir", repo.Path(), "pack-refs", "--all", "--prune").CombinedOutput(); err != nil {
		t.Fatalf("pack refs: %v\n%s", err, output)
	}
	if err := os.MkdirAll(filepath.Join(repo.Path(), "refs", "heads"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo.Path(), "refs", "heads", "feature"), []byte(commit+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, endpoint := range []string{
		"/branches",
		"/commits?ref=feature",
		"/tree?ref=feature",
		"/tree?ref=" + string(commit) + "&path=src",
		"/blob?ref=feature&path=README.md",
	} {
		response := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repository.ID+endpoint, "", account.Credential.Token, http.StatusOK)
		var body map[string]any
		if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if endpoint != "/branches" && body["revision"] != string(commit) {
			t.Fatalf("%s revision = %#v", endpoint, body["revision"])
		}
	}

	missing := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repository.ID+"/tree?ref=missing", "", account.Credential.Token, http.StatusNotFound)
	missing.Body.Close()

	history := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repository.ID+"/commits?ref=feature&limit=1", "", account.Credential.Token, http.StatusOK)
	var historyBody struct {
		Commits    []browseCommit `json:"commits"`
		NextCursor *string        `json:"next_cursor"`
	}
	if err := json.NewDecoder(history.Body).Decode(&historyBody); err != nil {
		t.Fatal(err)
	}
	history.Body.Close()
	if len(historyBody.Commits) != 1 || historyBody.NextCursor == nil {
		t.Fatalf("paginated history = %#v", historyBody)
	}
	nextHistory := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repository.ID+"/commits?ref=feature&limit=1&after="+*historyBody.NextCursor, "", account.Credential.Token, http.StatusOK)
	var nextHistoryBody struct {
		Commits    []browseCommit `json:"commits"`
		NextCursor *string        `json:"next_cursor"`
	}
	if err := json.NewDecoder(nextHistory.Body).Decode(&nextHistoryBody); err != nil {
		t.Fatal(err)
	}
	nextHistory.Body.Close()
	if len(nextHistoryBody.Commits) != 1 || nextHistoryBody.Commits[0].ID != string(base) || nextHistoryBody.NextCursor != nil {
		t.Fatalf("second history page = %#v", nextHistoryBody)
	}

	preview := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repository.ID+"/blob?ref=feature&path=large.txt", "", account.Credential.Token, http.StatusOK)
	var previewBody struct {
		Content   string `json:"content"`
		Size      int64  `json:"size"`
		Truncated bool   `json:"truncated"`
	}
	if err := json.NewDecoder(preview.Body).Decode(&previewBody); err != nil {
		t.Fatal(err)
	}
	preview.Body.Close()
	if len(previewBody.Content) != maxBlobPreviewBytes || previewBody.Size != maxBlobPreviewBytes+1024 || !previewBody.Truncated {
		t.Fatalf("bounded preview = content %d, size %d, truncated %v", len(previewBody.Content), previewBody.Size, previewBody.Truncated)
	}
	if err := repo.DeleteReference("refs/heads/feature"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ReadReference("refs/heads/feature"); !errors.Is(err, storage.ErrReferenceNotFound) {
		t.Fatalf("deleted packed and loose branch: %v", err)
	}
	references, err := repo.ListReferences()
	if err != nil {
		t.Fatal(err)
	}
	for _, reference := range references {
		if reference.Name == "refs/heads/feature" {
			t.Fatalf("deleted branch remained listed: %#v", reference)
		}
	}
}

func TestPublicRepositoryBrowsingIsAnonymousAndPrivateBrowsingIsHidden(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	server := httptest.NewServer(newAuthenticatedAppHandler(gitStore, identities, credentials, catalog))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "browse-owner")
	created := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"visible"}`, owner.Credential.Token, http.StatusCreated)
	var repository repositories.Repository
	json.NewDecoder(created.Body).Decode(&repository)
	created.Body.Close()

	requestStatus(t, http.MethodGet, server.URL+"/repositories/"+repository.ID+"/branches", "", http.StatusUnauthorized).Body.Close()
	authenticatedRequest(t, http.MethodPatch, server.URL+"/repositories/"+repository.ID, `{"visibility":"public"}`, owner.Credential.Token, http.StatusOK).Body.Close()
	requestStatus(t, http.MethodGet, server.URL+"/repositories/"+repository.ID+"/branches", "", http.StatusOK).Body.Close()
}
