package extensions

import (
	"strings"
	"testing"
	"time"
)

func TestOperationsExposeFailuresConsumptionPermissionUseAndNotices(t *testing.T) {
	s, _ := New(t.TempDir())
	owner, repo := strings.Repeat("a", 32), strings.Repeat("b", 32)
	registration := Registration{Name: "Review lens", OperatorContact: "ops@example.test", Capabilities: []string{"review"}, CallbackURL: "https://example.test/events", ActionURL: "https://example.test/actions", RequestedPermissions: []Permission{{Resource: "pull_requests", Actions: []string{"read", "write"}}}, SupportedEvents: []string{"pull_request.opened"}, CredentialRotation: RotationPolicy{IntervalDays: 30}}
	ext, _ := s.Create(owner, registration, time.Now())
	installation, _ := s.CreateInstallation(ext.ID, owner, InstallationInput{OwnerType: "repository", OwnerID: repo, RepositoryIDs: []string{repo}, ResourceTypes: []string{"pull_requests"}, CapabilityDecisions: []CapabilityDecision{{Capability: "review", Decision: "approved"}}, Settings: map[string]string{}})
	deliveries, _ := s.EnqueueProjectEvent(ProjectEvent{ID: strings.Repeat("c", 32), Type: "pull_request.opened", RepositoryID: repo, ResourceType: "pull_requests", ResourceID: strings.Repeat("d", 32), ActorID: owner, OccurredAt: time.Now().Add(time.Second)})
	for range 5 {
		_, _ = s.RecordDeliveryAttempt(installation.ID, deliveries[0].ID, "failed", 503, "unavailable")
	}
	registration.Capabilities = []string{"review", "new capability"}
	_, _ = s.UpdateContract(ext.ID, owner, 1, registration, time.Now())
	o, err := s.InspectOperations(installation.ID)
	if err != nil || o.Deliveries != 1 || o.Failures != 5 || o.DeadLetters != 1 || o.PermissionUse["events:pull_request.opened"] != 1 || len(o.Notices) < 2 {
		t.Fatalf("operations=%#v error=%v", o, err)
	}
}
