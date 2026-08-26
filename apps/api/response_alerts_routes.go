package main

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/activities"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/auth"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/incidents"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/proposals"
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
type responseOutcomeWork struct {
	Kind         string    `json:"kind"`
	Title        string    `json:"title"`
	Outcome      string    `json:"outcome"`
	AssigneeType string    `json:"assignee_type"`
	AssigneeID   string    `json:"assignee_id"`
	DueAt        time.Time `json:"due_at"`
}
type responseOutcomeInput struct {
	responsealerts.OutcomeReviewInput
	Work *responseOutcomeWork `json:"work,omitempty"`
}

func registerResponseAlertRoutes(mux *http.ServeMux, catalog *repositories.Store, credentials *auth.Store, policies *responsepolicies.Store, alerts *responsealerts.Store, incidentStore *incidents.Store, activity *activities.Store, proposalStore *proposals.Store) {
	mux.HandleFunc("POST /repositories/{id}/response-alerts/{alert_id}/responder-load-consents", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:read")
		if !ok {
			return
		}
		current, err := alerts.Get(r.PathValue("alert_id"))
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "response_alert_not_found", "response alert not found")
			return
		}
		var in responsealerts.ResponderLoadConsentInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "an exact responder-authored load consent is required")
			return
		}
		out, consentErr := alerts.ConsentResponderLoad(current.ID, actor.UserID, in, actor.UserID == current.Workspace.ResponderID)
		writeResponseAlert(w, out, consentErr, 201)
	})
	mux.HandleFunc("GET /repositories/{id}/response-outcomes", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryRead(w, r, catalog, credentials, r.PathValue("id"))
		if !ok {
			return
		}
		repo, err := catalog.GetByID(r.PathValue("id"))
		if err != nil || repo.OwnerID != actor.UserID {
			writeAPIError(w, 403, "response_outcomes_forbidden", "only a current repository owner can inspect response outcomes")
			return
		}
		values, err := alerts.List(repo.ID)
		if err != nil {
			writeAPIError(w, 500, "response_outcomes_unavailable", "response outcomes could not be read")
			return
		}
		writeJSON(w, 200, responseOutcomeReport(values, time.Now().UTC()))
	})
	mux.HandleFunc("POST /repositories/{id}/response-alerts/{alert_id}/outcomes", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:write")
		if !ok {
			return
		}
		repo, err := catalog.GetByID(r.PathValue("id"))
		if err != nil || repo.OwnerID != actor.UserID {
			writeAPIError(w, 403, "response_outcomes_forbidden", "only a current repository owner can review response outcomes")
			return
		}
		current, err := alerts.Get(r.PathValue("alert_id"))
		if err != nil || current.RepositoryID != repo.ID {
			writeAPIError(w, 404, "response_alert_not_found", "response alert not found")
			return
		}
		var in responseOutcomeInput
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a caller-stable attributable outcome review is required")
			return
		}
		if responsealerts.ValidateOutcomeReview(in.OutcomeReviewInput) != nil {
			writeAPIError(w, 400, "invalid_response_outcome", "the outcome review is incomplete or includes non-consented data")
			return
		}
		out, reviewErr := alerts.ReviewOutcome(current.ID, actor.UserID, in.OutcomeReviewInput, true)
		if reviewErr != nil {
			writeResponseAlert(w, out, reviewErr, 201)
			return
		}
		if in.Work != nil {
			if proposalStore == nil || (in.Work.Kind != "reliability" && in.Work.Kind != "documentation" && in.Work.Kind != "automation" && in.Work.Kind != "staffing") {
				writeAPIError(w, 400, "invalid_response_work", "linked work must be reliability, documentation, automation, or staffing work")
				return
			}
			p, task, createErr := proposalStore.CreateCorrectiveWork(proposals.CorrectiveWorkInput{ResponseAlertID: current.ID, OperationID: in.RequestID, RepositoryID: repo.ID, ActorID: actor.UserID, ProposalTitle: in.Work.Title, ProposalBody: "Governed " + in.Work.Kind + " follow-up from response alert " + current.ID + ". This work grants no authority beyond ordinary proposal and task controls.", TaskTitle: in.Work.Title, Outcome: in.Work.Outcome, AssigneeType: in.Work.AssigneeType, AssigneeID: in.Work.AssigneeID, BaseRevision: current.Signal.SourceRevision, DueAt: in.Work.DueAt})
			if createErr != nil {
				writeAPIError(w, 409, "response_work_conflict", "ordinary linked work could not be reconciled")
				return
			}
			out, reviewErr = alerts.LinkOutcomeWork(current.ID, actor.UserID, in.RequestID, p.ID, task.ID)
			if reviewErr != nil {
				writeResponseAlert(w, out, reviewErr, 201)
				return
			}
		} else if proposalStore != nil {
			if p, task, findErr := proposalStore.FindResponseCorrectiveWork(current.ID, in.RequestID); findErr == nil {
				out, reviewErr = alerts.LinkOutcomeWork(current.ID, actor.UserID, in.RequestID, p.ID, task.ID)
				if reviewErr != nil {
					writeResponseAlert(w, out, reviewErr, 201)
					return
				}
			}
		}
		writeResponseAlert(w, out, reviewErr, 201)
	})
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
				visible = append(visible, projectResponseAlert(v, actor.UserID, current, owner))
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
		writeJSON(w, 200, projectResponseAlert(v, actor.UserID, current, owner))
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
		backupRecipients := []string{}
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
					for _, backup := range shift.BackupUserIDs {
						if current[backup] {
							backupRecipients = append(backupRecipients, backup)
						}
					}
				}
			}
		}
		directive := alerts.RoutingDirective(repo.ID, responseAlertRuleID(rev, in.Signal))
		if directive == "" {
			directive = automaticResponseRoutingDirective(alerts, repo.ID, responseAlertRuleID(rev, in.Signal), len(backupRecipients) > 0, time.Now().UTC())
		}
		if directive == "backup" && len(backupRecipients) > 0 {
			recipients = backupRecipients[:1]
		}
		out, err := alerts.CreateControlled(repo.ID, actor.UserID, in.RequestID, in.Signal, policy, recipients, directive)
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
		repository, _ := catalog.GetByID(current.RepositoryID)
		participants := map[string]bool{}
		for _, id := range repository.ParticipantIDs() {
			participants[id] = true
		}
		rotations, _ := policies.ListRotations(current.RepositoryID)
		allowed := responseAlertEventAllowed(current, actor.UserID, rotations, participants)
		out, err := alerts.Append(current.ID, in.RequestID, in.Kind, actor.UserID, in.Reason, allowed)
		writeResponseAlert(w, out, err, 200)
	})
	mux.HandleFunc("POST /repositories/{id}/response-alerts/{alert_id}/workspace", func(w http.ResponseWriter, r *http.Request) {
		actor, _, ok := authorizeRepositoryParticipant(w, r, catalog, credentials, r.PathValue("id"), "repositories:read")
		if !ok {
			return
		}
		var in responsealerts.WorkspaceCommand
		if decodeJSON(r, &in) != nil {
			writeAPIError(w, 400, "invalid_request", "a caller-stable bounded workspace command is required")
			return
		}
		current, err := alerts.Get(r.PathValue("alert_id"))
		if err != nil || current.RepositoryID != r.PathValue("id") {
			writeAPIError(w, 404, "response_alert_not_found", "response alert not found")
			return
		}
		allowed := responseWorkspaceParticipant(current, actor.UserID)
		roleIDs := []string{}
		if (in.Kind == "invite" || in.Kind == "reassign") && in.TargetUserID != "" {
			if !responseAlertAudienceMember(current, in.TargetUserID) {
				writeAPIError(w, 422, "invalid_response_audience", "owners and responders must already belong to the alert audience")
				return
			}
			roleIDs = append(roleIDs, in.TargetUserID)
		}
		if in.Kind == "promote_incident" && in.IncidentID == "" {
			if current.Workspace.IncidentID != "" {
				in.IncidentID = current.Workspace.IncidentID
			}
		}
		writeAuthorized := true
		if in.Kind == "promote_incident" && current.Workspace.IncidentID == "" {
			if strings.TrimSpace(in.RequestID) == "" || strings.TrimSpace(in.Message) == "" {
				writeAPIError(w, 400, "invalid_request", "incident promotion requires a caller-stable identity and rationale")
				return
			}
			if !allowed || incidentStore == nil {
				writeAPIError(w, 403, "response_alert_forbidden", "incident declaration is unavailable to this responder")
				return
			}
			if _, _, writeOK := authorizeRepositoryParticipant(w, r, catalog, credentials, current.RepositoryID, "repositories:write"); !writeOK {
				writeAuthorized = false
				return
			}
			roleIDs = append(roleIDs, current.Workspace.ParticipantIDs...)
		}
		if !writeAuthorized {
			return
		}
		var out responsealerts.Alert
		applyErr := catalog.WithIncidentAuthorization(actor.UserID, []string{current.RepositoryID}, roleIDs, func() error {
			locked, readErr := alerts.Get(current.ID)
			if readErr != nil {
				return readErr
			}
			lockedAllowed := responseWorkspaceParticipant(locked, actor.UserID)
			if (in.Kind == "invite" || in.Kind == "reassign") && !responseAlertAudienceMember(locked, in.TargetUserID) {
				return responsealerts.ErrForbidden
			}
			if in.Kind == "promote_incident" && locked.Workspace.IncidentID == "" {
				roles := []incidents.Role{{Name: "incident_commander", UserID: actor.UserID}}
				for _, id := range locked.Workspace.ParticipantIDs {
					if id != actor.UserID {
						roles = append(roles, incidents.Role{Name: "responder_" + id[:8], UserID: id})
					}
				}
				created, createErr := incidentStore.Create(incidents.Incident{Title: locked.Signal.Summary, Summary: "Promoted from response alert " + locked.ID + ". " + strings.TrimSpace(in.Message), Severity: responseIncidentSeverity(locked.Signal.Severity), Status: "investigating", Scopes: []incidents.Scope{{RepositoryID: locked.RepositoryID}}, Roles: roles, DeclaredBy: actor.UserID})
				if createErr != nil {
					return createErr
				}
				in.IncidentID = created.ID
			} else if in.Kind == "promote_incident" {
				in.IncidentID = locked.Workspace.IncidentID
			}
			var mutationErr error
			out, mutationErr = alerts.ApplyWorkspace(locked.ID, actor.UserID, in, lockedAllowed)
			return mutationErr
		})
		if errors.Is(applyErr, repositories.ErrInvalidCollaborator) || errors.Is(applyErr, repositories.ErrNotFound) {
			writeAPIError(w, 404, "response_alert_not_found", "response alert not found")
			return
		}
		if errors.Is(applyErr, incidents.ErrInvalid) {
			writeAPIError(w, 409, "incident_promotion_failed", "the alert could not become an incident")
			return
		}
		writeResponseAlert(w, out, applyErr, 200)
	})
}

func responseAlertEventAllowed(v responsealerts.Alert, actor string, rotations []responsepolicies.Rotation, currentParticipants map[string]bool) bool {
	// Delivery freezes who received the original page. An accepted exact-duty
	// handoff changes who may complete an alert that arose during its historical
	// shift. Later schedule revisions do not rewrite that accepted transfer.
	var transferredTo string
	var transferredAt time.Time
	for _, rotation := range rotations {
		if len(rotation.Revisions) == 0 {
			continue
		}
		for _, event := range rotation.Events {
			if event.Status != "accepted" {
				continue
			}
			namesAlert := false
			for _, context := range event.Context {
				if context.ResourceID == v.ID {
					namesAlert = true
					break
				}
			}
			if !namesAlert || event.RotationVersion < 1 || event.RotationVersion > len(rotation.Revisions) {
				continue
			}
			historical := rotation.Revisions[event.RotationVersion-1]
			if historical.PolicyID != v.PolicyID || historical.TeamID != v.TeamID {
				continue
			}
			for _, shift := range historical.Shifts {
				if shift.ID != event.ShiftID || v.Signal.OccurredAt.Before(shift.StartsAt) || !v.Signal.OccurredAt.Before(shift.EndsAt) {
					continue
				}
				acceptedAt := event.CreatedAt
				if event.AcceptedAt != nil {
					acceptedAt = *event.AcceptedAt
				}
				if transferredTo == "" || acceptedAt.After(transferredAt) {
					transferredTo, transferredAt = event.ToUserID, acceptedAt
				}
			}
		}
	}
	if transferredTo != "" {
		return transferredTo == actor && currentParticipants[actor]
	}
	for _, delivery := range v.Routing {
		if delivery.RecipientID == actor && delivery.Status == "delivered" && currentParticipants[actor] {
			return true
		}
	}
	return false
}

func responseAlertAudienceMember(v responsealerts.Alert, user string) bool {
	for _, id := range v.AudienceIDs {
		if id == user {
			return true
		}
	}
	for _, delivery := range v.Routing {
		if delivery.RecipientID == user && delivery.Status == "delivered" {
			return true
		}
	}
	return false
}

func responseIncidentSeverity(v string) string {
	return map[string]string{"critical": "sev1", "high": "sev2", "medium": "sev3", "low": "sev4"}[v]
}

func responseWorkspaceParticipant(v responsealerts.Alert, user string) bool {
	for _, id := range v.Workspace.ParticipantIDs {
		if id == user {
			return true
		}
	}
	for _, d := range v.Routing {
		if d.RecipientID == user && d.Status == "delivered" {
			return true
		}
	}
	return false
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
	for _, id := range v.Workspace.ParticipantIDs {
		if id == user {
			return true
		}
	}
	if repositoryOwner && (responseAlertContains(v.Diagnostics, "delivery_failed") || v.State == "suppressed" || v.State == "maintenance" || v.State == "routing_paused") {
		return true
	}
	return false
}

func projectResponseAlert(v responsealerts.Alert, user string, current []responsepolicies.Policy, owner bool) responsealerts.Alert {
	if !owner {
		v.OutcomeReviews = []responsealerts.OutcomeReview{}
		consents := v.ResponderLoadConsents[:0]
		for _, consent := range v.ResponderLoadConsents {
			if consent.ResponderID == user {
				consents = append(consents, consent)
			}
		}
		v.ResponderLoadConsents = consents
	}
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
	redactContext := func(values []responsealerts.ContextBinding) {
		for i := range values {
			for j := range values[i].Evidence {
				evidence := &values[i].Evidence[j]
				if len(evidence.AccessibleTo) > 0 && !responseAlertContains(evidence.AccessibleTo, user) {
					evidence.URL, evidence.Digest, evidence.Summary, evidence.Available = "", "", "Restricted evidence is unavailable to this reader.", false
				}
			}
		}
	}
	redactContext(v.Workspace.Context)
	for i := range v.Workspace.Investigations {
		redactContext(v.Workspace.Investigations[i].Context)
	}
	if len(current) > 0 && (current[0].ID != v.PolicyID || current[0].CurrentVersion != v.PolicyVersion) && !responseAlertContains(v.Diagnostics, "policy_changed") {
		v.Diagnostics = append(v.Diagnostics, "policy_changed")
	}
	return v
}

func responseAlertRuleID(rev responsepolicies.Revision, signal responsealerts.Signal) string {
	for _, rule := range rev.Rules {
		if rule.SignalClass == signal.SignalClass && rule.Severity == signal.Severity && responseAlertIntersects(rule.ResourceIDs, signal.ResourceIDs) {
			return rule.ID
		}
	}
	return ""
}

func automaticResponseRoutingDirective(store *responsealerts.Store, repositoryID, ruleID string, hasBackup bool, now time.Time) string {
	values, err := store.List(repositoryID)
	if err != nil {
		return ""
	}
	recentPages, missed := 0, 0
	for _, alert := range values {
		if alert.RuleID != ruleID {
			continue
		}
		if now.Sub(alert.FirstSeenAt) <= 24*time.Hour && len(alert.Routing) > 0 {
			recentPages++
		}
		acknowledged := false
		for _, event := range alert.Events {
			if event.Kind == "acknowledge" {
				acknowledged = true
				break
			}
		}
		if !acknowledged && now.After(alert.AcknowledgeBy) {
			missed++
		}
	}
	if missed >= 2 && hasBackup {
		return "backup"
	}
	if missed >= 2 || recentPages >= 3 {
		return "pause"
	}
	return ""
}

func responseOutcomeReport(values []responsealerts.Alert, now time.Time) map[string]any {
	volume, correlated, falsePositives, handoffs, escalations, missedAck, missedResolve, incidents, interruptions, agentCost := len(values), 0, 0, 0, 0, 0, 0, 0, 0, 0
	ackSeconds, resolveSeconds := []int64{}, []int64{}
	responderLoad := map[string]int{}
	userOutcomes := []map[string]string{}
	individual := []map[string]any{}
	for _, alert := range values {
		if alert.EventCount > 1 {
			correlated += alert.EventCount - 1
		}
		var ack, resolved *time.Time
		for i := range alert.Events {
			event := &alert.Events[i]
			if event.Kind == "acknowledge" && ack == nil {
				ack = &event.CreatedAt
			}
			if event.Kind == "resolve" && resolved == nil {
				resolved = &event.CreatedAt
			}
		}
		if ack != nil {
			ackSeconds = append(ackSeconds, int64(ack.Sub(alert.FirstSeenAt).Seconds()))
		} else if now.After(alert.AcknowledgeBy) {
			missedAck++
		}
		if resolved != nil {
			resolveSeconds = append(resolveSeconds, int64(resolved.Sub(alert.FirstSeenAt).Seconds()))
		} else if now.After(alert.ResolveBy) {
			missedResolve++
		}
		for _, entry := range alert.Workspace.Timeline {
			if entry.Kind == "reassign" || entry.Kind == "invite" {
				handoffs++
			}
			if entry.Kind == "escalate" {
				escalations++
			}
		}
		if alert.Workspace.IncidentID != "" {
			incidents++
		}
		for _, review := range alert.OutcomeReviews {
			if review.Classification == "false_positive" {
				falsePositives++
			}
			agentCost += review.AgentCost
			if review.ResponderLoadConsent {
				interruptions += review.InterruptionMinutes
				for _, consent := range alert.ResponderLoadConsents {
					if consent.ID == review.ResponderLoadConsentID {
						responderLoad[consent.ResponderID] += review.InterruptionMinutes
						break
					}
				}
			}
			if review.UserOutcomeConsent {
				userOutcomes = append(userOutcomes, map[string]string{"alert_id": alert.ID, "outcome": review.UserOutcome})
			}
			individual = append(individual, map[string]any{"alert_id": alert.ID, "review_id": review.ID, "classification": review.Classification, "rationale": review.Rationale, "routing_action": review.RoutingAction, "proposal_id": review.ProposalID, "task_id": review.TaskID, "created_at": review.CreatedAt})
		}
	}
	return map[string]any{"alert_volume": volume, "deduplicated_events": correlated, "false_positives": falsePositives, "handoffs": handoffs, "escalations": escalations, "missed_acknowledgements": missedAck, "missed_resolutions": missedResolve, "acknowledgement_seconds": ackSeconds, "resolution_seconds": resolveSeconds, "consented_interruption_minutes": interruptions, "consented_responder_load": responderLoad, "agent_cost": agentCost, "incidents": incidents, "consented_user_outcomes": userOutcomes, "individual_outcomes": individual}
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
