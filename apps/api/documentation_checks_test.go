package main

import (
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
)

func TestDocumentationDefinitionAffectedUsesDeclaredInputs(t *testing.T) {
	definition := checkruns.Definition{Documentation: &checkruns.DocumentationEvidence{DependencyPaths: []string{"docs/guide.md", "go.mod"}}}
	if documentationDefinitionAffected(definition, map[string]bool{"src/app.go": true}) {
		t.Fatal("unrelated source change affected documentation evidence")
	}
	if !documentationDefinitionAffected(definition, map[string]bool{"docs/guide.md": true}) {
		t.Fatal("declared documentation change was ignored")
	}
	if !documentationDefinitionAffected(definition, map[string]bool{checkruns.DocumentationConfigPath: true}) {
		t.Fatal("configuration change must affect every matrix cell")
	}
}
