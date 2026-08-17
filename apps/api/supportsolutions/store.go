// Package supportsolutions persists published, version-exact outcomes from support.
package supportsolutions

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

var ErrNotFound = errors.New("support solution not found")
var ErrInvalid = errors.New("invalid support solution")
var ErrConflict = errors.New("support solution changed")

type Link struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id,omitempty"`
	Label      string `json:"label"`
}
type Credit struct {
	ActorID string `json:"actor_id"`
	Role    string `json:"role"`
}
type Event struct {
	ID                string    `json:"id"`
	Kind              string    `json:"kind"`
	ActorID           string    `json:"actor_id"`
	Message           string    `json:"message,omitempty"`
	RelatedSolutionID string    `json:"related_solution_id,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}
type Notification struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Kind      string    `json:"kind"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}
type Solution struct {
	ID                    string         `json:"id"`
	RepositoryID          string         `json:"repository_id"`
	ThreadID              string         `json:"thread_id"`
	AnswerID              string         `json:"answer_id"`
	AnswerRevisionID      string         `json:"answer_revision_id"`
	VerificationAttemptID string         `json:"verification_attempt_id"`
	Title                 string         `json:"title"`
	Summary               string         `json:"summary"`
	Instructions          string         `json:"instructions"`
	Audience              string         `json:"audience"`
	ApplicableVersions    []string       `json:"applicable_versions"`
	Limitations           []string       `json:"limitations"`
	Links                 []Link         `json:"links"`
	Status                string         `json:"status"`
	DuplicateOf           string         `json:"duplicate_of,omitempty"`
	RevalidationVersions  []string       `json:"revalidation_versions,omitempty"`
	Credits               []Credit       `json:"credits"`
	Events                []Event        `json:"events"`
	Notifications         []Notification `json:"notifications"`
	Version               int            `json:"version"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
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
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	return &Store{root: root, now: func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }}, nil
}
func (s *Store) Create(v Solution, actor string) (Solution, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !valid(v) || actor == "" {
		return Solution{}, ErrInvalid
	}
	if existing, found, err := s.findResolution(v.RepositoryID, v.ThreadID, v.AnswerRevisionID, v.VerificationAttemptID); err != nil {
		return Solution{}, err
	} else if found {
		return existing, nil
	}
	now := s.now()
	v.ID = id()
	v.Status = "published"
	v.Version = 1
	v.CreatedAt = now
	v.UpdatedAt = now
	v.Events = []Event{{ID: id(), Kind: "published", ActorID: actor, CreatedAt: now}}
	addNotifications(&v, "solution_published", "A tested support solution was published.", now)
	return v, s.write(v)
}
func (s *Store) Get(repo, sid string) (Solution, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read(repo, sid)
}

// DeleteResolution compensates a failed source-thread close. It removes only the
// exact evidence-bound solution the caller just attempted to attach.
func (s *Store) DeleteResolution(repo, sid, thread, revision, attempt string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.read(repo, sid)
	if err != nil {
		return err
	}
	if v.ThreadID != thread || v.AnswerRevisionID != revision || v.VerificationAttemptID != attempt {
		return ErrInvalid
	}
	return os.Remove(filepath.Join(s.root, repo, sid+".json"))
}
func (s *Store) List(repo string) ([]Solution, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	es, e := os.ReadDir(filepath.Join(s.root, repo))
	if os.IsNotExist(e) {
		return []Solution{}, nil
	}
	if e != nil {
		return nil, e
	}
	out := []Solution{}
	for _, x := range es {
		if x.IsDir() || !strings.HasSuffix(x.Name(), ".json") {
			continue
		}
		v, er := s.read(repo, strings.TrimSuffix(x.Name(), ".json"))
		if er != nil {
			return nil, er
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}
func (s *Store) Transition(repo, sid, actor, action, message, related string, versions []string, expected int) (Solution, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !segment(repo) || !segment(sid) || strings.TrimSpace(actor) == "" {
		return Solution{}, ErrInvalid
	}
	v, e := s.read(repo, sid)
	if e != nil {
		return v, e
	}
	if v.Version != expected {
		return v, ErrConflict
	}
	kind, status, notice := "", "", ""
	if v.Status == "merged" || v.Status == "archived" {
		return v, ErrInvalid
	}
	switch action {
	case "archive":
		kind, status, notice = "archived", "archived", "A reusable support solution was archived as obsolete."
	case "request_revalidation":
		if len(clean(versions)) == 0 {
			return v, ErrInvalid
		}
		kind, status, notice = "revalidation_requested", "needs_revalidation", "A support solution needs testing for newer versions."
		v.RevalidationVersions = clean(versions)
	case "merge_duplicate":
		if related == "" || related == v.ID {
			return v, ErrInvalid
		}
		kind, status, notice = "duplicate_merged", "merged", "A duplicate support solution was merged."
		v.DuplicateOf = related
	default:
		return v, ErrInvalid
	}
	now := s.now()
	v.Status = status
	v.Version++
	v.UpdatedAt = now
	v.Events = append(v.Events, Event{ID: id(), Kind: kind, ActorID: actor, Message: strings.TrimSpace(message), RelatedSolutionID: related, CreatedAt: now})
	addNotifications(&v, kind, notice, now)
	return v, s.write(v)
}
func (s *Store) findResolution(repo, thread, revision, attempt string) (Solution, bool, error) {
	dir := filepath.Join(s.root, repo)
	es, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return Solution{}, false, nil
	}
	if err != nil {
		return Solution{}, false, err
	}
	for _, entry := range es {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		v, readErr := s.read(repo, strings.TrimSuffix(entry.Name(), ".json"))
		if readErr != nil {
			return Solution{}, false, readErr
		}
		if v.ThreadID == thread && v.AnswerRevisionID == revision && v.VerificationAttemptID == attempt {
			return v, true, nil
		}
	}
	return Solution{}, false, nil
}
func valid(v Solution) bool {
	if !segment(v.RepositoryID) || !segment(v.ThreadID) || !segment(v.AnswerID) || !segment(v.AnswerRevisionID) || !segment(v.VerificationAttemptID) || strings.TrimSpace(v.Title) == "" || strings.TrimSpace(v.Summary) == "" || strings.TrimSpace(v.Instructions) == "" || !map[string]bool{"public": true, "participants": true}[v.Audience] || len(clean(v.ApplicableVersions)) == 0 {
		return false
	}
	for _, x := range v.Links {
		if !map[string]bool{"search": true, "documentation": true, "package": true, "release": true, "contributor_guidance": true}[x.Kind] || strings.TrimSpace(x.Label) == "" {
			return false
		}
	}
	return true
}
func segment(v string) bool {
	return v != "" && v != "." && v != ".." && !strings.ContainsAny(v, `/\\`)
}
func clean(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, x := range in {
		x = strings.TrimSpace(x)
		if x != "" && !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}
func addNotifications(v *Solution, kind, message string, now time.Time) {
	seen := map[string]bool{}
	for _, c := range v.Credits {
		if c.ActorID != "" && !seen[c.ActorID] {
			seen[c.ActorID] = true
			v.Notifications = append(v.Notifications, Notification{ID: id(), UserID: c.ActorID, Kind: kind, Message: message, CreatedAt: now})
		}
	}
}
func (s *Store) read(repo, sid string) (Solution, error) {
	var v Solution
	if !segment(repo) || !segment(sid) {
		return v, ErrNotFound
	}
	b, e := os.ReadFile(filepath.Join(s.root, repo, sid+".json"))
	if os.IsNotExist(e) {
		return v, ErrNotFound
	}
	if e != nil {
		return v, e
	}
	if json.Unmarshal(b, &v) != nil || v.RepositoryID != repo {
		return v, ErrInvalid
	}
	return v, nil
}
func (s *Store) write(v Solution) error {
	d := filepath.Join(s.root, v.RepositoryID)
	if e := os.MkdirAll(d, 0700); e != nil {
		return e
	}
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(d, ".solution-*")
	if e != nil {
		return e
	}
	name := tmp.Name()
	defer os.Remove(name)
	_ = tmp.Chmod(0600)
	if _, e = tmp.Write(b); e == nil {
		e = tmp.Sync()
	}
	if c := tmp.Close(); e == nil {
		e = c
	}
	if e == nil {
		e = os.Rename(name, filepath.Join(d, v.ID+".json"))
	}
	return e
}
func id() string { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
