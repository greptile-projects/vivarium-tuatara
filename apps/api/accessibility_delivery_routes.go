package main

import (
	"errors"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/accessibilityassessments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/accessibilitydelivery"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/checkruns"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/previews"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

func registerAccessibilityDeliveryRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, pulls *pullrequests.Store, releasesStore *releases.Store, previewStore *previews.Store, checks *checkruns.Store, assessments *accessibilityassessments.Store, delivery *accessibilitydelivery.Store) {
	mux.HandleFunc("POST /repositories/{id}/accessibility-delivery-policies", func(w http.ResponseWriter, r *http.Request) {
		actor, owner, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if !owner {
			writeAPIError(w, 403, "accessibility_policy_forbidden", "only the repository owner may set delivery accessibility policy")
			return
		}
		var in accessibilitydelivery.Policy
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		out, err := delivery.CreatePolicy(r.PathValue("id"), actor.UserID, in)
		writeAccessibilityDelivery(w, out, err, 201)
	})
	mux.HandleFunc("GET /repositories/{id}/accessibility-delivery-policies", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		out, err := delivery.Policies(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "accessibility_delivery_unavailable", "accessibility delivery records could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"policies": out})
	})
	mux.HandleFunc("POST /repositories/{id}/accessibility-evaluations/invitations", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in accessibilitydelivery.Invitation
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		if previewStore == nil {
			writeAPIError(w, 503, "accessibility_preview_unavailable", "revision-exact previews are unavailable")
			return
		}
		preview, err := previewStore.Get(r.PathValue("id"), in.PullRequestID, in.PreviewID)
		if err != nil || !accessibilityPreviewMatchesCurrentRevision(preview, in.Revision) {
			writeAPIError(w, 422, "invalid_accessibility_preview", "invitation must name a preview at the exact candidate revision")
			return
		}
		guestAccess := false
		for _, invitation := range preview.Invitations {
			if invitation.UserID == in.UserID && invitation.RevokedAt == nil && invitation.ExpiresAt.Equal(in.ExpiresAt) {
				guestAccess = true
				break
			}
		}
		if !guestAccess {
			writeAPIError(w, 422, "accessibility_preview_access_missing", "the evaluator must first have bounded guest access to this exact preview for the same expiry")
			return
		}
		out, err := delivery.Invite(r.PathValue("id"), actor.UserID, in)
		writeAccessibilityDelivery(w, out, err, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/accessibility-evaluations/invitations/{invitation_id}/outcome", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		var in struct {
			Decision  string `json:"decision"`
			Rationale string `json:"rationale"`
			Revision  string `json:"revision"`
		}
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		out, err := delivery.Respond(r.PathValue("id"), r.PathValue("invitation_id"), actor.UserID, in.Decision, in.Rationale, in.Revision)
		writeAccessibilityDelivery(w, out, err, 200)
	})
	mux.HandleFunc("POST /repositories/{id}/accessibility-delivery-overrides", func(w http.ResponseWriter, r *http.Request) {
		actor, owner, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if !owner {
			writeAPIError(w, 403, "accessibility_override_forbidden", "only the repository owner may justify an accessibility override")
			return
		}
		var in accessibilitydelivery.Override
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		out, err := delivery.Override(r.PathValue("id"), actor.UserID, in)
		writeAccessibilityDelivery(w, out, err, 201)
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/accessibility-readiness", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		p, err := pulls.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if err != nil {
			writeAPIError(w, 404, "pull_request_not_found", "pull request not found")
			return
		}
		paths := []string{}
		changes, err := pulls.Changes(p.RepositoryID, p.ID)
		if err != nil {
			writeAPIError(w, 500, "accessibility_readiness_unavailable", "accessibility readiness could not be evaluated")
			return
		}
		for _, c := range changes {
			paths = append(paths, c.Path)
		}
		out, err := accessibilityReadiness(delivery, assessments, checks, p.RepositoryID, p.ID, p.ID, p.SourceCommitID, p.TargetBranch, paths, r.URL.Query()["journey"], r.URL.Query()["risk_class"])
		if err != nil {
			writeAPIError(w, 500, "accessibility_readiness_unavailable", "accessibility readiness could not be evaluated")
			return
		}
		writeJSON(w, 200, out)
	})
	mux.HandleFunc("GET /repositories/{id}/releases/{release_id}/accessibility-readiness", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		release, err := releasesStore.Get(r.PathValue("id"), r.PathValue("release_id"))
		if err != nil {
			writeAPIError(w, 404, "release_not_found", "release candidate not found")
			return
		}
		if release.TargetBranch == "" || release.ChangedPaths == nil {
			writeAPIError(w, 500, "accessibility_readiness_unavailable", "release accessibility policy context is unavailable")
			return
		}
		out, err := accessibilityReadiness(delivery, assessments, checks, release.RepositoryID, release.ID, "", release.CommitID, release.TargetBranch, release.ChangedPaths, r.URL.Query()["journey"], r.URL.Query()["risk_class"])
		if err != nil {
			writeAPIError(w, 500, "accessibility_readiness_unavailable", "accessibility readiness could not be evaluated")
			return
		}
		writeJSON(w, 200, out)
	})
}

func accessibilityReadiness(store *accessibilitydelivery.Store, assessments *accessibilityassessments.Store, checks *checkruns.Store, repo, checkContext, assessmentPull, revision, branch string, paths, journeys, risks []string) (accessibilitydelivery.Readiness, error) {
	status := map[string]string{}
	if checks != nil && checkContext != "" {
		runs, err := checks.List(repo, checkContext)
		if err != nil {
			return accessibilitydelivery.Readiness{}, err
		}
		for _, run := range runs {
			if run.CommitID != revision {
				if _, ok := status[run.Definition.Name]; !ok {
					status[run.Definition.Name] = "stale"
				}
				continue
			}
			v := "pending"
			if run.State == "succeeded" {
				v = "passed"
			} else if run.State == "failed" {
				v = "failed"
			}
			status[run.Definition.Name] = v
		}
	}
	evidence, err := assessments.List(repo, "", assessmentPull)
	if err != nil {
		return accessibilitydelivery.Readiness{}, err
	}
	for _, assessment := range evidence {
		for _, check := range assessment.Checks {
			if check.JourneyID != "" {
				journeys = append(journeys, check.JourneyID)
			}
		}
		for _, finding := range assessment.Findings {
			risks = append(risks, finding.Severity)
			journeys = append(journeys, finding.JourneyIDs...)
		}
	}
	releaseID := ""
	if assessmentPull == "" {
		releaseID = checkContext
	}
	return store.Evaluate(repo, revision, branch, assessmentPull, releaseID, paths, journeys, risks, status, evidence)
}
func writeAccessibilityDelivery(w http.ResponseWriter, out any, err error, status int) {
	switch {
	case err == nil:
		writeJSON(w, status, out)
	case errors.Is(err, accessibilitydelivery.ErrNotFound):
		writeAPIError(w, 404, "accessibility_delivery_not_found", "accessibility delivery record not found")
	case errors.Is(err, accessibilitydelivery.ErrConflict):
		writeAPIError(w, 409, "accessibility_evaluation_conflict", "the invitation is stale, expired, already decided, or belongs to another evaluator")
	case errors.Is(err, accessibilitydelivery.ErrInvalid):
		writeAPIError(w, 422, "invalid_accessibility_delivery", "policy, exact preview invitation, outcome, or override is incomplete or inconsistent")
	default:
		writeAPIError(w, 500, "accessibility_delivery_unavailable", "accessibility delivery records could not be persisted")
	}
}
