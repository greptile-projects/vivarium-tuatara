// Package securityfindings retains confidential, attributable security finding decisions and repair links.
package securityfindings

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

var ErrNotFound = errors.New("security finding not found")
var ErrInvalid = errors.New("invalid security finding")
var ErrConflict = errors.New("security finding changed")

type Evidence struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Summary string `json:"summary"`
	SHA256  string `json:"sha256,omitempty"`
}
type Event struct {
	ID             string    `json:"id"`
	Kind           string    `json:"kind"`
	Classification string    `json:"classification,omitempty"`
	Rationale      string    `json:"rationale"`
	ActorID        string    `json:"actor_id"`
	CreatedAt      time.Time `json:"created_at"`
}
type Repair struct {
	ProposalID     string    `json:"proposal_id"`
	TaskID         string    `json:"task_id"`
	AssigneeType   string    `json:"assignee_type"`
	AssigneeID     string    `json:"assignee_id"`
	PullRequestID  string    `json:"pull_request_id,omitempty"`
	RepairCommitID string    `json:"repair_commit_id,omitempty"`
	ScenarioID     string    `json:"scenario_id,omitempty"`
	State          string    `json:"state"`
	CreatedAt      time.Time `json:"created_at"`
}
type Finding struct {
	ID                 string     `json:"id"`
	RepositoryID       string     `json:"repository_id"`
	ThreatModelID      string     `json:"threat_model_id"`
	ThreatModelVersion int        `json:"threat_model_version"`
	AbusePathID        string     `json:"abuse_path_id"`
	CandidateCommitID  string     `json:"candidate_commit_id"`
	Title              string     `json:"title"`
	Description        string     `json:"description"`
	Severity           string     `json:"severity"`
	Audience           []string   `json:"audience"`
	Evidence           []Evidence `json:"evidence"`
	AcceptanceCriteria []string   `json:"acceptance_criteria"`
	ReporterID         string     `json:"reporter_id"`
	Version            int        `json:"version"`
	Events             []Event    `json:"events"`
	Repair             *Repair    `json:"repair,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
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
func (s *Store) Create(repo, reporter string, v Finding) (Finding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !valid(v) || blank(reporter) {
		return Finding{}, ErrInvalid
	}
	v.ID = newID()
	v.RepositoryID = repo
	v.ReporterID = reporter
	v.Version = 1
	v.Events = []Event{}
	v.Repair = nil
	v.CreatedAt = s.now()
	v.UpdatedAt = v.CreatedAt
	return v, s.write(v)
}
func (s *Store) Get(repo, id string) (Finding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, e := s.read(id)
	if e != nil || v.RepositoryID != repo {
		return Finding{}, ErrNotFound
	}
	return v, nil
}
func (s *Store) List(repo, actor string) ([]Finding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	es, e := os.ReadDir(s.root)
	if e != nil {
		return nil, e
	}
	out := []Finding{}
	for _, x := range es {
		if x.IsDir() || !strings.HasSuffix(x.Name(), ".json") {
			continue
		}
		v, e := s.read(strings.TrimSuffix(x.Name(), ".json"))
		if e != nil {
			return nil, e
		}
		if v.RepositoryID == repo && contains(v.Audience, actor) {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}
func (s *Store) Decide(repo, id, actor string, expected int, classification, rationale string, audience []string) (Finding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, e := s.read(id)
	if e != nil || v.RepositoryID != repo {
		return Finding{}, ErrNotFound
	}
	if v.Version != expected {
		return Finding{}, ErrConflict
	}
	if !one(classification, "confirmed", "suspected_duplicate", "false_positive", "accepted_risk", "embargoed", "failed_repair") || blank(rationale) || len(audience) == 0 {
		return Finding{}, ErrInvalid
	}
	v.Audience = dedupe(audience)
	if len(v.Audience) == 0 {
		return Finding{}, ErrInvalid
	}
	v.Version++
	v.UpdatedAt = s.now()
	v.Events = append(v.Events, Event{newID(), "classification", classification, rationale, actor, v.UpdatedAt})
	return v, s.write(v)
}
func (s *Store) LinkRepair(repo, id, actor string, expected int, r Repair) (Finding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, e := s.read(id)
	if e != nil || v.RepositoryID != repo {
		return Finding{}, ErrNotFound
	}
	if v.Version != expected {
		return Finding{}, ErrConflict
	}
	if current(v) != "confirmed" || v.Repair != nil || blank(r.ProposalID) || blank(r.TaskID) || !one(r.AssigneeType, "human", "agent") || blank(r.AssigneeID) {
		return Finding{}, ErrInvalid
	}
	r.State = "in_progress"
	r.CreatedAt = s.now()
	v.Repair = &r
	v.Version++
	v.UpdatedAt = r.CreatedAt
	v.Events = append(v.Events, Event{newID(), "repair_started", "", "Governed repair work created", actor, v.UpdatedAt})
	return v, s.write(v)
}
func (s *Store) Complete(repo, id, actor string, expected int, pull, commit, scenario string) (Finding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, e := s.read(id)
	if e != nil || v.RepositoryID != repo {
		return Finding{}, ErrNotFound
	}
	if v.Version != expected {
		return Finding{}, ErrConflict
	}
	if v.Repair == nil || v.Repair.State != "in_progress" || blank(pull) || len(commit) != 40 || blank(scenario) {
		return Finding{}, ErrInvalid
	}
	v.Repair.PullRequestID = pull
	v.Repair.RepairCommitID = commit
	v.Repair.ScenarioID = scenario
	v.Repair.State = "protected"
	v.Version++
	v.UpdatedAt = s.now()
	v.Events = append(v.Events, Event{newID(), "repair_protected", "", "Exact pull and maintained security scenario linked", actor, v.UpdatedAt})
	return v, s.write(v)
}
func Visible(v Finding, actor string) bool   { return contains(v.Audience, actor) }
func CurrentClassification(v Finding) string { return current(v) }
func valid(v Finding) bool {
	if blank(v.ThreatModelID) || v.ThreatModelVersion < 1 || blank(v.AbusePathID) || len(v.CandidateCommitID) != 40 || blank(v.Title) || blank(v.Description) || !one(v.Severity, "low", "medium", "high", "critical") || len(dedupe(v.Audience)) == 0 || len(v.Evidence) == 0 || len(v.AcceptanceCriteria) == 0 {
		return false
	}
	for _, x := range v.Evidence {
		if blank(x.ID) || blank(x.Kind) || blank(x.Summary) || (x.SHA256 != "" && len(x.SHA256) != 64) {
			return false
		}
	}
	for _, x := range v.AcceptanceCriteria {
		if blank(x) {
			return false
		}
	}
	return true
}
func current(v Finding) string {
	for i := len(v.Events) - 1; i >= 0; i-- {
		if v.Events[i].Kind == "classification" {
			return v.Events[i].Classification
		}
	}
	return "unclassified"
}
func (s *Store) read(id string) (Finding, error) {
	if len(id) != 32 {
		return Finding{}, ErrNotFound
	}
	b, e := os.ReadFile(filepath.Join(s.root, id+".json"))
	if e != nil {
		return Finding{}, ErrNotFound
	}
	var v Finding
	if json.Unmarshal(b, &v) != nil {
		return Finding{}, ErrInvalid
	}
	return v, nil
}
func (s *Store) write(v Finding) error {
	b, _ := json.MarshalIndent(v, "", "  ")
	tmp, e := os.CreateTemp(s.root, "finding-*")
	if e != nil {
		return e
	}
	defer os.Remove(tmp.Name())
	if e = tmp.Chmod(0600); e == nil {
		_, e = tmp.Write(b)
	}
	if e == nil {
		e = tmp.Sync()
	}
	if c := tmp.Close(); e == nil {
		e = c
	}
	if e == nil {
		e = os.Rename(tmp.Name(), filepath.Join(s.root, v.ID+".json"))
	}
	return e
}
func newID() string       { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func blank(v string) bool { return strings.TrimSpace(v) == "" || len(v) > 4096 }
func one(v string, x ...string) bool {
	for _, q := range x {
		if v == q {
			return true
		}
	}
	return false
}
func contains(v []string, x string) bool {
	for _, q := range v {
		if q == x {
			return true
		}
	}
	return false
}
func dedupe(v []string) []string {
	o := []string{}
	for _, x := range v {
		if !blank(x) && !contains(o, x) {
			o = append(o, x)
		}
	}
	return o
}
