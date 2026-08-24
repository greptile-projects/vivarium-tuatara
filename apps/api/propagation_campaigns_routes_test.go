package main

import (
	"reflect"
	"testing"
)

func TestPropagationSymbolEvidenceKeepsDeclarationsNotSimilarity(t *testing.T) {
	diff := "diff --git a/parser.go b/parser.go\n+++ b/parser.go\n+func Parse(input string) error {\n+  Parse(input)\n+type Result struct {\n+export interface Decoder {\n"
	got := propagationSymbolEvidence(diff)
	want := []string{"Decoder", "Parse", "Result"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected declared-symbol evidence: %#v", got)
	}
}
