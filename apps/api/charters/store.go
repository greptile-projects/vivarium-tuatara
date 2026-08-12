// Package charters retains immutable project governance revisions and attributed decisions.
package charters

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
	ErrNotFound = errors.New("charter not found")
	ErrInvalid  = errors.New("invalid charter")
	ErrConflict = errors.New("charter version changed")
)

type Role struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Eligibility []string `json:"eligibility"`
}
type DecisionClass struct {
	Name               string   `json:"name"`
	Description        string   `json:"description"`
	EligibleRoles      []string `json:"eligible_roles"`
	Participation      int      `json:"participation"`
	Quorum             int      `json:"quorum"`
	Approval           string   `json:"approval"`
	ProtectedResources []string `json:"protected_resources"`
}
type Procedures struct {
	Terms      string `json:"terms"`
	Removal    string `json:"removal"`
	Succession string `json:"succession"`
	Amendments string `json:"amendments"`
}
type Revision struct {
	ID              string          `json:"id"`
	ScopeType       string          `json:"scope_type"`
	ScopeID         string          `json:"scope_id"`
	Version         int             `json:"version"`
	Status          string          `json:"status"`
	Title           string          `json:"title"`
	Summary         string          `json:"summary"`
	Roles           []Role          `json:"roles"`
	DecisionClasses []DecisionClass `json:"decision_classes"`
	Procedures      Procedures      `json:"procedures"`
	CreatedBy       string          `json:"created_by"`
	CreatedAt       time.Time       `json:"created_at"`
	ActivatedBy     string          `json:"activated_by,omitempty"`
	ActivatedAt     *time.Time      `json:"activated_at,omitempty"`
}
type Approval struct {
	ID        string    `json:"id"`
	Version   int       `json:"version"`
	ActorID   string    `json:"actor_id"`
	Decision  string    `json:"decision"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}
type Exception struct {
	ID            string    `json:"id"`
	Version       int       `json:"version"`
	DecisionClass string    `json:"decision_class"`
	Resource      string    `json:"resource"`
	Reason        string    `json:"reason"`
	ExpiresAt     time.Time `json:"expires_at"`
	CreatedBy     string    `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
}
type Evidence struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Summary    string `json:"summary"`
}
type StandingEvent struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	ActorID   string    `json:"actor_id"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

// Standing is governance participation only. It intentionally contains no
// repository permission or credential material.
type Standing struct {
	ID                 string          `json:"id"`
	CharterVersion     int             `json:"charter_version"`
	PrincipalType      string          `json:"principal_type"`
	PrincipalID        string          `json:"principal_id"`
	Role               string          `json:"role"`
	Responsibilities   string          `json:"responsibilities"`
	Evidence           []Evidence      `json:"evidence"`
	Status             string          `json:"status"`
	ConflictOfInterest string          `json:"conflict_of_interest,omitempty"`
	StartsAt           *time.Time      `json:"starts_at,omitempty"`
	ExpiresAt          time.Time       `json:"expires_at"`
	InvitedBy          string          `json:"invited_by"`
	InvitedAt          time.Time       `json:"invited_at"`
	Events             []StandingEvent `json:"events"`
}

// ContinuityAction records a charter-bound transfer or narrowly scoped recovery.
// Resource authority is referenced, never minted here; its owning subsystem must
// approve and revoke the corresponding access independently.
type ContinuityAction struct {
	ID                    string          `json:"id"`
	CharterVersion        int             `json:"charter_version"`
	Kind                  string          `json:"kind"`
	Role                  string          `json:"role"`
	FromStandingID        string          `json:"from_standing_id,omitempty"`
	ToStandingID          string          `json:"to_standing_id,omitempty"`
	GovernanceProposalID  string          `json:"governance_proposal_id"`
	GovernanceTallySHA256 string          `json:"governance_tally_sha256"`
	Reason                string          `json:"reason"`
	Resources             []string        `json:"resources"`
	Status                string          `json:"status"`
	ExpiresAt             time.Time       `json:"expires_at"`
	ReviewAt              time.Time       `json:"review_at"`
	CreatedBy             string          `json:"created_by"`
	CreatedAt             time.Time       `json:"created_at"`
	ResolvedBy            string          `json:"resolved_by,omitempty"`
	ResolvedAt            *time.Time      `json:"resolved_at,omitempty"`
	Events                []StandingEvent `json:"events"`
}
type Record struct {
	ScopeType     string             `json:"scope_type"`
	ScopeID       string             `json:"scope_id"`
	ActiveVersion int                `json:"active_version"`
	Revisions     []Revision         `json:"revisions"`
	Approvals     []Approval         `json:"approvals"`
	Exceptions    []Exception        `json:"exceptions"`
	Standings     []Standing         `json:"standings"`
	Continuity    []ContinuityAction `json:"continuity"`
}

func (s *Store) CreateContinuity(kind, id, actor string, expected, version int, in ContinuityAction) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Record{}, err
	}
	defer unlock()
	r, err := s.read(kind, id)
	if err != nil {
		return r, err
	}
	now := s.now().UTC()
	if len(r.Continuity) != expected || version != r.ActiveVersion || version < 1 ||
		!map[string]bool{"nomination": true, "election": true, "recall": true, "succession": true, "emergency": true}[in.Kind] ||
		strings.TrimSpace(in.Role) == "" || strings.TrimSpace(in.GovernanceProposalID) == "" || strings.TrimSpace(in.GovernanceTallySHA256) == "" || strings.TrimSpace(in.Reason) == "" || len(in.Resources) == 0 ||
		!in.ExpiresAt.After(now) || !in.ReviewAt.After(now) || in.ReviewAt.After(in.ExpiresAt) {
		return r, ErrInvalid
	}
	roleFound := false
	for _, role := range r.Revisions[version-1].Roles {
		if role.Name == in.Role {
			roleFound = true
		}
	}
	standing := func(sid string) bool {
		if sid == "" {
			return false
		}
		for _, st := range r.Standings {
			if st.ID == sid && st.CharterVersion == version && st.ExpiresAt.After(now) && st.Status == "active" && st.Role == in.Role {
				return true
			}
		}
		return false
	}
	if !roleFound || (in.Kind != "emergency" && !standing(in.ToStandingID)) || ((in.Kind == "recall" || in.Kind == "succession") && !standing(in.FromStandingID)) {
		return r, ErrInvalid
	}
	for _, resource := range in.Resources {
		if !declaredResource(r.Revisions[version-1], resource) {
			return r, ErrInvalid
		}
	}
	in.ID = randomID()
	in.CharterVersion = version
	in.Status = "pending"
	in.CreatedBy = actor
	in.CreatedAt = now
	in.Events = []StandingEvent{{ID: randomID(), Kind: "created", ActorID: actor, Reason: in.Reason, CreatedAt: now}}
	r.Continuity = append(r.Continuity, in)
	return r, s.write(r)
}

func (s *Store) ActOnContinuity(kind, id, actionID, actor, action, reason string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Record{}, err
	}
	defer unlock()
	r, err := s.read(kind, id)
	if err != nil {
		return r, err
	}
	now := s.now().UTC()
	for i := range r.Continuity {
		x := &r.Continuity[i]
		if x.ID != actionID {
			continue
		}
		if strings.TrimSpace(reason) == "" {
			return r, ErrInvalid
		}
		next := ""
		switch action {
		case "approve":
			if x.Status == "pending" {
				next = "active"
			}
		case "complete":
			if x.Status == "active" {
				next = "completed"
			}
		case "relinquish":
			if x.Status == "active" {
				next = "relinquished"
			}
		case "appeal":
			if x.Status == "pending" || x.Status == "active" {
				next = x.Status
			}
		}
		if next == "" || ((!x.ExpiresAt.After(now) || !x.ReviewAt.After(now)) && action != "relinquish") {
			return r, ErrConflict
		}
		x.Status = next
		x.Events = append(x.Events, StandingEvent{ID: randomID(), Kind: action, ActorID: actor, Reason: reason, CreatedAt: now})
		if next == "completed" || next == "relinquished" {
			x.ResolvedBy = actor
			x.ResolvedAt = &now
		}
		return r, s.write(r)
	}
	return r, ErrNotFound
}

func declaredResource(v Revision, resource string) bool {
	for _, d := range v.DecisionClasses {
		if slicesContains(d.ProtectedResources, resource) {
			return true
		}
	}
	return false
}

func (s *Store) Invite(kind, id, actor string, expected, charterVersion int, principalType, principalID, role, responsibilities string, evidence []Evidence, expires time.Time) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Record{}, err
	}
	defer unlock()
	r, err := s.read(kind, id)
	if err != nil {
		return r, err
	}
	if len(r.Standings) != expected || charterVersion != r.ActiveVersion || charterVersion < 1 || !expires.After(s.now()) || principalType != "human" || strings.TrimSpace(principalID) == "" || len(evidence) == 0 {
		return r, ErrInvalid
	}
	var description string
	for _, candidate := range r.Revisions[charterVersion-1].Roles {
		if candidate.Name == role {
			description = candidate.Description
		}
	}
	if description == "" || strings.TrimSpace(responsibilities) == "" {
		return r, ErrInvalid
	}
	for _, item := range evidence {
		if !map[string]bool{"contribution": true, "review": true, "support": true, "ownership": true, "membership": true}[item.Kind] || strings.TrimSpace(item.ResourceID) == "" || strings.TrimSpace(item.Summary) == "" {
			return r, ErrInvalid
		}
	}
	for _, standing := range r.Standings {
		if standing.PrincipalID == principalID && standing.Role == role && (standing.Status == "invited" || standing.Status == "active" || standing.Status == "recused" || standing.Status == "suspended") {
			return r, ErrConflict
		}
	}
	now := s.now().UTC()
	event := StandingEvent{ID: randomID(), Kind: "invited", ActorID: actor, Reason: "Governance standing invited from reviewed evidence", CreatedAt: now}
	r.Standings = append(r.Standings, Standing{ID: randomID(), CharterVersion: charterVersion, PrincipalType: principalType, PrincipalID: principalID, Role: role, Responsibilities: responsibilities, Evidence: evidence, Status: "invited", ExpiresAt: expires.UTC(), InvitedBy: actor, InvitedAt: now, Events: []StandingEvent{event}})
	return r, s.write(r)
}

func (s *Store) ActOnStanding(kind, id, standingID, actor, action, reason, conflict string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Record{}, err
	}
	defer unlock()
	r, err := s.read(kind, id)
	if err != nil {
		return r, err
	}
	for i := range r.Standings {
		st := &r.Standings[i]
		if st.ID != standingID {
			continue
		}
		now := s.now().UTC()
		if !st.ExpiresAt.After(now) {
			return r, ErrConflict
		}
		self := actor == st.PrincipalID
		allowed := (self && ((action == "accept" && st.Status == "invited") || (action == "decline" && st.Status == "invited") || (action == "recuse" && st.Status == "active") || (action == "appeal" && (st.Status == "suspended" || st.Status == "revoked")))) || (!self && ((action == "suspend" && (st.Status == "active" || st.Status == "recused")) || (action == "reinstate" && st.Status == "suspended") || (action == "revoke" && st.Status != "revoked")))
		if !allowed || strings.TrimSpace(reason) == "" {
			return r, ErrInvalid
		}
		next := map[string]string{"accept": "active", "decline": "declined", "recuse": "recused", "suspend": "suspended", "reinstate": "active", "revoke": "revoked"}[action]
		if action == "appeal" {
			next = st.Status
		}
		if action == "accept" {
			st.StartsAt = &now
		}
		st.Status = next
		if action == "recuse" {
			st.ConflictOfInterest = strings.TrimSpace(conflict)
			if st.ConflictOfInterest == "" {
				return r, ErrInvalid
			}
		}
		st.Events = append(st.Events, StandingEvent{ID: randomID(), Kind: action, ActorID: actor, Reason: reason, CreatedAt: now})
		return r, s.write(r)
	}
	return r, ErrNotFound
}

type Store struct {
	root string
	mu   sync.Mutex
	now  func() time.Time
}

// Eligibility entries are intentionally closed identity-source rules. Free-form
// descriptions cannot safely confer governance eligibility.
var eligibilityRules = map[string]map[string]bool{
	"repository": {
		"repository_owner": true, "repository_collaborator": true,
	},
	"organization": {
		"organization_owner": true, "organization_member": true,
		"team_maintainer": true, "approved_agent": true,
	},
}

func New(root string) (*Store, error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	return &Store{root: root, now: time.Now}, nil
}
func (s *Store) Get(kind, id string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read(kind, id)
}

// WithGovernanceAdmission holds the same in-process and cross-process boundary
// used by standing mutations from exact active-charter validation through fn.
// Governance stores may commit inside fn; callers must not call back into this
// charter store while the admission is held.
func (s *Store) WithGovernanceAdmission(kind, id string, version int, fn func(Record) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return err
	}
	defer unlock()
	record, err := s.read(kind, id)
	if err != nil {
		return err
	}
	if version < 1 || record.ActiveVersion != version || version > len(record.Revisions) || record.Revisions[version-1].Status != "active" {
		return ErrConflict
	}
	return fn(record)
}
func (s *Store) Publish(kind, id, actor string, expected int, in Revision) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Record{}, err
	}
	defer unlock()
	r, err := s.readOrEmpty(kind, id)
	if err != nil {
		return r, err
	}
	if len(r.Revisions) != expected {
		return r, ErrConflict
	}
	in.ScopeType, in.ScopeID, in.Version, in.Status, in.CreatedBy, in.CreatedAt = kind, id, len(r.Revisions)+1, "draft", actor, s.now().UTC().Truncate(time.Microsecond)
	in.ID = randomID()
	if !valid(in) {
		return r, ErrInvalid
	}
	r.Revisions = append(r.Revisions, in)
	return r, s.write(r)
}
func (s *Store) Approve(kind, id, actor string, version int, decision, reason string) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Record{}, err
	}
	defer unlock()
	r, err := s.read(kind, id)
	if err != nil {
		return r, err
	}
	if version < 1 || version > len(r.Revisions) || (decision != "approved" && decision != "rejected") {
		return r, ErrInvalid
	}
	for _, a := range r.Approvals {
		if a.Version == version && a.ActorID == actor {
			return r, ErrConflict
		}
	}
	r.Approvals = append(r.Approvals, Approval{ID: randomID(), Version: version, ActorID: actor, Decision: decision, Reason: strings.TrimSpace(reason), CreatedAt: s.now().UTC()})
	return r, s.write(r)
}
func (s *Store) Activate(kind, id, actor string, version int) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Record{}, err
	}
	defer unlock()
	r, err := s.read(kind, id)
	if err != nil {
		return r, err
	}
	if version < 1 || version > len(r.Revisions) || r.Revisions[version-1].Status != "draft" {
		return r, ErrConflict
	}
	approved := false
	for _, a := range r.Approvals {
		if a.Version == version && a.Decision == "approved" {
			approved = true
		}
	}
	if !approved {
		return r, ErrInvalid
	}
	now := s.now().UTC()
	r.ActiveVersion = version
	r.Revisions[version-1].Status = "active"
	r.Revisions[version-1].ActivatedBy = actor
	r.Revisions[version-1].ActivatedAt = &now
	return r, s.write(r)
}
func (s *Store) Except(kind, id, actor string, version int, class, resource, reason string, expires time.Time) (Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lock()
	if err != nil {
		return Record{}, err
	}
	defer unlock()
	r, err := s.read(kind, id)
	if err != nil {
		return r, err
	}
	if version < 1 || version != r.ActiveVersion || version > len(r.Revisions) || r.Revisions[version-1].Status != "active" || strings.TrimSpace(class) == "" || strings.TrimSpace(resource) == "" || strings.TrimSpace(reason) == "" || !expires.After(s.now()) {
		return r, ErrInvalid
	}
	declared := false
	for _, decision := range r.Revisions[version-1].DecisionClasses {
		if decision.Name == class && slicesContains(decision.ProtectedResources, resource) {
			declared = true
			break
		}
	}
	if !declared {
		return r, ErrInvalid
	}
	r.Exceptions = append(r.Exceptions, Exception{ID: randomID(), Version: version, DecisionClass: class, Resource: resource, Reason: reason, ExpiresAt: expires.UTC(), CreatedBy: actor, CreatedAt: s.now().UTC()})
	return r, s.write(r)
}
func valid(v Revision) bool {
	if (v.ScopeType != "repository" && v.ScopeType != "organization") || strings.TrimSpace(v.Title) == "" || strings.TrimSpace(v.Summary) == "" || len(v.Roles) == 0 || len(v.DecisionClasses) == 0 || v.Procedures.Terms == "" || v.Procedures.Removal == "" || v.Procedures.Succession == "" || v.Procedures.Amendments == "" {
		return false
	}
	names := map[string]bool{}
	for _, x := range v.Roles {
		if x.Name == "" || x.Description == "" || len(x.Eligibility) == 0 || names[x.Name] {
			return false
		}
		for _, rule := range x.Eligibility {
			if !eligibilityRules[v.ScopeType][rule] {
				return false
			}
		}
		names[x.Name] = true
	}
	for _, d := range v.DecisionClasses {
		if d.Name == "" || d.Description == "" || len(d.EligibleRoles) == 0 || d.Participation < 1 || d.Quorum < 1 || d.Quorum > d.Participation || len(d.ProtectedResources) == 0 || (d.Approval != "majority" && d.Approval != "consensus" && d.Approval != "supermajority") {
			return false
		}
		for _, n := range d.EligibleRoles {
			if !names[n] {
				return false
			}
		}
	}
	return true
}
func (s *Store) path(k, id string) string { return filepath.Join(s.root, k+"-"+id+".json") }
func (s *Store) readOrEmpty(k, id string) (Record, error) {
	r, e := s.read(k, id)
	if errors.Is(e, ErrNotFound) {
		return Record{ScopeType: k, ScopeID: id, Revisions: []Revision{}, Approvals: []Approval{}, Exceptions: []Exception{}, Standings: []Standing{}, Continuity: []ContinuityAction{}}, nil
	}
	return r, e
}
func (s *Store) read(k, id string) (Record, error) {
	if !safe(k) || !safe(id) {
		return Record{}, ErrNotFound
	}
	b, e := os.ReadFile(s.path(k, id))
	if errors.Is(e, os.ErrNotExist) {
		return Record{}, ErrNotFound
	}
	if e != nil {
		return Record{}, e
	}
	var r Record
	if json.Unmarshal(b, &r) != nil || r.ScopeType != k || r.ScopeID != id {
		return Record{}, ErrNotFound
	}
	return r, nil
}
func (s *Store) write(r Record) error {
	b, e := json.MarshalIndent(r, "", "  ")
	if e != nil {
		return e
	}
	p := s.path(r.ScopeType, r.ScopeID)
	f, e := os.CreateTemp(s.root, filepath.Base(p)+"-*.tmp")
	if e != nil {
		return e
	}
	tmp := f.Name()
	if e = f.Chmod(0600); e != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return e
	}
	if _, e = f.Write(b); e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e != nil {
		_ = os.Remove(tmp)
		return e
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if e = os.Rename(tmp, p); e != nil {
		_ = os.Remove(tmp)
		return e
	}
	d, e := os.Open(s.root)
	if e != nil {
		return e
	}
	e = d.Sync()
	if closeErr = d.Close(); e == nil {
		e = closeErr
	}
	return e
}
func safe(v string) bool { return v != "" && !strings.ContainsAny(v, "/\\.") }
func randomID() string   { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func slicesContains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
func (s *Store) lock() (func(), error) {
	f, err := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}
func SortExceptions(v []Exception) {
	sort.Slice(v, func(i, j int) bool { return v[i].CreatedAt.Before(v[j].CreatedAt) })
}
