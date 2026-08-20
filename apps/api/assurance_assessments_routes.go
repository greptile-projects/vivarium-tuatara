package main

import (
	"errors"
	"net/http"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/assuranceassessments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/assuranceevidence"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/assuranceprograms"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

func registerAssuranceAssessmentRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, people *users.Store, programs *assuranceprograms.Store, evidence *assuranceevidence.Store, assessments *assuranceassessments.Store) {
	mux.HandleFunc("GET /repositories/{id}/assurance-assessments", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		values, err := assessments.List(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "assessments_unavailable", "assessments could not be read")
			return
		}
		out := values[:0]
		now := time.Now().UTC()
		for _, a := range values {
			if a.OwnerID == actor.UserID || (a.Assessor.UserID == actor.UserID && assessorWindowOpen(a, now)) {
				out = append(out, a)
			}
		}
		writeJSON(w, 200, map[string]any{"assessments": out})
	})
	mux.HandleFunc("GET /repositories/{id}/assurance-assessments/{assessment_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		a, err := assessments.Get(r.PathValue("assessment_id"))
		if err != nil || a.RepositoryID != r.PathValue("id") || !assessmentParty(a, actor.UserID) {
			writeAPIError(w, 404, "assessment_not_found", "assessment not found")
			return
		}
		if actor.UserID == a.Assessor.UserID && !writeAssessorWindowError(w, a, time.Now().UTC()) {
			return
		}
		packages := []assuranceevidence.Package{}
		for _, id := range a.EvidencePackageIDs {
			p, e := evidence.GetPackage(id)
			if e == nil && p.RepositoryID == a.RepositoryID {
				packages = append(packages, p)
			}
		}
		writeJSON(w, 200, map[string]any{"assessment": a, "evidence_packages": packages, "authority_note": "assessment access grants no repository, source-system, production, review, release, deployment, or project mutation authority"})
	})
	mux.HandleFunc("POST /repositories/{id}/assurance-assessments", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in assuranceassessments.Assessment
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete bounded assessment is required")
			return
		}
		in.RepositoryID = r.PathValue("id")
		in.OwnerID = actor.UserID
		program, err := programs.Get(in.ProgramID)
		if err != nil || program.RepositoryID != in.RepositoryID || in.ProgramVersion < 1 || in.ProgramVersion > len(program.Revisions) {
			writeAPIError(w, 400, "invalid_program_revision", "an exact repository assurance program revision is required")
			return
		}
		revision := program.Revisions[in.ProgramVersion-1]
		if !hasID(revision.OwnerIDs, actor.UserID) {
			writeAPIError(w, 403, "program_owner_required", "only a named program owner may open an independent assessment")
			return
		}
		if _, err = people.Get(in.Assessor.UserID); err != nil {
			writeAPIError(w, 400, "invalid_assessor", "the assessor must have an identified platform account")
			return
		}
		if !assessmentScopeValid(revision, in.Scope) {
			writeAPIError(w, 400, "invalid_assessment_scope", "controls, systems, and releases must be selected from the exact program revision")
			return
		}
		for _, id := range in.EvidencePackageIDs {
			p, e := evidence.GetPackage(id)
			if e != nil || p.RepositoryID != in.RepositoryID || p.ProgramID != in.ProgramID || p.ProgramVersion != in.ProgramVersion || !hasID(in.Scope.ControlIDs, p.ControlID) || p.PeriodStartsAt.Before(in.Scope.PeriodStartsAt) || p.PeriodEndsAt.After(in.Scope.PeriodEndsAt) {
				writeAPIError(w, 400, "invalid_assessment_evidence", "evidence must be explicitly selected from the exact program, controls, and assessment period")
				return
			}
		}
		out, err := assessments.Create(in)
		writeAssessment(w, out, err, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/assurance-assessments/{assessment_id}/events", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		a, err := assessments.Get(r.PathValue("assessment_id"))
		if err != nil || a.RepositoryID != r.PathValue("id") || !assessmentParty(a, actor.UserID) {
			writeAPIError(w, 404, "assessment_not_found", "assessment not found")
			return
		}
		role := "assessor"
		if actor.UserID == a.OwnerID {
			role = "owner"
		}
		var in struct {
			ExpectedVersion int `json:"expected_version"`
			assuranceassessments.Event
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete attributable assessment event is required")
			return
		}
		out, err := assessments.Append(a.ID, in.ExpectedVersion, actor.UserID, role, in.Event)
		writeAssessment(w, out, err, 201)
	})
}
func assessmentParty(a assuranceassessments.Assessment, id string) bool {
	return a.OwnerID == id || a.Assessor.UserID == id
}
func assessorWindowOpen(a assuranceassessments.Assessment, now time.Time) bool {
	return !now.Before(a.StartsAt) && now.Before(a.ExpiresAt)
}
func writeAssessorWindowError(w http.ResponseWriter, a assuranceassessments.Assessment, now time.Time) bool {
	if now.Before(a.StartsAt) {
		writeAPIError(w, 403, "assessment_access_not_started", "the assessor's bounded evidence access has not started")
		return false
	}
	if !now.Before(a.ExpiresAt) {
		writeAPIError(w, 403, "assessment_access_expired", "the assessor's bounded evidence access has expired")
		return false
	}
	return true
}
func hasID(xs []string, id string) bool {
	for _, x := range xs {
		if x == id {
			return true
		}
	}
	return false
}
func assessmentScopeValid(r assuranceprograms.Revision, s assuranceassessments.Scope) bool {
	for _, id := range s.ControlIDs {
		found := false
		for _, c := range r.Controls {
			if c.ID == id {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	for _, id := range s.SystemIDs {
		found := false
		for _, x := range r.Scopes {
			if x.ID == id && (x.Kind == "repository" || x.Kind == "data_flow" || x.Kind == "infrastructure" || x.Kind == "environment") {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	for _, id := range s.ReleaseIDs {
		found := false
		for _, x := range r.Scopes {
			if x.ID == id && x.Kind == "release" {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}
func writeAssessment(w http.ResponseWriter, a assuranceassessments.Assessment, err error, status int) {
	if err == nil {
		writeJSON(w, status, a)
		return
	}
	switch {
	case errors.Is(err, assuranceassessments.ErrConflict):
		writeAPIError(w, 409, "assessment_version_conflict", "the assessment changed; reload before appending")
	case errors.Is(err, assuranceassessments.ErrExpired):
		writeAPIError(w, 403, "assessment_access_expired", "bounded assessment access has expired")
	case errors.Is(err, assuranceassessments.ErrNotStarted):
		writeAPIError(w, 403, "assessment_access_not_started", "bounded assessment access has not started")
	case errors.Is(err, assuranceassessments.ErrForbidden):
		writeAPIError(w, 403, "assessment_action_forbidden", "this party cannot perform that assessment action")
	default:
		writeAPIError(w, 400, "invalid_assessment", "the assessment record is invalid")
	}
}
