package apicontracts

import (
	"testing"
	"time"
)

func TestContractMigrationReadinessRequiresAcknowledgedTestedZeroTraffic(t *testing.T) {
	now := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	v := ContractMigration{
		Applications: []MigrationApplication{{ApplicationID: "app", OwnerID: "owner", ConsumerRepositoryID: "consumer"}},
		Stages:       []MigrationStage{{ID: "sunset", Name: "Sunset", Deadline: now.Add(24 * time.Hour), RequiredEvidence: "passing dual-version candidate and zero old traffic", ObservationMaxAgeHours: 24, MaxRemainingRequests: 0}},
		CurrentStage: "sunset",
	}
	apps := map[string]Application{"app": {ID: "app", Status: "approved"}}
	observations := map[string][]OperationalObservation{"app": {{WindowEndedAt: now, Requests: 4}}}
	out := ProjectContractMigration(v, apps, nil, observations, now)
	if out.Readiness.Ready || len(out.Readiness.Blockers) != 3 {
		t.Fatalf("expected acknowledgement, test, and traffic blockers: %#v", out.Readiness)
	}

	candidate := IntegrationCandidate{ID: "candidate", Scenarios: []IntegrationScenario{{Name: "provider"}, {Name: "consumer"}}, Evidence: []IntegrationEvidence{{Scenario: "provider", Status: "passed"}, {Scenario: "consumer", Status: "passed"}}}
	v.Acknowledgements = []MigrationAcknowledgement{{ApplicationID: "app"}}
	v.Attestations = []MigrationAttestation{{ApplicationID: "app", IntegrationWorkID: "work", CandidateID: "candidate"}}
	work := map[string][]IntegrationWork{"app": {{ID: "work", Candidates: []IntegrationCandidate{candidate}}}}
	observations["app"] = []OperationalObservation{{WindowEndedAt: now, Requests: 0}}
	out = ProjectContractMigration(v, apps, work, observations, now)
	if !out.Readiness.Ready || !out.Readiness.Consumers[0].Tested {
		t.Fatalf("passing exact candidate and zero traffic should be ready: %#v", out.Readiness)
	}

	apps["app"] = Application{ID: "app", Status: "revoked"}
	out = ProjectContractMigration(v, apps, work, observations, now)
	if out.Readiness.Ready || out.Readiness.Consumers[0].AccessState != "revoked" {
		t.Fatal("revoked application access must remain an explicit retirement blocker")
	}
}

func TestContractMigrationExceptionIsBoundedAlternativeToTest(t *testing.T) {
	now := time.Now().UTC()
	v := ContractMigration{Applications: []MigrationApplication{{ApplicationID: "app", OwnerID: "owner"}}, Stages: []MigrationStage{{ID: "dual", Name: "Dual", Deadline: now.Add(time.Hour), RequiredEvidence: "test or exception", ObservationMaxAgeHours: 24, MaxRemainingRequests: 10}}, CurrentStage: "dual", Acknowledgements: []MigrationAcknowledgement{{ApplicationID: "app"}}, Exceptions: []MigrationException{{ApplicationID: "app", ExpiresAt: now.Add(time.Hour)}}}
	observations := map[string][]OperationalObservation{"app": {{WindowEndedAt: now, Requests: 2}}}
	out := ProjectContractMigration(v, map[string]Application{"app": {ID: "app", Status: "approved"}}, nil, observations, now)
	if !out.Readiness.Ready || out.Readiness.Consumers[0].ExceptionUntil == nil {
		t.Fatalf("active bounded exception should stage migration without claiming a test: %#v", out.Readiness)
	}
	out = ProjectContractMigration(v, map[string]Application{"app": {ID: "app", Status: "approved"}}, nil, observations, now.Add(2*time.Hour))
	if out.Readiness.Ready {
		t.Fatal("expired exception must stop satisfying migration readiness")
	}
}

func TestContractMigrationPersistsCASPolicyWithoutConsumers(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	created, err := store.CreateContractMigration(ContractMigration{RepositoryID: "provider", ContractID: "contract", FromVersion: 1, ToVersion: 2, Kind: "new_version", EvolutionID: "evolution", CreatedBy: "owner", Changes: []MigrationChange{{Kind: "removed_field", Summary: "legacy field is removed", Classification: "breaking"}}, Stages: []MigrationStage{{ID: "dual", Name: "Dual run", Deadline: now.Add(24 * time.Hour), RequiredEvidence: "current usage and conformance", ObservationMaxAgeHours: 24, MaxRemainingRequests: 10}}})
	if err != nil || created.Version != 1 || created.State != "planned" {
		t.Fatalf("create = %#v, %v", created, err)
	}
	updated, err := store.MutateContractMigration(created.ID, 1, func(v *ContractMigration) error { v.State = "active"; return nil })
	if err != nil || updated.Version != 2 {
		t.Fatalf("update = %#v, %v", updated, err)
	}
	if _, err = store.MutateContractMigration(created.ID, 1, func(v *ContractMigration) error { return nil }); err != ErrConflict {
		t.Fatalf("stale update = %v", err)
	}
	listed, err := store.ListContractMigrations("provider", "contract")
	if err != nil || len(listed) != 1 || listed[0].State != "active" {
		t.Fatalf("list = %#v, %v", listed, err)
	}
}
