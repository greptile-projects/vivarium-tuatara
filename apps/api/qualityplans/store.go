// Package qualityplans persists versioned repository quality intent.
package qualityplans

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

var ErrNotFound = errors.New("quality plan not found")
var ErrInvalid = errors.New("invalid quality plan")
var ErrConflict = errors.New("quality plan version conflict")

type Scope struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id,omitempty"`
	Name       string `json:"name"`
}
type Environment struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Supported   bool   `json:"supported"`
}
type Requirement struct {
	ID                 string   `json:"id"`
	SourceKind         string   `json:"source_kind"`
	SourceID           string   `json:"source_id"`
	Title              string   `json:"title"`
	Rationale          string   `json:"rationale"`
	ExpectedBehavior   string   `json:"expected_behavior"`
	Risk               string   `json:"risk"`
	TestLevels         []string `json:"test_levels"`
	RepresentativeData string   `json:"representative_data"`
	CoverageGoal       string   `json:"coverage_goal"`
	OwnerIDs           []string `json:"owner_ids"`
	JudgeIDs           []string `json:"judge_ids"`
	EnvironmentIDs     []string `json:"environment_ids"`
	Schedule           string   `json:"schedule"`
	ReleaseThreshold   string   `json:"release_threshold"`
	EvidenceIDs        []string `json:"evidence_ids"`
	ConflictsWith      []string `json:"conflicts_with,omitempty"`
	Verification       string   `json:"verification,omitempty"`
}
type Evidence struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	ResourceKind string `json:"resource_kind"`
	ResourceID   string `json:"resource_id"`
	Revision     string `json:"revision,omitempty"`
	Summary      string `json:"summary"`
	Status       string `json:"status"`
	AddedBy      string `json:"added_by,omitempty"`
}
type Exception struct {
	ID            string    `json:"id"`
	RequirementID string    `json:"requirement_id"`
	Rationale     string    `json:"rationale"`
	GrantedBy     string    `json:"granted_by"`
	ExpiresAt     time.Time `json:"expires_at"`
	FollowUp      string    `json:"follow_up"`
}
type Diagnostic struct {
	Kind          string `json:"kind"`
	Severity      string `json:"severity"`
	Message       string `json:"message"`
	RequirementID string `json:"requirement_id,omitempty"`
	AttributedTo  string `json:"attributed_to"`
}
type Revision struct {
	Version        int           `json:"version"`
	Title          string        `json:"title"`
	Summary        string        `json:"summary"`
	Scopes         []Scope       `json:"scopes"`
	Environments   []Environment `json:"supported_environments"`
	Requirements   []Requirement `json:"requirements"`
	Evidence       []Evidence    `json:"evidence"`
	Exceptions     []Exception   `json:"exceptions"`
	OwnerIDs       []string      `json:"owner_ids"`
	ReviewSchedule string        `json:"review_schedule"`
	Rationale      string        `json:"rationale"`
	CreatedBy      string        `json:"created_by"`
	CreatedAt      time.Time     `json:"created_at"`
}
type Plan struct {
	ID             string       `json:"id"`
	RepositoryID   string       `json:"repository_id"`
	CurrentVersion int          `json:"current_version"`
	Revisions      []Revision   `json:"revisions"`
	Diagnostics    []Diagnostic `json:"diagnostics"`
	CreatedAt      time.Time    `json:"created_at"`
	UpdatedAt      time.Time    `json:"updated_at"`
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
func (s *Store) Create(repo, actor string, r Revision) (Plan, error) {
	var out Plan
	err := s.lock(func() error {
		if validate(r) != nil {
			return ErrInvalid
		}
		now := s.now()
		stamp(&r, 1, actor, now)
		out = Plan{ID: id(), RepositoryID: repo, CurrentVersion: 1, Revisions: []Revision{r}, CreatedAt: now, UpdatedAt: now}
		return s.write(out)
	})
	return s.project(out), err
}
func (s *Store) Revise(id string, expected int, actor string, r Revision) (Plan, error) {
	var out Plan
	err := s.lock(func() error {
		v, e := s.read(id)
		if e != nil {
			return e
		}
		if v.CurrentVersion != expected {
			return ErrConflict
		}
		if validate(r) != nil {
			return ErrInvalid
		}
		stamp(&r, expected+1, actor, s.now())
		v.CurrentVersion = r.Version
		v.Revisions = append(v.Revisions, r)
		v.UpdatedAt = r.CreatedAt
		out = v
		return s.write(v)
	})
	return s.project(out), err
}
func (s *Store) Get(id string) (Plan, error) {
	var out Plan
	err := s.lock(func() error { var e error; out, e = s.read(id); return e })
	return s.project(out), err
}
func (s *Store) List(repo string) ([]Plan, error) {
	out := []Plan{}
	err := s.lock(func() error {
		entries, e := os.ReadDir(s.root)
		if e != nil {
			return e
		}
		for _, x := range entries {
			if x.IsDir() || !strings.HasSuffix(x.Name(), ".json") {
				continue
			}
			v, e := s.read(strings.TrimSuffix(x.Name(), ".json"))
			if e != nil {
				return e
			}
			if v.RepositoryID == repo {
				out = append(out, s.project(v))
			}
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, err
}
func stamp(r *Revision, v int, actor string, now time.Time) {
	r.Version = v
	r.CreatedBy = actor
	r.CreatedAt = now
	for i := range r.Evidence {
		r.Evidence[i].AddedBy = actor
	}
}

func validate(r Revision) error {
	if strings.TrimSpace(r.Title) == "" || len(r.Title) > 200 || strings.TrimSpace(r.Summary) == "" || len(r.Scopes) == 0 || len(r.Environments) == 0 || len(r.Requirements) == 0 || strings.TrimSpace(r.ReviewSchedule) == "" || strings.TrimSpace(r.Rationale) == "" {
		return ErrInvalid
	}
	for _, scope := range r.Scopes {
		if !set("repository", "release", "journey", "interface")[scope.Kind] || strings.TrimSpace(scope.Name) == "" || (scope.Kind != "repository" && strings.TrimSpace(scope.ResourceID) == "") {
			return ErrInvalid
		}
	}
	seenEnv := map[string]bool{}
	for _, e := range r.Environments {
		if strings.TrimSpace(e.ID) == "" || strings.TrimSpace(e.Name) == "" || seenEnv[e.ID] {
			return ErrInvalid
		}
		seenEnv[e.ID] = true
	}
	seenReq := map[string]bool{}
	validSource := set("issue", "decision", "design", "accessibility", "privacy", "performance", "reliability")
	validRisk := set("low", "medium", "high", "critical")
	validLevel := set("unit", "integration", "contract", "system", "end_to_end", "exploratory", "accessibility", "performance", "reliability", "security", "manual")
	for _, q := range r.Requirements {
		if strings.TrimSpace(q.ID) == "" || seenReq[q.ID] || !validSource[q.SourceKind] || strings.TrimSpace(q.SourceID) == "" || strings.TrimSpace(q.Title) == "" || strings.TrimSpace(q.ExpectedBehavior) == "" || !validRisk[q.Risk] || len(q.TestLevels) == 0 || strings.TrimSpace(q.RepresentativeData) == "" || strings.TrimSpace(q.CoverageGoal) == "" || strings.TrimSpace(q.Schedule) == "" || strings.TrimSpace(q.ReleaseThreshold) == "" {
			return ErrInvalid
		}
		seenReq[q.ID] = true
		for _, l := range q.TestLevels {
			if !validLevel[l] {
				return ErrInvalid
			}
		}
		for _, e := range q.EnvironmentIDs {
			if !seenEnv[e] {
				return ErrInvalid
			}
		}
	}
	seenEvidence := map[string]bool{}
	for _, e := range r.Evidence {
		if strings.TrimSpace(e.ID) == "" || seenEvidence[e.ID] || !set("automated", "manual")[e.Kind] || strings.TrimSpace(e.ResourceKind) == "" || strings.TrimSpace(e.ResourceID) == "" || !set("passing", "failing", "missing", "stale", "unknown")[e.Status] {
			return ErrInvalid
		}
		seenEvidence[e.ID] = true
	}
	for _, q := range r.Requirements {
		for _, e := range q.EvidenceIDs {
			if !seenEvidence[e] {
				return ErrInvalid
			}
		}
		for _, c := range q.ConflictsWith {
			if !seenReq[c] || c == q.ID {
				return ErrInvalid
			}
		}
	}
	for _, x := range r.Exceptions {
		if strings.TrimSpace(x.ID) == "" || !seenReq[x.RequirementID] || strings.TrimSpace(x.Rationale) == "" || strings.TrimSpace(x.GrantedBy) == "" || x.ExpiresAt.IsZero() || strings.TrimSpace(x.FollowUp) == "" {
			return ErrInvalid
		}
	}
	return nil
}
func set(v ...string) map[string]bool {
	m := map[string]bool{}
	for _, x := range v {
		m[x] = true
	}
	return m
}
func (s *Store) project(v Plan) Plan {
	if len(v.Revisions) == 0 {
		return v
	}
	r := v.Revisions[len(v.Revisions)-1]
	d := []Diagnostic{}
	attr := r.CreatedBy
	if len(r.OwnerIDs) == 0 {
		d = append(d, Diagnostic{"missing_ownership", "blocking", "The plan has no accountable owner.", "", attr})
	}
	evidence := map[string]Evidence{}
	for _, e := range r.Evidence {
		evidence[e.ID] = e
	}
	for _, q := range r.Requirements {
		if len(q.OwnerIDs) == 0 || len(q.JudgeIDs) == 0 {
			d = append(d, Diagnostic{"missing_ownership", "blocking", "Expected behavior needs both an owner and a release judge.", q.ID, attr})
		}
		if strings.TrimSpace(q.Verification) == "" {
			d = append(d, Diagnostic{"untestable_claim", "blocking", "No observable verification method explains how this behavior can be judged.", q.ID, attr})
		}
		if len(q.ConflictsWith) > 0 {
			d = append(d, Diagnostic{"contradictory_expectation", "blocking", "This expectation explicitly conflicts with another retained requirement.", q.ID, attr})
		}
		if len(q.EvidenceIDs) == 0 {
			d = append(d, Diagnostic{"missing_evidence", "warning", "No automated or manual evidence is linked.", q.ID, attr})
		} else {
			for _, id := range q.EvidenceIDs {
				if evidence[id].Status == "missing" || evidence[id].Status == "unknown" {
					d = append(d, Diagnostic{"missing_evidence", "warning", "Linked evidence is not currently available.", q.ID, evidence[id].AddedBy})
				}
			}
		}
	}
	now := s.now()
	for _, x := range r.Exceptions {
		severity := "warning"
		message := "This quality exception expires within seven days."
		if !x.ExpiresAt.After(now) {
			severity = "blocking"
			message = "This quality exception has expired."
		} else if x.ExpiresAt.After(now.Add(7 * 24 * time.Hour)) {
			continue
		}
		d = append(d, Diagnostic{"expiring_exception", severity, message, x.RequirementID, x.GrantedBy})
	}
	v.Diagnostics = d
	return v
}
func id() string                       { b := make([]byte, 12); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func (s *Store) path(id string) string { return filepath.Join(s.root, id+".json") }
func (s *Store) read(id string) (Plan, error) {
	if strings.ContainsAny(id, "/\\") {
		return Plan{}, ErrNotFound
	}
	b, e := os.ReadFile(s.path(id))
	if os.IsNotExist(e) {
		return Plan{}, ErrNotFound
	}
	if e != nil {
		return Plan{}, e
	}
	var v Plan
	if json.Unmarshal(b, &v) != nil || v.ID != id {
		return Plan{}, ErrInvalid
	}
	return v, nil
}
func (s *Store) write(v Plan) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	tmp := s.path(v.ID) + ".tmp-" + id()
	if e = os.WriteFile(tmp, b, 0600); e != nil {
		return e
	}
	if e = os.Rename(tmp, s.path(v.ID)); e != nil {
		_ = os.Remove(tmp)
		return e
	}
	return nil
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
