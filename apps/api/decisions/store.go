// Package decisions persists collaborative technical decision scopes and discussion.
package decisions

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

var ErrNotFound = errors.New("decision not found")
var ErrInvalid = errors.New("invalid decision")
var ErrConflict = errors.New("decision version changed")

type Source struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id,omitempty"`
}
type Resource struct {
	Kind         string `json:"kind"`
	RepositoryID string `json:"repository_id,omitempty"`
	ResourceID   string `json:"resource_id,omitempty"`
	Label        string `json:"label"`
}
type Participant struct {
	UserID  string    `json:"user_id"`
	AddedBy string    `json:"added_by"`
	AddedAt time.Time `json:"added_at"`
}
type Scope struct {
	Question          string        `json:"question"`
	Constraints       []string      `json:"constraints"`
	SuccessMeasures   []string      `json:"success_measures"`
	Deadline          *time.Time    `json:"deadline,omitempty"`
	AffectedResources []Resource    `json:"affected_resources"`
	Participants      []Participant `json:"participants"`
	OwnerID           string        `json:"owner_id"`
}
type History struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	ActorID   string    `json:"actor_id"`
	Version   int       `json:"version"`
	Summary   string    `json:"summary"`
	Body      string    `json:"body,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
type Evidence struct {
	Kind         string    `json:"kind"`
	RepositoryID string    `json:"repository_id,omitempty"`
	ResourceID   string    `json:"resource_id"`
	Revision     string    `json:"revision"`
	Path         string    `json:"path,omitempty"`
	StartLine    int       `json:"start_line,omitempty"`
	EndLine      int       `json:"end_line,omitempty"`
	Label        string    `json:"label"`
	CapturedAt   time.Time `json:"captured_at"`
}
type CriterionAssessment struct {
	Criterion string     `json:"criterion"`
	Outcome   string     `json:"outcome"`
	Evidence  []Evidence `json:"evidence"`
}
type Alternative struct {
	ID                  string                `json:"id"`
	Title               string                `json:"title"`
	Summary             string                `json:"summary"`
	Assumptions         []string              `json:"assumptions"`
	Tradeoffs           []string              `json:"tradeoffs"`
	Risks               []string              `json:"risks"`
	CompatibilityImpact string                `json:"compatibility_impact"`
	Cost                string                `json:"cost"`
	ExpectedOutcomes    []string              `json:"expected_outcomes"`
	Evidence            []Evidence            `json:"evidence"`
	Criteria            []CriterionAssessment `json:"criteria"`
	ProposedBy          string                `json:"proposed_by"`
	Version             int                   `json:"version"`
	SupersededBy        string                `json:"superseded_by,omitempty"`
	CreatedAt           time.Time             `json:"created_at"`
	UpdatedAt           time.Time             `json:"updated_at"`
	EvidenceStatus      EvidenceStatus        `json:"evidence_status"`
}
type EvidenceStatus struct {
	MissingKinds    []string   `json:"missing_kinds"`
	Stale           []Evidence `json:"stale"`
	MissingCriteria []string   `json:"missing_criteria"`
}
type Finding struct {
	ID            string     `json:"id"`
	AlternativeID string     `json:"alternative_id"`
	Body          string     `json:"body"`
	Position      string     `json:"position"`
	Uncertainty   string     `json:"uncertainty"`
	Citations     []Evidence `json:"citations"`
	ActorID       string     `json:"actor_id"`
	SupersedesID  string     `json:"supersedes_id,omitempty"`
	Superseded    bool       `json:"superseded"`
	CreatedAt     time.Time  `json:"created_at"`
}
type Measurement struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
}
type Artifact struct {
	Label  string `json:"label"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}
type ExperimentEvidence struct {
	ID             string        `json:"id"`
	CheckpointIDs  []string      `json:"checkpoint_ids"`
	CommandIDs     []string      `json:"command_ids"`
	Measurements   []Measurement `json:"measurements"`
	Artifacts      []Artifact    `json:"artifacts"`
	CPUSeconds     float64       `json:"cpu_seconds"`
	MemoryMBHours  float64       `json:"memory_mb_hours"`
	StorageMBHours float64       `json:"storage_mb_hours"`
	Notes          string        `json:"notes,omitempty"`
	RecordedBy     string        `json:"recorded_by"`
	RecordedAt     time.Time     `json:"recorded_at"`
}
type Experiment struct {
	ID                      string               `json:"id"`
	AlternativeID           string               `json:"alternative_id"`
	WorkspaceID             string               `json:"workspace_id"`
	Revision                string               `json:"revision"`
	DefinitionSHA256        string               `json:"definition_sha256"`
	DefaultBranchRevision   string               `json:"default_branch_revision"`
	DefaultDefinitionSHA256 string               `json:"default_definition_sha256"`
	Commands                []string             `json:"commands"`
	LaunchedBy              string               `json:"launched_by"`
	LaunchedAt              time.Time            `json:"launched_at"`
	Version                 int                  `json:"version"`
	Evidence                []ExperimentEvidence `json:"evidence"`
	Invalidated             bool                 `json:"invalidated"`
	InvalidationReasons     []string             `json:"invalidation_reasons"`
}
type ApprovalRequest struct {
	ID           string     `json:"id"`
	Kind         string     `json:"kind"`
	RepositoryID string     `json:"repository_id,omitempty"`
	PolicyID     string     `json:"policy_id,omitempty"`
	PolicyRule   string     `json:"policy_rule,omitempty"`
	ApproverID   string     `json:"approver_id"`
	Reason       string     `json:"reason"`
	Status       string     `json:"status"`
	RequestedBy  string     `json:"requested_by"`
	RequestedAt  time.Time  `json:"requested_at"`
	DecidedBy    string     `json:"decided_by,omitempty"`
	DecisionNote string     `json:"decision_note,omitempty"`
	DecidedAt    *time.Time `json:"decided_at,omitempty"`
}
type Exception struct {
	ApprovalRequestID string    `json:"approval_request_id"`
	PolicyID          string    `json:"policy_id"`
	PolicyRule        string    `json:"policy_rule"`
	Reason            string    `json:"reason"`
	ExpiresAt         time.Time `json:"expires_at"`
}
type Commitment struct {
	Version                int               `json:"version"`
	DecisionVersion        int               `json:"decision_version"`
	Status                 string            `json:"status"`
	SelectedAlternativeID  string            `json:"selected_alternative_id"`
	RejectedAlternativeIDs []string          `json:"rejected_alternative_ids"`
	Rationale              string            `json:"rationale"`
	AcceptedTradeoffs      []string          `json:"accepted_tradeoffs"`
	DissentFindingIDs      []string          `json:"dissent_finding_ids"`
	Conditions             []string          `json:"conditions"`
	ReviewDate             time.Time         `json:"review_date"`
	Evidence               []Evidence        `json:"evidence"`
	Approvals              []ApprovalRequest `json:"approvals"`
	Exceptions             []Exception       `json:"exceptions"`
	PublishedBy            string            `json:"published_by"`
	PublishedAt            time.Time         `json:"published_at"`
	ReopenedAt             *time.Time        `json:"reopened_at,omitempty"`
	ReopenReason           string            `json:"reopen_reason,omitempty"`
}
type Decision struct {
	ID               string            `json:"id"`
	RepositoryID     string            `json:"repository_id"`
	Source           Source            `json:"source"`
	Status           string            `json:"status"`
	Scope            Scope             `json:"scope"`
	CreatedBy        string            `json:"created_by"`
	Version          int               `json:"version"`
	History          []History         `json:"history"`
	Alternatives     []Alternative     `json:"alternatives"`
	Findings         []Finding         `json:"findings"`
	Experiments      []Experiment      `json:"experiments"`
	ApprovalRequests []ApprovalRequest `json:"approval_requests"`
	Commitments      []Commitment      `json:"commitments"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
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
	return &Store{root: root, now: func() time.Time { return time.Now().UTC() }}, nil
}
func randomID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
func cleanList(v []string) ([]string, bool) {
	out := []string{}
	for _, x := range v {
		x = strings.TrimSpace(x)
		if x == "" || len(x) > 500 {
			return nil, false
		}
		out = append(out, x)
	}
	return out, true
}
func validSource(k string) bool {
	switch k {
	case "repository", "proposal", "investigation", "incident", "evolution_plan", "stewardship_opportunity":
		return true
	}
	return false
}
func validate(s Scope) (Scope, error) {
	s.Question = strings.TrimSpace(s.Question)
	s.OwnerID = strings.TrimSpace(s.OwnerID)
	if s.Question == "" || len(s.Question) > 2000 || s.OwnerID == "" {
		return s, ErrInvalid
	}
	var ok bool
	if s.Constraints, ok = cleanList(s.Constraints); !ok {
		return s, ErrInvalid
	}
	if s.SuccessMeasures, ok = cleanList(s.SuccessMeasures); !ok {
		return s, ErrInvalid
	}
	if len(s.Constraints) > 50 || len(s.SuccessMeasures) > 50 || len(s.AffectedResources) > 100 || len(s.Participants) > 100 {
		return s, ErrInvalid
	}
	if len(s.Constraints) == 0 || len(s.SuccessMeasures) == 0 || len(s.AffectedResources) == 0 || len(s.Participants) == 0 || s.Deadline == nil || s.Deadline.IsZero() {
		return s, ErrInvalid
	}
	seen := map[string]bool{}
	for _, p := range s.Participants {
		if p.UserID == "" || seen[p.UserID] {
			return s, ErrInvalid
		}
		seen[p.UserID] = true
	}
	if !seen[s.OwnerID] {
		return s, ErrInvalid
	}
	for i := range s.AffectedResources {
		s.AffectedResources[i].Label = strings.TrimSpace(s.AffectedResources[i].Label)
		if s.AffectedResources[i].Kind == "" || s.AffectedResources[i].Label == "" || len(s.AffectedResources[i].Label) > 300 {
			return s, ErrInvalid
		}
	}
	return s, nil
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
	return func() { syscall.Flock(int(f.Fd()), syscall.LOCK_UN); f.Close() }, nil
}
func (s *Store) write(v Decision) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(s.root, ".decision-")
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
		e = os.Rename(name, filepath.Join(s.root, v.ID+".json"))
	}
	return e
}
func (s *Store) read(id string) (Decision, error) {
	var v Decision
	b, e := os.ReadFile(filepath.Join(s.root, id+".json"))
	if os.IsNotExist(e) {
		return v, ErrNotFound
	}
	if e != nil {
		return v, e
	}
	if json.Unmarshal(b, &v) != nil {
		return v, ErrInvalid
	}
	return v, nil
}
func (s *Store) Create(repo string, source Source, scope Scope, actor string) (Decision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, e := s.lock()
	if e != nil {
		return Decision{}, e
	}
	defer u()
	source.Kind = strings.TrimSpace(source.Kind)
	if repo == "" || actor == "" || !validSource(source.Kind) || (source.Kind != "repository" && source.ResourceID == "") {
		return Decision{}, ErrInvalid
	}
	now := s.now()
	for i := range scope.Participants {
		scope.Participants[i].AddedBy = actor
		scope.Participants[i].AddedAt = now
	}
	scope, e = validate(scope)
	if e != nil {
		return Decision{}, e
	}
	x, e := randomID()
	if e != nil {
		return Decision{}, e
	}
	h, _ := randomID()
	v := Decision{ID: x, RepositoryID: repo, Source: source, Status: "pending", Scope: scope, CreatedBy: actor, Version: 1, CreatedAt: now, UpdatedAt: now, History: []History{{ID: h, Kind: "scope_created", ActorID: actor, Version: 1, Summary: "Opened the decision", CreatedAt: now}}, Alternatives: []Alternative{}, Findings: []Finding{}, Experiments: []Experiment{}, ApprovalRequests: []ApprovalRequest{}, Commitments: []Commitment{}}
	return v, s.write(v)
}

func (s *Store) LaunchExperiment(id, actor, alternativeID, workspaceID, revision, definition, defaultRevision, defaultDefinition string, commands []string) (Decision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, e := s.lock()
	if e != nil {
		return Decision{}, e
	}
	defer u()
	v, e := s.read(id)
	if e != nil {
		return v, e
	}
	if !isParticipant(v, actor) {
		return v, ErrNotFound
	}
	found := false
	for _, a := range v.Alternatives {
		found = found || a.ID == alternativeID && a.SupersededBy == ""
	}
	if !found || workspaceID == "" || revision == "" || definition == "" || defaultRevision == "" || defaultDefinition == "" || len(commands) == 0 || len(commands) > 20 {
		return v, ErrInvalid
	}
	for _, x := range v.Experiments {
		if x.WorkspaceID == workspaceID {
			if x.AlternativeID == alternativeID && x.Revision == revision && x.DefinitionSHA256 == definition {
				return projectEvidence(v, s.now()), nil
			}
			return v, ErrConflict
		}
	}
	for _, command := range commands {
		if strings.TrimSpace(command) == "" || len(command) > 2000 {
			return v, ErrInvalid
		}
	}
	now := s.now()
	experimentID, _ := randomID()
	historyID, _ := randomID()
	v.Experiments = append(v.Experiments, Experiment{ID: experimentID, AlternativeID: alternativeID, WorkspaceID: workspaceID, Revision: revision, DefinitionSHA256: definition, DefaultBranchRevision: defaultRevision, DefaultDefinitionSHA256: defaultDefinition, Commands: commands, LaunchedBy: actor, LaunchedAt: now, Version: 1, Evidence: []ExperimentEvidence{}, InvalidationReasons: []string{}})
	v.Version++
	v.UpdatedAt = now
	reopen(&v, actor, "A new experiment changed the decision context", now)
	v.History = append(v.History, History{ID: historyID, Kind: "experiment_launched", ActorID: actor, Version: v.Version, Summary: "Launched a bounded alternative experiment", CreatedAt: now})
	if e = s.write(v); e != nil {
		return v, e
	}
	return projectEvidence(v, now), nil
}

func (s *Store) AttachExperimentEvidence(id, experimentID, actor string, expected int, input ExperimentEvidence) (Decision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, e := s.lock()
	if e != nil {
		return Decision{}, e
	}
	defer u()
	v, e := s.read(id)
	if e != nil {
		return v, e
	}
	if !isParticipant(v, actor) {
		return v, ErrNotFound
	}
	index := -1
	for i := range v.Experiments {
		if v.Experiments[i].ID == experimentID {
			index = i
		}
	}
	if index < 0 {
		return v, ErrNotFound
	}
	experiment := &v.Experiments[index]
	if experiment.Version != expected {
		return v, ErrConflict
	}
	if input.CPUSeconds < 0 || input.MemoryMBHours < 0 || input.StorageMBHours < 0 || len(input.CheckpointIDs) > 100 || len(input.CommandIDs) > 100 || len(input.Measurements) > 100 || len(input.Artifacts) > 100 {
		return v, ErrInvalid
	}
	input.Notes = strings.TrimSpace(input.Notes)
	if len(input.Notes) > 4000 {
		return v, ErrInvalid
	}
	for i := range input.Measurements {
		input.Measurements[i].Name = strings.TrimSpace(input.Measurements[i].Name)
		input.Measurements[i].Unit = strings.TrimSpace(input.Measurements[i].Unit)
		if input.Measurements[i].Name == "" || input.Measurements[i].Unit == "" {
			return v, ErrInvalid
		}
	}
	for i := range input.Artifacts {
		a := &input.Artifacts[i]
		a.Label, a.Path, a.SHA256 = strings.TrimSpace(a.Label), strings.TrimSpace(a.Path), strings.ToLower(strings.TrimSpace(a.SHA256))
		_, digestErr := hex.DecodeString(a.SHA256)
		cleanPath := filepath.Clean(a.Path)
		if a.Label == "" || a.Path == "" || filepath.IsAbs(a.Path) || cleanPath == ".." || strings.HasPrefix(cleanPath, ".."+string(filepath.Separator)) || len(a.SHA256) != 64 || digestErr != nil || a.Size < 0 {
			return v, ErrInvalid
		}
	}
	now := s.now()
	input.ID, _ = randomID()
	input.RecordedBy, input.RecordedAt = actor, now
	experiment.Evidence = append(experiment.Evidence, input)
	experiment.Version++
	v.Version++
	v.UpdatedAt = now
	reopen(&v, actor, "New experiment evidence changed the decision context", now)
	historyID, _ := randomID()
	v.History = append(v.History, History{ID: historyID, Kind: "experiment_evidence", ActorID: actor, Version: v.Version, Summary: "Attached attributed experiment evidence", CreatedAt: now})
	if e = s.write(v); e != nil {
		return v, e
	}
	return projectEvidence(v, now), nil
}
func (s *Store) Get(id string) (Decision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, e := s.read(id)
	return projectEvidence(v, s.now()), e
}
func (s *Store) List() (out []Decision, e error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ents, e := os.ReadDir(s.root)
	if e != nil {
		return nil, e
	}
	for _, x := range ents {
		if strings.HasSuffix(x.Name(), ".json") {
			v, er := s.read(strings.TrimSuffix(x.Name(), ".json"))
			if er != nil {
				return nil, er
			}
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	for i := range out {
		out[i] = projectEvidence(out[i], s.now())
	}
	return out, nil
}
func projectEvidence(v Decision, now time.Time) Decision {
	kinds := []string{"code", "dependency", "release", "incident", "usage"}
	for i := range v.Alternatives {
		a := &v.Alternatives[i]
		a.EvidenceStatus = EvidenceStatus{MissingKinds: []string{}, Stale: []Evidence{}, MissingCriteria: []string{}}
		seen := map[string]bool{}
		for _, e := range a.Evidence {
			seen[e.Kind] = true
			if now.Sub(e.CapturedAt) > 30*24*time.Hour {
				a.EvidenceStatus.Stale = append(a.EvidenceStatus.Stale, e)
			}
		}
		for _, kind := range kinds {
			if !seen[kind] {
				a.EvidenceStatus.MissingKinds = append(a.EvidenceStatus.MissingKinds, kind)
			}
		}
		for _, c := range a.Criteria {
			if strings.EqualFold(c.Outcome, "not yet demonstrated") || len(c.Evidence) == 0 {
				a.EvidenceStatus.MissingCriteria = append(a.EvidenceStatus.MissingCriteria, c.Criterion)
			}
			for _, e := range c.Evidence {
				if now.Sub(e.CapturedAt) > 30*24*time.Hour {
					a.EvidenceStatus.Stale = append(a.EvidenceStatus.Stale, e)
				}
			}
		}
	}
	return v
}
func reopen(v *Decision, actor, reason string, now time.Time) {
	if v.Status != "published" || len(v.Commitments) == 0 {
		return
	}
	last := &v.Commitments[len(v.Commitments)-1]
	last.Status, last.ReopenReason = "reopened", reason
	last.ReopenedAt = &now
	v.Status = "pending"
	for i := range v.ApprovalRequests {
		v.ApprovalRequests[i].Status = "superseded"
	}
	h, _ := randomID()
	v.History = append(v.History, History{ID: h, Kind: "decision_reopened", ActorID: actor, Version: v.Version, Summary: reason, CreatedAt: now})
}
func (s *Store) Update(id, actor string, expected int, scope Scope, summary string) (Decision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, e := s.lock()
	if e != nil {
		return Decision{}, e
	}
	defer u()
	v, e := s.read(id)
	if e != nil {
		return v, e
	}
	if v.Version != expected {
		return v, ErrConflict
	}
	if !isParticipant(v, actor) {
		return v, ErrNotFound
	}
	scope, e = validate(scope)
	if e != nil {
		return v, e
	}
	now := s.now()
	previous := map[string]Participant{}
	for _, p := range v.Scope.Participants {
		previous[p.UserID] = p
	}
	for i := range scope.Participants {
		if retained, ok := previous[scope.Participants[i].UserID]; ok {
			scope.Participants[i] = retained
		} else {
			scope.Participants[i].AddedAt = now
			scope.Participants[i].AddedBy = actor
		}
	}
	summary = strings.TrimSpace(summary)
	if summary == "" || len(summary) > 500 {
		return v, ErrInvalid
	}
	v.Scope = scope
	v.Version++
	v.UpdatedAt = now
	reopen(&v, actor, "Material scope changes require a fresh decision", now)
	h, _ := idgen()
	v.History = append(v.History, History{ID: h, Kind: "scope_changed", ActorID: actor, Version: v.Version, Summary: summary, CreatedAt: now})
	if e = s.write(v); e != nil {
		return v, e
	}
	return projectEvidence(v, now), nil
}
func idgen() (string, error) { return randomID() }
func (s *Store) Discuss(id, actor, body string) (Decision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, e := s.lock()
	if e != nil {
		return Decision{}, e
	}
	defer u()
	v, e := s.read(id)
	if e != nil {
		return v, e
	}
	if !isParticipant(v, actor) {
		return v, ErrNotFound
	}
	body = strings.TrimSpace(body)
	if body == "" || len(body) > 4000 {
		return v, ErrInvalid
	}
	h, _ := randomID()
	now := s.now()
	v.History = append(v.History, History{ID: h, Kind: "discussion", ActorID: actor, Version: v.Version, Summary: "Added to the discussion", Body: body, CreatedAt: now})
	v.UpdatedAt = now
	if e = s.write(v); e != nil {
		return v, e
	}
	return projectEvidence(v, now), nil
}
func isParticipant(v Decision, u string) bool {
	for _, p := range v.Scope.Participants {
		if p.UserID == u {
			return true
		}
	}
	return false
}

func validateEvidence(items []Evidence, now time.Time) ([]Evidence, error) {
	if len(items) == 0 || len(items) > 100 {
		return nil, ErrInvalid
	}
	seen := map[string]bool{}
	for i := range items {
		e := &items[i]
		e.Kind = strings.TrimSpace(e.Kind)
		e.ResourceID = strings.TrimSpace(e.ResourceID)
		e.Revision = strings.TrimSpace(e.Revision)
		e.Label = strings.TrimSpace(e.Label)
		e.Path = strings.TrimSpace(e.Path)
		if e.Kind != "code" && e.Kind != "dependency" && e.Kind != "release" && e.Kind != "incident" && e.Kind != "usage" {
			return nil, ErrInvalid
		}
		if e.ResourceID == "" || e.Revision == "" || e.Label == "" || len(e.Label) > 300 || len(e.Revision) > 200 || (e.Kind == "code" && (e.RepositoryID == "" || e.Path == "" || e.StartLine < 1 || e.EndLine < e.StartLine)) {
			return nil, ErrInvalid
		}
		key := fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%d\x00%d", e.Kind, e.ResourceID, e.Revision, e.Path, e.StartLine, e.EndLine)
		if seen[key] {
			return nil, ErrInvalid
		}
		seen[key] = true
		e.CapturedAt = now
	}
	return items, nil
}
func validateAlternative(a Alternative, now time.Time, measures []string) (Alternative, error) {
	a.Title, a.Summary, a.CompatibilityImpact, a.Cost = strings.TrimSpace(a.Title), strings.TrimSpace(a.Summary), strings.TrimSpace(a.CompatibilityImpact), strings.TrimSpace(a.Cost)
	if a.Title == "" || len(a.Title) > 200 || a.Summary == "" || len(a.Summary) > 4000 || a.CompatibilityImpact == "" || len(a.CompatibilityImpact) > 2000 || a.Cost == "" || len(a.Cost) > 1000 {
		return a, ErrInvalid
	}
	var ok bool
	if a.Assumptions, ok = cleanList(a.Assumptions); !ok || len(a.Assumptions) == 0 {
		return a, ErrInvalid
	}
	if a.Tradeoffs, ok = cleanList(a.Tradeoffs); !ok || len(a.Tradeoffs) == 0 {
		return a, ErrInvalid
	}
	if a.Risks, ok = cleanList(a.Risks); !ok || len(a.Risks) == 0 {
		return a, ErrInvalid
	}
	if a.ExpectedOutcomes, ok = cleanList(a.ExpectedOutcomes); !ok || len(a.ExpectedOutcomes) == 0 {
		return a, ErrInvalid
	}
	var e error
	if a.Evidence, e = validateEvidence(a.Evidence, now); e != nil {
		return a, e
	}
	if len(a.Criteria) != len(measures) {
		return a, ErrInvalid
	}
	seen := map[string]bool{}
	for i := range a.Criteria {
		c := &a.Criteria[i]
		c.Criterion = strings.TrimSpace(c.Criterion)
		c.Outcome = strings.TrimSpace(c.Outcome)
		if c.Outcome == "" || len(c.Outcome) > 1000 || seen[c.Criterion] {
			return a, ErrInvalid
		}
		seen[c.Criterion] = true
		if c.Evidence, e = validateEvidence(c.Evidence, now); e != nil {
			return a, e
		}
	}
	for _, m := range measures {
		if !seen[m] {
			return a, ErrInvalid
		}
	}
	return a, nil
}
func (s *Store) AddAlternative(id, actor string, expected int, input Alternative) (Decision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, e := s.lock()
	if e != nil {
		return Decision{}, e
	}
	defer u()
	v, e := s.read(id)
	if e != nil {
		return v, e
	}
	if v.Version != expected {
		return v, ErrConflict
	}
	if !isParticipant(v, actor) {
		return v, ErrNotFound
	}
	now := s.now()
	input.EvidenceStatus = EvidenceStatus{}
	input, e = validateAlternative(input, now, v.Scope.SuccessMeasures)
	if e != nil {
		return v, e
	}
	input.ID, _ = randomID()
	input.ProposedBy = actor
	input.Version = 1
	input.CreatedAt = now
	input.UpdatedAt = now
	v.Alternatives = append(v.Alternatives, input)
	v.Version++
	v.UpdatedAt = now
	reopen(&v, actor, "A new alternative changed the decision context", now)
	h, _ := randomID()
	v.History = append(v.History, History{ID: h, Kind: "alternative_proposed", ActorID: actor, Version: v.Version, Summary: "Proposed alternative: " + input.Title, CreatedAt: now})
	if e = s.write(v); e != nil {
		return v, e
	}
	return projectEvidence(v, now), nil
}
func (s *Store) AddFinding(id, actor string, input Finding) (Decision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, e := s.lock()
	if e != nil {
		return Decision{}, e
	}
	defer u()
	v, e := s.read(id)
	if e != nil {
		return v, e
	}
	found := false
	for _, a := range v.Alternatives {
		found = found || a.ID == input.AlternativeID
	}
	if !found {
		return v, ErrInvalid
	}
	input.Body = strings.TrimSpace(input.Body)
	input.Uncertainty = strings.TrimSpace(input.Uncertainty)
	if input.Body == "" || len(input.Body) > 4000 || input.Uncertainty == "" || len(input.Uncertainty) > 2000 || (input.Position != "support" && input.Position != "oppose" && input.Position != "neutral") {
		return v, ErrInvalid
	}
	now := s.now()
	if input.Citations, e = validateEvidence(input.Citations, now); e != nil {
		return v, e
	}
	if input.SupersedesID != "" {
		matched := false
		for i := range v.Findings {
			if v.Findings[i].ID == input.SupersedesID && v.Findings[i].AlternativeID == input.AlternativeID && !v.Findings[i].Superseded {
				v.Findings[i].Superseded = true
				matched = true
			}
		}
		if !matched {
			return v, ErrInvalid
		}
	}
	input.ID, _ = randomID()
	input.ActorID = actor
	input.CreatedAt = now
	v.Findings = append(v.Findings, input)
	v.Version++
	v.UpdatedAt = now
	reopen(&v, actor, "New evidence or dissent changed the decision context", now)
	h, _ := randomID()
	v.History = append(v.History, History{ID: h, Kind: "research_finding", ActorID: actor, Version: v.Version, Summary: "Added cited " + input.Position + " finding", Body: input.Body, CreatedAt: now})
	if e = s.write(v); e != nil {
		return v, e
	}
	return projectEvidence(v, now), nil
}

func (s *Store) RequestApproval(id, actor string, expected int, input ApprovalRequest) (Decision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, e := s.lock()
	if e != nil {
		return Decision{}, e
	}
	defer u()
	v, e := s.read(id)
	if e != nil {
		return v, e
	}
	if v.Version != expected {
		return v, ErrConflict
	}
	if actor != v.Scope.OwnerID || v.Status != "pending" {
		return v, ErrNotFound
	}
	input.Kind, input.RepositoryID, input.PolicyID, input.PolicyRule, input.ApproverID, input.Reason = strings.TrimSpace(input.Kind), strings.TrimSpace(input.RepositoryID), strings.TrimSpace(input.PolicyID), strings.TrimSpace(input.PolicyRule), strings.TrimSpace(input.ApproverID), strings.TrimSpace(input.Reason)
	if (input.Kind != "affected_owner" && input.Kind != "policy") || input.ApproverID == "" || input.Reason == "" || len(input.Reason) > 1000 || (input.Kind == "affected_owner" && input.RepositoryID == "") || (input.Kind == "policy" && (input.PolicyID == "" || input.PolicyRule == "")) {
		return v, ErrInvalid
	}
	for i := range v.ApprovalRequests {
		x := &v.ApprovalRequests[i]
		if x.Status == "pending" && x.Kind == input.Kind && x.RepositoryID == input.RepositoryID && x.PolicyID == input.PolicyID && x.PolicyRule == input.PolicyRule && x.ApproverID == input.ApproverID {
			return projectEvidence(v, s.now()), nil
		}
		if x.Status == "rejected" && x.Kind == input.Kind && x.RepositoryID == input.RepositoryID && x.PolicyID == input.PolicyID && x.PolicyRule == input.PolicyRule && x.ApproverID == input.ApproverID {
			x.Status = "superseded"
		}
	}
	now := s.now()
	input.ID, _ = randomID()
	input.Status, input.RequestedBy, input.RequestedAt = "pending", actor, now
	input.DecidedBy, input.DecisionNote, input.DecidedAt = "", "", nil
	v.ApprovalRequests = append(v.ApprovalRequests, input)
	v.Version++
	v.UpdatedAt = now
	h, _ := randomID()
	v.History = append(v.History, History{ID: h, Kind: "approval_requested", ActorID: actor, Version: v.Version, Summary: "Requested " + input.Kind + " approval", CreatedAt: now})
	if e = s.write(v); e != nil {
		return v, e
	}
	return projectEvidence(v, now), nil
}

func (s *Store) RespondApproval(id, requestID, actor, response, note string) (Decision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, e := s.lock()
	if e != nil {
		return Decision{}, e
	}
	defer u()
	v, e := s.read(id)
	if e != nil {
		return v, e
	}
	note = strings.TrimSpace(note)
	if (response != "approve" && response != "reject") || len(note) > 2000 {
		return v, ErrInvalid
	}
	index := -1
	for i := range v.ApprovalRequests {
		if v.ApprovalRequests[i].ID == requestID {
			index = i
		}
	}
	if index < 0 || v.ApprovalRequests[index].ApproverID != actor || v.ApprovalRequests[index].Status != "pending" {
		return v, ErrNotFound
	}
	now := s.now()
	x := &v.ApprovalRequests[index]
	x.Status, x.DecidedBy, x.DecisionNote, x.DecidedAt = map[bool]string{true: "approved", false: "rejected"}[response == "approve"], actor, note, &now
	v.Version++
	v.UpdatedAt = now
	h, _ := randomID()
	v.History = append(v.History, History{ID: h, Kind: "approval_" + x.Status, ActorID: actor, Version: v.Version, Summary: "Approval request " + x.Status, Body: note, CreatedAt: now})
	if e = s.write(v); e != nil {
		return v, e
	}
	return projectEvidence(v, now), nil
}

func evidenceKey(e Evidence) string {
	return fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%d\x00%d", e.Kind, e.ResourceID, e.Revision, e.Path, e.StartLine, e.EndLine)
}

func (s *Store) Publish(id, actor string, expected int, input Commitment) (Decision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, e := s.lock()
	if e != nil {
		return Decision{}, e
	}
	defer u()
	v, e := s.read(id)
	if e != nil {
		return v, e
	}
	if v.Version != expected {
		return v, ErrConflict
	}
	if actor != v.Scope.OwnerID || v.Status != "pending" {
		return v, ErrNotFound
	}
	input.Rationale = strings.TrimSpace(input.Rationale)
	if input.Rationale == "" || len(input.Rationale) > 8000 || input.ReviewDate.IsZero() || !input.ReviewDate.After(s.now()) {
		return v, ErrInvalid
	}
	var ok bool
	if input.AcceptedTradeoffs, ok = cleanList(input.AcceptedTradeoffs); !ok || len(input.AcceptedTradeoffs) == 0 {
		return v, ErrInvalid
	}
	if input.Conditions, ok = cleanList(input.Conditions); !ok {
		return v, ErrInvalid
	}
	alts := map[string]bool{}
	for _, a := range v.Alternatives {
		if a.SupersededBy == "" {
			alts[a.ID] = true
		}
	}
	if !alts[input.SelectedAlternativeID] {
		return v, ErrInvalid
	}
	seenReject := map[string]bool{}
	for _, rid := range input.RejectedAlternativeIDs {
		if !alts[rid] || rid == input.SelectedAlternativeID || seenReject[rid] {
			return v, ErrInvalid
		}
		seenReject[rid] = true
	}
	if len(seenReject) != len(alts)-1 {
		return v, ErrInvalid
	}
	findings := map[string]bool{}
	for _, f := range v.Findings {
		if f.Position == "oppose" {
			findings[f.ID] = true
		}
	}
	for _, fid := range input.DissentFindingIDs {
		if !findings[fid] {
			return v, ErrInvalid
		}
	}
	available := map[string]Evidence{}
	for _, a := range v.Alternatives {
		for _, x := range a.Evidence {
			available[evidenceKey(x)] = x
		}
		for _, c := range a.Criteria {
			for _, x := range c.Evidence {
				available[evidenceKey(x)] = x
			}
		}
	}
	for _, f := range v.Findings {
		for _, x := range f.Citations {
			available[evidenceKey(x)] = x
		}
	}
	if len(input.Evidence) == 0 {
		return v, ErrInvalid
	}
	for i, x := range input.Evidence {
		retained, exists := available[evidenceKey(x)]
		if !exists {
			return v, ErrInvalid
		}
		input.Evidence[i] = retained
	}
	for _, request := range v.ApprovalRequests {
		if request.Status == "pending" || request.Status == "rejected" {
			return v, ErrConflict
		}
	}
	approved := map[string]ApprovalRequest{}
	for _, request := range v.ApprovalRequests {
		if request.Status == "approved" {
			approved[request.ID] = request
		}
	}
	for _, x := range input.Exceptions {
		request, exists := approved[x.ApprovalRequestID]
		if !exists || request.Kind != "policy" || request.PolicyID != x.PolicyID || request.PolicyRule != x.PolicyRule || strings.TrimSpace(x.Reason) == "" || !x.ExpiresAt.After(s.now()) {
			return v, ErrInvalid
		}
	}
	now := s.now()
	input.ReopenedAt, input.ReopenReason = nil, ""
	input.Version = len(v.Commitments) + 1
	input.DecisionVersion = v.Version
	input.Status = "published"
	input.Approvals = append([]ApprovalRequest(nil), v.ApprovalRequests...)
	input.PublishedBy = actor
	input.PublishedAt = now
	v.Commitments = append(v.Commitments, input)
	v.Status = "published"
	v.Version++
	v.UpdatedAt = now
	h, _ := randomID()
	v.History = append(v.History, History{ID: h, Kind: "decision_published", ActorID: actor, Version: v.Version, Summary: "Published decision version", Body: input.Rationale, CreatedAt: now})
	if e = s.write(v); e != nil {
		return v, e
	}
	return projectEvidence(v, now), nil
}
