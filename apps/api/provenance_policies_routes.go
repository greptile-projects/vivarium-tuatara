package main

import (
	"errors"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/agentprojects"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/contributorpathways"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	packages "github.com/greptile-projects/vivarium-tuatara/apps/api/packages"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/provenancepolicies"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/releases"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"net/http"
	"strings"
)

type provenancePolicyInput struct {
	ExpectedVersion int                         `json:"expected_version"`
	Revision        provenancepolicies.Revision `json:"revision"`
}

func registerProvenancePolicyRoutes(mux *http.ServeMux, repos *repositories.Store, orgs *organizations.Store, credentials *auth.Store, store *provenancepolicies.Store, pathways *contributorpathways.Store, agents *agentprojects.Store, packageStore *packages.Store, releaseStore *releases.Store) {
	register := func(kind, prefix string) {
		authorize := func(w http.ResponseWriter, r *http.Request, write bool) (auth.Credential, bool) {
			id := r.PathValue("id")
			if kind == "repository" {
				if !write {
					a, _, ok := authorizeRepositoryRead(w, r, repos, credentials, id)
					return a, ok
				}
				a, owner, ok := authorizeRepositoryParticipant(w, r, repos, credentials, id, "repositories:write")
				if !ok {
					return a, false
				}
				if !owner {
					writeAPIError(w, 403, "provenance_policy_owner_required", "only the repository owner may publish provenance policy")
					return a, false
				}
				return a, true
			}
			scope := "repositories:read"
			if write {
				scope = "repositories:write"
			}
			a, ok := authenticateRequest(w, r, credentials, scope, false)
			if !ok {
				return a, false
			}
			if a.RepositoryID != "" {
				writeAPIError(w, 404, "organization_not_found", "organization not found")
				return a, false
			}
			if orgs == nil {
				writeAPIError(w, 404, "organization_not_found", "organization not found")
				return a, false
			}
			o, e := orgs.Get(id)
			if e != nil {
				writeAPIError(w, 404, "organization_not_found", "organization not found")
				return a, false
			}
			member := o.CreatedBy == a.UserID
			for _, m := range o.Members {
				if m.UserID == a.UserID {
					member = true
				}
			}
			if !member || write && o.CreatedBy != a.UserID {
				writeAPIError(w, 404, "organization_not_found", "organization not found")
				return a, false
			}
			return a, true
		}
		mux.HandleFunc("GET "+prefix+"/{id}/provenance-policies", func(w http.ResponseWriter, r *http.Request) {
			if _, ok := authorize(w, r, false); !ok {
				return
			}
			v, e := store.List(kind, r.PathValue("id"))
			if e != nil {
				writeAPIError(w, 500, "provenance_policies_unavailable", "provenance policies could not be read")
				return
			}
			writeJSON(w, 200, map[string]any{"policies": v})
		})
		mux.HandleFunc("GET "+prefix+"/{id}/provenance-policies/{policy_id}", func(w http.ResponseWriter, r *http.Request) {
			if _, ok := authorize(w, r, false); !ok {
				return
			}
			v, e := store.Get(r.PathValue("policy_id"))
			if e != nil || v.ScopeKind != kind || v.ScopeID != r.PathValue("id") {
				writeAPIError(w, 404, "provenance_policy_not_found", "provenance policy not found")
				return
			}
			writeJSON(w, 200, v)
		})
		publish := func(revise bool) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) {
				actor, ok := authorize(w, r, true)
				if !ok {
					return
				}
				var in provenancePolicyInput
				if decodeJSON(r, &in) != nil {
					writeAPIError(w, 400, "invalid_request", "a complete provenance policy revision is required")
					return
				}
				if !validProvenanceLinks(kind, r.PathValue("id"), in.Revision.Links, repos, pathways, agents, packageStore, releaseStore) {
					writeAPIError(w, 422, "invalid_provenance_link", "every linked pathway, agent contract, package, release, and contribution boundary must resolve within the policy scope")
					return
				}
				var out provenancepolicies.Policy
				var e error
				save := func() error {
					if revise {
						current, x := store.Get(r.PathValue("policy_id"))
						if x != nil || current.ScopeKind != kind || current.ScopeID != r.PathValue("id") {
							return provenancepolicies.ErrNotFound
						}
						out, x = store.Revise(current.ID, in.ExpectedVersion, actor.UserID, in.Revision)
						return x
					}
					out, e = store.Create(kind, r.PathValue("id"), actor.UserID, in.Revision)
					return e
				}
				if kind == "repository" {
					participants := append([]string{}, in.Revision.OwnerIDs...)
					for _, x := range in.Revision.Rules {
						participants = append(participants, x.ReviewOwnerIDs...)
					}
					for _, x := range in.Revision.Exceptions {
						participants = append(participants, x.OwnerID)
					}
					e = repos.WithCurrentParticipants(participants, r.PathValue("id"), save)
				} else {
					repositoryIDs := []string{}
					for _, link := range in.Revision.Links {
						if link.RepositoryID != "" {
							repositoryIDs = append(repositoryIDs, link.RepositoryID)
						}
					}
					if len(repositoryIDs) == 0 {
						e = save()
					} else {
						e = repos.WithCurrentOrganizationRepositories(r.PathValue("id"), repositoryIDs, save)
					}
				}
				status := 201
				if revise {
					status = 200
				}
				writeProvenancePolicy(w, out, e, status)
			}
		}
		mux.HandleFunc("POST "+prefix+"/{id}/provenance-policies", publish(false))
		mux.HandleFunc("POST "+prefix+"/{id}/provenance-policies/{policy_id}/revisions", publish(true))
	}
	register("repository", "/repositories")
	register("organization", "/organizations")
}

func validProvenanceLinks(scopeKind, scopeID string, links []provenancepolicies.Link, repos *repositories.Store, pathways *contributorpathways.Store, agents *agentprojects.Store, packageStore *packages.Store, releaseStore *releases.Store) bool {
	for _, link := range links {
		repositoryID := link.RepositoryID
		if scopeKind == "repository" {
			if repositoryID != "" && repositoryID != scopeID {
				return false
			}
			repositoryID = scopeID
		} else {
			if repositoryID == "" {
				return false
			}
			repository, err := repos.GetByID(repositoryID)
			if err != nil || repository.OrganizationID != scopeID {
				return false
			}
		}
		switch link.Kind {
		case "contributor_pathway":
			if pathways == nil {
				return false
			}
			values, err := pathways.List(repositoryID)
			if err != nil {
				return false
			}
			found := false
			for _, value := range values {
				if value.ID == link.ResourceID {
					found = true
				}
			}
			if !found {
				return false
			}
		case "agent_contract":
			if agents == nil {
				return false
			}
			value, err := agents.Get(link.ResourceID)
			if err != nil || value.RepositoryID != repositoryID {
				return false
			}
		case "package":
			if packageStore == nil {
				return false
			}
			name, version, ok := strings.Cut(link.ResourceID, "@")
			if !ok {
				return false
			}
			value, err := packageStore.Get(name, version)
			if err != nil || value.RepositoryID != repositoryID {
				return false
			}
		case "release":
			if releaseStore == nil {
				return false
			}
			if _, err := releaseStore.Get(repositoryID, link.ResourceID); err != nil {
				return false
			}
		case "contribution_boundary":
			if link.ResourceID != repositoryID {
				return false
			}
		default:
			return false
		}
	}
	return true
}
func writeProvenancePolicy(w http.ResponseWriter, v provenancepolicies.Policy, e error, status int) {
	switch {
	case e == nil:
		writeJSON(w, status, v)
	case errors.Is(e, provenancepolicies.ErrNotFound):
		writeAPIError(w, 404, "provenance_policy_not_found", "provenance policy not found")
	case errors.Is(e, provenancepolicies.ErrConflict):
		writeAPIError(w, 409, "provenance_policy_conflict", "the policy changed; reload before publishing")
	case errors.Is(e, provenancepolicies.ErrInvalid):
		writeAPIError(w, 400, "invalid_provenance_policy", "material rules, licenses, owners, distribution contexts, links, and exceptions must be complete and consistent")
	case errors.Is(e, repositories.ErrInvalidCollaborator):
		writeAPIError(w, 403, "provenance_policy_owner_invalid", "policy, review, and exception owners must be current repository participants")
	case errors.Is(e, repositories.ErrNotFound):
		writeAPIError(w, 422, "invalid_provenance_link", "a linked repository no longer belongs to the policy scope")
	default:
		writeAPIError(w, 500, "provenance_policies_unavailable", "provenance policy could not be persisted")
	}
}
