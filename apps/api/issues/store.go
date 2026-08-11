// Package issues persists structured, repository-scoped unexpected-behavior reports.
package issues

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
	"time"
)

var (
	ErrNotFound = errors.New("issue not found")
	ErrInvalid  = errors.New("invalid issue")
	ErrConflict = errors.New("issue changed")
)

type Attachment struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Name      string    `json:"name"`
	MediaType string    `json:"media_type"`
	Size      int       `json:"size"`
	Data      string    `json:"data,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type Comment struct {
	ID        string    `json:"id"`
	AuthorID  string    `json:"author_id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

type HistoryEntry struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	ActorID   string    `json:"actor_id"`
	From      string    `json:"from,omitempty"`
	To        string    `json:"to,omitempty"`
	Message   string    `json:"message,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type Issue struct {
	ID                string         `json:"id"`
	RepositoryID      string         `json:"repository_id"`
	ReleaseID         string         `json:"release_id,omitempty"`
	AffectedVersion   string         `json:"affected_version,omitempty"`
	Title             string         `json:"title"`
	ExpectedBehavior  string         `json:"expected_behavior"`
	ObservedBehavior  string         `json:"observed_behavior"`
	Severity          string         `json:"severity"`
	Environment       string         `json:"environment"`
	ReproductionSteps []string       `json:"reproduction_steps"`
	Visibility        string         `json:"visibility"`
	Status            string         `json:"status"`
	ReporterID        string         `json:"reporter_id"`
	Attachments       []Attachment   `json:"attachments"`
	Discussion        []Comment      `json:"discussion"`
	History           []HistoryEntry `json:"history"`
	Version           int            `json:"version"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

type Store struct {
	root string
	mu   sync.Mutex
}

func New(root string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("issue root required")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	return &Store{root: root}, nil
}

func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func (s *Store) Create(v Issue) (Issue, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v.Title, v.ExpectedBehavior, v.ObservedBehavior, v.Environment = strings.TrimSpace(v.Title), strings.TrimSpace(v.ExpectedBehavior), strings.TrimSpace(v.ObservedBehavior), strings.TrimSpace(v.Environment)
	if v.RepositoryID == "" || v.ReporterID == "" || v.Title == "" || v.ExpectedBehavior == "" || v.ObservedBehavior == "" || v.Environment == "" || !validSeverity(v.Severity) || !validVisibility(v.Visibility) || len(v.ReproductionSteps) == 0 {
		return Issue{}, ErrInvalid
	}
	for i := range v.ReproductionSteps {
		v.ReproductionSteps[i] = strings.TrimSpace(v.ReproductionSteps[i])
		if v.ReproductionSteps[i] == "" {
			return Issue{}, ErrInvalid
		}
	}
	if err := validateAttachments(v.Attachments); err != nil {
		return Issue{}, err
	}
	now := time.Now().UTC()
	v.ID, v.Status, v.Version, v.CreatedAt, v.UpdatedAt = newID(), "open", 1, now, now
	for i := range v.Attachments {
		v.Attachments[i].ID, v.Attachments[i].CreatedAt = newID(), now
	}
	v.History = []HistoryEntry{{ID: newID(), Kind: "opened", ActorID: v.ReporterID, To: "open", CreatedAt: now}}
	if err := s.write(v); err != nil {
		return Issue{}, err
	}
	return v, nil
}

func (s *Store) Get(repositoryID, id string) (Issue, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read(repositoryID, id)
}

func (s *Store) List(repositoryID string) ([]Issue, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	out := []Issue{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, e := os.ReadFile(filepath.Join(s.root, entry.Name()))
		if e != nil {
			return nil, e
		}
		var v Issue
		if json.Unmarshal(data, &v) != nil {
			return nil, errors.New("corrupt issue")
		}
		if v.RepositoryID == repositoryID {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) AddComment(repositoryID, id, actor, body string) (Issue, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.read(repositoryID, id)
	body = strings.TrimSpace(body)
	if err != nil || body == "" {
		if err != nil {
			return Issue{}, err
		}
		return Issue{}, ErrInvalid
	}
	now := time.Now().UTC()
	v.Discussion = append(v.Discussion, Comment{ID: newID(), AuthorID: actor, Body: body, CreatedAt: now})
	v.History = append(v.History, HistoryEntry{ID: newID(), Kind: "commented", ActorID: actor, Message: body, CreatedAt: now})
	v.Version++
	v.UpdatedAt = now
	if err = s.write(v); err != nil {
		return Issue{}, err
	}
	return v, nil
}

func (s *Store) UpdateStatus(repositoryID, id, actor, status string, expected int, message string) (Issue, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.read(repositoryID, id)
	if err != nil {
		return Issue{}, err
	}
	if expected != v.Version {
		return Issue{}, ErrConflict
	}
	if !validStatus(status) {
		return Issue{}, ErrInvalid
	}
	if status == v.Status {
		return Issue{}, ErrInvalid
	}
	now := time.Now().UTC()
	from := v.Status
	v.Status = status
	v.Version++
	v.UpdatedAt = now
	v.History = append(v.History, HistoryEntry{ID: newID(), Kind: "status_changed", ActorID: actor, From: from, To: status, Message: strings.TrimSpace(message), CreatedAt: now})
	if err = s.write(v); err != nil {
		return Issue{}, err
	}
	return v, nil
}

func (s *Store) read(repo, id string) (Issue, error) {
	data, err := os.ReadFile(filepath.Join(s.root, id+".json"))
	if os.IsNotExist(err) {
		return Issue{}, ErrNotFound
	}
	if err != nil {
		return Issue{}, err
	}
	var v Issue
	if json.Unmarshal(data, &v) != nil || v.ID != id || v.RepositoryID != repo {
		return Issue{}, ErrNotFound
	}
	return v, nil
}
func (s *Store) write(v Issue) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(s.root, ".issue-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0o600); err == nil {
		_, err = f.Write(data)
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(name, filepath.Join(s.root, v.ID+".json"))
	}
	return err
}
func validSeverity(v string) bool {
	return v == "low" || v == "medium" || v == "high" || v == "critical"
}
func validVisibility(v string) bool { return v == "public" || v == "repository" }
func validStatus(v string) bool {
	return v == "open" || v == "triaged" || v == "in_progress" || v == "resolved" || v == "closed"
}
func validateAttachments(items []Attachment) error {
	if len(items) > 10 {
		return ErrInvalid
	}
	for i := range items {
		a := &items[i]
		a.Name = strings.TrimSpace(filepath.Base(a.Name))
		if a.Name == "" || a.Size < 0 || a.Size > 1<<20 || len(a.Data) > 1400000 {
			return ErrInvalid
		}
		permitted := a.Kind == "log" && a.MediaType == "text/plain" ||
			a.Kind == "trace" && (a.MediaType == "application/json" || a.MediaType == "application/octet-stream") ||
			a.Kind == "sample" && (a.MediaType == "application/json" || a.MediaType == "text/plain" || a.MediaType == "application/octet-stream") ||
			a.Kind == "screenshot" && (a.MediaType == "image/png" || a.MediaType == "image/jpeg" || a.MediaType == "image/webp")
		if !permitted {
			return ErrInvalid
		}
	}
	return nil
}
