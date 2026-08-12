package main

import (
	"errors"
	"net/http"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/acceptance"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

func registerAcceptanceRoutes(mux *http.ServeMux, catalog *repositories.Store, pulls *pullrequests.Store, store *acceptance.Store, authStore *auth.Store) {
	mux.HandleFunc("GET /repositories/{id}/branches/{branch}/preview-acceptance", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, authStore, r.PathValue("id")); !ok {
			return
		}
		policy, err := store.Policy(r.PathValue("id"), r.PathValue("branch"))
		if err != nil {
			writeAPIError(w, 500, "preview_acceptance_unavailable", "preview acceptance policy unavailable")
			return
		}
		writeJSON(w, 200, policy)
	})
	mux.HandleFunc("PUT /repositories/{id}/branches/{branch}/preview-acceptance", func(w http.ResponseWriter, r *http.Request) {
		actor, owner, ok := authorizeRepositoryParticipant(w, r, catalog, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		if !owner {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return
		}
		var input struct {
			Requirements []acceptance.Requirement `json:"requirements"`
		}
		if decodeJSON(r, &input) != nil || input.Requirements == nil {
			writeAPIError(w, 400, "invalid_preview_acceptance", "requirements must be an array")
			return
		}
		policy, err := store.SetPolicy(r.PathValue("id"), r.PathValue("branch"), actor.UserID, input.Requirements)
		if errors.Is(err, acceptance.ErrInvalid) {
			writeAPIError(w, 400, "invalid_preview_acceptance", "requirements need unique ids, bounded path/risk selectors, and scenarios with owner, contributor, or author roles")
			return
		}
		if err != nil {
			writeAPIError(w, 500, "preview_acceptance_unavailable", "preview acceptance policy unavailable")
			return
		}
		writeJSON(w, 200, policy)
	})
	mux.HandleFunc("GET /repositories/{id}/pulls/{pull_id}/preview-acceptance", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorizeRepositoryRead(w, r, catalog, authStore, r.PathValue("id"))
		if !ok {
			return
		}
		repo, err := catalog.GetByID(r.PathValue("id"))
		if err != nil {
			writeRepositoryError(w, err)
			return
		}
		report, err := pulls.Readiness(r.PathValue("id"), r.PathValue("pull_id"), authenticated && actor.UserID == repo.OwnerID)
		if writePullRequestError(w, err) {
			return
		}
		writeJSON(w, 200, report.PreviewAcceptance)
	})
	mux.HandleFunc("POST /repositories/{id}/pulls/{pull_id}/preview-acceptance/decisions", func(w http.ResponseWriter, r *http.Request) {
		actor, owner, ok := authorizeRepositoryParticipant(w, r, catalog, authStore, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		pull, err := pulls.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if writePullRequestError(w, err) {
			return
		}
		var input struct {
			Revision      string   `json:"revision"`
			RequirementID string   `json:"requirement_id"`
			Scenario      string   `json:"scenario"`
			Role          string   `json:"role"`
			Outcome       string   `json:"outcome"`
			RiskClasses   []string `json:"risk_classes"`
			Rationale     string   `json:"rationale"`
		}
		if decodeJSON(r, &input) != nil || input.Revision != pull.SourceCommitID {
			writeAPIError(w, 409, "preview_revision_changed", "decision must name the pull request's exact current revision")
			return
		}
		roleOK := input.Role == "owner" && owner || input.Role == "author" && actor.UserID == pull.AuthorID || input.Role == "contributor" && !owner
		if !roleOK {
			writeAPIError(w, 403, "acceptance_role_required", "actor does not hold the stated participant role")
			return
		}
		if input.Outcome == "overridden" && !owner {
			writeAPIError(w, 403, "owner_override_required", "only the repository owner can override acceptance")
			return
		}
		policy, e := store.Policy(pull.RepositoryID, pull.TargetBranch)
		if e != nil {
			writeAPIError(w, 500, "preview_acceptance_unavailable", "preview acceptance policy unavailable")
			return
		}
		found := false
		for _, q := range policy.Requirements {
			if q.ID != input.RequirementID {
				continue
			}
			for _, scenario := range q.Scenarios {
				if scenario.Name == input.Scenario && scenario.Role == input.Role {
					found = true
				}
			}
		}
		if !found {
			writeAPIError(w, 422, "acceptance_scenario_not_required", "scenario and role are not in the target branch policy")
			return
		}
		decision, e := store.Decide(acceptance.Decision{RepositoryID: pull.RepositoryID, PullRequestID: pull.ID, Revision: input.Revision, PolicyVersion: policy.Version, RequirementID: input.RequirementID, Scenario: input.Scenario, Role: input.Role, Outcome: input.Outcome, RiskClasses: input.RiskClasses, Rationale: strings.TrimSpace(input.Rationale), ActorID: actor.UserID})
		if errors.Is(e, acceptance.ErrInvalid) {
			writeAPIError(w, 400, "invalid_preview_acceptance", "decision is invalid; rejection and override require rationale")
			return
		}
		if e != nil {
			writeAPIError(w, 500, "preview_acceptance_unavailable", "preview acceptance decision unavailable")
			return
		}
		writeJSON(w, 201, decision)
	})
}
