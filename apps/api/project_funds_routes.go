package main

import (
	"errors"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/projectfunds"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

type fundCommitInput struct {
	Amount            int64  `json:"amount"`
	Source            string `json:"source"`
	ExternalReference string `json:"external_reference"`
	IdempotencyKey    string `json:"idempotency_key"`
	Note              string `json:"note"`
}
type fundReconcileInput struct {
	ExpectedVersion int                         `json:"expected_version"`
	Status          string                      `json:"status"`
	CompletedAmount int64                       `json:"completed_amount"`
	Note            string                      `json:"note"`
	TransferProof   *projectfunds.TransferProof `json:"transfer_proof"`
}

func registerProjectFundRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, store *projectfunds.Store) {
	mux.HandleFunc("POST /repositories/{id}/funds", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var terms projectfunds.Terms
		if decodeJSON(r, &terms) != nil {
			writeAPIError(w, 400, "invalid_request", "complete governed fund terms are required")
			return
		}
		var f projectfunds.Fund
		err := catalog.WithCurrentParticipant(actor.UserID, r.PathValue("id"), func() error { var e error; f, e = store.Create(r.PathValue("id"), actor.UserID, terms); return e })
		writeFund(w, f, err, 201)
	})
	mux.HandleFunc("GET /repositories/{id}/funds", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		fs, err := store.List(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "funds_unavailable", "project funds could not be read")
			return
		}
		visible := fs[:0]
		participant := actor.UserID != "" && catalog.WithCurrentParticipant(actor.UserID, r.PathValue("id"), func() error { return nil }) == nil
		for _, f := range fs {
			if f.Terms.LedgerVisibility == "public" || participant {
				visible = append(visible, f)
			}
		}
		writeJSON(w, 200, map[string]any{"funds": visible})
	})
	mux.HandleFunc("GET /repositories/{id}/funds/{fund_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		f, e := store.Get(r.PathValue("fund_id"))
		if e != nil || f.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "fund_not_found", "project fund not found")
			return
		}
		participant := actor.UserID != "" && catalog.WithCurrentParticipant(actor.UserID, r.PathValue("id"), func() error { return nil }) == nil
		if f.Terms.LedgerVisibility == "participants" && !participant {
			writeAPIError(w, 403, "fund_ledger_forbidden", "this ledger is limited to project participants")
			return
		}
		writeJSON(w, 200, f)
	})
	mux.HandleFunc("POST /repositories/{id}/funds/{fund_id}/commitments", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok || actor.UserID == "" {
			if ok {
				writeAPIError(w, 401, "authentication_required", "an authenticated contributor is required")
			}
			return
		}
		f, e := store.Get(r.PathValue("fund_id"))
		if e != nil || f.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "fund_not_found", "project fund not found")
			return
		}
		var in fundCommitInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a valid commitment is required")
			return
		}
		f, e = store.Commit(f.ID, actor.UserID, in.Source, in.ExternalReference, in.Amount, in.IdempotencyKey, in.Note)
		writeFund(w, f, e, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/funds/{fund_id}/commitments/{entry_id}/reconcile", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		f, e := store.Get(r.PathValue("fund_id"))
		if e != nil || f.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "fund_not_found", "project fund not found")
			return
		}
		var in fundReconcileInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a reconciliation decision is required")
			return
		}
		f, e = store.Reconcile(f.ID, r.PathValue("entry_id"), actor.UserID, in.Status, in.CompletedAmount, in.TransferProof, in.Note, in.ExpectedVersion)
		writeFund(w, f, e, 200)
	})
}
func writeFund(w http.ResponseWriter, f projectfunds.Fund, err error, status int) {
	switch {
	case err == nil:
		writeJSON(w, status, f)
	case errors.Is(err, projectfunds.ErrInvalid):
		writeAPIError(w, 400, "invalid_fund", "fund terms or transfer state are invalid")
	case errors.Is(err, projectfunds.ErrConflict):
		writeAPIError(w, 409, "fund_conflict", "the fund or transfer already changed")
	case errors.Is(err, projectfunds.ErrForbidden):
		writeAPIError(w, 403, "fund_steward_required", "only a named current-project steward may reconcile transfers")
	case errors.Is(err, projectfunds.ErrNotFound):
		writeAPIError(w, 404, "fund_not_found", "project fund or commitment not found")
	default:
		writeAPIError(w, 500, "funds_unavailable", "project fund could not be persisted")
	}
}
