// Package adoptionworkspaces retains evidence-backed software fit evaluations.
package adoptionworkspaces

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

var (
	ErrNotFound  = errors.New("adoption workspace not found")
	ErrInvalid   = errors.New("invalid adoption workspace")
	ErrConflict  = errors.New("adoption workspace changed")
	ErrForbidden = errors.New("adoption workspace mutation forbidden")
)

type Source struct {
	Kind         string `json:"kind"`
	RepositoryID string `json:"repository_id,omitempty"`
	ResourceID   string `json:"resource_id"`
	Label        string `json:"label"`
	Resolution   string `json:"resolution"`
	Detail       string `json:"detail,omitempty"`
}

type Invitation struct {
	ID             string     `json:"id"`
	PrincipalType  string     `json:"principal_type"`
	PrincipalID    string     `json:"principal_id"`
	OrganizationID string     `json:"organization_id,omitempty"`
	Role           string     `json:"role"`
	Access         string     `json:"access"`
	Status         string     `json:"status"`
	InvitedBy      string     `json:"invited_by"`
	InvitedAt      time.Time  `json:"invited_at"`
	RespondedAt    *time.Time `json:"responded_at,omitempty"`
}
type Owner struct {
	PrincipalID    string `json:"principal_id"`
	Responsibility string `json:"responsibility"`
}
type Criterion struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Requirement string `json:"requirement"`
	Weight      int    `json:"weight"`
	OwnerID     string `json:"owner_id"`
}
type Evidence struct {
	ID              string    `json:"id"`
	Dimension       string    `json:"dimension"`
	Summary         string    `json:"summary"`
	Reference       string    `json:"reference"`
	RepositoryID    string    `json:"repository_id,omitempty"`
	ObservedVersion string    `json:"observed_version,omitempty"`
	Visibility      string    `json:"visibility"`
	Resolution      string    `json:"resolution"`
	Detail          string    `json:"detail,omitempty"`
	RecordedAt      time.Time `json:"recorded_at"`
}
type Candidate struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Provider        string     `json:"provider"`
	Version         string     `json:"version"`
	Revision        string     `json:"revision,omitempty"`
	SourceKind      string     `json:"source_kind"`
	SourceReference string     `json:"source_reference"`
	Evidence        []Evidence `json:"evidence"`
	FitStatus       string     `json:"fit_status"`
	Gaps            []string   `json:"gaps"`
}
type TrialSource struct {
	Kind         string `json:"kind"`
	RepositoryID string `json:"repository_id,omitempty"`
	ResourceID   string `json:"resource_id"`
	Revision     string `json:"revision"`
	Attestation  string `json:"attestation,omitempty"`
	Resolution   string `json:"resolution"`
}
type TrialDefinition struct {
	ID                 string         `json:"id"`
	CandidateID        string         `json:"candidate_id"`
	Source             TrialSource    `json:"source"`
	Packages           []string       `json:"packages"`
	APIs               []string       `json:"apis"`
	DataKind           string         `json:"data_kind"`
	DataDescription    string         `json:"data_description"`
	Journeys           []string       `json:"journeys"`
	Policies           []string       `json:"policies"`
	Setup              []string       `json:"setup"`
	Configuration      []string       `json:"configuration"`
	Commands           []string       `json:"commands"`
	IntegrationChanges []string       `json:"integration_changes"`
	MaximumCostCents   int64          `json:"maximum_cost_cents"`
	CreatedBy          string         `json:"created_by"`
	CreatedAt          time.Time      `json:"created_at"`
	Attempts           []TrialAttempt `json:"attempts"`
}
type TrialAttempt struct {
	ID              string    `json:"id"`
	Status          string    `json:"status"`
	Reproducible    bool      `json:"reproducible"`
	Checks          []string  `json:"checks"`
	Previews        []string  `json:"previews"`
	Measurements    []string  `json:"measurements"`
	CostCents       int64     `json:"cost_cents"`
	Findings        []string  `json:"findings"`
	UserFeedback    []string  `json:"user_feedback"`
	ArtifactDigests []string  `json:"artifact_digests"`
	RecordedBy      string    `json:"recorded_by"`
	RecordedAt      time.Time `json:"recorded_at"`
}
type Workspace struct {
	ID               string            `json:"id"`
	Version          int               `json:"version"`
	Title            string            `json:"title"`
	Outcome          string            `json:"outcome"`
	Source           Source            `json:"source"`
	RequiredJourneys []string          `json:"required_journeys"`
	Environments     []string          `json:"environments"`
	Constraints      []string          `json:"constraints"`
	BudgetCents      int64             `json:"budget_cents"`
	Currency         string            `json:"currency"`
	Owners           []Owner           `json:"owners"`
	Criteria         []Criterion       `json:"evaluation_criteria"`
	Candidates       []Candidate       `json:"candidates"`
	Invitations      []Invitation      `json:"invitations"`
	Trials           []TrialDefinition `json:"trials"`
	CreatedBy        string            `json:"created_by"`
	CreatedAt        time.Time         `json:"created_at"`
	UpdatedAt        time.Time         `json:"updated_at"`
}

type Viewer struct {
	PrincipalType  string
	PrincipalID    string
	OrganizationID string
	RepositoryID   string
}

type PendingInvitation struct {
	WorkspaceID string     `json:"workspace_id"`
	Version     int        `json:"version"`
	Invitation  Invitation `json:"invitation"`
}

type Store struct {
	root              string
	mu                sync.Mutex
	now               func() time.Time
	canReadRepository func(Viewer, string) bool
}

func New(root string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, ErrInvalid
	}
	if e := os.MkdirAll(root, 0700); e != nil {
		return nil, e
	}
	return &Store{root: root, now: time.Now, canReadRepository: func(Viewer, string) bool { return false }}, nil
}

func (s *Store) ConfigureRepositoryAccess(check func(Viewer, string) bool) {
	if check != nil {
		s.canReadRepository = check
	}
}
func id() string                { b := make([]byte, 16); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func text(v string, n int) bool { l := len(strings.TrimSpace(v)); return l > 0 && l <= n }
func safeText(v string, n int) bool {
	if !text(v, n) {
		return false
	}
	l := strings.ToLower(v)
	return !strings.Contains(l, "authorization: bearer") && !strings.Contains(l, "authorization: basic") && !strings.Contains(l, "-----begin private key") && !strings.Contains(l, "aws_secret_access_key") && !strings.Contains(l, "ghp_") && !strings.Contains(l, "github_pat_")
}
func revision(v string) bool { _, e := hex.DecodeString(v); return len(v) == 40 && e == nil }
func list(v []string, max int) bool {
	if len(v) == 0 || len(v) > max {
		return false
	}
	seen := map[string]bool{}
	for _, x := range v {
		x = strings.TrimSpace(x)
		if !text(x, 2000) || seen[x] {
			return false
		}
		seen[x] = true
	}
	return true
}
func (s *Store) lock() (*os.File, error) {
	f, e := os.OpenFile(filepath.Join(s.root, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e == nil {
		e = syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
	}
	return f, e
}
func (s *Store) path(v string) string { return filepath.Join(s.root, v+".json") }
func (s *Store) read(v string) (Workspace, error) {
	if len(v) != 32 {
		return Workspace{}, ErrNotFound
	}
	b, e := os.ReadFile(s.path(v))
	if errors.Is(e, os.ErrNotExist) {
		return Workspace{}, ErrNotFound
	}
	var x Workspace
	if e == nil {
		e = json.Unmarshal(b, &x)
	}
	return x, e
}
func (s *Store) write(x Workspace) error {
	b, e := json.Marshal(x)
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(s.root, ".adoption-*")
	if e != nil {
		return e
	}
	n := f.Name()
	defer os.Remove(n)
	_ = f.Chmod(0600)
	if _, e = f.Write(b); e == nil {
		e = f.Sync()
	}
	if ce := f.Close(); e == nil {
		e = ce
	}
	if e == nil {
		e = os.Rename(n, s.path(x.ID))
	}
	if e == nil {
		d, de := os.Open(s.root)
		if de == nil {
			e = d.Sync()
			_ = d.Close()
		} else {
			e = de
		}
	}
	return e
}

var dimensions = map[string]bool{"capabilities": true, "provenance": true, "support": true, "security": true, "data_use": true, "compatibility": true, "known_gaps": true}

func derive(c *Candidate) {
	c.Gaps = []string{}
	current := map[string]bool{}
	for i := range c.Evidence {
		e := &c.Evidence[i]
		if e.Resolution == "resolved" && e.ObservedVersion != "" && e.ObservedVersion != c.Version {
			e.Resolution = "stale"
			e.Detail = "Evidence describes a different candidate version"
		}
		if e.Resolution == "resolved" {
			current[e.Dimension] = true
		} else {
			c.Gaps = append(c.Gaps, e.Dimension+": "+e.Resolution)
		}
	}
	for d := range dimensions {
		if !current[d] {
			found := false
			for _, g := range c.Gaps {
				found = found || strings.HasPrefix(g, d+":")
			}
			if !found {
				c.Gaps = append(c.Gaps, d+": missing")
			}
		}
	}
	sort.Strings(c.Gaps)
	if len(c.Gaps) == 0 {
		c.FitStatus = "evidence_complete"
	} else {
		c.FitStatus = "undetermined"
	}
}
func valid(x Workspace) bool {
	if !text(x.Title, 200) || !text(x.Outcome, 10000) || !list(x.RequiredJourneys, 30) || !list(x.Environments, 30) || !list(x.Constraints, 30) || x.BudgetCents < 0 || !text(x.Currency, 10) || !map[string]bool{"roadmap_outcome": true, "support_gap": true, "incubator": true, "decision": true, "package": true, "api": true, "federated_repository": true}[x.Source.Kind] || !text(x.Source.ResourceID, 500) || !text(x.Source.Label, 300) || !map[string]bool{"resolved": true, "missing": true, "inaccessible": true, "stale": true}[x.Source.Resolution] || len(x.Owners) == 0 || len(x.Owners) > 30 || len(x.Criteria) == 0 || len(x.Criteria) > 50 || len(x.Candidates) == 0 || len(x.Candidates) > 20 {
		return false
	}
	for _, o := range x.Owners {
		if !text(o.PrincipalID, 100) || !text(o.Responsibility, 500) {
			return false
		}
	}
	for _, c := range x.Criteria {
		if !text(c.Name, 300) || !text(c.Requirement, 2000) || c.Weight < 1 || c.Weight > 100 || !text(c.OwnerID, 100) {
			return false
		}
	}
	for _, c := range x.Candidates {
		if !text(c.Name, 300) || !text(c.Provider, 300) || !text(c.Version, 200) || !text(c.SourceKind, 30) || !text(c.SourceReference, 1000) || len(c.Evidence) > 100 {
			return false
		}
		for _, e := range c.Evidence {
			if !dimensions[e.Dimension] || !text(e.Summary, 5000) || !text(e.Reference, 1000) || !map[string]bool{"public": true, "participants": true}[e.Visibility] || !map[string]bool{"resolved": true, "missing": true, "inaccessible": true}[e.Resolution] {
				return false
			}
		}
	}
	return true
}
func (s *Store) Create(x Workspace, actor string, invitations []Invitation) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, e := s.lock()
	if e != nil {
		return Workspace{}, e
	}
	defer f.Close()
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	if !valid(x) || !text(actor, 100) || len(invitations) > 50 {
		return Workspace{}, ErrInvalid
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	seen := map[string]bool{}
	for i := range invitations {
		v := &invitations[i]
		key := v.PrincipalType + ":" + v.PrincipalID
		if seen[key] || !map[string]bool{"human": true, "agent": true}[v.PrincipalType] || !text(v.PrincipalID, 100) || !map[string]bool{"provider_maintainer": true, "affected_user": true, "observer": true}[v.Role] {
			return Workspace{}, ErrInvalid
		}
		if v.PrincipalType == "agent" {
			if !text(v.OrganizationID, 100) || v.Role != "observer" {
				return Workspace{}, ErrInvalid
			}
			v.Access = "read"
			v.Status = "accepted"
		} else {
			v.Access = "comment"
			v.Status = "pending"
		}
		seen[key] = true
		v.ID = id()
		v.InvitedBy = actor
		v.InvitedAt = now
	}
	owner := false
	for _, o := range x.Owners {
		owner = owner || o.PrincipalID == actor
	}
	if !owner {
		return Workspace{}, ErrInvalid
	}
	for i := range x.Criteria {
		x.Criteria[i].ID = id()
	}
	for i := range x.Candidates {
		x.Candidates[i].ID = id()
		seenEvidence := map[string]bool{}
		for j := range x.Candidates[i].Evidence {
			v := &x.Candidates[i].Evidence[j]
			key := v.Dimension + ":" + v.Reference
			if seenEvidence[key] {
				return Workspace{}, ErrInvalid
			}
			seenEvidence[key] = true
			v.ID = id()
			v.RecordedAt = now
		}
		derive(&x.Candidates[i])
	}
	x.ID = id()
	x.Version = 1
	x.CreatedBy = actor
	x.CreatedAt = now
	x.UpdatedAt = now
	x.Invitations = invitations
	e = s.write(x)
	return x, e
}
func visible(x Workspace, viewer Viewer) bool {
	if viewer.PrincipalType == "human" && x.CreatedBy == viewer.PrincipalID {
		return true
	}
	for _, v := range x.Invitations {
		if v.PrincipalType == viewer.PrincipalType && v.PrincipalID == viewer.PrincipalID && v.Status == "accepted" && (v.PrincipalType != "agent" || v.OrganizationID == viewer.OrganizationID) {
			return true
		}
	}
	return false
}
func (s *Store) project(x Workspace, viewer Viewer) Workspace {
	if x.Source.RepositoryID != "" && !s.canReadRepository(viewer, x.Source.RepositoryID) {
		x.Source = Source{Kind: x.Source.Kind, Label: "Restricted source", Resolution: "inaccessible", Detail: "Starting context is outside this viewer's current read boundary"}
	}
	for i := range x.Candidates {
		out := []Evidence{}
		for _, e := range x.Candidates[i].Evidence {
			if e.RepositoryID != "" && !s.canReadRepository(viewer, e.RepositoryID) {
				out = append(out, Evidence{ID: e.ID, Dimension: e.Dimension, Summary: "Restricted evidence", Reference: "Restricted evidence", Visibility: e.Visibility, Resolution: "inaccessible", Detail: "Repository evidence is outside this viewer's current read boundary", RecordedAt: e.RecordedAt})
				continue
			}
			if e.Visibility == "public" || visible(x, viewer) {
				out = append(out, e)
			}
		}
		x.Candidates[i].Evidence = out
		derive(&x.Candidates[i])
	}
	for i := range x.Trials {
		if x.Trials[i].Source.RepositoryID != "" && !s.canReadRepository(viewer, x.Trials[i].Source.RepositoryID) {
			x.Trials[i].Source = TrialSource{Kind: x.Trials[i].Source.Kind, Resolution: "inaccessible"}
		}
	}
	return x
}
func (s *Store) Get(v string, viewer Viewer) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	x, e := s.read(v)
	if e != nil || !visible(x, viewer) {
		return Workspace{}, ErrNotFound
	}
	return s.project(x, viewer), nil
}
func (s *Store) List(viewer Viewer) ([]Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	files, e := filepath.Glob(filepath.Join(s.root, "*.json"))
	if e != nil {
		return nil, e
	}
	out := []Workspace{}
	for _, p := range files {
		b, e := os.ReadFile(p)
		var x Workspace
		if e == nil {
			e = json.Unmarshal(b, &x)
		}
		if e != nil {
			return nil, e
		}
		if visible(x, viewer) {
			out = append(out, s.project(x, viewer))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

func (s *Store) Pending(viewer Viewer) ([]PendingInvitation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if viewer.PrincipalType != "human" || viewer.PrincipalID == "" {
		return []PendingInvitation{}, nil
	}
	files, e := filepath.Glob(filepath.Join(s.root, "*.json"))
	if e != nil {
		return nil, e
	}
	out := []PendingInvitation{}
	for _, p := range files {
		b, readErr := os.ReadFile(p)
		var x Workspace
		if readErr == nil {
			readErr = json.Unmarshal(b, &x)
		}
		if readErr != nil {
			return nil, readErr
		}
		for _, invitation := range x.Invitations {
			if invitation.PrincipalType == "human" && invitation.PrincipalID == viewer.PrincipalID && invitation.Status == "pending" {
				out = append(out, PendingInvitation{WorkspaceID: x.ID, Version: x.Version, Invitation: invitation})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Invitation.InvitedAt.After(out[j].Invitation.InvitedAt) })
	return out, nil
}
func (s *Store) Consent(workspace, invitation, actor, decision string, expected int) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, e := s.lock()
	if e != nil {
		return Workspace{}, e
	}
	defer f.Close()
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	x, e := s.read(workspace)
	if e != nil {
		return Workspace{}, ErrNotFound
	}
	if x.Version != expected {
		return Workspace{}, ErrConflict
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	for i := range x.Invitations {
		v := &x.Invitations[i]
		if v.ID == invitation && v.PrincipalType == "human" && v.PrincipalID == actor && v.Status == "pending" && map[string]bool{"accepted": true, "declined": true}[decision] {
			v.Status = decision
			v.RespondedAt = &now
			x.Version++
			x.UpdatedAt = now
			e = s.write(x)
			if decision == "declined" {
				return Workspace{ID: x.ID, Version: x.Version, Invitations: []Invitation{*v}, UpdatedAt: x.UpdatedAt}, e
			}
			return s.project(x, Viewer{PrincipalType: "human", PrincipalID: actor}), e
		}
	}
	return Workspace{}, ErrInvalid
}

func participant(x Workspace, viewer Viewer) bool { return visible(x, viewer) }

// CreateTrial appends a reproducible definition. It confers no package, API,
// repository, runtime, credential, or deployment authority.
func (s *Store) CreateTrial(workspace string, in TrialDefinition, viewer Viewer, expected int) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, e := s.lock()
	if e != nil {
		return Workspace{}, e
	}
	defer f.Close()
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	x, e := s.read(workspace)
	if e != nil {
		return Workspace{}, ErrNotFound
	}
	if !participant(x, viewer) {
		return Workspace{}, ErrNotFound
	}
	if x.Version != expected {
		return Workspace{}, ErrConflict
	}
	candidate := false
	for _, c := range x.Candidates {
		candidate = candidate || c.ID == in.CandidateID
	}
	if !candidate || !map[string]bool{"attested_release": true, "exact_revision": true}[in.Source.Kind] || !text(in.Source.ResourceID, 500) || !revision(in.Source.Revision) || in.Source.Resolution != "resolved" || len(in.Packages) > 50 || len(in.APIs) > 50 || !map[string]bool{"synthetic": true, "permitted": true}[in.DataKind] || !safeText(in.DataDescription, 2000) || !list(in.Journeys, 30) || !list(in.Policies, 50) || !list(in.Setup, 50) || !list(in.Configuration, 50) || !list(in.Commands, 50) || !list(in.IntegrationChanges, 50) || in.MaximumCostCents < 0 {
		return Workspace{}, ErrInvalid
	}
	for _, values := range [][]string{in.Packages, in.APIs, in.Journeys, in.Policies, in.Setup, in.Configuration, in.Commands, in.IntegrationChanges} {
		for _, v := range values {
			if !safeText(v, 2000) {
				return Workspace{}, ErrInvalid
			}
		}
	}
	// A trial must exercise only journeys already declared by the adopter.
	allowed := map[string]bool{}
	for _, v := range x.RequiredJourneys {
		allowed[v] = true
	}
	for _, v := range in.Journeys {
		if !allowed[v] {
			return Workspace{}, ErrInvalid
		}
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	in.ID = id()
	in.CreatedBy = viewer.PrincipalID
	in.CreatedAt = now
	in.Attempts = []TrialAttempt{}
	x.Trials = append(x.Trials, in)
	x.Version++
	x.UpdatedAt = now
	e = s.write(x)
	return s.project(x, viewer), e
}

func (s *Store) RecordTrialAttempt(workspace, trial string, in TrialAttempt, viewer Viewer, expected int) (Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, e := s.lock()
	if e != nil {
		return Workspace{}, e
	}
	defer f.Close()
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	x, e := s.read(workspace)
	if e != nil {
		return Workspace{}, ErrNotFound
	}
	if !participant(x, viewer) {
		return Workspace{}, ErrNotFound
	}
	if x.Version != expected {
		return Workspace{}, ErrConflict
	}
	if !map[string]bool{"passed": true, "failed": true, "blocked": true, "non_reproducible": true}[in.Status] || in.CostCents < 0 {
		return Workspace{}, ErrInvalid
	}
	for _, values := range [][]string{in.Checks, in.Previews, in.Measurements, in.Findings, in.UserFeedback, in.ArtifactDigests} {
		if len(values) > 100 {
			return Workspace{}, ErrInvalid
		}
		for _, v := range values {
			if !safeText(v, 5000) {
				return Workspace{}, ErrInvalid
			}
		}
	}
	for i := range x.Trials {
		if x.Trials[i].ID == trial {
			spent := int64(0)
			for _, attempt := range x.Trials[i].Attempts {
				if attempt.CostCents < 0 || spent > x.Trials[i].MaximumCostCents-attempt.CostCents {
					return Workspace{}, ErrInvalid
				}
				spent += attempt.CostCents
			}
			if in.CostCents > x.Trials[i].MaximumCostCents-spent {
				return Workspace{}, ErrInvalid
			}
			now := s.now().UTC().Truncate(time.Microsecond)
			in.ID = id()
			in.RecordedBy = viewer.PrincipalID
			in.RecordedAt = now
			if in.Status == "non_reproducible" {
				in.Reproducible = false
			}
			x.Trials[i].Attempts = append(x.Trials[i].Attempts, in)
			x.Version++
			x.UpdatedAt = now
			e = s.write(x)
			return s.project(x, viewer), e
		}
	}
	return Workspace{}, ErrNotFound
}
