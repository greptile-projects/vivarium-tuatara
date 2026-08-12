package main

import (
	"strings"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/previews"
)

func TestPreviewBuildUsesDisposableWritableCopy(t *testing.T) {
	command := previewBuildCommand(previews.Config{
		Build:            "mkdir -p dist && printf ok > dist/index.html",
		WorkingDirectory: ".",
		OutputPath:       "dist",
	})
	for _, expected := range []string{"cp -R /workspace/. /tmp/vivarium-preview/", "cd '/tmp/vivarium-preview/.'", "test -d 'dist'", `cp -R 'dist'/. "$VIVARIUM_OUTPUT"/`} {
		if !strings.Contains(command, expected) {
			t.Fatalf("command %q does not contain %q", command, expected)
		}
	}
}
