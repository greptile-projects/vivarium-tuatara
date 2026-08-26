package main

import (
	"errors"
	"log"
	"net/http"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/activities"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/repositories"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/responsealerts"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/responsepolicies"
)

type responseAlertInput struct {
	RequestID string                `json:"request_id"`
	Signal    responsealerts.Signal `json:"signal"`
}
type responseAlertEventInput struct {
	RequestID string `json:"request_id"`
	Kind      string `json:"kind"`
	Reason    string `json:"reason"`
}

func registerResponseAlertRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, policies *responsepolicies.Store, alerts *responsealerts.Store, activity *activities.Store) {
	mux.HandleFunc("GET /repositories/{id}/response-alerts", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		repository, _ := catalog.GetByID(r.PathValue("id"))
		owner := actor.UserID != "" && repository.OwnerID == actor.UserID
		values, err := alerts.List(r.PathValue("id"))
		if err != nil {
			writeAPIError(w, 500, "response_alerts_unavailable", "response alerts could not be read")
			return
		}
		current, _ := policies.List(r.PathValue("id"))
		visible := values[:0]
		for _, v := range values {
			if alertVisible(v, actor.UserID, owner) {
				visible = append(visible, projectResponseAlert(v, actor.UserID, current))
			}
		}
		writeJSON(w, 200, map[string]any{"response_alerts": visible})
	})
	mux.HandleFunc("GET /repositories/{id}/response-alerts/{alert_id}", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		repository, _ := catalog.GetByID(r.PathValue("id"))
		owner := actor.UserID != "" && repository.OwnerID == actor.UserID
		v, e := alerts.Get(r.PathValue("alert_id"))
		if e != nil || v.RepositoryID != r.PathValue("id") || !alertVisible(v, actor.UserID, owner) {
			writeAPIError(w, 404, "response_alert_not_found", "response alert not found")
			return
		}
		current, _ := policies.List(r.PathValue("id"))
		writeJSON(w, 200, projectResponseAlert(v, actor.UserID, current))
	})
	mux.HandleFunc("POST /repositories/{id}/response-alerts", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		repo, repoErr := catalog.GetByID(r.PathValue("id"))
		if repoErr != nil {
			writeAPIError(w, 404, "repository_not_found", "repository not found")
			return
		}
		var in responseAlertInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a caller-stable identity and complete revision-bound signal are required")
			return
		}
		list, e := policies.List(repo.ID)
		if e != nil || len(list) == 0 {
			writeAPIError(w, 409, "response_policy_missing", "an active response policy is required")
			return
		}
		policy := list[0]
		rev := policy.Revisions[len(policy.Revisions)-1]
		teamID := ""
		for _, rule := range rev.Rules {
			if rule.SignalClass == in.Signal.SignalClass && rule.Severity == in.Signal.Severity && responseAlertIntersects(rule.ResourceIDs, in.Signal.ResourceIDs) {
				if teamID != "" && teamID != rule.AccountableTeamID {
					writeAPIError(w, 409, "response_policy_ambiguous", "active policy routes this signal to conflicting teams")
					return
				}
				teamID = rule.AccountableTeamID
			}
		}
		recipients := []string{}
		for _, team := range rev.Teams {
			if team.ID == teamID {
				recipients = append(recipients, team.MemberIDs...)
			}
		}
		// An active exact-policy rotation narrows delivery to the effective duty owner.
		rotations, _ := policies.ListRotations(repo.ID)
		current := map[string]bool{}
		for _, id := range repo.ParticipantIDs() {
			current[id] = true
		}
		currentRecipients := recipients[:0]
		for _, id := range recipients {
			if current[id] {
				currentRecipients = append(currentRecipients, id)
			}
		}
		recipients = currentRecipients
		for _, rotation := range rotations {
			rr := rotation.Revisions[len(rotation.Revisions)-1]
			if rr.PolicyID != policy.ID || rr.TeamID != teamID {
				continue
			}
			projected := responsepolicies.ProjectRotation(rotation, current, in.Signal.OccurredAt)
			for _, shift := range rr.Shifts {
				if !in.Signal.OccurredAt.Before(shift.StartsAt) && in.Signal.OccurredAt.Before(shift.EndsAt) {
					if owner := projected.EffectiveOwnerByShift[shift.ID]; owner != "" {
						recipients = []string{owner}
					}
				}
			}
		}
		out, err := alerts.Create(repo.ID, actor.UserID, in.RequestID, in.Signal, policy, recipients)
		if err == nil && activity != nil {
			for _, delivery := range out.Routing {
				if delivery.Status != "delivered" {
					continue
				}
				target := delivery.RecipientID
				_, activityErr := activity.AppendOnce("response-alert:"+out.ID+":"+target, activities.Event{Kind: "response_alert.routed", ActorID: actor.UserID, RepositoryID: repo.ID, RepositoryName: repo.Name, ResourceType: "response_alert", ResourceID: out.ID, ResourceRevision: in.Signal.SourceRevision, ResourceTitle: in.Signal.Summary, TargetUserID: &target, CreatedAt: out.FirstSeenAt})
				if activityErr != nil {
					log.Printf("response alert inbox projection: %v", activityErr)
				}
			}
		}
		writeResponseAlert(w, out, err, 201)
	})
	mux.HandleFunc("POST /repositories/{id}/response-alerts/{alert_id}/events", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:read")
		if !ok {
			return
		}
		var in responseAlertEventInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a caller-stable alert event is required")
			return
		}
		current, e := alerts.Get(r.PathValue("alert_id"))
		if e != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "response_alert_not_found", "response alert not found")
			return
		}
		allowed := false
		for _, d := range current.Routing {
			if d.RecipientID == actor.UserID && d.Status == "delivered" {
				allowed = true
			}
		}
		out, err := alerts.Append(current.ID, in.RequestID, in.Kind, actor.UserID, in.Reason, allowed)
		writeResponseAlert(w, out, err, 200)
	})
}
func responseAlertIntersects(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}
func alertVisible(v responsealerts.Alert, user string, repositoryOwner bool) bool {
	if v.Signal.AffectedUserCount == 0 {
	}
	for _, d := range v.Routing {
		if d.RecipientID == user {
			return true
		}
	}
	for _, id := range v.AudienceIDs {
		if id == user {
			return true
		}
	}
	if repositoryOwner && (responseAlertContains(v.Diagnostics, "delivery_failed") || v.State == "suppressed" || v.State == "maintenance") {
		return true
	}
	return false
}

func projectResponseAlert(v responsealerts.Alert, user string, current []responsepolicies.Policy) responsealerts.Alert {
	for i := range v.Signal.Evidence {
		evidence := &v.Signal.Evidence[i]
		if len(evidence.AccessibleTo) > 0 && !responseAlertContains(evidence.AccessibleTo, user) {
			evidence.URL = ""
			evidence.Summary = "Restricted evidence is unavailable to this reader."
			evidence.Digest = ""
			evidence.Available = false
			if !responseAlertContains(v.Diagnostics, "inaccessible_evidence") {
				v.Diagnostics = append(v.Diagnostics, "inaccessible_evidence")
			}
		}
	}
	if len(current) > 0 && (current[0].ID != v.PolicyID || current[0].CurrentVersion != v.PolicyVersion) && !responseAlertContains(v.Diagnostics, "policy_changed") {
		v.Diagnostics = append(v.Diagnostics, "policy_changed")
	}
	return v
}
func responseAlertContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
func writeResponseAlert(w http.ResponseWriter, out responsealerts.Alert, err error, status int) {
	switch {
	case err == nil:
		writeJSON(w, status, out)
	case errors.Is(err, responsealerts.ErrNotFound):
		writeAPIError(w, 404, "response_alert_not_found", "response alert not found")
	case errors.Is(err, responsealerts.ErrConflict):
		writeAPIError(w, 409, "response_alert_conflict", "the request identity was reused with different signal or event content")
	case errors.Is(err, responsealerts.ErrForbidden):
		writeAPIError(w, 403, "response_alert_forbidden", "only a successfully routed current responder can act on the alert")
	case errors.Is(err, responsealerts.ErrInvalid):
		writeAPIError(w, 400, "invalid_response_alert", "the signal must match exactly one active policy rule and include complete revision-bound evidence")
	default:
		log.Printf("response alert storage: %v", err)
		writeAPIError(w, 500, "response_alerts_unavailable", "response alert could not be persisted")
	}
}
