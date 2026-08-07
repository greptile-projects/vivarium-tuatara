// Package proposals stores repository-scoped ideas and their attributable discussion.
package proposals

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
)

var (
	ErrNotFound = errors.New("proposal not found")
	ErrInvalid  = errors.New("invalid proposal")
)

const (
	Open   = "open"
	Closed = "closed"
)

type Proposal struct {
	ID           string     `json:"id"`
	RepositoryID string     `json:"repository_id"`
	AuthorID     string     `json:"author_id"`
	Title        string     `json:"title"`
	Body         string     `json:"body"`
	Status       string     `json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	ClosedAt     *time.Time `json:"closed_at"`
}

type Comment struct {
	ID         string    `json:"id"`
	ProposalID string    `json:"proposal_id"`
	AuthorID   string    `json:"author_id"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
}

type Patch struct {
	Title  *string
	Body   *string
	Status *string
}

type record struct {
	Proposal Proposal  `json:"proposal"`
	Comments []Comment `json:"comments,omitempty"`
}

type Store struct {
	root          string
	mu            sync.Mutex
	now           func() time.Time
	directorySync func(string) error
	readFile      func(string) ([]byte, error)
}

func New(root string) (*Store, error) {
	if root == "" {
		return nil, errors.New("proposal storage root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, fmt.Errorf("create proposal store: %w", err)
	}
	return &Store{root: abs, now: func() time.Time { return time.Now().UTC() }, directorySync: syncDirectory, readFile: os.ReadFile}, nil
}

func (s *Store) Create(repositoryID, authorID, title, body string) (Proposal, error) {
	if !validID(repositoryID) || !validID(authorID) {
		return Proposal{}, ErrInvalid
	}
	title, body, err := validateContent(title, body)
	if err != nil {
		return Proposal{}, err
	}
	id, err := newID()
	if err != nil {
		return Proposal{}, err
	}
	now := s.now().Truncate(time.Microsecond)
	p := Proposal{ID: id, RepositoryID: repositoryID, AuthorID: authorID, Title: title, Body: body, Status: Open, CreatedAt: now, UpdatedAt: now}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Proposal{}, err
	}
	defer unlock()
	desired := record{Proposal: p}
	if committed, err := s.write(desired); err != nil {
		if committed {
			return p, nil
		}
		return Proposal{}, err
	}
	return p, nil
}

func (s *Store) Get(repositoryID, id string) (Proposal, error) {
	r, err := s.read(id)
	if err != nil || r.Proposal.RepositoryID != repositoryID {
		return Proposal{}, ErrNotFound
	}
	return r.Proposal, nil
}

func (s *Store) List(repositoryID string) ([]Proposal, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	result := []Proposal{}
	for _, entry := range entries {
		id, ok := strings.CutSuffix(entry.Name(), ".json")
		if entry.IsDir() || !ok || !validID(id) {
			continue
		}
		r, err := s.read(id)
		if err != nil {
			return nil, err
		}
		if r.Proposal.RepositoryID == repositoryID {
			result = append(result, r.Proposal)
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

func (s *Store) Update(repositoryID, id string, patch Patch) (Proposal, error) {
	if patch.Title == nil && patch.Body == nil && patch.Status == nil {
		return Proposal{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Proposal{}, err
	}
	defer unlock()
	r, err := s.read(id)
	if err != nil || r.Proposal.RepositoryID != repositoryID {
		return Proposal{}, ErrNotFound
	}
	p := r.Proposal
	if patch.Title != nil {
		title, _, e := validateContent(*patch.Title, p.Body)
		if e != nil {
			return Proposal{}, e
		}
		p.Title = title
	}
	if patch.Body != nil {
		_, body, e := validateContent(p.Title, *patch.Body)
		if e != nil {
			return Proposal{}, e
		}
		p.Body = body
	}
	if patch.Status != nil {
		if *patch.Status != Closed || p.Status == Closed {
			return Proposal{}, ErrInvalid
		}
		closed := s.now().Truncate(time.Microsecond)
		p.Status, p.ClosedAt = Closed, &closed
	}
	p.UpdatedAt = s.now().Truncate(time.Microsecond)
	r.Proposal = p
	if committed, err := s.write(r); err != nil {
		if committed {
			return p, nil
		}
		return Proposal{}, err
	}
	return p, nil
}

func (s *Store) AddComment(repositoryID, proposalID, authorID, body string) (Comment, error) {
	if !validID(repositoryID) || !validID(authorID) {
		return Comment{}, ErrInvalid
	}
	body = strings.TrimSpace(body)
	if body == "" || len([]rune(body)) > 10000 {
		return Comment{}, ErrInvalid
	}
	id, err := newID()
	if err != nil {
		return Comment{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Comment{}, err
	}
	defer unlock()
	r, err := s.read(proposalID)
	if err != nil || r.Proposal.RepositoryID != repositoryID {
		return Comment{}, ErrNotFound
	}
	c := Comment{ID: id, ProposalID: proposalID, AuthorID: authorID, Body: body, CreatedAt: s.now().Truncate(time.Microsecond)}
	r.Comments = append(r.Comments, c)
	if committed, err := s.write(r); err != nil {
		if committed {
			return c, nil
		}
		return Comment{}, err
	}
	return c, nil
}

func (s *Store) ListComments(repositoryID, proposalID string) ([]Comment, error) {
	r, err := s.read(proposalID)
	if err != nil || r.Proposal.RepositoryID != repositoryID {
		return nil, ErrNotFound
	}
	return append([]Comment(nil), r.Comments...), nil
}

func validateContent(title, body string) (string, string, error) {
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

func (s *Store) read(id string) (record, error) {
	if !validID(id) {
		return record{}, ErrNotFound
	}
	data, err := s.readFile(s.path(id))
	if errors.Is(err, os.ErrNotExist) {
		return record{}, ErrNotFound
	}
	if err != nil {
		return record{}, err
	}
	var r record
	if json.Unmarshal(data, &r) != nil || r.Proposal.ID != id || !validID(r.Proposal.RepositoryID) || !validID(r.Proposal.AuthorID) || (r.Proposal.Status != Open && r.Proposal.Status != Closed) || (r.Proposal.Status == Open && r.Proposal.ClosedAt != nil) || (r.Proposal.Status == Closed && r.Proposal.ClosedAt == nil) {
		return record{}, fmt.Errorf("corrupt proposal %s", id)
	}
	if _, _, err := validateContent(r.Proposal.Title, r.Proposal.Body); err != nil {
		return record{}, fmt.Errorf("corrupt proposal %s", id)
	}
	seen := map[string]bool{}
	for _, c := range r.Comments {
		if !validID(c.ID) || c.ProposalID != id || !validID(c.AuthorID) || strings.TrimSpace(c.Body) == "" || len([]rune(c.Body)) > 10000 || seen[c.ID] {
			return record{}, fmt.Errorf("corrupt proposal %s", id)
		}
		seen[c.ID] = true
	}
	return r, nil
}

// write reports whether the atomic rename made the requested state visible.
// Once committed, callers must preserve the resource result even if syncing
// the parent directory cannot confirm crash durability; reporting an ordinary
// failure would discard generated IDs and make client retries duplicate work.
func (s *Store) write(r record) (bool, error) {
	data, err := json.Marshal(r)
	if err != nil {
		return false, err
	}
	temp, err := os.CreateTemp(s.root, ".writing-")
	if err != nil {
		return false, err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
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
	if err := os.Rename(tempPath, s.path(r.Proposal.ID)); err != nil {
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
		f.Close()
		return nil, err
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}
