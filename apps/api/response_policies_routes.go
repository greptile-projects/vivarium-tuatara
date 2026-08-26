package main

import (
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/responsepolicies"
)

type responsePolicyInput struct {
	RequestID       string                    `json:"request_id"`
	ExpectedVersion int                       `json:"expected_version"`
	Revision        responsepolicies.Revision `json:"revision"`
}
type responseRotationInput struct {
	RequestID       string                            `json:"request_id"`
	ExpectedVersion int                               `json:"expected_version"`
	Revision        responsepolicies.RotationRevision `json:"revision"`
}
type responseDutyInput struct {
	RequestID       string                         `json:"request_id"`
	ExpectedVersion int                            `json:"expected_version"`
	Kind            string                         `json:"kind"`
	ShiftID         string                         `json:"shift_id"`
	ToUserID        string                         `json:"to_user_id"`
	Reason          string                         `json:"reason"`
	Context         []responsepolicies.DutyContext `json:"context"`
}
type responseDutyAcceptInput struct {
	ExpectedVersion int `json:"expected_version"`
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
	project := func(actor, repositoryID string, value responsepolicies.Rotation) responsepolicies.Rotation {
		repository, err := catalog.Get(actor, repositoryID)
		current := map[string]bool{}
		if err == nil {
			for _, id := range repository.ParticipantIDs() {
				current[id] = true
			}
		}
		return responsepolicies.ProjectRotation(value, current, time.Now().UTC())
	}
	currentTeamMembers := func(rotation responsepolicies.Rotation) (map[string]bool, error) {
		if len(rotation.Revisions) == 0 {
			return nil, responsepolicies.ErrInvalid
		}
		revision := rotation.Revisions[len(rotation.Revisions)-1]
		policy, err := store.Get(revision.PolicyID)
		if err != nil || policy.RepositoryID != rotation.RepositoryID || len(policy.Revisions) == 0 {
			return nil, responsepolicies.ErrInvalid
		}
		members := map[string]bool{}
		for _, team := range policy.Revisions[len(policy.Revisions)-1].Teams {
			if team.ID == revision.TeamID {
				for _, id := range team.MemberIDs {
					members[id] = true
				}
				return members, nil
			}
		}
		return nil, responsepolicies.ErrInvalid
	}
	mux.HandleFunc("GET /repositories/{id}/response-rotations", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		values, err := store.ListRotations(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "response_rotations_unavailable", "response rotations could not be read")
			return
		}
		for i := range values {
			values[i] = project(actor.UserID, r.PathValue("id"), values[i])
		}
		writeJSON(w, 200, map[string]any{"response_rotations": values})
	})
	mux.HandleFunc("GET /repositories/{id}/response-rotations/{rotation_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		out, err := store.GetRotation(r.PathValue("rotation_id"))
		if err != nil || out.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "response_rotation_not_found", "response rotation not found")
			return
		}
		writeJSON(w, 200, project(actor.UserID, r.PathValue("id"), out))
	})
	publishRotation := func(revise bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
			if !ok {
				return
			}
			var in responseRotationInput
			if decodeJSON(r, &in) != nil {
				writeAPIError(w, 400, "invalid_request", "a caller-stable identity and complete rotation are required")
				return
			}
			policy, err := store.Get(in.Revision.PolicyID)
			if err != nil || policy.RepositoryID != r.PathValue("id") {
				writeAPIError(w, 400, "invalid_response_rotation", "the rotation must bind a response policy in this repository")
				return
			}
			latest := policy.Revisions[len(policy.Revisions)-1]
			members := []string{}
			currentResponders := []string{actor.UserID}
			teamFound := false
			for _, team := range latest.Teams {
				if team.ID == in.Revision.TeamID {
					teamFound = true
					members = append(members, team.MemberIDs...)
				}
			}
			for _, responder := range in.Revision.Responders {
				if !responseRotationContains(members, responder.UserID) {
					writeAPIError(w, 400, "invalid_response_rotation", "every responder must belong to the policy team")
					return
				}
				currentResponders = append(currentResponders, responder.UserID)
			}
			if !teamFound {
				writeAPIError(w, 400, "invalid_response_rotation", "the accountable policy team was not found")
				return
			}
			var out responsepolicies.Rotation
			err = catalog.WithCurrentParticipants(currentResponders, r.PathValue("id"), func() error {
				var e error
				if revise {
					current, x := store.GetRotation(r.PathValue("rotation_id"))
					if x != nil || current.RepositoryID != r.PathValue("id") {
						return responsepolicies.ErrNotFound
					}
					out, e = store.ReviseRotation(current.ID, in.ExpectedVersion, actor.UserID, in.RequestID, in.Revision)
				} else {
					out, e = store.CreateRotation(r.PathValue("id"), actor.UserID, in.RequestID, in.Revision)
				}
				return e
			})
			writeResponseRotation(w, project(actor.UserID, r.PathValue("id"), out), err, map[bool]int{false: 201, true: 200}[revise])
		}
	}
	mux.HandleFunc("POST /repositories/{id}/response-rotations", publishRotation(false))
	mux.HandleFunc("POST /repositories/{id}/response-rotations/{rotation_id}/revisions", publishRotation(true))
	mux.HandleFunc("POST /repositories/{id}/response-rotations/{rotation_id}/duty-events", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:read")
		if !ok {
			return
		}
		var in responseDutyInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a complete duty event is required")
			return
		}
		current, err := store.GetRotation(r.PathValue("rotation_id"))
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "response_rotation_not_found", "response rotation not found")
			return
		}
		members, membershipErr := currentTeamMembers(current)
		if membershipErr != nil || !members[actor.UserID] || (in.ToUserID != "" && !members[in.ToUserID]) {
			writeAPIError(w, 403, "response_rotation_forbidden", "duty events require current accountable-team membership")
			return
		}
		participants := []string{actor.UserID}
		if in.ToUserID != "" {
			participants = append(participants, in.ToUserID)
		}
		var out responsepolicies.Rotation
		err = catalog.WithCurrentParticipants(participants, r.PathValue("id"), func() error {
			var e error
			out, e = store.AppendDutyEvent(current.ID, actor.UserID, in.RequestID, in.Kind, in.ShiftID, in.ToUserID, in.Reason, in.Context, in.ExpectedVersion)
			return e
		})
		writeResponseRotation(w, project(actor.UserID, r.PathValue("id"), out), err, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/response-rotations/{rotation_id}/duty-events/{event_id}/accept", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:read")
		if !ok {
			return
		}
		var in responseDutyAcceptInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "expected_version is required")
			return
		}
		current, err := store.GetRotation(r.PathValue("rotation_id"))
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "response_rotation_not_found", "response rotation not found")
			return
		}
		members, membershipErr := currentTeamMembers(current)
		var pendingSender string
		for _, event := range current.Events {
			if event.ID == r.PathValue("event_id") {
				pendingSender = event.FromUserID
				break
			}
		}
		if membershipErr != nil || !members[actor.UserID] || pendingSender == "" || !members[pendingSender] {
			writeAPIError(w, 403, "response_rotation_forbidden", "duty acceptance requires current accountable-team membership")
			return
		}
		var out responsepolicies.Rotation
		err = catalog.WithCurrentParticipants([]string{actor.UserID, pendingSender}, r.PathValue("id"), func() error {
			var e error
			out, e = store.AcceptDutyEvent(current.ID, r.PathValue("event_id"), actor.UserID, in.ExpectedVersion)
			return e
		})
		writeResponseRotation(w, project(actor.UserID, r.PathValue("id"), out), err, 200)
	})
}
func responseRotationContains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
func writeResponseRotation(w http.ResponseWriter, out responsepolicies.Rotation, err error, status int) {
	switch {
	case err == nil:
		writeJSON(w, status, out)
	case errors.Is(err, responsepolicies.ErrNotFound):
		writeAPIError(w, 404, "response_rotation_not_found", "response rotation or duty event not found")
	case errors.Is(err, responsepolicies.ErrConflict):
		writeAPIError(w, 409, "response_rotation_conflict", "the rotation changed or this request identity was reused")
	case errors.Is(err, responsepolicies.ErrInvalid):
		writeAPIError(w, 400, "invalid_response_rotation", "rotation, qualification, workload, handoff, context, and acceptance requirements were not met")
	case errors.Is(err, repositories.ErrInvalidCollaborator), errors.Is(err, repositories.ErrNotFound):
		writeAPIError(w, 403, "response_rotation_forbidden", "every assigned responder must remain a current repository participant")
	default:
		log.Printf("response rotation storage: %v", err)
		writeAPIError(w, 500, "response_rotations_unavailable", "response rotation could not be persisted")
	}
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
