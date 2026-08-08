// Package storage provides the durable boundary around Git repositories.
package storage

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"unicode/utf8"
)

const defaultBranch = "main"

// MaxObjectSize bounds both accepted writes and allocations while reading
// untrusted loose objects. It matches the common 100 MiB hosting limit.
const MaxObjectSize int64 = 100 << 20

const maxObjectHeaderSize = 64

var (
	// ErrInvalidID indicates that an identifier cannot safely name a repository.
	ErrInvalidID = errors.New("invalid repository id")
	// ErrRepositoryExists indicates that Create was called for an existing ID.
	ErrRepositoryExists = errors.New("repository already exists")
	// ErrRepositoryNotFound indicates that no repository exists for an ID.
	ErrRepositoryNotFound = errors.New("repository not found")
	// ErrInvalidRepository indicates that an ID exists but is not a repository.
	ErrInvalidRepository = errors.New("invalid repository")
	// ErrInvalidObject indicates an unsupported type or malformed object ID.
	ErrInvalidObject = errors.New("invalid git object")
	// ErrObjectNotFound indicates that a repository does not contain an object.
	ErrObjectNotFound = errors.New("git object not found")
	// ErrCorruptObject indicates that stored bytes do not match their object ID.
	ErrCorruptObject = errors.New("corrupt git object")
	// ErrInvalidReference indicates an invalid reference name or target.
	ErrInvalidReference = errors.New("invalid git reference")
	// ErrReferenceNotFound indicates that a named reference does not exist.
	ErrReferenceNotFound = errors.New("git reference not found")
	// ErrReferenceExists indicates that a reference cannot be created twice.
	ErrReferenceExists = errors.New("git reference already exists")
	// ErrReferenceLocked indicates that another reference mutation is in progress.
	ErrReferenceLocked = errors.New("git reference is locked")
	// ErrCorruptReference indicates malformed reference storage.
	ErrCorruptReference = errors.New("corrupt git reference")

	validID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
)

// ObjectType is one of the four object kinds stored by Git.
type ObjectType string

const (
	BlobObject   ObjectType = "blob"
	TreeObject   ObjectType = "tree"
	CommitObject ObjectType = "commit"
	TagObject    ObjectType = "tag"
)

// ObjectID is the lowercase hexadecimal SHA-1 identity of a Git object.
type ObjectID string

// Object is the canonical uncompressed representation returned by ReadObject.
type Object struct {
	ID      ObjectID
	Type    ObjectType
	Size    int64
	Content []byte
}

// ListObjects returns every loose object in the repository, ordered by object
// ID. Each object is fully verified through the same boundary as ReadObject.
func (r *Repository) ListObjects() ([]Object, error) {
	objectsPath := filepath.Join(r.path, "objects")
	fanouts, err := os.ReadDir(objectsPath)
	if err != nil {
		return nil, fmt.Errorf("list object directories: %w", err)
	}

	var ids []ObjectID
	for _, fanout := range fanouts {
		if !fanout.IsDir() || !isLowerHex(fanout.Name(), 2) {
			continue
		}
		entries, err := os.ReadDir(filepath.Join(objectsPath, fanout.Name()))
		if err != nil {
			return nil, fmt.Errorf("list object directory %s: %w", fanout.Name(), err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !isLowerHex(entry.Name(), 38) {
				continue
			}
			ids = append(ids, ObjectID(fanout.Name()+entry.Name()))
		}
	}

	// os.ReadDir sorts each directory independently, not the combined IDs.
	slices.Sort(ids)
	objects := make([]Object, 0, len(ids))
	for _, id := range ids {
		object, err := r.ReadObject(id)
		if err != nil {
			return nil, fmt.Errorf("enumerate object %s: %w", id, err)
		}
		objects = append(objects, object)
	}
	return objects, nil
}

// Store owns bare Git repositories below a filesystem directory.
type Store struct {
	root      string
	removeAll func(string) error
}

// Repository identifies an opened bare Git repository.
type Repository struct {
	id     string
	path   string
	device uint64
	inode  uint64
}

// Info is a validated snapshot of repository metadata.
type Info struct {
	ID            string
	DefaultBranch string
	Bare          bool
	Empty         bool
}

// New returns a filesystem-backed repository store. The root is created when
// the first repository is created.
func New(root string) (*Store, error) {
	if root == "" {
		return nil, fmt.Errorf("storage root: %w", ErrInvalidRepository)
	}

	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve storage root: %w", err)
	}
	return &Store{root: abs, removeAll: os.RemoveAll}, nil
}

// Create atomically initializes and opens an empty bare Git repository.
func (s *Store) Create(id string) (*Repository, error) {
	path, err := s.pathFor(id)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		return nil, fmt.Errorf("create storage root: %w", err)
	}
	if _, err := os.Lstat(s.deletionPath(id)); err == nil {
		return nil, fmt.Errorf("%s deletion is pending: %w", id, ErrRepositoryExists)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect pending repository deletion: %w", err)
	}

	temp, err := os.MkdirTemp(s.root, ".creating-")
	if err != nil {
		return nil, fmt.Errorf("create repository staging directory: %w", err)
	}
	defer os.RemoveAll(temp)

	if err := initializeBareRepository(temp); err != nil {
		return nil, err
	}
	if err := os.Rename(temp, path); err != nil {
		if _, statErr := os.Stat(path); statErr == nil {
			return nil, fmt.Errorf("%s: %w", id, ErrRepositoryExists)
		}
		return nil, fmt.Errorf("publish repository: %w", err)
	}
	if err := syncDirectory(path); err != nil {
		return nil, fmt.Errorf("sync repository: %w", err)
	}
	if err := syncDirectory(s.root); err != nil {
		return nil, fmt.Errorf("sync storage root: %w", err)
	}

	return s.Open(id)
}

// Fork atomically creates an independent bare repository from source. It uses
// upload-pack rather than a local filesystem copy so only published reachable
// objects cross the repository boundary.
func (s *Store) Fork(sourceID, id string) (*Repository, error) {
	source, err := s.Open(sourceID)
	if err != nil {
		return nil, err
	}
	path, err := s.pathFor(id)
	if err != nil {
		return nil, err
	}
	if sourceID == id {
		return nil, fmt.Errorf("fork source and target match: %w", ErrInvalidID)
	}
	if err := os.MkdirAll(s.root, 0o755); err != nil {
		return nil, fmt.Errorf("create storage root: %w", err)
	}
	if _, err := os.Lstat(s.deletionPath(id)); err == nil {
		return nil, fmt.Errorf("%s deletion is pending: %w", id, ErrRepositoryExists)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect pending repository deletion: %w", err)
	}
	staging, err := os.MkdirTemp(s.root, ".forking-")
	if err != nil {
		return nil, fmt.Errorf("create fork staging path: %w", err)
	}
	if err := os.Remove(staging); err != nil {
		return nil, fmt.Errorf("prepare fork staging path: %w", err)
	}
	defer os.RemoveAll(staging)
	command := exec.Command("git", "clone", "--bare", "--no-local", source.Path(), staging)
	if output, err := command.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("clone fork repository: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if err := unpackCloneObjects(staging); err != nil {
		return nil, err
	}
	// The application metadata is the durable lineage boundary. Do not retain
	// a machine-local path as a user-visible or operational Git remote.
	removeOrigin := exec.Command("git", "--git-dir="+staging, "config", "--remove-section", "remote.origin")
	if output, err := removeOrigin.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("remove fork staging remote: %w: %s", err, strings.TrimSpace(string(output)))
	}
	// A source receive or maintenance process may overlap a local clone. Never
	// publish its snapshot until stock Git proves every copied reference and
	// reachable object is complete and internally consistent.
	fsck := exec.Command("git", "--git-dir="+staging, "fsck", "--full")
	if output, err := fsck.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("verify fork repository: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if err := os.Rename(staging, path); err != nil {
		if _, statErr := os.Stat(path); statErr == nil {
			return nil, fmt.Errorf("%s: %w", id, ErrRepositoryExists)
		}
		return nil, fmt.Errorf("publish fork repository: %w", err)
	}
	if err := syncDirectory(path); err != nil {
		return nil, fmt.Errorf("sync fork repository: %w", err)
	}
	if err := syncDirectory(s.root); err != nil {
		return nil, fmt.Errorf("sync storage root: %w", err)
	}
	return s.Open(id)
}

// unpackCloneObjects restores the storage package's verified loose-object
// boundary after upload-pack delivers its safe reachable snapshot as packs.
func unpackCloneObjects(repositoryPath string) error {
	packs, err := filepath.Glob(filepath.Join(repositoryPath, "objects", "pack", "*.pack"))
	if err != nil {
		return fmt.Errorf("list fork object packs: %w", err)
	}
	type stagedPack struct{ original, staged string }
	staged := make([]stagedPack, 0, len(packs))
	for index, pack := range packs {
		target := filepath.Join(repositoryPath, fmt.Sprintf(".fork-pack-%d", index))
		if err := os.Rename(pack, target); err != nil {
			return fmt.Errorf("stage fork object pack: %w", err)
		}
		staged = append(staged, stagedPack{original: pack, staged: target})
	}
	for _, pack := range staged {
		input, err := os.Open(pack.staged)
		if err != nil {
			return fmt.Errorf("open fork object pack: %w", err)
		}
		command := exec.Command("git", "--git-dir="+repositoryPath, "unpack-objects", "-r")
		command.Stdin = input
		output, runErr := command.CombinedOutput()
		closeErr := input.Close()
		if runErr != nil {
			return fmt.Errorf("unpack fork objects: %w: %s", runErr, strings.TrimSpace(string(output)))
		}
		if closeErr != nil {
			return fmt.Errorf("close fork object pack: %w", closeErr)
		}
		base := strings.TrimSuffix(pack.original, ".pack")
		for _, path := range []string{pack.staged, base + ".idx", base + ".rev"} {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove unpacked fork object pack: %w", err)
			}
		}
	}
	return nil
}

// ImportCommit copies the objects reachable from one exact source commit into
// this repository without changing any reference. A high unpack limit keeps
// imported objects behind the package's loose-object verification boundary.
func (r *Repository) ImportCommit(source *Repository, commit ObjectID) error {
	if source == nil || !validObjectID(commit) {
		return fmt.Errorf("import commit: %w", ErrInvalidObject)
	}
	command := exec.Command("git", "-c", "fetch.unpackLimit=2147483647", "--git-dir="+r.Path(), "fetch", "--no-tags", "--no-write-fetch-head", source.Path(), string(commit))
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("import commit %s: %w: %s", commit, err, strings.TrimSpace(string(output)))
	}
	if _, err := r.ReadCommit(commit); err != nil {
		return fmt.Errorf("verify imported commit %s: %w", commit, err)
	}
	return nil
}

// Open reopens an existing repository and verifies its storage boundary.
func (s *Store) Open(id string) (*Repository, error) {
	path, err := s.pathFor(id)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%s: %w", id, ErrRepositoryNotFound)
		}
		return nil, fmt.Errorf("open repository: %w", err)
	}

	root, err := openRepositoryDirectory(path)
	if err != nil {
		return nil, invalidRepository("open repository root", err)
	}
	var identity syscall.Stat_t
	if err := syscall.Fstat(int(root.Fd()), &identity); err != nil {
		_ = root.Close()
		return nil, invalidRepository("inspect repository root", err)
	}
	_ = root.Close()
	repo := &Repository{id: id, path: path, device: uint64(identity.Dev), inode: identity.Ino}
	if _, err := repo.Inspect(); err != nil {
		return nil, err
	}
	return repo, nil
}

// Delete removes a repository by first atomically detaching its stable ID.
func (s *Store) Delete(id string) error {
	path, err := s.pathFor(id)
	if err != nil {
		return err
	}
	tombstone := s.deletionPath(id)
	if _, err := os.Lstat(tombstone); err == nil {
		if err := s.removeAll(tombstone); err != nil {
			return fmt.Errorf("resume repository deletion: %w", err)
		}
		if err := syncDirectory(s.root); err != nil {
			return fmt.Errorf("sync resumed repository deletion: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect pending repository deletion: %w", err)
	}
	if _, err := s.Open(id); err != nil {
		if errors.Is(err, ErrRepositoryNotFound) {
			return nil
		}
		return err
	}
	if err := os.Rename(path, tombstone); err != nil {
		return fmt.Errorf("detach repository: %w", err)
	}
	if err := syncDirectory(s.root); err != nil {
		return fmt.Errorf("sync repository deletion: %w", err)
	}
	if err := s.removeAll(tombstone); err != nil {
		return fmt.Errorf("remove repository: %w", err)
	}
	return syncDirectory(s.root)
}

func (s *Store) deletionPath(id string) string {
	return filepath.Join(s.root, ".deleting-"+id)
}

// ID returns the stable identifier assigned by the store.
func (r *Repository) ID() string { return r.id }

// Path returns the absolute bare-repository path for Git storage operations.
func (r *Repository) Path() string { return r.path }

// WriteObject stores content in Git's loose-object format and returns its
// content-derived identity. Publishing is atomic and never replaces an object.
func (r *Repository) WriteObject(objectType ObjectType, content []byte) (ObjectID, error) {
	if !validObjectType(objectType) {
		return "", fmt.Errorf("type %q: %w", objectType, ErrInvalidObject)
	}
	if int64(len(content)) > MaxObjectSize {
		return "", fmt.Errorf("object exceeds %d bytes: %w", MaxObjectSize, ErrInvalidObject)
	}

	header := []byte(fmt.Sprintf("%s %d\x00", objectType, len(content)))
	hash := sha1.New()
	_, _ = hash.Write(header)
	_, _ = hash.Write(content)
	id := ObjectID(hex.EncodeToString(hash.Sum(nil)))
	path, _ := r.objectPath(id)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create object directory: %w", err)
	}

	temp, err := os.CreateTemp(filepath.Dir(path), ".writing-")
	if err != nil {
		return "", fmt.Errorf("create object staging file: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)

	compressed := zlib.NewWriter(temp)
	_, writeErr := compressed.Write(header)
	if writeErr == nil {
		_, writeErr = compressed.Write(content)
	}
	if closeErr := compressed.Close(); writeErr == nil {
		writeErr = closeErr
	}
	if writeErr != nil {
		_ = temp.Close()
		return "", fmt.Errorf("write object: %w", writeErr)
	}
	if err := os.Chmod(tempPath, 0o444); err != nil {
		_ = temp.Close()
		return "", fmt.Errorf("set object permissions: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return "", fmt.Errorf("sync object: %w", err)
	}
	if err := temp.Close(); err != nil {
		return "", fmt.Errorf("close object: %w", err)
	}

	if err := os.Link(tempPath, path); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("publish object: %w", err)
		}
		// An existing path has the same name but may have been externally
		// corrupted, so verify it before reporting a successful idempotent write.
		if _, readErr := r.ReadObject(id); readErr != nil {
			return "", readErr
		}
	}
	// Sync both levels even on an idempotent retry: the existing link may be
	// from a prior call whose directory sync failed after publication.
	if err := syncDirectory(filepath.Dir(path)); err != nil {
		return "", fmt.Errorf("sync object directory: %w", err)
	}
	// The fanout directory's entry is persisted by its parent objects directory.
	if err := syncDirectory(filepath.Dir(filepath.Dir(path))); err != nil {
		return "", fmt.Errorf("sync objects directory: %w", err)
	}
	return id, nil
}

// ReadObject retrieves and verifies an object without altering its contents.
func (r *Repository) ReadObject(id ObjectID) (Object, error) {
	path, err := r.objectPath(id)
	if err != nil {
		return Object{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Object{}, fmt.Errorf("%s: %w", id, ErrObjectNotFound)
		}
		return Object{}, fmt.Errorf("open object: %w", err)
	}
	defer file.Close()

	compressed := bufio.NewReader(file)
	decompressed, err := zlib.NewReader(compressed)
	if err != nil {
		return Object{}, corruptObject(id, err)
	}
	reader := bufio.NewReaderSize(decompressed, maxObjectHeaderSize)
	header, headerErr := reader.ReadSlice(0)
	if headerErr != nil {
		_ = decompressed.Close()
		return Object{}, corruptObject(id, errors.New("invalid or oversized object header"))
	}
	// ReadSlice aliases the reader's buffer, which may be reused while reading
	// larger content. Preserve the header because it is needed for the final
	// identity verification.
	header = bytes.Clone(header[:len(header)-1])
	objectTypeText, sizeText, found := strings.Cut(string(header), " ")
	objectType := ObjectType(objectTypeText)
	size, sizeErr := strconv.ParseInt(sizeText, 10, 64)
	if !found || !validObjectType(objectType) || sizeErr != nil || size < 0 ||
		sizeText != strconv.FormatInt(size, 10) || size > MaxObjectSize {
		_ = decompressed.Close()
		return Object{}, corruptObject(id, errors.New("invalid object header"))
	}

	content := make([]byte, int(size))
	if _, err := io.ReadFull(reader, content); err != nil {
		_ = decompressed.Close()
		return Object{}, corruptObject(id, errors.New("object content shorter than declared size"))
	}
	if _, err := reader.ReadByte(); !errors.Is(err, io.EOF) {
		_ = decompressed.Close()
		return Object{}, corruptObject(id, errors.New("object content exceeds declared size"))
	}
	if err := decompressed.Close(); err != nil {
		return Object{}, corruptObject(id, err)
	}
	if _, err := compressed.ReadByte(); !errors.Is(err, io.EOF) {
		return Object{}, corruptObject(id, errors.New("garbage after compressed object"))
	}
	hash := sha1.New()
	_, _ = hash.Write(header)
	_, _ = hash.Write([]byte{0})
	_, _ = hash.Write(content)
	if hex.EncodeToString(hash.Sum(nil)) != string(id) {
		return Object{}, corruptObject(id, errors.New("object ID mismatch"))
	}
	return Object{ID: id, Type: objectType, Size: int64(len(content)), Content: content}, nil
}

// ReadBlobPreview verifies an entire loose blob while retaining at most limit
// bytes of content. It avoids allocating the full blob for browser previews.
func (r *Repository) ReadBlobPreview(id ObjectID, limit int64) (Object, bool, bool, error) {
	if limit < 0 {
		return Object{}, false, false, fmt.Errorf("preview limit: %w", ErrInvalidObject)
	}
	path, err := r.objectPath(id)
	if err != nil {
		return Object{}, false, false, err
	}
	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Object{}, false, false, fmt.Errorf("%s: %w", id, ErrObjectNotFound)
		}
		return Object{}, false, false, fmt.Errorf("open object: %w", err)
	}
	defer file.Close()
	compressed := bufio.NewReader(file)
	decompressed, err := zlib.NewReader(compressed)
	if err != nil {
		return Object{}, false, false, corruptObject(id, err)
	}
	reader := bufio.NewReaderSize(decompressed, maxObjectHeaderSize)
	header, headerErr := reader.ReadSlice(0)
	if headerErr != nil {
		_ = decompressed.Close()
		return Object{}, false, false, corruptObject(id, errors.New("invalid or oversized object header"))
	}
	header = bytes.Clone(header[:len(header)-1])
	typeText, sizeText, found := strings.Cut(string(header), " ")
	size, sizeErr := strconv.ParseInt(sizeText, 10, 64)
	if !found || ObjectType(typeText) != BlobObject || sizeErr != nil || size < 0 || sizeText != strconv.FormatInt(size, 10) || size > MaxObjectSize {
		_ = decompressed.Close()
		return Object{}, false, false, corruptObject(id, errors.New("invalid blob header"))
	}
	hash := sha1.New()
	_, _ = hash.Write(header)
	_, _ = hash.Write([]byte{0})
	sink := &blobPreviewSink{hash: hash, limit: limit}
	if written, err := io.CopyN(sink, reader, size); err != nil || written != size {
		_ = decompressed.Close()
		return Object{}, false, false, corruptObject(id, errors.New("object content shorter than declared size"))
	}
	if _, err := reader.ReadByte(); !errors.Is(err, io.EOF) {
		_ = decompressed.Close()
		return Object{}, false, false, corruptObject(id, errors.New("object content exceeds declared size"))
	}
	if err := decompressed.Close(); err != nil {
		return Object{}, false, false, corruptObject(id, err)
	}
	if _, err := compressed.ReadByte(); !errors.Is(err, io.EOF) {
		return Object{}, false, false, corruptObject(id, errors.New("garbage after compressed object"))
	}
	if hex.EncodeToString(hash.Sum(nil)) != string(id) {
		return Object{}, false, false, corruptObject(id, errors.New("object ID mismatch"))
	}
	if len(sink.carry) != 0 {
		sink.binary = true
	}
	if !sink.binary {
		for len(sink.prefix) > 0 && !utf8.Valid(sink.prefix) {
			sink.prefix = sink.prefix[:len(sink.prefix)-1]
		}
	}
	return Object{ID: id, Type: BlobObject, Size: size, Content: sink.prefix}, size > int64(len(sink.prefix)), sink.binary, nil
}

type blobPreviewSink struct {
	hash   io.Writer
	limit  int64
	prefix []byte
	carry  []byte
	binary bool
}

func (w *blobPreviewSink) Write(content []byte) (int, error) {
	if _, err := w.hash.Write(content); err != nil {
		return 0, err
	}
	remaining := int(w.limit) - len(w.prefix)
	if remaining > len(content) {
		remaining = len(content)
	}
	if remaining > 0 {
		w.prefix = append(w.prefix, content[:remaining]...)
	}
	if bytes.IndexByte(content, 0) >= 0 {
		w.binary = true
	}
	data := append(w.carry, content...)
	w.carry = w.carry[:0]
	for len(data) > 0 {
		if !utf8.FullRune(data) {
			w.carry = append(w.carry, data...)
			break
		}
		runeValue, size := utf8.DecodeRune(data)
		if runeValue == utf8.RuneError && size == 1 {
			w.binary = true
		}
		data = data[size:]
	}
	return len(content), nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

// Inspect validates the bare repository and reports its lifecycle metadata.
func (r *Repository) Inspect() (Info, error) {
	head, err := r.ReadReference("HEAD")
	if err != nil {
		return Info{}, invalidRepository("read HEAD", err)
	}
	const prefix = "refs/heads/"
	if !head.Symbolic || !strings.HasPrefix(head.Target, prefix) || len(head.Target) == len(prefix) {
		return Info{}, invalidRepository("HEAD is not a branch reference", nil)
	}

	config, err := os.Open(filepath.Join(r.path, "config"))
	if err != nil {
		return Info{}, invalidRepository("read config", err)
	}
	defer config.Close()
	core, err := readConfigSection(config, "core")
	if err != nil {
		return Info{}, invalidRepository("parse config", err)
	}
	if !strings.EqualFold(core["bare"], "true") {
		return Info{}, invalidRepository("core.bare is not true", nil)
	}
	// Version 0 is the repository format this package creates and understands.
	// Version 1 may introduce extensions whose compatibility must be evaluated
	// individually, so accepting it without extension support would be unsafe.
	if core["repositoryformatversion"] != "0" {
		return Info{}, invalidRepository("unsupported core.repositoryformatversion", nil)
	}

	for _, directory := range []string{"objects", "refs"} {
		info, err := os.Stat(filepath.Join(r.path, directory))
		if err != nil || !info.IsDir() {
			return Info{}, invalidRepository("missing "+directory+" directory", err)
		}
	}

	empty, err := directoryEmpty(filepath.Join(r.path, "objects"))
	if err != nil {
		return Info{}, invalidRepository("inspect objects", err)
	}
	return Info{
		ID:            r.id,
		DefaultBranch: strings.TrimPrefix(head.Target, prefix),
		Bare:          true,
		Empty:         empty,
	}, nil
}

func (s *Store) pathFor(id string) (string, error) {
	if !validID.MatchString(id) || id == "." || id == ".." {
		return "", fmt.Errorf("%q: %w", id, ErrInvalidID)
	}
	return filepath.Join(s.root, id+".git"), nil
}

func (r *Repository) objectPath(id ObjectID) (string, error) {
	value := string(id)
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha1.Size || value != strings.ToLower(value) {
		return "", fmt.Errorf("object ID %q: %w", id, ErrInvalidObject)
	}
	return filepath.Join(r.path, "objects", value[:2], value[2:]), nil
}

func validObjectType(objectType ObjectType) bool {
	switch objectType {
	case BlobObject, TreeObject, CommitObject, TagObject:
		return true
	default:
		return false
	}
}

func isLowerHex(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func corruptObject(id ObjectID, err error) error {
	return fmt.Errorf("%s: %w: %v", id, ErrCorruptObject, err)
}

func initializeBareRepository(path string) error {
	directories := []string{
		"branches", "hooks", "info", "objects/info", "objects/pack",
		"refs/heads", "refs/tags",
	}
	for _, directory := range directories {
		if err := os.MkdirAll(filepath.Join(path, directory), 0o755); err != nil {
			return fmt.Errorf("initialize repository: %w", err)
		}
	}

	files := map[string]string{
		"HEAD":        "ref: refs/heads/" + defaultBranch + "\n",
		"config":      "[core]\n\trepositoryformatversion = 0\n\tfilemode = true\n\tbare = true\n\tlogallrefupdates = false\n",
		"description": "Unnamed repository; edit this file to name the repository.\n",
	}
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(path, name), []byte(contents), 0o644); err != nil {
			return fmt.Errorf("initialize repository: %w", err)
		}
	}
	return nil
}

func directoryEmpty(path string) (bool, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if entry.Name() != "info" && entry.Name() != "pack" {
			return false, nil
		}
		children, err := os.ReadDir(filepath.Join(path, entry.Name()))
		if err != nil {
			return false, err
		}
		if len(children) != 0 {
			return false, nil
		}
	}
	return true, nil
}

func readConfigSection(file *os.File, wanted string) (map[string]string, error) {
	values := make(map[string]string)
	section := ""
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			closingBracket := strings.IndexByte(line, ']')
			if closingBracket < 0 {
				return nil, errors.New("unterminated config section")
			}
			remainder := strings.TrimSpace(line[closingBracket+1:])
			if remainder != "" && !strings.HasPrefix(remainder, "#") && !strings.HasPrefix(remainder, ";") {
				return nil, errors.New("invalid text after config section")
			}
			section = strings.ToLower(strings.TrimSpace(line[1:closingBracket]))
			continue
		}
		if section != wanted {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		value, err := parseConfigValue(value)
		if err != nil {
			return nil, err
		}
		values[strings.ToLower(strings.TrimSpace(key))] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

func parseConfigValue(raw string) (string, error) {
	var value strings.Builder
	quoted := false
	escaped := false
	for _, character := range strings.TrimSpace(raw) {
		if escaped {
			switch character {
			case 'n':
				value.WriteByte('\n')
			case 't':
				value.WriteByte('\t')
			case 'b':
				value.WriteByte('\b')
			case '\\', '"':
				value.WriteRune(character)
			default:
				return "", fmt.Errorf("invalid config escape %q", character)
			}
			escaped = false
			continue
		}
		if character == '\\' {
			escaped = true
			continue
		}
		if character == '"' {
			quoted = !quoted
			continue
		}
		if !quoted && (character == '#' || character == ';') {
			break
		}
		value.WriteRune(character)
	}
	if quoted || escaped {
		return "", errors.New("unterminated quoted config value")
	}
	return strings.TrimSpace(value.String()), nil
}

func invalidRepository(message string, err error) error {
	if err == nil {
		return fmt.Errorf("%s: %w", message, ErrInvalidRepository)
	}
	return fmt.Errorf("%s: %w: %v", message, ErrInvalidRepository, err)
}
