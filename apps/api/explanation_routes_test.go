package main

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/explanations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
)

func TestGroundedExplanationStreamsAndRetainsExactEvidence(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), gitStore)
	explanationStore, _ := explanations.New(t.TempDir())
	workspaceStore, _ := workspaces.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(gitStore, identities, credentials, catalog, nil, nil, nil, nil, nil, explanationStore, workspaceStore))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "explainer")
	created := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"grounded"}`, owner.Credential.Token, http.StatusCreated)
	var repository repositories.Repository
	json.NewDecoder(created.Body).Decode(&repository)
	created.Body.Close()
	repo, _ := gitStore.Open(repository.ID)
	source, _ := repo.WriteObject(storage.BlobObject, []byte("package access\n\n// Authorize fails closed when identity is absent.\nfunc Authorize(identity string) bool { return identity != \"\" }\n"))
	docs, _ := repo.WriteObject(storage.BlobObject, []byte("# Authorization\n\nAuthorize rejects missing identity so private source stays protected.\n"))
	tree := writeTestTree(t, repo, testTreeEntry{"100644", "README.md", docs}, testTreeEntry{"100644", "authorize.go", source})
	commit := writeTestCommit(t, repo, tree, nil, 1700000000, "document authorization")
	if err := repo.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(commit)}); err != nil {
		t.Fatal(err)
	}
	if resolved, err := resolveExplanationContext(repo, repository.ID, explanationInput{Ref: "main", Context: explanations.Context{Kind: "file", Path: "authorize.go"}}, nil, nil, nil, nil); err != nil || resolved != commit {
		t.Fatalf("resolve context = %s, %v", resolved, err)
	}

	requestBody := `{"question":"Why does Authorize reject a missing identity?","ref":"main","context":{"kind":"file","path":"authorize.go"}}`
	request, _ := http.NewRequest(http.MethodPost, server.URL+"/repositories/"+repository.ID+"/explanations", strings.NewReader(requestBody))
	request.Header.Set("Authorization", "Bearer "+owner.Credential.Token)
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusCreated {
		var failure any
		json.NewDecoder(response.Body).Decode(&failure)
		t.Fatalf("create status = %d: %#v", response.StatusCode, failure)
	}
	if got := response.Header.Get("Content-Type"); !strings.Contains(got, "application/x-ndjson") {
		t.Fatalf("content type = %q", got)
	}
	var final explanations.Conversation
	events := []string{}
	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		var event struct {
			Event        string                    `json:"event"`
			Conversation explanations.Conversation `json:"conversation"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			t.Fatal(err)
		}
		events = append(events, event.Event)
		if event.Event == "done" {
			final = event.Conversation
		}
	}
	response.Body.Close()
	if len(events) < 3 || events[0] != "conversation" || events[len(events)-1] != "done" {
		t.Fatalf("events = %#v", events)
	}
	if final.Revision != string(commit) || final.AskedBy != owner.User.ID || len(final.Claims) == 0 {
		t.Fatalf("conversation = %#v", final)
	}
	first := final.Claims[0]
	if first.Basis != "evidence" || len(first.Citations) != 1 || first.Citations[0].Revision != string(commit) || first.Citations[0].Path != "authorize.go" || first.Citations[0].StartLine == 0 {
		t.Fatalf("claim = %#v", first)
	}

	read := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repository.ID+"/explanations/"+final.ID, "", owner.Credential.Token, http.StatusOK)
	var retained explanations.Conversation
	json.NewDecoder(read.Body).Decode(&retained)
	read.Body.Close()
	if retained.ID != final.ID || retained.Answer == "" || retained.Revision != string(commit) {
		t.Fatalf("retained = %#v", retained)
	}
	request, _ = http.NewRequest(http.MethodGet, server.URL+"/repositories/"+repository.ID+"/explanations/"+final.ID, nil)
	unauthorized, _ := http.DefaultClient.Do(request)
	if unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d", unauthorized.StatusCode)
	}
	unauthorized.Body.Close()

	privatePolicy := workspaces.DefaultPolicy()
	privatePolicy.Sharing = "private"
	privateWorkspace, err := workspaceStore.Create(workspaces.Workspace{RepositoryID: repository.ID, CommitID: string(commit), CreatorID: owner.User.ID, Policy: privatePolicy}, []byte(`{"version":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if privateWorkspace.Policy.Sharing != "private" {
		t.Fatalf("workspace sharing = %q", privateWorkspace.Policy.Sharing)
	}
	storedWorkspace, _ := workspaceStore.Get(privateWorkspace.ID)
	if storedWorkspace.Policy.Sharing != "private" {
		t.Fatalf("stored workspace sharing = %q", storedWorkspace.Policy.Sharing)
	}
	privateResponse := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/explanations", `{"question":"Why does Authorize reject identity?","context":{"kind":"workspace","resource_id":"`+privateWorkspace.ID+`"}}`, owner.Credential.Token, http.StatusCreated)
	privateLocation := privateResponse.Header.Get("Location")
	privateResponse.Body.Close()
	privateID := privateLocation[strings.LastIndex(privateLocation, "/")+1:]
	if privateID == "" {
		t.Fatal("private conversation location is empty")
	}

	collaborator := createTestAccount(t, server.URL, "explanation-reader")
	if collaborator.User.ID == owner.User.ID || collaborator.User.ID == privateWorkspace.CreatorID {
		t.Fatalf("collaborator identity unexpectedly equals owner: %#v %#v", collaborator.User, owner.User)
	}
	if explanationVisibleTo(collaborator.User.ID, explanations.Conversation{RepositoryID: repository.ID, Context: explanations.Context{Kind: "workspace", ResourceID: privateWorkspace.ID}}, catalog, workspaceStore) {
		t.Fatal("private workspace context is visible to collaborator before HTTP request")
	}
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/collaborators", `{"user_id":"`+collaborator.User.ID+`"}`, owner.Credential.Token, http.StatusCreated).Body.Close()
	authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+repository.ID+"/explanations", `{"question":"What happened here?","context":{"kind":"workspace","resource_id":"`+privateWorkspace.ID+`"}}`, collaborator.Credential.Token, http.StatusNotFound).Body.Close()
	historyResponse := authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repository.ID+"/explanations", "", collaborator.Credential.Token, http.StatusOK)
	var history struct {
		Conversations []explanations.Conversation `json:"conversations"`
	}
	if err := json.NewDecoder(historyResponse.Body).Decode(&history); err != nil {
		t.Fatal(err)
	}
	historyResponse.Body.Close()
	for _, item := range history.Conversations {
		if item.ID == privateID {
			t.Fatalf("private conversation disclosed in history: %#v", item)
		}
	}
	authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repository.ID+"/explanations/"+privateID, "", collaborator.Credential.Token, http.StatusNotFound).Body.Close()
	authenticatedRequest(t, http.MethodGet, server.URL+"/repositories/"+repository.ID+"/explanations/"+privateID, "", owner.Credential.Token, http.StatusOK).Body.Close()
}
