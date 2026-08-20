// Package assuranceimpact retains revision-exact, collaborative compliance impact decisions.
package assuranceimpact

import (
	"crypto/rand"
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
)

var ErrNotFound = errors.New("assurance impact not found")
var ErrInvalid = errors.New("invalid assurance impact")
var ErrConflict = errors.New("assurance impact version conflict")
var ErrForbidden = errors.New("assurance impact action forbidden")

type Candidate struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Revision string `json:"revision"`
}
type Action struct {
	ID          string   `json:"id"`
	Description string   `json:"description"`
	OwnerIDs    []string `json:"owner_ids,omitempty"`
}
type ControlImpact struct {
	ControlID            string   `json:"control_id"`
	ControlTitle         string   `json:"control_title"`
	ControlDigest        string   `json:"control_digest"`
	Applicability        string   `json:"applicability"`
	Rationale            string   `json:"rationale"`
	AffectedPaths        []string `json:"affected_paths,omitempty"`
	ChangedEvidenceIDs   []string `json:"changed_evidence_ids,omitempty"`
	RequiredOwnerIDs     []string `json:"required_owner_ids,omitempty"`
	Tests                []Action `json:"tests,omitempty"`
	Notices              []Action `json:"notices,omitempty"`
	RetentionActions     []Action `json:"retention_actions,omitempty"`
	ExceptionIDs         []string `json:"exception_ids,omitempty"`
	Mitigation           string   `json:"mitigation,omitempty"`
	ResidualRisk         string   `json:"residual_risk,omitempty"`
	AcknowledgedOwnerIDs []string `json:"acknowledged_owner_ids,omitempty"`
	Current              bool     `json:"current"`
}
type Citation struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Revision   string `json:"revision"`
	Summary    string `json:"summary"`
}
type Event struct {
	ID        string     `json:"id"`
	Kind      string     `json:"kind"`
	ControlID string     `json:"control_id"`
	Body      string     `json:"body"`
	Citations []Citation `json:"citations"`
	ActorID   string     `json:"actor_id"`
	ActorType string     `json:"actor_type"`
	CreatedAt time.Time  `json:"created_at"`
}
type Assessment struct {
	ID             string          `json:"id"`
	RepositoryID   string          `json:"repository_id"`
	Candidate      Candidate       `json:"candidate"`
	ProgramID      string          `json:"program_id"`
	ProgramVersion int             `json:"program_version"`
	Version        int             `json:"version"`
	Impacts        []ControlImpact `json:"impacts"`
	Events         []Event         `json:"events"`
	Ready          bool            `json:"ready"`
	Stale          bool            `json:"stale"`
	CreatedBy      string          `json:"created_by"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
	Authority      string          `json:"authority"`
}
type Current struct {
	CandidateRevision string
	ControlDigests    map[string]string
	ParticipantIDs    map[string]bool
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

func (s *Store) Create(repo, actor string, candidate Candidate, programID string, programVersion int, impacts []ControlImpact) (Assessment, error) {
	var out Assessment
	err := s.lock(func() error {
		if repo == "" || actor == "" || programID == "" || programVersion < 1 || !validCandidate(candidate) || !validImpacts(impacts) {
			return ErrInvalid
		}
		now := s.now()
		out = Assessment{ID: newID(), RepositoryID: repo, Candidate: candidate, ProgramID: programID, ProgramVersion: programVersion, Version: 1, Impacts: impacts, CreatedBy: actor, CreatedAt: now, UpdatedAt: now, Authority: "Compliance impact records govern readiness only when current controls require acknowledgement; they grant no Git, review, merge, release, evidence, policy, or linked-system authority."}
		return s.write(out)
	})
	return project(out, Current{CandidateRevision: candidate.Revision, ControlDigests: impactDigests(impacts)}), err
}
func (s *Store) AddEvent(repo, id string, expected int, actor, actorType string, event Event, current Current) (Assessment, error) {
	var out Assessment
	err := s.lock(func() error {
		p, e := s.read(id)
		if e != nil {
			return e
		}
		if p.RepositoryID != repo {
			return ErrNotFound
		}
		if p.Version != expected {
			return ErrConflict
		}
		if actor == "" || !one(actorType, "human", "agent") || !one(event.Kind, "analysis", "challenge", "mitigation", "residual_risk") || event.ControlID == "" || strings.TrimSpace(event.Body) == "" || len(event.Body) > 4000 {
			return ErrInvalid
		}
		if !hasControl(p, event.ControlID) {
			return ErrInvalid
		}
		for _, c := range event.Citations {
			if c.Kind == "" || c.ResourceID == "" || c.Revision == "" || c.Summary == "" {
				return ErrInvalid
			}
		}
		event.ID = newID()
		event.ActorID = actor
		event.ActorType = actorType
		event.CreatedAt = s.now()
		p.Events = append(p.Events, event)
		p.Version++
		p.UpdatedAt = event.CreatedAt
		out = p
		return s.write(p)
	})
	return project(out, current), err
}
func (s *Store) Acknowledge(repo, id, controlID, actor string, expected int, current Current) (Assessment, error) {
	var out Assessment
	err := s.lock(func() error {
		p, e := s.read(id)
		if e != nil {
			return e
		}
		if p.RepositoryID != repo {
			return ErrNotFound
		}
		if p.Version != expected {
			return ErrConflict
		}
		found := false
		for i := range p.Impacts {
			if p.Impacts[i].ControlID != controlID {
				continue
			}
			if !contains(p.Impacts[i].RequiredOwnerIDs, actor) {
				return ErrForbidden
			}
			found = true
			if !contains(p.Impacts[i].AcknowledgedOwnerIDs, actor) {
				p.Impacts[i].AcknowledgedOwnerIDs = append(p.Impacts[i].AcknowledgedOwnerIDs, actor)
			}
		}
		if !found {
			return ErrInvalid
		}
		p.Version++
		p.UpdatedAt = s.now()
		out = p
		return s.write(p)
	})
	return project(out, current), err
}
func (s *Store) Get(repo, id string, current Current) (Assessment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, e := s.read(id)
	if e != nil || p.RepositoryID != repo {
		if e == nil {
			e = ErrNotFound
		}
		return Assessment{}, e
	}
	return project(p, current), nil
}
func (s *Store) List(repo string, current func(Assessment) Current) ([]Assessment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	es, e := os.ReadDir(s.root)
	if e != nil {
		return nil, e
	}
	out := []Assessment{}
	for _, x := range es {
		if x.IsDir() || !strings.HasSuffix(x.Name(), ".json") {
			continue
		}
		p, e := s.read(strings.TrimSuffix(x.Name(), ".json"))
		if e != nil {
			return nil, e
		}
		if p.RepositoryID == repo {
			out = append(out, project(p, current(p)))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

func project(p Assessment, current Current) Assessment {
	p.Ready = true
	p.Stale = current.CandidateRevision == "" || current.CandidateRevision != p.Candidate.Revision
	seen := map[string]bool{}
	for i := range p.Impacts {
		impact := &p.Impacts[i]
		seen[impact.ControlID] = true
		impact.Current = !p.Stale && current.ControlDigests[impact.ControlID] == impact.ControlDigest
		if !impact.Current {
			p.Stale = true
		}
		if impact.Applicability == "uncertain" || !impact.Current || !allCurrent(impact.RequiredOwnerIDs, impact.AcknowledgedOwnerIDs, current.ParticipantIDs) {
			p.Ready = false
		}
	}
	for controlID := range current.ControlDigests {
		if !seen[controlID] {
			p.Stale = true
			p.Ready = false
		}
	}
	return p
}
func Digest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func validCandidate(c Candidate) bool {
	return one(c.Kind, "pull_request", "infrastructure_plan", "schema_migration", "extension_installation", "package_update", "release_candidate") && c.ID != "" && c.Revision != ""
}
func validImpacts(xs []ControlImpact) bool {
	seen := map[string]bool{}
	for _, x := range xs {
		if x.ControlID == "" || x.ControlTitle == "" || x.ControlDigest == "" || seen[x.ControlID] || !one(x.Applicability, "affected", "not_affected", "uncertain") || x.Rationale == "" {
			return false
		}
		seen[x.ControlID] = true
		for _, a := range append(append(x.Tests, x.Notices...), x.RetentionActions...) {
			if a.ID == "" || a.Description == "" {
				return false
			}
		}
	}
	return true
}
func impactDigests(xs []ControlImpact) map[string]string {
	m := map[string]string{}
	for _, x := range xs {
		m[x.ControlID] = x.ControlDigest
	}
	return m
}
func hasControl(p Assessment, id string) bool {
	for _, x := range p.Impacts {
		if x.ControlID == id {
			return true
		}
	}
	return false
}
func all(required, actual []string) bool {
	for _, x := range required {
		if !contains(actual, x) {
			return false
		}
	}
	return true
}
func allCurrent(required, actual []string, participants map[string]bool) bool {
	for _, x := range required {
		if !contains(actual, x) || !participants[x] {
			return false
		}
	}
	return true
}
func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
func one(v string, xs ...string) bool {
	for _, x := range xs {
		if v == x {
			return true
		}
	}
	return false
}
func newID() string                    { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func (s *Store) path(id string) string { return filepath.Join(s.root, id+".json") }
func (s *Store) read(id string) (Assessment, error) {
	b, e := os.ReadFile(s.path(id))
	if os.IsNotExist(e) {
		return Assessment{}, ErrNotFound
	}
	if e != nil {
		return Assessment{}, e
	}
	var p Assessment
	if json.Unmarshal(b, &p) != nil {
		return Assessment{}, ErrInvalid
	}
	return p, nil
}
func (s *Store) write(p Assessment) error {
	b, e := json.MarshalIndent(p, "", "  ")
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(s.root, "impact-*.tmp")
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
	if e == nil {
		e = os.Rename(name, s.path(p.ID))
	}
	if e == nil {
		if d, x := os.Open(s.root); x == nil {
			e = d.Sync()
			_ = d.Close()
		}
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
