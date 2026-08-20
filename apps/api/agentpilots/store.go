// Package agentpilots retains bounded, consent-based trials of exact agent candidates.
package agentpilots

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"
)

var ErrInvalid = errors.New("invalid agent pilot")
var ErrNotFound = errors.New("agent pilot not found")
var ErrConflict = errors.New("agent pilot changed")
var ErrDenied = errors.New("agent pilot action denied")

var allowedActions = []string{"repository.read", "draft.create", "draft.update", "task.comment"}

type Budget struct {
	MaxMinutes int     `json:"max_minutes"`
	MaxActions int     `json:"max_actions"`
	MaxCost    float64 `json:"max_cost"`
}
type Invitation struct {
	ParticipantID string     `json:"participant_id"`
	Role          string     `json:"role"`
	RepositoryIDs []string   `json:"repository_ids"`
	TaskKinds     []string   `json:"task_kinds"`
	Actions       []string   `json:"actions"`
	ConsentedAt   *time.Time `json:"consented_at,omitempty"`
	RevokedAt     *time.Time `json:"revoked_at,omitempty"`
}
type SessionEvent struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	ActorID   string    `json:"actor_id"`
	Summary   string    `json:"summary"`
	Action    string    `json:"action,omitempty"`
	Cost      float64   `json:"cost,omitempty"`
	Minutes   int       `json:"minutes,omitempty"`
	Unsafe    bool      `json:"unsafe,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
type Session struct {
	ID              string         `json:"id"`
	ParticipantID   string         `json:"participant_id"`
	RepositoryID    string         `json:"repository_id"`
	TaskKind        string         `json:"task_kind"`
	TaskID          string         `json:"task_id"`
	ExpectedOutcome string         `json:"expected_outcome"`
	Status          string         `json:"status"`
	Events          []SessionEvent `json:"events"`
	CreatedAt       time.Time      `json:"created_at"`
}
type Feedback struct {
	ID                string    `json:"id"`
	ParticipantID     string    `json:"participant_id"`
	CandidateRevision string    `json:"candidate_revision"`
	SessionID         string    `json:"session_id,omitempty"`
	Outcome           string    `json:"outcome"`
	ExpectedOutcome   string    `json:"expected_outcome"`
	Correction        string    `json:"correction"`
	CreatedAt         time.Time `json:"created_at"`
}
type Pilot struct {
	ID                string       `json:"id"`
	RepositoryID      string       `json:"repository_id"`
	PullRequestID     string       `json:"pull_request_id"`
	CandidateID       string       `json:"candidate_id"`
	CandidateRevision string       `json:"candidate_revision"`
	OwnerID           string       `json:"owner_id"`
	RepositoryIDs     []string     `json:"repository_ids"`
	Roles             []string     `json:"roles"`
	TaskKinds         []string     `json:"task_kinds"`
	Actions           []string     `json:"actions"`
	Budget            Budget       `json:"budget"`
	StartsAt          time.Time    `json:"starts_at"`
	ExpiresAt         time.Time    `json:"expires_at"`
	Version           int          `json:"version"`
	Paused            bool         `json:"paused"`
	PauseReason       string       `json:"pause_reason,omitempty"`
	Invitations       []Invitation `json:"invitations"`
	Sessions          []Session    `json:"sessions"`
	Feedback          []Feedback   `json:"feedback"`
	CreatedAt         time.Time    `json:"created_at"`
	UpdatedAt         time.Time    `json:"updated_at"`
}
type Access struct {
	Status        string   `json:"status"`
	PauseReasons  []string `json:"pause_reasons"`
	Actions       []string `json:"actions"`
	RepositoryIDs []string `json:"repository_ids"`
	TaskKinds     []string `json:"task_kinds"`
	Budget        Budget   `json:"budget"`
	Used          Budget   `json:"used"`
	Remaining     Budget   `json:"remaining"`
	AuthorityNote string   `json:"authority_note"`
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
func id() string { var b [16]byte; _, _ = rand.Read(b[:]); return hex.EncodeToString(b[:]) }
func valid(v string, n int) bool {
	return strings.TrimSpace(v) != "" && len(v) <= n && !strings.ContainsRune(v, '\x00')
}
func (s *Store) path(v string) string { return filepath.Join(s.root, v+".json") }
func write(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".pilot-")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	if ce := f.Close(); e == nil {
		e = ce
	}
	if e == nil {
		e = os.Rename(name, path)
	}
	if e == nil {
		d, de := os.Open(filepath.Dir(path))
		if de == nil {
			e = d.Sync()
			_ = d.Close()
		} else {
			e = de
		}
	}
	return e
}
func (s *Store) get(v string) (Pilot, error) {
	var p Pilot
	b, e := os.ReadFile(s.path(v))
	if os.IsNotExist(e) {
		return p, ErrNotFound
	}
	if e != nil {
		return p, e
	}
	e = json.Unmarshal(b, &p)
	return p, e
}
func subset(values, allowed []string) bool {
	seen := map[string]bool{}
	for _, v := range values {
		if !valid(v, 100) || seen[v] || !slices.Contains(allowed, v) {
			return false
		}
		seen[v] = true
	}
	return len(values) > 0
}
func (s *Store) Create(p Pilot) (Pilot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if !valid(p.RepositoryID, 64) || !valid(p.PullRequestID, 64) || !valid(p.CandidateID, 64) || len(p.CandidateRevision) != 40 || !valid(p.OwnerID, 64) || len(p.RepositoryIDs) == 0 || len(p.Roles) == 0 || len(p.TaskKinds) == 0 || !subset(p.Actions, allowedActions) || p.Budget.MaxMinutes < 1 || p.Budget.MaxActions < 1 || p.Budget.MaxCost <= 0 || p.StartsAt.Before(now.Add(-time.Minute)) || !p.ExpiresAt.After(p.StartsAt) || p.ExpiresAt.After(p.StartsAt.Add(30*24*time.Hour)) {
		return Pilot{}, ErrInvalid
	}
	participants := map[string]bool{}
	for i := range p.Invitations {
		x := &p.Invitations[i]
		if !valid(x.ParticipantID, 64) || participants[x.ParticipantID] || !slices.Contains(p.Roles, x.Role) || !subset(x.Actions, p.Actions) || !subset(x.RepositoryIDs, p.RepositoryIDs) || !subset(x.TaskKinds, p.TaskKinds) {
			return Pilot{}, ErrInvalid
		}
		participants[x.ParticipantID] = true
	}
	if len(participants) == 0 {
		return Pilot{}, ErrInvalid
	}
	p.ID = id()
	p.Version = 1
	p.CreatedAt = now
	p.UpdatedAt = now
	p.Sessions = []Session{}
	p.Feedback = []Feedback{}
	return p, write(s.path(p.ID), p)
}
func (s *Store) Get(v string) (Pilot, error) { return s.get(v) }
func (s *Store) List(repo, pull string) ([]Pilot, error) {
	entries, e := os.ReadDir(s.root)
	if e != nil {
		return nil, e
	}
	out := []Pilot{}
	for _, x := range entries {
		if x.IsDir() || !strings.HasSuffix(x.Name(), ".json") {
			continue
		}
		p, e := s.get(strings.TrimSuffix(x.Name(), ".json"))
		if e != nil {
			return nil, e
		}
		if p.RepositoryID == repo && p.PullRequestID == pull {
			out = append(out, p)
		}
	}
	return out, nil
}
func usage(p Pilot) Budget {
	var u Budget
	for _, s := range p.Sessions {
		for _, e := range s.Events {
			if e.Kind == "result" {
				u.MaxActions++
				u.MaxCost += e.Cost
				u.MaxMinutes += e.Minutes
			}
		}
	}
	return u
}
func (s *Store) Effective(p Pilot, participant string, candidateChanged, accessRevoked bool) Access {
	now := s.now()
	reasons := []string{}
	var invite *Invitation
	for i := range p.Invitations {
		if p.Invitations[i].ParticipantID == participant {
			invite = &p.Invitations[i]
			break
		}
	}
	u := usage(p)
	if invite == nil {
		reasons = append(reasons, "not_invited")
	} else if invite.ConsentedAt == nil {
		reasons = append(reasons, "consent_pending")
	} else if invite.RevokedAt != nil {
		reasons = append(reasons, "consent_revoked")
	}
	if now.Before(p.StartsAt) {
		reasons = append(reasons, "not_started")
	}
	if !now.Before(p.ExpiresAt) {
		reasons = append(reasons, "expired")
	}
	if p.Paused {
		reasons = append(reasons, p.PauseReason)
	}
	if candidateChanged {
		reasons = append(reasons, "candidate_changed")
	}
	if accessRevoked {
		reasons = append(reasons, "repository_access_revoked")
	}
	if u.MaxActions >= p.Budget.MaxActions || u.MaxMinutes >= p.Budget.MaxMinutes || u.MaxCost >= p.Budget.MaxCost {
		reasons = append(reasons, "budget_exhausted")
	}
	a := Access{Status: "active", PauseReasons: reasons, Budget: p.Budget, Used: u, Remaining: Budget{p.Budget.MaxMinutes - u.MaxMinutes, p.Budget.MaxActions - u.MaxActions, p.Budget.MaxCost - u.MaxCost}, AuthorityNote: "Pilot authority is read and draft-only; output cannot merge, deploy, disclose, publish, or mutate authoritative resources."}
	if invite != nil {
		a.Actions = invite.Actions
		a.RepositoryIDs = invite.RepositoryIDs
		a.TaskKinds = invite.TaskKinds
	}
	if len(reasons) > 0 {
		a.Status = "paused"
	}
	return a
}
func (s *Store) mutate(pid string, expected int, fn func(*Pilot) error) (Pilot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, e := s.get(pid)
	if e != nil {
		return p, e
	}
	if p.Version != expected {
		return p, ErrConflict
	}
	if e = fn(&p); e != nil {
		return Pilot{}, e
	}
	p.Version++
	p.UpdatedAt = s.now()
	return p, write(s.path(pid), p)
}
func (s *Store) Consent(pid, participant string, expected int, revoke bool) (Pilot, error) {
	return s.mutate(pid, expected, func(p *Pilot) error {
		for i := range p.Invitations {
			x := &p.Invitations[i]
			if x.ParticipantID == participant {
				now := s.now()
				if revoke {
					x.RevokedAt = &now
					p.Paused = true
					p.PauseReason = "consent_revoked"
				} else if x.RevokedAt == nil {
					x.ConsentedAt = &now
				}
				return nil
			}
		}
		return ErrDenied
	})
}
func (s *Store) StartSession(pid, participant string, expected int, in Session, candidateChanged, accessRevoked bool) (Pilot, error) {
	return s.mutate(pid, expected, func(p *Pilot) error {
		a := s.Effective(*p, participant, candidateChanged, accessRevoked)
		if a.Status != "active" || !slices.Contains(a.RepositoryIDs, in.RepositoryID) || !slices.Contains(a.TaskKinds, in.TaskKind) || !valid(in.TaskID, 160) || !valid(in.ExpectedOutcome, 2000) {
			return ErrDenied
		}
		in.ID = id()
		in.ParticipantID = participant
		in.Status = "running"
		in.CreatedAt = s.now()
		in.Events = []SessionEvent{}
		p.Sessions = append(p.Sessions, in)
		return nil
	})
}
func (s *Store) AppendEvent(pid, participant string, expected int, sessionID string, e SessionEvent, candidateChanged, accessRevoked bool) (Pilot, error) {
	return s.mutate(pid, expected, func(p *Pilot) error {
		a := s.Effective(*p, participant, candidateChanged, accessRevoked)
		if a.Status != "active" && e.Kind != "stop" {
			return ErrDenied
		}
		if !slices.Contains([]string{"guidance", "stop", "result", "policy_denial", "escalation"}, e.Kind) || !valid(e.Summary, 2000) || e.Cost < 0 || e.Minutes < 0 {
			return ErrInvalid
		}
		if e.Kind == "result" && !slices.Contains(a.Actions, e.Action) {
			e.Kind = "policy_denial"
			e.Summary = "authoritative or ungranted action denied by pilot policy"
			e.Cost = 0
			e.Minutes = 0
		}
		for i := range p.Sessions {
			if p.Sessions[i].ID == sessionID && p.Sessions[i].ParticipantID == participant {
				e.ID = id()
				e.ActorID = participant
				e.CreatedAt = s.now()
				p.Sessions[i].Events = append(p.Sessions[i].Events, e)
				if e.Kind == "stop" {
					p.Sessions[i].Status = "stopped"
				}
				if e.Unsafe {
					p.Paused = true
					p.PauseReason = "unsafe_behavior"
					p.Sessions[i].Status = "paused"
				}
				u := usage(*p)
				if u.MaxActions >= p.Budget.MaxActions || u.MaxMinutes >= p.Budget.MaxMinutes || u.MaxCost >= p.Budget.MaxCost {
					p.Paused = true
					p.PauseReason = "budget_exhausted"
					p.Sessions[i].Status = "paused"
				}
				return nil
			}
		}
		return ErrNotFound
	})
}
func (s *Store) AddFeedback(pid, participant string, expected int, f Feedback) (Pilot, error) {
	return s.mutate(pid, expected, func(p *Pilot) error {
		invited := false
		for _, x := range p.Invitations {
			if x.ParticipantID == participant && x.ConsentedAt != nil && x.RevokedAt == nil {
				invited = true
			}
		}
		if !invited || f.CandidateRevision != p.CandidateRevision || !valid(f.Outcome, 2000) || !valid(f.ExpectedOutcome, 2000) || !valid(f.Correction, 4000) {
			return ErrDenied
		}
		f.ID = id()
		f.ParticipantID = participant
		f.CreatedAt = s.now()
		p.Feedback = append(p.Feedback, f)
		return nil
	})
}
