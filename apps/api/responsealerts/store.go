// Package responsealerts retains revision-bound urgent signals and their delivery ledger.
package responsealerts

import (
	"crypto/sha256"
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

	"github.com/greptile-projects/vivarium-tuatara/apps/api/responsepolicies"
)

var ErrNotFound = errors.New("response alert not found")
var ErrInvalid = errors.New("invalid response alert")
var ErrConflict = errors.New("response alert conflict")
var ErrForbidden = errors.New("response alert forbidden")

type Evidence struct {
	Kind         string   `json:"kind"`
	ResourceID   string   `json:"resource_id"`
	Revision     string   `json:"revision"`
	Digest       string   `json:"digest"`
	URL          string   `json:"url,omitempty"`
	Summary      string   `json:"summary"`
	AccessibleTo []string `json:"accessible_to,omitempty"`
	Available    bool     `json:"available"`
}
type Signal struct {
	SignalClass        string     `json:"signal_class"`
	Severity           string     `json:"severity"`
	ResourceIDs        []string   `json:"resource_ids"`
	AffectedUserCount  int        `json:"affected_user_count"`
	AffectedUserGroups []string   `json:"affected_user_groups"`
	Summary            string     `json:"summary"`
	Uncertainty        string     `json:"uncertainty"`
	OccurredAt         time.Time  `json:"occurred_at"`
	SourceRevision     string     `json:"source_revision"`
	CorrelationKey     string     `json:"correlation_key"`
	Evidence           []Evidence `json:"evidence"`
	SuppressionKey     string     `json:"suppression_key,omitempty"`
	SuppressedUntil    *time.Time `json:"suppressed_until,omitempty"`
	MaintenanceEndsAt  *time.Time `json:"maintenance_ends_at,omitempty"`
	RateLimitSeconds   int        `json:"rate_limit_seconds,omitempty"`
}
type Delivery struct {
	ID          string    `json:"id"`
	RecipientID string    `json:"recipient_id"`
	Channel     string    `json:"channel"`
	Status      string    `json:"status"`
	AttemptedAt time.Time `json:"attempted_at"`
	Failure     string    `json:"failure,omitempty"`
}
type Event struct {
	ID        string    `json:"id"`
	RequestID string    `json:"request_id"`
	Kind      string    `json:"kind"`
	ActorID   string    `json:"actor_id"`
	Reason    string    `json:"reason,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
type ContextBinding struct {
	Kind       string     `json:"kind"`
	ResourceID string     `json:"resource_id"`
	Revision   string     `json:"revision"`
	Summary    string     `json:"summary"`
	WindowFrom time.Time  `json:"window_from"`
	WindowTo   time.Time  `json:"window_to"`
	Evidence   []Evidence `json:"evidence,omitempty"`
}
type WorkspaceEntry struct {
	ID            string           `json:"id"`
	RequestID     string           `json:"request_id"`
	Kind          string           `json:"kind"`
	ActorID       string           `json:"actor_id"`
	Message       string           `json:"message"`
	AudienceIDs   []string         `json:"audience_ids,omitempty"`
	Context       []ContextBinding `json:"context,omitempty"`
	CreatedAt     time.Time        `json:"created_at"`
	RequestDigest string           `json:"request_digest"`
}
type DiagnosticRun struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	State        string    `json:"state"`
	OutputDigest string    `json:"output_digest"`
	Summary      string    `json:"summary"`
	ActorID      string    `json:"actor_id"`
	CreatedAt    time.Time `json:"created_at"`
}
type Investigation struct {
	ID             string           `json:"id"`
	AgentID        string           `json:"agent_id"`
	Mandate        string           `json:"mandate"`
	State          string           `json:"state"`
	Context        []ContextBinding `json:"context"`
	PermittedTools []string         `json:"permitted_tools"`
	Budget         int              `json:"budget"`
	CreatedBy      string           `json:"created_by"`
	CreatedAt      time.Time        `json:"created_at"`
}
type Workspace struct {
	Version        int              `json:"version"`
	Classification string           `json:"classification,omitempty"`
	ResponderID    string           `json:"responder_id,omitempty"`
	ParticipantIDs []string         `json:"participant_ids"`
	Context        []ContextBinding `json:"context"`
	Timeline       []WorkspaceEntry `json:"timeline"`
	Diagnostics    []DiagnosticRun  `json:"diagnostic_runs"`
	Investigations []Investigation  `json:"investigations"`
	IncidentID     string           `json:"incident_id,omitempty"`
}
type OutcomeReview struct {
	ID                   string    `json:"id"`
	RequestID            string    `json:"request_id"`
	ActorID              string    `json:"actor_id"`
	Classification       string    `json:"classification"`
	UserOutcome          string    `json:"user_outcome,omitempty"`
	UserOutcomeConsent   bool      `json:"user_outcome_consent"`
	InterruptionMinutes  int       `json:"interruption_minutes,omitempty"`
	ResponderLoadConsent bool      `json:"responder_load_consent"`
	AgentCost            int       `json:"agent_cost,omitempty"`
	CorrectionKind       string    `json:"correction_kind,omitempty"`
	RoutingAction        string    `json:"routing_action,omitempty"`
	Rationale            string    `json:"rationale"`
	ProposalID           string    `json:"proposal_id,omitempty"`
	TaskID               string    `json:"task_id,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
	RequestDigest        string    `json:"request_digest"`
}
type OutcomeReviewInput struct {
	RequestID            string `json:"request_id"`
	Classification       string `json:"classification"`
	UserOutcome          string `json:"user_outcome,omitempty"`
	UserOutcomeConsent   bool   `json:"user_outcome_consent"`
	InterruptionMinutes  int    `json:"interruption_minutes,omitempty"`
	ResponderLoadConsent bool   `json:"responder_load_consent"`
	AgentCost            int    `json:"agent_cost,omitempty"`
	CorrectionKind       string `json:"correction_kind,omitempty"`
	RoutingAction        string `json:"routing_action,omitempty"`
	Rationale            string `json:"rationale"`
	ProposalID           string `json:"proposal_id,omitempty"`
	TaskID               string `json:"task_id,omitempty"`
}
type WorkspaceCommand struct {
	RequestID       string           `json:"request_id"`
	Kind            string           `json:"kind"`
	Message         string           `json:"message"`
	Classification  string           `json:"classification,omitempty"`
	TargetUserID    string           `json:"target_user_id,omitempty"`
	Context         []ContextBinding `json:"context,omitempty"`
	DiagnosticName  string           `json:"diagnostic_name,omitempty"`
	DiagnosticState string           `json:"diagnostic_state,omitempty"`
	OutputDigest    string           `json:"output_digest,omitempty"`
	AgentID         string           `json:"agent_id,omitempty"`
	PermittedTools  []string         `json:"permitted_tools,omitempty"`
	Budget          int              `json:"budget,omitempty"`
	IncidentID      string           `json:"incident_id,omitempty"`
}
type Alert struct {
	ID                string          `json:"id"`
	RepositoryID      string          `json:"repository_id"`
	RequestID         string          `json:"request_id"`
	RequestDigest     string          `json:"request_digest"`
	PolicyID          string          `json:"policy_id"`
	PolicyVersion     int             `json:"policy_version"`
	RuleID            string          `json:"rule_id"`
	TeamID            string          `json:"team_id"`
	Signal            Signal          `json:"signal"`
	FirstSeenAt       time.Time       `json:"first_seen_at"`
	LastSeenAt        time.Time       `json:"last_seen_at"`
	EventCount        int             `json:"event_count"`
	AcknowledgeBy     time.Time       `json:"acknowledge_by"`
	ResolveBy         time.Time       `json:"resolve_by"`
	State             string          `json:"state"`
	Routing           []Delivery      `json:"routing"`
	Events            []Event         `json:"events"`
	Diagnostics       []string        `json:"diagnostics"`
	ExpectedActions   []string        `json:"expected_actions"`
	PermittedActions  []string        `json:"permitted_actions"`
	ProhibitedActions []string        `json:"prohibited_actions"`
	AudienceIDs       []string        `json:"audience_ids"`
	UpdatedAt         time.Time       `json:"updated_at"`
	Workspace         Workspace       `json:"workspace"`
	OutcomeReviews    []OutcomeReview `json:"outcome_reviews"`
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

func (s *Store) Create(repo, actor, request string, signal Signal, policy responsepolicies.Policy, recipients []string) (Alert, error) {
	return s.CreateControlled(repo, actor, request, signal, policy, recipients, "")
}
func (s *Store) CreateControlled(repo, actor, request string, signal Signal, policy responsepolicies.Policy, recipients []string, routingDirective string) (Alert, error) {
	var out Alert
	err := s.lock(func() error {
		if request == "" || validateSignal(signal) != nil || len(policy.Revisions) == 0 {
			return ErrInvalid
		}
		digest := digest(signal)
		id := stable(repo, actor, request)
		if old, e := s.read(id); e == nil {
			if old.RequestDigest != digest {
				return ErrConflict
			}
			out = old
			return nil
		} else if !errors.Is(e, ErrNotFound) {
			return e
		}
		values, listErr := s.listUnlocked(repo)
		if listErr != nil {
			return listErr
		}
		// A correlated event owns its caller-stable identity independently of
		// the parent alert's later lifecycle or policy state. Reconcile that
		// identity before deciding whether a new occurrence may correlate.
		for _, value := range values {
			for _, event := range value.Events {
				if event.Kind != "correlated" || event.ActorID != actor || event.RequestID != request {
					continue
				}
				if event.Reason != digest {
					return ErrConflict
				}
				out = value
				return nil
			}
		}
		rev := policy.Revisions[len(policy.Revisions)-1]
		var rule *responsepolicies.Rule
		for i := range rev.Rules {
			r := &rev.Rules[i]
			if r.SignalClass == signal.SignalClass && r.Severity == signal.Severity && intersects(r.ResourceIDs, signal.ResourceIDs) {
				if rule != nil {
					return ErrInvalid
				}
				rule = r
			}
		}
		if rule == nil {
			return ErrInvalid
		}
		now := s.now()
		state := "open"
		diagnostics := []string{}
		if signal.SuppressedUntil != nil && signal.SuppressedUntil.After(now) {
			state = "suppressed"
			diagnostics = append(diagnostics, "suppressed")
		}
		if signal.MaintenanceEndsAt != nil && signal.MaintenanceEndsAt.After(now) {
			state = "maintenance"
			diagnostics = append(diagnostics, "maintenance_window")
		}
		if routingDirective == "pause" {
			state = "routing_paused"
			diagnostics = append(diagnostics, "routing_paused")
		}
		if routingDirective == "backup" {
			diagnostics = append(diagnostics, "declared_backup_activated")
		}
		for _, e := range signal.Evidence {
			if !e.Available {
				diagnostics = append(diagnostics, "inaccessible_evidence")
			}
		}
		if signal.OccurredAt.Before(now.Add(-24 * time.Hour)) {
			diagnostics = append(diagnostics, "stale_signal")
		}
		routing := []Delivery{}
		if state == "open" {
			for _, recipient := range unique(recipients) {
				routing = append(routing, Delivery{ID: stable(id, recipient, "web"), RecipientID: recipient, Channel: "web", Status: "delivered", AttemptedAt: now})
			}
			if len(routing) == 0 {
				diagnostics = append(diagnostics, "delivery_failed")
			}
		}
		participants := append([]string{}, recipients...)
		out = Alert{ID: id, RepositoryID: repo, RequestID: request, RequestDigest: digest, PolicyID: policy.ID, PolicyVersion: rev.Version, RuleID: rule.ID, TeamID: rule.AccountableTeamID, Signal: signal, FirstSeenAt: now, LastSeenAt: now, EventCount: 1, AcknowledgeBy: now.Add(time.Duration(rule.AcknowledgeSeconds) * time.Second), ResolveBy: now.Add(time.Duration(rule.ResolveSeconds) * time.Second), State: state, Routing: routing, Events: []Event{}, Diagnostics: diagnostics, ExpectedActions: rule.ExpectedActions, PermittedActions: rule.Authority.PermittedActions, ProhibitedActions: rule.Authority.ProhibitedActions, AudienceIDs: rule.CommunicationAudienceIDs, UpdatedAt: now, Workspace: Workspace{Version: 1, ResponderID: first(participants), ParticipantIDs: unique(participants), Context: signalContext(signal), Timeline: []WorkspaceEntry{}, Diagnostics: []DiagnosticRun{}, Investigations: []Investigation{}}}
		// Correlated events join the existing open alert without producing another page.
		for _, v := range values {
			if v.Signal.CorrelationKey != "" && v.Signal.CorrelationKey == signal.CorrelationKey && v.RuleID == rule.ID && v.State == "open" && v.PolicyID == policy.ID && v.PolicyVersion == rev.Version && v.TeamID == rule.AccountableTeamID {
				v.LastSeenAt = now
				v.EventCount++
				v.Events = append(v.Events, Event{ID: stable(v.ID, actor, request), RequestID: request, Kind: "correlated", ActorID: actor, Reason: digest, CreatedAt: now})
				if signal.RateLimitSeconds > 0 && now.Sub(v.UpdatedAt) < time.Duration(signal.RateLimitSeconds)*time.Second && !containsString(v.Diagnostics, "rate_limited") {
					v.Diagnostics = append(v.Diagnostics, "rate_limited")
				}
				v.UpdatedAt = now
				if severityRank(signal.Severity) > severityRank(v.Signal.Severity) {
					v.Signal.Severity = signal.Severity
				}
				out = v
				return s.write(v)
			}
		}
		return s.write(out)
	})
	return out, err
}

func (s *Store) ReviewOutcome(id, actor string, in OutcomeReviewInput, allowed bool) (Alert, error) {
	var out Alert
	err := s.lock(func() error {
		v, err := s.read(id)
		if err != nil {
			return err
		}
		d := outcomeDigest(in)
		for _, old := range v.OutcomeReviews {
			if old.RequestID == in.RequestID {
				if old.ActorID != actor || old.RequestDigest != d {
					return ErrConflict
				}
				out = v
				return nil
			}
		}
		if !allowed || ValidateOutcomeReview(in) != nil {
			return ErrInvalid
		}
		now := s.now()
		v.OutcomeReviews = append(v.OutcomeReviews, OutcomeReview{ID: stable(id, actor, in.RequestID), RequestID: in.RequestID, ActorID: actor, Classification: in.Classification, UserOutcome: in.UserOutcome, UserOutcomeConsent: in.UserOutcomeConsent, InterruptionMinutes: in.InterruptionMinutes, ResponderLoadConsent: in.ResponderLoadConsent, AgentCost: in.AgentCost, CorrectionKind: in.CorrectionKind, RoutingAction: in.RoutingAction, Rationale: strings.TrimSpace(in.Rationale), ProposalID: in.ProposalID, TaskID: in.TaskID, CreatedAt: now, RequestDigest: d})
		v.UpdatedAt = now
		out = v
		return s.write(v)
	})
	return out, err
}

func ValidateOutcomeReview(in OutcomeReviewInput) error {
	validCorrection := in.CorrectionKind == "" || in.CorrectionKind == "signal" || in.CorrectionKind == "routing"
	validRouting := in.RoutingAction == "" || in.RoutingAction == "pause" || in.RoutingAction == "activate_backup" || in.RoutingAction == "resume"
	if in.RequestID == "" || strings.TrimSpace(in.Rationale) == "" || !validClassification(in.Classification) || !validCorrection || !validRouting || in.InterruptionMinutes < 0 || (in.InterruptionMinutes > 0 && !in.ResponderLoadConsent) || in.AgentCost < 0 || (!in.UserOutcomeConsent && in.UserOutcome != "") {
		return ErrInvalid
	}
	return nil
}

func (s *Store) LinkOutcomeWork(id, actor, requestID, proposalID, taskID string) (Alert, error) {
	var out Alert
	err := s.lock(func() error {
		v, err := s.read(id)
		if err != nil {
			return err
		}
		if proposalID == "" || taskID == "" {
			return ErrInvalid
		}
		for i := range v.OutcomeReviews {
			review := &v.OutcomeReviews[i]
			if review.RequestID != requestID || review.ActorID != actor {
				continue
			}
			if review.ProposalID != "" || review.TaskID != "" {
				if review.ProposalID != proposalID || review.TaskID != taskID {
					return ErrConflict
				}
				out = v
				return nil
			}
			review.ProposalID, review.TaskID = proposalID, taskID
			v.UpdatedAt = s.now()
			out = v
			return s.write(v)
		}
		return ErrNotFound
	})
	return out, err
}

func (s *Store) RoutingDirective(repo, rule string) string {
	values, err := s.List(repo)
	if err != nil {
		return ""
	}
	var latest *OutcomeReview
	for _, v := range values {
		if v.RuleID != rule {
			continue
		}
		for i := range v.OutcomeReviews {
			review := &v.OutcomeReviews[i]
			if review.RoutingAction == "" {
				continue
			}
			if latest == nil || review.CreatedAt.After(latest.CreatedAt) || (review.CreatedAt.Equal(latest.CreatedAt) && review.ID > latest.ID) {
				copy := *review
				latest = &copy
			}
		}
	}
	if latest == nil || latest.RoutingAction == "resume" {
		return ""
	}
	if latest.RoutingAction == "activate_backup" {
		return "backup"
	}
	return "pause"
}

func outcomeDigest(v OutcomeReviewInput) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func (s *Store) ApplyWorkspace(id, actor string, cmd WorkspaceCommand, allowed bool) (Alert, error) {
	var out Alert
	err := s.lock(func() error {
		v, err := s.read(id)
		if err != nil {
			return err
		}
		commandDigest := workspaceDigest(cmd)
		for _, entry := range v.Workspace.Timeline {
			if entry.RequestID == cmd.RequestID {
				if entry.Kind != cmd.Kind || entry.ActorID != actor || entry.RequestDigest != commandDigest {
					return ErrConflict
				}
				out = v
				return nil
			}
		}
		if !allowed || strings.TrimSpace(cmd.RequestID) == "" || strings.TrimSpace(cmd.Message) == "" {
			return ErrForbidden
		}
		now := s.now()
		entry := WorkspaceEntry{ID: stable(id, actor, cmd.RequestID), RequestID: cmd.RequestID, Kind: cmd.Kind, ActorID: actor, Message: strings.TrimSpace(cmd.Message), CreatedAt: now}
		entry.RequestDigest = commandDigest
		switch cmd.Kind {
		case "classify":
			if !validClassification(cmd.Classification) {
				return ErrInvalid
			}
			v.Workspace.Classification = cmd.Classification
		case "invite":
			if cmd.TargetUserID == "" {
				return ErrInvalid
			}
			v.Workspace.ParticipantIDs = unique(append(v.Workspace.ParticipantIDs, cmd.TargetUserID))
		case "reassign":
			if cmd.TargetUserID == "" {
				return ErrInvalid
			}
			v.Workspace.ResponderID = cmd.TargetUserID
			v.Workspace.ParticipantIDs = unique(append(v.Workspace.ParticipantIDs, cmd.TargetUserID))
		case "observe", "action", "correlate", "escalate":
			if len(cmd.Context) > 0 {
				if !validContext(cmd.Context) {
					return ErrInvalid
				}
				entry.Context = cmd.Context
				v.Workspace.Context = mergeContext(v.Workspace.Context, cmd.Context)
			}
		case "suppress":
			v.State = "suppressed"
		case "diagnostic":
			if !approvedDiagnostic(cmd.DiagnosticName) || (cmd.DiagnosticState != "passed" && cmd.DiagnosticState != "failed") || cmd.OutputDigest == "" {
				return ErrInvalid
			}
			v.Workspace.Diagnostics = append(v.Workspace.Diagnostics, DiagnosticRun{ID: entry.ID, Name: cmd.DiagnosticName, State: cmd.DiagnosticState, OutputDigest: cmd.OutputDigest, Summary: entry.Message, ActorID: actor, CreatedAt: now})
		case "delegate_agent":
			if cmd.AgentID == "" || cmd.Budget < 1 || cmd.Budget > 100 || len(cmd.PermittedTools) == 0 || !validReadOnlyTools(cmd.PermittedTools) || !validContext(cmd.Context) {
				return ErrInvalid
			}
			v.Workspace.Investigations = append(v.Workspace.Investigations, Investigation{ID: entry.ID, AgentID: cmd.AgentID, Mandate: entry.Message, State: "active", Context: cmd.Context, PermittedTools: unique(cmd.PermittedTools), Budget: cmd.Budget, CreatedBy: actor, CreatedAt: now})
			entry.Context = cmd.Context
		case "promote_incident":
			if cmd.IncidentID == "" || v.Workspace.IncidentID != "" {
				return ErrInvalid
			}
			v.Workspace.IncidentID = cmd.IncidentID
		default:
			return ErrInvalid
		}
		v.Workspace.Timeline = append(v.Workspace.Timeline, entry)
		v.Workspace.Version++
		v.UpdatedAt = now
		out = v
		return s.write(v)
	})
	return out, err
}

func signalContext(v Signal) []ContextBinding {
	out := []ContextBinding{}
	for _, e := range v.Evidence {
		out = append(out, ContextBinding{Kind: e.Kind, ResourceID: e.ResourceID, Revision: e.Revision, Summary: e.Summary, WindowFrom: v.OccurredAt, WindowTo: v.OccurredAt, Evidence: []Evidence{e}})
	}
	return out
}
func validClassification(v string) bool {
	return v == "actionable" || v == "false_positive" || v == "duplicate" || v == "needs_information" || v == "incident_candidate"
}
func validContext(v []ContextBinding) bool {
	if len(v) == 0 || len(v) > 50 {
		return false
	}
	kinds := map[string]bool{"release": true, "deployment": true, "code": true, "infrastructure": true, "dependency": true, "runbook": true, "evidence": true}
	for _, x := range v {
		if !kinds[x.Kind] || x.ResourceID == "" || x.Revision == "" || x.Summary == "" || x.WindowFrom.IsZero() || x.WindowTo.IsZero() || x.WindowTo.Before(x.WindowFrom) {
			return false
		}
	}
	return true
}
func mergeContext(a, b []ContextBinding) []ContextBinding {
	for _, x := range b {
		found := false
		for i := range a {
			if a[i].Kind == x.Kind && a[i].ResourceID == x.ResourceID && a[i].Revision == x.Revision {
				found = true
			}
		}
		if !found {
			a = append(a, x)
		}
	}
	return a
}
func approvedDiagnostic(v string) bool {
	return v == "health_snapshot" || v == "release_diff" || v == "dependency_status" || v == "log_summary" || v == "runbook_check"
}
func validReadOnlyTools(v []string) bool {
	allowed := map[string]bool{"read_context": true, "query_logs": true, "compare_releases": true, "inspect_dependencies": true, "read_runbook": true}
	for _, x := range v {
		if !allowed[x] {
			return false
		}
	}
	return true
}
func first(v []string) string {
	if len(v) > 0 {
		return v[0]
	}
	return ""
}
func workspaceDigest(v WorkspaceCommand) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func (s *Store) Append(id, request, kind, actor, reason string, allowed bool) (Alert, error) {
	var out Alert
	err := s.lock(func() error {
		v, e := s.read(id)
		if e != nil {
			return e
		}
		for _, x := range v.Events {
			if x.RequestID == request {
				if x.Kind != kind || x.ActorID != actor || x.Reason != reason {
					return ErrConflict
				}
				out = v
				return nil
			}
		}
		if request == "" || !allowed || (kind != "acknowledge" && kind != "resolve") {
			return ErrForbidden
		}
		now := s.now()
		v.Events = append(v.Events, Event{ID: stable(id, actor, request), RequestID: request, Kind: kind, ActorID: actor, Reason: reason, CreatedAt: now})
		v.State = map[string]string{"acknowledge": "acknowledged", "resolve": "resolved"}[kind]
		v.UpdatedAt = now
		out = v
		return s.write(v)
	})
	return out, err
}
func (s *Store) Get(id string) (Alert, error) {
	var v Alert
	e := s.lock(func() error { var x error; v, x = s.read(id); return x })
	return v, e
}
func (s *Store) List(repo string) ([]Alert, error) {
	var v []Alert
	e := s.lock(func() error { var x error; v, x = s.listUnlocked(repo); return x })
	return v, e
}
func (s *Store) listUnlocked(repo string) ([]Alert, error) {
	out := []Alert{}
	entries, e := os.ReadDir(s.root)
	if e != nil {
		return nil, e
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		v, x := s.read(strings.TrimSuffix(entry.Name(), ".json"))
		if x != nil {
			return nil, x
		}
		if v.RepositoryID == repo {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}
func validateSignal(v Signal) error {
	valid := map[string]bool{"reliability": true, "deployment": true, "security": true, "privacy": true, "dependency": true, "workflow": true, "user_impact": true}
	sev := map[string]bool{"low": true, "medium": true, "high": true, "critical": true}
	if !valid[v.SignalClass] || !sev[v.Severity] || len(v.ResourceIDs) == 0 || v.Summary == "" || v.Uncertainty == "" || v.OccurredAt.IsZero() || v.SourceRevision == "" || len(v.Evidence) == 0 || v.AffectedUserCount < 0 {
		return ErrInvalid
	}
	for _, e := range v.Evidence {
		if e.Kind == "" || e.ResourceID == "" || e.Revision == "" || e.Digest == "" || e.Summary == "" {
			return ErrInvalid
		}
	}
	return nil
}
func intersects(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}
func unique(v []string) []string {
	m := map[string]bool{}
	o := []string{}
	for _, x := range v {
		if x != "" && !m[x] {
			m[x] = true
			o = append(o, x)
		}
	}
	return o
}
func severityRank(v string) int {
	return map[string]int{"low": 1, "medium": 2, "high": 3, "critical": 4}[v]
}
func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
func digest(v Signal) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func stable(v ...string) string {
	h := sha256.Sum256([]byte(strings.Join(v, "\x00")))
	return hex.EncodeToString(h[:16])
}
func (s *Store) read(id string) (Alert, error) {
	var v Alert
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
func (s *Store) write(v Alert) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(s.root, ".alert-")
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
	if x := tmp.Close(); e == nil {
		e = x
	}
	if e == nil {
		e = os.Rename(name, filepath.Join(s.root, v.ID+".json"))
	}
	return e
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
