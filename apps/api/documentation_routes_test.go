package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	docscollections "github.com/greptile-projects/vivarium-tuatara/apps/api/docscollections"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func TestMaintainerPublishesReviewedDocumentationAndHealthTracksSource(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	docs, _ := docscollections.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, docs))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "docs-owner")
	created := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"owned-docs"}`, owner.Credential.Token, http.StatusCreated)
	var repository repositories.Repository
	decodeResponse(t, created, &repository)
	gr, _ := gitStore.Open(repository.ID)
	blob, _ := gr.WriteObject(storage.BlobObject, []byte("# Install\n\nUse the reviewed API."))
	tree := writeTestTree(t, gr, testTreeEntry{mode: "100644", name: "guide.md", id: blob})
	commit := writeTestCommit(t, gr, tree, nil, 1, "review docs")
	_ = gr.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(commit)})
	body := `{"expected_version":0,"collection":{"name":"User guide","description":"Reviewed guidance","root_path":"guide.md","source_ref":"main","audience":"public","owners":[{"actor_id":"` + owner.User.ID + `","role":"maintainer"}],"supported_versions":[{"label":"main","source_ref":"main","revision":"` + string(commit) + `"}],"rendering":{"format":"markdown","syntax_highlighting":true,"table_of_contents":true},"publication_policy":{"review_required":true,"source_branch":"main","publish_on_merge":true}}}`
	response := authenticatedRequest(t, http.MethodPut, server.URL+"/repositories/"+repository.ID+"/documentation/new", body, owner.Credential.Token, http.StatusCreated)
	var published docscollections.Revision
	decodeResponse(t, response, &published)
	if published.SourceRevision != string(commit) || len(published.Pages) != 1 || published.Pages[0].Authors[0] != "Test" || published.Pages[0].Status != "current" {
		t.Fatalf("published = %#v", published)
	}
	public := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repository.ID+"/documentation/"+published.CollectionID, "", owner.Credential.Token, http.StatusOK)
	var projection struct {
		Collection docscollections.Revision   `json:"collection"`
		History    []docscollections.Revision `json:"history"`
	}
	decodeResponse(t, public, &projection)
	if len(projection.History) != 1 || len(projection.Collection.Diagnostics) != 0 {
		t.Fatalf("projection = %#v", projection)
	}
	newBlob, _ := gr.WriteObject(storage.BlobObject, []byte("# Install\n\nChanged without publication."))
	newTree := writeTestTree(t, gr, testTreeEntry{mode: "100644", name: "guide.md", id: newBlob})
	next := writeTestCommit(t, gr, newTree, []storage.ObjectID{commit}, 2, "change docs")
	_ = gr.UpdateReference(storage.Reference{Name: "refs/heads/main", Target: string(next)})
	stale := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repository.ID+"/documentation/"+published.CollectionID, "", owner.Credential.Token, http.StatusOK)
	decodeResponse(t, stale, &projection)
	codes := []string{}
	for _, d := range projection.Collection.Diagnostics {
		codes = append(codes, d.Code)
	}
	if !strings.Contains(strings.Join(codes, ","), "stale_source") || !strings.Contains(strings.Join(codes, ","), "stale_version_mapping") {
		t.Fatalf("diagnostics = %#v", projection.Collection.Diagnostics)
	}
}
