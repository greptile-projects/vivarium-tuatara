package checkruns

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestParseDocumentationConfigExpandsVersionMatrixAndHashesDependencies(t *testing.T) {
	data := []byte(`{"version":1,"checks":[{"name":"guide","collection_id":"0123456789abcdef0123456789abcdef","image":"alpine:3.22","command":"verify docs","selectors":["links","symbols","build","samples","commands","tutorials"],"dependency_paths":["docs/guide.md","go.mod"],"targets":[{"version":"source/main","revision":"1111111111111111111111111111111111111111","source":"source"},{"version":"v1.0.0","revision":"1111111111111111111111111111111111111111","source":"release"}]}]}`)
	files := map[string][]byte{"docs/guide.md": []byte("guide"), "go.mod": []byte("module example")}
	_, definitions, err := ParseDocumentationConfig(data, "1111111111111111111111111111111111111111", func(path string) ([]byte, error) {
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
	if _, _, err := ParseDocumentationConfig(data, "1111111111111111111111111111111111111111", func(string) ([]byte, error) { return nil, errors.New("missing") }); err == nil {
		t.Fatal("expected missing dependency to fail closed")
	}
}

func TestParseDocumentationConfigRejectsTrailingDocument(t *testing.T) {
	data := []byte(`{"version":1,"checks":[]} {}`)
	if _, _, err := ParseDocumentationConfig(data, "1111111111111111111111111111111111111111", func(string) ([]byte, error) { return nil, nil }); err == nil {
		t.Fatal("expected trailing document rejection")
	}
}

func TestParseDocumentationConfigRejectsUnexecutedTarget(t *testing.T) {
	data := []byte(`{"version":1,"checks":[{"name":"guide","collection_id":"0123456789abcdef0123456789abcdef","image":"alpine:3.22","command":"verify","selectors":["links"],"dependency_paths":["docs/guide.md"],"targets":[{"version":"v1","revision":"2222222222222222222222222222222222222222","source":"release"}]}]}`)
	if _, _, err := ParseDocumentationConfig(data, "1111111111111111111111111111111111111111", func(string) ([]byte, error) { return []byte("guide"), nil }); err == nil {
		t.Fatal("expected unexecuted target rejection")
	}
}

func TestParseDocumentationConfigFreezesOmittedTargetToExecutedRevision(t *testing.T) {
	revision := strings.Repeat("1", 40)
	data := []byte(`{"version":1,"checks":[{"name":"guide","collection_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","image":"alpine:3.22","command":"true","selectors":["commands"],"dependency_paths":["docs/guide.md"],"targets":[{"version":"v1","source":"release"}]}]}`)
	_, definitions, err := ParseDocumentationConfig(data, revision, func(string) ([]byte, error) { return []byte("guide"), nil })
	if err != nil || len(definitions) != 1 || definitions[0].Documentation.Revision != revision {
		t.Fatalf("definitions = %#v, err = %v", definitions, err)
	}
}

func TestParseDocumentationConfigCanonicalizesAndDeduplicatesDependencies(t *testing.T) {
	base := `{"version":1,"checks":[{"name":"guide","collection_id":"0123456789abcdef0123456789abcdef","image":"alpine:3.22","command":"verify","selectors":["links"],"dependency_paths":%s,"targets":[{"version":"v1","revision":"1111111111111111111111111111111111111111","source":"source"}]}]}`
	_, definitions, err := ParseDocumentationConfig([]byte(fmt.Sprintf(base, `["./docs/guide.md"]`)), strings.Repeat("1", 40), func(path string) ([]byte, error) {
		if path != "docs/guide.md" {
			t.Fatalf("read path = %q", path)
		}
		return []byte("guide"), nil
	})
	if err != nil || definitions[0].Documentation.DependencyPaths[0] != "docs/guide.md" {
		t.Fatalf("definitions=%#v err=%v", definitions, err)
	}
	if _, _, err = ParseDocumentationConfig([]byte(fmt.Sprintf(base, `["./docs/guide.md","docs//guide.md"]`)), strings.Repeat("1", 40), func(string) ([]byte, error) { return []byte("guide"), nil }); err == nil {
		t.Fatal("expected duplicate canonical dependency rejection")
	}
}
