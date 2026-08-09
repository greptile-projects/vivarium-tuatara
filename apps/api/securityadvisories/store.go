// Package securityadvisories persists private vulnerability coordination records.
package securityadvisories

import (
	"crypto/rand"
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
)

var (
	ErrNotFound = errors.New("security advisory not found")
	ErrInvalid  = errors.New("invalid security advisory")
	ErrConflict = errors.New("security advisory changed")
)

type AffectedRepository struct {
	RepositoryID string   `json:"repository_id"`
	Versions     []string `json:"versions"`
}

type Evidence struct {
	ID           string    `json:"id,omitempty"`
	Kind         string    `json:"kind,omitempty"`
	RepositoryID string    `json:"repository_id,omitempty"`
	CommitID     string    `json:"commit_id,omitempty"`
	ReleaseID    string    `json:"release_id,omitempty"`
	BuildID      string    `json:"build_id,omitempty"`
	ArtifactID   string    `json:"artifact_id,omitempty"`
	DeploymentID string    `json:"deployment_id,omitempty"`
	Dependency   string    `json:"dependency,omitempty"`
	Label        string    `json:"label"`
	Description  string    `json:"description"`
	CapturedAt   time.Time `json:"captured_at,omitempty"`
}

type Finding struct {
	ID              string    `json:"id"`
	Kind            string    `json:"kind"`
	ActorID         string    `json:"actor_id"`
	Statement       string    `json:"statement"`
	EvidenceIDs     []string  `json:"evidence_ids"`
	InvestigationID string    `json:"investigation_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}
type Impact struct {
	RepositoryID string    `json:"repository_id"`
	VersionLine  string    `json:"version_line"`
	Environment  string    `json:"environment"`
	State        string    `json:"state"`
	EvidenceIDs  []string  `json:"evidence_ids"`
	Rationale    string    `json:"rationale"`
	ActorID      string    `json:"actor_id"`
	UpdatedAt    time.Time `json:"updated_at"`
}
type Investigation struct {
	ID           string     `json:"id"`
	AgentID      string     `json:"agent_id"`
	InitiatorID  string     `json:"initiator_id"`
	CredentialID string     `json:"credential_id,omitempty"`
	Mandate      string     `json:"mandate"`
	State        string     `json:"state"`
	Evidence     []Evidence `json:"evidence"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type Message struct {
	ID        string    `json:"id"`
	ActorID   string    `json:"actor_id"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

type AccessEvent struct {
	ID        string    `json:"id"`
	ActorID   string    `json:"actor_id"`
	Action    string    `json:"action"`
	Detail    string    `json:"detail,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type Advisory struct {
	ID                   string               `json:"id"`
	Title                string               `json:"title"`
	Description          string               `json:"description"`
	AffectedRepositories []AffectedRepository `json:"affected_repositories"`
	Evidence             []Evidence           `json:"evidence"`
	Contact              string               `json:"contact"`
	ReporterID           string               `json:"reporter_id"`
	ResponseTeam         []string             `json:"response_team"`
	Severity             string               `json:"severity"`
	EmbargoState         string               `json:"embargo_state"`
	Messages             []Message            `json:"messages"`
	AccessLog            []AccessEvent        `json:"access_log"`
	Findings             []Finding            `json:"findings"`
	ImpactMatrix         []Impact             `json:"impact_matrix"`
	Investigations       []Investigation      `json:"investigations"`
	Version              int                  `json:"version"`
	CreatedAt            time.Time            `json:"created_at"`
	UpdatedAt            time.Time            `json:"updated_at"`
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
	if err := os.Chmod(root, 0700); err != nil {
		return nil, err
	}
	return &Store{root: root, now: func() time.Time { return time.Now().UTC() }}, nil
}

func (s *Store) Create(v Advisory) (Advisory, error) {
	if err := validateCreate(v); err != nil {
		return Advisory{}, err
	}
	v.Title, v.Description, v.Contact = strings.TrimSpace(v.Title), strings.TrimSpace(v.Description), strings.TrimSpace(v.Contact)
	for i := range v.AffectedRepositories {
		for j := range v.AffectedRepositories[i].Versions {
			v.AffectedRepositories[i].Versions[j] = strings.TrimSpace(v.AffectedRepositories[i].Versions[j])
		}
	}
	for i := range v.Evidence {
		v.Evidence[i].Label, v.Evidence[i].Description = strings.TrimSpace(v.Evidence[i].Label), strings.TrimSpace(v.Evidence[i].Description)
	}
	now := s.now()
	v.ID, v.Severity, v.EmbargoState, v.Version = mustID(), "untriaged", "reported", 1
	v.CreatedAt, v.UpdatedAt = now, now
	v.ResponseTeam, v.Messages = []string{}, []Message{}
	v.Findings, v.ImpactMatrix, v.Investigations = []Finding{}, []Impact{}, []Investigation{}
	for i := range v.Evidence {
		v.Evidence[i].ID, v.Evidence[i].CapturedAt = mustID(), now
	}
	v.AccessLog = []AccessEvent{{ID: mustID(), ActorID: v.ReporterID, Action: "reported", CreatedAt: now}}
	err := s.mutate(func() error { return s.write(v) })
	return v, err
}

func (s *Store) AddEvidence(id, actor string, evidence Evidence) (Advisory, error) {
	return s.update(id, func(v *Advisory) error {
		evidence.Label, evidence.Description, evidence.Dependency = strings.TrimSpace(evidence.Label), strings.TrimSpace(evidence.Description), strings.TrimSpace(evidence.Dependency)
		if !validID(actor) || !oneOf(evidence.Kind, "commit", "dependency", "build", "artifact", "release", "deployment") || evidence.Label == "" || len(evidence.Label) > 200 || len(evidence.Description) > 10000 {
			return ErrInvalid
		}
		evidence.ID, evidence.CapturedAt = mustID(), s.now()
		v.Evidence = append(v.Evidence, evidence)
		v.Version++
		v.AccessLog = append(v.AccessLog, AccessEvent{ID: mustID(), ActorID: actor, Action: "evidence_connected", Detail: evidence.Kind + " / " + evidence.Label, CreatedAt: evidence.CapturedAt})
		return nil
	})
}

func evidenceSelected(v *Advisory, ids []string) bool {
	for _, id := range ids {
		found := false
		for _, e := range v.Evidence {
			if e.ID == id {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
func (s *Store) AddFinding(id, actor, kind, statement, investigationID string, evidenceIDs []string) (Advisory, error) {
	return s.update(id, func(v *Advisory) error {
		statement = strings.TrimSpace(statement)
		if !validID(actor) || !oneOf(kind, "hypothesis", "conclusion", "uncertainty") || statement == "" || len(statement) > 10000 || len(evidenceIDs) > 50 || !evidenceSelected(v, evidenceIDs) {
			return ErrInvalid
		}
		now := s.now()
		v.Findings = append(v.Findings, Finding{ID: mustID(), Kind: kind, ActorID: actor, Statement: statement, EvidenceIDs: evidenceIDs, InvestigationID: investigationID, CreatedAt: now})
		v.Version++
		v.AccessLog = append(v.AccessLog, AccessEvent{ID: mustID(), ActorID: actor, Action: kind + "_recorded", CreatedAt: now})
		return nil
	})
}
func (s *Store) SetImpact(id, actor string, expected int, impact Impact) (Advisory, error) {
	return s.update(id, func(v *Advisory) error {
		impact.VersionLine, impact.Environment, impact.Rationale = strings.TrimSpace(impact.VersionLine), strings.TrimSpace(impact.Environment), strings.TrimSpace(impact.Rationale)
		if v.Version != expected {
			return ErrConflict
		}
		if !validID(actor) || !validID(impact.RepositoryID) || impact.VersionLine == "" || impact.Environment == "" || len(impact.VersionLine) > 200 || len(impact.Environment) > 200 || len(impact.Rationale) > 10000 || !oneOf(impact.State, "confirmed", "suspected", "unaffected", "fixed") || !evidenceSelected(v, impact.EvidenceIDs) {
			return ErrInvalid
		}
		now := s.now()
		impact.ActorID, impact.UpdatedAt = actor, now
		replaced := false
		for i := range v.ImpactMatrix {
			if v.ImpactMatrix[i].RepositoryID == impact.RepositoryID && v.ImpactMatrix[i].VersionLine == impact.VersionLine && v.ImpactMatrix[i].Environment == impact.Environment {
				v.ImpactMatrix[i] = impact
				replaced = true
			}
		}
		if !replaced {
			v.ImpactMatrix = append(v.ImpactMatrix, impact)
		}
		v.Version++
		v.AccessLog = append(v.AccessLog, AccessEvent{ID: mustID(), ActorID: actor, Action: "impact_updated", Detail: impact.VersionLine + " / " + impact.Environment + " / " + impact.State, CreatedAt: now})
		return nil
	})
}
func (s *Store) StartInvestigation(id, actor, agent, credential, mandate string, evidenceIDs []string) (Advisory, Investigation, error) {
	var out Investigation
	v, e := s.update(id, func(v *Advisory) error {
		mandate = strings.TrimSpace(mandate)
		if !validID(actor) || !validID(agent) || !validID(credential) || mandate == "" || len(mandate) > 10000 || len(evidenceIDs) == 0 || !evidenceSelected(v, evidenceIDs) {
			return ErrInvalid
		}
		selected := []Evidence{}
		for _, eid := range evidenceIDs {
			for _, x := range v.Evidence {
				if x.ID == eid {
					selected = append(selected, x)
				}
			}
		}
		now := s.now()
		out = Investigation{ID: mustID(), AgentID: agent, InitiatorID: actor, CredentialID: credential, Mandate: mandate, State: "running", Evidence: selected, CreatedAt: now, UpdatedAt: now}
		v.Investigations = append(v.Investigations, out)
		v.Version++
		v.AccessLog = append(v.AccessLog, AccessEvent{ID: mustID(), ActorID: actor, Action: "investigation_delegated", Detail: out.ID, CreatedAt: now})
		return nil
	})
	return v, out, e
}
func (s *Store) Investigation(id, investigationID, credentialID string) (Advisory, Investigation, error) {
	v, e := s.Get(id)
	if e != nil {
		return v, Investigation{}, e
	}
	for _, x := range v.Investigations {
		if x.ID == investigationID && x.CredentialID == credentialID && x.State == "running" {
			return v, x, nil
		}
	}
	return v, Investigation{}, ErrNotFound
}

func (s *Store) Get(id string) (Advisory, error) {
	var v Advisory
	if !validID(id) {
		return v, ErrNotFound
	}
	if err := s.read(id, &v); err != nil {
		return Advisory{}, ErrNotFound
	}
	return v, nil
}

func (s *Store) List() ([]Advisory, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	out := []Advisory{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		var v Advisory
		if err := s.read(strings.TrimSuffix(entry.Name(), ".json"), &v); err != nil {
			continue
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *Store) RecordAccess(id, actor string) (Advisory, error) {
	return s.update(id, func(v *Advisory) error {
		if !validID(actor) {
			return ErrInvalid
		}
		v.AccessLog = append(v.AccessLog, AccessEvent{ID: mustID(), ActorID: actor, Action: "viewed", CreatedAt: s.now()})
		return nil
	})
}

func (s *Store) Triage(id, actor string, expected int, severity, embargo string) (Advisory, error) {
	return s.update(id, func(v *Advisory) error {
		if v.Version != expected {
			return ErrConflict
		}
		if !validID(actor) || !oneOf(severity, "low", "moderate", "high", "critical") || !oneOf(embargo, "reported", "triaging", "embargoed", "coordinating") {
			return ErrInvalid
		}
		v.Severity, v.EmbargoState, v.Version = severity, embargo, v.Version+1
		v.AccessLog = append(v.AccessLog, AccessEvent{ID: mustID(), ActorID: actor, Action: "triage_updated", Detail: severity + " / " + embargo, CreatedAt: s.now()})
		return nil
	})
}

func (s *Store) Invite(id, actor, userID string) (Advisory, error) {
	return s.update(id, func(v *Advisory) error {
		if !validID(actor) || !validID(userID) {
			return ErrInvalid
		}
		for _, id := range v.ResponseTeam {
			if id == userID {
				return nil
			}
		}
		if userID == v.ReporterID {
			return nil
		}
		if len(v.ResponseTeam) >= 20 {
			return ErrInvalid
		}
		v.ResponseTeam = append(v.ResponseTeam, userID)
		v.Version++
		v.AccessLog = append(v.AccessLog, AccessEvent{ID: mustID(), ActorID: actor, Action: "responder_invited", Detail: userID, CreatedAt: s.now()})
		return nil
	})
}

func (s *Store) AddMessage(id, actor, body string) (Advisory, error) {
	return s.update(id, func(v *Advisory) error {
		body = strings.TrimSpace(body)
		if !validID(actor) || body == "" || len(body) > 20000 {
			return ErrInvalid
		}
		now := s.now()
		v.Messages = append(v.Messages, Message{ID: mustID(), ActorID: actor, Body: body, CreatedAt: now})
		v.Version++
		v.AccessLog = append(v.AccessLog, AccessEvent{ID: mustID(), ActorID: actor, Action: "message_added", CreatedAt: now})
		return nil
	})
}

func (s *Store) update(id string, fn func(*Advisory) error) (Advisory, error) {
	var v Advisory
	err := s.mutate(func() error {
		if err := s.read(id, &v); err != nil {
			return ErrNotFound
		}
		if err := fn(&v); err != nil {
			return err
		}
		v.UpdatedAt = s.now()
		return s.write(v)
	})
	return v, err
}

func validateCreate(v Advisory) error {
	v.Title, v.Description, v.Contact = strings.TrimSpace(v.Title), strings.TrimSpace(v.Description), strings.TrimSpace(v.Contact)
	if !validID(v.ReporterID) || v.Title == "" || len(v.Title) > 200 || v.Description == "" || len(v.Description) > 20000 || v.Contact == "" || len(v.Contact) > 500 || len(v.AffectedRepositories) == 0 || len(v.AffectedRepositories) > 20 || len(v.Evidence) > 20 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, scope := range v.AffectedRepositories {
		if !validID(scope.RepositoryID) || seen[scope.RepositoryID] || len(scope.Versions) == 0 || len(scope.Versions) > 50 {
			return ErrInvalid
		}
		seen[scope.RepositoryID] = true
		for _, version := range scope.Versions {
			if strings.TrimSpace(version) == "" || len(version) > 200 {
				return ErrInvalid
			}
		}
	}
	for _, evidence := range v.Evidence {
		if strings.TrimSpace(evidence.Label) == "" || len(evidence.Label) > 200 || strings.TrimSpace(evidence.Description) == "" || len(evidence.Description) > 10000 {
			return ErrInvalid
		}
	}
	return nil
}

func oneOf(v string, choices ...string) bool {
	for _, choice := range choices {
		if v == choice {
			return true
		}
	}
	return false
}
func validID(v string) bool {
	if len(v) != 32 {
		return false
	}
	_, err := hex.DecodeString(v)
	return err == nil
}
func mustID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func (s *Store) path(id string) string { return filepath.Join(s.root, id+".json") }
func (s *Store) read(id string, out *Advisory) error {
	data, err := os.ReadFile(s.path(id))
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}
func (s *Store) write(v Advisory) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.root, ".advisory-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(data)
	}
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(name, s.path(v.ID))
	}
	return err
}
func (s *Store) mutate(fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	return fn()
}
