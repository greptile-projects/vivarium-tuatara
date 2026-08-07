// Package pullrequests stores repository-scoped requests to merge one branch into another.
package pullrequests

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

var (
	ErrNotFound            = errors.New("pull request not found")
	ErrInvalid             = errors.New("invalid pull request")
	ErrBranchNotFound      = errors.New("pull request branch not found")
	ErrDurabilityUncertain = errors.New("pull request is visible but durability is uncertain")
)

const Open = "open"

type PullRequest struct {
	ID             string    `json:"id"`
	RepositoryID   string    `json:"repository_id"`
	AuthorID       string    `json:"author_id"`
	Title          string    `json:"title"`
	Body           string    `json:"body"`
	SourceBranch   string    `json:"source_branch"`
	TargetBranch   string    `json:"target_branch"`
	SourceCommitID string    `json:"source_commit_id"`
	TargetCommitID string    `json:"target_commit_id"`
	ProposalID     *string   `json:"proposal_id"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type Commit struct {
	ID      string         `json:"id"`
	TreeID  string         `json:"tree_id"`
	Parents []string       `json:"parent_ids"`
	Headers []CommitHeader `json:"headers"`
	Message string         `json:"message"`
}

type CommitHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type FileChange struct {
	Path    string  `json:"path"`
	Status  string  `json:"status"`
	OldID   *string `json:"old_id"`
	NewID   *string `json:"new_id"`
	OldMode *string `json:"old_mode"`
	NewMode *string `json:"new_mode"`
}

type Comment struct {
	ID            string    `json:"id"`
	PullRequestID string    `json:"pull_request_id"`
	AuthorID      string    `json:"author_id"`
	Body          string    `json:"body"`
	CreatedAt     time.Time `json:"created_at"`
}

type commentRecord struct {
	Comments []Comment `json:"comments"`
}

type Store struct {
	root          string
	git           *storage.Store
	mu            sync.Mutex
	now           func() time.Time
	directorySync func(string) error
	rootSync      func(string) error
}

func New(root string, git *storage.Store) (*Store, error) {
	if root == "" || git == nil {
		return nil, errors.New("pull request storage requires metadata and Git storage")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, fmt.Errorf("create pull request store: %w", err)
	}
	return &Store{root: abs, git: git, now: func() time.Time { return time.Now().UTC() }, directorySync: syncDirectory, rootSync: syncDirectory}, nil
}

func (s *Store) Create(repositoryID, authorID, title, body, sourceBranch, targetBranch string, proposalID *string) (PullRequest, error) {
	if !validID(repositoryID) || !validID(authorID) {
		return PullRequest{}, ErrInvalid
	}
	title, body, err := validatePurpose(title, body)
	if err != nil {
		return PullRequest{}, err
	}
	sourceBranch, targetBranch = strings.TrimSpace(sourceBranch), strings.TrimSpace(targetBranch)
	if sourceBranch == "" || targetBranch == "" || sourceBranch == targetBranch || strings.HasPrefix(sourceBranch, "refs/") || strings.HasPrefix(targetBranch, "refs/") {
		return PullRequest{}, ErrInvalid
	}
	if proposalID != nil && !validID(*proposalID) {
		return PullRequest{}, ErrInvalid
	}
	repository, err := s.git.Open(repositoryID)
	if err != nil {
		return PullRequest{}, fmt.Errorf("open Git repository: %w", err)
	}
	sourceCommit, err := branchCommit(repository, sourceBranch)
	if err != nil {
		return PullRequest{}, err
	}
	targetCommit, err := branchCommit(repository, targetBranch)
	if err != nil {
		return PullRequest{}, err
	}
	id, err := newID()
	if err != nil {
		return PullRequest{}, err
	}
	now := s.now().Truncate(time.Microsecond)
	p := PullRequest{ID: id, RepositoryID: repositoryID, AuthorID: authorID, Title: title, Body: body, SourceBranch: sourceBranch, TargetBranch: targetBranch, SourceCommitID: sourceCommit, TargetCommitID: targetCommit, ProposalID: proposalID, Status: Open, CreatedAt: now, UpdatedAt: now}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return PullRequest{}, err
	}
	defer unlock()
	if err := s.ensureRepositoryDirectory(repositoryID); err != nil {
		return PullRequest{}, err
	}
	if committed, err := s.write(p); err != nil {
		if committed {
			return p, fmt.Errorf("%w: %v", ErrDurabilityUncertain, err)
		}
		return PullRequest{}, err
	}
	return p, nil
}

func branchCommit(repository *storage.Repository, branch string) (string, error) {
	ref, err := repository.ReadReference("refs/heads/" + branch)
	if errors.Is(err, storage.ErrReferenceNotFound) || ref.Symbolic {
		return "", ErrBranchNotFound
	}
	if err != nil {
		return "", fmt.Errorf("read branch %q: %w", branch, err)
	}
	object, err := repository.ReadObject(storage.ObjectID(ref.Target))
	if err != nil {
		return "", fmt.Errorf("read branch %q target: %w", branch, err)
	}
	if object.Type != storage.CommitObject {
		return "", ErrBranchNotFound
	}
	return ref.Target, nil
}

func (s *Store) Get(repositoryID, id string) (PullRequest, error) {
	if !validID(repositoryID) {
		return PullRequest{}, ErrNotFound
	}
	p, err := s.read(repositoryID, id)
	if err != nil {
		return PullRequest{}, err
	}
	if p.RepositoryID != repositoryID {
		return PullRequest{}, ErrNotFound
	}
	return p, nil
}

func (s *Store) List(repositoryID string) ([]PullRequest, error) {
	if !validID(repositoryID) {
		return nil, ErrNotFound
	}
	entries, err := os.ReadDir(s.repositoryPath(repositoryID))
	if errors.Is(err, os.ErrNotExist) {
		return []PullRequest{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := []PullRequest{}
	for _, entry := range entries {
		id, ok := strings.CutSuffix(entry.Name(), ".json")
		if entry.IsDir() || !ok || !validID(id) {
			continue
		}
		p, err := s.read(repositoryID, id)
		if err != nil {
			return nil, err
		}
		if p.RepositoryID == repositoryID {
			result = append(result, p)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result, nil
}

// Commits returns the source commits that are not reachable from the target
// snapshot, in depth-first parent order from the snapshotted source tip.
func (s *Store) Commits(repositoryID, id string) ([]Commit, error) {
	p, err := s.Get(repositoryID, id)
	if err != nil {
		return nil, err
	}
	repository, err := s.git.Open(repositoryID)
	if err != nil {
		return nil, err
	}
	target, err := repository.ListCommitAncestry(storage.ObjectID(p.TargetCommitID))
	if err != nil {
		return nil, err
	}
	excluded := make(map[storage.ObjectID]bool, len(target))
	for _, commit := range target {
		excluded[commit.ID] = true
	}
	source, err := repository.ListCommitAncestry(storage.ObjectID(p.SourceCommitID))
	if err != nil {
		return nil, err
	}
	result := []Commit{}
	for _, commit := range source {
		if excluded[commit.ID] {
			continue
		}
		parents := make([]string, len(commit.Parents))
		for i, parent := range commit.Parents {
			parents[i] = string(parent)
		}
		headers := make([]CommitHeader, len(commit.Headers))
		for i, header := range commit.Headers {
			headers[i] = CommitHeader{Name: header.Name, Value: header.Value}
		}
		result = append(result, Commit{ID: string(commit.ID), TreeID: string(commit.Tree), Parents: parents, Headers: headers, Message: string(commit.Message)})
	}
	return result, nil
}

// Changes compares the complete target and source snapshots by path. Tree
// container entries are omitted; files, symlinks, and gitlinks are included.
func (s *Store) Changes(repositoryID, id string) ([]FileChange, error) {
	p, err := s.Get(repositoryID, id)
	if err != nil {
		return nil, err
	}
	repository, err := s.git.Open(repositoryID)
	if err != nil {
		return nil, err
	}
	oldCommit, err := repository.ReadCommit(storage.ObjectID(p.TargetCommitID))
	if err != nil {
		return nil, err
	}
	newCommit, err := repository.ReadCommit(storage.ObjectID(p.SourceCommitID))
	if err != nil {
		return nil, err
	}
	oldPaths, err := repository.WalkTree(oldCommit.Tree)
	if err != nil {
		return nil, err
	}
	newPaths, err := repository.WalkTree(newCommit.Tree)
	if err != nil {
		return nil, err
	}
	oldFiles, newFiles := map[string]storage.TreeEntry{}, map[string]storage.TreeEntry{}
	for _, entry := range oldPaths {
		if entry.Type != storage.TreeObject {
			oldFiles[entry.Path] = entry.TreeEntry
		}
	}
	for _, entry := range newPaths {
		if entry.Type != storage.TreeObject {
			newFiles[entry.Path] = entry.TreeEntry
		}
	}
	paths := make([]string, 0, len(oldFiles)+len(newFiles))
	seen := map[string]bool{}
	for path := range oldFiles {
		paths = append(paths, path)
		seen[path] = true
	}
	for path := range newFiles {
		if !seen[path] {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	result := []FileChange{}
	for _, path := range paths {
		oldEntry, oldOK := oldFiles[path]
		newEntry, newOK := newFiles[path]
		if oldOK && newOK && oldEntry.ID == newEntry.ID && oldEntry.Mode == newEntry.Mode {
			continue
		}
		change := FileChange{Path: path}
		if oldOK {
			value, mode := string(oldEntry.ID), oldEntry.Mode
			change.OldID, change.OldMode = &value, &mode
		}
		if newOK {
			value, mode := string(newEntry.ID), newEntry.Mode
			change.NewID, change.NewMode = &value, &mode
		}
		switch {
		case !oldOK:
			change.Status = "added"
		case !newOK:
			change.Status = "deleted"
		default:
			change.Status = "modified"
		}
		result = append(result, change)
	}
	return result, nil
}

func (s *Store) AddComment(repositoryID, pullRequestID, authorID, body string) (Comment, error) {
	if !validID(authorID) {
		return Comment{}, ErrInvalid
	}
	body = strings.TrimSpace(body)
	if body == "" || len([]rune(body)) > 10000 {
		return Comment{}, ErrInvalid
	}
	if _, err := s.Get(repositoryID, pullRequestID); err != nil {
		return Comment{}, err
	}
	commentID, err := newID()
	if err != nil {
		return Comment{}, err
	}
	comment := Comment{ID: commentID, PullRequestID: pullRequestID, AuthorID: authorID, Body: body, CreatedAt: s.now().Truncate(time.Microsecond)}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Comment{}, err
	}
	defer unlock()
	record, err := s.readComments(repositoryID, pullRequestID)
	if err != nil {
		return Comment{}, err
	}
	record.Comments = append(record.Comments, comment)
	if committed, err := s.writeComments(repositoryID, pullRequestID, record); err != nil {
		if committed {
			return comment, fmt.Errorf("%w: %v", ErrDurabilityUncertain, err)
		}
		return Comment{}, err
	}
	return comment, nil
}

func (s *Store) ListComments(repositoryID, pullRequestID string) ([]Comment, error) {
	if _, err := s.Get(repositoryID, pullRequestID); err != nil {
		return nil, err
	}
	record, err := s.readComments(repositoryID, pullRequestID)
	if err != nil {
		return nil, err
	}
	return append([]Comment(nil), record.Comments...), nil
}

func (s *Store) commentsPath(repositoryID, pullRequestID string) string {
	return filepath.Join(s.repositoryPath(repositoryID), pullRequestID+".comments.json")
}

func (s *Store) readComments(repositoryID, pullRequestID string) (commentRecord, error) {
	data, err := os.ReadFile(s.commentsPath(repositoryID, pullRequestID))
	if errors.Is(err, os.ErrNotExist) {
		return commentRecord{Comments: []Comment{}}, nil
	}
	if err != nil {
		return commentRecord{}, err
	}
	var record commentRecord
	if json.Unmarshal(data, &record) != nil {
		return commentRecord{}, fmt.Errorf("corrupt pull request comments %s", pullRequestID)
	}
	seen := map[string]bool{}
	for _, comment := range record.Comments {
		if !validID(comment.ID) || comment.PullRequestID != pullRequestID || !validID(comment.AuthorID) || strings.TrimSpace(comment.Body) == "" || len([]rune(comment.Body)) > 10000 || comment.CreatedAt.IsZero() || seen[comment.ID] {
			return commentRecord{}, fmt.Errorf("corrupt pull request comments %s", pullRequestID)
		}
		seen[comment.ID] = true
	}
	return record, nil
}

func (s *Store) writeComments(repositoryID, pullRequestID string, record commentRecord) (bool, error) {
	data, err := json.Marshal(record)
	if err != nil {
		return false, err
	}
	directory := s.repositoryPath(repositoryID)
	temp, err := os.CreateTemp(directory, ".writing-comments-")
	if err != nil {
		return false, err
	}
	path := temp.Name()
	defer os.Remove(path)
	if err = temp.Chmod(0o600); err == nil {
		_, err = temp.Write(append(data, '\n'))
	}
	if err == nil {
		err = temp.Sync()
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return false, err
	}
	if err := os.Rename(path, s.commentsPath(repositoryID, pullRequestID)); err != nil {
		return false, err
	}
	return true, s.directorySync(directory)
}

func validatePurpose(title, body string) (string, string, error) {
	title, body = strings.TrimSpace(title), strings.TrimSpace(body)
	if title == "" || len([]rune(title)) > 200 || strings.ContainsAny(title, "\r\n") || len([]rune(body)) > 10000 {
		return "", "", ErrInvalid
	}
	return title, body, nil
}

func newID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
func validID(id string) bool {
	if len(id) != 32 || id != strings.ToLower(id) {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}
func (s *Store) repositoryPath(repositoryID string) string {
	return filepath.Join(s.root, repositoryID)
}

func (s *Store) path(repositoryID, id string) string {
	return filepath.Join(s.repositoryPath(repositoryID), id+".json")
}

func (s *Store) read(repositoryID, id string) (PullRequest, error) {
	if !validID(repositoryID) || !validID(id) {
		return PullRequest{}, ErrNotFound
	}
	data, err := os.ReadFile(s.path(repositoryID, id))
	if errors.Is(err, os.ErrNotExist) {
		return PullRequest{}, ErrNotFound
	}
	if err != nil {
		return PullRequest{}, err
	}
	var p PullRequest
	if json.Unmarshal(data, &p) != nil || p.ID != id || !validID(p.RepositoryID) || !validID(p.AuthorID) || p.Status != Open || !validCommitID(p.SourceCommitID) || !validCommitID(p.TargetCommitID) || p.SourceBranch == p.TargetBranch || p.SourceBranch == "" || p.TargetBranch == "" || strings.HasPrefix(p.SourceBranch, "refs/") || strings.HasPrefix(p.TargetBranch, "refs/") || p.CreatedAt.IsZero() || p.UpdatedAt.IsZero() || (p.ProposalID != nil && !validID(*p.ProposalID)) {
		return PullRequest{}, fmt.Errorf("corrupt pull request %s", id)
	}
	if _, _, err := validatePurpose(p.Title, p.Body); err != nil {
		return PullRequest{}, fmt.Errorf("corrupt pull request %s", id)
	}
	return p, nil
}

func validCommitID(id string) bool {
	if len(id) != 40 || id != strings.ToLower(id) {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

func (s *Store) write(p PullRequest) (bool, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return false, err
	}
	repositoryPath := s.repositoryPath(p.RepositoryID)
	temp, err := os.CreateTemp(repositoryPath, ".writing-")
	if err != nil {
		return false, err
	}
	path := temp.Name()
	defer os.Remove(path)
	if err = temp.Chmod(0o600); err == nil {
		_, err = temp.Write(append(data, '\n'))
	}
	if err == nil {
		err = temp.Sync()
	}
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return false, err
	}
	if err := os.Rename(path, s.path(p.RepositoryID, p.ID)); err != nil {
		return false, err
	}
	return true, s.directorySync(repositoryPath)
}

func (s *Store) ensureRepositoryDirectory(repositoryID string) error {
	path := s.repositoryPath(repositoryID)
	if err := os.Mkdir(path, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("create pull request repository directory: %w", err)
	}
	// Sync the root even for an existing directory. A retry after an uncertain
	// directory publication must not acknowledge writes beneath an unsynced entry.
	if err := s.rootSync(s.root); err != nil {
		return fmt.Errorf("sync pull request storage root: %w", err)
	}
	return nil
}

func syncDirectory(path string) error {
	d, err := os.Open(path)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
func (s *Store) lock() (func(), error) {
	f, err := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}
