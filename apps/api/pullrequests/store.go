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

type Store struct {
	root          string
	git           *storage.Store
	mu            sync.Mutex
	now           func() time.Time
	directorySync func(string) error
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
	return &Store{root: abs, git: git, now: func() time.Time { return time.Now().UTC() }, directorySync: syncDirectory}, nil
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
		return PullRequest{}, ErrBranchNotFound
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
	if err != nil || ref.Symbolic {
		return "", ErrBranchNotFound
	}
	object, err := repository.ReadObject(storage.ObjectID(ref.Target))
	if err != nil || object.Type != storage.CommitObject {
		return "", ErrBranchNotFound
	}
	return ref.Target, nil
}

func (s *Store) Get(repositoryID, id string) (PullRequest, error) {
	p, err := s.read(id)
	if err != nil || p.RepositoryID != repositoryID {
		return PullRequest{}, ErrNotFound
	}
	return p, nil
}

func (s *Store) List(repositoryID string) ([]PullRequest, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	result := []PullRequest{}
	for _, entry := range entries {
		id, ok := strings.CutSuffix(entry.Name(), ".json")
		if entry.IsDir() || !ok || !validID(id) {
			continue
		}
		p, err := s.read(id)
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
func (s *Store) path(id string) string { return filepath.Join(s.root, id+".json") }

func (s *Store) read(id string) (PullRequest, error) {
	if !validID(id) {
		return PullRequest{}, ErrNotFound
	}
	data, err := os.ReadFile(s.path(id))
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
	temp, err := os.CreateTemp(s.root, ".writing-")
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
	if err := os.Rename(path, s.path(p.ID)); err != nil {
		return false, err
	}
	return true, s.directorySync(s.root)
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
