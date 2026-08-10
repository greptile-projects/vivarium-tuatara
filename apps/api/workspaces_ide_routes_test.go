package main

import (
	"strings"
	"testing"
)

func TestWorkspaceEditRequiresCompleteDigest(t *testing.T) {
	valid := strings.Repeat("a", 64)
	for _, test := range []struct {
		value string
		want  bool
	}{{valid, true}, {"", false}, {strings.Repeat("a", 63), false}, {strings.Repeat("z", 64), false}} {
		if got := validWorkspaceDigest(test.value); got != test.want {
			t.Fatalf("validWorkspaceDigest(%q) = %v, want %v", test.value, got, test.want)
		}
	}
}
