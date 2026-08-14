package projectfunds

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"testing"
	"time"
)

func outcomeTerms() OutcomeTerms {
	return OutcomeTerms{
		Title: "Fund the latency repair", Source: OutcomeSource{Kind: "issue", ID: "issue-1", Revision: "3", Visibility: "public"},
		Scope: "Reduce p95 latency without changing response semantics.", AcceptanceCriteria: []string{"p95 below 200ms", "all correctness checks pass"}, EvidenceRequirements: []string{"exact-revision benchmark", "merged pull and production observation"},
		Budget: 10000, Deadline: time.Now().UTC().Add(7 * 24 * time.Hour), ContributorEligibility: []string{"repository_participant", "approved_agent_with_live_grant"}, AllocationMethod: "milestone_claim", CancellationTerms: "Return backing for unaccepted milestones; retain attributable evidence.",
		Dependencies: []string{"benchmark environment available"}, Risks: []string{"noisy production traffic"}, Conflicts: []string{"open optimization pull"},
		Milestones: []Milestone{{ID: "diagnose", Title: "Reproduce and diagnose", Budget: 3000, AcceptanceCriteria: []string{"owner confirms diagnosis"}, EvidenceRequirements: []string{"sanitized flame trace"}}, {ID: "deliver", Title: "Deliver verified repair", Budget: 7000, AcceptanceCriteria: []string{"production target met"}, EvidenceRequirements: []string{"release observation"}, Dependencies: []string{"diagnose"}}},
	}
}

func outcomeStore(t *testing.T) (*Store, Fund) {
	t.Helper()
	s, err := New(t.TempDir(), map[string]string{"card": base64Key(testPublicKey)})
	if err != nil {
		t.Fatal(err)
	}
	f, err := s.Create("repo", "owner", Terms{Name: "Fund", Purpose: "Work", Stewards: []string{"owner"}, FundingSources: []string{"card"}, Unit: "USD", Precision: 2, SpendingLimits: []Limit{{Period: "monthly", Amount: 100000}}, ApprovalRules: []ApprovalRule{{RequiredApprovals: 1, EligibleApprovers: []string{"owner"}}}, EligibleRecipients: []string{"contributors"}, RefundPolicy: "Return unallocated funds", LedgerVisibility: "public"})
	if err != nil {
		t.Fatal(err)
	}
	return s, f
}

func base64Key(key []byte) string { return base64.StdEncoding.EncodeToString(key) }

func TestOutcomeFundingLifecycleMakesAcceptanceAndReplanningExplicit(t *testing.T) {
	s, fund := outcomeStore(t)
	terms := outcomeTerms()
	out, err := s.CreateOutcome("repo", fund.ID, "owner", terms)
	if err != nil {
		t.Fatal(err)
	}
	if out.Diagnostics[0].Kind != "insufficient_funds" || out.Revisions[0].Terms.EvidenceRequirements[0] == "" {
		t.Fatalf("initial projection = %+v", out)
	}
	out, err = s.PledgeOutcome(out.ID, "backer", "diagnose", 3000, "pledge-1", "Fund diagnosis", out.Version)
	if err != nil {
		t.Fatal(err)
	}
	out, err = s.PledgeOutcome(out.ID, "backer", "deliver", 7000, "pledge-2", "Fund delivery", out.Version)
	if err != nil {
		t.Fatal(err)
	}
	if out.Pledged != terms.Budget || out.MilestonePledged["deliver"] != 7000 {
		t.Fatalf("pledges = %+v", out)
	}
	terms.Scope = "Reduce p95 and document the fallback."
	out, err = s.ReviseOutcome(out.ID, "owner", out.Version, terms, "Production risk requires a documented fallback.")
	if err != nil {
		t.Fatal(err)
	}
	if out.Pledged != 0 || out.Pledges[0].Status != "reconfirmation_required" || out.Diagnostics[0].Kind != "insufficient_funds" {
		t.Fatalf("changed scope = %+v", out)
	}
	out, err = s.ChangePledge(out.ID, out.Pledges[0].ID, "backer", "reconfirm", "Still worth pursuing under the revised evidence contract.", out.Version)
	if err != nil {
		t.Fatal(err)
	}
	out, err = s.ChangePledge(out.ID, out.Pledges[1].ID, "backer", "withdraw", "Deadline no longer fits.", out.Version)
	if err != nil {
		t.Fatal(err)
	}
	if out.Pledged != 3000 || out.Replans[len(out.Replans)-1].Kind != "withdrawn_backing" {
		t.Fatalf("withdrawal = %+v", out)
	}
}

func TestOutcomeFundingRejectsAmbiguousMilestonesAndStaleWrites(t *testing.T) {
	s, fund := outcomeStore(t)
	terms := outcomeTerms()
	terms.Milestones[0].Budget++
	if _, err := s.CreateOutcome("repo", fund.ID, "owner", terms); !errors.Is(err, ErrInvalid) {
		t.Fatalf("milestone budget err = %v", err)
	}
	terms = outcomeTerms()
	out, err := s.CreateOutcome("repo", fund.ID, "owner", terms)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.PledgeOutcome(out.ID, "backer", "unknown", 10, "key", "", out.Version); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown milestone err = %v", err)
	}
	if _, err := s.PledgeOutcome(out.ID, "backer", "", 10, "key", "", out.Version+1); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale pledge err = %v", err)
	}
}

func TestDeliverySelectionRequiresRecipientAcceptanceAndReservesOnlyCompensation(t *testing.T) {
	s, fund := outcomeStore(t)
	fund, err := s.Commit(fund.ID, "backer", "card", "delivery-backing", 10000, "delivery-backing-key", "commission")
	if err != nil {
		t.Fatal(err)
	}
	p := proof("card", "delivery-backing", "settled", 10000)
	p.Nonce = "delivery-selection-proof"
	p.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(testPrivateKey, transferProofMessage(*p)))
	fund, err = s.Reconcile(fund.ID, fund.Ledger[0].ID, "owner", "settled", 10000, p, "verified", fund.Version)
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.CreateOutcome("repo", fund.ID, "owner", outcomeTerms())
	if err != nil {
		t.Fatal(err)
	}
	out, err = s.SubmitDeliveryProposal(out.ID, DeliveryApplicant{Kind: "human", ID: "contributor", SubmittedBy: "contributor"}, DeliveryProposalTerms{Approach: "Profile, repair, and verify the critical path.", Milestones: []string{"Reproduce latency", "Deliver verified repair"}, Cost: 9000, Dependencies: []string{"benchmark environment"}, Availability: "Available for the next two weeks.", RequiredAccess: []string{"repository read", "task branch write"}, RelevantWork: []AttributedWork{{Kind: "pull", ID: "prior-optimization", Note: "Previously reduced allocation pressure."}}})
	if err != nil {
		t.Fatal(err)
	}
	proposalID := out.DeliveryProposals[0].ID
	if _, err = s.SelectDeliveryProposals(out.ID, "owner", out.Version, []string{proposalID}, []string{"owner"}, "No conflicts known.", "Best evidence and availability."); !errors.Is(err, ErrConflict) {
		t.Fatalf("unaccepted selection err = %v", err)
	}
	out, err = s.AcceptDeliveryProposal(out.ID, proposalID, "contributor", out.Version)
	if err != nil {
		t.Fatal(err)
	}
	out, err = s.SelectDeliveryProposals(out.ID, "owner", out.Version, []string{proposalID}, []string{"owner"}, "No financial or review conflicts known.", "Approach covers both milestones within budget.")
	if err != nil {
		t.Fatal(err)
	}
	if out.DeliveryProposals[0].Status != "selected" || len(out.DeliverySelections) != 1 || len(out.DeliverySelections[0].Tasks) != 2 {
		t.Fatalf("selection = %+v", out)
	}
	fund, err = s.Get(fund.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fund.Balances.Reserved != 9000 || fund.Balances.Available != 1000 {
		t.Fatalf("reserved balance = %+v", fund.Balances)
	}
	if out.DeliveryProposals[0].AuthorityNote == "" || out.DeliverySelections[0].Tasks[0].Status != "planned" {
		t.Fatalf("authority/task projection = %+v", out)
	}
}

func TestDeliverySelectionCompensatesFailedOutcomePersistence(t *testing.T) {
	s, fund := outcomeStore(t)
	fund, err := s.Commit(fund.ID, "backer", "card", "compensated-backing", 10000, "compensated-backing-key", "commission")
	if err != nil {
		t.Fatal(err)
	}
	p := proof("card", "compensated-backing", "settled", 10000)
	p.Nonce = "compensated-selection-proof"
	p.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(testPrivateKey, transferProofMessage(*p)))
	fund, err = s.Reconcile(fund.ID, fund.Ledger[0].ID, "owner", "settled", 10000, p, "verified", fund.Version)
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.CreateOutcome("repo", fund.ID, "owner", outcomeTerms())
	if err != nil {
		t.Fatal(err)
	}
	proposalTerms := DeliveryProposalTerms{Approach: "Repair and verify.", Milestones: []string{"deliver"}, Cost: 9000, Availability: "This week", RelevantWork: []AttributedWork{{Kind: "pull", ID: "prior", Note: "Related repair"}}}
	out, err = s.SubmitDeliveryProposal(out.ID, DeliveryApplicant{Kind: "human", ID: "contributor", SubmittedBy: "contributor"}, proposalTerms)
	if err != nil {
		t.Fatal(err)
	}
	out, err = s.AcceptDeliveryProposal(out.ID, out.DeliveryProposals[0].ID, "contributor", out.Version)
	if err != nil {
		t.Fatal(err)
	}
	injected := errors.New("outcome disk unavailable")
	s.afterDeliveryReservationWrite = func() error { return injected }
	if _, err = s.SelectDeliveryProposals(out.ID, "owner", out.Version, []string{out.DeliveryProposals[0].ID}, []string{"owner"}, "No conflicts.", "Best proposal."); !errors.Is(err, injected) {
		t.Fatalf("selection error = %v", err)
	}
	fund, err = s.Get(fund.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fund.Balances.Reserved != 0 || fund.Balances.Available != 10000 {
		t.Fatalf("failed selection stranded reservation: %+v", fund.Balances)
	}
	current, err := s.GetOutcome(out.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Version != out.Version || current.DeliveryProposals[0].Status != "accepted" {
		t.Fatalf("failed selection changed outcome: %+v", current)
	}
	s.afterDeliveryReservationWrite = nil
	current, err = s.SelectDeliveryProposals(out.ID, "owner", current.Version, []string{current.DeliveryProposals[0].ID}, []string{"owner"}, "No conflicts.", "Best proposal.")
	if err != nil || len(current.DeliverySelections) != 1 {
		t.Fatalf("retry = %+v, %v", current, err)
	}
}

func TestFundedDeliveryProjectsWorkSpendAndStewardIntervention(t *testing.T) {
	s, fund := outcomeStore(t)
	fund, err := s.Commit(fund.ID, "backer", "card", "execution-backing", 10000, "execution-key", "commission")
	if err != nil {
		t.Fatal(err)
	}
	p := proof("card", "execution-backing", "settled", 10000)
	p.Nonce = "execution-proof"
	p.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(testPrivateKey, transferProofMessage(*p)))
	fund, err = s.Reconcile(fund.ID, fund.Ledger[0].ID, "owner", "settled", 10000, p, "verified", fund.Version)
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.CreateOutcome("repo", fund.ID, "owner", outcomeTerms())
	if err != nil {
		t.Fatal(err)
	}
	recipient := DeliveryApplicant{Kind: "human", ID: "contributor", SubmittedBy: "contributor"}
	out, err = s.SubmitDeliveryProposal(out.ID, recipient, DeliveryProposalTerms{Approach: "Deliver and verify.", Milestones: []string{"repair"}, Cost: 9000, Availability: "Now", RelevantWork: []AttributedWork{{Kind: "pull", ID: "prior", Note: "Prior work"}}})
	if err != nil {
		t.Fatal(err)
	}
	out, err = s.AcceptDeliveryProposal(out.ID, out.DeliveryProposals[0].ID, "contributor", out.Version)
	if err != nil {
		t.Fatal(err)
	}
	out, err = s.SelectDeliveryProposals(out.ID, "owner", out.Version, []string{out.DeliveryProposals[0].ID}, []string{"owner"}, "None.", "Best fit.")
	if err != nil {
		t.Fatal(err)
	}
	selection := out.DeliverySelections[0]
	task := selection.Tasks[0]
	forecast := time.Now().UTC().Add(48 * time.Hour)
	out, err = s.RecordDeliveryUpdate(out.ID, selection.ID, recipient, "contributor", out.Version, DeliveryUpdateInput{TaskID: task.ID, Status: "active", Progress: 50, Summary: "Pull is ready for checks.", Resources: []DeliveryResource{{Kind: "pull", ID: "42", Revision: "abc", Status: "open"}, {Kind: "check", ID: "ci", Revision: "abc", Status: "passed"}}, Evidence: []DeliveryEvidence{{Kind: "check", Summary: "CI passed.", Resource: DeliveryResource{Kind: "check", ID: "ci", Revision: "abc", Status: "passed"}}}, ForecastAt: &forecast, AgentMinutes: 30})
	if err != nil {
		t.Fatal(err)
	}
	out, err = s.RequestDeliveryExpense(out.ID, selection.ID, recipient, "contributor", out.Version, DeliveryExpenseInput{TaskID: task.ID, Amount: 2000, Category: "agent_compute", Description: "Bounded implementation compute.", Evidence: []DeliveryResource{{Kind: "session", ID: "session-1", Revision: "abc", Status: "completed"}}})
	if err != nil {
		t.Fatal(err)
	}
	expense := out.DeliverySelections[0].Execution.Expenses[0]
	out, err = s.DecideDeliveryExpense(out.ID, selection.ID, expense.ID, "owner", "approved", "Evidence matches the selected milestone.", out.Version)
	if err != nil {
		t.Fatal(err)
	}
	exec := out.DeliverySelections[0].Execution
	if exec.Progress != 50 || exec.AgentMinutes != 30 || exec.Spent != 2000 || exec.ForecastAt == nil {
		t.Fatalf("execution projection = %+v", exec)
	}
	fund, _ = s.Get(fund.ID)
	if fund.Balances.Spent != 2000 || fund.Balances.Reserved != 7000 {
		t.Fatalf("expense balances = %+v", fund.Balances)
	}
	out, err = s.ControlDelivery(out.ID, selection.ID, "owner", "pause", "Recipient handoff needs review.", 0, nil, out.Version)
	if err != nil {
		t.Fatal(err)
	}
	if !out.DeliverySelections[0].Execution.SpendingBlocked {
		t.Fatal("paused work did not stop spending")
	}
	if _, err = s.RequestDeliveryExpense(out.ID, selection.ID, recipient, "contributor", out.Version, DeliveryExpenseInput{TaskID: task.ID, Amount: 1, Category: "labor", Description: "late spend", Evidence: []DeliveryResource{{Kind: "task", ID: task.ID, Status: "active"}}}); !errors.Is(err, ErrConflict) {
		t.Fatalf("paused expense err = %v", err)
	}
	out, err = s.ControlDelivery(out.ID, selection.ID, "owner", "cancel_remaining", "Return unused reservation.", 0, nil, out.Version)
	if err != nil {
		t.Fatal(err)
	}
	fund, _ = s.Get(fund.ID)
	if fund.Balances.Reserved != 0 || fund.Balances.Available != 8000 || fund.Balances.Spent != 2000 {
		t.Fatalf("cancel balances = %+v", fund.Balances)
	}
}

func TestDeliverySpendingStopsWithoutInitialActivityAndRevocationSurvivesResume(t *testing.T) {
	s, fund, out, recipient, selection, task := selectedDelivery(t)
	selectedAt := selection.SelectedAt
	s.now = func() time.Time { return selectedAt.Add(15 * 24 * time.Hour) }
	input := DeliveryExpenseInput{TaskID: task.ID, Amount: 1, Category: "labor", Description: "late expense", Evidence: []DeliveryResource{{Kind: "task", ID: task.ID, Status: "active"}}}
	if _, err := s.RequestDeliveryExpense(out.ID, selection.ID, recipient, "contributor", out.Version, input); !errors.Is(err, ErrConflict) {
		t.Fatalf("inactive expense err = %v", err)
	}
	s.now = func() time.Time { return selectedAt.Add(time.Hour) }
	out, err := s.RecordDeliveryUpdate(out.ID, selection.ID, recipient, "contributor", out.Version, DeliveryUpdateInput{TaskID: task.ID, Status: "completed", Progress: 100, Summary: "First milestone retained.", Resources: []DeliveryResource{{Kind: "pull", ID: "completed-pull", Status: "merged"}}})
	if err != nil {
		t.Fatal(err)
	}
	out, err = s.ControlDelivery(out.ID, selection.ID, "owner", "access_revoked", "Repository access was removed.", 0, nil, out.Version)
	if err != nil {
		t.Fatal(err)
	}
	out, err = s.ControlDelivery(out.ID, selection.ID, "owner", "resume", "Resume reporting only.", 0, nil, out.Version)
	if err != nil {
		t.Fatal(err)
	}
	if !out.DeliverySelections[0].Execution.SpendingBlocked {
		t.Fatal("resume cleared revoked access")
	}
	sameRecipient := recipient
	if _, err = s.ControlDelivery(out.ID, selection.ID, "owner", "replace_recipient", "Reassign to the same principal.", 0, &sameRecipient, out.Version); !errors.Is(err, ErrConflict) {
		t.Fatalf("same-recipient replacement err = %v", err)
	}
	current, err := s.GetOutcome(out.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !current.DeliverySelections[0].Execution.SpendingBlocked {
		t.Fatal("same-recipient replacement cleared revoked access")
	}
	replacement := DeliveryApplicant{Kind: "human", ID: "replacement", SubmittedBy: "owner"}
	out, err = s.ControlDelivery(out.ID, selection.ID, "owner", "replace_recipient", "Move unfinished work to a distinct principal.", 0, &replacement, out.Version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.RecordDeliveryUpdate(out.ID, selection.ID, recipient, "contributor", out.Version, DeliveryUpdateInput{TaskID: task.ID, Status: "completed", Progress: 100, Summary: "Attempt to mutate retained completed work.", Resources: []DeliveryResource{{Kind: "pull", ID: "completed-pull", Status: "merged"}}}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("revoked former-recipient update err = %v", err)
	}
	if _, err = s.RequestDeliveryExpense(out.ID, selection.ID, recipient, "contributor", out.Version, input); !errors.Is(err, ErrForbidden) {
		t.Fatalf("revoked expense err = %v", err)
	}
	_ = fund
}

func TestDeliveryReplacementPreservesTerminalMilestoneOwnership(t *testing.T) {
	for _, status := range []string{"completed", "accepted", "partially_accepted", "payment_failed", "withdrawn", "timed_out", "refunded"} {
		if !deliveryTaskTerminal(DeliveryTask{Status: status}) {
			t.Fatalf("status %q should retain its recipient during replacement", status)
		}
	}
	for _, status := range []string{"planned", "active", "blocked", "handoff_failed", "correction_requested", "rejected", "disputed", "appealed"} {
		if deliveryTaskTerminal(DeliveryTask{Status: status}) {
			t.Fatalf("status %q should remain replaceable", status)
		}
	}
}

func TestPausedDeliveryRetainsReportingAndExpenseApprovalRecoversFromInterruptedPublication(t *testing.T) {
	s, _, out, recipient, selection, task := selectedDelivery(t)
	var err error
	out, err = s.ControlDelivery(out.ID, selection.ID, "owner", "pause", "Inspect the current handoff.", 0, nil, out.Version)
	if err != nil {
		t.Fatal(err)
	}
	out, err = s.RecordDeliveryUpdate(out.ID, selection.ID, recipient, "contributor", out.Version, DeliveryUpdateInput{TaskID: task.ID, Status: "handoff_failed", Progress: 60, Summary: "Retain work completed before pause.", Resources: []DeliveryResource{{Kind: "pull", ID: "42", Revision: "abc", Status: "open"}}, Evidence: []DeliveryEvidence{{Kind: "handoff", Summary: "The handoff failed after the pull was published.", Resource: DeliveryResource{Kind: "pull", ID: "42", Revision: "abc", Status: "open"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.DeliverySelections[0].Execution.Updates) != 1 {
		t.Fatal("paused update was discarded")
	}
	out, err = s.ControlDelivery(out.ID, selection.ID, "owner", "resume", "Continue after inspecting evidence.", 0, nil, out.Version)
	if err != nil {
		t.Fatal(err)
	}
	out, err = s.RecordDeliveryUpdate(out.ID, selection.ID, recipient, "contributor", out.Version, DeliveryUpdateInput{TaskID: task.ID, Status: "active", Progress: 65, Summary: "Handoff recovered with the retained pull.", Resources: []DeliveryResource{{Kind: "pull", ID: "42", Revision: "abc", Status: "open"}}})
	if err != nil {
		t.Fatal(err)
	}
	out, err = s.RequestDeliveryExpense(out.ID, selection.ID, recipient, "contributor", out.Version, DeliveryExpenseInput{TaskID: task.ID, Amount: 100, Category: "labor", Description: "Retained implementation.", Evidence: []DeliveryResource{{Kind: "pull", ID: "42", Revision: "abc", Status: "open"}}})
	if err != nil {
		t.Fatal(err)
	}
	expense := out.DeliverySelections[0].Execution.Expenses[0]
	injected := errors.New("outcome publication interrupted")
	s.afterDeliveryExpenseOutcomeWrite = func() error { return injected }
	if _, err = s.DecideDeliveryExpense(out.ID, selection.ID, expense.ID, "owner", "approved", "Evidence is bounded.", out.Version); !errors.Is(err, injected) {
		t.Fatalf("approval err = %v", err)
	}
	s.afterDeliveryExpenseOutcomeWrite = nil
	recovered, err := s.GetOutcome(out.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.DeliverySelections[0].Execution.Expenses[0].Status != "approved" {
		t.Fatalf("recovered outcome = %+v", recovered.DeliverySelections[0].Execution)
	}
	fund, err := s.Get(recovered.FundID)
	if err != nil {
		t.Fatal(err)
	}
	if fund.Balances.Spent != 100 {
		t.Fatalf("recovered fund = %+v", fund.Balances)
	}
}

func TestMilestoneAcceptancePaysOnlyDesignatedEvidenceBackedResults(t *testing.T) {
	s, fund, out, recipient, selection, task := selectedDelivery(t)
	measure := OutcomeMeasure{Name: "p95 latency", Status: "met", Value: "184ms", Evidence: DeliveryResource{Kind: "release", ID: "release-1", Revision: "abc", Status: "published"}}
	if _, err := s.ReviewMilestone(out.ID, selection.ID, task.ID, "outsider", out.Version, MilestoneReviewInput{Decision: "accepted", Rationale: "Looks good.", OutcomeMeasures: []OutcomeMeasure{measure}}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("undesignated review err = %v", err)
	}
	out, err := s.RecordDeliveryUpdate(out.ID, selection.ID, recipient, "contributor", out.Version, DeliveryUpdateInput{TaskID: task.ID, Status: "completed", Progress: 100, Summary: "Merged and released with the declared result measured.", Resources: []DeliveryResource{{Kind: "commit", ID: "abc", Revision: "abc", Status: "authored"}, {Kind: "pull", ID: "42", Revision: "abc", Status: "merged"}, {Kind: "check", ID: "ci", Revision: "abc", Status: "passed"}, {Kind: "preview", ID: "preview-1", Revision: "abc", Status: "approved"}, {Kind: "release", ID: "release-1", Revision: "abc", Status: "published"}, {Kind: "deployment", ID: "prod-1", Revision: "abc", Status: "successful"}}, Evidence: []DeliveryEvidence{{Kind: "handoff", Summary: "Maintainer received the exact commit.", Resource: DeliveryResource{Kind: "commit", ID: "abc", Revision: "abc", Status: "authored"}}}})
	if err != nil {
		t.Fatal(err)
	}
	out, err = s.ReviewMilestone(out.ID, selection.ID, task.ID, "owner", out.Version, MilestoneReviewInput{Decision: "partial_award", AwardAmount: 4000, Rationale: "The accepted contribution met the result; residual documentation was out of scope for this award.", Dissent: []string{"One reviewer preferred another benchmark window."}, OutcomeMeasures: []OutcomeMeasure{measure}})
	if err != nil {
		t.Fatal(err)
	}
	got := out.DeliverySelections[0].Tasks[0]
	if got.Status != "partially_accepted" || len(got.Reviews) != 1 || got.Reviews[0].Authorship[0] != "contributor" || got.Reviews[0].ReleasedAmount != 500 {
		t.Fatalf("milestone review = %+v", got)
	}
	fund, err = s.Get(fund.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fund.Balances.Spent != 4000 || fund.Balances.Reserved != 4500 || fund.Balances.Available != 1500 {
		t.Fatalf("partial award balances = %+v", fund.Balances)
	}
	if _, err = s.ReviewMilestone(out.ID, selection.ID, task.ID, "owner", out.Version, MilestoneReviewInput{Decision: "accepted", Rationale: "Duplicate.", OutcomeMeasures: []OutcomeMeasure{measure}}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate award err = %v", err)
	}
	_ = recipient
}

func TestMilestoneCorrectionAppealAndPaymentRecoveryAreDeterministic(t *testing.T) {
	s, fund, out, recipient, selection, task := selectedDelivery(t)
	var err error
	out, err = s.RecordDeliveryUpdate(out.ID, selection.ID, recipient, "contributor", out.Version, DeliveryUpdateInput{TaskID: task.ID, Status: "completed", Progress: 100, Summary: "Candidate result.", Resources: []DeliveryResource{{Kind: "commit", ID: "abc", Revision: "abc", Status: "authored"}}})
	if err != nil {
		t.Fatal(err)
	}
	out, err = s.ReviewMilestone(out.ID, selection.ID, task.ID, "owner", out.Version, MilestoneReviewInput{Decision: "correction_requested", Rationale: "The release observation is missing.", Dissent: []string{"Implementation checks did pass."}})
	if err != nil {
		t.Fatal(err)
	}
	out, err = s.RecoverMilestone(out.ID, selection.ID, task.ID, "contributor", "appeal", "The release was delayed independently of the merged contribution.", out.Version)
	if err != nil || out.DeliverySelections[0].Tasks[0].Status != "appealed" {
		t.Fatalf("appeal = %+v, %v", out, err)
	}
	measure := OutcomeMeasure{Name: "correctness", Status: "met", Value: "all checks passed", Evidence: DeliveryResource{Kind: "check", ID: "ci", Revision: "abc", Status: "passed"}}
	out, err = s.ReviewMilestone(out.ID, selection.ID, task.ID, "owner", out.Version, MilestoneReviewInput{Decision: "accepted", Rationale: "Appeal resolved from exact release evidence.", OutcomeMeasures: []OutcomeMeasure{measure}})
	if err != nil {
		t.Fatal(err)
	}
	out, err = s.RecoverMilestone(out.ID, selection.ID, task.ID, "owner", "payment_failed", "Payment processor rejected the transfer.", out.Version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.RecordDeliveryUpdate(out.ID, selection.ID, recipient, "contributor", out.Version, DeliveryUpdateInput{TaskID: task.ID, Status: "active", Progress: 100, Summary: "Attempt to reopen accepted work after payment failure.", Resources: []DeliveryResource{{Kind: "check", ID: "ci", Revision: "abc", Status: "passed"}}}); !errors.Is(err, ErrConflict) {
		t.Fatalf("payment-failed reopen err = %v", err)
	}
	replacement := DeliveryApplicant{Kind: "human", ID: "replacement", SubmittedBy: "owner"}
	out, err = s.ControlDelivery(out.ID, selection.ID, "owner", "replace_recipient", "Reassign only the unfinished sibling milestone.", 0, &replacement, out.Version)
	if err != nil {
		t.Fatal(err)
	}
	failedTask := deliveryTask(&out.DeliverySelections[0], task.ID)
	if failedTask == nil || failedTask.RecipientID != recipient.ID {
		t.Fatalf("payment-failed award recipient changed = %+v", failedTask)
	}
	fund, _ = s.Get(fund.ID)
	if fund.Balances.Spent != 0 || fund.Balances.Reserved != 9000 {
		t.Fatalf("failed payment balances = %+v", fund.Balances)
	}
	out, err = s.RecoverMilestone(out.ID, selection.ID, task.ID, "owner", "retry_payment", "Recipient details were corrected.", out.Version)
	if err != nil {
		t.Fatal(err)
	}
	fund, _ = s.Get(fund.ID)
	if entry := fund.Ledger[len(fund.Ledger)-1]; entry.Kind != "milestone_award" || entry.ContributorID != recipient.ID {
		t.Fatalf("retried award attribution = %+v", entry)
	}
	out, err = s.RecoverMilestone(out.ID, selection.ID, task.ID, "owner", "refund", "Accepted refund under the original fund policy.", out.Version)
	if err != nil {
		t.Fatal(err)
	}
	fund, _ = s.Get(fund.ID)
	if fund.Balances.Spent != 0 || fund.Balances.Refunded != 4500 || fund.Balances.Available != 5500 {
		t.Fatalf("refund balances = %+v", fund.Balances)
	}
	if _, err = s.RecoverMilestone(out.ID, selection.ID, task.ID, "contributor", "withdraw", "Attempt a second release after refund.", out.Version); !errors.Is(err, ErrForbidden) {
		t.Fatalf("post-refund withdrawal err = %v", err)
	}
}

func TestMilestoneWithdrawalAndTimeoutReleaseOnlyTheirAllocation(t *testing.T) {
	s, fund, out, recipient, selection, task := selectedDelivery(t)
	var err error
	out, err = s.RecordDeliveryUpdate(out.ID, selection.ID, recipient, "contributor", out.Version, DeliveryUpdateInput{TaskID: task.ID, Status: "completed", Progress: 100, Summary: "Completed result awaiting compensation review.", Resources: []DeliveryResource{{Kind: "check", ID: "ci", Revision: "abc", Status: "passed"}}})
	if err != nil {
		t.Fatal(err)
	}
	out, err = s.RecoverMilestone(out.ID, selection.ID, task.ID, "contributor", "withdraw", "Recipient cannot complete this milestone.", out.Version)
	if err != nil {
		t.Fatal(err)
	}
	fund, _ = s.Get(fund.ID)
	if fund.Balances.Available != 5500 || fund.Balances.Reserved != 4500 {
		t.Fatalf("withdrawal balances = %+v", fund.Balances)
	}
	if _, err = s.RecoverMilestone(out.ID, selection.ID, task.ID, "contributor", "withdraw", "Duplicate withdrawal.", out.Version); !errors.Is(err, ErrForbidden) {
		t.Fatalf("duplicate withdrawal err = %v", err)
	}
	measure := OutcomeMeasure{Name: "result", Status: "met", Value: "passed", Evidence: DeliveryResource{Kind: "check", ID: "ci", Revision: "abc", Status: "passed"}}
	if _, err = s.ReviewMilestone(out.ID, selection.ID, task.ID, "owner", out.Version, MilestoneReviewInput{Decision: "accepted", Rationale: "A terminal release cannot be reopened by a later award.", OutcomeMeasures: []OutcomeMeasure{measure}}); !errors.Is(err, ErrConflict) {
		t.Fatalf("post-withdrawal award err = %v", err)
	}
	fund, _ = s.Get(fund.ID)
	if fund.Balances.Available != 5500 || fund.Balances.Reserved != 4500 {
		t.Fatalf("duplicate withdrawal changed balances = %+v", fund.Balances)
	}

	s2, fund2, out2, _, selection2, task2 := selectedDelivery(t)
	deadline := out2.Revisions[len(out2.Revisions)-1].Terms.Deadline
	s2.now = func() time.Time { return deadline.Add(time.Second) }
	out2, err = s2.RecoverMilestone(out2.ID, selection2.ID, task2.ID, "owner", "timeout", "The original acceptance window expired.", out2.Version)
	if err != nil {
		t.Fatal(err)
	}
	fund2, _ = s2.Get(fund2.ID)
	if fund2.Balances.Available != 5500 || out2.DeliverySelections[0].Tasks[0].Status != "timed_out" {
		t.Fatalf("timeout = %+v %+v", fund2.Balances, out2.DeliverySelections[0].Tasks[0])
	}
	if _, err = s2.RecoverMilestone(out2.ID, selection2.ID, task2.ID, "owner", "timeout", "Duplicate timeout.", out2.Version); !errors.Is(err, ErrForbidden) {
		t.Fatalf("duplicate timeout err = %v", err)
	}
	fund2, _ = s2.Get(fund2.ID)
	if fund2.Balances.Available != 5500 || fund2.Balances.Reserved != 4500 {
		t.Fatalf("duplicate timeout changed balances = %+v", fund2.Balances)
	}
}

func TestPartialAwardFailureRecoveryPreservesSiblingReservation(t *testing.T) {
	for _, action := range []string{"withdraw", "timeout"} {
		t.Run(action, func(t *testing.T) {
			s, fund, out, recipient, selection, task := selectedDelivery(t)
			var err error
			out, err = s.RecordDeliveryUpdate(out.ID, selection.ID, recipient, "contributor", out.Version, DeliveryUpdateInput{TaskID: task.ID, Status: "completed", Progress: 100, Summary: "Measured partial result.", Resources: []DeliveryResource{{Kind: "check", ID: "ci", Revision: "abc", Status: "passed"}}})
			if err != nil {
				t.Fatal(err)
			}
			measure := OutcomeMeasure{Name: "result", Status: "met", Value: "bounded result", Evidence: DeliveryResource{Kind: "check", ID: "ci", Revision: "abc", Status: "passed"}}
			out, err = s.ReviewMilestone(out.ID, selection.ID, task.ID, "owner", out.Version, MilestoneReviewInput{Decision: "partial_award", AwardAmount: 4000, Rationale: "Accept the bounded portion.", OutcomeMeasures: []OutcomeMeasure{measure}})
			if err != nil {
				t.Fatal(err)
			}
			out, err = s.RecoverMilestone(out.ID, selection.ID, task.ID, "owner", "payment_failed", "Processor rejected payment.", out.Version)
			if err != nil {
				t.Fatal(err)
			}
			actor := "contributor"
			if action == "timeout" {
				actor = "owner"
				deadline := out.Revisions[len(out.Revisions)-1].Terms.Deadline
				s.now = func() time.Time { return deadline.Add(time.Second) }
			}
			out, err = s.RecoverMilestone(out.ID, selection.ID, task.ID, actor, action, "Close the failed award through its declared recovery.", out.Version)
			if err != nil {
				t.Fatal(err)
			}
			fund, _ = s.Get(fund.ID)
			if fund.Balances.Available != 5500 || fund.Balances.Reserved != 4500 || fund.Balances.Spent != 0 {
				t.Fatalf("%s balances = %+v", action, fund.Balances)
			}
		})
	}
}

func TestCancelRemainingUsesAwardAndReleaseProjection(t *testing.T) {
	s, fund, out, recipient, selection, task := selectedDelivery(t)
	var err error
	out, err = s.RecordDeliveryUpdate(out.ID, selection.ID, recipient, "contributor", out.Version, DeliveryUpdateInput{TaskID: task.ID, Status: "completed", Progress: 100, Summary: "Accepted result.", Resources: []DeliveryResource{{Kind: "release", ID: "release-1", Revision: "abc", Status: "published"}}})
	if err != nil {
		t.Fatal(err)
	}
	measure := OutcomeMeasure{Name: "result", Status: "met", Value: "released", Evidence: DeliveryResource{Kind: "release", ID: "release-1", Revision: "abc", Status: "published"}}
	out, err = s.ReviewMilestone(out.ID, selection.ID, task.ID, "owner", out.Version, MilestoneReviewInput{Decision: "accepted", Rationale: "The exact released result meets the contract.", OutcomeMeasures: []OutcomeMeasure{measure}})
	if err != nil {
		t.Fatal(err)
	}
	if out.DeliverySelections[0].Execution.Spent != 4500 {
		t.Fatalf("award projection = %+v", out.DeliverySelections[0].Execution)
	}
	out, err = s.ControlDelivery(out.ID, selection.ID, "owner", "cancel_remaining", "Release only the unspent sibling allocation.", 0, nil, out.Version)
	if err != nil {
		t.Fatal(err)
	}
	fund, _ = s.Get(fund.ID)
	if fund.Balances.Reserved != 0 || fund.Balances.Available != 5500 || fund.Balances.Spent != 4500 {
		t.Fatalf("award cancellation balances = %+v", fund.Balances)
	}
}

func TestCancelRemainingAggregatesEveryRetainedAwardHistory(t *testing.T) {
	s, fund, out, recipient, selection, task := selectedDelivery(t)
	var err error
	out, err = s.RecordDeliveryUpdate(out.ID, selection.ID, recipient, "contributor", out.Version, DeliveryUpdateInput{TaskID: task.ID, Status: "completed", Progress: 100, Summary: "Measured partial result.", Resources: []DeliveryResource{{Kind: "check", ID: "ci", Revision: "abc", Status: "passed"}}})
	if err != nil {
		t.Fatal(err)
	}
	measure := OutcomeMeasure{Name: "result", Status: "met", Value: "bounded result", Evidence: DeliveryResource{Kind: "check", ID: "ci", Revision: "abc", Status: "passed"}}
	out, err = s.ReviewMilestone(out.ID, selection.ID, task.ID, "owner", out.Version, MilestoneReviewInput{Decision: "partial_award", AwardAmount: 4000, Rationale: "Accept the bounded portion.", OutcomeMeasures: []OutcomeMeasure{measure}})
	if err != nil {
		t.Fatal(err)
	}
	out, err = s.RecoverMilestone(out.ID, selection.ID, task.ID, "owner", "payment_failed", "Processor rejected the first payment.", out.Version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ReviewMilestone(out.ID, selection.ID, task.ID, "owner", out.Version, MilestoneReviewInput{Decision: "partial_award", AwardAmount: 4000, Rationale: "Do not create a second award; retry the retained one.", OutcomeMeasures: []OutcomeMeasure{measure}}); !errors.Is(err, ErrConflict) {
		t.Fatalf("second award err = %v", err)
	}

	// Retained records from before the single-award invariant can contain more
	// than one award history. Cancellation must still match their complete ledger.
	err = s.lock(func() error {
		stored, readErr := s.readOutcome(out.ID)
		if readErr != nil {
			return readErr
		}
		sel := deliverySelection(stored, selection.ID)
		retainedTask := deliveryTask(sel, task.ID)
		now := s.now()
		retainedTask.Reviews = append(retainedTask.Reviews, MilestoneReview{ID: randomID(), Decision: "partial_award", ReviewerID: "owner", Rationale: "Retained successor award.", AwardAmount: 4000, PaymentStatus: "paid", CreatedAt: now})
		retainedTask.Status = "partially_accepted"
		stored.Version++
		stored.UpdatedAt = now
		storedFund, readErr := s.read(stored.FundID)
		if readErr != nil {
			return readErr
		}
		storedFund.Ledger = append(storedFund.Ledger, Entry{ID: randomID(), Kind: "milestone_award", Amount: 4000, Status: "paid", ExternalReference: stored.ID + ":retained-award", ContributorID: task.RecipientID, ActorID: "owner", Note: "Retained successor award.", CreatedAt: now})
		storedFund.Version++
		storedFund.UpdatedAt = now
		storedFund.Balances = derive(storedFund.Terms, storedFund.Ledger)
		if writeErr := s.write(storedFund); writeErr != nil {
			return writeErr
		}
		return s.writeOutcome(stored)
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err = s.GetOutcome(out.ID)
	if err != nil {
		t.Fatal(err)
	}
	execution := out.DeliverySelections[0].Execution
	if execution.Spent != 4000 || execution.Released != 500 {
		t.Fatalf("retained projection = %+v", execution)
	}
	out, err = s.ControlDelivery(out.ID, selection.ID, "owner", "cancel_remaining", "Release the exact retained remainder.", 0, nil, out.Version)
	if err != nil {
		t.Fatal(err)
	}
	fund, _ = s.Get(fund.ID)
	if fund.Balances.Reserved != 0 || fund.Balances.Available != 6000 || fund.Balances.Spent != 4000 {
		t.Fatalf("retained cancellation balances = %+v", fund.Balances)
	}
}

func selectedDelivery(t *testing.T) (*Store, Fund, FundedOutcome, DeliveryApplicant, DeliverySelection, DeliveryTask) {
	t.Helper()
	s, fund := outcomeStore(t)
	fund, err := s.Commit(fund.ID, "backer", "card", "selected-delivery-backing", 10000, "selected-delivery-key", "commission")
	if err != nil {
		t.Fatal(err)
	}
	p := proof("card", "selected-delivery-backing", "settled", 10000)
	p.Nonce = "selected-delivery-proof"
	p.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(testPrivateKey, transferProofMessage(*p)))
	fund, err = s.Reconcile(fund.ID, fund.Ledger[0].ID, "owner", "settled", 10000, p, "verified", fund.Version)
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.CreateOutcome("repo", fund.ID, "owner", outcomeTerms())
	if err != nil {
		t.Fatal(err)
	}
	recipient := DeliveryApplicant{Kind: "human", ID: "contributor", SubmittedBy: "contributor"}
	out, err = s.SubmitDeliveryProposal(out.ID, recipient, DeliveryProposalTerms{Approach: "Deliver and verify.", Milestones: []string{"diagnose", "repair"}, Cost: 9000, Availability: "Now", RelevantWork: []AttributedWork{{Kind: "pull", ID: "prior", Note: "Prior work"}}})
	if err != nil {
		t.Fatal(err)
	}
	out, err = s.AcceptDeliveryProposal(out.ID, out.DeliveryProposals[0].ID, "contributor", out.Version)
	if err != nil {
		t.Fatal(err)
	}
	out, err = s.SelectDeliveryProposals(out.ID, "owner", out.Version, []string{out.DeliveryProposals[0].ID}, []string{"owner"}, "None.", "Best fit.")
	if err != nil {
		t.Fatal(err)
	}
	selection := out.DeliverySelections[0]
	return s, fund, out, recipient, selection, selection.Tasks[0]
}

func TestOutcomeFundingProjectsOverlappingAndEmbargoedWork(t *testing.T) {
	s, fund := outcomeStore(t)
	terms := outcomeTerms()
	terms.Source.Kind = "security_repair"
	terms.Source.Visibility = "embargoed"
	a, err := s.CreateOutcome("repo", fund.ID, "owner", terms)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateOutcome("repo", fund.ID, "owner", terms); err != nil {
		t.Fatal(err)
	}
	a, err = s.GetOutcome(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	for _, d := range a.Diagnostics {
		kinds[d.Kind] = true
	}
	if !kinds["embargoed_work"] || !kinds["overlapping_award"] {
		t.Fatalf("diagnostics = %+v", a.Diagnostics)
	}
}

func TestOutcomeFundingRejectsStorageTraversalIdentifiers(t *testing.T) {
	s, _ := outcomeStore(t)
	if _, err := s.GetOutcome("../fund"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("traversal read err = %v", err)
	}
	if _, err := s.CreateOutcome("repo", "../fund", "owner", outcomeTerms()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("traversal fund err = %v", err)
	}
}

func TestOutcomeFundingDiagnosesAggregateSharedFundOvercommitment(t *testing.T) {
	s, fund := outcomeStore(t)
	fund, err := s.Commit(fund.ID, "backer", "card", "shared-backing", 100, "shared-key", "")
	if err != nil {
		t.Fatal(err)
	}
	fund, err = s.Reconcile(fund.ID, fund.Ledger[0].ID, "owner", "settled", 100, proof("card", "shared-backing", "settled", 100), "", fund.Version)
	if err != nil {
		t.Fatal(err)
	}
	firstTerms := outcomeTerms()
	firstTerms.Budget = 60
	firstTerms.Milestones = nil
	first, err := s.CreateOutcome("repo", fund.ID, "owner", firstTerms)
	if err != nil {
		t.Fatal(err)
	}
	first, err = s.PledgeOutcome(first.ID, "backer", "", 60, "first-allocation", "", first.Version)
	if err != nil {
		t.Fatal(err)
	}
	secondTerms := firstTerms
	secondTerms.Source.ID = "issue-2"
	second, err := s.CreateOutcome("repo", fund.ID, "owner", secondTerms)
	if err != nil {
		t.Fatal(err)
	}
	second, err = s.PledgeOutcome(second.ID, "backer", "", 60, "second-allocation", "", second.Version)
	if err != nil {
		t.Fatal(err)
	}
	first, err = s.GetOutcome(first.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, outcome := range []FundedOutcome{first, second} {
		kinds := map[string]bool{}
		for _, diagnostic := range outcome.Diagnostics {
			kinds[diagnostic.Kind] = true
		}
		if !kinds["unsettled_backing"] || kinds["overlapping_award"] {
			t.Fatalf("shared-fund diagnostics = %+v", outcome.Diagnostics)
		}
	}
}
