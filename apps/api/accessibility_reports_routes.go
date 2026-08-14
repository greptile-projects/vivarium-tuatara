package main

import (
	"errors"
	"log"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/accessibilityreports"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

func registerAccessibilityReportRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, reports *accessibilityreports.Store) {
	authorize := func(w http.ResponseWriter, r *http.Request) (auth.Credential, bool, bool) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return actor, false, false
		}
		if actor.UserID == "" {
			writeAuthenticationRequired(w, false)
			return actor, false, false
		}
		repo, err := catalog.GetByID(r.PathValue("id"))
		if err != nil {
			writeRepositoryError(w, err)
			return actor, false, false
		}
		participant := actor.UserID == repo.OwnerID
		if !participant {
			participant, _ = catalog.HasCollaborator(actor.UserID, repo.ID)
		}
		return actor, participant, true
	}
	mux.HandleFunc("POST /repositories/{id}/accessibility-reports", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorize(w, r)
		if !ok {
			return
		}
		var in accessibilityreports.Report
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete accessibility barrier report is required")
			return
		}
		out, err := reports.Create(r.PathValue("id"), actor.UserID, in)
		writeAccessibilityReport(w, out, err, 201)
	})
	mux.HandleFunc("GET /repositories/{id}/accessibility-reports", func(w http.ResponseWriter, r *http.Request) {
		actor, participant, ok := authorize(w, r)
		if !ok {
			return
		}
		values, err := reports.List(r.PathValue("id"))
		if err != nil {
			writeAccessibilityReport(w, accessibilityreports.Report{}, err, 0)
			return
		}
		for i := range values {
			values[i] = accessibilityreports.Project(values[i], actor.UserID, participant)
		}
		writeJSON(w, 200, map[string]any{"reports": values})
	})
	mux.HandleFunc("GET /repositories/{id}/accessibility-reports/{report_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, participant, ok := authorize(w, r)
		if !ok {
			return
		}
		out, err := reports.Get(r.PathValue("id"), r.PathValue("report_id"))
		if err != nil || out.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "accessibility_report_not_found", "accessibility report not found")
			return
		}
		writeJSON(w, 200, accessibilityreports.Project(out, actor.UserID, participant))
	})
	mux.HandleFunc("POST /repositories/{id}/accessibility-reports/{report_id}/attempts", func(w http.ResponseWriter, r *http.Request) {
		actor, participant, ok := authorize(w, r)
		if !ok {
			return
		}
		if !participant {
			writeAPIError(w, 403, "accessibility_attempt_forbidden", "only a current repository participant may run a reproduction scenario")
			return
		}
		current, err := reports.Get(r.PathValue("id"), r.PathValue("report_id"))
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "accessibility_report_not_found", "accessibility report not found")
			return
		}
		var in accessibilityreports.Attempt
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a bounded environment and classified result are required")
			return
		}
		out, err := reports.AddAttempt(current.RepositoryID, current.ID, actor.UserID, in)
		writeAccessibilityReport(w, accessibilityreports.Project(out, actor.UserID, true), err, 201)
	})
}

func writeAccessibilityReport(w http.ResponseWriter, out accessibilityreports.Report, err error, status int) {
	switch {
	case err == nil:
		writeJSON(w, status, out)
	case errors.Is(err, accessibilityreports.ErrInvalid):
		writeAPIError(w, 400, "invalid_accessibility_report", "targets require an exact revision, access needs, outcome, steps, redacted bounded evidence, and valid reproduction settings")
	case errors.Is(err, accessibilityreports.ErrNotFound):
		writeAPIError(w, 404, "accessibility_report_not_found", "accessibility report not found")
	default:
		log.Printf("accessibility report storage: %v", err)
		writeAPIError(w, 500, "accessibility_reports_unavailable", "accessibility reports could not be persisted")
	}
}
