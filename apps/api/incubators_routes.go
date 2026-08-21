package main

import (
	"errors"
	"log"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	productfeedback "github.com/greptile-projects/vivarium-tuatara/apps/api/feedback"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/governance"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/incubators"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/supportthreads"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

type incubatorCreateInput struct {
	incubators.Incubator
	Invitations []incubators.Invitation `json:"invitations"`
}
type incubatorEventInput struct {
	ExpectedVersion int    `json:"expected_version"`
	Kind            string `json:"kind"`
	DecisionKind    string `json:"decision_kind"`
	Body            string `json:"body"`
	Visibility      string `json:"visibility"`
}
type incubatorConsentInput struct {
	ExpectedVersion int    `json:"expected_version"`
	Decision        string `json:"decision"`
}

func registerIncubatorRoutes(mux *http.ServeMux, credentials *auth.Store, identities *users.Store, catalog *repositories.Store, orgs *organizations.Store, store *incubators.Store, feedback *productfeedback.Store, support *supportthreads.Store, proposals *governance.Store) {
	authn := func(w http.ResponseWriter, r *http.Request) (auth.Credential, bool) {
		a, ok := authenticateRequest(w, r, credentials, "repositories:read", false)
		if !ok {
			return a, false
		}
		if a.UserID == "" && a.AgentID == "" {
			writeAuthenticationRequired(w, false)
			return a, false
		}
		return a, true
	}
	resolve := func(source incubators.Source, actor auth.Credential) incubators.Source {
		if source.Kind == "new_idea" {
			source.Resolution = "resolved"
			source.Detail = "Original idea supplied by the creator"
			return source
		}
		source.Resolution = "inaccessible"
		source.Detail = "Source context is unavailable to this collaborator"
		repo, err := catalog.GetByID(source.RepositoryID)
		if err != nil {
			return source
		}
		allowed := actor.UserID == repo.OwnerID
		if !allowed {
			allowed, _ = catalog.HasCollaborator(actor.UserID, repo.ID)
		}
		if repo.Visibility == repositories.Public {
			allowed = true
		}
		if !allowed {
			return source
		}
		exists := false
		switch source.Kind {
		case "feedback":
			if feedback != nil {
				x, e := feedback.Get(source.ResourceID)
				exists = e == nil && x.RepositoryID == repo.ID
			}
		case "support_gap":
			if support != nil {
				_, e := support.Get(repo.ID, source.ResourceID)
				exists = e == nil
			}
		case "governed_proposal":
			if proposals != nil {
				x, e := proposals.Get(source.ResourceID)
				exists = e == nil && x.ScopeType == "repository" && x.ScopeID == repo.ID
			}
		}
		if exists {
			source.Resolution = "resolved"
			source.Detail = "Source context resolved for the creator"
		} else {
			source.Resolution = "missing"
			source.Detail = "Source context does not resolve in the selected repository"
		}
		return source
	}
	mux.HandleFunc("POST /incubators", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authn(w, r)
		if !ok {
			return
		}
		if actor.AgentID != "" {
			writeAPIError(w, 403, "human_creator_required", "a human collaborator must open an incubator")
			return
		}
		var in incubatorCreateInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "complete incubator intent and invitations are required")
			return
		}
		for _, id := range in.SponsorIDs {
			if _, e := identities.Get(id); e != nil {
				writeAPIError(w, 422, "invalid_sponsor", "sponsors must be existing human identities")
				return
			}
		}
		for _, invite := range in.Invitations {
			if invite.PrincipalType == "human" {
				if _, e := identities.Get(invite.PrincipalID); e != nil {
					writeAPIError(w, 422, "invalid_invitee", "human invitees must exist")
					return
				}
			} else if invite.PrincipalType == "agent" {
				org, e := orgs.Get(invite.OrganizationID)
				found := false
				if e == nil {
					for _, a := range org.Agents {
						if a.ID == invite.PrincipalID {
							found = true
						}
					}
				}
				if !found {
					writeAPIError(w, 422, "unapproved_agent", "agent invitees must be approved organization agents")
					return
				}
			}
		}
		in.Source = resolve(in.Source, actor)
		out, e := store.Create(in.Incubator, actor.UserID, in.Invitations)
		writeIncubator(w, out, e, 201)
	})
	mux.HandleFunc("GET /incubators", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authn(w, r)
		if !ok {
			return
		}
		viewer := actor.UserID
		if actor.AgentID != "" {
			viewer = actor.AgentID
		}
		all, e := store.List(viewer)
		if e != nil {
			writeIncubator(w, incubators.Incubator{}, e, 500)
			return
		}
		writeJSON(w, 200, map[string]any{"incubators": all})
	})
	mux.HandleFunc("GET /incubators/{incubator_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authn(w, r)
		if !ok {
			return
		}
		viewer := actor.UserID
		if actor.AgentID != "" {
			viewer = actor.AgentID
		}
		out, e := store.Get(r.PathValue("incubator_id"), viewer)
		writeIncubator(w, out, e, 200)
	})
	mux.HandleFunc("POST /incubators/{incubator_id}/events", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authn(w, r)
		if !ok {
			return
		}
		var in incubatorEventInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an attributable event and expected version are required")
			return
		}
		typ, id := "human", actor.UserID
		if actor.AgentID != "" {
			typ, id = "agent", actor.AgentID
		}
		out, e := store.AddEvent(r.PathValue("incubator_id"), typ, id, in.ExpectedVersion, incubators.Event{Kind: in.Kind, DecisionKind: in.DecisionKind, Body: in.Body, Visibility: in.Visibility})
		writeIncubator(w, out, e, 200)
	})
	mux.HandleFunc("POST /incubators/{incubator_id}/invitations/{invitation_id}/consent", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authn(w, r)
		if !ok {
			return
		}
		if actor.AgentID != "" {
			writeAPIError(w, 403, "human_consent_required", "agent participation is governed by its existing approval")
			return
		}
		var in incubatorConsentInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an invitation decision and expected version are required")
			return
		}
		out, e := store.Consent(r.PathValue("incubator_id"), r.PathValue("invitation_id"), actor.UserID, in.Decision, in.ExpectedVersion)
		writeIncubator(w, out, e, 200)
	})
}

func writeIncubator(w http.ResponseWriter, x incubators.Incubator, e error, status int) {
	switch {
	case e == nil:
		writeJSON(w, status, x)
	case errors.Is(e, incubators.ErrNotFound):
		writeAPIError(w, 404, "incubator_not_found", "incubator not found")
	case errors.Is(e, incubators.ErrConflict):
		writeAPIError(w, 409, "incubator_changed", "incubator changed; refresh before appending")
	case errors.Is(e, incubators.ErrInvalid):
		writeAPIError(w, 422, "invalid_incubator", "incubator intent, attribution, consent, or version is invalid")
	default:
		log.Printf("incubator storage: %v", e)
		writeAPIError(w, 500, "incubator_unavailable", "incubator could not be persisted")
	}
}
