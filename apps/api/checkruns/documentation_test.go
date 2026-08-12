package checkruns

import (
	"errors"
	"testing"
)

func TestParseDocumentationConfigExpandsVersionMatrixAndHashesDependencies(t *testing.T) {
	data := []byte(`{"version":1,"checks":[{"name":"guide","collection_id":"0123456789abcdef0123456789abcdef","image":"alpine:3.22","command":"verify docs","selectors":["links","symbols","build","samples","commands","tutorials"],"dependency_paths":["docs/guide.md","go.mod"],"targets":[{"version":"source/main","revision":"1111111111111111111111111111111111111111","source":"source"},{"version":"v1.0.0","revision":"2222222222222222222222222222222222222222","source":"release"}]}]}`)
	files := map[string][]byte{"docs/guide.md": []byte("guide"), "go.mod": []byte("module example")}
	_, definitions, err := ParseDocumentationConfig(data, func(path string) ([]byte, error) {
		body, ok := files[path]
		if !ok {
			return nil, errors.New("missing")
		}
		return body, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(definitions) != 2 || definitions[0].Name != "docs/guide [source/main]" || definitions[1].Name != "docs/guide [v1.0.0]" {
		t.Fatalf("definitions = %#v", definitions)
	}
	for _, definition := range definitions {
		if definition.Documentation == nil || definition.Documentation.DependencySHA256 == "" || len(definition.Documentation.Selectors) != 6 || definition.TimeoutSeconds != 600 {
			t.Fatalf("evidence = %#v", definition)
		}
	}
}

func TestParseDocumentationConfigFailsClosedOnMissingDependency(t *testing.T) {
	data := []byte(`{"version":1,"checks":[{"name":"guide","collection_id":"0123456789abcdef0123456789abcdef","image":"alpine:3.22","command":"verify","selectors":["links"],"dependency_paths":["docs/missing.md"],"targets":[{"version":"v1","revision":"1111111111111111111111111111111111111111","source":"package"}]}]}`)
	if _, _, err := ParseDocumentationConfig(data, func(string) ([]byte, error) { return nil, errors.New("missing") }); err == nil {
		t.Fatal("expected missing dependency to fail closed")
	}
}
