// Package debugworkspaces persists shared, revision-exact starting context for production debugging.
package debugworkspaces

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

var ErrNotFound = errors.New("debugging workspace not found")
var ErrInvalid = errors.New("invalid debugging workspace")
var ErrConflict = errors.New("debugging workspace changed")

type Reference struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id,omitempty"`
	Revision   string `json:"revision,omitempty"`
	Label      string `json:"label"`
}
type Evidence struct {
	ID                string `json:"id"`
	Kind              string `json:"kind"`
	Reference         string `json:"reference"`
	Label             string `json:"label"`
	Visibility        string `json:"visibility"`
	Sanitization      string `json:"sanitization"`
	Available         bool   `json:"available"`
	UnavailableReason string `json:"unavailable_reason,omitempty"`
}
type Hypothesis struct {
	ID        string    `json:"id"`
	Statement string    `json:"statement"`
	Status    string    `json:"status"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}
type Event struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	ActorID   string    `json:"actor_id"`
	From      string    `json:"from,omitempty"`
	To        string    `json:"to,omitempty"`
	Message   string    `json:"message,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
type Workspace struct {
	ID                 string       `json:"id"`
	RepositoryID       string       `json:"repository_id"`
	Version            int          `json:"version"`
	Title              string       `json:"title"`
	Summary            string       `json:"summary"`
	Trigger            Reference    `json:"trigger"`
	Release            Reference    `json:"release"`
	Environment        Reference    `json:"environment"`
	TimeStart          time.Time    `json:"time_start"`
	TimeEnd            time.Time    `json:"time_end"`
	UserJourney        string       `json:"user_journey"`
	OwnerIDs           []string     `json:"owner_ids"`
	Severity           string       `json:"severity"`
	Audience           string       `json:"audience"`
	AccessUserIDs      []string     `json:"access_user_ids"`
	Status             string       `json:"status"`
	Source             Reference    `json:"source"`
	Packages           []Reference  `json:"packages"`
	Configuration      Reference    `json:"configuration"`
	Infrastructure     Reference    `json:"infrastructure"`
	Evidence           []Evidence   `json:"permitted_evidence"`
	UnavailableContext []string     `json:"unavailable_context"`
	Hypotheses         []Hypothesis `json:"hypotheses"`
	History            []Event      `json:"history"`
	CreatedBy          string       `json:"created_by"`
	CreatedAt          time.Time    `json:"created_at"`
	UpdatedAt          time.Time    `json:"updated_at"`
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
func (s *Store) Create(v Workspace, actor string) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Workspace{}, err
	}
	defer unlock()
	if !valid(v, actor) {
		return Workspace{}, ErrInvalid
	}
	now := s.now()
	v.ID = id()
	v.Version = 1
	v.Status = "open"
	v.CreatedBy = actor
	v.CreatedAt = now
	v.UpdatedAt = now
	v.OwnerIDs = unique(v.OwnerIDs)
	v.AccessUserIDs = unique(v.AccessUserIDs)
	if v.Evidence == nil {
		v.Evidence = []Evidence{}
	}
	if v.Packages == nil {
		v.Packages = []Reference{}
	}
	if v.UnavailableContext == nil {
		v.UnavailableContext = []string{}
	}
	for i := range v.Evidence {
		v.Evidence[i].ID = id()
	}
	v.Hypotheses = []Hypothesis{}
	v.History = []Event{{ID: id(), Kind: "opened", ActorID: actor, To: "open", CreatedAt: now}}
	return v, s.write(v)
}
func (s *Store) Get(repo, wid string) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read(repo, wid)
}
func (s *Store) List(repo string) ([]Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(filepath.Join(s.root, repo))
	if errors.Is(err, os.ErrNotExist) {
		return []Workspace{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []Workspace{}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		v, x := s.read(repo, strings.TrimSuffix(e.Name(), ".json"))
		if x != nil {
			return nil, x
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}
func (s *Store) Update(repo, wid, actor, kind, value, message string, expected int) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Workspace{}, err
	}
	defer unlock()
	v, err := s.read(repo, wid)
	if err != nil {
		return Workspace{}, err
	}
	if v.Version != expected {
		return Workspace{}, ErrConflict
	}
	now := s.now()
	ev := Event{ID: id(), Kind: kind, ActorID: actor, Message: strings.TrimSpace(message), CreatedAt: now}
	switch kind {
	case "status":
		if !one(value, "open", "investigating", "blocked", "resolved", "closed") {
			return Workspace{}, ErrInvalid
		}
		ev.From = v.Status
		ev.To = value
		v.Status = value
	case "hypothesis":
		if strings.TrimSpace(value) == "" || len(value) > 4000 {
			return Workspace{}, ErrInvalid
		}
		h := Hypothesis{ID: id(), Statement: strings.TrimSpace(value), Status: "proposed", CreatedBy: actor, CreatedAt: now}
		v.Hypotheses = append(v.Hypotheses, h)
		ev.To = h.ID
	default:
		return Workspace{}, ErrInvalid
	}
	v.Version++
	v.UpdatedAt = now
	v.History = append(v.History, ev)
	return v, s.write(v)
}
func valid(v Workspace, actor string) bool {
	if !closed(v.RepositoryID) || !closed(actor) || strings.TrimSpace(v.Title) == "" || len(v.Title) > 200 || strings.TrimSpace(v.Summary) == "" || len(v.Summary) > 5000 || !one(v.Trigger.Kind, "issue", "incident", "support_thread", "deployment", "service_objective", "trace", "manual_observation") || strings.TrimSpace(v.Trigger.Label) == "" || !closed(v.Release.ResourceID) || len(v.Release.Revision) != 40 || !closed(v.Environment.ResourceID) || v.TimeStart.IsZero() || !v.TimeStart.Before(v.TimeEnd) || v.TimeEnd.Sub(v.TimeStart) > 31*24*time.Hour || strings.TrimSpace(v.UserJourney) == "" || len(v.OwnerIDs) == 0 || !one(v.Severity, "low", "medium", "high", "critical") || !one(v.Audience, "repository", "restricted") || len(v.Source.Revision) != 40 {
		return false
	}
	if v.Trigger.Kind != "manual_observation" && v.Trigger.Kind != "trace" && !closed(v.Trigger.ResourceID) {
		return false
	}
	for _, id := range append(append([]string{}, v.OwnerIDs...), v.AccessUserIDs...) {
		if !closed(id) {
			return false
		}
	}
	if v.Audience == "restricted" && len(v.AccessUserIDs) == 0 {
		return false
	}
	for _, e := range v.Evidence {
		if !one(e.Kind, "log", "trace", "metric", "profile", "snapshot", "report", "link", "observation") || strings.TrimSpace(e.Label) == "" || len(e.Reference) > 2000 || !one(e.Visibility, "repository", "restricted") || strings.TrimSpace(e.Sanitization) == "" || (e.Available && strings.TrimSpace(e.Reference) == "") || (!e.Available && strings.TrimSpace(e.UnavailableReason) == "") {
			return false
		}
	}
	return true
}
func (s *Store) read(repo, wid string) (Workspace, error) {
	var v Workspace
	if !closed(repo) || !closed(wid) {
		return v, ErrNotFound
	}
	b, err := os.ReadFile(filepath.Join(s.root, repo, wid+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return v, ErrNotFound
	}
	if err != nil {
		return v, err
	}
	if json.Unmarshal(b, &v) != nil || v.ID != wid || v.RepositoryID != repo {
		return Workspace{}, ErrNotFound
	}
	return v, nil
}
func (s *Store) write(v Workspace) error {
	dir := filepath.Join(s.root, v.RepositoryID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".debug-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(b)
	}
	if err == nil {
		err = tmp.Sync()
	}
	cerr := tmp.Close()
	if err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(name, filepath.Join(dir, v.ID+".json"))
	}
	if err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	err = d.Sync()
	cerr = d.Close()
	if err == nil {
		err = cerr
	}
	return err
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
func id() string { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func closed(v string) bool {
	if len(v) != 32 {
		return false
	}
	b, e := hex.DecodeString(v)
	return e == nil && len(b) == 16 && v == strings.ToLower(v)
}
func one(v string, a ...string) bool {
	for _, x := range a {
		if v == x {
			return true
		}
	}
	return false
}
func unique(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range in {
		if closed(v) && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}
