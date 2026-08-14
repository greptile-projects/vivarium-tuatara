// Package accessibilityassessments retains revision-exact automated and accountable accessibility evidence.
package accessibilityassessments

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

var ErrNotFound = errors.New("accessibility assessment not found")
var ErrInvalid = errors.New("invalid accessibility assessment")

type Check struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Category        string     `json:"category"`
	Outcome         string     `json:"outcome"`
	JourneyID       string     `json:"journey_id,omitempty"`
	SourceLocations []string   `json:"source_locations"`
	AudienceIDs     []string   `json:"audience_ids"`
	Summary         string     `json:"summary"`
	EvidenceRef     string     `json:"evidence_ref,omitempty"`
	RequiresHuman   bool       `json:"requires_human"`
	InvalidatedAt   *time.Time `json:"invalidated_at,omitempty"`
}
type Citation struct {
	Kind        string `json:"kind"`
	ResourceID  string `json:"resource_id"`
	Revision    string `json:"revision"`
	Location    string `json:"location,omitempty"`
	EvidenceRef string `json:"evidence_ref"`
}
type Decision struct {
	Classification string    `json:"classification"`
	Reason         string    `json:"reason"`
	ActorID        string    `json:"actor_id"`
	CreatedAt      time.Time `json:"created_at"`
}
type Finding struct {
	ID              string     `json:"id"`
	AuthorKind      string     `json:"author_kind"`
	AuthorID        string     `json:"author_id"`
	Title           string     `json:"title"`
	Detail          string     `json:"detail"`
	Severity        string     `json:"severity"`
	AudienceIDs     []string   `json:"audience_ids"`
	SourceLocations []string   `json:"source_locations"`
	JourneyIDs      []string   `json:"journey_ids"`
	Uncertainty     string     `json:"uncertainty"`
	RequiresHuman   bool       `json:"requires_human"`
	DuplicateOf     string     `json:"duplicate_of,omitempty"`
	Citations       []Citation `json:"citations"`
	Decision        *Decision  `json:"decision,omitempty"`
	InvalidatedAt   *time.Time `json:"invalidated_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	Repair          *Repair    `json:"repair,omitempty"`
}
type RepairEvidence struct {
	Kind        string `json:"kind"`
	ResourceID  string `json:"resource_id"`
	EvidenceRef string `json:"evidence_ref"`
	Summary     string `json:"summary"`
}
type Repair struct {
	RecoveryID         string           `json:"recovery_id"`
	State              string           `json:"state"`
	ProposalID         string           `json:"proposal_id"`
	TaskID             string           `json:"task_id"`
	BaseRevision       string           `json:"base_revision"`
	AcceptanceCriteria []string         `json:"acceptance_criteria"`
	CommitmentID       string           `json:"commitment_id"`
	CommitmentVersion  int              `json:"commitment_version"`
	CommitmentTitle    string           `json:"commitment_title"`
	ComponentGuidance  []string         `json:"component_guidance"`
	PermittedEvidence  []RepairEvidence `json:"permitted_reproduction_evidence"`
	AssigneeType       string           `json:"assignee_type"`
	AssigneeID         string           `json:"assignee_id"`
	CreatedBy          string           `json:"created_by"`
	CreatedAt          time.Time        `json:"created_at"`
}
type Assessment struct {
	ID            string    `json:"id"`
	RepositoryID  string    `json:"repository_id"`
	PullRequestID string    `json:"pull_request_id,omitempty"`
	Revision      string    `json:"revision"`
	Checks        []Check   `json:"checks"`
	Findings      []Finding `json:"findings"`
	CreatedBy     string    `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
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
func (s *Store) Create(repo, actor string, x Assessment) (Assessment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, e := s.lock(syscall.LOCK_EX)
	if e != nil {
		return x, e
	}
	defer u()
	x.ID = newID()
	x.RepositoryID = repo
	x.CreatedBy = actor
	x.Findings = nil
	x.CreatedAt = s.now()
	x.UpdatedAt = x.CreatedAt
	for i := range x.Checks {
		if x.Checks[i].ID == "" {
			x.Checks[i].ID = newID()
		}
	}
	if !validAssessment(x) {
		return x, ErrInvalid
	}
	return x, s.write(x)
}
func (s *Store) AddFinding(repo, id, kind, actor string, x Finding) (Assessment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, e := s.lock(syscall.LOCK_EX)
	if e != nil {
		return Assessment{}, e
	}
	defer u()
	v, e := s.get(repo, id)
	if e != nil {
		return v, e
	}
	x.ID = newID()
	x.AuthorKind = kind
	x.AuthorID = actor
	x.Decision = nil
	x.CreatedAt = s.now()
	if !validFinding(x, v.Revision, v.Findings) {
		return v, ErrInvalid
	}
	v.Findings = append(v.Findings, x)
	v.UpdatedAt = x.CreatedAt
	return v, s.write(v)
}
func (s *Store) Decide(repo, id, findingID, actor, classification, reason string) (Assessment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, e := s.lock(syscall.LOCK_EX)
	if e != nil {
		return Assessment{}, e
	}
	defer u()
	v, e := s.get(repo, id)
	if e != nil {
		return v, e
	}
	if classification != "accepted" && classification != "false_positive" {
		return v, ErrInvalid
	}
	for i := range v.Findings {
		if v.Findings[i].ID == findingID {
			if v.Findings[i].InvalidatedAt != nil {
				return v, ErrInvalid
			}
			v.Findings[i].Decision = &Decision{Classification: classification, Reason: strings.TrimSpace(reason), ActorID: actor, CreatedAt: s.now()}
			if v.Findings[i].Decision.Reason == "" {
				return v, ErrInvalid
			}
			v.UpdatedAt = v.Findings[i].Decision.CreatedAt
			return v, s.write(v)
		}
	}
	return v, ErrNotFound
}

// ReserveRepair persists a recovery identity before implementation work is
// created. Exact retries converge on it, including after finding invalidation.
func (s *Store) ReserveRepair(repo, id, findingID, actor string, repair Repair) (Assessment, Repair, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, e := s.lock(syscall.LOCK_EX)
	if e != nil {
		return Assessment{}, Repair{}, e
	}
	defer u()
	v, e := s.get(repo, id)
	if e != nil {
		return v, Repair{}, e
	}
	for i := range v.Findings {
		f := &v.Findings[i]
		if f.ID != findingID {
			continue
		}
		if f.Repair != nil {
			if sameRepairRequest(*f.Repair, repair) {
				return v, *f.Repair, nil
			}
			return v, Repair{}, ErrInvalid
		}
		if f.InvalidatedAt != nil || f.Decision == nil || f.Decision.Classification != "accepted" {
			return v, Repair{}, ErrInvalid
		}
		if repair.AssigneeType == "agent" && strings.TrimSpace(repair.AssigneeID) == "" {
			repair.AssigneeID = newID()
		}
		repair.RecoveryID, repair.State, repair.CreatedBy, repair.CreatedAt = newID(), "pending", actor, s.now()
		if !validRepair(repair, v.Revision, f.Citations) {
			return v, Repair{}, ErrInvalid
		}
		f.Repair = &repair
		v.UpdatedAt = repair.CreatedAt
		return v, repair, s.write(v)
	}
	return v, Repair{}, ErrNotFound
}
func (s *Store) FinalizeRepair(repo, id, findingID, recoveryID, proposalID, taskID string) (Assessment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, e := s.lock(syscall.LOCK_EX)
	if e != nil {
		return Assessment{}, e
	}
	defer u()
	v, e := s.get(repo, id)
	if e != nil {
		return v, e
	}
	for i := range v.Findings {
		f := &v.Findings[i]
		if f.ID != findingID {
			continue
		}
		if f.Repair == nil || f.Repair.RecoveryID != recoveryID {
			return v, ErrInvalid
		}
		if f.Repair.State == "linked" {
			if f.Repair.ProposalID == proposalID && f.Repair.TaskID == taskID {
				return v, nil
			}
			return v, ErrInvalid
		}
		if f.Repair.State != "pending" || !bounded(proposalID, 64) || !bounded(taskID, 64) {
			return v, ErrInvalid
		}
		f.Repair.State, f.Repair.ProposalID, f.Repair.TaskID = "linked", proposalID, taskID
		v.UpdatedAt = s.now()
		return v, s.write(v)
	}
	return v, ErrNotFound
}
func (s *Store) CancelRepair(repo, id, findingID, recoveryID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, e := s.lock(syscall.LOCK_EX)
	if e != nil {
		return e
	}
	defer u()
	v, e := s.get(repo, id)
	if e != nil {
		return e
	}
	for i := range v.Findings {
		f := &v.Findings[i]
		if f.ID == findingID && f.Repair != nil && f.Repair.RecoveryID == recoveryID && f.Repair.State == "pending" {
			f.Repair = nil
			v.UpdatedAt = s.now()
			return s.write(v)
		}
	}
	return ErrNotFound
}
func (s *Store) Invalidate(repo, id, actor string, paths, journeys []string) (Assessment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, e := s.lock(syscall.LOCK_EX)
	if e != nil {
		return Assessment{}, e
	}
	defer u()
	v, e := s.get(repo, id)
	if e != nil {
		return v, e
	}
	if actor == "" || (len(paths) == 0 && len(journeys) == 0) {
		return v, ErrInvalid
	}
	now := s.now()
	for i := range v.Checks {
		if overlaps(v.Checks[i].SourceLocations, paths) || (v.Checks[i].JourneyID != "" && contains(journeys, v.Checks[i].JourneyID)) {
			v.Checks[i].InvalidatedAt = &now
		}
	}
	for i := range v.Findings {
		if overlaps(v.Findings[i].SourceLocations, paths) || overlaps(v.Findings[i].JourneyIDs, journeys) {
			v.Findings[i].InvalidatedAt = &now
			v.Findings[i].Decision = nil
		}
	}
	v.UpdatedAt = now
	return v, s.write(v)
}
func (s *Store) Get(repo, id string) (Assessment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, e := s.lock(syscall.LOCK_SH)
	if e != nil {
		return Assessment{}, e
	}
	defer u()
	return s.get(repo, id)
}
func (s *Store) List(repo, revision, pull string) ([]Assessment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, e := s.lock(syscall.LOCK_SH)
	if e != nil {
		return nil, e
	}
	defer u()
	entries, e := os.ReadDir(filepath.Join(s.root, repo))
	if errors.Is(e, os.ErrNotExist) {
		return []Assessment{}, nil
	}
	if e != nil {
		return nil, e
	}
	out := []Assessment{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		v, e := s.get(repo, strings.TrimSuffix(entry.Name(), ".json"))
		if e != nil {
			return nil, e
		}
		if (revision == "" || v.Revision == revision) && (pull == "" || v.PullRequestID == pull) {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}
func validAssessment(x Assessment) bool {
	if x.RepositoryID == "" || !bounded(x.Revision, 256) || len(x.Checks) == 0 || len(x.Checks) > 100 {
		return false
	}
	seen := map[string]bool{}
	for _, c := range x.Checks {
		if seen[c.ID] || !contains([]string{"semantics", "keyboard", "focus", "contrast", "motion", "captions", "journey"}, c.Category) || !contains([]string{"passed", "failed", "unevaluated"}, c.Outcome) || !bounded(c.Name, 200) || !bounded(c.Summary, 4000) || len(c.SourceLocations) > 30 || len(c.AudienceIDs) == 0 || len(c.AudienceIDs) > 30 || !boundedList(c.SourceLocations, 500) || !boundedList(c.AudienceIDs, 200) {
			return false
		}
		seen[c.ID] = true
	}
	return true
}
func validFinding(x Finding, revision string, existing []Finding) bool {
	if !contains([]string{"human", "agent"}, x.AuthorKind) || !bounded(x.Title, 200) || !bounded(x.Detail, 8000) || !contains([]string{"critical", "major", "moderate", "minor", "note"}, x.Severity) || len(x.AudienceIDs) == 0 || len(x.AudienceIDs) > 30 || len(x.SourceLocations) > 30 || len(x.JourneyIDs) > 30 || len(x.Citations) == 0 || len(x.Citations) > 12 || !bounded(x.Uncertainty, 1000) || !boundedList(x.AudienceIDs, 200) || !boundedList(x.SourceLocations, 500) || !boundedList(x.JourneyIDs, 256) {
		return false
	}
	if x.DuplicateOf != "" {
		ok := false
		for _, f := range existing {
			ok = ok || f.ID == x.DuplicateOf
		}
		if !ok {
			return false
		}
	}
	for _, c := range x.Citations {
		if !contains([]string{"preview", "reproduction"}, c.Kind) || c.Revision != revision || !strings.HasPrefix(c.EvidenceRef, "artifact://") {
			return false
		}
	}
	return true
}
func validRepair(x Repair, revision string, citations []Citation) bool {
	if !bounded(x.RecoveryID, 64) || x.State != "pending" || x.ProposalID != "" || x.TaskID != "" || x.BaseRevision != revision || !bounded(x.CommitmentID, 64) || x.CommitmentVersion < 1 || !bounded(x.CommitmentTitle, 300) || len(x.AcceptanceCriteria) == 0 || len(x.AcceptanceCriteria) > 20 || len(x.ComponentGuidance) == 0 || len(x.ComponentGuidance) > 20 || len(x.PermittedEvidence) == 0 || len(x.PermittedEvidence) > 12 || !boundedList(x.AcceptanceCriteria, 1000) || !boundedList(x.ComponentGuidance, 1000) || !contains([]string{"human", "agent"}, x.AssigneeType) || !bounded(x.AssigneeID, 64) {
		return false
	}
	allowed := map[string]bool{}
	for _, c := range citations {
		allowed[c.Kind+"\x00"+c.ResourceID+"\x00"+c.EvidenceRef] = true
	}
	for _, e := range x.PermittedEvidence {
		if !allowed[e.Kind+"\x00"+e.ResourceID+"\x00"+e.EvidenceRef] || !bounded(e.Summary, 2000) {
			return false
		}
	}
	return true
}
func sameRepairRequest(a, b Repair) bool {
	if a.AssigneeType == "agent" && b.AssigneeType == "agent" && strings.TrimSpace(b.AssigneeID) == "" {
		b.AssigneeID = a.AssigneeID
	}
	a.RecoveryID, a.State, a.ProposalID, a.TaskID, a.CreatedBy, a.CreatedAt = "", "", "", "", "", time.Time{}
	b.RecoveryID, b.State, b.ProposalID, b.TaskID, b.CreatedBy, b.CreatedAt = "", "", "", "", "", time.Time{}
	return reflect.DeepEqual(a, b)
}
func bounded(v string, n int) bool { v = strings.TrimSpace(v); return v != "" && len(v) <= n }
func boundedList(values []string, n int) bool {
	for _, value := range values {
		if !bounded(value, n) {
			return false
		}
	}
	return true
}
func contains(xs []string, v string) bool {
	for _, x := range xs {
		if x == v {
			return true
		}
	}
	return false
}
func overlaps(a, b []string) bool {
	for _, x := range a {
		if contains(b, x) {
			return true
		}
	}
	return false
}
func (s *Store) get(repo, id string) (Assessment, error) {
	if repo == "" || id == "" || strings.ContainsAny(repo+id, "/\\") {
		return Assessment{}, ErrNotFound
	}
	b, e := os.ReadFile(filepath.Join(s.root, repo, id+".json"))
	if errors.Is(e, os.ErrNotExist) {
		return Assessment{}, ErrNotFound
	}
	var x Assessment
	if e == nil {
		e = json.Unmarshal(b, &x)
	}
	return x, e
}
func (s *Store) write(x Assessment) error {
	b, e := json.MarshalIndent(x, "", "  ")
	if e != nil {
		return e
	}
	dir := filepath.Join(s.root, x.RepositoryID)
	if e = os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	f, e := os.CreateTemp(dir, ".assessment-*.tmp")
	if e != nil {
		return e
	}
	name := f.Name()
	defer os.Remove(name)
	_ = f.Chmod(0600)
	if _, e = f.Write(b); e == nil {
		e = f.Sync()
	}
	if ce := f.Close(); e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	return os.Rename(name, filepath.Join(dir, x.ID+".json"))
}
func (s *Store) lock(mode int) (func(), error) {
	f, e := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = syscall.Flock(int(f.Fd()), mode); e != nil {
		_ = f.Close()
		return nil, e
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}
func newID() string { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
