// Package organizations persists accountable groups, membership invitations,
// and accepted repository stewardship changes.
package organizations

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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

// PolicyRules is intentionally explicit: clients can explain every supported
// organization baseline without interpreting an open-ended policy language.
type PolicyRules struct {
	RepositoryVisibility string   `json:"repository_visibility,omitempty"`
	MinimumReviews       int      `json:"minimum_reviews,omitempty"`
	RequiredChecks       []string `json:"required_checks,omitempty"`
	Integration          string   `json:"integration,omitempty"`
	ReleaseProvenance    string   `json:"release_provenance,omitempty"`
	DependencyUse        string   `json:"dependency_use,omitempty"`
	PromotionApprovals   int      `json:"promotion_approvals,omitempty"`
	AgentAuthority       string   `json:"agent_authority,omitempty"`
}

type PolicyTarget struct {
	Kind string `json:"kind"`
	ID   string `json:"id,omitempty"`
}

type Policy struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Version     int            `json:"version"`
	Status      string         `json:"status"`
	Targets     []PolicyTarget `json:"targets"`
	Rules       PolicyRules    `json:"rules"`
	CreatedBy   string         `json:"created_by"`
	CreatedAt   time.Time      `json:"created_at"`
	ActivatedBy string         `json:"activated_by,omitempty"`
	ActivatedAt *time.Time     `json:"activated_at,omitempty"`
	// Activation governs new decisions; existing pulls, releases, deployments,
	// and credentials retain the exact policy evidence they began with.
	AppliesToNewWork bool `json:"applies_to_new_work"`
}

type PolicyException struct {
	ID             string     `json:"id"`
	PolicyID       string     `json:"policy_id"`
	RepositoryID   string     `json:"repository_id"`
	Rule           string     `json:"rule"`
	RequestedValue string     `json:"requested_value"`
	Reason         string     `json:"reason"`
	RequesterID    string     `json:"requester_id"`
	ExpiresAt      time.Time  `json:"expires_at"`
	Status         string     `json:"status"`
	CreatedAt      time.Time  `json:"created_at"`
	DecidedBy      string     `json:"decided_by,omitempty"`
	DecidedAt      *time.Time `json:"decided_at,omitempty"`
}

type InitiativeSource struct {
	Kind         string `json:"kind"`
	RepositoryID string `json:"repository_id,omitempty"`
	ID           string `json:"id"`
}

type InitiativeOwner struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type InitiativeWorkItem struct {
	ID            string            `json:"id"`
	Title         string            `json:"title"`
	RepositoryID  string            `json:"repository_id"`
	Contribution  *InitiativeSource `json:"contribution,omitempty"`
	Owner         InitiativeOwner   `json:"owner"`
	DependencyIDs []string          `json:"dependency_ids"`
	Status        string            `json:"status"`
	Position      int               `json:"position"`
	CreatedBy     string            `json:"created_by"`
	CreatedAt     time.Time         `json:"created_at"`
}

type Initiative struct {
	ID          string               `json:"id"`
	Title       string               `json:"title"`
	Description string               `json:"description,omitempty"`
	Source      InitiativeSource     `json:"source"`
	Status      string               `json:"status"`
	Version     int                  `json:"version"`
	WorkItems   []InitiativeWorkItem `json:"work_items"`
	CreatedBy   string               `json:"created_by"`
	CreatedAt   time.Time            `json:"created_at"`
	UpdatedAt   time.Time            `json:"updated_at"`
}

// StewardshipMandate is a coordination contract. It deliberately contains no
// credential or grant identifier: allowed actions describe expectations and
// never confer repository, Git, review, or merge authority.
type MandateRepository struct {
	RepositoryID string   `json:"repository_id"`
	Branches     []string `json:"branches"`
}

type MandateBudget struct {
	MaxAgentMinutes int `json:"max_agent_minutes"`
	MaxActions      int `json:"max_actions"`
}

type OpportunityPolicy struct {
	EvidenceType    string `json:"evidence_type"`
	MinimumSeverity string `json:"minimum_severity"`
	Mode            string `json:"mode"`
	MaxAgentMinutes int    `json:"max_agent_minutes"`
}

type MandateRevision struct {
	Version                int                 `json:"version"`
	DesiredOutcomes        []string            `json:"desired_outcomes"`
	Repositories           []MandateRepository `json:"repositories"`
	TrustedSignals         []string            `json:"trusted_signals"`
	Exclusions             []string            `json:"exclusions"`
	Budget                 MandateBudget       `json:"budget"`
	StartsAt               time.Time           `json:"starts_at"`
	ExpiresAt              time.Time           `json:"expires_at"`
	AgentID                string              `json:"agent_id"`
	AllowedActions         []string            `json:"allowed_actions"`
	RequiredHumanDecisions []string            `json:"required_human_decisions"`
	OpportunityPolicies    []OpportunityPolicy `json:"opportunity_policies"`
	Reason                 string              `json:"reason"`
	CreatedBy              string              `json:"created_by"`
	CreatedAt              time.Time           `json:"created_at"`
}

type MandateAcceptance struct {
	Version    int       `json:"version"`
	OperatorID string    `json:"operator_id"`
	AcceptedAt time.Time `json:"accepted_at"`
}

type StewardshipMandate struct {
	ID               string                   `json:"id"`
	Title            string                   `json:"title"`
	Version          int                      `json:"version"`
	Status           string                   `json:"status"`
	Revisions        []MandateRevision        `json:"revisions"`
	Acceptance       *MandateAcceptance       `json:"acceptance,omitempty"`
	PausedBy         string                   `json:"paused_by,omitempty"`
	PausedAt         *time.Time               `json:"paused_at,omitempty"`
	RevokedBy        string                   `json:"revoked_by,omitempty"`
	RevokedAt        *time.Time               `json:"revoked_at,omitempty"`
	Opportunities    []StewardshipOpportunity `json:"opportunities"`
	UsedAgentMinutes int                      `json:"used_agent_minutes"`
	UsedActions      int                      `json:"used_actions"`
}

type OpportunityCitation struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Revision   string `json:"revision"`
	Label      string `json:"label"`
	URL        string `json:"url,omitempty"`
	Stale      bool   `json:"stale"`
}

type OpportunityComment struct {
	ID        string    `json:"id"`
	ActorID   string    `json:"actor_id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

type OpportunityApproval struct {
	Decision  string    `json:"decision"`
	ActorID   string    `json:"actor_id"`
	Reason    string    `json:"reason"`
	Version   int       `json:"opportunity_version"`
	CreatedAt time.Time `json:"created_at"`
}

type OpportunityWorkLink struct {
	ProposalID   string    `json:"proposal_id"`
	TaskIDs      []string  `json:"task_ids"`
	BaseRevision string    `json:"base_revision"`
	CreatedBy    string    `json:"created_by"`
	CreatedAt    time.Time `json:"created_at"`
}

// StewardshipOpportunity is reviewable coordination evidence, not an agent
// command. DedupeKey is retained server-side so repeated evaluations converge.
type StewardshipOpportunity struct {
	ID                   string                `json:"id"`
	DedupeKey            string                `json:"dedupe_key"`
	MandateVersion       int                   `json:"mandate_version"`
	RepositoryID         string                `json:"repository_id"`
	EvidenceType         string                `json:"evidence_type"`
	EvidenceID           string                `json:"evidence_id"`
	EvidenceRevision     string                `json:"evidence_revision"`
	Title                string                `json:"title"`
	Summary              string                `json:"summary"`
	Severity             string                `json:"severity"`
	ExpectedValue        string                `json:"expected_value"`
	Confidence           float64               `json:"confidence"`
	AffectedOwnerIDs     []string              `json:"affected_owner_ids"`
	AffectedRevisions    []string              `json:"affected_revisions"`
	Citations            []OpportunityCitation `json:"citations"`
	InScopeReason        string                `json:"in_scope_reason"`
	Status               string                `json:"status"`
	Rank                 int                   `json:"rank"`
	SnoozedUntil         *time.Time            `json:"snoozed_until,omitempty"`
	DecisionReason       string                `json:"decision_reason,omitempty"`
	Version              int                   `json:"version"`
	EvaluationVersion    int                   `json:"evaluation_version"`
	EvaluatedBy          string                `json:"evaluated_by"`
	EvaluatedAt          time.Time             `json:"evaluated_at"`
	UpdatedBy            string                `json:"updated_by"`
	UpdatedAt            time.Time             `json:"updated_at"`
	Comments             []OpportunityComment  `json:"comments"`
	Admission            string                `json:"admission"`
	MaxAgentMinutes      int                   `json:"max_agent_minutes"`
	PolicyFingerprint    string                `json:"policy_fingerprint"`
	ReservedAgentMinutes int                   `json:"reserved_agent_minutes"`
	Blockers             []string              `json:"blockers"`
	Approval             *OpportunityApproval  `json:"approval,omitempty"`
	Work                 *OpportunityWorkLink  `json:"work,omitempty"`
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
	ID                  string               `json:"id"`
	Name                string               `json:"name"`
	Slug                string               `json:"slug"`
	Description         string               `json:"description,omitempty"`
	CreatedBy           string               `json:"created_by"`
	CreatedAt           time.Time            `json:"created_at"`
	Members             []Member             `json:"members"`
	Invitations         []Invitation         `json:"invitations"`
	Transfers           []Transfer           `json:"transfers"`
	Teams               []Team               `json:"teams"`
	Agents              []Agent              `json:"agents"`
	AccessGrants        []AccessGrant        `json:"access_grants"`
	AccessRequests      []AccessRequest      `json:"access_requests"`
	Policies            []Policy             `json:"policies"`
	PolicyExceptions    []PolicyException    `json:"policy_exceptions"`
	Initiatives         []Initiative         `json:"initiatives"`
	StewardshipMandates []StewardshipMandate `json:"stewardship_mandates"`
	Events              []Event              `json:"events"`
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
		created = Organization{ID: id, Name: name, Slug: slug, Description: strings.TrimSpace(description), CreatedBy: actor, CreatedAt: now, Members: []Member{{UserID: actor, Role: "owner", JoinedAt: now}}, Invitations: []Invitation{}, Transfers: []Transfer{}, Teams: []Team{}, Agents: []Agent{}, AccessGrants: []AccessGrant{}, AccessRequests: []AccessRequest{}, Policies: []Policy{}, PolicyExceptions: []PolicyException{}, Initiatives: []Initiative{}, StewardshipMandates: []StewardshipMandate{}, Events: []Event{}}
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
	if v.Policies == nil {
		v.Policies = []Policy{}
	}
	if v.PolicyExceptions == nil {
		v.PolicyExceptions = []PolicyException{}
	}
	if v.Initiatives == nil {
		v.Initiatives = []Initiative{}
	}
	if v.StewardshipMandates == nil {
		v.StewardshipMandates = []StewardshipMandate{}
	}
	for i := range v.StewardshipMandates {
		if v.StewardshipMandates[i].Opportunities == nil {
			v.StewardshipMandates[i].Opportunities = []StewardshipOpportunity{}
		}
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
func (s *Store) RemoveMember(id, actor, user string, beforeCommit func(Organization) error) (Organization, error) {
	return s.mutate(id, func(v *Organization) error {
		if !HasRole(*v, actor, "owner") {
			return ErrNotFound
		}
		for i, m := range v.Members {
			if m.UserID == user {
				if m.Role == "owner" {
					return ErrConflict
				}
				if beforeCommit == nil {
					return ErrInvalid
				}
				if err := beforeCommit(*v); err != nil {
					return err
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
				for i := range v.StewardshipMandates {
					mandate := &v.StewardshipMandates[i]
					if mandate.Acceptance == nil || mandate.Acceptance.OperatorID != user || mandate.Status == "revoked" {
						continue
					}
					mandate.Acceptance = nil
					mandate.PausedBy, mandate.PausedAt = "", nil
					mandate.Status = "pending_acceptance"
					if err := s.event(v, "stewardship_mandate.acceptance.invalidated", actor, mandate.ID, map[string]any{"removed_operator_id": user, "version": mandate.Version}); err != nil {
						return err
					}
				}
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

var policyRuleNames = map[string]bool{
	"repository_visibility": true, "minimum_reviews": true, "required_checks": true,
	"integration": true, "release_provenance": true, "dependency_use": true,
	"promotion_approvals": true, "agent_authority": true,
}

func validPolicyRules(r PolicyRules) bool {
	if r.RepositoryVisibility != "" && r.RepositoryVisibility != "public" && r.RepositoryVisibility != "private" {
		return false
	}
	if r.MinimumReviews < 0 || r.MinimumReviews > 20 || r.PromotionApprovals < 0 || r.PromotionApprovals > 20 {
		return false
	}
	if r.Integration != "" && r.Integration != "direct" && r.Integration != "queue" {
		return false
	}
	if r.ReleaseProvenance != "" && r.ReleaseProvenance != "attested" {
		return false
	}
	if r.DependencyUse != "" && r.DependencyUse != "active-only" && r.DependencyUse != "approved-only" {
		return false
	}
	if r.AgentAuthority != "" && r.AgentAuthority != "explicit-grants" && r.AgentAuthority != "disabled" {
		return false
	}
	checks, ok := normalizeList(r.RequiredChecks, func(v string) bool { _, valid := clean(v, 100); return valid })
	return ok && len(checks) == len(r.RequiredChecks) && (r.RepositoryVisibility != "" || r.MinimumReviews > 0 || len(r.RequiredChecks) > 0 || r.Integration != "" || r.ReleaseProvenance != "" || r.DependencyUse != "" || r.PromotionApprovals > 0 || r.AgentAuthority != "")
}

func validPolicyTargets(v *Organization, targets []PolicyTarget) bool {
	if len(targets) == 0 || len(targets) > 100 {
		return false
	}
	seen := map[string]bool{}
	for _, target := range targets {
		if target.Kind == "organization" {
			if target.ID != "" {
				return false
			}
		} else if target.Kind == "team" {
			if teamIndex(v, target.ID) < 0 {
				return false
			}
		} else if target.Kind == "repository" {
			if !validID(target.ID) {
				return false
			}
		} else {
			return false
		}
		key := target.Kind + ":" + target.ID
		if seen[key] {
			return false
		}
		seen[key] = true
	}
	return true
}

func (s *Store) CreatePolicy(id, actor, name, description string, targets []PolicyTarget, rules PolicyRules) (Organization, error) {
	name, ok := clean(name, 100)
	if !ok || len(description) > 1000 || !validPolicyRules(rules) {
		return Organization{}, ErrInvalid
	}
	return s.mutate(id, func(v *Organization) error {
		if !HasRole(*v, actor, "owner") {
			return ErrNotFound
		}
		if !validPolicyTargets(v, targets) {
			return ErrInvalid
		}
		pid, err := newID()
		if err != nil {
			return err
		}
		now := s.now().Truncate(time.Microsecond)
		v.Policies = append(v.Policies, Policy{ID: pid, Name: name, Description: strings.TrimSpace(description), Version: 1, Status: "draft", Targets: targets, Rules: rules, CreatedBy: actor, CreatedAt: now, AppliesToNewWork: true})
		return s.event(v, "policy.drafted", actor, pid, nil)
	})
}

func (s *Store) ActivatePolicy(id, policyID, actor string, expected int) (Organization, error) {
	return s.mutate(id, func(v *Organization) error {
		if !HasRole(*v, actor, "owner") {
			return ErrNotFound
		}
		for i := range v.Policies {
			p := &v.Policies[i]
			if p.ID != policyID {
				continue
			}
			if p.Status != "draft" || p.Version != expected {
				return ErrConflict
			}
			now := s.now().Truncate(time.Microsecond)
			p.Status, p.ActivatedBy, p.ActivatedAt = "active", actor, &now
			p.Version++
			return s.event(v, "policy.activated", actor, p.ID, map[string]any{"applies_to_new_work": true})
		}
		return ErrNotFound
	})
}

func IsRepositoryMaintainer(v Organization, user, repositoryID string) bool {
	if HasRole(v, user, "owner") {
		return true
	}
	for _, team := range v.Teams {
		responsible := false
		for _, r := range team.Responsibilities {
			if r.RepositoryID == repositoryID {
				responsible = true
			}
		}
		if !responsible {
			continue
		}
		for _, m := range team.Members {
			if m.UserID == user && m.Role == "maintainer" {
				return true
			}
		}
	}
	return false
}

func ResponsibleTeamIDs(v Organization, repositoryID string) []string {
	ids := []string{}
	for _, team := range v.Teams {
		for _, responsibility := range team.Responsibilities {
			if responsibility.RepositoryID == repositoryID {
				ids = append(ids, team.ID)
				break
			}
		}
	}
	return ids
}

func policyApplies(p Policy, repositoryID string, teamIDs []string) bool {
	teams := map[string]bool{}
	for _, id := range teamIDs {
		teams[id] = true
	}
	for _, target := range p.Targets {
		if target.Kind == "organization" || (target.Kind == "repository" && target.ID == repositoryID) || (target.Kind == "team" && teams[target.ID]) {
			return true
		}
	}
	return false
}

func policyDefinesRule(p Policy, rule string) bool {
	switch rule {
	case "repository_visibility":
		return p.Rules.RepositoryVisibility != ""
	case "minimum_reviews":
		return p.Rules.MinimumReviews > 0
	case "required_checks":
		return len(p.Rules.RequiredChecks) > 0
	case "integration":
		return p.Rules.Integration != ""
	case "release_provenance":
		return p.Rules.ReleaseProvenance != ""
	case "dependency_use":
		return p.Rules.DependencyUse != ""
	case "promotion_approvals":
		return p.Rules.PromotionApprovals > 0
	case "agent_authority":
		return p.Rules.AgentAuthority != ""
	}
	return false
}

func (s *Store) RequestPolicyException(id, actor, policyID, repositoryID, rule, value, reason string, expires time.Time) (Organization, error) {
	return s.mutate(id, func(v *Organization) error {
		if !IsRepositoryMaintainer(*v, actor, repositoryID) || !policyRuleNames[rule] || !validID(repositoryID) || !expires.After(s.now()) {
			return ErrInvalid
		}
		if _, ok := clean(value, 200); !ok || !validPolicyExceptionValue(rule, value) {
			return ErrInvalid
		}
		if _, ok := clean(reason, 1000); !ok {
			return ErrInvalid
		}
		found := false
		teamIDs := ResponsibleTeamIDs(*v, repositoryID)
		for _, p := range v.Policies {
			if p.ID == policyID && p.Status == "active" && policyApplies(p, repositoryID, teamIDs) && policyDefinesRule(p, rule) {
				found = true
			}
		}
		if !found {
			return ErrNotFound
		}
		eid, err := newID()
		if err != nil {
			return err
		}
		now := s.now().Truncate(time.Microsecond)
		v.PolicyExceptions = append(v.PolicyExceptions, PolicyException{ID: eid, PolicyID: policyID, RepositoryID: repositoryID, Rule: rule, RequestedValue: strings.TrimSpace(value), Reason: strings.TrimSpace(reason), RequesterID: actor, ExpiresAt: expires.UTC(), Status: "pending", CreatedAt: now})
		return s.event(v, "policy.exception.requested", actor, eid, map[string]any{"policy_id": policyID, "repository_id": repositoryID, "rule": rule})
	})
}
func validPolicyExceptionValue(rule, value string) bool {
	test := PolicyRules{}
	applyPolicyException(&test, PolicyException{Rule: rule, RequestedValue: value})
	switch rule {
	case "repository_visibility":
		return test.RepositoryVisibility == "public" || test.RepositoryVisibility == "private"
	case "minimum_reviews":
		var n int
		count, err := fmt.Sscanf(value, "%d", &n)
		return count == 1 && err == nil && n >= 0 && n <= 20
	case "required_checks":
		return len(test.RequiredChecks) <= 100
	case "integration":
		return test.Integration == "direct" || test.Integration == "queue"
	case "release_provenance":
		return test.ReleaseProvenance == "" || test.ReleaseProvenance == "attested"
	case "dependency_use":
		return test.DependencyUse == "active-only" || test.DependencyUse == "approved-only"
	case "promotion_approvals":
		var n int
		count, err := fmt.Sscanf(value, "%d", &n)
		return count == 1 && err == nil && n >= 0 && n <= 20
	case "agent_authority":
		return test.AgentAuthority == "explicit-grants" || test.AgentAuthority == "disabled"
	}
	return false
}

func (s *Store) DecidePolicyException(id, exceptionID, actor, decision string) (Organization, error) {
	return s.mutate(id, func(v *Organization) error {
		if !HasRole(*v, actor, "owner") || (decision != "approve" && decision != "deny") {
			return ErrNotFound
		}
		for i := range v.PolicyExceptions {
			x := &v.PolicyExceptions[i]
			if x.ID != exceptionID {
				continue
			}
			if x.Status != "pending" || !x.ExpiresAt.After(s.now()) {
				return ErrConflict
			}
			now := s.now().Truncate(time.Microsecond)
			x.Status = decision + "d"
			if decision == "deny" {
				x.Status = "denied"
			}
			x.DecidedBy = actor
			x.DecidedAt = &now
			return s.event(v, "policy.exception."+x.Status, actor, x.ID, nil)
		}
		return ErrNotFound
	})
}

type EffectivePolicy struct {
	RepositoryID  string            `json:"repository_id"`
	Policies      []Policy          `json:"policies"`
	Exceptions    []PolicyException `json:"exceptions"`
	BaselineRules PolicyRules       `json:"baseline_rules"`
	Rules         PolicyRules       `json:"rules"`
}

func EffectivePolicies(v Organization, repositoryID string, teamIDs []string, includeDraft bool, now time.Time) EffectivePolicy {
	out := EffectivePolicy{RepositoryID: repositoryID, Policies: []Policy{}, Exceptions: []PolicyException{}}
	for _, p := range v.Policies {
		if p.Status != "active" && !(includeDraft && p.Status == "draft") {
			continue
		}
		if !policyApplies(p, repositoryID, teamIDs) {
			continue
		}
		out.Policies = append(out.Policies, p)
		mergePolicyRules(&out.BaselineRules, p.Rules)
	}
	selected := map[string]Policy{}
	for _, p := range out.Policies {
		selected[p.ID] = p
	}
	exceptionsByPolicy := map[string][]PolicyException{}
	for _, x := range v.PolicyExceptions {
		policy, validPolicy := selected[x.PolicyID]
		if x.RepositoryID == repositoryID && x.Status == "approved" && x.ExpiresAt.After(now) && validPolicy && policyDefinesRule(policy, x.Rule) {
			out.Exceptions = append(out.Exceptions, x)
			exceptionsByPolicy[x.PolicyID] = append(exceptionsByPolicy[x.PolicyID], x)
		}
	}
	for _, p := range out.Policies {
		contribution := p.Rules
		for _, x := range exceptionsByPolicy[p.ID] {
			applyPolicyException(&contribution, x)
		}
		mergePolicyRules(&out.Rules, contribution)
	}
	return out
}
func applyPolicyException(r *PolicyRules, x PolicyException) {
	switch x.Rule {
	case "repository_visibility":
		r.RepositoryVisibility = x.RequestedValue
	case "minimum_reviews":
		fmt.Sscanf(x.RequestedValue, "%d", &r.MinimumReviews)
	case "required_checks":
		r.RequiredChecks = []string{}
		for _, v := range strings.Split(x.RequestedValue, ",") {
			if v = strings.TrimSpace(v); v != "" {
				r.RequiredChecks = append(r.RequiredChecks, v)
			}
		}
	case "integration":
		r.Integration = x.RequestedValue
	case "release_provenance":
		r.ReleaseProvenance = x.RequestedValue
	case "dependency_use":
		r.DependencyUse = x.RequestedValue
	case "promotion_approvals":
		fmt.Sscanf(x.RequestedValue, "%d", &r.PromotionApprovals)
	case "agent_authority":
		r.AgentAuthority = x.RequestedValue
	}
}
func mergePolicyRules(dst *PolicyRules, src PolicyRules) {
	if src.RepositoryVisibility == "private" {
		dst.RepositoryVisibility = "private"
	} else if dst.RepositoryVisibility == "" {
		dst.RepositoryVisibility = src.RepositoryVisibility
	}
	if src.MinimumReviews > dst.MinimumReviews {
		dst.MinimumReviews = src.MinimumReviews
	}
	dst.RequiredChecks, _ = normalizeList(append(dst.RequiredChecks, src.RequiredChecks...), func(v string) bool { return v != "" })
	if src.Integration == "queue" {
		dst.Integration = "queue"
	} else if dst.Integration == "" {
		dst.Integration = src.Integration
	}
	if src.ReleaseProvenance != "" {
		dst.ReleaseProvenance = src.ReleaseProvenance
	}
	if src.DependencyUse == "approved-only" || dst.DependencyUse == "" {
		dst.DependencyUse = src.DependencyUse
	}
	if src.PromotionApprovals > dst.PromotionApprovals {
		dst.PromotionApprovals = src.PromotionApprovals
	}
	if src.AgentAuthority == "disabled" {
		dst.AgentAuthority = "disabled"
	} else if dst.AgentAuthority == "" {
		dst.AgentAuthority = src.AgentAuthority
	}
}

func mandateIndex(v *Organization, id string) int {
	for i := range v.StewardshipMandates {
		if v.StewardshipMandates[i].ID == id {
			return i
		}
	}
	return -1
}

func validateMandateRevision(v *Organization, r MandateRevision, now time.Time) bool {
	if agentIndex(v, r.AgentID) < 0 || len(r.DesiredOutcomes) == 0 || len(r.DesiredOutcomes) > 20 || len(r.Repositories) == 0 || len(r.Repositories) > 50 || len(r.TrustedSignals) == 0 || len(r.TrustedSignals) > 50 || len(r.Exclusions) == 0 || len(r.Exclusions) > 50 || len(r.AllowedActions) == 0 || len(r.AllowedActions) > 20 || len(r.RequiredHumanDecisions) == 0 || len(r.RequiredHumanDecisions) > 20 || r.Budget.MaxAgentMinutes < 1 || r.Budget.MaxAgentMinutes > 525600 || r.Budget.MaxActions < 1 || r.Budget.MaxActions > 100000 || !r.ExpiresAt.After(now) || !r.ExpiresAt.After(r.StartsAt) || r.ExpiresAt.After(r.StartsAt.Add(366*24*time.Hour)) {
		return false
	}
	validateText := func(values []string, max int) bool {
		_, ok := normalizeList(values, func(x string) bool { _, valid := clean(x, max); return valid })
		return ok
	}
	if !validateText(r.DesiredOutcomes, 1000) || !validateText(r.TrustedSignals, 300) || !validateText(r.Exclusions, 1000) || !validateText(r.AllowedActions, 100) || !validateText(r.RequiredHumanDecisions, 500) {
		return false
	}
	seenPolicies := map[string]bool{}
	for _, policy := range r.OpportunityPolicies {
		if !slices.Contains([]string{"repository", "dependency", "check", "release", "incident", "security", "usage"}, policy.EvidenceType) || !slices.Contains([]string{"critical", "high", "medium", "low"}, policy.MinimumSeverity) || !slices.Contains([]string{"approval_required", "auto_start"}, policy.Mode) || policy.MaxAgentMinutes < 0 || policy.MaxAgentMinutes > r.Budget.MaxAgentMinutes || seenPolicies[policy.EvidenceType] {
			return false
		}
		seenPolicies[policy.EvidenceType] = true
	}
	seen := map[string]bool{}
	for _, scope := range r.Repositories {
		if !validID(scope.RepositoryID) || seen[scope.RepositoryID] || len(scope.Branches) == 0 || len(scope.Branches) > 100 {
			return false
		}
		seen[scope.RepositoryID] = true
		if !validateText(scope.Branches, 255) {
			return false
		}
		for _, branch := range scope.Branches {
			if strings.HasPrefix(branch, "-") || strings.ContainsAny(branch, " ~^:?*[\\") || strings.Contains(branch, "..") || strings.Contains(branch, "@{") {
				return false
			}
		}
	}
	return len(r.Reason) <= 1000
}

func (s *Store) CreateStewardshipMandate(id, actor, title string, revision MandateRevision) (Organization, StewardshipMandate, error) {
	title, ok := clean(title, 200)
	if !ok {
		return Organization{}, StewardshipMandate{}, ErrInvalid
	}
	var created StewardshipMandate
	v, err := s.mutate(id, func(v *Organization) error {
		if !HasRole(*v, actor, "owner") || revision.StartsAt.Before(s.now().Add(-time.Minute)) || !validateMandateRevision(v, revision, s.now()) {
			return ErrInvalid
		}
		mid, e := newID()
		if e != nil {
			return e
		}
		now := s.now().Truncate(time.Microsecond)
		revision.Version, revision.CreatedBy, revision.CreatedAt = 1, actor, now
		revision.Reason = strings.TrimSpace(revision.Reason)
		created = StewardshipMandate{ID: mid, Title: title, Version: 1, Status: "pending_acceptance", Revisions: []MandateRevision{revision}, Opportunities: []StewardshipOpportunity{}}
		v.StewardshipMandates = append(v.StewardshipMandates, created)
		return s.event(v, "stewardship_mandate.created", actor, mid, map[string]any{"version": 1, "agent_id": revision.AgentID})
	})
	return v, created, err
}

func (s *Store) ReviseStewardshipMandate(id, mandateID, actor string, expected int, revision MandateRevision) (Organization, StewardshipMandate, error) {
	var out StewardshipMandate
	v, err := s.mutate(id, func(v *Organization) error {
		if !HasRole(*v, actor, "owner") || !validateMandateRevision(v, revision, s.now()) {
			return ErrInvalid
		}
		i := mandateIndex(v, mandateID)
		if i < 0 {
			return ErrNotFound
		}
		m := &v.StewardshipMandates[i]
		if m.Version != expected || m.Status == "revoked" {
			return ErrConflict
		}
		now := s.now().Truncate(time.Microsecond)
		m.Version++
		revision.Version, revision.CreatedBy, revision.CreatedAt = m.Version, actor, now
		m.Revisions = append(m.Revisions, revision)
		m.Status = "pending_acceptance"
		m.Acceptance = nil
		m.PausedBy = ""
		m.PausedAt = nil
		out = *m
		return s.event(v, "stewardship_mandate.revised", actor, mandateID, map[string]any{"version": m.Version})
	})
	return v, out, err
}

func (s *Store) AcceptStewardshipMandate(id, mandateID, actor string, expected int) (Organization, StewardshipMandate, error) {
	var out StewardshipMandate
	v, err := s.mutate(id, func(v *Organization) error {
		i := mandateIndex(v, mandateID)
		if i < 0 {
			return ErrNotFound
		}
		m := &v.StewardshipMandates[i]
		if m.Version != expected || m.Status != "pending_acceptance" {
			return ErrConflict
		}
		latest := m.Revisions[len(m.Revisions)-1]
		i = agentIndex(v, latest.AgentID)
		if i < 0 {
			return ErrNotFound
		}
		agent := v.Agents[i]
		if !slices.Contains(agent.OperatorIDs, actor) || !latest.ExpiresAt.After(s.now()) {
			return ErrNotFound
		}
		now := s.now().Truncate(time.Microsecond)
		m.Acceptance = &MandateAcceptance{Version: m.Version, OperatorID: actor, AcceptedAt: now}
		m.Status = "active"
		out = *m
		if err := s.event(v, "stewardship_mandate.accepted", actor, mandateID, map[string]any{"version": m.Version}); err != nil {
			return err
		}
		return s.event(v, "stewardship_evaluation.requested", actor, mandateID, map[string]any{"version": m.Version, "reason": "activation"})
	})
	return v, out, err
}

func (s *Store) ChangeStewardshipMandateState(id, mandateID, actor, action string, expected int) (Organization, StewardshipMandate, error) {
	var out StewardshipMandate
	v, err := s.mutate(id, func(v *Organization) error {
		if !HasRole(*v, actor, "owner") {
			return ErrNotFound
		}
		i := mandateIndex(v, mandateID)
		if i < 0 {
			return ErrNotFound
		}
		m := &v.StewardshipMandates[i]
		if m.Version != expected {
			return ErrConflict
		}
		now := s.now().Truncate(time.Microsecond)
		if action != "revoke" && !m.Revisions[len(m.Revisions)-1].ExpiresAt.After(now) {
			return ErrConflict
		}
		switch action {
		case "pause":
			if m.Status != "active" {
				return ErrConflict
			}
			m.Status, m.PausedBy, m.PausedAt = "paused", actor, &now
		case "resume":
			latest := m.Revisions[len(m.Revisions)-1]
			i := agentIndex(v, latest.AgentID)
			if m.Status != "paused" || m.Acceptance == nil || m.Acceptance.Version != m.Version || i < 0 || !slices.Contains(v.Agents[i].OperatorIDs, m.Acceptance.OperatorID) || !latest.ExpiresAt.After(now) {
				return ErrConflict
			}
			m.Status, m.PausedBy, m.PausedAt = "active", "", nil
		case "revoke":
			if m.Status == "revoked" {
				out = *m
				return nil
			}
			m.Status, m.RevokedBy, m.RevokedAt = "revoked", actor, &now
		default:
			return ErrInvalid
		}
		out = *m
		return s.event(v, "stewardship_mandate."+action+"d", actor, mandateID, map[string]any{"version": m.Version})
	})
	return v, out, err
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
			if resource.Kind == "repository" && EffectivePolicies(*v, resource.ID, ResponsibleTeamIDs(*v, resource.ID), false, now).Rules.AgentAuthority == "disabled" {
				return ErrConflict
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

func validInitiativeSource(source InitiativeSource) bool {
	if !validID(source.ID) || (source.RepositoryID != "" && !validID(source.RepositoryID)) {
		return false
	}
	return slices.Contains([]string{"proposal", "evolution", "incident", "security"}, source.Kind)
}

// CreateInitiative publishes the planning map only after the caller has
// revalidated every referenced repository and source through authoritative
// workflow stores. Source records remain authoritative for their own state.
func (s *Store) CreateInitiative(id, actor, title, description string, source InitiativeSource, items []InitiativeWorkItem, validate func(Organization, InitiativeSource, []InitiativeWorkItem) error) (Organization, Initiative, error) {
	title, ok := clean(title, 200)
	if !ok || len(description) > 2000 || !validInitiativeSource(source) || len(items) == 0 || len(items) > 100 {
		return Organization{}, Initiative{}, ErrInvalid
	}
	seen := map[string]bool{}
	for i := range items {
		itemTitle, valid := clean(items[i].Title, 200)
		if !valid || !validID(items[i].RepositoryID) || !slices.Contains([]string{"human", "team", "agent"}, items[i].Owner.Type) || !validID(items[i].Owner.ID) || !slices.Contains([]string{"todo", "in_progress", "completed"}, items[i].Status) {
			return Organization{}, Initiative{}, ErrInvalid
		}
		items[i].Title = itemTitle
		if items[i].ID == "" {
			generated, err := newID()
			if err != nil {
				return Organization{}, Initiative{}, err
			}
			items[i].ID = generated
		}
		if !validID(items[i].ID) || seen[items[i].ID] || (items[i].Contribution != nil && !validInitiativeSource(*items[i].Contribution)) {
			return Organization{}, Initiative{}, ErrInvalid
		}
		seen[items[i].ID] = true
		items[i].Position = i + 1
		if items[i].DependencyIDs == nil {
			items[i].DependencyIDs = []string{}
		}
	}
	for _, item := range items {
		for _, dependencyID := range item.DependencyIDs {
			if !seen[dependencyID] || dependencyID == item.ID {
				return Organization{}, Initiative{}, ErrInvalid
			}
		}
	}
	dependencies := map[string][]string{}
	for _, item := range items {
		dependencies[item.ID] = item.DependencyIDs
	}
	state := map[string]uint8{}
	var cyclic func(string) bool
	cyclic = func(id string) bool {
		if state[id] == 1 {
			return true
		}
		if state[id] == 2 {
			return false
		}
		state[id] = 1
		for _, dependencyID := range dependencies[id] {
			if cyclic(dependencyID) {
				return true
			}
		}
		state[id] = 2
		return false
	}
	for id := range dependencies {
		if cyclic(id) {
			return Organization{}, Initiative{}, ErrInvalid
		}
	}
	var created Initiative
	v, err := s.mutate(id, func(v *Organization) error {
		if !HasRole(*v, actor, "") {
			return ErrNotFound
		}
		if validate != nil {
			if err := validate(*v, source, items); err != nil {
				return err
			}
		}
		now := s.now().Truncate(time.Microsecond)
		iid, err := newID()
		if err != nil {
			return err
		}
		for i := range items {
			items[i].CreatedBy, items[i].CreatedAt = actor, now
		}
		created = Initiative{ID: iid, Title: title, Description: strings.TrimSpace(description), Source: source, Status: "active", Version: 1, WorkItems: items, CreatedBy: actor, CreatedAt: now, UpdatedAt: now}
		v.Initiatives = append(v.Initiatives, created)
		return s.event(v, "initiative.created", actor, iid, map[string]any{"source_kind": source.Kind, "source_id": source.ID})
	})
	return v, created, err
}

func (s *Store) UpdateInitiativeItem(id, initiativeID, itemID, actor string, owner InitiativeOwner, status string, expected int, validate func(Organization, InitiativeWorkItem) error) (Organization, error) {
	if !validID(initiativeID) || !validID(itemID) || !validID(owner.ID) || !slices.Contains([]string{"human", "team", "agent"}, owner.Type) || !slices.Contains([]string{"todo", "in_progress", "completed"}, status) {
		return Organization{}, ErrInvalid
	}
	return s.mutate(id, func(v *Organization) error {
		if !HasRole(*v, actor, "") {
			return ErrNotFound
		}
		for i := range v.Initiatives {
			initiative := &v.Initiatives[i]
			if initiative.ID != initiativeID {
				continue
			}
			if initiative.Version != expected {
				return ErrConflict
			}
			for j := range initiative.WorkItems {
				item := &initiative.WorkItems[j]
				if item.ID != itemID {
					continue
				}
				candidate := *item
				candidate.Owner, candidate.Status = owner, status
				if validate != nil {
					if err := validate(*v, candidate); err != nil {
						return err
					}
				}
				item.Owner, item.Status = owner, status
				initiative.Version++
				initiative.UpdatedAt = s.now().Truncate(time.Microsecond)
				return s.event(v, "initiative.item.updated", actor, itemID, map[string]any{"initiative_id": initiativeID, "owner_type": owner.Type, "owner_id": owner.ID, "status": status})
			}
			return ErrNotFound
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
