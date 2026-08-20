package main

import (
	"errors"
	"net/http"
	"path"
	"strings"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/agentprojects"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
)

type agentProjectInput struct {
	ExpectedVersion int                    `json:"expected_version"`
	Revision        agentprojects.Revision `json:"revision"`
}

func registerAgentProjectRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, store *agentprojects.Store) {
	mux.HandleFunc("GET /repositories/{id}/agent-projects", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		v, e := store.List(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "agent_projects_unavailable", "agent projects could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"projects": projectAgentSources(git, catalog, actor.UserID, v)})
	})
	mux.HandleFunc("GET /repositories/{id}/agent-projects/{project_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		v, e := store.Get(r.PathValue("project_id"))
		if e != nil || v.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "agent_project_not_found", "agent project not found")
			return
		}
		writeJSON(w, 200, projectAgentSources(git, catalog, actor.UserID, []agentprojects.Project{v})[0])
	})
	publish := func(revise bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
			if !ok {
				return
			}
			var in agentProjectInput
			if decodeJSON(r, &in) != nil {
				writeAPIError(w, 400, "invalid_request", "a complete agent project revision is required")
				return
			}
			repo := r.PathValue("id")
			deps := []string{}
			for _, s := range in.Revision.Sources {
				deps = append(deps, s.RepositoryID)
			}
			owners := append([]string{actor.UserID}, in.Revision.OwnerIDs...)
			for _, e := range in.Revision.Escalations {
				owners = append(owners, e.OwnerIDs...)
			}
			var out agentprojects.Project
			var e error
			e = catalog.WithCurrentParticipantsAndReadAccess(owners, repo, actor.UserID, deps, func() error {
				for _, source := range in.Revision.Sources {
					if !agentProjectSourceResolves(git, source) {
						return agentprojects.ErrInvalid
					}
				}
				if revise {
					current, x := store.Get(r.PathValue("project_id"))
					if x != nil || current.RepositoryID != repo {
						return agentprojects.ErrNotFound
					}
					out, x = store.Revise(current.ID, in.ExpectedVersion, actor.UserID, in.Revision)
					return x
				}
				out, e = store.Create(repo, actor.UserID, in.Revision)
				return e
			})
			status := 201
			if revise {
				status = 200
			}
			if e == nil {
				out = projectAgentSources(git, catalog, actor.UserID, []agentprojects.Project{out})[0]
			}
			writeAgentProject(w, out, e, status)
		}
	}
	mux.HandleFunc("POST /repositories/{id}/agent-projects", publish(false))
	mux.HandleFunc("POST /repositories/{id}/agent-projects/{project_id}/revisions", publish(true))
}

func projectAgentSources(git *storage.Store, catalog *repositories.Store, reader string, values []agentprojects.Project) []agentprojects.Project {
	for i := range values {
		for revisionIndex := range values[i].Revisions {
			r := &values[i].Revisions[revisionIndex]
			for sourceIndex := range r.Sources {
				source := &r.Sources[sourceIndex]
				dependency, err := catalog.GetByID(source.RepositoryID)
				readable := err == nil && (dependency.Visibility == repositories.Public || catalog.WithCurrentParticipant(reader, source.RepositoryID, func() error { return nil }) == nil)
				visible := readable && agentProjectSourceResolves(git, *source)
				if visible {
					continue
				}
				kind, message := "inaccessible_source", "A selected source is no longer reachable from a visible repository branch."
				if source.RepositoryID != values[i].RepositoryID && !readable {
					kind, message = "inaccessible_dependency", "A selected dependency is no longer accessible to this reader."
				}
				values[i].Diagnostics = append(values[i].Diagnostics, agentprojects.Diagnostic{Kind: kind, Severity: "blocking", Message: message, SourceID: source.ID, AttributedTo: r.CreatedBy})
				source.RepositoryID, source.Revision, source.Path, source.Purpose = "restricted", "", "", "restricted dependency"
			}
		}
	}
	return values
}
func agentProjectSourceResolves(git *storage.Store, s agentprojects.Source) bool {
	if git == nil || s.RepositoryID == "" || s.Path == "" || path.Clean(s.Path) != s.Path || strings.HasPrefix(s.Path, "/") || strings.HasPrefix(s.Path, "../") {
		return false
	}
	repo, e := git.Open(s.RepositoryID)
	if e != nil {
		return false
	}
	commit, e := repo.ReadCommit(storage.ObjectID(strings.ToLower(s.Revision)))
	if e != nil {
		return false
	}
	visible, e := revisionReachableFromVisibleBranch(repo, commit.ID)
	if e != nil || !visible {
		return false
	}
	entries, e := repo.WalkTree(commit.Tree)
	if e != nil {
		return false
	}
	for _, x := range entries {
		if x.Path == s.Path && x.Type == storage.BlobObject {
			return true
		}
	}
	return false
}
func writeAgentProject(w http.ResponseWriter, v agentprojects.Project, e error, status int) {
	switch {
	case e == nil:
		writeJSON(w, status, v)
	case errors.Is(e, agentprojects.ErrNotFound):
		writeAPIError(w, 404, "agent_project_not_found", "agent project not found")
	case errors.Is(e, agentprojects.ErrConflict):
		writeAPIError(w, 409, "agent_project_conflict", "the project changed; reload before revising")
	case errors.Is(e, agentprojects.ErrInvalid):
		writeAPIError(w, 400, "invalid_agent_project", "the agent project revision is incomplete")
	default:
		writeAPIError(w, 403, "agent_project_forbidden", "owners and dependencies must be current and accessible")
	}
}
