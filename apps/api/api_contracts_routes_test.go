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
	definition, _ := repository.WriteObject(storage.BlobObject, []byte(`{"openapi":"3.1.0","paths":{"/widgets":{"get":{"responses":{"200":{"description":"ok"}}}}}}`))
	documentation, _ := repository.WriteObject(storage.BlobObject, []byte("# API"))
	unrelated, _ := repository.WriteObject(storage.BlobObject, []byte("# Repository"))
	tree := writeTestTree(t, repository,
		testTreeEntry{mode: "100644", name: "README.md", id: unrelated},
		testTreeEntry{mode: "100644", name: "api.md", id: documentation},
		testTreeEntry{mode: "100644", name: "openapi.json", id: definition},
	)
	commit := writeTestCommit(t, repository, tree, nil, 1, "review API contract")
	revision := apicontracts.Revision{Source: apicontracts.Source{CommitID: string(commit), DefinitionPath: "openapi.json", DocumentationPath: "api.md"}, Operations: []apicontracts.Operation{{Method: "GET", Path: "/widgets"}}}
	if !apiContractSourcePathsResolve(gitStore, "api-contract-paths", revision) {
		t.Fatal("reviewed files were rejected")
	}
	revision.Source.DefinitionPath = "missing.json"
	if apiContractSourcePathsResolve(gitStore, "api-contract-paths", revision) {
		t.Fatal("missing definition was accepted")
	}
	revision.Source.DefinitionPath = "../openapi.json"
	if apiContractSourcePathsResolve(gitStore, "api-contract-paths", revision) {
		t.Fatal("traversal path was accepted")
	}
	revision.Source.DefinitionPath = "README.md"
	if apiContractSourcePathsResolve(gitStore, "api-contract-paths", revision) {
		t.Fatal("unrelated reviewed blob was accepted as an API definition")
	}
	invalidDefinition, _ := repository.WriteObject(storage.BlobObject, []byte(`{"openapi":"3.1.0","paths":{"/widgets":null}}`))
	invalidTree := writeTestTree(t, repository,
		testTreeEntry{mode: "100644", name: "api.md", id: documentation},
		testTreeEntry{mode: "100644", name: "openapi.json", id: invalidDefinition},
	)
	invalidCommit := writeTestCommit(t, repository, invalidTree, []storage.ObjectID{commit}, 2, "invalid API path item")
	revision.Source = apicontracts.Source{CommitID: string(invalidCommit), DefinitionPath: "openapi.json", DocumentationPath: "api.md"}
	if apiContractSourcePathsResolve(gitStore, "api-contract-paths", revision) {
		t.Fatal("null OpenAPI path item was accepted")
	}
}
