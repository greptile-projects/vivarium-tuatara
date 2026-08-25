package main

import (
	"errors"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/organizations"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/provenancepolicies"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"net/http"
)

type provenancePolicyInput struct {
	ExpectedVersion int                         `json:"expected_version"`
	Revision        provenancepolicies.Revision `json:"revision"`
}

func registerProvenancePolicyRoutes(mux *http.ServeMux, repos *repositories.Store, orgs *organizations.Store, credentials *auth.Store, store *provenancepolicies.Store) {
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
			a, ok := authenticateRequest(w, r, credentials, "organizations:write", false)
			if !ok {
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
					e = save()
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
	default:
		writeAPIError(w, 500, "provenance_policies_unavailable", "provenance policy could not be persisted")
	}
}
