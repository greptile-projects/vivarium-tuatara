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
