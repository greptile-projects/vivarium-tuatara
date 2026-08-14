package projectfunds

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Outcome funding allocates verified project-fund backing to an evaluable body of work.
// It deliberately grants no authority to perform or accept that work.
type OutcomeSource struct {
	Kind       string `json:"kind"`
	ID         string `json:"id"`
	Revision   string `json:"revision"`
	Visibility string `json:"visibility"`
}

type Milestone struct {
	ID                   string   `json:"id"`
	Title                string   `json:"title"`
	Budget               int64    `json:"budget"`
	AcceptanceCriteria   []string `json:"acceptance_criteria"`
	EvidenceRequirements []string `json:"evidence_requirements"`
	Dependencies         []string `json:"dependencies"`
}

type OutcomeTerms struct {
	Title                  string        `json:"title"`
	Source                 OutcomeSource `json:"source"`
	Scope                  string        `json:"scope"`
	AcceptanceCriteria     []string      `json:"acceptance_criteria"`
	EvidenceRequirements   []string      `json:"evidence_requirements"`
	Budget                 int64         `json:"budget"`
	Deadline               time.Time     `json:"deadline"`
	ContributorEligibility []string      `json:"contributor_eligibility"`
	AllocationMethod       string        `json:"allocation_method"`
	CancellationTerms      string        `json:"cancellation_terms"`
	Dependencies           []string      `json:"dependencies"`
	Risks                  []string      `json:"risks"`
	Conflicts              []string      `json:"conflicts"`
	Milestones             []Milestone   `json:"milestones"`
}

type OutcomeRevision struct {
	Version   int          `json:"version"`
	Terms     OutcomeTerms `json:"terms"`
	ActorID   string       `json:"actor_id"`
	Reason    string       `json:"reason"`
	CreatedAt time.Time    `json:"created_at"`
}

type Pledge struct {
	ID             string    `json:"id"`
	BackerID       string    `json:"backer_id"`
	Amount         int64     `json:"amount"`
	MilestoneID    string    `json:"milestone_id,omitempty"`
	IdempotencyKey string    `json:"idempotency_key"`
	Status         string    `json:"status"`
	Note           string    `json:"note"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type Replan struct {
	Kind      string    `json:"kind"`
	ActorID   string    `json:"actor_id"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"created_at"`
}

type Diagnostic struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

type FundedOutcome struct {
	ID                 string              `json:"id"`
	RepositoryID       string              `json:"repository_id"`
	FundID             string              `json:"fund_id"`
	Version            int                 `json:"version"`
	Status             string              `json:"status"`
	Revisions          []OutcomeRevision   `json:"revisions"`
	Pledges            []Pledge            `json:"pledges"`
	Replans            []Replan            `json:"replans"`
	Pledged            int64               `json:"pledged"`
	MilestonePledged   map[string]int64    `json:"milestone_pledged"`
	Diagnostics        []Diagnostic        `json:"diagnostics"`
	CreatedBy          string              `json:"created_by"`
	CreatedAt          time.Time           `json:"created_at"`
	UpdatedAt          time.Time           `json:"updated_at"`
	AuthorityNote      string              `json:"authority_note"`
	DeliveryProposals  []DeliveryProposal  `json:"delivery_proposals"`
	DeliverySelections []DeliverySelection `json:"delivery_selections"`
}

func (s *Store) CreateOutcome(repositoryID, fundID, actor string, terms OutcomeTerms) (FundedOutcome, error) {
	var out FundedOutcome
	err := s.lock(func() error {
		if !validOutcomeID(fundID) {
			return ErrNotFound
		}
		fund, err := s.read(fundID)
		if err != nil || fund.RepositoryID != repositoryID {
			return ErrNotFound
		}
		if !validOutcomeTerms(terms) {
			return ErrInvalid
		}
		now := s.now()
		out = FundedOutcome{ID: randomID(), RepositoryID: repositoryID, FundID: fundID, Version: 1, Status: "open", CreatedBy: actor, CreatedAt: now, UpdatedAt: now, AuthorityNote: "Funding and pledges grant no repository, Git, task, credential, review, acceptance, merge, deployment, or security authority."}
		out.Revisions = []OutcomeRevision{{Version: 1, Terms: normalizeOutcomeTerms(terms), ActorID: actor, Reason: "initial funding contract", CreatedAt: now}}
		out.Replans = []Replan{{Kind: "insufficient_funds", ActorID: actor, Reason: "Funding opened below its declared budget.", CreatedAt: now}}
		if err := s.writeOutcome(out); err != nil {
			return err
		}
		return s.projectOutcome(&out)
	})
	return out, err
}

func (s *Store) ListOutcomes(repositoryID string) ([]FundedOutcome, error) {
	var out []FundedOutcome
	err := s.lock(func() error {
		entries, err := os.ReadDir(s.outcomeRoot())
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			v, err := s.readOutcome(strings.TrimSuffix(entry.Name(), ".json"))
			if err != nil {
				return err
			}
			if v.RepositoryID == repositoryID {
				if err := s.projectOutcome(&v); err != nil {
					return err
				}
				out = append(out, v)
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
		return nil
	})
	return out, err
}

func (s *Store) GetOutcome(id string) (FundedOutcome, error) {
	var out FundedOutcome
	err := s.lock(func() error {
		var err error
		out, err = s.readOutcome(id)
		if err != nil {
			return err
		}
		return s.projectOutcome(&out)
	})
	return out, err
}

func (s *Store) ReviseOutcome(id, actor string, expected int, terms OutcomeTerms, reason string) (FundedOutcome, error) {
	var out FundedOutcome
	err := s.lock(func() error {
		v, err := s.readOutcome(id)
		if err != nil {
			return err
		}
		if v.Version != expected || v.Status != "open" {
			return ErrConflict
		}
		if !validOutcomeTerms(terms) || !validOutcomeText(reason, 5000) {
			return ErrInvalid
		}
		now := s.now()
		v.Version++
		v.UpdatedAt = now
		v.Revisions = append(v.Revisions, OutcomeRevision{Version: v.Version, Terms: normalizeOutcomeTerms(terms), ActorID: actor, Reason: strings.TrimSpace(reason), CreatedAt: now})
		for i := range v.Pledges {
			if v.Pledges[i].Status == "active" {
				v.Pledges[i].Status = "reconfirmation_required"
				v.Pledges[i].UpdatedAt = now
			}
		}
		v.Replans = append(v.Replans, Replan{Kind: "changed_scope", ActorID: actor, Reason: strings.TrimSpace(reason), CreatedAt: now})
		if err := s.writeOutcome(v); err != nil {
			return err
		}
		out = v
		return s.projectOutcome(&out)
	})
	return out, err
}

func (s *Store) PledgeOutcome(id, actor, milestoneID string, amount int64, key, note string, expected int) (FundedOutcome, error) {
	var out FundedOutcome
	err := s.lock(func() error {
		v, err := s.readOutcome(id)
		if err != nil {
			return err
		}
		if v.Version != expected || v.Status != "open" {
			return ErrConflict
		}
		if amount <= 0 || !validOutcomeText(key, 300) || len(note) > 5000 || !validMilestone(v.Revisions[len(v.Revisions)-1].Terms, milestoneID) {
			return ErrInvalid
		}
		for _, p := range v.Pledges {
			if p.IdempotencyKey == key {
				return ErrConflict
			}
		}
		now := s.now()
		v.Version++
		v.UpdatedAt = now
		v.Pledges = append(v.Pledges, Pledge{ID: randomID(), BackerID: actor, Amount: amount, MilestoneID: milestoneID, IdempotencyKey: key, Status: "active", Note: strings.TrimSpace(note), CreatedAt: now, UpdatedAt: now})
		if err := s.writeOutcome(v); err != nil {
			return err
		}
		out = v
		return s.projectOutcome(&out)
	})
	return out, err
}

func (s *Store) ChangePledge(id, pledgeID, actor, action, reason string, expected int) (FundedOutcome, error) {
	var out FundedOutcome
	err := s.lock(func() error {
		v, err := s.readOutcome(id)
		if err != nil {
			return err
		}
		if v.Version != expected {
			return ErrConflict
		}
		if !validOutcomeText(reason, 5000) {
			return ErrInvalid
		}
		idx := -1
		for i := range v.Pledges {
			if v.Pledges[i].ID == pledgeID && v.Pledges[i].BackerID == actor {
				idx = i
				break
			}
		}
		if idx < 0 {
			return ErrForbidden
		}
		now := s.now()
		switch action {
		case "withdraw":
			if v.Pledges[idx].Status == "withdrawn" {
				return ErrConflict
			}
			v.Pledges[idx].Status = "withdrawn"
			v.Replans = append(v.Replans, Replan{Kind: "withdrawn_backing", ActorID: actor, Reason: strings.TrimSpace(reason), CreatedAt: now})
		case "reconfirm":
			if v.Pledges[idx].Status != "reconfirmation_required" || v.Status != "open" {
				return ErrConflict
			}
			v.Pledges[idx].Status = "active"
		default:
			return ErrInvalid
		}
		v.Pledges[idx].UpdatedAt = now
		v.Version++
		v.UpdatedAt = now
		if err := s.writeOutcome(v); err != nil {
			return err
		}
		out = v
		return s.projectOutcome(&out)
	})
	return out, err
}

func (s *Store) CancelOutcome(id, actor, reason string, expected int) (FundedOutcome, error) {
	var out FundedOutcome
	err := s.lock(func() error {
		v, err := s.readOutcome(id)
		if err != nil {
			return err
		}
		if v.Version != expected || v.Status != "open" {
			return ErrConflict
		}
		if !validOutcomeText(reason, 5000) {
			return ErrInvalid
		}
		now := s.now()
		v.Version++
		v.Status = "cancelled"
		v.UpdatedAt = now
		v.Replans = append(v.Replans, Replan{Kind: "cancelled", ActorID: actor, Reason: strings.TrimSpace(reason), CreatedAt: now})
		if err := s.writeOutcome(v); err != nil {
			return err
		}
		out = v
		return s.projectOutcome(&out)
	})
	return out, err
}

func (s *Store) projectOutcome(v *FundedOutcome) error {
	v.Pledged = 0
	v.MilestonePledged = map[string]int64{}
	v.Diagnostics = nil
	for _, p := range v.Pledges {
		if p.Status == "active" {
			v.Pledged += p.Amount
			v.MilestonePledged[p.MilestoneID] += p.Amount
		}
	}
	if len(v.Revisions) == 0 {
		return ErrInvalid
	}
	terms := v.Revisions[len(v.Revisions)-1].Terms
	if v.Pledged < terms.Budget {
		v.Diagnostics = append(v.Diagnostics, Diagnostic{Kind: "insufficient_funds", Message: "Active backing is below the declared outcome budget."})
	}
	if terms.Source.Visibility == "embargoed" {
		v.Diagnostics = append(v.Diagnostics, Diagnostic{Kind: "embargoed_work", Message: "This outcome is permission-bounded; public solicitation and evidence are withheld."})
	}
	if len(v.Revisions) > 1 {
		v.Diagnostics = append(v.Diagnostics, Diagnostic{Kind: "changed_scope", Message: "The contract changed; earlier pledges require explicit reconfirmation."})
	}
	for _, p := range v.Pledges {
		if p.Status == "withdrawn" {
			v.Diagnostics = append(v.Diagnostics, Diagnostic{Kind: "withdrawn_backing", Message: "A backer withdrew under the declared cancellation terms."})
			break
		}
	}
	entries, err := os.ReadDir(s.outcomeRoot())
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	fund, fundErr := s.read(v.FundID)
	overallocated := false
	overlapping := false
	allocated := int64(0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		other, err := s.readOutcome(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			return err
		}
		if other.FundID == v.FundID && other.Status == "open" && fundErr == nil {
			for _, pledge := range other.Pledges {
				if pledge.Status != "active" {
					continue
				}
				if pledge.Amount > fund.Balances.Available-allocated {
					overallocated = true
				} else {
					allocated += pledge.Amount
				}
			}
		}
		if other.ID == v.ID {
			continue
		}
		if other.RepositoryID == v.RepositoryID && other.Status == "open" && len(other.Revisions) > 0 {
			a, b := terms.Source, other.Revisions[len(other.Revisions)-1].Terms.Source
			if a.Kind == b.Kind && a.ID == b.ID {
				overlapping = true
			}
		}
	}
	if overlapping {
		v.Diagnostics = append(v.Diagnostics, Diagnostic{Kind: "overlapping_award", Message: "Another open funding contract targets this outcome; allocation must be reconciled."})
	}
	if overallocated {
		v.Diagnostics = append(v.Diagnostics, Diagnostic{Kind: "unsettled_backing", Message: "Active pledges across open outcomes exceed cryptographically settled fund value."})
	}
	return nil
}

func validOutcomeTerms(t OutcomeTerms) bool {
	kinds := []string{"issue", "roadmap_outcome", "proposal", "stewardship_opportunity", "incident_follow_up", "security_repair"}
	methods := []string{"first_accepted", "proportional", "maintainer_selection", "milestone_claim"}
	if !validOutcomeText(t.Title, 500) || !contains(kinds, t.Source.Kind) || !validOutcomeText(t.Source.ID, 300) || !validOutcomeText(t.Source.Revision, 300) || !contains([]string{"public", "participants", "embargoed"}, t.Source.Visibility) || !validOutcomeText(t.Scope, 10000) || !validOutcomeTexts(t.AcceptanceCriteria, 100, 5000) || !validOutcomeTexts(t.EvidenceRequirements, 100, 5000) || t.Budget <= 0 || t.Deadline.IsZero() || !validOutcomeTexts(t.ContributorEligibility, 100, 500) || !contains(methods, t.AllocationMethod) || !validOutcomeText(t.CancellationTerms, 10000) || !validOptionalOutcomeTexts(t.Dependencies, 100, 1000) || !validOptionalOutcomeTexts(t.Risks, 100, 1000) || !validOptionalOutcomeTexts(t.Conflicts, 100, 1000) || len(t.Milestones) > 100 {
		return false
	}
	seen := map[string]bool{}
	var total int64
	for _, m := range t.Milestones {
		if !validOutcomeText(m.ID, 200) || seen[m.ID] || !validOutcomeText(m.Title, 500) || m.Budget <= 0 || m.Budget > t.Budget-total || !validOutcomeTexts(m.AcceptanceCriteria, 100, 5000) || !validOutcomeTexts(m.EvidenceRequirements, 100, 5000) || !validOptionalOutcomeTexts(m.Dependencies, 100, 200) {
			return false
		}
		seen[m.ID] = true
		total += m.Budget
		for _, d := range m.Dependencies {
			if d == m.ID {
				return false
			}
		}
	}
	return len(t.Milestones) == 0 || total == t.Budget
}

func validOutcomeText(v string, max int) bool {
	n := len(strings.TrimSpace(v))
	return n > 0 && n <= max
}
func validOutcomeTexts(v []string, count, max int) bool {
	return len(v) > 0 && validOptionalOutcomeTexts(v, count, max)
}
func validOptionalOutcomeTexts(v []string, count, max int) bool {
	if len(v) > count {
		return false
	}
	for _, x := range v {
		if !validOutcomeText(x, max) {
			return false
		}
	}
	return true
}

func normalizeOutcomeTerms(t OutcomeTerms) OutcomeTerms {
	t.Title = strings.TrimSpace(t.Title)
	t.Scope = strings.TrimSpace(t.Scope)
	t.CancellationTerms = strings.TrimSpace(t.CancellationTerms)
	return t
}
func validMilestone(t OutcomeTerms, id string) bool {
	if id == "" {
		return true
	}
	for _, m := range t.Milestones {
		if m.ID == id {
			return true
		}
	}
	return false
}
func (s *Store) outcomeRoot() string { return filepath.Join(s.root, "outcomes") }
func (s *Store) readOutcome(id string) (FundedOutcome, error) {
	var v FundedOutcome
	if !validOutcomeID(id) {
		return v, ErrNotFound
	}
	b, err := os.ReadFile(filepath.Join(s.outcomeRoot(), id+".json"))
	if os.IsNotExist(err) {
		return v, ErrNotFound
	}
	if err != nil {
		return v, err
	}
	err = json.Unmarshal(b, &v)
	return v, err
}
func validOutcomeID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, r := range id {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}
func (s *Store) writeOutcome(v FundedOutcome) error {
	if err := os.MkdirAll(s.outcomeRoot(), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.outcomeRoot(), "outcome-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(b)
	}
	if err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(name, filepath.Join(s.outcomeRoot(), v.ID+".json"))
	}
	return err
}
