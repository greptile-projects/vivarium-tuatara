package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestWorkspaceListUsesPortableFindActions(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "feature.txt"), []byte("planned\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(directory, ".vivarium"), 0700); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("find", ".", "-mindepth", "1", "-maxdepth", "1", "-exec", "sh", "-c", workspaceListScript, "sh", "{}", "+")
	command.Dir = directory
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("portable listing: %v: %s", err, output)
	}
	listing := string(output)
	if !strings.Contains(listing, "f\t8\tfeature.txt\n") || !strings.Contains(listing, "d\t0\t.vivarium\n") {
		t.Fatalf("listing = %q", listing)
	}
}

func TestWorkspaceExecAttachesProvidedStdin(t *testing.T) {
	bin := t.TempDir()
	docker := filepath.Join(bin, "docker")
	if err := os.WriteFile(docker, []byte("#!/bin/sh\ncase \" $* \" in *\" exec -i \"*) cat ;; *) exit 42 ;; esac\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	output, err := workspaceExec("0123456789abcdef0123456789abcdef", time.Second, "/workspace", strings.NewReader("saved body"), "sh", "-c", "cat")
	if err != nil {
		t.Fatalf("workspace exec: %v", err)
	}
	if string(output) != "saved body" {
		t.Fatalf("output = %q", output)
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
