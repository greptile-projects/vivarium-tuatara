package storage_test

import (
	"bytes"
	"compress/zlib"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

func TestWriteAndReadObjectsAreGitCompatible(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, err := store.Create("objects")
	if err != nil {
		t.Fatal(err)
	}

	blob := []byte("hello from storage\n")
	blobID := writeObject(t, repo, storage.BlobObject, blob)
	tree := append([]byte("100644 greeting.txt\x00"), decodeObjectID(t, blobID)...)
	treeID := writeObject(t, repo, storage.TreeObject, tree)
	commit := []byte(fmt.Sprintf("tree %s\nauthor Test Author <test@example.com> 1700000000 +0000\ncommitter Test Author <test@example.com> 1700000000 +0000\n\ninitial commit\n", treeID))
	commitID := writeObject(t, repo, storage.CommitObject, commit)
	tag := []byte(fmt.Sprintf("object %s\ntype commit\ntag v1.0.0\ntagger Test Author <test@example.com> 1700000000 +0000\n\nrelease\n", commitID))
	tagID := writeObject(t, repo, storage.TagObject, tag)

	objects := []struct {
		id      storage.ObjectID
		kind    storage.ObjectType
		content []byte
	}{
		{blobID, storage.BlobObject, blob},
		{treeID, storage.TreeObject, tree},
		{commitID, storage.CommitObject, commit},
		{tagID, storage.TagObject, tag},
	}
	for _, want := range objects {
		got, err := repo.ReadObject(want.id)
		if err != nil {
			t.Fatalf("ReadObject(%s): %v", want.id, err)
		}
		if got.ID != want.id || got.Type != want.kind || got.Size != int64(len(want.content)) || !bytes.Equal(got.Content, want.content) {
			t.Errorf("ReadObject(%s) = %#v, want type %q and exact content", want.id, got, want.kind)
		}

		gitType := gitOutput(t, repo.Path(), "cat-file", "-t", string(want.id))
		if gitType != string(want.kind)+"\n" {
			t.Errorf("git type for %s = %q, want %q", want.id, gitType, want.kind)
		}
		gitContent := gitOutputBytes(t, repo.Path(), "cat-file", string(want.kind), string(want.id))
		if !bytes.Equal(gitContent, want.content) {
			t.Errorf("git content for %s changed: got %q, want %q", want.id, gitContent, want.content)
		}
	}

	// Writing identical canonical bytes is idempotent.
	if id, err := repo.WriteObject(storage.BlobObject, blob); err != nil || id != blobID {
		t.Fatalf("duplicate WriteObject = %s, %v; want %s", id, err, blobID)
	}
	gitOutput(t, repo.Path(), "fsck", "--full")
}

func TestTreesAndCommitAncestryAreGitCompatible(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, err := store.Create("history")
	if err != nil {
		t.Fatal(err)
	}

	readmeID := writeObject(t, repo, storage.BlobObject, []byte("# project\n"))
	mainID := writeObject(t, repo, storage.BlobObject, []byte("package main\n"))
	sourceTree := append([]byte("100644 main.go\x00"), decodeObjectID(t, mainID)...)
	sourceTreeID := writeObject(t, repo, storage.TreeObject, sourceTree)
	rootTree := append([]byte("100644 README.md\x00"), decodeObjectID(t, readmeID)...)
	rootTree = append(rootTree, []byte("40000 src\x00")...)
	rootTree = append(rootTree, decodeObjectID(t, sourceTreeID)...)
	rootTreeID := writeObject(t, repo, storage.TreeObject, rootTree)

	firstID := writeObject(t, repo, storage.CommitObject, commitContent(rootTreeID, nil, 1700000000, "initial snapshot"))
	secondID := writeObject(t, repo, storage.CommitObject, commitContent(rootTreeID, []storage.ObjectID{firstID}, 1700000100, "add source"))
	sideID := writeObject(t, repo, storage.CommitObject, commitContent(rootTreeID, []storage.ObjectID{firstID}, 1700000200, "side work"))
	mergeID := writeObject(t, repo, storage.CommitObject, commitContent(rootTreeID, []storage.ObjectID{secondID, sideID}, 1700000300, "merge work"))
	if err := repo.CreateReference(storage.Reference{Name: "refs/heads/main", Target: string(mergeID)}); err != nil {
		t.Fatal(err)
	}

	entries, err := repo.ReadTree(rootTreeID)
	if err != nil {
		t.Fatal(err)
	}
	wantEntries := []storage.TreeEntry{
		{Name: "README.md", Mode: "100644", ID: readmeID, Type: storage.BlobObject},
		{Name: "src", Mode: "40000", ID: sourceTreeID, Type: storage.TreeObject},
	}
	if !slices.Equal(entries, wantEntries) {
		t.Fatalf("ReadTree = %#v, want %#v", entries, wantEntries)
	}
	walked, err := repo.WalkTree(rootTreeID)
	if err != nil {
		t.Fatal(err)
	}
	wantPaths := []string{"README.md", "src", "src/main.go"}
	for index, want := range wantPaths {
		if walked[index].Path != want {
			t.Errorf("walked path %d = %q, want %q", index, walked[index].Path, want)
		}
	}
	gitTree := gitOutput(t, repo.Path(), "ls-tree", "-r", "--name-only", string(rootTreeID))
	if gitTree != "README.md\nsrc/main.go\n" {
		t.Fatalf("git ls-tree = %q", gitTree)
	}

	merge, err := repo.ReadCommit(mergeID)
	if err != nil {
		t.Fatal(err)
	}
	if merge.Tree != rootTreeID || !slices.Equal(merge.Parents, []storage.ObjectID{secondID, sideID}) || string(merge.Message) != "merge work\n" {
		t.Fatalf("ReadCommit = %#v", merge)
	}
	ancestry, err := repo.ListCommitAncestry(mergeID)
	if err != nil {
		t.Fatal(err)
	}
	gotIDs := make([]storage.ObjectID, len(ancestry))
	for index, commit := range ancestry {
		gotIDs[index] = commit.ID
	}
	wantIDs := []storage.ObjectID{mergeID, secondID, firstID, sideID}
	if !slices.Equal(gotIDs, wantIDs) {
		t.Fatalf("ListCommitAncestry IDs = %v, want %v", gotIDs, wantIDs)
	}
	if got := gitOutput(t, repo.Path(), "log", "--format=%H", "--all"); got != strings.Join([]string{string(mergeID), string(sideID), string(secondID), string(firstID)}, "\n")+"\n" {
		t.Fatalf("git log = %q", got)
	}
	if got := gitOutput(t, repo.Path(), "cat-file", "-p", string(mergeID)); got != string(commitContent(rootTreeID, []storage.ObjectID{secondID, sideID}, 1700000300, "merge work")) {
		t.Fatalf("git cat-file commit changed content: %q", got)
	}
}

func TestGraphReadersRejectWrongTypesAndBrokenEdges(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, err := store.Create("broken-graph")
	if err != nil {
		t.Fatal(err)
	}
	blobID := writeObject(t, repo, storage.BlobObject, []byte("blob"))
	if _, err := repo.ReadTree(blobID); !errors.Is(err, storage.ErrCorruptObject) {
		t.Fatalf("ReadTree(blob) error = %v", err)
	}
	badTree := append([]byte("40000 directory\x00"), decodeObjectID(t, blobID)...)
	badTreeID := writeObject(t, repo, storage.TreeObject, badTree)
	if _, err := repo.WalkTree(badTreeID); !errors.Is(err, storage.ErrCorruptObject) {
		t.Fatalf("WalkTree(type mismatch) error = %v", err)
	}
	missingParent := storage.ObjectID(strings.Repeat("0", 40))
	emptyTreeID := writeObject(t, repo, storage.TreeObject, nil)
	commitID := writeObject(t, repo, storage.CommitObject, commitContent(emptyTreeID, []storage.ObjectID{missingParent}, 1700000000, "broken"))
	if _, err := repo.ListCommitAncestry(commitID); !errors.Is(err, storage.ErrCorruptObject) {
		t.Fatalf("ListCommitAncestry(missing parent) error = %v", err)
	}
}

func TestReadTreeRejectsDuplicateAndNoncanonicalEntries(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, err := store.Create("tree-order")
	if err != nil {
		t.Fatal(err)
	}
	blobID := writeObject(t, repo, storage.BlobObject, []byte("content"))
	entry := func(name string) []byte {
		return append([]byte("100644 "+name+"\x00"), decodeObjectID(t, blobID)...)
	}
	for _, test := range []struct {
		name    string
		entries []string
	}{
		{name: "duplicate", entries: []string{"same", "same"}},
		{name: "unsorted", entries: []string{"second", "first"}},
	} {
		var content []byte
		for _, name := range test.entries {
			content = append(content, entry(name)...)
		}
		id := writeObject(t, repo, storage.TreeObject, content)
		if _, err := repo.ReadTree(id); !errors.Is(err, storage.ErrCorruptObject) {
			t.Errorf("ReadTree(%s) error = %v", test.name, err)
		}
	}
	emptyTreeID := writeObject(t, repo, storage.TreeObject, nil)
	mixedMode := entry("same")
	mixedMode = append(mixedMode, []byte("40000 same\x00")...)
	mixedMode = append(mixedMode, decodeObjectID(t, emptyTreeID)...)
	mixedModeID := writeObject(t, repo, storage.TreeObject, mixedMode)
	if _, err := repo.ReadTree(mixedModeID); !errors.Is(err, storage.ErrCorruptObject) {
		t.Fatalf("ReadTree(mixed-mode duplicate) error = %v", err)
	}
}

func TestReadCommitRejectsLateParentHeader(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, err := store.Create("late-parent")
	if err != nil {
		t.Fatal(err)
	}
	treeID := writeObject(t, repo, storage.TreeObject, nil)
	parentID := writeObject(t, repo, storage.CommitObject, commitContent(treeID, nil, 1700000000, "parent"))
	content := []byte(fmt.Sprintf("tree %s\nauthor Test Author <test@example.com> 1700000001 +0000\ncommitter Test Author <test@example.com> 1700000001 +0000\nparent %s\n\nlate parent\n", treeID, parentID))
	commitID := writeObject(t, repo, storage.CommitObject, content)
	if _, err := repo.ReadCommit(commitID); !errors.Is(err, storage.ErrCorruptObject) {
		t.Fatalf("ReadCommit(late parent) error = %v", err)
	}
}

func commitContent(tree storage.ObjectID, parents []storage.ObjectID, timestamp int64, message string) []byte {
	var content strings.Builder
	fmt.Fprintf(&content, "tree %s\n", tree)
	for _, parent := range parents {
		fmt.Fprintf(&content, "parent %s\n", parent)
	}
	fmt.Fprintf(&content, "author Test Author <test@example.com> %d +0000\n", timestamp)
	fmt.Fprintf(&content, "committer Test Author <test@example.com> %d +0000\n\n%s\n", timestamp, message)
	return []byte(content.String())
}

func TestListObjectsMatchesGitBatchAllObjects(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, err := store.Create("enumeration")
	if err != nil {
		t.Fatal(err)
	}

	blob := []byte("discoverable content\n")
	blobID := writeObject(t, repo, storage.BlobObject, blob)
	tree := append([]byte("100644 file.txt\x00"), decodeObjectID(t, blobID)...)
	treeID := writeObject(t, repo, storage.TreeObject, tree)
	commit := []byte(fmt.Sprintf("tree %s\nauthor Test Author <test@example.com> 1700000000 +0000\ncommitter Test Author <test@example.com> 1700000000 +0000\n\nenumerate objects\n", treeID))
	commitID := writeObject(t, repo, storage.CommitObject, commit)
	tag := []byte(fmt.Sprintf("object %s\ntype commit\ntag discovered\ntagger Test Author <test@example.com> 1700000000 +0000\n\nvisible tag\n", commitID))
	writeObject(t, repo, storage.TagObject, tag)

	got, err := repo.ListObjects()
	if err != nil {
		t.Fatal(err)
	}
	gitLines := strings.Split(strings.TrimSpace(gitOutput(t, repo.Path(), "cat-file",
		"--batch-check=%(objectname) %(objecttype) %(objectsize)", "--batch-all-objects")), "\n")
	if len(got) != len(gitLines) {
		t.Fatalf("ListObjects returned %d objects, git returned %d: %q", len(got), len(gitLines), gitLines)
	}
	for index, object := range got {
		wantLine := fmt.Sprintf("%s %s %d", object.ID, object.Type, object.Size)
		if wantLine != gitLines[index] {
			t.Errorf("object %d metadata = %q, git reports %q", index, wantLine, gitLines[index])
		}
		gitContent := gitOutputBytes(t, repo.Path(), "cat-file", string(object.Type), string(object.ID))
		if !bytes.Equal(object.Content, gitContent) {
			t.Errorf("object %s content differs from git cat-file", object.ID)
		}
		if object.Size != int64(len(object.Content)) {
			t.Errorf("object %s size = %d, content length = %d", object.ID, object.Size, len(object.Content))
		}
	}
}

func TestListObjectsRejectsCorruptDiscoverableObject(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, err := store.Create("corrupt-enumeration")
	if err != nil {
		t.Fatal(err)
	}
	id := writeObject(t, repo, storage.BlobObject, []byte("content"))
	path := filepath.Join(repo.Path(), "objects", string(id)[:2], string(id)[2:])
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not an object"), 0o444); err != nil {
		t.Fatal(err)
	}

	if _, err := repo.ListObjects(); !errors.Is(err, storage.ErrCorruptObject) {
		t.Fatalf("ListObjects corrupt object error = %v", err)
	}
}

func TestReferenceLifecycleIsGitCompatible(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, err := store.Create("references")
	if err != nil {
		t.Fatal(err)
	}

	head, err := repo.ReadReference("HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if head != (storage.Reference{Name: "HEAD", Target: "refs/heads/main", Symbolic: true}) {
		t.Fatalf("initial HEAD = %#v", head)
	}
	if got := gitOutput(t, repo.Path(), "symbolic-ref", "HEAD"); got != "refs/heads/main\n" {
		t.Fatalf("git symbolic-ref HEAD = %q", got)
	}

	firstID := writeObject(t, repo, storage.BlobObject, []byte("first"))
	main := storage.Reference{Name: "refs/heads/main", Target: string(firstID)}
	if err := repo.CreateReference(main); err != nil {
		t.Fatal(err)
	}
	if got := gitOutput(t, repo.Path(), "rev-parse", "refs/heads/main"); got != string(firstID)+"\n" {
		t.Fatalf("git main = %q", got)
	}

	secondID := writeObject(t, repo, storage.BlobObject, []byte("second"))
	main.Target = string(secondID)
	if err := repo.UpdateReference(main); err != nil {
		t.Fatal(err)
	}
	if got, err := repo.ReadReference(main.Name); err != nil || got != main {
		t.Fatalf("updated main = %#v, %v", got, err)
	}

	if err := repo.CreateReference(storage.Reference{Name: "refs/tags/latest", Target: "refs/heads/main", Symbolic: true}); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateReference(storage.Reference{Name: "HEAD", Target: "refs/heads/next", Symbolic: true}); err != nil {
		t.Fatal(err)
	}
	references, err := repo.ListReferences()
	if err != nil {
		t.Fatal(err)
	}
	want := []storage.Reference{
		{Name: "HEAD", Target: "refs/heads/next", Symbolic: true},
		main,
		{Name: "refs/tags/latest", Target: "refs/heads/main", Symbolic: true},
	}
	if !slices.Equal(references, want) {
		t.Fatalf("ListReferences = %#v, want %#v", references, want)
	}
	if got := gitOutput(t, repo.Path(), "symbolic-ref", "HEAD"); got != "refs/heads/next\n" {
		t.Fatalf("updated git HEAD = %q", got)
	}

	if err := repo.DeleteReference("refs/tags/latest"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ReadReference("refs/tags/latest"); !errors.Is(err, storage.ErrReferenceNotFound) {
		t.Fatalf("deleted reference error = %v", err)
	}
}

func TestReferencesRejectInvalidOperationsAndCorruption(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, err := store.Create("reference-validation")
	if err != nil {
		t.Fatal(err)
	}
	id := writeObject(t, repo, storage.BlobObject, []byte("target"))

	for _, name := range []string{"", "main", "refs/heads/../escape", "refs/heads/.hidden", "refs/heads/a.lock", "refs/heads/a b"} {
		err := repo.CreateReference(storage.Reference{Name: name, Target: string(id)})
		if !errors.Is(err, storage.ErrInvalidReference) {
			t.Errorf("CreateReference(%q) error = %v", name, err)
		}
	}
	missingID := strings.Repeat("0", 40)
	if err := repo.CreateReference(storage.Reference{Name: "refs/heads/missing", Target: missingID}); !errors.Is(err, storage.ErrObjectNotFound) {
		t.Fatalf("missing target error = %v", err)
	}
	if err := repo.CreateReference(storage.Reference{Name: "refs/heads/bad-symbolic", Target: "HEAD", Symbolic: true}); !errors.Is(err, storage.ErrInvalidReference) {
		t.Fatalf("invalid symbolic target error = %v", err)
	}

	branch := storage.Reference{Name: "refs/heads/main", Target: string(id)}
	if err := repo.CreateReference(branch); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateReference(branch); !errors.Is(err, storage.ErrReferenceExists) {
		t.Fatalf("duplicate create error = %v", err)
	}
	if err := repo.UpdateReference(storage.Reference{Name: "refs/heads/absent", Target: string(id)}); !errors.Is(err, storage.ErrReferenceNotFound) {
		t.Fatalf("missing update error = %v", err)
	}
	if err := repo.DeleteReference("refs/heads/absent"); !errors.Is(err, storage.ErrReferenceNotFound) {
		t.Fatalf("missing delete error = %v", err)
	}

	badPath := filepath.Join(repo.Path(), "refs", "heads", "corrupt")
	if err := os.WriteFile(badPath, []byte("not-an-object\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ReadReference("refs/heads/corrupt"); !errors.Is(err, storage.ErrCorruptReference) {
		t.Fatalf("corrupt read error = %v", err)
	}
	if _, err := repo.ListReferences(); !errors.Is(err, storage.ErrCorruptReference) {
		t.Fatalf("corrupt list error = %v", err)
	}
}

func TestReferencesRejectIntermediateSymlinks(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, err := store.Create("symlinked-references")
	if err != nil {
		t.Fatal(err)
	}
	id := writeObject(t, repo, storage.BlobObject, []byte("target"))
	external := t.TempDir()
	for _, name := range []string{"created", "updated", "deleted"} {
		if err := os.WriteFile(filepath.Join(external, name), []byte("outside\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	heads := filepath.Join(repo.Path(), "refs", "heads")
	if err := os.Remove(heads); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, heads); err != nil {
		t.Fatal(err)
	}

	reference := func(name string) storage.Reference {
		return storage.Reference{Name: "refs/heads/" + name, Target: string(id)}
	}
	if err := repo.CreateReference(reference("created")); err == nil {
		t.Fatal("CreateReference followed an intermediate symlink")
	}
	if err := repo.UpdateReference(reference("updated")); err == nil {
		t.Fatal("UpdateReference followed an intermediate symlink")
	}
	if err := repo.DeleteReference("refs/heads/deleted"); err == nil {
		t.Fatal("DeleteReference followed an intermediate symlink")
	}
	if _, err := repo.ReadReference("refs/heads/updated"); err == nil {
		t.Fatal("ReadReference followed an intermediate symlink")
	}
	if _, err := repo.ListReferences(); err == nil {
		t.Fatal("ListReferences followed an intermediate symlink")
	}
	for _, name := range []string{"created", "updated", "deleted"} {
		contents, err := os.ReadFile(filepath.Join(external, name))
		if err != nil || string(contents) != "outside\n" {
			t.Fatalf("external %s changed: %q, %v", name, contents, err)
		}
	}
}

func TestReferencesRejectRepositoryRootReplacement(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, err := store.Create("replaced-root")
	if err != nil {
		t.Fatal(err)
	}
	parked := repo.Path() + ".parked"
	if err := os.Rename(repo.Path(), parked); err != nil {
		t.Fatal(err)
	}
	external := t.TempDir()
	if err := os.MkdirAll(filepath.Join(external, "refs", "heads"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, repo.Path()); err != nil {
		t.Fatal(err)
	}

	escaped := storage.Reference{Name: "refs/heads/escaped", Target: "refs/heads/main", Symbolic: true}
	if err := repo.CreateReference(escaped); !errors.Is(err, storage.ErrInvalidRepository) {
		t.Fatalf("CreateReference after root replacement error = %v", err)
	}
	if _, err := repo.ReadReference(escaped.Name); err == nil {
		t.Fatal("ReadReference followed a replaced repository root")
	}
	if _, err := repo.ListReferences(); err == nil {
		t.Fatal("ListReferences followed a replaced repository root")
	}
	if _, err := os.Stat(filepath.Join(external, "refs", "heads", "escaped")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("external reference was created: %v", err)
	}
}

func TestObjectsRejectInvalidInputsAndCorruption(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, err := store.Create("validation")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := repo.WriteObject(storage.ObjectType("note"), []byte("content")); !errors.Is(err, storage.ErrInvalidObject) {
		t.Fatalf("unsupported object type error = %v", err)
	}
	for _, id := range []storage.ObjectID{"", "abc", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "../0000000000000000000000000000000000000"} {
		if _, err := repo.ReadObject(id); !errors.Is(err, storage.ErrInvalidObject) {
			t.Errorf("ReadObject(%q) error = %v", id, err)
		}
	}
	missing := storage.ObjectID("0000000000000000000000000000000000000000")
	if _, err := repo.ReadObject(missing); !errors.Is(err, storage.ErrObjectNotFound) {
		t.Fatalf("missing object error = %v", err)
	}

	id := writeObject(t, repo, storage.BlobObject, []byte("original"))
	path := filepath.Join(repo.Path(), "objects", string(id)[:2], string(id)[2:])
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("not zlib"), 0o444); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ReadObject(id); !errors.Is(err, storage.ErrCorruptObject) {
		t.Fatalf("corrupt object error = %v", err)
	}
	if _, err := repo.WriteObject(storage.BlobObject, []byte("original")); !errors.Is(err, storage.ErrCorruptObject) {
		t.Fatalf("write over corrupt object error = %v", err)
	}
}

func TestReadObjectRejectsTrailingFileData(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, err := store.Create("trailing")
	if err != nil {
		t.Fatal(err)
	}
	id := writeObject(t, repo, storage.BlobObject, []byte("content"))
	path := filepath.Join(repo.Path(), "objects", string(id)[:2], string(id)[2:])
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte{0}); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ReadObject(id); !errors.Is(err, storage.ErrCorruptObject) {
		t.Fatalf("object with trailing file data error = %v", err)
	}
	if _, err := repo.WriteObject(storage.BlobObject, []byte("content")); !errors.Is(err, storage.ErrCorruptObject) {
		t.Fatalf("duplicate write with trailing file data error = %v", err)
	}
}

func TestReadObjectBoundsMalformedDecompression(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo, err := store.Create("bounded")
	if err != nil {
		t.Fatal(err)
	}
	id := storage.ObjectID("0000000000000000000000000000000000000000")
	path := filepath.Join(repo.Path(), "objects", string(id)[:2], string(id)[2:])
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	compressed := zlib.NewWriter(file)
	if _, err := compressed.Write([]byte("blob 1\x00")); err != nil {
		t.Fatal(err)
	}
	if _, err := compressed.Write(bytes.Repeat([]byte{'x'}, 8<<20)); err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	if _, err := repo.ReadObject(id); !errors.Is(err, storage.ErrCorruptObject) {
		t.Fatalf("oversized decompressed content error = %v", err)
	}
}

func writeObject(t *testing.T, repo *storage.Repository, kind storage.ObjectType, content []byte) storage.ObjectID {
	t.Helper()
	id, err := repo.WriteObject(kind, content)
	if err != nil {
		t.Fatalf("WriteObject(%q): %v", kind, err)
	}
	return id
}

func decodeObjectID(t *testing.T, id storage.ObjectID) []byte {
	t.Helper()
	decoded, err := hex.DecodeString(string(id))
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

func gitOutput(t *testing.T, gitDir string, arguments ...string) string {
	t.Helper()
	return string(gitOutputBytes(t, gitDir, arguments...))
}

func gitOutputBytes(t *testing.T, gitDir string, arguments ...string) []byte {
	t.Helper()
	command := exec.Command("git", append([]string{"--git-dir=" + gitDir}, arguments...)...)
	output, err := command.Output()
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(arguments, " "), err)
	}
	return output
}

func TestRepositoryLifecycle(t *testing.T) {
	store, err := storage.New(filepath.Join(t.TempDir(), "repositories"))
	if err != nil {
		t.Fatal(err)
	}

	repo, err := store.Create("project-1")
	if err != nil {
		t.Fatal(err)
	}
	if repo.ID() != "project-1" || !filepath.IsAbs(repo.Path()) {
		t.Fatalf("unexpected identity: ID=%q Path=%q", repo.ID(), repo.Path())
	}

	info, err := repo.Inspect()
	if err != nil {
		t.Fatal(err)
	}
	if info.ID != "project-1" || info.DefaultBranch != "main" || !info.Bare || !info.Empty {
		t.Fatalf("unexpected repository info: %+v", info)
	}

	reopened, err := store.Open("project-1")
	if err != nil {
		t.Fatal(err)
	}
	if reopened.ID() != repo.ID() || reopened.Path() != repo.Path() {
		t.Fatalf("reopened repository changed identity: %#v", reopened)
	}

	git := exec.Command("git", "--git-dir="+reopened.Path(), "rev-parse", "--is-bare-repository")
	output, err := git.CombinedOutput()
	if err != nil {
		t.Fatalf("stock Git rejected repository: %v\n%s", err, output)
	}
	if string(output) != "true\n" {
		t.Fatalf("git reported unexpected repository type: %q", output)
	}

	fsck := exec.Command("git", "--git-dir="+reopened.Path(), "fsck", "--full")
	if output, err := fsck.CombinedOutput(); err != nil {
		t.Fatalf("git fsck rejected repository: %v\n%s", err, output)
	}
}

func TestCreateRejectsDuplicateAndUnsafeIDs(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("same"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Create("same"); !errors.Is(err, storage.ErrRepositoryExists) {
		t.Fatalf("duplicate Create error = %v", err)
	}

	for _, id := range []string{"", ".", "..", "../escape", "nested/repo", "with space"} {
		if _, err := store.Create(id); !errors.Is(err, storage.ErrInvalidID) {
			t.Errorf("Create(%q) error = %v", id, err)
		}
	}
}

func TestOpenDistinguishesMissingAndInvalidRepositories(t *testing.T) {
	root := t.TempDir()
	store, err := storage.New(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Open("missing"); !errors.Is(err, storage.ErrRepositoryNotFound) {
		t.Fatalf("missing Open error = %v", err)
	}

	if err := os.Mkdir(filepath.Join(root, "broken.git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Open("broken"); !errors.Is(err, storage.ErrInvalidRepository) {
		t.Fatalf("invalid Open error = %v", err)
	}
}

func TestOpenParsesRepositoryFormat(t *testing.T) {
	store, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		id          string
		replacement string
		valid       bool
	}{
		{id: "quoted", replacement: `repositoryformatversion = "0"`, valid: true},
		{id: "commented", replacement: "repositoryformatversion = 0 # version", valid: true},
		{id: "section-comment", replacement: "repositoryformatversion = 0", valid: true},
		{id: "unsupported", replacement: "repositoryformatversion = 999", valid: false},
		{id: "missing", replacement: "", valid: false},
	} {
		repo, err := store.Create(test.id)
		if err != nil {
			t.Fatal(err)
		}
		configPath := filepath.Join(repo.Path(), "config")
		config, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatal(err)
		}
		mutated := strings.Replace(string(config), "repositoryformatversion = 0", test.replacement, 1)
		if test.id == "section-comment" {
			mutated = strings.Replace(mutated, "[core]", "[core] # repository settings", 1)
		}
		if err := os.WriteFile(configPath, []byte(mutated), 0o644); err != nil {
			t.Fatal(err)
		}

		_, err = store.Open(test.id)
		if test.valid && err != nil {
			t.Errorf("Open(%q) rejected valid Git config: %v", test.id, err)
		}
		if !test.valid && !errors.Is(err, storage.ErrInvalidRepository) {
			t.Errorf("Open(%q) error = %v, want ErrInvalidRepository", test.id, err)
		}
	}
}
