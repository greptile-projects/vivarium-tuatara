package projectfunds

import (
	"strings"
	"time"
)

// OutcomeMeasure is a recipient-declared result inspected by a reviewer. It is
// evidence, never a replacement for the authority of its linked project resource.
type OutcomeMeasure struct {
	Name     string           `json:"name"`
	Status   string           `json:"status"`
	Value    string           `json:"value"`
	Evidence DeliveryResource `json:"evidence"`
}

type MilestoneReview struct {
	ID              string             `json:"id"`
	Decision        string             `json:"decision"`
	ReviewerID      string             `json:"reviewer_id"`
	Rationale       string             `json:"rationale"`
	Dissent         []string           `json:"dissent"`
	AwardAmount     int64              `json:"award_amount"`
	ReleasedAmount  int64              `json:"released_amount"`
	PaymentStatus   string             `json:"payment_status"`
	UpdateID        string             `json:"update_id"`
	Authorship      []string           `json:"authorship"`
	Resources       []DeliveryResource `json:"resources"`
	Evidence        []DeliveryEvidence `json:"evidence"`
	OutcomeMeasures []OutcomeMeasure   `json:"outcome_measures"`
	CreatedAt       time.Time          `json:"created_at"`
}

type MilestoneRecovery struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	ActorID   string    `json:"actor_id"`
	Reason    string    `json:"reason"`
	Amount    int64     `json:"amount"`
	CreatedAt time.Time `json:"created_at"`
}

type MilestoneReviewInput struct {
	Decision        string           `json:"decision"`
	Rationale       string           `json:"rationale"`
	Dissent         []string         `json:"dissent"`
	AwardAmount     int64            `json:"award_amount"`
	OutcomeMeasures []OutcomeMeasure `json:"outcome_measures"`
}

func (s *Store) ReviewMilestone(outcomeID, selectionID, taskID, reviewer string, expected int, in MilestoneReviewInput) (FundedOutcome, error) {
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
		sel := deliverySelection(v, selectionID)
		task := deliveryTask(sel, taskID)
		if v.Version != expected || v.Status != "open" || sel == nil || task == nil || sel.Execution.Status == "cancelled" {
			return ErrConflict
		}
		if !contains(task.ReviewerIDs, reviewer) {
			return ErrForbidden
		}
		if !validOutcomeText(in.Rationale, 5000) || !validOptionalOutcomeTexts(in.Dissent, 50, 2000) || !contains([]string{"accepted", "rejected", "correction_requested", "partial_award", "disputed"}, in.Decision) {
			return ErrInvalid
		}
		update := latestTaskUpdate(sel.Execution, taskID)
		if update == nil {
			return ErrConflict
		}
		if lastAward(task) != nil || task.Status == "withdrawn" || task.Status == "timed_out" || task.Status == "refunded" {
			return ErrConflict
		}
		amount := int64(0)
		if in.Decision == "accepted" || in.Decision == "partial_award" {
			if update.Status != "completed" || len(update.Resources) == 0 || len(in.OutcomeMeasures) == 0 || !validOutcomeMeasures(in.OutcomeMeasures) {
				return ErrConflict
			}
			amount = task.AwardAmount
			if in.Decision == "accepted" && !allOutcomeMeasuresMet(in.OutcomeMeasures) {
				return ErrConflict
			}
			if in.Decision == "partial_award" {
				amount = in.AwardAmount
			}
			if amount <= 0 || amount > task.AwardAmount || amount > sel.Execution.Budget-sel.Execution.Spent {
				return ErrInvalid
			}
		}
		now := s.now()
		authors := []string{update.ActorID}
		review := MilestoneReview{ID: randomID(), Decision: in.Decision, ReviewerID: reviewer, Rationale: strings.TrimSpace(in.Rationale), Dissent: append([]string(nil), in.Dissent...), AwardAmount: amount, PaymentStatus: "not_applicable", UpdateID: update.ID, Authorship: authors, Resources: append([]DeliveryResource(nil), update.Resources...), Evidence: append([]DeliveryEvidence(nil), update.Evidence...), OutcomeMeasures: append([]OutcomeMeasure(nil), in.OutcomeMeasures...), CreatedAt: now}
		if amount > 0 {
			review.PaymentStatus = "paid"
			if in.Decision == "partial_award" {
				review.ReleasedAmount = task.AwardAmount - amount
			}
		}
		task.Reviews = append(task.Reviews, review)
		switch in.Decision {
		case "accepted":
			task.Status = "accepted"
		case "partial_award":
			task.Status = "partially_accepted"
		case "correction_requested":
			task.Status = "correction_requested"
		case "rejected":
			task.Status = "rejected"
		case "disputed":
			task.Status = "disputed"
		}
		v.Version++
		v.UpdatedAt = now
		if amount > 0 {
			fund.Version++
			fund.UpdatedAt = now
			entry := Entry{ID: randomID(), Kind: "milestone_award", Amount: amount, Status: "paid", ExternalReference: outcomeID + ":" + selectionID + ":" + taskID + ":" + review.ID, ContributorID: task.RecipientID, ActorID: reviewer, Note: review.Rationale, CreatedAt: now}
			fund.Ledger = append(fund.Ledger, entry)
			if review.ReleasedAmount > 0 {
				fund.Ledger = append(fund.Ledger, Entry{ID: randomID(), Kind: "delivery_reservation_release", Amount: review.ReleasedAmount, Status: "released", ExternalReference: entry.ExternalReference + ":remainder", ContributorID: task.RecipientID, ActorID: reviewer, Note: "Unawarded milestone remainder released after partial acceptance.", CreatedAt: now})
			}
			fund.Balances = derive(fund.Terms, fund.Ledger)
			if fund.Balances.Reserved < 0 {
				return ErrConflict
			}
			tx := deliveryTransaction{ID: entry.ID, Fund: fund, Outcome: v}
			if err = s.writeDeliveryTransaction(tx); err != nil {
				return err
			}
			if err = s.write(fund); err != nil {
				return err
			}
			if err = s.writeOutcome(v); err != nil {
				return err
			}
			if err = s.removeDeliveryTransactionsForExpense(entry.ID); err != nil {
				return err
			}
		} else if err = s.writeOutcome(v); err != nil {
			return err
		}
		out = v
		return s.projectOutcome(&out)
	})
	return out, err
}

func (s *Store) RecoverMilestone(outcomeID, selectionID, taskID, actor, action, reason string, expected int) (FundedOutcome, error) {
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
		sel := deliverySelection(v, selectionID)
		task := deliveryTask(sel, taskID)
		if v.Version != expected || sel == nil || task == nil || !validOutcomeText(reason, 5000) {
			return ErrConflict
		}
		isRecipient := actor == task.RecipientID
		isReviewer := contains(task.ReviewerIDs, actor)
		isSteward := contains(fund.Terms.Stewards, actor)
		amount, kind, status := int64(0), "", ""
		now := s.now()
		switch action {
		case "appeal":
			if !isRecipient || (task.Status != "correction_requested" && task.Status != "rejected" && task.Status != "disputed") {
				return ErrForbidden
			}
			task.Status = "appealed"
		case "withdraw":
			if !isRecipient || !taskReleaseEligible(task) {
				return ErrForbidden
			}
			amount = releasableTaskAmount(task)
			if amount <= 0 {
				return ErrConflict
			}
			kind, status, task.Status = "milestone_withdrawal", "released", "withdrawn"
		case "timeout":
			if !isReviewer || now.Before(v.Revisions[len(v.Revisions)-1].Terms.Deadline) || !taskReleaseEligible(task) {
				return ErrForbidden
			}
			amount = releasableTaskAmount(task)
			if amount <= 0 {
				return ErrConflict
			}
			kind, status, task.Status = "milestone_timeout", "released", "timed_out"
		case "payment_failed":
			if !isSteward || !taskPaid(task) {
				return ErrForbidden
			}
			amount, kind, status, task.Status = lastAward(task).AwardAmount, "milestone_payment_failure", "failed", "payment_failed"
			lastAward(task).PaymentStatus = "failed"
		case "retry_payment":
			if !isSteward || task.Status != "payment_failed" {
				return ErrForbidden
			}
			amount, kind, status, task.Status = lastAward(task).AwardAmount, "milestone_award", "paid", "accepted"
			lastAward(task).PaymentStatus = "paid"
		case "refund":
			if !isSteward || !taskPaid(task) {
				return ErrForbidden
			}
			amount, kind, status, task.Status = lastAward(task).AwardAmount, "milestone_refund", "refunded", "refunded"
			lastAward(task).PaymentStatus = "refunded"
		default:
			return ErrInvalid
		}
		task.Recoveries = append(task.Recoveries, MilestoneRecovery{ID: randomID(), Kind: action, ActorID: actor, Reason: strings.TrimSpace(reason), Amount: amount, CreatedAt: now})
		v.Version++
		v.UpdatedAt = now
		if amount > 0 {
			fund.Version++
			fund.UpdatedAt = now
			entry := Entry{ID: randomID(), Kind: kind, Amount: amount, Status: status, ExternalReference: outcomeID + ":" + selectionID + ":" + taskID + ":" + action, ContributorID: task.RecipientID, ActorID: actor, Note: reason, CreatedAt: now}
			fund.Ledger = append(fund.Ledger, entry)
			fund.Balances = derive(fund.Terms, fund.Ledger)
			if fund.Balances.Reserved < 0 || fund.Balances.Spent < 0 {
				return ErrConflict
			}
			tx := deliveryTransaction{ID: entry.ID, Fund: fund, Outcome: v}
			if err = s.writeDeliveryTransaction(tx); err != nil {
				return err
			}
			if err = s.write(fund); err != nil {
				return err
			}
			if err = s.writeOutcome(v); err != nil {
				return err
			}
			if err = s.removeDeliveryTransactionsForExpense(entry.ID); err != nil {
				return err
			}
		} else if err = s.writeOutcome(v); err != nil {
			return err
		}
		out = v
		return s.projectOutcome(&out)
	})
	return out, err
}

func deliveryTask(sel *DeliverySelection, id string) *DeliveryTask {
	if sel == nil {
		return nil
	}
	for i := range sel.Tasks {
		if sel.Tasks[i].ID == id {
			return &sel.Tasks[i]
		}
	}
	return nil
}
func latestTaskUpdate(e DeliveryExecution, id string) *DeliveryUpdate {
	var out *DeliveryUpdate
	for i := range e.Updates {
		if e.Updates[i].TaskID == id && (out == nil || e.Updates[i].CreatedAt.After(out.CreatedAt)) {
			out = &e.Updates[i]
		}
	}
	return out
}
func validOutcomeMeasures(values []OutcomeMeasure) bool {
	if len(values) > 50 {
		return false
	}
	for _, v := range values {
		if !validOutcomeText(v.Name, 500) || !contains([]string{"met", "not_met", "inconclusive"}, v.Status) || !validOutcomeText(v.Value, 2000) || !validDeliveryResource(v.Evidence) {
			return false
		}
	}
	return true
}
func allOutcomeMeasuresMet(values []OutcomeMeasure) bool {
	for _, value := range values {
		if value.Status != "met" {
			return false
		}
	}
	return true
}
func lastAward(t *DeliveryTask) *MilestoneReview {
	for i := len(t.Reviews) - 1; i >= 0; i-- {
		if t.Reviews[i].AwardAmount > 0 {
			return &t.Reviews[i]
		}
	}
	return nil
}
func taskPaid(t *DeliveryTask) bool { a := lastAward(t); return a != nil && a.PaymentStatus == "paid" }
func releasableTaskAmount(t *DeliveryTask) int64 {
	if award := lastAward(t); award != nil && award.PaymentStatus == "failed" {
		return award.AwardAmount
	}
	return t.AwardAmount
}
func taskReleaseEligible(t *DeliveryTask) bool {
	if t.Status == "withdrawn" || t.Status == "timed_out" || t.Status == "refunded" {
		return false
	}
	award := lastAward(t)
	return award == nil || award.PaymentStatus == "failed"
}
