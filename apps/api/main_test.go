package main

import (
	"context"
	"fmt"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

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
