package adoptionworkspaces

import "testing"

func fixture() Workspace {
	return Workspace{Title: "Adopt relay", Outcome: "Deliver events reliably", Source: Source{Kind: "package", RepositoryID: "repo", ResourceID: "relay@2.0.0", Label: "Relay package", Resolution: "resolved"}, RequiredJourneys: []string{"publish and replay"}, Environments: []string{"linux"}, Constraints: []string{"no payload retention"}, BudgetCents: 10000, Currency: "USD", Owners: []Owner{{PrincipalID: "owner", Responsibility: "decision"}}, Criteria: []Criterion{{Name: "journey fit", Requirement: "passes replay", Weight: 100, OwnerID: "owner"}}, Candidates: []Candidate{{Name: "Relay", Provider: "provider", Version: "2.0.0", SourceKind: "package", SourceReference: "relay@2.0.0", Evidence: []Evidence{{Dimension: "capabilities", Summary: "ordered events", Reference: "https://example.test/capabilities", ObservedVersion: "2.0.0", Visibility: "public", Resolution: "resolved"}, {Dimension: "support", Summary: "older policy", Reference: "https://example.test/support", ObservedVersion: "1.0.0", Visibility: "participants", Resolution: "resolved"}}}}}
}

func TestWorkspaceConsentAndEvidenceGaps(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	x, err := s.Create(fixture(), "owner", []Invitation{{PrincipalType: "human", PrincipalID: "maintainer", Role: "provider_maintainer"}, {PrincipalType: "agent", PrincipalID: "reader", OrganizationID: "org", Role: "observer"}})
	if err != nil {
		t.Fatal(err)
	}
	if x.Candidates[0].FitStatus != "undetermined" || x.Candidates[0].Evidence[1].Resolution != "stale" {
		t.Fatalf("expected version-derived gaps, got %+v", x.Candidates[0])
	}
	maintainer := Viewer{PrincipalType: "human", PrincipalID: "maintainer"}
	if _, err = s.Get(x.ID, maintainer); !errorsIs(err, ErrNotFound) {
		t.Fatalf("pending invite exposed workspace: %v", err)
	}
	pending, err := s.Pending(maintainer)
	if err != nil || len(pending) != 1 || pending[0].WorkspaceID != x.ID {
		t.Fatalf("pending invitation not discoverable: %+v, %v", pending, err)
	}
	var invitation string
	for _, v := range x.Invitations {
		if v.PrincipalID == "maintainer" {
			invitation = v.ID
		}
	}
	x, err = s.Consent(x.ID, invitation, "maintainer", "accepted", x.Version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Get(x.ID, maintainer); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Get(x.ID, Viewer{PrincipalType: "agent", PrincipalID: "maintainer", OrganizationID: "org"}); !errorsIs(err, ErrNotFound) {
		t.Fatalf("human invitation authorized colliding agent: %v", err)
	}
	if _, err = s.Get(x.ID, Viewer{PrincipalType: "agent", PrincipalID: "reader", OrganizationID: "org"}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Get(x.ID, Viewer{PrincipalType: "human", PrincipalID: "reader"}); !errorsIs(err, ErrNotFound) {
		t.Fatalf("agent invitation authorized colliding human: %v", err)
	}
}

func TestRepositoryEvidenceIsRedactedForParticipantWithoutAccess(t *testing.T) {
	s, _ := New(t.TempDir())
	s.ConfigureRepositoryAccess(func(viewer Viewer, repository string) bool {
		return viewer.PrincipalID == "owner" && repository == "private-repo"
	})
	in := fixture()
	in.Candidates[0].Evidence[0].RepositoryID = "private-repo"
	in.Candidates[0].Evidence[0].Summary = "SECRET_PRIVATE_EVIDENCE"
	x, err := s.Create(in, "owner", []Invitation{{PrincipalType: "human", PrincipalID: "participant", Role: "affected_user"}})
	if err != nil {
		t.Fatal(err)
	}
	invitation := x.Invitations[0].ID
	x, err = s.Consent(x.ID, invitation, "participant", "accepted", x.Version)
	if err != nil {
		t.Fatal(err)
	}
	got := x.Candidates[0].Evidence[0]
	if got.Resolution != "inaccessible" || got.RepositoryID != "" || got.Summary != "Restricted evidence" || got.Reference != "Restricted evidence" {
		t.Fatalf("private evidence leaked: %+v", got)
	}
}

func TestAgentCannotReceiveWriteRole(t *testing.T) {
	s, _ := New(t.TempDir())
	_, err := s.Create(fixture(), "owner", []Invitation{{PrincipalType: "agent", PrincipalID: "agent", OrganizationID: "org", Role: "provider_maintainer"}})
	if !errorsIs(err, ErrInvalid) {
		t.Fatalf("expected invalid, got %v", err)
	}
}

func TestBoundedTrialRetainsFailedEvidenceAndRejectsCredentials(t *testing.T) {
	s, _ := New(t.TempDir())
	x, err := s.Create(fixture(), "owner", []Invitation{{PrincipalType: "agent", PrincipalID: "trial-agent", OrganizationID: "org", Role: "observer"}})
	if err != nil {
		t.Fatal(err)
	}
	viewer := Viewer{PrincipalType: "agent", PrincipalID: "trial-agent", OrganizationID: "org"}
	trial := TrialDefinition{CandidateID: x.Candidates[0].ID, Source: TrialSource{Kind: "exact_revision", RepositoryID: "repo", ResourceID: "0123456789012345678901234567890123456789", Revision: "0123456789012345678901234567890123456789", Resolution: "resolved"}, Packages: []string{"relay@2.0.0"}, APIs: []string{"events/v2"}, DataKind: "synthetic", DataDescription: "generated ordered events", Journeys: []string{"publish and replay"}, Policies: []string{"no payload retention"}, Setup: []string{"create an empty fixture"}, Configuration: []string{"retention=off"}, Commands: []string{"relay verify fixture.json"}, IntegrationChanges: []string{"wire the test adapter"}, MaximumCostCents: 500}
	x, err = s.CreateTrial(x.ID, trial, viewer, x.Version)
	if err != nil {
		t.Fatal(err)
	}
	x, err = s.RecordTrialAttempt(x.ID, x.Trials[0].ID, TrialAttempt{Status: "failed", Reproducible: true, Checks: []string{"replay check failed"}, Measurements: []string{"p95=31ms"}, CostCents: 120, Findings: []string{"duplicates after reconnect"}}, viewer, x.Version)
	if err != nil {
		t.Fatal(err)
	}
	if len(x.Trials[0].Attempts) != 1 || x.Trials[0].Attempts[0].Status != "failed" {
		t.Fatalf("failed trial disappeared: %+v", x.Trials)
	}
	for _, credential := range []string{"Authorization: Bearer secret", "Authorization: Basic dXNlcjpwYXNz", "github_pat_11ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"} {
		bad := trial
		bad.Commands = []string{"curl -H '" + credential + "'"}
		if _, err = s.CreateTrial(x.ID, bad, viewer, x.Version); !errorsIs(err, ErrInvalid) {
			t.Fatalf("credential-shaped command %q accepted: %v", credential, err)
		}
	}
}

func TestTrialCostCeilingIsCumulative(t *testing.T) {
	s, _ := New(t.TempDir())
	x, _ := s.Create(fixture(), "owner", nil)
	v := Viewer{PrincipalType: "human", PrincipalID: "owner"}
	trial := TrialDefinition{CandidateID: x.Candidates[0].ID, Source: TrialSource{Kind: "exact_revision", ResourceID: "0123456789012345678901234567890123456789", Revision: "0123456789012345678901234567890123456789", Resolution: "resolved"}, DataKind: "synthetic", DataDescription: "fixture", Journeys: []string{"publish and replay"}, Policies: []string{"retention"}, Setup: []string{"setup"}, Configuration: []string{"config"}, Commands: []string{"check"}, IntegrationChanges: []string{"adapter"}, MaximumCostCents: 100}
	x, err := s.CreateTrial(x.ID, trial, v, x.Version)
	if err != nil {
		t.Fatal(err)
	}
	attempt := TrialAttempt{Status: "failed", CostCents: 60, Findings: []string{"retryable failure"}}
	x, err = s.RecordTrialAttempt(x.ID, x.Trials[0].ID, attempt, v, x.Version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.RecordTrialAttempt(x.ID, x.Trials[0].ID, attempt, v, x.Version); !errorsIs(err, ErrInvalid) {
		t.Fatalf("cumulative cost overage accepted: %v", err)
	}
}

func TestTrialMustUseDeclaredJourneyAndBudget(t *testing.T) {
	s, _ := New(t.TempDir())
	x, _ := s.Create(fixture(), "owner", nil)
	v := Viewer{PrincipalType: "human", PrincipalID: "owner"}
	trial := TrialDefinition{CandidateID: x.Candidates[0].ID, Source: TrialSource{Kind: "exact_revision", ResourceID: "0123456789012345678901234567890123456789", Revision: "0123456789012345678901234567890123456789", Resolution: "resolved"}, DataKind: "permitted", DataDescription: "approved anonymized fixture", Journeys: []string{"undeclared journey"}, Policies: []string{"retention"}, Setup: []string{"setup"}, Configuration: []string{"config"}, Commands: []string{"check"}, IntegrationChanges: []string{"adapter"}}
	if _, err := s.CreateTrial(x.ID, trial, v, x.Version); !errorsIs(err, ErrInvalid) {
		t.Fatalf("undeclared journey accepted: %v", err)
	}
}

func TestTrialSourceIsRedactedAfterRepositoryAccessLoss(t *testing.T) {
	s, _ := New(t.TempDir())
	allowed := true
	s.ConfigureRepositoryAccess(func(Viewer, string) bool { return allowed })
	x, _ := s.Create(fixture(), "owner", nil)
	v := Viewer{PrincipalType: "human", PrincipalID: "owner"}
	trial := TrialDefinition{CandidateID: x.Candidates[0].ID, Source: TrialSource{Kind: "exact_revision", RepositoryID: "private", ResourceID: "0123456789012345678901234567890123456789", Revision: "0123456789012345678901234567890123456789", Resolution: "resolved"}, DataKind: "synthetic", DataDescription: "fixture", Journeys: []string{"publish and replay"}, Policies: []string{"retention"}, Setup: []string{"setup"}, Configuration: []string{"config"}, Commands: []string{"check"}, IntegrationChanges: []string{"adapter"}}
	x, err := s.CreateTrial(x.ID, trial, v, x.Version)
	if err != nil {
		t.Fatal(err)
	}
	allowed = false
	x, err = s.Get(x.ID, v)
	if err != nil {
		t.Fatal(err)
	}
	if x.Trials[0].Source.Resolution != "inaccessible" || x.Trials[0].Source.Revision != "" || x.Trials[0].Source.ResourceID != "" {
		t.Fatalf("restricted trial source leaked: %+v", x.Trials[0].Source)
	}
}

func TestSuccessfulTrialCreatesOwnedOrderedAdoptionAgreement(t *testing.T) {
	s, _ := New(t.TempDir())
	x, _ := s.Create(fixture(), "owner", []Invitation{{PrincipalType: "human", PrincipalID: "maintainer", Role: "provider_maintainer"}})
	x, _ = s.Consent(x.ID, x.Invitations[0].ID, "maintainer", "accepted", x.Version)
	v := Viewer{PrincipalType: "human", PrincipalID: "owner"}
	trial := TrialDefinition{CandidateID: x.Candidates[0].ID, Source: TrialSource{Kind: "exact_revision", ResourceID: "0123456789012345678901234567890123456789", Revision: "0123456789012345678901234567890123456789", Resolution: "resolved"}, DataKind: "synthetic", DataDescription: "fixture", Journeys: []string{"publish and replay"}, Policies: []string{"retention"}, Setup: []string{"setup"}, Configuration: []string{"config"}, Commands: []string{"check"}, IntegrationChanges: []string{"adapter"}, MaximumCostCents: 100}
	x, _ = s.CreateTrial(x.ID, trial, v, x.Version)
	x, _ = s.RecordTrialAttempt(x.ID, x.Trials[0].ID, TrialAttempt{Status: "passed", Reproducible: true, Checks: []string{"journey passed"}}, Viewer{PrincipalType: "human", PrincipalID: "maintainer"}, x.Version)
	plan := validAdoptionPlan(x)
	s.ConfigureEnvironmentResolver(func(repositoryID, environmentID string) bool {
		return repositoryID == "consumer" && environmentID == "env-live"
	})
	plan.Work[1].Kind, plan.Work[1].EnvironmentID = "environment", "env-live"
	bad := plan
	bad.Work = append([]AdoptionWork(nil), plan.Work...)
	bad.Work[1].EnvironmentID = "missing-or-cross-repository"
	if _, err := s.CreatePlan(x.ID, bad, v, x.Version); !errorsIs(err, ErrInvalid) {
		t.Fatalf("unresolved environment accepted: %v", err)
	}
	x, err := s.CreatePlan(x.ID, plan, v, x.Version)
	if err != nil {
		t.Fatal(err)
	}
	if len(x.Plans) != 1 || len(x.Plans[0].Work) != 2 || len(x.Plans[0].Work[1].DependencyIDs) != 1 || x.Plans[0].Work[1].DependencyIDs[0] != x.Plans[0].Work[0].ID || x.Plans[0].Work[0].Authority != "no_authority_granted" {
		t.Fatalf("agreement did not retain ordered authority-safe work: %+v", x.Plans)
	}
	s.ConfigurePlanTargetProjection(func(_ Viewer, work AdoptionWork) AdoptionWork {
		work.EffectiveAccess, work.RepositoryID, work.OwnerID = "inaccessible", "restricted", "restricted"
		return work
	})
	projected, _ := s.Get(x.ID, v)
	if projected.Plans[0].Work[0].EffectiveAccess != "inaccessible" || projected.Plans[0].Work[0].RepositoryID != "restricted" || projected.Plans[0].Work[0].OwnerID != "restricted" {
		t.Fatalf("current target projection leaked stale facts: %+v", projected.Plans[0].Work[0])
	}
}

func TestAdoptionAgreementRequiresIndependentlyReproducedPass(t *testing.T) {
	s, _ := New(t.TempDir())
	x, _ := s.Create(fixture(), "owner", nil)
	v := Viewer{PrincipalType: "human", PrincipalID: "owner"}
	trial := TrialDefinition{CandidateID: x.Candidates[0].ID, Source: TrialSource{Kind: "exact_revision", ResourceID: "0123456789012345678901234567890123456789", Revision: "0123456789012345678901234567890123456789", Resolution: "resolved"}, DataKind: "synthetic", DataDescription: "fixture", Journeys: []string{"publish and replay"}, Policies: []string{"retention"}, Setup: []string{"setup"}, Configuration: []string{"config"}, Commands: []string{"check"}, IntegrationChanges: []string{"adapter"}}
	x, _ = s.CreateTrial(x.ID, trial, v, x.Version)
	x, _ = s.RecordTrialAttempt(x.ID, x.Trials[0].ID, TrialAttempt{Status: "passed", Reproducible: true, Checks: []string{"self asserted"}}, v, x.Version)
	plan := validAdoptionPlan(x)
	if _, err := s.CreatePlan(x.ID, plan, v, x.Version); !errorsIs(err, ErrInvalid) {
		t.Fatalf("unproven agreement accepted: %v", err)
	}
}

func validAdoptionPlan(x Workspace) AdoptionPlan {
	return AdoptionPlan{CandidateID: x.Candidates[0].ID, TrialID: x.Trials[0].ID, SelectedVersion: "2.0.0", IntegrationArchitecture: "Consumer adapter calls the provider API", ConfigurationOwnership: []DecisionOwner{{Decision: "Consumer retry policy", OwnerID: "owner", Party: "adopter"}}, UpdatePolicy: "Monthly review; security updates within seven days", SupportPolicy: "Provider triages protocol defects; adopter owns local operations", ServiceBoundaries: []string{"Provider ends at the public API"}, DataBoundaries: []string{"No payload retention"}, RequiredExceptions: []string{"Temporary retry-policy exception tracked by consumer"}, ExitStrategy: "Remove the adapter and replay queued events", UnresolvedFitGaps: []string{"Windows remains unverified"}, CompatibilityPromises: []string{"Provider supports events/v2 through 2027"}, RecurringCostCents: 2500, Currency: "USD", Work: []AdoptionWork{{Position: 1, Kind: "consumer_repository", Title: "Implement adapter", RepositoryID: "consumer", Paths: []string{"src/adapter"}, OwnerType: "human", OwnerID: "owner", OwnerStatus: "current", AcceptanceCriteria: []string{"Journey passes"}, EffectiveAccess: "owner"}, {Position: 2, Kind: "documentation", Title: "Document support boundary", RepositoryID: "consumer", Paths: []string{"docs/relay.md"}, OwnerType: "human", OwnerID: "owner", OwnerStatus: "current", AcceptanceCriteria: []string{"Owners approve"}, EffectiveAccess: "owner"}}}
}

func errorsIs(got, want error) bool { return got == want }
