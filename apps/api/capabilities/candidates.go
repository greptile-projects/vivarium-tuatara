package capabilities

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

var candidateStages = map[string]bool{"old_only": true, "dual_support": true, "replacement": true, "rollback": true, "journey": true}

type CandidateRevision struct {
	RepositoryID string   `json:"repository_id"`
	Revision     string   `json:"revision"`
	Paths        []string `json:"paths"`
}

type CandidateCheck struct {
	ID           string              `json:"id"`
	Stage        string              `json:"stage"`
	Journey      string              `json:"journey,omitempty"`
	RepositoryID string              `json:"repository_id"`
	Revision     string              `json:"revision"`
	Command      string              `json:"command"`
	Paths        []string            `json:"paths"`
	Expectation  string              `json:"expectation"`
	Evidence     []CandidateEvidence `json:"evidence,omitempty"`
	Status       string              `json:"status"`
}

type CandidateArtifact struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
	Size   int    `json:"size"`
}
type CandidateEvidence struct {
	ID            string              `json:"id"`
	WorkspaceID   string              `json:"workspace_id"`
	OutcomeID     string              `json:"outcome_id"`
	Status        string              `json:"status"`
	ExitCode      int                 `json:"exit_code"`
	SanitizedLog  string              `json:"sanitized_log"`
	CommandDigest string              `json:"command_digest"`
	DurationMS    int64               `json:"duration_ms"`
	CostUnits     float64             `json:"cost_units"`
	Artifacts     []CandidateArtifact `json:"artifacts,omitempty"`
	CreatedBy     string              `json:"created_by"`
	CreatedAt     time.Time           `json:"created_at"`
	Superseded    bool                `json:"superseded"`
	Stale         bool                `json:"stale"`
}

type UsageObservation struct {
	ID              string    `json:"id"`
	ConsumerIndex   int       `json:"consumer_index"`
	RepositoryID    string    `json:"repository_id,omitempty"`
	Revision        string    `json:"revision,omitempty"`
	State           string    `json:"state"`
	OldBehaviorUses int64     `json:"old_behavior_uses"`
	TotalUses       int64     `json:"total_uses"`
	WindowStartsAt  time.Time `json:"window_starts_at"`
	WindowEndsAt    time.Time `json:"window_ends_at"`
	ArtifactDigest  string    `json:"artifact_digest,omitempty"`
	Summary         string    `json:"summary"`
	OwnerID         string    `json:"owner_id,omitempty"`
	Acknowledged    bool      `json:"acknowledged"`
	CreatedAt       time.Time `json:"created_at"`
	Superseded      bool      `json:"superseded"`
}

type CleanupRequirement struct {
	ID              string `json:"id"`
	Kind            string `json:"kind"`
	RepositoryID    string `json:"repository_id"`
	Path            string `json:"path"`
	Revision        string `json:"revision"`
	PreviousBlob    string `json:"previous_blob"`
	Expectation     string `json:"expectation"`
	ReplacementBlob string `json:"replacement_blob,omitempty"`
}

type MigrationCandidate struct {
	ID                  string               `json:"id"`
	PlanVersion         int                  `json:"plan_version"`
	CapabilityVersion   int                  `json:"capability_version"`
	ProviderRevision    string               `json:"provider_revision"`
	ReleaseID           string               `json:"release_id"`
	ReleaseVersion      string               `json:"release_version"`
	SchemaConfiguration []CandidateRevision  `json:"schema_configuration"`
	Consumers           []CandidateRevision  `json:"consumers"`
	Environment         string               `json:"environment"`
	Checks              []CandidateCheck     `json:"checks"`
	Usage               []UsageObservation   `json:"usage_observations"`
	CleanupRequirements []CleanupRequirement `json:"cleanup_requirements"`
	Status              string               `json:"status"`
	RemovalReady        bool                 `json:"removal_ready"`
	Blockers            []RetirementBlocker  `json:"blockers"`
	CreatedBy           string               `json:"created_by"`
	CreatedAt           time.Time            `json:"created_at"`
	UpdatedAt           time.Time            `json:"updated_at"`
}

func CommandDigest(command string) string {
	sum := sha256.Sum256([]byte(command))
	return hex.EncodeToString(sum[:])
}

func validateCandidate(c MigrationCandidate, p RetirementPlan, r Revision) bool {
	if strings.TrimSpace(c.Environment) == "" || len(c.Checks) < 5 || p.CapabilityVersion != r.Version || len(c.CleanupRequirements) < len(cleanupKinds) {
		return false
	}
	seen, stages := map[string]bool{}, map[string]bool{}
	for _, x := range c.Checks {
		if x.ID == "" || seen[x.ID] || !candidateStages[x.Stage] || x.RepositoryID == "" || len(x.Revision) != 40 || strings.TrimSpace(x.Command) == "" || strings.TrimSpace(x.Expectation) == "" || len(x.Paths) == 0 {
			return false
		}
		if x.Stage == "journey" && strings.TrimSpace(x.Journey) == "" {
			return false
		}
		seen[x.ID], stages[x.Stage] = true, true
	}
	for stage := range candidateStages {
		if !stages[stage] {
			return false
		}
	}
	requirementIDs, requirementKinds, requirementSurfaces := map[string]bool{}, map[string]bool{}, map[string]bool{}
	capabilityPaths, coveredPaths := map[string]bool{}, map[string]bool{}
	for _, item := range r.Items {
		if item.Path != "" {
			capabilityPaths[item.Path] = true
		}
	}
	for _, requirement := range c.CleanupRequirements {
		surface := requirement.Kind + "\x00" + requirement.Path
		if requirement.ID == "" || requirementIDs[requirement.ID] || requirementSurfaces[surface] || !cleanupKinds[requirement.Kind] || requirement.RepositoryID == "" || !capabilityPaths[requirement.Path] || len(requirement.Revision) != 40 || requirement.PreviousBlob == "" || (requirement.Expectation != "removed" && requirement.Expectation != "replaced") || (requirement.Expectation == "replaced" && requirement.ReplacementBlob == requirement.PreviousBlob) {
			return false
		}
		requirementIDs[requirement.ID], requirementKinds[requirement.Kind] = true, true
		requirementSurfaces[surface] = true
		coveredPaths[requirement.Path] = true
	}
	for kind := range cleanupKinds {
		if !requirementKinds[kind] {
			return false
		}
	}
	for path := range capabilityPaths {
		if !coveredPaths[path] {
			return false
		}
	}
	return true
}

func ProjectCandidate(c *MigrationCandidate, p RetirementPlan) {
	c.Blockers = nil
	usedOutcomes := map[string]string{}
	for i := range c.Checks {
		x := &c.Checks[i]
		x.Status = "missing"
		for j := range x.Evidence {
			x.Evidence[j].Superseded = j != len(x.Evidence)-1
		}
		if len(x.Evidence) > 0 {
			e := x.Evidence[len(x.Evidence)-1]
			if e.Stale {
				x.Status = "stale"
			} else if priorCheck, reused := usedOutcomes[e.OutcomeID]; e.OutcomeID == "" || (reused && priorCheck != x.ID) {
				x.Status = "reused_outcome"
			} else {
				x.Status = e.Status
				usedOutcomes[e.OutcomeID] = x.ID
			}
		}
		if x.Status != "passed" {
			c.Blockers = append(c.Blockers, RetirementBlocker{Kind: "check_" + x.Status, Message: "Current " + x.Stage + " proof is not passing."})
		}
	}
	latest := map[int]UsageObservation{}
	for i := range c.Usage {
		c.Usage[i].Superseded = true
		latest[c.Usage[i].ConsumerIndex] = c.Usage[i]
	}
	for i := range c.Usage {
		if v := latest[c.Usage[i].ConsumerIndex]; v.ID == c.Usage[i].ID {
			c.Usage[i].Superseded = false
		}
	}
	for i, audience := range p.Audiences {
		u, ok := latest[i]
		if !ok || u.State == "unmeasured" || u.State == "inaccessible" {
			c.Blockers = append(c.Blockers, RetirementBlocker{Kind: "usage_not_measured", Message: "Remaining use is inaccessible or unmeasured.", Audience: audience.Name})
			continue
		}
		if u.State != "measured" || u.OldBehaviorUses != 0 {
			c.Blockers = append(c.Blockers, RetirementBlocker{Kind: "residual_dependent", Message: "Measured use of the old behavior remains.", Audience: audience.Name})
		}
		if !u.Acknowledged {
			c.Blockers = append(c.Blockers, RetirementBlocker{Kind: "usage_owner_acknowledgement_required", Message: "The affected owner has not acknowledged the usage observation.", Audience: audience.Name, OwnerID: u.OwnerID})
		}
	}
	c.RemovalReady = len(c.Blockers) == 0
	if c.RemovalReady {
		c.Status = "ready"
	} else {
		c.Status = "blocked"
	}
}

func (s *Store) CreateMigrationCandidate(repo, capabilityID, planID, actor string, c MigrationCandidate) (Capability, MigrationCandidate, error) {
	var out Capability
	err := s.lock(func() error {
		v, err := s.read(repo, capabilityID)
		if err != nil {
			return err
		}
		var p *RetirementPlan
		for i := range v.RetirementPlans {
			if v.RetirementPlans[i].ID == planID {
				p = &v.RetirementPlans[i]
			}
		}
		if p == nil {
			return ErrPlanNotFound
		}
		if p.CapabilityVersion < 1 || p.CapabilityVersion > len(v.Revisions) || !validateCandidate(c, *p, v.Revisions[p.CapabilityVersion-1]) {
			return ErrInvalid
		}
		r := v.Revisions[p.CapabilityVersion-1]
		allowedRevisions := map[string]string{repo: r.CommitID}
		for _, consumer := range r.Consumers {
			if consumer.RepositoryID != "" {
				allowedRevisions[consumer.RepositoryID] = consumer.Revision
			}
		}
		for _, check := range c.Checks {
			if allowedRevisions[check.RepositoryID] == "" || !strings.EqualFold(allowedRevisions[check.RepositoryID], check.Revision) {
				return ErrInvalid
			}
		}
		for _, requirement := range c.CleanupRequirements {
			if requirement.RepositoryID != repo || !strings.EqualFold(requirement.Revision, r.CommitID) {
				return ErrInvalid
			}
		}
		now := s.now()
		c.ID = randomID()
		c.PlanVersion = p.WorkVersion
		c.CapabilityVersion = p.CapabilityVersion
		c.ProviderRevision = r.CommitID
		c.ReleaseID = r.ReleaseID
		c.ReleaseVersion = r.ReleaseVersion
		c.CreatedBy = actor
		c.CreatedAt = now
		c.UpdatedAt = now
		c.Consumers = nil
		for _, x := range r.Consumers {
			c.Consumers = append(c.Consumers, CandidateRevision{RepositoryID: x.RepositoryID, Revision: x.Revision})
		}
		for _, x := range r.Items {
			if x.Kind == "schema" || x.Kind == "configuration" {
				c.SchemaConfiguration = append(c.SchemaConfiguration, CandidateRevision{RepositoryID: repo, Revision: x.Revision, Paths: []string{x.Path}})
			}
		}
		ProjectCandidate(&c, *p)
		p.Candidates = append(p.Candidates, c)
		p.UpdatedAt = now
		v.UpdatedAt = now
		out = v
		return s.write(v)
	})
	return s.project(out), c, err
}

func (s *Store) AddCandidateEvidence(repo, capabilityID, planID, candidateID, actor, checkID string, e CandidateEvidence) (Capability, error) {
	var out Capability
	err := s.lock(func() error {
		v, err := s.read(repo, capabilityID)
		if err != nil {
			return err
		}
		for pi := range v.RetirementPlans {
			p := &v.RetirementPlans[pi]
			if p.ID != planID {
				continue
			}
			for ci := range p.Candidates {
				c := &p.Candidates[ci]
				if c.ID != candidateID {
					continue
				}
				for xi := range c.Checks {
					if c.Checks[xi].ID == checkID {
						if e.OutcomeID == "" {
							return ErrInvalid
						}
						for otherCheckIndex := range c.Checks {
							if c.Checks[otherCheckIndex].ID == checkID {
								continue
							}
							for _, retained := range c.Checks[otherCheckIndex].Evidence {
								if retained.OutcomeID == e.OutcomeID {
									return ErrInvalid
								}
							}
						}
						e.ID = randomID()
						e.CreatedBy = actor
						e.CreatedAt = s.now()
						c.Checks[xi].Evidence = append(c.Checks[xi].Evidence, e)
						c.UpdatedAt = s.now()
						ProjectCandidate(c, *p)
						out = v
						return s.write(v)
					}
				}
				return ErrInvalid
			}
			return ErrInvalid
		}
		return ErrPlanNotFound
	})
	return s.project(out), err
}

func (s *Store) AddUsageObservation(repo, capabilityID, planID, candidateID, actor string, u UsageObservation) (Capability, error) {
	var out Capability
	err := s.lock(func() error {
		v, err := s.read(repo, capabilityID)
		if err != nil {
			return err
		}
		for pi := range v.RetirementPlans {
			p := &v.RetirementPlans[pi]
			if p.ID != planID {
				continue
			}
			for ci := range p.Candidates {
				c := &p.Candidates[ci]
				if c.ID != candidateID {
					continue
				}
				if u.ConsumerIndex < 0 || u.ConsumerIndex >= len(p.Audiences) || (u.State != "measured" && u.State != "unmeasured" && u.State != "inaccessible") || u.Summary == "" || u.WindowEndsAt.Before(u.WindowStartsAt) || u.OldBehaviorUses < 0 || u.TotalUses < u.OldBehaviorUses {
					return ErrInvalid
				}
				u.ID = randomID()
				u.CreatedAt = s.now()
				if u.OwnerID == actor && contains(p.Audiences[u.ConsumerIndex].OwnerIDs, actor) {
					u.Acknowledged = true
				}
				c.Usage = append(c.Usage, u)
				c.UpdatedAt = s.now()
				ProjectCandidate(c, *p)
				out = v
				return s.write(v)
			}
			return ErrInvalid
		}
		return ErrPlanNotFound
	})
	return s.project(out), err
}
