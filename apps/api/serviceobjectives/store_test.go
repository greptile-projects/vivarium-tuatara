package serviceobjectives

import (
	"errors"
	"testing"
	"time"
)

func TestReliabilityEvidenceRetainsExactMappingHistoryAndDerivesAttainment(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return time.Date(2026, 8, 14, 20, 0, 0, 0, time.UTC) }
	contract, err := s.Create("repo", "owner", completeRevision())
	if err != nil {
		t.Fatal(err)
	}
	mapping := SignalMappingRevision{ContractVersion: 1, ObjectiveID: "availability", InstrumentationRevision: "otel-v1", Calculation: "ratio", Unit: "percent", Rationale: "Connect release telemetry", Sources: []SignalSource{
		{Kind: "metric", Name: "Successful requests", Reference: "metrics://requests/success", Visibility: "public", Sanitization: "aggregate counts only"},
		{Kind: "log", Name: "Failure classes", Reference: "logs://service/failures", Visibility: "participants", Sanitization: "messages and user fields removed"},
		{Kind: "trace", Name: "Critical journey", Reference: "traces://journey/checkout", Visibility: "public", Sanitization: "span names and duration only"},
	}}
	contract, err = s.PublishMapping(contract.ID, "owner", mapping)
	if err != nil {
		t.Fatal(err)
	}
	mappingID := contract.SignalMappings[0].ID
	observation := Observation{MappingID: mappingID, MappingVersion: 1, ContractVersion: 1, ObjectiveID: "availability", WindowStart: s.now().Add(-time.Hour), WindowEnd: s.now(), GoodEvents: 995, TotalEvents: 1000, Uncertainty: .2, Summary: "Release window aggregate", Gaps: []EvidenceGap{{Kind: "support_delay", Detail: "Support reports can arrive late."}}, Software: []SoftwareReference{{Kind: "release", ID: "release-1", Revision: "abc123", Label: "v1.2.0"}, {Kind: "deployment", ID: "deploy-1", Revision: "abc123", Label: "production"}, {Kind: "commit", ID: "abc123", Revision: "abc123", Label: "checkout fix"}, {Kind: "pull_request", ID: "42", Revision: "abc123", Label: "Reduce retries"}, {Kind: "package", ID: "web", Revision: "1.2.0", Label: "web package"}, {Kind: "dependent_service", ID: "payments", Revision: "api-v3", Label: "Payments"}}}
	contract, err = s.RecordObservation(contract.ID, "owner", observation)
	if err != nil {
		t.Fatal(err)
	}
	got := contract.Observations[0]
	if got.Attainment == nil || *got.Attainment != 99.5 || got.TargetMet == nil || *got.TargetMet || got.ErrorBudgetConsumed == nil || *got.ErrorBudgetConsumed != 500 {
		t.Fatalf("derived observation = %#v", got)
	}
	mapping.InstrumentationRevision = "otel-v2"
	mapping.Rationale = "Replace counter cardinality"
	contract, err = s.ReviseMapping(contract.ID, mappingID, 1, "owner", mapping)
	if err != nil {
		t.Fatal(err)
	}
	observation.MappingVersion = 2
	observation.WindowStart = observation.WindowEnd
	observation.WindowEnd = observation.WindowEnd.Add(time.Hour)
	contract, err = s.RecordObservation(contract.ID, "owner", observation)
	if err != nil {
		t.Fatal(err)
	}
	if contract.Observations[1].ComparableToPrevious || contract.Observations[1].ComparisonReason == "" {
		t.Fatalf("instrumentation change was silently compared: %#v", contract.Observations[1])
	}
	public := s.ProjectForReader(contract, false)
	if public.SignalMappings[0].Revisions[0].Sources[1].Reference != "restricted" {
		t.Fatalf("restricted source leaked: %#v", public.SignalMappings[0])
	}
	if len(public.Observations[0].Software) != 6 || public.Observations[0].Gaps[0].Kind != "support_delay" {
		t.Fatalf("public provenance missing: %#v", public.Observations[0])
	}
}

func TestReliabilityEvidenceRejectsCredentials(t *testing.T) {
	s, _ := New(t.TempDir())
	contract, _ := s.Create("repo", "owner", completeRevision())
	for _, reference := range []string{"https://metrics/?api_key=leak", "https://metrics/?access_token=leak", "https://metrics/?access_token%3dleak", "https://metrics/?access_token%3dleak%zz", "https://metrics/?access%5ftoken%253dleak", "https://metrics/?token=leak", "Authorization: Basic leak", "Cookie: session=leak", "X-API-Key: leak"} {
		_, err := s.PublishMapping(contract.ID, "owner", SignalMappingRevision{ContractVersion: 1, ObjectiveID: "availability", InstrumentationRevision: "v1", Calculation: "ratio", Unit: "percent", Rationale: "connect", Sources: []SignalSource{{Kind: "metric", Name: "requests", Reference: reference, Visibility: "public", Sanitization: "aggregate"}}})
		if !errors.Is(err, ErrInvalid) {
			t.Errorf("credential-bearing reference %q error = %v", reference, err)
		}
	}
	contract.SignalMappings = []SignalMapping{{ID: "legacy", CurrentVersion: 1, Revisions: []SignalMappingRevision{{Version: 1, Sources: []SignalSource{{Kind: "metric", Name: "requests", Reference: "https://metrics/?access_token%3dlegacy%zz", Visibility: "public", Sanitization: "aggregate"}}}}}}
	if got := s.ProjectForReader(contract, false).SignalMappings[0].Revisions[0].Sources[0].Reference; got != "redacted_unsafe_reference" {
		t.Fatalf("legacy credential projection = %q", got)
	}
}

func TestReliabilityInvestigationRetainsEvidenceDisputeOwnerInputAndStaleness(t *testing.T) {
	s, _ := New(t.TempDir())
	c, _ := s.Create("repo", "owner", completeRevision())
	c, _ = s.PublishMapping(c.ID, "owner", SignalMappingRevision{ContractVersion: 1, ObjectiveID: "availability", InstrumentationRevision: "v1", Calculation: "ratio", Unit: "percent", Rationale: "aggregate", Sources: []SignalSource{{Kind: "metric", Name: "success", Reference: "metrics://success", Visibility: "public", Sanitization: "aggregate"}}})
	now := time.Now().UTC()
	c, _ = s.RecordObservation(c.ID, "owner", Observation{MappingID: c.SignalMappings[0].ID, MappingVersion: 1, ContractVersion: 1, ObjectiveID: "availability", WindowStart: now.Add(-2 * time.Hour), WindowEnd: now.Add(-time.Hour), GoodEvents: 999, TotalEvents: 1000, Summary: "baseline", Uncertainty: 1})
	c, _ = s.RecordObservation(c.ID, "owner", Observation{MappingID: c.SignalMappings[0].ID, MappingVersion: 1, ContractVersion: 1, ObjectiveID: "availability", WindowStart: now.Add(-time.Hour), WindowEnd: now, GoodEvents: 990, TotalEvents: 1000, Summary: "affected", Uncertainty: 3, Software: []SoftwareReference{{Kind: "pull_request", ID: "42", Revision: "abc123", Label: "retry change"}}})
	c, err := s.OpenInvestigation(c.ID, "agent:reader", Investigation{ContractVersion: 1, ObjectiveID: "availability", Title: "Checkout may be degrading", Trigger: InvestigationTrigger{Kind: "pull_request", ID: "42", Revision: "abc123"}, BaselineObservationIDs: []string{c.Observations[0].ID}, AffectedObservationIDs: []string{c.Observations[1].ID}, JourneyIDs: []string{"buy"}, Evidence: []InvestigationEvidence{{Kind: "code", ResourceID: "diff-42", Revision: "abc123", Label: "retry change", Visibility: "public"}, {Kind: "trace", ResourceID: "trace-window", Revision: "window-2", Label: "sanitized critical path", Visibility: "participants"}}})
	if err != nil {
		t.Fatal(err)
	}
	x := c.Investigations[0]
	c, err = s.MutateInvestigation(c.ID, x.ID, "agent:reader", "agent", "finding", x.Version, InvestigationFinding{Kind: "hypothesis", Statement: "Retries correlate with lower completion.", Uncertainty: "Dependency saturation was not observed.", Confidence: "medium", CitationIDs: []string{c.Observations[1].ID, "diff-42"}}, InvestigationResponse{}, InputRequest{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	x = c.Investigations[0]
	c, err = s.MutateInvestigation(c.ID, x.ID, "owner", "human", "response", x.Version, InvestigationFinding{}, InvestigationResponse{FindingID: x.Findings[0].ID, Kind: "dispute", Body: "The deployment preceded this pull."}, InputRequest{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	x = c.Investigations[0]
	c, err = s.MutateInvestigation(c.ID, x.ID, "owner", "human", "request", x.Version, InvestigationFinding{}, InvestigationResponse{}, InputRequest{OwnerID: "owner", DependencyID: "payments", Question: "Did payment latency change?"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	x = c.Investigations[0]
	c, err = s.MutateInvestigation(c.ID, x.ID, "owner", "human", "reply", x.Version, InvestigationFinding{}, InvestigationResponse{}, InputRequest{ID: x.InputRequests[0].ID, Response: "No regional latency change."}, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := completeRevision()
	r.Summary = "successor"
	c, err = s.Revise(c.ID, 1, "owner", r)
	if err != nil {
		t.Fatal(err)
	}
	x = c.Investigations[0]
	if !x.Inconclusive || len(x.StaleEvidenceIDs) == 0 || x.Responses[0].Kind != "dispute" || x.InputRequests[0].Status != "answered" {
		t.Fatalf("investigation projection=%#v", x)
	}
	public := s.ProjectForReader(c, false)
	if len(public.Investigations) != 0 {
		t.Fatalf("investigation leaked to anonymous reader: %#v", public.Investigations)
	}
}

func TestNativeUnitObservationUsesDirectionalBudget(t *testing.T) {
	s, _ := New(t.TempDir())
	revision := completeRevision()
	revision.Indicators[0].Calculation, revision.Indicators[0].Unit = "count", "requests"
	revision.Objectives[0].Target, revision.Objectives[0].Comparator = 10, "at_most"
	revision.ErrorBudgets[0].AllowedFailure, revision.ErrorBudgets[0].Unit = 2, "requests"
	contract, err := s.Create("repo", "owner", revision)
	if err != nil {
		t.Fatal(err)
	}
	contract, err = s.PublishMapping(contract.ID, "owner", SignalMappingRevision{ContractVersion: 1, ObjectiveID: "availability", InstrumentationRevision: "counter-v1", Calculation: "count", Unit: "requests", Rationale: "Count failed requests", Sources: []SignalSource{{Kind: "metric", Name: "Failed requests", Reference: "metrics://failed", Visibility: "public", Sanitization: "aggregate count"}}})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	contract, err = s.RecordObservation(contract.ID, "owner", Observation{MappingID: contract.SignalMappings[0].ID, MappingVersion: 1, ContractVersion: 1, ObjectiveID: "availability", WindowStart: now.Add(-time.Hour), WindowEnd: now, GoodEvents: 15, TotalEvents: 1000, Summary: "Failure count", Uncertainty: 1})
	if err != nil {
		t.Fatal(err)
	}
	got := contract.Observations[0]
	if got.Attainment == nil || *got.Attainment != 15 || got.TargetMet == nil || *got.TargetMet || got.ErrorBudgetConsumed == nil || *got.ErrorBudgetConsumed != 250 {
		t.Fatalf("native-unit projection = %#v", got)
	}
}

func completeRevision() Revision {
	return Revision{Title: "Checkout reliability", Summary: "People can complete checkout reliably.", Scopes: []Scope{{Kind: "environment", ResourceID: "production", Name: "Production checkout"}}, Indicators: []Indicator{{ID: "success", Name: "Successful checkout", Description: "Eligible checkouts that complete", Signal: "checkout.completed", Calculation: "ratio", Unit: "percent", GoodEvent: "completed", TotalEvent: "started"}}, Windows: []Window{{ID: "month", Name: "Rolling month", Duration: "720h", Rolling: true}}, Journeys: []Journey{{ID: "buy", Name: "Buy", Description: "Complete payment", OwnerIDs: []string{"owner"}}}, Objectives: []Objective{{ID: "availability", Name: "Checkout availability", IndicatorID: "success", WindowID: "month", Target: 99.9, Comparator: "at_least", JourneyIDs: []string{"buy"}, OwnerIDs: []string{"owner"}}}, Dependencies: []Dependency{{ID: "payments", Name: "Payments", Kind: "service", OwnerIDs: []string{"owner"}, ObjectiveIDs: []string{"availability"}}}, ErrorBudgets: []ErrorBudget{{ObjectiveID: "availability", AllowedFailure: .1, Unit: "percent", BurnPolicy: "Pause rollout"}}, Severities: []Severity{{Level: "warning", BudgetConsumedPercent: 50, Response: "Investigate", OwnerIDs: []string{"owner"}}, {Level: "critical", BudgetConsumedPercent: 100, Response: "Contain", OwnerIDs: []string{"owner"}}}, OwnerIDs: []string{"owner"}, CommitmentLinks: []CommitmentLink{{Kind: "performance", ID: "perf", Version: 2}, {Kind: "privacy", ID: "data", Version: 1}}, ExceptionPolicy: ExceptionPolicy{MaximumDuration: "168h", ApprovalOwnerIDs: []string{"owner"}, FollowUpRequired: true}, Rationale: "Initial shared contract"}
}

func TestVersioningAndExplicitDiagnostics(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	created, err := s.Create("repo", "owner", completeRevision())
	if err != nil || created.CurrentVersion != 1 || len(created.Diagnostics) != 0 {
		t.Fatalf("create = %#v, %v", created, err)
	}
	r := completeRevision()
	r.Indicators[0].Signal = ""
	r.Indicators[0].Calculation = "histogram_magic"
	r.Objectives[0].OwnerIDs = nil
	r.Objectives = append(r.Objectives, Objective{ID: "conflict", Name: "Conflicting target", IndicatorID: "success", WindowID: "month", Target: 99, Comparator: "at_least", JourneyIDs: []string{"buy"}, OwnerIDs: []string{"owner"}})
	r.ErrorBudgets = append(r.ErrorBudgets, ErrorBudget{ObjectiveID: "conflict", AllowedFailure: 1, Unit: "percent", BurnPolicy: "Investigate"})
	r.Exceptions = []Exception{{ID: "temporary", ObjectiveIDs: []string{"availability"}, Reason: "Migration", ApprovedBy: "owner", ExpiresAt: now.Add(48 * time.Hour), FollowUp: "proposal/1"}}
	updated, err := s.Revise(created.ID, 1, "owner", r)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"missing_signal": false, "unsupported_calculation": false, "missing_ownership": false, "conflicting_target": false, "expiring_exception": false}
	for _, d := range updated.Diagnostics {
		want[d.Kind] = true
	}
	for kind, found := range want {
		if !found {
			t.Errorf("missing %s: %#v", kind, updated.Diagnostics)
		}
	}
	if _, err = s.Revise(created.ID, 1, "owner", completeRevision()); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale error = %v", err)
	}
	got, _ := s.Get(created.ID)
	if len(got.Revisions) != 2 {
		t.Fatalf("history = %#v", got.Revisions)
	}
}

func TestRejectsBrokenReferences(t *testing.T) {
	s, _ := New(t.TempDir())
	cases := []func(*Revision){func(r *Revision) { r.Objectives[0].IndicatorID = "missing" }, func(r *Revision) { r.ErrorBudgets = nil }, func(r *Revision) { r.Severities[1].BudgetConsumedPercent = 25 }, func(r *Revision) { r.CommitmentLinks[0].Kind = "operations" }, func(r *Revision) { r.Scopes[0].ResourceID = "" }, func(r *Revision) { r.Windows[0].Duration = "0s" }, func(r *Revision) { r.Windows[0].Duration = "-1s" }, func(r *Revision) { r.ExceptionPolicy.MaximumDuration = "0s" }, func(r *Revision) { r.ExceptionPolicy.MaximumDuration = "-1s" }}
	for i, mutate := range cases {
		r := completeRevision()
		mutate(&r)
		if _, err := s.Create("repo", "owner", r); !errors.Is(err, ErrInvalid) {
			t.Errorf("case %d error = %v", i, err)
		}
	}
}
