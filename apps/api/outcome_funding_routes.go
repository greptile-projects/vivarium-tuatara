package main

import (
	"errors"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
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

func registerOutcomeFundingRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, store *projectfunds.Store) {
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
		writeAPIError(w, 403, "outcome_funding_forbidden", "only the backing contributor may change this pledge")
	case errors.Is(err, projectfunds.ErrNotFound):
		writeAPIError(w, 404, "funded_outcome_not_found", "funded outcome or project fund not found")
	default:
		writeAPIError(w, 500, "outcome_funding_unavailable", "outcome funding could not be persisted")
	}
}
