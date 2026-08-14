package projectfunds

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type DeliveryResource struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Revision string `json:"revision,omitempty"`
	URL      string `json:"url,omitempty"`
	Status   string `json:"status"`
}
type DeliveryEvidence struct {
	Kind     string           `json:"kind"`
	Summary  string           `json:"summary"`
	Resource DeliveryResource `json:"resource"`
}
type DeliveryUpdate struct {
	ID            string             `json:"id"`
	TaskID        string             `json:"task_id"`
	RecipientKind string             `json:"recipient_kind"`
	RecipientID   string             `json:"recipient_id"`
	ActorID       string             `json:"actor_id"`
	Status        string             `json:"status"`
	Progress      int                `json:"progress"`
	Summary       string             `json:"summary"`
	Blockers      []string           `json:"blockers"`
	Resources     []DeliveryResource `json:"resources"`
	Evidence      []DeliveryEvidence `json:"evidence"`
	ForecastAt    *time.Time         `json:"forecast_at,omitempty"`
	AgentMinutes  int64              `json:"agent_minutes"`
	CreatedAt     time.Time          `json:"created_at"`
}
type DeliveryExpense struct {
	ID             string             `json:"id"`
	TaskID         string             `json:"task_id"`
	RecipientKind  string             `json:"recipient_kind"`
	RecipientID    string             `json:"recipient_id"`
	Amount         int64              `json:"amount"`
	Category       string             `json:"category"`
	Description    string             `json:"description"`
	Evidence       []DeliveryResource `json:"evidence"`
	Status         string             `json:"status"`
	RequestedBy    string             `json:"requested_by"`
	DecidedBy      string             `json:"decided_by,omitempty"`
	DecisionReason string             `json:"decision_reason,omitempty"`
	CreatedAt      time.Time          `json:"created_at"`
	DecidedAt      *time.Time         `json:"decided_at,omitempty"`
}
type DeliveryControl struct {
	Kind          string              `json:"kind"`
	ActorID       string              `json:"actor_id"`
	Reason        string              `json:"reason"`
	RecipientKind string              `json:"recipient_kind,omitempty"`
	RecipientID   string              `json:"recipient_id,omitempty"`
	Amount        int64               `json:"amount,omitempty"`
	Recipients    []DeliveryPrincipal `json:"recipients,omitempty"`
	CreatedAt     time.Time           `json:"created_at"`
}
type DeliveryPrincipal struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}
type DeliveryExecution struct {
	Status           string            `json:"status"`
	Budget           int64             `json:"budget"`
	Spent            int64             `json:"spent"`
	Released         int64             `json:"released"`
	PendingExpenses  int64             `json:"pending_expenses"`
	AgentMinutes     int64             `json:"agent_minutes"`
	Progress         int               `json:"progress"`
	ForecastAt       *time.Time        `json:"forecast_at,omitempty"`
	LastActivityAt   *time.Time        `json:"last_activity_at,omitempty"`
	Updates          []DeliveryUpdate  `json:"updates"`
	Expenses         []DeliveryExpense `json:"expenses"`
	Controls         []DeliveryControl `json:"controls"`
	Blockers         []string          `json:"blockers"`
	SpendingBlocked  bool              `json:"spending_blocked"`
	SpendingBlockers []string          `json:"spending_blockers"`
}

type DeliveryUpdateInput struct {
	TaskID       string             `json:"task_id"`
	Status       string             `json:"status"`
	Progress     int                `json:"progress"`
	Summary      string             `json:"summary"`
	Blockers     []string           `json:"blockers"`
	Resources    []DeliveryResource `json:"resources"`
	Evidence     []DeliveryEvidence `json:"evidence"`
	ForecastAt   *time.Time         `json:"forecast_at"`
	AgentMinutes int64              `json:"agent_minutes"`
}
type DeliveryExpenseInput struct {
	TaskID      string             `json:"task_id"`
	Amount      int64              `json:"amount"`
	Category    string             `json:"category"`
	Description string             `json:"description"`
	Evidence    []DeliveryResource `json:"evidence"`
}

func (s *Store) RecordDeliveryUpdate(outcomeID, selectionID string, recipient DeliveryApplicant, actor string, expected int, in DeliveryUpdateInput) (FundedOutcome, error) {
	var out FundedOutcome
	err := s.lock(func() error {
		v, err := s.readOutcome(outcomeID)
		if err != nil {
			return err
		}
		sel := deliverySelection(v, selectionID)
		if v.Version != expected || v.Status != "open" || sel == nil || sel.Execution.Status == "cancelled" {
			return ErrConflict
		}
		if !selectionHasRecipient(*sel, recipient) || !validDeliveryUpdate(*sel, recipient, in) {
			return ErrInvalid
		}
		if deliveryRecipientRevoked(sel.Execution, recipient) {
			return ErrForbidden
		}
		now := s.now()
		v.Version++
		v.UpdatedAt = now
		sel.Execution.Updates = append(sel.Execution.Updates, DeliveryUpdate{ID: randomID(), TaskID: in.TaskID, RecipientKind: recipient.Kind, RecipientID: recipient.ID, ActorID: actor, Status: in.Status, Progress: in.Progress, Summary: strings.TrimSpace(in.Summary), Blockers: in.Blockers, Resources: in.Resources, Evidence: in.Evidence, ForecastAt: in.ForecastAt, AgentMinutes: in.AgentMinutes, CreatedAt: now})
		for i := range sel.Tasks {
			if sel.Tasks[i].ID == in.TaskID {
				sel.Tasks[i].Status = in.Status
			}
		}
		projectDeliveryExecution(sel, now)
		if err := s.writeOutcome(v); err != nil {
			return err
		}
		out = v
		return s.projectOutcome(&out)
	})
	return out, err
}

func (s *Store) RequestDeliveryExpense(outcomeID, selectionID string, recipient DeliveryApplicant, actor string, expected int, in DeliveryExpenseInput) (FundedOutcome, error) {
	var out FundedOutcome
	err := s.lock(func() error {
		v, err := s.readOutcome(outcomeID)
		if err != nil {
			return err
		}
		sel := deliverySelection(v, selectionID)
		if v.Version != expected || v.Status != "open" || sel == nil {
			return ErrConflict
		}
		projectDeliveryExecution(sel, s.now())
		if sel.Execution.SpendingBlocked {
			return ErrConflict
		}
		if !selectionHasRecipient(*sel, recipient) || !validDeliveryExpense(*sel, recipient, in) {
			return ErrInvalid
		}
		if deliveryRecipientRevoked(sel.Execution, recipient) {
			return ErrForbidden
		}
		now := s.now()
		v.Version++
		v.UpdatedAt = now
		sel.Execution.Expenses = append(sel.Execution.Expenses, DeliveryExpense{ID: randomID(), TaskID: in.TaskID, RecipientKind: recipient.Kind, RecipientID: recipient.ID, Amount: in.Amount, Category: in.Category, Description: strings.TrimSpace(in.Description), Evidence: in.Evidence, Status: "pending", RequestedBy: actor, CreatedAt: now})
		projectDeliveryExecution(sel, now)
		if err := s.writeOutcome(v); err != nil {
			return err
		}
		out = v
		return s.projectOutcome(&out)
	})
	return out, err
}

func (s *Store) DecideDeliveryExpense(outcomeID, selectionID, expenseID, steward, decision, reason string, expected int) (FundedOutcome, error) {
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
		if v.Version != expected || v.Status != "open" || sel == nil {
			return ErrConflict
		}
		if !contains(fund.Terms.Stewards, steward) {
			return ErrForbidden
		}
		idx := -1
		for i := range sel.Execution.Expenses {
			if sel.Execution.Expenses[i].ID == expenseID {
				idx = i
			}
		}
		if idx < 0 {
			return ErrNotFound
		}
		exp := &sel.Execution.Expenses[idx]
		if exp.Status != "pending" || !contains([]string{"approved", "rejected"}, decision) || !validOutcomeText(reason, 5000) {
			return ErrInvalid
		}
		projectDeliveryExecution(sel, s.now())
		if decision == "approved" && (sel.Execution.SpendingBlocked || exp.Amount > sel.Execution.Budget-sel.Execution.Spent) {
			return ErrConflict
		}
		now := s.now()
		exp.Status = decision
		exp.DecidedBy = steward
		exp.DecisionReason = strings.TrimSpace(reason)
		exp.DecidedAt = &now
		v.Version++
		v.UpdatedAt = now
		projectDeliveryExecution(sel, now)
		if decision == "approved" {
			fund.Version++
			fund.UpdatedAt = now
			fund.Ledger = append(fund.Ledger, Entry{ID: randomID(), Kind: "delivery_expense", Amount: exp.Amount, Status: "approved", ExternalReference: outcomeID + ":" + selectionID + ":" + expenseID, ContributorID: exp.RecipientID, ActorID: steward, Note: exp.Description, CreatedAt: now})
			fund.Balances = derive(fund.Terms, fund.Ledger)
			if fund.Balances.Reserved < 0 {
				return ErrConflict
			}
			tx := deliveryTransaction{ID: fund.Ledger[len(fund.Ledger)-1].ID, Fund: fund, Outcome: v}
			if err = s.writeDeliveryTransaction(tx); err != nil {
				return err
			}
			if err = s.write(fund); err != nil {
				return err
			}
			if s.afterDeliveryExpenseFundWrite != nil {
				if err = s.afterDeliveryExpenseFundWrite(); err != nil {
					return err
				}
			}
		}
		if s.afterDeliveryExpenseOutcomeWrite != nil && decision == "approved" {
			if err = s.afterDeliveryExpenseOutcomeWrite(); err != nil {
				return err
			}
		}
		if err = s.writeOutcome(v); err != nil {
			return err
		}
		if decision == "approved" {
			if err = s.removeDeliveryTransactionsForExpense(fund.Ledger[len(fund.Ledger)-1].ID); err != nil {
				return err
			}
		}
		out = v
		return s.projectOutcome(&out)
	})
	return out, err
}

func (s *Store) ControlDelivery(outcomeID, selectionID, steward, action, reason string, amount int64, replacement *DeliveryApplicant, expected int) (FundedOutcome, error) {
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
		original := fund
		sel := deliverySelection(v, selectionID)
		if v.Version != expected || v.Status != "open" || sel == nil || !validOutcomeText(reason, 5000) {
			return ErrConflict
		}
		if !contains(fund.Terms.Stewards, steward) {
			return ErrForbidden
		}
		now := s.now()
		projectDeliveryExecution(sel, now)
		ledgerAmount := int64(0)
		ledgerKind := ""
		controlRecipients := []DeliveryPrincipal{}
		switch action {
		case "pause":
			if sel.Execution.Status == "cancelled" {
				return ErrConflict
			}
			sel.Execution.Status = "paused"
		case "access_revoked":
			if sel.Execution.Status == "cancelled" {
				return ErrConflict
			}
			sel.Execution.Status = "paused"
			revoked := map[string]bool{}
			for _, task := range sel.Tasks {
				revoked[task.RecipientKind+"\x00"+task.RecipientID] = true
			}
			for key := range revoked {
				parts := strings.SplitN(key, "\x00", 2)
				controlRecipients = append(controlRecipients, DeliveryPrincipal{Kind: parts[0], ID: parts[1]})
			}
		case "resume":
			if sel.Execution.Status != "paused" {
				return ErrConflict
			}
			sel.Execution.Status = "active"
		case "cancel_remaining":
			if sel.Execution.Status == "cancelled" {
				return ErrConflict
			}
			ledgerAmount = sel.Execution.Budget - sel.Execution.Spent - sel.Execution.Released
			if ledgerAmount < 0 {
				return ErrConflict
			}
			ledgerKind = "delivery_reservation_release"
			sel.Execution.Status = "cancelled"
		case "increase_budget":
			if amount <= 0 || !singleStewardApprovalAuthorized(fund.Terms, steward, amount) {
				return ErrForbidden
			}
			fund.Balances = derive(fund.Terms, fund.Ledger)
			if amount > fund.Balances.Available {
				return ErrConflict
			}
			ledgerAmount = amount
			ledgerKind = "delivery_budget_increase"
			sel.Execution.Budget += amount
		case "replace_recipient":
			if replacement == nil || !validDeliveryApplicant(*replacement) {
				return ErrInvalid
			}
			oldKind, oldID := "", ""
			for i := range sel.Tasks {
				if !deliveryTaskTerminal(sel.Tasks[i]) {
					if oldID == "" {
						oldKind, oldID = sel.Tasks[i].RecipientKind, sel.Tasks[i].RecipientID
					}
					if sel.Tasks[i].RecipientKind != oldKind || sel.Tasks[i].RecipientID != oldID {
						return ErrConflict
					}
					sel.Tasks[i].RecipientKind = replacement.Kind
					sel.Tasks[i].RecipientID = replacement.ID
				}
			}
			if oldID == "" {
				return ErrConflict
			}
			if replacement.Kind == oldKind && replacement.ID == oldID {
				return ErrConflict
			}
			for i := range v.DeliveryProposals {
				if v.DeliveryProposals[i].Applicant.Kind == oldKind && v.DeliveryProposals[i].Applicant.ID == oldID {
					v.DeliveryProposals[i].Status = "replaced"
				}
			}
		default:
			return ErrInvalid
		}
		v.Version++
		v.UpdatedAt = now
		sel.Execution.Controls = append(sel.Execution.Controls, DeliveryControl{Kind: action, ActorID: steward, Reason: strings.TrimSpace(reason), Recipients: controlRecipients, RecipientKind: func() string {
			if replacement != nil {
				return replacement.Kind
			}
			return ""
		}(), RecipientID: func() string {
			if replacement != nil {
				return replacement.ID
			}
			return ""
		}(), Amount: amount, CreatedAt: now})
		if ledgerKind != "" && ledgerAmount > 0 {
			status := "reserved"
			if ledgerKind == "delivery_reservation_release" {
				status = "released"
			}
			fund.Version++
			fund.UpdatedAt = now
			fund.Ledger = append(fund.Ledger, Entry{ID: randomID(), Kind: ledgerKind, Amount: ledgerAmount, Status: status, ExternalReference: outcomeID + ":" + selectionID, ActorID: steward, Note: reason, CreatedAt: now})
			fund.Balances = derive(fund.Terms, fund.Ledger)
			if fund.Balances.Reserved < 0 {
				return ErrConflict
			}
			if err = s.write(fund); err != nil {
				return err
			}
		}
		projectDeliveryExecution(sel, now)
		if err = s.writeOutcome(v); err != nil {
			if ledgerKind != "" {
				_ = s.write(original)
			}
			return err
		}
		out = v
		return s.projectOutcome(&out)
	})
	return out, err
}

func deliverySelection(v FundedOutcome, id string) *DeliverySelection {
	for i := range v.DeliverySelections {
		if v.DeliverySelections[i].ID == id {
			return &v.DeliverySelections[i]
		}
	}
	return nil
}
func selectionHasRecipient(s DeliverySelection, a DeliveryApplicant) bool {
	for _, t := range s.Tasks {
		if t.RecipientKind == a.Kind && t.RecipientID == a.ID {
			return true
		}
	}
	return false
}
func selectionTask(s DeliverySelection, id string, a DeliveryApplicant) bool {
	for _, t := range s.Tasks {
		if t.ID == id && t.RecipientKind == a.Kind && t.RecipientID == a.ID {
			return true
		}
	}
	return false
}
func deliveryTaskTerminal(task DeliveryTask) bool {
	return contains([]string{"completed", "accepted", "partially_accepted", "withdrawn", "timed_out", "refunded"}, task.Status)
}
func validDeliveryResource(r DeliveryResource) bool {
	return contains([]string{"task", "session", "workspace", "fork", "pull", "check", "preview", "delivery_team", "commit", "release", "deployment"}, r.Kind) && validOutcomeText(r.ID, 500) && len(r.Revision) <= 500 && len(r.URL) <= 2000 && validOutcomeText(r.Status, 100)
}
func validDeliveryUpdate(s DeliverySelection, a DeliveryApplicant, in DeliveryUpdateInput) bool {
	if !selectionTask(s, in.TaskID, a) || !contains([]string{"active", "blocked", "completed", "handoff_failed"}, in.Status) || in.Progress < 0 || in.Progress > 100 || !validOutcomeText(in.Summary, 5000) || in.AgentMinutes < 0 || len(in.Blockers) > 100 || len(in.Resources) > 100 || len(in.Evidence) > 100 {
		return false
	}
	for _, r := range in.Resources {
		if !validDeliveryResource(r) {
			return false
		}
	}
	for _, e := range in.Evidence {
		if !contains([]string{"work", "check", "preview", "acceptance", "expense", "handoff"}, e.Kind) || !validOutcomeText(e.Summary, 5000) || !validDeliveryResource(e.Resource) {
			return false
		}
	}
	return validOptionalOutcomeTexts(in.Blockers, 100, 1000)
}
func validDeliveryExpense(s DeliverySelection, a DeliveryApplicant, in DeliveryExpenseInput) bool {
	if !selectionTask(s, in.TaskID, a) || in.Amount <= 0 || !contains([]string{"labor", "agent_compute", "service", "materials", "other"}, in.Category) || !validOutcomeText(in.Description, 5000) || len(in.Evidence) == 0 || len(in.Evidence) > 20 {
		return false
	}
	for _, r := range in.Evidence {
		if !validDeliveryResource(r) {
			return false
		}
	}
	return true
}
func projectDeliveryExecution(s *DeliverySelection, now time.Time) {
	e := &s.Execution
	if e.Status == "" {
		e.Status = "active"
	}
	if e.Budget == 0 {
		e.Budget = s.ReservedAmount
	}
	e.Spent = 0
	e.Released = 0
	e.PendingExpenses = 0
	e.AgentMinutes = 0
	e.Progress = 0
	e.Blockers = nil
	e.SpendingBlockers = nil
	latestByTask := map[string]*DeliveryUpdate{}
	var latest *time.Time
	for i := range e.Updates {
		u := &e.Updates[i]
		e.AgentMinutes += u.AgentMinutes
		if prior := latestByTask[u.TaskID]; prior == nil || u.CreatedAt.After(prior.CreatedAt) {
			latestByTask[u.TaskID] = u
		}
		if latest == nil || u.CreatedAt.After(*latest) {
			x := u.CreatedAt
			latest = &x
			e.ForecastAt = u.ForecastAt
		}
	}
	for _, u := range latestByTask {
		e.Progress += u.Progress
		e.Blockers = append(e.Blockers, u.Blockers...)
		if u.Status == "handoff_failed" {
			e.SpendingBlockers = append(e.SpendingBlockers, "failed_handoff")
		}
	}
	if len(s.Tasks) > 0 {
		e.Progress /= len(s.Tasks)
	}
	activityBaseline := s.SelectedAt
	if latest != nil {
		activityBaseline = *latest
	}
	e.LastActivityAt = &activityBaseline
	for _, x := range e.Expenses {
		if x.Status == "approved" {
			e.Spent += x.Amount
		}
		if x.Status == "pending" {
			e.PendingExpenses += x.Amount
		}
	}
	for i := range s.Tasks {
		task := &s.Tasks[i]
		for j := range task.Reviews {
			award := &task.Reviews[j]
			e.Released += award.ReleasedAmount
			if award.PaymentStatus == "paid" {
				e.Spent += award.AwardAmount
			}
			if award.PaymentStatus == "refunded" {
				e.Released += award.AwardAmount
			}
		}
		for _, recovery := range task.Recoveries {
			if recovery.Kind == "withdraw" || recovery.Kind == "timeout" {
				e.Released += recovery.Amount
			}
		}
	}
	if e.Spent+e.PendingExpenses > e.Budget {
		e.SpendingBlockers = append(e.SpendingBlockers, "budget_overrun")
	}
	if now.Sub(activityBaseline) > 14*24*time.Hour {
		e.SpendingBlockers = append(e.SpendingBlockers, "inactivity")
	}
	if e.Status == "paused" || e.Status == "cancelled" {
		e.SpendingBlockers = append(e.SpendingBlockers, e.Status)
	}
	revokedRecipients := map[string]bool{}
	legacyAccessRevoked := false
	for _, control := range e.Controls {
		if control.Kind == "access_revoked" {
			if len(control.Recipients) == 0 {
				legacyAccessRevoked = true
			}
			for _, recipient := range control.Recipients {
				revokedRecipients[recipient.Kind+"\x00"+recipient.ID] = true
			}
		}
		if control.Kind == "recipient_access_revoked" {
			for _, recipient := range control.Recipients {
				revokedRecipients[recipient.Kind+"\x00"+recipient.ID] = true
			}
		}
	}
	activeRevoked := legacyAccessRevoked
	if !activeRevoked {
		for _, task := range s.Tasks {
			if task.Status != "completed" && revokedRecipients[task.RecipientKind+"\x00"+task.RecipientID] {
				activeRevoked = true
			}
		}
	}
	if activeRevoked {
		e.SpendingBlockers = append(e.SpendingBlockers, "revoked_access")
	}
	e.SpendingBlocked = len(e.SpendingBlockers) > 0
}
func deliveryRecipientRevoked(e DeliveryExecution, recipient DeliveryApplicant) bool {
	key := recipient.Kind + "\x00" + recipient.ID
	for _, control := range e.Controls {
		if control.Kind == "access_revoked" && len(control.Recipients) == 0 {
			return true
		}
		if control.Kind == "access_revoked" || control.Kind == "recipient_access_revoked" {
			for _, revoked := range control.Recipients {
				if revoked.Kind+"\x00"+revoked.ID == key {
					return true
				}
			}
		}
	}
	return false
}

type deliveryTransaction struct {
	ID      string        `json:"id"`
	Fund    Fund          `json:"fund"`
	Outcome FundedOutcome `json:"outcome"`
}

func (s *Store) deliveryTransactionRoot() string {
	return filepath.Join(s.root, "delivery-transactions")
}
func (s *Store) writeDeliveryTransaction(tx deliveryTransaction) error {
	if err := os.MkdirAll(s.deliveryTransactionRoot(), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(tx, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.deliveryTransactionRoot(), "transaction-*.tmp")
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
		err = os.Rename(name, filepath.Join(s.deliveryTransactionRoot(), tx.ID+".json"))
	}
	return err
}
func (s *Store) recoverDeliveryTransactions() error {
	entries, err := os.ReadDir(s.deliveryTransactionRoot())
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
		b, err := os.ReadFile(filepath.Join(s.deliveryTransactionRoot(), entry.Name()))
		if err != nil {
			return err
		}
		var tx deliveryTransaction
		if err = json.Unmarshal(b, &tx); err != nil {
			return err
		}
		if err = s.write(tx.Fund); err != nil {
			return err
		}
		if err = s.writeOutcome(tx.Outcome); err != nil {
			return err
		}
		if err = os.Remove(filepath.Join(s.deliveryTransactionRoot(), entry.Name())); err != nil {
			return err
		}
	}
	return nil
}
func (s *Store) removeDeliveryTransactionsForExpense(id string) error {
	return os.Remove(filepath.Join(s.deliveryTransactionRoot(), id+".json"))
}
