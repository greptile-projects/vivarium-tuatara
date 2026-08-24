// Package restructuringplans retains reviewable repository topology changes before migration.
package restructuringplans

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

var ErrNotFound = errors.New("restructuring plan not found")
var ErrInvalid = errors.New("invalid restructuring plan")
var ErrConflict = errors.New("restructuring plan request changed")
var ErrVersion = errors.New("restructuring plan version changed")

type SourceRepository struct {
	RepositoryID string `json:"repository_id"`
	Revision     string `json:"revision"`
	Role         string `json:"role"`
}
type Destination struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	OwnerIDs         []string `json:"owner_ids"`
	Visibility       string   `json:"visibility"`
	DefaultBranch    string   `json:"default_branch"`
	RetainedIdentity string   `json:"retained_identity,omitempty"`
}
type ContentMapping struct {
	ID                 string `json:"id"`
	SourceRepositoryID string `json:"source_repository_id"`
	SourcePath         string `json:"source_path"`
	DestinationID      string `json:"destination_id,omitempty"`
	DestinationPath    string `json:"destination_path,omitempty"`
	HistoryMode        string `json:"history_mode"`
	RetainIdentity     bool   `json:"retain_identity"`
	Disposition        string `json:"disposition"`
}
type InventoryItem struct {
	ID             string   `json:"id"`
	Kind           string   `json:"kind"`
	RepositoryID   string   `json:"repository_id"`
	ResourceID     string   `json:"resource_id"`
	Revision       string   `json:"revision"`
	OwnerIDs       []string `json:"owner_ids"`
	DestinationIDs []string `json:"destination_ids,omitempty"`
	Disposition    string   `json:"disposition"`
	State          string   `json:"state"`
	Summary        string   `json:"summary"`
	Citation       string   `json:"citation"`
}
type Finding struct {
	ID               string    `json:"id"`
	RequestID        string    `json:"request_id"`
	Version          int       `json:"version"`
	InventoryItemIDs []string  `json:"inventory_item_ids"`
	Body             string    `json:"body"`
	Citations        []string  `json:"citations"`
	ActorID          string    `json:"actor_id"`
	ActorKind        string    `json:"actor_kind"`
	CreatedAt        time.Time `json:"created_at"`
}
type CandidateRepository struct {
	ID               string   `json:"id"`
	DestinationID    string   `json:"destination_id"`
	DefaultBranch    string   `json:"default_branch"`
	Tip              string   `json:"tip"`
	Tree             string   `json:"tree"`
	ObjectCount      int      `json:"object_count"`
	SizeBytes        int64    `json:"size_bytes"`
	Mappings         []string `json:"mapping_ids"`
	SourceCommits    []string `json:"source_commits"`
	PreservedTags    []string `json:"preserved_tags,omitempty"`
	LicensePaths     []string `json:"license_paths,omitempty"`
	ProvenanceSHA256 string   `json:"provenance_sha256"`
	SignatureState   string   `json:"signature_state"`
}
type CandidateGap struct {
	Kind             string `json:"kind"`
	ResourceID       string `json:"resource_id"`
	State            string `json:"state"`
	Summary          string `json:"summary"`
	RequiredDecision string `json:"required_decision"`
}
type CandidateGapDecision struct {
	RequestID  string    `json:"request_id"`
	GapKind    string    `json:"gap_kind"`
	ResourceID string    `json:"resource_id"`
	Decision   string    `json:"decision"`
	Rationale  string    `json:"rationale"`
	ActorID    string    `json:"actor_id"`
	CreatedAt  time.Time `json:"created_at"`
}
type CandidateSet struct {
	ID                   string                 `json:"id"`
	RequestID            string                 `json:"request_id"`
	RequestDigest        string                 `json:"request_digest,omitempty"`
	PlanVersion          int                    `json:"plan_version"`
	Repositories         []CandidateRepository  `json:"repositories"`
	CrossRepositoryLinks []string               `json:"cross_repository_links,omitempty"`
	Gaps                 []CandidateGap         `json:"gaps,omitempty"`
	GapDecisions         []CandidateGapDecision `json:"gap_decisions,omitempty"`
	CreatedBy            string                 `json:"created_by"`
	CreatedAt            time.Time              `json:"created_at"`
	Authority            string                 `json:"authority"`
	Rehearsals           []Rehearsal            `json:"rehearsals,omitempty"`
}
type Scenario struct {
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	DestinationID  string `json:"destination_id"`
	Image          string `json:"image,omitempty"`
	Command        string `json:"command,omitempty"`
	Expectation    string `json:"expectation"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}
type Outcome struct {
	ScenarioID    string `json:"scenario_id"`
	Kind          string `json:"kind"`
	DestinationID string `json:"destination_id"`
	State         string `json:"state"`
	ExitCode      int    `json:"exit_code"`
	Output        string `json:"output,omitempty"`
	DurationMS    int64  `json:"duration_ms"`
}
type Rehearsal struct {
	ID                string     `json:"id"`
	RequestID         string     `json:"request_id"`
	Scenarios         []Scenario `json:"scenarios"`
	Outcomes          []Outcome  `json:"outcomes"`
	State             string     `json:"state"`
	CostUnits         float64    `json:"cost_units"`
	RequiredDecisions []string   `json:"required_decisions,omitempty"`
	CreatedBy         string     `json:"created_by"`
	CreatedAt         time.Time  `json:"created_at"`
}
type CollaborationDestination struct {
	DestinationID      string   `json:"destination_id"`
	ResourceID         string   `json:"resource_id"`
	Revision           string   `json:"revision"`
	OwnerIDs           []string `json:"owner_ids"`
	DependencyIDs      []string `json:"dependency_ids,omitempty"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
}
type CollaborationSnapshot struct {
	AuthorshipIDs      []string `json:"authorship_ids"`
	DiscussionIDs      []string `json:"discussion_ids,omitempty"`
	ReviewIDs          []string `json:"review_ids,omitempty"`
	DependencyIDs      []string `json:"dependency_ids,omitempty"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
}
type MappingDecision struct {
	RequestID      string    `json:"request_id"`
	ActorID        string    `json:"actor_id"`
	Decision       string    `json:"decision"`
	SourceRevision string    `json:"source_revision"`
	Note           string    `json:"note,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}
type CollaborationMapping struct {
	ID                 string                     `json:"id"`
	RequestID          string                     `json:"request_id"`
	InventoryItemID    string                     `json:"inventory_item_id"`
	Kind               string                     `json:"kind"`
	SourceRepositoryID string                     `json:"source_repository_id"`
	SourceResourceID   string                     `json:"source_resource_id"`
	SourceRevision     string                     `json:"source_revision"`
	Snapshot           CollaborationSnapshot      `json:"snapshot"`
	Destinations       []CollaborationDestination `json:"destinations,omitempty"`
	Disposition        string                     `json:"disposition"`
	State              string                     `json:"state"`
	BlockedReason      string                     `json:"blocked_reason,omitempty"`
	Embargoed          bool                       `json:"embargoed,omitempty"`
	Decisions          []MappingDecision          `json:"decisions,omitempty"`
	CreatedBy          string                     `json:"created_by"`
	CreatedAt          time.Time                  `json:"created_at"`
	Authority          string                     `json:"authority"`
}
type CompatibilityWindow struct {
	StartsAt time.Time `json:"starts_at"`
	EndsAt   time.Time `json:"ends_at"`
}
type ReplacementRemote struct {
	DestinationID string `json:"destination_id"`
	RemoteURL     string `json:"remote_url"`
	Ref           string `json:"ref,omitempty"`
}
type DependencyLinkMapping struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
}
type PropagationWork struct {
	ActorKind    string `json:"actor_kind"`
	RepositoryID string `json:"repository_id"`
	TaskID       string `json:"task_id,omitempty"`
	PullID       string `json:"pull_id,omitempty"`
	ReleaseID    string `json:"release_id,omitempty"`
}
type DependentEvent struct {
	RequestID  string    `json:"request_id"`
	ActorID    string    `json:"actor_id"`
	State      string    `json:"state"`
	Evidence   string    `json:"evidence,omitempty"`
	NextAction string    `json:"next_action"`
	CreatedAt  time.Time `json:"created_at"`
}
type DependentMigration struct {
	ID                  string                  `json:"id"`
	RequestID           string                  `json:"request_id"`
	Kind                string                  `json:"kind"`
	ResourceID          string                  `json:"resource_id"`
	OwnerID             string                  `json:"owner_id"`
	Audience            string                  `json:"audience"`
	State               string                  `json:"state"`
	CompatibilityWindow CompatibilityWindow     `json:"compatibility_window"`
	NextAction          string                  `json:"next_action"`
	ReplacementRemotes  []ReplacementRemote     `json:"replacement_remotes,omitempty"`
	Mappings            []DependencyLinkMapping `json:"mappings,omitempty"`
	Synchronization     []string                `json:"synchronization"`
	Propagation         *PropagationWork        `json:"propagation,omitempty"`
	Events              []DependentEvent        `json:"events,omitempty"`
	CreatedBy           string                  `json:"created_by"`
	CreatedAt           time.Time               `json:"created_at"`
	Authority           string                  `json:"authority"`
}
type CutoverApproval struct {
	RequestID     string    `json:"request_id"`
	ActorID       string    `json:"actor_id"`
	DestinationID string    `json:"destination_id"`
	Decision      string    `json:"decision"`
	CreatedAt     time.Time `json:"created_at"`
}
type CutoverDestination struct {
	DestinationID string `json:"destination_id"`
	RepositoryID  string `json:"repository_id,omitempty"`
	State         string `json:"state"`
	Revision      string `json:"revision,omitempty"`
	Health        string `json:"health"`
}
type CutoverObservation struct {
	RequestID  string    `json:"request_id"`
	ActorID    string    `json:"actor_id"`
	Kind       string    `json:"kind"`
	ResourceID string    `json:"resource_id"`
	State      string    `json:"state"`
	Evidence   string    `json:"evidence"`
	CreatedAt  time.Time `json:"created_at"`
}
type Cutover struct {
	ID            string               `json:"id"`
	RequestID     string               `json:"request_id"`
	CandidateID   string               `json:"candidate_id"`
	State         string               `json:"state"`
	SourceState   string               `json:"source_state"`
	PauseKinds    []string             `json:"pause_kinds"`
	CleanupPolicy string               `json:"cleanup_policy"`
	Approvals     []CutoverApproval    `json:"approvals,omitempty"`
	Destinations  []CutoverDestination `json:"destinations"`
	Observations  []CutoverObservation `json:"observations,omitempty"`
	Blockers      []string             `json:"blockers,omitempty"`
	CreatedBy     string               `json:"created_by"`
	CreatedAt     time.Time            `json:"created_at"`
	ActivatedAt   *time.Time           `json:"activated_at,omitempty"`
	CompletedAt   *time.Time           `json:"completed_at,omitempty"`
	Authority     string               `json:"authority"`
}
type Plan struct {
	ID                    string                 `json:"id"`
	RequestID             string                 `json:"request_id"`
	RequestDigest         string                 `json:"request_digest,omitempty"`
	RepositoryID          string                 `json:"repository_id"`
	Title                 string                 `json:"title"`
	Intent                string                 `json:"intent"`
	Sources               []SourceRepository     `json:"sources"`
	Destinations          []Destination          `json:"destinations"`
	Mappings              []ContentMapping       `json:"mappings"`
	Inventory             []InventoryItem        `json:"inventory"`
	Deadline              time.Time              `json:"deadline"`
	SuccessCriteria       []string               `json:"success_criteria"`
	RollbackLimits        []string               `json:"rollback_limits"`
	Findings              []Finding              `json:"findings,omitempty"`
	CandidateSets         []CandidateSet         `json:"candidate_sets,omitempty"`
	CollaborationMappings []CollaborationMapping `json:"collaboration_mappings,omitempty"`
	DependentMigrations   []DependentMigration   `json:"dependent_migrations,omitempty"`
	Cutover               *Cutover               `json:"cutover,omitempty"`
	Version               int                    `json:"version"`
	CreatedBy             string                 `json:"created_by"`
	CreatedAt             time.Time              `json:"created_at"`
	Authority             string                 `json:"authority"`
}

func (s *Store) StartCutover(repo, id, actor string, expected int, in Cutover) (Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return Plan{}, err
	}
	defer unlock()
	v, err := s.read(repo, id)
	if err != nil {
		return Plan{}, err
	}
	if v.Cutover != nil {
		if v.Cutover.RequestID == in.RequestID && v.Cutover.CandidateID == in.CandidateID {
			return v, nil
		}
		return Plan{}, ErrConflict
	}
	if expected != v.Version || !bounded(in.RequestID, 1, 200) || !map[string]bool{"read_only": true, "archive": true, "remove": true}[in.CleanupPolicy] || len(in.PauseKinds) == 0 {
		return Plan{}, ErrInvalid
	}
	var candidate *CandidateSet
	for i := range v.CandidateSets {
		if v.CandidateSets[i].ID == in.CandidateID {
			candidate = &v.CandidateSets[i]
		}
	}
	if candidate == nil || !allCandidateGapsDecided(*candidate) || len(candidate.Rehearsals) == 0 || candidate.Rehearsals[len(candidate.Rehearsals)-1].State != "passed" {
		return Plan{}, ErrInvalid
	}
	for _, m := range v.CollaborationMappings {
		if m.State != "approved" && m.State != "archived" {
			return Plan{}, ErrInvalid
		}
	}
	seen := map[string]bool{}
	for _, k := range in.PauseKinds {
		if seen[k] || !map[string]bool{"git": true, "collaboration": true, "automation": true, "release": true}[k] {
			return Plan{}, ErrInvalid
		}
		seen[k] = true
	}
	in.ID = randomID()
	in.State = "awaiting_approval"
	in.SourceState = "active"
	in.CreatedBy = actor
	in.CreatedAt = s.now()
	in.Approvals = nil
	in.Observations = nil
	in.Blockers = nil
	in.Destinations = nil
	for _, d := range candidate.Repositories {
		in.Destinations = append(in.Destinations, CutoverDestination{DestinationID: d.DestinationID, State: "candidate", Revision: d.Tip, Health: "pending"})
	}
	in.Authority = "cutover coordination only; each source and destination owner retains repository, Git, policy, collaboration, release, and cleanup authority"
	v.Cutover = &in
	v.Version++
	return v, s.write(v)
}

func allCandidateGapsDecided(candidate CandidateSet) bool {
	for _, gap := range candidate.Gaps {
		decided := false
		for _, decision := range candidate.GapDecisions {
			if decision.GapKind == gap.Kind && decision.ResourceID == gap.ResourceID {
				decided = true
				break
			}
		}
		if !decided {
			return false
		}
	}
	return true
}

func (s *Store) DecideCandidateGap(repo, id, candidateID, actor string, expected int, in CandidateGapDecision) (Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return Plan{}, err
	}
	defer unlock()
	v, err := s.read(repo, id)
	if err != nil {
		return Plan{}, err
	}
	if expected != v.Version {
		return Plan{}, ErrVersion
	}
	for i := range v.CandidateSets {
		candidate := &v.CandidateSets[i]
		if candidate.ID != candidateID {
			continue
		}
		for _, decision := range candidate.GapDecisions {
			if decision.RequestID == in.RequestID {
				if decision.ActorID == actor && decision.GapKind == in.GapKind && decision.ResourceID == in.ResourceID && decision.Decision == in.Decision && decision.Rationale == in.Rationale {
					return v, nil
				}
				return Plan{}, ErrConflict
			}
			if decision.GapKind == in.GapKind && decision.ResourceID == in.ResourceID {
				return Plan{}, ErrConflict
			}
		}
		found := false
		for _, gap := range candidate.Gaps {
			if gap.Kind == in.GapKind && gap.ResourceID == in.ResourceID {
				found = true
				break
			}
		}
		allowed := map[string]bool{"retain_external": true, "recreate": true, "exclude": true, "accept_shared": true}
		if !found || !allowed[in.Decision] || !bounded(in.RequestID, 1, 200) || !bounded(in.Rationale, 1, 1000) {
			return Plan{}, ErrInvalid
		}
		// A collision describes an invalid materialized tree and cannot be waived.
		if in.GapKind == "path_collision" {
			return Plan{}, ErrInvalid
		}
		in.ActorID, in.CreatedAt = actor, s.now()
		candidate.GapDecisions = append(candidate.GapDecisions, in)
		v.Version++
		return v, s.write(v)
	}
	return Plan{}, ErrNotFound
}

func (s *Store) ApproveCutover(repo, id, actor string, expected int, in CutoverApproval) (Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, e := s.lockRoot()
	if e != nil {
		return Plan{}, e
	}
	defer unlock()
	v, e := s.read(repo, id)
	if e != nil {
		return Plan{}, e
	}
	if v.Cutover == nil {
		return Plan{}, ErrNotFound
	}
	for _, x := range v.Cutover.Approvals {
		if x.RequestID == in.RequestID {
			if x.ActorID == actor && x.DestinationID == in.DestinationID && x.Decision == in.Decision {
				return v, nil
			}
			return Plan{}, ErrConflict
		}
	}
	if expected != v.Version || v.Cutover.State != "awaiting_approval" || in.Decision != "approve" || !bounded(in.RequestID, 1, 200) {
		return Plan{}, ErrInvalid
	}
	var owners []string
	for _, d := range v.Destinations {
		if d.ID == in.DestinationID {
			owners = d.OwnerIDs
		}
	}
	if !contains(owners, actor) {
		return Plan{}, ErrInvalid
	}
	in.ActorID = actor
	in.CreatedAt = s.now()
	v.Cutover.Approvals = append(v.Cutover.Approvals, in)
	v.Version++
	return v, s.write(v)
}

func (s *Store) ActivateCutover(repo, id, actor string, expected int, repositories map[string]string) (Plan, error) {
	return s.ActivateCutoverWith(repo, id, actor, expected, repositories, nil)
}
func (s *Store) ActivateCutoverWith(repo, id, actor string, expected int, repositories map[string]string, publish func() error) (Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, e := s.lockRoot()
	if e != nil {
		return Plan{}, e
	}
	defer unlock()
	v, e := s.read(repo, id)
	if e != nil {
		return Plan{}, e
	}
	if v.Cutover == nil {
		return Plan{}, ErrNotFound
	}
	if expected != v.Version || (v.Cutover.State != "awaiting_approval" && v.Cutover.State != "publication_blocked") || v.CreatedBy != actor {
		return Plan{}, ErrInvalid
	}
	for _, d := range v.Destinations {
		ok := false
		for _, a := range v.Cutover.Approvals {
			if a.DestinationID == d.ID && contains(d.OwnerIDs, a.ActorID) {
				ok = true
			}
		}
		if !ok || repositories[d.ID] == "" {
			return Plan{}, ErrInvalid
		}
	}
	now := s.now()
	v.Cutover.State = "publishing"
	v.Cutover.SourceState = "writes_paused"
	v.Cutover.ActivatedAt = &now
	for i := range v.Cutover.Destinations {
		v.Cutover.Destinations[i].RepositoryID = repositories[v.Cutover.Destinations[i].DestinationID]
		v.Cutover.Destinations[i].State = "publishing"
		v.Cutover.Destinations[i].Health = "pending"
	}
	v.Version++
	if e = s.write(v); e != nil {
		return Plan{}, e
	}
	if publish != nil {
		if e = publish(); e != nil {
			v.Cutover.State = "publication_blocked"
			v.Cutover.Blockers = append(v.Cutover.Blockers, "destination publication or target-checked compensation failed; source writes remain paused and exact activation must be retried")
			for i := range v.Cutover.Destinations {
				v.Cutover.Destinations[i].State = "publication_uncertain"
				v.Cutover.Destinations[i].Health = "blocked"
			}
			v.Version++
			if writeErr := s.write(v); writeErr != nil {
				return Plan{}, writeErr
			}
			return v, e
		}
	}
	v.Cutover.State = "active"
	v.Cutover.Blockers = nil
	for i := range v.Cutover.Destinations {
		v.Cutover.Destinations[i].State = "authoritative"
		v.Cutover.Destinations[i].Health = "observing"
	}
	v.Version++
	return v, s.write(v)
}

func (s *Store) ObserveCutover(repo, id, actor string, expected int, in CutoverObservation) (Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, e := s.lockRoot()
	if e != nil {
		return Plan{}, e
	}
	defer unlock()
	v, e := s.read(repo, id)
	if e != nil {
		return Plan{}, e
	}
	if v.Cutover == nil {
		return Plan{}, ErrNotFound
	}
	for _, x := range v.Cutover.Observations {
		if x.RequestID == in.RequestID {
			if x.ActorID == actor && x.Kind == in.Kind && x.ResourceID == in.ResourceID && x.State == in.State && x.Evidence == in.Evidence {
				return v, nil
			}
			return Plan{}, ErrConflict
		}
	}
	if expected != v.Version || v.Cutover.State != "active" || !map[string]bool{"build": true, "release": true, "permission": true, "link": true, "consumer": true, "contribution": true, "git_traffic": true, "late_write": true}[in.Kind] || !map[string]bool{"passed": true, "failed": true, "residual": true}[in.State] || !bounded(in.ResourceID, 1, 500) || !bounded(in.Evidence, 1, 1000) {
		return Plan{}, ErrInvalid
	}
	in.ActorID = actor
	in.CreatedAt = s.now()
	v.Cutover.Observations = append(v.Cutover.Observations, in)
	v.Version++
	return v, s.write(v)
}

func (s *Store) FinishCutover(repo, id, actor string, expected int, rollback bool) (Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, e := s.lockRoot()
	if e != nil {
		return Plan{}, e
	}
	defer unlock()
	v, e := s.read(repo, id)
	if e != nil {
		return Plan{}, e
	}
	if v.Cutover == nil {
		return Plan{}, ErrNotFound
	}
	if expected != v.Version || (v.Cutover.State != "active" && !(rollback && v.Cutover.State == "publication_blocked")) || v.CreatedBy != actor {
		return Plan{}, ErrInvalid
	}
	if rollback {
		v.Cutover.State = "rolled_back"
		v.Cutover.SourceState = "active"
		for i := range v.Cutover.Destinations {
			if v.Cutover.Destinations[i].State == "publication_uncertain" {
				v.Cutover.Destinations[i].State = "independently_changed"
				v.Cutover.Destinations[i].Health = "requires_destination_owner"
			} else {
				v.Cutover.Destinations[i].State = "inactive"
			}
		}
		v.Version++
		return v, s.write(v)
	}
	requiredKinds := []string{"build", "release", "permission", "link", "consumer", "contribution", "git_traffic"}
	latest := map[string]CutoverObservation{}
	for _, o := range v.Cutover.Observations {
		latest[o.Kind+"\x00"+o.ResourceID] = o
	}
	for _, destination := range v.Cutover.Destinations {
		for _, kind := range requiredKinds {
			o, ok := latest[kind+"\x00"+destination.DestinationID]
			if !ok || o.State != "passed" {
				return Plan{}, ErrInvalid
			}
		}
	}
	for _, o := range latest {
		if o.Kind == "late_write" && o.State != "passed" {
			return Plan{}, ErrInvalid
		}
	}
	for _, d := range v.DependentMigrations {
		if d.State != "adopted" {
			return Plan{}, ErrInvalid
		}
	}
	now := s.now()
	v.Cutover.State = "completed"
	v.Cutover.SourceState = v.Cutover.CleanupPolicy
	v.Cutover.CompletedAt = &now
	for i := range v.Cutover.Destinations {
		v.Cutover.Destinations[i].Health = "healthy"
	}
	v.Version++
	return v, s.write(v)
}

func (s *Store) AddDependentMigration(repo, id, actor string, expected int, in DependentMigration) (Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return Plan{}, err
	}
	defer unlock()
	v, err := s.read(repo, id)
	if err != nil {
		return Plan{}, err
	}
	for _, x := range v.DependentMigrations {
		if x.RequestID == in.RequestID {
			in.ID = x.ID
			in.CreatedBy = x.CreatedBy
			in.CreatedAt = x.CreatedAt
			in.Events = x.Events
			in.Authority = x.Authority
			a, _ := json.Marshal(x)
			b, _ := json.Marshal(in)
			if string(a) == string(b) {
				return v, nil
			}
			return Plan{}, ErrConflict
		}
	}
	if expected != v.Version {
		return Plan{}, ErrVersion
	}
	if !validDependentMigration(v, in) {
		return Plan{}, ErrInvalid
	}
	in.ID = randomID()
	in.CreatedBy = actor
	in.CreatedAt = s.now()
	in.Events = nil
	in.Authority = "coordination and discovery only; this record grants no Git, package, API, extension, workflow, documentation, deployment, federation, pull, release, or consumer authority"
	v.DependentMigrations = append(v.DependentMigrations, in)
	v.Version++
	return v, s.write(v)
}

func (s *Store) AddDependentEvent(repo, id, migrationID, actor string, expected int, in DependentEvent) (Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return Plan{}, err
	}
	defer unlock()
	v, err := s.read(repo, id)
	if err != nil {
		return Plan{}, err
	}
	for i := range v.DependentMigrations {
		m := &v.DependentMigrations[i]
		if m.ID != migrationID {
			continue
		}
		for _, e := range m.Events {
			if e.RequestID == in.RequestID {
				if e.ActorID == actor && e.State == in.State && e.Evidence == in.Evidence && e.NextAction == in.NextAction {
					return v, nil
				}
				return Plan{}, ErrConflict
			}
		}
		if expected != v.Version {
			return Plan{}, ErrVersion
		}
		if actor != m.OwnerID || !validDependentState(in.State) || !bounded(in.RequestID, 1, 200) || !bounded(in.Evidence, 0, 1000) || !bounded(in.NextAction, 1, 1000) || (in.State == "adopted" && strings.TrimSpace(in.Evidence) == "") {
			return Plan{}, ErrInvalid
		}
		in.ActorID = actor
		in.CreatedAt = s.now()
		m.Events = append(m.Events, in)
		m.State = in.State
		m.NextAction = in.NextAction
		v.Version++
		return v, s.write(v)
	}
	return Plan{}, ErrNotFound
}

func validDependentState(v string) bool {
	return map[string]bool{"discovered": true, "planned": true, "in_progress": true, "adopted": true, "blocked": true, "unavailable": true, "rejected": true, "stale_credentials": true, "unmigrated": true}[v]
}
func validDependentMigration(p Plan, m DependentMigration) bool {
	kinds := map[string]bool{"clone": true, "fork": true, "package": true, "api": true, "dependency": true, "extension": true, "workflow": true, "documentation_link": true, "deployment": true, "federated_follower": true}
	if !bounded(m.RequestID, 1, 200) || !kinds[m.Kind] || !bounded(m.ResourceID, 1, 500) || !bounded(m.OwnerID, 1, 200) || !map[string]bool{"public": true, "participants": true, "owner": true}[m.Audience] || !validDependentState(m.State) || !bounded(m.NextAction, 1, 1000) || len(m.Synchronization) == 0 || m.CompatibilityWindow.StartsAt.IsZero() || !m.CompatibilityWindow.EndsAt.After(m.CompatibilityWindow.StartsAt) {
		return false
	}
	seen := map[string]bool{}
	for _, r := range m.ReplacementRemotes {
		u, err := url.Parse(r.RemoteURL)
		if seen[r.DestinationID] || !containsDestination(p.Destinations, r.DestinationID) || !bounded(r.RemoteURL, 1, 1000) || err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" || u.User != nil || r.RemoteURL == m.ResourceID {
			return false
		}
		seen[r.DestinationID] = true
	}
	for _, x := range m.Mappings {
		if !bounded(x.From, 1, 500) || !bounded(x.To, 1, 500) || x.From == x.To || !map[string]bool{"dependency": true, "link": true, "package": true, "api": true}[x.Kind] {
			return false
		}
	}
	for _, x := range m.Synchronization {
		if !bounded(x, 1, 1000) {
			return false
		}
	}
	if (m.Kind == "clone" || m.Kind == "fork" || m.Kind == "federated_follower") && len(m.ReplacementRemotes) == 0 {
		return false
	}
	if m.Propagation != nil && (!map[string]bool{"human": true, "agent": true}[m.Propagation.ActorKind] || !bounded(m.Propagation.RepositoryID, 1, 200) || (m.Propagation.TaskID == "" && m.Propagation.PullID == "" && m.Propagation.ReleaseID == "")) {
		return false
	}
	return true
}

func (s *Store) AddCollaborationMapping(repo, id, actor string, expected int, in CollaborationMapping) (Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return Plan{}, err
	}
	defer unlock()
	v, err := s.read(repo, id)
	if err != nil {
		return Plan{}, err
	}
	for _, x := range v.CollaborationMappings {
		if x.RequestID == in.RequestID {
			a, _ := json.Marshal(x)
			in.ID = x.ID
			in.State = x.State
			in.BlockedReason = x.BlockedReason
			in.Decisions = x.Decisions
			in.CreatedBy = x.CreatedBy
			in.CreatedAt = x.CreatedAt
			in.Authority = x.Authority
			b, _ := json.Marshal(in)
			if string(a) == string(b) {
				return v, nil
			}
			return Plan{}, ErrConflict
		}
	}
	if expected != v.Version {
		return Plan{}, ErrVersion
	}
	var item *InventoryItem
	for i := range v.Inventory {
		if v.Inventory[i].ID == in.InventoryItemID {
			item = &v.Inventory[i]
			break
		}
	}
	if item == nil || item.RepositoryID != in.SourceRepositoryID || item.ResourceID != in.SourceResourceID || item.Revision != in.SourceRevision || !validCollaborationMapping(v, in) {
		return Plan{}, ErrInvalid
	}
	// Inventory ownership is frozen when the exact source resource is admitted.
	// A later mapping proposer cannot narrow that required-consent set.
	if !sameStringSet(in.Snapshot.AuthorshipIDs, item.OwnerIDs) {
		return Plan{}, ErrInvalid
	}
	in.Snapshot.AuthorshipIDs = append([]string(nil), item.OwnerIDs...)
	in.ID = randomID()
	in.CreatedBy = actor
	in.CreatedAt = s.now()
	in.Decisions = nil
	in.Authority = "retained collaboration intent only; mapping grants no source or destination Git, review, queue, session, workspace, task, issue, or independently owned resource authority"
	if in.Embargoed || item.State == "inaccessible" {
		in.State = "blocked"
	} else if in.Disposition == "archive" {
		in.State = "archived"
	} else {
		in.State = "proposed"
	}
	v.CollaborationMappings = append(v.CollaborationMappings, in)
	v.Version++
	return v, s.write(v)
}

func (s *Store) DecideCollaborationMapping(repo, id, mappingID, actor string, expected int, in MappingDecision) (Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return Plan{}, err
	}
	defer unlock()
	v, err := s.read(repo, id)
	if err != nil {
		return Plan{}, err
	}
	if expected != v.Version {
		return Plan{}, ErrVersion
	}
	for i := range v.CollaborationMappings {
		m := &v.CollaborationMappings[i]
		if m.ID != mappingID {
			continue
		}
		for _, d := range m.Decisions {
			if d.RequestID == in.RequestID {
				if d.ActorID == actor && d.Decision == in.Decision && d.SourceRevision == in.SourceRevision && d.Note == in.Note {
					return v, nil
				}
				return Plan{}, ErrConflict
			}
		}
		if m.State != "proposed" {
			return Plan{}, ErrInvalid
		}
		if (in.Decision != "approve" && in.Decision != "reject") || in.SourceRevision != m.SourceRevision || !bounded(in.RequestID, 1, 200) || !bounded(in.Note, 0, 1000) || !contains(m.Snapshot.AuthorshipIDs, actor) {
			return Plan{}, ErrInvalid
		}
		in.ActorID = actor
		in.CreatedAt = s.now()
		m.Decisions = append(m.Decisions, in)
		if in.Decision == "reject" {
			m.State = "blocked"
			m.BlockedReason = "a source participant rejected the proposed mapping"
		} else if allApproved(*m) {
			m.State = "approved"
		}
		v.Version++
		return v, s.write(v)
	}
	return Plan{}, ErrNotFound
}

func validCollaborationMapping(p Plan, m CollaborationMapping) bool {
	allowed := map[string]bool{"branch": true, "pull_request": true, "issue": true, "proposal": true, "task": true, "decision": true, "check": true, "session": true, "workspace": true, "queue": true}
	if !bounded(m.RequestID, 1, 200) || !allowed[m.Kind] || len(m.SourceRevision) != 40 || len(m.Snapshot.AuthorshipIDs) == 0 || len(m.Snapshot.AcceptanceCriteria) == 0 || (m.Disposition != "move" && m.Disposition != "divide" && m.Disposition != "archive") {
		return false
	}
	if m.Disposition != "archive" && len(m.Destinations) == 0 {
		return false
	}
	seen := map[string]bool{}
	for _, d := range m.Destinations {
		if seen[d.DestinationID] || !containsDestination(p.Destinations, d.DestinationID) || !bounded(d.ResourceID, 1, 500) || len(d.Revision) != 40 || len(d.OwnerIDs) == 0 || len(d.AcceptanceCriteria) == 0 {
			return false
		}
		seen[d.DestinationID] = true
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
func containsDestination(xs []Destination, v string) bool {
	for _, x := range xs {
		if x.ID == v {
			return true
		}
	}
	return false
}
func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := make(map[string]bool, len(a))
	for _, x := range a {
		if seen[x] {
			return false
		}
		seen[x] = true
	}
	for _, x := range b {
		if !seen[x] {
			return false
		}
	}
	return true
}
func allApproved(m CollaborationMapping) bool {
	for _, decision := range m.Decisions {
		if decision.Decision == "reject" {
			return false
		}
	}
	for _, owner := range m.Snapshot.AuthorshipIDs {
		ok := false
		for _, d := range m.Decisions {
			if d.ActorID == owner && d.Decision == "approve" {
				ok = true
			}
		}
		if !ok {
			return false
		}
	}
	return true
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

// Create deliberately rejects unresolved ownership. Public callers must enter
// through CreateResolved after authoritative resource stores have supplied the
// exact consent sets; accepting Plan.Inventory directly would make OwnerIDs an
// authorization assertion.
func (s *Store) Create(v Plan, actor, digest string) (Plan, error) {
	return Plan{}, ErrInvalid
}

func (s *Store) CreateResolved(v Plan, actor, digest string, owners map[string][]string) (Plan, error) {
	for i := range v.Inventory {
		resolved := owners[v.Inventory[i].ID]
		if len(resolved) == 0 || !sameStringSet(v.Inventory[i].OwnerIDs, resolved) {
			return Plan{}, ErrInvalid
		}
		v.Inventory[i].OwnerIDs = append([]string(nil), resolved...)
	}
	if validate(v) != nil {
		return Plan{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return Plan{}, err
	}
	defer unlock()
	xs, err := s.list(v.RepositoryID)
	if err != nil {
		return Plan{}, err
	}
	for _, x := range xs {
		if x.RequestID == v.RequestID {
			if x.RequestDigest != digest {
				return Plan{}, ErrConflict
			}
			return x, nil
		}
	}
	v.ID = randomID()
	v.RequestDigest = digest
	v.Version = 1
	v.CreatedBy = actor
	v.CreatedAt = s.now()
	v.Authority = "coordination only; this plan grants no repository creation, Git, resource migration, ownership, policy, visibility, or destination write authority"
	return v, s.write(v)
}
func (s *Store) Get(repo, id string) (Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return Plan{}, err
	}
	defer unlock()
	return s.read(repo, id)
}
func (s *Store) Reconcile(repo, requestID, digest string) (Plan, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return Plan{}, false, err
	}
	defer unlock()
	xs, err := s.list(repo)
	if err != nil {
		return Plan{}, false, err
	}
	for _, x := range xs {
		if x.RequestID == requestID {
			if x.RequestDigest != digest {
				return Plan{}, true, ErrConflict
			}
			return x, true, nil
		}
	}
	return Plan{}, false, nil
}
func (s *Store) List(repo string) ([]Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return nil, err
	}
	defer unlock()
	xs, e := s.list(repo)
	sort.Slice(xs, func(i, j int) bool { return xs[i].CreatedAt.After(xs[j].CreatedAt) })
	return xs, e
}
func (s *Store) AddFinding(repo, id, actor, kind string, in Finding) (Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return Plan{}, err
	}
	defer unlock()
	v, e := s.read(repo, id)
	if e != nil {
		return Plan{}, e
	}
	for _, x := range v.Findings {
		if x.RequestID == in.RequestID {
			if sameFinding(x, in) {
				return v, nil
			}
			return Plan{}, ErrConflict
		}
	}
	if in.Version != v.Version || strings.TrimSpace(in.RequestID) == "" || len(in.InventoryItemIDs) == 0 || len(in.Citations) == 0 || !bounded(in.Body, 1, 4000) {
		if in.Version != v.Version {
			return Plan{}, ErrVersion
		}
		return Plan{}, ErrInvalid
	}
	ids := map[string]bool{}
	for _, x := range v.Inventory {
		ids[x.ID] = true
	}
	for _, x := range in.InventoryItemIDs {
		if !ids[x] {
			return Plan{}, ErrInvalid
		}
	}
	for _, x := range in.Citations {
		if !bounded(x, 1, 500) {
			return Plan{}, ErrInvalid
		}
	}
	in.ID = randomID()
	in.ActorID = actor
	in.ActorKind = kind
	in.CreatedAt = s.now()
	v.Findings = append(v.Findings, in)
	v.Version++
	return v, s.write(v)
}
func (s *Store) AddCandidateSet(repo, id, actor string, expected int, in CandidateSet) (Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return Plan{}, err
	}
	defer unlock()
	v, err := s.read(repo, id)
	if err != nil {
		return Plan{}, err
	}
	for _, x := range v.CandidateSets {
		if x.RequestID == in.RequestID {
			if x.RequestDigest == in.RequestDigest {
				return v, nil
			}
			return Plan{}, ErrConflict
		}
	}
	if expected != v.Version || !bounded(in.RequestID, 1, 200) || len(in.Repositories) == 0 {
		if expected != v.Version {
			return Plan{}, ErrVersion
		}
		return Plan{}, ErrInvalid
	}
	if in.ID == "" {
		in.ID = randomID()
	} else if !safeID(in.ID) {
		return Plan{}, ErrInvalid
	}
	in.PlanVersion = v.Version
	in.CreatedBy = actor
	in.CreatedAt = s.now()
	in.Authority = "immutable rehearsal material only; candidates grant no destination repository, Git, collaboration, resource migration, or publication authority"
	v.CandidateSets = append(v.CandidateSets, in)
	v.Version++
	return v, s.write(v)
}

// CreateCandidateSet serializes reconciliation, assembly publication, and
// ledger registration under the cross-process root lock. This prevents a
// losing plan-version update from leaving an unregistered bare repository and
// makes overlapping exact requests reconcile before filesystem publication.
func (s *Store) CreateCandidateSet(repo, id, actor string, expected int, requestID, digest string, assemble func(Plan) (CandidateSet, error)) (Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return Plan{}, err
	}
	defer unlock()
	v, err := s.read(repo, id)
	if err != nil {
		return Plan{}, err
	}
	for _, x := range v.CandidateSets {
		if x.RequestID != requestID {
			continue
		}
		if x.RequestDigest != digest {
			return Plan{}, ErrConflict
		}
		return v, nil
	}
	if expected != v.Version {
		return Plan{}, ErrVersion
	}
	in, err := assemble(v)
	cleanup := func() {
		if safeID(in.ID) {
			_ = os.RemoveAll(filepath.Dir(s.CandidatePath(repo, id, in.ID, "unused")))
		}
	}
	if err != nil {
		cleanup()
		return Plan{}, err
	}
	if !safeID(in.ID) || in.RequestID != requestID || in.RequestDigest != digest || len(in.Repositories) == 0 {
		cleanup()
		return Plan{}, ErrInvalid
	}
	in.PlanVersion = v.Version
	in.CreatedBy = actor
	in.CreatedAt = s.now()
	in.Authority = "immutable rehearsal material only; candidates grant no destination repository, Git, collaboration, resource migration, or publication authority"
	v.CandidateSets = append(v.CandidateSets, in)
	v.Version++
	if err = s.write(v); err != nil {
		cleanup()
		return Plan{}, err
	}
	return v, nil
}
func (s *Store) AddRehearsal(repo, id, candidateID, actor string, expected int, in Rehearsal) (Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	unlock, err := s.lockRoot()
	if err != nil {
		return Plan{}, err
	}
	defer unlock()
	v, err := s.read(repo, id)
	if err != nil {
		return Plan{}, err
	}
	if expected != v.Version {
		return Plan{}, ErrVersion
	}
	for i := range v.CandidateSets {
		c := &v.CandidateSets[i]
		if c.ID != candidateID {
			continue
		}
		for _, x := range c.Rehearsals {
			if x.RequestID == in.RequestID {
				a, _ := json.Marshal(x.Scenarios)
				b, _ := json.Marshal(in.Scenarios)
				if string(a) == string(b) {
					return v, nil
				}
				return Plan{}, ErrConflict
			}
		}
		in.ID = randomID()
		in.CreatedBy = actor
		in.CreatedAt = s.now()
		c.Rehearsals = append(c.Rehearsals, in)
		v.Version++
		return v, s.write(v)
	}
	return Plan{}, ErrNotFound
}
func (s *Store) CandidatePath(repo, plan, candidate, destination string) string {
	return filepath.Join(s.root, "candidates", repo, plan, candidate, destination+".git")
}
func sameFinding(a, b Finding) bool {
	return a.Version == b.Version && strings.Join(a.InventoryItemIDs, "\x00") == strings.Join(b.InventoryItemIDs, "\x00") && a.Body == b.Body && strings.Join(a.Citations, "\x00") == strings.Join(b.Citations, "\x00")
}

var kinds = map[string]bool{"ref": true, "pull_request": true, "issue": true, "task": true, "release": true, "package": true, "documentation": true, "policy": true, "workspace": true, "automation": true, "consumer": true, "federated_relationship": true}

func validate(v Plan) error {
	if !bounded(v.RequestID, 1, 200) || !bounded(v.RepositoryID, 1, 200) || !bounded(v.Title, 1, 300) || !bounded(v.Intent, 1, 4000) || len(v.Sources) == 0 || len(v.Destinations) == 0 || len(v.Destinations) > 20 || len(v.Mappings) == 0 || len(v.Inventory) == 0 || v.Deadline.IsZero() || len(v.SuccessCriteria) == 0 || len(v.RollbackLimits) == 0 {
		return ErrInvalid
	}
	src, dst, inv := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, x := range v.Sources {
		if !bounded(x.RepositoryID, 1, 200) || len(x.Revision) != 40 || (x.Role != "primary" && x.Role != "contributing") || src[x.RepositoryID] {
			return ErrInvalid
		}
		src[x.RepositoryID] = true
	}
	for _, x := range v.Destinations {
		if !safeID(x.ID) || !bounded(x.Name, 1, 200) || len(x.OwnerIDs) == 0 || (x.Visibility != "public" && x.Visibility != "private" && x.Visibility != "internal") || !safeRef(x.DefaultBranch) || dst[x.ID] {
			return ErrInvalid
		}
		dst[x.ID] = true
	}
	for _, x := range v.Mappings {
		if !safeID(x.ID) || !src[x.SourceRepositoryID] || !safePath(x.SourcePath) || (x.DestinationPath != "" && !safePath(x.DestinationPath)) || (x.Disposition != "move" && x.Disposition != "copy" && x.Disposition != "remain") || (x.HistoryMode != "full" && x.HistoryMode != "selected" && x.HistoryMode != "none") || (x.Disposition != "remain" && !dst[x.DestinationID]) {
			return ErrInvalid
		}
	}
	covered := map[string]bool{}
	for _, x := range v.Inventory {
		if !bounded(x.ID, 1, 100) || inv[x.ID] || !kinds[x.Kind] || !src[x.RepositoryID] || len(x.Revision) != 40 || !bounded(x.ResourceID, 1, 500) || len(x.OwnerIDs) == 0 || !bounded(x.Summary, 1, 1000) || !bounded(x.Citation, 1, 500) || (x.State != "resolved" && x.State != "inaccessible" && x.State != "ambiguous" && x.State != "shared") || (x.Disposition != "move" && x.Disposition != "remain" && x.Disposition != "divide" && x.Disposition != "unknown") {
			return ErrInvalid
		}
		for _, d := range x.DestinationIDs {
			if !dst[d] {
				return ErrInvalid
			}
		}
		inv[x.ID] = true
		covered[x.Kind] = true
	}
	for k := range kinds {
		if !covered[k] {
			return ErrInvalid
		}
	}
	for _, x := range append(append([]string{}, v.SuccessCriteria...), v.RollbackLimits...) {
		if !bounded(x, 1, 1000) {
			return ErrInvalid
		}
	}
	return nil
}
func bounded(s string, min, max int) bool {
	n := len(strings.TrimSpace(s))
	return n >= min && n <= max
}
func safeID(v string) bool {
	if !bounded(v, 1, 100) {
		return false
	}
	for _, r := range v {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}
func safePath(v string) bool {
	if !bounded(v, 1, 1000) || filepath.IsAbs(v) {
		return false
	}
	clean := filepath.Clean(v)
	return clean != ".." && !strings.HasPrefix(clean, ".."+string(filepath.Separator))
}
func safeRef(v string) bool {
	return bounded(v, 1, 200) && !strings.ContainsAny(v, " ~^:?*[\\") && !strings.Contains(v, "..") && !strings.HasPrefix(v, "/") && !strings.HasSuffix(v, "/") && !strings.HasSuffix(v, ".")
}
func randomID() string                       { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func (s *Store) dir(repo string) string      { return filepath.Join(s.root, repo) }
func (s *Store) path(repo, id string) string { return filepath.Join(s.dir(repo), id+".json") }
func (s *Store) lockRoot() (func(), error) {
	f, err := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}
func (s *Store) write(v Plan) error {
	if e := os.MkdirAll(s.dir(v.RepositoryID), 0700); e != nil {
		return e
	}
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(s.dir(v.RepositoryID), ".plan-")
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
	if closeErr := tmp.Close(); e == nil {
		e = closeErr
	}
	if e != nil {
		return e
	}
	return os.Rename(name, s.path(v.RepositoryID, v.ID))
}
func (s *Store) read(repo, id string) (Plan, error) {
	var v Plan
	b, e := os.ReadFile(s.path(repo, id))
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
func (s *Store) list(repo string) ([]Plan, error) {
	entries, e := os.ReadDir(s.dir(repo))
	if os.IsNotExist(e) {
		return []Plan{}, nil
	}
	if e != nil {
		return nil, e
	}
	out := []Plan{}
	for _, x := range entries {
		if x.IsDir() || filepath.Ext(x.Name()) != ".json" {
			continue
		}
		v, e := s.read(repo, strings.TrimSuffix(x.Name(), ".json"))
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, nil
}
