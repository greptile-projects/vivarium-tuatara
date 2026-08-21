package main

import (
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/adoptionworkspaces"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/apicontracts"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/decisions"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/federation"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/incubators"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	packageversions "github.com/greptile-projects/vivarium-tuatara/apps/api/packages"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/roadmaps"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/supportthreads"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/users"
)

type adoptionCreateInput struct {
	adoptionworkspaces.Workspace
	Invitations []adoptionworkspaces.Invitation `json:"invitations"`
}
type adoptionConsentInput struct {
	Decision        string `json:"decision"`
	ExpectedVersion int    `json:"expected_version"`
}

func registerAdoptionWorkspaceRoutes(mux *http.ServeMux, credentials *auth.Store, identities *users.Store, catalog *repositories.Store, orgs *organizations.Store, incubatorStore *incubators.Store, federationStore *federation.Store, roadmapStore *roadmaps.Store, supportStore *supportthreads.Store, decisionStore *decisions.Store, packageStore *packageversions.Store, apiStore *apicontracts.Store, store *adoptionworkspaces.Store) {
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
	canReadRepository := func(actor auth.Credential, id string) bool {
		repo, e := catalog.GetByID(id)
		if e != nil {
			return false
		}
		if repo.Visibility == repositories.Public || repo.OwnerID == actor.UserID {
			return true
		}
		if actor.AgentID != "" {
			return actor.RepositoryID == id
		}
		ok, _ := catalog.HasCollaborator(actor.UserID, id)
		return ok
	}
	resolveSource := func(source adoptionworkspaces.Source, actor auth.Credential) adoptionworkspaces.Source {
		source.Resolution, source.Detail = "inaccessible", "Starting context is outside this collaborator's current read boundary"
		if source.Kind == "federated_repository" {
			if federationStore == nil {
				return source
			}
			cache, e := federationStore.RepositoryCache(source.ResourceID)
			if e != nil {
				source.Resolution, source.Detail = "missing", "Federated repository has not been resolved"
			} else if cache.Status != "current" || cache.Snapshot == nil || !cache.SignatureVerified {
				source.Resolution, source.Detail = "stale", "Federated repository evidence is unavailable or stale"
			} else {
				source.Resolution, source.Detail = "resolved", "Current signed federated repository snapshot"
			}
			return source
		}
		if !canReadRepository(actor, source.RepositoryID) {
			return source
		}
		exists := strings.TrimSpace(source.ResourceID) != ""
		switch source.Kind {
		case "roadmap_outcome":
			exists = false
			if roadmapStore != nil {
				if x, e := roadmapStore.Get(source.RepositoryID); e == nil {
					for _, r := range x.Revisions {
						for _, item := range r.Items {
							exists = exists || item.ID == source.ResourceID || item.OpportunityID == source.ResourceID
						}
					}
				}
			}
		case "support_gap":
			if supportStore != nil {
				x, e := supportStore.Get(source.RepositoryID, source.ResourceID)
				exists = e == nil && x.RepositoryID == source.RepositoryID
			}
		case "incubator":
			exists = false
			if incubatorStore != nil {
				_, e := incubatorStore.Get(source.ResourceID, actor.UserID)
				exists = e == nil
			}
		case "decision":
			if decisionStore != nil {
				x, e := decisionStore.Get(source.ResourceID)
				exists = e == nil && x.RepositoryID == source.RepositoryID
			}
		case "package":
			parts := strings.SplitN(source.ResourceID, "@", 2)
			exists = false
			if packageStore != nil && len(parts) == 2 {
				x, e := packageStore.Get(parts[0], parts[1])
				exists = e == nil && x.RepositoryID == source.RepositoryID
			}
		case "api":
			if apiStore != nil {
				x, e := apiStore.Get(source.ResourceID)
				exists = e == nil && x.RepositoryID == source.RepositoryID
			}
		}
		if exists {
			source.Resolution, source.Detail = "resolved", "Starting context resolved inside the creator's current read boundary"
		} else {
			source.Resolution, source.Detail = "missing", "Starting context does not resolve"
		}
		return source
	}
	projectEvidence := func(in *adoptionCreateInput, actor auth.Credential) {
		for i := range in.Candidates {
			c := &in.Candidates[i]
			for j := range c.Evidence {
				e := &c.Evidence[j]
				e.Resolution = "missing"
				e.Detail = "Evidence reference does not resolve"
				if e.RepositoryID != "" {
					if canReadRepository(actor, e.RepositoryID) {
						e.Resolution, e.Detail = "resolved", "Repository evidence admitted within current read access"
					} else {
						e.Resolution, e.Detail = "inaccessible", "Repository evidence is outside the creator's read boundary"
						e.RepositoryID = ""
						e.Reference = "Restricted evidence"
						e.Summary = "Restricted evidence"
					}
					continue
				}
				u, err := url.ParseRequestURI(e.Reference)
				if err == nil && u.Scheme == "https" && u.Host != "" {
					e.Resolution, e.Detail = "resolved", "Public HTTPS evidence"
				}
			}
		}
	}
	mux.HandleFunc("POST /adoption-workspaces", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authn(w, r)
		if !ok {
			return
		}
		if actor.AgentID != "" {
			writeAPIError(w, 403, "human_creator_required", "a human collaborator must open an adoption workspace")
			return
		}
		var in adoptionCreateInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "complete adoption requirements, candidates, and invitations are required")
			return
		}
		for _, v := range in.Invitations {
			if v.PrincipalType == "human" {
				if _, e := identities.Get(v.PrincipalID); e != nil {
					writeAPIError(w, 422, "invalid_invitee", "human invitees must exist")
					return
				}
			} else if v.PrincipalType == "agent" {
				org, e := orgs.Get(v.OrganizationID)
				approved := false
				if e == nil {
					for _, a := range org.Agents {
						approved = approved || a.ID == v.PrincipalID
					}
				}
				if !approved {
					writeAPIError(w, 422, "unapproved_agent", "agents must be approved by the selected organization")
					return
				}
			}
		}
		in.Source = resolveSource(in.Source, actor)
		projectEvidence(&in, actor)
		out, e := store.Create(in.Workspace, actor.UserID, in.Invitations)
		writeAdoptionWorkspace(w, out, e, 201)
	})
	mux.HandleFunc("GET /adoption-workspaces", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authn(w, r)
		if !ok {
			return
		}
		viewer := actor.UserID
		if actor.AgentID != "" {
			viewer = actor.AgentID
		}
		out, e := store.List(viewer)
		if e != nil {
			writeAdoptionWorkspace(w, adoptionworkspaces.Workspace{}, e, 500)
			return
		}
		writeJSON(w, 200, map[string]any{"adoption_workspaces": out})
	})
	mux.HandleFunc("GET /adoption-workspaces/{workspace_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authn(w, r)
		if !ok {
			return
		}
		viewer := actor.UserID
		if actor.AgentID != "" {
			viewer = actor.AgentID
		}
		out, e := store.Get(r.PathValue("workspace_id"), viewer)
		writeAdoptionWorkspace(w, out, e, 200)
	})
	mux.HandleFunc("POST /adoption-workspaces/{workspace_id}/invitations/{invitation_id}/consent", func(w http.ResponseWriter, r *http.Request) {
		actor, ok := authn(w, r)
		if !ok {
			return
		}
		if actor.AgentID != "" {
			writeAPIError(w, 403, "human_consent_required", "agents are admitted only under their existing organization approval")
			return
		}
		var in adoptionConsentInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a decision and expected version are required")
			return
		}
		out, e := store.Consent(r.PathValue("workspace_id"), r.PathValue("invitation_id"), actor.UserID, in.Decision, in.ExpectedVersion)
		writeAdoptionWorkspace(w, out, e, 200)
	})
}

func writeAdoptionWorkspace(w http.ResponseWriter, x adoptionworkspaces.Workspace, e error, status int) {
	switch {
	case e == nil:
		writeJSON(w, status, x)
	case errors.Is(e, adoptionworkspaces.ErrNotFound):
		writeAPIError(w, 404, "adoption_workspace_not_found", "adoption workspace not found")
	case errors.Is(e, adoptionworkspaces.ErrConflict):
		writeAPIError(w, 409, "adoption_workspace_changed", "adoption workspace changed; refresh before responding")
	case errors.Is(e, adoptionworkspaces.ErrInvalid):
		writeAPIError(w, 422, "invalid_adoption_workspace", "adoption requirements, evidence, permissions, or versions are invalid")
	default:
		log.Printf("adoption workspace storage: %v", e)
		writeAPIError(w, 500, "adoption_workspace_unavailable", "adoption workspace could not be persisted")
	}
}
