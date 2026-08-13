// Package productexperiments persists versioned product-learning contracts.
package productexperiments

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
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

var ErrNotFound = errors.New("product experiment not found")
var ErrInvalid = errors.New("invalid product experiment")
var ErrConflict = errors.New("product experiment conflict")

type Source struct {
	Kind       string `json:"kind"`
	ResourceID string `json:"resource_id"`
	Label      string `json:"label"`
}
type Variant struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Control     bool   `json:"control"`
}
type Audience struct {
	Description string   `json:"description"`
	Eligibility []string `json:"eligibility"`
	Exclusions  []string `json:"exclusions"`
}
type Signal struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Version  int    `json:"version"`
	Event    string `json:"event"`
	Property string `json:"property,omitempty"`
	Unit     string `json:"unit"`
	Privacy  string `json:"privacy"`
	Status   string `json:"status"`
}
type Metric struct {
	Name          string  `json:"name"`
	Kind          string  `json:"kind"`
	Direction     string  `json:"direction"`
	Threshold     float64 `json:"threshold"`
	SignalID      string  `json:"signal_id"`
	SignalVersion int     `json:"signal_version"`
}
type Revision struct {
	Version         int       `json:"version"`
	Hypothesis      string    `json:"hypothesis"`
	Variants        []Variant `json:"variants"`
	Audience        Audience  `json:"target_audience"`
	Metrics         []Metric  `json:"metrics"`
	MinimumEvidence int       `json:"minimum_evidence"`
	DurationDays    int       `json:"duration_days"`
	Owners          []string  `json:"owners"`
	StopConditions  []string  `json:"stop_conditions"`
	Assumptions     []string  `json:"assumptions"`
	Rationale       string    `json:"rationale"`
	CreatedBy       string    `json:"created_by"`
	CreatedAt       time.Time `json:"created_at"`
}
type Comment struct {
	ID        string    `json:"id"`
	Body      string    `json:"body"`
	AuthorID  string    `json:"author_id"`
	CreatedAt time.Time `json:"created_at"`
}
type Approval struct {
	UserID    string    `json:"user_id"`
	Version   int       `json:"version"`
	Decision  string    `json:"decision"`
	Note      string    `json:"note,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
type Diagnostic struct {
	Kind                string `json:"kind"`
	Severity            string `json:"severity"`
	Message             string `json:"message"`
	AttributedTo        string `json:"attributed_to"`
	RelatedExperimentID string `json:"related_experiment_id,omitempty"`
}
type WorkLink struct {
	ID                string    `json:"id"`
	ExperimentVersion int       `json:"experiment_version"`
	VariantKeys       []string  `json:"variant_keys"`
	OwnerType         string    `json:"owner_type"`
	OwnerID           string    `json:"owner_id"`
	ProposalID        string    `json:"proposal_id,omitempty"`
	TaskID            string    `json:"task_id,omitempty"`
	SessionID         string    `json:"session_id,omitempty"`
	WorkspaceID       string    `json:"workspace_id,omitempty"`
	PullRequestID     string    `json:"pull_request_id"`
	CommitID          string    `json:"commit_id"`
	EventDefinitions  []string  `json:"event_definitions"`
	ExposureRules     []string  `json:"exposure_rules"`
	Privacy           string    `json:"privacy_classification"`
	RemovalPlan       string    `json:"removal_plan"`
	CheckNames        []string  `json:"check_names"`
	LinkedBy          string    `json:"linked_by"`
	CreatedAt         time.Time `json:"created_at"`
}
type Allocation struct {
	VariantKey  string `json:"variant_key"`
	BasisPoints int    `json:"basis_points"`
}
type AudienceContract struct {
	ID                   string       `json:"id"`
	ExperimentVersion    int          `json:"experiment_version"`
	ReleaseID            string       `json:"release_id"`
	ReleaseCommitID      string       `json:"release_commit_id"`
	VariantKeys          []string     `json:"variant_keys"`
	Eligibility          []string     `json:"eligibility"`
	Exclusions           []string     `json:"exclusions"`
	OrganizationIDs      []string     `json:"organization_ids,omitempty"`
	Regions              []string     `json:"regions,omitempty"`
	RandomizationUnit    string       `json:"randomization_unit"`
	RandomizationSalt    string       `json:"-"`
	MutualExclusionGroup string       `json:"mutual_exclusion_group"`
	Allocation           []Allocation `json:"allocation"`
	Consent              string       `json:"consent"`
	DataFields           []string     `json:"data_fields"`
	RetentionDays        int          `json:"retention_days"`
	ApprovedBy           string       `json:"approved_by"`
	ApprovedAt           time.Time    `json:"approved_at"`
}
type AssignmentReceipt struct {
	ID            string    `json:"id"`
	ContractID    string    `json:"contract_id"`
	SubjectDigest string    `json:"subject_digest"`
	VariantKey    string    `json:"variant_key,omitempty"`
	Eligible      bool      `json:"eligible"`
	Reason        string    `json:"reason"`
	CreatedAt     time.Time `json:"created_at"`
}
type AssignmentContext struct {
	Eligibility    []string `json:"eligibility"`
	Exclusions     []string `json:"exclusions"`
	OrganizationID string   `json:"organization_id,omitempty"`
	Region         string   `json:"region,omitempty"`
	Consented      bool     `json:"consented"`
}
type ExclusionMembership struct {
	GroupDigest  string    `json:"group_digest"`
	SubjectToken string    `json:"subject_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}
type Experiment struct {
	ID                   string                `json:"id"`
	RepositoryID         string                `json:"repository_id"`
	Source               Source                `json:"source"`
	CurrentVersion       int                   `json:"current_version"`
	Revisions            []Revision            `json:"revisions"`
	Signals              []Signal              `json:"signals"`
	Comments             []Comment             `json:"comments"`
	Approvals            []Approval            `json:"approvals"`
	Work                 []WorkLink            `json:"work"`
	AudienceContracts    []AudienceContract    `json:"audience_contracts"`
	AssignmentAudit      []AssignmentReceipt   `json:"assignment_audit"`
	ExclusionMemberships []ExclusionMembership `json:"exclusion_memberships"`
	Diagnostics          []Diagnostic          `json:"diagnostics"`
	CreatedAt            time.Time             `json:"created_at"`
	UpdatedAt            time.Time             `json:"updated_at"`
}

func (s *Store) ApproveAudience(id, actor string, expected int, input AudienceContract) (Experiment, error) {
	return s.mutate(id, func(v *Experiment) error {
		if expected != v.CurrentVersion {
			return ErrConflict
		}
		if !validAudienceContract(*v, input) {
			return ErrInvalid
		}
		for _, existing := range v.AudienceContracts {
			if existing.ReleaseID == input.ReleaseID || (input.MutualExclusionGroup != "" && existing.MutualExclusionGroup == input.MutualExclusionGroup) {
				return ErrConflict
			}
		}
		input.ID, input.ExperimentVersion, input.ApprovedBy, input.ApprovedAt = idgen(), expected, actor, s.now()
		input.RandomizationSalt = idgen()
		v.AudienceContracts = append(v.AudienceContracts, input)
		return nil
	})
}

func (s *Store) Assign(id, contractID, subject string, context AssignmentContext) (Experiment, AssignmentReceipt, error) {
	var receipt AssignmentReceipt
	var out Experiment
	err := s.lock(func() error {
		v, err := s.read(id)
		if err != nil {
			return err
		}
		changed := s.pruneAssignments(&v)
		var contract *AudienceContract
		for i := range v.AudienceContracts {
			if v.AudienceContracts[i].ID == contractID {
				contract = &v.AudienceContracts[i]
			}
		}
		if contract == nil || contract.ExperimentVersion != v.CurrentVersion || strings.TrimSpace(subject) == "" {
			return ErrConflict
		}
		subjectDigest := s.subjectDigest(v.RepositoryID, subject)
		for _, prior := range v.AssignmentAudit {
			if prior.ContractID == contractID && prior.SubjectDigest == subjectDigest {
				receipt = prior
				out = v
				if changed {
					return s.write(v)
				}
				return nil
			}
		}
		receipt = AssignmentReceipt{ID: idgen(), ContractID: contractID, SubjectDigest: subjectDigest, Eligible: true, CreatedAt: s.now()}
		if !assignmentEligible(*contract, context) {
			receipt.Eligible, receipt.Reason = false, "audience_ineligible"
		} else if contract.Consent == "explicit" && !context.Consented {
			receipt.Eligible, receipt.Reason = false, "consent_required"
		} else {
			conflict, err := s.hasGroupAssignment(v.RepositoryID, v.ID, contract.MutualExclusionGroup, subjectDigest)
			if err != nil {
				return err
			}
			if conflict {
				receipt.Eligible, receipt.Reason = false, "mutually_excluded"
			} else {
				digest := sha256.Sum256([]byte(contract.RandomizationSalt + ":" + subject))
				bucket := int(digest[0])<<8 | int(digest[1])
				bucket %= 10000
				cumulative := 0
				for _, allocation := range contract.Allocation {
					cumulative += allocation.BasisPoints
					if bucket < cumulative {
						receipt.VariantKey = allocation.VariantKey
						break
					}
				}
				if receipt.VariantKey == "" {
					receipt.Eligible, receipt.Reason = false, "unallocated"
				} else {
					receipt.Reason = "assigned"
					v.ExclusionMemberships = append(v.ExclusionMemberships, ExclusionMembership{GroupDigest: s.groupDigest(v.RepositoryID, contract.MutualExclusionGroup), SubjectToken: s.groupSubjectToken(v.RepositoryID, contract.MutualExclusionGroup, subjectDigest), ExpiresAt: contract.ApprovedAt.Add(time.Duration(currentRevision(&v).DurationDays) * 24 * time.Hour)})
				}
			}
		}
		v.AssignmentAudit = append(v.AssignmentAudit, receipt)
		out = v
		if err := s.write(v); err != nil {
			return err
		}
		s.scheduleCleanupAt(receipt.CreatedAt.Add(time.Duration(contract.RetentionDays) * 24 * time.Hour))
		return nil
	})
	if err != nil {
		return Experiment{}, AssignmentReceipt{}, err
	}
	return s.project(out), receipt, nil
}

func currentRevision(v *Experiment) Revision {
	for i := len(v.Revisions) - 1; i >= 0; i-- {
		if v.Revisions[i].Version == v.CurrentVersion {
			return v.Revisions[i]
		}
	}
	return Revision{}
}

func (s *Store) groupDigest(repositoryID, group string) string {
	return s.protectedToken(repositoryID + ":group:" + group)
}
func (s *Store) groupSubjectToken(repositoryID, group, subjectDigest string) string {
	return s.protectedToken(repositoryID + ":member:" + group + ":" + subjectDigest)
}
func (s *Store) protectedToken(value string) string {
	h := hmac.New(sha256.New, s.subjectKey)
	_, _ = h.Write([]byte(value))
	return hex.EncodeToString(h.Sum(nil))
}

func (s *Store) subjectDigest(repositoryID, subject string) string {
	h := hmac.New(sha256.New, s.subjectKey)
	_, _ = h.Write([]byte(repositoryID + ":" + subject))
	return hex.EncodeToString(h.Sum(nil))
}

func (s *Store) hasGroupAssignment(repositoryID, experimentID, group, digest string) (bool, error) {
	groupDigest, subjectToken := s.groupDigest(repositoryID, group), s.groupSubjectToken(repositoryID, group, digest)
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		v, err := s.read(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return false, err
		}
		if v.RepositoryID != repositoryID || v.ID == experimentID {
			continue
		}
		if s.pruneAssignments(&v) {
			if err := s.write(v); err != nil {
				return false, err
			}
		}
		for _, membership := range v.ExclusionMemberships {
			if membership.ExpiresAt.After(s.now()) && membership.GroupDigest == groupDigest && membership.SubjectToken == subjectToken {
				return true, nil
			}
		}
	}
	return false, nil
}

func (s *Store) pruneAssignments(v *Experiment) bool {
	retention := map[string]time.Duration{}
	for _, c := range v.AudienceContracts {
		retention[c.ID] = time.Duration(c.RetentionDays) * 24 * time.Hour
	}
	next := make([]AssignmentReceipt, 0, len(v.AssignmentAudit))
	now := s.now()
	for _, receipt := range v.AssignmentAudit {
		duration, ok := retention[receipt.ContractID]
		if ok && receipt.CreatedAt.Add(duration).After(now) {
			next = append(next, receipt)
		}
	}
	changed := len(next) != len(v.AssignmentAudit)
	v.AssignmentAudit = next
	members := make([]ExclusionMembership, 0, len(v.ExclusionMemberships))
	for _, membership := range v.ExclusionMemberships {
		if membership.ExpiresAt.After(now) {
			members = append(members, membership)
		}
	}
	changed = changed || len(members) != len(v.ExclusionMemberships)
	v.ExclusionMemberships = members
	return changed
}

func (s *Store) CleanupExpired() error {
	return s.lock(func() error {
		entries, err := os.ReadDir(s.root)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			v, err := s.read(strings.TrimSuffix(entry.Name(), ".json"))
			if err != nil {
				return err
			}
			if s.pruneAssignments(&v) {
				if err = s.write(v); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (s *Store) scheduleCleanupAt(deadline time.Time) {
	delay := deadline.Sub(s.now())
	if delay < 0 {
		delay = 0
	}
	time.AfterFunc(delay, func() { _ = s.CleanupExpired() })
}

func loadOrCreateSubjectKey(root string) ([]byte, error) {
	path := filepath.Join(root, ".subject-key")
	key, err := os.ReadFile(path)
	if err == nil {
		if len(key) != 32 {
			return nil, ErrInvalid
		}
		return key, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	key = make([]byte, 32)
	if _, err = rand.Read(key); err != nil {
		return nil, err
	}
	if err = os.WriteFile(path, key, 0600); err != nil {
		return nil, err
	}
	return key, nil
}

func assignmentEligible(contract AudienceContract, context AssignmentContext) bool {
	has := func(values []string, wanted string) bool {
		for _, value := range values {
			if value == wanted {
				return true
			}
		}
		return false
	}
	eligible := false
	for _, rule := range contract.Eligibility {
		if has(context.Eligibility, rule) {
			eligible = true
		}
	}
	for _, rule := range context.Exclusions {
		if has(contract.Exclusions, rule) {
			return false
		}
	}
	if len(contract.Regions) > 0 && !has(contract.Regions, context.Region) {
		return false
	}
	if len(contract.OrganizationIDs) > 0 && !has(contract.OrganizationIDs, context.OrganizationID) {
		return false
	}
	return eligible
}

func validAudienceContract(v Experiment, x AudienceContract) bool {
	if x.ReleaseID == "" || !isHexCommit(x.ReleaseCommitID) || len(x.VariantKeys) < 2 || len(x.Eligibility) == 0 || x.RandomizationUnit != "user" || x.MutualExclusionGroup == "" || (x.Consent != "none" && x.Consent != "explicit") || len(x.DataFields) == 0 || x.RetentionDays < 1 || x.RetentionDays > 730 {
		return false
	}
	allowedData := map[string]bool{"assignment": true, "exposure": true, "metric": true, "region": true, "organization": true}
	for _, field := range x.DataFields {
		if !allowedData[field] {
			return false
		}
	}
	variants := map[string]bool{}
	workCommit := false
	for _, revision := range v.Revisions {
		if revision.Version == v.CurrentVersion {
			for _, variant := range revision.Variants {
				variants[variant.Key] = true
			}
		}
	}
	for _, work := range v.Work {
		if work.ExperimentVersion == v.CurrentVersion && work.CommitID == x.ReleaseCommitID {
			workCommit = true
		}
	}
	seen := map[string]bool{}
	total := 0
	for _, allocation := range x.Allocation {
		if !variants[allocation.VariantKey] || seen[allocation.VariantKey] || allocation.BasisPoints < 0 {
			return false
		}
		seen[allocation.VariantKey] = true
		total += allocation.BasisPoints
	}
	for _, key := range x.VariantKeys {
		if !variants[key] || !seen[key] {
			return false
		}
	}
	return workCommit && total > 0 && total <= 10000
}

type Store struct {
	root       string
	mu         sync.Mutex
	now        func() time.Time
	subjectKey []byte
}

func New(root string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, ErrInvalid
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	key, err := loadOrCreateSubjectKey(root)
	if err != nil {
		return nil, err
	}
	s := &Store{root: root, subjectKey: key, now: func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) }}
	if err := s.schedulePersistedCleanup(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) schedulePersistedCleanup() error {
	return s.lock(func() error {
		entries, err := os.ReadDir(s.root)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			v, err := s.read(strings.TrimSuffix(entry.Name(), ".json"))
			if err != nil {
				return err
			}
			if s.pruneAssignments(&v) {
				if err = s.write(v); err != nil {
					return err
				}
			}
			retention := map[string]int{}
			for _, c := range v.AudienceContracts {
				retention[c.ID] = c.RetentionDays
			}
			for _, receipt := range v.AssignmentAudit {
				s.scheduleCleanupAt(receipt.CreatedAt.Add(time.Duration(retention[receipt.ContractID]) * 24 * time.Hour))
			}
		}
		return nil
	})
}
func (s *Store) Create(repo, actor string, source Source, revision Revision, signals []Signal) (Experiment, error) {
	var out Experiment
	err := s.lock(func() error {
		if !validSource(source) || !validRevision(revision, signals) {
			return ErrInvalid
		}
		now := s.now()
		revision.Version = 1
		revision.CreatedBy = actor
		revision.CreatedAt = now
		out = Experiment{ID: id(), RepositoryID: repo, Source: source, CurrentVersion: 1, Revisions: []Revision{revision}, Signals: signals, CreatedAt: now, UpdatedAt: now}
		return s.write(out)
	})
	if err != nil {
		return Experiment{}, err
	}
	return s.project(out), nil
}
func (s *Store) Revise(id string, expected int, actor string, revision Revision, signals []Signal) (Experiment, error) {
	var out Experiment
	err := s.lock(func() error {
		v, e := s.read(id)
		if e != nil {
			return e
		}
		if v.CurrentVersion != expected {
			return ErrConflict
		}
		if !validRevision(revision, signals) {
			return ErrInvalid
		}
		revision.Version = expected + 1
		revision.CreatedBy = actor
		revision.CreatedAt = s.now()
		v.CurrentVersion++
		v.Revisions = append(v.Revisions, revision)
		v.Signals = signals
		v.UpdatedAt = revision.CreatedAt
		out = v
		return s.write(v)
	})
	if err != nil {
		return Experiment{}, err
	}
	return s.project(out), nil
}
func (s *Store) Comment(id, actor, body string) (Experiment, error) {
	return s.mutate(id, func(v *Experiment) error {
		if strings.TrimSpace(body) == "" || len(body) > 4000 {
			return ErrInvalid
		}
		v.Comments = append(v.Comments, Comment{ID: idgen(), Body: strings.TrimSpace(body), AuthorID: actor, CreatedAt: s.now()})
		return nil
	})
}
func (s *Store) Approve(id, actor, decision, note string, expected int) (Experiment, error) {
	return s.mutate(id, func(v *Experiment) error {
		if expected != v.CurrentVersion {
			return ErrConflict
		}
		if decision != "approve" && decision != "request_changes" {
			return ErrInvalid
		}
		next := []Approval{}
		for _, a := range v.Approvals {
			if a.UserID != actor {
				next = append(next, a)
			}
		}
		v.Approvals = append(next, Approval{UserID: actor, Version: expected, Decision: decision, Note: strings.TrimSpace(note), CreatedAt: s.now()})
		return nil
	})
}
func (s *Store) LinkWork(id, actor string, expected int, input WorkLink) (Experiment, error) {
	return s.mutate(id, func(v *Experiment) error {
		for _, existing := range v.Work {
			if existing.PullRequestID == input.PullRequestID {
				if exactWorkReplay(existing, actor, expected, input) {
					return nil
				}
				return ErrConflict
			}
		}
		if expected != v.CurrentVersion || !validWorkLink(*v, input) {
			if expected != v.CurrentVersion {
				return ErrConflict
			}
			return ErrInvalid
		}
		input.ID, input.ExperimentVersion, input.LinkedBy, input.CreatedAt = idgen(), expected, actor, s.now()
		v.Work = append(v.Work, input)
		return nil
	})
}

// ExistingWorkReplay recognizes an already-persisted immutable request without
// consulting mutable pull, task, assignment, or check projections. New work is
// never admitted here; a reused pull identity with changed evidence conflicts.
func (s *Store) ExistingWorkReplay(id, actor string, expected int, input WorkLink) (Experiment, bool, error) {
	v, err := s.Get(id)
	if err != nil {
		return Experiment{}, false, err
	}
	for _, existing := range v.Work {
		if existing.PullRequestID != input.PullRequestID {
			continue
		}
		if exactWorkReplay(existing, actor, expected, input) {
			return v, true, nil
		}
		return Experiment{}, false, ErrConflict
	}
	return v, false, nil
}

func exactWorkReplay(existing WorkLink, actor string, expected int, input WorkLink) bool {
	requested := input
	requested.ID, requested.ExperimentVersion, requested.LinkedBy, requested.CreatedAt = existing.ID, expected, actor, existing.CreatedAt
	return reflect.DeepEqual(existing, requested)
}

func validWorkLink(v Experiment, x WorkLink) bool {
	if (x.OwnerType != "human" && x.OwnerType != "agent") || strings.TrimSpace(x.OwnerID) == "" || strings.TrimSpace(x.PullRequestID) == "" || !isHexCommit(x.CommitID) || len(x.VariantKeys) == 0 || len(x.EventDefinitions) == 0 || len(x.ExposureRules) == 0 || strings.TrimSpace(x.RemovalPlan) == "" || len(x.RemovalPlan) > 4000 || len(x.CheckNames) == 0 {
		return false
	}
	if x.Privacy != "aggregate" && x.Privacy != "pseudonymous" && x.Privacy != "consented" {
		return false
	}
	variants := map[string]bool{}
	for _, revision := range v.Revisions {
		if revision.Version == v.CurrentVersion {
			for _, variant := range revision.Variants {
				variants[variant.Key] = true
			}
		}
	}
	seen := map[string]bool{}
	for _, key := range x.VariantKeys {
		if !variants[key] || seen[key] {
			return false
		}
		seen[key] = true
	}
	for _, values := range [][]string{x.EventDefinitions, x.ExposureRules, x.CheckNames} {
		for _, value := range values {
			if strings.TrimSpace(value) == "" || len(value) > 500 {
				return false
			}
		}
	}
	return true
}
func isHexCommit(value string) bool {
	if len(value) != 40 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
func (s *Store) mutate(key string, f func(*Experiment) error) (Experiment, error) {
	var out Experiment
	err := s.lock(func() error {
		v, e := s.read(key)
		if e != nil {
			return e
		}
		s.pruneAssignments(&v)
		if e = f(&v); e != nil {
			return e
		}
		v.UpdatedAt = s.now()
		out = v
		return s.write(v)
	})
	if err != nil {
		return Experiment{}, err
	}
	return s.project(out), nil
}
func (s *Store) Get(key string) (Experiment, error) {
	var out Experiment
	err := s.lock(func() error {
		var e error
		out, e = s.read(key)
		if e == nil && s.pruneAssignments(&out) {
			e = s.write(out)
		}
		return e
	})
	if err != nil {
		return Experiment{}, err
	}
	return s.project(out), nil
}
func (s *Store) List(repo string) ([]Experiment, error) {
	out := []Experiment{}
	err := s.lock(func() error {
		entries, e := os.ReadDir(s.root)
		if e != nil {
			return e
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			v, e := s.read(strings.TrimSuffix(entry.Name(), ".json"))
			if e != nil {
				return e
			}
			if s.pruneAssignments(&v) {
				if e = s.write(v); e != nil {
					return e
				}
			}
			if v.RepositoryID == repo {
				out = append(out, s.project(v))
			}
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, err
}
func (s *Store) project(v Experiment) Experiment {
	v.Diagnostics = nil
	v.ExclusionMemberships = nil
	if len(v.Revisions) == 0 {
		return v
	}
	r := v.Revisions[len(v.Revisions)-1]
	signals := map[string]Signal{}
	for _, x := range v.Signals {
		signals[x.ID] = x
	}
	for _, m := range r.Metrics {
		x, ok := signals[m.SignalID]
		if !ok || x.Version != m.SignalVersion || x.Status != "available" {
			v.Diagnostics = append(v.Diagnostics, Diagnostic{"missing_instrumentation", "blocking", "Metric " + m.Name + " is not connected to an available signal at the declared version.", r.CreatedBy, ""})
		}
	}
	if len(r.Audience.Eligibility) == 0 {
		v.Diagnostics = append(v.Diagnostics, Diagnostic{"ineligible_audience", "blocking", "The target audience has no permitted eligibility rule.", r.CreatedBy, ""})
	}
	for _, a := range v.Approvals {
		if a.Version != v.CurrentVersion {
			v.Diagnostics = append(v.Diagnostics, Diagnostic{"changed_assumptions", "warning", "A prior approval no longer applies to the current plan version.", a.UserID, ""})
		}
	}
	return v
}
func Overlaps(a, b Experiment) bool {
	if len(a.Revisions) == 0 || len(b.Revisions) == 0 {
		return false
	}
	x, y := a.Revisions[len(a.Revisions)-1], b.Revisions[len(b.Revisions)-1]
	for _, xe := range x.Audience.Eligibility {
		for _, ye := range y.Audience.Eligibility {
			if xe == ye {
				for _, xm := range x.Metrics {
					for _, ym := range y.Metrics {
						if xm.SignalID == ym.SignalID {
							return true
						}
					}
				}
			}
		}
	}
	return false
}
func AddOverlap(v *Experiment, other Experiment) {
	v.Diagnostics = append(v.Diagnostics, Diagnostic{"overlapping_experiment", "warning", "This plan shares an audience and product signal with another experiment.", other.Revisions[len(other.Revisions)-1].CreatedBy, other.ID})
}
func validSource(v Source) bool {
	ok := map[string]bool{"proposal": true, "issue": true, "decision": true, "pull_request": true, "preview": true, "release": true}
	return ok[v.Kind] && strings.TrimSpace(v.ResourceID) != "" && strings.TrimSpace(v.Label) != ""
}
func validRevision(r Revision, signals []Signal) bool {
	if strings.TrimSpace(r.Hypothesis) == "" || len(r.Variants) < 2 || len(r.Metrics) == 0 || r.MinimumEvidence < 1 || r.DurationDays < 1 || len(r.Owners) == 0 || len(r.StopConditions) == 0 {
		return false
	}
	seen := map[string]bool{}
	for _, v := range r.Variants {
		if v.Key == "" || v.Name == "" || seen[v.Key] {
			return false
		}
		seen[v.Key] = true
	}
	for _, s := range signals {
		if s.ID == "" || s.Name == "" || s.Version < 1 || s.Event == "" || (s.Status != "available" && s.Status != "planned" && s.Status != "retired") || (s.Privacy != "aggregate" && s.Privacy != "pseudonymous" && s.Privacy != "consented") {
			return false
		}
	}
	for _, m := range r.Metrics {
		if m.Name == "" || (m.Kind != "success" && m.Kind != "guardrail") || m.SignalID == "" || m.SignalVersion < 1 {
			return false
		}
	}
	return true
}
func (s *Store) lock(fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, err := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	return fn()
}
func (s *Store) read(key string) (Experiment, error) {
	b, e := os.ReadFile(filepath.Join(s.root, key+".json"))
	if os.IsNotExist(e) {
		return Experiment{}, ErrNotFound
	}
	if e != nil {
		return Experiment{}, e
	}
	var v Experiment
	if json.Unmarshal(b, &v) != nil {
		return Experiment{}, ErrInvalid
	}
	return v, nil
}
func (s *Store) write(v Experiment) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	tmp, e := os.CreateTemp(s.root, ".experiment-")
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
	return os.Rename(name, filepath.Join(s.root, v.ID+".json"))
}
func id() string    { return "experiment-" + idgen() }
func idgen() string { b := make([]byte, 12); _, _ = rand.Read(b); return hex.EncodeToString(b) }
