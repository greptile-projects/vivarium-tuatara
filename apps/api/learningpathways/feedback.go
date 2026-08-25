package learningpathways

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Outcome is deliberately learner-published. The ledger never infers activity
// from page views or workspace inspection, and a subject can keep an outcome
// private while still preserving it as their own historical evidence.
type Outcome struct {
	ID             string    `json:"id"`
	RequestID      string    `json:"request_id"`
	RepositoryID   string    `json:"repository_id"`
	PathwaySlug    string    `json:"pathway_slug"`
	PathwayVersion int       `json:"pathway_version"`
	ModuleID       string    `json:"module_id,omitempty"`
	ActorID        string    `json:"actor_id,omitempty"`
	Kind           string    `json:"kind"`
	State          string    `json:"state"`
	Detail         string    `json:"detail,omitempty"`
	Visibility     string    `json:"visibility"`
	Consent        bool      `json:"consent"`
	OccurredAt     time.Time `json:"occurred_at"`
}

type Finding struct {
	ID             string    `json:"id"`
	RequestID      string    `json:"request_id"`
	RepositoryID   string    `json:"repository_id"`
	PathwaySlug    string    `json:"pathway_slug"`
	PathwayVersion int       `json:"pathway_version"`
	Kind           string    `json:"kind"`
	Summary        string    `json:"summary"`
	OutcomeIDs     []string  `json:"outcome_ids"`
	CreatedBy      string    `json:"created_by"`
	CreatedAt      time.Time `json:"created_at"`
	Stale          bool      `json:"stale"`
}
type UpdateProposal struct {
	ID                        string     `json:"id"`
	RequestID                 string     `json:"request_id"`
	RepositoryID              string     `json:"repository_id"`
	PathwaySlug               string     `json:"pathway_slug"`
	BaseVersion               int        `json:"base_version"`
	FindingID                 string     `json:"finding_id"`
	TargetKind                string     `json:"target_kind"`
	TargetID                  string     `json:"target_id"`
	Summary                   string     `json:"summary"`
	MaterialRequirementChange bool       `json:"material_requirement_change"`
	Status                    string     `json:"status"`
	ProposedBy                string     `json:"proposed_by"`
	ReviewedBy                string     `json:"reviewed_by,omitempty"`
	ReviewRationale           string     `json:"review_rationale,omitempty"`
	CreatedAt                 time.Time  `json:"created_at"`
	ReviewedAt                *time.Time `json:"reviewed_at,omitempty"`
	Stale                     bool       `json:"stale"`
	RevalidationRequired      bool       `json:"revalidation_required"`
}

func (s *Store) feedbackDir(repo, slug string) string {
	return filepath.Join(s.root, repo, slug, "feedback")
}
func (s *Store) AddOutcome(o Outcome) (Outcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validID(o.RepositoryID) || !validSlug(o.PathwaySlug) || !validID(o.ActorID) || !validRequestID(o.RequestID) || o.PathwayVersion < 1 || !oneOf(o.Kind, "module_completion", "recurring_question", "setup_failure", "assessment_gap", "mentor_load", "contribution_outcome", "reviewer_correction", "retention") || !oneOf(o.Visibility, "private", "maintainers", "aggregate") || !o.Consent || strings.TrimSpace(o.State) == "" || len(o.State) > 200 || len(o.Detail) > 1000 || credentialLike(o.State, o.Detail) {
		return Outcome{}, ErrInvalid
	}
	items, _ := s.outcomes(o.RepositoryID, o.PathwaySlug)
	for _, x := range items {
		if x.ActorID == o.ActorID && x.RequestID == o.RequestID {
			if x.Kind != o.Kind || x.State != o.State || x.PathwayVersion != o.PathwayVersion {
				return Outcome{}, ErrRequestChanged
			}
			return x, nil
		}
	}
	o.ID = randomID()
	o.OccurredAt = s.now().UTC()
	d := s.feedbackDir(o.RepositoryID, o.PathwaySlug)
	if os.MkdirAll(d, 0700) != nil || writeJSON(filepath.Join(d, "outcome-"+o.ID+".json"), o) != nil {
		return Outcome{}, ErrInvalid
	}
	return o, nil
}
func (s *Store) outcomes(repo, slug string) ([]Outcome, error) {
	es, e := os.ReadDir(s.feedbackDir(repo, slug))
	if errors.Is(e, os.ErrNotExist) {
		return []Outcome{}, nil
	}
	if e != nil {
		return nil, e
	}
	out := []Outcome{}
	for _, f := range es {
		if strings.HasPrefix(f.Name(), "outcome-") {
			var x Outcome
			if readJSON(filepath.Join(s.feedbackDir(repo, slug), f.Name()), &x) == nil {
				out = append(out, x)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].OccurredAt.Before(out[j].OccurredAt) })
	return out, nil
}
func (s *Store) Outcomes(repo, slug string) ([]Outcome, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.outcomes(repo, slug)
}
func (s *Store) AddFinding(f Finding) (Finding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validID(f.RepositoryID) || !validSlug(f.PathwaySlug) || !validRequestID(f.RequestID) || !validID(f.CreatedBy) || f.PathwayVersion < 1 || strings.TrimSpace(f.Summary) == "" || len(f.Summary) > 1000 || len(f.OutcomeIDs) == 0 || !oneOf(f.Kind, "completion", "questions", "setup", "assessment", "mentor_load", "contribution", "review_correction", "retention") || credentialLike("", f.Summary) {
		return Finding{}, ErrInvalid
	}
	outcomes, _ := s.outcomes(f.RepositoryID, f.PathwaySlug)
	allowed := map[string]bool{}
	aggregateCounts := map[string]int{}
	for _, x := range outcomes {
		if x.Visibility == "aggregate" {
			aggregateCounts[x.Kind]++
		}
	}
	for _, x := range outcomes {
		if x.Visibility == "maintainers" || x.Visibility == "aggregate" && aggregateCounts[x.Kind] >= 3 {
			allowed[x.ID] = true
		}
	}
	for _, id := range f.OutcomeIDs {
		if !allowed[id] {
			return Finding{}, ErrInvalid
		}
	}
	existing, _ := s.findings(f.RepositoryID, f.PathwaySlug)
	for _, x := range existing {
		if x.RequestID == f.RequestID {
			return x, nil
		}
	}
	f.ID = randomID()
	f.CreatedAt = s.now().UTC()
	d := s.feedbackDir(f.RepositoryID, f.PathwaySlug)
	if os.MkdirAll(d, 0700) != nil || writeJSON(filepath.Join(d, "finding-"+f.ID+".json"), f) != nil {
		return Finding{}, ErrInvalid
	}
	return f, nil
}
func (s *Store) findings(repo, slug string) ([]Finding, error) {
	es, e := os.ReadDir(s.feedbackDir(repo, slug))
	if errors.Is(e, os.ErrNotExist) {
		return []Finding{}, nil
	}
	if e != nil {
		return nil, e
	}
	out := []Finding{}
	for _, v := range es {
		if strings.HasPrefix(v.Name(), "finding-") {
			var x Finding
			if readJSON(filepath.Join(s.feedbackDir(repo, slug), v.Name()), &x) == nil {
				out = append(out, x)
			}
		}
	}
	return out, nil
}
func (s *Store) Findings(repo, slug string) ([]Finding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.findings(repo, slug)
}
func (s *Store) AddProposal(p UpdateProposal) (UpdateProposal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validID(p.RepositoryID) || !validSlug(p.PathwaySlug) || !validRequestID(p.RequestID) || !validID(p.ProposedBy) || p.BaseVersion < 1 || !oneOf(p.TargetKind, "documentation", "exercise", "workspace", "pathway") || strings.TrimSpace(p.Summary) == "" || len(p.Summary) > 1000 || credentialLike("", p.Summary) {
		return UpdateProposal{}, ErrInvalid
	}
	fs, _ := s.findings(p.RepositoryID, p.PathwaySlug)
	found := false
	for _, f := range fs {
		if f.ID == p.FindingID && f.PathwayVersion == p.BaseVersion {
			found = true
		}
	}
	if !found {
		return UpdateProposal{}, ErrInvalid
	}
	ps, _ := s.proposals(p.RepositoryID, p.PathwaySlug)
	for _, x := range ps {
		if x.RequestID == p.RequestID {
			return x, nil
		}
	}
	p.ID = randomID()
	p.Status = "proposed"
	p.CreatedAt = s.now().UTC()
	d := s.feedbackDir(p.RepositoryID, p.PathwaySlug)
	if os.MkdirAll(d, 0700) != nil || writeJSON(filepath.Join(d, "proposal-"+p.ID+".json"), p) != nil {
		return UpdateProposal{}, ErrInvalid
	}
	return p, nil
}
func (s *Store) proposals(repo, slug string) ([]UpdateProposal, error) {
	es, e := os.ReadDir(s.feedbackDir(repo, slug))
	if errors.Is(e, os.ErrNotExist) {
		return []UpdateProposal{}, nil
	}
	if e != nil {
		return nil, e
	}
	out := []UpdateProposal{}
	for _, v := range es {
		if strings.HasPrefix(v.Name(), "proposal-") {
			var x UpdateProposal
			if readJSON(filepath.Join(s.feedbackDir(repo, slug), v.Name()), &x) == nil {
				out = append(out, x)
			}
		}
	}
	return out, nil
}
func (s *Store) Proposals(repo, slug string) ([]UpdateProposal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.proposals(repo, slug)
}
func (s *Store) ReviewProposal(repo, slug, id, reviewer, decision, rationale string) (UpdateProposal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !oneOf(decision, "accepted", "rejected") || strings.TrimSpace(rationale) == "" || len(rationale) > 1000 {
		return UpdateProposal{}, ErrInvalid
	}
	path := filepath.Join(s.feedbackDir(repo, slug), "proposal-"+id+".json")
	var p UpdateProposal
	if readJSON(path, &p) != nil {
		return p, ErrNotFound
	}
	if p.Status != "proposed" {
		return p, ErrConflict
	}
	now := s.now().UTC()
	p.Status, p.ReviewedBy, p.ReviewRationale, p.ReviewedAt = decision, reviewer, rationale, &now
	if writeJSON(path, p) != nil {
		return p, ErrInvalid
	}
	return p, nil
}
