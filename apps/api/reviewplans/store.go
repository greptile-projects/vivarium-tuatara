// Package reviewplans retains revision-exact plans for ordinary pull review.
package reviewplans

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
	"time"
)

var ErrInvalid = errors.New("invalid review plan")
var ErrNotFound = errors.New("review plan not found")

type Evidence struct {
	Kind        string `json:"kind"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}
type Area struct {
	ID             string     `json:"id"`
	Title          string     `json:"title"`
	Rationale      string     `json:"rationale"`
	Paths          []string   `json:"paths"`
	OwnerIDs       []string   `json:"owner_ids,omitempty"`
	Questions      []string   `json:"acceptance_questions"`
	Evidence       []Evidence `json:"required_evidence"`
	DependsOn      []string   `json:"depends_on,omitempty"`
	CompletionRule string     `json:"completion_rule"`
}
type Diagnostic struct {
	Code         string `json:"code"`
	AreaID       string `json:"area_id,omitempty"`
	Message      string `json:"message"`
	AttributedTo string `json:"attributed_to,omitempty"`
}
type Plan struct {
	ID                  string       `json:"id"`
	RepositoryID        string       `json:"repository_id"`
	PullRequestID       string       `json:"pull_request_id"`
	Version             int          `json:"version"`
	SourceRevision      string       `json:"source_revision"`
	TargetRevision      string       `json:"target_revision"`
	Intent              string       `json:"intent"`
	RiskSummary         string       `json:"risk_summary"`
	ChangedPaths        []string     `json:"changed_paths"`
	PolicyRequirements  []string     `json:"policy_requirements"`
	AffectedCommitments []string     `json:"affected_commitments"`
	Areas               []Area       `json:"areas"`
	CompletionRule      string       `json:"completion_rule"`
	Diagnostics         []Diagnostic `json:"diagnostics"`
	Stale               bool         `json:"stale"`
	CreatedBy           string       `json:"created_by"`
	CreatedAt           time.Time    `json:"created_at"`
	Authority           string       `json:"authority"`
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
func (s *Store) Create(plan Plan) (Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if plan.RepositoryID == "" || plan.PullRequestID == "" || len(plan.SourceRevision) != 40 || len(plan.TargetRevision) != 40 || plan.CreatedBy == "" || strings.TrimSpace(plan.Intent) == "" || len(plan.Areas) == 0 {
		return Plan{}, ErrInvalid
	}
	values, err := s.read(plan.RepositoryID, plan.PullRequestID)
	if err != nil {
		return Plan{}, err
	}
	plan.ID = newID()
	plan.Version = len(values) + 1
	plan.CreatedAt = s.now()
	plan.Stale = false
	plan.Authority = "A review plan coordinates the expertise and evidence this exact change needs; it grants no repository, evidence, review, approval, merge, policy, commitment, or disclosure authority."
	values = append(values, plan)
	return plan, s.write(plan.RepositoryID, plan.PullRequestID, values)
}
func (s *Store) List(repo, pull, currentSource, currentTarget string) ([]Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	values, err := s.read(repo, pull)
	if err != nil {
		return nil, err
	}
	for i := range values {
		values[i].Stale = values[i].SourceRevision != currentSource || values[i].TargetRevision != currentTarget
		if values[i].Stale {
			values[i].Diagnostics = append(values[i].Diagnostics, Diagnostic{Code: "stale_analysis", Message: "The pull source or target moved after this plan was derived.", AttributedTo: values[i].CreatedBy})
		}
	}
	return values, nil
}
func (s *Store) read(repo, pull string) ([]Plan, error) {
	data, err := os.ReadFile(s.path(repo, pull))
	if errors.Is(err, os.ErrNotExist) {
		return []Plan{}, nil
	}
	if err != nil {
		return nil, err
	}
	var v []Plan
	if json.Unmarshal(data, &v) != nil {
		return nil, ErrInvalid
	}
	return v, nil
}
func (s *Store) write(repo, pull string, v []Plan) error {
	if err := os.MkdirAll(filepath.Dir(s.path(repo, pull)), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path(repo, pull) + ".tmp"
	if err = os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path(repo, pull))
}
func (s *Store) path(repo, pull string) string { return filepath.Join(s.root, repo, pull+".json") }
func newID() string                            { b := make([]byte, 12); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func Normalize(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}
