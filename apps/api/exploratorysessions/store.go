// Package exploratorysessions retains bounded, revision-exact collaborative exploration.
package exploratorysessions

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

var ErrNotFound = errors.New("exploratory session not found")
var ErrInvalid = errors.New("invalid exploratory session")
var ErrConflict = errors.New("exploratory session version conflict")
var ErrFindingNotConfirmed = errors.New("exploratory finding is not a confirmed bug")

type Source struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Revision   string `json:"revision"`
	Label      string `json:"label"`
}
type Limits struct {
	ExpiresAt       time.Time `json:"expires_at"`
	MaxCostCents    int       `json:"max_cost_cents"`
	MaxAgentActions int       `json:"max_agent_actions"`
	AllowedActions  []string  `json:"allowed_actions"`
	TestData        []string  `json:"test_data"`
}
type Charter struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	Risk           string   `json:"risk"`
	Mission        string   `json:"mission"`
	AssigneeType   string   `json:"assignee_type"`
	AssigneeID     string   `json:"assignee_id"`
	AllowedActions []string `json:"allowed_actions"`
	Coverage       []string `json:"coverage"`
	Uncertainty    string   `json:"uncertainty"`
}
type Artifact struct {
	Kind        string `json:"kind"`
	SHA256      string `json:"sha256"`
	MediaType   string `json:"media_type"`
	Description string `json:"description"`
}
type Event struct {
	ID                string     `json:"id"`
	Version           int        `json:"version"`
	Kind              string     `json:"kind"`
	CharterID         string     `json:"charter_id,omitempty"`
	FindingID         string     `json:"finding_id,omitempty"`
	Summary           string     `json:"summary"`
	Route             string     `json:"route,omitempty"`
	Inputs            []string   `json:"inputs,omitempty"`
	Command           string     `json:"command,omitempty"`
	Coverage          []string   `json:"coverage,omitempty"`
	Uncertainty       string     `json:"uncertainty,omitempty"`
	Classification    string     `json:"classification,omitempty"`
	Artifacts         []Artifact `json:"artifacts,omitempty"`
	ReproducesEventID string     `json:"reproduces_event_id,omitempty"`
	ActorType         string     `json:"actor_type"`
	ActorID           string     `json:"actor_id"`
	CreatedAt         time.Time  `json:"created_at"`
}
type Repair struct {
	FindingID          string    `json:"finding_id"`
	RecoveryID         string    `json:"recovery_id"`
	State              string    `json:"state"`
	AffectedRevision   string    `json:"affected_revision"`
	EvidenceEventIDs   []string  `json:"evidence_event_ids"`
	ReproductionID     string    `json:"reproduction_event_id"`
	AcceptanceCriteria []string  `json:"acceptance_criteria"`
	AssigneeType       string    `json:"assignee_type"`
	AssigneeID         string    `json:"assignee_id"`
	QualityPlanID      string    `json:"quality_plan_id,omitempty"`
	QualityPlanVersion int       `json:"quality_plan_version,omitempty"`
	RequirementIDs     []string  `json:"requirement_ids,omitempty"`
	IssueID            string    `json:"issue_id,omitempty"`
	ProposalID         string    `json:"proposal_id,omitempty"`
	TaskID             string    `json:"task_id,omitempty"`
	ScenarioID         string    `json:"scenario_id,omitempty"`
	PullRequestID      string    `json:"pull_request_id,omitempty"`
	RegressionCommitID string    `json:"regression_commit_id,omitempty"`
	CreatedBy          string    `json:"created_by"`
	CreatedAt          time.Time `json:"created_at"`
}
type Session struct {
	ID           string    `json:"id"`
	RepositoryID string    `json:"repository_id"`
	Title        string    `json:"title"`
	Source       Source    `json:"source"`
	Access       []string  `json:"access"`
	Limits       Limits    `json:"limits"`
	Charters     []Charter `json:"charters"`
	Status       string    `json:"status"`
	Version      int       `json:"version"`
	Events       []Event   `json:"events"`
	Repairs      []Repair  `json:"repairs,omitempty"`
	CreatedBy    string    `json:"created_by"`
	CreatedAt    time.Time `json:"created_at"`
	Stale        bool      `json:"stale"`
	StaleReason  string    `json:"stale_reason,omitempty"`
}
type RepairInput struct {
	ExpectedVersion     int      `json:"expected_version"`
	FindingID           string   `json:"finding_id"`
	EvidenceEventIDs    []string `json:"evidence_event_ids"`
	ReproductionEventID string   `json:"reproduction_event_id"`
	AcceptanceCriteria  []string `json:"acceptance_criteria"`
	AssigneeType        string   `json:"assignee_type"`
	AssigneeID          string   `json:"assignee_id"`
	QualityPlanID       string   `json:"quality_plan_id,omitempty"`
	QualityPlanVersion  int      `json:"quality_plan_version,omitempty"`
	RequirementIDs      []string `json:"requirement_ids,omitempty"`
}
type EventInput struct {
	ExpectedVersion   int        `json:"expected_version"`
	Kind              string     `json:"kind"`
	CharterID         string     `json:"charter_id,omitempty"`
	FindingID         string     `json:"finding_id,omitempty"`
	Summary           string     `json:"summary"`
	Route             string     `json:"route,omitempty"`
	Inputs            []string   `json:"inputs,omitempty"`
	Command           string     `json:"command,omitempty"`
	Coverage          []string   `json:"coverage,omitempty"`
	Uncertainty       string     `json:"uncertainty,omitempty"`
	Classification    string     `json:"classification,omitempty"`
	Artifacts         []Artifact `json:"artifacts,omitempty"`
	ReproducesEventID string     `json:"reproduces_event_id,omitempty"`
	ActorType         string     `json:"actor_type,omitempty"`
	ActorID           string     `json:"actor_id,omitempty"`
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
func (s *Store) Create(repo, actor string, v Session) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !ValidSession(v, s.now()) {
		return Session{}, ErrInvalid
	}
	v.ID = newID()
	v.RepositoryID = repo
	v.CreatedBy = actor
	v.CreatedAt = s.now()
	v.Status = "active"
	v.Version = 1
	v.Events = []Event{}
	v.Stale = false
	v.StaleReason = ""
	if e := s.write(v); e != nil {
		return Session{}, e
	}
	return v, nil
}
func (s *Store) Get(id string) (Session, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.read(id) }
func (s *Store) List(repo string) ([]Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	xs, e := os.ReadDir(s.root)
	if e != nil {
		return nil, e
	}
	out := []Session{}
	for _, x := range xs {
		if x.IsDir() || !strings.HasSuffix(x.Name(), ".json") {
			continue
		}
		v, e := s.read(strings.TrimSuffix(x.Name(), ".json"))
		if e != nil {
			return nil, e
		}
		if v.RepositoryID == repo {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}
func (s *Store) Append(id, actor string, in EventInput) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if in.ActorType == "agent" {
		if actor != in.ActorID {
			return Session{}, ErrInvalid
		}
	} else {
		in.ActorType = "human"
		in.ActorID = actor
	}
	v, e := s.read(id)
	if e != nil {
		return Session{}, e
	}
	if in.ExpectedVersion != v.Version {
		return Session{}, ErrConflict
	}
	if !ValidEvent(v, in) {
		return Session{}, ErrInvalid
	}
	actorType := "human"
	actorID := actor
	if in.ActorType == "agent" {
		actorType = "agent"
		actorID = in.ActorID
	}
	now := s.now()
	if now.After(v.Limits.ExpiresAt) {
		return Session{}, ErrInvalid
	}
	ev := Event{ID: newID(), Version: v.Version + 1, Kind: in.Kind, CharterID: in.CharterID, FindingID: in.FindingID, Summary: in.Summary, Route: in.Route, Inputs: in.Inputs, Command: in.Command, Coverage: in.Coverage, Uncertainty: in.Uncertainty, Classification: in.Classification, Artifacts: in.Artifacts, ReproducesEventID: in.ReproducesEventID, ActorType: actorType, ActorID: actorID, CreatedAt: now}
	v.Version++
	v.Events = append(v.Events, ev)
	switch in.Kind {
	case "pause":
		v.Status = "paused"
	case "resume":
		v.Status = "active"
	case "close":
		v.Status = "closed"
	}
	if e = s.write(v); e != nil {
		return Session{}, e
	}
	return v, nil
}

// ReserveRepair freezes the exact confirmed finding handoff before any
// ordinary issue or task is created. Exact retries return the same recovery
// identity, allowing cross-store publication to reconcile without duplicates.
func (s *Store) ReserveRepair(id, actor string, in RepairInput) (Session, Repair, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.read(id)
	if err != nil {
		return Session{}, Repair{}, err
	}
	for _, repair := range v.Repairs {
		if repair.FindingID == in.FindingID {
			if sameRepairRequest(repair, in) {
				return v, repair, nil
			}
			return Session{}, Repair{}, ErrConflict
		}
	}
	if in.ExpectedVersion != v.Version {
		return Session{}, Repair{}, ErrConflict
	}
	if v.Status == "closed" || s.now().After(v.Limits.ExpiresAt) || !validRepair(v, in) {
		return Session{}, Repair{}, ErrFindingNotConfirmed
	}
	repair := Repair{FindingID: in.FindingID, RecoveryID: newID(), State: "pending", AffectedRevision: v.Source.Revision, EvidenceEventIDs: append([]string(nil), in.EvidenceEventIDs...), ReproductionID: in.ReproductionEventID, AcceptanceCriteria: append([]string(nil), in.AcceptanceCriteria...), AssigneeType: in.AssigneeType, AssigneeID: strings.TrimSpace(in.AssigneeID), QualityPlanID: in.QualityPlanID, QualityPlanVersion: in.QualityPlanVersion, RequirementIDs: append([]string(nil), in.RequirementIDs...), CreatedBy: actor, CreatedAt: s.now()}
	v.Repairs = append(v.Repairs, repair)
	v.Version++
	if err = s.write(v); err != nil {
		return Session{}, Repair{}, err
	}
	return v, repair, nil
}

func (s *Store) FinalizeRepair(id, recoveryID, issueID, proposalID, taskID string) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.read(id)
	if err != nil {
		return Session{}, err
	}
	for i := range v.Repairs {
		if v.Repairs[i].RecoveryID != recoveryID {
			continue
		}
		if v.Repairs[i].State == "linked" {
			if v.Repairs[i].IssueID == issueID && v.Repairs[i].ProposalID == proposalID && v.Repairs[i].TaskID == taskID {
				return v, nil
			}
			return Session{}, ErrConflict
		}
		v.Repairs[i].State, v.Repairs[i].IssueID, v.Repairs[i].ProposalID, v.Repairs[i].TaskID = "linked", issueID, proposalID, taskID
		v.Version++
		if err = s.write(v); err != nil {
			return Session{}, err
		}
		return v, nil
	}
	return Session{}, ErrNotFound
}

func (s *Store) LinkCoverage(id, findingID, scenarioID, pullID, commitID string) (Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.read(id)
	if err != nil {
		return Session{}, err
	}
	for i := range v.Repairs {
		repair := &v.Repairs[i]
		if repair.FindingID != findingID {
			continue
		}
		if repair.State != "linked" {
			return Session{}, ErrInvalid
		}
		if repair.ScenarioID != "" {
			if repair.ScenarioID == scenarioID && repair.PullRequestID == pullID && repair.RegressionCommitID == commitID {
				return v, nil
			}
			return Session{}, ErrConflict
		}
		repair.ScenarioID, repair.PullRequestID, repair.RegressionCommitID = scenarioID, pullID, commitID
		v.Version++
		if err = s.write(v); err != nil {
			return Session{}, err
		}
		return v, nil
	}
	return Session{}, ErrNotFound
}

func validRepair(v Session, in RepairInput) bool {
	if bad(in.FindingID) || len(in.EvidenceEventIDs) == 0 || len(in.EvidenceEventIDs) > 20 || bad(in.ReproductionEventID) || len(in.AcceptanceCriteria) == 0 || len(in.AcceptanceCriteria) > 20 || !one(in.AssigneeType, "human", "agent") || bad(in.AssigneeID) {
		return false
	}
	classified, reproduced := false, false
	events := map[string]Event{}
	for _, event := range v.Events {
		events[event.ID] = event
		classified = classified || event.Kind == "classify" && event.FindingID == in.FindingID && event.Classification == "bug"
		reproduced = reproduced || event.ID == in.ReproductionEventID && event.Kind == "reproduce" && event.FindingID == in.FindingID
	}
	if !classified || !reproduced {
		return false
	}
	seen := map[string]bool{}
	for _, id := range in.EvidenceEventIDs {
		event, ok := events[id]
		if !ok || seen[id] || event.FindingID != in.FindingID || !one(event.Kind, "observation", "reproduce") {
			return false
		}
		seen[id] = true
	}
	for _, criterion := range in.AcceptanceCriteria {
		if strings.TrimSpace(criterion) == "" || len(criterion) > 1000 {
			return false
		}
	}
	if (in.QualityPlanID == "") != (in.QualityPlanVersion == 0) || (in.QualityPlanID == "" && len(in.RequirementIDs) > 0) {
		return false
	}
	return true
}

func sameRepairRequest(r Repair, in RepairInput) bool {
	return r.FindingID == in.FindingID && r.ReproductionID == in.ReproductionEventID && r.AssigneeType == in.AssigneeType && r.AssigneeID == strings.TrimSpace(in.AssigneeID) && r.QualityPlanID == in.QualityPlanID && r.QualityPlanVersion == in.QualityPlanVersion && slicesEqual(r.EvidenceEventIDs, in.EvidenceEventIDs) && slicesEqual(r.AcceptanceCriteria, in.AcceptanceCriteria) && slicesEqual(r.RequirementIDs, in.RequirementIDs)
}
func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func ValidSession(v Session, now time.Time) bool {
	if bad(v.Title) || bad(v.Source.ResourceID) || bad(v.Source.Label) || len(v.Source.Revision) != 40 || !hexOnly(v.Source.Revision) || !one(v.Source.Kind, "pull_preview", "release_candidate", "issue", "quality_plan") || len(v.Access) == 0 || len(v.Charters) == 0 || !v.Limits.ExpiresAt.After(now) || v.Limits.ExpiresAt.After(now.Add(24*time.Hour)) || v.Limits.MaxCostCents < 0 || v.Limits.MaxCostCents > 100000 || v.Limits.MaxAgentActions < 0 || v.Limits.MaxAgentActions > 1000 || len(v.Limits.AllowedActions) == 0 || len(v.Limits.TestData) == 0 {
		return false
	}
	allowed := map[string]bool{}
	for _, a := range v.Limits.AllowedActions {
		if !one(a, "navigate", "input", "screenshot", "trace", "command", "observe", "guide", "pause", "resume", "reproduce", "classify", "discard", "close") {
			return false
		}
		allowed[a] = true
	}
	for _, d := range v.Limits.TestData {
		if !one(d, "synthetic", "anonymized", "public") {
			return false
		}
	}
	seen := map[string]bool{}
	for _, c := range v.Charters {
		if bad(c.ID) || bad(c.Title) || bad(c.Risk) || bad(c.Mission) || bad(c.AssigneeID) || seen[c.ID] || !one(c.AssigneeType, "human", "agent") || len(c.AllowedActions) == 0 || len(c.Coverage) == 0 || bad(c.Uncertainty) {
			return false
		}
		seen[c.ID] = true
		for _, a := range c.AllowedActions {
			if !allowed[a] {
				return false
			}
		}
	}
	return true
}
func ValidEvent(v Session, in EventInput) bool {
	if in.ExpectedVersion < 1 || bad(in.Summary) || !one(in.Kind, "observation", "guide", "pause", "resume", "reproduce", "classify", "discard", "close") {
		return false
	}
	if !one(in.Kind, "classify", "discard") && in.Classification != "" {
		return false
	}
	if v.Status == "closed" {
		return false
	}
	if v.Status == "paused" && !one(in.Kind, "resume", "guide", "close") {
		return false
	}
	if in.Kind == "resume" && v.Status != "paused" {
		return false
	}
	if in.Kind == "pause" && v.Status != "active" {
		return false
	}
	var charter *Charter
	if in.CharterID != "" {
		for i := range v.Charters {
			if v.Charters[i].ID == in.CharterID {
				charter = &v.Charters[i]
			}
		}
		if charter == nil {
			return false
		}
	}
	if in.ActorType == "agent" {
		if charter == nil || charter.AssigneeType != "agent" || charter.AssigneeID != in.ActorID {
			return false
		}
		n := 0
		for _, e := range v.Events {
			if e.ActorType == "agent" {
				n++
			}
		}
		if n >= v.Limits.MaxAgentActions {
			return false
		}
	} else if in.ActorType != "human" {
		return false
	}
	if in.Kind != "guide" {
		if charter == nil || charter.AssigneeType != in.ActorType || charter.AssigneeID != in.ActorID {
			return false
		}
	}
	required := []string{in.Kind}
	if in.Route != "" {
		required = append(required, "navigate")
	}
	if len(in.Inputs) > 0 {
		required = append(required, "input")
	}
	if in.Command != "" {
		required = append(required, "command")
	}
	if in.Kind == "observation" {
		required[0] = "observe"
	}
	for _, artifact := range in.Artifacts {
		if artifact.Kind == "screenshot" {
			required = append(required, "screenshot")
		}
		if artifact.Kind == "trace" || artifact.Kind == "recording" || artifact.Kind == "log" || artifact.Kind == "command_output" {
			required = append(required, "trace")
		}
	}
	for _, action := range required {
		if !contains(v.Limits.AllowedActions, action) || ((in.ActorType == "agent" || in.Kind != "guide") && !contains(charter.AllowedActions, action)) {
			return false
		}
	}
	if one(in.Kind, "classify", "discard") && (bad(in.FindingID) || !one(in.Classification, "bug", "risk", "question", "expected", "duplicate", "flaky", "environment_specific", "not_reproducible", "discarded")) {
		return false
	}
	if one(in.Kind, "classify", "discard") && !findingExists(v, in.FindingID) {
		return false
	}
	if in.Kind == "reproduce" {
		if bad(in.ReproducesEventID) || bad(in.FindingID) {
			return false
		}
		referenced, ok := eventByID(v, in.ReproducesEventID)
		if !ok || !one(referenced.Kind, "observation", "reproduce") || referenced.FindingID != in.FindingID {
			return false
		}
	}
	for _, a := range in.Artifacts {
		if !one(a.Kind, "screenshot", "trace", "recording", "log", "command_output") || len(a.SHA256) != 64 || !hexOnly(a.SHA256) || bad(a.MediaType) || bad(a.Description) {
			return false
		}
	}
	return true
}
func (s *Store) read(id string) (Session, error) {
	if id == "" || strings.ContainsAny(id, "/\\") {
		return Session{}, ErrNotFound
	}
	b, e := os.ReadFile(filepath.Join(s.root, id+".json"))
	if os.IsNotExist(e) {
		return Session{}, ErrNotFound
	}
	if e != nil {
		return Session{}, e
	}
	var v Session
	if json.Unmarshal(b, &v) != nil {
		return Session{}, ErrInvalid
	}
	return v, nil
}
func (s *Store) write(v Session) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(s.root, "session-*.tmp")
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
	ce := tmp.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	return os.Rename(name, filepath.Join(s.root, v.ID+".json"))
}
func newID() string     { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func bad(x string) bool { return strings.TrimSpace(x) == "" || len(x) > 4096 }
func one(x string, xs ...string) bool {
	for _, v := range xs {
		if x == v {
			return true
		}
	}
	return false
}
func contains(xs []string, wanted string) bool {
	for _, x := range xs {
		if x == wanted {
			return true
		}
	}
	return false
}
func eventByID(v Session, id string) (Event, bool) {
	for _, event := range v.Events {
		if event.ID == id {
			return event, true
		}
	}
	return Event{}, false
}
func findingExists(v Session, id string) bool {
	for _, event := range v.Events {
		if event.FindingID == id && one(event.Kind, "observation", "reproduce") {
			return true
		}
	}
	return false
}
func hexOnly(x string) bool { _, e := hex.DecodeString(x); return e == nil }
