package main

import (
	"errors"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/accessibilityassessments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

func registerAccessibilityAssessmentRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, pulls *pullrequests.Store, assessments *accessibilityassessments.Store) {
	authorize := func(w http.ResponseWriter, r *http.Request) (auth.Credential, bool, bool) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return actor, false, false
		}
		if actor.UserID == "" && actor.AgentID == "" {
			writeAuthenticationRequired(w, false)
			return actor, false, false
		}
		repo, err := catalog.GetByID(r.PathValue("id"))
		if err != nil {
			writeRepositoryError(w, err)
			return actor, false, false
		}
		participant := actor.UserID != "" && actor.AgentID == "" && actor.UserID == repo.OwnerID
		if !participant && actor.UserID != "" && actor.AgentID == "" {
			participant, _ = catalog.HasCollaborator(actor.UserID, repo.ID)
		}
		return actor, participant, true
	}
	mux.HandleFunc("POST /repositories/{id}/accessibility-assessments", func(w http.ResponseWriter, r *http.Request) {
		actor, participant, ok := authorize(w, r)
		if !ok {
			return
		}
		if !participant {
			writeAPIError(w, 403, "accessibility_assessment_forbidden", "only a current repository participant may publish repository-defined checks")
			return
		}
		var in accessibilityassessments.Assessment
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a revision and complete accessibility checks are required")
			return
		}
		if in.PullRequestID != "" {
			if pulls == nil {
				writeAPIError(w, 503, "pull_requests_unavailable", "pull request evidence could not be verified")
				return
			}
			pull, err := pulls.Get(r.PathValue("id"), in.PullRequestID)
			if err != nil || pull.SourceCommitID != in.Revision {
				writeAPIError(w, 400, "invalid_accessibility_pull_revision", "the pull request must exist in this repository at the assessment revision")
				return
			}
		}
		out, err := assessments.Create(r.PathValue("id"), actor.UserID, in)
		writeAccessibilityAssessment(w, out, err, 201)
	})
	mux.HandleFunc("GET /repositories/{id}/accessibility-assessments", func(w http.ResponseWriter, r *http.Request) {
		_, _, ok := authorize(w, r)
		if !ok {
			return
		}
		out, err := assessments.List(r.PathValue("id"), r.URL.Query().Get("revision"), r.URL.Query().Get("pull_request_id"))
		if err != nil {
			writeAccessibilityAssessment(w, accessibilityassessments.Assessment{}, err, 0)
			return
		}
		writeJSON(w, 200, map[string]any{"assessments": out})
	})
	mux.HandleFunc("POST /repositories/{id}/accessibility-assessments/{assessment_id}/findings", func(w http.ResponseWriter, r *http.Request) {
		actor, participant, ok := authorize(w, r)
		if !ok {
			return
		}
		if !participant && actor.AgentID == "" {
			writeAPIError(w, 403, "accessibility_finding_forbidden", "only repository participants and repository-bound read-only agents may add findings")
			return
		}
		var in accessibilityassessments.Finding
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a cited accessibility finding is required")
			return
		}
		kind, id := "human", actor.UserID
		if actor.AgentID != "" {
			kind, id = "agent", actor.AgentID
		}
		out, err := assessments.AddFinding(r.PathValue("id"), r.PathValue("assessment_id"), kind, id, in)
		writeAccessibilityAssessment(w, out, err, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/accessibility-assessments/{assessment_id}/findings/{finding_id}/decision", func(w http.ResponseWriter, r *http.Request) {
		actor, participant, ok := authorize(w, r)
		if !ok {
			return
		}
		if !participant {
			writeAPIError(w, 403, "accessibility_decision_forbidden", "only a current human repository participant may decide a finding")
			return
		}
		var in struct {
			Classification string `json:"classification"`
			Reason         string `json:"reason"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a classification and reason are required")
			return
		}
		out, err := assessments.Decide(r.PathValue("id"), r.PathValue("assessment_id"), r.PathValue("finding_id"), actor.UserID, in.Classification, in.Reason)
		writeAccessibilityAssessment(w, out, err, 200)
	})
	mux.HandleFunc("POST /repositories/{id}/accessibility-assessments/{assessment_id}/invalidate", func(w http.ResponseWriter, r *http.Request) {
		actor, participant, ok := authorize(w, r)
		if !ok {
			return
		}
		if !participant {
			writeAPIError(w, 403, "accessibility_invalidation_forbidden", "only a current repository participant may invalidate affected evidence")
			return
		}
		var in struct {
			SourceLocations []string `json:"source_locations"`
			JourneyIDs      []string `json:"journey_ids"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "changed source locations or journeys are required")
			return
		}
		out, err := assessments.Invalidate(r.PathValue("id"), r.PathValue("assessment_id"), actor.UserID, in.SourceLocations, in.JourneyIDs)
		writeAccessibilityAssessment(w, out, err, 200)
	})
}

func writeAccessibilityAssessment(w http.ResponseWriter, out accessibilityassessments.Assessment, err error, status int) {
	switch {
	case err == nil:
		writeJSON(w, status, out)
	case errors.Is(err, accessibilityassessments.ErrNotFound):
		writeAPIError(w, 404, "accessibility_assessment_not_found", "accessibility assessment not found")
	case errors.Is(err, accessibilityassessments.ErrInvalid):
		writeAPIError(w, 400, "invalid_accessibility_assessment", "checks and findings must be bounded, revision-exact, cited, and use supported classifications")
	default:
		writeAPIError(w, 500, "accessibility_assessments_unavailable", "accessibility assessments could not be persisted")
	}
}
