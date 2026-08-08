// Package pullrequests stores repository-scoped requests to merge one branch into another.
package pullrequests

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

var (
	ErrNotFound            = errors.New("pull request not found")
	ErrInvalid             = errors.New("invalid pull request")
	ErrBranchNotFound      = errors.New("pull request branch not found")
	ErrSourceChanged       = errors.New("pull request source branch must be synchronized")
	ErrDurabilityUncertain = errors.New("pull request is visible but durability is uncertain")
	ErrNotReady            = errors.New("pull request is not ready to merge")
)

const (
	Open   = "open"
	Merged = "merged"
)

const (
	Approved         = "approved"
	ChangesRequested = "changes_requested"
	Withdrawn        = "withdrawn"
)

type PullRequest struct {
	ID             string     `json:"id"`
	RepositoryID   string     `json:"repository_id"`
	AuthorID       string     `json:"author_id"`
	Title          string     `json:"title"`
	Body           string     `json:"body"`
	SourceBranch   string     `json:"source_branch"`
	TargetBranch   string     `json:"target_branch"`
	SourceCommitID string     `json:"source_commit_id"`
	TargetCommitID string     `json:"target_commit_id"`
	ProposalID     *string    `json:"proposal_id"`
	Status         string     `json:"status"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	MergedAt       *time.Time `json:"merged_at"`
	MergedBy       *string    `json:"merged_by"`
	MergeCommitID  *string    `json:"merge_commit_id"`
	mergeIntent    *mergeIntent
}

type mergeIntent struct {
	CommitID string    `json:"commit_id"`
	MergerID string    `json:"merger_id"`
	MergedAt time.Time `json:"merged_at"`
}

type pullRequestRecord struct {
	PullRequest
	MergeIntent *mergeIntent `json:"merge_intent,omitempty"`
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

// Review is one participant's current decision. ReviewedCommitID identifies
// the live source-branch tip the participant evaluated; Stale is derived when
// the record is read and is never trusted as durable state.
type Review struct {
	ID               string    `json:"id"`
	PullRequestID    string    `json:"pull_request_id"`
	ReviewerID       string    `json:"reviewer_id"`
	Decision         string    `json:"decision"`
	ReviewedCommitID string    `json:"reviewed_commit_id"`
	Stale            bool      `json:"stale"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type BranchState struct {
	Branch           string  `json:"branch"`
	SnapshotCommitID string  `json:"snapshot_commit_id"`
	CurrentCommitID  *string `json:"current_commit_id"`
	State            string  `json:"state"`
}

type ReadinessBlocker struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type CheckRequirement struct {
	Name     string  `json:"name"`
	Status   string  `json:"status"`
	CommitID *string `json:"commit_id,omitempty"`
	RunID    *string `json:"run_id,omitempty"`
}

// MergeReadiness is derived entirely from durable pull-request, review, and
// Git state. It is never persisted and computing it does not modify the
// repository.
type MergeReadiness struct {
	Mergeable         bool               `json:"mergeable"`
	CanMerge          bool               `json:"can_merge"`
	RequiredApprovals int                `json:"required_approvals"`
	Approvals         int                `json:"approvals"`
	EvaluatedCommitID string             `json:"evaluated_commit_id"`
	RequiredChecks    []CheckRequirement `json:"required_checks"`
	Source            BranchState        `json:"source"`
	Target            BranchState        `json:"target"`
	HasConflicts      bool               `json:"has_conflicts"`
	Blockers          []ReadinessBlocker `json:"blockers"`
}

type commentRecord struct {
	Comments []Comment `json:"comments"`
}

type reviewRecord struct {
	Reviews []Review `json:"reviews"`
}

type Store struct {
	root          string
	git           *storage.Store
	mu            sync.Mutex
	now           func() time.Time
	directorySync func(string) error
	rootSync      func(string) error
	checkRuns     *checkruns.Store
	requirements  interface {
		RequiredChecks(string, string) ([]string, error)
		LockRequiredChecks() (func(), error)
	}
}

func (s *Store) ConfigureRequiredChecks(requirements interface {
	RequiredChecks(string, string) ([]string, error)
	LockRequiredChecks() (func(), error)
}, runs *checkruns.Store) {
	s.requirements, s.checkRuns = requirements, runs
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

// SynchronizeSource adopts the source branch's current commit as the next
// reviewable revision of an open pull request. Existing reviews retain the
// commit they evaluated and therefore become stale when the branch advanced.
func (s *Store) SynchronizeSource(repositoryID, id string) (PullRequest, error) {
	return s.SynchronizeSourceAfter(repositoryID, id, nil)
}

// SynchronizeSourceAfter checks synchronization eligibility and the live
// source tip under the pull-request lock, then invokes before immediately
// before publishing the new snapshot. Callers can durably prepare a related
// mutation without terminalizing it when a merge intent already blocks sync.
func (s *Store) SynchronizeSourceAfter(repositoryID, id string, before func() error) (PullRequest, error) {
	if !validID(repositoryID) || !validID(id) {
		return PullRequest{}, ErrNotFound
	}
	repository, err := s.git.Open(repositoryID)
	if err != nil {
		return PullRequest{}, fmt.Errorf("open Git repository: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return PullRequest{}, err
	}
	defer unlock()
	p, err := s.read(repositoryID, id)
	if err != nil {
		return PullRequest{}, err
	}
	if p.Status != Open || p.mergeIntent != nil {
		return PullRequest{}, ErrNotReady
	}
	commitID, err := branchCommit(repository, p.SourceBranch)
	if err != nil {
		return PullRequest{}, err
	}
	if before != nil {
		if err := before(); err != nil {
			return PullRequest{}, err
		}
	}
	if commitID == p.SourceCommitID {
		return p, nil
	}
	p.SourceCommitID = commitID
	p.UpdatedAt = s.now().Truncate(time.Microsecond)
	if committed, err := s.write(p); err != nil {
		if committed {
			return p, fmt.Errorf("%w: %v", ErrDurabilityUncertain, err)
		}
		return PullRequest{}, err
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

// CompareCommits returns the path-ordered file delta between two exact commit
// snapshots. Change-session publication uses this to describe only the work
// produced by a run, independently of the pull request's older target.
func (s *Store) CompareCommits(repositoryID, oldCommitID, newCommitID string) ([]FileChange, error) {
	repository, err := s.git.Open(repositoryID)
	if err != nil {
		return nil, err
	}
	oldCommit, err := repository.ReadCommit(storage.ObjectID(oldCommitID))
	if err != nil {
		return nil, err
	}
	newCommit, err := repository.ReadCommit(storage.ObjectID(newCommitID))
	if err != nil {
		return nil, err
	}
	return compareTrees(repository, oldCommit.Tree, newCommit.Tree)
}

func compareTrees(repository *storage.Repository, oldTree, newTree storage.ObjectID) ([]FileChange, error) {
	oldPaths, err := repository.WalkTree(oldTree)
	if err != nil {
		return nil, err
	}
	newPaths, err := repository.WalkTree(newTree)
	if err != nil {
		return nil, err
	}
	return compareTreeEntries(oldPaths, newPaths), nil
}

func compareTreeEntries(oldPaths, newPaths []storage.TreePath) []FileChange {
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
	paths, seen := make([]string, 0, len(oldFiles)+len(newFiles)), map[string]bool{}
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
	return result
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

// SetReview creates or replaces the actor's decision against the recorded
// source revision. A reviewer has exactly one durable current review.
func (s *Store) SetReview(repositoryID, pullRequestID, reviewerID, decision string) (Review, error) {
	if !validID(reviewerID) || (decision != Approved && decision != ChangesRequested) {
		return Review{}, ErrInvalid
	}
	repository, err := s.git.Open(repositoryID)
	if err != nil {
		return Review{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Review{}, err
	}
	defer unlock()
	p, err := s.read(repositoryID, pullRequestID)
	if err != nil {
		return Review{}, err
	}
	if p.Status != Open {
		return Review{}, ErrNotReady
	}
	commitID, err := branchCommit(repository, p.SourceBranch)
	if err != nil {
		return Review{}, err
	}
	if commitID != p.SourceCommitID {
		return Review{}, ErrSourceChanged
	}
	record, err := s.readReviews(repositoryID, pullRequestID)
	if err != nil {
		return Review{}, err
	}
	now := s.now().Truncate(time.Microsecond)
	var review Review
	for i := range record.Reviews {
		if record.Reviews[i].ReviewerID == reviewerID {
			review = record.Reviews[i]
			review.Decision = decision
			review.ReviewedCommitID = commitID
			review.UpdatedAt = now
			record.Reviews[i] = review
			break
		}
	}
	if review.ID == "" {
		id, err := newID()
		if err != nil {
			return Review{}, err
		}
		review = Review{ID: id, PullRequestID: pullRequestID, ReviewerID: reviewerID, Decision: decision, ReviewedCommitID: commitID, CreatedAt: now, UpdatedAt: now}
		record.Reviews = append(record.Reviews, review)
	}
	if committed, err := s.writeReviews(repositoryID, pullRequestID, record); err != nil {
		if committed {
			return review, fmt.Errorf("%w: %v", ErrDurabilityUncertain, err)
		}
		return Review{}, err
	}
	return review, nil
}

// WithdrawReview retains who evaluated which commit while making clear that
// the participant no longer has an active approval or change request.
func (s *Store) WithdrawReview(repositoryID, pullRequestID, reviewID, reviewerID string) (Review, error) {
	if !validID(reviewID) || !validID(reviewerID) {
		return Review{}, ErrNotFound
	}
	if _, err := s.Get(repositoryID, pullRequestID); err != nil {
		return Review{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Review{}, err
	}
	defer unlock()
	record, err := s.readReviews(repositoryID, pullRequestID)
	if err != nil {
		return Review{}, err
	}
	var review Review
	for i := range record.Reviews {
		if record.Reviews[i].ID == reviewID && record.Reviews[i].ReviewerID == reviewerID {
			review = record.Reviews[i]
			review.Decision = Withdrawn
			review.UpdatedAt = s.now().Truncate(time.Microsecond)
			record.Reviews[i] = review
			break
		}
	}
	if review.ID == "" {
		return Review{}, ErrNotFound
	}
	if committed, err := s.writeReviews(repositoryID, pullRequestID, record); err != nil {
		if committed {
			return review, fmt.Errorf("%w: %v", ErrDurabilityUncertain, err)
		}
		return Review{}, err
	}
	return review, nil
}

func (s *Store) ListReviews(repositoryID, pullRequestID string) ([]Review, error) {
	p, err := s.Get(repositoryID, pullRequestID)
	if err != nil {
		return nil, err
	}
	repository, err := s.git.Open(repositoryID)
	if err != nil {
		return nil, err
	}
	currentCommitID, err := branchCommit(repository, p.SourceBranch)
	if errors.Is(err, ErrBranchNotFound) {
		currentCommitID = ""
		err = nil
	}
	if err != nil {
		return nil, err
	}
	return s.reviewsAtCommit(repositoryID, pullRequestID, currentCommitID)
}

func (s *Store) reviewsAtCommit(repositoryID, pullRequestID, currentCommitID string) ([]Review, error) {
	record, err := s.readReviews(repositoryID, pullRequestID)
	if err != nil {
		return nil, err
	}
	result := append([]Review(nil), record.Reviews...)
	for i := range result {
		result[i].Stale = currentCommitID == "" || result[i].ReviewedCommitID != currentCommitID
	}
	return result, nil
}

// Readiness recomputes every repository-level merge condition against live
// branch state. The caller supplies whether the inspecting actor has merge
// authority so the report can distinguish mergeability from permission.
func (s *Store) Readiness(repositoryID, pullRequestID string, actorCanMerge bool) (MergeReadiness, error) {
	p, err := s.Get(repositoryID, pullRequestID)
	if err != nil {
		return MergeReadiness{}, err
	}
	repository, err := s.git.Open(repositoryID)
	if err != nil {
		return MergeReadiness{}, err
	}
	report := MergeReadiness{
		RequiredApprovals: 1,
		EvaluatedCommitID: p.SourceCommitID,
		RequiredChecks:    []CheckRequirement{},
		Source:            BranchState{Branch: p.SourceBranch, SnapshotCommitID: p.SourceCommitID},
		Target:            BranchState{Branch: p.TargetBranch, SnapshotCommitID: p.TargetCommitID},
		Blockers:          []ReadinessBlocker{},
	}
	addBlocker := func(code, message string) {
		report.Blockers = append(report.Blockers, ReadinessBlocker{Code: code, Message: message})
	}
	if p.Status != Open {
		addBlocker("pull_request_not_open", "pull request must be open")
	}

	sourceID, sourceState, err := liveBranchState(repository, p.SourceBranch, p.SourceCommitID)
	if err != nil {
		return MergeReadiness{}, err
	}
	report.Source.State, report.Source.CurrentCommitID = sourceState, sourceID
	if sourceID == nil {
		addBlocker("source_branch_missing", "source branch must identify a commit")
	} else if *sourceID != p.SourceCommitID {
		addBlocker("source_branch_changed", "source branch no longer matches the pull request snapshot")
	}
	targetID, targetState, err := liveBranchState(repository, p.TargetBranch, p.TargetCommitID)
	if err != nil {
		return MergeReadiness{}, err
	}
	report.Target.State, report.Target.CurrentCommitID = targetState, targetID
	if targetID == nil {
		addBlocker("target_branch_missing", "target branch must identify a commit")
	}

	currentSourceID := ""
	if sourceID != nil {
		currentSourceID = *sourceID
	}
	reviews, err := s.reviewsAtCommit(repositoryID, pullRequestID, currentSourceID)
	if err != nil {
		return MergeReadiness{}, err
	}
	changesRequested := false
	for _, review := range reviews {
		if review.Stale || review.Decision == Withdrawn {
			continue
		}
		if review.Decision == Approved {
			report.Approvals++
		} else if review.Decision == ChangesRequested {
			changesRequested = true
		}
	}
	if report.Approvals < report.RequiredApprovals {
		addBlocker("approval_required", "at least one current approval is required")
	}
	if changesRequested {
		addBlocker("changes_requested", "a current review requests changes")
	}
	if s.requirements != nil {
		names, requirementErr := s.requirements.RequiredChecks(repositoryID, p.TargetBranch)
		if requirementErr != nil {
			return MergeReadiness{}, requirementErr
		}
		runs := []checkruns.Run{}
		if s.checkRuns != nil {
			runs, requirementErr = s.checkRuns.List(repositoryID, pullRequestID)
		}
		if requirementErr != nil {
			return MergeReadiness{}, requirementErr
		}
		for _, name := range names {
			requirement := CheckRequirement{Name: name, Status: "missing"}
			var stale *checkruns.Run
			for i := len(runs) - 1; i >= 0; i-- {
				run := runs[i]
				if run.Definition.Name != name {
					continue
				}
				if run.CommitID != p.SourceCommitID {
					if stale == nil {
						candidate := run
						stale = &candidate
					}
					continue
				}
				commit, runID := run.CommitID, run.ID
				requirement.CommitID, requirement.RunID = &commit, &runID
				switch run.State {
				case "succeeded":
					requirement.Status = "passed"
				case "failed":
					requirement.Status = "failed"
				case "canceled":
					requirement.Status = "cancelled"
				default:
					requirement.Status = "pending"
				}
				break
			}
			if requirement.Status == "missing" && stale != nil {
				commit, runID := stale.CommitID, stale.ID
				requirement.Status, requirement.CommitID, requirement.RunID = "stale", &commit, &runID
			}
			report.RequiredChecks = append(report.RequiredChecks, requirement)
			if requirement.Status != "passed" {
				addBlocker("required_check_"+requirement.Status, fmt.Sprintf("required check %q is %s for revision %s", name, requirement.Status, p.SourceCommitID))
			}
		}
	}

	if sourceID != nil && targetID != nil && *sourceID == p.SourceCommitID {
		merged, err := commitReachable(repository, *sourceID, *targetID)
		if err != nil {
			return MergeReadiness{}, err
		}
		if merged {
			addBlocker("already_merged", "source commit is already reachable from the target branch")
		} else {
			report.HasConflicts, err = mergeConflicts(repository, *targetID, *sourceID)
			if err != nil {
				return MergeReadiness{}, err
			}
			if report.HasConflicts {
				addBlocker("merge_conflict", "source and target branches have merge conflicts")
			}
		}
	}
	report.Mergeable = len(report.Blockers) == 0
	report.CanMerge = report.Mergeable && actorCanMerge
	return report, nil
}

// Merge revalidates readiness while holding the cross-process mutation lock,
// writes an attributable two-parent commit, advances the target with compare-
// and-swap semantics, and closes the pull request.
func (s *Store) Merge(repositoryID, pullRequestID, mergerID string) (PullRequest, error) {
	if !validID(mergerID) {
		return PullRequest{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return PullRequest{}, err
	}
	defer unlock()

	p, err := s.Get(repositoryID, pullRequestID)
	if err != nil {
		return PullRequest{}, err
	}
	// A merge retry is idempotent. This also lets the application complete a
	// linked-proposal close after an earlier cross-store failure.
	if p.Status == Merged {
		return p, nil
	}
	repository, err := s.git.Open(repositoryID)
	if err != nil {
		return PullRequest{}, err
	}
	if reconciled, found, reconcileErr := s.reconcileMerged(repository, p); reconcileErr != nil {
		return PullRequest{}, reconcileErr
	} else if found {
		return reconciled, nil
	}
	if s.requirements != nil {
		unlockRequirements, lockErr := s.requirements.LockRequiredChecks()
		if lockErr != nil {
			return PullRequest{}, lockErr
		}
		defer unlockRequirements()
	}
	report, err := s.Readiness(repositoryID, pullRequestID, true)
	if err != nil {
		return PullRequest{}, err
	}
	if !report.CanMerge || report.Source.CurrentCommitID == nil || report.Target.CurrentCommitID == nil {
		return PullRequest{}, ErrNotReady
	}
	tree, err := mergeTree(repository, *report.Target.CurrentCommitID, *report.Source.CurrentCommitID)
	if err != nil {
		return PullRequest{}, err
	}
	now := s.now().Truncate(time.Microsecond)
	stamp := fmt.Sprintf("%d +0000", now.Unix())
	message := p.Title + "\n"
	if p.Body != "" {
		message += "\n" + p.Body + "\n"
	}
	message += fmt.Sprintf("\nPull-Request: %s\nAuthored-by: %s\nMerged-by: %s\n", p.ID, p.AuthorID, mergerID)
	if p.ProposalID != nil {
		message += "Proposal: " + *p.ProposalID + "\n"
	}
	content := fmt.Sprintf("tree %s\nparent %s\nparent %s\nauthor Vivarium Author <%s@users.vivarium> %s\ncommitter Vivarium Maintainer <%s@users.vivarium> %s\n\n%s", tree, *report.Target.CurrentCommitID, *report.Source.CurrentCommitID, p.AuthorID, stamp, mergerID, stamp, message)
	commit, err := repository.WriteObject(storage.CommitObject, []byte(content))
	if err != nil {
		return PullRequest{}, err
	}
	p.mergeIntent = &mergeIntent{CommitID: string(commit), MergerID: mergerID, MergedAt: now}
	intentUncertain := false
	if committed, intentErr := s.write(p); intentErr != nil {
		if !committed {
			return PullRequest{}, intentErr
		}
		intentUncertain = true
	}
	if err := repository.UpdateReferenceIfTarget(storage.Reference{Name: "refs/heads/" + p.TargetBranch, Target: string(commit)}, *report.Target.CurrentCommitID); err != nil {
		p.mergeIntent = nil
		_, _ = s.write(p)
		return PullRequest{}, ErrNotReady
	}
	mergedBy, commitID := mergerID, string(commit)
	p.Status, p.UpdatedAt, p.MergedAt, p.MergedBy, p.MergeCommitID, p.mergeIntent = Merged, now, &now, &mergedBy, &commitID, nil
	if committed, err := s.write(p); err != nil {
		if committed {
			return p, fmt.Errorf("%w: %v", ErrDurabilityUncertain, err)
		}
		return PullRequest{}, err
	}
	if intentUncertain {
		return p, ErrDurabilityUncertain
	}
	return p, nil
}

// reconcileMerged repairs metadata when the target publication succeeded but
// a later metadata write and compensating reference update both failed. The
// private durable intent identifies the exact server-generated commit even
// after later target commits have descended from it. Git metadata alone is
// never authorization provenance because contributors can forge it.
func (s *Store) reconcileMerged(repository *storage.Repository, p PullRequest) (PullRequest, bool, error) {
	if p.mergeIntent == nil || !validCommitID(p.mergeIntent.CommitID) || !validID(p.mergeIntent.MergerID) || p.mergeIntent.MergedAt.IsZero() {
		return PullRequest{}, false, nil
	}
	target, err := branchCommit(repository, p.TargetBranch)
	if errors.Is(err, ErrBranchNotFound) {
		return PullRequest{}, false, nil
	}
	if err != nil {
		return PullRequest{}, false, err
	}
	ancestry, err := repository.ListCommitAncestry(storage.ObjectID(target))
	if err != nil {
		return PullRequest{}, false, err
	}
	for _, commit := range ancestry {
		if string(commit.ID) != p.mergeIntent.CommitID {
			continue
		}
		merger, mergedAt := p.mergeIntent.MergerID, p.mergeIntent.MergedAt
		commitID := string(commit.ID)
		p.Status, p.UpdatedAt, p.MergedAt, p.MergedBy, p.MergeCommitID, p.mergeIntent = Merged, mergedAt, &mergedAt, &merger, &commitID, nil
		if committed, writeErr := s.write(p); writeErr != nil {
			if committed {
				return p, true, fmt.Errorf("%w: %v", ErrDurabilityUncertain, writeErr)
			}
			return PullRequest{}, false, writeErr
		}
		return p, true, nil
	}
	// The intent never reached the target (for example, its CAS lost). Remove
	// it before evaluating a fresh merge attempt.
	p.mergeIntent = nil
	if committed, writeErr := s.write(p); writeErr != nil && !committed {
		return PullRequest{}, false, writeErr
	}
	return PullRequest{}, false, nil
}

func mergeTree(repository *storage.Repository, target, source string) (storage.ObjectID, error) {
	temporary, err := os.MkdirTemp("", "vivarium-merge-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(temporary)
	objects := filepath.Join(temporary, "objects")
	if err := os.Mkdir(objects, 0o700); err != nil {
		return "", err
	}
	env := append(os.Environ(), "GIT_OBJECT_DIRECTORY="+objects, "GIT_ALTERNATE_OBJECT_DIRECTORIES="+filepath.Join(repository.Path(), "objects"))
	command := exec.Command("git", "-C", repository.Path(), "merge-tree", "--write-tree", target, source)
	command.Env = env
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("calculate merge tree: %w: %s", err, strings.TrimSpace(string(output)))
	}
	id := storage.ObjectID(strings.TrimSpace(string(output)))
	seen := map[storage.ObjectID]bool{}
	var importTree func(storage.ObjectID) error
	importTree = func(tree storage.ObjectID) error {
		if seen[tree] {
			return nil
		}
		seen[tree] = true
		cat := exec.Command("git", "-C", repository.Path(), "cat-file", "tree", string(tree))
		cat.Env = env
		content, err := cat.Output()
		if err != nil {
			return err
		}
		written, err := repository.WriteObject(storage.TreeObject, content)
		if err != nil || written != tree {
			return fmt.Errorf("import merge tree %s: %v", tree, err)
		}
		list := exec.Command("git", "-C", repository.Path(), "ls-tree", "-z", string(tree))
		list.Env = env
		listed, err := list.Output()
		if err != nil {
			return err
		}
		for _, record := range strings.Split(string(listed), "\x00") {
			fields := strings.Fields(record)
			if len(fields) >= 3 && fields[1] == "tree" {
				if err := importTree(storage.ObjectID(fields[2])); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := importTree(id); err != nil {
		return "", err
	}
	return id, nil
}

func liveBranchState(repository *storage.Repository, branch, snapshot string) (*string, string, error) {
	current, err := branchCommit(repository, branch)
	if errors.Is(err, ErrBranchNotFound) {
		return nil, "missing", nil
	}
	if err != nil {
		return nil, "", err
	}
	state := "current"
	if current != snapshot {
		advanced, err := commitReachable(repository, snapshot, current)
		if err != nil {
			return nil, "", err
		}
		if advanced {
			state = "advanced"
		} else {
			state = "rewritten"
		}
	}
	return &current, state, nil
}

func commitReachable(repository *storage.Repository, ancestor, descendant string) (bool, error) {
	commits, err := repository.ListCommitAncestry(storage.ObjectID(descendant))
	if err != nil {
		return false, err
	}
	for _, commit := range commits {
		if string(commit.ID) == ancestor {
			return true, nil
		}
	}
	return false, nil
}

// mergeConflicts asks stock Git to perform its merge calculation while
// redirecting every object write into a disposable object directory. The bare
// repository remains byte-for-byte read-only.
func mergeConflicts(repository *storage.Repository, target, source string) (bool, error) {
	temporary, err := os.MkdirTemp("", "vivarium-merge-readiness-")
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(temporary)
	objects := filepath.Join(temporary, "objects")
	if err := os.Mkdir(objects, 0o700); err != nil {
		return false, err
	}
	command := exec.Command("git", "-C", repository.Path(), "merge-tree", "--write-tree", target, source)
	command.Env = append(os.Environ(), "GIT_OBJECT_DIRECTORY="+objects, "GIT_ALTERNATE_OBJECT_DIRECTORIES="+filepath.Join(repository.Path(), "objects"))
	if output, err := command.CombinedOutput(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return true, nil
		}
		return false, fmt.Errorf("calculate merge: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return false, nil
}

func (s *Store) commentsPath(repositoryID, pullRequestID string) string {
	return filepath.Join(s.repositoryPath(repositoryID), pullRequestID+".comments.json")
}

func (s *Store) reviewsPath(repositoryID, pullRequestID string) string {
	return filepath.Join(s.repositoryPath(repositoryID), pullRequestID+".reviews.json")
}

func (s *Store) readReviews(repositoryID, pullRequestID string) (reviewRecord, error) {
	data, err := os.ReadFile(s.reviewsPath(repositoryID, pullRequestID))
	if errors.Is(err, os.ErrNotExist) {
		return reviewRecord{Reviews: []Review{}}, nil
	}
	if err != nil {
		return reviewRecord{}, err
	}
	var record reviewRecord
	if json.Unmarshal(data, &record) != nil {
		return reviewRecord{}, fmt.Errorf("corrupt pull request reviews %s", pullRequestID)
	}
	seenIDs, seenReviewers := map[string]bool{}, map[string]bool{}
	for _, review := range record.Reviews {
		validDecision := review.Decision == Approved || review.Decision == ChangesRequested || review.Decision == Withdrawn
		if !validID(review.ID) || review.PullRequestID != pullRequestID || !validID(review.ReviewerID) || !validDecision || !validCommitID(review.ReviewedCommitID) || review.CreatedAt.IsZero() || review.UpdatedAt.IsZero() || review.UpdatedAt.Before(review.CreatedAt) || seenIDs[review.ID] || seenReviewers[review.ReviewerID] {
			return reviewRecord{}, fmt.Errorf("corrupt pull request reviews %s", pullRequestID)
		}
		seenIDs[review.ID], seenReviewers[review.ReviewerID] = true, true
	}
	sort.Slice(record.Reviews, func(i, j int) bool {
		if record.Reviews[i].CreatedAt.Equal(record.Reviews[j].CreatedAt) {
			return record.Reviews[i].ID < record.Reviews[j].ID
		}
		return record.Reviews[i].CreatedAt.Before(record.Reviews[j].CreatedAt)
	})
	return record, nil
}

func (s *Store) writeReviews(repositoryID, pullRequestID string, record reviewRecord) (bool, error) {
	data, err := json.Marshal(record)
	if err != nil {
		return false, err
	}
	directory := s.repositoryPath(repositoryID)
	temp, err := os.CreateTemp(directory, ".writing-reviews-")
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
	if err := os.Rename(path, s.reviewsPath(repositoryID, pullRequestID)); err != nil {
		return false, err
	}
	return true, s.directorySync(directory)
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
	var record pullRequestRecord
	if json.Unmarshal(data, &record) != nil {
		return PullRequest{}, fmt.Errorf("corrupt pull request %s", id)
	}
	p := record.PullRequest
	p.mergeIntent = record.MergeIntent
	validOutcome := (p.Status == Open && p.MergedAt == nil && p.MergedBy == nil && p.MergeCommitID == nil) || (p.Status == Merged && p.MergedAt != nil && p.MergedBy != nil && validID(*p.MergedBy) && p.MergeCommitID != nil && validCommitID(*p.MergeCommitID))
	validIntent := p.mergeIntent == nil || (p.Status == Open && validCommitID(p.mergeIntent.CommitID) && validID(p.mergeIntent.MergerID) && !p.mergeIntent.MergedAt.IsZero())
	if p.ID != id || !validID(p.RepositoryID) || !validID(p.AuthorID) || !validOutcome || !validIntent || !validCommitID(p.SourceCommitID) || !validCommitID(p.TargetCommitID) || p.SourceBranch == p.TargetBranch || p.SourceBranch == "" || p.TargetBranch == "" || strings.HasPrefix(p.SourceBranch, "refs/") || strings.HasPrefix(p.TargetBranch, "refs/") || p.CreatedAt.IsZero() || p.UpdatedAt.IsZero() || (p.ProposalID != nil && !validID(*p.ProposalID)) {
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
	data, err := json.Marshal(pullRequestRecord{PullRequest: p, MergeIntent: p.mergeIntent})
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
