// Package agentreleases retains attested releases and governed deployments of reviewed agents.
package agentreleases

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
	"sync"
	"time"
)

var ErrInvalid = errors.New("invalid agent release")
var ErrNotFound = errors.New("agent release not found")
var ErrConflict = errors.New("agent release changed")
var ErrDenied = errors.New("agent release action denied")

type Approval struct {
	Kind       string    `json:"kind"`
	OwnerID    string    `json:"owner_id"`
	EvidenceID string    `json:"evidence_id"`
	Decision   string    `json:"decision"`
	ApprovedAt time.Time `json:"approved_at"`
}
type Release struct {
	ID                string     `json:"id"`
	OrganizationID    string     `json:"organization_id"`
	RepositoryID      string     `json:"repository_id"`
	AgentID           string     `json:"agent_id"`
	CandidateID       string     `json:"candidate_id"`
	CandidateRevision string     `json:"candidate_revision"`
	ProjectID         string     `json:"project_id"`
	ProjectVersion    int        `json:"project_version"`
	ContractDigest    string     `json:"contract_digest"`
	ModelVersions     []string   `json:"model_versions"`
	ToolVersions      []string   `json:"tool_versions"`
	Roles             []string   `json:"roles"`
	Approvals         []Approval `json:"approvals"`
	PilotID           string     `json:"pilot_id"`
	Attestation       string     `json:"attestation"`
	Status            string     `json:"status"`
	Version           int        `json:"version"`
	CreatedBy         string     `json:"created_by"`
	CreatedAt         time.Time  `json:"created_at"`
	Events            []Event    `json:"events"`
}
type Budget struct {
	MaxCost    float64 `json:"max_cost"`
	MaxActions int     `json:"max_actions"`
	MaxMinutes int     `json:"max_minutes"`
}
type Deployment struct {
	ID                string    `json:"id"`
	ReleaseID         string    `json:"release_id"`
	Identity          string    `json:"identity"`
	Roles             []string  `json:"roles"`
	CredentialScopes  []string  `json:"credential_scopes"`
	Budget            Budget    `json:"budget"`
	RollbackReleaseID string    `json:"rollback_release_id"`
	OperatorTerms     string    `json:"operator_terms"`
	Status            string    `json:"status"`
	Version           int       `json:"version"`
	CreatedBy         string    `json:"created_by"`
	CreatedAt         time.Time `json:"created_at"`
	Events            []Event   `json:"events"`
	Signals           []Signal  `json:"signals"`
}
type Event struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	ActorID   string    `json:"actor_id"`
	Summary   string    `json:"summary"`
	CreatedAt time.Time `json:"created_at"`
}
type Signal struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	Outcome     string    `json:"outcome"`
	Corrections int       `json:"corrections"`
	Cost        float64   `json:"cost"`
	LatencyMS   int64     `json:"latency_ms"`
	Policy      string    `json:"policy"`
	Safety      string    `json:"safety"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
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
	if e := os.MkdirAll(root, 0700); e != nil {
		return nil, e
	}
	return &Store{root: root, now: func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }}, nil
}
func uid() string { var b [16]byte; _, _ = rand.Read(b[:]); return hex.EncodeToString(b[:]) }
func valid(v string, n int) bool {
	return strings.TrimSpace(v) != "" && len(v) <= n && !strings.ContainsRune(v, '\x00')
}
func validList(v []string) bool {
	if len(v) == 0 {
		return false
	}
	seen := map[string]bool{}
	for _, x := range v {
		if !valid(x, 200) || seen[x] {
			return false
		}
		seen[x] = true
	}
	return true
}
func (s *Store) path(kind, id string) string { return filepath.Join(s.root, kind+"-"+id+".json") }
func write(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".agent-release-")
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
	if ce := f.Close(); e == nil {
		e = ce
	}
	if e == nil {
		e = os.Rename(name, path)
	}
	if e == nil {
		d, de := os.Open(filepath.Dir(path))
		if de != nil {
			return de
		}
		e = d.Sync()
		_ = d.Close()
	}
	return e
}
func read[T any](path string) (T, error) {
	var v T
	b, e := os.ReadFile(path)
	if os.IsNotExist(e) {
		return v, ErrNotFound
	}
	if e != nil {
		return v, e
	}
	e = json.Unmarshal(b, &v)
	return v, e
}
func digest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func (s *Store) CreateRelease(v Release) (Release, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !valid(v.OrganizationID, 64) || !valid(v.RepositoryID, 64) || !valid(v.AgentID, 64) || !valid(v.CandidateID, 64) || len(v.CandidateRevision) != 40 || !valid(v.ProjectID, 64) || v.ProjectVersion < 1 || len(v.ContractDigest) != 64 || !validList(v.ModelVersions) || !validList(v.ToolVersions) || !validList(v.Roles) || !valid(v.PilotID, 64) {
		return Release{}, ErrInvalid
	}
	required := []string{"evaluation", "domain_review", "pilot_acceptance", "data_policy", "resources"}
	seenApprovals := map[string]bool{}
	for _, a := range v.Approvals {
		if !slices.Contains(required, a.Kind) || seenApprovals[a.Kind] || a.Decision != "approved" || !valid(a.OwnerID, 64) || !valid(a.EvidenceID, 200) || a.ApprovedAt.IsZero() || a.ApprovedAt.After(s.now().Add(time.Minute)) {
			return Release{}, ErrInvalid
		}
		seenApprovals[a.Kind] = true
	}
	for _, kind := range required {
		ok := false
		for _, a := range v.Approvals {
			if a.Kind == kind && a.Decision == "approved" && valid(a.OwnerID, 64) && valid(a.EvidenceID, 200) {
				ok = true
			}
		}
		if !ok {
			return Release{}, ErrDenied
		}
	}
	now := s.now()
	v.ID = uid()
	v.Version = 1
	v.Status = "attested"
	v.CreatedAt = now
	v.Attestation = digest(struct {
		Candidate            string
		Revision             string
		Contract             string
		Models, Tools, Roles []string
		Approvals            []Approval
	}{v.CandidateID, v.CandidateRevision, v.ContractDigest, v.ModelVersions, v.ToolVersions, v.Roles, v.Approvals})
	v.Events = []Event{{ID: uid(), Kind: "release.attested", ActorID: v.CreatedBy, Summary: "Required evidence and owner approvals passed.", CreatedAt: now}}
	return v, write(s.path("release", v.ID), v)
}
func (s *Store) GetRelease(id string) (Release, error) { return read[Release](s.path("release", id)) }
func (s *Store) ListReleases(repo string) ([]Release, error) {
	entries, e := os.ReadDir(s.root)
	if e != nil {
		return nil, e
	}
	out := []Release{}
	for _, x := range entries {
		if strings.HasPrefix(x.Name(), "release-") {
			v, e := read[Release](filepath.Join(s.root, x.Name()))
			if e != nil {
				return nil, e
			}
			if v.RepositoryID == repo {
				out = append(out, v)
			}
		}
	}
	return out, nil
}
func (s *Store) CreateDeployment(v Deployment) (Deployment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rel, e := s.GetRelease(v.ReleaseID)
	if e != nil {
		return Deployment{}, e
	}
	if rel.Status != "attested" || !valid(v.Identity, 200) || !validList(v.Roles) || !validList(v.CredentialScopes) || !subset(v.CredentialScopes, []string{"repository.read", "repository.write", "draft.create", "draft.update", "task.comment"}) || v.Budget.MaxCost <= 0 || v.Budget.MaxActions < 1 || v.Budget.MaxMinutes < 1 || !valid(v.RollbackReleaseID, 64) || !valid(v.OperatorTerms, 3000) {
		return Deployment{}, ErrInvalid
	}
	if !slices.Equal(v.Roles, rel.Roles) {
		return Deployment{}, ErrDenied
	}
	rollback, e := s.GetRelease(v.RollbackReleaseID)
	if e != nil || rollback.AgentID != rel.AgentID {
		return Deployment{}, ErrDenied
	}
	now := s.now()
	v.ID = uid()
	v.Status = "active"
	v.Version = 1
	v.CreatedAt = now
	v.Events = []Event{{ID: uid(), Kind: "deployment.activated", ActorID: v.CreatedBy, Summary: "Exact attested release activated with bounded authority.", CreatedAt: now}}
	v.Signals = []Signal{}
	return v, write(s.path("deployment", v.ID), v)
}

func subset(values, allowed []string) bool {
	for _, value := range values {
		if !slices.Contains(allowed, value) {
			return false
		}
	}
	return true
}
func (s *Store) GetDeployment(id string) (Deployment, error) {
	return read[Deployment](s.path("deployment", id))
}
func (s *Store) mutate(id string, expected int, fn func(*Deployment) error) (Deployment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, e := s.GetDeployment(id)
	if e != nil {
		return v, e
	}
	if v.Version != expected {
		return v, ErrConflict
	}
	if e = fn(&v); e != nil {
		return Deployment{}, e
	}
	v.Version++
	e = write(s.path("deployment", id), v)
	return v, e
}
func (s *Store) Signal(id, actor string, expected int, in Signal) (Deployment, error) {
	return s.mutate(id, expected, func(v *Deployment) error {
		allowed := []string{"outcome", "correction", "cost", "latency", "policy", "safety"}
		if !slices.Contains(allowed, in.Kind) || !valid(in.Outcome, 1000) || in.Corrections < 0 || in.Cost < 0 || in.LatencyMS < 0 {
			return ErrInvalid
		}
		in.ID = uid()
		in.CreatedBy = actor
		in.CreatedAt = s.now()
		v.Signals = append(v.Signals, in)
		return nil
	})
}
func (s *Store) Control(id, actor string, expected int, kind, summary string) (Deployment, error) {
	return s.mutate(id, expected, func(v *Deployment) error {
		if !slices.Contains([]string{"narrow", "pause", "rollback", "reopen_finding", "repair_human", "repair_agent"}, kind) || !valid(summary, 2000) {
			return ErrInvalid
		}
		switch kind {
		case "pause", "reopen_finding", "repair_human", "repair_agent":
			v.Status = "paused"
		case "rollback":
			v.Status = "rolled_back"
		case "narrow":
			v.Status = "narrowed"
		}
		v.Events = append(v.Events, Event{ID: uid(), Kind: kind, ActorID: actor, Summary: summary, CreatedAt: s.now()})
		return nil
	})
}
