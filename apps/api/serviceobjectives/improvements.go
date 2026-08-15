package serviceobjectives

import (
	"strings"
	"time"
)

// Improvement keeps the operational reason for ordinary proposal work frozen
// alongside the exact objective evidence that authorized it.
type Improvement struct {
	ID                     string    `json:"id"`
	ContractVersion        int       `json:"contract_version"`
	ObjectiveID            string    `json:"objective_id"`
	InvestigationID        string    `json:"investigation_id,omitempty"`
	FindingID              string    `json:"finding_id,omitempty"`
	ImpactID               string    `json:"impact_id,omitempty"`
	BaselineObservationIDs []string  `json:"baseline_observation_ids"`
	AffectedObservationIDs []string  `json:"affected_observation_ids"`
	AffectedRevisions      []string  `json:"affected_revisions"`
	DependencyContext      []string  `json:"dependency_context"`
	EvidenceIDs            []string  `json:"evidence_ids"`
	AcceptanceCriteria     []string  `json:"acceptance_criteria"`
	ProposalID             string    `json:"proposal_id"`
	TaskIDs                []string  `json:"task_ids"`
	CreatedBy              string    `json:"created_by"`
	CreatedAt              time.Time `json:"created_at"`
}

type RolloutVerification struct {
	ID                    string    `json:"id"`
	ImprovementID         string    `json:"improvement_id"`
	Kind                  string    `json:"kind"`
	ResourceID            string    `json:"resource_id"`
	Revision              string    `json:"revision"`
	BaselineObservationID string    `json:"baseline_observation_id"`
	CurrentObservationID  string    `json:"current_observation_id"`
	Decision              string    `json:"decision"`
	Rationale             string    `json:"rationale"`
	BudgetBefore          *float64  `json:"budget_before_percent,omitempty"`
	BudgetAfter           *float64  `json:"budget_after_percent,omitempty"`
	Improved              bool      `json:"improved"`
	BudgetRestored        bool      `json:"budget_restored"`
	RecordedBy            string    `json:"recorded_by"`
	RecordedAt            time.Time `json:"recorded_at"`
}

func (s *Store) ValidateImprovement(contractID string, in Improvement) error {
	v, err := s.read(contractID)
	if err != nil {
		return err
	}
	if !validImprovement(v, in) {
		return ErrInvalid
	}
	return nil
}

func (s *Store) LinkImprovement(contractID, actor string, in Improvement) (Contract, error) {
	var out Contract
	err := s.lock(func() error {
		v, e := s.read(contractID)
		if e != nil {
			return e
		}
		if actor == "" || !validImprovement(v, in) {
			return ErrInvalid
		}
		for _, x := range v.Improvements {
			if x.InvestigationID == in.InvestigationID && x.FindingID == in.FindingID && x.ImpactID == in.ImpactID {
				if x.ProposalID == in.ProposalID {
					out = v
					return nil
				}
				return ErrConflict
			}
		}
		in.ID, in.CreatedBy, in.CreatedAt = id(), actor, s.now()
		v.Improvements = append(v.Improvements, in)
		v.UpdatedAt = in.CreatedAt
		out = v
		return s.write(v)
	})
	return s.project(out), err
}

func validImprovement(v Contract, x Improvement) bool {
	if x.ContractVersion < 1 || x.ObjectiveID == "" || (x.FindingID == "") == (x.ImpactID == "") || x.ProposalID == "" || len(x.TaskIDs) == 0 || len(x.TaskIDs) > 20 || len(x.AcceptanceCriteria) == 0 || len(x.AcceptanceCriteria) > 50 || len(x.BaselineObservationIDs) == 0 || len(x.AffectedObservationIDs) == 0 || len(x.AffectedRevisions) == 0 || len(x.EvidenceIDs) == 0 {
		return false
	}
	r := deliveryRevision(v, x.ContractVersion)
	if r == nil || !deliveryObjective(*r, x.ObjectiveID) {
		return false
	}
	observations, affectedRevisions, evidence := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, o := range v.Observations {
		if o.ContractVersion == x.ContractVersion && o.ObjectiveID == x.ObjectiveID {
			observations[o.ID] = true
			evidence[o.ID] = true
			if deliveryContains(x.AffectedObservationIDs, o.ID) {
				for _, software := range o.Software {
					affectedRevisions[software.Revision] = true
				}
			}
		}
	}
	for _, oid := range append(append([]string{}, x.BaselineObservationIDs...), x.AffectedObservationIDs...) {
		if !observations[oid] {
			return false
		}
	}
	if x.FindingID != "" {
		found := false
		for _, inv := range v.Investigations {
			if inv.ID == x.InvestigationID && inv.ContractVersion == x.ContractVersion && inv.ObjectiveID == x.ObjectiveID {
				for _, item := range inv.Evidence {
					evidence[item.ResourceID] = true
				}
				for _, f := range inv.Findings {
					if f.ID == x.FindingID {
						found = true
						for _, citation := range f.CitationIDs {
							evidence[citation] = true
						}
					}
				}
			}
		}
		if !found {
			return false
		}
	}
	if x.ImpactID != "" {
		found := false
		for _, impact := range v.ReliabilityImpacts {
			if impact.ID == x.ImpactID {
				for _, objective := range impact.ObjectiveImpacts {
					found = found || objective.ObjectiveID == x.ObjectiveID
					evidence[objective.ObservationID] = true
				}
			}
		}
		if !found {
			return false
		}
	}
	for _, revision := range x.AffectedRevisions {
		if !affectedRevisions[revision] {
			return false
		}
	}
	for _, evidenceID := range x.EvidenceIDs {
		if !evidence[evidenceID] {
			return false
		}
	}
	for _, values := range [][]string{x.AcceptanceCriteria, x.AffectedRevisions, x.DependencyContext, x.EvidenceIDs, x.TaskIDs} {
		for _, value := range values {
			if strings.TrimSpace(value) == "" || len(value) > 1000 {
				return false
			}
		}
	}
	return true
}

func (s *Store) VerifyImprovement(contractID, actor string, in RolloutVerification) (Contract, error) {
	var out Contract
	err := s.lock(func() error {
		v, e := s.read(contractID)
		if e != nil {
			return e
		}
		var improvement *Improvement
		for i := range v.Improvements {
			if v.Improvements[i].ID == in.ImprovementID {
				improvement = &v.Improvements[i]
			}
		}
		if improvement == nil || actor == "" || !oneOf(in.Kind, "release", "deployment") || !oneOf(in.Decision, "restore_budget", "contain", "rollback", "revisit") || strings.TrimSpace(in.Rationale) == "" {
			return ErrInvalid
		}
		projected := s.project(v)
		var baseline, current *Observation
		for i := range projected.Observations {
			o := &projected.Observations[i]
			if o.ID == in.BaselineObservationID {
				baseline = o
			}
			if o.ID == in.CurrentObservationID {
				current = o
			}
		}
		if baseline == nil || current == nil || baseline.ContractVersion != improvement.ContractVersion || current.ContractVersion != improvement.ContractVersion || baseline.ObjectiveID != improvement.ObjectiveID || current.ObjectiveID != improvement.ObjectiveID || baseline.ErrorBudgetConsumed == nil || current.ErrorBudgetConsumed == nil || !current.WindowEnd.After(baseline.WindowEnd) || !observationDelivers(*current, in.Kind, in.ResourceID, in.Revision) {
			return ErrInvalid
		}
		in.ID, in.BudgetBefore, in.BudgetAfter = id(), baseline.ErrorBudgetConsumed, current.ErrorBudgetConsumed
		in.Improved = *current.ErrorBudgetConsumed < *baseline.ErrorBudgetConsumed && current.TargetMet != nil && *current.TargetMet
		in.BudgetRestored = in.Improved && in.Decision == "restore_budget"
		if (in.Decision == "restore_budget") != in.Improved {
			return ErrInvalid
		}
		in.RecordedBy, in.RecordedAt = actor, s.now()
		v.RolloutVerifications = append(v.RolloutVerifications, in)
		v.UpdatedAt = in.RecordedAt
		out = v
		return s.write(v)
	})
	return s.project(out), err
}

func observationDelivers(o Observation, kind, resourceID, revision string) bool {
	for _, x := range o.Software {
		if x.Kind == kind && x.ID == resourceID && x.Revision == revision {
			return true
		}
	}
	return false
}
