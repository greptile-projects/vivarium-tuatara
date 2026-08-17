package main

import (
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/apicontracts"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

func TestAPIContractSourcePathsResolveAtExactCommit(t *testing.T) {
	gitStore, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repository, err := gitStore.Create("api-contract-paths")
	if err != nil {
		t.Fatal(err)
	}
	definition, _ := repository.WriteObject(storage.BlobObject, []byte(`{"openapi":"3.1.0","paths":{}}`))
	documentation, _ := repository.WriteObject(storage.BlobObject, []byte("# API"))
	unrelated, _ := repository.WriteObject(storage.BlobObject, []byte("# Repository"))
	tree := writeTestTree(t, repository,
		testTreeEntry{mode: "100644", name: "README.md", id: unrelated},
		testTreeEntry{mode: "100644", name: "api.md", id: documentation},
		testTreeEntry{mode: "100644", name: "openapi.json", id: definition},
	)
	commit := writeTestCommit(t, repository, tree, nil, 1, "review API contract")
	source := apicontracts.Source{CommitID: string(commit), DefinitionPath: "openapi.json", DocumentationPath: "api.md"}
	if !apiContractSourcePathsResolve(gitStore, "api-contract-paths", source) {
		t.Fatal("reviewed files were rejected")
	}
	source.DefinitionPath = "missing.json"
	if apiContractSourcePathsResolve(gitStore, "api-contract-paths", source) {
		t.Fatal("missing definition was accepted")
	}
	source.DefinitionPath = "../openapi.json"
	if apiContractSourcePathsResolve(gitStore, "api-contract-paths", source) {
		t.Fatal("traversal path was accepted")
	}
	source.DefinitionPath = "README.md"
	if apiContractSourcePathsResolve(gitStore, "api-contract-paths", source) {
		t.Fatal("unrelated reviewed blob was accepted as an API definition")
	}
}
