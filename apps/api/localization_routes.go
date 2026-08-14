package main

import (
	"errors"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/localeplans"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/localization"
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

func registerLocalizationRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, pulls *pullrequests.Store, plans *localeplans.Store, store *localization.Store) {
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
		if in.Mutation == "decide" {
			kind, _ := in.Payload["kind"].(string)
			if actor.UserID == "" {
				writeAPIError(w, 403, "localization_human_decision_required", "only a human reviewer may decide an agent suggestion")
				return
			}
			if kind == "approve" || kind == "reject" {
				current, readErr := store.Get(p.RepositoryID, p.ID, p.SourceCommitID)
				suggestionID, _ := in.Payload["suggestion_id"].(string)
				locale, _ := in.Payload["locale"].(string)
				planID, planVersion := "", 0
				if readErr == nil {
					for _, suggestion := range current.Suggestions {
						if suggestion.ID == suggestionID && suggestion.Locale == locale {
							planID, planVersion = suggestion.LocalePlanID, suggestion.LocalePlanVersion
						}
					}
				}
				plan, planErr := plans.Get(planID, "")
				reviewer := false
				if planErr == nil && plan.CurrentVersion == planVersion {
					for _, declared := range plan.Revisions[len(plan.Revisions)-1].Locales {
						if declared.ID == locale {
							for _, id := range declared.ReviewerIDs {
								reviewer = reviewer || id == actor.UserID
							}
						}
					}
				}
				if !reviewer {
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
		e := pulls.WithSourceRevision(p.RepositoryID, p.ID, in.SourceRevision, func(current pullrequests.PullRequest) error {
			var mutationErr error
			v, mutationErr = store.Mutate(current.RepositoryID, current.ID, in.SourceRevision, in.ExpectedVersion, in.Mutation, actorType, actorID, in.Payload)
			return mutationErr
		})
		switch {
		case errors.Is(e, pullrequests.ErrSourceChanged) || errors.Is(e, pullrequests.ErrNotReady):
			writeAPIError(w, 409, "localization_revision_changed", "the pull source changed; reload before continuing")
		case errors.Is(e, localization.ErrConflict):
			writeAPIError(w, 409, "localization_workspace_conflict", "the workspace changed; reload before continuing")
		case errors.Is(e, localization.ErrInvalid):
			writeAPIError(w, 400, "invalid_localization_workspace_change", "the change is incomplete, unauthorized for this actor type, protected, embargoed, or not grounded in current evidence")
		case e != nil:
			writeAPIError(w, 500, "localization_unavailable", "the localization workspace could not be persisted")
		default:
			writeJSON(w, 201, v)
		}
	})
}
