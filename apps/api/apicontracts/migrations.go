package apicontracts

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

var ErrMigrationBlocked = errors.New("api contract migration blocked")

// ContractMigration coordinates policy and acknowledgement around existing
// evolution, application, integration-work, and operational evidence. It never
// grants authority in any of those systems.
type ContractMigration struct {
	ID               string                     `json:"id"`
	RepositoryID     string                     `json:"repository_id"`
	ContractID       string                     `json:"contract_id"`
	FromVersion      int                        `json:"from_version"`
	ToVersion        int                        `json:"to_version,omitempty"`
	Kind             string                     `json:"kind"`
	EvolutionID      string                     `json:"evolution_id"`
	Changes          []MigrationChange          `json:"changes"`
	Stages           []MigrationStage           `json:"stages"`
	Applications     []MigrationApplication     `json:"applications"`
	Acknowledgements []MigrationAcknowledgement `json:"acknowledgements"`
	Attestations     []MigrationAttestation     `json:"attestations"`
	Exceptions       []MigrationException       `json:"exceptions"`
	State            string                     `json:"state"`
	CurrentStage     string                     `json:"current_stage"`
	CreatedBy        string                     `json:"created_by"`
	CreatedAt        time.Time                  `json:"created_at"`
	UpdatedAt        time.Time                  `json:"updated_at"`
	Version          int                        `json:"version"`
	Readiness        MigrationReadiness         `json:"readiness"`
}
type MigrationChange struct {
	Kind           string `json:"kind"`
	Summary        string `json:"summary"`
	Classification string `json:"classification"`
}
type MigrationStage struct {
	ID                     string    `json:"id"`
	Name                   string    `json:"name"`
	Deadline               time.Time `json:"deadline"`
	RequiredEvidence       string    `json:"required_evidence"`
	ObservationMaxAgeHours int       `json:"observation_max_age_hours"`
	MaxRemainingRequests   int64     `json:"max_remaining_requests"`
}
type MigrationApplication struct {
	ApplicationID        string `json:"application_id"`
	OwnerID              string `json:"owner_id"`
	ConsumerRepositoryID string `json:"consumer_repository_id,omitempty"`
	IntegrationWorkID    string `json:"integration_work_id,omitempty"`
}
type MigrationAcknowledgement struct {
	ApplicationID string    `json:"application_id"`
	ActorID       string    `json:"actor_id"`
	Note          string    `json:"note"`
	CreatedAt     time.Time `json:"created_at"`
}
type MigrationAttestation struct {
	ApplicationID     string    `json:"application_id"`
	ActorID           string    `json:"actor_id"`
	IntegrationWorkID string    `json:"integration_work_id"`
	CandidateID       string    `json:"candidate_id"`
	CreatedAt         time.Time `json:"created_at"`
}
type MigrationException struct {
	ApplicationID string    `json:"application_id"`
	ActorID       string    `json:"actor_id"`
	Reason        string    `json:"reason"`
	ExpiresAt     time.Time `json:"expires_at"`
	CreatedAt     time.Time `json:"created_at"`
}
type MigrationReadiness struct {
	Ready     bool                         `json:"ready"`
	Blockers  []MigrationBlocker           `json:"blockers"`
	Consumers []MigrationConsumerReadiness `json:"consumers"`
}
type MigrationBlocker struct {
	ApplicationID string `json:"application_id,omitempty"`
	Code          string `json:"code"`
	Detail        string `json:"detail"`
}
type MigrationConsumerReadiness struct {
	ApplicationID        string     `json:"application_id"`
	OwnerID              string     `json:"owner_id"`
	ConsumerRepositoryID string     `json:"consumer_repository_id,omitempty"`
	Acknowledged         bool       `json:"acknowledged"`
	Tested               bool       `json:"tested"`
	Attested             bool       `json:"attested"`
	ExceptionUntil       *time.Time `json:"exception_until,omitempty"`
	RemainingRequests    int64      `json:"remaining_requests"`
	LastObservedAt       *time.Time `json:"last_observed_at,omitempty"`
	AccessState          string     `json:"access_state"`
	Blockers             []string   `json:"blockers"`
}

func (s *Store) CreateContractMigration(v ContractMigration) (ContractMigration, error) {
	var out ContractMigration
	err := s.lock(func() error {
		if v.RepositoryID == "" || v.ContractID == "" || v.FromVersion < 1 || v.EvolutionID == "" || !map[string]bool{"new_version": true, "deprecation": true}[v.Kind] || (v.Kind == "new_version" && v.ToVersion <= v.FromVersion) || len(v.Changes) == 0 || len(v.Stages) == 0 {
			return ErrInvalid
		}
		seenStage, seenApp := map[string]bool{}, map[string]bool{}
		for _, x := range v.Changes {
			if strings.TrimSpace(x.Summary) == "" || !map[string]bool{"compatible": true, "conditionally_compatible": true, "breaking": true, "removed": true}[x.Classification] {
				return ErrInvalid
			}
		}
		for _, x := range v.Stages {
			if x.ID == "" || x.Name == "" || seenStage[x.ID] || x.Deadline.IsZero() || x.RequiredEvidence == "" || x.ObservationMaxAgeHours < 1 || x.ObservationMaxAgeHours > 2160 || x.MaxRemainingRequests < 0 {
				return ErrInvalid
			}
			seenStage[x.ID] = true
		}
		for _, x := range v.Applications {
			if x.ApplicationID == "" || x.OwnerID == "" || seenApp[x.ApplicationID] {
				return ErrInvalid
			}
			seenApp[x.ApplicationID] = true
		}
		now := s.now()
		v.ID, v.State, v.CurrentStage, v.Version = randomID(), "planned", v.Stages[0].ID, 1
		v.CreatedAt, v.UpdatedAt = now, now
		v.Acknowledgements, v.Attestations, v.Exceptions = []MigrationAcknowledgement{}, []MigrationAttestation{}, []MigrationException{}
		out = v
		return s.writeContractMigration(v)
	})
	return out, err
}

func (s *Store) GetContractMigration(id string) (ContractMigration, error) {
	var out ContractMigration
	err := s.lock(func() error { var e error; out, e = s.readContractMigration(id); return e })
	return out, err
}
func (s *Store) ListContractMigrations(repo, contract string) ([]ContractMigration, error) {
	out := []ContractMigration{}
	err := s.readOperationalDir("contract-migrations", func(b []byte) error {
		var v ContractMigration
		if e := json.Unmarshal(b, &v); e != nil {
			return e
		}
		if v.RepositoryID == repo && v.ContractID == contract {
			out = append(out, v)
		}
		return nil
	})
	return out, err
}
func (s *Store) MutateContractMigration(id string, expected int, fn func(*ContractMigration) error) (ContractMigration, error) {
	var out ContractMigration
	err := s.lock(func() error {
		v, e := s.readContractMigration(id)
		if e != nil {
			return e
		}
		if v.Version != expected {
			return ErrConflict
		}
		if e = fn(&v); e != nil {
			return e
		}
		v.Version++
		v.UpdatedAt = s.now()
		out = v
		return s.writeContractMigration(v)
	})
	return out, err
}
func (s *Store) readContractMigration(id string) (ContractMigration, error) {
	var v ContractMigration
	b, e := os.ReadFile(filepath.Join(s.root, "contract-migrations", id+".json"))
	if os.IsNotExist(e) {
		return v, ErrNotFound
	}
	if e != nil {
		return v, e
	}
	e = json.Unmarshal(b, &v)
	return v, e
}
func (s *Store) writeContractMigration(v ContractMigration) error {
	return s.writeOperational("contract-migrations", v.ID, v)
}

func ProjectContractMigration(v ContractMigration, apps map[string]Application, work map[string][]IntegrationWork, observations map[string][]OperationalObservation, now time.Time) ContractMigration {
	stage := v.Stages[0]
	for _, x := range v.Stages {
		if x.ID == v.CurrentStage {
			stage = x
		}
	}
	v.Readiness = MigrationReadiness{Ready: true, Blockers: []MigrationBlocker{}, Consumers: []MigrationConsumerReadiness{}}
	for _, linked := range v.Applications {
		app, ok := apps[linked.ApplicationID]
		c := MigrationConsumerReadiness{ApplicationID: linked.ApplicationID, OwnerID: linked.OwnerID, ConsumerRepositoryID: linked.ConsumerRepositoryID, AccessState: "unavailable", Blockers: []string{}}
		c.Acknowledged = slices.ContainsFunc(v.Acknowledgements, func(x MigrationAcknowledgement) bool { return x.ApplicationID == linked.ApplicationID })
		attest := slices.IndexFunc(v.Attestations, func(x MigrationAttestation) bool { return x.ApplicationID == linked.ApplicationID })
		c.Attested = attest >= 0
		if attest >= 0 {
			for _, w := range work[linked.ApplicationID] {
				if w.ID == v.Attestations[attest].IntegrationWorkID {
					for _, candidate := range w.Candidates {
						if candidate.ID == v.Attestations[attest].CandidateID && candidatePassing(candidate) {
							c.Tested = true
						}
					}
				}
			}
		}
		for _, x := range v.Exceptions {
			if x.ApplicationID == linked.ApplicationID && x.ExpiresAt.After(now) {
				until := x.ExpiresAt
				c.ExceptionUntil = &until
			}
		}
		if ok {
			c.AccessState = app.Status
			if app.Status != "approved" {
				c.Blockers = append(c.Blockers, "application access is revoked, expired, or unapproved")
			}
		}
		for _, o := range observations[linked.ApplicationID] {
			if c.LastObservedAt == nil || o.WindowEndedAt.After(*c.LastObservedAt) {
				at := o.WindowEndedAt
				c.LastObservedAt = &at
				c.RemainingRequests = o.Requests
			}
		}
		if c.LastObservedAt == nil || c.LastObservedAt.Before(now.Add(-time.Duration(stage.ObservationMaxAgeHours)*time.Hour)) {
			c.Blockers = append(c.Blockers, "current old-version usage evidence is missing or stale")
		}
		if !c.Acknowledged {
			c.Blockers = append(c.Blockers, "consumer has not acknowledged migration work")
		}
		if !c.Tested && c.ExceptionUntil == nil {
			c.Blockers = append(c.Blockers, "no passing dual-version candidate or active exception")
		}
		if c.RemainingRequests > stage.MaxRemainingRequests {
			c.Blockers = append(c.Blockers, "remaining old-version traffic exceeds the stage threshold")
		}
		for _, code := range c.Blockers {
			v.Readiness.Blockers = append(v.Readiness.Blockers, MigrationBlocker{ApplicationID: c.ApplicationID, Code: "consumer_not_ready", Detail: code})
		}
		v.Readiness.Consumers = append(v.Readiness.Consumers, c)
	}
	v.Readiness.Ready = len(v.Readiness.Blockers) == 0
	return v
}

func candidatePassing(candidate IntegrationCandidate) bool {
	if len(candidate.Scenarios) == 0 {
		return false
	}
	for _, scenario := range candidate.Scenarios {
		status := ""
		for _, evidence := range candidate.Evidence {
			if evidence.Scenario == scenario.Name && evidence.Side == scenario.OwnerSide {
				status = evidence.Status
			}
		}
		if status != "passed" {
			return false
		}
	}
	return true
}
