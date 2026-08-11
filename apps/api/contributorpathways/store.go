// Package contributorpathways retains the published expectations a newcomer
// should understand before investing effort in a repository.
package contributorpathways

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	ErrNotFound            = errors.New("contributor pathway not found")
	ErrInvalid             = errors.New("invalid contributor pathway")
	ErrConflict            = errors.New("contributor pathway version changed")
	ErrAcknowledged        = errors.New("contributor pathway already acknowledged")
	ErrDurabilityUncertain = errors.New("contributor pathway mutation is visible but durability is uncertain")
)

type Setup struct {
	Summary              string   `json:"summary"`
	WorkspacePath        string   `json:"workspace_path,omitempty"`
	VerificationCommands []string `json:"verification_commands"`
}

type WorkCategory struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Audience    string `json:"audience"`
}

type RequirementLink struct {
	Kind         string `json:"kind"`
	Label        string `json:"label"`
	ResourceID   string `json:"resource_id,omitempty"`
	Path         string `json:"path,omitempty"`
	Revision     string `json:"revision,omitempty"`
	Status       string `json:"status,omitempty"`
	StatusDetail string `json:"status_detail,omitempty"`
}

type Revision struct {
	ID             string            `json:"id"`
	RepositoryID   string            `json:"repository_id"`
	Version        int               `json:"version"`
	Goals          string            `json:"goals"`
	Prerequisites  []string          `json:"prerequisites"`
	Conduct        string            `json:"conduct"`
	Security       string            `json:"security"`
	Setup          Setup             `json:"setup"`
	Communication  string            `json:"communication"`
	ReviewPolicy   string            `json:"review_policy"`
	WorkCategories []WorkCategory    `json:"work_categories"`
	Requirements   []RequirementLink `json:"requirements"`
	PublishedBy    string            `json:"published_by"`
	PublishedAt    time.Time         `json:"published_at"`
}

type Acknowledgement struct {
	ID             string    `json:"id"`
	RepositoryID   string    `json:"repository_id"`
	Version        int       `json:"version"`
	ActorID        string    `json:"actor_id"`
	AcknowledgedAt time.Time `json:"acknowledged_at"`
}

type Store struct {
	root          string
	mu            sync.Mutex
	now           func() time.Time
	directorySync func(string) error
}

// WithCurrentVersion holds the pathway publication boundary while fn commits
// work whose governing provenance names expectedVersion.
func (s *Store) WithCurrentVersion(repositoryID string, expectedVersion int, fn func(Revision) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()
	items, err := s.list(repositoryID)
	if err != nil {
		return err
	}
	if len(items) == 0 || items[len(items)-1].Version != expectedVersion {
		return ErrConflict
	}
	return fn(items[len(items)-1])
}

func New(root string) (*Store, error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	return &Store{root: root, now: time.Now, directorySync: syncDir}, nil
}

func (s *Store) Publish(input Revision, expectedVersion int) (Revision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Revision{}, err
	}
	defer unlock()
	items, err := s.list(input.RepositoryID)
	if err != nil {
		return Revision{}, err
	}
	if len(items) != expectedVersion {
		return Revision{}, ErrConflict
	}
	if err := validate(input); err != nil {
		return Revision{}, err
	}
	id, err := randomID()
	if err != nil {
		return Revision{}, err
	}
	input.ID, input.Version, input.PublishedAt = id, len(items)+1, s.now().UTC().Truncate(time.Microsecond)
	dir := filepath.Join(s.root, input.RepositoryID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return Revision{}, err
	}
	if err := writeJSON(filepath.Join(dir, "revision-"+formatVersion(input.Version)+".json"), input); err != nil {
		return Revision{}, err
	}
	if err := s.directorySync(dir); err != nil {
		return input, errors.Join(ErrDurabilityUncertain, err)
	}
	return input, nil
}

func (s *Store) Current(repositoryID string) (Revision, error) {
	items, err := s.List(repositoryID)
	if err != nil || len(items) == 0 {
		if err != nil {
			return Revision{}, err
		}
		return Revision{}, ErrNotFound
	}
	return items[len(items)-1], nil
}

func (s *Store) List(repositoryID string) ([]Revision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.list(repositoryID)
}
func (s *Store) list(repositoryID string) ([]Revision, error) {
	if !validID(repositoryID) {
		return nil, ErrNotFound
	}
	dir := filepath.Join(s.root, repositoryID)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []Revision{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := []Revision{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "revision-") || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		var v Revision
		if readJSON(filepath.Join(dir, entry.Name()), &v) == nil && v.RepositoryID == repositoryID {
			result = append(result, v)
		}
	}
	// Fixed-width filenames preserve version order.
	return result, nil
}

func (s *Store) Acknowledge(repositoryID string, version int, actorID string) (Acknowledgement, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Acknowledgement{}, err
	}
	defer unlock()
	items, err := s.list(repositoryID)
	if err != nil || version < 1 || version > len(items) {
		return Acknowledgement{}, ErrNotFound
	}
	dir := filepath.Join(s.root, repositoryID, "acknowledgements")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return Acknowledgement{}, err
	}
	name := filepath.Join(dir, formatVersion(version)+"-"+actorID+".json")
	if _, err := os.Stat(name); err == nil {
		return Acknowledgement{}, ErrAcknowledged
	}
	id, err := randomID()
	if err != nil {
		return Acknowledgement{}, err
	}
	v := Acknowledgement{ID: id, RepositoryID: repositoryID, Version: version, ActorID: actorID, AcknowledgedAt: s.now().UTC().Truncate(time.Microsecond)}
	if err := writeJSON(name, v); err != nil {
		return Acknowledgement{}, err
	}
	if err := s.directorySync(dir); err != nil {
		return v, errors.Join(ErrDurabilityUncertain, err)
	}
	return v, nil
}

func (s *Store) Acknowledgements(repositoryID string) ([]Acknowledgement, error) {
	dir := filepath.Join(s.root, repositoryID, "acknowledgements")
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []Acknowledgement{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := []Acknowledgement{}
	for _, entry := range entries {
		var v Acknowledgement
		if !entry.IsDir() && readJSON(filepath.Join(dir, entry.Name()), &v) == nil {
			result = append(result, v)
		}
	}
	return result, nil
}

func validate(v Revision) error {
	if !validID(v.RepositoryID) || !validID(v.PublishedBy) || len(strings.TrimSpace(v.Goals)) < 3 || len(v.Goals) > 10000 || len(strings.TrimSpace(v.Conduct)) < 3 || len(strings.TrimSpace(v.Security)) < 3 || len(strings.TrimSpace(v.Setup.Summary)) < 3 || len(strings.TrimSpace(v.Communication)) < 3 || len(strings.TrimSpace(v.ReviewPolicy)) < 3 || len(v.Prerequisites) == 0 || len(v.Prerequisites) > 50 || len(v.WorkCategories) == 0 || len(v.WorkCategories) > 50 || len(v.Requirements) > 100 {
		return ErrInvalid
	}
	for _, value := range v.Prerequisites {
		if strings.TrimSpace(value) == "" || len(value) > 500 {
			return ErrInvalid
		}
	}
	for _, value := range v.Setup.VerificationCommands {
		if strings.TrimSpace(value) == "" || len(value) > 500 {
			return ErrInvalid
		}
	}
	for _, c := range v.WorkCategories {
		if strings.TrimSpace(c.Name) == "" || strings.TrimSpace(c.Description) == "" || c.Audience != "human" && c.Audience != "agent" && c.Audience != "human_or_agent" {
			return ErrInvalid
		}
	}
	for _, link := range v.Requirements {
		if strings.TrimSpace(link.Label) == "" || !oneOf(link.Kind, "documentation", "ownership", "release", "issue", "proposal", "workspace_definition") {
			return ErrInvalid
		}
		if link.Kind == "documentation" && (link.Path == "" || strings.HasPrefix(link.Path, "/") || strings.Contains(link.Path, "..")) {
			return ErrInvalid
		}
		if link.Kind != "documentation" && link.Kind != "ownership" && !validID(link.ResourceID) {
			return ErrInvalid
		}
	}
	return nil
}
func oneOf(v string, values ...string) bool {
	for _, x := range values {
		if v == x {
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
	return err == nil && v == strings.ToLower(v)
}
func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
func formatVersion(v int) string { return fmt.Sprintf("%09d", v) }
func readJSON(name string, out any) error {
	data, err := os.ReadFile(name)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}
func writeJSON(name string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(name), ".pathway-*")
	if err != nil {
		return err
	}
	n := tmp.Name()
	defer os.Remove(n)
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
		return err
	}
	return os.Rename(n, name)
}
func syncDir(name string) error {
	d, err := os.Open(name)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
func (s *Store) lock() (func(), error) {
	f, err := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return func() { syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }, nil
}
