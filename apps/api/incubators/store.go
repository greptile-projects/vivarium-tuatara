// Package incubators retains collaboration context before a repository exists.
package incubators

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
	ErrNotFound = errors.New("incubator not found")
	ErrInvalid  = errors.New("invalid incubator")
	ErrConflict = errors.New("incubator version changed")
)

type Source struct {
	Kind         string `json:"kind"`
	RepositoryID string `json:"repository_id,omitempty"`
	ResourceID   string `json:"resource_id,omitempty"`
	Label        string `json:"label"`
	Resolution   string `json:"resolution"`
	Detail       string `json:"detail,omitempty"`
}
type DecisionRight struct {
	Kind         string   `json:"kind"`
	Decision     string   `json:"decision"`
	PrincipalIDs []string `json:"principal_ids"`
	Rule         string   `json:"rule"`
}
type Invitation struct {
	ID             string     `json:"id"`
	PrincipalType  string     `json:"principal_type"`
	PrincipalID    string     `json:"principal_id"`
	OrganizationID string     `json:"organization_id,omitempty"`
	Role           string     `json:"role"`
	Status         string     `json:"status"`
	InvitedBy      string     `json:"invited_by"`
	InvitedAt      time.Time  `json:"invited_at"`
	RespondedAt    *time.Time `json:"responded_at,omitempty"`
}
type Event struct {
	ID           string    `json:"id"`
	Kind         string    `json:"kind"`
	DecisionKind string    `json:"decision_kind,omitempty"`
	Body         string    `json:"body"`
	Visibility   string    `json:"visibility"`
	ActorType    string    `json:"actor_type"`
	ActorID      string    `json:"actor_id"`
	CreatedAt    time.Time `json:"created_at"`
}
type Duplicate struct {
	IncubatorID string `json:"incubator_id"`
	Title       string `json:"title"`
	Reason      string `json:"reason"`
}
type Incubator struct {
	ID                  string          `json:"id"`
	Version             int             `json:"version"`
	Title               string          `json:"title"`
	Audience            string          `json:"audience"`
	Problem             string          `json:"problem"`
	DesiredOutcome      string          `json:"desired_outcome"`
	Constraints         []string        `json:"constraints"`
	SuccessMeasures     []string        `json:"success_measures"`
	SponsorIDs          []string        `json:"sponsor_ids"`
	DecisionRights      []DecisionRight `json:"decision_rights"`
	Visibility          string          `json:"visibility"`
	Source              Source          `json:"source"`
	CreatedBy           string          `json:"created_by"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
	Invitations         []Invitation    `json:"invitations"`
	Events              []Event         `json:"events"`
	PotentialDuplicates []Duplicate     `json:"potential_duplicates"`
	DurabilityUncertain bool            `json:"durability_uncertain"`
}
type Store struct {
	root    string
	mu      sync.Mutex
	now     func() time.Time
	syncDir func() error
}

func New(root string) (*Store, error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	s := &Store{root: root, now: time.Now}
	s.syncDir = func() error {
		d, e := os.Open(root)
		if e != nil {
			return e
		}
		defer d.Close()
		return d.Sync()
	}
	return s, nil
}
func uid() string               { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func text(v string, n int) bool { l := len(strings.TrimSpace(v)); return l > 0 && l <= n }
func validList(v []string, max int) bool {
	if len(v) == 0 || len(v) > max {
		return false
	}
	seen := map[string]bool{}
	for _, x := range v {
		x = strings.TrimSpace(x)
		if !text(x, 2000) || seen[x] {
			return false
		}
		seen[x] = true
	}
	return true
}
func validInput(x Incubator) bool {
	if !text(x.Title, 200) || !text(x.Audience, 5000) || !text(x.Problem, 10000) || !text(x.DesiredOutcome, 10000) || !validList(x.Constraints, 30) || !validList(x.SuccessMeasures, 30) || !validList(x.SponsorIDs, 30) || len(x.DecisionRights) == 0 || len(x.DecisionRights) > 30 || !map[string]bool{"private": true, "participants": true, "public": true}[x.Visibility] || !map[string]bool{"feedback": true, "support_gap": true, "governed_proposal": true, "new_idea": true}[x.Source.Kind] || !text(x.Source.Label, 300) {
		return false
	}
	if x.Source.Kind == "new_idea" {
		if x.Source.RepositoryID != "" || x.Source.ResourceID != "" {
			return false
		}
	} else if !text(x.Source.RepositoryID, 100) || !text(x.Source.ResourceID, 200) {
		return false
	}
	kinds := map[string]bool{}
	for _, d := range x.DecisionRights {
		if !map[string]bool{"scope_change": true, "visibility_change": true, "project_update": true}[d.Kind] || kinds[d.Kind] || !text(d.Decision, 500) || !validList(d.PrincipalIDs, 30) || !map[string]bool{"owner": true, "consent": true, "majority": true, "consensus": true}[d.Rule] {
			return false
		}
		kinds[d.Kind] = true
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
func (s *Store) publish(x Incubator) (bool, error) {
	b, e := json.Marshal(x)
	if e != nil {
		return false, e
	}
	f, e := os.CreateTemp(s.root, ".incubator-*")
	if e != nil {
		return false, e
	}
	n := f.Name()
	defer os.Remove(n)
	_ = f.Chmod(0600)
	if _, e = f.Write(b); e == nil {
		e = f.Sync()
	}
	if ce := f.Close(); e == nil {
		e = ce
	}
	if e == nil {
		e = os.Rename(n, filepath.Join(s.root, x.ID+".json"))
	}
	renamed := e == nil
	if e == nil {
		e = s.syncDir()
	}
	return renamed, e
}
func (s *Store) write(x *Incubator) error {
	// Publish a conservative marker first. Once its directory entry is synced,
	// every later failure is a committed result, never an ordinary failed write.
	x.DurabilityUncertain = true
	renamed, e := s.publish(*x)
	if e != nil {
		if renamed {
			return nil
		}
		return e
	}
	x.DurabilityUncertain = false
	if renamed, e = s.publish(*x); e != nil {
		x.DurabilityUncertain = true
		_, _ = s.publish(*x)
		return nil
	}
	return nil
}
func (s *Store) read(id string) (Incubator, error) {
	if len(id) != 32 {
		return Incubator{}, ErrNotFound
	}
	b, e := os.ReadFile(filepath.Join(s.root, id+".json"))
	if os.IsNotExist(e) {
		return Incubator{}, ErrNotFound
	}
	var x Incubator
	if e == nil {
		e = json.Unmarshal(b, &x)
	}
	return x, e
}
func (s *Store) all() ([]Incubator, error) {
	files, e := filepath.Glob(filepath.Join(s.root, "*.json"))
	if e != nil {
		return nil, e
	}
	out := []Incubator{}
	for _, f := range files {
		b, e := os.ReadFile(f)
		if e != nil {
			return nil, e
		}
		var x Incubator
		if e = json.Unmarshal(b, &x); e != nil {
			return nil, e
		}
		out = append(out, x)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}
func visible(x Incubator, actor string) bool {
	if x.Visibility == "public" || x.CreatedBy == actor {
		return true
	}
	if x.Visibility == "private" {
		return false
	}
	for _, i := range x.Invitations {
		if i.PrincipalID == actor {
			return true
		}
	}
	return false
}
func participant(x Incubator, typ, id string) bool {
	if typ == "human" && x.CreatedBy == id {
		return true
	}
	for _, i := range x.Invitations {
		if i.PrincipalType == typ && i.PrincipalID == id && i.Status == "accepted" {
			return true
		}
	}
	return false
}
func decisionFor(x Incubator, kind string) (DecisionRight, bool) {
	for _, right := range x.DecisionRights {
		if right.Kind == kind {
			return right, true
		}
	}
	return DecisionRight{}, false
}
func principalForDecision(x Incubator, kind, actor string) bool {
	right, ok := decisionFor(x, kind)
	if !ok {
		return false
	}
	for _, principal := range right.PrincipalIDs {
		if principal == actor {
			return true
		}
	}
	return false
}
func decisionSatisfied(x Incubator, right DecisionRight, actor, body string) bool {
	if !principalForDecision(x, right.Kind, actor) {
		return false
	}
	if right.Rule == "owner" {
		return right.PrincipalIDs[0] == actor
	}
	support := map[string]bool{actor: true}
	for _, event := range x.Events {
		if event.Kind == "decision_support" && event.DecisionKind == right.Kind && event.Body == body && principalForDecision(x, right.Kind, event.ActorID) {
			support[event.ActorID] = true
		}
	}
	if right.Rule == "majority" {
		return len(support) > len(right.PrincipalIDs)/2
	}
	return len(support) == len(right.PrincipalIDs)
}

func (s *Store) Create(x Incubator, actor string, invitations []Invitation) (Incubator, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, e := s.lock()
	if e != nil {
		return Incubator{}, e
	}
	defer f.Close()
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	if !validInput(x) || !text(actor, 100) || len(invitations) > 50 {
		return Incubator{}, ErrInvalid
	}
	seen := map[string]bool{}
	now := s.now().UTC().Truncate(time.Microsecond)
	for j := range invitations {
		i := &invitations[j]
		key := i.PrincipalType + ":" + i.PrincipalID
		if seen[key] || !map[string]bool{"human": true, "agent": true}[i.PrincipalType] || !text(i.PrincipalID, 100) || !text(i.Role, 200) || (i.PrincipalType == "agent" && !text(i.OrganizationID, 100)) {
			return Incubator{}, ErrInvalid
		}
		seen[key] = true
		i.ID = uid()
		i.Status = "pending"
		if i.PrincipalType == "agent" {
			i.Status = "accepted"
		}
		i.InvitedBy = actor
		i.InvitedAt = now
	}
	x.ID = uid()
	x.Version = 1
	x.CreatedBy = actor
	x.CreatedAt = now
	x.UpdatedAt = now
	x.Invitations = invitations
	if x.Invitations == nil {
		x.Invitations = []Invitation{}
	}
	x.PotentialDuplicates = []Duplicate{}
	x.Events = []Event{{ID: uid(), Kind: "opened", Body: "Incubator opened", Visibility: "participants", ActorType: "human", ActorID: actor, CreatedAt: now}}
	all, e := s.all()
	if e != nil {
		return Incubator{}, e
	}
	for _, other := range all {
		if visible(other, actor) && (strings.EqualFold(strings.TrimSpace(other.Problem), strings.TrimSpace(x.Problem)) || strings.EqualFold(strings.TrimSpace(other.Title), strings.TrimSpace(x.Title))) {
			x.PotentialDuplicates = append(x.PotentialDuplicates, Duplicate{IncubatorID: other.ID, Title: other.Title, Reason: "matching title or problem statement"})
		}
	}
	e = s.write(&x)
	return x, e
}
func (s *Store) Get(id, actor string) (Incubator, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	x, e := s.read(id)
	if e != nil || !visible(x, actor) {
		return Incubator{}, ErrNotFound
	}
	return x, nil
}
func (s *Store) List(actor string) ([]Incubator, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	all, e := s.all()
	if e != nil {
		return nil, e
	}
	out := []Incubator{}
	for _, x := range all {
		if visible(x, actor) {
			out = append(out, x)
		}
	}
	return out, nil
}
func (s *Store) AddEvent(id, typ, actor string, expected int, in Event) (Incubator, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, e := s.lock()
	if e != nil {
		return Incubator{}, e
	}
	defer f.Close()
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	x, e := s.read(id)
	if e != nil || !participant(x, typ, actor) {
		return Incubator{}, ErrNotFound
	}
	if x.Version != expected {
		return Incubator{}, ErrConflict
	}
	if !map[string]bool{"discussion": true, "evidence": true, "assumption": true, "decision_support": true, "scope_change": true, "visibility_change": true}[in.Kind] || !text(in.Body, 10000) || !map[string]bool{"participants": true, "public": true}[in.Visibility] {
		return Incubator{}, ErrInvalid
	}
	if in.Kind == "decision_support" {
		if !map[string]bool{"scope_change": true, "visibility_change": true}[in.DecisionKind] || !principalForDecision(x, in.DecisionKind, actor) {
			return Incubator{}, ErrInvalid
		}
	}
	if in.Kind == "scope_change" {
		right, ok := decisionFor(x, "scope_change")
		if !ok || !decisionSatisfied(x, right, actor, in.Body) {
			return Incubator{}, ErrInvalid
		}
	}
	if in.Kind == "visibility_change" {
		right, ok := decisionFor(x, "visibility_change")
		if !ok || !decisionSatisfied(x, right, actor, in.Body) || !map[string]bool{"private": true, "participants": true, "public": true}[strings.TrimSpace(in.Body)] {
			return Incubator{}, ErrInvalid
		}
		x.Visibility = strings.TrimSpace(in.Body)
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	in.ID = uid()
	in.ActorType = typ
	in.ActorID = actor
	in.CreatedAt = now
	x.Events = append(x.Events, in)
	x.Version++
	x.UpdatedAt = now
	e = s.write(&x)
	return x, e
}
func (s *Store) Consent(id, invitation, actor, decision string, expected int) (Incubator, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, e := s.lock()
	if e != nil {
		return Incubator{}, e
	}
	defer f.Close()
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	x, e := s.read(id)
	if e != nil {
		return Incubator{}, ErrNotFound
	}
	if x.Version != expected {
		return Incubator{}, ErrConflict
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	for j := range x.Invitations {
		i := &x.Invitations[j]
		if i.ID == invitation && i.PrincipalType == "human" && i.PrincipalID == actor && i.Status == "pending" && map[string]bool{"accepted": true, "declined": true}[decision] {
			i.Status = decision
			i.RespondedAt = &now
			x.Events = append(x.Events, Event{ID: uid(), Kind: "consent_" + decision, Body: "Invitation " + decision, Visibility: "participants", ActorType: "human", ActorID: actor, CreatedAt: now})
			x.Version++
			x.UpdatedAt = now
			e = s.write(&x)
			return x, e
		}
	}
	return Incubator{}, ErrInvalid
}
