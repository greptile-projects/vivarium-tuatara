package main

import (
	"strings"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/agentprojects"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
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
