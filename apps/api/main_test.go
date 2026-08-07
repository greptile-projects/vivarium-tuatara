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

func TestGitFetchAndPullAdvancedPrimaryBranch(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, err := store.Create("synchronized")
	if err != nil {
		t.Fatal(err)
	}

	initialContent, err := repo.WriteObject(storage.BlobObject, []byte("first\n"))
	if err != nil {
		t.Fatal(err)
	}
	initialTree := writeTestTree(t, repo, testTreeEntry{mode: "100644", name: "status.txt", id: initialContent})
	initial := writeTestCommit(t, repo, initialTree, nil, 1700000000, "initial")
	if err := repo.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(initial)}); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(newHandler(store))
	t.Cleanup(server.Close)
	workingCopy := filepath.Join(t.TempDir(), "working-copy")
	gitCommand(t, "", "clone", server.URL+"/git/"+repo.ID()+".git", workingCopy)

	fetchedContent, err := repo.WriteObject(storage.BlobObject, []byte("second\n"))
	if err != nil {
		t.Fatal(err)
	}
	fetchedTree := writeTestTree(t, repo, testTreeEntry{mode: "100644", name: "status.txt", id: fetchedContent})
	fetched := writeTestCommit(t, repo, fetchedTree, []storage.ObjectID{initial}, 1700000001, "second")
	if err := repo.UpdateReference(storage.Reference{Name: "refs/heads/main", Target: string(fetched)}); err != nil {
		t.Fatal(err)
	}

	packsBeforeFetch := packIndexes(t, workingCopy)
	fetchTrace := gitCommandWithEnv(t, workingCopy, []string{"GIT_TRACE_PACKET=1"}, "-c", "fetch.unpackLimit=1", "fetch", "origin")
	if !strings.Contains(fetchTrace, "have "+string(initial)) {
		t.Fatalf("fetch negotiation did not report existing commit %s as a have:\n%s", initial, fetchTrace)
	}
	packsAfterFetch := packIndexes(t, workingCopy)
	newPacks := difference(packsAfterFetch, packsBeforeFetch)
	if len(newPacks) != 1 {
		t.Fatalf("new fetch pack indexes = %v, want exactly one", newPacks)
	}
	gotTransferred := packedObjectIDs(t, workingCopy, newPacks[0])
	wantTransferred := []string{string(fetchedContent), string(fetchedTree), string(fetched)}
	sort.Strings(wantTransferred)
	if strings.Join(gotTransferred, "\n") != strings.Join(wantTransferred, "\n") {
		t.Fatalf("fetch transferred object IDs =\n%s\nwant only missing objects:\n%s", strings.Join(gotTransferred, "\n"), strings.Join(wantTransferred, "\n"))
	}
	if got := gitCommand(t, workingCopy, "rev-parse", "HEAD"); got != string(initial)+"\n" {
		t.Fatalf("HEAD after fetch = %q, want unchanged %s", got, initial)
	}
	if got := gitCommand(t, workingCopy, "rev-parse", "refs/remotes/origin/main"); got != string(fetched)+"\n" {
		t.Fatalf("origin/main after fetch = %q, want %s", got, fetched)
	}
	gitCommand(t, workingCopy, "cat-file", "-e", string(fetched)+"^{commit}")
	assertFile(t, filepath.Join(workingCopy, "status.txt"), "first\n", false)

	pulledContent, err := repo.WriteObject(storage.BlobObject, []byte("third\n"))
	if err != nil {
		t.Fatal(err)
	}
	pulledTree := writeTestTree(t, repo, testTreeEntry{mode: "100644", name: "status.txt", id: pulledContent})
	pulled := writeTestCommit(t, repo, pulledTree, []storage.ObjectID{fetched}, 1700000002, "third")
	if err := repo.UpdateReference(storage.Reference{Name: "refs/heads/main", Target: string(pulled)}); err != nil {
		t.Fatal(err)
	}

	gitCommand(t, workingCopy, "pull", "--ff-only")
	if got := gitCommand(t, workingCopy, "rev-parse", "HEAD"); got != string(pulled)+"\n" {
		t.Fatalf("HEAD after pull = %q, want %s", got, pulled)
	}
	if got := gitCommand(t, workingCopy, "rev-list", "--first-parent", "HEAD"); got != fmt.Sprintf("%s\n%s\n%s\n", pulled, fetched, initial) {
		t.Fatalf("history after pull = %q", got)
	}
	assertFile(t, filepath.Join(workingCopy, "status.txt"), "third\n", false)
}

func TestGitPushCreatesAndAdvancesPrimaryBranchWithoutLosingHistory(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, err := store.Create("published")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(newHandler(store))
	t.Cleanup(server.Close)

	workingCopy := filepath.Join(t.TempDir(), "publisher")
	gitCommand(t, "", "init", "--initial-branch=main", workingCopy)
	gitCommand(t, workingCopy, "config", "user.name", "Publisher")
	gitCommand(t, workingCopy, "config", "user.email", "publisher@example.com")
	gitCommand(t, workingCopy, "remote", "add", "origin", server.URL+"/git/"+repo.ID()+".git")
	if err := os.WriteFile(filepath.Join(workingCopy, "progress.txt"), []byte("first\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, workingCopy, "add", "progress.txt")
	gitCommand(t, workingCopy, "commit", "-m", "initial progress")
	initial := strings.TrimSpace(gitCommand(t, workingCopy, "rev-parse", "HEAD"))
	gitCommand(t, workingCopy, "push", "-u", "origin", "main")
	if got := gitCommand(t, repo.Path(), "rev-parse", "refs/heads/main"); got != initial+"\n" {
		t.Fatalf("initial remote main = %q, want %s", got, initial)
	}

	if err := os.WriteFile(filepath.Join(workingCopy, "progress.txt"), []byte("second\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, workingCopy, "commit", "-am", "continue progress")
	advanced := strings.TrimSpace(gitCommand(t, workingCopy, "rev-parse", "HEAD"))
	gitCommand(t, workingCopy, "push", "origin", "main")
	if got := gitCommand(t, repo.Path(), "rev-list", "--first-parent", "refs/heads/main"); got != advanced+"\n"+initial+"\n" {
		t.Fatalf("advanced remote history = %q", got)
	}

	gitCommand(t, workingCopy, "reset", "--hard", initial)
	if err := os.WriteFile(filepath.Join(workingCopy, "progress.txt"), []byte("replacement\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, workingCopy, "commit", "-am", "replace progress")
	gitCommandFails(t, workingCopy, "push", "origin", "main")
	if got := gitCommand(t, repo.Path(), "rev-parse", "refs/heads/main"); got != advanced+"\n" {
		t.Fatalf("remote main after unforced rewrite = %q, want %s", got, advanced)
	}
	gitCommand(t, repo.Path(), "fsck", "--full")
	if _, err := repo.ListObjects(); err != nil {
		t.Fatalf("objects after rejected rewrite are inconsistent: %v", err)
	}

	gitCommand(t, workingCopy, "branch", "secondary", advanced)
	gitCommand(t, workingCopy, "tag", "secondary-tag", advanced)
	gitCommand(t, workingCopy, "push", "origin", "secondary")
	gitCommand(t, workingCopy, "push", "origin", advanced+":refs/heads/feature/main")
	gitCommandFails(t, workingCopy, "push", "origin", "secondary-tag")
	wantRefs := advanced + "\tHEAD\n" + advanced + "\trefs/heads/feature/main\n" + advanced + "\trefs/heads/main\n" + advanced + "\trefs/heads/secondary\n"
	if got := lsRemote(t, server.URL+"/git/"+repo.ID()+".git"); got != wantRefs {
		t.Fatalf("remote refs after branch pushes = %q", got)
	}
}

func TestGitStockClientCandidateBranchLifecycleLeavesPrimaryUnchanged(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, err := store.Create("candidate-branches")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(newHandler(store))
	t.Cleanup(server.Close)
	remoteURL := server.URL + "/git/" + repo.ID() + ".git"

	publisher := filepath.Join(t.TempDir(), "publisher")
	gitCommand(t, "", "init", "--initial-branch=main", publisher)
	gitCommand(t, publisher, "config", "user.name", "Contributor")
	gitCommand(t, publisher, "config", "user.email", "contributor@example.com")
	gitCommand(t, publisher, "remote", "add", "origin", remoteURL)
	if err := os.WriteFile(filepath.Join(publisher, "maintained.txt"), []byte("stable\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, publisher, "add", "maintained.txt")
	gitCommand(t, publisher, "commit", "-m", "maintained version")
	mainCommit := strings.TrimSpace(gitCommand(t, publisher, "rev-parse", "HEAD"))
	gitCommand(t, publisher, "push", "-u", "origin", "main")

	gitCommand(t, publisher, "switch", "-c", "candidate/parser")
	if err := os.WriteFile(filepath.Join(publisher, "candidate.txt"), []byte("first draft\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, publisher, "add", "candidate.txt")
	gitCommand(t, publisher, "commit", "-m", "publish candidate")
	firstCandidate := strings.TrimSpace(gitCommand(t, publisher, "rev-parse", "HEAD"))
	gitCommand(t, publisher, "push", "-u", "origin", "candidate/parser")

	wantRefs := mainCommit + "\tHEAD\n" + firstCandidate + "\trefs/heads/candidate/parser\n" + mainCommit + "\trefs/heads/main\n"
	if got := lsRemote(t, remoteURL); got != wantRefs {
		t.Fatalf("discovered refs = %q, want %q", got, wantRefs)
	}

	observer := filepath.Join(t.TempDir(), "observer")
	gitCommand(t, "", "clone", remoteURL, observer)
	gitCommand(t, observer, "fetch", "origin", "candidate/parser")
	if got := gitCommand(t, observer, "rev-parse", "refs/remotes/origin/candidate/parser"); got != firstCandidate+"\n" {
		t.Fatalf("fetched candidate = %q, want %s", got, firstCandidate)
	}
	if got := gitCommand(t, observer, "rev-parse", "main"); got != mainCommit+"\n" {
		t.Fatalf("observer main after candidate fetch = %q, want %s", got, mainCommit)
	}

	if err := os.WriteFile(filepath.Join(publisher, "candidate.txt"), []byte("second draft\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, publisher, "commit", "-am", "revise candidate")
	secondCandidate := strings.TrimSpace(gitCommand(t, publisher, "rev-parse", "HEAD"))
	gitCommand(t, publisher, "push", "origin", "candidate/parser")
	gitCommand(t, observer, "fetch", "origin")
	if got := gitCommand(t, observer, "rev-parse", "refs/remotes/origin/candidate/parser"); got != secondCandidate+"\n" {
		t.Fatalf("updated candidate = %q, want %s", got, secondCandidate)
	}

	gitCommand(t, publisher, "push", "origin", "--delete", "candidate/parser")
	if got := lsRemote(t, remoteURL); got != mainCommit+"\tHEAD\n"+mainCommit+"\trefs/heads/main\n" {
		t.Fatalf("refs after candidate deletion = %q", got)
	}
	if got := gitCommand(t, repo.Path(), "rev-parse", "refs/heads/main"); got != mainCommit+"\n" {
		t.Fatalf("main after candidate lifecycle = %q, want %s", got, mainCommit)
	}
}

func TestGitStockClientSingleBranchWorkflow(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, err := store.Create("destructive-lifecycle")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(newHandler(store))
	t.Cleanup(server.Close)
	remoteURL := server.URL + "/git/" + repo.ID() + ".git"

	publisher := filepath.Join(t.TempDir(), "publisher")
	gitCommand(t, "", "init", "--initial-branch=main", publisher)
	gitCommand(t, publisher, "config", "user.name", "Publisher")
	gitCommand(t, publisher, "config", "user.email", "publisher@example.com")
	gitCommand(t, publisher, "remote", "add", "origin", remoteURL)
	if err := os.WriteFile(filepath.Join(publisher, "history.txt"), []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, publisher, "add", "history.txt")
	gitCommand(t, publisher, "commit", "-m", "original history")
	original := strings.TrimSpace(gitCommand(t, publisher, "rev-parse", "HEAD"))
	gitCommand(t, publisher, "push", "-u", "origin", "main")

	observer := filepath.Join(t.TempDir(), "observer")
	gitCommand(t, "", "clone", remoteURL, observer)
	assertFile(t, filepath.Join(observer, "history.txt"), "original\n", false)

	if err := os.WriteFile(filepath.Join(publisher, "history.txt"), []byte("advanced\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, publisher, "commit", "-am", "advance history")
	advanced := strings.TrimSpace(gitCommand(t, publisher, "rev-parse", "HEAD"))
	gitCommand(t, publisher, "push", "origin", "main")
	gitCommand(t, observer, "pull", "--ff-only")
	if got := gitCommand(t, observer, "rev-list", "--first-parent", "HEAD"); got != advanced+"\n"+original+"\n" {
		t.Fatalf("observer history after ordinary push and pull = %q", got)
	}
	assertFile(t, filepath.Join(observer, "history.txt"), "advanced\n", false)

	gitCommand(t, publisher, "checkout", "--orphan", "replacement")
	gitCommand(t, publisher, "rm", "-rf", ".")
	if err := os.WriteFile(filepath.Join(publisher, "history.txt"), []byte("replacement\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, publisher, "add", "history.txt")
	gitCommand(t, publisher, "commit", "-m", "replacement history")
	replacement := strings.TrimSpace(gitCommand(t, publisher, "rev-parse", "HEAD"))
	gitCommand(t, publisher, "branch", "-M", "main")

	gitCommand(t, publisher, "push", "--force", "origin", "main")
	wantReplacementRefs := replacement + "\tHEAD\n" + replacement + "\trefs/heads/main\n"
	if got := lsRemote(t, remoteURL); got != wantReplacementRefs {
		t.Fatalf("remote refs after force update = %q, want %q", got, wantReplacementRefs)
	}
	replacementClone := filepath.Join(t.TempDir(), "replacement-clone")
	gitCommand(t, "", "clone", remoteURL, replacementClone)
	assertFile(t, filepath.Join(replacementClone, "history.txt"), "replacement\n", false)
	gitCommand(t, observer, "fetch", "--prune", "origin")
	if got := gitCommand(t, observer, "rev-parse", "refs/remotes/origin/main"); got != replacement+"\n" {
		t.Fatalf("observer origin/main after force update = %q, want %s", got, replacement)
	}
	if got := gitCommand(t, observer, "rev-parse", "HEAD"); got != advanced+"\n" {
		t.Fatalf("observer HEAD moved by fetch = %q, want %s", got, advanced)
	}

	gitCommand(t, publisher, "push", "origin", ":main")
	if got := lsRemote(t, "--symref", remoteURL); got != "" {
		t.Fatalf("remote refs after deletion = %q, want empty", got)
	}
	emptyClone := filepath.Join(t.TempDir(), "empty-clone")
	gitCommand(t, "", "clone", remoteURL, emptyClone)
	if got := gitCommand(t, emptyClone, "symbolic-ref", "HEAD"); got != "refs/heads/main\n" {
		t.Fatalf("empty clone HEAD = %q, want refs/heads/main", got)
	}
	gitCommand(t, observer, "fetch", "--prune", "origin")
	gitCommandFails(t, observer, "rev-parse", "--verify", "refs/remotes/origin/main")

	gitCommand(t, publisher, "push", "origin", "main")
	if got := lsRemote(t, remoteURL); got != wantReplacementRefs {
		t.Fatalf("remote refs after recovery = %q, want %q", got, wantReplacementRefs)
	}
	gitCommand(t, emptyClone, "pull", "origin", "main")
	if got := gitCommand(t, emptyClone, "rev-parse", "HEAD"); got != replacement+"\n" {
		t.Fatalf("recovered clone HEAD = %q, want %s", got, replacement)
	}
	assertFile(t, filepath.Join(emptyClone, "history.txt"), "replacement\n", false)
	gitCommand(t, repo.Path(), "fsck", "--full")
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
	if response.StatusCode != 404 {
		t.Fatalf("missing receive-pack repository status = %d", response.StatusCode)
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

func gitCommandFails(t *testing.T, directory string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = directory
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("git %s unexpectedly succeeded:\n%s", strings.Join(arguments, " "), output)
	}
	return string(output)
}

func gitCommandWithEnv(t *testing.T, directory string, environment []string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = directory
	command.Env = append(os.Environ(), environment...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(arguments, " "), err, output)
	}
	return string(output)
}

func packIndexes(t *testing.T, workingCopy string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(workingCopy, ".git", "objects", "pack", "*.idx"))
	if err != nil {
		t.Fatal(err)
	}
	return matches
}

func difference(after, before []string) []string {
	existing := make(map[string]struct{}, len(before))
	for _, value := range before {
		existing[value] = struct{}{}
	}
	var added []string
	for _, value := range after {
		if _, ok := existing[value]; !ok {
			added = append(added, value)
		}
	}
	return added
}

func packedObjectIDs(t *testing.T, workingCopy, indexPath string) []string {
	t.Helper()
	output := gitCommand(t, workingCopy, "verify-pack", "-v", indexPath)
	var ids []string
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && len(fields[0]) == 40 {
			if _, err := hex.DecodeString(fields[0]); err == nil {
				ids = append(ids, fields[0])
			}
		}
	}
	sort.Strings(ids)
	return ids
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
