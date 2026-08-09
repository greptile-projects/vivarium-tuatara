// Package securityadvisories persists private vulnerability coordination records.
package securityadvisories

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
	ErrNotFound = errors.New("security advisory not found")
	ErrInvalid  = errors.New("invalid security advisory")
	ErrConflict = errors.New("security advisory changed")
)

type AffectedRepository struct {
	RepositoryID string   `json:"repository_id"`
	Versions     []string `json:"versions"`
}

type Evidence struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

type Message struct {
	ID        string    `json:"id"`
	ActorID   string    `json:"actor_id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

type AccessEvent struct {
	ID        string    `json:"id"`
	ActorID   string    `json:"actor_id"`
	Action    string    `json:"action"`
	Detail    string    `json:"detail,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type Advisory struct {
	ID                   string               `json:"id"`
	Title                string               `json:"title"`
	Description          string               `json:"description"`
	AffectedRepositories []AffectedRepository `json:"affected_repositories"`
	Evidence             []Evidence           `json:"evidence"`
	Contact              string               `json:"contact"`
	ReporterID           string               `json:"reporter_id"`
	ResponseTeam         []string             `json:"response_team"`
	Severity             string               `json:"severity"`
	EmbargoState         string               `json:"embargo_state"`
	Messages             []Message            `json:"messages"`
	AccessLog            []AccessEvent        `json:"access_log"`
	Version              int                  `json:"version"`
	CreatedAt            time.Time            `json:"created_at"`
	UpdatedAt            time.Time            `json:"updated_at"`
}

type Store struct {
	root string
	mu   sync.Mutex
	now  func() time.Time
}

func New(root string) (*Store, error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	if err := os.Chmod(root, 0700); err != nil {
		return nil, err
	}
	return &Store{root: root, now: func() time.Time { return time.Now().UTC() }}, nil
}

func (s *Store) Create(v Advisory) (Advisory, error) {
	if err := validateCreate(v); err != nil {
		return Advisory{}, err
	}
	v.Title, v.Description, v.Contact = strings.TrimSpace(v.Title), strings.TrimSpace(v.Description), strings.TrimSpace(v.Contact)
	for i := range v.AffectedRepositories {
		for j := range v.AffectedRepositories[i].Versions {
			v.AffectedRepositories[i].Versions[j] = strings.TrimSpace(v.AffectedRepositories[i].Versions[j])
		}
	}
	for i := range v.Evidence {
		v.Evidence[i].Label, v.Evidence[i].Description = strings.TrimSpace(v.Evidence[i].Label), strings.TrimSpace(v.Evidence[i].Description)
	}
	now := s.now()
	v.ID, v.Severity, v.EmbargoState, v.Version = mustID(), "untriaged", "reported", 1
	v.CreatedAt, v.UpdatedAt = now, now
	v.ResponseTeam, v.Messages = []string{}, []Message{}
	v.AccessLog = []AccessEvent{{ID: mustID(), ActorID: v.ReporterID, Action: "reported", CreatedAt: now}}
	err := s.mutate(func() error { return s.write(v) })
	return v, err
}

func (s *Store) Get(id string) (Advisory, error) {
	var v Advisory
	if !validID(id) {
		return v, ErrNotFound
	}
	if err := s.read(id, &v); err != nil {
		return Advisory{}, ErrNotFound
	}
	return v, nil
}

func (s *Store) List() ([]Advisory, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	out := []Advisory{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		var v Advisory
		if err := s.read(strings.TrimSuffix(entry.Name(), ".json"), &v); err != nil {
			continue
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *Store) RecordAccess(id, actor string) (Advisory, error) {
	return s.update(id, func(v *Advisory) error {
		if !validID(actor) {
			return ErrInvalid
		}
		v.AccessLog = append(v.AccessLog, AccessEvent{ID: mustID(), ActorID: actor, Action: "viewed", CreatedAt: s.now()})
		return nil
	})
}

func (s *Store) Triage(id, actor string, expected int, severity, embargo string) (Advisory, error) {
	return s.update(id, func(v *Advisory) error {
		if v.Version != expected {
			return ErrConflict
		}
		if !validID(actor) || !oneOf(severity, "low", "moderate", "high", "critical") || !oneOf(embargo, "reported", "triaging", "embargoed", "coordinating") {
			return ErrInvalid
		}
		v.Severity, v.EmbargoState, v.Version = severity, embargo, v.Version+1
		v.AccessLog = append(v.AccessLog, AccessEvent{ID: mustID(), ActorID: actor, Action: "triage_updated", Detail: severity + " / " + embargo, CreatedAt: s.now()})
		return nil
	})
}

func (s *Store) Invite(id, actor, userID string) (Advisory, error) {
	return s.update(id, func(v *Advisory) error {
		if !validID(actor) || !validID(userID) {
			return ErrInvalid
		}
		for _, id := range v.ResponseTeam {
			if id == userID {
				return nil
			}
		}
		if userID == v.ReporterID {
			return nil
		}
		if len(v.ResponseTeam) >= 20 {
			return ErrInvalid
		}
		v.ResponseTeam = append(v.ResponseTeam, userID)
		v.Version++
		v.AccessLog = append(v.AccessLog, AccessEvent{ID: mustID(), ActorID: actor, Action: "responder_invited", Detail: userID, CreatedAt: s.now()})
		return nil
	})
}

func (s *Store) AddMessage(id, actor, body string) (Advisory, error) {
	return s.update(id, func(v *Advisory) error {
		body = strings.TrimSpace(body)
		if !validID(actor) || body == "" || len(body) > 20000 {
			return ErrInvalid
		}
		now := s.now()
		v.Messages = append(v.Messages, Message{ID: mustID(), ActorID: actor, Body: body, CreatedAt: now})
		v.Version++
		v.AccessLog = append(v.AccessLog, AccessEvent{ID: mustID(), ActorID: actor, Action: "message_added", CreatedAt: now})
		return nil
	})
}

func (s *Store) update(id string, fn func(*Advisory) error) (Advisory, error) {
	var v Advisory
	err := s.mutate(func() error {
		if err := s.read(id, &v); err != nil {
			return ErrNotFound
		}
		if err := fn(&v); err != nil {
			return err
		}
		v.UpdatedAt = s.now()
		return s.write(v)
	})
	return v, err
}

func validateCreate(v Advisory) error {
	v.Title, v.Description, v.Contact = strings.TrimSpace(v.Title), strings.TrimSpace(v.Description), strings.TrimSpace(v.Contact)
	if !validID(v.ReporterID) || v.Title == "" || len(v.Title) > 200 || v.Description == "" || len(v.Description) > 20000 || v.Contact == "" || len(v.Contact) > 500 || len(v.AffectedRepositories) == 0 || len(v.AffectedRepositories) > 20 || len(v.Evidence) > 20 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, scope := range v.AffectedRepositories {
		if !validID(scope.RepositoryID) || seen[scope.RepositoryID] || len(scope.Versions) == 0 || len(scope.Versions) > 50 {
			return ErrInvalid
		}
		seen[scope.RepositoryID] = true
		for _, version := range scope.Versions {
			if strings.TrimSpace(version) == "" || len(version) > 200 {
				return ErrInvalid
			}
		}
	}
	for _, evidence := range v.Evidence {
		if strings.TrimSpace(evidence.Label) == "" || len(evidence.Label) > 200 || strings.TrimSpace(evidence.Description) == "" || len(evidence.Description) > 10000 {
			return ErrInvalid
		}
	}
	return nil
}

func oneOf(v string, choices ...string) bool {
	for _, choice := range choices {
		if v == choice {
			return true
		}
	}
	return false
}
func validID(v string) bool {
	if len(v) != 32 {
		return false
	}
	_, err := hex.DecodeString(v)
	return err == nil
}
func mustID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func (s *Store) path(id string) string { return filepath.Join(s.root, id+".json") }
func (s *Store) read(id string, out *Advisory) error {
	data, err := os.ReadFile(s.path(id))
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}
func (s *Store) write(v Advisory) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.root, ".advisory-*.tmp")
	if err != nil {
		return err
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
	if err == nil {
		err = os.Rename(name, s.path(v.ID))
	}
	return err
}
func (s *Store) mutate(fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	return fn()
}
