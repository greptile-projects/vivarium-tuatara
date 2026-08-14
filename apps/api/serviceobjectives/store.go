// Package serviceobjectives persists repository reliability contracts.
package serviceobjectives

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var ErrNotFound = errors.New("service objective not found")
var ErrInvalid = errors.New("invalid service objective")
var ErrConflict = errors.New("service objective version conflict")
var ErrMappingNotFound = errors.New("signal mapping not found")

type Scope struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id,omitempty"`
	Name       string `json:"name"`
}
type Indicator struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Signal      string `json:"signal,omitempty"`
	Calculation string `json:"calculation"`
	Unit        string `json:"unit"`
	GoodEvent   string `json:"good_event"`
	TotalEvent  string `json:"total_event"`
}
type Window struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Duration string `json:"duration"`
	Rolling  bool   `json:"rolling"`
}
type Objective struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	IndicatorID string   `json:"indicator_id"`
	WindowID    string   `json:"window_id"`
	Target      float64  `json:"target"`
	Comparator  string   `json:"comparator"`
	JourneyIDs  []string `json:"journey_ids"`
	OwnerIDs    []string `json:"owner_ids"`
}
type Journey struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	OwnerIDs    []string `json:"owner_ids"`
}
type Dependency struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Kind         string   `json:"kind"`
	ResourceID   string   `json:"resource_id,omitempty"`
	OwnerIDs     []string `json:"owner_ids"`
	ObjectiveIDs []string `json:"objective_ids"`
}
type ErrorBudget struct {
	ObjectiveID    string  `json:"objective_id"`
	AllowedFailure float64 `json:"allowed_failure"`
	Unit           string  `json:"unit"`
	BurnPolicy     string  `json:"burn_policy"`
}
type Severity struct {
	Level                 string   `json:"level"`
	BudgetConsumedPercent float64  `json:"budget_consumed_percent"`
	Response              string   `json:"response"`
	OwnerIDs              []string `json:"owner_ids"`
}
type CommitmentLink struct {
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	Version int    `json:"version"`
}
type ExceptionPolicy struct {
	MaximumDuration  string   `json:"maximum_duration"`
	ApprovalOwnerIDs []string `json:"approval_owner_ids"`
	FollowUpRequired bool     `json:"follow_up_required"`
}
type Exception struct {
	ID           string    `json:"id"`
	ObjectiveIDs []string  `json:"objective_ids"`
	Reason       string    `json:"reason"`
	ApprovedBy   string    `json:"approved_by"`
	ExpiresAt    time.Time `json:"expires_at"`
	FollowUp     string    `json:"follow_up,omitempty"`
}
type Diagnostic struct {
	Kind         string `json:"kind"`
	Severity     string `json:"severity"`
	Message      string `json:"message"`
	ResourceID   string `json:"resource_id,omitempty"`
	AttributedTo string `json:"attributed_to"`
}
type SignalSource struct {
	Kind         string `json:"kind"`
	Name         string `json:"name"`
	Reference    string `json:"reference"`
	Visibility   string `json:"visibility"`
	Sanitization string `json:"sanitization"`
}
type SignalMappingRevision struct {
	Version                 int            `json:"version"`
	ContractVersion         int            `json:"contract_version"`
	ObjectiveID             string         `json:"objective_id"`
	InstrumentationRevision string         `json:"instrumentation_revision"`
	Sources                 []SignalSource `json:"sources"`
	Calculation             string         `json:"calculation"`
	Unit                    string         `json:"unit"`
	Rationale               string         `json:"rationale"`
	CreatedBy               string         `json:"created_by"`
	CreatedAt               time.Time      `json:"created_at"`
}
type SignalMapping struct {
	ID             string                  `json:"id"`
	CurrentVersion int                     `json:"current_version"`
	Revisions      []SignalMappingRevision `json:"revisions"`
}
type SoftwareReference struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Revision string `json:"revision"`
	Label    string `json:"label"`
}
type EvidenceGap struct {
	Kind   string `json:"kind"`
	Detail string `json:"detail"`
}
type Observation struct {
	ID                   string              `json:"id"`
	MappingID            string              `json:"mapping_id"`
	MappingVersion       int                 `json:"mapping_version"`
	ContractVersion      int                 `json:"contract_version"`
	ObjectiveID          string              `json:"objective_id"`
	WindowStart          time.Time           `json:"window_start"`
	WindowEnd            time.Time           `json:"window_end"`
	GoodEvents           float64             `json:"good_events"`
	TotalEvents          float64             `json:"total_events"`
	ObservedValue        *float64            `json:"observed_value,omitempty"`
	Uncertainty          float64             `json:"uncertainty"`
	Gaps                 []EvidenceGap       `json:"gaps"`
	Software             []SoftwareReference `json:"software"`
	Summary              string              `json:"summary"`
	Attainment           *float64            `json:"attainment,omitempty"`
	TargetMet            *bool               `json:"target_met,omitempty"`
	ErrorBudgetConsumed  *float64            `json:"error_budget_consumed_percent,omitempty"`
	ComparableToPrevious bool                `json:"comparable_to_previous"`
	ComparisonReason     string              `json:"comparison_reason,omitempty"`
	RecordedBy           string              `json:"recorded_by"`
	RecordedAt           time.Time           `json:"recorded_at"`
}
type InvestigationTrigger struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Revision string `json:"revision"`
}
type InvestigationEvidence struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Revision   string `json:"revision"`
	Label      string `json:"label"`
	Visibility string `json:"visibility"`
}
type InvestigationFinding struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	Statement   string    `json:"statement"`
	Uncertainty string    `json:"uncertainty"`
	Confidence  string    `json:"confidence"`
	CitationIDs []string  `json:"citation_ids"`
	CreatedBy   string    `json:"created_by"`
	ActorType   string    `json:"actor_type"`
	CreatedAt   time.Time `json:"created_at"`
}
type InvestigationResponse struct {
	ID        string    `json:"id"`
	FindingID string    `json:"finding_id"`
	Kind      string    `json:"kind"`
	Body      string    `json:"body"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}
type InputRequest struct {
	ID           string     `json:"id"`
	OwnerID      string     `json:"owner_id"`
	DependencyID string     `json:"dependency_id,omitempty"`
	Question     string     `json:"question"`
	Status       string     `json:"status"`
	Response     string     `json:"response,omitempty"`
	RequestedBy  string     `json:"requested_by"`
	RespondedBy  string     `json:"responded_by,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	RespondedAt  *time.Time `json:"responded_at,omitempty"`
}
type InvestigationOutcome struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Summary    string `json:"summary"`
}
type Investigation struct {
	ID                     string                  `json:"id"`
	Version                int                     `json:"version"`
	ContractVersion        int                     `json:"contract_version"`
	ObjectiveID            string                  `json:"objective_id"`
	Title                  string                  `json:"title"`
	Trigger                InvestigationTrigger    `json:"trigger"`
	BaselineObservationIDs []string                `json:"baseline_observation_ids"`
	AffectedObservationIDs []string                `json:"affected_observation_ids"`
	JourneyIDs             []string                `json:"journey_ids"`
	Evidence               []InvestigationEvidence `json:"evidence"`
	Findings               []InvestigationFinding  `json:"findings"`
	Responses              []InvestigationResponse `json:"responses"`
	InputRequests          []InputRequest          `json:"input_requests"`
	Outcome                *InvestigationOutcome   `json:"outcome,omitempty"`
	Status                 string                  `json:"status"`
	StaleEvidenceIDs       []string                `json:"stale_evidence_ids"`
	HiddenDependencyIDs    []string                `json:"hidden_dependency_ids"`
	Inconclusive           bool                    `json:"inconclusive"`
	CreatedBy              string                  `json:"created_by"`
	CreatedAt              time.Time               `json:"created_at"`
	UpdatedAt              time.Time               `json:"updated_at"`
}
type Revision struct {
	Version         int              `json:"version"`
	Title           string           `json:"title"`
	Summary         string           `json:"summary"`
	Scopes          []Scope          `json:"scopes"`
	Indicators      []Indicator      `json:"indicators"`
	Objectives      []Objective      `json:"objectives"`
	Windows         []Window         `json:"measurement_windows"`
	Journeys        []Journey        `json:"user_journeys"`
	Dependencies    []Dependency     `json:"dependencies"`
	ErrorBudgets    []ErrorBudget    `json:"error_budgets"`
	Severities      []Severity       `json:"severity_thresholds"`
	OwnerIDs        []string         `json:"owner_ids"`
	CommitmentLinks []CommitmentLink `json:"commitment_links"`
	ExceptionPolicy ExceptionPolicy  `json:"exception_policy"`
	Exceptions      []Exception      `json:"exceptions"`
	Rationale       string           `json:"rationale"`
	CreatedBy       string           `json:"created_by"`
	CreatedAt       time.Time        `json:"created_at"`
}
type Contract struct {
	ID             string          `json:"id"`
	RepositoryID   string          `json:"repository_id"`
	CurrentVersion int             `json:"current_version"`
	Revisions      []Revision      `json:"revisions"`
	SignalMappings []SignalMapping `json:"signal_mappings"`
	Observations   []Observation   `json:"observations"`
	Investigations []Investigation `json:"investigations"`
	Diagnostics    []Diagnostic    `json:"diagnostics"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

func (s *Store) OpenInvestigation(contractID, actor string, in Investigation) (Contract, error) {
	var out Contract
	err := s.lock(func() error {
		v, e := s.read(contractID)
		if e != nil {
			return e
		}
		if !validInvestigation(v, in) || actor == "" {
			return ErrInvalid
		}
		now := s.now()
		in.ID = id()
		in.Version = 1
		in.CreatedBy = actor
		in.CreatedAt = now
		in.UpdatedAt = now
		in.Status = "open"
		in.Findings = []InvestigationFinding{}
		in.Responses = []InvestigationResponse{}
		in.InputRequests = []InputRequest{}
		v.Investigations = append(v.Investigations, in)
		v.UpdatedAt = now
		out = v
		return s.write(v)
	})
	return s.project(out), err
}
func (s *Store) MutateInvestigation(contractID, investigationID, actor, actorType, action string, expected int, finding InvestigationFinding, response InvestigationResponse, request InputRequest, outcome *InvestigationOutcome) (Contract, error) {
	var out Contract
	err := s.lock(func() error {
		v, e := s.read(contractID)
		if e != nil {
			return e
		}
		var x *Investigation
		for i := range v.Investigations {
			if v.Investigations[i].ID == investigationID {
				x = &v.Investigations[i]
			}
		}
		if x == nil {
			return ErrNotFound
		}
		if x.Version != expected {
			return ErrConflict
		}
		now := s.now()
		switch action {
		case "finding":
			if !validFinding(*x, finding) || !oneOf(actorType, "human", "agent") {
				return ErrInvalid
			}
			finding.ID = id()
			finding.CreatedBy = actor
			finding.ActorType = actorType
			finding.CreatedAt = now
			x.Findings = append(x.Findings, finding)
		case "response":
			if !oneOf(response.Kind, "dispute", "confirm") || strings.TrimSpace(response.Body) == "" || !findingExists(*x, response.FindingID) {
				return ErrInvalid
			}
			response.ID = id()
			response.CreatedBy = actor
			response.CreatedAt = now
			x.Responses = append(x.Responses, response)
		case "request":
			if !validInputRequest(v, *x, request) {
				return ErrInvalid
			}
			request.ID = id()
			request.Status = "requested"
			request.RequestedBy = actor
			request.CreatedAt = now
			x.InputRequests = append(x.InputRequests, request)
		case "reply":
			found := false
			for i := range x.InputRequests {
				q := &x.InputRequests[i]
				if q.ID == request.ID && q.OwnerID == actor && q.Status == "requested" && strings.TrimSpace(request.Response) != "" {
					q.Status = "answered"
					q.Response = strings.TrimSpace(request.Response)
					q.RespondedBy = actor
					q.RespondedAt = &now
					found = true
				}
			}
			if !found {
				return ErrInvalid
			}
		case "outcome":
			if outcome == nil || !oneOf(outcome.Kind, "issue", "incident", "decision", "planned_improvement") || outcome.ResourceID == "" || outcome.Summary == "" {
				return ErrInvalid
			}
			x.Outcome = outcome
			x.Status = "concluded"
		default:
			return ErrInvalid
		}
		x.Version++
		x.UpdatedAt = now
		v.UpdatedAt = now
		out = v
		return s.write(v)
	})
	return s.project(out), err
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
func (s *Store) Create(repo, actor string, r Revision) (Contract, error) {
	var out Contract
	err := s.lock(func() error {
		now := s.now()
		if validateAt(r, now) != nil {
			return ErrInvalid
		}
		stamp(&r, 1, actor, now)
		out = Contract{ID: id(), RepositoryID: repo, CurrentVersion: 1, Revisions: []Revision{r}, CreatedAt: now, UpdatedAt: now}
		return s.write(out)
	})
	return s.project(out), err
}
func (s *Store) Revise(contractID string, expected int, actor string, r Revision) (Contract, error) {
	var out Contract
	err := s.lock(func() error {
		v, e := s.read(contractID)
		if e != nil {
			return e
		}
		if v.CurrentVersion != expected {
			return ErrConflict
		}
		now := s.now()
		if validateAt(r, now) != nil {
			return ErrInvalid
		}
		stamp(&r, expected+1, actor, now)
		v.CurrentVersion = r.Version
		v.Revisions = append(v.Revisions, r)
		v.UpdatedAt = r.CreatedAt
		out = v
		return s.write(v)
	})
	return s.project(out), err
}
func (s *Store) Get(id string) (Contract, error) {
	var v Contract
	err := s.lock(func() error { var e error; v, e = s.read(id); return e })
	return s.project(v), err
}
func (s *Store) List(repo string) ([]Contract, error) {
	values := []Contract{}
	err := s.lock(func() error {
		entries, e := os.ReadDir(filepath.Join(s.root, repo))
		if os.IsNotExist(e) {
			return nil
		}
		if e != nil {
			return e
		}
		for _, x := range entries {
			if x.IsDir() || !strings.HasSuffix(x.Name(), ".json") {
				continue
			}
			v, e := s.readFile(filepath.Join(s.root, repo, x.Name()))
			if e != nil {
				return e
			}
			values = append(values, s.project(v))
		}
		return nil
	})
	sort.Slice(values, func(i, j int) bool { return values[i].UpdatedAt.After(values[j].UpdatedAt) })
	return values, err
}
func (s *Store) PublishMapping(contractID string, actor string, revision SignalMappingRevision) (Contract, error) {
	var out Contract
	err := s.lock(func() error {
		v, e := s.read(contractID)
		if e != nil {
			return e
		}
		if revision.ContractVersion < 1 || revision.ContractVersion > v.CurrentVersion || !validMapping(v, revision) {
			return ErrInvalid
		}
		now := s.now()
		revision.Version, revision.CreatedBy, revision.CreatedAt = 1, actor, now
		v.SignalMappings = append(v.SignalMappings, SignalMapping{ID: id(), CurrentVersion: 1, Revisions: []SignalMappingRevision{revision}})
		v.UpdatedAt, out = now, v
		return s.write(v)
	})
	return s.project(out), err
}

// ReviseMapping preserves every prior instrumentation definition and uses the mapping version as a CAS token.
func (s *Store) ReviseMapping(contractID, mappingID string, expected int, actor string, revision SignalMappingRevision) (Contract, error) {
	var out Contract
	err := s.lock(func() error {
		v, e := s.read(contractID)
		if e != nil {
			return e
		}
		m := mappingByID(&v, mappingID)
		if m == nil {
			return ErrMappingNotFound
		}
		if m.CurrentVersion != expected {
			return ErrConflict
		}
		if latest := mappingRevision(*m, m.CurrentVersion); latest == nil || latest.ObjectiveID != revision.ObjectiveID {
			return ErrInvalid
		}
		if revision.ContractVersion < 1 || revision.ContractVersion > v.CurrentVersion || !validMapping(v, revision) {
			return ErrInvalid
		}
		now := s.now()
		revision.Version, revision.CreatedBy, revision.CreatedAt = expected+1, actor, now
		m.CurrentVersion = revision.Version
		m.Revisions = append(m.Revisions, revision)
		v.UpdatedAt, out = now, v
		return s.write(v)
	})
	return s.project(out), err
}

func (s *Store) RecordObservation(contractID string, actor string, observation Observation) (Contract, error) {
	var out Contract
	err := s.lock(func() error {
		v, e := s.read(contractID)
		if e != nil {
			return e
		}
		m := mappingByID(&v, observation.MappingID)
		if m == nil {
			return ErrMappingNotFound
		}
		mr := mappingRevision(*m, observation.MappingVersion)
		if mr == nil || observation.ContractVersion != mr.ContractVersion || observation.ObjectiveID != mr.ObjectiveID || !validObservation(observation) {
			return ErrInvalid
		}
		observation.ID, observation.RecordedBy, observation.RecordedAt = id(), actor, s.now()
		v.Observations = append(v.Observations, observation)
		v.UpdatedAt, out = observation.RecordedAt, v
		return s.write(v)
	})
	return s.project(out), err
}

func (s *Store) ProjectForReader(v Contract, participant bool) Contract {
	v = s.project(v)
	if !participant {
		v.Investigations = []Investigation{}
	}
	for mi := range v.SignalMappings {
		for ri := range v.SignalMappings[mi].Revisions {
			for si := range v.SignalMappings[mi].Revisions[ri].Sources {
				source := &v.SignalMappings[mi].Revisions[ri].Sources[si]
				if unsafe(source.Reference) {
					source.Reference = "redacted_unsafe_reference"
					source.Sanitization = "credential-shaped legacy source reference removed"
				} else if source.Visibility == "participants" && !participant {
					source.Reference = "restricted"
					source.Sanitization = "restricted source detail omitted"
				}
			}
		}
	}
	for ii := range v.Investigations {
		for ei := range v.Investigations[ii].Evidence {
			e := &v.Investigations[ii].Evidence[ei]
			if unsafe(e.ResourceID + e.Revision + e.Label) {
				e.ResourceID = "redacted_unsafe_reference"
				e.Revision = "redacted"
				e.Label = "unsafe legacy evidence removed"
			} else if e.Visibility == "participants" && !participant {
				e.ResourceID = "restricted"
				e.Revision = "restricted"
				e.Label = "restricted evidence omitted"
			}
		}
	}
	return v
}

func validMapping(v Contract, r SignalMappingRevision) bool {
	if r.ObjectiveID == "" || r.InstrumentationRevision == "" || len(r.Sources) == 0 || r.Calculation == "" || r.Unit == "" || r.Rationale == "" || unsafe(r.InstrumentationRevision+r.Rationale) {
		return false
	}
	revision := revisionAt(v, r.ContractVersion)
	if revision == nil {
		return false
	}
	indicatorID := ""
	for _, o := range revision.Objectives {
		if o.ID == r.ObjectiveID {
			indicatorID = o.IndicatorID
		}
	}
	if indicatorID == "" {
		return false
	}
	matchedIndicator := false
	for _, indicator := range revision.Indicators {
		if indicator.ID == indicatorID && indicator.Calculation == r.Calculation && indicator.Unit == r.Unit {
			matchedIndicator = true
		}
	}
	if !matchedIndicator {
		return false
	}
	kinds := map[string]bool{"metric": true, "log": true, "trace": true, "health_check": true, "support_report": true, "deployment": true, "release": true, "commit": true, "pull_request": true, "package": true, "dependent_service": true}
	for _, source := range r.Sources {
		if !kinds[source.Kind] || source.Name == "" || source.Reference == "" || !oneOf(source.Visibility, "public", "participants") || source.Sanitization == "" || unsafe(source.Name+source.Reference+source.Sanitization) {
			return false
		}
	}
	return true
}
func validObservation(o Observation) bool {
	if o.MappingID == "" || o.MappingVersion < 1 || o.ContractVersion < 1 || o.ObjectiveID == "" || o.WindowStart.IsZero() || !o.WindowEnd.After(o.WindowStart) || o.TotalEvents < 0 || o.GoodEvents < 0 || o.GoodEvents > o.TotalEvents || o.Uncertainty < 0 || o.Uncertainty > 100 || o.Summary == "" || unsafe(o.Summary) {
		return false
	}
	if o.ObservedValue != nil && (math.IsNaN(*o.ObservedValue) || math.IsInf(*o.ObservedValue, 0)) {
		return false
	}
	kinds := map[string]bool{"deployment": true, "release": true, "commit": true, "pull_request": true, "package": true, "dependent_service": true}
	for _, x := range o.Software {
		if !kinds[x.Kind] || x.ID == "" || x.Revision == "" || x.Label == "" || unsafe(x.ID+x.Revision+x.Label) {
			return false
		}
	}
	for _, x := range o.Gaps {
		if x.Kind == "" || x.Detail == "" || unsafe(x.Kind+x.Detail) {
			return false
		}
	}
	return true
}
func validInvestigation(v Contract, x Investigation) bool {
	if x.ContractVersion < 1 || x.ObjectiveID == "" || strings.TrimSpace(x.Title) == "" || len(x.Title) > 200 || !oneOf(x.Trigger.Kind, "objective", "pull_request", "deployment", "budget_consumption") || x.Trigger.ID == "" || x.Trigger.Revision == "" || len(x.JourneyIDs) == 0 || len(x.Evidence) == 0 {
		return false
	}
	r := revisionAt(v, x.ContractVersion)
	if r == nil {
		return false
	}
	objectives := map[string]bool{}
	journeys := map[string]bool{}
	for _, o := range r.Objectives {
		objectives[o.ID] = true
	}
	for _, j := range r.Journeys {
		journeys[j.ID] = true
	}
	if !objectives[x.ObjectiveID] {
		return false
	}
	if x.Trigger.Kind == "objective" && (x.Trigger.ID != x.ObjectiveID || x.Trigger.Revision != "contract:"+strconv.Itoa(x.ContractVersion)) {
		return false
	}
	for _, j := range x.JourneyIDs {
		if !journeys[j] {
			return false
		}
	}
	observations := map[string]bool{}
	for _, o := range v.Observations {
		if o.ContractVersion == x.ContractVersion && o.ObjectiveID == x.ObjectiveID {
			observations[o.ID] = true
		}
	}
	triggerResolved := x.Trigger.Kind == "objective"
	for _, o := range v.Observations {
		if x.Trigger.Kind == "budget_consumption" && o.ID == x.Trigger.ID && x.Trigger.Revision == "observation:"+o.ID {
			triggerResolved = true
		}
		for _, sw := range o.Software {
			if sw.ID == x.Trigger.ID && sw.Revision == x.Trigger.Revision && ((x.Trigger.Kind == "pull_request" && sw.Kind == "pull_request") || (x.Trigger.Kind == "deployment" && sw.Kind == "deployment")) {
				triggerResolved = true
			}
		}
	}
	if !triggerResolved {
		return false
	}
	for _, oid := range append(append([]string{}, x.BaselineObservationIDs...), x.AffectedObservationIDs...) {
		if !observations[oid] {
			return false
		}
	}
	seen := map[string]bool{}
	for _, e := range x.Evidence {
		if !oneOf(e.Kind, "metric", "log", "trace", "health_check", "support_report", "code", "commit", "pull_request", "deployment", "release", "package", "dependent_service") || e.ResourceID == "" || e.Revision == "" || e.Label == "" || !oneOf(e.Visibility, "public", "participants") || unsafe(e.ResourceID+e.Revision+e.Label) || seen[e.ResourceID] {
			return false
		}
		seen[e.ResourceID] = true
	}
	return true
}
func validFinding(x Investigation, f InvestigationFinding) bool {
	if !oneOf(f.Kind, "hypothesis", "comparison", "uncertainty", "conclusion") || strings.TrimSpace(f.Statement) == "" || strings.TrimSpace(f.Uncertainty) == "" || !oneOf(f.Confidence, "low", "medium", "high") || len(f.CitationIDs) == 0 {
		return false
	}
	allowed := map[string]bool{}
	for _, e := range x.Evidence {
		allowed[e.ResourceID] = true
	}
	for _, id := range append(append([]string{}, x.BaselineObservationIDs...), x.AffectedObservationIDs...) {
		allowed[id] = true
	}
	for _, id := range f.CitationIDs {
		if !allowed[id] {
			return false
		}
	}
	return !unsafe(f.Statement + f.Uncertainty)
}
func findingExists(x Investigation, id string) bool {
	for _, f := range x.Findings {
		if f.ID == id {
			return true
		}
	}
	return false
}
func validInputRequest(v Contract, x Investigation, q InputRequest) bool {
	if q.OwnerID == "" || strings.TrimSpace(q.Question) == "" || unsafe(q.Question) {
		return false
	}
	r := revisionAt(v, x.ContractVersion)
	if r == nil {
		return false
	}
	for _, o := range r.Objectives {
		if o.ID == x.ObjectiveID {
			for _, id := range o.OwnerIDs {
				if id == q.OwnerID && q.DependencyID == "" {
					return true
				}
			}
		}
	}
	for _, d := range r.Dependencies {
		if d.ID == q.DependencyID {
			for _, id := range d.OwnerIDs {
				if id == q.OwnerID {
					return true
				}
			}
		}
	}
	return false
}
func unsafe(v string) bool {
	if len(v) > 4000 {
		return true
	}
	markers := []string{"bearer ", "api_key", "apikey", "api-key", "access_token=", "access-token=", "token=", "password=", "passwd=", "secret=", "authorization:", "proxy-authorization:", "cookie:", "set-cookie:", "x-api-key", "-----begin"}
	canonical := v
	for {
		lower := strings.ToLower(canonical)
		for _, marker := range markers {
			if strings.Contains(lower, marker) {
				return true
			}
		}
		decoded, err := url.PathUnescape(canonical)
		if err != nil {
			return true
		}
		if decoded == canonical {
			return false
		}
		canonical = decoded
	}
}
func mappingByID(v *Contract, id string) *SignalMapping {
	for i := range v.SignalMappings {
		if v.SignalMappings[i].ID == id {
			return &v.SignalMappings[i]
		}
	}
	return nil
}
func mappingRevision(v SignalMapping, version int) *SignalMappingRevision {
	for i := range v.Revisions {
		if v.Revisions[i].Version == version {
			return &v.Revisions[i]
		}
	}
	return nil
}
func revisionAt(v Contract, version int) *Revision {
	for i := range v.Revisions {
		if v.Revisions[i].Version == version {
			return &v.Revisions[i]
		}
	}
	return nil
}
func stamp(r *Revision, v int, actor string, now time.Time) {
	r.Version = v
	r.CreatedBy = actor
	r.CreatedAt = now
}

func validate(r Revision) error {
	if strings.TrimSpace(r.Title) == "" || len(r.Title) > 200 || strings.TrimSpace(r.Summary) == "" || len(r.Scopes) == 0 || len(r.Indicators) == 0 || len(r.Objectives) == 0 || len(r.Windows) == 0 || len(r.Journeys) == 0 || len(r.ErrorBudgets) == 0 || len(r.Severities) == 0 || len(r.OwnerIDs) == 0 || strings.TrimSpace(r.Rationale) == "" || strings.TrimSpace(r.ExceptionPolicy.MaximumDuration) == "" || len(r.ExceptionPolicy.ApprovalOwnerIDs) == 0 {
		return ErrInvalid
	}
	for _, s := range r.Scopes {
		if !oneOf(s.Kind, "repository", "release", "environment") || s.Name == "" || (s.Kind != "repository" && s.ResourceID == "") {
			return ErrInvalid
		}
	}
	indicators := map[string]bool{}
	for _, x := range r.Indicators {
		if x.ID == "" || x.Name == "" || x.Calculation == "" || x.Unit == "" || x.GoodEvent == "" || x.TotalEvent == "" || indicators[x.ID] {
			return ErrInvalid
		}
		indicators[x.ID] = true
	}
	windows := map[string]bool{}
	for _, x := range r.Windows {
		if x.ID == "" || x.Name == "" || x.Duration == "" || windows[x.ID] {
			return ErrInvalid
		}
		if duration, e := time.ParseDuration(x.Duration); e != nil || duration <= 0 {
			return ErrInvalid
		}
		windows[x.ID] = true
	}
	journeys := map[string]bool{}
	for _, x := range r.Journeys {
		if x.ID == "" || x.Name == "" || journeys[x.ID] {
			return ErrInvalid
		}
		journeys[x.ID] = true
	}
	objectives := map[string]bool{}
	for _, x := range r.Objectives {
		if x.ID == "" || x.Name == "" || !indicators[x.IndicatorID] || !windows[x.WindowID] || !oneOf(x.Comparator, "at_least", "at_most") || objectives[x.ID] {
			return ErrInvalid
		}
		for _, j := range x.JourneyIDs {
			if !journeys[j] {
				return ErrInvalid
			}
		}
		objectives[x.ID] = true
	}
	budgets := map[string]bool{}
	for _, x := range r.ErrorBudgets {
		if !objectives[x.ObjectiveID] || budgets[x.ObjectiveID] || x.AllowedFailure < 0 || x.Unit == "" || x.BurnPolicy == "" {
			return ErrInvalid
		}
		budgets[x.ObjectiveID] = true
	}
	for x := range objectives {
		if !budgets[x] {
			return ErrInvalid
		}
	}
	last := -1.0
	for _, x := range r.Severities {
		if x.Level == "" || x.Response == "" || x.BudgetConsumedPercent < 0 || x.BudgetConsumedPercent <= last {
			return ErrInvalid
		}
		last = x.BudgetConsumedPercent
	}
	for _, x := range r.Dependencies {
		if x.ID == "" || x.Name == "" || !oneOf(x.Kind, "repository", "service", "external") {
			return ErrInvalid
		}
		for _, o := range x.ObjectiveIDs {
			if !objectives[o] {
				return ErrInvalid
			}
		}
	}
	if duration, err := time.ParseDuration(r.ExceptionPolicy.MaximumDuration); err != nil || duration <= 0 {
		return ErrInvalid
	}
	for _, x := range r.CommitmentLinks {
		if !oneOf(x.Kind, "product", "performance", "accessibility", "privacy", "release") || x.ID == "" || x.Version < 1 {
			return ErrInvalid
		}
	}
	for _, x := range r.Exceptions {
		if x.ID == "" || x.Reason == "" || x.ApprovedBy == "" || x.ExpiresAt.IsZero() {
			return ErrInvalid
		}
		for _, o := range x.ObjectiveIDs {
			if !objectives[o] {
				return ErrInvalid
			}
		}
		if r.ExceptionPolicy.FollowUpRequired && x.FollowUp == "" {
			return ErrInvalid
		}
	}
	return nil
}

func validateAt(r Revision, now time.Time) error {
	if err := validate(r); err != nil {
		return err
	}
	maximum, _ := time.ParseDuration(r.ExceptionPolicy.MaximumDuration)
	for _, x := range r.Exceptions {
		if x.ExpiresAt.After(now) && x.ExpiresAt.Sub(now) > maximum {
			return ErrInvalid
		}
	}
	return nil
}
func (s *Store) project(v Contract) Contract {
	if v.SignalMappings == nil {
		v.SignalMappings = []SignalMapping{}
	}
	if v.Observations == nil {
		v.Observations = []Observation{}
	}
	if v.Investigations == nil {
		v.Investigations = []Investigation{}
	}
	if len(v.Revisions) == 0 {
		return v
	}
	for i := range v.Revisions {
		if v.Revisions[i].Dependencies == nil {
			v.Revisions[i].Dependencies = []Dependency{}
		}
		if v.Revisions[i].CommitmentLinks == nil {
			v.Revisions[i].CommitmentLinks = []CommitmentLink{}
		}
		if v.Revisions[i].Exceptions == nil {
			v.Revisions[i].Exceptions = []Exception{}
		}
	}
	r := v.Revisions[len(v.Revisions)-1]
	d := []Diagnostic{}
	add := func(k, severity, msg, id, actor string) { d = append(d, Diagnostic{k, severity, msg, id, actor}) }
	supported := map[string]bool{"ratio": true, "availability": true, "latency_percentile": true, "count": true}
	for _, x := range r.Indicators {
		if strings.TrimSpace(x.Signal) == "" {
			add("missing_signal", "blocking", "Indicator has no declared measurement signal.", x.ID, r.CreatedBy)
		}
		if !supported[x.Calculation] {
			add("unsupported_calculation", "blocking", "Indicator calculation is not supported.", x.ID, r.CreatedBy)
		}
	}
	for _, x := range r.Objectives {
		if len(x.OwnerIDs) == 0 {
			add("missing_ownership", "blocking", "Objective has no accountable owner.", x.ID, r.CreatedBy)
		}
	}
	for _, x := range r.Journeys {
		if len(x.OwnerIDs) == 0 {
			add("missing_ownership", "blocking", "User journey has no accountable owner.", x.ID, r.CreatedBy)
		}
	}
	for _, x := range r.Dependencies {
		if len(x.OwnerIDs) == 0 {
			add("missing_ownership", "blocking", "Dependency has no accountable owner.", x.ID, r.CreatedBy)
		}
	}
	for _, x := range r.Severities {
		if len(x.OwnerIDs) == 0 {
			add("missing_ownership", "blocking", "Severity response has no accountable owner.", x.Level, r.CreatedBy)
		}
	}
	for i, a := range r.Objectives {
		for _, b := range r.Objectives[i+1:] {
			if a.IndicatorID == b.IndicatorID && a.WindowID == b.WindowID && (a.Comparator != b.Comparator || a.Target != b.Target) {
				add("conflicting_target", "blocking", "Objectives set conflicting targets for the same indicator and window.", a.ID, r.CreatedBy)
			}
		}
	}
	now := s.now()
	for _, x := range r.Exceptions {
		if !x.ExpiresAt.After(now) {
			add("expired_exception", "blocking", "Reliability exception has expired.", x.ID, x.ApprovedBy)
		} else if x.ExpiresAt.Sub(now) <= 7*24*time.Hour {
			add("expiring_exception", "warning", "Reliability exception expires within seven days.", x.ID, x.ApprovedBy)
		}
	}
	v.Diagnostics = d
	for i := range v.Observations {
		o := &v.Observations[i]
		if o.Gaps == nil {
			o.Gaps = []EvidenceGap{}
		}
		if o.Software == nil {
			o.Software = []SoftwareReference{}
		}
		revision := revisionAt(v, o.ContractVersion)
		if revision != nil {
			for _, objective := range revision.Objectives {
				if objective.ID == o.ObjectiveID {
					var indicator *Indicator
					for j := range revision.Indicators {
						if revision.Indicators[j].ID == objective.IndicatorID {
							indicator = &revision.Indicators[j]
							break
						}
					}
					if indicator == nil {
						continue
					}
					var value *float64
					switch indicator.Calculation {
					case "ratio", "availability":
						if o.TotalEvents > 0 {
							derived := o.GoodEvents / o.TotalEvents * 100
							value = &derived
						}
					case "count":
						derived := o.GoodEvents
						value = &derived
					case "latency_percentile":
						value = o.ObservedValue
					default:
						value = o.ObservedValue
					}
					o.Attainment = value
					if value == nil {
						continue
					}
					met := (*o.Attainment >= objective.Target && objective.Comparator == "at_least") || (*o.Attainment <= objective.Target && objective.Comparator == "at_most")
					o.TargetMet = &met
					for _, budget := range revision.ErrorBudgets {
						if budget.ObjectiveID == objective.ID && budget.AllowedFailure > 0 {
							consumed := 0.0
							if (indicator.Calculation == "ratio" || indicator.Calculation == "availability") && indicator.Unit == "percent" && objective.Comparator == "at_least" {
								consumed = math.Max(0, (100-*value)/budget.AllowedFailure*100)
							} else if objective.Comparator == "at_most" {
								consumed = math.Max(0, (*value-objective.Target)/budget.AllowedFailure*100)
							} else {
								consumed = math.Max(0, (objective.Target-*value)/budget.AllowedFailure*100)
							}
							o.ErrorBudgetConsumed = &consumed
						}
					}
				}
			}
		}
		o.ComparableToPrevious = true
		for previous := i - 1; previous >= 0; previous-- {
			p := v.Observations[previous]
			if p.ObjectiveID == o.ObjectiveID {
				if p.MappingID != o.MappingID || p.MappingVersion != o.MappingVersion || p.WindowEnd.Sub(p.WindowStart) != o.WindowEnd.Sub(o.WindowStart) {
					o.ComparableToPrevious = false
					o.ComparisonReason = "instrumentation revision or measurement window changed"
				}
				break
			}
		}
	}
	for i := range v.Investigations {
		x := &v.Investigations[i]
		if x.Findings == nil {
			x.Findings = []InvestigationFinding{}
		}
		if x.Responses == nil {
			x.Responses = []InvestigationResponse{}
		}
		if x.InputRequests == nil {
			x.InputRequests = []InputRequest{}
		}
		x.StaleEvidenceIDs = []string{}
		x.HiddenDependencyIDs = []string{}
		x.Inconclusive = false
		if x.ContractVersion != v.CurrentVersion {
			x.StaleEvidenceIDs = append(x.StaleEvidenceIDs, "contract:"+x.ID)
		}
		obs := map[string]Observation{}
		for _, o := range v.Observations {
			obs[o.ID] = o
		}
		for _, oid := range append(append([]string{}, x.BaselineObservationIDs...), x.AffectedObservationIDs...) {
			o, ok := obs[oid]
			mappingCurrent := false
			for _, m := range v.SignalMappings {
				if m.ID == o.MappingID && m.CurrentVersion == o.MappingVersion {
					mappingCurrent = true
				}
			}
			if !ok || o.ContractVersion != x.ContractVersion || !mappingCurrent {
				x.StaleEvidenceIDs = append(x.StaleEvidenceIDs, oid)
			}
		}
		rr := revisionAt(v, x.ContractVersion)
		if rr != nil {
			for _, d := range rr.Dependencies {
				if len(d.OwnerIDs) == 0 {
					x.HiddenDependencyIDs = append(x.HiddenDependencyIDs, d.ID)
				}
			}
		}
		disputed := false
		for _, r := range x.Responses {
			if r.Kind == "dispute" {
				disputed = true
			}
		}
		hasConclusion := false
		for _, f := range x.Findings {
			if f.Kind == "conclusion" {
				hasConclusion = true
			}
		}
		x.Inconclusive = !hasConclusion || disputed || len(x.StaleEvidenceIDs) > 0
	}
	return v
}
func oneOf(v string, vs ...string) bool {
	for _, x := range vs {
		if v == x {
			return true
		}
	}
	return false
}
func id() string { b := make([]byte, 12); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func (s *Store) write(v Contract) error {
	dir := filepath.Join(s.root, v.RepositoryID)
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(dir, ".objective-*")
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
	if e != nil {
		return e
	}
	if e = os.Rename(name, filepath.Join(dir, v.ID+".json")); e != nil {
		return e
	}
	d, e := os.Open(dir)
	if e == nil {
		e = d.Sync()
		_ = d.Close()
	}
	return e
}
func (s *Store) read(id string) (Contract, error) {
	entries, e := os.ReadDir(s.root)
	if e != nil {
		return Contract{}, e
	}
	for _, repo := range entries {
		if !repo.IsDir() {
			continue
		}
		v, e := s.readFile(filepath.Join(s.root, repo.Name(), id+".json"))
		if e == nil {
			return v, nil
		}
		if !os.IsNotExist(e) {
			return Contract{}, e
		}
	}
	return Contract{}, ErrNotFound
}
func (s *Store) readFile(p string) (Contract, error) {
	b, e := os.ReadFile(p)
	if e != nil {
		return Contract{}, e
	}
	var v Contract
	e = json.Unmarshal(b, &v)
	return v, e
}
func (s *Store) lock(fn func() error) error {
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
