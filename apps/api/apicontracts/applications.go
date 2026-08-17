package apicontracts

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type ApplicationEvent struct {
	Type    string    `json:"type"`
	ActorID string    `json:"actor_id"`
	Detail  string    `json:"detail"`
	At      time.Time `json:"at"`
}
type ApplicationCredential struct {
	ID         string     `json:"id"`
	Prefix     string     `json:"prefix"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	Hash       string     `json:"hash,omitempty"`
}
type Application struct {
	ID                     string                  `json:"id"`
	RepositoryID           string                  `json:"repository_id"`
	ContractID             string                  `json:"contract_id"`
	ContractVersion        int                     `json:"contract_version"`
	OwnerID                string                  `json:"owner_id"`
	Name                   string                  `json:"name"`
	ProjectURL             string                  `json:"project_url"`
	Environments           []string                `json:"environments"`
	RequestedCapabilities  []string                `json:"requested_capabilities"`
	ApprovedCapabilities   []string                `json:"approved_capabilities"`
	Status                 string                  `json:"status"`
	DecisionReason         string                  `json:"decision_reason,omitempty"`
	DecidedBy              string                  `json:"decided_by,omitempty"`
	DecidedAt              *time.Time              `json:"decided_at,omitempty"`
	ApprovalExpiresAt      *time.Time              `json:"approval_expires_at,omitempty"`
	Credentials            []ApplicationCredential `json:"credentials"`
	Events                 []ApplicationEvent      `json:"events"`
	CreatedAt              time.Time               `json:"created_at"`
	UpdatedAt              time.Time               `json:"updated_at"`
	SandboxWindowStartedAt *time.Time              `json:"sandbox_window_started_at,omitempty"`
	SandboxRequestCount    int                     `json:"sandbox_request_count,omitempty"`
}
type IssuedApplicationCredential struct {
	ApplicationCredential
	Secret string `json:"secret"`
}

// IntegrationWork is the review bridge between an approved consumer application
// and ordinary project work. Preload is declarative and deliberately contains no
// application credential or inaccessible contract payload.
type IntegrationWork struct {
	ID                   string                 `json:"id"`
	ApplicationID        string                 `json:"application_id"`
	ProducerRepositoryID string                 `json:"producer_repository_id"`
	ConsumerRepositoryID string                 `json:"consumer_repository_id"`
	ConsumerRevision     string                 `json:"consumer_revision"`
	ContractID           string                 `json:"contract_id"`
	ContractVersion      int                    `json:"contract_version"`
	Kind                 string                 `json:"kind"`
	OwnerType            string                 `json:"owner_type"`
	OwnerID              string                 `json:"owner_id"`
	Title                string                 `json:"title"`
	Preload              IntegrationPreload     `json:"preload"`
	Candidates           []IntegrationCandidate `json:"candidates"`
	CreatedBy            string                 `json:"created_by"`
	CreatedAt            time.Time              `json:"created_at"`
}
type IntegrationPreload struct {
	DefinitionPath      string   `json:"definition_path"`
	DefinitionCommit    string   `json:"definition_commit"`
	SDKs                []Link   `json:"sdks"`
	Examples            []Link   `json:"examples"`
	Environments        []string `json:"sandbox_environments"`
	Operations          []string `json:"sandbox_operations"`
	SyntheticOnly       bool     `json:"synthetic_only"`
	CredentialsIncluded bool     `json:"credentials_included"`
}
type IntegrationScenario struct {
	Name      string `json:"name"`
	OwnerSide string `json:"owner_side"`
	Command   string `json:"command"`
}
type IntegrationEvidence struct {
	ID              string             `json:"id"`
	Scenario        string             `json:"scenario"`
	Side            string             `json:"side"`
	Status          string             `json:"status"`
	Requests        []string           `json:"requests"`
	Responses       []string           `json:"responses"`
	Logs            []string           `json:"logs"`
	Artifacts       []EvidenceArtifact `json:"artifacts"`
	CoveragePercent float64            `json:"coverage_percent"`
	CostUnits       int                `json:"cost_units"`
	AuthoredBy      string             `json:"authored_by"`
	CreatedAt       time.Time          `json:"created_at"`
}
type EvidenceArtifact struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}
type IntegrationCandidate struct {
	ID                    string                `json:"id"`
	ProducerPullRequestID string                `json:"producer_pull_request_id"`
	ProducerRevision      string                `json:"producer_revision"`
	ConsumerPullRequestID string                `json:"consumer_pull_request_id"`
	ConsumerRevision      string                `json:"consumer_revision"`
	Scenarios             []IntegrationScenario `json:"scenarios"`
	Evidence              []IntegrationEvidence `json:"evidence"`
	CreatedBy             string                `json:"created_by"`
	CreatedAt             time.Time             `json:"created_at"`
}

func (s *Store) CreateIntegrationWork(app Application, actor, consumerRepo, consumerRevision, kind, ownerType, ownerID, title string, preload IntegrationPreload) (IntegrationWork, error) {
	var out IntegrationWork
	err := s.lock(func() error {
		if app.Status != "approved" || app.ApprovalExpiresAt == nil || !app.ApprovalExpiresAt.After(s.now()) || consumerRepo == "" || consumerRevision == "" || strings.TrimSpace(title) == "" || !map[string]bool{"task": true, "session": true, "workspace": true}[kind] || !map[string]bool{"human": true, "agent": true}[ownerType] || strings.TrimSpace(ownerID) == "" {
			return ErrInvalid
		}
		now := s.now()
		out = IntegrationWork{ID: randomID(), ApplicationID: app.ID, ProducerRepositoryID: app.RepositoryID, ConsumerRepositoryID: consumerRepo, ConsumerRevision: consumerRevision, ContractID: app.ContractID, ContractVersion: app.ContractVersion, Kind: kind, OwnerType: ownerType, OwnerID: ownerID, Title: strings.TrimSpace(title), Preload: preload, Candidates: []IntegrationCandidate{}, CreatedBy: actor, CreatedAt: now}
		return s.writeIntegrationWork(out)
	})
	return out, err
}
func (s *Store) ListIntegrationWork(applicationID string) ([]IntegrationWork, error) {
	out := []IntegrationWork{}
	err := s.lock(func() error {
		entries, e := os.ReadDir(filepath.Join(s.root, "integration-work"))
		if os.IsNotExist(e) {
			return nil
		}
		if e != nil {
			return e
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			var v IntegrationWork
			b, e := os.ReadFile(filepath.Join(s.root, "integration-work", entry.Name()))
			if e != nil {
				return e
			}
			if e = json.Unmarshal(b, &v); e != nil {
				return e
			}
			if v.ApplicationID == applicationID {
				out = append(out, v)
			}
		}
		return nil
	})
	return out, err
}
func (s *Store) GetIntegrationWork(id string) (IntegrationWork, error) {
	var out IntegrationWork
	err := s.lock(func() error { var e error; out, e = s.readIntegrationWork(id); return e })
	return out, err
}
func (s *Store) AddIntegrationCandidate(workID, actor string, candidate IntegrationCandidate) (IntegrationWork, error) {
	var out IntegrationWork
	err := s.lock(func() error {
		v, e := s.readIntegrationWork(workID)
		if e != nil {
			return e
		}
		if len(candidate.Scenarios) == 0 || len(candidate.Scenarios) > 32 {
			return ErrInvalid
		}
		for _, scenario := range candidate.Scenarios {
			text := strings.ToLower(scenario.Name + " " + scenario.Command)
			if strings.TrimSpace(scenario.Name) == "" || len(scenario.Name) > 120 || strings.TrimSpace(scenario.Command) == "" || len(scenario.Command) > 1000 || !map[string]bool{"producer": true, "consumer": true}[scenario.OwnerSide] {
				return ErrInvalid
			}
			for _, marker := range []string{"authorization:", "bearer ", "password=", "token=", "secret=", "private key", "vva_"} {
				if strings.Contains(text, marker) {
					return ErrInvalid
				}
			}
		}
		candidate.ID = randomID()
		candidate.CreatedBy = actor
		candidate.CreatedAt = s.now()
		candidate.Evidence = []IntegrationEvidence{}
		v.Candidates = append(v.Candidates, candidate)
		out = v
		return s.writeIntegrationWork(v)
	})
	return out, err
}
func (s *Store) AddIntegrationEvidence(workID, candidateID, actor string, evidence IntegrationEvidence) (IntegrationWork, error) {
	var out IntegrationWork
	err := s.lock(func() error {
		v, e := s.readIntegrationWork(workID)
		if e != nil {
			return e
		}
		for i := range v.Candidates {
			if v.Candidates[i].ID != candidateID {
				continue
			}
			if !slices.ContainsFunc(v.Candidates[i].Scenarios, func(x IntegrationScenario) bool { return x.Name == evidence.Scenario && x.OwnerSide == evidence.Side }) || !map[string]bool{"passed": true, "failed": true}[evidence.Status] || evidence.CoveragePercent < 0 || evidence.CoveragePercent > 100 || evidence.CostUnits < 0 {
				return ErrInvalid
			}
			evidence.ID = randomID()
			evidence.AuthoredBy = actor
			evidence.CreatedAt = s.now()
			v.Candidates[i].Evidence = append(v.Candidates[i].Evidence, evidence)
			out = v
			return s.writeIntegrationWork(v)
		}
		return ErrNotFound
	})
	return out, err
}
func (s *Store) readIntegrationWork(id string) (IntegrationWork, error) {
	var v IntegrationWork
	b, e := os.ReadFile(filepath.Join(s.root, "integration-work", id+".json"))
	if os.IsNotExist(e) {
		return v, ErrNotFound
	}
	if e != nil {
		return v, e
	}
	e = json.Unmarshal(b, &v)
	return v, e
}
func (s *Store) writeIntegrationWork(v IntegrationWork) error {
	dir := filepath.Join(s.root, "integration-work")
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(dir, ".integration-work-")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e == nil {
		e = closeErr
	}
	if e == nil {
		e = os.Rename(name, filepath.Join(dir, v.ID+".json"))
	}
	return e
}

func (s *Store) CreateApplication(repo, contract, owner, name, project string, version int, environments, capabilities []string) (Application, error) {
	var out Application
	err := s.lock(func() error {
		if repo == "" || contract == "" || owner == "" || strings.TrimSpace(name) == "" || version < 1 || len(environments) == 0 || len(capabilities) == 0 || !uniqueNonempty(environments) || !uniqueNonempty(capabilities) {
			return ErrInvalid
		}
		now := s.now()
		out = Application{ID: randomID(), RepositoryID: repo, ContractID: contract, ContractVersion: version, OwnerID: owner, Name: strings.TrimSpace(name), ProjectURL: strings.TrimSpace(project), Environments: slices.Clone(environments), RequestedCapabilities: slices.Clone(capabilities), ApprovedCapabilities: []string{}, Status: "pending", Credentials: []ApplicationCredential{}, Events: []ApplicationEvent{{Type: "requested", ActorID: owner, Detail: "Application access requested", At: now}}, CreatedAt: now, UpdatedAt: now}
		return s.writeApplication(out)
	})
	return projectApplication(out), err
}
func (s *Store) GetApplication(id string) (Application, error) {
	var out Application
	err := s.lock(func() error { var e error; out, e = s.readApplication(id); return e })
	return projectApplication(out), err
}
func (s *Store) ListApplications(repo, contract string) ([]Application, error) {
	out := []Application{}
	err := s.lock(func() error {
		entries, e := os.ReadDir(filepath.Join(s.root, "applications"))
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
			v, e := s.readApplication(strings.TrimSuffix(x.Name(), ".json"))
			if e != nil {
				return e
			}
			if v.RepositoryID == repo && v.ContractID == contract {
				out = append(out, projectApplication(v))
			}
		}
		return nil
	})
	return out, err
}
func (s *Store) DecideApplication(id, actor, status, reason string, capabilities []string, expires time.Time) (Application, error) {
	var out Application
	err := s.lock(func() error {
		v, e := s.readApplication(id)
		if e != nil {
			return e
		}
		if v.Status != "pending" || !map[string]bool{"approved": true, "denied": true}[status] || strings.TrimSpace(reason) == "" {
			return ErrConflict
		}
		if status == "approved" {
			if expires.Before(s.now()) || !subset(capabilities, v.RequestedCapabilities) || len(capabilities) == 0 {
				return ErrInvalid
			}
			v.ApprovedCapabilities = slices.Clone(capabilities)
			v.ApprovalExpiresAt = &expires
		}
		now := s.now()
		v.Status = status
		v.DecisionReason = reason
		v.DecidedBy = actor
		v.DecidedAt = &now
		v.UpdatedAt = now
		v.Events = append(v.Events, ApplicationEvent{Type: status, ActorID: actor, Detail: reason, At: now})
		out = v
		return s.writeApplication(v)
	})
	return projectApplication(out), err
}
func (s *Store) IssueApplicationCredential(id, owner string, lifetime time.Duration) (Application, IssuedApplicationCredential, error) {
	var out Application
	var issued IssuedApplicationCredential
	err := s.lock(func() error {
		v, e := s.readApplication(id)
		if e != nil {
			return e
		}
		now := s.now()
		if v.OwnerID != owner || v.Status != "approved" || v.ApprovalExpiresAt == nil || !v.ApprovalExpiresAt.After(now) {
			return ErrConflict
		}
		if lifetime <= 0 || lifetime > 30*24*time.Hour {
			return ErrInvalid
		}
		expires := now.Add(lifetime)
		if expires.After(*v.ApprovalExpiresAt) {
			expires = *v.ApprovalExpiresAt
		}
		raw := make([]byte, 32)
		if _, e = rand.Read(raw); e != nil {
			return e
		}
		secret := "vva_" + hex.EncodeToString(raw)
		sum := sha256.Sum256([]byte(secret))
		c := ApplicationCredential{ID: randomID(), Prefix: secret[:12], CreatedAt: now, ExpiresAt: expires, Hash: hex.EncodeToString(sum[:])}
		for i := range v.Credentials {
			if v.Credentials[i].RevokedAt == nil {
				v.Credentials[i].RevokedAt = &now
			}
		}
		v.Credentials = append(v.Credentials, c)
		v.Events = append(v.Events, ApplicationEvent{Type: "credential_rotated", ActorID: owner, Detail: "A replacement sandbox credential was issued", At: now})
		v.UpdatedAt = now
		out = v
		publicCredential := c
		publicCredential.Hash = ""
		issued = IssuedApplicationCredential{ApplicationCredential: publicCredential, Secret: secret}
		return s.writeApplication(v)
	})
	return projectApplication(out), issued, err
}
func (s *Store) RevokeApplication(id, actor, event string) (Application, error) {
	var out Application
	err := s.lock(func() error {
		v, e := s.readApplication(id)
		if e != nil {
			return e
		}
		now := s.now()
		v.Status = "revoked"
		for i := range v.Credentials {
			if v.Credentials[i].RevokedAt == nil {
				v.Credentials[i].RevokedAt = &now
			}
		}
		v.Events = append(v.Events, ApplicationEvent{Type: event, ActorID: actor, Detail: "All application credentials were revoked", At: now})
		v.UpdatedAt = now
		out = v
		return s.writeApplication(v)
	})
	return projectApplication(out), err
}
func (s *Store) TransferApplication(id, owner, successor string) (Application, error) {
	var out Application
	err := s.lock(func() error {
		v, e := s.readApplication(id)
		if e != nil {
			return e
		}
		if v.OwnerID != owner || strings.TrimSpace(successor) == "" || successor == owner {
			return ErrInvalid
		}
		now := s.now()
		for i := range v.Credentials {
			if v.Credentials[i].RevokedAt == nil {
				v.Credentials[i].RevokedAt = &now
			}
		}
		v.OwnerID = successor
		v.Status = "pending"
		v.ApprovedCapabilities = []string{}
		v.ApprovalExpiresAt = nil
		v.DecisionReason = "Ownership changed; producer reapproval is required"
		v.Events = append(v.Events, ApplicationEvent{Type: "ownership_changed", ActorID: owner, Detail: "Ownership transferred to " + successor + "; credentials revoked and approval reset", At: now})
		v.UpdatedAt = now
		out = v
		return s.writeApplication(v)
	})
	return projectApplication(out), err
}
func (s *Store) AuthenticateApplication(id, secret string) (Application, error) {
	return s.AuthenticateApplicationRequest(id, secret, 0, 0)
}

var ErrQuotaExceeded = errors.New("application sandbox quota exceeded")

// AuthenticateApplicationRequest authenticates and atomically consumes one
// request from the application's contract-defined fixed window. Credential
// rotation therefore cannot reset or bypass the application quota. A zero limit
// retains the authentication-only behavior used by storage callers.
func (s *Store) AuthenticateApplicationRequest(id, secret string, limit, windowSeconds int) (Application, error) {
	var out Application
	err := s.lock(func() error {
		v, e := s.readApplication(id)
		if e != nil {
			return e
		}
		now := s.now()
		if v.Status != "approved" || v.ApprovalExpiresAt == nil || !v.ApprovalExpiresAt.After(now) {
			return ErrNotFound
		}
		sum := sha256.Sum256([]byte(secret))
		hash := hex.EncodeToString(sum[:])
		for i := range v.Credentials {
			c := &v.Credentials[i]
			if c.Hash == hash && c.RevokedAt == nil && c.ExpiresAt.After(now) {
				if limit > 0 {
					if windowSeconds <= 0 {
						return ErrInvalid
					}
					if v.SandboxWindowStartedAt == nil || !now.Before(v.SandboxWindowStartedAt.Add(time.Duration(windowSeconds)*time.Second)) {
						v.SandboxWindowStartedAt = &now
						v.SandboxRequestCount = 0
					}
					if v.SandboxRequestCount >= limit {
						return ErrQuotaExceeded
					}
					v.SandboxRequestCount++
				}
				c.LastUsedAt = &now
				v.UpdatedAt = now
				out = v
				return s.writeApplication(v)
			}
		}
		return ErrNotFound
	})
	return projectApplication(out), err
}
func uniqueNonempty(v []string) bool {
	seen := map[string]bool{}
	for _, x := range v {
		if strings.TrimSpace(x) == "" || seen[x] {
			return false
		}
		seen[x] = true
	}
	return true
}
func subset(v, allowed []string) bool {
	for _, x := range v {
		if !slices.Contains(allowed, x) {
			return false
		}
	}
	return uniqueNonempty(v)
}
func projectApplication(v Application) Application {
	for i := range v.Credentials {
		v.Credentials[i].Hash = ""
	}
	return v
}
func (s *Store) readApplication(id string) (Application, error) {
	var v Application
	b, e := os.ReadFile(filepath.Join(s.root, "applications", id+".json"))
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
func (s *Store) writeApplication(v Application) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	dir := filepath.Join(s.root, "applications")
	if e = os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(dir, ".application-")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e == nil {
		e = os.Rename(name, filepath.Join(dir, v.ID+".json"))
	}
	return e
}
