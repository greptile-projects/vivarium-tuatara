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
	"syscall"
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
	Stale      bool   `json:"stale,omitempty"`
}

type Participant struct {
	UserID    string    `json:"user_id"`
	InvitedBy string    `json:"invited_by,omitempty"`
	JoinedAt  time.Time `json:"joined_at"`
}

// Entry is one ordered, attributable step on the shared investigation canvas.
// Resource attachments contain identifiers and bounded observations, never
// credentials or private workspace output copied behind its access boundary.
type Entry struct {
	ID           string     `json:"id"`
	Kind         string     `json:"kind"`
	Body         string     `json:"body"`
	ActorID      string     `json:"actor_id"`
	Revision     string     `json:"revision"`
	Citations    []Citation `json:"citations,omitempty"`
	ResourceID   string     `json:"resource_id,omitempty"`
	SupersedesID string     `json:"supersedes_id,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

type Claim struct {
	ID         string     `json:"id"`
	Text       string     `json:"text"`
	Basis      string     `json:"basis"` // evidence, inference, or uncertainty
	Confidence string     `json:"confidence"`
	Citations  []Citation `json:"citations"`
}

type Conversation struct {
	ID             string        `json:"id"`
	RepositoryID   string        `json:"repository_id"`
	Revision       string        `json:"revision"`
	Context        Context       `json:"context"`
	Question       string        `json:"question"`
	AskedBy        string        `json:"asked_by"`
	Agent          string        `json:"agent"`
	Answer         string        `json:"answer"`
	Claims         []Claim       `json:"claims"`
	Participants   []Participant `json:"participants,omitempty"`
	Entries        []Entry       `json:"entries,omitempty"`
	AnalysisStatus string        `json:"analysis_status"`
	AnalysisReason string        `json:"analysis_reason,omitempty"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at,omitempty"`
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
	unlock, err := s.lockRoot()
	if err != nil {
		return Conversation{}, err
	}
	defer unlock()
	value.Question = strings.TrimSpace(value.Question)
	if value.RepositoryID == "" || len(value.Revision) != 40 || value.Question == "" || len(value.Question) > 2000 || value.AskedBy == "" || value.Context.Kind == "" || len(value.Claims) == 0 {
		return Conversation{}, ErrInvalid
	}
	id, err := randomID()
	if err != nil {
		return Conversation{}, err
	}
	value.ID, value.CreatedAt = id, s.now()
	value.UpdatedAt = value.CreatedAt
	value.Participants = []Participant{{UserID: value.AskedBy, JoinedAt: value.CreatedAt}}
	if value.Agent == "" {
		value.Agent = "vivarium-evidence-agent-v1"
	}
	for i := range value.Claims {
		value.Claims[i].ID = id + "-" + string(rune('a'+i))
		value.Entries = append(value.Entries, Entry{ID: value.Claims[i].ID, Kind: "agent_finding", Body: value.Claims[i].Text, ActorID: value.Agent, Revision: value.Revision, Citations: value.Claims[i].Citations, CreatedAt: value.CreatedAt})
	}
	err = s.write(value)
	return value, err
}

func (s *Store) Update(id string, mutate func(*Conversation) error) (Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return Conversation{}, err
	}
	defer unlock()
	value, err := s.read(id)
	if err != nil {
		return Conversation{}, err
	}
	if err = mutate(&value); err != nil {
		return Conversation{}, err
	}
	value.UpdatedAt = s.now()
	if err = s.write(value); err != nil {
		return Conversation{}, err
	}
	return value, nil
}

func (s *Store) AddParticipant(id, repositoryID, actor, userID string) (Conversation, error) {
	return s.Update(id, func(v *Conversation) error {
		if v.RepositoryID != repositoryID || !participant(v.Participants, actor) || userID == "" {
			return ErrNotFound
		}
		if participant(v.Participants, userID) {
			return nil
		}
		v.Participants = append(v.Participants, Participant{UserID: userID, InvitedBy: actor, JoinedAt: s.now()})
		return nil
	})
}

func (s *Store) AddEntry(id string, entry Entry) (Conversation, error) {
	return s.Update(id, func(v *Conversation) error {
		entry.Body = strings.TrimSpace(entry.Body)
		validKind := map[string]bool{"code_reference": true, "query": true, "runtime_observation": true, "hypothesis": true, "agent_finding": true, "conclusion": true, "challenge": true}
		if !participant(v.Participants, entry.ActorID) || !validKind[entry.Kind] || entry.Body == "" || len(entry.Body) > 4000 {
			return ErrInvalid
		}
		if entry.SupersedesID != "" {
			found := false
			for _, x := range v.Entries {
				if x.ID == entry.SupersedesID {
					found = true
				}
			}
			if !found {
				return ErrInvalid
			}
		}
		var err error
		entry.ID, err = randomID()
		if err != nil {
			return err
		}
		entry.Revision = v.Revision
		entry.CreatedAt = s.now()
		v.Entries = append(v.Entries, entry)
		return nil
	})
}

func (s *Store) Rerun(id, actor, revision, answer, status, reason string, claims []Claim) (Conversation, error) {
	return s.Update(id, func(v *Conversation) error {
		if !participant(v.Participants, actor) || len(revision) != 40 || len(claims) == 0 {
			return ErrInvalid
		}
		v.Revision, v.Answer, v.AnalysisStatus, v.AnalysisReason, v.Claims = revision, answer, status, reason, claims
		for i := range v.Claims {
			var err error
			v.Claims[i].ID, err = randomID()
			if err != nil {
				return err
			}
			v.Entries = append(v.Entries, Entry{ID: v.Claims[i].ID, Kind: "agent_finding", Body: v.Claims[i].Text, ActorID: v.Agent, Revision: revision, Citations: v.Claims[i].Citations, CreatedAt: s.now()})
		}
		return nil
	})
}

func participant(items []Participant, userID string) bool {
	for _, x := range items {
		if x.UserID == userID {
			return true
		}
	}
	return false
}

// migrateLegacy projects the durable historical asker as the sole participant
// for records written before explicit invitations existed. No other repository
// participant inherits access merely because participant data is absent.
func migrateLegacy(value Conversation) Conversation {
	if len(value.Participants) == 0 && value.AskedBy != "" {
		value.Participants = []Participant{{UserID: value.AskedBy, JoinedAt: value.CreatedAt}}
	}
	return value
}

func IsParticipant(value Conversation, userID string) bool {
	return participant(value.Participants, userID)
}

func (s *Store) write(value Conversation) error {
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.root, ".explanation-*")
	if err != nil {
		return err
	}
	name := filepath.Join(s.root, value.ID+".json")
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
		return err
	}
	return nil
}

func (s *Store) read(id string) (Conversation, error) {
	var value Conversation
	body, err := os.ReadFile(filepath.Join(s.root, filepath.Base(id)+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return value, ErrNotFound
	}
	if err != nil || json.Unmarshal(body, &value) != nil || value.ID != id {
		return value, ErrNotFound
	}
	return migrateLegacy(value), nil
}

func (s *Store) Get(id string) (Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return Conversation{}, err
	}
	defer unlock()
	return s.read(id)
}

func (s *Store) List(repositoryID string) ([]Conversation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return nil, err
	}
	defer unlock()
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
			out = append(out, migrateLegacy(value))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) lockRoot() (func(), error) {
	file, err := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		file.Close()
		return nil, err
	}
	return func() { _ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN); _ = file.Close() }, nil
}

func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
