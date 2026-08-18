package main

import (
	"errors"
	"log"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/deployments"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/infrastructure"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

type infrastructureInput struct {
	ExpectedVersion int                     `json:"expected_version"`
	Revision        infrastructure.Revision `json:"revision"`
}

func registerInfrastructureRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, definitions *infrastructure.Store, releaseStore *releases.Store, deploymentStore *deployments.Store) {
	mux.HandleFunc("GET /repositories/{id}/infrastructure", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		values, err := definitions.List(r.PathValue("id"), infrastructureParticipant(catalog, r.PathValue("id"), actor, authenticated))
		if err != nil {
			writeAPIError(w, 500, "infrastructure_unavailable", "infrastructure definitions could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"definitions": values})
	})
	mux.HandleFunc("GET /repositories/{id}/infrastructure/{definition_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, authenticated, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		out, err := definitions.Get(r.PathValue("definition_id"), infrastructureParticipant(catalog, r.PathValue("id"), actor, authenticated))
		if err != nil || out.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "infrastructure_not_found", "infrastructure definition not found")
			return
		}
		writeJSON(w, 200, out)
	})
	mux.HandleFunc("POST /repositories/{id}/infrastructure", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		var in infrastructureInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete infrastructure revision is required")
			return
		}
		var out infrastructure.Definition
		err := catalog.WithCurrentParticipants(infrastructureOwners(actor.UserID, in.Revision), r.PathValue("id"), func() error {
			if !infrastructureRevisionResolves(git, r.PathValue("id"), in.Revision, releaseStore, deploymentStore) {
				return infrastructure.ErrInvalid
			}
			var e error
			out, e = definitions.Create(r.PathValue("id"), actor.UserID, in.Revision)
			return e
		})
		writeInfrastructure(w, out, err, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/infrastructure/{definition_id}/revisions", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		current, err := definitions.Get(r.PathValue("definition_id"), true)
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "infrastructure_not_found", "infrastructure definition not found")
			return
		}
		var in infrastructureInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "expected_version and a complete infrastructure revision are required")
			return
		}
		var out infrastructure.Definition
		err = catalog.WithCurrentParticipants(infrastructureOwners(actor.UserID, in.Revision), current.RepositoryID, func() error {
			if !infrastructureRevisionResolves(git, current.RepositoryID, in.Revision, releaseStore, deploymentStore) {
				return infrastructure.ErrInvalid
			}
			var e error
			out, e = definitions.Revise(current.ID, in.ExpectedVersion, actor.UserID, in.Revision)
			return e
		})
		writeInfrastructure(w, out, err, 200)
	})
	mux.HandleFunc("POST /repositories/{id}/infrastructure/{definition_id}/observations", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		current, err := definitions.Get(r.PathValue("definition_id"), true)
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "infrastructure_not_found", "infrastructure definition not found")
			return
		}
		var in infrastructure.Observation
		if decodeJSON(r, &in) != nil || !infrastructureLinksResolve(r.PathValue("id"), in.EnvironmentID, in.ReleaseID, releaseStore, deploymentStore) {
			writeAPIError(w, 400, "invalid_infrastructure_observation", "observation must be sanitized and bind exact definition, provider, environment, and release revisions")
			return
		}
		out, err := definitions.Observe(current.ID, actor.UserID, in)
		writeInfrastructure(w, out, err, 201)
	})
}

func infrastructureParticipant(catalog *repositories.Store, repo string, actor auth.Credential, authenticated bool) bool {
	if !authenticated {
		return false
	}
	if actor.AgentID != "" {
		return true
	}
	v, e := catalog.GetByID(repo)
	if e != nil {
		return false
	}
	if v.OwnerID == actor.UserID {
		return true
	}
	ok, e := catalog.HasCollaborator(actor.UserID, repo)
	return e == nil && ok
}
func infrastructureOwners(actor string, r infrastructure.Revision) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(x string) {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	add(actor)
	for _, x := range r.OwnerIDs {
		add(x)
	}
	for _, resource := range r.Resources {
		for _, x := range resource.OwnerIDs {
			add(x)
		}
	}
	return out
}
func infrastructureRevisionResolves(git *storage.Store, repo string, r infrastructure.Revision, releases *releases.Store, deployments *deployments.Store) bool {
	if git == nil {
		return false
	}
	gr, e := git.Open(repo)
	if e != nil {
		return false
	}
	if _, e = gr.ReadCommit(storage.ObjectID(r.Revision)); e != nil {
		return false
	}
	for _, x := range r.Resources {
		if !infrastructureLinksResolve(repo, x.EnvironmentID, x.ReleaseID, releases, deployments) {
			return false
		}
	}
	return true
}
func infrastructureLinksResolve(repo, environmentID, releaseID string, releases *releases.Store, deployments *deployments.Store) bool {
	if environmentID != "" {
		if deployments == nil {
			return false
		}
		if _, e := deployments.GetEnvironment(repo, environmentID); e != nil {
			return false
		}
	}
	if releaseID != "" {
		if releases == nil {
			return false
		}
		if _, e := releases.Get(repo, releaseID); e != nil {
			return false
		}
	}
	return true
}
func writeInfrastructure(w http.ResponseWriter, v infrastructure.Definition, err error, status int) {
	switch {
	case err == nil:
		writeJSON(w, status, v)
	case errors.Is(err, infrastructure.ErrConflict):
		writeAPIError(w, 409, "infrastructure_conflict", "the definition changed; reload before publishing another revision")
	case errors.Is(err, infrastructure.ErrInvalid):
		writeAPIError(w, 400, "invalid_infrastructure", "publish an exact commit with complete resources, owners, providers, boundaries, constraints, commitments, and sanitized observations")
	case errors.Is(err, repositories.ErrInvalidCollaborator), errors.Is(err, repositories.ErrNotFound):
		writeAPIError(w, 403, "infrastructure_forbidden", "every declared owner must be a current repository participant")
	default:
		log.Printf("infrastructure storage: %v", err)
		writeAPIError(w, 500, "infrastructure_unavailable", "infrastructure definition could not be persisted")
	}
}
