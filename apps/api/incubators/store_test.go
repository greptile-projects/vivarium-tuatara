package incubators

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func fixture() Incubator {
	return Incubator{Title: "Shared developer onboarding", Audience: "Developers adopting the platform", Problem: "Teams cannot explore a shared project need before choosing a repository", DesiredOutcome: "Collaborators agree on the outcome and authority first", Constraints: []string{"No repository required"}, SuccessMeasures: []string{"Every sponsor consents"}, SponsorIDs: []string{"human-a"}, DecisionRights: []DecisionRight{{Kind: "scope_change", Decision: "Change the desired outcome", PrincipalIDs: []string{"human-a"}, Rule: "owner"}}, Visibility: "participants", Source: Source{Kind: "new_idea", Label: "A new project idea", Resolution: "resolved"}}
}

func TestResearchComparisonExperimentAndSupersessionRemainReproducible(t *testing.T) {
	s, _ := New(t.TempDir())
	x, e := s.Create(fixture(), "human-a", nil)
	if e != nil {
		t.Fatal(e)
	}
	x, e = s.AddAlternative(x.ID, "human", "human-a", 1, []ResearchSource{{Kind: "public", Label: "Upstream benchmark", URL: "https://example.test/benchmark", Resolution: "resolved"}}, Alternative{Name: "Adopt a service", ProductBoundary: "Own orchestration, adopt storage", Architecture: "Stateless API over managed storage", Interfaces: []string{"HTTP API"}, Dependencies: []string{"managed-store"}, Licenses: []string{"Apache-2.0 client"}, OperatingCosts: []string{"$200/month estimate"}, SecurityRisks: []string{"supplier compromise"}, DataRisks: []string{"regional residency"}, BuildOrAdopt: "hybrid", Unknowns: []string{"peak latency"}})
	if e != nil || len(x.ResearchSources) != 1 || len(x.Alternatives) != 1 {
		t.Fatalf("alternative = %#v, %v", x, e)
	}
	a := x.Alternatives[0]
	x, e = s.AddExperiment(x.ID, "human", "human-a", 2, ExperimentDefinition{AlternativeID: a.ID, Question: "Does latency fit the budget?", Environment: "ephemeral local container", Commands: []string{"benchmark --fixture synthetic.json"}, Inputs: []string{"synthetic request shape v1"}, ExpectedMeasures: []string{"p95 latency ms"}, SafetyLimits: []string{"no network writes"}, SourceIDs: a.SourceIDs})
	if e != nil || len(x.Experiments) != 1 || len(x.Experiments[0].DefinitionSHA256) != 64 || !strings.Contains(x.Experiments[0].Authority, "no_code_or_infrastructure_authority") {
		t.Fatalf("experiment = %#v, %v", x.Experiments, e)
	}
	x, e = s.AddExperimentResult(x.ID, x.Experiments[0].ID, "human", "human-a", 3, ExperimentResult{Outcome: "inconclusive", Measurements: []Measurement{{Name: "p95 latency", Value: 82, Unit: "ms"}}, ArtifactSHA256: []string{strings.Repeat("a", 64)}, Unknowns: []string{"production network variance"}})
	if e != nil {
		t.Fatal(e)
	}
	x, e = s.AddResearchNote(x.ID, "human", "human-a", 4, ResearchNote{Kind: "assumption", Body: "Traffic remains below 100 rps", AlternativeID: a.ID, SourceIDs: a.SourceIDs})
	if e != nil {
		t.Fatal(e)
	}
	old := x.ResearchNotes[0]
	x, e = s.AddResearchNote(x.ID, "human", "human-a", 5, ResearchNote{Kind: "dissent", Body: "The traffic assumption is not supported", AlternativeID: a.ID, SourceIDs: a.SourceIDs, SupersedesID: old.ID})
	if e != nil || !x.ResearchNotes[0].Superseded || x.ResearchNotes[1].Kind != "dissent" {
		t.Fatalf("supersession = %#v, %v", x.ResearchNotes, e)
	}
	if _, e = s.AddExperiment(x.ID, "human", "human-a", 5, ExperimentDefinition{}); e != ErrConflict {
		t.Fatalf("stale write = %v", e)
	}
	if _, e = s.AddExperimentResult(x.ID, x.Experiments[0].ID, "human", "human-a", 6, ExperimentResult{Outcome: "passed", Notes: strings.Repeat("n", 10001)}); e != ErrInvalid {
		t.Fatalf("oversized result note = %v", e)
	}
}

func TestBootstrapPreviewApprovalActivationAndRollbackAreAtomic(t *testing.T) {
	s, _ := New(t.TempDir())
	x, _ := s.Create(fixture(), "human-a", nil)
	x, _ = s.AddAlternative(x.ID, "human", "human-a", 1, nil, Alternative{Name: "Accepted direction", ProductBoundary: "Developer service", Architecture: "API and web", Interfaces: []string{"HTTP"}, Dependencies: []string{"Bun"}, Licenses: []string{"MIT"}, OperatingCosts: []string{"$10/month"}, SecurityRisks: []string{"credentials"}, DataRisks: []string{"user data"}, BuildOrAdopt: "build", Unknowns: []string{"scale"}})
	kinds := []string{"organization", "repository", "team", "package", "agent_role", "contributor_pathway", "documentation", "environment", "review_policy", "security_policy", "privacy_policy", "quality_policy", "release_policy"}
	resources := make([]BootstrapResource, 0, len(kinds))
	for _, kind := range kinds {
		resources = append(resources, BootstrapResource{Kind: kind, Mode: "create", Name: "project-" + kind, OwnerIDs: []string{"human-a"}, EffectiveAccess: []string{"fabricated:admin"}, MonthlyCostEstimateCents: 100, GeneratedContent: []string{"fabricated baseline"}, InheritedPolicies: []string{"fabricated policy"}})
	}
	x, e := s.PreviewBootstrap(x.ID, "human-a", 2, x.Alternatives[0].ID, resources)
	if e != nil || x.BootstrapPlans[0].RecurringCostEstimateCents != 1300 || x.BootstrapPlans[0].Status != "preview" || len(x.BootstrapPlans[0].Resources[0].ResourceID) != 32 || x.BootstrapPlans[0].Resources[0].EffectiveAccess[0] == "fabricated:admin" || x.BootstrapPlans[0].Resources[0].CostBasis != "participant_estimate_unverified" {
		t.Fatalf("preview = %#v, %v", x.BootstrapPlans, e)
	}
	p := x.BootstrapPlans[0]
	x, e = s.DecideBootstrap(x.ID, p.ID, "human-a", "approved", 3, 1)
	if e != nil || x.BootstrapPlans[0].Status != "approved" {
		t.Fatalf("approval = %#v, %v", x.BootstrapPlans, e)
	}
	x, e = s.FinishBootstrap(x.ID, p.ID, "human-a", "activate", 4, 2)
	if e != nil || x.BootstrapPlans[0].Status != "active" || x.BootstrapPlans[0].ActivatedAt == nil {
		t.Fatalf("activation = %#v, %v", x.BootstrapPlans, e)
	}
	if _, e = s.FinishBootstrap(x.ID, p.ID, "human-a", "rollback", 5, 3); e != ErrInvalid {
		t.Fatalf("active rollback = %v", e)
	}
}

func TestDeliveryPlanConnectsOrderedMixedWorkAndAttributableEvidence(t *testing.T) {
	s, _ := New(t.TempDir())
	x, _ := s.Create(fixture(), "human-a", []Invitation{{PrincipalType: "agent", PrincipalID: "agent-a", OrganizationID: "org-a", Role: "builder"}})
	x, _ = s.AddAlternative(x.ID, "human", "human-a", 1, nil, Alternative{Name: "Running slice", ProductBoundary: "Journey", Architecture: "Web and API", Interfaces: []string{"HTTP"}, Dependencies: []string{"runtime"}, Licenses: []string{"MIT"}, OperatingCosts: []string{"bounded"}, SecurityRisks: []string{"access"}, DataRisks: []string{"feedback"}, BuildOrAdopt: "build", Unknowns: []string{"adoption"}})
	resources := []BootstrapResource{}
	for kind := range bootstrapKinds {
		resource := BootstrapResource{Kind: kind, Mode: "create", Name: "slice-" + kind, OwnerIDs: []string{"human-a"}}
		if kind == "repository" {
			resource.Mode, resource.ResourceID = "connect", "repo-a"
		}
		resources = append(resources, resource)
	}
	x, _ = s.PreviewBootstrap(x.ID, "human-a", 2, x.Alternatives[0].ID, resources)
	p := x.BootstrapPlans[0]
	x, _ = s.DecideBootstrap(x.ID, p.ID, "human-a", "approved", 3, 1)
	x, _ = s.FinishBootstrap(x.ID, p.ID, "human-a", "activate", 4, 2)
	kinds := []string{"code", "tests", "documentation", "infrastructure", "interface"}
	items := []DeliveryWorkItem{}
	for i, kind := range kinds {
		deps := []string{}
		if i > 0 {
			deps = []string{fmt.Sprintf("%d", i)}
		}
		items = append(items, DeliveryWorkItem{Kind: kind, Title: "Deliver " + kind, RepositoryID: "repo-a", BaseRevision: strings.Repeat("a", 40), OwnerType: []string{"human", "agent"}[i%2], OwnerID: []string{"human-a", "agent-a"}[i%2], DependencyIDs: deps, Acceptance: []string{"ordinary check passes"}, IntegrationOrder: i + 1})
	}
	x, e := s.CreateDeliveryPlan(x.ID, "human-a", 5, DeliveryPlan{BootstrapPlanID: p.ID, Journey: "An invited developer exercises the running slice", Participants: []DeliveryParticipant{{PrincipalType: "human", PrincipalID: "human-a", Role: "lead"}, {PrincipalType: "agent", PrincipalID: "agent-a", Role: "builder"}}, WorkItems: items})
	if e != nil || len(x.DeliveryPlans) != 1 || x.DeliveryPlans[0].WorkItems[1].DependencyIDs[0] != x.DeliveryPlans[0].WorkItems[0].ID {
		t.Fatalf("delivery plan = %#v, %v", x.DeliveryPlans, e)
	}
	d := x.DeliveryPlans[0]
	x, e = s.AddDeliveryReport(x.ID, d.ID, "agent", "agent-a", 6, 1, DeliveryReport{WorkItemID: d.WorkItems[0].ID, Kind: "agent_action", Revision: strings.Repeat("b", 40), Summary: "Implemented the bounded slice", CostCents: 25})
	if e != nil || x.DeliveryPlans[0].Reports[0].ActorID != "agent-a" || x.DeliveryPlans[0].Reports[0].RepositoryID != "repo-a" || x.DeliveryPlans[0].Version != 2 {
		t.Fatalf("report = %#v, %v", x.DeliveryPlans[0], e)
	}
	if _, e = s.AddDeliveryReport(x.ID, d.ID, "agent", "agent-a", 7, 2, DeliveryReport{WorkItemID: d.WorkItems[0].ID, Kind: "review", RepositoryID: "unrelated-repository", ResourceID: "pull:review", Revision: strings.Repeat("b", 40), Summary: "Review from unrelated scope"}); e != ErrInvalid {
		t.Fatalf("cross-repository evidence = %v", e)
	}
}

func TestDeliveryPlanRejectsNonAdmittedWorkOwner(t *testing.T) {
	s, _ := New(t.TempDir())
	x, _ := s.Create(fixture(), "human-a", nil)
	x.BootstrapPlans = []BootstrapPlan{{ID: "bootstrap-a", Status: "active", Resources: []BootstrapResource{{Kind: "repository", ResourceID: "repo-a"}}}}
	if e := s.write(&x); e != nil {
		t.Fatal(e)
	}
	items := []DeliveryWorkItem{}
	for i, kind := range []string{"code", "tests", "documentation", "infrastructure", "interface"} {
		items = append(items, DeliveryWorkItem{Kind: kind, Title: kind, RepositoryID: "repo-a", BaseRevision: strings.Repeat("a", 40), OwnerType: "agent", OwnerID: "agent-b", Acceptance: []string{"passes"}, IntegrationOrder: i + 1})
	}
	_, e := s.CreateDeliveryPlan(x.ID, "human-a", 1, DeliveryPlan{BootstrapPlanID: "bootstrap-a", Journey: "Running slice", Participants: []DeliveryParticipant{{PrincipalType: "human", PrincipalID: "human-a", Role: "lead"}, {PrincipalType: "agent", PrincipalID: "agent-b", Role: "builder"}}, WorkItems: items})
	if e != ErrInvalid {
		t.Fatalf("non-admitted owner persisted: %v", e)
	}
}

func TestConsentAttributionVisibilityAndCAS(t *testing.T) {
	s, e := New(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	x, e := s.Create(fixture(), "human-a", []Invitation{{PrincipalType: "human", PrincipalID: "human-b", Role: "co-designer"}})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Get(x.ID, "stranger"); e != ErrNotFound {
		t.Fatalf("private read = %v", e)
	}
	x, e = s.Consent(x.ID, x.Invitations[0].ID, "human-b", "accepted", 1)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.AddEvent(x.ID, "human", "human-b", 2, Event{Kind: "scope_change", Body: "Replace the audience", Visibility: "participants"}); e != ErrInvalid {
		t.Fatalf("undeclared decision right = %v", e)
	}
	x, e = s.AddEvent(x.ID, "human", "human-b", 2, Event{Kind: "assumption", Body: "The first audience is internal platform teams", Visibility: "participants"})
	if e != nil {
		t.Fatal(e)
	}
	if x.Events[len(x.Events)-1].ActorID != "human-b" {
		t.Fatal("event attribution lost")
	}
	if _, e = s.AddEvent(x.ID, "human", "human-b", 2, Event{Kind: "discussion", Body: "stale", Visibility: "participants"}); e != ErrConflict {
		t.Fatalf("stale write = %v", e)
	}
}

func TestPotentialDuplicatesAreReportedNotCollapsed(t *testing.T) {
	s, _ := New(t.TempDir())
	a := fixture()
	first, e := s.Create(a, "human-a", nil)
	if e != nil {
		t.Fatal(e)
	}
	second, e := s.Create(a, "human-a", nil)
	if e != nil {
		t.Fatal(e)
	}
	if first.ID == second.ID || len(second.PotentialDuplicates) != 1 || second.PotentialDuplicates[0].IncubatorID != first.ID {
		t.Fatalf("duplicates not explicit: %#v", second.PotentialDuplicates)
	}
}

func TestScopeChangeUsesOnlyTypedRightAndEvaluatesMajority(t *testing.T) {
	s, _ := New(t.TempDir())
	x := fixture()
	x.DecisionRights = []DecisionRight{{Kind: "project_update", Decision: "Publish updates", PrincipalIDs: []string{"human-b"}, Rule: "majority"}}
	x, e := s.Create(x, "human-a", []Invitation{{PrincipalType: "human", PrincipalID: "human-b", Role: "publisher"}})
	if e != nil {
		t.Fatal(e)
	}
	x, _ = s.Consent(x.ID, x.Invitations[0].ID, "human-b", "accepted", 1)
	if _, e = s.AddEvent(x.ID, "human", "human-b", 2, Event{Kind: "scope_change", Body: "Serve external teams", Visibility: "participants"}); e != ErrInvalid {
		t.Fatalf("unrelated right authorized scope: %v", e)
	}

	y := fixture()
	y.DecisionRights = []DecisionRight{{Kind: "scope_change", Decision: "Change scope", PrincipalIDs: []string{"human-a", "human-b", "human-c"}, Rule: "majority"}}
	y, e = s.Create(y, "human-a", []Invitation{{PrincipalType: "human", PrincipalID: "human-b", Role: "designer"}, {PrincipalType: "human", PrincipalID: "human-c", Role: "sponsor"}})
	if e != nil {
		t.Fatal(e)
	}
	y, _ = s.Consent(y.ID, y.Invitations[0].ID, "human-b", "accepted", 1)
	y, _ = s.Consent(y.ID, y.Invitations[1].ID, "human-c", "accepted", 2)
	y, e = s.AddEvent(y.ID, "human", "human-b", 3, Event{Kind: "decision_support", DecisionKind: "scope_change", Body: "Serve external teams", Visibility: "participants"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.AddEvent(y.ID, "human", "human-c", 4, Event{Kind: "scope_change", Body: "Serve external teams", Visibility: "participants"}); e != nil {
		t.Fatalf("majority scope change rejected: %v", e)
	}
}

func TestVisibilityChangeRequiresConfiguredDecision(t *testing.T) {
	s, _ := New(t.TempDir())
	x := fixture()
	x.DecisionRights = append(x.DecisionRights, DecisionRight{Kind: "visibility_change", Decision: "Publish incubator", PrincipalIDs: []string{"human-b"}, Rule: "owner"})
	x, e := s.Create(x, "human-a", []Invitation{{PrincipalType: "human", PrincipalID: "human-b", Role: "publication owner"}})
	if e != nil {
		t.Fatal(e)
	}
	x, e = s.Consent(x.ID, x.Invitations[0].ID, "human-b", "accepted", 1)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.AddEvent(x.ID, "human", "human-a", 2, Event{Kind: "visibility_change", Body: "public", Visibility: "participants"}); e != ErrInvalid {
		t.Fatalf("creator bypassed visibility owner: %v", e)
	}
	x, e = s.AddEvent(x.ID, "human", "human-b", 2, Event{Kind: "visibility_change", Body: "public", Visibility: "participants"})
	if e != nil || x.Visibility != "public" {
		t.Fatalf("declared visibility owner failed: %#v, %v", x, e)
	}
	if _, e = s.Get(x.ID, "outsider"); e != nil {
		t.Fatalf("published incubator not readable: %v", e)
	}
}

func TestPendingAndDeclinedInvitationsDoNotGrantContextVisibility(t *testing.T) {
	s, _ := New(t.TempDir())
	x, e := s.Create(fixture(), "human-a", []Invitation{{PrincipalType: "human", PrincipalID: "human-b", Role: "invited designer"}})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Get(x.ID, "human-b"); e != ErrNotFound {
		t.Fatalf("pending detail read = %v", e)
	}
	if listed, e := s.List("human-b"); e != nil || len(listed) != 0 {
		t.Fatalf("pending list = %#v, %v", listed, e)
	}
	x, e = s.Consent(x.ID, x.Invitations[0].ID, "human-b", "declined", 1)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Get(x.ID, "human-b"); e != ErrNotFound {
		t.Fatalf("declined detail read = %v", e)
	}
	if listed, e := s.List("human-b"); e != nil || len(listed) != 0 {
		t.Fatalf("declined list = %#v, %v", listed, e)
	}
}

func TestPostRenameSyncFailureReturnsCommittedUncertainState(t *testing.T) {
	s, _ := New(t.TempDir())
	calls := 0
	s.syncDir = func() error {
		calls++
		if calls == 1 {
			return errors.New("injected directory sync failure")
		}
		return nil
	}
	x, e := s.Create(fixture(), "human-a", nil)
	if e != nil || !x.DurabilityUncertain {
		t.Fatalf("create = %#v, %v", x, e)
	}
	stored, e := s.Get(x.ID, "human-a")
	if e != nil || !stored.DurabilityUncertain {
		t.Fatalf("stored marker = %#v, %v", stored, e)
	}
	x, e = s.AddEvent(x.ID, "human", "human-a", 1, Event{Kind: "discussion", Body: "Retry-safe follow-up", Visibility: "participants"})
	if e != nil || x.DurabilityUncertain {
		t.Fatalf("marker did not clear after synced mutation: %#v, %v", x, e)
	}
}
