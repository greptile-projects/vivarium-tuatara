// Package recoveryexercises retains redacted continuity exercise evidence.
package recoveryexercises

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
)

var ErrInvalid = errors.New("invalid recovery exercise")
var ErrNotFound = errors.New("recovery exercise not found")

type Step struct {
	ID        string   `json:"id"`
	Kind      string   `json:"kind"`
	Name      string   `json:"name"`
	DependsOn []string `json:"depends_on,omitempty"`
	Command   string   `json:"command"`
	Objective string   `json:"objective"`
}
type StepResult struct {
	StepID            string    `json:"step_id"`
	Kind              string    `json:"kind"`
	Command           string    `json:"command"`
	Status            string    `json:"status"`
	StartedAt         time.Time `json:"started_at"`
	FinishedAt        time.Time `json:"finished_at"`
	DurationMS        int64     `json:"duration_ms"`
	Log               string    `json:"log"`
	Artifact          string    `json:"artifact,omitempty"`
	Gap               string    `json:"gap,omitempty"`
	Manual            bool      `json:"manual"`
	ObjectiveAchieved bool      `json:"objective_achieved"`
}
type Evidence struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Revision   string `json:"revision,omitempty"`
	Summary    string `json:"summary"`
}
type Finding struct {
	ID          string    `json:"id"`
	Statement   string    `json:"statement"`
	Uncertainty string    `json:"uncertainty"`
	Confidence  string    `json:"confidence"`
	CitationIDs []string  `json:"citation_ids"`
	CreatedBy   string    `json:"created_by"`
	ActorType   string    `json:"actor_type"`
	CreatedAt   time.Time `json:"created_at"`
}
type Investigation struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	Evidence  []Evidence `json:"evidence"`
	Findings  []Finding  `json:"findings"`
	OpenedBy  string     `json:"opened_by"`
	ActorType string     `json:"actor_type"`
	Version   int        `json:"version"`
	OpenedAt  time.Time  `json:"opened_at"`
}
type Improvement struct {
	ID                string    `json:"id"`
	InvestigationID   string    `json:"investigation_id"`
	FindingID         string    `json:"finding_id"`
	ProposalID        string    `json:"proposal_id"`
	TaskIDs           []string  `json:"task_ids"`
	BaseRevision      string    `json:"base_revision"`
	Criteria          []string  `json:"acceptance_criteria"`
	RequiredResultIDs []string  `json:"required_result_ids"`
	CreatedBy         string    `json:"created_by"`
	CreatedAt         time.Time `json:"created_at"`
	FollowUpID        string    `json:"follow_up_exercise_id,omitempty"`
	Status            string    `json:"status"`
}
type Exercise struct {
	ID                 string          `json:"id"`
	RepositoryID       string          `json:"repository_id"`
	Name               string          `json:"name"`
	Scenario           string          `json:"scenario"`
	PlanID             string          `json:"plan_id"`
	PlanVersion        int             `json:"plan_version"`
	CommitmentID       string          `json:"commitment_id"`
	CommitmentVersion  int             `json:"commitment_version"`
	CaptureID          string          `json:"capture_id"`
	SourceRevision     string          `json:"source_revision"`
	EnvironmentID      string          `json:"environment_id"`
	Isolation          string          `json:"isolation"`
	Steps              []Step          `json:"steps"`
	Results            []StepResult    `json:"results"`
	Status             string          `json:"status"`
	AchievedObjectives []string        `json:"achieved_objectives"`
	Gaps               []string        `json:"gaps"`
	ManualSteps        []string        `json:"manual_steps"`
	StartedBy          string          `json:"started_by"`
	StartedAt          time.Time       `json:"started_at"`
	FinishedAt         time.Time       `json:"finished_at"`
	DurationMS         int64           `json:"duration_ms"`
	Current            bool            `json:"current"`
	StaleReasons       []string        `json:"stale_reasons"`
	Investigations     []Investigation `json:"investigations"`
	Improvements       []Improvement   `json:"improvements"`
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
func (s *Store) Run(repo, actor string, in Exercise, execute func(Step) (string, string, string, bool)) (Exercise, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !valid(in) {
		return Exercise{}, ErrInvalid
	}
	now := s.now()
	in.ID = id()
	in.RepositoryID = repo
	in.EnvironmentID = "recovery-exercise-" + in.ID
	in.Isolation = "ephemeral-no-network-no-production-credentials"
	in.Status = "running"
	in.StartedBy = actor
	in.StartedAt = now
	in.Current = true
	in.Results = []StepResult{}
	in.Gaps = []string{}
	in.ManualSteps = []string{}
	in.AchievedObjectives = []string{}
	in.Investigations = []Investigation{}
	in.Improvements = []Improvement{}
	completed := map[string]bool{}
	for _, step := range in.Steps {
		start := s.now()
		blocked := false
		for _, d := range step.DependsOn {
			if !completed[d] {
				blocked = true
			}
		}
		status, log, artifact, manual := "blocked", "dependency not restored", "", false
		if !blocked {
			status, log, artifact, manual = execute(step)
		}
		finish := s.now()
		result := StepResult{StepID: step.ID, Kind: step.Kind, Command: step.Command, Status: status, StartedAt: start, FinishedAt: finish, DurationMS: finish.Sub(start).Milliseconds(), Log: log, Artifact: artifact, Manual: manual, ObjectiveAchieved: status == "passed"}
		if status == "passed" {
			completed[step.ID] = true
			in.AchievedObjectives = append(in.AchievedObjectives, step.Objective)
		} else {
			result.Gap = step.Name + ": " + log
			in.Gaps = append(in.Gaps, result.Gap)
		}
		if manual {
			in.ManualSteps = append(in.ManualSteps, step.Name)
		}
		in.Results = append(in.Results, result)
	}
	in.FinishedAt = s.now()
	in.DurationMS = in.FinishedAt.Sub(in.StartedAt).Milliseconds()
	in.Status = "passed"
	if len(in.Gaps) > 0 {
		in.Status = "gaps_found"
	}
	return in, s.write(in)
}
func (s *Store) Get(repo, exerciseID string) (Exercise, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read(repo, exerciseID)
}
func (s *Store) OpenInvestigation(repo, exerciseID, actor, actorType string, in Investigation) (Exercise, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	x, err := s.read(repo, exerciseID)
	if err != nil || !contains([]string{"passed", "gaps_found"}, x.Status) || !safeText(in.Title) || len(in.Evidence) == 0 || len(in.Evidence) > 32 || !contains([]string{"human", "agent"}, actorType) {
		return Exercise{}, ErrInvalid
	}
	seen := map[string]bool{}
	for i := range in.Evidence {
		e := &in.Evidence[i]
		key := e.Kind + ":" + e.ResourceID + ":" + e.Revision
		e.ID = id()
		if !contains([]string{"exercise_result", "code", "dependency", "release", "configuration", "ownership", "protection_plan", "recovery_commitment"}, e.Kind) || !safeText(e.ResourceID) || !safeText(e.Summary) || len(e.Revision) > 128 || seen[key] {
			return Exercise{}, ErrInvalid
		}
		seen[key] = true
	}
	in.ID, in.OpenedBy, in.ActorType, in.Version, in.OpenedAt, in.Findings = id(), actor, actorType, 1, s.now(), []Finding{}
	x.Investigations = append(x.Investigations, in)
	return x, s.write(x)
}
func (s *Store) AddFinding(repo, exerciseID, investigationID, actor, actorType string, expected int, in Finding) (Exercise, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	x, err := s.read(repo, exerciseID)
	if err != nil {
		return Exercise{}, err
	}
	for i := range x.Investigations {
		v := &x.Investigations[i]
		if v.ID != investigationID {
			continue
		}
		if v.Version != expected || !safeText(in.Statement) || !safeText(in.Uncertainty) || !contains([]string{"low", "medium", "high"}, in.Confidence) || len(in.CitationIDs) == 0 || len(in.CitationIDs) > 16 || !contains([]string{"human", "agent"}, actorType) {
			return Exercise{}, ErrInvalid
		}
		known := map[string]bool{}
		for _, e := range v.Evidence {
			known[e.ID] = true
		}
		for _, result := range x.Results {
			known[result.StepID] = true
		}
		for _, citation := range in.CitationIDs {
			if !known[citation] {
				return Exercise{}, ErrInvalid
			}
		}
		in.ID, in.CreatedBy, in.ActorType, in.CreatedAt = id(), actor, actorType, s.now()
		v.Findings = append(v.Findings, in)
		v.Version++
		return x, s.write(x)
	}
	return Exercise{}, ErrNotFound
}
func (s *Store) LinkImprovement(repo, exerciseID, actor string, in Improvement) (Exercise, Improvement, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	x, err := s.read(repo, exerciseID)
	if err != nil {
		return Exercise{}, Improvement{}, err
	}
	found := false
	var sourceFinding Finding
	for _, investigation := range x.Investigations {
		if investigation.ID == in.InvestigationID {
			for _, finding := range investigation.Findings {
				if finding.ID == in.FindingID {
					found = true
					sourceFinding = finding
				}
			}
		}
	}
	if !found || in.ProposalID == "" || len(in.TaskIDs) == 0 || len(in.Criteria) == 0 || len(in.Criteria) > 20 || len(in.BaseRevision) != 40 {
		return Exercise{}, Improvement{}, ErrInvalid
	}
	for _, criterion := range in.Criteria {
		if !safeText(criterion) {
			return Exercise{}, Improvement{}, ErrInvalid
		}
	}
	resultIDs := map[string]bool{}
	for _, result := range x.Results {
		resultIDs[result.StepID] = true
		if result.Status != "passed" {
			in.RequiredResultIDs = append(in.RequiredResultIDs, result.StepID)
		}
	}
	if len(in.RequiredResultIDs) == 0 {
		for _, citation := range sourceFinding.CitationIDs {
			if resultIDs[citation] {
				in.RequiredResultIDs = append(in.RequiredResultIDs, citation)
			}
		}
	}
	if len(in.RequiredResultIDs) == 0 {
		for _, result := range x.Results {
			in.RequiredResultIDs = append(in.RequiredResultIDs, result.StepID)
		}
	}
	for _, prior := range x.Improvements {
		if prior.InvestigationID == in.InvestigationID && prior.FindingID == in.FindingID {
			return x, prior, nil
		}
	}
	in.ID, in.CreatedBy, in.CreatedAt, in.Status = id(), actor, s.now(), "work_open"
	x.Improvements = append(x.Improvements, in)
	return x, in, s.write(x)
}
func (s *Store) VerifyImprovement(repo, exerciseID, improvementID, followUpID string) (Exercise, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	x, err := s.read(repo, exerciseID)
	if err != nil {
		return Exercise{}, err
	}
	follow, err := s.read(repo, followUpID)
	if err != nil {
		return Exercise{}, err
	}
	for i := range x.Improvements {
		v := &x.Improvements[i]
		if v.ID != improvementID {
			continue
		}
		if follow.ID == x.ID || follow.Status != "passed" || !follow.Current || follow.Scenario != x.Scenario || follow.PlanID != x.PlanID || follow.CommitmentID != x.CommitmentID || (follow.PlanVersion <= x.PlanVersion && follow.SourceRevision == x.SourceRevision) || !sameExerciseContract(x, follow) || !resultsPassed(follow, v.RequiredResultIDs) {
			return Exercise{}, ErrInvalid
		}
		v.FollowUpID, v.Status = follow.ID, "verified"
		return x, s.write(x)
	}
	return Exercise{}, ErrNotFound
}
func sameExerciseContract(original, follow Exercise) bool {
	if len(original.Steps) != len(follow.Steps) {
		return false
	}
	for i, step := range original.Steps {
		candidate := follow.Steps[i]
		if step.ID != candidate.ID || step.Kind != candidate.Kind || step.Command != candidate.Command || step.Objective != candidate.Objective || !slices.Equal(step.DependsOn, candidate.DependsOn) {
			return false
		}
	}
	return true
}
func resultsPassed(exercise Exercise, required []string) bool {
	passed := map[string]bool{}
	for _, result := range exercise.Results {
		passed[result.StepID] = result.Status == "passed" && result.ObjectiveAchieved
	}
	for _, id := range required {
		if !passed[id] {
			return false
		}
	}
	return len(required) > 0
}
func (s *Store) List(repo string) ([]Exercise, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	xs, e := os.ReadDir(s.root)
	if e != nil {
		return nil, e
	}
	out := []Exercise{}
	for _, x := range xs {
		if x.IsDir() || !strings.HasSuffix(x.Name(), ".json") {
			continue
		}
		var v Exercise
		b, e := os.ReadFile(filepath.Join(s.root, x.Name()))
		if e != nil || json.Unmarshal(b, &v) != nil {
			return nil, ErrInvalid
		}
		if v.RepositoryID == repo {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	return out, nil
}
func (s *Store) read(repo, exerciseID string) (Exercise, error) {
	var x Exercise
	b, err := os.ReadFile(filepath.Join(s.root, exerciseID+".json"))
	if err != nil {
		if os.IsNotExist(err) {
			return x, ErrNotFound
		}
		return x, err
	}
	if json.Unmarshal(b, &x) != nil || x.RepositoryID != repo {
		return Exercise{}, ErrNotFound
	}
	if x.Investigations == nil {
		x.Investigations = []Investigation{}
	}
	if x.Improvements == nil {
		x.Improvements = []Improvement{}
	}
	return x, nil
}
func contains(xs []string, value string) bool {
	for _, x := range xs {
		if x == value {
			return true
		}
	}
	return false
}
func valid(x Exercise) bool {
	if !safeText(x.Name) || !safeText(x.Scenario) || x.PlanID == "" || x.PlanVersion < 1 || x.CommitmentID == "" || x.CommitmentVersion < 1 || x.CaptureID == "" || x.SourceRevision == "" || len(x.Steps) == 0 || len(x.Steps) > 32 {
		return false
	}
	seen := map[string]bool{}
	for _, s := range x.Steps {
		if s.ID == "" || len(s.ID) > 64 || seen[s.ID] || !safeText(s.Name) || !safeText(s.Objective) || len(s.Command) > 128 || strings.ContainsAny(s.Command, "\r\n") || (s.Kind != "restore" && s.Kind != "integrity" && s.Kind != "journey" && s.Kind != "manual") {
			return false
		}
		seen[s.ID] = true
		for _, d := range s.DependsOn {
			if !seen[d] {
				return false
			}
		}
	}
	return true
}
func safeText(v string) bool {
	v = strings.TrimSpace(v)
	if v == "" || len(v) > 256 || strings.ContainsAny(v, "\r\n") {
		return false
	}
	lower := strings.ToLower(v)
	for _, marker := range []string{"password=", "token=", "secret=", "authorization:", "private key"} {
		if strings.Contains(lower, marker) {
			return false
		}
	}
	return true
}
func (s *Store) write(x Exercise) error {
	b, e := json.MarshalIndent(x, "", "  ")
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(s.root, ".exercise-")
	if e != nil {
		return e
	}
	name := tmp.Name()
	defer os.Remove(name)
	_ = tmp.Chmod(0600)
	if _, e = tmp.Write(b); e == nil {
		e = tmp.Sync()
	}
	closeErr := tmp.Close()
	if e == nil {
		e = closeErr
	}
	if e == nil {
		e = os.Rename(name, filepath.Join(s.root, x.ID+".json"))
	}
	return e
}
func id() string { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
