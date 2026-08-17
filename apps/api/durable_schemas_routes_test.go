package main

import (
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/durableschemas"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

func TestDurableSchemaDefinitionResolvesExactReviewedBlob(t *testing.T) {
	gitStore, _ := storage.New(t.TempDir())
	repository, _ := gitStore.Create("schema-paths")
	definition, _ := repository.WriteObject(storage.BlobObject, []byte("create table orders(id uuid primary key);\n"))
	tree := writeTestTree(t, repository, testTreeEntry{mode: "100644", name: "orders.sql", id: definition})
	commit := writeTestCommit(t, repository, tree, nil, 1, "review schema")
	revision := durableschemas.Revision{ReviewedCommit: string(commit), DefinitionPath: "orders.sql", Definition: "create table orders(id uuid primary key);\n"}
	if !durableSchemaDefinitionResolves(gitStore, "schema-paths", revision) {
		t.Fatal("exact reviewed definition rejected")
	}
	revision.Definition = "drop table orders;"
	if durableSchemaDefinitionResolves(gitStore, "schema-paths", revision) {
		t.Fatal("caller definition that differs from reviewed blob accepted")
	}
	revision.Definition = "create table orders(id uuid primary key);\n"
	revision.DefinitionPath = "../orders.sql"
	if durableSchemaDefinitionResolves(gitStore, "schema-paths", revision) {
		t.Fatal("traversal definition path accepted")
	}
}
