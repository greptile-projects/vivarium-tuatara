package main

import (
	"errors"
	"log"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/responsepolicies"
)

type responsePolicyInput struct {
	RequestID       string                    `json:"request_id"`
	ExpectedVersion int                       `json:"expected_version"`
	Revision        responsepolicies.Revision `json:"revision"`
}

func registerResponsePolicyRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, store *responsepolicies.Store) {
	mux.HandleFunc("GET /repositories/{id}/response-policies", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		values, err := store.List(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "response_policies_unavailable", "response policies could not be read")
			return
		}
		writeJSON(w, 200, map[string]any{"response_policies": values})
	})
	mux.HandleFunc("GET /repositories/{id}/response-policies/{policy_id}", func(w http.ResponseWriter, r *http.Request) {
		if _, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id")); !ok {
			return
		}
		out, err := store.Get(r.PathValue("policy_id"))
		if err != nil || out.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "response_policy_not_found", "response policy not found")
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
			var in responsePolicyInput
			if decodeJSON(r, &in) != nil {
				writeAPIError(w, 400, "invalid_request", "a caller-stable identity and complete response policy revision are required")
				return
			}
			participants := []string{actor.UserID}
			for _, team := range in.Revision.Teams {
				participants = append(participants, team.MemberIDs...)
			}
			var out responsepolicies.Policy
			err := catalog.WithCurrentParticipants(participants, r.PathValue("id"), func() error {
				var e error
				if revise {
					current, x := store.Get(r.PathValue("policy_id"))
					if x != nil || current.RepositoryID != r.PathValue("id") {
						return responsepolicies.ErrNotFound
					}
					out, e = store.Revise(current.ID, in.ExpectedVersion, actor.UserID, in.RequestID, in.Revision)
				} else {
					out, e = store.Create(r.PathValue("id"), actor.UserID, in.RequestID, in.Revision)
				}
				return e
			})
			writeResponsePolicy(w, out, err, map[bool]int{false: 201, true: 200}[revise])
		}
	}
	mux.HandleFunc("POST /repositories/{id}/response-policies", publish(false))
	mux.HandleFunc("POST /repositories/{id}/response-policies/{policy_id}/revisions", publish(true))
}
func writeResponsePolicy(w http.ResponseWriter, out responsepolicies.Policy, err error, status int) {
	switch {
	case err == nil:
		writeJSON(w, status, out)
	case errors.Is(err, responsepolicies.ErrNotFound):
		writeAPIError(w, 404, "response_policy_not_found", "response policy not found")
	case errors.Is(err, responsepolicies.ErrConflict):
		writeAPIError(w, 409, "response_policy_conflict", "the policy changed or this request identity was reused with different content")
	case errors.Is(err, responsepolicies.ErrCommitted):
		writeAPIError(w, 503, "response_policy_commit_ambiguous", "the policy may have committed; retry the unchanged request_id")
	case errors.Is(err, responsepolicies.ErrInvalid):
		writeAPIError(w, 400, "invalid_response_policy", "the policy must completely define resources, teams, urgent conditions, targets, escalation, audiences, incident criteria, and authority boundaries")
	case errors.Is(err, repositories.ErrInvalidCollaborator), errors.Is(err, repositories.ErrNotFound):
		writeAPIError(w, 403, "response_policy_forbidden", "every declared response-team member must be a current repository participant")
	default:
		log.Printf("response policy storage: %v", err)
		writeAPIError(w, 500, "response_policies_unavailable", "response policies could not be persisted")
	}
}
