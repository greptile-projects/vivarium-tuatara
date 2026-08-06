package main

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

func TestGitCloneEmptyAndPopulatedRepositories(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	empty, err := store.Create("empty-clone")
	if err != nil {
		t.Fatal(err)
	}
	populated, err := store.Create("populated-clone")
	if err != nil {
		t.Fatal(err)
	}

	readme, err := populated.WriteObject(storage.BlobObject, []byte("# project\n\nReady to collaborate.\n"))
	if err != nil {
		t.Fatal(err)
	}
	oldReadme, err := populated.WriteObject(storage.BlobObject, []byte("# project\n"))
	if err != nil {
		t.Fatal(err)
	}
	mainSource, err := populated.WriteObject(storage.BlobObject, []byte("package main\n\nfunc main() {}\n"))
	if err != nil {
		t.Fatal(err)
	}
	featureSource, err := populated.WriteObject(storage.BlobObject, []byte("package feature\n"))
	if err != nil {
		t.Fatal(err)
	}
	script, err := populated.WriteObject(storage.BlobObject, []byte("#!/bin/sh\necho ready\n"))
	if err != nil {
		t.Fatal(err)
	}

	baseTree := writeTestTree(t, populated, testTreeEntry{mode: "100644", name: "README.md", id: oldReadme})
	base := writeTestCommit(t, populated, baseTree, nil, 1700000000, "initial commit")
	mainTree := writeTestTree(t, populated,
		testTreeEntry{mode: "100644", name: "README.md", id: readme},
		testTreeEntry{mode: "100644", name: "main.go", id: mainSource},
	)
	mainCommit := writeTestCommit(t, populated, mainTree, []storage.ObjectID{base}, 1700000001, "build main")
	featureTree := writeTestTree(t, populated, testTreeEntry{mode: "100644", name: "feature.go", id: featureSource})
	featureCommit := writeTestCommit(t, populated, featureTree, []storage.ObjectID{base}, 1700000002, "build feature")
	srcTree := writeTestTree(t, populated,
		testTreeEntry{mode: "100644", name: "feature.go", id: featureSource},
		testTreeEntry{mode: "100644", name: "main.go", id: mainSource},
	)
	mergedTree := writeTestTree(t, populated,
		testTreeEntry{mode: "100644", name: "README.md", id: readme},
		testTreeEntry{mode: "100755", name: "run.sh", id: script},
		testTreeEntry{mode: "40000", name: "src", id: srcTree},
	)
	merge := writeTestCommit(t, populated, mergedTree, []storage.ObjectID{mainCommit, featureCommit}, 1700000003, "merge feature")
	tag := writeTag(t, populated, merge)
	for _, reference := range []storage.Reference{
		{Name: "refs/heads/main", Target: string(merge)},
		{Name: "refs/tags/v1", Target: string(tag)},
	} {
		if err := populated.CreateReference(reference); err != nil {
			t.Fatal(err)
		}
	}

	server := httptest.NewServer(newHandler(store))
	t.Cleanup(server.Close)
	clones := t.TempDir()
	emptyPath := filepath.Join(clones, "empty")
	gitCommand(t, "", "clone", server.URL+"/git/"+empty.ID()+".git", emptyPath)
	if got := gitCommand(t, emptyPath, "symbolic-ref", "HEAD"); got != "refs/heads/main\n" {
		t.Fatalf("empty clone HEAD = %q, want refs/heads/main", got)
	}
	command := exec.Command("git", "rev-parse", "--verify", "HEAD")
	command.Dir = emptyPath
	if err := command.Run(); err == nil {
		t.Fatal("empty clone unexpectedly has a commit")
	}

	populatedPath := filepath.Join(clones, "populated")
	gitCommand(t, "", "clone", server.URL+"/git/"+populated.ID()+".git", populatedPath)
	if got := gitCommand(t, populatedPath, "branch", "--show-current"); got != "main\n" {
		t.Fatalf("populated clone branch = %q, want main", got)
	}
	if got := gitCommand(t, populatedPath, "rev-parse", "HEAD"); got != string(merge)+"\n" {
		t.Fatalf("populated clone HEAD = %q, want %s", got, merge)
	}
	if got := gitCommand(t, populatedPath, "rev-list", "--parents", "-n", "1", "HEAD"); got != fmt.Sprintf("%s %s %s\n", merge, mainCommit, featureCommit) {
		t.Fatalf("cloned merge parents = %q", got)
	}
	gotHistory := strings.Fields(gitCommand(t, populatedPath, "rev-list", "HEAD"))
	wantHistory := []string{string(base), string(mainCommit), string(featureCommit), string(merge)}
	sort.Strings(gotHistory)
	sort.Strings(wantHistory)
	if strings.Join(gotHistory, "\n") != strings.Join(wantHistory, "\n") {
		t.Fatalf("cloned history =\n%s\nwant:\n%s", strings.Join(gotHistory, "\n"), strings.Join(wantHistory, "\n"))
	}
	if got := gitCommand(t, populatedPath, "rev-parse", "v1^{}"); got != string(merge)+"\n" {
		t.Fatalf("cloned tag target = %q, want %s", got, merge)
	}
	assertFile(t, filepath.Join(populatedPath, "README.md"), "# project\n\nReady to collaborate.\n", false)
	assertFile(t, filepath.Join(populatedPath, "run.sh"), "#!/bin/sh\necho ready\n", true)
	assertFile(t, filepath.Join(populatedPath, "src", "main.go"), "package main\n\nfunc main() {}\n", false)
	assertFile(t, filepath.Join(populatedPath, "src", "feature.go"), "package feature\n", false)

	objects, err := populated.ListObjects()
	if err != nil {
		t.Fatal(err)
	}
	wantObjects := make([]string, len(objects))
	for index, object := range objects {
		wantObjects[index] = string(object.ID)
	}
	gotObjects := strings.Fields(gitCommand(t, populatedPath, "cat-file", "--batch-all-objects", "--batch-check=%(objectname)"))
	sort.Strings(gotObjects)
	if strings.Join(gotObjects, "\n") != strings.Join(wantObjects, "\n") {
		t.Fatalf("clone object IDs =\n%s\nwant:\n%s", strings.Join(gotObjects, "\n"), strings.Join(wantObjects, "\n"))
	}
}

func TestGitLsRemoteAdvertisesEmptyAndPopulatedRepositories(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	empty, err := store.Create("empty")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(newHandler(store))
	t.Cleanup(server.Close)

	if got := lsRemote(t, server.URL+"/git/"+empty.ID()+".git"); got != "" {
		t.Fatalf("empty ls-remote = %q, want no refs", got)
	}

	populated, err := store.Create("populated")
	if err != nil {
		t.Fatal(err)
	}
	first := writeCommit(t, populated, 1700000000, "first")
	main := writeCommit(t, populated, 1700000001, "main")
	tag := writeTag(t, populated, main)
	for _, reference := range []storage.Reference{
		{Name: "refs/heads/main", Target: string(main)},
		{Name: "refs/heads/old", Target: string(first)},
		{Name: "refs/tags/v1", Target: string(tag)},
	} {
		if err := populated.CreateReference(reference); err != nil {
			t.Fatal(err)
		}
	}

	got := lsRemote(t, "--symref", server.URL+"/git/"+populated.ID()+".git")
	want := strings.Join([]string{
		"ref: refs/heads/main\tHEAD",
		string(main) + "\tHEAD",
		string(main) + "\trefs/heads/main",
		string(first) + "\trefs/heads/old",
		string(tag) + "\trefs/tags/v1",
		string(main) + "\trefs/tags/v1^{}",
	}, "\n") + "\n"
	if got != want {
		t.Fatalf("populated ls-remote =\n%s\nwant:\n%s", got, want)
	}
}

func TestGitDiscoveryRejectsUnknownRepositoriesAndServices(t *testing.T) {
	store, _ := storage.New(t.TempDir())
	server := httptest.NewServer(newHandler(store))
	t.Cleanup(server.Close)

	response, err := server.Client().Get(server.URL + "/git/missing.git/info/refs?service=git-upload-pack")
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 404 {
		t.Fatalf("missing repository status = %d", response.StatusCode)
	}
	response.Body.Close()

	response, err = server.Client().Get(server.URL + "/git/missing.git/info/refs?service=git-receive-pack")
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 400 {
		t.Fatalf("unsupported service status = %d", response.StatusCode)
	}
	response.Body.Close()
}

func TestUploadPackStopsWhenRequestIsCanceled(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, err := store.Create("canceled")
	if err != nil {
		t.Fatal(err)
	}

	requestContext, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	reader := &blockingReader{context: requestContext, started: started}
	request := httptest.NewRequest("POST", "/git/canceled.git/git-upload-pack", reader)
	request = request.WithContext(requestContext)
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		runUploadPack(response, request, repo, false)
		close(done)
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("git upload-pack did not begin reading the request")
	}
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("git upload-pack did not stop after request cancellation")
	}
}

type blockingReader struct {
	context context.Context
	started chan struct{}
	once    sync.Once
}

func (reader *blockingReader) Read([]byte) (int, error) {
	reader.once.Do(func() { close(reader.started) })
	<-reader.context.Done()
	return 0, reader.context.Err()
}

func lsRemote(t *testing.T, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"ls-remote"}, arguments...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git ls-remote failed: %v\n%s", err, output)
	}
	return string(output)
}

func writeCommit(t *testing.T, repo *storage.Repository, timestamp int64, message string) storage.ObjectID {
	t.Helper()
	tree, err := repo.WriteObject(storage.TreeObject, nil)
	if err != nil {
		t.Fatal(err)
	}
	content := []byte(fmt.Sprintf("tree %s\nauthor Test <test@example.com> %d +0000\ncommitter Test <test@example.com> %d +0000\n\n%s\n", tree, timestamp, timestamp, message))
	id, err := repo.WriteObject(storage.CommitObject, content)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func writeTag(t *testing.T, repo *storage.Repository, target storage.ObjectID) storage.ObjectID {
	t.Helper()
	content := []byte(fmt.Sprintf("object %s\ntype commit\ntag v1\ntagger Test <test@example.com> 1700000002 +0000\n\nrelease\n", target))
	id, err := repo.WriteObject(storage.TagObject, content)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

type testTreeEntry struct {
	mode string
	name string
	id   storage.ObjectID
}

func writeTestTree(t *testing.T, repo *storage.Repository, entries ...testTreeEntry) storage.ObjectID {
	t.Helper()
	var content bytes.Buffer
	for _, entry := range entries {
		fmt.Fprintf(&content, "%s %s%c", entry.mode, entry.name, byte(0))
		decoded, err := hex.DecodeString(string(entry.id))
		if err != nil {
			t.Fatalf("decode object ID %q: %v", entry.id, err)
		}
		content.Write(decoded)
	}
	id, err := repo.WriteObject(storage.TreeObject, content.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func writeTestCommit(t *testing.T, repo *storage.Repository, tree storage.ObjectID, parents []storage.ObjectID, timestamp int64, message string) storage.ObjectID {
	t.Helper()
	var content strings.Builder
	fmt.Fprintf(&content, "tree %s\n", tree)
	for _, parent := range parents {
		fmt.Fprintf(&content, "parent %s\n", parent)
	}
	fmt.Fprintf(&content, "author Test <test@example.com> %d +0000\ncommitter Test <test@example.com> %d +0000\n\n%s\n", timestamp, timestamp, message)
	id, err := repo.WriteObject(storage.CommitObject, []byte(content.String()))
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func gitCommand(t *testing.T, directory string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", arguments...)
	if directory != "" {
		command.Dir = directory
	}
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(arguments, " "), err, output)
	}
	return string(output)
}

func assertFile(t *testing.T, path, wantContent string, wantExecutable bool) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != wantContent {
		t.Fatalf("%s content = %q, want %q", path, content, wantContent)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if gotExecutable := info.Mode().Perm()&0o111 != 0; gotExecutable != wantExecutable {
		t.Fatalf("%s mode = %o, executable = %t, want %t", path, info.Mode().Perm(), gotExecutable, wantExecutable)
	}
}
