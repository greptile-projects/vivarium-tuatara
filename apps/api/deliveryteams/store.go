// Package deliveryteams persists the operating contract for temporary outcome teams.
package deliveryteams

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var ErrNotFound = errors.New("delivery team not found")
var ErrInvalid = errors.New("invalid delivery team")
var ErrConflict = errors.New("delivery team version changed")
var ErrForbidden = errors.New("delivery team mutation forbidden")

type Outcome struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Title      string `json:"title"`
}
type Budget struct {
	Unit  string `json:"unit"`
	Limit int    `json:"limit"`
}
type AccessRequirement struct {
	RepositoryID string `json:"repository_id"`
	Level        string `json:"level"`
}
type AccessPreview struct {
	RepositoryID string `json:"repository_id"`
	Required     string `json:"required"`
	Effective    string `json:"effective"`
	Source       string `json:"source"`
	Sufficient   bool   `json:"sufficient"`
}
type Participant struct {
	ID             string              `json:"id"`
	PrincipalType  string              `json:"principal_type"`
	PrincipalID    string              `json:"principal_id"`
	Role           string              `json:"role"`
	Responsibility string              `json:"responsibility"`
	Why            string              `json:"why"`
	Budget         *Budget             `json:"budget,omitempty"`
	Deadline       *time.Time          `json:"deadline,omitempty"`
	Escalation     string              `json:"escalation"`
	RequiredAccess []AccessRequirement `json:"required_access"`
	AccessPreview  []AccessPreview     `json:"access_preview"`
	Status         string              `json:"status"`
	InvitedBy      string              `json:"invited_by"`
	InvitedAt      time.Time           `json:"invited_at"`
	RespondedBy    string              `json:"responded_by,omitempty"`
	RespondedAt    *time.Time          `json:"responded_at,omitempty"`
	ReplacedBy     string              `json:"replaced_by,omitempty"`
}
type Event struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	ActorID   string    `json:"actor_id"`
	Summary   string    `json:"summary"`
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
}
type Team struct {
	ID             string        `json:"id"`
	RepositoryID   string        `json:"repository_id"`
	Outcome        Outcome       `json:"outcome"`
	Name           string        `json:"name"`
	Purpose        string        `json:"purpose"`
	OrganizerID    string        `json:"organizer_id"`
	OverallBudget  *Budget       `json:"overall_budget,omitempty"`
	Deadline       *time.Time    `json:"deadline,omitempty"`
	EscalationPath string        `json:"escalation_path"`
	Version        int           `json:"version"`
	Participants   []Participant `json:"participants"`
	Events         []Event       `json:"events"`
	CreatedAt      time.Time     `json:"created_at"`
	UpdatedAt      time.Time     `json:"updated_at"`
}
type Charter struct {
	Name           string        `json:"name"`
	Purpose        string        `json:"purpose"`
	OverallBudget  *Budget       `json:"overall_budget,omitempty"`
	Deadline       *time.Time    `json:"deadline,omitempty"`
	EscalationPath string        `json:"escalation_path"`
	Participants   []Participant `json:"participants"`
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
	p, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &Store{root: p, now: func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }}, nil
}
func id() (string, error) {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return hex.EncodeToString(b), nil
}
func validBudget(b *Budget) bool {
	return b == nil || (b.Limit > 0 && (b.Unit == "minutes" || b.Unit == "credits" || b.Unit == "usd"))
}
func validCharter(c Charter) bool {
	if strings.TrimSpace(c.Name) == "" || strings.TrimSpace(c.Purpose) == "" || strings.TrimSpace(c.EscalationPath) == "" || !validBudget(c.OverallBudget) {
		return false
	}
	seen := map[string]bool{}
	for _, p := range c.Participants {
		key := p.PrincipalType + ":" + p.PrincipalID
		if p.ID == "" || seen[key] || (p.PrincipalType != "human" && p.PrincipalType != "agent") || p.PrincipalID == "" || p.Role == "" || p.Responsibility == "" || p.Why == "" || p.Escalation == "" || !validBudget(p.Budget) {
			return false
		}
		seen[key] = true
	}
	return len(c.Participants) > 0
}
func (s *Store) path(v string) string { return filepath.Join(s.root, v+".json") }
func (s *Store) read(v string) (Team, error) {
	b, e := os.ReadFile(s.path(v))
	if os.IsNotExist(e) {
		return Team{}, ErrNotFound
	}
	if e != nil {
		return Team{}, e
	}
	var t Team
	if json.Unmarshal(b, &t) != nil {
		return Team{}, ErrInvalid
	}
	return t, nil
}
func (s *Store) write(t Team) error {
	b, e := json.MarshalIndent(t, "", "  ")
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(s.root, ".team-")
	if e != nil {
		return e
	}
	n := tmp.Name()
	defer os.Remove(n)
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
		e = os.Rename(n, s.path(t.ID))
	}
	return e
}
func event(kind, actor, summary string, version int, at time.Time) Event {
	i, _ := id()
	return Event{ID: i, Kind: kind, ActorID: actor, Summary: summary, Version: version, CreatedAt: at}
}
func (s *Store) Create(repositoryID string, outcome Outcome, c Charter, actor string) (Team, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if repositoryID == "" || actor == "" || outcome.Kind == "" || outcome.ResourceID == "" || outcome.Title == "" || !validCharter(c) {
		return Team{}, ErrInvalid
	}
	i, e := id()
	if e != nil {
		return Team{}, e
	}
	now := s.now()
	t := Team{ID: i, RepositoryID: repositoryID, Outcome: outcome, Name: c.Name, Purpose: c.Purpose, OrganizerID: actor, OverallBudget: c.OverallBudget, Deadline: c.Deadline, EscalationPath: c.EscalationPath, Version: 1, Participants: c.Participants, CreatedAt: now, UpdatedAt: now}
	for j := range t.Participants {
		t.Participants[j].Status = "pending"
		t.Participants[j].InvitedBy = actor
		t.Participants[j].InvitedAt = now
	}
	t.Events = []Event{event("team.created", actor, "Created the team charter", 1, now)}
	return t, s.write(t)
}
func (s *Store) Get(v string) (Team, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.read(v) }
func (s *Store) List() ([]Team, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	files, e := filepath.Glob(filepath.Join(s.root, "*.json"))
	if e != nil {
		return nil, e
	}
	out := []Team{}
	for _, f := range files {
		b, e := os.ReadFile(f)
		if e != nil {
			return nil, e
		}
		var t Team
		if e = json.Unmarshal(b, &t); e != nil {
			return nil, e
		}
		out = append(out, t)
	}
	return out, nil
}
func (s *Store) Update(v, actor string, expected int, c Charter) (Team, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, e := s.read(v)
	if e != nil {
		return t, e
	}
	if t.OrganizerID != actor {
		return t, ErrForbidden
	}
	if t.Version != expected {
		return t, ErrConflict
	}
	if !validCharter(c) {
		return t, ErrInvalid
	}
	now := s.now()
	old := map[string]Participant{}
	for _, p := range t.Participants {
		old[p.PrincipalType+":"+p.PrincipalID] = p
	}
	for i := range c.Participants {
		p := &c.Participants[i]
		if prior, ok := old[p.PrincipalType+":"+p.PrincipalID]; ok {
			p.Status = prior.Status
			p.InvitedBy = prior.InvitedBy
			p.InvitedAt = prior.InvitedAt
			p.RespondedBy = prior.RespondedBy
			p.RespondedAt = prior.RespondedAt
		} else {
			p.Status = "pending"
			p.InvitedBy = actor
			p.InvitedAt = now
		}
	}
	t.Name = c.Name
	t.Purpose = c.Purpose
	t.OverallBudget = c.OverallBudget
	t.Deadline = c.Deadline
	t.EscalationPath = c.EscalationPath
	t.Participants = c.Participants
	t.Version++
	t.UpdatedAt = now
	t.Events = append(t.Events, event("charter.changed", actor, "Changed roles, responsibilities, or operating boundaries", t.Version, now))
	return t, s.write(t)
}
func (s *Store) Respond(v, participantID, actor, decision string, expected int) (Team, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, e := s.read(v)
	if e != nil {
		return t, e
	}
	if t.Version != expected {
		return t, ErrConflict
	}
	if decision != "accepted" && decision != "declined" {
		return t, ErrInvalid
	}
	found := false
	now := s.now()
	for i := range t.Participants {
		p := &t.Participants[i]
		if p.ID == participantID && p.Status == "pending" {
			p.Status = decision
			p.RespondedBy = actor
			p.RespondedAt = &now
			found = true
		}
	}
	if !found {
		return t, ErrForbidden
	}
	t.Version++
	t.UpdatedAt = now
	t.Events = append(t.Events, event("invitation."+decision, actor, "Responded to the delivery-team invitation", t.Version, now))
	return t, s.write(t)
}
