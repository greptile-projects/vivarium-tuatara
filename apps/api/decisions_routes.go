package main

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/activities"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/decisions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/explanations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/incidents"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/relationships"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

type decisionCreateInput struct {
	Source decisions.Source `json:"source"`
	Scope  decisions.Scope  `json:"scope"`
}
type decisionUpdateInput struct {
	ExpectedVersion int             `json:"expected_version"`
	Scope           decisions.Scope `json:"scope"`
	Summary         string          `json:"summary"`
}
type decisionDiscussionInput struct {
	Body string `json:"body"`
}
type decisionAlternativeInput struct {
	ExpectedVersion int                   `json:"expected_version"`
	Alternative     decisions.Alternative `json:"alternative"`
}
type decisionResearchCredentialInput struct {
	ExpiresIn     int    `json:"expires_in"`
	AlternativeID string `json:"alternative_id"`
}

func registerDecisionRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, identities *users.Store, store *decisions.Store, activity *activities.Store, proposalStore *proposals.Store, explanationStore *explanations.Store, incidentStore *incidents.Store, relationshipStore *relationships.Store, organizationStore *organizations.Store) {
	mux.HandleFunc("POST /repositories/{id}/decisions", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in decisionCreateInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "source and scope are required")
			return
		}
		if in.Source.Kind == "repository" {
			in.Source.ResourceID = r.PathValue("id")
		}
		if !decisionSourceExists(w, r.PathValue("id"), actor.UserID, in.Source, proposalStore, explanationStore, incidentStore, relationshipStore, organizationStore) {
			return
		}
		if !validateDecisionUsers(w, identities, in.Scope) {
			return
		}
		v, err := store.Create(r.PathValue("id"), in.Source, in.Scope, actor.UserID)
		if writeDecisionError(w, err) {
			return
		}
		recordActivity(activity, catalog, activities.Event{Kind: "decision.opened", ActorID: actor.UserID, RepositoryID: v.RepositoryID, ResourceType: "decision", ResourceID: v.ID, ResourceTitle: v.Scope.Question})
		writeJSON(w, 201, v)
	})
	mux.HandleFunc("GET /decisions", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return
		}
		repos, err := catalog.ListAccessible(actor.UserID)
		if err != nil {
			writeAPIError(w, 500, "decision_storage_unavailable", "decisions could not be loaded")
			return
		}
		allowed := map[string]bool{}
		for _, x := range repos {
			allowed[x.ID] = true
		}
		all, err := store.List()
		if writeDecisionError(w, err) {
			return
		}
		out := []decisions.Decision{}
		repoFilter := strings.TrimSpace(r.URL.Query().Get("repository_id"))
		sourceKind := strings.TrimSpace(r.URL.Query().Get("source_kind"))
		sourceID := strings.TrimSpace(r.URL.Query().Get("source_id"))
		for _, x := range all {
			if allowed[x.RepositoryID] && (repoFilter == "" || x.RepositoryID == repoFilter) && (sourceKind == "" || x.Source.Kind == sourceKind) && (sourceID == "" || x.Source.ResourceID == sourceID) {
				out = append(out, x)
			}
		}
		writeJSON(w, 200, map[string]any{"decisions": out})
	})
	mux.HandleFunc("GET /decisions/{id}", func(w http.ResponseWriter, r *http.Request) {
		v, actor, ok := authorizeDecision(w, r, catalog, credentials, store, "repositories:read")
		if !ok {
			return
		}
		_ = actor
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("PUT /decisions/{id}", func(w http.ResponseWriter, r *http.Request) {
		_, actor, ok := authorizeDecision(w, r, catalog, credentials, store, "repositories:write")
		if !ok {
			return
		}
		var in decisionUpdateInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "expected_version, scope, and summary are required")
			return
		}
		if !validateDecisionUsers(w, identities, in.Scope) {
			return
		}
		v, err := store.Update(r.PathValue("id"), actor.UserID, in.ExpectedVersion, in.Scope, in.Summary)
		if writeDecisionError(w, err) {
			return
		}
		recordActivity(activity, catalog, activities.Event{Kind: "decision.scope_changed", ActorID: actor.UserID, RepositoryID: v.RepositoryID, ResourceType: "decision", ResourceID: v.ID, ResourceTitle: v.Scope.Question})
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("POST /decisions/{id}/discussion", func(w http.ResponseWriter, r *http.Request) {
		_, actor, ok := authorizeDecision(w, r, catalog, credentials, store, "repositories:write")
		if !ok {
			return
		}
		var in decisionDiscussionInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "body is required")
			return
		}
		v, err := store.Discuss(r.PathValue("id"), actor.UserID, in.Body)
		if writeDecisionError(w, err) {
			return
		}
		recordActivity(activity, catalog, activities.Event{Kind: "decision.discussed", ActorID: actor.UserID, RepositoryID: v.RepositoryID, ResourceType: "decision", ResourceID: v.ID, ResourceTitle: v.Scope.Question})
		writeJSON(w, 201, v)
	})
	mux.HandleFunc("POST /decisions/{id}/alternatives", func(w http.ResponseWriter, r *http.Request) {
		_, actor, ok := authorizeDecision(w, r, catalog, credentials, store, "repositories:write")
		if !ok {
			return
		}
		var in decisionAlternativeInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "expected_version and alternative are required")
			return
		}
		v, err := store.AddAlternative(r.PathValue("id"), actor.UserID, in.ExpectedVersion, in.Alternative)
		if writeDecisionError(w, err) {
			return
		}
		recordActivity(activity, catalog, activities.Event{Kind: "decision.alternative_proposed", ActorID: actor.UserID, RepositoryID: v.RepositoryID, ResourceType: "decision", ResourceID: v.ID, ResourceTitle: v.Scope.Question})
		writeJSON(w, 201, v)
	})
	mux.HandleFunc("POST /decisions/{id}/research-credentials", func(w http.ResponseWriter, r *http.Request) {
		v, actor, ok := authorizeDecision(w, r, catalog, credentials, store, "repositories:write")
		if !ok {
			return
		}
		var in decisionResearchCredentialInput
		if decodeJSON(r, &in) != nil || in.ExpiresIn < 60 || in.ExpiresIn > 86400 {
			writeAPIError(w, 400, "invalid_request", "expires_in must be between 60 and 86400 seconds")
			return
		}
		selected := false
		for _, alternative := range v.Alternatives {
			selected = selected || alternative.ID == in.AlternativeID && alternative.SupersededBy == ""
		}
		if !selected {
			writeAPIError(w, 400, "invalid_alternative", "a current decision alternative must be selected")
			return
		}
		issued, err := credentials.IssueBound(actor.UserID, auth.API, "Decision research "+v.ID+":"+in.AlternativeID, []string{"decisions:research", "repositories:read"}, time.Duration(in.ExpiresIn)*time.Second, v.RepositoryID, "")
		if err != nil {
			writeAPIError(w, 500, "credential_storage_unavailable", "research credential could not be issued")
			return
		}
		writeJSON(w, 201, issued)
	})
	mux.HandleFunc("POST /decisions/{id}/findings", func(w http.ResponseWriter, r *http.Request) {
		v, err := store.Get(r.PathValue("id"))
		if writeDecisionError(w, err) {
			return
		}
		actor, ok := authenticateRequest(w, r, credentials, "decisions:research", false)
		if !ok {
			return
		}
		prefix := "Decision research " + v.ID + ":"
		if actor.RepositoryID != v.RepositoryID || !strings.HasPrefix(actor.Name, prefix) {
			writeAPIError(w, 404, "decision_not_found", "decision not found")
			return
		}
		var in decisions.Finding
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "finding and citations are required")
			return
		}
		if in.AlternativeID != strings.TrimPrefix(actor.Name, prefix) {
			writeAPIError(w, 404, "alternative_not_found", "alternative not found")
			return
		}
		v, err = store.AddFinding(v.ID, actor.UserID, in)
		if writeDecisionError(w, err) {
			return
		}
		writeJSON(w, 201, v)
	})
}

func validateDecisionUsers(w http.ResponseWriter, identities *users.Store, scope decisions.Scope) bool {
	if identities == nil {
		writeAPIError(w, http.StatusServiceUnavailable, "decision_identity_unavailable", "decision identities are unavailable")
		return false
	}
	ids := append([]decisions.Participant(nil), scope.Participants...)
	ownerFound := false
	for _, participant := range ids {
		if participant.UserID == scope.OwnerID {
			ownerFound = true
		}
		if _, err := identities.Get(participant.UserID); err != nil {
			if errors.Is(err, users.ErrNotFound) {
				writeAPIError(w, http.StatusBadRequest, "invalid_decision_participant", "every decision participant must be an existing user")
			} else {
				writeAPIError(w, http.StatusInternalServerError, "decision_identity_unavailable", "decision identities are unavailable")
			}
			return false
		}
	}
	if !ownerFound {
		writeAPIError(w, http.StatusBadRequest, "invalid_decision_owner", "the decision owner must be an existing participant")
		return false
	}
	return true
}

func decisionSourceExists(w http.ResponseWriter, repositoryID, actor string, source decisions.Source, proposalStore *proposals.Store, explanationStore *explanations.Store, incidentStore *incidents.Store, relationshipStore *relationships.Store, organizationStore *organizations.Store) bool {
	found, available := false, true
	switch source.Kind {
	case "repository":
		found = source.ResourceID == repositoryID
	case "proposal":
		if proposalStore == nil {
			available = false
		} else {
			_, err := proposalStore.Get(repositoryID, source.ResourceID)
			found = err == nil
		}
	case "investigation":
		if explanationStore == nil {
			available = false
		} else if v, err := explanationStore.Get(source.ResourceID); err == nil && v.RepositoryID == repositoryID {
			for _, p := range v.Participants {
				found = found || p.UserID == actor
			}
		}
	case "incident":
		if incidentStore == nil {
			available = false
		} else if v, err := incidentStore.Get(source.ResourceID); err == nil {
			for _, scope := range v.Scopes {
				found = found || scope.RepositoryID == repositoryID
			}
		}
	case "evolution_plan":
		if relationshipStore == nil {
			available = false
		} else {
			_, err := relationshipStore.GetEvolution(repositoryID, source.ResourceID)
			found = err == nil
		}
	case "stewardship_opportunity":
		if organizationStore == nil {
			available = false
		} else if groups, err := organizationStore.ListFor(actor); err != nil {
			available = false
		} else {
			for _, group := range groups {
				for _, mandate := range group.StewardshipMandates {
					for _, opportunity := range mandate.Opportunities {
						found = found || opportunity.ID == source.ResourceID && opportunity.RepositoryID == repositoryID
					}
				}
			}
		}
	}
	if !available {
		writeAPIError(w, http.StatusServiceUnavailable, "decision_context_unavailable", "the selected decision context is unavailable")
		return false
	}
	if !found {
		writeAPIError(w, http.StatusNotFound, "decision_context_not_found", "the selected decision context was not found")
		return false
	}
	return true
}
func authorizeDecision(w http.ResponseWriter, r *http.Request, c *repositories.Store, a *auth.Store, s *decisions.Store, scope string) (decisions.Decision, auth.Credential, bool) {
	v, e := s.Get(r.PathValue("id"))
	if errors.Is(e, decisions.ErrNotFound) {
		writeAPIError(w, 404, "decision_not_found", "decision not found")
		return v, auth.Credential{}, false
	}
	if writeDecisionError(w, e) {
		return v, auth.Credential{}, false
	}
	actor, _, ok := authorizeRepositoryParticipant(w, r, c, a, v.RepositoryID, scope)
	return v, actor, ok
}
func writeDecisionError(w http.ResponseWriter, e error) bool {
	if e == nil {
		return false
	}
	switch {
	case errors.Is(e, decisions.ErrNotFound):
		writeAPIError(w, 404, "decision_not_found", "decision not found")
	case errors.Is(e, decisions.ErrInvalid):
		writeAPIError(w, 400, "invalid_decision", "decision scope is invalid")
	case errors.Is(e, decisions.ErrConflict):
		writeAPIError(w, 409, "decision_changed", "the decision changed; reload before editing")
	default:
		writeAPIError(w, 500, "decision_storage_unavailable", "decision storage is unavailable")
	}
	return true
}
