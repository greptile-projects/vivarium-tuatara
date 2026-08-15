package serviceobjectives

import (
	"errors"
	"sort"
	"strings"
	"time"
)

var ErrPolicyNotFound = errors.New("reliability delivery policy not found")

// DeliveryPolicy makes an objective actionable at delivery boundaries. It is
// policy context only: RequiredOwnerIDs acknowledge risk, but neither the
// policy nor an acknowledgement grants merge or environment authority.
type DeliveryPolicy struct {
	ID                       string    `json:"id"`
	Version                  int       `json:"version"`
	ContractVersion          int       `json:"contract_version"`
	ObjectiveIDs             []string  `json:"objective_ids"`
	Branches                 []string  `json:"branches"`
	Services                 []string  `json:"services"`
	EnvironmentIDs           []string  `json:"environment_ids"`
	JourneyIDs               []string  `json:"journey_ids"`
	RiskClasses              []string  `json:"risk_classes"`
	MaximumBudgetConsumed    float64   `json:"maximum_budget_consumed_percent"`
	MaximumPredictedIncrease float64   `json:"maximum_predicted_budget_increase_percent"`
	RequireCurrentEvidence   bool      `json:"require_current_evidence"`
	RequireDependencies      bool      `json:"require_dependencies"`
	RequiredOwnerIDs         []string  `json:"required_owner_ids"`
	MinimumAcknowledgements  int       `json:"minimum_acknowledgements"`
	OnMissingEvidence        string    `json:"on_missing_evidence"`
	OnBudgetExhausted        string    `json:"on_budget_exhausted"`
	OnRegression             string    `json:"on_regression"`
	OnDependencyFailure      string    `json:"on_dependency_failure"`
	Rationale                string    `json:"rationale"`
	CreatedBy                string    `json:"created_by"`
	CreatedAt                time.Time `json:"created_at"`
}

type ReliabilityImpact struct {
	ID                    string                 `json:"id"`
	PolicyID              string                 `json:"policy_id"`
	PolicyVersion         int                    `json:"policy_version"`
	Kind                  string                 `json:"kind"`
	ResourceID            string                 `json:"resource_id"`
	Revision              string                 `json:"revision"`
	Branch                string                 `json:"branch,omitempty"`
	Service               string                 `json:"service,omitempty"`
	EnvironmentID         string                 `json:"environment_id,omitempty"`
	JourneyIDs            []string               `json:"journey_ids"`
	RiskClasses           []string               `json:"risk_classes"`
	ObjectiveImpacts      []ObjectiveImpact      `json:"objective_impacts"`
	DependencyFailures    []string               `json:"dependency_failures"`
	Summary               string                 `json:"summary"`
	OwnerAcknowledgements []OwnerAcknowledgement `json:"owner_acknowledgements"`
	Exception             *DeliveryException     `json:"active_exception,omitempty"`
	RecordedBy            string                 `json:"recorded_by"`
	RecordedAt            time.Time              `json:"recorded_at"`
}
type ObjectiveImpact struct {
	ObjectiveID             string   `json:"objective_id"`
	ObservationID           string   `json:"observation_id,omitempty"`
	PredictedBudgetIncrease float64  `json:"predicted_budget_increase_percent"`
	ObservedBudgetConsumed  *float64 `json:"observed_budget_consumed_percent,omitempty"`
	Confidence              string   `json:"confidence"`
}
type OwnerAcknowledgement struct {
	OwnerID   string    `json:"owner_id"`
	Rationale string    `json:"rationale"`
	CreatedAt time.Time `json:"created_at"`
}
type DeliveryException struct {
	ID         string    `json:"id"`
	Reason     string    `json:"reason"`
	ApprovedBy string    `json:"approved_by"`
	ExpiresAt  time.Time `json:"expires_at"`
	FollowUp   string    `json:"follow_up"`
}
type DeliveryEvaluation struct {
	PolicyID             string             `json:"policy_id"`
	PolicyVersion        int                `json:"policy_version"`
	ImpactID             string             `json:"impact_id,omitempty"`
	State                string             `json:"state"`
	Effect               string             `json:"effect"`
	Blockers             []string           `json:"blockers"`
	RequiredOwnerIDs     []string           `json:"required_owner_ids"`
	AcknowledgedOwnerIDs []string           `json:"acknowledged_owner_ids"`
	ActiveException      *DeliveryException `json:"active_exception,omitempty"`
	AvailableNextActions []string           `json:"available_next_actions"`
	AuthorityNote        string             `json:"authority_note"`
}

func (s *Store) PublishDeliveryPolicy(contractID, actor string, expected int, p DeliveryPolicy) (Contract, error) {
	var out Contract
	err := s.lock(func() error {
		v, e := s.read(contractID)
		if e != nil {
			return e
		}
		if expected != v.CurrentVersion || !validDeliveryPolicy(v, p) {
			return ErrInvalid
		}
		p.ID, p.Version, p.CreatedBy, p.CreatedAt = id(), 1, actor, s.now()
		v.DeliveryPolicies = append(v.DeliveryPolicies, p)
		v.UpdatedAt = p.CreatedAt
		out = v
		return s.write(v)
	})
	return s.project(out), err
}

func (s *Store) RecordReliabilityImpact(contractID, actor string, x ReliabilityImpact) (Contract, error) {
	var out Contract
	err := s.lock(func() error {
		v, e := s.read(contractID)
		if e != nil {
			return e
		}
		p := deliveryPolicy(v, x.PolicyID)
		if p == nil || p.Version != x.PolicyVersion || !validImpact(*p, x) {
			return ErrInvalid
		}
		x.ID, x.RecordedBy, x.RecordedAt = id(), actor, s.now()
		x.OwnerAcknowledgements = []OwnerAcknowledgement{}
		v.ReliabilityImpacts = append(v.ReliabilityImpacts, x)
		v.UpdatedAt = x.RecordedAt
		out = v
		return s.write(v)
	})
	return s.project(out), err
}

func (s *Store) AcknowledgeReliabilityImpact(contractID, impactID, actor, rationale string) (Contract, error) {
	var out Contract
	err := s.lock(func() error {
		v, e := s.read(contractID)
		if e != nil {
			return e
		}
		for i := range v.ReliabilityImpacts {
			x := &v.ReliabilityImpacts[i]
			if x.ID != impactID {
				continue
			}
			p := deliveryPolicy(v, x.PolicyID)
			if p == nil || !deliveryContains(p.RequiredOwnerIDs, actor) || strings.TrimSpace(rationale) == "" {
				return ErrInvalid
			}
			for _, a := range x.OwnerAcknowledgements {
				if a.OwnerID == actor {
					return ErrConflict
				}
			}
			x.OwnerAcknowledgements = append(x.OwnerAcknowledgements, OwnerAcknowledgement{OwnerID: actor, Rationale: strings.TrimSpace(rationale), CreatedAt: s.now()})
			v.UpdatedAt = s.now()
			out = v
			return s.write(v)
		}
		return ErrNotFound
	})
	return s.project(out), err
}

func (s *Store) ExceptReliabilityImpact(contractID, impactID, actor string, ex DeliveryException) (Contract, error) {
	var out Contract
	err := s.lock(func() error {
		v, e := s.read(contractID)
		if e != nil {
			return e
		}
		for i := range v.ReliabilityImpacts {
			x := &v.ReliabilityImpacts[i]
			if x.ID != impactID {
				continue
			}
			p := deliveryPolicy(v, x.PolicyID)
			if p == nil || !deliveryContains(p.RequiredOwnerIDs, actor) || strings.TrimSpace(ex.Reason) == "" || !ex.ExpiresAt.After(s.now()) || ex.ExpiresAt.After(s.now().Add(30*24*time.Hour)) {
				return ErrInvalid
			}
			ex.ID = id()
			ex.ApprovedBy = actor
			x.Exception = &ex
			v.UpdatedAt = s.now()
			out = v
			return s.write(v)
		}
		return ErrNotFound
	})
	return s.project(out), err
}

func (s *Store) EvaluateReliability(repo, kind, resourceID, revision, branch, service, environment string, journeys, risks []string) ([]DeliveryEvaluation, error) {
	contracts, err := s.List(repo)
	if err != nil {
		return nil, err
	}
	out := []DeliveryEvaluation{}
	for _, v := range contracts {
		for _, p := range v.DeliveryPolicies {
			if !matches(p.Branches, branch) || !matches(p.Services, service) || !matches(p.EnvironmentIDs, environment) || !overlaps(p.JourneyIDs, journeys) || !overlaps(p.RiskClasses, risks) {
				continue
			}
			var latest *ReliabilityImpact
			for i := range v.ReliabilityImpacts {
				x := &v.ReliabilityImpacts[i]
				if x.PolicyID == p.ID && x.Kind == kind && x.ResourceID == resourceID && x.Revision == revision && (latest == nil || x.RecordedAt.After(latest.RecordedAt)) {
					latest = x
				}
			}
			e := evaluatePolicy(p, latest, s.now())
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PolicyID < out[j].PolicyID })
	return out, nil
}

func validDeliveryPolicy(v Contract, p DeliveryPolicy) bool {
	if p.ContractVersion < 1 || p.ContractVersion > v.CurrentVersion || len(p.ObjectiveIDs) == 0 || len(p.RequiredOwnerIDs) == 0 || p.MinimumAcknowledgements < 0 || p.MinimumAcknowledgements > len(p.RequiredOwnerIDs) || p.MaximumBudgetConsumed < 0 || p.MaximumBudgetConsumed > 100 || p.MaximumPredictedIncrease < 0 || strings.TrimSpace(p.Rationale) == "" {
		return false
	}
	for _, a := range []string{p.OnMissingEvidence, p.OnBudgetExhausted, p.OnRegression, p.OnDependencyFailure} {
		if !oneOf(a, "block", "slow", "pause", "rollback", "warn") {
			return false
		}
	}
	r := deliveryRevision(v, p.ContractVersion)
	if r == nil {
		return false
	}
	for _, x := range p.ObjectiveIDs {
		if !deliveryObjective(*r, x) {
			return false
		}
	}
	for _, x := range p.RequiredOwnerIDs {
		if !deliveryContains(r.OwnerIDs, x) {
			found := false
			for _, o := range r.Objectives {
				found = found || deliveryContains(o.OwnerIDs, x)
			}
			if !found {
				return false
			}
		}
	}
	return true
}
func validImpact(p DeliveryPolicy, x ReliabilityImpact) bool {
	if !oneOf(x.Kind, "pull_request", "integration_queue", "release", "deployment") || x.ResourceID == "" || x.Revision == "" || strings.TrimSpace(x.Summary) == "" || len(x.ObjectiveImpacts) == 0 {
		return false
	}
	for _, i := range x.ObjectiveImpacts {
		if !deliveryContains(p.ObjectiveIDs, i.ObjectiveID) || i.PredictedBudgetIncrease < 0 || !oneOf(i.Confidence, "low", "medium", "high") {
			return false
		}
	}
	return true
}
func deliveryPolicy(v Contract, id string) *DeliveryPolicy {
	for i := range v.DeliveryPolicies {
		if v.DeliveryPolicies[i].ID == id {
			return &v.DeliveryPolicies[i]
		}
	}
	return nil
}
func evaluatePolicy(p DeliveryPolicy, x *ReliabilityImpact, now time.Time) DeliveryEvaluation {
	e := DeliveryEvaluation{PolicyID: p.ID, PolicyVersion: p.Version, State: "passed", Effect: "none", RequiredOwnerIDs: p.RequiredOwnerIDs, AcknowledgedOwnerIDs: []string{}, Blockers: []string{}, AvailableNextActions: []string{}, AuthorityNote: "Reliability policy and acknowledgements grant no merge, release, queue, deployment, or environment authority."}
	add := func(msg, effect string) {
		e.Blockers = append(e.Blockers, msg)
		if rankEffect(effect) > rankEffect(e.Effect) {
			e.Effect = effect
		}
		e.State = "blocked"
	}
	if x == nil {
		if p.RequireCurrentEvidence {
			add("current reliability impact evidence is missing", p.OnMissingEvidence)
		}
		e.AvailableNextActions = []string{"record_predicted_impact", "collect_current_observation"}
		return e
	}
	for _, a := range x.OwnerAcknowledgements {
		e.AcknowledgedOwnerIDs = append(e.AcknowledgedOwnerIDs, a.OwnerID)
	}
	if x.Exception != nil && x.Exception.ExpiresAt.After(now) {
		e.ActiveException = x.Exception
		e.State = "excepted"
		e.AvailableNextActions = []string{"monitor_exception", "complete_follow_up"}
		return e
	}
	for _, i := range x.ObjectiveImpacts {
		if i.PredictedBudgetIncrease > p.MaximumPredictedIncrease {
			add("predicted error-budget increase exceeds policy", p.OnRegression)
		}
		if i.ObservedBudgetConsumed != nil && *i.ObservedBudgetConsumed >= p.MaximumBudgetConsumed {
			add("error budget is exhausted", p.OnBudgetExhausted)
		}
		if p.RequireCurrentEvidence && i.ObservationID == "" {
			add("current objective evidence is missing", p.OnMissingEvidence)
		}
	}
	if p.RequireDependencies && len(x.DependencyFailures) > 0 {
		add("dependency reliability has failed", p.OnDependencyFailure)
	}
	if len(e.AcknowledgedOwnerIDs) < p.MinimumAcknowledgements {
		add("required reliability owner acknowledgements are missing", "block")
	}
	if len(e.Blockers) == 0 {
		e.State = "passed"
	}
	e.AvailableNextActions = []string{"acknowledge_risk", "request_exception", "collect_evidence", "open_reliability_investigation"}
	if e.Effect == "pause" || e.Effect == "rollback" {
		e.AvailableNextActions = append(e.AvailableNextActions, "pause_affected_rollout", "restore_known_good")
	}
	return e
}
func rankEffect(x string) int {
	return map[string]int{"none": 0, "warn": 1, "slow": 2, "block": 3, "pause": 4, "rollback": 5}[x]
}
func matches(s []string, v string) bool { return len(s) == 0 || v == "" || deliveryContains(s, v) }
func overlaps(s, v []string) bool {
	if len(s) == 0 || len(v) == 0 {
		return true
	}
	for _, x := range s {
		if deliveryContains(v, x) {
			return true
		}
	}
	return false
}

func deliveryContains(values []string, value string) bool {
	for _, x := range values {
		if x == value {
			return true
		}
	}
	return false
}
func deliveryRevision(v Contract, version int) *Revision {
	for i := range v.Revisions {
		if v.Revisions[i].Version == version {
			return &v.Revisions[i]
		}
	}
	return nil
}
func deliveryObjective(v Revision, id string) bool {
	for _, x := range v.Objectives {
		if x.ID == id {
			return true
		}
	}
	return false
}
