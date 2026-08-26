package main

import (
	"testing"
	"time"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/responsealerts"
	"github.com/greptile-projects/vivarium-tuatara/apps/api/responsepolicies"
)

func TestResponseOutcomeReportHonorsConsentAndAutomaticContainment(t *testing.T) {
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	store, _ := responsealerts.New(t.TempDir())
	storeNow := now.Add(-2 * time.Hour)
	policy := responsepolicies.Policy{ID: "policy", RepositoryID: "repo", Revisions: []responsepolicies.Revision{{Version: 1, Rules: []responsepolicies.Rule{{ID: "rule", ResourceIDs: []string{"service"}, SignalClass: "reliability", Severity: "critical", AccountableTeamID: "ops", AcknowledgeSeconds: 60, ResolveSeconds: 600}}}}}
	signal := responsealerts.Signal{SignalClass: "reliability", Severity: "critical", ResourceIDs: []string{"service"}, Summary: "failure", Uncertainty: "sampled", OccurredAt: storeNow, SourceRevision: "0123456789012345678901234567890123456789", Evidence: []responsealerts.Evidence{{Kind: "check", ResourceID: "check", Revision: "run", Digest: "digest", Summary: "restricted", Available: true}}}
	for _, request := range []string{"one", "two"} {
		if _, err := store.Create("repo", "source", request, signal, policy, []string{"responder"}); err != nil {
			t.Fatal(err)
		}
		signal.CorrelationKey = request
	}
	values, _ := store.List("repo")
	if automaticResponseRoutingDirective(store, "repo", "rule", true, now) != "backup" {
		t.Fatal("missed acknowledgements did not activate a declared backup")
	}
	review := responsealerts.OutcomeReviewInput{RequestID: "review", Classification: "false_positive", ResponderLoadConsent: false, UserOutcomeConsent: false, AgentCost: 4, Rationale: "noise"}
	if _, err := store.ReviewOutcome(values[0].ID, "owner", review, true); err != nil {
		t.Fatal(err)
	}
	report := responseOutcomeReport(mustAlerts(t, store, "repo"), now)
	if report["consented_interruption_minutes"].(int) != 0 || report["agent_cost"].(int) != 4 || report["missed_acknowledgements"].(int) != 2 {
		t.Fatalf("report=%#v", report)
	}
	if _, leaked := report["alerts"]; leaked {
		t.Fatal("outcome report copied restricted alert evidence")
	}
}

func TestResponseOutcomeReportCreditsImmutableConsentAuthorAfterReassignment(t *testing.T) {
	now := time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC)
	store, _ := responsealerts.New(t.TempDir())
	policy := responsepolicies.Policy{ID: "policy", RepositoryID: "repo", Revisions: []responsepolicies.Revision{{Version: 1, Rules: []responsepolicies.Rule{{ID: "rule", ResourceIDs: []string{"service"}, SignalClass: "reliability", Severity: "critical", AccountableTeamID: "ops", AcknowledgeSeconds: 60, ResolveSeconds: 600}}}}}
	signal := responsealerts.Signal{SignalClass: "reliability", Severity: "critical", ResourceIDs: []string{"service"}, Summary: "failure", Uncertainty: "sampled", OccurredAt: now, SourceRevision: "0123456789012345678901234567890123456789", CorrelationKey: "attribution", Evidence: []responsealerts.Evidence{{Kind: "check", ResourceID: "check", Revision: "run", Digest: "digest", Summary: "failure", Available: true}}}
	alert, _ := store.Create("repo", "source", "attribution", signal, policy, []string{"responder-a"})
	consented, _ := store.ConsentResponderLoad(alert.ID, "responder-a", responsealerts.ResponderLoadConsentInput{RequestID: "consent", InterruptionMinutes: 17}, true)
	consent := consented.ResponderLoadConsents[0]
	_, _ = store.ReviewOutcome(alert.ID, "owner", responsealerts.OutcomeReviewInput{RequestID: "review", Classification: "actionable", Rationale: "outcome", InterruptionMinutes: 17, ResponderLoadConsentID: consent.ID}, true)
	_, _ = store.ApplyWorkspace(alert.ID, "responder-a", responsealerts.WorkspaceCommand{RequestID: "reassign", Kind: "reassign", Message: "handoff", TargetUserID: "responder-b"}, true)
	report := responseOutcomeReport(mustAlerts(t, store, "repo"), now)
	load := report["consented_responder_load"].(map[string]int)
	if load["responder-a"] != 17 || load["responder-b"] != 0 {
		t.Fatalf("load followed reassignment: %#v", load)
	}
}

func mustAlerts(t *testing.T, store *responsealerts.Store, repositoryID string) []responsealerts.Alert {
	t.Helper()
	values, err := store.List(repositoryID)
	if err != nil {
		t.Fatal(err)
	}
	return values
}

func TestResponseAlertFallbackVisibilityRequiresRepositoryOwner(t *testing.T) {
	values := []responsealerts.Alert{
		{State: "open", Diagnostics: []string{"delivery_failed"}},
		{State: "suppressed"},
		{State: "maintenance"},
	}
	for _, value := range values {
		if alertVisible(value, "outsider", false) {
			t.Fatalf("outsider can see %s fallback", value.State)
		}
		if !alertVisible(value, "owner", true) {
			t.Fatalf("owner cannot see %s fallback", value.State)
		}
	}
	routed := responsealerts.Alert{State: "open", Routing: []responsealerts.Delivery{{RecipientID: "responder"}}}
	if !alertVisible(routed, "responder", false) || alertVisible(routed, "outsider", false) {
		t.Fatal("routing audience visibility is incorrect")
	}
}

func TestRoutingPausedAlertFallbackVisibilityRequiresOwner(t *testing.T) {
	paused := responsealerts.Alert{State: "routing_paused"}
	if !alertVisible(paused, "owner", true) || alertVisible(paused, "reader", false) {
		t.Fatal("routing pause was hidden from its owner or disclosed to a reader")
	}
}

func TestResponseWorkspaceAudienceAndIncidentSeverityBoundaries(t *testing.T) {
	alert := responsealerts.Alert{AudienceIDs: []string{"audience"}, Routing: []responsealerts.Delivery{{RecipientID: "responder", Status: "delivered"}}}
	if !responseAlertAudienceMember(alert, "audience") || !responseAlertAudienceMember(alert, "responder") || responseAlertAudienceMember(alert, "collaborator-only") {
		t.Fatal("workspace invitation escaped the frozen alert audience")
	}
	want := map[string]string{"critical": "sev1", "high": "sev2", "medium": "sev3", "low": "sev4"}
	for alertSeverity, incidentSeverity := range want {
		if got := responseIncidentSeverity(alertSeverity); got != incidentSeverity {
			t.Fatalf("%s mapped to %s", alertSeverity, got)
		}
	}
}

func TestAcceptedDutyHandoffTransfersActiveAlertEventAuthority(t *testing.T) {
	now := time.Now().UTC()
	alert := responsealerts.Alert{ID: "alert", PolicyID: "policy", TeamID: "service", Signal: responsealerts.Signal{OccurredAt: now}, Routing: []responsealerts.Delivery{{RecipientID: "primary", Status: "delivered"}}}
	rotation := responsepolicies.Rotation{
		Revisions: []responsepolicies.RotationRevision{
			{PolicyID: "policy", TeamID: "service", Shifts: []responsepolicies.Shift{{ID: "active", StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour), PrimaryUserID: "primary", BackupUserIDs: []string{"backup"}}}},
			{PolicyID: "policy", TeamID: "service", Shifts: []responsepolicies.Shift{{ID: "successor", StartsAt: now.Add(time.Hour), EndsAt: now.Add(2 * time.Hour), PrimaryUserID: "primary", BackupUserIDs: []string{"backup"}}}},
		},
		Events:         []responsepolicies.DutyEvent{{Kind: "delegate", ShiftID: "active", FromUserID: "primary", ToUserID: "backup", Status: "accepted", RotationVersion: 1, Context: []responsepolicies.DutyContext{{Kind: "active_alert", ResourceID: "alert", Revision: "4", Summary: "continuing response"}}}},
		CurrentVersion: 2,
	}
	participants := map[string]bool{"primary": true, "backup": true}
	if responseAlertEventAllowed(alert, "primary", []responsepolicies.Rotation{rotation}, participants) {
		t.Fatal("former primary retained active-alert event authority after accepted handoff")
	}
	if !responseAlertEventAllowed(alert, "backup", []responsepolicies.Rotation{rotation}, participants) {
		t.Fatal("accepted effective owner could not complete the active alert after schedule revision")
	}
	unrelated := alert
	unrelated.ID = "unrelated-alert"
	if !responseAlertEventAllowed(unrelated, "primary", []responsepolicies.Rotation{rotation}, participants) || responseAlertEventAllowed(unrelated, "backup", []responsepolicies.Rotation{rotation}, participants) {
		t.Fatal("handoff transferred active-alert authority beyond its frozen context")
	}
}
