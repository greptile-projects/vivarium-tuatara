// Package recoveryexercises retains redacted continuity exercise evidence.
package recoveryexercises

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
type Exercise struct {
	ID                 string       `json:"id"`
	RepositoryID       string       `json:"repository_id"`
	Name               string       `json:"name"`
	Scenario           string       `json:"scenario"`
	PlanID             string       `json:"plan_id"`
	PlanVersion        int          `json:"plan_version"`
	CommitmentID       string       `json:"commitment_id"`
	CommitmentVersion  int          `json:"commitment_version"`
	CaptureID          string       `json:"capture_id"`
	SourceRevision     string       `json:"source_revision"`
	EnvironmentID      string       `json:"environment_id"`
	Isolation          string       `json:"isolation"`
	Steps              []Step       `json:"steps"`
	Results            []StepResult `json:"results"`
	Status             string       `json:"status"`
	AchievedObjectives []string     `json:"achieved_objectives"`
	Gaps               []string     `json:"gaps"`
	ManualSteps        []string     `json:"manual_steps"`
	StartedBy          string       `json:"started_by"`
	StartedAt          time.Time    `json:"started_at"`
	FinishedAt         time.Time    `json:"finished_at"`
	DurationMS         int64        `json:"duration_ms"`
	Current            bool         `json:"current"`
	StaleReasons       []string     `json:"stale_reasons"`
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
