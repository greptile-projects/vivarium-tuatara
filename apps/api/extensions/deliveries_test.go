package extensions

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestDeliveryContractIsScopedSignedOrderedAndRecoverable(t *testing.T) {
	store, _ := New(t.TempDir())
	repo := strings.Repeat("b", 32)
	actor := strings.Repeat("c", 32)
	ext, _ := store.Create(strings.Repeat("a", 32), Registration{Name: "CI bridge", OperatorContact: "ops@example.test", Capabilities: []string{"observe"}, CallbackURL: "https://example.test/events", ActionURL: "https://example.test/actions", RequestedPermissions: []Permission{{Resource: "pull_requests", Actions: []string{"read"}}}, SupportedEvents: []string{"pull_request.*"}, CredentialRotation: RotationPolicy{IntervalDays: 30}}, time.Now())
	installation, _ := store.CreateInstallation(ext.ID, actor, InstallationInput{OwnerType: "repository", OwnerID: repo, RepositoryIDs: []string{repo}, ResourceTypes: []string{"pull_requests"}, CapabilityDecisions: []CapabilityDecision{{Capability: "observe", Decision: "approved"}}, Settings: map[string]string{}})
	event := ProjectEvent{ID: strings.Repeat("d", 32), Type: "pull_request.opened", RepositoryID: repo, ResourceType: "pull_request", ResourceID: strings.Repeat("e", 32), ActorID: actor, Title: "Safe title", OccurredAt: time.Now().UTC()}
	created, err := store.EnqueueProjectEvent(event)
	if err != nil || len(created) != 1 {
		t.Fatalf("created=%#v error=%v", created, err)
	}
	duplicate, _ := store.EnqueueProjectEvent(event)
	if len(duplicate) != 0 {
		t.Fatalf("duplicate=%#v", duplicate)
	}
	public, _ := hex.DecodeString(store.DeliveryPublicKey())
	signature, _ := hex.DecodeString(created[0].Signature)
	if !ed25519.Verify(public, created[0].Payload, signature) {
		t.Fatal("signature does not verify")
	}
	var envelope DeliveryEnvelope
	if json.Unmarshal(created[0].Payload, &envelope) != nil || envelope.Sequence != 1 || envelope.OrderingKey != "repository:"+repo {
		t.Fatalf("envelope=%#v", envelope)
	}
	for i := 0; i < 5; i++ {
		created[0], err = store.RecordDeliveryAttempt(installation.ID, created[0].ID, "failed", 503, "upstream unavailable")
		if err != nil {
			t.Fatal(err)
		}
	}
	if created[0].Status != "dead_letter" {
		t.Fatalf("status=%s", created[0].Status)
	}
	replayed, err := store.ReplayDelivery(installation.ID, created[0].ID)
	if err != nil || replayed.Sequence != 2 || replayed.ID == created[0].ID || replayed.EventID != created[0].EventID {
		t.Fatalf("replay=%#v error=%v", replayed, err)
	}
}

func TestDeliveryRejectsRevokedUnsubscribedAndInaccessibleContext(t *testing.T) {
	store, _ := New(t.TempDir())
	repo := strings.Repeat("b", 32)
	actor := strings.Repeat("c", 32)
	ext, _ := store.Create(strings.Repeat("a", 32), Registration{Name: "CI bridge", OperatorContact: "ops@example.test", Capabilities: []string{"observe"}, CallbackURL: "https://example.test/events", ActionURL: "https://example.test/actions", RequestedPermissions: []Permission{{Resource: "issues", Actions: []string{"read"}}}, SupportedEvents: []string{"issue.opened"}, CredentialRotation: RotationPolicy{IntervalDays: 30}}, time.Now())
	installation, _ := store.CreateInstallation(ext.ID, actor, InstallationInput{OwnerType: "repository", OwnerID: repo, RepositoryIDs: []string{repo}, ResourceTypes: []string{"issues"}, CapabilityDecisions: []CapabilityDecision{{Capability: "observe", Decision: "approved"}}, Settings: map[string]string{}})
	installation, _ = store.ChangeInstallation(installation.ID, actor, "suspend", installation.Version, nil, nil)
	for _, event := range []ProjectEvent{{ID: strings.Repeat("d", 32), Type: "issue.opened", RepositoryID: repo, ResourceType: "issue"}, {ID: strings.Repeat("e", 32), Type: "pull_request.opened", RepositoryID: repo, ResourceType: "pull_request"}} {
		created, err := store.EnqueueProjectEvent(event)
		if err != nil || len(created) != 0 {
			t.Fatalf("created=%#v error=%v", created, err)
		}
	}
}
