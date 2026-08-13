// Package feedback persists privacy-controlled product feedback.
package feedback

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

var ErrNotFound = errors.New("feedback not found")
var ErrInvalid = errors.New("invalid feedback")

type Target struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id,omitempty"`
	Label      string `json:"label"`
}
type Evidence struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Summary    string `json:"summary"`
	URL        string `json:"url,omitempty"`
	Visibility string `json:"visibility"`
	Redacted   bool   `json:"redacted"`
}
type Link struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
}
type Comment struct {
	ID         string    `json:"id"`
	Body       string    `json:"body"`
	AuthorID   string    `json:"author_id,omitempty"`
	AuthorRole string    `json:"author_role"`
	CreatedAt  time.Time `json:"created_at"`
}
type Event struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	ActorID   string    `json:"actor_id,omitempty"`
	Detail    string    `json:"detail"`
	CreatedAt time.Time `json:"created_at"`
}
type Item struct {
	ID                 string     `json:"id"`
	RepositoryID       string     `json:"repository_id"`
	OrganizationID     string     `json:"organization_id,omitempty"`
	Target             Target     `json:"target"`
	Need               string     `json:"need"`
	DesiredOutcome     string     `json:"desired_outcome"`
	Frequency          string     `json:"frequency"`
	Impact             string     `json:"impact"`
	Audience           string     `json:"audience"`
	IdentityVisibility string     `json:"identity_visibility"`
	ContactPreference  string     `json:"contact_preference"`
	Contact            string     `json:"contact,omitempty"`
	ReporterID         string     `json:"reporter_id,omitempty"`
	Evidence           []Evidence `json:"evidence"`
	Links              []Link     `json:"links"`
	Comments           []Comment  `json:"comments"`
	History            []Event    `json:"history"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
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
	return &Store{root: root, now: time.Now}, nil
}
func id() (string, error) {
	b := make([]byte, 16)
	_, e := rand.Read(b)
	return hex.EncodeToString(b), e
}
func validID(v string) bool {
	if len(v) != 32 {
		return false
	}
	_, e := hex.DecodeString(v)
	return e == nil
}
func validText(v string, max int) bool { n := len(strings.TrimSpace(v)); return n > 0 && n <= max }
func validAudience(v string) bool      { return v == "project" || v == "organization_private" }
func validVisibility(v string) bool {
	return v == "audience" || v == "maintainers" || v == "reporter_only"
}
func validate(x Item) bool {
	if !validID(x.RepositoryID) || !validText(x.Target.Label, 200) || !validText(x.Need, 10000) || !validText(x.DesiredOutcome, 10000) || !validText(x.Frequency, 500) || !validText(x.Impact, 5000) || !validAudience(x.Audience) || !validVisibility(x.IdentityVisibility) || (x.ContactPreference != "none" && x.ContactPreference != "discussion" && x.ContactPreference != "direct") || len(x.Evidence) > 10 || len(x.Links) > 20 {
		return false
	}
	if x.Target.Kind != "project" && x.Target.Kind != "release" && x.Target.Kind != "journey" && x.Target.Kind != "preview" {
		return false
	}
	if x.Target.Kind != "project" && !validText(x.Target.ResourceID, 200) {
		return false
	}
	if x.ContactPreference == "direct" && !validText(x.Contact, 500) {
		return false
	}
	if x.ContactPreference != "direct" && strings.TrimSpace(x.Contact) != "" {
		return false
	}
	for _, e := range x.Evidence {
		if !validText(e.Name, 200) || !validText(e.Kind, 100) || !validText(e.Summary, 5000) || !validVisibility(e.Visibility) || !e.Redacted || len(e.URL) > 2000 {
			return false
		}
	}
	for _, l := range x.Links {
		if (l.Kind != "issue" && l.Kind != "experiment") || !validText(l.ResourceID, 200) {
			return false
		}
	}
	return true
}
func (s *Store) lock() (*os.File, error) {
	f, e := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e == nil {
		e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
	}
	return f, e
}
func (s *Store) write(x Item) error {
	b, e := json.Marshal(x)
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(s.root, ".feedback-*")
	if e != nil {
		return e
	}
	n := tmp.Name()
	defer os.Remove(n)
	_ = tmp.Chmod(0600)
	if _, e = tmp.Write(b); e == nil {
		e = tmp.Sync()
	}
	ce := tmp.Close()
	if e == nil {
		e = ce
	}
	if e == nil {
		e = os.Rename(n, filepath.Join(s.root, x.ID+".json"))
	}
	return e
}
func (s *Store) Create(x Item, actor string) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validate(x) || !validID(actor) {
		return Item{}, ErrInvalid
	}
	f, e := s.lock()
	if e != nil {
		return Item{}, e
	}
	defer f.Close()
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	x.ID, e = id()
	if e != nil {
		return Item{}, e
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	x.ReporterID = actor
	x.CreatedAt = now
	x.UpdatedAt = now
	for i := range x.Evidence {
		x.Evidence[i].ID, _ = id()
	}
	ev, _ := id()
	x.History = []Event{{ID: ev, Kind: "submitted", ActorID: actor, Detail: "Feedback submitted with explicit audience and consent preferences.", CreatedAt: now}}
	if e = s.write(x); e != nil {
		return Item{}, e
	}
	return x, nil
}
func (s *Store) AddComment(itemID, actor, body, role string) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validID(itemID) || !validID(actor) || !validText(body, 5000) || (role != "reporter" && role != "maintainer") {
		return Item{}, ErrInvalid
	}
	f, e := s.lock()
	if e != nil {
		return Item{}, e
	}
	defer f.Close()
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	x, e := s.get(itemID)
	if e != nil {
		return Item{}, e
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	cid, _ := id()
	x.Comments = append(x.Comments, Comment{ID: cid, Body: strings.TrimSpace(body), AuthorID: actor, AuthorRole: role, CreatedAt: now})
	eid, _ := id()
	x.History = append(x.History, Event{ID: eid, Kind: "commented", ActorID: actor, Detail: role + " added discussion", CreatedAt: now})
	x.UpdatedAt = now
	e = s.write(x)
	return x, e
}
func (s *Store) get(itemID string) (Item, error) {
	if !validID(itemID) {
		return Item{}, ErrNotFound
	}
	b, e := os.ReadFile(filepath.Join(s.root, itemID+".json"))
	if errors.Is(e, os.ErrNotExist) {
		return Item{}, ErrNotFound
	}
	var x Item
	if e == nil {
		e = json.Unmarshal(b, &x)
	}
	return x, e
}
func (s *Store) Get(itemID string) (Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.get(itemID)
}
func (s *Store) List(repositoryID string) ([]Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, e := os.ReadDir(s.root)
	if e != nil {
		return nil, e
	}
	out := []Item{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		x, e := s.get(strings.TrimSuffix(entry.Name(), ".json"))
		if e != nil {
			return nil, e
		}
		if x.RepositoryID == repositoryID {
			out = append(out, x)
		}
	}
	return out, nil
}
