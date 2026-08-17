package apicontracts

import (
	"testing"
	"time"
)

func TestOperationalEvidenceAndInvestigationFreezeApplicationContract(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	expires := now.Add(time.Hour)
	app := Application{ID: "app-1", RepositoryID: "provider", ContractID: "contract-1", ContractVersion: 3, OwnerID: "consumer", Status: "approved", ApprovalExpiresAt: &expires, Environments: []string{"production"}}
	observation, err := s.AddOperationalObservation(app, "consumer", OperationalObservation{Environment: "production", ReleaseID: "release-7", WindowStartedAt: now.Add(-time.Hour), WindowEndedAt: now, Requests: 100, Available: 97, Errors: 3, SchemaValid: 99, LatencyP95MS: 240, QuotaRejected: 1, UsageUnits: 100, ErrorCodes: []string{"upstream_timeout"}, Sanitization: "aggregate counts; identifiers and payloads removed", Visibility: "shared"})
	if err != nil {
		t.Fatal(err)
	}
	if observation.ContractVersion != 3 || observation.ApplicationID != app.ID {
		t.Fatalf("observation not frozen to application: %#v", observation)
	}
	inv, err := s.CreateAPIInvestigation(app, "consumer", "Production timeouts", []string{observation.ID})
	if err != nil {
		t.Fatal(err)
	}
	if inv.ContractVersion != 3 || len(inv.ObservationIDs) != 1 {
		t.Fatalf("investigation lost provenance: %#v", inv)
	}
	other := observation
	other.ID = "unrelated"
	if _, err = s.CreateAPIInvestigation(app, "consumer", "cross-app evidence", []string{other.ID}); err != ErrInvalid {
		t.Fatalf("unretained evidence accepted: %v", err)
	}
}

func TestOperationalEvidenceRejectsSecretsAndImpossibleAggregates(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	app := Application{ID: "app", ContractID: "contract", ContractVersion: 1, Status: "approved", Environments: []string{"production"}}
	base := OperationalObservation{Environment: "production", ReleaseID: "r1", WindowStartedAt: now.Add(-time.Minute), WindowEndedAt: now, Requests: 2, Available: 2, SchemaValid: 2, Sanitization: "payloads removed", Visibility: "shared"}
	bad := base
	bad.Sanitization = "Authorization: Bearer vva_leaked"
	if _, err := s.AddOperationalObservation(app, "owner", bad); err != ErrInvalid {
		t.Fatalf("secret accepted: %v", err)
	}
	bad = base
	bad.Errors = 3
	if _, err := s.AddOperationalObservation(app, "owner", bad); err != ErrInvalid {
		t.Fatalf("impossible aggregate accepted: %v", err)
	}
}
