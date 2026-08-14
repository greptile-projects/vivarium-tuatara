package main

import (
	"errors"
	"net/http"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/accessibilityassessments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/accessibilityreports"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/previews"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

func registerAccessibilityAssessmentRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, pulls *pullrequests.Store, previewStore *previews.Store, reportStore *accessibilityreports.Store, assessments *accessibilityassessments.Store) {
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
			var out accessibilityassessments.Assessment
			err := pulls.WithSourceRevision(r.PathValue("id"), in.PullRequestID, in.Revision, func(_ pullrequests.PullRequest) error {
				var createErr error
				out, createErr = assessments.Create(r.PathValue("id"), actor.UserID, in)
				return createErr
			})
			if errors.Is(err, pullrequests.ErrSourceChanged) || errors.Is(err, pullrequests.ErrNotReady) {
				writeAPIError(w, 409, "accessibility_pull_changed", "the pull request changed while accessibility evidence was published")
				return
			}
			if errors.Is(err, pullrequests.ErrNotFound) || errors.Is(err, pullrequests.ErrInvalid) {
				writeAPIError(w, 400, "invalid_accessibility_pull_revision", "the pull request must exist in this repository at the assessment revision")
				return
			}
			writeAccessibilityAssessment(w, out, err, 201)
			return
		} else if !accessibilityRevisionIsVisible(git, r.PathValue("id"), in.Revision) {
			writeAPIError(w, 400, "invalid_accessibility_revision", "the assessment revision must be a commit reachable from a visible repository branch")
			return
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
		assessment, err := assessments.Get(r.PathValue("id"), r.PathValue("assessment_id"))
		if err != nil {
			writeAccessibilityAssessment(w, accessibilityassessments.Assessment{}, err, 0)
			return
		}
		kind, id := "human", actor.UserID
		if actor.AgentID != "" {
			kind, id = "agent", actor.AgentID
		}
		guardPullID := assessment.PullRequestID
		for _, citation := range in.Citations {
			if citation.Kind != "preview" || previewStore == nil {
				continue
			}
			preview, previewErr := previewStore.Find(r.PathValue("id"), citation.ResourceID)
			if previewErr != nil || (guardPullID != "" && guardPullID != preview.PullRequestID) {
				writeAPIError(w, 400, "invalid_accessibility_citation", "preview citations must resolve to the assessment's single guarded pull request")
				return
			}
			guardPullID = preview.PullRequestID
		}
		var out accessibilityassessments.Assessment
		persist := func(current *pullrequests.PullRequest) error {
			for _, citation := range in.Citations {
				if !accessibilityCitationResolves(r.PathValue("id"), assessment.Revision, citation, current, previewStore, reportStore) {
					return accessibilityassessments.ErrInvalid
				}
			}
			var addErr error
			out, addErr = assessments.AddFinding(r.PathValue("id"), r.PathValue("assessment_id"), kind, id, in)
			return addErr
		}
		if guardPullID != "" {
			if pulls == nil {
				writeAPIError(w, 503, "pull_requests_unavailable", "preview evidence freshness could not be verified")
				return
			}
			err = pulls.WithSourceRevision(r.PathValue("id"), guardPullID, assessment.Revision, func(current pullrequests.PullRequest) error { return persist(&current) })
		} else {
			err = persist(nil)
		}
		if errors.Is(err, pullrequests.ErrSourceChanged) || errors.Is(err, pullrequests.ErrNotReady) {
			writeAPIError(w, 409, "accessibility_citation_stale", "the pull request changed while cited accessibility evidence was published")
			return
		}
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

func accessibilityRevisionIsVisible(git *storage.Store, repositoryID, revision string) bool {
	if git == nil || len(revision) != 40 || revision != strings.ToLower(revision) {
		return false
	}
	repository, err := git.Open(repositoryID)
	if err != nil {
		return false
	}
	if _, err = repository.ReadCommit(storage.ObjectID(revision)); err != nil {
		return false
	}
	refs, err := repository.ListReferences()
	if err != nil {
		return false
	}
	for _, ref := range refs {
		if !strings.HasPrefix(ref.Name, "refs/heads/") || strings.HasPrefix(ref.Name, "refs/heads/vivarium-security/") {
			continue
		}
		ancestry, ancestryErr := repository.ListCommitAncestry(storage.ObjectID(ref.Target))
		if ancestryErr != nil {
			continue
		}
		for _, commit := range ancestry {
			if string(commit.ID) == revision {
				return true
			}
		}
	}
	return false
}

func accessibilityCitationResolves(repositoryID, revision string, citation accessibilityassessments.Citation, currentPull *pullrequests.PullRequest, previewStore *previews.Store, reportStore *accessibilityreports.Store) bool {
	switch citation.Kind {
	case "preview":
		if previewStore == nil {
			return false
		}
		preview, err := previewStore.Find(repositoryID, citation.ResourceID)
		if err != nil || preview.Revision != revision || currentPull == nil || currentPull.ID != preview.PullRequestID {
			return false
		}
		if !accessibilityPreviewMatchesCurrentRevision(preview, currentPull.SourceCommitID) {
			return false
		}
		return accessibilityPreviewArtifactResolves(preview, citation.EvidenceRef)
	case "reproduction":
		if reportStore == nil {
			return false
		}
		reports, err := reportStore.List(repositoryID)
		if err != nil {
			return false
		}
		for _, report := range reports {
			if report.Target.Revision != revision {
				continue
			}
			for _, attempt := range report.Attempts {
				if attempt.ID != citation.ResourceID {
					continue
				}
				for _, evidence := range attempt.Evidence {
					if evidence.ContentRef == citation.EvidenceRef && evidence.Redacted {
						return true
					}
				}
			}
		}
	}
	return false
}

func accessibilityPreviewMatchesCurrentRevision(preview previews.Preview, currentRevision string) bool {
	return currentRevision != "" && preview.Revision == currentRevision
}

func accessibilityPreviewArtifactResolves(preview previews.Preview, evidenceRef string) bool {
	for _, finding := range preview.Findings {
		for _, evidence := range finding.Evidence {
			if evidenceRef == "artifact://"+evidence.ID {
				return true
			}
		}
	}
	return false
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
