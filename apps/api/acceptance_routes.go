package main

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/acceptance"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/previews"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/pullrequests"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
)

func registerAcceptanceRoutes(mux *http.ServeMux, catalog *repositories.Store, pulls *pullrequests.Store, store *acceptance.Store, previewStore *previews.Store, authStore *auth.Store) {
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
			writeAPIError(w, 400, "invalid_preview_acceptance", "requirements need unique ids, bounded path/risk selectors, and scenarios with owner, contributor, author, or stakeholder roles")
			return
		}
		if errors.Is(err, acceptance.ErrDurabilityUncertain) {
			w.Header().Set("Vivarium-Durability", "uncertain")
			writeJSON(w, http.StatusAccepted, policy)
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
		actor, ok := authenticateRequest(w, r, authStore, "repositories:read", false)
		if !ok {
			return
		}
		repository, repoErr := catalog.GetByID(r.PathValue("id"))
		if repoErr != nil {
			writeRepositoryError(w, repoErr)
			return
		}
		collaborator, _ := catalog.HasCollaborator(actor.UserID, repository.ID)
		owner := actor.UserID == repository.OwnerID
		pull, err := pulls.Get(r.PathValue("id"), r.PathValue("pull_id"))
		if writePullRequestError(w, err) {
			return
		}
		var input struct {
			IdempotencyKey string   `json:"idempotency_key"`
			Revision       string   `json:"revision"`
			RequirementID  string   `json:"requirement_id"`
			Scenario       string   `json:"scenario"`
			Role           string   `json:"role"`
			Outcome        string   `json:"outcome"`
			RiskClasses    []string `json:"risk_classes"`
			Rationale      string   `json:"rationale"`
		}
		if decodeJSON(r, &input) != nil || input.Revision != pull.SourceCommitID {
			writeAPIError(w, 409, "preview_revision_changed", "decision must name the pull request's exact current revision")
			return
		}
		participant := owner || collaborator
		stakeholder := false
		if input.Role == "stakeholder" && previewStore != nil {
			all, listErr := previewStore.List(pull.RepositoryID, pull.ID, pull.SourceCommitID)
			if listErr != nil {
				writeAPIError(w, 503, "preview_acceptance_unavailable", "preview audience unavailable")
				return
			}
			for _, preview := range all {
				if preview.Revision != pull.SourceCommitID {
					continue
				}
				for _, invitation := range preview.Invitations {
					if invitation.UserID == actor.UserID && invitation.Role == "feedback" && invitation.RevokedAt == nil && invitation.ExpiresAt.After(time.Now().UTC()) {
						stakeholder = true
					}
				}
			}
		}
		roleOK := input.Role == "owner" && owner || input.Role == "author" && participant && actor.UserID == pull.AuthorID || input.Role == "contributor" && collaborator || input.Role == "stakeholder" && stakeholder
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
		decision, e := store.Decide(acceptance.Decision{RepositoryID: pull.RepositoryID, PullRequestID: pull.ID, Revision: input.Revision, PolicyVersion: policy.Version, IdempotencyKey: input.IdempotencyKey, RequirementID: input.RequirementID, Scenario: input.Scenario, Role: input.Role, Outcome: input.Outcome, RiskClasses: input.RiskClasses, Rationale: strings.TrimSpace(input.Rationale), ActorID: actor.UserID})
		if errors.Is(e, acceptance.ErrInvalid) {
			writeAPIError(w, 400, "invalid_preview_acceptance", "decision requires an idempotency key; rejection and override also require rationale")
			return
		}
		if errors.Is(e, acceptance.ErrRejected) {
			writeAPIError(w, 409, "preview_acceptance_rejected", "a rejection remains blocking until the repository owner records a justified override")
			return
		}
		if errors.Is(e, acceptance.ErrConflict) {
			writeAPIError(w, 409, "preview_acceptance_idempotency_conflict", "idempotency key already identifies a different decision")
			return
		}
		if errors.Is(e, acceptance.ErrDurabilityUncertain) {
			w.Header().Set("Vivarium-Durability", "uncertain")
			writeJSON(w, http.StatusAccepted, decision)
			return
		}
		if e != nil {
			writeAPIError(w, 500, "preview_acceptance_unavailable", "preview acceptance decision unavailable")
			return
		}
		writeJSON(w, 201, decision)
	})
}
