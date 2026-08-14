package projectfunds

import (
	"errors"
	"strings"
	"time"
)

// Delivery proposals keep contributor choice, compensation, and operational
// authority separate. Applicant eligibility is revalidated by the HTTP
// boundary; this store atomically records selection and its fund reservation.
type DeliveryApplicant struct {
	Kind           string `json:"kind"`
	ID             string `json:"id"`
	SubmittedBy    string `json:"submitted_by"`
	OrganizationID string `json:"organization_id,omitempty"`
}

type AttributedWork struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	URL  string `json:"url,omitempty"`
	Note string `json:"note"`
}

type DeliveryProposalTerms struct {
	Approach       string           `json:"approach"`
	Milestones     []string         `json:"milestones"`
	Cost           int64            `json:"cost"`
	Dependencies   []string         `json:"dependencies"`
	Availability   string           `json:"availability"`
	RequiredAccess []string         `json:"required_access"`
	RelevantWork   []AttributedWork `json:"relevant_work"`
}

type DeliveryTask struct {
	ID             string              `json:"id"`
	Title          string              `json:"title"`
	RecipientKind  string              `json:"recipient_kind"`
	RecipientID    string              `json:"recipient_id"`
	MilestoneIndex int                 `json:"milestone_index"`
	Dependencies   []string            `json:"dependencies"`
	Status         string              `json:"status"`
	ReviewerIDs    []string            `json:"reviewer_ids"`
	AwardAmount    int64               `json:"award_amount"`
	Reviews        []MilestoneReview   `json:"reviews"`
	Recoveries     []MilestoneRecovery `json:"recoveries"`
}

type DeliverySelection struct {
	ID             string            `json:"id"`
	ProposalIDs    []string          `json:"proposal_ids"`
	Disclosure     string            `json:"conflict_disclosure"`
	Rationale      string            `json:"rationale"`
	ReservedAmount int64             `json:"reserved_amount"`
	ReservationID  string            `json:"reservation_id"`
	Tasks          []DeliveryTask    `json:"tasks"`
	SelectedBy     string            `json:"selected_by"`
	SelectedAt     time.Time         `json:"selected_at"`
	Execution      DeliveryExecution `json:"execution"`
}

type DeliveryProposal struct {
	ID            string                `json:"id"`
	Applicant     DeliveryApplicant     `json:"applicant"`
	Terms         DeliveryProposalTerms `json:"terms"`
	Status        string                `json:"status"`
	SubmittedAt   time.Time             `json:"submitted_at"`
	AcceptedBy    string                `json:"accepted_by,omitempty"`
	AcceptedAt    *time.Time            `json:"accepted_at,omitempty"`
	SelectionID   string                `json:"selection_id,omitempty"`
	AuthorityNote string                `json:"authority_note"`
}

func (s *Store) SubmitDeliveryProposal(outcomeID string, applicant DeliveryApplicant, terms DeliveryProposalTerms) (FundedOutcome, error) {
	var out FundedOutcome
	err := s.lock(func() error {
		v, err := s.readOutcome(outcomeID)
		if err != nil {
			return err
		}
		if v.Status != "open" || !validDeliveryApplicant(applicant) || !validDeliveryTerms(terms) {
			return ErrInvalid
		}
		for _, proposal := range v.DeliveryProposals {
			if proposal.Applicant.Kind == applicant.Kind && proposal.Applicant.ID == applicant.ID && (proposal.Status == "submitted" || proposal.Status == "accepted") {
				return ErrConflict
			}
		}
		now := s.now()
		v.Version++
		v.UpdatedAt = now
		v.DeliveryProposals = append(v.DeliveryProposals, DeliveryProposal{ID: randomID(), Applicant: applicant, Terms: normalizeDeliveryTerms(terms), Status: "submitted", SubmittedAt: now, AuthorityNote: deliveryAuthorityNote})
		if err := s.writeOutcome(v); err != nil {
			return err
		}
		out = v
		return s.projectOutcome(&out)
	})
	return out, err
}

func (s *Store) AcceptDeliveryProposal(outcomeID, proposalID, actor string, expected int) (FundedOutcome, error) {
	var out FundedOutcome
	err := s.lock(func() error {
		v, err := s.readOutcome(outcomeID)
		if err != nil {
			return err
		}
		if v.Version != expected || v.Status != "open" {
			return ErrConflict
		}
		idx := deliveryProposalIndex(v, proposalID)
		if idx < 0 {
			return ErrNotFound
		}
		if v.DeliveryProposals[idx].Status != "submitted" {
			return ErrConflict
		}
		now := s.now()
		v.Version++
		v.UpdatedAt = now
		v.DeliveryProposals[idx].Status = "accepted"
		v.DeliveryProposals[idx].AcceptedBy = actor
		v.DeliveryProposals[idx].AcceptedAt = &now
		if err := s.writeOutcome(v); err != nil {
			return err
		}
		out = v
		return s.projectOutcome(&out)
	})
	return out, err
}

func (s *Store) SelectDeliveryProposals(outcomeID, steward string, expected int, proposalIDs, reviewerIDs []string, disclosure, rationale string) (FundedOutcome, error) {
	var out FundedOutcome
	err := s.lock(func() error {
		v, err := s.readOutcome(outcomeID)
		if err != nil {
			return err
		}
		fund, err := s.read(v.FundID)
		if err != nil {
			return err
		}
		if v.Version != expected || v.Status != "open" {
			return ErrConflict
		}
		if !contains(fund.Terms.Stewards, steward) {
			return ErrForbidden
		}
		if len(proposalIDs) == 0 || len(proposalIDs) > 20 || len(reviewerIDs) == 0 || len(reviewerIDs) > 20 || !validOutcomeTexts(reviewerIDs, 20, 300) || !validOutcomeText(disclosure, 5000) || !validOutcomeText(rationale, 5000) {
			return ErrInvalid
		}
		seen := map[string]bool{}
		total := int64(0)
		tasks := []DeliveryTask{}
		for _, id := range proposalIDs {
			if seen[id] {
				return ErrInvalid
			}
			seen[id] = true
			i := deliveryProposalIndex(v, id)
			if i < 0 || v.DeliveryProposals[i].Status != "accepted" {
				return ErrConflict
			}
			p := &v.DeliveryProposals[i]
			total += p.Terms.Cost
			if total <= 0 {
				return ErrInvalid
			}
			for mi, milestone := range p.Terms.Milestones {
				award := p.Terms.Cost / int64(len(p.Terms.Milestones))
				if mi == len(p.Terms.Milestones)-1 {
					award += p.Terms.Cost % int64(len(p.Terms.Milestones))
				}
				tasks = append(tasks, DeliveryTask{ID: randomID(), Title: milestone, RecipientKind: p.Applicant.Kind, RecipientID: p.Applicant.ID, MilestoneIndex: mi, Dependencies: append([]string(nil), p.Terms.Dependencies...), Status: "planned", ReviewerIDs: append([]string(nil), reviewerIDs...), AwardAmount: award, Reviews: []MilestoneReview{}, Recoveries: []MilestoneRecovery{}})
			}
		}
		originalFund := fund
		fund.Balances = derive(fund.Terms, fund.Ledger)
		if total > fund.Balances.Available {
			return ErrConflict
		}
		if !singleStewardApprovalAuthorized(fund.Terms, steward, total) {
			return ErrForbidden
		}
		now := s.now()
		selectionID, reservationID := randomID(), randomID()
		selection := DeliverySelection{ID: selectionID, ProposalIDs: append([]string(nil), proposalIDs...), Disclosure: strings.TrimSpace(disclosure), Rationale: strings.TrimSpace(rationale), ReservedAmount: total, ReservationID: reservationID, Tasks: tasks, SelectedBy: steward, SelectedAt: now, Execution: DeliveryExecution{Status: "active", Budget: total, Updates: []DeliveryUpdate{}, Expenses: []DeliveryExpense{}, Controls: []DeliveryControl{}, Blockers: []string{}, SpendingBlockers: []string{}}}
		for i := range v.DeliveryProposals {
			if seen[v.DeliveryProposals[i].ID] {
				v.DeliveryProposals[i].Status = "selected"
				v.DeliveryProposals[i].SelectionID = selectionID
			}
		}
		v.Version++
		v.UpdatedAt = now
		v.DeliverySelections = append(v.DeliverySelections, selection)
		fund.Version++
		fund.UpdatedAt = now
		fund.Ledger = append(fund.Ledger, Entry{ID: reservationID, Kind: "delivery_reservation", Amount: total, Status: "reserved", ExternalReference: outcomeID + ":" + selectionID, ActorID: steward, Note: strings.TrimSpace(rationale), CreatedAt: now})
		fund.Balances = derive(fund.Terms, fund.Ledger)
		// Both files are serialized under the fund lock. Write the reservation first:
		// an interrupted second write fails safe by retaining non-spendable value.
		if err := s.write(fund); err != nil {
			return err
		}
		var outcomeWriteErr error
		if s.afterDeliveryReservationWrite != nil {
			outcomeWriteErr = s.afterDeliveryReservationWrite()
		}
		if outcomeWriteErr == nil {
			outcomeWriteErr = s.writeOutcome(v)
		}
		if outcomeWriteErr != nil {
			if rollbackErr := s.write(originalFund); rollbackErr != nil {
				return errors.Join(outcomeWriteErr, rollbackErr)
			}
			return outcomeWriteErr
		}
		out = v
		return s.projectOutcome(&out)
	})
	return out, err
}

func singleStewardApprovalAuthorized(terms Terms, actor string, amount int64) bool {
	for _, limit := range terms.SpendingLimits {
		if amount > limit.Amount {
			return false
		}
	}
	matched := false
	for _, rule := range terms.ApprovalRules {
		if amount < rule.MinimumAmount {
			continue
		}
		matched = true
		if rule.RequiredApprovals != 1 || !contains(rule.EligibleApprovers, actor) {
			return false
		}
	}
	return matched
}

func deliveryProposalIndex(v FundedOutcome, id string) int {
	for i := range v.DeliveryProposals {
		if v.DeliveryProposals[i].ID == id {
			return i
		}
	}
	return -1
}
func validDeliveryApplicant(a DeliveryApplicant) bool {
	return contains([]string{"human", "team", "approved_agent"}, a.Kind) && validOutcomeText(a.ID, 300) && validOutcomeText(a.SubmittedBy, 300)
}
func validDeliveryTerms(t DeliveryProposalTerms) bool {
	return validOutcomeText(t.Approach, 10000) && validOutcomeTexts(t.Milestones, 100, 2000) && t.Cost > 0 && validOptionalOutcomeTexts(t.Dependencies, 100, 1000) && validOutcomeText(t.Availability, 2000) && validOptionalOutcomeTexts(t.RequiredAccess, 100, 1000) && len(t.RelevantWork) > 0 && len(t.RelevantWork) <= 100 && validAttributedWork(t.RelevantWork)
}
func validAttributedWork(v []AttributedWork) bool {
	for _, x := range v {
		if !contains([]string{"repository", "pull", "proposal", "issue", "release", "external"}, x.Kind) || !validOutcomeText(x.ID, 500) || len(x.URL) > 2000 || !validOutcomeText(x.Note, 2000) {
			return false
		}
	}
	return true
}
func normalizeDeliveryTerms(t DeliveryProposalTerms) DeliveryProposalTerms {
	t.Approach = strings.TrimSpace(t.Approach)
	t.Availability = strings.TrimSpace(t.Availability)
	return t
}

const deliveryAuthorityNote = "Selection reserves compensation only. It grants no repository, secret, credential, task-execution, review, merge, environment, deployment, fund-withdrawal, or recipient-acceptance authority; every operational scope remains separately approved."
