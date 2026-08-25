// Package releases stores immutable repository release candidates.
package releases

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	ErrNotFound      = errors.New("release candidate not found")
	ErrInvalid       = errors.New("invalid release candidate")
	ErrVersionExists = errors.New("release version already exists")
)

type Inclusion struct {
	PullRequestIDs []string       `json:"pull_request_ids"`
	PullEvidence   []PullEvidence `json:"pull_evidence,omitempty"`
	ProposalIDs    []string       `json:"proposal_ids"`
	TaskIDs        []string       `json:"task_ids"`
	ContributorIDs []string       `json:"contributor_ids"`
}

type PullEvidence struct {
	PullRequestID  string   `json:"pull_request_id"`
	SourceCommitID string   `json:"source_commit_id"`
	ChangedPaths   []string `json:"changed_paths"`
}

type Candidate struct {
	ID                string    `json:"id"`
	RepositoryID      string    `json:"repository_id"`
	Version           string    `json:"version"`
	Notes             string    `json:"notes"`
	CommitID          string    `json:"commit_id"`
	TargetBranch      string    `json:"target_branch"`
	ChangedPaths      []string  `json:"changed_paths"`
	PreviousReleaseID *string   `json:"previous_release_id,omitempty"`
	PreviousCommitID  *string   `json:"previous_commit_id,omitempty"`
	Status            string    `json:"status"`
	CreatedBy         string    `json:"created_by"`
	CreatedAt         time.Time `json:"created_at"`
	Inclusions        Inclusion `json:"inclusions"`
}

type Store struct {
	root            string
	mu              sync.Mutex
	now             func() time.Time
	provenanceReady func(Candidate) (bool, error)
}

func (s *Store) ConfigureProvenanceReadiness(fn func(Candidate) (bool, error)) {
	s.provenanceReady = fn
}
func (s *Store) ProvenanceReady(candidate Candidate) (bool, error) {
	if s.provenanceReady == nil {
		return true, nil
	}
	return s.provenanceReady(candidate)
}

func New(root string) (*Store, error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	return &Store{root: root, now: time.Now}, nil
}

func (s *Store) Create(candidate Candidate) (Candidate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return Candidate{}, err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return Candidate{}, err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	if !validID(candidate.RepositoryID) || !validID(candidate.CreatedBy) || !validCommit(candidate.CommitID) || strings.TrimSpace(candidate.Version) == "" || len(candidate.Version) > 100 || strings.TrimSpace(candidate.Notes) == "" || len(candidate.Notes) > 10000 {
		return Candidate{}, ErrInvalid
	}
	if len(candidate.Inclusions.PullEvidence) > 0 {
		included := map[string]bool{}
		for _, pullID := range candidate.Inclusions.PullRequestIDs {
			included[pullID] = true
		}
		seen := map[string]bool{}
		for _, evidence := range candidate.Inclusions.PullEvidence {
			if !included[evidence.PullRequestID] || seen[evidence.PullRequestID] || !validCommit(evidence.SourceCommitID) {
				return Candidate{}, ErrInvalid
			}
			seen[evidence.PullRequestID] = true
			for _, changedPath := range evidence.ChangedPaths {
				if changedPath == "" || strings.HasPrefix(changedPath, "/") || filepath.Clean(changedPath) != changedPath || strings.HasPrefix(changedPath, "../") {
					return Candidate{}, ErrInvalid
				}
			}
		}
		if len(seen) != len(included) {
			return Candidate{}, ErrInvalid
		}
	}
	items, err := s.list(candidate.RepositoryID)
	if err != nil {
		return Candidate{}, err
	}
	for _, item := range items {
		if strings.EqualFold(item.Version, candidate.Version) {
			return Candidate{}, ErrVersionExists
		}
	}
	id, err := randomID()
	if err != nil {
		return Candidate{}, err
	}
	candidate.ID, candidate.Version, candidate.Notes, candidate.Status = id, strings.TrimSpace(candidate.Version), strings.TrimSpace(candidate.Notes), "candidate"
	candidate.CreatedAt = s.now().UTC().Truncate(time.Microsecond)
	for _, values := range [][]string{candidate.Inclusions.PullRequestIDs, candidate.Inclusions.ProposalIDs, candidate.Inclusions.TaskIDs, candidate.Inclusions.ContributorIDs} {
		sort.Strings(values)
	}
	for index := range candidate.Inclusions.PullEvidence {
		sort.Strings(candidate.Inclusions.PullEvidence[index].ChangedPaths)
	}
	sort.Slice(candidate.Inclusions.PullEvidence, func(i, j int) bool {
		return candidate.Inclusions.PullEvidence[i].PullRequestID < candidate.Inclusions.PullEvidence[j].PullRequestID
	})
	sort.Strings(candidate.ChangedPaths)
	dir := filepath.Join(s.root, candidate.RepositoryID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return Candidate{}, err
	}
	data, err := json.Marshal(candidate)
	if err != nil {
		return Candidate{}, err
	}
	tmp, err := os.CreateTemp(dir, ".release-*")
	if err != nil {
		return Candidate{}, err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(data)
	}
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return Candidate{}, err
	}
	if err := os.Rename(name, filepath.Join(dir, id+".json")); err != nil {
		return Candidate{}, err
	}
	d, err := os.Open(dir)
	if err != nil {
		return Candidate{}, err
	}
	err = d.Sync()
	closeErr = d.Close()
	if err == nil {
		err = closeErr
	}
	return candidate, err
}

func (s *Store) Get(repositoryID, id string) (Candidate, error) {
	if !validID(repositoryID) || !validID(id) {
		return Candidate{}, ErrNotFound
	}
	data, err := os.ReadFile(filepath.Join(s.root, repositoryID, id+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return Candidate{}, ErrNotFound
	}
	if err != nil {
		return Candidate{}, err
	}
	var result Candidate
	if json.Unmarshal(data, &result) != nil || result.ID != id || result.RepositoryID != repositoryID {
		return Candidate{}, ErrNotFound
	}
	return result, nil
}

func (s *Store) List(repositoryID string) ([]Candidate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.list(repositoryID)
}
func (s *Store) list(repositoryID string) ([]Candidate, error) {
	if !validID(repositoryID) {
		return nil, ErrNotFound
	}
	entries, err := os.ReadDir(filepath.Join(s.root, repositoryID))
	if errors.Is(err, os.ErrNotExist) {
		return []Candidate{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := []Candidate{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		item, err := s.Get(repositoryID, strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.Before(result[j].CreatedAt) })
	return result, nil
}

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
func validID(v string) bool {
	if len(v) != 32 {
		return false
	}
	_, err := hex.DecodeString(v)
	return err == nil && v == strings.ToLower(v)
}
func validCommit(v string) bool {
	if len(v) != 40 {
		return false
	}
	_, err := hex.DecodeString(v)
	return err == nil && v == strings.ToLower(v)
}
