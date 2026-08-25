package main

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/contributoropportunities"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/issues"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/learningassessments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/learningpathways"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workspaces"
)

func registerContributorOpportunityRoutes(mux *http.ServeMux, git *storage.Store, repos *repositories.Store, opportunities *contributoropportunities.Store, issueStore *issues.Store, proposalStore *proposals.Store, pulls *pullrequests.Store, releaseStore *releases.Store, assessments *learningassessments.Store, learning *learningpathways.Store, workspaceStore *workspaces.Store, credentials *auth.Store) {
	read := func(w http.ResponseWriter, r *http.Request) (auth.Credential, bool) {
		actor, _, ok := authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id"))
		return actor, ok
	}
	mux.HandleFunc("GET /repositories/{id}/contribution-opportunities", func(w http.ResponseWriter, r *http.Request) {
		if _, ok := read(w, r); !ok {
			return
		}
		items, err := opportunities.List(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "opportunities_read_failed", "contribution opportunities could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"opportunities": items})
	})
	mux.HandleFunc("POST /repositories/{id}/contribution-opportunity-matches", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := read(w, r)
		if !ok {
			return
		}
		var input struct {
			contributoropportunities.Profile
			LearningAttemptIDs []string `json:"learning_attempt_ids"`
		}
		if decodeJSON(r, &input) != nil || input.AvailableMinutes < 15 || input.AvailableMinutes > 10080 || input.MaximumRisk != "low" && input.MaximumRisk != "medium" && input.MaximumRisk != "high" || len(input.LearningAttemptIDs) > 20 {
			writeAPIError(w, 422, "invalid_match_profile", "skills, interests, available minutes, and maximum risk must describe realistic constraints")
			return
		}
		learningEvidence := []workspaces.ContributionLearningEvidence{}
		for _, attemptID := range input.LearningAttemptIDs {
			evidence, skills, err := resolveContributionLearningEvidence(r.PathValue("id"), actor.UserID, attemptID, assessments, learning, workspaceStore)
			if err != nil {
				writeAPIError(w, 422, "learning_evidence_ineligible", "only the learner's current demonstrated module evidence can inform a match")
				return
			}
			learningEvidence = append(learningEvidence, evidence)
			for _, skill := range skills {
				if !slices.Contains(input.Skills, skill) {
					input.Skills = append(input.Skills, skill)
				}
			}
		}
		items, err := opportunities.List(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "opportunities_read_failed", "contribution opportunities could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"matches": contributoropportunities.MatchAll(items, input.Profile, time.Now()), "learning_evidence": learningEvidence})
	})
	mux.HandleFunc("PUT /repositories/{id}/contribution-opportunities/{opportunity}", func(w http.ResponseWriter, r *http.Request) {
		actor, owner, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if !owner {
			writeAPIError(w, 403, "owner_required", "only the repository owner can publish contribution opportunities")
			return
		}
		var input struct {
			ExpectedVersion int                                  `json:"expected_version"`
			Opportunity     contributoropportunities.Opportunity `json:"opportunity"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		input.Opportunity.RepositoryID = r.PathValue("id")
		input.Opportunity.PublishedBy = actor.UserID
		pathID := r.PathValue("opportunity")
		if pathID != "new" {
			input.Opportunity.ID = pathID
		} else {
			input.Opportunity.ID = ""
		}
		// A published opportunity must remain grounded in a live collaboration record.
		sourceOK := false
		switch input.Opportunity.Source.Kind {
		case "issue":
			if issueStore != nil {
				v, e := issueStore.Get(r.PathValue("id"), input.Opportunity.Source.ID)
				sourceOK = e == nil && v.Triage.Classification != ""
			}
		case "proposal":
			if proposalStore != nil {
				_, e := proposalStore.Get(r.PathValue("id"), input.Opportunity.Source.ID)
				sourceOK = e == nil
			}
		case "task":
			if proposalStore != nil && input.Opportunity.Source.ParentID != "" {
				_, e := proposalStore.GetTask(r.PathValue("id"), input.Opportunity.Source.ParentID, input.Opportunity.Source.ID)
				sourceOK = e == nil
			}
		case "stewardship":
			sourceOK = input.Opportunity.Source.ID != ""
		}
		if !sourceOK {
			writeAPIError(w, 422, "invalid_opportunity_source", "the source must resolve to a triaged issue, proposal, planned task, or stewardship finding")
			return
		}
		v, err := opportunities.Publish(input.Opportunity, input.ExpectedVersion)
		if errors.Is(err, contributoropportunities.ErrConflict) {
			writeAPIError(w, 409, "opportunity_changed", "contribution opportunity changed")
			return
		}
		if errors.Is(err, contributoropportunities.ErrInvalid) || errors.Is(err, contributoropportunities.ErrNotFound) {
			writeAPIError(w, 422, "invalid_opportunity", "bounded outcome, skills, scope, risk, revision, source, and estimate are required")
			return
		}
		if err != nil {
			writeAPIError(w, 500, "opportunity_write_failed", "contribution opportunity could not be retained")
			return
		}
		writeJSON(w, 201, v)
	})
	mux.HandleFunc("POST /repositories/{id}/contribution-opportunities/{opportunity}/claim", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		if _, _, ok = authorizeRepositoryRead(w, r, repos, credentials, r.PathValue("id")); !ok {
			return
		}
		var input struct {
			ExpectedVersion int    `json:"expected_version"`
			Hours           int    `json:"hours"`
			Note            string `json:"note"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		v, err := opportunities.Claim(r.PathValue("id"), r.PathValue("opportunity"), actor.UserID, input.Note, time.Duration(input.Hours)*time.Hour, input.ExpectedVersion)
		opportunityResult(w, v, err, true)
	})
	mux.HandleFunc("POST /repositories/{id}/contribution-opportunities/{opportunity}/release", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		repo, err := repos.GetByID(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return
		}
		var input struct {
			ExpectedVersion int `json:"expected_version"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		var v contributoropportunities.Opportunity
		var releaseErr error
		err = repos.WithCurrentReadAccess(actor.UserID, []string{repo.ID}, func() error {
			v, releaseErr = opportunities.Release(repo.ID, r.PathValue("opportunity"), actor.UserID, actor.UserID == repo.OwnerID, input.ExpectedVersion)
			return releaseErr
		})
		if errors.Is(err, repositories.ErrNotFound) || errors.Is(err, repositories.ErrInvalidCollaborator) {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return
		}
		opportunityResult(w, v, err, false)
	})
	mux.HandleFunc("POST /repositories/{id}/contribution-opportunities/{opportunity}/completion", func(w http.ResponseWriter, r *http.Request) {
		actor, owner, ok := authorizeRepositoryParticipant(w, r, repos, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if !owner {
			writeAPIError(w, 403, "owner_required", "only the repository owner can record a delivered contribution")
			return
		}
		var input struct {
			ExpectedVersion   int      `json:"expected_version"`
			PullRequestID     string   `json:"pull_request_id"`
			ReleaseID         string   `json:"release_id"`
			Feedback          string   `json:"feedback"`
			Credit            []string `json:"credit"`
			ReadyForNext      bool     `json:"ready_for_next"`
			SkillsRecognized  []string `json:"skills_recognized"`
			NextOpportunityID string   `json:"next_opportunity_id"`
			ReadinessNote     string   `json:"readiness_note"`
		}
		if decodeJSON(r, &input) != nil {
			writeAPIError(w, 400, "invalid_json", "request body must be valid JSON")
			return
		}
		pull, pullErr := pulls.Get(r.PathValue("id"), input.PullRequestID)
		release, releaseErr := releaseStore.Get(r.PathValue("id"), input.ReleaseID)
		containsMerge := pullErr == nil && releaseErr == nil && pull.MergeCommitID != nil && releaseContainsCommit(git, r.PathValue("id"), release.CommitID, *pull.MergeCommitID)
		if pullErr != nil || releaseErr != nil || !validContributionDelivery(r.PathValue("opportunity"), input.ExpectedVersion, pull, release, containsMerge) {
			writeAPIError(w, 422, "contribution_delivery_invalid", "completion requires the exact merged guided pull and a release that credits its contributor")
			return
		}
		if input.NextOpportunityID != "" {
			next, nextErr := opportunities.Get(r.PathValue("id"), input.NextOpportunityID)
			if nextErr != nil || next.Status != "open" {
				writeAPIError(w, 422, "next_opportunity_invalid", "the recommended next opportunity must be currently open")
				return
			}
		}
		completion := contributoropportunities.Completion{
			ContributorID: pull.AuthorID, PullRequestID: pull.ID, ReleaseID: release.ID, ReleaseVersion: release.Version, MergeCommitID: *pull.MergeCommitID,
			Credit: cleanContributionText(input.Credit), Feedback: strings.TrimSpace(input.Feedback), RecordedBy: actor.UserID,
			SupportEffort: contributoropportunities.SupportEffort{SetupAttempts: len(pull.ContributionEvidence.SetupEvidence), MentorGuidanceItems: len(pull.ContributionEvidence.MentorGuidanceIDs), AgentAssistanceItems: len(pull.ContributionEvidence.AgentAssistanceIDs)},
			Readiness:     contributoropportunities.Readiness{ReadyForNext: input.ReadyForNext, SkillsRecognized: cleanContributionText(input.SkillsRecognized), NextOpportunityID: input.NextOpportunityID, Note: strings.TrimSpace(input.ReadinessNote)},
		}
		v, err := opportunities.Complete(r.PathValue("id"), r.PathValue("opportunity"), input.ExpectedVersion, completion)
		if errors.Is(err, contributoropportunities.ErrConflict) {
			writeAPIError(w, 409, "opportunity_changed", "contribution opportunity changed")
			return
		}
		if errors.Is(err, contributoropportunities.ErrInvalid) {
			writeAPIError(w, 422, "contribution_completion_invalid", "credit, feedback, and a bounded readiness assessment are required")
			return
		}
		if err != nil {
			writeAPIError(w, 500, "opportunity_write_failed", "contribution completion could not be retained")
			return
		}
		writeJSON(w, 200, v)
	})
}

func validContributionDelivery(opportunityID string, version int, pull pullrequests.PullRequest, release releases.Candidate, releaseContainsMerge bool) bool {
	return pull.Status == pullrequests.Merged && pull.MergeCommitID != nil && pull.ContributionEvidence != nil &&
		pull.ContributionEvidence.OpportunityID == opportunityID && pull.ContributionEvidence.OpportunityVersion == version &&
		releaseContainsMerge && slices.Contains(release.Inclusions.PullRequestIDs, pull.ID) && slices.Contains(release.Inclusions.ContributorIDs, pull.AuthorID)
}

func releaseContainsCommit(git *storage.Store, repositoryID, releaseCommitID, includedCommitID string) bool {
	repository, err := git.Open(repositoryID)
	if err != nil {
		return false
	}
	ancestry, err := repository.ListCommitAncestry(storage.ObjectID(releaseCommitID))
	if err != nil {
		return false
	}
	return slices.ContainsFunc(ancestry, func(commit storage.Commit) bool { return string(commit.ID) == includedCommitID })
}

func cleanContributionText(values []string) []string {
	out := []string{}
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" && len(value) <= 200 && !slices.Contains(out, value) {
			out = append(out, value)
		}
	}
	return out
}
func opportunityResult(w http.ResponseWriter, v contributoropportunities.Opportunity, err error, created bool) {
	if errors.Is(err, contributoropportunities.ErrConflict) {
		writeAPIError(w, 409, "opportunity_changed", "contribution opportunity changed")
		return
	}
	if errors.Is(err, contributoropportunities.ErrClaimed) {
		writeAPIError(w, 409, "opportunity_claimed", "another contributor currently holds this opportunity")
		return
	}
	if errors.Is(err, contributoropportunities.ErrInvalid) {
		writeAPIError(w, 422, "invalid_opportunity_claim", "claim duration or opportunity state is invalid")
		return
	}
	if errors.Is(err, contributoropportunities.ErrNotFound) {
		writeAPIError(w, 404, "opportunity_not_found", "contribution opportunity not found")
		return
	}
	if err != nil {
		writeAPIError(w, 500, "opportunity_write_failed", "contribution opportunity could not be updated")
		return
	}
	status := 200
	if created {
		status = 201
	}
	writeJSON(w, status, v)
}
