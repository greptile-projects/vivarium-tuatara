// Package decisions persists collaborative technical decision scopes and discussion.
package decisions

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

var ErrNotFound = errors.New("decision not found")
var ErrInvalid = errors.New("invalid decision")
var ErrConflict = errors.New("decision version changed")

type Source struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id,omitempty"`
}
type Resource struct {
	Kind         string `json:"kind"`
	RepositoryID string `json:"repository_id,omitempty"`
	ResourceID   string `json:"resource_id,omitempty"`
	Label        string `json:"label"`
}
type Participant struct {
	UserID  string    `json:"user_id"`
	AddedBy string    `json:"added_by"`
	AddedAt time.Time `json:"added_at"`
}
type Scope struct {
	Question          string        `json:"question"`
	Constraints       []string      `json:"constraints"`
	SuccessMeasures   []string      `json:"success_measures"`
	Deadline          *time.Time    `json:"deadline,omitempty"`
	AffectedResources []Resource    `json:"affected_resources"`
	Participants      []Participant `json:"participants"`
	OwnerID           string        `json:"owner_id"`
}
type History struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	ActorID   string    `json:"actor_id"`
	Version   int       `json:"version"`
	Summary   string    `json:"summary"`
	Body      string    `json:"body,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
type Decision struct {
	ID           string    `json:"id"`
	RepositoryID string    `json:"repository_id"`
	Source       Source    `json:"source"`
	Status       string    `json:"status"`
	Scope        Scope     `json:"scope"`
	CreatedBy    string    `json:"created_by"`
	Version      int       `json:"version"`
	History      []History `json:"history"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
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
	return &Store{root: root, now: func() time.Time { return time.Now().UTC() }}, nil
}
func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
func cleanList(v []string) ([]string, bool) {
	out := []string{}
	for _, x := range v {
		x = strings.TrimSpace(x)
		if x == "" || len(x) > 500 {
			return nil, false
		}
		out = append(out, x)
	}
	return out, true
}
func validSource(k string) bool {
	switch k {
	case "repository", "proposal", "investigation", "incident", "evolution_plan", "stewardship_opportunity":
		return true
	}
	return false
}
func validate(s Scope) (Scope, error) {
	s.Question = strings.TrimSpace(s.Question)
	s.OwnerID = strings.TrimSpace(s.OwnerID)
	if s.Question == "" || len(s.Question) > 2000 || s.OwnerID == "" {
		return s, ErrInvalid
	}
	var ok bool
	if s.Constraints, ok = cleanList(s.Constraints); !ok {
		return s, ErrInvalid
	}
	if s.SuccessMeasures, ok = cleanList(s.SuccessMeasures); !ok {
		return s, ErrInvalid
	}
	if len(s.Constraints) > 50 || len(s.SuccessMeasures) > 50 || len(s.AffectedResources) > 100 || len(s.Participants) > 100 {
		return s, ErrInvalid
	}
	if len(s.Constraints) == 0 || len(s.SuccessMeasures) == 0 || len(s.AffectedResources) == 0 || len(s.Participants) == 0 || s.Deadline == nil || s.Deadline.IsZero() {
		return s, ErrInvalid
	}
	seen := map[string]bool{}
	for _, p := range s.Participants {
		if p.UserID == "" || seen[p.UserID] {
			return s, ErrInvalid
		}
		seen[p.UserID] = true
	}
	if !seen[s.OwnerID] {
		return s, ErrInvalid
	}
	for i := range s.AffectedResources {
		s.AffectedResources[i].Label = strings.TrimSpace(s.AffectedResources[i].Label)
		if s.AffectedResources[i].Kind == "" || s.AffectedResources[i].Label == "" || len(s.AffectedResources[i].Label) > 300 {
			return s, ErrInvalid
		}
	}
	return s, nil
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
	return func() { syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }, nil
}
func (s *Store) write(v Decision) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(s.root, ".decision-")
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
	if ce := tmp.Close(); e == nil {
		e = ce
	}
	if e == nil {
		e = os.Rename(name, filepath.Join(s.root, v.ID+".json"))
	}
	return e
}
func (s *Store) read(id string) (Decision, error) {
	var v Decision
	b, e := os.ReadFile(filepath.Join(s.root, id+".json"))
	if os.IsNotExist(e) {
		return v, ErrNotFound
	}
	if e != nil {
		return v, e
	}
	if json.Unmarshal(b, &v) != nil {
		return v, ErrInvalid
	}
	return v, nil
}
func (s *Store) Create(repo string, source Source, scope Scope, actor string) (Decision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, e := s.lock()
	if e != nil {
		return Decision{}, e
	}
	defer u()
	source.Kind = strings.TrimSpace(source.Kind)
	if repo == "" || actor == "" || !validSource(source.Kind) || (source.Kind != "repository" && source.ResourceID == "") {
		return Decision{}, ErrInvalid
	}
	now := s.now()
	for i := range scope.Participants {
		scope.Participants[i].AddedBy = actor
		scope.Participants[i].AddedAt = now
	}
	scope, e = validate(scope)
	if e != nil {
		return Decision{}, e
	}
	x, e := randomID()
	if e != nil {
		return Decision{}, e
	}
	h, _ := randomID()
	v := Decision{ID: x, RepositoryID: repo, Source: source, Status: "pending", Scope: scope, CreatedBy: actor, Version: 1, CreatedAt: now, UpdatedAt: now, History: []History{{ID: h, Kind: "scope_created", ActorID: actor, Version: 1, Summary: "Opened the decision", CreatedAt: now}}}
	return v, s.write(v)
}
func (s *Store) Get(id string) (Decision, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.read(id) }
func (s *Store) List() (out []Decision, e error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ents, e := os.ReadDir(s.root)
	if e != nil {
		return nil, e
	}
	for _, x := range ents {
		if strings.HasSuffix(x.Name(), ".json") {
			v, er := s.read(strings.TrimSuffix(x.Name(), ".json"))
			if er != nil {
				return nil, er
			}
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}
func (s *Store) Update(id, actor string, expected int, scope Scope, summary string) (Decision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, e := s.lock()
	if e != nil {
		return Decision{}, e
	}
	defer u()
	v, e := s.read(id)
	if e != nil {
		return v, e
	}
	if v.Version != expected {
		return v, ErrConflict
	}
	if !isParticipant(v, actor) {
		return v, ErrNotFound
	}
	scope, e = validate(scope)
	if e != nil {
		return v, e
	}
	now := s.now()
	previous := map[string]Participant{}
	for _, p := range v.Scope.Participants {
		previous[p.UserID] = p
	}
	for i := range scope.Participants {
		if retained, ok := previous[scope.Participants[i].UserID]; ok {
			scope.Participants[i] = retained
		} else {
			scope.Participants[i].AddedAt = now
			scope.Participants[i].AddedBy = actor
		}
	}
	summary = strings.TrimSpace(summary)
	if summary == "" || len(summary) > 500 {
		return v, ErrInvalid
	}
	v.Scope = scope
	v.Version++
	v.UpdatedAt = now
	h, _ := idgen()
	v.History = append(v.History, History{ID: h, Kind: "scope_changed", ActorID: actor, Version: v.Version, Summary: summary, CreatedAt: now})
	return v, s.write(v)
}
func idgen() (string, error) { return randomID() }
func (s *Store) Discuss(id, actor, body string) (Decision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, e := s.lock()
	if e != nil {
		return Decision{}, e
	}
	defer u()
	v, e := s.read(id)
	if e != nil {
		return v, e
	}
	if !isParticipant(v, actor) {
		return v, ErrNotFound
	}
	body = strings.TrimSpace(body)
	if body == "" || len(body) > 4000 {
		return v, ErrInvalid
	}
	h, _ := randomID()
	now := s.now()
	v.History = append(v.History, History{ID: h, Kind: "discussion", ActorID: actor, Version: v.Version, Summary: "Added to the discussion", Body: body, CreatedAt: now})
	v.UpdatedAt = now
	return v, s.write(v)
}
func isParticipant(v Decision, u string) bool {
	for _, p := range v.Scope.Participants {
		if p.UserID == u {
			return true
		}
	}
	return false
}
