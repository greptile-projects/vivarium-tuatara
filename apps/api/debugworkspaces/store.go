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
var ErrForbidden = errors.New("debugging workspace action forbidden")

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

// Citation is a server-resolved, revision-aware pointer used to make a diagnostic
// claim independently inspectable. Detail is deliberately bounded metadata, not
// runtime payload content.
type Citation struct {
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	ResourceID    string `json:"resource_id,omitempty"`
	Revision      string `json:"revision,omitempty"`
	Path          string `json:"path,omitempty"`
	Symbol        string `json:"symbol,omitempty"`
	LineStart     int    `json:"line_start,omitempty"`
	LineEnd       int    `json:"line_end,omitempty"`
	Label         string `json:"label"`
	EvidenceID    string `json:"evidence_id,omitempty"`
	Accessible    bool   `json:"accessible"`
	BlockedReason string `json:"blocked_reason,omitempty"`
}
type ClaimResponse struct {
	ID          string    `json:"id"`
	ActorID     string    `json:"actor_id"`
	Kind        string    `json:"kind"`
	Message     string    `json:"message"`
	CitationIDs []string  `json:"citation_ids"`
	CreatedAt   time.Time `json:"created_at"`
}
type Claim struct {
	ID                   string          `json:"id"`
	Kind                 string          `json:"kind"`
	Statement            string          `json:"statement"`
	Uncertainty          string          `json:"uncertainty"`
	Confidence           string          `json:"confidence"`
	CitationIDs          []string        `json:"citation_ids"`
	Status               string          `json:"status"`
	CreatedBy            string          `json:"created_by"`
	AgentInvestigationID string          `json:"agent_investigation_id,omitempty"`
	Responses            []ClaimResponse `json:"responses"`
	CreatedAt            time.Time       `json:"created_at"`
}
type OwnerRequest struct {
	ID          string    `json:"id"`
	OwnerType   string    `json:"owner_type"`
	OwnerID     string    `json:"owner_id"`
	Question    string    `json:"question"`
	CitationIDs []string  `json:"citation_ids"`
	Status      string    `json:"status"`
	RequestedBy string    `json:"requested_by"`
	Response    string    `json:"response,omitempty"`
	RespondedBy string    `json:"responded_by,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}
type AgentInvestigation struct {
	ID           string    `json:"id"`
	AgentID      string    `json:"agent_id"`
	InitiatorID  string    `json:"initiator_id"`
	CredentialID string    `json:"credential_id"`
	Mandate      string    `json:"mandate"`
	CitationIDs  []string  `json:"citation_ids"`
	State        string    `json:"state"`
	Guidance     []Event   `json:"guidance"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
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
type ProbePolicy struct {
	DataCategories []string `json:"data_categories"`
	Privacy        string   `json:"privacy"`
	Security       string   `json:"security"`
	RetentionHours int      `json:"retention_hours"`
	SamplePercent  int      `json:"sample_percent"`
	MaxCostCents   int      `json:"max_cost_cents"`
	MaxLoadPercent int      `json:"max_load_percent"`
}
type ProbeArtifact struct {
	Kind      string `json:"kind"`
	Digest    string `json:"digest"`
	SizeBytes int64  `json:"size_bytes"`
	Reference string `json:"reference"`
	Redaction string `json:"redaction"`
}
type ProbeAction struct {
	ID              string          `json:"id"`
	ActorID         string          `json:"actor_id"`
	Outcome         string          `json:"outcome"`
	StartedAt       time.Time       `json:"started_at"`
	FinishedAt      time.Time       `json:"finished_at"`
	Provenance      string          `json:"provenance"`
	Transformations []string        `json:"transformations"`
	Gaps            []string        `json:"gaps"`
	Artifacts       []ProbeArtifact `json:"artifacts"`
	CreatedAt       time.Time       `json:"created_at"`
}
type Probe struct {
	ID                 string        `json:"id"`
	Version            int           `json:"version"`
	Kind               string        `json:"kind"`
	Purpose            string        `json:"purpose"`
	DefinitionPath     string        `json:"definition_path,omitempty"`
	DefinitionRevision string        `json:"definition_revision,omitempty"`
	AudienceUserIDs    []string      `json:"audience_user_ids"`
	RequestedPolicy    ProbePolicy   `json:"requested_policy"`
	ApprovedPolicy     *ProbePolicy  `json:"approved_policy,omitempty"`
	Status             string        `json:"status"`
	RequestedBy        string        `json:"requested_by"`
	RequestedAt        time.Time     `json:"requested_at"`
	DecidedBy          string        `json:"decided_by,omitempty"`
	DecisionReason     string        `json:"decision_reason,omitempty"`
	ApprovedAt         *time.Time    `json:"approved_at,omitempty"`
	ExpiresAt          time.Time     `json:"expires_at"`
	RevokedBy          string        `json:"revoked_by,omitempty"`
	RevokedAt          *time.Time    `json:"revoked_at,omitempty"`
	Actions            []ProbeAction `json:"actions"`
}
type Workspace struct {
	ID                  string               `json:"id"`
	RepositoryID        string               `json:"repository_id"`
	Version             int                  `json:"version"`
	Title               string               `json:"title"`
	Summary             string               `json:"summary"`
	Trigger             Reference            `json:"trigger"`
	Release             Reference            `json:"release"`
	Environment         Reference            `json:"environment"`
	TimeStart           time.Time            `json:"time_start"`
	TimeEnd             time.Time            `json:"time_end"`
	UserJourney         string               `json:"user_journey"`
	OwnerIDs            []string             `json:"owner_ids"`
	Severity            string               `json:"severity"`
	Audience            string               `json:"audience"`
	AccessUserIDs       []string             `json:"access_user_ids"`
	Status              string               `json:"status"`
	Source              Reference            `json:"source"`
	Packages            []Reference          `json:"packages"`
	Configuration       Reference            `json:"configuration"`
	Infrastructure      Reference            `json:"infrastructure"`
	Evidence            []Evidence           `json:"permitted_evidence"`
	UnavailableContext  []string             `json:"unavailable_context"`
	Hypotheses          []Hypothesis         `json:"hypotheses"`
	History             []Event              `json:"history"`
	Probes              []Probe              `json:"probes"`
	Citations           []Citation           `json:"citations"`
	Claims              []Claim              `json:"claims"`
	OwnerRequests       []OwnerRequest       `json:"owner_requests"`
	AgentInvestigations []AgentInvestigation `json:"agent_investigations"`
	CreatedBy           string               `json:"created_by"`
	CreatedAt           time.Time            `json:"created_at"`
	UpdatedAt           time.Time            `json:"updated_at"`
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
	v.Probes = []Probe{}
	v.Citations, v.Claims, v.OwnerRequests, v.AgentInvestigations = []Citation{}, []Claim{}, []OwnerRequest{}, []AgentInvestigation{}
	v.History = []Event{{ID: id(), Kind: "opened", ActorID: actor, To: "open", CreatedAt: now}}
	return v, s.write(v)
}

func (s *Store) AddClaim(repo, wid, actor string, citations []Citation, claim Claim, expected int) (Workspace, error) {
	return s.mutate(repo, wid, expected, func(v *Workspace, now time.Time) error {
		if !one(claim.Kind, "hypothesis", "query", "finding", "uncertainty") || strings.TrimSpace(claim.Statement) == "" || len(claim.Statement) > 8000 || strings.TrimSpace(claim.Uncertainty) == "" || !one(claim.Confidence, "low", "medium", "high") || len(citations) == 0 || sensitive(claim.Statement+claim.Uncertainty) {
			return ErrInvalid
		}
		ids := []string{}
		for i := range citations {
			citations[i].ID = id()
			ids = append(ids, citations[i].ID)
			v.Citations = append(v.Citations, citations[i])
		}
		claim.ID, claim.CreatedBy, claim.CreatedAt, claim.CitationIDs, claim.Responses = id(), actor, now, ids, []ClaimResponse{}
		claim.Status = deriveClaimStatus(claim, v.Citations)
		v.Claims = append(v.Claims, claim)
		v.History = append(v.History, Event{ID: id(), Kind: "claim_published", ActorID: actor, To: claim.ID, Message: claim.Kind, CreatedAt: now})
		return nil
	})
}

func (s *Store) RespondClaim(repo, wid, claimID, actor, kind, message string, citationIDs []string, expected int) (Workspace, error) {
	return s.mutate(repo, wid, expected, func(v *Workspace, now time.Time) error {
		if !one(kind, "support", "dispute", "mark_stale") || strings.TrimSpace(message) == "" || len(message) > 4000 || !allCitationIDs(*v, citationIDs) {
			return ErrInvalid
		}
		for i := range v.Claims {
			if v.Claims[i].ID == claimID {
				r := ClaimResponse{ID: id(), ActorID: actor, Kind: kind, Message: strings.TrimSpace(message), CitationIDs: uniqueWords(citationIDs), CreatedAt: now}
				v.Claims[i].Responses = append(v.Claims[i].Responses, r)
				if kind == "dispute" {
					v.Claims[i].Status = "disputed"
				} else if kind == "mark_stale" {
					v.Claims[i].Status = "stale"
				} else if v.Claims[i].Status != "disputed" && v.Claims[i].Status != "stale" {
					v.Claims[i].Status = deriveClaimStatus(v.Claims[i], v.Citations)
				}
				v.History = append(v.History, Event{ID: id(), Kind: "claim_" + kind, ActorID: actor, To: claimID, Message: r.Message, CreatedAt: now})
				return nil
			}
		}
		return ErrNotFound
	})
}

func (s *Store) RequestOwner(repo, wid, actor string, in OwnerRequest, expected int) (Workspace, error) {
	return s.mutate(repo, wid, expected, func(v *Workspace, now time.Time) error {
		if !one(in.OwnerType, "code", "service", "privacy", "security") || !closed(in.OwnerID) || strings.TrimSpace(in.Question) == "" || len(in.Question) > 4000 || !allCitationIDs(*v, in.CitationIDs) {
			return ErrInvalid
		}
		in.ID, in.Status, in.RequestedBy, in.CreatedAt = id(), "open", actor, now
		in.CitationIDs = uniqueWords(in.CitationIDs)
		v.OwnerRequests = append(v.OwnerRequests, in)
		v.History = append(v.History, Event{ID: id(), Kind: "owner_input_requested", ActorID: actor, To: in.ID, Message: in.OwnerType, CreatedAt: now})
		return nil
	})
}

func (s *Store) AnswerOwner(repo, wid, requestID, actor, response string, expected int) (Workspace, error) {
	return s.mutate(repo, wid, expected, func(v *Workspace, now time.Time) error {
		for i := range v.OwnerRequests {
			x := &v.OwnerRequests[i]
			if x.ID == requestID {
				if x.OwnerID != actor || x.Status != "open" || strings.TrimSpace(response) == "" || len(response) > 4000 {
					return ErrForbidden
				}
				x.Status, x.Response, x.RespondedBy = "answered", strings.TrimSpace(response), actor
				v.History = append(v.History, Event{ID: id(), Kind: "owner_input_answered", ActorID: actor, To: x.ID, CreatedAt: now})
				return nil
			}
		}
		return ErrNotFound
	})
}

func (s *Store) StartAgent(repo, wid, actor, agentID, credentialID, mandate string, citationIDs []string, expected int) (Workspace, AgentInvestigation, error) {
	var out AgentInvestigation
	v, e := s.mutate(repo, wid, expected, func(v *Workspace, now time.Time) error {
		if !closed(agentID) || !closed(credentialID) || strings.TrimSpace(mandate) == "" || len(mandate) > 4000 || len(citationIDs) == 0 || !allCitationIDs(*v, citationIDs) {
			return ErrInvalid
		}
		out = AgentInvestigation{ID: id(), AgentID: agentID, InitiatorID: actor, CredentialID: credentialID, Mandate: strings.TrimSpace(mandate), CitationIDs: uniqueWords(citationIDs), State: "running", Guidance: []Event{}, CreatedAt: now, UpdatedAt: now}
		v.AgentInvestigations = append(v.AgentInvestigations, out)
		v.History = append(v.History, Event{ID: id(), Kind: "agent_started", ActorID: actor, To: out.ID, CreatedAt: now})
		return nil
	})
	return v, out, e
}

func (s *Store) ControlAgent(repo, wid, iid, actor, action, message string, expected int) (Workspace, AgentInvestigation, error) {
	var out AgentInvestigation
	v, e := s.mutate(repo, wid, expected, func(v *Workspace, now time.Time) error {
		for i := range v.AgentInvestigations {
			x := &v.AgentInvestigations[i]
			if x.ID != iid {
				continue
			}
			switch action {
			case "guide":
				if strings.TrimSpace(message) == "" || x.State == "revoked" {
					return ErrConflict
				}
				x.Guidance = append(x.Guidance, Event{ID: id(), Kind: "guide", ActorID: actor, Message: strings.TrimSpace(message), CreatedAt: now})
			case "pause":
				if x.State != "running" {
					return ErrConflict
				}
				x.State = "paused"
			case "resume":
				if x.State != "paused" {
					return ErrConflict
				}
				x.State = "running"
			case "revoke":
				if x.State == "revoked" {
					out = *x
					return nil
				}
				x.State = "revoked"
			default:
				return ErrInvalid
			}
			x.UpdatedAt = now
			out = *x
			v.History = append(v.History, Event{ID: id(), Kind: "agent_" + action, ActorID: actor, To: iid, Message: strings.TrimSpace(message), CreatedAt: now})
			return nil
		}
		return ErrNotFound
	})
	return v, out, e
}

func (s *Store) AgentClaim(repo, wid, iid, credentialID string, claim Claim, expected int) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, e := s.lock()
	if e != nil {
		return Workspace{}, e
	}
	defer unlock()
	v, e := s.read(repo, wid)
	if e != nil {
		return Workspace{}, e
	}
	if v.Version != expected {
		return Workspace{}, ErrConflict
	}
	var x *AgentInvestigation
	for i := range v.AgentInvestigations {
		if v.AgentInvestigations[i].ID == iid {
			x = &v.AgentInvestigations[i]
		}
	}
	if x == nil {
		return Workspace{}, ErrNotFound
	}
	if x.CredentialID != credentialID || x.State != "running" {
		return Workspace{}, ErrForbidden
	}
	if !one(claim.Kind, "hypothesis", "query", "finding", "uncertainty") || strings.TrimSpace(claim.Statement) == "" || len(claim.Statement) > 8000 || strings.TrimSpace(claim.Uncertainty) == "" || !one(claim.Confidence, "low", "medium", "high") || len(claim.CitationIDs) == 0 || !subset(claim.CitationIDs, x.CitationIDs) || !allCitationIDs(v, claim.CitationIDs) || sensitive(claim.Statement+claim.Uncertainty) {
		return Workspace{}, ErrInvalid
	}
	now := s.now()
	claim.ID, claim.CreatedBy, claim.AgentInvestigationID, claim.CreatedAt, claim.Responses = id(), x.AgentID, x.ID, now, []ClaimResponse{}
	claim.Status = deriveClaimStatus(claim, v.Citations)
	v.Claims = append(v.Claims, claim)
	x.UpdatedAt = now
	v.Version++
	v.UpdatedAt = now
	v.History = append(v.History, Event{ID: id(), Kind: "agent_claim_published", ActorID: x.AgentID, To: claim.ID, Message: claim.Kind, CreatedAt: now})
	return v, s.write(v)
}

func (s *Store) mutate(repo, wid string, expected int, fn func(*Workspace, time.Time) error) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, e := s.lock()
	if e != nil {
		return Workspace{}, e
	}
	defer unlock()
	v, e := s.read(repo, wid)
	if e != nil {
		return Workspace{}, e
	}
	if v.Version != expected {
		return Workspace{}, ErrConflict
	}
	now := s.now()
	if e = fn(&v, now); e != nil {
		return Workspace{}, e
	}
	v.Version++
	v.UpdatedAt = now
	return v, s.write(v)
}
func allCitationIDs(v Workspace, ids []string) bool {
	if len(ids) == 0 {
		return true
	}
	for _, wanted := range ids {
		found := false
		for _, c := range v.Citations {
			found = found || c.ID == wanted
		}
		if !found {
			return false
		}
	}
	return true
}
func subset(a, b []string) bool {
	for _, x := range a {
		if !contains(b, x) {
			return false
		}
	}
	return true
}
func deriveClaimStatus(c Claim, citations []Citation) string {
	for _, id := range c.CitationIDs {
		for _, x := range citations {
			if x.ID == id && !x.Accessible {
				return "blocked"
			}
		}
	}
	return "supported"
}

func (s *Store) RequestProbe(repo, wid, actor string, in Probe, expected int) (Workspace, error) {
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
	if !contains(in.AudienceUserIDs, actor) || !validProbe(in, v, now) {
		return Workspace{}, ErrInvalid
	}
	in.ID, in.Version, in.Status, in.RequestedBy, in.RequestedAt = id(), 1, "pending", actor, now
	in.AudienceUserIDs = unique(in.AudienceUserIDs)
	in.RequestedPolicy.DataCategories = uniqueWords(in.RequestedPolicy.DataCategories)
	in.ApprovedPolicy, in.Actions = nil, []ProbeAction{}
	v.Probes = append(v.Probes, in)
	v.Version++
	v.UpdatedAt = now
	v.History = append(v.History, Event{ID: id(), Kind: "probe_requested", ActorID: actor, To: in.ID, CreatedAt: now})
	return v, s.write(v)
}

func (s *Store) DecideProbe(repo, wid, pid, actor, decision, reason string, policy ProbePolicy, expires time.Time, expected int) (Workspace, error) {
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
	if !contains(v.OwnerIDs, actor) {
		return Workspace{}, ErrForbidden
	}
	i := probeIndex(v.Probes, pid)
	if i < 0 {
		return Workspace{}, ErrNotFound
	}
	p, now := &v.Probes[i], s.now()
	if p.Status != "pending" || !one(decision, "approved", "denied") || strings.TrimSpace(reason) == "" {
		return Workspace{}, ErrInvalid
	}
	if decision == "approved" {
		policy.DataCategories = uniqueWords(policy.DataCategories)
		if !validPolicy(policy) || !narrower(policy, p.RequestedPolicy) || !expires.After(now) || expires.After(p.ExpiresAt) {
			return Workspace{}, ErrInvalid
		}
		p.ApprovedPolicy, p.ApprovedAt, p.ExpiresAt = &policy, &now, expires
	}
	p.Status, p.DecidedBy, p.DecisionReason, p.Version = decision, actor, strings.TrimSpace(reason), p.Version+1
	v.Version++
	v.UpdatedAt = now
	v.History = append(v.History, Event{ID: id(), Kind: "probe_" + decision, ActorID: actor, To: pid, Message: p.DecisionReason, CreatedAt: now})
	return v, s.write(v)
}

func (s *Store) ReportProbe(repo, wid, pid, actor string, in ProbeAction, expected int) (Workspace, error) {
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
	i := probeIndex(v.Probes, pid)
	if i < 0 {
		return Workspace{}, ErrNotFound
	}
	p, now := &v.Probes[i], s.now()
	if p.RequestedBy != actor {
		return Workspace{}, ErrForbidden
	}
	if p.Status != "approved" || !now.Before(p.ExpiresAt) {
		return Workspace{}, ErrInvalid
	}
	if !validAction(in, *p, now) {
		return Workspace{}, ErrInvalid
	}
	in.ID, in.ActorID, in.CreatedAt = id(), actor, now
	p.Actions = append(p.Actions, in)
	p.Version++
	if in.Outcome == "complete" {
		p.Status = "completed"
	} else {
		p.Status = in.Outcome
	}
	v.Version++
	v.UpdatedAt = now
	v.History = append(v.History, Event{ID: id(), Kind: "probe_" + in.Outcome, ActorID: actor, To: pid, CreatedAt: now})
	return v, s.write(v)
}

func (s *Store) RevokeProbe(repo, wid, pid, actor, reason string, expected int) (Workspace, error) {
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
	if !contains(v.OwnerIDs, actor) {
		return Workspace{}, ErrForbidden
	}
	i := probeIndex(v.Probes, pid)
	if i < 0 {
		return Workspace{}, ErrNotFound
	}
	p, now := &v.Probes[i], s.now()
	if !one(p.Status, "pending", "approved") || strings.TrimSpace(reason) == "" {
		return Workspace{}, ErrInvalid
	}
	p.Status, p.RevokedBy, p.RevokedAt, p.DecisionReason, p.Version = "revoked", actor, &now, strings.TrimSpace(reason), p.Version+1
	v.Version++
	v.UpdatedAt = now
	v.History = append(v.History, Event{ID: id(), Kind: "probe_revoked", ActorID: actor, To: pid, Message: reason, CreatedAt: now})
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
func uniqueWords(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}
func contains(in []string, wanted string) bool {
	for _, v := range in {
		if v == wanted {
			return true
		}
	}
	return false
}
func probeIndex(in []Probe, wanted string) int {
	for i := range in {
		if in[i].ID == wanted {
			return i
		}
	}
	return -1
}
func validProbe(p Probe, w Workspace, now time.Time) bool {
	if !one(p.Kind, "logs", "traces", "profile", "state_snapshot", "dynamic_diagnostic") || strings.TrimSpace(p.Purpose) == "" || len(p.Purpose) > 2000 || !validPolicy(p.RequestedPolicy) || !p.ExpiresAt.After(now) || p.ExpiresAt.After(now.Add(24*time.Hour)) || len(p.AudienceUserIDs) == 0 {
		return false
	}
	allowed := append(append([]string{}, w.OwnerIDs...), w.AccessUserIDs...)
	allowed = append(allowed, w.CreatedBy)
	for _, actor := range p.AudienceUserIDs {
		if !closed(actor) || (w.Audience == "restricted" && !contains(allowed, actor)) {
			return false
		}
	}
	if p.Kind == "dynamic_diagnostic" {
		path := strings.TrimSpace(p.DefinitionPath)
		if !strings.HasPrefix(path, ".vivarium/diagnostics/") || !strings.HasSuffix(path, ".json") || strings.Contains(path, "..") || p.DefinitionRevision != w.Source.Revision {
			return false
		}
	} else if p.DefinitionPath != "" || p.DefinitionRevision != "" {
		return false
	}
	return !sensitive(p.Purpose + p.DefinitionPath)
}
func validPolicy(p ProbePolicy) bool {
	if len(p.DataCategories) == 0 || privacyRank(p.Privacy) == 0 || securityRank(p.Security) == 0 || p.RetentionHours < 1 || p.RetentionHours > 720 || p.SamplePercent < 1 || p.SamplePercent > 100 || p.MaxCostCents < 0 || p.MaxCostCents > 10000000 || p.MaxLoadPercent < 1 || p.MaxLoadPercent > 100 {
		return false
	}
	for _, c := range p.DataCategories {
		if !one(c, "application_logs", "request_metadata", "stack_traces", "timing_spans", "runtime_profile", "configuration_shape", "aggregate_state") {
			return false
		}
	}
	return true
}
func narrower(a, requested ProbePolicy) bool {
	if privacyRank(a.Privacy) < privacyRank(requested.Privacy) || securityRank(a.Security) < securityRank(requested.Security) {
		return false
	}
	if a.RetentionHours > requested.RetentionHours || a.SamplePercent > requested.SamplePercent || a.MaxCostCents > requested.MaxCostCents || a.MaxLoadPercent > requested.MaxLoadPercent {
		return false
	}
	for _, c := range a.DataCategories {
		if !contains(requested.DataCategories, c) {
			return false
		}
	}
	return true
}
func privacyRank(v string) int {
	switch v {
	case "hash_user_identifiers":
		return 1
	case "remove_user_identifiers":
		return 2
	case "remove_user_data":
		return 3
	default:
		return 0
	}
}
func securityRank(v string) int {
	switch v {
	case "detect_secrets":
		return 1
	case "redact_secrets":
		return 2
	case "drop_secret_bearing_records":
		return 3
	default:
		return 0
	}
}
func validAction(a ProbeAction, p Probe, now time.Time) bool {
	if p.ApprovedAt == nil || !one(a.Outcome, "complete", "partial", "overloaded", "denied") || a.StartedAt.IsZero() || a.FinishedAt.Before(a.StartedAt) || a.FinishedAt.After(now) || a.StartedAt.Before(*p.ApprovedAt) || a.FinishedAt.After(p.ExpiresAt) || strings.TrimSpace(a.Provenance) == "" || len(a.Provenance) > 2000 {
		return false
	}
	if a.Outcome == "complete" && len(a.Gaps) > 0 {
		return false
	}
	if a.Outcome != "complete" && len(a.Gaps) == 0 {
		return false
	}
	if len(a.Artifacts) > 20 || len(a.Transformations) == 0 || sensitive(a.Provenance+strings.Join(a.Transformations, " ")+strings.Join(a.Gaps, " ")) {
		return false
	}
	for _, x := range a.Artifacts {
		if !one(x.Kind, "log", "trace", "profile", "snapshot", "diagnostic") || len(x.Digest) != 64 || !hexLower(x.Digest) || x.SizeBytes < 0 || x.SizeBytes > 100*1024*1024 || strings.TrimSpace(x.Reference) == "" || strings.TrimSpace(x.Redaction) == "" || sensitive(x.Reference+x.Redaction) {
			return false
		}
	}
	return true
}
func hexLower(v string) bool {
	b, e := hex.DecodeString(v)
	return e == nil && len(b) == 32 && v == strings.ToLower(v)
}
func sensitive(v string) bool {
	l := strings.ToLower(v)
	for _, marker := range []string{"bearer ", "password=", "secret=", "api_key=", "-----begin private key", "ghp_", "github_pat_", "sk-proj-", "xoxb-"} {
		if strings.Contains(l, marker) {
			return true
		}
	}
	return false
}
