// Package changestacks persists ordered, revision-exact collaboration outcomes.
package changestacks

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

var ErrNotFound = errors.New("change stack not found")
var ErrInvalid = errors.New("invalid change stack")

type Permission struct {
	Read    bool `json:"read"`
	Publish bool `json:"publish"`
	Review  bool `json:"review"`
	Push    bool `json:"push"`
}

type Scope struct {
	CommitCount int      `json:"commit_count"`
	Files       []string `json:"files"`
	Additions   int      `json:"additions"`
	Deletions   int      `json:"deletions"`
}

type Diagnostic struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Blocking bool   `json:"blocking"`
}

type Member struct {
	ID                   string            `json:"id"`
	Position             int               `json:"position"`
	Title                string            `json:"title"`
	SourceRepositoryID   string            `json:"source_repository_id,omitempty"`
	SourceBranch         string            `json:"source_branch"`
	PullRequestID        string            `json:"pull_request_id,omitempty"`
	Revision             string            `json:"revision,omitempty"`
	BaseRevision         string            `json:"base_revision,omitempty"`
	ExpectedBaseRevision string            `json:"expected_base_revision,omitempty"`
	DependsOn            []string          `json:"depends_on,omitempty"`
	AcceptanceCriteria   []string          `json:"acceptance_criteria"`
	Authors              []string          `json:"authors"`
	CommitIDs            []string          `json:"commit_ids"`
	IndividualScope      Scope             `json:"individual_scope"`
	CumulativeScope      Scope             `json:"cumulative_scope"`
	Permissions          Permission        `json:"effective_permissions"`
	Diagnostics          []Diagnostic      `json:"diagnostics"`
	ReviewState          string            `json:"review_state"`
	PublishedAt          *time.Time        `json:"published_at,omitempty"`
	Acknowledgements     []Acknowledgement `json:"owner_acknowledgements,omitempty"`
}

// RevisionLineage preserves the review identity that preceded an applied
// restack. Rewritten commits are new Git objects; they do not replace the
// attributable revision that collaborators originally published.
type RevisionLineage struct {
	Revision     string    `json:"revision"`
	BaseRevision string    `json:"base_revision"`
	SucceededBy  string    `json:"succeeded_by,omitempty"`
	RestackID    string    `json:"restack_id"`
	ChangedBy    string    `json:"changed_by"`
	ChangedAt    time.Time `json:"changed_at"`
}

type RestackImpact struct {
	ReviewsInvalidated int `json:"reviews_invalidated"`
	ChecksInvalidated  int `json:"checks_invalidated"`
}

type RestackMember struct {
	Member                Member            `json:"member"`
	Action                string            `json:"action"`
	OldPosition           int               `json:"old_position,omitempty"`
	OldRevision           string            `json:"old_revision,omitempty"`
	ExpectedBranchTip     string            `json:"expected_branch_tip,omitempty"`
	CandidateRevision     string            `json:"candidate_revision,omitempty"`
	CandidateBase         string            `json:"candidate_base,omitempty"`
	RewrittenCommits      map[string]string `json:"rewritten_commits,omitempty"`
	Impact                RestackImpact     `json:"impact"`
	PublishedBranchUpdate bool              `json:"published_branch_update"`
	Diagnostics           []Diagnostic      `json:"diagnostics"`
}

type Restack struct {
	ID             string          `json:"id"`
	RequestID      string          `json:"request_id"`
	RequestDigest  string          `json:"request_digest"`
	Status         string          `json:"status"`
	TargetRevision string          `json:"target_revision"`
	Members        []RestackMember `json:"members"`
	Removed        []Member        `json:"removed_members,omitempty"`
	Diagnostics    []Diagnostic    `json:"diagnostics"`
	CreatedBy      string          `json:"created_by"`
	CreatedAt      time.Time       `json:"created_at"`
	AppliedBy      string          `json:"applied_by,omitempty"`
	AppliedAt      *time.Time      `json:"applied_at,omitempty"`
	Authority      string          `json:"authority"`
}

// Acknowledgement is an affected repository owner's decision against one
// exact layer and the exact upstream stack revisions visible at that time.
type Acknowledgement struct {
	ID                string            `json:"id"`
	MemberID          string            `json:"member_id"`
	Revision          string            `json:"revision"`
	UpstreamRevisions map[string]string `json:"upstream_revisions"`
	Decision          string            `json:"decision"`
	Note              string            `json:"note,omitempty"`
	OwnerID           string            `json:"owner_id"`
	CreatedAt         time.Time         `json:"created_at"`
}

// Assignment preserves who controls one layer without transferring the
// source repository or branch authority attached to that layer.
type Assignment struct {
	ID            string    `json:"id"`
	MemberID      string    `json:"member_id"`
	PrincipalType string    `json:"principal_type"`
	PrincipalID   string    `json:"principal_id"`
	OperatorID    string    `json:"operator_id,omitempty"`
	AccessGrantID string    `json:"access_grant_id,omitempty"`
	AssignedBy    string    `json:"assigned_by"`
	AssignedAt    time.Time `json:"assigned_at"`
	Status        string    `json:"status"`
}

type WorkEvidence struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Revision   string `json:"revision"`
}

// WorkLaunch is the immutable input boundary for a scoped change session,
// shared workspace, or conflict-resolution workspace. CurrentUpstream is
// derived on reads and is deliberately not persisted as authority.
type WorkLaunch struct {
	ID                 string            `json:"id"`
	RequestID          string            `json:"request_id"`
	RequestDigest      string            `json:"request_digest"`
	MemberID           string            `json:"member_id"`
	Kind               string            `json:"kind"`
	AssignmentID       string            `json:"assignment_id"`
	Outcome            string            `json:"outcome"`
	Revision           string            `json:"revision"`
	ParentRevision     string            `json:"parent_revision"`
	UpstreamRevisions  map[string]string `json:"upstream_revisions"`
	AcceptanceCriteria []string          `json:"acceptance_criteria"`
	Evidence           []WorkEvidence    `json:"evidence"`
	OpenedBy           string            `json:"opened_by"`
	OpenedAt           time.Time         `json:"opened_at"`
	CurrentUpstream    bool              `json:"current_upstream"`
	ChangedUpstream    []string          `json:"changed_upstream,omitempty"`
	Authority          string            `json:"authority"`
}

type TimelineEvent struct {
	ID                string            `json:"id"`
	RequestID         string            `json:"request_id"`
	RequestDigest     string            `json:"request_digest"`
	MemberID          string            `json:"member_id"`
	Kind              string            `json:"kind"`
	Summary           string            `json:"summary"`
	Revision          string            `json:"revision"`
	UpstreamRevisions map[string]string `json:"upstream_revisions"`
	WorkLaunchID      string            `json:"work_launch_id,omitempty"`
	RestackID         string            `json:"restack_id,omitempty"`
	FromPrincipalID   string            `json:"from_principal_id,omitempty"`
	ToPrincipalID     string            `json:"to_principal_id,omitempty"`
	ActorID           string            `json:"actor_id"`
	ActorType         string            `json:"actor_type"`
	CreatedAt         time.Time         `json:"created_at"`
	CurrentUpstream   bool              `json:"current_upstream"`
	ChangedUpstream   []string          `json:"changed_upstream,omitempty"`
}

type Stack struct {
	ID              string                       `json:"id"`
	RequestID       string                       `json:"request_id"`
	RequestDigest   string                       `json:"request_digest,omitempty"`
	RepositoryID    string                       `json:"repository_id"`
	Title           string                       `json:"title"`
	Outcome         string                       `json:"outcome"`
	TargetBranch    string                       `json:"target_branch"`
	TargetRevision  string                       `json:"target_revision,omitempty"`
	Members         []Member                     `json:"members"`
	Diagnostics     []Diagnostic                 `json:"diagnostics"`
	CreatedBy       string                       `json:"created_by"`
	CreatedAt       time.Time                    `json:"created_at"`
	Authority       string                       `json:"authority"`
	Restacks        []Restack                    `json:"restacks,omitempty"`
	RevisionLineage map[string][]RevisionLineage `json:"revision_lineage,omitempty"`
	Assignments     []Assignment                 `json:"assignments,omitempty"`
	WorkLaunches    []WorkLaunch                 `json:"work_launches,omitempty"`
	Timeline        []TimelineEvent              `json:"timeline,omitempty"`
}

type Store struct {
	root string
	mu   sync.Mutex
}

func New(root string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, ErrInvalid
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	return &Store{root: abs}, nil
}

func (s *Store) Create(v Stack, actor string) (Stack, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(v.RequestID) == "" || len(v.RequestID) > 128 || strings.TrimSpace(v.RequestDigest) == "" || strings.TrimSpace(v.Title) == "" || len(v.Title) > 200 || strings.TrimSpace(v.Outcome) == "" || len(v.Outcome) > 4000 || strings.TrimSpace(v.TargetBranch) == "" || len(v.Members) == 0 || len(v.Members) > 50 || actor == "" {
		return Stack{}, ErrInvalid
	}
	if existing, found, err := s.reconcile(v.RepositoryID, v.RequestID, v.RequestDigest); found || err != nil {
		return existing, err
	}
	seen := map[string]bool{}
	for i := range v.Members {
		m := &v.Members[i]
		if strings.TrimSpace(m.ID) == "" {
			m.ID = randomID()
		}
		if seen[m.ID] || strings.TrimSpace(m.Title) == "" || strings.TrimSpace(m.SourceBranch) == "" || len(m.AcceptanceCriteria) == 0 {
			return Stack{}, ErrInvalid
		}
		seen[m.ID] = true
		m.Position = i + 1
		for _, c := range m.AcceptanceCriteria {
			if strings.TrimSpace(c) == "" || len(c) > 1000 {
				return Stack{}, ErrInvalid
			}
		}
	}
	v.ID = randomID()
	v.CreatedBy = actor
	v.CreatedAt = time.Now().UTC()
	v.Authority = "stack coordination grants no Git, branch, pull, review, or merge authority"
	if err := os.MkdirAll(filepath.Join(s.root, v.RepositoryID), 0755); err != nil {
		return Stack{}, err
	}
	return v, s.write(v)
}

func (s *Store) reconcile(repo, requestID, digest string) (Stack, bool, error) {
	items, err := s.list(repo)
	if err != nil {
		return Stack{}, false, err
	}
	for _, item := range items {
		if item.RequestID != requestID {
			continue
		}
		if item.RequestDigest != digest {
			return Stack{}, true, ErrInvalid
		}
		return item, true, nil
	}
	return Stack{}, false, nil
}

func (s *Store) Get(repo, id string) (Stack, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read(repo, id)
}
func (s *Store) List(repo string) ([]Stack, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.list(repo)
}
func (s *Store) list(repo string) ([]Stack, error) {
	entries, err := os.ReadDir(filepath.Join(s.root, repo))
	if errors.Is(err, os.ErrNotExist) {
		return []Stack{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []Stack{}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		v, x := s.read(repo, strings.TrimSuffix(e.Name(), ".json"))
		if x != nil {
			return nil, x
		}
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}
func (s *Store) Update(v Stack) error { s.mu.Lock(); defer s.mu.Unlock(); return s.write(v) }

// ProposeRestack appends one immutable, caller-stable preview. Git and
// authorization validation is performed by the public route before this
// persistence boundary.
func (s *Store) ProposeRestack(repo, stackID string, proposal Restack) (Stack, Restack, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.read(repo, stackID)
	if err != nil {
		return Stack{}, Restack{}, err
	}
	if proposal.RequestID == "" || proposal.RequestDigest == "" || proposal.CreatedBy == "" || len(proposal.Members) == 0 {
		return Stack{}, Restack{}, ErrInvalid
	}
	for _, existing := range v.Restacks {
		if existing.RequestID != proposal.RequestID {
			continue
		}
		if existing.RequestDigest != proposal.RequestDigest {
			return Stack{}, Restack{}, ErrInvalid
		}
		return v, existing, nil
	}
	proposal.ID = randomID()
	proposal.Status = "previewed"
	if proposal.CreatedAt.IsZero() {
		proposal.CreatedAt = time.Now().UTC()
	}
	proposal.Authority = "preview grants no Git or pull authority; apply rechecks every branch, permission, and conflict"
	v.Restacks = append(v.Restacks, proposal)
	return v, proposal, s.write(v)
}

// ApplyRestack records the already-CAS-published result and advances the
// stack's collaboration projection without discarding its old lineage.
func (s *Store) ApplyRestack(repo, stackID, restackID, actor string, members []Member) (Stack, Restack, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.read(repo, stackID)
	if err != nil {
		return Stack{}, Restack{}, err
	}
	index := -1
	for i := range v.Restacks {
		if v.Restacks[i].ID == restackID {
			index = i
			break
		}
	}
	if index < 0 {
		return Stack{}, Restack{}, ErrNotFound
	}
	r := &v.Restacks[index]
	if r.Status == "applied" {
		return v, *r, nil
	}
	if r.Status != "previewed" || actor == "" || !validMemberDependencies(members) {
		return Stack{}, Restack{}, ErrInvalid
	}
	now := time.Now().UTC()
	if v.RevisionLineage == nil {
		v.RevisionLineage = map[string][]RevisionLineage{}
	}
	old := map[string]Member{}
	for _, m := range v.Members {
		old[m.ID] = m
	}
	for i := range members {
		members[i].Position = i + 1
		if prior, ok := old[members[i].ID]; ok && prior.Revision != members[i].Revision {
			v.RevisionLineage[members[i].ID] = append(v.RevisionLineage[members[i].ID], RevisionLineage{Revision: prior.Revision, BaseRevision: prior.BaseRevision, SucceededBy: members[i].Revision, RestackID: r.ID, ChangedBy: actor, ChangedAt: now})
		}
	}
	for _, removed := range r.Removed {
		v.RevisionLineage[removed.ID] = append(v.RevisionLineage[removed.ID], RevisionLineage{Revision: removed.Revision, BaseRevision: removed.BaseRevision, RestackID: r.ID, ChangedBy: actor, ChangedAt: now})
	}
	v.Members = members
	r.Status, r.AppliedBy, r.AppliedAt = "applied", actor, &now
	return v, *r, s.write(v)
}

func validMemberDependencies(members []Member) bool {
	ids := map[string]bool{}
	graph := map[string][]string{}
	for _, member := range members {
		if member.ID == "" || ids[member.ID] {
			return false
		}
		ids[member.ID] = true
		graph[member.ID] = member.DependsOn
	}
	for _, member := range members {
		for _, dependency := range member.DependsOn {
			if !ids[dependency] {
				return false
			}
		}
		if memberDependencyCycle(member.ID, graph, map[string]bool{}, map[string]bool{}) {
			return false
		}
	}
	return true
}

func memberDependencyCycle(id string, graph map[string][]string, visiting, done map[string]bool) bool {
	if visiting[id] {
		return true
	}
	if done[id] {
		return false
	}
	visiting[id] = true
	for _, dependency := range graph[id] {
		if memberDependencyCycle(dependency, graph, visiting, done) {
			return true
		}
	}
	delete(visiting, id)
	done[id] = true
	return false
}
func (s *Store) Acknowledge(repo, stackID, memberID, owner, decision, note string) (Stack, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if owner == "" || (decision != "acknowledged" && decision != "changes_requested") || len(note) > 2000 {
		return Stack{}, ErrInvalid
	}
	v, err := s.read(repo, stackID)
	if err != nil {
		return Stack{}, err
	}
	upstream := map[string]string{}
	index := -1
	for i := range v.Members {
		if v.Members[i].ID == memberID {
			index = i
			break
		}
		upstream[v.Members[i].ID] = v.Members[i].Revision
	}
	if index < 0 || v.Members[index].Revision == "" {
		return Stack{}, ErrInvalid
	}
	now := time.Now().UTC()
	a := Acknowledgement{ID: randomID(), MemberID: memberID, Revision: v.Members[index].Revision, UpstreamRevisions: upstream, Decision: decision, Note: strings.TrimSpace(note), OwnerID: owner, CreatedAt: now}
	v.Members[index].Acknowledgements = append(v.Members[index].Acknowledgements, a)
	return v, s.write(v)
}

func (s *Store) Assign(repo, stackID, memberID string, assignment Assignment) (Stack, Assignment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.read(repo, stackID)
	if err != nil {
		return Stack{}, Assignment{}, err
	}
	if assignment.AssignedBy == "" || assignment.PrincipalID == "" || (assignment.PrincipalType != "human" && assignment.PrincipalType != "agent") {
		return Stack{}, Assignment{}, ErrInvalid
	}
	found := false
	for _, member := range v.Members {
		if member.ID == memberID {
			found = true
			break
		}
	}
	if !found || (assignment.PrincipalType == "agent" && (assignment.OperatorID == "" || assignment.AccessGrantID == "")) {
		return Stack{}, Assignment{}, ErrInvalid
	}
	for i := range v.Assignments {
		if v.Assignments[i].MemberID == memberID && v.Assignments[i].Status == "active" {
			v.Assignments[i].Status = "superseded"
		}
	}
	assignment.ID, assignment.MemberID, assignment.Status = randomID(), memberID, "active"
	assignment.AssignedAt = time.Now().UTC()
	v.Assignments = append(v.Assignments, assignment)
	return v, assignment, s.write(v)
}

func (s *Store) OpenWork(repo, stackID string, launch WorkLaunch) (Stack, WorkLaunch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.read(repo, stackID)
	if err != nil {
		return Stack{}, WorkLaunch{}, err
	}
	if launch.RequestID == "" || launch.RequestDigest == "" || launch.OpenedBy == "" || launch.AssignmentID == "" || (launch.Kind != "change_session" && launch.Kind != "shared_workspace" && launch.Kind != "conflict_resolution_workspace") {
		return Stack{}, WorkLaunch{}, ErrInvalid
	}
	for _, prior := range v.WorkLaunches {
		if prior.RequestID == launch.RequestID {
			if prior.RequestDigest != launch.RequestDigest {
				return Stack{}, WorkLaunch{}, ErrInvalid
			}
			return v, prior, nil
		}
	}
	var member *Member
	for i := range v.Members {
		if v.Members[i].ID == launch.MemberID {
			member = &v.Members[i]
			break
		}
	}
	active := false
	for _, a := range v.Assignments {
		if a.ID == launch.AssignmentID && a.MemberID == launch.MemberID && a.Status == "active" {
			active = true
		}
	}
	if member == nil || !active || member.Revision == "" {
		return Stack{}, WorkLaunch{}, ErrInvalid
	}
	launch.ID, launch.Outcome, launch.Revision, launch.ParentRevision = randomID(), v.Outcome, member.Revision, member.BaseRevision
	launch.AcceptanceCriteria = append([]string(nil), member.AcceptanceCriteria...)
	launch.UpstreamRevisions = upstreamSnapshot(v, member.ID)
	sourceRepositoryID := member.SourceRepositoryID
	if sourceRepositoryID == "" {
		sourceRepositoryID = repo
	}
	launch.Evidence = []WorkEvidence{{Kind: "source_revision", ResourceID: sourceRepositoryID, Revision: member.Revision}}
	if member.PullRequestID != "" {
		launch.Evidence = append(launch.Evidence, WorkEvidence{Kind: "pull_request", ResourceID: member.PullRequestID, Revision: member.Revision})
	}
	launch.OpenedAt, launch.Authority = time.Now().UTC(), "work context grants no repository, branch, pull, review, merge, federation, fork, or disclosure authority"
	v.WorkLaunches = append(v.WorkLaunches, launch)
	return v, launch, s.write(v)
}

func (s *Store) AppendTimeline(repo, stackID string, event TimelineEvent) (Stack, TimelineEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.read(repo, stackID)
	if err != nil {
		return Stack{}, TimelineEvent{}, err
	}
	if event.RequestID == "" || event.RequestDigest == "" || event.ActorID == "" || len(strings.TrimSpace(event.Summary)) == 0 || len(event.Summary) > 2000 || (event.Kind != "checkpoint" && event.Kind != "question" && event.Kind != "handoff" && event.Kind != "restack_proposal") {
		return Stack{}, TimelineEvent{}, ErrInvalid
	}
	for _, prior := range v.Timeline {
		if prior.RequestID == event.RequestID {
			if prior.RequestDigest != event.RequestDigest {
				return Stack{}, TimelineEvent{}, ErrInvalid
			}
			return v, prior, nil
		}
	}
	var member *Member
	for i := range v.Members {
		if v.Members[i].ID == event.MemberID {
			member = &v.Members[i]
			break
		}
	}
	if member == nil || member.Revision == "" || event.Revision != member.Revision {
		return Stack{}, TimelineEvent{}, ErrInvalid
	}
	if event.Kind == "handoff" && (event.FromPrincipalID == "" || event.ToPrincipalID == "") {
		return Stack{}, TimelineEvent{}, ErrInvalid
	}
	if event.WorkLaunchID != "" {
		found := false
		for _, launch := range v.WorkLaunches {
			if launch.ID == event.WorkLaunchID && launch.MemberID == event.MemberID && launch.Revision == event.Revision {
				found = true
			}
		}
		if !found {
			return Stack{}, TimelineEvent{}, ErrInvalid
		}
	}
	if event.Kind == "restack_proposal" {
		found := false
		for _, r := range v.Restacks {
			if r.ID == event.RestackID {
				found = true
			}
		}
		if !found {
			return Stack{}, TimelineEvent{}, ErrInvalid
		}
	}
	event.ID, event.Summary, event.UpstreamRevisions, event.CreatedAt = randomID(), strings.TrimSpace(event.Summary), upstreamSnapshot(v, member.ID), time.Now().UTC()
	v.Timeline = append(v.Timeline, event)
	return v, event, s.write(v)
}

func upstreamSnapshot(v Stack, memberID string) map[string]string {
	out := map[string]string{}
	for _, m := range v.Members {
		if m.ID == memberID {
			break
		}
		out[m.ID] = m.Revision
	}
	return out
}
func (s *Store) read(repo, id string) (Stack, error) {
	var v Stack
	b, err := os.ReadFile(filepath.Join(s.root, repo, id+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return v, ErrNotFound
	}
	if err != nil {
		return v, err
	}
	if json.Unmarshal(b, &v) != nil {
		return v, ErrInvalid
	}
	return v, nil
}
func (s *Store) write(v Stack) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Join(s.root, v.RepositoryID)
	if err = os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".stack-")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, filepath.Join(dir, v.ID+".json"))
}
func randomID() string { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
