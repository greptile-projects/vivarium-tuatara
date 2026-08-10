package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
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

func TestWorkspaceFileWriteDoesNotReplaceConcurrentPathCreation(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "source.txt")
	original := []byte("opened version")
	if err := os.WriteFile(target, original, 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(original)
	bin := filepath.Join(directory, "bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	shim := "#!/bin/sh\nprintf 'workspace update' >\"$WORKSPACE_TEST_TARGET\"\nexec /usr/bin/sha256sum \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "sha256sum"), []byte(shim), 0700); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("sh", "-c", workspaceFileWriteScript, "sh", target, hex.EncodeToString(digest[:]))
	command.Stdin = strings.NewReader("client save")
	command.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "WORKSPACE_TEST_TARGET="+target)
	if err := command.Run(); err == nil {
		t.Fatal("concurrent path creation unexpectedly succeeded")
	} else if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 41 {
		t.Fatalf("write error = %v", err)
	}
	body, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "workspace update" {
		t.Fatalf("target = %q, want competing update", body)
	}
}
