// Package organizations persists accountable groups, membership invitations,
// and accepted repository stewardship changes.
package organizations

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	ErrNotFound = errors.New("organization not found")
	ErrInvalid  = errors.New("invalid organization")
	ErrConflict = errors.New("organization state changed")
)

type Member struct {
	UserID   string    `json:"user_id"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
}

type Invitation struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	InvitedBy string    `json:"invited_by"`
	CreatedAt time.Time `json:"created_at"`
}

type Transfer struct {
	ID           string     `json:"id"`
	RepositoryID string     `json:"repository_id"`
	FromOwnerID  string     `json:"from_owner_id"`
	RequestedBy  string     `json:"requested_by"`
	Status       string     `json:"status"`
	RequestedAt  time.Time  `json:"requested_at"`
	AcceptedBy   string     `json:"accepted_by,omitempty"`
	AcceptedAt   *time.Time `json:"accepted_at,omitempty"`
}

type TeamMember struct {
	UserID  string    `json:"user_id"`
	Role    string    `json:"role"`
	AddedBy string    `json:"added_by"`
	AddedAt time.Time `json:"added_at"`
}

type Responsibility struct {
	ID           string    `json:"id"`
	RepositoryID string    `json:"repository_id"`
	Area         string    `json:"area"`
	Description  string    `json:"description,omitempty"`
	AddedBy      string    `json:"added_by"`
	AddedAt      time.Time `json:"added_at"`
}

type Team struct {
	ID               string           `json:"id"`
	Name             string           `json:"name"`
	Slug             string           `json:"slug"`
	Description      string           `json:"description,omitempty"`
	ParentID         string           `json:"parent_id,omitempty"`
	Visibility       string           `json:"visibility"`
	Version          int              `json:"version"`
	CreatedBy        string           `json:"created_by"`
	CreatedAt        time.Time        `json:"created_at"`
	Members          []TeamMember     `json:"members"`
	Responsibilities []Responsibility `json:"responsibilities"`
}

type Agent struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Slug         string    `json:"slug"`
	Description  string    `json:"description,omitempty"`
	Visibility   string    `json:"visibility"`
	Capabilities []string  `json:"capabilities"`
	OperatorIDs  []string  `json:"operator_ids"`
	TeamIDs      []string  `json:"team_ids"`
	Version      int       `json:"version"`
	RegisteredBy string    `json:"registered_by"`
	RegisteredAt time.Time `json:"registered_at"`
}

type ResourceScope struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type AccessException struct {
	Resource ResourceScope `json:"resource"`
	Reason   string        `json:"reason"`
}

type DerivedCredential struct {
	ID         string    `json:"id"`
	OperatorID string    `json:"operator_id"`
	CreatedAt  time.Time `json:"created_at"`
}

type AccessGrant struct {
	ID                 string              `json:"id"`
	PrincipalType      string              `json:"principal_type"`
	PrincipalID        string              `json:"principal_id"`
	Role               string              `json:"role"`
	Resources          []ResourceScope     `json:"resources"`
	Exceptions         []AccessException   `json:"exceptions"`
	Reason             string              `json:"reason"`
	ExpiresAt          *time.Time          `json:"expires_at,omitempty"`
	Version            int                 `json:"version"`
	GrantedBy          string              `json:"granted_by"`
	GrantedAt          time.Time           `json:"granted_at"`
	RevokedBy          string              `json:"revoked_by,omitempty"`
	RevokedAt          *time.Time          `json:"revoked_at,omitempty"`
	DerivedCredentials []DerivedCredential `json:"derived_credentials"`
}

type AccessRequest struct {
	ID            string            `json:"id"`
	RequesterID   string            `json:"requester_id"`
	PrincipalType string            `json:"principal_type"`
	PrincipalID   string            `json:"principal_id"`
	Role          string            `json:"role"`
	Resources     []ResourceScope   `json:"resources"`
	Exceptions    []AccessException `json:"exceptions"`
	Reason        string            `json:"reason"`
	ExpiresAt     *time.Time        `json:"expires_at,omitempty"`
	Status        string            `json:"status"`
	CreatedAt     time.Time         `json:"created_at"`
	DecidedBy     string            `json:"decided_by,omitempty"`
	DecidedAt     *time.Time        `json:"decided_at,omitempty"`
	GrantID       string            `json:"grant_id,omitempty"`
}

type Event struct {
	ID        string         `json:"id"`
	Action    string         `json:"action"`
	ActorID   string         `json:"actor_id"`
	TargetID  string         `json:"target_id,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	Details   map[string]any `json:"details,omitempty"`
}

type Organization struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Slug           string          `json:"slug"`
	Description    string          `json:"description,omitempty"`
	CreatedBy      string          `json:"created_by"`
	CreatedAt      time.Time       `json:"created_at"`
	Members        []Member        `json:"members"`
	Invitations    []Invitation    `json:"invitations"`
	Transfers      []Transfer      `json:"transfers"`
	Teams          []Team          `json:"teams"`
	Agents         []Agent         `json:"agents"`
	AccessGrants   []AccessGrant   `json:"access_grants"`
	AccessRequests []AccessRequest `json:"access_requests"`
	Events         []Event         `json:"events"`
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
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &Store{root: abs, now: func() time.Time { return time.Now().UTC() }}, nil
}

func validID(v string) bool {
	if len(v) != 32 {
		return false
	}
	_, err := hex.DecodeString(v)
	return err == nil
}
func newID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
func clean(v string, max int) (string, bool) {
	v = strings.TrimSpace(v)
	return v, v != "" && len([]rune(v)) <= max && !strings.ContainsAny(v, "\x00\r\n")
}

func (s *Store) locked(fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return fn()
}

func (s *Store) Create(name, slug, description, actor string) (Organization, error) {
	name, ok := clean(name, 100)
	if !ok || !validID(actor) {
		return Organization{}, ErrInvalid
	}
	slug = strings.ToLower(strings.TrimSpace(slug))
	if slug == "" || len(slug) > 60 {
		return Organization{}, ErrInvalid
	}
	for _, r := range slug {
		if !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '-' {
			return Organization{}, ErrInvalid
		}
	}
	if len(description) > 1000 {
		return Organization{}, ErrInvalid
	}
	var created Organization
	err := s.locked(func() error {
		items, err := s.list()
		if err != nil {
			return err
		}
		for _, item := range items {
			if item.Slug == slug {
				return ErrConflict
			}
		}
		id, err := newID()
		if err != nil {
			return err
		}
		now := s.now().Truncate(time.Microsecond)
		created = Organization{ID: id, Name: name, Slug: slug, Description: strings.TrimSpace(description), CreatedBy: actor, CreatedAt: now, Members: []Member{{UserID: actor, Role: "owner", JoinedAt: now}}, Invitations: []Invitation{}, Transfers: []Transfer{}, Teams: []Team{}, Agents: []Agent{}, AccessGrants: []AccessGrant{}, AccessRequests: []AccessRequest{}, Events: []Event{}}
		if err := s.event(&created, "organization.created", actor, id, nil); err != nil {
			return err
		}
		return s.write(created)
	})
	return created, err
}

func (s *Store) Get(id string) (Organization, error) {
	if !validID(id) {
		return Organization{}, ErrNotFound
	}
	var v Organization
	data, err := os.ReadFile(filepath.Join(s.root, id+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return v, ErrNotFound
	}
	if err != nil || json.Unmarshal(data, &v) != nil || v.ID != id {
		return v, ErrNotFound
	}
	if v.Teams == nil {
		v.Teams = []Team{}
	}
	if v.Agents == nil {
		v.Agents = []Agent{}
	}
	if v.AccessGrants == nil {
		v.AccessGrants = []AccessGrant{}
	}
	if v.AccessRequests == nil {
		v.AccessRequests = []AccessRequest{}
	}
	if v.Events == nil {
		v.Events = []Event{}
	}
	return v, nil
}
func (s *Store) ListFor(user string) ([]Organization, error) {
	items, err := s.list()
	if err != nil {
		return nil, err
	}
	out := []Organization{}
	for _, v := range items {
		if HasRole(v, user, "") || invited(v, user) {
			out = append(out, v)
		}
	}
	return out, nil
}
func (s *Store) list() ([]Organization, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	out := []Organization{}
	for _, e := range entries {
		id, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok || !validID(id) {
			continue
		}
		v, err := s.Get(id)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	return out, nil
}
func HasRole(v Organization, user, role string) bool {
	for _, m := range v.Members {
		if m.UserID == user && (role == "" || m.Role == role) {
			return true
		}
	}
	return false
}
func invited(v Organization, user string) bool {
	for _, i := range v.Invitations {
		if i.UserID == user {
			return true
		}
	}
	return false
}

func (s *Store) mutate(id string, fn func(*Organization) error) (Organization, error) {
	var out Organization
	err := s.locked(func() error {
		v, err := s.Get(id)
		if err != nil {
			return err
		}
		if err = fn(&v); err != nil {
			return err
		}
		if err = s.write(v); err != nil {
			return err
		}
		out = v
		return nil
	})
	return out, err
}
func (s *Store) Invite(id, actor, user string) (Organization, error) {
	if !validID(user) || actor == user {
		return Organization{}, ErrInvalid
	}
	return s.mutate(id, func(v *Organization) error {
		if !HasRole(*v, actor, "owner") {
			return ErrNotFound
		}
		if HasRole(*v, user, "") {
			return ErrConflict
		}
		if invited(*v, user) {
			return nil
		}
		iid, e := newID()
		if e != nil {
			return e
		}
		v.Invitations = append(v.Invitations, Invitation{ID: iid, UserID: user, InvitedBy: actor, CreatedAt: s.now().Truncate(time.Microsecond)})
		return s.event(v, "member.invited", actor, user, nil)
	})
}
func (s *Store) AcceptInvitation(id, invitationID, actor string) (Organization, error) {
	return s.mutate(id, func(v *Organization) error {
		if HasRole(*v, actor, "") {
			return nil
		}
		for i, x := range v.Invitations {
			if x.ID == invitationID && x.UserID == actor {
				v.Invitations = append(v.Invitations[:i], v.Invitations[i+1:]...)
				v.Members = append(v.Members, Member{UserID: actor, Role: "member", JoinedAt: s.now().Truncate(time.Microsecond)})
				return s.event(v, "member.joined", actor, actor, nil)
			}
		}
		return ErrNotFound
	})
}
func (s *Store) RemoveMember(id, actor, user string) (Organization, error) {
	return s.mutate(id, func(v *Organization) error {
		if !HasRole(*v, actor, "owner") {
			return ErrNotFound
		}
		for i, m := range v.Members {
			if m.UserID == user {
				if m.Role == "owner" {
					return ErrConflict
				}
				v.Members = append(v.Members[:i], v.Members[i+1:]...)
				for ti := range v.Teams {
					kept := v.Teams[ti].Members[:0]
					for _, membership := range v.Teams[ti].Members {
						if membership.UserID != user {
							kept = append(kept, membership)
						}
					}
					if len(kept) != len(v.Teams[ti].Members) {
						v.Teams[ti].Members = kept
						v.Teams[ti].Version++
					}
				}
				agents := v.Agents[:0]
				for _, agent := range v.Agents {
					priorOperators := len(agent.OperatorIDs)
					operators := agent.OperatorIDs[:0]
					for _, operator := range agent.OperatorIDs {
						if operator != user {
							operators = append(operators, operator)
						}
					}
					agent.OperatorIDs = operators
					if len(operators) > 0 {
						if len(operators) != priorOperators {
							agent.Version++
						}
						agents = append(agents, agent)
					}
				}
				v.Agents = agents
				return s.event(v, "member.removed", actor, user, nil)
			}
		}
		return nil
	})
}

func validSlug(v string) bool {
	v = strings.TrimSpace(strings.ToLower(v))
	if v == "" || len(v) > 60 {
		return false
	}
	for _, r := range v {
		if !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '-' {
			return false
		}
	}
	return true
}

func validVisibility(v string) bool { return v == "public" || v == "organization" }

func (s *Store) event(v *Organization, action, actor, target string, details map[string]any) error {
	id, err := newID()
	if err != nil {
		return err
	}
	v.Events = append(v.Events, Event{ID: id, Action: action, ActorID: actor, TargetID: target, CreatedAt: s.now().Truncate(time.Microsecond), Details: details})
	return nil
}

func teamIndex(v *Organization, id string) int {
	for i := range v.Teams {
		if v.Teams[i].ID == id {
			return i
		}
	}
	return -1
}
func agentIndex(v *Organization, id string) int {
	for i := range v.Agents {
		if v.Agents[i].ID == id {
			return i
		}
	}
	return -1
}

func (s *Store) CreateTeam(id, actor, name, slug, description, parentID, visibility string) (Organization, error) {
	name, ok := clean(name, 100)
	if !ok || !validSlug(slug) || !validVisibility(visibility) || len(description) > 1000 {
		return Organization{}, ErrInvalid
	}
	return s.mutate(id, func(v *Organization) error {
		if !HasRole(*v, actor, "owner") {
			return ErrNotFound
		}
		for _, t := range v.Teams {
			if t.Slug == strings.ToLower(strings.TrimSpace(slug)) {
				return ErrConflict
			}
		}
		if parentID != "" && teamIndex(v, parentID) < 0 {
			return ErrInvalid
		}
		tid, err := newID()
		if err != nil {
			return err
		}
		now := s.now().Truncate(time.Microsecond)
		v.Teams = append(v.Teams, Team{ID: tid, Name: name, Slug: strings.ToLower(strings.TrimSpace(slug)), Description: strings.TrimSpace(description), ParentID: parentID, Visibility: visibility, Version: 1, CreatedBy: actor, CreatedAt: now, Members: []TeamMember{}, Responsibilities: []Responsibility{}})
		return s.event(v, "team.created", actor, tid, map[string]any{"parent_id": parentID})
	})
}

func (s *Store) AddTeamMember(id, teamID, actor, user, role string, expected int) (Organization, error) {
	if !validID(user) || (role != "member" && role != "maintainer") {
		return Organization{}, ErrInvalid
	}
	return s.mutate(id, func(v *Organization) error {
		if !HasRole(*v, actor, "owner") || !HasRole(*v, user, "") {
			return ErrNotFound
		}
		i := teamIndex(v, teamID)
		if i < 0 {
			return ErrNotFound
		}
		t := &v.Teams[i]
		if t.Version != expected {
			return ErrConflict
		}
		for j := range t.Members {
			if t.Members[j].UserID == user {
				t.Members[j].Role = role
				t.Members[j].AddedBy = actor
				t.Members[j].AddedAt = s.now().Truncate(time.Microsecond)
				t.Version++
				return s.event(v, "team.member.updated", actor, user, map[string]any{"team_id": teamID, "role": role})
			}
		}
		t.Members = append(t.Members, TeamMember{UserID: user, Role: role, AddedBy: actor, AddedAt: s.now().Truncate(time.Microsecond)})
		t.Version++
		return s.event(v, "team.member.added", actor, user, map[string]any{"team_id": teamID, "role": role})
	})
}

func (s *Store) RemoveTeamMember(id, teamID, actor, user string, expected int) (Organization, error) {
	return s.mutate(id, func(v *Organization) error {
		if !HasRole(*v, actor, "owner") {
			return ErrNotFound
		}
		i := teamIndex(v, teamID)
		if i < 0 {
			return ErrNotFound
		}
		t := &v.Teams[i]
		if t.Version != expected {
			return ErrConflict
		}
		for j, m := range t.Members {
			if m.UserID == user {
				t.Members = append(t.Members[:j], t.Members[j+1:]...)
				t.Version++
				return s.event(v, "team.member.removed", actor, user, map[string]any{"team_id": teamID})
			}
		}
		return nil
	})
}

func (s *Store) AddResponsibility(id, teamID, actor, repositoryID, area, description string, expected int, coordinate func(func() error) error) (Organization, error) {
	area, ok := clean(area, 100)
	if !ok || !validID(repositoryID) || len(description) > 1000 {
		return Organization{}, ErrInvalid
	}
	var out Organization
	err := s.locked(func() error {
		v, err := s.Get(id)
		if err != nil {
			return err
		}
		if !HasRole(v, actor, "owner") {
			return ErrNotFound
		}
		i := teamIndex(&v, teamID)
		if i < 0 {
			return ErrNotFound
		}
		t := &v.Teams[i]
		if t.Version != expected {
			return ErrConflict
		}
		if coordinate == nil {
			return ErrInvalid
		}
		return coordinate(func() error {
			rid, e := newID()
			if e != nil {
				return e
			}
			t.Responsibilities = append(t.Responsibilities, Responsibility{ID: rid, RepositoryID: repositoryID, Area: area, Description: strings.TrimSpace(description), AddedBy: actor, AddedAt: s.now().Truncate(time.Microsecond)})
			t.Version++
			if e = s.event(&v, "team.responsibility.added", actor, rid, map[string]any{"team_id": teamID, "repository_id": repositoryID, "area": area}); e != nil {
				return e
			}
			if e = s.write(v); e != nil {
				return e
			}
			out = v
			return nil
		})
	})
	return out, err
}

func normalizeList(values []string, validate func(string) bool) ([]string, bool) {
	out := []string{}
	seen := map[string]bool{}
	for _, x := range values {
		x = strings.TrimSpace(x)
		if !validate(x) {
			return nil, false
		}
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return out, true
}

func (s *Store) RegisterAgent(id, actor, name, slug, description, visibility string, capabilities, operators, teamIDs []string) (Organization, error) {
	name, ok := clean(name, 100)
	caps, cok := normalizeList(capabilities, func(x string) bool { _, yes := clean(x, 100); return yes })
	ops, ook := normalizeList(operators, validID)
	teams, tok := normalizeList(teamIDs, validID)
	if !ok || !cok || !ook || !tok || len(caps) == 0 || len(ops) == 0 || !validSlug(slug) || !validVisibility(visibility) || len(description) > 1000 {
		return Organization{}, ErrInvalid
	}
	return s.mutate(id, func(v *Organization) error {
		if !HasRole(*v, actor, "owner") {
			return ErrNotFound
		}
		for _, a := range v.Agents {
			if a.Slug == strings.ToLower(strings.TrimSpace(slug)) {
				return ErrConflict
			}
		}
		for _, op := range ops {
			if !HasRole(*v, op, "") {
				return ErrInvalid
			}
		}
		for _, tid := range teams {
			if teamIndex(v, tid) < 0 {
				return ErrInvalid
			}
		}
		aid, e := newID()
		if e != nil {
			return e
		}
		v.Agents = append(v.Agents, Agent{ID: aid, Name: name, Slug: strings.ToLower(strings.TrimSpace(slug)), Description: strings.TrimSpace(description), Visibility: visibility, Capabilities: caps, OperatorIDs: ops, TeamIDs: teams, Version: 1, RegisteredBy: actor, RegisteredAt: s.now().Truncate(time.Microsecond)})
		return s.event(v, "agent.registered", actor, aid, map[string]any{"operators": ops, "capabilities": caps})
	})
}

type EffectiveMember struct {
	UserID       string `json:"user_id"`
	Role         string `json:"role"`
	Reason       string `json:"reason"`
	SourceTeamID string `json:"source_team_id"`
}
type TeamDirectory struct {
	Team             Team              `json:"team"`
	EffectiveMembers []EffectiveMember `json:"effective_members"`
}
type Directory struct {
	OrganizationID string          `json:"organization_id"`
	Name           string          `json:"name"`
	Slug           string          `json:"slug"`
	Teams          []TeamDirectory `json:"teams"`
	Agents         []Agent         `json:"agents"`
	Events         []Event         `json:"events,omitempty"`
}

func ProjectDirectory(v Organization, member bool, publicRepositories map[string]bool) Directory {
	visible := map[string]bool{}
	for _, t := range v.Teams {
		if member || t.Visibility == "public" {
			visible[t.ID] = true
		}
	}
	out := Directory{OrganizationID: v.ID, Name: v.Name, Slug: v.Slug, Teams: []TeamDirectory{}, Agents: []Agent{}}
	for _, t := range v.Teams {
		if !visible[t.ID] {
			continue
		}
		copy := t
		if !member && !visible[copy.ParentID] {
			copy.ParentID = ""
		}
		if !member {
			rs := []Responsibility{}
			for _, r := range copy.Responsibilities {
				if publicRepositories[r.RepositoryID] {
					rs = append(rs, r)
				}
			}
			copy.Responsibilities = rs
		} else {
			rs := []Responsibility{}
			for _, r := range copy.Responsibilities {
				if publicRepositories[r.RepositoryID] {
					rs = append(rs, r)
				}
			}
			copy.Responsibilities = rs
		}
		effective := []EffectiveMember{}
		seen := map[string]bool{}
		var add func(string, string)
		add = func(id, reason string) {
			for _, candidate := range v.Teams {
				if candidate.ID == id {
					for _, m := range candidate.Members {
						if !seen[m.UserID] {
							seen[m.UserID] = true
							effective = append(effective, EffectiveMember{UserID: m.UserID, Role: m.Role, Reason: reason, SourceTeamID: candidate.ID})
						}
					}
				}
			}
			for _, child := range v.Teams {
				if child.ParentID == id && (member || child.Visibility == "public") {
					add(child.ID, "nested team "+child.Name)
				}
			}
		}
		add(t.ID, "direct membership")
		out.Teams = append(out.Teams, TeamDirectory{Team: copy, EffectiveMembers: effective})
	}
	for _, a := range v.Agents {
		if member || a.Visibility == "public" {
			if !member {
				filtered := []string{}
				for _, id := range a.TeamIDs {
					if visible[id] {
						filtered = append(filtered, id)
					}
				}
				a.TeamIDs = filtered
			}
			out.Agents = append(out.Agents, a)
		}
	}
	if member {
		out.Events = v.Events
	}
	return out
}

func validPrincipal(v *Organization, kind, id string) bool {
	if kind == "team" {
		return teamIndex(v, id) >= 0
	}
	if kind == "agent" {
		return agentIndex(v, id) >= 0
	}
	return false
}
func validRole(role string) bool {
	return role == "viewer" || role == "contributor" || role == "maintainer" || role == "operator"
}
func validResourceKind(kind string) bool {
	return kind == "repository" || kind == "package" || kind == "environment" || kind == "collaboration"
}
func validateAccess(role, reason string, resources []ResourceScope, exceptions []AccessException, expires *time.Time, now time.Time) bool {
	if !validRole(role) || len(resources) == 0 || len(resources) > 100 || len(exceptions) > 100 || len(reason) > 1000 || (expires != nil && !expires.After(now)) {
		return false
	}
	seen := map[string]bool{}
	for _, r := range resources {
		if !validResourceKind(r.Kind) || !validID(r.ID) || seen[r.Kind+":"+r.ID] {
			return false
		}
		seen[r.Kind+":"+r.ID] = true
	}
	for _, x := range exceptions {
		if !validResourceKind(x.Resource.Kind) || !validID(x.Resource.ID) || !seen[x.Resource.Kind+":"+x.Resource.ID] {
			return false
		}
		if _, ok := clean(x.Reason, 500); !ok {
			return false
		}
	}
	return true
}

func (s *Store) CreateAccessRequest(id, actor, principalType, principalID, role, reason string, resources []ResourceScope, exceptions []AccessException, expires *time.Time) (Organization, error) {
	return s.mutate(id, func(v *Organization) error {
		if !HasRole(*v, actor, "") || !validPrincipal(v, principalType, principalID) || !validateAccess(role, reason, resources, exceptions, expires, s.now()) {
			return ErrInvalid
		}
		if principalType == "team" {
			found := false
			for _, m := range v.Teams[teamIndex(v, principalID)].Members {
				if m.UserID == actor {
					found = true
				}
			}
			if !found && !HasRole(*v, actor, "owner") {
				return ErrNotFound
			}
		}
		if principalType == "agent" {
			found := false
			for _, op := range v.Agents[agentIndex(v, principalID)].OperatorIDs {
				if op == actor {
					found = true
				}
			}
			if !found && !HasRole(*v, actor, "owner") {
				return ErrNotFound
			}
		}
		rid, err := newID()
		if err != nil {
			return err
		}
		now := s.now().Truncate(time.Microsecond)
		v.AccessRequests = append(v.AccessRequests, AccessRequest{ID: rid, RequesterID: actor, PrincipalType: principalType, PrincipalID: principalID, Role: role, Resources: resources, Exceptions: exceptions, Reason: strings.TrimSpace(reason), ExpiresAt: expires, Status: "pending", CreatedAt: now})
		return s.event(v, "access.requested", actor, rid, map[string]any{"principal_type": principalType, "principal_id": principalID, "role": role})
	})
}

func (s *Store) DecideAccessRequest(id, requestID, actor, decision string, validate func(AccessRequest) error) (Organization, error) {
	return s.mutate(id, func(v *Organization) error {
		if !HasRole(*v, actor, "owner") || (decision != "approve" && decision != "deny") {
			return ErrNotFound
		}
		for i := range v.AccessRequests {
			r := &v.AccessRequests[i]
			if r.ID != requestID {
				continue
			}
			if r.Status != "pending" {
				return ErrConflict
			}
			now := s.now().Truncate(time.Microsecond)
			r.DecidedBy, r.DecidedAt = actor, &now
			if decision == "deny" {
				r.Status = "denied"
				return s.event(v, "access.request.denied", actor, r.ID, nil)
			}
			if r.ExpiresAt != nil && !r.ExpiresAt.After(now) {
				return ErrConflict
			}
			if !validPrincipal(v, r.PrincipalType, r.PrincipalID) {
				return ErrConflict
			}
			if validate == nil {
				return ErrInvalid
			}
			if err := validate(*r); err != nil {
				return err
			}
			gid, err := newID()
			if err != nil {
				return err
			}
			r.Status, r.GrantID = "approved", gid
			v.AccessGrants = append(v.AccessGrants, AccessGrant{ID: gid, PrincipalType: r.PrincipalType, PrincipalID: r.PrincipalID, Role: r.Role, Resources: r.Resources, Exceptions: r.Exceptions, Reason: r.Reason, ExpiresAt: r.ExpiresAt, Version: 1, GrantedBy: actor, GrantedAt: now, DerivedCredentials: []DerivedCredential{}})
			return s.event(v, "access.granted", actor, gid, map[string]any{"request_id": r.ID, "role": r.Role})
		}
		return ErrNotFound
	})
}

func (s *Store) RevokeAccessGrant(id, grantID, actor string, expected int, revoke func(DerivedCredential) error) (Organization, error) {
	return s.mutate(id, func(v *Organization) error {
		if !HasRole(*v, actor, "owner") {
			return ErrNotFound
		}
		for i := range v.AccessGrants {
			g := &v.AccessGrants[i]
			if g.ID != grantID {
				continue
			}
			if g.Version != expected {
				return ErrConflict
			}
			if g.RevokedAt != nil {
				return nil
			}
			for _, credential := range g.DerivedCredentials {
				if revoke != nil {
					if err := revoke(credential); err != nil {
						return err
					}
				}
			}
			now := s.now().Truncate(time.Microsecond)
			g.RevokedAt, g.RevokedBy, g.Version = &now, actor, g.Version+1
			return s.event(v, "access.revoked", actor, grantID, nil)
		}
		return ErrNotFound
	})
}

func resourceDenied(g AccessGrant, resource ResourceScope) bool {
	for _, x := range g.Exceptions {
		if x.Resource == resource {
			return true
		}
	}
	return false
}
func (s *Store) RecordDerivedCredential(id, grantID, agentID, operatorID, credentialID string, resource ResourceScope) (Organization, error) {
	return s.mutate(id, func(v *Organization) error {
		if !validID(credentialID) {
			return ErrInvalid
		}
		for i := range v.AccessGrants {
			g := &v.AccessGrants[i]
			if g.ID != grantID {
				continue
			}
			now := s.now()
			if g.PrincipalType != "agent" || g.PrincipalID != agentID || g.RevokedAt != nil || (g.ExpiresAt != nil && !g.ExpiresAt.After(now)) || resourceDenied(*g, resource) {
				return ErrNotFound
			}
			allowed := false
			for _, x := range g.Resources {
				if x == resource {
					allowed = true
				}
			}
			if !allowed {
				return ErrNotFound
			}
			ai := agentIndex(v, agentID)
			if ai < 0 || !slices.Contains(v.Agents[ai].OperatorIDs, operatorID) {
				return ErrNotFound
			}
			g.DerivedCredentials = append(g.DerivedCredentials, DerivedCredential{ID: credentialID, OperatorID: operatorID, CreatedAt: now.Truncate(time.Microsecond)})
			return s.event(v, "access.credential.issued", operatorID, credentialID, map[string]any{"grant_id": grantID, "resource_kind": resource.Kind, "resource_id": resource.ID})
		}
		return ErrNotFound
	})
}
func (s *Store) RequestTransfer(id, repositoryID, owner string) (Organization, error) {
	if !validID(repositoryID) {
		return Organization{}, ErrInvalid
	}
	return s.mutate(id, func(v *Organization) error {
		for _, t := range v.Transfers {
			if t.RepositoryID == repositoryID && t.Status == "pending" {
				if t.FromOwnerID == owner {
					return nil
				}
				return ErrConflict
			}
		}
		tid, e := newID()
		if e != nil {
			return e
		}
		v.Transfers = append(v.Transfers, Transfer{ID: tid, RepositoryID: repositoryID, FromOwnerID: owner, RequestedBy: owner, Status: "pending", RequestedAt: s.now().Truncate(time.Microsecond)})
		return nil
	})
}
func (s *Store) AcceptTransfer(id, transferID, actor string, apply func(Transfer, Organization) error) (Organization, error) {
	return s.mutate(id, func(v *Organization) error {
		if !HasRole(*v, actor, "owner") {
			return ErrNotFound
		}
		for i := range v.Transfers {
			t := &v.Transfers[i]
			if t.ID == transferID {
				if t.Status == "accepted" {
					return nil
				}
				if apply != nil {
					if e := apply(*t, *v); e != nil {
						return e
					}
				}
				now := s.now().Truncate(time.Microsecond)
				t.Status = "accepted"
				t.AcceptedBy = actor
				t.AcceptedAt = &now
				return nil
			}
		}
		return ErrNotFound
	})
}

func (s *Store) write(v Organization) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.root, ".writing-")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(append(data, '\n'))
	}
	if err == nil {
		err = tmp.Sync()
	}
	if e := tmp.Close(); err == nil {
		err = e
	}
	if err == nil {
		err = os.Rename(name, filepath.Join(s.root, v.ID+".json"))
	}
	return err
}
