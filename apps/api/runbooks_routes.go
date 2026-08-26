package main

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/runbooks"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/storage"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/workflowcomponents"
)

type runbookInput struct {
	RequestID       string            `json:"request_id"`
	ExpectedVersion int               `json:"expected_version"`
	Revision        runbooks.Revision `json:"revision"`
}

func registerRunbookRoutes(mux *http.ServeMux, git *storage.Store, catalog *repositories.Store, credentials *auth.Store, store *runbooks.Store, workflows *workflowcomponents.Store, orgs *organizations.Store) {
	mux.HandleFunc("GET /repositories/{id}/runbooks", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		out, e := store.List(r.PathValue("id"))
		if e != nil {
			writeAPIError(w, 500, "runbooks_unavailable", "runbooks could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"runbooks": out})
	})
	mux.HandleFunc("GET /repositories/{id}/runbooks/{runbook_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		out, e := store.Get(r.PathValue("runbook_id"))
		if e != nil || out.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "runbook_not_found", "runbook not found")
			return
		}
		writeJSON(w, 200, out)
	})
	publish := func(revise bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
			if !ok {
				return
			}
			var in runbookInput
			if decodeJSON(r, &in) != nil {
				writeAPIError(w, 400, "invalid_request", "a caller-stable identity and complete runbook revision are required")
				return
			}
			resolveRunbookReferences(git, catalog, workflows, orgs, actor.UserID, r.PathValue("id"), &in.Revision)
			participants := append([]string{actor.UserID}, in.Revision.OwnerIDs...)
			for _, s := range in.Revision.Steps {
				participants = append(participants, s.OwnerIDs...)
			}
			for _, e := range in.Revision.Escalations {
				participants = append(participants, e.OwnerID)
			}
			var out runbooks.Runbook
			err := catalog.WithCurrentParticipants(participants, r.PathValue("id"), func() error {
				var e error
				if revise {
					current, x := store.Get(r.PathValue("runbook_id"))
					if x != nil || current.RepositoryID != r.PathValue("id") {
						return runbooks.ErrNotFound
					}
					out, e = store.Revise(current.ID, in.ExpectedVersion, actor.UserID, in.RequestID, in.Revision)
				} else {
					out, e = store.Create(r.PathValue("id"), actor.UserID, in.RequestID, in.Revision)
				}
				return e
			})
			status := 201
			if revise {
				status = 200
			}
			switch {
			case err == nil:
				writeJSON(w, status, out)
			case errors.Is(err, runbooks.ErrNotFound):
				writeAPIError(w, 404, "runbook_not_found", "runbook not found")
			case errors.Is(err, runbooks.ErrConflict):
				writeAPIError(w, 409, "runbook_conflict", "the request identity or expected version conflicts")
			case errors.Is(err, runbooks.ErrInvalid):
				writeAPIError(w, 400, "invalid_runbook", "the runbook revision is incomplete or invalid")
			case errors.Is(err, repositories.ErrInvalidCollaborator), errors.Is(err, repositories.ErrNotFound):
				writeAPIError(w, 400, "invalid_runbook", "owners and escalation recipients must be current repository participants")
			default:
				writeAPIError(w, 500, "runbooks_unavailable", "the retained runbook could not be read or written")
			}
		}
	}
	mux.HandleFunc("POST /repositories/{id}/runbooks", publish(false))
	mux.HandleFunc("POST /repositories/{id}/runbooks/{runbook_id}/revisions", publish(true))
}

// resolveRunbookReferences ignores caller status flags and derives them from the
// immutable resource and its current authority source.
func resolveRunbookReferences(git *storage.Store, catalog *repositories.Store, workflows *workflowcomponents.Store, orgs *organizations.Store, actorID, repositoryID string, revision *runbooks.Revision) {
	repository, _ := catalog.Get(actorID, repositoryID)
	for i := range revision.Steps {
		for j := range revision.Steps[i].References {
			ref := &revision.Steps[i].References[j]
			ref.Accessible, ref.Reviewed, ref.Approved = false, false, false
			switch ref.Kind {
			case "command", "documentation":
				_, _, ref.Accessible = infrastructureCommitBlob(git, repositoryID, ref.Revision, ref.ResourceID)
				ref.Reviewed = ref.Accessible && ref.Kind == "command" && runbookRevisionPublished(git, repositoryID, repository.DefaultBranch, ref.Revision)
			case "workflow_component":
				if workflows != nil {
					component, err := workflows.Get(ref.ResourceID)
					ref.Accessible = err == nil && component.Definition.Version == ref.Revision
					ref.Reviewed = ref.Accessible && component.Attestation.DefinitionSHA256 != ""
				}
			case "agent":
				if orgs != nil && repository.OrganizationID != "" {
					organization, err := orgs.Get(repository.OrganizationID)
					if err != nil {
						continue
					}
					for _, agent := range organization.Agents {
						if agent.ID != ref.ResourceID || strconv.Itoa(agent.Version) != ref.Revision {
							continue
						}
						ref.Accessible = true
						for _, grant := range organization.AccessGrants {
							resource := organizations.ResourceScope{Kind: "repository", ID: repositoryID}
							if !runbookAgentGrantCurrent(grant, agent.ID, resource, time.Now().UTC()) {
								continue
							}
							for _, granted := range grant.Resources {
								if granted == resource {
									ref.Approved = true
								}
							}
						}
					}
				}
			}
		}
	}
}

func runbookAgentGrantCurrent(grant organizations.AccessGrant, agentID string, resource organizations.ResourceScope, now time.Time) bool {
	if grant.PrincipalType != "agent" || grant.PrincipalID != agentID || grant.RevokedAt != nil || (grant.ExpiresAt != nil && !grant.ExpiresAt.After(now)) {
		return false
	}
	for _, exception := range grant.Exceptions {
		if exception.Resource == resource {
			return false
		}
	}
	return true
}

func runbookRevisionPublished(git *storage.Store, repositoryID, defaultBranch, revision string) bool {
	repository, err := git.Open(repositoryID)
	if err != nil {
		return false
	}
	ref, err := repository.ReadReference("refs/heads/" + defaultBranch)
	if err != nil {
		return false
	}
	commits, err := repository.ListCommitAncestry(storage.ObjectID(ref.Target))
	if err != nil {
		return false
	}
	for _, commit := range commits {
		if string(commit.ID) == revision {
			return true
		}
	}
	return false
}
