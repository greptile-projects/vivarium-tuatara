package infrastructure

import (
	"testing"
	"time"
)

func TestInfrastructureRehearsalRequiresCompleteSafeCoverageAndRetainsRuns(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	resource := Resource{ID: "api", Kind: "service", Name: "API", Description: "candidate", OwnerIDs: []string{"owner"}, Provider: "cloud", ProviderAccess: "participant", DependsOn: []string{}, Configuration: []ConfigurationBoundary{}, Constraints: []Constraint{}, Commitments: Commitments{}}
	plan := ChangePlan{ID: "plan", RepositoryID: "repo", PullRequestID: "pull", SourceRevision: "revision", Candidate: Revision{Resources: []Resource{resource}}, Changes: []PlanChange{{ResourceID: "api", Action: "change", After: &resource}}, Rehearsals: []Rehearsal{}}
	if err := s.lock(func() error { return s.writePlan(plan) }); err != nil {
		t.Fatal(err)
	}
	checks := []RehearsalCheck{}
	for _, kind := range []string{"provisioning", "connectivity", "access", "policy", "service_journey", "failure", "cost", "teardown", "recovery"} {
		checks = append(checks, RehearsalCheck{ID: kind, Kind: kind, Command: "./verify " + kind, ResourceIDs: []string{"api"}, Expectation: kind + " succeeds"})
	}
	in := Rehearsal{Name: "ephemeral candidate", Scope: RehearsalScope{EnvironmentKind: "isolated", EnvironmentID: "preview-1", CredentialResourceIDs: []string{"api"}, CredentialExpiresAt: now.Add(time.Hour), StateKind: "synthetic", StateDescription: "generated request state"}, Checks: checks}
	plan, rehearsal, err := s.CreateRehearsal("plan", "owner", in)
	if err != nil || len(plan.Rehearsals) != 1 || rehearsal.PlanSourceRevision != "revision" {
		t.Fatalf("create rehearsal = %#v, %v", rehearsal, err)
	}
	outcomes := []RehearsalOutcome{}
	for _, c := range checks {
		outcomes = append(outcomes, RehearsalOutcome{CheckID: c.ID, Kind: c.Kind, Status: "passed"})
	}
	plan, run, err := s.AddRehearsalRun("plan", rehearsal.ID, "owner", RehearsalRun{WorkspaceID: "workspace", Result: "passed", Outcomes: outcomes, ResourceGraph: plan.Candidate.Resources})
	if err != nil || run.ID == "" || len(plan.Rehearsals[0].Runs) != 1 {
		t.Fatalf("add run = %#v, %v", run, err)
	}
	bad := in
	bad.Checks = bad.Checks[:len(bad.Checks)-1]
	if _, _, err = s.CreateRehearsal("plan", "owner", bad); err != ErrInvalid {
		t.Fatalf("incomplete coverage = %v", err)
	}
}

func TestInfrastructureRehearsalLabelsDestructiveEffectsUnsupported(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	resource := Resource{ID: "db", Kind: "data_store", Name: "DB", OwnerIDs: []string{"owner"}, Provider: "cloud", ProviderAccess: "participant"}
	plan := ChangePlan{ID: "destructive", RepositoryID: "repo", SourceRevision: "revision", Candidate: Revision{Resources: []Resource{}}, Changes: []PlanChange{{ResourceID: "db", Action: "destroy", Before: &resource}}}
	_ = s.lock(func() error { return s.writePlan(plan) })
	checks := []RehearsalCheck{}
	for _, kind := range []string{"provisioning", "connectivity", "access", "policy", "service_journey", "failure", "cost", "teardown", "recovery"} {
		checks = append(checks, RehearsalCheck{ID: kind, Kind: kind, Command: kind, ResourceIDs: []string{"db"}, Expectation: "bounded"})
	}
	in := Rehearsal{Name: "destructive boundary", Scope: RehearsalScope{EnvironmentKind: "isolated", EnvironmentID: "preview", CredentialResourceIDs: []string{"db"}, CredentialExpiresAt: now.Add(time.Hour), StateKind: "synthetic", StateDescription: "generated"}, Checks: checks}
	if _, _, err := s.CreateRehearsal(plan.ID, "owner", in); err != ErrInvalid {
		t.Fatalf("destructive effect represented as rehearsed = %v", err)
	}
	in.UnsupportedEffects = []UnsupportedEffect{{ResourceID: "db", Effect: "authoritative provider deletion", Reason: "ephemeral deletion cannot prove durable production data effects"}}
	if _, _, err := s.CreateRehearsal(plan.ID, "owner", in); err != nil {
		t.Fatalf("explicit unsupported effect = %v", err)
	}
}
