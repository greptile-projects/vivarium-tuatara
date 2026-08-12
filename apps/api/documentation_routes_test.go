package main

import (
	"fmt"
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
	_ = gr.CreateReference(storage.Reference{Name: "refs/heads/docs", Target: string(commit)})
	body := `{"expected_version":0,"collection":{"name":"User guide","description":"Reviewed guidance","root_path":"guide.md","source_ref":"docs","audience":"public","owners":[{"actor_id":"` + owner.User.ID + `","role":"maintainer"}],"supported_versions":[{"label":"next","source_ref":"docs"},{"label":"stable","source_ref":"main"}],"rendering":{"format":"markdown","syntax_highlighting":true,"table_of_contents":true},"publication_policy":{"review_required":true,"source_branch":"docs","publish_on_merge":true}}}`
	response := authenticatedRequest(t, http.MethodPut, server.URL+"/repositories/"+repository.ID+"/documentation/new", body, owner.Credential.Token, http.StatusCreated)
	var published docscollections.Revision
	decodeResponse(t, response, &published)
	if published.SourceRevision != string(commit) || published.SupportedVersions[0].Revision != string(commit) || published.SupportedVersions[1].Revision != string(commit) || len(published.Pages) != 1 || published.Pages[0].Authors[0] != "Test" || published.Pages[0].Status != "current" {
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
	_ = gr.UpdateReference(storage.Reference{Name: "refs/heads/docs", Target: string(next)})
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

func TestCollaboratorCreatesGroundedDocumentationTask(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	docs, _ := docscollections.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, docs))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "task-owner")
	created := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"task-docs"}`, owner.Credential.Token, http.StatusCreated)
	var repository repositories.Repository
	decodeResponse(t, created, &repository)
	gr, _ := gitStore.Open(repository.ID)
	blob, _ := gr.WriteObject(storage.BlobObject, []byte("# API\nGrounded behavior."))
	tree := writeTestTree(t, gr, testTreeEntry{mode: "100644", name: "api.md", id: blob})
	commit := writeTestCommit(t, gr, tree, nil, 1, "source")
	_ = gr.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(commit)})
	createBody := `{"title":"Explain API behavior","path":"docs/api.md","source":{"kind":"proposal","resource_id":"11111111111111111111111111111111","revision":"` + string(commit) + `","label":"Proposal discussion"}}`
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/documentation-tasks", createBody, owner.Credential.Token, http.StatusCreated)
	var task docscollections.Task
	decodeResponse(t, response, &task)
	if task.Branch != "docs/tasks/"+task.ID {
		t.Fatalf("task = %#v", task)
	}
	if ref, e := gr.ReadReference("refs/heads/" + task.Branch); e != nil || ref.Target != string(commit) {
		t.Fatalf("branch = %#v, %v", ref, e)
	}
	draft := fmt.Sprintf(`{"expected_version":%d,"body":"# API behavior\nThe handler returns JSON.","references":[{"path":"api.md","start_line":1,"end_line":2,"revision":"%s","label":"API source"}]}`, task.Version, commit)
	response = authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/documentation-tasks/"+task.ID+"/drafts", draft, owner.Credential.Token, http.StatusCreated)
	decodeResponse(t, response, &task)
	if len(task.Drafts) != 1 || !strings.Contains(task.Drafts[0].RenderedHTML, "API behavior") {
		t.Fatalf("draft = %#v", task.Drafts)
	}
	entry := fmt.Sprintf(`{"expected_version":%d,"kind":"agent_assistance","body":"The exact error behavior is uncertain.","agent_id":"docs-agent","uncertain":true,"references":[{"path":"api.md","revision":"%s","label":"Current source"}]}`, task.Version, commit)
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/documentation-tasks/"+task.ID+"/entries", entry, owner.Credential.Token, http.StatusUnprocessableEntity).Body.Close()
	badRange := fmt.Sprintf(`{"expected_version":%d,"kind":"suggestion","body":"Bad citation.","references":[{"path":"api.md","start_line":9,"end_line":8,"revision":"%s","label":"Invalid range"}]}`, task.Version, commit)
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/documentation-tasks/"+task.ID+"/entries", badRange, owner.Credential.Token, http.StatusUnprocessableEntity).Body.Close()
	missing := fmt.Sprintf(`{"expected_version":%d,"body":"Missing source.","references":[{"path":"missing.md","start_line":1,"end_line":1,"revision":"%s","label":"Missing"}]}`, task.Version, commit)
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/documentation-tasks/"+task.ID+"/drafts", missing, owner.Credential.Token, http.StatusUnprocessableEntity).Body.Close()
}

func TestDocumentationHistoryFiltersEveryRevisionAudience(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	docs, _ := docscollections.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, docs))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "docs-history-owner")
	created := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"history-docs"}`, owner.Credential.Token, http.StatusCreated)
	var repository repositories.Repository
	decodeResponse(t, created, &repository)
	authenticatedRequest(t, http.MethodPatch, server.URL+"/repositories/"+repository.ID, `{"visibility":"public"}`, owner.Credential.Token, http.StatusOK).Body.Close()
	gr, _ := gitStore.Open(repository.ID)
	blob, _ := gr.WriteObject(storage.BlobObject, []byte("# Protected history"))
	tree := writeTestTree(t, gr, testTreeEntry{mode: "100644", name: "guide.md", id: blob})
	commit := writeTestCommit(t, gr, tree, nil, 1, "review docs")
	_ = gr.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(commit)})
	collection := func(expected int, audience string) string {
		return `{"expected_version":` + fmt.Sprint(expected) + `,"collection":{"name":"Guide","description":"Reviewed guidance","root_path":"guide.md","source_ref":"main","audience":"` + audience + `","owners":[{"actor_id":"` + owner.User.ID + `","role":"maintainer"}],"supported_versions":[{"label":"main","source_ref":"main"}],"rendering":{"format":"markdown"},"publication_policy":{"review_required":true,"source_branch":"main"}}}`
	}
	first := authenticatedRequest(t, http.MethodPut, server.URL+"/repositories/"+repository.ID+"/documentation/new", collection(0, "maintainers"), owner.Credential.Token, http.StatusCreated)
	var published docscollections.Revision
	decodeResponse(t, first, &published)
	authenticatedRequest(t, http.MethodPut, server.URL+"/repositories/"+repository.ID+"/documentation/"+published.CollectionID, collection(1, "public"), owner.Credential.Token, http.StatusCreated).Body.Close()
	ownerDetail := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repository.ID+"/documentation/"+published.CollectionID, "", owner.Credential.Token, http.StatusOK)
	var projection struct {
		History []docscollections.Revision `json:"history"`
	}
	decodeResponse(t, ownerDetail, &projection)
	if len(projection.History) != 2 {
		t.Fatalf("owner history = %#v", projection.History)
	}
	publicDetail, err := http.Get(server.URL + "/repositories/" + repository.ID + "/documentation/" + published.CollectionID)
	if err != nil {
		t.Fatal(err)
	}
	decodeResponse(t, publicDetail, &projection)
	if len(projection.History) != 1 || projection.History[0].Audience != "public" {
		t.Fatalf("public history = %#v", projection.History)
	}
}
