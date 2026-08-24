// Package propagationcampaigns persists shared outcomes that must reach maintained targets.
package propagationcampaigns

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
	"syscall"
	"time"
)

var ErrNotFound = errors.New("propagation campaign not found")
var ErrInvalid = errors.New("invalid propagation campaign")
var ErrConflict = errors.New("propagation campaign request changed")
var ErrVersion = errors.New("propagation campaign assessment changed")
var ErrProofVersion = errors.New("propagation campaign equivalence proof changed")

type Source struct {
	Kind         string   `json:"kind"`
	ResourceID   string   `json:"resource_id"`
	RepositoryID string   `json:"repository_id"`
	Commits      []string `json:"commits"`
	Label        string   `json:"label"`
}
type Target struct {
	ID                 string    `json:"id"`
	Kind               string    `json:"kind"`
	RepositoryID       string    `json:"repository_id,omitempty"`
	ReleaseLine        string    `json:"release_line,omitempty"`
	Package            string    `json:"package,omitempty"`
	OwnerIDs           []string  `json:"owner_ids"`
	DependsOn          []string  `json:"depends_on,omitempty"`
	Deadline           time.Time `json:"deadline"`
	AcceptanceCriteria []string  `json:"acceptance_criteria,omitempty"`
	State              string    `json:"state"`
	Diagnostic         string    `json:"diagnostic,omitempty"`
	Authority          string    `json:"authority"`
}
type CompletionPolicy struct {
	Mode              string `json:"mode"`
	MinimumTargets    int    `json:"minimum_targets,omitempty"`
	RequireAcceptance bool   `json:"require_acceptance"`
}
type Comparison struct {
	Kind     string   `json:"kind"`
	Status   string   `json:"status"`
	Summary  string   `json:"summary"`
	Evidence []string `json:"evidence,omitempty"`
}
type Citation struct {
	Kind      string `json:"kind"`
	Reference string `json:"reference"`
	Revision  string `json:"revision,omitempty"`
}
type AssessmentEntry struct {
	ID        string     `json:"id"`
	Kind      string     `json:"kind"`
	Body      string     `json:"body"`
	Citations []Citation `json:"citations,omitempty"`
	ActorID   string     `json:"actor_id"`
	ActorKind string     `json:"actor_kind"`
	CreatedAt time.Time  `json:"created_at"`
}
type Assessment struct {
	ID                 string            `json:"id"`
	TargetID           string            `json:"target_id"`
	Version            int               `json:"version"`
	Classification     string            `json:"classification"`
	TargetRevision     string            `json:"target_revision"`
	SourceBaseRevision string            `json:"source_base_revision,omitempty"`
	SourceRevision     string            `json:"source_revision"`
	ChangedPaths       []string          `json:"changed_paths"`
	Comparisons        []Comparison      `json:"comparisons"`
	Entries            []AssessmentEntry `json:"entries"`
	Invalidated        bool              `json:"invalidated"`
	InvalidationReason string            `json:"invalidation_reason,omitempty"`
	CreatedBy          string            `json:"created_by"`
	CreatedAt          time.Time         `json:"created_at"`
}
type Contribution struct {
	ID                string    `json:"id"`
	TargetID          string    `json:"target_id"`
	AssessmentID      string    `json:"assessment_id"`
	AssessmentVersion int       `json:"assessment_version"`
	TargetRevision    string    `json:"target_revision"`
	Application       string    `json:"application"`
	Deviation         string    `json:"deviation,omitempty"`
	Topology          string    `json:"topology"`
	Constraints       []string  `json:"constraints"`
	ProposalID        string    `json:"proposal_id"`
	TaskIDs           []string  `json:"task_ids"`
	PublishedBy       string    `json:"published_by"`
	PublishedAt       time.Time `json:"published_at"`
	Authority         string    `json:"authority"`
}
type EquivalenceScenario struct {
	Name               string     `json:"name"`
	SourceCommand      string     `json:"source_command"`
	TargetCommand      string     `json:"target_command,omitempty"`
	Coverage           []string   `json:"coverage"`
	State              string     `json:"state"`
	CheckRunID         string     `json:"check_run_id,omitempty"`
	Logs               string     `json:"logs,omitempty"`
	Artifacts          []Artifact `json:"artifacts,omitempty"`
	Cost               float64    `json:"cost"`
	SubstituteEvidence []Citation `json:"substitute_evidence,omitempty"`
}
type Artifact struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}
type OrdinaryCheck struct {
	Name       string     `json:"name"`
	Command    string     `json:"command"`
	State      string     `json:"state"`
	CheckRunID string     `json:"check_run_id"`
	Logs       string     `json:"logs,omitempty"`
	Artifacts  []Artifact `json:"artifacts,omitempty"`
	Cost       float64    `json:"cost"`
}
type OwnerDecision struct {
	OwnerID   string    `json:"owner_id"`
	Decision  string    `json:"decision"`
	Rationale string    `json:"rationale"`
	CreatedAt time.Time `json:"created_at"`
}
type EquivalenceProof struct {
	ID                      string                `json:"id"`
	RequestID               string                `json:"request_id"`
	RequestDigest           string                `json:"request_digest"`
	TargetID                string                `json:"target_id"`
	Version                 int                   `json:"version"`
	TargetRevision          string                `json:"target_revision"`
	SourceRevision          string                `json:"source_revision"`
	SourceAssumptionsSHA256 string                `json:"source_assumptions_sha256"`
	DependencySHA256        string                `json:"dependency_sha256"`
	EvidenceRequirements    []string              `json:"evidence_requirements"`
	Scenarios               []EquivalenceScenario `json:"scenarios"`
	OrdinaryChecks          []OrdinaryCheck       `json:"ordinary_checks"`
	OwnerDecisions          []OwnerDecision       `json:"owner_decisions"`
	State                   string                `json:"state"`
	ResidualDifferences     []string              `json:"residual_differences"`
	Invalidated             bool                  `json:"invalidated"`
	InvalidationReasons     []string              `json:"invalidation_reasons,omitempty"`
	CreatedBy               string                `json:"created_by"`
	CreatedAt               time.Time             `json:"created_at"`
	Authority               string                `json:"authority"`
}
type Campaign struct {
	ID                 string             `json:"id"`
	RequestID          string             `json:"request_id"`
	RequestDigest      string             `json:"request_digest,omitempty"`
	RepositoryID       string             `json:"repository_id"`
	Title              string             `json:"title"`
	Intent             string             `json:"intent"`
	AcceptanceCriteria []string           `json:"acceptance_criteria"`
	Source             Source             `json:"source"`
	Targets            []Target           `json:"targets"`
	CompletionPolicy   CompletionPolicy   `json:"completion_policy"`
	Assessments        []Assessment       `json:"assessments,omitempty"`
	Contributions      []Contribution     `json:"contributions,omitempty"`
	EquivalenceProofs  []EquivalenceProof `json:"equivalence_proofs,omitempty"`
	CreatedBy          string             `json:"created_by"`
	CreatedAt          time.Time          `json:"created_at"`
}

func (s *Store) CreateEquivalenceProof(repo, campaignID, actor, digest string, in EquivalenceProof) (Campaign, EquivalenceProof, error) {
	var campaign Campaign
	var out EquivalenceProof
	e := s.lock(func() error {
		v, e := s.read(repo, campaignID)
		if e != nil {
			return e
		}
		found := false
		for _, target := range v.Targets {
			if target.ID == in.TargetID && target.Kind == "repository" {
				found = true
			}
		}
		if !found || strings.TrimSpace(in.RequestID) == "" || len(in.TargetRevision) != 40 || len(in.SourceRevision) != 40 || len(in.EvidenceRequirements) == 0 || len(in.Scenarios) == 0 || len(in.OrdinaryChecks) == 0 {
			return ErrInvalid
		}
		for _, x := range v.EquivalenceProofs {
			if x.RequestID == in.RequestID {
				if x.RequestDigest != digest {
					return ErrConflict
				}
				campaign, out = v, x
				return nil
			}
		}
		for _, scenario := range in.Scenarios {
			if strings.TrimSpace(scenario.Name) == "" || len(scenario.Coverage) == 0 || (scenario.State == "unsupported" && len(scenario.SubstituteEvidence) == 0) || (scenario.State != "unsupported" && scenario.CheckRunID == "") {
				return ErrInvalid
			}
			for _, citation := range scenario.SubstituteEvidence {
				if strings.TrimSpace(citation.Kind) == "" || strings.TrimSpace(citation.Reference) == "" || citation.Revision != in.TargetRevision {
					return ErrInvalid
				}
			}
		}
		in.ID, in.Version, in.RequestDigest, in.CreatedBy, in.CreatedAt = randomID(), 1, digest, actor, s.now()
		in.Authority = "equivalence evidence grants no Git, check, review, merge, release, deployment, or target authority"
		v.EquivalenceProofs = append(v.EquivalenceProofs, in)
		if e = s.write(v); e != nil {
			return e
		}
		campaign, out = v, in
		return nil
	})
	return campaign, out, e
}

func (s *Store) DecideEquivalenceProof(repo, campaignID, proofID, owner, decision, rationale string, expected int) (Campaign, EquivalenceProof, error) {
	var campaign Campaign
	var out EquivalenceProof
	e := s.lock(func() error {
		v, e := s.read(repo, campaignID)
		if e != nil {
			return e
		}
		for i := range v.EquivalenceProofs {
			p := &v.EquivalenceProofs[i]
			if p.ID != proofID {
				continue
			}
			if p.Version != expected {
				return ErrProofVersion
			}
			if !map[string]bool{"accepted": true, "rejected": true}[decision] || strings.TrimSpace(rationale) == "" {
				return ErrInvalid
			}
			p.OwnerDecisions = append(p.OwnerDecisions, OwnerDecision{OwnerID: owner, Decision: decision, Rationale: rationale, CreatedAt: s.now()})
			p.Version++
			if decision == "rejected" {
				p.State = "rejected"
			} else if p.State == "demonstrated" {
				p.State = "accepted"
			}
			if e = s.write(v); e != nil {
				return e
			}
			campaign, out = v, *p
			return nil
		}
		return ErrNotFound
	})
	return campaign, out, e
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
func (s *Store) Create(v Campaign, actor, digest string) (Campaign, error) {
	if validate(v) != nil {
		return Campaign{}, ErrInvalid
	}
	var out Campaign
	e := s.lock(func() error {
		values, e := s.list(v.RepositoryID)
		if e != nil {
			return e
		}
		for _, x := range values {
			if x.RequestID == v.RequestID {
				if x.RequestDigest != digest {
					return ErrConflict
				}
				out = x
				return nil
			}
		}
		v.ID = randomID()
		v.RequestDigest = digest
		v.CreatedBy = actor
		v.CreatedAt = s.now()
		out = v
		return s.write(v)
	})
	return out, e
}
func (s *Store) Get(repo, id string) (Campaign, error) {
	var out Campaign
	e := s.lock(func() error { var x error; out, x = s.read(repo, id); return x })
	return out, e
}
func (s *Store) List(repo string) ([]Campaign, error) {
	var out []Campaign
	e := s.lock(func() error { var x error; out, x = s.list(repo); return x })
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, e
}
func (s *Store) CreateAssessment(repo, campaignID, actor string, in Assessment) (Campaign, Assessment, error) {
	var campaign Campaign
	var out Assessment
	e := s.lock(func() error {
		v, e := s.read(repo, campaignID)
		if e != nil {
			return e
		}
		found := false
		for _, target := range v.Targets {
			if target.ID == in.TargetID && target.Kind == "repository" {
				found = true
			}
		}
		comparisonKinds := map[string]bool{"histories": true, "symbols": true, "dependencies": true, "interfaces": true, "schemas": true, "prior_fixes": true, "release_commitments": true}
		seenKinds := map[string]bool{}
		for _, comparison := range in.Comparisons {
			if !comparisonKinds[comparison.Kind] || seenKinds[comparison.Kind] || strings.TrimSpace(comparison.Summary) == "" {
				return ErrInvalid
			}
			seenKinds[comparison.Kind] = true
		}
		if !found || !validClassification(in.Classification) || len(in.TargetRevision) != 40 || len(in.SourceRevision) != 40 || len(seenKinds) != len(comparisonKinds) {
			return ErrInvalid
		}
		for _, existing := range v.Assessments {
			if existing.TargetID == in.TargetID && existing.TargetRevision == in.TargetRevision && existing.SourceRevision == in.SourceRevision {
				campaign = v
				out = existing
				return nil
			}
		}
		in.ID, in.Version, in.CreatedBy, in.CreatedAt = randomID(), 1, actor, s.now()
		in.Entries = []AssessmentEntry{}
		v.Assessments = append(v.Assessments, in)
		if e = s.write(v); e != nil {
			return e
		}
		campaign, out = v, in
		return nil
	})
	return campaign, out, e
}
func (s *Store) AddAssessmentEntry(repo, campaignID, assessmentID, actor, actorKind string, expected int, in AssessmentEntry) (Campaign, Assessment, error) {
	var campaign Campaign
	var out Assessment
	e := s.lock(func() error {
		v, e := s.read(repo, campaignID)
		if e != nil {
			return e
		}
		for i := range v.Assessments {
			a := &v.Assessments[i]
			if a.ID != assessmentID {
				continue
			}
			if a.Version != expected {
				return ErrVersion
			}
			if !map[string]bool{"finding": true, "risk": true, "uncertainty": true, "owner_acknowledgement": true}[in.Kind] || strings.TrimSpace(in.Body) == "" || len(in.Body) > 8000 || len(in.Citations) == 0 || len(in.Citations) > 20 {
				return ErrInvalid
			}
			for _, c := range in.Citations {
				if strings.TrimSpace(c.Kind) == "" || strings.TrimSpace(c.Reference) == "" || (c.Revision != "" && len(c.Revision) != 40) {
					return ErrInvalid
				}
			}
			in.ID, in.ActorID, in.ActorKind, in.CreatedAt = randomID(), actor, actorKind, s.now()
			a.Entries = append(a.Entries, in)
			a.Version++
			out = *a
			if e = s.write(v); e != nil {
				return e
			}
			campaign = v
			return nil
		}
		return ErrNotFound
	})
	return campaign, out, e
}
func (s *Store) LinkContribution(repo, campaignID, actor string, in Contribution) (Campaign, Contribution, error) {
	var campaign Campaign
	var out Contribution
	e := s.lock(func() error {
		v, e := s.read(repo, campaignID)
		if e != nil {
			return e
		}
		var assessment *Assessment
		for i := range v.Assessments {
			if v.Assessments[i].ID == in.AssessmentID && v.Assessments[i].TargetID == in.TargetID {
				assessment = &v.Assessments[i]
			}
		}
		if assessment == nil || assessment.Version != in.AssessmentVersion || assessment.TargetRevision != in.TargetRevision || !map[string]bool{"direct": true, "adapted": true}[in.Application] || (in.Application == "adapted" && strings.TrimSpace(in.Deviation) == "") || !map[string]bool{"local_branch": true, "fork": true, "federated": true}[in.Topology] || len(in.ProposalID) != 32 || len(in.TaskIDs) == 0 {
			return ErrInvalid
		}
		for _, existing := range v.Contributions {
			if existing.TargetID == in.TargetID {
				if existing.AssessmentID != in.AssessmentID || existing.AssessmentVersion != in.AssessmentVersion || existing.ProposalID != in.ProposalID {
					return ErrConflict
				}
				campaign, out = v, existing
				return nil
			}
		}
		in.ID, in.PublishedBy, in.PublishedAt = randomID(), actor, s.now()
		in.Authority = "ordinary target repository permissions govern sessions, branches, forks, pulls, review, and merge"
		v.Contributions = append(v.Contributions, in)
		if e = s.write(v); e != nil {
			return e
		}
		campaign, out = v, in
		return nil
	})
	return campaign, out, e
}
func validClassification(v string) bool {
	return map[string]bool{"directly_applicable": true, "already_satisfied": true, "adaptation_required": true, "conflicting": true, "not_applicable": true}[v]
}
func validate(v Campaign) error {
	kinds := map[string]bool{"merged_pull": true, "security_repair": true, "regression_correction": true, "policy_change": true, "package_release": true, "interface_evolution": true}
	if v.RequestID == "" || v.RepositoryID == "" || strings.TrimSpace(v.Title) == "" || strings.TrimSpace(v.Intent) == "" || !kinds[v.Source.Kind] || v.Source.ResourceID == "" || v.Source.RepositoryID != v.RepositoryID || len(v.Source.Commits) == 0 || len(v.AcceptanceCriteria) == 0 || len(v.Targets) == 0 {
		return ErrInvalid
	}
	ids := map[string]bool{}
	for _, t := range v.Targets {
		if t.ID == "" || ids[t.ID] || (t.Kind != "repository" && t.Kind != "package") || len(t.OwnerIDs) == 0 || t.Deadline.IsZero() {
			return ErrInvalid
		}
		ids[t.ID] = true
		if t.Kind == "repository" && (t.RepositoryID == "" || t.ReleaseLine == "") {
			return ErrInvalid
		}
		if t.Kind == "package" && (t.Package == "" || t.ReleaseLine == "") {
			return ErrInvalid
		}
	}
	for _, t := range v.Targets {
		for _, d := range t.DependsOn {
			if !ids[d] || d == t.ID {
				return ErrInvalid
			}
		}
	}
	dependencies := map[string][]string{}
	for _, t := range v.Targets {
		dependencies[t.ID] = t.DependsOn
	}
	visiting, visited := map[string]bool{}, map[string]bool{}
	var cyclic func(string) bool
	cyclic = func(id string) bool {
		if visiting[id] {
			return true
		}
		if visited[id] {
			return false
		}
		visiting[id] = true
		for _, dependency := range dependencies[id] {
			if cyclic(dependency) {
				return true
			}
		}
		visiting[id] = false
		visited[id] = true
		return false
	}
	for id := range ids {
		if cyclic(id) {
			return ErrInvalid
		}
	}
	if v.CompletionPolicy.Mode != "all" && v.CompletionPolicy.Mode != "minimum" && v.CompletionPolicy.Mode != "ordered" {
		return ErrInvalid
	}
	if v.CompletionPolicy.Mode == "minimum" && (v.CompletionPolicy.MinimumTargets < 1 || v.CompletionPolicy.MinimumTargets > len(v.Targets)) {
		return ErrInvalid
	}
	return nil
}
func (s *Store) list(repo string) ([]Campaign, error) {
	out := []Campaign{}
	entries, e := os.ReadDir(filepath.Join(s.root, repo))
	if os.IsNotExist(e) {
		return out, nil
	}
	if e != nil {
		return nil, e
	}
	for _, x := range entries {
		if x.IsDir() || !strings.HasSuffix(x.Name(), ".json") {
			continue
		}
		b, e := os.ReadFile(filepath.Join(s.root, repo, x.Name()))
		if e != nil {
			return nil, e
		}
		var v Campaign
		if json.Unmarshal(b, &v) != nil {
			return nil, ErrInvalid
		}
		out = append(out, v)
	}
	return out, nil
}
func (s *Store) read(repo, id string) (Campaign, error) {
	b, e := os.ReadFile(filepath.Join(s.root, repo, id+".json"))
	if os.IsNotExist(e) {
		return Campaign{}, ErrNotFound
	}
	if e != nil {
		return Campaign{}, e
	}
	var v Campaign
	if json.Unmarshal(b, &v) != nil {
		return Campaign{}, ErrInvalid
	}
	return v, nil
}
func (s *Store) write(v Campaign) error {
	dir := filepath.Join(s.root, v.RepositoryID)
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	tmp, e := os.CreateTemp(dir, ".campaign-*")
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
	if c := tmp.Close(); e == nil {
		e = c
	}
	if e == nil {
		e = os.Rename(name, filepath.Join(dir, v.ID+".json"))
	}
	if e == nil {
		d, x := os.Open(dir)
		if x == nil {
			e = d.Sync()
			d.Close()
		}
	}
	return e
}
func (s *Store) lock(fn func() error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, e := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	if e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); e != nil {
		return e
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return fn()
}
func randomID() string { b := make([]byte, 12); _, _ = rand.Read(b); return hex.EncodeToString(b) }
