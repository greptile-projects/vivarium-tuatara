// Package signalevaluations retains reproducible signal findings, decisions, and retirement history.
package signalevaluations

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
	"time"
)

var ErrNotFound = errors.New("signal evaluation not found")
var ErrInvalid = errors.New("invalid signal evaluation")
var ErrConflict = errors.New("signal evaluation conflict")

type Correlation struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Revision   string `json:"revision"`
	Label      string `json:"label"`
}
type Citation struct {
	Kind        string    `json:"kind"`
	ResourceID  string    `json:"resource_id"`
	Revision    string    `json:"revision"`
	Digest      string    `json:"digest"`
	Query       string    `json:"query"`
	WindowStart time.Time `json:"window_start"`
	WindowEnd   time.Time `json:"window_end"`
}
type Criterion struct {
	ID          string   `json:"id"`
	Result      string   `json:"result"`
	Rationale   string   `json:"rationale"`
	CitationIDs []string `json:"citation_ids"`
}
type Finding struct {
	RequestID    string      `json:"request_id"`
	ID           string      `json:"id"`
	ActorKind    string      `json:"actor_kind"`
	ActorID      string      `json:"actor_id"`
	Summary      string      `json:"summary"`
	Method       string      `json:"method"`
	Reproduction string      `json:"reproduction"`
	Uncertainty  string      `json:"uncertainty"`
	Citations    []Citation  `json:"citations"`
	Criteria     []Criterion `json:"criteria"`
	CreatedAt    time.Time   `json:"created_at"`
}
type Consumer struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Revision   string `json:"revision"`
	OwnerID    string `json:"owner_id"`
	Impact     string `json:"impact"`
}
type Update struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Revision   string `json:"revision"`
	Summary    string `json:"summary"`
}
type Repair struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Summary    string `json:"summary"`
}
type Decision struct {
	RequestID        string     `json:"request_id"`
	ID               string     `json:"id"`
	ExpectedVersion  int        `json:"expected_version"`
	Action           string     `json:"action"`
	Rationale        string     `json:"rationale"`
	FindingIDs       []string   `json:"finding_ids"`
	PolicyApproval   string     `json:"policy_approval"`
	Consumers        []Consumer `json:"consumers"`
	Updates          []Update   `json:"updates"`
	Repair           *Repair    `json:"repair,omitempty"`
	StopVerification *Citation  `json:"stop_verification,omitempty"`
	ActorID          string     `json:"actor_id"`
	CreatedAt        time.Time  `json:"created_at"`
}
type Evaluation struct {
	RequestID       string        `json:"request_id"`
	ID              string        `json:"id"`
	RepositoryID    string        `json:"repository_id"`
	GapID           string        `json:"gap_id"`
	GapVersion      int           `json:"gap_version"`
	ContractID      string        `json:"contract_id"`
	ContractVersion int           `json:"contract_version"`
	RolloutID       string        `json:"rollout_id"`
	RolloutVersion  int           `json:"rollout_version"`
	SignalIDs       []string      `json:"signal_ids"`
	Question        string        `json:"question"`
	OwnerIDs        []string      `json:"owner_ids"`
	Correlations    []Correlation `json:"correlations"`
	Consumers       []Consumer    `json:"consumers"`
	Status          string        `json:"status"`
	Version         int           `json:"version"`
	Findings        []Finding     `json:"findings"`
	Decisions       []Decision    `json:"decisions"`
	CreatedBy       string        `json:"created_by"`
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
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
func (s *Store) Create(repo, actor string, in Evaluation) (Evaluation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if in.RequestID == "" || in.GapID == "" || in.GapVersion < 1 || in.ContractID == "" || in.ContractVersion < 1 || in.RolloutID == "" || in.RolloutVersion < 1 || strings.TrimSpace(in.Question) == "" || len(in.OwnerIDs) == 0 || len(in.SignalIDs) == 0 {
		return Evaluation{}, ErrInvalid
	}
	in.ID = stable(repo, actor, in.RequestID)
	in.RepositoryID = repo
	in.CreatedBy = actor
	if old, e := s.read(in.ID); e == nil {
		if digest(createView(old)) != digest(createView(in)) {
			return Evaluation{}, ErrConflict
		}
		return old, nil
	}
	n := s.now()
	in.Status = "evaluating"
	in.Version = 1
	in.Findings = []Finding{}
	in.Decisions = []Decision{}
	in.CreatedAt = n
	in.UpdatedAt = n
	if in.Correlations == nil {
		in.Correlations = []Correlation{}
	}
	if in.Consumers == nil {
		in.Consumers = []Consumer{}
	}
	return in, s.write(in)
}
func (s *Store) Get(repo, id string) (Evaluation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	x, e := s.read(id)
	if e != nil || x.RepositoryID != repo {
		return Evaluation{}, ErrNotFound
	}
	return x, nil
}
func (s *Store) List(repo string) ([]Evaluation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []Evaluation{}
	fs, e := os.ReadDir(s.root)
	if e != nil {
		return nil, e
	}
	for _, f := range fs {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
			continue
		}
		var x Evaluation
		if e = s.decode(f.Name(), &x); e != nil {
			return nil, e
		}
		if x.RepositoryID == repo {
			out = append(out, x)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}
func (s *Store) AddFinding(repo, id string, f Finding) (Evaluation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	x, e := s.read(id)
	if e != nil || x.RepositoryID != repo {
		return Evaluation{}, ErrNotFound
	}
	for _, old := range x.Findings {
		if old.RequestID == f.RequestID {
			candidate := f
			candidate.ID = old.ID
			candidate.ActorKind = old.ActorKind
			candidate.ActorID = old.ActorID
			candidate.CreatedAt = old.CreatedAt
			if digest(old) != digest(candidate) {
				return Evaluation{}, ErrConflict
			}
			return x, nil
		}
	}
	if f.RequestID == "" || strings.TrimSpace(f.Summary) == "" || strings.TrimSpace(f.Method) == "" || strings.TrimSpace(f.Reproduction) == "" || strings.TrimSpace(f.Uncertainty) == "" || len(f.Citations) == 0 || len(f.Criteria) == 0 {
		return Evaluation{}, ErrInvalid
	}
	for _, c := range f.Citations {
		if c.Kind == "" || c.ResourceID == "" || c.Revision == "" || len(c.Digest) != 64 || c.Query == "" || c.WindowStart.IsZero() || !c.WindowEnd.After(c.WindowStart) {
			return Evaluation{}, ErrInvalid
		}
	}
	f.ID = stable(x.ID, f.RequestID)
	f.CreatedAt = s.now()
	x.Findings = append(x.Findings, f)
	x.Version++
	x.UpdatedAt = f.CreatedAt
	return x, s.write(x)
}
func (s *Store) Decide(repo, id string, d Decision) (Evaluation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	x, e := s.read(id)
	if e != nil || x.RepositoryID != repo {
		return Evaluation{}, ErrNotFound
	}
	for _, old := range x.Decisions {
		if old.RequestID == d.RequestID {
			candidate := d
			candidate.ID = old.ID
			candidate.ActorID = old.ActorID
			candidate.CreatedAt = old.CreatedAt
			if digest(old) != digest(candidate) {
				return Evaluation{}, ErrConflict
			}
			return x, nil
		}
	}
	valid := map[string]bool{"retain": true, "revise": true, "reduce": true, "archive": true, "remove": true}
	if d.RequestID == "" || d.ExpectedVersion != x.Version || !valid[d.Action] || d.Rationale == "" || len(d.FindingIDs) == 0 || d.PolicyApproval == "" {
		return Evaluation{}, ErrInvalid
	}
	known := map[string]bool{}
	for _, f := range x.Findings {
		known[f.ID] = true
	}
	for _, id := range d.FindingIDs {
		if !known[id] {
			return Evaluation{}, ErrInvalid
		}
	}
	if (d.Action == "revise" || d.Action == "reduce") && d.Repair == nil {
		return Evaluation{}, ErrInvalid
	}
	if d.Repair != nil && (strings.TrimSpace(d.Repair.Kind) == "" || strings.TrimSpace(d.Repair.ResourceID) == "" || strings.TrimSpace(d.Repair.Summary) == "") {
		return Evaluation{}, ErrInvalid
	}
	updateKinds := map[string]bool{"service_objective": true, "alert": true, "runbook": true, "investigation": true, "quality_check": true, "decision": true}
	for _, update := range d.Updates {
		if !updateKinds[update.Kind] || update.ResourceID == "" || update.Revision == "" || update.Summary == "" {
			return Evaluation{}, ErrInvalid
		}
	}
	if (d.Action == "archive" || d.Action == "remove") && (d.StopVerification == nil || len(d.StopVerification.Digest) != 64 || len(d.Consumers) == 0) {
		return Evaluation{}, ErrInvalid
	}
	if d.StopVerification != nil && (d.StopVerification.ResourceID == "" || d.StopVerification.Revision == "" || d.StopVerification.Query == "" || d.StopVerification.WindowStart.IsZero() || !d.StopVerification.WindowEnd.After(d.StopVerification.WindowStart)) {
		return Evaluation{}, ErrInvalid
	}
	d.ID = stable(x.ID, d.RequestID)
	d.CreatedAt = s.now()
	x.Decisions = append(x.Decisions, d)
	x.Status = map[string]string{"retain": "retained", "revise": "revision_required", "reduce": "reduction_required", "archive": "archived", "remove": "removed"}[d.Action]
	x.Version++
	x.UpdatedAt = d.CreatedAt
	return x, s.write(x)
}
func createView(x Evaluation) Evaluation {
	x.ID = ""
	x.RepositoryID = ""
	x.Status = ""
	x.Version = 0
	x.Findings = nil
	x.Decisions = nil
	x.CreatedBy = ""
	x.CreatedAt = time.Time{}
	x.UpdatedAt = time.Time{}
	return x
}
func stable(parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(h[:16])
}
func digest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func (s *Store) path(id string) string { return filepath.Join(s.root, id+".json") }
func (s *Store) read(id string) (Evaluation, error) {
	var x Evaluation
	e := s.decode(id+".json", &x)
	return x, e
}
func (s *Store) decode(name string, v any) error {
	b, e := os.ReadFile(filepath.Join(s.root, name))
	if e != nil {
		return e
	}
	return json.Unmarshal(b, v)
}
func (s *Store) write(x Evaluation) error {
	b, e := json.MarshalIndent(x, "", "  ")
	if e != nil {
		return e
	}
	tmp := s.path(x.ID) + ".tmp"
	if e = os.WriteFile(tmp, b, 0600); e != nil {
		return e
	}
	return os.Rename(tmp, s.path(x.ID))
}
