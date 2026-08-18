package main

import (
	"strings"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/interfacesystems"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

func TestInterfaceSystemRejectsForeignImplementationProvenance(t *testing.T) {
	gitStore, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repositoryID := strings.Repeat("1", 32)
	repository, err := gitStore.Create(repositoryID)
	if err != nil {
		t.Fatal(err)
	}
	component, _ := repository.WriteObject(storage.BlobObject, []byte("component"))
	interaction, _ := repository.WriteObject(storage.BlobObject, []byte("interaction"))
	content, _ := repository.WriteObject(storage.BlobObject, []byte("content"))
	tree := writeTestTree(t, repository,
		testTreeEntry{mode: "100644", name: "component.tsx", id: component},
		testTreeEntry{mode: "100644", name: "content.ts", id: content},
		testTreeEntry{mode: "100644", name: "interaction.ts", id: interaction},
	)
	commit := writeTestCommit(t, repository, tree, nil, 1, "interface system")
	releaseStore, err := releases.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	release, err := releaseStore.Create(releases.Candidate{RepositoryID: repositoryID, Version: "v1", Notes: "Interface system", CommitID: string(commit), CreatedBy: strings.Repeat("2", 32)})
	if err != nil {
		t.Fatal(err)
	}
	definition := interfacesystems.Definition{SourcePath: "component.tsx"}
	revision := interfacesystems.Revision{CommitID: string(commit), ReleaseID: release.ID, Components: []interfacesystems.Definition{definition}, InteractionPatterns: []interfacesystems.Definition{{SourcePath: "interaction.ts"}}, ContentRules: []interfacesystems.Definition{{SourcePath: "content.ts"}}, Implementations: []interfacesystems.Implementation{{RepositoryID: repositoryID, ReleaseID: release.ID, CommitID: string(commit)}}}
	if !interfaceSystemProvenanceResolves(gitStore, releaseStore, repositoryID, &revision) {
		t.Fatal("exact local implementation provenance was rejected")
	}
	revision.Implementations[0].RepositoryID = strings.Repeat("3", 32)
	revision.Implementations[0].ReleaseID = strings.Repeat("4", 32)
	revision.Implementations[0].CommitID = strings.Repeat("5", 40)
	if interfaceSystemProvenanceResolves(gitStore, releaseStore, repositoryID, &revision) {
		t.Fatal("fabricated foreign implementation provenance was accepted")
	}
}
