package capabilities

import (
	"strings"
	"time"
)

var deliveryKinds = map[string]bool{"merge_queue": true, "release": true, "schema_migration": true, "infrastructure_migration": true, "documentation": true, "deployment": true}
var cleanupKinds = map[string]bool{"code": true, "flags": true, "data": true, "credentials": true, "telemetry": true, "documentation": true, "policy_exceptions": true}

type DeliveryReference struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Revision   string `json:"revision"`
	Status     string `json:"status"`
}

type StageReport struct {
	Version               int                 `json:"version"`
	StageIndex            int                 `json:"stage_index"`
	Stage                 string              `json:"stage"`
	Action                string              `json:"action"`
	RemainingUse          int64               `json:"remaining_use"`
	Health                string              `json:"health"`
	Control               string              `json:"control"`
	RollbackBoundary      string              `json:"rollback_boundary"`
	NextAction            string              `json:"next_action"`
	UnexpectedConsumers   []string            `json:"unexpected_consumers,omitempty"`
	Delivery              []DeliveryReference `json:"delivery"`
	CompatibilityRestored bool                `json:"compatibility_restored"`
	ReportedBy            string              `json:"reported_by"`
	CreatedAt             time.Time           `json:"created_at"`
}

type CleanupProof struct {
	Kind     string   `json:"kind"`
	Revision string   `json:"revision"`
	Paths    []string `json:"paths"`
	Digest   string   `json:"digest"`
	Summary  string   `json:"summary"`
}

type RemovalCompletion struct {
	Proofs     []CleanupProof `json:"proofs"`
	VerifiedBy string         `json:"verified_by"`
	VerifiedAt time.Time      `json:"verified_at"`
}

type RemovalExecution struct {
	ID           string             `json:"id"`
	CandidateID  string             `json:"candidate_id"`
	Version      int                `json:"version"`
	Status       string             `json:"status"`
	ActiveStage  int                `json:"active_stage"`
	StageNames   []string           `json:"stage_names"`
	Reports      []StageReport      `json:"reports"`
	Completion   *RemovalCompletion `json:"completion,omitempty"`
	ControllerID string             `json:"controller_id"`
	StartedAt    time.Time          `json:"started_at"`
	UpdatedAt    time.Time          `json:"updated_at"`
}

func findPlanAndCandidate(v *Capability, planID, candidateID string) (*RetirementPlan, *MigrationCandidate) {
	for pi := range v.RetirementPlans {
		p := &v.RetirementPlans[pi]
		if p.ID != planID {
			continue
		}
		for ci := range p.Candidates {
			if p.Candidates[ci].ID == candidateID {
				return p, &p.Candidates[ci]
			}
		}
		return p, nil
	}
	return nil, nil
}

func (s *Store) StartRemoval(repo, capabilityID, planID, candidateID, actor string) (Capability, RemovalExecution, error) {
	var out Capability
	var execution RemovalExecution
	err := s.lock(func() error {
		v, err := s.read(repo, capabilityID)
		if err != nil {
			return err
		}
		p, c := findPlanAndCandidate(&v, planID, candidateID)
		if p == nil {
			return ErrPlanNotFound
		}
		if c == nil {
			return ErrInvalid
		}
		projectRetirement(p, v.CurrentVersion, s.project(v).Diagnostics, s.now())
		ProjectCandidate(c, *p)
		if actor == "" || len(p.Blockers) != 0 || !c.RemovalReady {
			return ErrConflict
		}
		for _, x := range p.Executions {
			if x.Status != "completed" && x.Status != "restored" {
				return ErrConflict
			}
		}
		stages := make([]string, len(p.Stages))
		for i := range p.Stages {
			stages[i] = p.Stages[i].Name
		}
		now := s.now()
		execution = RemovalExecution{ID: randomID(), CandidateID: candidateID, Version: 1, Status: "running", ActiveStage: 0, StageNames: stages, ControllerID: actor, StartedAt: now, UpdatedAt: now}
		p.Executions = append(p.Executions, execution)
		p.UpdatedAt, v.UpdatedAt, out = now, now, v
		return s.write(v)
	})
	return s.project(out), execution, err
}

func validStageReport(r StageReport) bool {
	if r.RemainingUse < 0 || !map[string]bool{"healthy": true, "degraded": true, "failed": true}[r.Health] || !map[string]bool{"advance": true, "pause": true, "resume": true, "restore": true}[r.Action] || strings.TrimSpace(r.Control) == "" || strings.TrimSpace(r.RollbackBoundary) == "" || strings.TrimSpace(r.NextAction) == "" || len(r.Delivery) == 0 {
		return false
	}
	for _, d := range r.Delivery {
		if !deliveryKinds[d.Kind] || d.ResourceID == "" || len(d.Revision) != 40 || !map[string]bool{"pending": true, "succeeded": true, "failed": true}[d.Status] {
			return false
		}
	}
	return true
}

func (s *Store) ReportRemovalStage(repo, capabilityID, planID, executionID, actor string, expected int, report StageReport) (Capability, error) {
	var out Capability
	err := s.lock(func() error {
		v, err := s.read(repo, capabilityID)
		if err != nil {
			return err
		}
		var p *RetirementPlan
		var x *RemovalExecution
		for pi := range v.RetirementPlans {
			if v.RetirementPlans[pi].ID == planID {
				p = &v.RetirementPlans[pi]
				for ei := range p.Executions {
					if p.Executions[ei].ID == executionID {
						x = &p.Executions[ei]
					}
				}
			}
		}
		if p == nil {
			return ErrPlanNotFound
		}
		if x == nil {
			return ErrInvalid
		}
		if x.ControllerID != actor || x.Version != expected || !validStageReport(report) || x.Status == "completed" || x.Status == "awaiting_verification" {
			return ErrConflict
		}
		if report.StageIndex != x.ActiveStage || report.Stage != x.StageNames[x.ActiveStage] {
			return ErrInvalid
		}
		if x.Status == "paused" && report.Action != "resume" && report.Action != "restore" {
			return ErrConflict
		}
		if x.Status != "paused" && report.Action == "resume" {
			return ErrConflict
		}
		unsafe := report.Health != "healthy" || report.RemainingUse > 0 || len(report.UnexpectedConsumers) > 0
		if report.Action == "advance" && unsafe {
			report.Action = "pause"
		}
		if report.Action == "restore" && !report.CompatibilityRestored {
			return ErrInvalid
		}
		now := s.now()
		report.Version = len(x.Reports) + 1
		report.ReportedBy = actor
		report.CreatedAt = now
		x.Reports = append(x.Reports, report)
		x.Version++
		x.UpdatedAt = now
		switch report.Action {
		case "pause":
			x.Status = "paused"
		case "resume":
			if unsafe {
				return ErrConflict
			}
			x.Status = "running"
		case "restore":
			x.Status = "restored"
		case "advance":
			for _, d := range report.Delivery {
				if d.Status != "succeeded" {
					x.Status = "paused"
					p.UpdatedAt = now
					v.UpdatedAt = now
					out = v
					return s.write(v)
				}
			}
			if x.ActiveStage == len(x.StageNames)-1 {
				x.Status = "awaiting_verification"
			} else {
				x.ActiveStage++
				x.Status = "running"
			}
		}
		p.UpdatedAt = now
		v.UpdatedAt = now
		out = v
		return s.write(v)
	})
	return s.project(out), err
}

func validCleanup(proofs []CleanupProof) bool {
	seen := map[string]bool{}
	for _, p := range proofs {
		if !cleanupKinds[p.Kind] || seen[p.Kind] || len(p.Revision) != 40 || len(p.Paths) == 0 || p.Digest == "" || p.Summary == "" {
			return false
		}
		seen[p.Kind] = true
	}
	return len(seen) == len(cleanupKinds)
}

func (s *Store) CompleteRemoval(repo, capabilityID, planID, executionID, actor string, expected int, proofs []CleanupProof) (Capability, error) {
	var out Capability
	err := s.lock(func() error {
		v, err := s.read(repo, capabilityID)
		if err != nil {
			return err
		}
		var p *RetirementPlan
		var x *RemovalExecution
		for pi := range v.RetirementPlans {
			if v.RetirementPlans[pi].ID == planID {
				p = &v.RetirementPlans[pi]
				for ei := range p.Executions {
					if p.Executions[ei].ID == executionID {
						x = &p.Executions[ei]
					}
				}
			}
		}
		if p == nil {
			return ErrPlanNotFound
		}
		if x == nil {
			return ErrInvalid
		}
		var candidate *MigrationCandidate
		for ci := range p.Candidates {
			if p.Candidates[ci].ID == x.CandidateID {
				candidate = &p.Candidates[ci]
			}
		}
		projectRetirement(p, v.CurrentVersion, s.project(v).Diagnostics, s.now())
		if candidate != nil {
			ProjectCandidate(candidate, *p)
		}
		if x.ControllerID != actor || x.Version != expected || x.Status != "awaiting_verification" || candidate == nil || !candidate.RemovalReady || len(p.Blockers) != 0 || !validCleanup(proofs) {
			return ErrConflict
		}
		now := s.now()
		x.Version++
		x.Status = "completed"
		x.Completion = &RemovalCompletion{Proofs: proofs, VerifiedBy: actor, VerifiedAt: now}
		x.UpdatedAt = now
		p.UpdatedAt = now
		v.UpdatedAt = now
		out = v
		return s.write(v)
	})
	return s.project(out), err
}
