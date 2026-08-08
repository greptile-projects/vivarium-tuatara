// Package incidents persists the shared operating picture for service incidents.
package incidents

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
	ErrNotFound = errors.New("incident not found")
	ErrInvalid  = errors.New("invalid incident")
	ErrConflict = errors.New("incident changed")
)

type Scope struct {
	RepositoryID   string   `json:"repository_id"`
	EnvironmentIDs []string `json:"environment_ids"`
}
type Source struct {
	RepositoryID string `json:"repository_id"`
	DeploymentID string `json:"deployment_id"`
	Stage        string `json:"stage,omitempty"`
	Signal       string `json:"signal,omitempty"`
}
type Role struct {
	Name   string `json:"name"`
	UserID string `json:"user_id"`
}
type Entry struct {
	ID             string    `json:"id"`
	Kind           string    `json:"kind"`
	ActorID        string    `json:"actor_id"`
	Message        string    `json:"message"`
	Audience       string    `json:"audience"`
	CreatedAt      time.Time `json:"created_at"`
	AcknowledgedBy []string  `json:"acknowledged_by,omitempty"`
}
type Incident struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	Summary    string     `json:"summary"`
	Severity   string     `json:"severity"`
	Status     string     `json:"status"`
	Scopes     []Scope    `json:"scopes"`
	Roles      []Role     `json:"roles"`
	Source     *Source    `json:"source,omitempty"`
	DeclaredBy string     `json:"declared_by"`
	Timeline   []Entry    `json:"timeline"`
	Version    int        `json:"version"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
}
type Store struct {
	root          string
	mu            sync.Mutex
	now           func() time.Time
	directorySync func(string) error
}

func New(root string) (*Store, error) {
	if root == "" {
		return nil, errors.New("incident root required")
	}
	abs, e := filepath.Abs(root)
	if e == nil {
		e = os.MkdirAll(abs, 0700)
	}
	if e != nil {
		return nil, e
	}
	return &Store{root: abs, now: func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }, directorySync: syncDirectory}, nil
}

func (s *Store) Create(v Incident) (Incident, error) {
	if !validIncident(v) {
		return Incident{}, ErrInvalid
	}
	id, e := newID()
	if e != nil {
		return Incident{}, e
	}
	now := s.now()
	v.ID = id
	v.Title = strings.TrimSpace(v.Title)
	v.Summary = strings.TrimSpace(v.Summary)
	v.Version = 1
	v.CreatedAt = now
	v.UpdatedAt = now
	v.Timeline = []Entry{{ID: mustID(), Kind: "declared", ActorID: v.DeclaredBy, Message: v.Summary, Audience: "participants", CreatedAt: now}}
	e = s.mutate(func() error { return s.write(v) })
	return v, e
}
func (s *Store) Get(id string) (Incident, error) {
	if !validID(id) {
		return Incident{}, ErrNotFound
	}
	var v Incident
	e := s.read(id, &v)
	return v, e
}
func (s *Store) List() ([]Incident, error) {
	es, e := os.ReadDir(s.root)
	if e != nil {
		return nil, e
	}
	out := []Incident{}
	for _, x := range es {
		if x.IsDir() || !strings.HasSuffix(x.Name(), ".json") {
			continue
		}
		var v Incident
		if e = s.read(strings.TrimSuffix(x.Name(), ".json"), &v); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

func (s *Store) Update(id, actor string, expected int, severity, status string, roles []Role, message string) (Incident, error) {
	var v Incident
	e := s.mutate(func() error {
		var x error
		if x = s.read(id, &v); x != nil {
			return x
		}
		if v.Version != expected {
			return ErrConflict
		}
		if !validID(actor) || !validSeverity(severity) || !validStatus(status) || !validRoles(roles) || len(message) > 10000 {
			return ErrInvalid
		}
		now := s.now()
		changes := []string{}
		if severity != v.Severity {
			changes = append(changes, "severity "+v.Severity+" → "+severity)
		}
		if status != v.Status {
			changes = append(changes, "status "+v.Status+" → "+status)
		}
		v.Severity = severity
		v.Status = status
		v.Roles = roles
		v.Version++
		v.UpdatedAt = now
		if status == "resolved" && v.ResolvedAt == nil {
			v.ResolvedAt = &now
		}
		if status != "resolved" {
			v.ResolvedAt = nil
		}
		text := strings.TrimSpace(message)
		if text == "" {
			text = strings.Join(changes, ", ")
		}
		v.Timeline = append(v.Timeline, Entry{ID: mustID(), Kind: "state_changed", ActorID: actor, Message: text, Audience: "participants", CreatedAt: now})
		return s.write(v)
	})
	return v, e
}
func (s *Store) AddUpdate(id, operationID, actor, message, audience string) (Incident, error) {
	var v Incident
	e := s.mutate(func() error {
		var x error
		if x = s.read(id, &v); x != nil {
			return x
		}
		message = strings.TrimSpace(message)
		if !validID(operationID) || !validID(actor) || message == "" || len(message) > 10000 || (audience != "participants" && audience != "public") {
			return ErrInvalid
		}
		for _, entry := range v.Timeline {
			if entry.ID == operationID {
				if entry.Kind != "update" || entry.ActorID != actor || entry.Message != message || entry.Audience != audience {
					return ErrConflict
				}
				return nil
			}
		}
		now := s.now()
		v.Version++
		v.UpdatedAt = now
		v.Timeline = append(v.Timeline, Entry{ID: operationID, Kind: "update", ActorID: actor, Message: message, Audience: audience, CreatedAt: now})
		return s.write(v)
	})
	return v, e
}
func (s *Store) Acknowledge(id, entryID, actor string) (Incident, error) {
	var v Incident
	e := s.mutate(func() error {
		var x error
		if x = s.read(id, &v); x != nil {
			return x
		}
		if !validID(actor) || !validID(entryID) {
			return ErrInvalid
		}
		found := false
		for i := range v.Timeline {
			if v.Timeline[i].ID == entryID {
				found = true
				for _, a := range v.Timeline[i].AcknowledgedBy {
					if a == actor {
						return nil
					}
				}
				v.Timeline[i].AcknowledgedBy = append(v.Timeline[i].AcknowledgedBy, actor)
			}
		}
		if !found {
			return ErrNotFound
		}
		v.Version++
		v.UpdatedAt = s.now()
		return s.write(v)
	})
	return v, e
}

func validIncident(v Incident) bool {
	return strings.TrimSpace(v.Title) != "" && len(v.Title) <= 200 && strings.TrimSpace(v.Summary) != "" && len(v.Summary) <= 10000 && validID(v.DeclaredBy) && validSeverity(v.Severity) && validStatus(v.Status) && len(v.Scopes) > 0 && len(v.Scopes) <= 25 && validRoles(v.Roles) && validScopes(v.Scopes)
}
func validScopes(v []Scope) bool {
	seen := map[string]bool{}
	for _, s := range v {
		if !validID(s.RepositoryID) || seen[s.RepositoryID] || len(s.EnvironmentIDs) > 25 {
			return false
		}
		seen[s.RepositoryID] = true
		for _, id := range s.EnvironmentIDs {
			if !validID(id) {
				return false
			}
		}
	}
	return true
}
func validRoles(v []Role) bool {
	if len(v) > 20 {
		return false
	}
	seen := map[string]bool{}
	for _, r := range v {
		r.Name = strings.TrimSpace(r.Name)
		if r.Name == "" || len(r.Name) > 50 || !validID(r.UserID) || seen[strings.ToLower(r.Name)] {
			return false
		}
		seen[strings.ToLower(r.Name)] = true
	}
	return true
}
func validSeverity(v string) bool { return v == "sev1" || v == "sev2" || v == "sev3" || v == "sev4" }
func validStatus(v string) bool {
	return v == "investigating" || v == "identified" || v == "monitoring" || v == "resolved"
}
func validID(v string) bool {
	if len(v) != 32 {
		return false
	}
	_, e := hex.DecodeString(v)
	return e == nil
}
func newID() (string, error) {
	b := make([]byte, 16)
	_, e := rand.Read(b)
	return hex.EncodeToString(b), e
}
func mustID() string {
	id, e := newID()
	if e != nil {
		panic(e)
	}
	return id
}
func (s *Store) read(id string, v *Incident) error {
	b, e := os.ReadFile(filepath.Join(s.root, id+".json"))
	if errors.Is(e, os.ErrNotExist) {
		return ErrNotFound
	}
	if e != nil {
		return e
	}
	if json.Unmarshal(b, v) != nil || v.ID != id {
		return errors.New("corrupt incident")
	}
	return nil
}
func (s *Store) write(v Incident) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(s.root, ".incident-*")
	if e != nil {
		return e
	}
	name := tmp.Name()
	defer os.Remove(name)
	if e = tmp.Chmod(0600); e == nil {
		_, e = tmp.Write(b)
	}
	if e == nil {
		e = tmp.Sync()
	}
	ce := tmp.Close()
	if e == nil {
		e = ce
	}
	if e == nil {
		e = os.Rename(name, filepath.Join(s.root, v.ID+".json"))
	}
	if e == nil {
		e = s.directorySync(s.root)
	}
	return e
}

func syncDirectory(path string) error {
	d, err := os.Open(path)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
func (s *Store) mutate(fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, e := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); e != nil {
		return e
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return fn()
}
