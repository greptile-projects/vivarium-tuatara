package main

import (
	"encoding/json"
	"errors"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/localeplans"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/localization"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/previews"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"net/http"
)

type extractionInput struct {
	SourceRevision string                     `json:"source_revision"`
	Map            localization.ExtractionMap `json:"map"`
	Locales        []string                   `json:"locales"`
	Units          []localization.Unit        `json:"units"`
}
type translationInput struct {
	SourceRevision string `json:"source_revision"`
	UnitID         string `json:"unit_id"`
	Locale         string `json:"locale"`
	Text           string `json:"text"`
	Note           string `json:"note"`
}
type localizationMutationInput struct {
	SourceRevision  string         `json:"source_revision"`
	ExpectedVersion int            `json:"expected_version"`
	Mutation        string         `json:"mutation"`
	Payload         map[string]any `json:"payload"`
}

var errLocalizationReviewerRequired = errors.New("current locale reviewer required")

func registerLocalizationRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, pulls *pullrequests.Store, plans *localeplans.Store, previewStore *previews.Store, store *localization.Store) {
	pull := func(w http.ResponseWriter, r *http.Request) (pullrequests.PullRequest, bool) {
		p, e := pulls.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if e != nil {
			writeAPIError(w, 404, "pull_not_found", "pull request not found")
			return p, false
		}
		return p, true
	}
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/localization", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		p, ok := pull(w, r)
		if !ok {
			return
		}
		v, e := store.Get(p.RepositoryID, p.ID, p.SourceCommitID)
		if errors.Is(e, localization.ErrNotFound) {
			writeJSON(w, 200, map[string]any{"repository_id": p.RepositoryID, "pull_id": p.ID, "current_revision": p.SourceCommitID, "extractions": []any{}, "translations": []any{}, "counts": map[string]any{}})
			return
		}
		if e != nil {
			writeAPIError(w, 500, "localization_unavailable", "localization review could not be read")
			return
		}
		applyLocalizationPlanVersions(plans, &v)
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/localization/extractions", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		p, ok := pull(w, r)
		if !ok {
			return
		}
		var in extractionInput
		if decodeJSON(r, &in) != nil || in.SourceRevision != p.SourceCommitID {
			writeAPIError(w, 409, "localization_revision_changed", "extraction must match the current pull source revision")
			return
		}
		var v localization.Review
		e := pulls.WithSourceRevision(p.RepositoryID, p.ID, in.SourceRevision, func(current pullrequests.PullRequest) error {
			var storeErr error
			v, storeErr = store.Extract(current.RepositoryID, current.ID, in.SourceRevision, actor.UserID, in.Map, in.Locales, in.Units)
			return storeErr
		})
		if errors.Is(e, pullrequests.ErrSourceChanged) || errors.Is(e, pullrequests.ErrNotReady) {
			writeAPIError(w, 409, "localization_revision_changed", "extraction must match the current open pull source revision")
			return
		}
		if errors.Is(e, localization.ErrInvalid) {
			writeAPIError(w, 400, "invalid_localization_extraction", "a complete repository-defined map and contextual units are required")
			return
		}
		if e != nil {
			writeAPIError(w, 500, "localization_unavailable", "localization extraction could not be persisted")
			return
		}
		writeJSON(w, 201, v)
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/localization/translations", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		if actor.UserID == "" {
			writeAuthenticationRequired(w, false)
			return
		}
		p, ok := pull(w, r)
		if !ok {
			return
		}
		var in translationInput
		if decodeJSON(r, &in) != nil || in.SourceRevision != p.SourceCommitID {
			writeAPIError(w, 409, "localization_revision_changed", "translation must match the current pull source revision")
			return
		}
		var v localization.Review
		e := pulls.WithSourceRevision(p.RepositoryID, p.ID, in.SourceRevision, func(current pullrequests.PullRequest) error {
			var storeErr error
			v, storeErr = store.Propose(current.RepositoryID, current.ID, in.SourceRevision, in.UnitID, in.Locale, in.Text, in.Note, actor.UserID)
			return storeErr
		})
		if errors.Is(e, pullrequests.ErrSourceChanged) || errors.Is(e, pullrequests.ErrNotReady) {
			writeAPIError(w, 409, "localization_revision_changed", "translation must match the current open pull source revision")
			return
		}
		if errors.Is(e, localization.ErrInvalid) || errors.Is(e, localization.ErrNotFound) {
			writeAPIError(w, 400, "invalid_translation", "the unit, locale, source revision, and translation must match the current extraction")
			return
		}
		if e != nil {
			writeAPIError(w, 500, "localization_unavailable", "translation proposal could not be persisted")
			return
		}
		writeJSON(w, 201, v)
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/localization/workspace", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		if actor.UserID == "" && actor.AgentID == "" {
			writeAuthenticationRequired(w, false)
			return
		}
		p, ok := pull(w, r)
		if !ok {
			return
		}
		var in localizationMutationInput
		if decodeJSON(r, &in) != nil || in.SourceRevision != p.SourceCommitID {
			writeAPIError(w, 409, "localization_revision_changed", "workspace changes must match the current pull source revision")
			return
		}
		if in.Mutation == "request_suggestion" {
			planID, _ := in.Payload["locale_plan_id"].(string)
			version := 0
			if x, ok := in.Payload["locale_plan_version"].(float64); ok {
				version = int(x)
			}
			locale, _ := in.Payload["locale"].(string)
			plan, e := plans.Get(planID, "")
			valid := e == nil && plan.RepositoryID == p.RepositoryID && plan.CurrentVersion == version
			if valid {
				valid = false
				for _, l := range plan.Revisions[len(plan.Revisions)-1].Locales {
					if l.ID == locale {
						valid = true
					}
				}
			}
			if !valid {
				writeAPIError(w, 409, "locale_plan_changed", "agent assistance requires the current locale plan version and a declared locale")
				return
			}
		}
		decisionPlanID, decisionPlanVersion, decisionLocale := "", 0, ""
		decisionNeedsReviewer := false
		if in.Mutation == "decide" {
			kind, _ := in.Payload["kind"].(string)
			if actor.UserID == "" {
				writeAPIError(w, 403, "localization_human_decision_required", "only a human reviewer may decide an agent suggestion")
				return
			}
			if kind == "approve" || kind == "reject" {
				decisionNeedsReviewer = true
				current, readErr := store.Get(p.RepositoryID, p.ID, p.SourceCommitID)
				suggestionID, _ := in.Payload["suggestion_id"].(string)
				decisionLocale, _ = in.Payload["locale"].(string)
				if readErr == nil {
					for _, suggestion := range current.Suggestions {
						if suggestion.ID == suggestionID && suggestion.Locale == decisionLocale {
							decisionPlanID, decisionPlanVersion = suggestion.LocalePlanID, suggestion.LocalePlanVersion
						}
					}
				}
				if decisionPlanID == "" || decisionPlanVersion < 1 {
					writeAPIError(w, 403, "localization_reviewer_required", "the current locale plan requires a declared human reviewer for approval or rejection")
					return
				}
			}
		}
		actorType, actorID := "user", actor.UserID
		if actor.AgentID != "" {
			actorType, actorID = "agent", actor.AgentID
		}
		var v localization.Review
		persist := func() error {
			return pulls.WithSourceRevision(p.RepositoryID, p.ID, in.SourceRevision, func(current pullrequests.PullRequest) error {
				var mutationErr error
				v, mutationErr = store.Mutate(current.RepositoryID, current.ID, in.SourceRevision, in.ExpectedVersion, in.Mutation, actorType, actorID, in.Payload)
				return mutationErr
			})
		}
		var e error
		if decisionNeedsReviewer {
			e = plans.WithCurrentVersion(decisionPlanID, decisionPlanVersion, func(plan localeplans.Plan) error {
				if plan.RepositoryID != p.RepositoryID {
					return errLocalizationReviewerRequired
				}
				reviewer := false
				for _, declared := range plan.Revisions[len(plan.Revisions)-1].Locales {
					if declared.ID == decisionLocale {
						for _, id := range declared.ReviewerIDs {
							reviewer = reviewer || id == actor.UserID
						}
					}
				}
				if !reviewer {
					return errLocalizationReviewerRequired
				}
				return persist()
			})
		} else {
			e = persist()
		}
		switch {
		case errors.Is(e, pullrequests.ErrSourceChanged) || errors.Is(e, pullrequests.ErrNotReady):
			writeAPIError(w, 409, "localization_revision_changed", "the pull source changed; reload before continuing")
		case errors.Is(e, localization.ErrConflict):
			writeAPIError(w, 409, "localization_workspace_conflict", "the workspace changed; reload before continuing")
		case errors.Is(e, localeplans.ErrConflict):
			writeAPIError(w, 409, "locale_plan_changed", "the locale plan changed; reload before deciding")
		case errors.Is(e, errLocalizationReviewerRequired):
			writeAPIError(w, 403, "localization_reviewer_required", "the current locale plan requires a declared human reviewer for approval or rejection")
		case errors.Is(e, localization.ErrInvalid):
			writeAPIError(w, 400, "invalid_localization_workspace_change", "the change is incomplete, unauthorized for this actor type, protected, embargoed, or not grounded in current evidence")
		case e != nil:
			writeAPIError(w, 500, "localization_unavailable", "the localization workspace could not be persisted")
		default:
			writeJSON(w, 201, v)
		}
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/localization/verification", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		p, ok := pull(w, r)
		if !ok {
			return
		}
		var in localizationMutationInput
		if decodeJSON(r, &in) != nil || in.SourceRevision != p.SourceCommitID {
			writeAPIError(w, 409, "localization_revision_changed", "verification must match the current pull source revision")
			return
		}
		if in.Mutation != "publish_candidate" && in.Mutation != "record_checks" {
			writeAPIError(w, 400, "invalid_localization_verification", "participants may publish candidates or repository-defined check results here")
			return
		}
		candidatePlanID, candidatePlanVersion := "", 0
		if in.Mutation == "publish_candidate" {
			previewID, _ := in.Payload["preview_id"].(string)
			locale, _ := in.Payload["locale"].(string)
			planID, _ := in.Payload["locale_plan_id"].(string)
			planVersion := 0
			if value, yes := in.Payload["locale_plan_version"].(float64); yes {
				planVersion = int(value)
			}
			preview, previewErr := previewStore.Get(p.RepositoryID, p.ID, previewID)
			plan, planErr := plans.Get(planID, "")
			validPlan := planErr == nil && plan.RepositoryID == p.RepositoryID && plan.CurrentVersion == planVersion
			journeys := map[string]bool{}
			localeDeclared := false
			if validPlan {
				for _, value := range plan.Revisions[len(plan.Revisions)-1].Locales {
					if value.ID == locale {
						localeDeclared = true
					}
				}
			}
			validPlan = validPlan && localeDeclared
			if validPlan {
				for _, value := range plan.Revisions[len(plan.Revisions)-1].Journeys {
					for _, id := range value.LocaleIDs {
						if id == locale {
							journeys[value.ID] = true
						}
					}
				}
			}
			var routes []localization.VerificationRoute
			validRoutes := decodePayload(in.Payload["routes"], &routes)
			for _, route := range routes {
				validRoutes = validRoutes && journeys[route.JourneyID]
			}
			if previewErr != nil || preview.Revision != p.SourceCommitID || preview.Stale || !validPlan || !validRoutes {
				writeAPIError(w, 409, "localization_candidate_invalid", "candidate preview, locale plan, journeys, and pull revision must all be current")
				return
			}
			in.Payload["preview_url"] = preview.URL
			candidatePlanID, candidatePlanVersion = planID, planVersion
		} else {
			candidateID, _ := in.Payload["candidate_id"].(string)
			current, readErr := store.Get(p.RepositoryID, p.ID, p.SourceCommitID)
			if readErr != nil {
				writeAPIError(w, 404, "localization_candidate_not_found", "locale candidate is not available")
				return
			}
			for _, candidate := range current.VerificationCandidates {
				if candidate.ID == candidateID {
					candidatePlanID, candidatePlanVersion = candidate.LocalePlanID, candidate.LocalePlanVersion
				}
			}
			if candidatePlanID == "" {
				writeAPIError(w, 404, "localization_candidate_not_found", "locale candidate is not available")
				return
			}
		}
		var out localization.Review
		persist := func() error {
			return pulls.WithSourceRevision(p.RepositoryID, p.ID, in.SourceRevision, func(current pullrequests.PullRequest) error {
				var mutationErr error
				out, mutationErr = store.Verify(current.RepositoryID, current.ID, in.SourceRevision, in.ExpectedVersion, in.Mutation, actor.UserID, "translator", candidatePlanVersion, in.Payload)
				return mutationErr
			})
		}
		var err error
		if candidatePlanID != "" {
			err = plans.WithCurrentVersion(candidatePlanID, candidatePlanVersion, func(plan localeplans.Plan) error {
				if plan.RepositoryID != p.RepositoryID {
					return localeplans.ErrInvalid
				}
				return persist()
			})
		} else {
			err = persist()
		}
		applyLocalizationPlanVersions(plans, &out)
		writeLocalizationVerification(w, err, out)
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/localization/previews/{preview_id}/review", func(w http.ResponseWriter, r *http.Request) {
		actor, invitation, preview, ok := authorizePreviewGuest(w, r, catalog, previewStore, credentials)
		if !ok {
			return
		}
		p, ok := pull(w, r)
		if !ok {
			return
		}
		var in localizationMutationInput
		if decodeJSON(r, &in) != nil || in.SourceRevision != p.SourceCommitID || preview.Revision != p.SourceCommitID || preview.Stale || (in.Mutation != "finding" && in.Mutation != "review") {
			writeAPIError(w, 409, "localization_preview_changed", "regional evidence must match the current bounded preview and pull revision")
			return
		}
		candidateID, _ := in.Payload["candidate_id"].(string)
		current, readErr := store.Get(p.RepositoryID, p.ID, p.SourceCommitID)
		applyLocalizationPlanVersions(plans, &current)
		candidateMatches := false
		candidateCurrent := false
		locale := ""
		candidatePlanID, candidatePlanVersion := "", 0
		for _, candidate := range current.VerificationCandidates {
			if candidate.ID == candidateID && candidate.PreviewID == preview.ID {
				candidateMatches, locale = true, candidate.Locale
				candidatePlanID, candidatePlanVersion = candidate.LocalePlanID, candidate.LocalePlanVersion
			}
		}
		for _, projection := range current.Verification {
			if projection.CandidateID == candidateID {
				candidateCurrent = projection.Current
			}
		}
		if readErr != nil || !candidateMatches {
			writeAPIError(w, 404, "localization_candidate_not_found", "locale candidate is not available through this preview")
			return
		}
		if !candidateCurrent {
			writeAPIError(w, 409, "localization_candidate_stale", "source, translation, or interface changes require a current locale candidate")
			return
		}
		in.Payload["locale"] = locale
		role := "regional_reviewer"
		repository, _ := catalog.GetByID(p.RepositoryID)
		collaborator, _ := catalog.HasCollaborator(actor.UserID, p.RepositoryID)
		if actor.UserID == repository.OwnerID || collaborator {
			role = "translator"
		} else if invitation.Role != "feedback" && invitation.Role != "test" {
			writeAPIError(w, 403, "localization_review_access_required", "the invitation must permit testing or feedback")
			return
		}
		var out localization.Review
		err := previewStore.WithAudienceAdmission(func() error {
			return plans.WithCurrentVersion(candidatePlanID, candidatePlanVersion, func(plan localeplans.Plan) error {
				if plan.RepositoryID != p.RepositoryID {
					return localeplans.ErrInvalid
				}
				return pulls.WithSourceRevision(p.RepositoryID, p.ID, in.SourceRevision, func(current pullrequests.PullRequest) error {
					var mutationErr error
					out, mutationErr = store.Verify(current.RepositoryID, current.ID, in.SourceRevision, in.ExpectedVersion, in.Mutation, actor.UserID, role, candidatePlanVersion, in.Payload)
					return mutationErr
				})
			})
		})
		applyLocalizationPlanVersions(plans, &out)
		writeLocalizationVerification(w, err, out)
	})
}

func applyLocalizationPlanVersions(plans *localeplans.Store, review *localization.Review) {
	versions := map[string]int{}
	for _, candidate := range review.VerificationCandidates {
		if _, seen := versions[candidate.LocalePlanID]; seen {
			continue
		}
		plan, err := plans.Get(candidate.LocalePlanID, "")
		if err == nil && plan.RepositoryID == review.RepositoryID {
			versions[candidate.LocalePlanID] = plan.CurrentVersion
		}
	}
	localization.ApplyLocalePlanVersions(review, versions)
}

func decodePayload(value any, out any) bool {
	b, err := json.Marshal(value)
	return err == nil && json.Unmarshal(b, out) == nil
}
func writeLocalizationVerification(w http.ResponseWriter, err error, out localization.Review) {
	switch {
	case errors.Is(err, pullrequests.ErrSourceChanged) || errors.Is(err, pullrequests.ErrNotReady):
		writeAPIError(w, 409, "localization_revision_changed", "the pull source changed; reload before continuing")
	case errors.Is(err, localization.ErrConflict):
		writeAPIError(w, 409, "localization_workspace_conflict", "the localization workspace changed; reload before continuing")
	case errors.Is(err, localeplans.ErrConflict):
		writeAPIError(w, 409, "locale_plan_changed", "the locale plan changed; publish verification against its current version")
	case errors.Is(err, localization.ErrInvalid):
		writeAPIError(w, 400, "invalid_localization_verification", "verification evidence must be locale-, route-, journey-, unit-, preview-, and revision-grounded")
	case err != nil:
		writeAPIError(w, 500, "localization_unavailable", "localization verification could not be persisted")
	default:
		writeJSON(w, 201, out)
	}
}
