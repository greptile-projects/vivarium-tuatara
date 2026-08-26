package main

import (
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/responsealerts"
)

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
