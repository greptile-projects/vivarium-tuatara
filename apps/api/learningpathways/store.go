// Package learningpathways retains immutable, project-grounded curricula.
package learningpathways

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
	ErrNotFound            = errors.New("learning pathway not found")
	ErrInvalid             = errors.New("invalid learning pathway")
	ErrConflict            = errors.New("learning pathway version changed")
	ErrRequestChanged      = errors.New("learning pathway request identity was reused with different content")
	ErrDurabilityUncertain = errors.New("learning pathway is visible but durability is uncertain")
)

type Mentor struct {
	UserID         string `json:"user_id"`
	Responsibility string `json:"responsibility"`
	Status         string `json:"status,omitempty"`
	StatusDetail   string `json:"status_detail,omitempty"`
}
type Environment struct {
	Name         string   `json:"name"`
	Requirements []string `json:"requirements"`
	Supported    bool     `json:"supported"`
	OwnerID      string   `json:"owner_id,omitempty"`
	Status       string   `json:"status,omitempty"`
	StatusDetail string   `json:"status_detail,omitempty"`
}
type Material struct {
	Kind           string `json:"kind"`
	Label          string `json:"label"`
	ResourceID     string `json:"resource_id,omitempty"`
	Path           string `json:"path,omitempty"`
	Symbol         string `json:"symbol,omitempty"`
	Revision       string `json:"revision,omitempty"`
	PackageVersion string `json:"package_version,omitempty"`
	OwnerID        string `json:"owner_id,omitempty"`
	Status         string `json:"status,omitempty"`
	StatusDetail   string `json:"status_detail,omitempty"`
}
type Exercise struct {
	ID                 string         `json:"id"`
	Title              string         `json:"title"`
	Kind               string         `json:"kind"`
	Instructions       string         `json:"instructions"`
	Revision           string         `json:"revision"`
	StarterCommands    []string       `json:"starter_commands"`
	AcceptanceCriteria []string       `json:"acceptance_criteria"`
	Hints              []string       `json:"hints,omitempty"`
	Data               []ExerciseData `json:"data,omitempty"`
	Evidence           []string       `json:"completion_evidence"`
}
type ExerciseData struct {
	Kind    string `json:"kind"`
	Path    string `json:"path"`
	Content string `json:"content,omitempty"`
	Source  string `json:"source,omitempty"`
}
type Module struct {
	ID               string     `json:"id"`
	Title            string     `json:"title"`
	WhyItMatters     string     `json:"why_it_matters"`
	Objectives       []string   `json:"objectives"`
	EstimatedMinutes int        `json:"estimated_minutes"`
	Exercises        []Exercise `json:"exercises"`
	Materials        []Material `json:"materials"`
}
type Revision struct {
	ID                 string        `json:"id"`
	RequestID          string        `json:"request_id"`
	RepositoryID       string        `json:"repository_id"`
	Slug               string        `json:"slug"`
	Version            int           `json:"version"`
	Role               string        `json:"role"`
	Outcome            string        `json:"outcome"`
	Prerequisites      []string      `json:"prerequisites"`
	Objectives         []string      `json:"objectives"`
	SupportedRevisions []string      `json:"supported_revisions"`
	Modules            []Module      `json:"modules"`
	Mentors            []Mentor      `json:"mentors"`
	ExpectedMinutes    int           `json:"expected_minutes"`
	AccessibilityNeeds []string      `json:"accessibility_needs"`
	Locales            []string      `json:"locales"`
	CompletionEvidence []string      `json:"completion_evidence"`
	Environments       []Environment `json:"environments"`
	PublishedBy        string        `json:"published_by"`
	PublishedAt        time.Time     `json:"published_at"`
}

type Store struct {
	root    string
	mu      sync.Mutex
	now     func() time.Time
	syncDir func(string) error
}

func New(root string) (*Store, error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	return &Store{root: root, now: time.Now, syncDir: directorySync}, nil
}

func (s *Store) Publish(v Revision, expected int) (Revision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Revision{}, err
	}
	defer unlock()
	items, err := s.list(v.RepositoryID, v.Slug)
	if err != nil {
		return Revision{}, err
	}
	for _, item := range items {
		if item.RequestID != v.RequestID {
			continue
		}
		if !sameRequest(item, v) {
			return Revision{}, ErrRequestChanged
		}
		dir := filepath.Join(s.root, v.RepositoryID, v.Slug)
		if err = s.syncDir(dir); err != nil {
			return item, errors.Join(ErrDurabilityUncertain, err)
		}
		return item, nil
	}
	if len(items) != expected {
		return Revision{}, ErrConflict
	}
	if validate(v) != nil {
		return Revision{}, ErrInvalid
	}
	v.ID = randomID()
	v.Version = len(items) + 1
	v.PublishedAt = s.now().UTC().Truncate(time.Microsecond)
	dir := filepath.Join(s.root, v.RepositoryID, v.Slug)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return Revision{}, err
	}
	if err = writeJSON(filepath.Join(dir, fmt.Sprintf("revision-%09d.json", v.Version)), v); err != nil {
		return Revision{}, err
	}
	if err = s.syncDir(dir); err != nil {
		return v, errors.Join(ErrDurabilityUncertain, err)
	}
	return v, nil
}
func (s *Store) List(repositoryID, slug string) ([]Revision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.list(repositoryID, slug)
}
func (s *Store) list(repositoryID, slug string) ([]Revision, error) {
	if !validID(repositoryID) || !validSlug(slug) {
		return nil, ErrNotFound
	}
	es, err := os.ReadDir(filepath.Join(s.root, repositoryID, slug))
	if errors.Is(err, os.ErrNotExist) {
		return []Revision{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []Revision{}
	for _, e := range es {
		var v Revision
		if !e.IsDir() && strings.HasPrefix(e.Name(), "revision-") && readJSON(filepath.Join(s.root, repositoryID, slug, e.Name()), &v) == nil && v.RepositoryID == repositoryID && v.Slug == slug {
			out = append(out, v)
		}
	}
	return out, nil
}
func (s *Store) Current(repositoryID, slug string) (Revision, error) {
	vs, err := s.List(repositoryID, slug)
	if err != nil {
		return Revision{}, err
	}
	if len(vs) == 0 {
		return Revision{}, ErrNotFound
	}
	return vs[len(vs)-1], nil
}
func (s *Store) Slugs(repositoryID string) ([]string, error) {
	if !validID(repositoryID) {
		return nil, ErrNotFound
	}
	es, err := os.ReadDir(filepath.Join(s.root, repositoryID))
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, e := range es {
		if e.IsDir() && validSlug(e.Name()) {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

func validate(v Revision) error {
	if !validID(v.RepositoryID) || !validID(v.PublishedBy) || !validRequestID(v.RequestID) || !validSlug(v.Slug) || len(strings.TrimSpace(v.Role)) < 2 || len(strings.TrimSpace(v.Outcome)) < 3 || v.ExpectedMinutes < 1 || v.ExpectedMinutes > 100000 || len(v.Prerequisites) > 50 || len(v.Objectives) == 0 || len(v.Objectives) > 50 || len(v.SupportedRevisions) == 0 || len(v.Modules) == 0 || len(v.Modules) > 100 || len(v.CompletionEvidence) == 0 || len(v.Locales) == 0 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, value := range append(append(append([]string{}, v.Prerequisites...), v.Objectives...), v.CompletionEvidence...) {
		if strings.TrimSpace(value) == "" || len(value) > 1000 {
			return ErrInvalid
		}
	}
	for _, mentor := range v.Mentors {
		if !validID(mentor.UserID) || strings.TrimSpace(mentor.Responsibility) == "" {
			return ErrInvalid
		}
	}
	for _, environment := range v.Environments {
		if strings.TrimSpace(environment.Name) == "" || environment.OwnerID != "" && !validID(environment.OwnerID) {
			return ErrInvalid
		}
	}
	for _, r := range v.SupportedRevisions {
		if !isRevision(r) {
			return ErrInvalid
		}
	}
	for _, m := range v.Modules {
		if !validSlug(m.ID) || seen[m.ID] || strings.TrimSpace(m.Title) == "" || strings.TrimSpace(m.WhyItMatters) == "" || m.EstimatedMinutes < 1 || len(m.Objectives) == 0 || len(m.Exercises) == 0 {
			return ErrInvalid
		}
		seen[m.ID] = true
		exercises := map[string]bool{}
		for _, x := range m.Exercises {
			if strings.TrimSpace(x.Title) == "" || strings.TrimSpace(x.Instructions) == "" || len(x.Evidence) == 0 {
				return ErrInvalid
			}
			practice := x.ID != "" || x.Kind != "" || x.Revision != "" || len(x.AcceptanceCriteria) != 0 || len(x.StarterCommands) != 0 || len(x.Hints) != 0 || len(x.Data) != 0
			supported := false
			for _, revision := range v.SupportedRevisions {
				if revision == x.Revision {
					supported = true
				}
			}
			if practice && (!validSlug(x.ID) || exercises[x.ID] || !oneOf(x.Kind, "exploration", "command", "debugging", "test", "api", "change") || !isRevision(x.Revision) || !supported || len(x.AcceptanceCriteria) == 0 || len(x.StarterCommands) > 20 || len(x.Hints) > 20 || len(x.Data) > 20) {
				return ErrInvalid
			}
			if practice {
				exercises[x.ID] = true
			}
			for _, command := range x.StarterCommands {
				if strings.TrimSpace(command) == "" || len(command) > 2000 {
					return ErrInvalid
				}
			}
			for _, criterion := range x.AcceptanceCriteria {
				if strings.TrimSpace(criterion) == "" || len(criterion) > 1000 {
					return ErrInvalid
				}
			}
			for _, hint := range x.Hints {
				if strings.TrimSpace(hint) == "" || len(hint) > 2000 {
					return ErrInvalid
				}
			}
			for _, data := range x.Data {
				if !oneOf(data.Kind, "synthetic", "permitted") || !safePath(data.Path) || (data.Kind == "synthetic" && (data.Content == "" || len(data.Content) > 65536 || credentialLike(data.Path, data.Content))) || (data.Kind == "permitted" && (!safePath(data.Source) || data.Content != "")) {
					return ErrInvalid
				}
			}
		}
		for _, x := range m.Materials {
			if !oneOf(x.Kind, "documentation", "symbol", "decision", "issue", "api", "package", "contributor_guidance") || strings.TrimSpace(x.Label) == "" {
				return ErrInvalid
			}
			if oneOf(x.Kind, "documentation", "symbol", "api") && (!safePath(x.Path) || !isRevision(x.Revision)) {
				return ErrInvalid
			}
			if x.Kind == "symbol" && strings.TrimSpace(x.Symbol) == "" {
				return ErrInvalid
			}
			if oneOf(x.Kind, "decision", "issue") && !validID(x.ResourceID) {
				return ErrInvalid
			}
			if x.Kind == "package" && (x.ResourceID == "" || x.PackageVersion == "") {
				return ErrInvalid
			}
			if x.Kind == "contributor_guidance" && x.ResourceID == "" {
				return ErrInvalid
			}
			if x.OwnerID != "" && !validID(x.OwnerID) {
				return ErrInvalid
			}
		}
	}
	return nil
}

func credentialLike(path, content string) bool {
	lower := strings.ToLower(path + "\n" + content)
	for _, marker := range []string{"-----begin private key-----", "authorization: bearer ", "authorization: basic ", ".env", "password=", "passwd=", "api_key=", "apikey=", "access_token=", "client_secret="} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}
func oneOf(v string, xs ...string) bool {
	for _, x := range xs {
		if v == x {
			return true
		}
	}
	return false
}
func safePath(v string) bool {
	return v != "" && !strings.HasPrefix(v, "/") && !strings.Contains(v, "..")
}
func validSlug(v string) bool {
	if len(v) < 1 || len(v) > 64 {
		return false
	}
	for _, r := range v {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return true
}
func isRevision(v string) bool {
	if len(v) != 40 {
		return false
	}
	_, e := hex.DecodeString(v)
	return e == nil && v == strings.ToLower(v)
}
func validID(v string) bool {
	if len(v) != 32 {
		return false
	}
	_, e := hex.DecodeString(v)
	return e == nil && v == strings.ToLower(v)
}
func validRequestID(v string) bool {
	return len(v) >= 8 && len(v) <= 100 && !strings.ContainsAny(v, "/\\\x00")
}
func randomID() string { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func sameRequest(stored, input Revision) bool {
	stored.ID, input.ID = "", ""
	stored.Version, input.Version = 0, 0
	stored.PublishedAt, input.PublishedAt = time.Time{}, time.Time{}
	a, aErr := json.Marshal(stored)
	b, bErr := json.Marshal(input)
	return aErr == nil && bErr == nil && string(a) == string(b)
}
func readJSON(p string, v any) error {
	b, e := os.ReadFile(p)
	if e != nil {
		return e
	}
	return json.Unmarshal(b, v)
}
func writeJSON(p string, v any) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(p), ".learning-*")
	if e != nil {
		return e
	}
	n := f.Name()
	defer os.Remove(n)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	return os.Rename(n, p)
}
func directorySync(p string) error {
	f, e := os.Open(p)
	if e != nil {
		return e
	}
	defer f.Close()
	return f.Sync()
}
func (s *Store) lock() (func(), error) {
	f, e := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); e != nil {
		f.Close()
		return nil, e
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}
