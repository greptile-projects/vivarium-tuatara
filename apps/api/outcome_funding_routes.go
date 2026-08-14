package main

import (
	"errors"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/projectfunds"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

type createFundedOutcomeInput struct {
	FundID string                    `json:"fund_id"`
	Terms  projectfunds.OutcomeTerms `json:"terms"`
}
type reviseFundedOutcomeInput struct {
	ExpectedVersion int                       `json:"expected_version"`
	Reason          string                    `json:"reason"`
	Terms           projectfunds.OutcomeTerms `json:"terms"`
}
type outcomePledgeInput struct {
	ExpectedVersion int    `json:"expected_version"`
	Amount          int64  `json:"amount"`
	MilestoneID     string `json:"milestone_id"`
	IdempotencyKey  string `json:"idempotency_key"`
	Note            string `json:"note"`
}
type outcomeActionInput struct {
	ExpectedVersion int    `json:"expected_version"`
	Action          string `json:"action"`
	Reason          string `json:"reason"`
}
type deliveryProposalInput struct {
	Applicant projectfunds.DeliveryApplicant     `json:"applicant"`
	Terms     projectfunds.DeliveryProposalTerms `json:"terms"`
}
type deliveryProposalAcceptanceInput struct {
	ExpectedVersion int `json:"expected_version"`
}
type deliverySelectionInput struct {
	ExpectedVersion    int      `json:"expected_version"`
	ProposalIDs        []string `json:"proposal_ids"`
	ReviewerIDs        []string `json:"reviewer_ids"`
	ConflictDisclosure string   `json:"conflict_disclosure"`
	Rationale          string   `json:"rationale"`
}
type deliveryUpdateRequest struct {
	ExpectedVersion int                              `json:"expected_version"`
	Recipient       projectfunds.DeliveryApplicant   `json:"recipient"`
	Update          projectfunds.DeliveryUpdateInput `json:"update"`
}
type deliveryExpenseRequest struct {
	ExpectedVersion int                               `json:"expected_version"`
	Recipient       projectfunds.DeliveryApplicant    `json:"recipient"`
	Expense         projectfunds.DeliveryExpenseInput `json:"expense"`
}
type deliveryExpenseDecisionRequest struct {
	ExpectedVersion int    `json:"expected_version"`
	Decision        string `json:"decision"`
	Reason          string `json:"reason"`
}
type deliveryControlRequest struct {
	ExpectedVersion int                             `json:"expected_version"`
	Action          string                          `json:"action"`
	Reason          string                          `json:"reason"`
	Amount          int64                           `json:"amount"`
	Replacement     *projectfunds.DeliveryApplicant `json:"replacement"`
}
type milestoneReviewRequest struct {
	ExpectedVersion int                               `json:"expected_version"`
	Review          projectfunds.MilestoneReviewInput `json:"review"`
}
type milestoneRecoveryRequest struct {
	ExpectedVersion int    `json:"expected_version"`
	Action          string `json:"action"`
	Reason          string `json:"reason"`
}

func registerOutcomeFundingRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, store *projectfunds.Store, orgs ...*organizations.Store) {
	var organizationStore *organizations.Store
	if len(orgs) > 0 {
		organizationStore = orgs[0]
	}
	mux.HandleFunc("POST /repositories/{id}/funded-outcomes", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in createFundedOutcomeInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete outcome funding contract is required")
			return
		}
		var out projectfunds.FundedOutcome
		err := catalog.WithCurrentParticipant(actor.UserID, r.PathValue("id"), func() error {
			var err error
			out, err = store.CreateOutcome(r.PathValue("id"), in.FundID, actor.UserID, in.Terms)
			return err
		})
		writeFundedOutcome(w, out, err, 201)
	})
	mux.HandleFunc("GET /repositories/{id}/funded-outcomes", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		values, err := store.ListOutcomes(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "outcome_funding_unavailable", "funded outcomes could not be read")
			return
		}
		participant := actor.UserID != "" && catalog.WithCurrentParticipant(actor.UserID, r.PathValue("id"), func() error { return nil }) == nil
		visible := values[:0]
		for _, v := range values {
			visibility := v.Revisions[len(v.Revisions)-1].Terms.Source.Visibility
			if visibility == "public" || participant {
				visible = append(visible, v)
			}
		}
		writeJSON(w, 200, map[string]any{"funded_outcomes": visible})
	})
	mux.HandleFunc("GET /repositories/{id}/funded-outcomes/{outcome_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		out, err := store.GetOutcome(r.PathValue("outcome_id"))
		if err != nil || out.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "funded_outcome_not_found", "funded outcome not found")
			return
		}
		visibility := out.Revisions[len(out.Revisions)-1].Terms.Source.Visibility
		if visibility != "public" && (actor.UserID == "" || catalog.WithCurrentParticipant(actor.UserID, r.PathValue("id"), func() error { return nil }) != nil) {
			writeAPIError(w, 403, "funded_outcome_forbidden", "this funding contract is limited to project participants")
			return
		}
		writeJSON(w, 200, out)
	})
	mux.HandleFunc("POST /repositories/{id}/funded-outcomes/{outcome_id}/revisions", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in reviseFundedOutcomeInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete successor contract and replan reason are required")
			return
		}
		out, err := store.GetOutcome(r.PathValue("outcome_id"))
		if err != nil || out.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "funded_outcome_not_found", "funded outcome not found")
			return
		}
		out, err = store.ReviseOutcome(out.ID, actor.UserID, in.ExpectedVersion, in.Terms, in.Reason)
		writeFundedOutcome(w, out, err, 200)
	})
	mux.HandleFunc("POST /repositories/{id}/funded-outcomes/{outcome_id}/pledges", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		if actor.UserID == "" {
			writeAPIError(w, 401, "authentication_required", "an authenticated backer is required")
			return
		}
		var in outcomePledgeInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a versioned whole-outcome or milestone pledge is required")
			return
		}
		out, err := store.GetOutcome(r.PathValue("outcome_id"))
		if err != nil || out.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "funded_outcome_not_found", "funded outcome not found")
			return
		}
		visibility := out.Revisions[len(out.Revisions)-1].Terms.Source.Visibility
		if visibility != "public" && catalog.WithCurrentParticipant(actor.UserID, r.PathValue("id"), func() error { return nil }) != nil {
			writeAPIError(w, 403, "funded_outcome_forbidden", "only project participants may back permission-bounded work")
			return
		}
		out, err = store.PledgeOutcome(out.ID, actor.UserID, in.MilestoneID, in.Amount, in.IdempotencyKey, in.Note, in.ExpectedVersion)
		writeFundedOutcome(w, out, err, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/funded-outcomes/{outcome_id}/pledges/{pledge_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		if actor.UserID == "" {
			writeAPIError(w, 401, "authentication_required", "the backing contributor is required")
			return
		}
		var in outcomeActionInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a pledge action and reason are required")
			return
		}
		out, err := store.GetOutcome(r.PathValue("outcome_id"))
		if err != nil || out.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "funded_outcome_not_found", "funded outcome not found")
			return
		}
		visibility := out.Revisions[len(out.Revisions)-1].Terms.Source.Visibility
		participant := catalog.WithCurrentParticipant(actor.UserID, r.PathValue("id"), func() error { return nil }) == nil
		if visibility != "public" && !participant {
			if in.Action != "withdraw" {
				writeAPIError(w, 403, "funded_outcome_forbidden", "current outcome terms are limited to project participants")
				return
			}
			out, err = store.ChangePledge(out.ID, r.PathValue("pledge_id"), actor.UserID, in.Action, in.Reason, in.ExpectedVersion)
			if err != nil {
				writeFundedOutcome(w, out, err, 200)
				return
			}
			for _, pledge := range out.Pledges {
				if pledge.ID == r.PathValue("pledge_id") {
					writeJSON(w, 200, map[string]any{"outcome_id": out.ID, "pledge": map[string]any{"id": pledge.ID, "status": pledge.Status, "updated_at": pledge.UpdatedAt}})
					return
				}
			}
			writeAPIError(w, 500, "outcome_funding_unavailable", "updated pledge could not be projected")
			return
		}
		out, err = store.ChangePledge(out.ID, r.PathValue("pledge_id"), actor.UserID, in.Action, in.Reason, in.ExpectedVersion)
		writeFundedOutcome(w, out, err, 200)
	})
	mux.HandleFunc("POST /repositories/{id}/funded-outcomes/{outcome_id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in outcomeActionInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a versioned cancellation reason is required")
			return
		}
		out, err := store.GetOutcome(r.PathValue("outcome_id"))
		if err != nil || out.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "funded_outcome_not_found", "funded outcome not found")
			return
		}
		out, err = store.CancelOutcome(out.ID, actor.UserID, in.Reason, in.ExpectedVersion)
		writeFundedOutcome(w, out, err, 200)
	})
	mux.HandleFunc("POST /repositories/{id}/funded-outcomes/{outcome_id}/delivery-proposals", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		if actor.UserID == "" {
			writeAPIError(w, 401, "authentication_required", "an attributable applicant is required")
			return
		}
		var in deliveryProposalInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete delivery proposal is required")
			return
		}
		in.Applicant.SubmittedBy = actor.UserID
		if in.Applicant.Kind == "human" && in.Applicant.ID != actor.UserID {
			writeAPIError(w, 403, "delivery_proposal_forbidden", "the applicant must be the human, a current team member, or a current approved-agent operator")
			return
		}
		if in.Applicant.Kind != "human" {
			if repository, err := catalog.GetByID(r.PathValue("id")); err == nil {
				in.Applicant.OrganizationID = repository.OrganizationID
			}
		}
		out, err := store.GetOutcome(r.PathValue("outcome_id"))
		if err != nil || out.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "funded_outcome_not_found", "funded outcome not found")
			return
		}
		visibility := out.Revisions[len(out.Revisions)-1].Terms.Source.Visibility
		requiredParticipant := ""
		if visibility != "public" && catalog.WithCurrentParticipant(actor.UserID, r.PathValue("id"), func() error { return nil }) != nil {
			writeAPIError(w, 403, "funded_outcome_forbidden", "permission-bounded work accepts proposals only from project participants")
			return
		}
		if visibility != "public" {
			requiredParticipant = actor.UserID
		}
		err = withCurrentDeliveryApplicants([]projectfunds.DeliveryApplicant{in.Applicant}, actor.UserID, requiredParticipant, r.PathValue("id"), catalog, organizationStore, func() error {
			var mutationErr error
			out, mutationErr = store.SubmitDeliveryProposal(out.ID, in.Applicant, in.Terms)
			return mutationErr
		})
		writeFundedOutcome(w, out, err, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/funded-outcomes/{outcome_id}/delivery-proposals/{proposal_id}/accept", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		if actor.UserID == "" {
			writeAPIError(w, 401, "authentication_required", "an attributable recipient is required")
			return
		}
		var in deliveryProposalAcceptanceInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "the current outcome version is required")
			return
		}
		out, err := store.GetOutcome(r.PathValue("outcome_id"))
		if err != nil || out.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "funded_outcome_not_found", "funded outcome not found")
			return
		}
		var applicant *projectfunds.DeliveryApplicant
		for i := range out.DeliveryProposals {
			if out.DeliveryProposals[i].ID == r.PathValue("proposal_id") {
				applicant = &out.DeliveryProposals[i].Applicant
				break
			}
		}
		if applicant == nil {
			writeAPIError(w, 404, "delivery_proposal_not_found", "delivery proposal not found")
			return
		}
		if applicant.Kind == "human" && applicant.ID != actor.UserID {
			writeAPIError(w, 403, "delivery_proposal_forbidden", "only the proposed human, team member, or approved-agent operator may accept delivery")
			return
		}
		requiredParticipant := ""
		if out.Revisions[len(out.Revisions)-1].Terms.Source.Visibility != "public" {
			requiredParticipant = actor.UserID
		}
		err = withCurrentDeliveryApplicants([]projectfunds.DeliveryApplicant{*applicant}, actor.UserID, requiredParticipant, r.PathValue("id"), catalog, organizationStore, func() error {
			var mutationErr error
			out, mutationErr = store.AcceptDeliveryProposal(out.ID, r.PathValue("proposal_id"), actor.UserID, in.ExpectedVersion)
			return mutationErr
		})
		writeFundedOutcome(w, out, err, 200)
	})
	mux.HandleFunc("POST /repositories/{id}/funded-outcomes/{outcome_id}/delivery-selections", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		if actor.UserID == "" {
			writeAPIError(w, 401, "authentication_required", "an attributable fund steward is required")
			return
		}
		var in deliverySelectionInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "accepted recipients, disclosure, rationale, and version are required")
			return
		}
		out, err := store.GetOutcome(r.PathValue("outcome_id"))
		if err != nil || out.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "funded_outcome_not_found", "funded outcome not found")
			return
		}
		applicants := make([]projectfunds.DeliveryApplicant, 0, len(in.ProposalIDs))
		for _, id := range in.ProposalIDs {
			pos := deliveryProposalPosition(out, id)
			if pos < 0 {
				writeAPIError(w, 409, "delivery_eligibility_changed", "a selected recipient is no longer an eligible human, team, or approved agent")
				return
			}
			applicants = append(applicants, out.DeliveryProposals[pos].Applicant)
		}
		if len(in.ReviewerIDs) == 0 {
			in.ReviewerIDs = []string{actor.UserID}
		}
		authorityApplicants := append([]projectfunds.DeliveryApplicant(nil), applicants...)
		for _, reviewerID := range in.ReviewerIDs {
			authorityApplicants = append(authorityApplicants, projectfunds.DeliveryApplicant{Kind: "human", ID: reviewerID, SubmittedBy: actor.UserID})
		}
		err = withCurrentDeliveryApplicants(authorityApplicants, "", actor.UserID, r.PathValue("id"), catalog, organizationStore, func() error {
			var mutationErr error
			out, mutationErr = store.SelectDeliveryProposals(out.ID, actor.UserID, in.ExpectedVersion, in.ProposalIDs, in.ReviewerIDs, in.ConflictDisclosure, in.Rationale)
			return mutationErr
		})
		writeFundedOutcome(w, out, err, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/funded-outcomes/{outcome_id}/delivery-selections/{selection_id}/updates", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		if actor.UserID == "" {
			writeAPIError(w, 401, "authentication_required", "an attributable selected contributor is required")
			return
		}
		var in deliveryUpdateRequest
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a versioned task progress update with exact resources is required")
			return
		}
		in.Recipient.SubmittedBy = actor.UserID
		if in.Recipient.Kind == "human" && in.Recipient.ID != actor.UserID {
			writeAPIError(w, 403, "delivery_execution_forbidden", "only the selected human, team member, or approved-agent operator may report delivery")
			return
		}
		out, err := store.GetOutcome(r.PathValue("outcome_id"))
		if err != nil || out.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "funded_outcome_not_found", "funded outcome not found")
			return
		}
		err = withCurrentDeliveryApplicants([]projectfunds.DeliveryApplicant{in.Recipient}, actor.UserID, "", r.PathValue("id"), catalog, organizationStore, func() error {
			var e error
			out, e = store.RecordDeliveryUpdate(out.ID, r.PathValue("selection_id"), in.Recipient, actor.UserID, in.ExpectedVersion, in.Update)
			return e
		})
		writeFundedOutcome(w, out, err, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/funded-outcomes/{outcome_id}/delivery-selections/{selection_id}/expenses", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		if actor.UserID == "" {
			writeAPIError(w, 401, "authentication_required", "an attributable selected contributor is required")
			return
		}
		var in deliveryExpenseRequest
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a bounded evidence-backed expense is required")
			return
		}
		in.Recipient.SubmittedBy = actor.UserID
		if in.Recipient.Kind == "human" && in.Recipient.ID != actor.UserID {
			writeAPIError(w, 403, "delivery_execution_forbidden", "only the selected human, team member, or approved-agent operator may request an expense")
			return
		}
		out, err := store.GetOutcome(r.PathValue("outcome_id"))
		if err != nil || out.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "funded_outcome_not_found", "funded outcome not found")
			return
		}
		err = withCurrentDeliveryApplicants([]projectfunds.DeliveryApplicant{in.Recipient}, actor.UserID, "", r.PathValue("id"), catalog, organizationStore, func() error {
			var e error
			out, e = store.RequestDeliveryExpense(out.ID, r.PathValue("selection_id"), in.Recipient, actor.UserID, in.ExpectedVersion, in.Expense)
			return e
		})
		writeFundedOutcome(w, out, err, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/funded-outcomes/{outcome_id}/delivery-selections/{selection_id}/expenses/{expense_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in deliveryExpenseDecisionRequest
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an attributed expense decision is required")
			return
		}
		out, err := store.GetOutcome(r.PathValue("outcome_id"))
		if err != nil || out.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "funded_outcome_not_found", "funded outcome not found")
			return
		}
		var recipient *projectfunds.DeliveryApplicant
		for _, selection := range out.DeliverySelections {
			if selection.ID == r.PathValue("selection_id") {
				for _, expense := range selection.Execution.Expenses {
					if expense.ID == r.PathValue("expense_id") {
						recipient = &projectfunds.DeliveryApplicant{Kind: expense.RecipientKind, ID: expense.RecipientID, SubmittedBy: actor.UserID}
					}
				}
			}
		}
		if recipient == nil {
			writeAPIError(w, 404, "delivery_expense_not_found", "delivery expense not found")
			return
		}
		err = withCurrentDeliveryApplicants([]projectfunds.DeliveryApplicant{*recipient}, "", "", r.PathValue("id"), catalog, organizationStore, func() error {
			var e error
			out, e = store.DecideDeliveryExpense(out.ID, r.PathValue("selection_id"), r.PathValue("expense_id"), actor.UserID, in.Decision, in.Reason, in.ExpectedVersion)
			return e
		})
		writeFundedOutcome(w, out, err, 200)
	})
	mux.HandleFunc("POST /repositories/{id}/funded-outcomes/{outcome_id}/delivery-selections/{selection_id}/tasks/{task_id}/reviews", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		if actor.UserID == "" {
			writeAPIError(w, 401, "authentication_required", "an attributable designated reviewer is required")
			return
		}
		var in milestoneReviewRequest
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a versioned milestone decision, rationale, and outcome measures are required")
			return
		}
		out, err := store.GetOutcome(r.PathValue("outcome_id"))
		if err != nil || out.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "funded_outcome_not_found", "funded outcome not found")
			return
		}
		err = catalog.WithCurrentDeliveryAuthority([]string{actor.UserID}, r.PathValue("id"), "", func() error {
			var e error
			out, e = store.ReviewMilestone(out.ID, r.PathValue("selection_id"), r.PathValue("task_id"), actor.UserID, in.ExpectedVersion, in.Review)
			return e
		})
		writeFundedOutcome(w, out, deliveryAuthorityError(err), 200)
	})
	mux.HandleFunc("POST /repositories/{id}/funded-outcomes/{outcome_id}/delivery-selections/{selection_id}/tasks/{task_id}/recoveries", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		if actor.UserID == "" {
			writeAPIError(w, 401, "authentication_required", "an attributable recovery actor is required")
			return
		}
		var in milestoneRecoveryRequest
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a versioned recovery action and retained reason are required")
			return
		}
		out, err := store.GetOutcome(r.PathValue("outcome_id"))
		if err != nil || out.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "funded_outcome_not_found", "funded outcome not found")
			return
		}
		out, err = store.RecoverMilestone(out.ID, r.PathValue("selection_id"), r.PathValue("task_id"), actor.UserID, in.Action, in.Reason, in.ExpectedVersion)
		writeFundedOutcome(w, out, err, 200)
	})
	mux.HandleFunc("POST /repositories/{id}/funded-outcomes/{outcome_id}/delivery-selections/{selection_id}/controls", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in deliveryControlRequest
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a bounded attributed delivery control is required")
			return
		}
		out, err := store.GetOutcome(r.PathValue("outcome_id"))
		if err != nil || out.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "funded_outcome_not_found", "funded outcome not found")
			return
		}
		if in.Replacement != nil {
			in.Replacement.SubmittedBy = actor.UserID
			err = withCurrentDeliveryApplicants([]projectfunds.DeliveryApplicant{*in.Replacement}, actor.UserID, "", r.PathValue("id"), catalog, organizationStore, func() error {
				var e error
				out, e = store.ControlDelivery(out.ID, r.PathValue("selection_id"), actor.UserID, in.Action, in.Reason, in.Amount, in.Replacement, in.ExpectedVersion)
				return e
			})
		} else {
			out, err = store.ControlDelivery(out.ID, r.PathValue("selection_id"), actor.UserID, in.Action, in.Reason, in.Amount, nil, in.ExpectedVersion)
		}
		writeFundedOutcome(w, out, err, 200)
	})
}

func deliveryProposalPosition(out projectfunds.FundedOutcome, id string) int {
	for i := range out.DeliveryProposals {
		if out.DeliveryProposals[i].ID == id {
			return i
		}
	}
	return -1
}
func withCurrentDeliveryApplicants(applicants []projectfunds.DeliveryApplicant, controller, requiredParticipant, repositoryID string, catalog *repositories.Store, orgs *organizations.Store, fn func() error) error {
	repo, err := catalog.GetByID(repositoryID)
	if err != nil || fn == nil {
		return projectfunds.ErrForbidden
	}
	humans := []string{}
	if requiredParticipant != "" {
		humans = append(humans, requiredParticipant)
	}
	requirements := []organizations.DeliveryPrincipalRequirement{}
	for _, applicant := range applicants {
		switch applicant.Kind {
		case "human":
			humans = append(humans, applicant.ID)
		case "team", "approved_agent":
			if repo.OrganizationID == "" || orgs == nil || (applicant.OrganizationID != "" && applicant.OrganizationID != repo.OrganizationID) {
				return projectfunds.ErrForbidden
			}
			requirements = append(requirements, organizations.DeliveryPrincipalRequirement{Kind: applicant.Kind, ID: applicant.ID, ControllerID: controller})
		default:
			return projectfunds.ErrInvalid
		}
	}
	commit := func() error {
		return catalog.WithCurrentDeliveryAuthority(humans, repositoryID, repo.OrganizationID, fn)
	}
	if len(requirements) == 0 {
		return deliveryAuthorityError(commit())
	}
	return deliveryAuthorityError(orgs.WithCurrentDeliveryPrincipals(repo.OrganizationID, requirements, commit))
}

func deliveryAuthorityError(err error) error {
	if err == nil || errors.Is(err, projectfunds.ErrInvalid) || errors.Is(err, projectfunds.ErrConflict) || errors.Is(err, projectfunds.ErrForbidden) || errors.Is(err, projectfunds.ErrNotFound) {
		return err
	}
	return projectfunds.ErrForbidden
}

func writeFundedOutcome(w http.ResponseWriter, out projectfunds.FundedOutcome, err error, status int) {
	switch {
	case err == nil:
		writeJSON(w, status, out)
	case errors.Is(err, projectfunds.ErrInvalid):
		writeAPIError(w, 400, "invalid_outcome_funding", "outcome funding terms or action are invalid")
	case errors.Is(err, projectfunds.ErrConflict):
		writeAPIError(w, 409, "outcome_funding_conflict", "the funding contract or pledge already changed")
	case errors.Is(err, projectfunds.ErrForbidden):
		writeAPIError(w, 403, "outcome_funding_forbidden", "the actor is not authorized for this funding action")
	case errors.Is(err, projectfunds.ErrNotFound):
		writeAPIError(w, 404, "funded_outcome_not_found", "funded outcome or project fund not found")
	default:
		writeAPIError(w, 500, "outcome_funding_unavailable", "outcome funding could not be persisted")
	}
}
