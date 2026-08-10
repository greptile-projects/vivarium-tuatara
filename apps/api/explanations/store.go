// Package explanations persists revision-pinned, attributable code explanations.
package explanations

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
	ErrNotFound = errors.New("explanation not found")
	ErrInvalid  = errors.New("invalid explanation")
)

type Context struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id,omitempty"`
	Path       string `json:"path,omitempty"`
}

type Citation struct {
	Kind       string `json:"kind"`
	Revision   string `json:"revision"`
	Path       string `json:"path,omitempty"`
	StartLine  int    `json:"start_line,omitempty"`
	EndLine    int    `json:"end_line,omitempty"`
	CommitID   string `json:"commit_id,omitempty"`
	ResourceID string `json:"resource_id,omitempty"`
	Label      string `json:"label"`
}

type Claim struct {
	ID         string     `json:"id"`
	Text       string     `json:"text"`
	Basis      string     `json:"basis"` // evidence, inference, or uncertainty
	Confidence string     `json:"confidence"`
	Citations  []Citation `json:"citations"`
}

type Conversation struct {
	ID             string    `json:"id"`
	RepositoryID   string    `json:"repository_id"`
	Revision       string    `json:"revision"`
	Context        Context   `json:"context"`
	Question       string    `json:"question"`
	AskedBy        string    `json:"asked_by"`
	Agent          string    `json:"agent"`
	Answer         string    `json:"answer"`
	Claims         []Claim   `json:"claims"`
	AnalysisStatus string    `json:"analysis_status"`
	AnalysisReason string    `json:"analysis_reason,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

type Store struct {
	root string
	mu   sync.Mutex
	now  func() time.Time
}

func New(root string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, ErrInvalid
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	return &Store{root: root, now: func() time.Time { return time.Now().UTC() }}, nil
}

func (s *Store) Create(value Conversation) (Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value.Question = strings.TrimSpace(value.Question)
	if value.RepositoryID == "" || len(value.Revision) != 40 || value.Question == "" || len(value.Question) > 2000 || value.AskedBy == "" || value.Context.Kind == "" || len(value.Claims) == 0 {
		return Conversation{}, ErrInvalid
	}
	id, err := randomID()
	if err != nil {
		return Conversation{}, err
	}
	value.ID, value.CreatedAt = id, s.now()
	if value.Agent == "" {
		value.Agent = "vivarium-evidence-agent-v1"
	}
	for i := range value.Claims {
		value.Claims[i].ID = id + "-" + string(rune('a'+i))
	}
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return Conversation{}, err
	}
	tmp, err := os.CreateTemp(s.root, ".explanation-*")
	if err != nil {
		return Conversation{}, err
	}
	name := filepath.Join(s.root, id+".json")
	defer os.Remove(tmp.Name())
	if err = tmp.Chmod(0o600); err == nil {
		_, err = tmp.Write(body)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), name)
	}
	if err != nil {
		return Conversation{}, err
	}
	return value, nil
}

func (s *Store) Get(id string) (Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var value Conversation
	body, err := os.ReadFile(filepath.Join(s.root, filepath.Base(id)+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return value, ErrNotFound
	}
	if err != nil || json.Unmarshal(body, &value) != nil || value.ID != id {
		return value, ErrNotFound
	}
	return value, nil
}

func (s *Store) List(repositoryID string) ([]Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	out := []Conversation{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		var value Conversation
		body, readErr := os.ReadFile(filepath.Join(s.root, entry.Name()))
		if readErr == nil && json.Unmarshal(body, &value) == nil && value.RepositoryID == repositoryID {
			out = append(out, value)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
