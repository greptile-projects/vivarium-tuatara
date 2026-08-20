package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/agentprojects"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func agentProjectCommit(t *testing.T, git *storage.Store, repositoryID, branch, file string, parent storage.ObjectID) storage.ObjectID {
	t.Helper()
	repo, _ := git.Open(repositoryID)
	blob, _ := repo.WriteObject(storage.BlobObject, []byte("reviewed\n"))
	tree := writeTestTree(t, repo, testTreeEntry{"100644", file, blob})
	parents := []storage.ObjectID(nil)
	if parent != "" {
		parents = []storage.ObjectID{parent}
	}
	commit := writeTestCommit(t, repo, tree, parents, 1700000000, branch)
	if err := repo.CreateReference(storage.Reference{Name: "refs/heads/" + branch, Target: string(commit)}); err != nil {
		t.Fatal(err)
	}
	return commit
}

func completeAgentProjectRevision(owner, repositoryID, revision, sourcePath string) agentprojects.Revision {
	return agentprojects.Revision{Title: "Review helper", Purpose: "Review changes", OwnerIDs: []string{owner}, Sources: []agentprojects.Source{{ID: "prompt", Kind: "prompt", RepositoryID: repositoryID, Revision: revision, Path: sourcePath, Purpose: "reviewed prompt"}}, Tools: []agentprojects.Tool{{Name: "git", Purpose: "inspect", Actions: []string{"read"}, Boundary: "read only"}}, Models: []agentprojects.Model{{Provider: "openai", Name: "codex", Version: "1", Purpose: "reasoning"}}, SupportedTasks: []string{"review"}, ExpectedOutputs: []string{"findings"}, ProhibitedActions: []string{"write"}, MemoryPolicy: "session", DataUseTerms: "repository only", Budget: agentprojects.Budget{MaxTokens: 100, MaxToolActions: 2, MaxRuntimeSeconds: 60}, Escalations: []agentprojects.Escalation{{Trigger: "uncertain", OwnerIDs: []string{owner}, Action: "stop"}}, DeploymentBoundaries: []agentprojects.DeploymentBoundary{{Environment: "isolated", RepositoryAccess: "selected", NetworkAccess: "none"}}, ChangeSummary: "successor"}
}

func TestAgentProjectSourcesRejectHiddenCommitAndRedactEveryHistoricalRevision(t *testing.T) {
	git, _ := storage.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), git)
	owner, dependencyOwner, reader := strings.Repeat("a", 32), strings.Repeat("b", 32), strings.Repeat("c", 32)
	projectRepo, _ := catalog.Create(owner, "project")
	dependencyRepo, _ := catalog.Create(dependencyOwner, "dependency")
	_, _ = catalog.AddCollaborator(owner, projectRepo.ID, reader)
	_, _ = catalog.AddCollaborator(dependencyOwner, dependencyRepo.ID, owner)
	_, _ = catalog.AddCollaborator(dependencyOwner, dependencyRepo.ID, reader)
	mainCommit := agentProjectCommit(t, git, projectRepo.ID, "main", "agent.md", "")
	dependencyCommit := agentProjectCommit(t, git, dependencyRepo.ID, "main", "knowledge.md", "")
	hiddenCommit := agentProjectCommit(t, git, projectRepo.ID, "vivarium-security/advisory", "secret.md", mainCommit)
	if agentProjectSourceResolves(git, agentprojects.Source{RepositoryID: projectRepo.ID, Revision: string(hiddenCommit), Path: "secret.md"}) {
		t.Fatal("security-only commit resolved")
	}
	if !agentProjectSourceResolves(git, agentprojects.Source{RepositoryID: projectRepo.ID, Revision: string(mainCommit), Path: "agent.md"}) {
		t.Fatal("visible commit did not resolve")
	}
	project := agentprojects.Project{RepositoryID: projectRepo.ID, Revisions: []agentprojects.Revision{
		{Version: 1, CreatedBy: owner, Sources: []agentprojects.Source{{ID: "dependency", RepositoryID: dependencyRepo.ID, Revision: string(dependencyCommit), Path: "knowledge.md", Purpose: "private knowledge"}}},
		{Version: 2, CreatedBy: owner, Sources: []agentprojects.Source{{ID: "prompt", RepositoryID: projectRepo.ID, Revision: string(mainCommit), Path: "agent.md", Purpose: "prompt"}}},
	}}
	if err := catalog.RemoveCollaborator(dependencyOwner, dependencyRepo.ID, reader); err != nil {
		t.Fatal(err)
	}
	projected := projectAgentSources(git, catalog, reader, []agentprojects.Project{project})[0]
	old := projected.Revisions[0].Sources[0]
	if old.RepositoryID != "restricted" || old.Revision != "" || old.Path != "" || old.Purpose != "restricted dependency" {
		t.Fatalf("historical source leaked: %#v", old)
	}
	if len(projected.Diagnostics) != 1 || projected.Diagnostics[0].Kind != "inaccessible_dependency" {
		t.Fatalf("diagnostics = %#v", projected.Diagnostics)
	}
}

func TestAgentProjectMutationProjectsHistoryAndPrivateProbesAreIndistinguishable(t *testing.T) {
	git, _ := storage.New(t.TempDir())
	identities, _ := users.New(t.TempDir())
	credentials, _ := auth.New(t.TempDir())
	catalog, _ := repositories.New(t.TempDir(), git)
	projects, _ := agentprojects.New(t.TempDir())
	server := httptest.NewServer(newPlatformHandlerWithChecks(git, identities, credentials, catalog, nil, nil, nil, nil, nil, projects))
	defer server.Close()
	owner := createTestAccount(t, server.URL, "agent-owner")
	victim := createTestAccount(t, server.URL, "dependency-owner")
	createRepo := func(name, token string) repositories.Repository {
		response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories", `{"name":"`+name+`"}`, token, http.StatusCreated)
		defer response.Body.Close()
		var repo repositories.Repository
		if err := json.NewDecoder(response.Body).Decode(&repo); err != nil {
			t.Fatal(err)
		}
		return repo
	}
	provider := createRepo("agent-provider", owner.Credential.Token)
	dependency := createRepo("private-knowledge", victim.Credential.Token)
	providerCommit := agentProjectCommit(t, git, provider.ID, "main", "agent.md", "")
	dependencyCommit := agentProjectCommit(t, git, dependency.ID, "main", "knowledge.md", "")
	historical := completeAgentProjectRevision(owner.User.ID, dependency.ID, string(dependencyCommit), "knowledge.md")
	project, err := projects.Create(provider.ID, owner.User.ID, historical)
	if err != nil {
		t.Fatal(err)
	}
	successor := completeAgentProjectRevision(owner.User.ID, provider.ID, string(providerCommit), "agent.md")
	body, _ := json.Marshal(map[string]any{"expected_version": 1, "revision": successor})
	response := authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+provider.ID+"/agent-projects/"+project.ID+"/revisions", string(body), owner.Credential.Token, http.StatusOK)
	var revised agentprojects.Project
	if err = json.NewDecoder(response.Body).Decode(&revised); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	old := revised.Revisions[0].Sources[0]
	if old.RepositoryID != "restricted" || old.Revision != "" || old.Path != "" {
		t.Fatalf("mutation leaked history: %#v", old)
	}
	validProbe := completeAgentProjectRevision(owner.User.ID, dependency.ID, string(dependencyCommit), "knowledge.md")
	invalidProbe := validProbe
	invalidProbe.Sources = append([]agentprojects.Source(nil), validProbe.Sources...)
	invalidProbe.Sources[0].Path = "missing.md"
	for _, probe := range []agentprojects.Revision{validProbe, invalidProbe} {
		payload, _ := json.Marshal(map[string]any{"revision": probe})
		authenticatedRequest(t, http.MethodPost, server.URL+"/repositories/"+provider.ID+"/agent-projects", string(payload), owner.Credential.Token, http.StatusForbidden).Body.Close()
	}
}
