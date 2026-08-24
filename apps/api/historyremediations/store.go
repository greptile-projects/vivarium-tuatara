// Package historyremediations retains restricted, payload-free history-repair boundaries.
package historyremediations

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

var ErrNotFound = errors.New("history remediation not found")
var ErrInvalid = errors.New("invalid history remediation")
var ErrConflict = errors.New("history remediation request changed")
var ErrVersionConflict = errors.New("history remediation version changed")

var credentialPattern = regexp.MustCompile(`(?i)(authorization\s*:|bearer\s+[a-z0-9._-]{12,}|-----begin [a-z ]*private key-----|(?:api[_-]?key|password|passwd|secret|token)\s*[:=]\s*[^\s]{8,}|(?:ghp|github_pat|glpat-|xox[baprs]-|sk-)[a-z0-9_-]{12,}|\b(?:AKIA|ASIA)[A-Z0-9]{16}\b|\beyJ[a-z0-9_-]{10,}\.[a-z0-9_-]{10,}\.[a-z0-9_-]{10,}\b)`)

type Source struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Revision   string `json:"revision,omitempty"`
}
type Scope struct {
	RepositoryID   string `json:"repository_id"`
	Kind           string `json:"kind"`
	ObjectID       string `json:"object_id"`
	Revision       string `json:"revision,omitempty"`
	Ref            string `json:"ref,omitempty"`
	ReleaseID      string `json:"release_id,omitempty"`
	Package        string `json:"package,omitempty"`
	ArtifactDigest string `json:"artifact_digest,omitempty"`
	EnvironmentID  string `json:"environment_id,omitempty"`
}
type Evidence struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	ResourceID   string `json:"resource_id"`
	SHA256       string `json:"sha256"`
	State        string `json:"state"`
	Note         string `json:"note,omitempty"`
	AttributedTo string `json:"attributed_to"`
}
type Constraint struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	ResourceID   string `json:"resource_id,omitempty"`
	State        string `json:"state"`
	Reason       string `json:"reason"`
	AttributedTo string `json:"attributed_to"`
}
type Approval struct {
	Role        string   `json:"role"`
	ApproverIDs []string `json:"approver_ids"`
	Required    int      `json:"required"`
}

// ExposureFinding is a payload-free observation about one place an affected
// object (or data derived from it) may still be reachable or in use.
type ExposureFinding struct {
	ID                      string    `json:"id"`
	RequestID               string    `json:"request_id"`
	CopyKind                string    `json:"copy_kind"`
	ResourceID              string    `json:"resource_id"`
	RepositoryID            string    `json:"repository_id,omitempty"`
	ObjectIDs               []string  `json:"object_ids"`
	DerivedKinds            []string  `json:"derived_kinds,omitempty"`
	State                   string    `json:"state"`
	IndependentlyControlled bool      `json:"independently_controlled,omitempty"`
	Restricted              bool      `json:"restricted,omitempty"`
	CitationKind            string    `json:"citation_kind"`
	CitationResourceID      string    `json:"citation_resource_id"`
	CitationSHA256          string    `json:"citation_sha256"`
	Note                    string    `json:"note,omitempty"`
	Uncertainty             string    `json:"uncertainty,omitempty"`
	AttributedTo            string    `json:"attributed_to"`
	CreatedAt               time.Time `json:"created_at"`
}

// RewriteRule is immutable remediation intent. Object IDs are retained only in
// the remediation's restricted projection and never published as Git refs.
type RewriteRule struct {
	ID                  string `json:"id"`
	AffectedObjectID    string `json:"affected_object_id"`
	Action              string `json:"action"`
	ReplacementObjectID string `json:"replacement_object_id,omitempty"`
	Reason              string `json:"reason"`
}
type RewriteRef struct {
	Name        string `json:"name"`
	ExpectedTip string `json:"expected_tip"`
}
type CommitMapping struct {
	OldCommitID string `json:"old_commit_id"`
	NewCommitID string `json:"new_commit_id"`
	Changed     bool   `json:"changed"`
}
type ObjectMapping struct {
	OldObjectID string `json:"old_object_id"`
	NewObjectID string `json:"new_object_id,omitempty"`
	Action      string `json:"action"`
}
type CandidateRef struct {
	Name   string `json:"name"`
	OldTip string `json:"old_tip"`
	NewTip string `json:"new_tip"`
}
type RewriteCandidate struct {
	ID                  string          `json:"id"`
	RequestID           string          `json:"request_id"`
	Rules               []RewriteRule   `json:"rules"`
	SelectedRefs        []RewriteRef    `json:"selected_refs"`
	CandidateRefs       []CandidateRef  `json:"candidate_refs"`
	CommitMap           []CommitMapping `json:"commit_map"`
	ObjectMap           []ObjectMapping `json:"object_map"`
	BrokenSignatures    []string        `json:"broken_signatures"`
	BrokenLinks         []string        `json:"broken_links"`
	Unrewritable        []string        `json:"unrewritable_resources"`
	OriginalBytes       int64           `json:"original_bytes"`
	CandidateBytes      int64           `json:"candidate_bytes"`
	RollbackLimit       string          `json:"rollback_limit"`
	CollaboratorActions []string        `json:"collaborator_actions"`
	CreatedBy           string          `json:"created_by"`
	CreatedAt           time.Time       `json:"created_at"`
	Rehearsals          []Rehearsal     `json:"rehearsals"`
}
type RehearsalScenario struct {
	ID             string `json:"id"`
	Kind           string `json:"kind"`
	Image          string `json:"image,omitempty"`
	Command        string `json:"command,omitempty"`
	Expectation    string `json:"expectation"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}
type RehearsalOutcome struct {
	ScenarioID string `json:"scenario_id"`
	Kind       string `json:"kind"`
	RefName    string `json:"ref_name"`
	State      string `json:"state"`
	ExitCode   int    `json:"exit_code"`
	Output     string `json:"output,omitempty"`
}
type Rehearsal struct {
	ID             string              `json:"id"`
	RequestID      string              `json:"request_id"`
	CandidateID    string              `json:"candidate_id"`
	Scenarios      []RehearsalScenario `json:"scenarios"`
	Outcomes       []RehearsalOutcome  `json:"outcomes"`
	State          string              `json:"state"`
	CreatedBy      string              `json:"created_by"`
	CreatedAt      time.Time           `json:"created_at"`
	ComputeSeconds int64               `json:"compute_seconds"`
}
type PublicationApproval struct {
	Role       string    `json:"role"`
	ApproverID string    `json:"approver_id"`
	ApprovedAt time.Time `json:"approved_at"`
}
type MigrationTarget struct {
	ID                      string `json:"id"`
	Kind                    string `json:"kind"`
	ResourceID              string `json:"resource_id"`
	OwnerID                 string `json:"owner_id"`
	RepositoryID            string `json:"repository_id,omitempty"`
	IndependentlyControlled bool   `json:"independently_controlled,omitempty"`
	State                   string `json:"state"`
	Instructions            string `json:"instructions"`
	AcknowledgedBy          string `json:"acknowledged_by,omitempty"`
	ReplacementRef          string `json:"replacement_ref"`
	ReplacementRevision     string `json:"replacement_revision"`
}
type Publication struct {
	ID                 string                `json:"id"`
	RequestID          string                `json:"request_id"`
	CandidateID        string                `json:"candidate_id"`
	Approvals          []PublicationApproval `json:"approvals"`
	QuarantinedObjects []string              `json:"quarantined_objects"`
	RevokedCredentials []string              `json:"revoked_credentials,omitempty"`
	PausedSystems      []string              `json:"paused_systems"`
	MigrationTargets   []MigrationTarget     `json:"migration_targets"`
	State              string                `json:"state"`
	PublishedBy        string                `json:"published_by"`
	PublishedAt        time.Time             `json:"published_at"`
}

// CompletionPolicy is the closed, retained definition used by every
// containment pass. A pass may be complete while still honestly reporting
// bounded residual copies; it may never call those copies erased.
type CompletionPolicy struct {
	RequiredKinds  []string `json:"required_kinds"`
	MinimumPassing int      `json:"minimum_passing"`
}
type ContainmentObservation struct {
	Kind                    string    `json:"kind"`
	ResourceID              string    `json:"resource_id"`
	State                   string    `json:"state"`
	EvidenceSHA256          string    `json:"evidence_sha256"`
	Summary                 string    `json:"summary"`
	IndependentlyControlled bool      `json:"independently_controlled,omitempty"`
	ExceptionExpiresAt      time.Time `json:"exception_expires_at,omitempty"`
	CheckedAt               time.Time `json:"checked_at"`
}
type ContainmentPass struct {
	ID           string                   `json:"id"`
	RequestID    string                   `json:"request_id"`
	Observations []ContainmentObservation `json:"observations"`
	State        string                   `json:"state"`
	ErasureClaim bool                     `json:"erasure_claim"`
	Blockers     []string                 `json:"blockers"`
	CheckedBy    string                   `json:"checked_by"`
	CheckedAt    time.Time                `json:"checked_at"`
}
type MigrationAction struct {
	ID                   string    `json:"id"`
	RequestID            string    `json:"request_id"`
	Kind                 string    `json:"kind"`
	ResourceID           string    `json:"resource_id"`
	Action               string    `json:"action"`
	ReplacementRevision  string    `json:"replacement_revision"`
	DiscussionPreserved  bool      `json:"discussion_preserved"`
	AttributionPreserved bool      `json:"attribution_preserved"`
	ActorID              string    `json:"actor_id"`
	CreatedAt            time.Time `json:"created_at"`
}
type Restoration struct {
	ID                string    `json:"id"`
	RequestID         string    `json:"request_id"`
	Systems           []string  `json:"systems"`
	ContainmentPassID string    `json:"containment_pass_id"`
	State             string    `json:"state"`
	RestoredBy        string    `json:"restored_by"`
	RestoredAt        time.Time `json:"restored_at"`
}
type Remediation struct {
	ID                   string                `json:"id"`
	RepositoryID         string                `json:"repository_id"`
	RequestID            string                `json:"request_id"`
	RequestDigest        string                `json:"request_digest,omitempty"`
	Title                string                `json:"title"`
	Source               Source                `json:"source"`
	ContentDescription   string                `json:"content_description"`
	Reason               string                `json:"reason"`
	Scopes               []Scope               `json:"scopes"`
	Evidence             []Evidence            `json:"discovery_evidence"`
	Constraints          []Constraint          `json:"constraints"`
	AudienceIDs          []string              `json:"audience_ids"`
	OwnerIDs             []string              `json:"owner_ids"`
	RequiredApprovals    []Approval            `json:"required_approvals"`
	CreatedBy            string                `json:"created_by"`
	CreatedAt            time.Time             `json:"created_at"`
	Authority            string                `json:"authority"`
	Version              int                   `json:"version"`
	ExposureMap          []ExposureFinding     `json:"exposure_map"`
	RewriteCandidates    []RewriteCandidate    `json:"rewrite_candidates"`
	PublicationApprovals []PublicationApproval `json:"publication_approvals"`
	Publication          *Publication          `json:"publication,omitempty"`
	CompletionPolicy     CompletionPolicy      `json:"completion_policy"`
	ContainmentPasses    []ContainmentPass     `json:"containment_passes"`
	MigrationActions     []MigrationAction     `json:"migration_actions"`
	Restorations         []Restoration         `json:"restorations"`
}

func (s *Store) ApprovePublication(repo, id string, expected int, role, actor string) (Remediation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(s.path(repo, id))
	if os.IsNotExist(err) {
		return Remediation{}, ErrNotFound
	}
	if err != nil {
		return Remediation{}, err
	}
	var v Remediation
	if err = json.Unmarshal(b, &v); err != nil {
		return Remediation{}, err
	}
	eligible := false
	for _, required := range v.RequiredApprovals {
		if required.Role == role {
			for _, id := range required.ApproverIDs {
				if id == actor {
					eligible = true
				}
			}
		}
	}
	if !eligible {
		return Remediation{}, ErrInvalid
	}
	for _, approval := range v.PublicationApprovals {
		if approval.Role == role && approval.ApproverID == actor {
			return v, nil
		}
	}
	if expected != v.Version || v.Publication != nil {
		return Remediation{}, ErrVersionConflict
	}
	v.PublicationApprovals = append(v.PublicationApprovals, PublicationApproval{Role: role, ApproverID: actor, ApprovedAt: s.now()})
	v.Version++
	out, _ := json.MarshalIndent(v, "", "  ")
	if err = atomicWrite(s.path(repo, id), out, s.beforeReplace); err != nil {
		return Remediation{}, err
	}
	return v, nil
}

type Store struct {
	root          string
	mu            sync.Mutex
	now           func() time.Time
	beforeReplace func() error
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
func defaultCompletionPolicy() CompletionPolicy {
	return CompletionPolicy{RequiredKinds: []string{"repository_reachability", "object_access", "fork_acknowledgement", "federation_acknowledgement", "package_replacement", "artifact_replacement", "credential_rotation", "deployment", "cache", "recovery_copy"}, MinimumPassing: 10}
}
func randomID() string                       { b := make([]byte, 12); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func (s *Store) path(repo, id string) string { return filepath.Join(s.root, repo, id+".json") }
func valid(v Remediation) bool {
	if strings.TrimSpace(v.RequestID) == "" || strings.TrimSpace(v.Title) == "" || strings.TrimSpace(v.ContentDescription) == "" || strings.TrimSpace(v.Reason) == "" || !map[string]bool{"security_finding": true, "privacy_incident": true, "support_case": true, "selected_object": true}[v.Source.Kind] || strings.TrimSpace(v.Source.ResourceID) == "" || len(v.Scopes) == 0 || len(v.Evidence) == 0 || len(v.AudienceIDs) == 0 || len(v.OwnerIDs) == 0 || len(v.RequiredApprovals) == 0 {
		return false
	}
	// This ledger accepts bounded descriptions and digests, never copied payloads or logs.
	if len(v.Title) > 160 || len(v.ContentDescription) > 500 || len(v.Reason) > 1000 || strings.ContainsAny(v.ContentDescription, "\r\n") {
		return false
	}
	encoded, err := json.Marshal(v)
	if err != nil || credentialPattern.Match(encoded) {
		return false
	}
	for _, x := range v.Scopes {
		if x.RepositoryID == "" || x.Kind == "" || x.ObjectID == "" {
			return false
		}
	}
	for _, x := range v.Evidence {
		if x.Kind == "" || x.ResourceID == "" || len(x.SHA256) != 64 || len(x.Note) > 300 || strings.ContainsAny(x.Note, "\r\n") || !map[string]bool{"matched": true, "false_match": true, "inaccessible": true}[x.State] || x.AttributedTo == "" {
			return false
		}
	}
	for _, x := range v.Constraints {
		if !map[string]bool{"legal_hold": true, "retention_commitment": true, "continuity_commitment": true, "inaccessible_resource": true, "false_match": true}[x.Kind] || x.State == "" || x.Reason == "" || len(x.Reason) > 500 || strings.ContainsAny(x.Reason, "\r\n") || x.AttributedTo == "" {
			return false
		}
	}
	for _, x := range v.RequiredApprovals {
		if x.Role == "" || x.Required < 1 || x.Required > len(x.ApproverIDs) {
			return false
		}
	}
	return true
}
func (s *Store) Create(v Remediation, actor, digest string) (Remediation, error) {
	if !valid(v) {
		return Remediation{}, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	xs, _ := s.list(v.RepositoryID)
	for _, x := range xs {
		if x.RequestID == v.RequestID {
			if x.RequestDigest != digest {
				return Remediation{}, ErrConflict
			}
			return x, nil
		}
	}
	v.ID = randomID()
	v.RequestDigest = digest
	v.CreatedBy = actor
	v.CreatedAt = s.now()
	v.Authority = "coordination record only; grants no inspection, Git, object deletion, ref rewrite, package, artifact, release, environment, disclosure, or delivery authority"
	v.Version = 1
	v.ExposureMap = []ExposureFinding{}
	v.RewriteCandidates = []RewriteCandidate{}
	v.CompletionPolicy = defaultCompletionPolicy()
	v.ContainmentPasses = []ContainmentPass{}
	v.MigrationActions = []MigrationAction{}
	v.Restorations = []Restoration{}
	if err := os.MkdirAll(filepath.Dir(s.path(v.RepositoryID, v.ID)), 0700); err != nil {
		return Remediation{}, err
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	if err := atomicWrite(s.path(v.RepositoryID, v.ID), b, s.beforeReplace); err != nil {
		return Remediation{}, err
	}
	return v, nil
}

func validObservation(x ContainmentObservation, now time.Time) bool {
	kinds := map[string]bool{"repository_reachability": true, "object_access": true, "fork_acknowledgement": true, "federation_acknowledgement": true, "package_replacement": true, "artifact_replacement": true, "credential_rotation": true, "deployment": true, "cache": true, "recovery_copy": true}
	states := map[string]bool{"passed": true, "failed": true, "unreachable": true, "independently_controlled": true, "retained_legal": true, "reintroduced": true, "exception": true}
	if !kinds[x.Kind] || x.ResourceID == "" || !states[x.State] || len(x.EvidenceSHA256) != 64 || x.Summary == "" || len(x.Summary) > 500 || strings.ContainsAny(x.Summary, "\r\n") {
		return false
	}
	if x.State == "exception" && (x.ExceptionExpiresAt.IsZero() || !x.ExceptionExpiresAt.After(now) || x.ExceptionExpiresAt.After(now.Add(30*24*time.Hour))) {
		return false
	}
	return true
}

// RecordContainmentPass appends an immutable current-evidence pass. Only a
// pass covering every policy kind can permit restoration. Residual states stay
// visible and ErasureClaim is always false by construction.
func (s *Store) RecordContainmentPass(repo, id string, expected int, in ContainmentPass, actor string) (Remediation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(s.path(repo, id))
	if os.IsNotExist(err) {
		return Remediation{}, ErrNotFound
	}
	if err != nil {
		return Remediation{}, err
	}
	var v Remediation
	if err = json.Unmarshal(b, &v); err != nil {
		return Remediation{}, err
	}
	if len(v.CompletionPolicy.RequiredKinds) == 0 {
		v.CompletionPolicy = defaultCompletionPolicy()
	}
	for _, x := range v.ContainmentPasses {
		if x.RequestID == in.RequestID {
			return v, nil
		}
	}
	if v.Publication == nil || v.Publication.State == "publishing" || expected != v.Version || in.RequestID == "" {
		return Remediation{}, ErrVersionConflict
	}
	now := s.now()
	covered := map[string]bool{}
	blockers := []string{}
	passing := 0
	for i := range in.Observations {
		x := &in.Observations[i]
		if !validObservation(*x, now) || covered[x.Kind] {
			return Remediation{}, ErrInvalid
		}
		covered[x.Kind] = true
		x.CheckedAt = now
		if x.State == "passed" {
			passing++
		} else if x.State != "exception" {
			blockers = append(blockers, x.Kind+":"+x.State)
		}
	}
	for _, kind := range v.CompletionPolicy.RequiredKinds {
		if !covered[kind] {
			blockers = append(blockers, kind+":missing")
		}
	}
	in.ID = randomID()
	in.CheckedBy = actor
	in.CheckedAt = now
	in.ErasureClaim = false
	in.Blockers = blockers
	in.State = "blocked"
	if len(blockers) == 0 && passing >= v.CompletionPolicy.MinimumPassing {
		in.State = "passing"
	}
	v.ContainmentPasses = append(v.ContainmentPasses, in)
	if in.State == "blocked" {
		v.Publication.State = "migration_in_progress"
		paused := false
		for _, system := range v.Publication.PausedSystems {
			paused = paused || system == "pushes"
		}
		if !paused {
			v.Publication.PausedSystems = append(v.Publication.PausedSystems, "pushes")
		}
	}
	v.Version++
	out, _ := json.MarshalIndent(v, "", "  ")
	if err = atomicWrite(s.path(repo, id), out, s.beforeReplace); err != nil {
		return Remediation{}, err
	}
	return v, nil
}

func (s *Store) RecordMigration(repo, id string, expected int, in MigrationAction, actor string) (Remediation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, e := os.ReadFile(s.path(repo, id))
	if os.IsNotExist(e) {
		return Remediation{}, ErrNotFound
	}
	if e != nil {
		return Remediation{}, e
	}
	var v Remediation
	if e = json.Unmarshal(b, &v); e != nil {
		return Remediation{}, e
	}
	for _, x := range v.MigrationActions {
		if x.RequestID == in.RequestID {
			return v, nil
		}
	}
	if expected != v.Version || v.Publication == nil || in.RequestID == "" || !map[string]bool{"pull_request": true, "workspace": true}[in.Kind] || !map[string]bool{"migrated": true, "closed": true}[in.Action] || !migrationTargetMatches(v, in) || !in.DiscussionPreserved || !in.AttributionPreserved {
		return Remediation{}, ErrInvalid
	}
	in.ID = randomID()
	in.ActorID = actor
	in.CreatedAt = s.now()
	v.MigrationActions = append(v.MigrationActions, in)
	v.Version++
	out, _ := json.MarshalIndent(v, "", "  ")
	if e = atomicWrite(s.path(repo, id), out, s.beforeReplace); e != nil {
		return Remediation{}, e
	}
	return v, nil
}

func migrationTargetMatches(v Remediation, action MigrationAction) bool {
	if v.Publication == nil || len(action.ReplacementRevision) != 40 {
		return false
	}
	for _, target := range v.Publication.MigrationTargets {
		if target.Kind == action.Kind && target.ResourceID == action.ResourceID && target.ReplacementRevision == action.ReplacementRevision {
			return true
		}
	}
	return false
}

func (s *Store) Restore(repo, id string, expected int, in Restoration, actor string) (Remediation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, e := os.ReadFile(s.path(repo, id))
	if os.IsNotExist(e) {
		return Remediation{}, ErrNotFound
	}
	if e != nil {
		return Remediation{}, e
	}
	var v Remediation
	if e = json.Unmarshal(b, &v); e != nil {
		return Remediation{}, e
	}
	for _, x := range v.Restorations {
		if x.RequestID == in.RequestID {
			return v, nil
		}
	}
	if expected != v.Version || v.Publication == nil || in.RequestID == "" || len(in.Systems) == 0 {
		return Remediation{}, ErrInvalid
	}
	var pass *ContainmentPass
	for i := range v.ContainmentPasses {
		if v.ContainmentPasses[i].ID == in.ContainmentPassID {
			pass = &v.ContainmentPasses[i]
		}
	}
	if pass == nil || pass.State != "passing" || pass.ID != v.ContainmentPasses[len(v.ContainmentPasses)-1].ID {
		return Remediation{}, ErrConflict
	}
	seen := map[string]bool{}
	for _, x := range in.Systems {
		if seen[x] || !map[string]bool{"pushes": true, "automation": true, "releases": true, "contributions": true}[x] {
			return Remediation{}, ErrInvalid
		}
		seen[x] = true
	}
	in.ID = randomID()
	in.State = "restored"
	in.RestoredBy = actor
	in.RestoredAt = s.now()
	v.Restorations = append(v.Restorations, in)
	if seen["pushes"] {
		v.Publication.PausedSystems = []string{}
	}
	if len(v.Publication.PausedSystems) == 0 {
		v.Publication.State = "contained_with_residuals"
	} else {
		v.Publication.State = "migration_in_progress"
	}
	v.Version++
	out, _ := json.MarshalIndent(v, "", "  ")
	if e = atomicWrite(s.path(repo, id), out, s.beforeReplace); e != nil {
		return Remediation{}, e
	}
	return v, nil
}

func validCandidate(v RewriteCandidate, remediation Remediation) bool {
	if v.RequestID == "" || len(v.Rules) == 0 || len(v.SelectedRefs) == 0 || len(v.CandidateRefs) != len(v.SelectedRefs) || len(v.CommitMap) == 0 || v.RollbackLimit == "" || len(v.CollaboratorActions) == 0 {
		return false
	}
	allowed := map[string]bool{}
	for _, scope := range remediation.Scopes {
		allowed[scope.ObjectID] = true
	}
	seen := map[string]bool{}
	affected := map[string]bool{}
	for _, rule := range v.Rules {
		if rule.ID == "" || seen[rule.ID] || affected[rule.AffectedObjectID] || !allowed[rule.AffectedObjectID] || !map[string]bool{"remove": true, "replace": true}[rule.Action] || (rule.Action == "replace") != (rule.ReplacementObjectID != "") || rule.Reason == "" || len(rule.Reason) > 500 {
			return false
		}
		seen[rule.ID] = true
		affected[rule.AffectedObjectID] = true
	}
	refs := map[string]bool{}
	for _, ref := range v.SelectedRefs {
		if ref.Name == "" || len(ref.ExpectedTip) != 40 || refs[ref.Name] {
			return false
		}
		refs[ref.Name] = true
	}
	b, err := json.Marshal(v)
	return err == nil && !credentialPattern.Match(b)
}

// AddRewriteCandidate appends an already assembled, unreferenced candidate
// under CAS. Assembly and Git validation remain route-owned.
func (s *Store) AddRewriteCandidate(repo, id string, expected int, in RewriteCandidate, actor string) (Remediation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(s.path(repo, id))
	if os.IsNotExist(err) {
		return Remediation{}, ErrNotFound
	}
	if err != nil {
		return Remediation{}, err
	}
	var v Remediation
	if err = json.Unmarshal(b, &v); err != nil {
		return Remediation{}, err
	}
	if v.Version == 0 {
		v.Version = 1
	}
	for _, existing := range v.RewriteCandidates {
		if existing.RequestID == in.RequestID {
			a, z := existing, in
			a.ID = ""
			a.CreatedBy = ""
			a.CreatedAt = time.Time{}
			a.Rehearsals = nil
			z.ID = ""
			z.CreatedBy = ""
			z.CreatedAt = time.Time{}
			z.Rehearsals = nil
			ab, _ := json.Marshal(a)
			zb, _ := json.Marshal(z)
			if string(ab) != string(zb) {
				return Remediation{}, ErrConflict
			}
			return v, nil
		}
	}
	if expected != v.Version {
		return Remediation{}, ErrVersionConflict
	}
	if !validCandidate(in, v) {
		return Remediation{}, ErrInvalid
	}
	in.ID = randomID()
	in.CreatedBy = actor
	in.CreatedAt = s.now()
	in.Rehearsals = []Rehearsal{}
	v.RewriteCandidates = append(v.RewriteCandidates, in)
	v.Version++
	out, _ := json.MarshalIndent(v, "", "  ")
	if err = atomicWrite(s.path(repo, id), out, s.beforeReplace); err != nil {
		return Remediation{}, err
	}
	return v, nil
}

func validRehearsal(v Rehearsal) bool {
	if v.RequestID == "" || len(v.Scenarios) < 7 || len(v.Outcomes) < len(v.Scenarios) {
		return false
	}
	kinds := map[string]bool{"repository_integrity": true, "build": true, "check": true, "release": true, "dependency": true, "clone": true, "fetch": true}
	seen := map[string]string{}
	for _, x := range v.Scenarios {
		if x.ID == "" || seen[x.ID] != "" || !kinds[x.Kind] || x.Expectation == "" || x.TimeoutSeconds < 1 || x.TimeoutSeconds > 600 {
			return false
		}
		seen[x.ID] = x.Kind
	}
	outcomes := map[string]bool{}
	for _, x := range v.Outcomes {
		key := x.RefName + "\x00" + x.ScenarioID
		if x.RefName == "" || seen[x.ScenarioID] != x.Kind || outcomes[key] || !map[string]bool{"passed": true, "failed": true, "unsupported": true}[x.State] || len(x.Output) > 2000 {
			return false
		}
		outcomes[key] = true
	}
	b, err := json.Marshal(v)
	return err == nil && !credentialPattern.Match(b)
}
func (s *Store) AddRehearsal(repo, id, candidateID string, expected int, in Rehearsal, actor string) (Remediation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(s.path(repo, id))
	if os.IsNotExist(err) {
		return Remediation{}, ErrNotFound
	}
	if err != nil {
		return Remediation{}, err
	}
	var v Remediation
	if err = json.Unmarshal(b, &v); err != nil {
		return Remediation{}, err
	}
	if v.Version == 0 {
		v.Version = 1
	}
	for ci := range v.RewriteCandidates {
		c := &v.RewriteCandidates[ci]
		if c.ID != candidateID {
			continue
		}
		for _, existing := range c.Rehearsals {
			if existing.RequestID == in.RequestID {
				return v, nil
			}
		}
		if expected != v.Version {
			return Remediation{}, ErrVersionConflict
		}
		in.CandidateID = candidateID
		for _, ref := range c.CandidateRefs {
			for _, scenario := range in.Scenarios {
				found := false
				for _, outcome := range in.Outcomes {
					if outcome.RefName == ref.Name && outcome.ScenarioID == scenario.ID {
						found = true
						break
					}
				}
				if !found {
					return Remediation{}, ErrInvalid
				}
			}
		}
		if !validRehearsal(in) {
			return Remediation{}, ErrInvalid
		}
		in.ID = randomID()
		in.CreatedBy = actor
		in.CreatedAt = s.now()
		in.State = "passed"
		for _, o := range in.Outcomes {
			if o.State != "passed" {
				in.State = "failed"
				break
			}
		}
		c.Rehearsals = append(c.Rehearsals, in)
		v.Version++
		out, _ := json.MarshalIndent(v, "", "  ")
		if err = atomicWrite(s.path(repo, id), out, s.beforeReplace); err != nil {
			return Remediation{}, err
		}
		return v, nil
	}
	return Remediation{}, ErrNotFound
}

// ReservePublication durably activates containment before authoritative refs
// move. Exact retries reconcile the same intent; a different request conflicts.
func (s *Store) ReservePublication(repo, id string, expected int, in Publication, actor string) (Remediation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(s.path(repo, id))
	if os.IsNotExist(err) {
		return Remediation{}, ErrNotFound
	}
	if err != nil {
		return Remediation{}, err
	}
	var v Remediation
	if err = json.Unmarshal(b, &v); err != nil {
		return Remediation{}, err
	}
	if v.Publication != nil {
		a, z := *v.Publication, in
		a.ID, a.State, a.PublishedBy, a.PublishedAt = "", "", "", time.Time{}
		z.ID, z.State, z.PublishedBy, z.PublishedAt = "", "", "", time.Time{}
		a.QuarantinedObjects, z.QuarantinedObjects = nil, nil
		a.Approvals, z.Approvals = nil, nil
		for i := range a.MigrationTargets {
			a.MigrationTargets[i].State, a.MigrationTargets[i].AcknowledgedBy = "", ""
			a.MigrationTargets[i].ReplacementRevision = ""
		}
		for i := range z.MigrationTargets {
			z.MigrationTargets[i].State, z.MigrationTargets[i].AcknowledgedBy = "", ""
			z.MigrationTargets[i].ReplacementRevision = ""
		}
		ab, _ := json.Marshal(a)
		zb, _ := json.Marshal(z)
		if v.Publication.RequestID == in.RequestID && string(ab) == string(zb) {
			return v, nil
		}
		return Remediation{}, ErrConflict
	}
	if expected != v.Version {
		return Remediation{}, ErrVersionConflict
	}
	var candidate *RewriteCandidate
	for i := range v.RewriteCandidates {
		if v.RewriteCandidates[i].ID == in.CandidateID {
			candidate = &v.RewriteCandidates[i]
		}
	}
	if candidate == nil || in.RequestID == "" || len(in.PausedSystems) != 1 || in.PausedSystems[0] != "pushes" || len(in.MigrationTargets) == 0 {
		return Remediation{}, ErrInvalid
	}
	passed := false
	for _, rehearsal := range candidate.Rehearsals {
		if rehearsal.State == "passed" {
			passed = true
		}
	}
	if !passed {
		return Remediation{}, ErrInvalid
	}
	in.Approvals = append([]PublicationApproval{}, v.PublicationApprovals...)
	roles := map[string]map[string]bool{}
	for _, a := range in.Approvals {
		if a.Role == "" || a.ApproverID == "" {
			return Remediation{}, ErrInvalid
		}
		if roles[a.Role] == nil {
			roles[a.Role] = map[string]bool{}
		}
		roles[a.Role][a.ApproverID] = true
	}
	for _, required := range v.RequiredApprovals {
		count := 0
		for _, id := range required.ApproverIDs {
			if roles[required.Role][id] {
				count++
			}
		}
		if count < required.Required {
			return Remediation{}, ErrInvalid
		}
	}
	migrationResources := map[string]bool{}
	for i := range in.MigrationTargets {
		t := &in.MigrationTargets[i]
		resourceKey := t.Kind + "\x00" + t.ResourceID
		if t.ID == "" || t.ResourceID == "" || migrationResources[resourceKey] || t.OwnerID == "" || t.Instructions == "" || !map[string]bool{"local_branch": true, "fork": true, "federated_copy": true, "pull_request": true, "workspace": true, "integration": true}[t.Kind] {
			return Remediation{}, ErrInvalid
		}
		migrationResources[resourceKey] = true
		t.ReplacementRevision = ""
		for _, candidate := range v.RewriteCandidates {
			if candidate.ID != in.CandidateID {
				continue
			}
			for _, ref := range candidate.CandidateRefs {
				if ref.Name == t.ReplacementRef {
					t.ReplacementRevision = ref.NewTip
				}
			}
		}
		if t.ReplacementRevision == "" {
			return Remediation{}, ErrInvalid
		}
		if t.IndependentlyControlled {
			t.State = "awaiting_owner"
		} else {
			t.State = "paused"
		}
	}
	in.ID = randomID()
	in.PublishedBy = actor
	in.PublishedAt = s.now()
	in.State = "publishing"
	in.QuarantinedObjects = []string{}
	for _, scope := range v.Scopes {
		in.QuarantinedObjects = append(in.QuarantinedObjects, scope.ObjectID)
	}
	v.Publication = &in
	v.Version++
	out, _ := json.MarshalIndent(v, "", "  ")
	if err = atomicWrite(s.path(repo, id), out, s.beforeReplace); err != nil {
		return Remediation{}, err
	}
	return v, nil
}

// CompletePublication finalizes only the matching durable intent after the Git
// ref transaction has committed. A lost response can safely retry this step.
func (s *Store) CompletePublication(repo, id, requestID, candidateID string) (Remediation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(s.path(repo, id))
	if os.IsNotExist(err) {
		return Remediation{}, ErrNotFound
	}
	if err != nil {
		return Remediation{}, err
	}
	var v Remediation
	if err = json.Unmarshal(b, &v); err != nil {
		return Remediation{}, err
	}
	if v.Publication == nil || v.Publication.RequestID != requestID || v.Publication.CandidateID != candidateID {
		return Remediation{}, ErrConflict
	}
	if v.Publication.State == "migration_in_progress" {
		return v, nil
	}
	if v.Publication.State != "publishing" {
		return Remediation{}, ErrConflict
	}
	v.Publication.State = "migration_in_progress"
	v.Version++
	out, _ := json.MarshalIndent(v, "", "  ")
	if err = atomicWrite(s.path(repo, id), out, s.beforeReplace); err != nil {
		return Remediation{}, err
	}
	return v, nil
}

func validFinding(v ExposureFinding, remediation Remediation) bool {
	kinds := map[string]bool{"branch": true, "tag": true, "pull_request": true, "fork": true, "federated_contribution": true, "workspace": true, "checkpoint": true, "cache": true, "package": true, "release_artifact": true, "documentation": true, "deployment": true, "backup": true, "active_clone": true}
	states := map[string]bool{"confirmed": true, "suspected": true, "unreachable": true, "independently_controlled": true, "unverifiable": true}
	if !kinds[v.CopyKind] || !states[v.State] || v.RequestID == "" || v.ResourceID == "" || len(v.ObjectIDs) == 0 || v.CitationKind == "" || v.CitationResourceID == "" || len(v.CitationSHA256) != 64 || len(v.Note) > 300 || len(v.Uncertainty) > 300 || strings.ContainsAny(v.Note+v.Uncertainty, "\r\n") {
		return false
	}
	allowed := map[string]bool{}
	for _, scope := range remediation.Scopes {
		allowed[scope.ObjectID] = true
	}
	for _, id := range v.ObjectIDs {
		if !allowed[id] {
			return false
		}
	}
	for _, kind := range v.DerivedKinds {
		if !map[string]bool{"credential": true, "personal_data": true, "confidential_data": true, "generated_artifact": true}[kind] {
			return false
		}
	}
	b, err := json.Marshal(v)
	return err == nil && !credentialPattern.Match(b)
}

// AddExposureFinding appends under compare-and-swap and reconciles retries.
func (s *Store) AddExposureFinding(repo, id string, expected int, in ExposureFinding, actor string) (Remediation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := os.ReadFile(s.path(repo, id))
	if os.IsNotExist(err) {
		return Remediation{}, ErrNotFound
	}
	if err != nil {
		return Remediation{}, err
	}
	var v Remediation
	if err = json.Unmarshal(b, &v); err != nil {
		return Remediation{}, err
	}
	if v.Version == 0 {
		v.Version = 1
	}
	for _, existing := range v.ExposureMap {
		if existing.RequestID == in.RequestID {
			cleanA, cleanB := existing, in
			cleanA.ID = ""
			cleanA.AttributedTo = ""
			cleanA.CreatedAt = time.Time{}
			cleanB.ID = ""
			cleanB.AttributedTo = ""
			cleanB.CreatedAt = time.Time{}
			a, _ := json.Marshal(cleanA)
			z, _ := json.Marshal(cleanB)
			if string(a) != string(z) {
				return Remediation{}, ErrConflict
			}
			return v, nil
		}
	}
	if expected != v.Version {
		return Remediation{}, ErrVersionConflict
	}
	if !validFinding(in, v) {
		return Remediation{}, ErrInvalid
	}
	in.ID = randomID()
	in.AttributedTo = actor
	in.CreatedAt = s.now()
	v.ExposureMap = append(v.ExposureMap, in)
	v.Version++
	out, _ := json.MarshalIndent(v, "", "  ")
	if err = atomicWrite(s.path(repo, id), out, s.beforeReplace); err != nil {
		return Remediation{}, err
	}
	return v, nil
}

func atomicWrite(path string, data []byte, beforeReplace func() error) (err error) {
	dir := filepath.Dir(path)
	temporary, err := os.CreateTemp(dir, ".history-remediation-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		if err != nil {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err = temporary.Chmod(0600); err != nil {
		return err
	}
	if _, err = temporary.Write(data); err != nil {
		return err
	}
	if err = temporary.Sync(); err != nil {
		return err
	}
	if err = temporary.Close(); err != nil {
		return err
	}
	if beforeReplace != nil {
		if err = beforeReplace(); err != nil {
			return err
		}
	}
	if err = os.Rename(temporaryPath, path); err != nil {
		return err
	}
	directory, openErr := os.Open(dir)
	if openErr != nil {
		return openErr
	}
	err = directory.Sync()
	closeErr := directory.Close()
	if err == nil {
		err = closeErr
	}
	return err
}

// Reconcile returns a previously committed request before callers consult
// mutable resource and participant state. Authentication remains route-owned.
func (s *Store) Reconcile(repo, requestID, digest string) (Remediation, bool, error) {
	if strings.TrimSpace(requestID) == "" || strings.TrimSpace(digest) == "" {
		return Remediation{}, false, ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	values, err := s.list(repo)
	if err != nil {
		return Remediation{}, false, err
	}
	for _, value := range values {
		if value.RequestID != requestID {
			continue
		}
		if value.RequestDigest != digest {
			return Remediation{}, false, ErrConflict
		}
		return value, true, nil
	}
	return Remediation{}, false, nil
}
func (s *Store) list(repo string) ([]Remediation, error) {
	files, err := filepath.Glob(filepath.Join(s.root, repo, "*.json"))
	if err != nil {
		return nil, err
	}
	xs := []Remediation{}
	for _, p := range files {
		b, e := os.ReadFile(p)
		if e != nil {
			return nil, e
		}
		var v Remediation
		if e = json.Unmarshal(b, &v); e != nil {
			return nil, e
		}
		xs = append(xs, v)
	}
	sort.Slice(xs, func(i, j int) bool { return xs[i].CreatedAt.After(xs[j].CreatedAt) })
	return xs, nil
}
func (s *Store) List(repo string) ([]Remediation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.list(repo)
}
func (s *Store) Get(repo, id string) (Remediation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, e := os.ReadFile(s.path(repo, id))
	if os.IsNotExist(e) {
		return Remediation{}, ErrNotFound
	}
	if e != nil {
		return Remediation{}, e
	}
	var v Remediation
	e = json.Unmarshal(b, &v)
	return v, e
}
