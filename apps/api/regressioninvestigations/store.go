// Package regressioninvestigations persists the agreed boundary for searching a suspected regression.
package regressioninvestigations

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

var ErrNotFound = errors.New("regression investigation not found")
var ErrInvalid = errors.New("invalid regression investigation")
var ErrConflict = errors.New("regression investigation changed")

type Reference struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Revision   string `json:"revision,omitempty"`
	Label      string `json:"label"`
}
type Boundary struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id,omitempty"`
	Revision   string `json:"revision"`
	Label      string `json:"label"`
}
type Evidence struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Revision   string `json:"revision,omitempty"`
	Label      string `json:"label"`
	Visibility string `json:"visibility"`
	Available  bool   `json:"available"`
	Stale      bool   `json:"stale"`
	Diagnostic string `json:"diagnostic,omitempty"`
}
type Entry struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	ActorID   string    `json:"actor_id"`
	Message   string    `json:"message"`
	From      string    `json:"from,omitempty"`
	To        string    `json:"to,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
type ScenarioInput struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Data   string `json:"data,omitempty"`
	Kind   string `json:"kind"`
}
type ScenarioEnvironment struct {
	Image            string            `json:"image"`
	WorkingDirectory string            `json:"working_directory"`
	SetupCommand     string            `json:"setup_command,omitempty"`
	Command          string            `json:"command"`
	Environment      map[string]string `json:"environment,omitempty"`
	TimeoutSeconds   int               `json:"timeout_seconds"`
	CPUs             float64           `json:"cpus"`
	MemoryMB         int               `json:"memory_mb"`
	StorageMB        int               `json:"storage_mb"`
}
type EnvironmentVariant struct {
	Revision    string              `json:"revision"`
	Environment ScenarioEnvironment `json:"environment"`
}
type Scenario struct {
	ID                  string               `json:"id"`
	Name                string               `json:"name"`
	ExpectedBehavior    string               `json:"expected_behavior"`
	RegressedBehavior   string               `json:"regressed_behavior"`
	Environment         ScenarioEnvironment  `json:"environment"`
	EnvironmentVariants []EnvironmentVariant `json:"environment_variants"`
	Inputs              []ScenarioInput      `json:"inputs"`
	AcceptanceCriteria  []string             `json:"acceptance_criteria"`
	CreatedBy           string               `json:"created_by"`
	CreatedAt           time.Time            `json:"created_at"`
}
type AttemptArtifact struct {
	Path        string `json:"path"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type"`
}
type AttemptRun struct {
	RunID       string            `json:"run_id,omitempty"`
	State       string            `json:"state"`
	ExitCode    *int              `json:"exit_code,omitempty"`
	Failure     string            `json:"failure,omitempty"`
	FailureKind string            `json:"failure_kind,omitempty"`
	Output      string            `json:"output,omitempty"`
	Logs        string            `json:"logs,omitempty"`
	Artifacts   []AttemptArtifact `json:"artifacts"`
	DurationMS  int64             `json:"duration_ms"`
}
type Dependency struct {
	Name          string `json:"name"`
	RepositoryID  string `json:"repository_id"`
	Revision      string `json:"revision"`
	Path          string `json:"path"`
	ArchiveSHA256 string `json:"archive_sha256,omitempty"`
}
type Attempt struct {
	ID                 string              `json:"id"`
	RequestID          string              `json:"request_id"`
	RequestDigest      string              `json:"request_digest"`
	ScenarioID         string              `json:"scenario_id"`
	TargetKind         string              `json:"target_kind"`
	TargetID           string              `json:"target_id,omitempty"`
	Revision           string              `json:"revision,omitempty"`
	Dependencies       []Dependency        `json:"dependencies"`
	Environment        ScenarioEnvironment `json:"environment"`
	Inputs             []ScenarioInput     `json:"inputs"`
	Command            string              `json:"command"`
	Repeats            int                 `json:"repeats"`
	Runs               []AttemptRun        `json:"runs"`
	Classification     string              `json:"classification"`
	State              string              `json:"state"`
	Diagnostic         string              `json:"diagnostic,omitempty"`
	CostComputeSeconds float64             `json:"cost_compute_seconds"`
	RequestedBy        string              `json:"requested_by"`
	CreatedAt          time.Time           `json:"created_at"`
	CompletedAt        time.Time           `json:"completed_at"`
}
type SearchCandidate struct {
	Kind           string   `json:"kind"`
	RepositoryID   string   `json:"repository_id"`
	Revision       string   `json:"revision"`
	Parents        []string `json:"parents"`
	Merge          bool     `json:"merge"`
	Classification string   `json:"classification"`
	AttemptIDs     []string `json:"attempt_ids"`
	Excluded       bool     `json:"excluded"`
	Exclusion      string   `json:"exclusion,omitempty"`
	Selected       bool     `json:"selected"`
	Subject        string   `json:"subject,omitempty"`
	ChangedPaths   []string `json:"changed_paths"`
	OwnerIDs       []string `json:"owner_ids"`
	PullRequestIDs []string `json:"pull_request_ids"`
}
type CulpritRange struct {
	Kind              string  `json:"kind"`
	RepositoryID      string  `json:"repository_id"`
	WorkingRevision   string  `json:"working_revision"`
	RegressedRevision string  `json:"regressed_revision"`
	Remaining         int     `json:"remaining"`
	Confidence        float64 `json:"confidence"`
	Ambiguity         string  `json:"ambiguity,omitempty"`
}
type CausalHypothesis struct {
	ID                 string    `json:"id"`
	ActorID            string    `json:"actor_id"`
	Claim              string    `json:"claim"`
	CandidateRevisions []string  `json:"candidate_revisions"`
	EvidenceIDs        []string  `json:"evidence_ids"`
	AttemptIDs         []string  `json:"attempt_ids"`
	Confidence         string    `json:"confidence"`
	CreatedAt          time.Time `json:"created_at"`
}
type SearchDecision struct {
	ID        string    `json:"id"`
	ActorID   string    `json:"actor_id"`
	Kind      string    `json:"kind"`
	Revision  string    `json:"revision,omitempty"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}
type Search struct {
	ID            string             `json:"id"`
	RequestID     string             `json:"request_id"`
	RequestDigest string             `json:"request_digest"`
	ScenarioID    string             `json:"scenario_id"`
	Version       int                `json:"version"`
	State         string             `json:"state"`
	Candidates    []SearchCandidate  `json:"candidates"`
	Ranges        []CulpritRange     `json:"culprit_ranges"`
	Hypotheses    []CausalHypothesis `json:"hypotheses"`
	Decisions     []SearchDecision   `json:"decisions"`
	CreatedBy     string             `json:"created_by"`
	CreatedAt     time.Time          `json:"created_at"`
	UpdatedAt     time.Time          `json:"updated_at"`
}
type ResponseOption struct {
	Kind               string   `json:"kind"`
	Summary            string   `json:"summary"`
	Benefits           []string `json:"benefits"`
	Risks              []string `json:"risks"`
	Constraints        []string `json:"constraints"`
	AffectedReleaseIDs []string `json:"affected_release_ids"`
	AffectedPullIDs    []string `json:"affected_pull_ids"`
	BackportTargets    []string `json:"backport_targets"`
}
type ResponseWork struct {
	Title              string   `json:"title"`
	Outcome            string   `json:"outcome"`
	AssigneeType       string   `json:"assignee_type"`
	AssigneeID         string   `json:"assignee_id"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
}
type Response struct {
	ID            string           `json:"id"`
	RequestID     string           `json:"request_id"`
	RequestDigest string           `json:"request_digest"`
	SearchID      string           `json:"search_id"`
	ScenarioID    string           `json:"scenario_id"`
	CulpritRanges []CulpritRange   `json:"culprit_ranges"`
	Options       []ResponseOption `json:"options"`
	SelectedKind  string           `json:"selected_kind,omitempty"`
	Rationale     string           `json:"rationale,omitempty"`
	Work          []ResponseWork   `json:"work,omitempty"`
	ProposalID    string           `json:"proposal_id,omitempty"`
	TaskIDs       []string         `json:"task_ids"`
	CreatedBy     string           `json:"created_by"`
	CreatedAt     time.Time        `json:"created_at"`
	PublishedAt   time.Time        `json:"published_at,omitempty"`
}

// Correction binds one exact repair or backport candidate to the historical
// scenario and the ordinary delivery evidence that must remain true.
type Correction struct {
	ID                 string    `json:"id"`
	RequestID          string    `json:"request_id"`
	RequestDigest      string    `json:"request_digest"`
	Kind               string    `json:"kind"`
	ResponseID         string    `json:"response_id"`
	TaskID             string    `json:"task_id"`
	PullRequestID      string    `json:"pull_request_id"`
	Revision           string    `json:"revision"`
	Target             string    `json:"target"`
	ScenarioID         string    `json:"scenario_id"`
	AttemptID          string    `json:"attempt_id"`
	RequiredChecks     []string  `json:"required_checks"`
	CheckRunIDs        []string  `json:"check_run_ids"`
	RequirementIDs     []string  `json:"requirement_ids"`
	AcceptanceCriteria []string  `json:"acceptance_criteria"`
	CriteriaSatisfied  []string  `json:"criteria_satisfied"`
	ProofState         string    `json:"proof_state"`
	DeliveryState      string    `json:"delivery_state"`
	ReleaseIDs         []string  `json:"release_ids"`
	DeploymentIDs      []string  `json:"deployment_ids"`
	Outcome            string    `json:"outcome,omitempty"`
	Reopened           bool      `json:"reopened"`
	ReopenReasons      []string  `json:"reopen_reasons"`
	CreatedBy          string    `json:"created_by"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}
type Investigation struct {
	ID                 string       `json:"id"`
	RequestID          string       `json:"request_id"`
	RequestDigest      string       `json:"request_digest"`
	RepositoryID       string       `json:"repository_id"`
	Version            int          `json:"version"`
	Title              string       `json:"title"`
	Source             Reference    `json:"source"`
	ExpectedBehavior   string       `json:"expected_behavior"`
	RegressedBehavior  string       `json:"regressed_behavior"`
	KnownGood          Boundary     `json:"known_good"`
	KnownBad           Boundary     `json:"known_bad"`
	Environments       []string     `json:"affected_environments"`
	Severity           string       `json:"severity"`
	OwnerIDs           []string     `json:"owner_ids"`
	AcceptanceCriteria []string     `json:"acceptance_criteria"`
	Evidence           []Evidence   `json:"evidence"`
	Diagnostics        []string     `json:"diagnostics"`
	Comparable         bool         `json:"comparable"`
	Status             string       `json:"status"`
	History            []Entry      `json:"history"`
	Scenarios          []Scenario   `json:"scenarios"`
	Attempts           []Attempt    `json:"attempts"`
	Searches           []Search     `json:"searches"`
	Responses          []Response   `json:"responses"`
	Corrections        []Correction `json:"corrections"`
	CreatedBy          string       `json:"created_by"`
	CreatedAt          time.Time    `json:"created_at"`
	UpdatedAt          time.Time    `json:"updated_at"`
}
type Store struct {
	root          string
	mu            sync.Mutex
	now           func() time.Time
	syncDirectory func(*os.File) error
}

func New(root string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, ErrInvalid
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	return &Store{root: root, now: func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }, syncDirectory: func(d *os.File) error { return d.Sync() }}, nil
}
func (s *Store) Create(v Investigation, actor string) (Investigation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, e := s.lock()
	if e != nil {
		return Investigation{}, e
	}
	defer unlock()
	if !valid(v, actor) {
		return Investigation{}, ErrInvalid
	}
	existing, found, digest, e := s.reconcile(v, actor)
	if e != nil || found {
		return existing, e
	}
	now := s.now()
	v.ID = id()
	v.RequestDigest = digest
	v.RepositoryID = strings.TrimSpace(v.RepositoryID)
	v.Version = 1
	v.Status = "open"
	v.CreatedBy = actor
	v.CreatedAt = now
	v.UpdatedAt = now
	v.OwnerIDs = uniq(v.OwnerIDs)
	v.Environments = uniq(v.Environments)
	v.AcceptanceCriteria = uniq(v.AcceptanceCriteria)
	if v.Evidence == nil {
		v.Evidence = []Evidence{}
	}
	for i := range v.Evidence {
		v.Evidence[i].ID = id()
	}
	v.History = []Entry{{ID: id(), Kind: "opened", ActorID: actor, To: "open", Message: "Search boundary agreed", CreatedAt: now}}
	v.Scenarios, v.Attempts, v.Searches, v.Responses, v.Corrections = []Scenario{}, []Attempt{}, []Search{}, []Response{}, []Correction{}
	return v, s.write(v)
}

func (s *Store) RecordCorrection(repo, wid, actor string, correction Correction, expected int) (Investigation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, e := s.lock()
	if e != nil {
		return Investigation{}, e
	}
	defer unlock()
	v, e := s.Get(repo, wid)
	if e != nil {
		return Investigation{}, e
	}
	if !token(actor) || !token(correction.RequestID) {
		return Investigation{}, ErrInvalid
	}
	correction.ID, correction.RequestDigest, correction.CreatedBy, correction.CreatedAt, correction.UpdatedAt = "", "", "", time.Time{}, time.Time{}
	correction.DeliveryState, correction.ReleaseIDs, correction.DeploymentIDs, correction.Reopened, correction.ReopenReasons = "", nil, nil, false, nil
	body, e := json.Marshal(struct {
		Actor string     `json:"actor"`
		Value Correction `json:"value"`
	}{actor, correction})
	if e != nil {
		return Investigation{}, e
	}
	digest := sha256.Sum256(body)
	encoded := hex.EncodeToString(digest[:])
	for _, retained := range v.Corrections {
		if retained.RequestID == correction.RequestID {
			if retained.RequestDigest == encoded && retained.CreatedBy == actor {
				return v, nil
			}
			return Investigation{}, ErrConflict
		}
	}
	if v.Version != expected || (correction.Kind != "repair" && correction.Kind != "backport") || len(correction.Revision) != 40 || !token(correction.PullRequestID) || !token(correction.TaskID) || strings.TrimSpace(correction.Target) == "" || correction.ProofState != "passed" {
		return Investigation{}, ErrInvalid
	}
	scenario, response, attempt := false, false, false
	for _, x := range v.Scenarios {
		scenario = scenario || x.ID == correction.ScenarioID
	}
	for _, x := range v.Responses {
		if x.ID == correction.ResponseID && x.ProposalID != "" && x.ScenarioID == correction.ScenarioID {
			for _, taskID := range x.TaskIDs {
				response = response || taskID == correction.TaskID
			}
		}
	}
	for _, x := range v.Attempts {
		attempt = attempt || (x.ID == correction.AttemptID && x.State == "completed" && x.ScenarioID == correction.ScenarioID && x.Revision == correction.Revision && x.Classification == "passed")
	}
	if !scenario || !response || !attempt || len(correction.RequiredChecks) == 0 || len(correction.RequiredChecks) != len(correction.CheckRunIDs) || len(correction.CriteriaSatisfied) != len(v.AcceptanceCriteria) || len(uniq(correction.RequiredChecks)) != len(correction.RequiredChecks) || len(uniq(correction.CheckRunIDs)) != len(correction.CheckRunIDs) {
		return Investigation{}, ErrInvalid
	}
	want := map[string]bool{}
	for _, x := range v.AcceptanceCriteria {
		want[x] = true
	}
	for _, x := range correction.CriteriaSatisfied {
		if !want[x] {
			return Investigation{}, ErrInvalid
		}
	}
	now := s.now()
	correction.ID, correction.RequestDigest, correction.CreatedBy, correction.CreatedAt, correction.UpdatedAt = id(), encoded, actor, now, now
	correction.AcceptanceCriteria = append([]string{}, v.AcceptanceCriteria...)
	correction.RequiredChecks, correction.CheckRunIDs, correction.RequirementIDs, correction.CriteriaSatisfied = uniq(correction.RequiredChecks), uniq(correction.CheckRunIDs), uniq(correction.RequirementIDs), uniq(correction.CriteriaSatisfied)
	if len(correction.RequiredChecks) != len(correction.CheckRunIDs) {
		return Investigation{}, ErrInvalid
	}
	correction.ReleaseIDs, correction.DeploymentIDs, correction.ReopenReasons = []string{}, []string{}, []string{}
	v.Corrections = append(v.Corrections, correction)
	v.History = append(v.History, Entry{ID: id(), Kind: "correction_proved", ActorID: actor, Message: "Exact " + correction.Kind + " candidate proved against retained regression evidence", CreatedAt: now})
	v.Version++
	v.UpdatedAt = now
	return v, s.write(v)
}

func (s *Store) CreateResponse(repo, wid, actor string, response Response, expected int) (Investigation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, e := s.lock()
	if e != nil {
		return Investigation{}, e
	}
	defer unlock()
	v, e := s.Get(repo, wid)
	if e != nil {
		return Investigation{}, e
	}
	body, e := json.Marshal(struct {
		Actor, SearchID, ScenarioID string
		Options                     []ResponseOption
	}{actor, response.SearchID, response.ScenarioID, response.Options})
	if e != nil {
		return Investigation{}, e
	}
	sum := sha256.Sum256(body)
	digest := hex.EncodeToString(sum[:])
	for _, retained := range v.Responses {
		if retained.RequestID == response.RequestID {
			if retained.RequestDigest != digest || retained.CreatedBy != actor {
				return Investigation{}, ErrConflict
			}
			return v, nil
		}
	}
	if v.Version != expected || !token(actor) || !token(response.RequestID) || len(response.Options) != 4 {
		return Investigation{}, ErrInvalid
	}
	var search *Search
	for i := range v.Searches {
		if v.Searches[i].ID == response.SearchID {
			search = &v.Searches[i]
		}
	}
	if search == nil || search.ScenarioID != response.ScenarioID || len(response.CulpritRanges) == 0 {
		return Investigation{}, ErrInvalid
	}
	kinds := map[string]bool{}
	for i := range response.Options {
		o := &response.Options[i]
		if (o.Kind != "revert" && o.Kind != "containment" && o.Kind != "dependency_adjustment" && o.Kind != "forward_repair") || kinds[o.Kind] || !text(o.Summary) {
			return Investigation{}, ErrInvalid
		}
		kinds[o.Kind] = true
		o.Benefits, o.Risks, o.Constraints, o.AffectedReleaseIDs, o.AffectedPullIDs, o.BackportTargets = uniq(o.Benefits), uniq(o.Risks), uniq(o.Constraints), uniq(o.AffectedReleaseIDs), uniq(o.AffectedPullIDs), uniq(o.BackportTargets)
	}
	now := s.now()
	response.ID, response.RequestDigest, response.CulpritRanges, response.TaskIDs, response.CreatedBy, response.CreatedAt = id(), digest, append([]CulpritRange(nil), response.CulpritRanges...), []string{}, actor, now
	v.Responses = append(v.Responses, response)
	v.History = append(v.History, Entry{ID: id(), Kind: "response_compared", ActorID: actor, Message: "Governed correction alternatives compared against retained evidence", CreatedAt: now})
	v.Version++
	v.UpdatedAt = now
	return v, s.write(v)
}

func (s *Store) PublishResponse(repo, wid, responseID, actor, selected, rationale, proposalID string, taskIDs []string, work []ResponseWork, expected int) (Investigation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, e := s.lock()
	if e != nil {
		return Investigation{}, e
	}
	defer unlock()
	v, e := s.Get(repo, wid)
	if e != nil {
		return Investigation{}, e
	}
	for i := range v.Responses {
		r := &v.Responses[i]
		if r.ID != responseID || r.ProposalID == "" {
			continue
		}
		retainedWork, _ := json.Marshal(r.Work)
		requestedWork, _ := json.Marshal(work)
		if r.SelectedKind == selected && r.Rationale == rationale && r.ProposalID == proposalID && strings.Join(r.TaskIDs, "\x00") == strings.Join(uniq(taskIDs), "\x00") && string(retainedWork) == string(requestedWork) {
			return v, nil
		}
		return Investigation{}, ErrConflict
	}
	if v.Version != expected || !token(actor) || !text(rationale) || !token(proposalID) || len(taskIDs) == 0 || len(work) == 0 {
		return Investigation{}, ErrInvalid
	}
	for _, item := range work {
		if !text(item.Title) || !text(item.Outcome) || (item.AssigneeType != "human" && item.AssigneeType != "agent") || !token(item.AssigneeID) || len(item.AcceptanceCriteria) == 0 {
			return Investigation{}, ErrInvalid
		}
	}
	for i := range v.Responses {
		r := &v.Responses[i]
		if r.ID != responseID {
			continue
		}
		found := false
		for _, o := range r.Options {
			if o.Kind == selected {
				found = true
			}
		}
		if !found {
			return Investigation{}, ErrInvalid
		}
		r.SelectedKind, r.Rationale, r.ProposalID, r.TaskIDs, r.Work, r.PublishedAt = selected, rationale, proposalID, uniq(taskIDs), append([]ResponseWork(nil), work...), s.now()
		v.History = append(v.History, Entry{ID: id(), Kind: "response_published", ActorID: actor, Message: "Selected correction published into ordinary governed work", CreatedAt: r.PublishedAt})
		v.Version++
		v.UpdatedAt = r.PublishedAt
		return v, s.write(v)
	}
	return Investigation{}, ErrNotFound
}

func (s *Store) CreateSearch(repo, wid, actor string, search Search, expected int) (Investigation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, e := s.lock()
	if e != nil {
		return Investigation{}, e
	}
	defer unlock()
	v, e := s.Get(repo, wid)
	if e != nil {
		return Investigation{}, e
	}
	digestBody, e := json.Marshal(struct {
		Actor      string            `json:"actor"`
		ScenarioID string            `json:"scenario_id"`
		Candidates []SearchCandidate `json:"candidates"`
	}{actor, search.ScenarioID, search.Candidates})
	if e != nil {
		return Investigation{}, e
	}
	digestSum := sha256.Sum256(digestBody)
	digest := hex.EncodeToString(digestSum[:])
	for _, retained := range v.Searches {
		if retained.RequestID == search.RequestID {
			if retained.RequestDigest != digest || retained.CreatedBy != actor {
				return Investigation{}, ErrConflict
			}
			return v, nil
		}
	}
	if v.Version != expected || !token(actor) || !token(search.RequestID) || !token(search.ScenarioID) || len(search.Candidates) < 2 {
		return Investigation{}, ErrInvalid
	}
	found := false
	for _, scenario := range v.Scenarios {
		if scenario.ID == search.ScenarioID {
			found = true
		}
	}
	if !found {
		return Investigation{}, ErrInvalid
	}
	now := s.now()
	search.ID, search.RequestDigest, search.Version, search.State, search.CreatedBy, search.CreatedAt, search.UpdatedAt = id(), digest, 1, "searching", actor, now, now
	if search.Ranges == nil {
		search.Ranges = []CulpritRange{}
	}
	search.Hypotheses, search.Decisions = []CausalHypothesis{}, []SearchDecision{}
	v.Searches = append(v.Searches, search)
	v.History = append(v.History, Entry{ID: id(), Kind: "search_scheduled", ActorID: actor, Message: "Evidence-driven commit graph search scheduled", CreatedAt: now})
	v.Version++
	v.UpdatedAt = now
	return v, s.write(v)
}

func (s *Store) GuideSearch(repo, wid, searchID, actor, kind, revision, classification, reason, claim, confidence string, evidenceIDs, attemptIDs, candidateRevisions []string, expectedInvestigation, expectedSearch int) (Investigation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, e := s.lock()
	if e != nil {
		return Investigation{}, e
	}
	defer unlock()
	v, e := s.Get(repo, wid)
	if e != nil {
		return Investigation{}, e
	}
	if v.Version != expectedInvestigation || !token(actor) || !text(reason) {
		return Investigation{}, ErrConflict
	}
	index := -1
	for i := range v.Searches {
		if v.Searches[i].ID == searchID {
			index = i
		}
	}
	if index < 0 || v.Searches[index].Version != expectedSearch {
		return Investigation{}, ErrConflict
	}
	search := &v.Searches[index]
	now := s.now()
	switch kind {
	case "classify", "exclude", "restore":
		candidate := -1
		for i := range search.Candidates {
			if search.Candidates[i].Revision == revision {
				candidate = i
			}
		}
		if candidate < 0 {
			return Investigation{}, ErrInvalid
		}
		c := &search.Candidates[candidate]
		if kind == "classify" {
			if classification != "working" && classification != "regressed" && classification != "flaky" && classification != "invalid" {
				return Investigation{}, ErrInvalid
			}
			c.Classification, c.Excluded, c.Exclusion = classification, classification == "invalid", ""
			if c.Excluded {
				c.Exclusion = reason
			}
		} else if kind == "exclude" {
			c.Excluded, c.Exclusion, c.Classification = true, reason, "invalid"
		} else {
			c.Excluded, c.Exclusion, c.Classification = false, "", ""
		}
	case "hypothesis":
		if !text(claim) || (confidence != "low" && confidence != "medium" && confidence != "high") || len(candidateRevisions) == 0 || len(evidenceIDs)+len(attemptIDs) == 0 {
			return Investigation{}, ErrInvalid
		}
		candidatesByRevision := map[string][]SearchCandidate{}
		for _, c := range search.Candidates {
			candidatesByRevision[c.Revision] = append(candidatesByRevision[c.Revision], c)
		}
		for _, revision := range candidateRevisions {
			if len(candidatesByRevision[revision]) != 1 {
				return Investigation{}, ErrInvalid
			}
		}
		validEvidence := map[string]bool{}
		attemptsByID := map[string]Attempt{}
		for _, evidence := range v.Evidence {
			validEvidence[evidence.ID] = true
		}
		for _, attempt := range v.Attempts {
			attemptsByID[attempt.ID] = attempt
		}
		for _, evidenceID := range evidenceIDs {
			if !validEvidence[evidenceID] {
				return Investigation{}, ErrInvalid
			}
		}
		for _, attemptID := range attemptIDs {
			attempt, ok := attemptsByID[attemptID]
			if !ok || attempt.ScenarioID != search.ScenarioID || !attemptSupportsCandidate(attempt, candidatesByRevision, candidateRevisions) {
				return Investigation{}, ErrInvalid
			}
		}
		search.Hypotheses = append(search.Hypotheses, CausalHypothesis{ID: id(), ActorID: actor, Claim: claim, CandidateRevisions: uniq(candidateRevisions), EvidenceIDs: uniq(evidenceIDs), AttemptIDs: uniq(attemptIDs), Confidence: confidence, CreatedAt: now})
	default:
		return Investigation{}, ErrInvalid
	}
	search.Decisions = append(search.Decisions, SearchDecision{ID: id(), ActorID: actor, Kind: kind, Revision: revision, Reason: reason, CreatedAt: now})
	search.Version++
	search.UpdatedAt = now
	v.Version++
	v.UpdatedAt = now
	return v, s.write(v)
}

func attemptSupportsCandidate(attempt Attempt, candidatesByRevision map[string][]SearchCandidate, candidateRevisions []string) bool {
	for _, candidateRevision := range candidateRevisions {
		candidates := candidatesByRevision[candidateRevision]
		if len(candidates) != 1 {
			continue
		}
		candidate := candidates[0]
		if candidate.Kind == "commit" && attempt.Revision == candidate.Revision {
			return true
		}
		if candidate.Kind == "dependency" {
			for _, dependency := range attempt.Dependencies {
				if dependency.RepositoryID == candidate.RepositoryID && dependency.Revision == candidate.Revision {
					return true
				}
			}
		}
	}
	return false
}

// Reconcile returns an already-published create before callers re-evaluate
// mutable repository facts. Current access authorization remains the route's
// responsibility; the stable actor and exact payload digest prevent takeover.
func (s *Store) Reconcile(v Investigation, actor string) (Investigation, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, e := s.lock()
	if e != nil {
		return Investigation{}, false, e
	}
	defer unlock()
	existing, found, _, e := s.reconcile(v, actor)
	return existing, found, e
}
func (s *Store) reconcile(v Investigation, actor string) (Investigation, bool, string, error) {
	if !token(v.RepositoryID) || !token(v.RequestID) || !token(actor) {
		return Investigation{}, false, "", ErrInvalid
	}
	digest, e := requestDigest(v, actor)
	if e != nil {
		return Investigation{}, false, "", e
	}
	existing, e := s.list(v.RepositoryID)
	if e != nil {
		return Investigation{}, false, "", e
	}
	for _, item := range existing {
		if item.RequestID != v.RequestID {
			continue
		}
		if item.RequestDigest != digest {
			return Investigation{}, false, "", ErrConflict
		}
		return item, true, digest, nil
	}
	return Investigation{}, false, digest, nil
}
func (s *Store) Get(repo, wid string) (Investigation, error) {
	if !token(repo) || !token(wid) {
		return Investigation{}, ErrNotFound
	}
	b, e := os.ReadFile(filepath.Join(s.root, repo, wid+".json"))
	if errors.Is(e, os.ErrNotExist) {
		return Investigation{}, ErrNotFound
	}
	var v Investigation
	if e == nil {
		e = json.Unmarshal(b, &v)
	}
	return v, e
}
func (s *Store) List(repo string) ([]Investigation, error) {
	if !token(repo) {
		return nil, ErrInvalid
	}
	return s.list(repo)
}
func (s *Store) list(repo string) ([]Investigation, error) {
	files, e := os.ReadDir(filepath.Join(s.root, repo))
	if errors.Is(e, os.ErrNotExist) {
		return []Investigation{}, nil
	}
	if e != nil {
		return nil, e
	}
	out := []Investigation{}
	for _, f := range files {
		if filepath.Ext(f.Name()) != ".json" {
			continue
		}
		v, x := s.Get(repo, strings.TrimSuffix(f.Name(), ".json"))
		if x != nil {
			return nil, x
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}
func (s *Store) Append(repo, wid, actor, kind, message, value string, expected int) (Investigation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, e := s.lock()
	if e != nil {
		return Investigation{}, e
	}
	defer unlock()
	v, e := s.Get(repo, wid)
	if e != nil {
		return Investigation{}, e
	}
	if v.Version != expected {
		return Investigation{}, ErrConflict
	}
	message = strings.TrimSpace(message)
	if len(message) < 1 || len(message) > 4000 || sensitive(message) {
		return Investigation{}, ErrInvalid
	}
	now := s.now()
	entry := Entry{ID: id(), Kind: kind, ActorID: actor, Message: message, CreatedAt: now}
	switch kind {
	case "discussion", "hypothesis":
	case "scope_change":
		entry.From = strings.Join(v.Environments, ", ")
		values := split(value)
		if len(values) == 0 {
			return Investigation{}, ErrInvalid
		}
		v.Environments = values
		entry.To = strings.Join(values, ", ")
	case "status_change":
		if value != "open" && value != "bounded" && value != "paused" && value != "closed" {
			return Investigation{}, ErrInvalid
		}
		entry.From = v.Status
		entry.To = value
		v.Status = value
	default:
		return Investigation{}, ErrInvalid
	}
	v.History = append(v.History, entry)
	v.Version++
	v.UpdatedAt = now
	return v, s.write(v)
}
func (s *Store) AddScenario(repo, wid, actor string, scenario Scenario, expected int) (Investigation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, e := s.lock()
	if e != nil {
		return Investigation{}, e
	}
	defer unlock()
	v, e := s.Get(repo, wid)
	if e != nil {
		return Investigation{}, e
	}
	if v.Version != expected {
		return Investigation{}, ErrConflict
	}
	if strings.TrimSpace(scenario.ExpectedBehavior) == "" {
		scenario.ExpectedBehavior = v.ExpectedBehavior
	}
	if strings.TrimSpace(scenario.RegressedBehavior) == "" {
		scenario.RegressedBehavior = v.RegressedBehavior
	}
	if len(scenario.AcceptanceCriteria) == 0 {
		scenario.AcceptanceCriteria = append([]string{}, v.AcceptanceCriteria...)
	}
	if !text(scenario.Name) || !text(scenario.ExpectedBehavior) || !text(scenario.RegressedBehavior) || !scenarioEnvironment(scenario.Environment) || len(scenario.AcceptanceCriteria) == 0 || len(scenario.Inputs) > 20 {
		return Investigation{}, ErrInvalid
	}
	for _, input := range scenario.Inputs {
		if !token(input.Name) || len(input.SHA256) != 64 || (input.Kind != "synthetic" && input.Kind != "privacy_preserving" && input.Kind != "unsafe") {
			return Investigation{}, ErrInvalid
		}
	}
	seenVariants := map[string]bool{}
	for _, variant := range scenario.EnvironmentVariants {
		if len(variant.Revision) != 40 || seenVariants[variant.Revision] || !scenarioEnvironment(variant.Environment) {
			return Investigation{}, ErrInvalid
		}
		seenVariants[variant.Revision] = true
	}
	now := s.now()
	scenario.ID, scenario.CreatedBy, scenario.CreatedAt = id(), actor, now
	scenario.AcceptanceCriteria = uniq(scenario.AcceptanceCriteria)
	if scenario.Inputs == nil {
		scenario.Inputs = []ScenarioInput{}
	}
	if scenario.EnvironmentVariants == nil {
		scenario.EnvironmentVariants = []EnvironmentVariant{}
	}
	v.Scenarios = append(v.Scenarios, scenario)
	v.History = append(v.History, Entry{ID: id(), Kind: "scenario_defined", ActorID: actor, Message: "Bounded historical comparison scenario defined: " + scenario.Name, CreatedAt: now})
	v.Version++
	v.UpdatedAt = now
	return v, s.write(v)
}
func (s *Store) RecordAttempt(repo, wid, actor string, attempt Attempt, expected int) (Investigation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, e := s.lock()
	if e != nil {
		return Investigation{}, e
	}
	defer unlock()
	v, e := s.Get(repo, wid)
	if e != nil {
		return Investigation{}, e
	}
	if v.Version != expected {
		return Investigation{}, ErrConflict
	}
	found := false
	for _, scenario := range v.Scenarios {
		if scenario.ID == attempt.ScenarioID {
			found = true
			break
		}
	}
	if !found || !attemptClassification(attempt.Classification) || !token(actor) {
		return Investigation{}, ErrInvalid
	}
	attempt.ID, attempt.RequestedBy = id(), actor
	if attempt.CreatedAt.IsZero() {
		attempt.CreatedAt = s.now()
	}
	if attempt.CompletedAt.IsZero() {
		attempt.CompletedAt = s.now()
	}
	if attempt.Runs == nil {
		attempt.Runs = []AttemptRun{}
	}
	if attempt.Dependencies == nil {
		attempt.Dependencies = []Dependency{}
	}
	v.Attempts = append(v.Attempts, attempt)
	v.History = append(v.History, Entry{ID: id(), Kind: "scenario_attempt", ActorID: actor, Message: "Historical comparison classified as " + attempt.Classification, CreatedAt: attempt.CompletedAt})
	v.Version++
	v.UpdatedAt = attempt.CompletedAt
	return v, s.write(v)
}

func (s *Store) ReserveAttempt(repo, wid, actor string, attempt Attempt, expected int) (Investigation, Attempt, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, e := s.lock()
	if e != nil {
		return Investigation{}, Attempt{}, false, e
	}
	defer unlock()
	v, e := s.Get(repo, wid)
	if e != nil {
		return Investigation{}, Attempt{}, false, e
	}
	if !token(attempt.RequestID) || !token(actor) {
		return Investigation{}, Attempt{}, false, ErrInvalid
	}
	digestBody, e := json.Marshal(struct {
		Actor   string  `json:"actor"`
		Attempt Attempt `json:"attempt"`
	}{actor, attempt})
	if e != nil {
		return Investigation{}, Attempt{}, false, e
	}
	digest := sha256.Sum256(digestBody)
	encoded := hex.EncodeToString(digest[:])
	for _, retained := range v.Attempts {
		if retained.RequestID != attempt.RequestID {
			continue
		}
		if retained.RequestDigest != encoded || retained.RequestedBy != actor {
			return Investigation{}, Attempt{}, false, ErrConflict
		}
		return v, retained, true, nil
	}
	if v.Version != expected {
		return Investigation{}, Attempt{}, false, ErrConflict
	}
	found := false
	for _, scenario := range v.Scenarios {
		if scenario.ID == attempt.ScenarioID {
			found = true
			break
		}
	}
	if !found {
		return Investigation{}, Attempt{}, false, ErrInvalid
	}
	now := s.now()
	attempt.ID, attempt.RequestDigest, attempt.RequestedBy, attempt.State = id(), encoded, actor, "running"
	attempt.CreatedAt = now
	attempt.Runs = []AttemptRun{}
	if attempt.Dependencies == nil {
		attempt.Dependencies = []Dependency{}
	}
	v.Attempts = append(v.Attempts, attempt)
	v.History = append(v.History, Entry{ID: id(), Kind: "scenario_attempt_reserved", ActorID: actor, Message: "Historical comparison execution reserved", CreatedAt: now})
	v.Version++
	v.UpdatedAt = now
	return v, attempt, false, s.write(v)
}

func (s *Store) FinalizeAttempt(repo, wid, actor, attemptID string, completed Attempt) (Investigation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, e := s.lock()
	if e != nil {
		return Investigation{}, e
	}
	defer unlock()
	v, e := s.Get(repo, wid)
	if e != nil {
		return Investigation{}, e
	}
	index := -1
	for i := range v.Attempts {
		if v.Attempts[i].ID == attemptID {
			index = i
			break
		}
	}
	if index < 0 || v.Attempts[index].RequestedBy != actor || v.Attempts[index].State != "running" || !attemptClassification(completed.Classification) {
		return Investigation{}, ErrConflict
	}
	reserved := v.Attempts[index]
	completed.ID, completed.RequestID, completed.RequestDigest, completed.RequestedBy, completed.CreatedAt = reserved.ID, reserved.RequestID, reserved.RequestDigest, reserved.RequestedBy, reserved.CreatedAt
	completed.State = "completed"
	if completed.CompletedAt.IsZero() {
		completed.CompletedAt = s.now()
	}
	v.Attempts[index] = completed
	v.History = append(v.History, Entry{ID: id(), Kind: "scenario_attempt", ActorID: actor, Message: "Historical comparison classified as " + completed.Classification, CreatedAt: completed.CompletedAt})
	v.Version++
	v.UpdatedAt = completed.CompletedAt
	return v, s.write(v)
}
func valid(v Investigation, actor string) bool {
	if !(token(v.RepositoryID) && token(v.RequestID) && token(actor) && strings.TrimSpace(v.Title) != "" && len(v.Title) <= 200 && text(v.ExpectedBehavior) && text(v.RegressedBehavior) && boundary(v.KnownGood) && boundary(v.KnownBad) && len(v.Environments) > 0 && len(v.OwnerIDs) > 0 && len(v.AcceptanceCriteria) > 0 && (v.Severity == "low" || v.Severity == "medium" || v.Severity == "high" || v.Severity == "critical") && v.Source.ResourceID != "") {
		return false
	}
	for _, evidence := range v.Evidence {
		if strings.TrimSpace(evidence.Kind) == "" || strings.TrimSpace(evidence.ResourceID) == "" || !text(evidence.Label) {
			return false
		}
	}
	return true
}
func requestDigest(v Investigation, actor string) (string, error) {
	v.ID, v.RequestDigest, v.CreatedBy, v.Status = "", "", "", ""
	v.Version, v.CreatedAt, v.UpdatedAt, v.History = 0, time.Time{}, time.Time{}, nil
	v.Scenarios, v.Attempts, v.Searches, v.Responses, v.Corrections = nil, nil, nil, nil, nil
	v.Diagnostics, v.Comparable = nil, false
	for i := range v.Evidence {
		v.Evidence[i].ID = ""
		v.Evidence[i].Available, v.Evidence[i].Stale, v.Evidence[i].Diagnostic = false, false, ""
	}
	b, e := json.Marshal(struct {
		Actor         string        `json:"actor"`
		Investigation Investigation `json:"investigation"`
	}{actor, v})
	if e != nil {
		return "", e
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
func scenarioEnvironment(v ScenarioEnvironment) bool {
	if !text(v.Image) || strings.ContainsAny(v.Image, " \t\r\n@") || !text(v.Command) || (v.SetupCommand != "" && !text(v.SetupCommand)) || len(v.WorkingDirectory) > 300 || v.TimeoutSeconds < 1 || v.TimeoutSeconds > 3600 || v.CPUs <= 0 || v.CPUs > 8 || v.MemoryMB < 32 || v.MemoryMB > 16384 || v.StorageMB < 1 || v.StorageMB > 4096 {
		return false
	}
	for key, value := range v.Environment {
		if !environmentName(key) || len(value) > 4000 || sensitive(value) {
			return false
		}
	}
	return true
}
func environmentName(v string) bool {
	if v == "" || len(v) > 100 {
		return false
	}
	for i, r := range v {
		if !(r == '_' || r >= 'A' && r <= 'Z' || i > 0 && r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}
func attemptClassification(v string) bool {
	switch v {
	case "passed", "failed", "flaky", "incompatible_setup", "missing_dependencies", "unsafe_fixture", "untestable_revision":
		return true
	}
	return false
}
func boundary(v Boundary) bool {
	return (v.Kind == "commit" || v.Kind == "release") && len(v.Revision) == 40 && v.Label != ""
}
func text(v string) bool {
	v = strings.TrimSpace(v)
	return v != "" && len(v) <= 10000 && !sensitive(v)
}
func sensitive(v string) bool {
	l := strings.ToLower(v)
	return strings.Contains(l, "-----begin") || strings.Contains(l, "api_key=") || strings.Contains(l, "password=") || strings.Contains(l, "token=")
}
func token(v string) bool {
	if len(v) < 1 || len(v) > 200 {
		return false
	}
	for _, r := range v {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}
func uniq(v []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, x := range v {
		x = strings.TrimSpace(x)
		if x != "" && !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}
func split(v string) []string { return uniq(strings.Split(v, ",")) }
func id() string              { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func (s *Store) write(v Investigation) error {
	dir := filepath.Join(s.root, v.RepositoryID)
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(dir, ".investigation-")
	if e != nil {
		return e
	}
	n := f.Name()
	defer os.Remove(n)
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
		e = os.Rename(n, filepath.Join(dir, v.ID+".json"))
	}
	if e != nil {
		return e
	}
	d, e := os.Open(dir)
	if e != nil {
		return e
	}
	e = s.syncDirectory(d)
	ce = d.Close()
	if e == nil {
		e = ce
	}
	return e
}
func (s *Store) lock() (func(), error) {
	f, e := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); e != nil {
		f.Close()
		return nil, e
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}
