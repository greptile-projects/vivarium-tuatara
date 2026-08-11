// Package deliveryteams persists the operating contract for temporary outcome teams.
package deliveryteams

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
	CanRespond     bool                `json:"can_respond"`
}
type Event struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	ActorID   string    `json:"actor_id"`
	Summary   string    `json:"summary"`
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
}
type RevisionScope struct {
	RepositoryID string   `json:"repository_id"`
	Reference    string   `json:"reference"`
	Revision     string   `json:"revision"`
	Paths        []string `json:"paths"`
}
type WorkInput struct {
	Name           string `json:"name"`
	SourceStreamID string `json:"source_stream_id,omitempty"`
	RepositoryID   string `json:"repository_id,omitempty"`
	Revision       string `json:"revision,omitempty"`
	Artifact       string `json:"artifact"`
}
type WorkStream struct {
	ID                 string          `json:"id"`
	Title              string          `json:"title"`
	OwnerParticipantID string          `json:"owner_participant_id"`
	Inputs             []WorkInput     `json:"inputs"`
	ExpectedArtifacts  []string        `json:"expected_artifacts"`
	DependencyIDs      []string        `json:"dependency_ids"`
	AcceptanceCriteria []string        `json:"acceptance_criteria"`
	RepositoryScope    []RevisionScope `json:"repository_scope"`
	IntegrationOrder   int             `json:"integration_order"`
	Budget             *Budget         `json:"budget,omitempty"`
	Assumptions        []string        `json:"assumptions"`
}
type PlanAcceptance struct {
	ParticipantID string     `json:"participant_id"`
	Revision      int        `json:"revision"`
	Status        string     `json:"status"`
	Required      bool       `json:"required"`
	RespondedBy   string     `json:"responded_by,omitempty"`
	RespondedAt   *time.Time `json:"responded_at,omitempty"`
	CanRespond    bool       `json:"can_respond"`
}
type PlanBlocker struct {
	Kind                string   `json:"kind"`
	StreamIDs           []string `json:"stream_ids"`
	OwnerParticipantIDs []string `json:"owner_participant_ids"`
	Summary             string   `json:"summary"`
}
type ExecutionPlan struct {
	Revision    int              `json:"revision"`
	Streams     []WorkStream     `json:"streams"`
	Acceptances []PlanAcceptance `json:"acceptances"`
	Blockers    []PlanBlocker    `json:"blockers"`
	ProposedBy  string           `json:"proposed_by"`
	UpdatedAt   time.Time        `json:"updated_at"`
}
type Team struct {
	ID             string          `json:"id"`
	RepositoryID   string          `json:"repository_id"`
	Outcome        Outcome         `json:"outcome"`
	Name           string          `json:"name"`
	Purpose        string          `json:"purpose"`
	OrganizerID    string          `json:"organizer_id"`
	OverallBudget  *Budget         `json:"overall_budget,omitempty"`
	Deadline       *time.Time      `json:"deadline,omitempty"`
	EscalationPath string          `json:"escalation_path"`
	Version        int             `json:"version"`
	Participants   []Participant   `json:"participants"`
	Events         []Event         `json:"events"`
	Plan           *ExecutionPlan  `json:"plan,omitempty"`
	PlanHistory    []ExecutionPlan `json:"plan_history"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}
type Charter struct {
	Name           string        `json:"name"`
	Purpose        string        `json:"purpose"`
	OverallBudget  *Budget       `json:"overall_budget,omitempty"`
	Deadline       *time.Time    `json:"deadline,omitempty"`
	EscalationPath string        `json:"escalation_path"`
	Participants   []Participant `json:"participants"`
}
type PlanInput struct {
	Streams []WorkStream `json:"streams"`
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

func sameInvitationTerms(a, b Participant) bool {
	if a.PrincipalType != b.PrincipalType || a.PrincipalID != b.PrincipalID ||
		a.Role != b.Role || a.Responsibility != b.Responsibility || a.Why != b.Why ||
		a.Escalation != b.Escalation || !sameBudget(a.Budget, b.Budget) || !sameTime(a.Deadline, b.Deadline) ||
		len(a.RequiredAccess) != len(b.RequiredAccess) {
		return false
	}
	for i := range a.RequiredAccess {
		if a.RequiredAccess[i] != b.RequiredAccess[i] {
			return false
		}
	}
	return true
}

func sameSharedCharterTerms(t Team, c Charter) bool {
	if t.Name != c.Name || t.Purpose != c.Purpose || t.EscalationPath != c.EscalationPath ||
		!sameBudget(t.OverallBudget, c.OverallBudget) || !sameTime(t.Deadline, c.Deadline) ||
		len(t.Participants) != len(c.Participants) {
		return false
	}
	principals := map[string]bool{}
	for _, p := range t.Participants {
		principals[p.PrincipalType+":"+p.PrincipalID] = true
	}
	for _, p := range c.Participants {
		if !principals[p.PrincipalType+":"+p.PrincipalID] {
			return false
		}
	}
	return true
}

func sameBudget(a, b *Budget) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

func sameTime(a, b *time.Time) bool {
	return a == nil && b == nil || a != nil && b != nil && a.Equal(*b)
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
	t := Team{ID: i, RepositoryID: repositoryID, Outcome: outcome, Name: c.Name, Purpose: c.Purpose, OrganizerID: actor, OverallBudget: c.OverallBudget, Deadline: c.Deadline, EscalationPath: c.EscalationPath, Version: 1, Participants: c.Participants, PlanHistory: []ExecutionPlan{}, CreatedAt: now, UpdatedAt: now}
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
	sharedTermsUnchanged := sameSharedCharterTerms(t, c)
	planAffected := !sharedTermsUnchanged
	old := map[string]Participant{}
	for _, p := range t.Participants {
		old[p.PrincipalType+":"+p.PrincipalID] = p
	}
	for i := range c.Participants {
		p := &c.Participants[i]
		if prior, ok := old[p.PrincipalType+":"+p.PrincipalID]; ok {
			if sharedTermsUnchanged && sameInvitationTerms(prior, *p) {
				p.Status = prior.Status
				p.InvitedBy = prior.InvitedBy
				p.InvitedAt = prior.InvitedAt
				p.RespondedBy = prior.RespondedBy
				p.RespondedAt = prior.RespondedAt
			} else {
				planAffected = true
				p.Status = "pending"
				p.InvitedBy = actor
				p.InvitedAt = now
				p.RespondedBy = ""
				p.RespondedAt = nil
			}
		} else {
			planAffected = true
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
	if t.Plan != nil && planAffected {
		t.PlanHistory = append(t.PlanHistory, clonePlan(*t.Plan))
		t.Plan.Revision++
		t.Plan.UpdatedAt = now
		t.Plan.ProposedBy = actor
		t.Plan.Acceptances = planAcceptances(t, t.Plan.Streams, t.Plan.Revision, "")
		t.Plan.Blockers = planBlockers(t, t.Plan.Streams, t.Plan.Acceptances)
		t.Plan.Blockers = append(t.Plan.Blockers, PlanBlocker{Kind: "charter_changed", Summary: "The team charter changed an upstream planning assumption"})
	}
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
	if t.Plan != nil {
		t.Plan.Blockers = planBlockers(t, t.Plan.Streams, t.Plan.Acceptances)
	}
	t.Version++
	t.UpdatedAt = now
	t.Events = append(t.Events, event("invitation."+decision, actor, "Responded to the delivery-team invitation", t.Version, now))
	return t, s.write(t)
}

func cleanList(values []string) ([]string, bool) {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return nil, false
		}
		seen[value] = true
		out = append(out, value)
	}
	return out, true
}

func validRevision(value string) bool {
	if len(value) != 40 {
		return false
	}
	for _, r := range value {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

func validatePlan(t Team, input PlanInput) ([]WorkStream, error) {
	if len(input.Streams) == 0 {
		return nil, ErrInvalid
	}
	participants := map[string]Participant{}
	for _, p := range t.Participants {
		participants[p.ID] = p
	}
	ids := map[string]bool{}
	orders := map[int]bool{}
	for i := range input.Streams {
		x := &input.Streams[i]
		x.Title = strings.TrimSpace(x.Title)
		owner, ok := participants[x.OwnerParticipantID]
		if x.ID == "" || ids[x.ID] || x.Title == "" || !ok || owner.Status == "declined" || x.IntegrationOrder < 1 || orders[x.IntegrationOrder] || !validBudget(x.Budget) {
			return nil, ErrInvalid
		}
		ids[x.ID], orders[x.IntegrationOrder] = true, true
		var valid bool
		if x.ExpectedArtifacts, valid = cleanList(x.ExpectedArtifacts); !valid {
			return nil, ErrInvalid
		}
		if x.AcceptanceCriteria, valid = cleanList(x.AcceptanceCriteria); !valid {
			return nil, ErrInvalid
		}
		if x.Assumptions, valid = cleanList(x.Assumptions); !valid {
			return nil, ErrInvalid
		}
		if len(x.RepositoryScope) == 0 {
			return nil, ErrInvalid
		}
		for j := range x.RepositoryScope {
			scope := &x.RepositoryScope[j]
			scope.Reference, scope.Revision = strings.TrimSpace(scope.Reference), strings.TrimSpace(scope.Revision)
			if scope.RepositoryID == "" || scope.Reference == "" || !validRevision(scope.Revision) {
				return nil, ErrInvalid
			}
			if scope.Paths, valid = cleanList(scope.Paths); !valid {
				return nil, ErrInvalid
			}
		}
		for j := range x.Inputs {
			in := &x.Inputs[j]
			in.Name, in.Artifact, in.Revision = strings.TrimSpace(in.Name), strings.TrimSpace(in.Artifact), strings.TrimSpace(in.Revision)
			if in.Name == "" || in.Artifact == "" || (in.SourceStreamID == "" && (in.RepositoryID == "" || !validRevision(in.Revision))) {
				return nil, ErrInvalid
			}
		}
	}
	for _, x := range input.Streams {
		for _, d := range x.DependencyIDs {
			if !ids[d] || d == x.ID {
				return nil, ErrInvalid
			}
		}
		for _, in := range x.Inputs {
			if in.SourceStreamID != "" && !ids[in.SourceStreamID] {
				return nil, ErrInvalid
			}
		}
	}
	visiting, visited := map[string]bool{}, map[string]bool{}
	byID := map[string]WorkStream{}
	for _, x := range input.Streams {
		byID[x.ID] = x
	}
	var visit func(string) bool
	visit = func(id string) bool {
		if visiting[id] {
			return false
		}
		if visited[id] {
			return true
		}
		visiting[id] = true
		for _, d := range byID[id].DependencyIDs {
			if !visit(d) {
				return false
			}
		}
		visiting[id] = false
		visited[id] = true
		return true
	}
	for id := range byID {
		if !visit(id) {
			return nil, ErrInvalid
		}
	}
	for _, stream := range input.Streams {
		for _, dependencyID := range stream.DependencyIDs {
			if byID[dependencyID].IntegrationOrder >= stream.IntegrationOrder {
				return nil, ErrInvalid
			}
		}
		for _, workInput := range stream.Inputs {
			if workInput.SourceStreamID != "" {
				source := byID[workInput.SourceStreamID]
				if !slices.Contains(stream.DependencyIDs, source.ID) || !slices.Contains(source.ExpectedArtifacts, workInput.Artifact) {
					return nil, ErrInvalid
				}
			}
		}
	}
	slices.SortFunc(input.Streams, func(a, b WorkStream) int { return a.IntegrationOrder - b.IntegrationOrder })
	return input.Streams, nil
}

func planBlockers(t Team, streams []WorkStream, acceptances []PlanAcceptance) []PlanBlocker {
	out := []PlanBlocker{}
	for _, a := range acceptances {
		if a.Required && a.Status != "accepted" {
			out = append(out, PlanBlocker{Kind: "replan_acceptance", OwnerParticipantIDs: []string{a.ParticipantID}, Summary: "The affected owner must accept this material plan revision"})
		}
	}
	if t.OverallBudget != nil {
		total := 0
		for _, x := range streams {
			if x.Budget != nil && x.Budget.Unit == t.OverallBudget.Unit {
				total += x.Budget.Limit
			}
		}
		if total > t.OverallBudget.Limit {
			out = append(out, PlanBlocker{Kind: "budget_exceeded", Summary: "Planned stream budgets exceed the team budget; the organizer must replan"})
		}
	}
	for i, a := range streams {
		p := participantByID(t, a.OwnerParticipantID)
		if p == nil || p.Status == "declined" {
			out = append(out, PlanBlocker{Kind: "owner_unavailable", StreamIDs: []string{a.ID}, OwnerParticipantIDs: []string{a.OwnerParticipantID}, Summary: "The planned stream owner is no longer available in the accepted charter"})
		} else if p.Budget != nil && a.Budget != nil && p.Budget.Unit == a.Budget.Unit && a.Budget.Limit > p.Budget.Limit {
			out = append(out, PlanBlocker{Kind: "budget_exceeded", StreamIDs: []string{a.ID}, OwnerParticipantIDs: []string{a.OwnerParticipantID}, Summary: "The stream exceeds its owner's accepted budget"})
		}
		for j := i + 1; j < len(streams); j++ {
			b := streams[j]
			overlap := false
			for _, ar := range a.RepositoryScope {
				for _, br := range b.RepositoryScope {
					if ar.RepositoryID == br.RepositoryID {
						for _, ap := range ar.Paths {
							for _, bp := range br.Paths {
								if ap == bp || strings.HasPrefix(ap, bp+"/") || strings.HasPrefix(bp, ap+"/") {
									overlap = true
								}
							}
						}
					}
				}
			}
			duplicate := false
			for _, aa := range a.ExpectedArtifacts {
				if slices.Contains(b.ExpectedArtifacts, aa) {
					duplicate = true
				}
			}
			if overlap {
				out = append(out, PlanBlocker{Kind: "overlapping_scope", StreamIDs: []string{a.ID, b.ID}, OwnerParticipantIDs: []string{a.OwnerParticipantID, b.OwnerParticipantID}, Summary: "Parallel streams claim overlapping repository paths"})
			}
			if duplicate {
				out = append(out, PlanBlocker{Kind: "duplicate_artifact", StreamIDs: []string{a.ID, b.ID}, OwnerParticipantIDs: []string{a.OwnerParticipantID, b.OwnerParticipantID}, Summary: "Parallel streams promise the same artifact"})
			}
		}
	}
	return out
}

func planAcceptances(t Team, streams []WorkStream, revision int, acceptedPrincipal string) []PlanAcceptance {
	required := map[string]bool{}
	for _, x := range streams {
		required[x.OwnerParticipantID] = true
	}
	out := []PlanAcceptance{}
	for _, p := range t.Participants {
		if required[p.ID] {
			status := "pending"
			if p.PrincipalID == acceptedPrincipal {
				status = "accepted"
			}
			out = append(out, PlanAcceptance{ParticipantID: p.ID, Revision: revision, Status: status, Required: true})
		}
	}
	return out
}

func clonePlan(plan ExecutionPlan) ExecutionPlan {
	data, _ := json.Marshal(plan)
	var cloned ExecutionPlan
	_ = json.Unmarshal(data, &cloned)
	return cloned
}
func participantByID(t Team, id string) *Participant {
	for i := range t.Participants {
		if t.Participants[i].ID == id {
			return &t.Participants[i]
		}
	}
	return nil
}

func (s *Store) PutPlan(teamID, actor, actingPrincipal string, expectedVersion int, input PlanInput) (Team, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := s.read(teamID)
	if err != nil {
		return t, err
	}
	if t.Version != expectedVersion {
		return t, ErrConflict
	}
	streams, err := validatePlan(t, input)
	if err != nil {
		return t, err
	}
	allowed := actor == t.OrganizerID
	for _, p := range t.Participants {
		if p.Status == "accepted" && p.PrincipalID == actingPrincipal {
			allowed = true
		}
	}
	if !allowed {
		return t, ErrForbidden
	}
	revision := 1
	if t.Plan != nil {
		t.PlanHistory = append(t.PlanHistory, clonePlan(*t.Plan))
		revision = t.Plan.Revision + 1
	}
	acceptances := planAcceptances(t, streams, revision, actingPrincipal)
	now := s.now()
	plan := &ExecutionPlan{Revision: revision, Streams: streams, Acceptances: acceptances, ProposedBy: actor, UpdatedAt: now}
	plan.Blockers = planBlockers(t, streams, acceptances)
	t.Plan = plan
	t.Version++
	t.UpdatedAt = now
	t.Events = append(t.Events, event("plan.revised", actor, "Proposed revision-bound parallel work streams", t.Version, now))
	return t, s.write(t)
}

func (s *Store) RespondPlan(teamID, participantID, actor, actingPrincipal, decision string, expectedVersion, expectedRevision int) (Team, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := s.read(teamID)
	if err != nil {
		return t, err
	}
	if t.Version != expectedVersion || t.Plan == nil || t.Plan.Revision != expectedRevision {
		return t, ErrConflict
	}
	if decision != "accepted" && decision != "declined" {
		return t, ErrInvalid
	}
	found := false
	now := s.now()
	for i := range t.Plan.Acceptances {
		a := &t.Plan.Acceptances[i]
		if a.ParticipantID == participantID && a.Required {
			p := participantByID(t, participantID)
			if p == nil || p.PrincipalID != actingPrincipal {
				return t, ErrForbidden
			}
			a.Status = decision
			a.RespondedBy = actor
			a.RespondedAt = &now
			found = true
		}
	}
	if !found {
		return t, ErrForbidden
	}
	t.Plan.Blockers = planBlockers(t, t.Plan.Streams, t.Plan.Acceptances)
	t.Version++
	t.UpdatedAt = now
	t.Events = append(t.Events, event("plan."+decision, actor, "Responded to material execution replanning", t.Version, now))
	return t, s.write(t)
}
