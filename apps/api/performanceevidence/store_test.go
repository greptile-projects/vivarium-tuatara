package performanceevidence

import (
	"strings"
	"testing"
)

func trial() Trial {
	return Trial{RepositoryID: "repo", CreatedBy: "user", Mode: "benchmark", Source: Source{Kind: "revision", Revision: "0123456789012345678901234567890123456789"}, Workload: "catalog", Inputs: "fixture-v1", Environment: Environment{Name: "linux", OS: "linux", Architecture: "amd64", Runtime: "go"}, Sampling: Sampling{Warmup: 2, Samples: 3, Method: "wall clock, sequential"}, Timings: []Timing{{Metric: "latency", Unit: "ms", Values: []float64{10, 20, 30}}}, Cost: Cost{Amount: 1, Unit: "minute"}}
}

func TestOptimizationEvaluationDerivesReviewEvidenceAndStaleness(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	base := trial()
	base.RepositoryID = "repo"
	base.GoalID = "goal"
	base.Source.Revision = strings.Repeat("a", 40)
	base.Timings[0].Values = []float64{100, 102, 98}
	base.Sampling.Samples = 3
	base.Resources = ResourceProfile{CPUSeconds: 10, PeakMemoryMB: 100}
	base.Cost = Cost{Amount: 2, Unit: "usd"}
	baseline, err := store.Create(base)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := store.CreateInvestigation(Investigation{RepositoryID: "repo", Title: "hot path", TrialIDs: []string{baseline.ID}, CreatedBy: "owner"}, func(Reference) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	candidateInput := base
	candidateInput.Source.Revision = strings.Repeat("b", 40)
	candidateInput.Timings[0].Values = []float64{70, 71, 69}
	candidateInput.Resources = ResourceProfile{CPUSeconds: 8, PeakMemoryMB: 110}
	candidateInput.Cost = Cost{Amount: 1.5, Unit: "usd"}
	candidate, err := store.Create(candidateInput)
	if err != nil {
		t.Fatal(err)
	}
	evaluation, err := store.CreateEvaluation(Evaluation{RepositoryID: "repo", PullRequestID: "pull", Revision: candidate.Source.Revision, GoalID: "goal", InvestigationID: inv.ID, BaselineTrialID: baseline.ID, CandidateTrialID: candidate.ID, AffectedScenarios: []string{"search"}, Commands: []string{"go test ./..."}, CorrectnessChecks: []CorrectnessCheck{{Name: "tests", Command: "go test ./...", Passed: true, Summary: "passed"}}, ResidualRisks: []string{"cache memory"}, CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	if evaluation.Confidence == nil || *evaluation.Confidence < .99 || evaluation.Comparisons[0].ChangePercent >= -25 || evaluation.ResourceChanges["cpu_seconds_percent"] != -20 || evaluation.CostChangePercent == nil || *evaluation.CostChangePercent != -25 || !evaluation.CorrectnessPassed {
		t.Fatalf("unexpected derived evidence: %+v", evaluation)
	}
	items, err := store.ListEvaluations("repo", "pull", strings.Repeat("c", 40))
	if err != nil || len(items) != 1 || !items[0].Stale {
		t.Fatalf("expected stale retained evaluation: %+v %v", items, err)
	}
}

func TestOptimizationEvaluationLeavesInvalidStatisticsUnavailable(t *testing.T) {
	store, _ := New(t.TempDir())
	base := trial()
	base.GoalID, base.Source.Revision = "goal", strings.Repeat("a", 40)
	base.Cost = Cost{Amount: 0, Unit: "usd"}
	base.Timings = []Timing{{Metric: "latency", Unit: "ms", Values: []float64{10}}}
	base.Sampling.Samples = 1
	baseline, _ := store.Create(base)
	inv, _ := store.CreateInvestigation(Investigation{RepositoryID: "repo", Title: "hot path", TrialIDs: []string{baseline.ID}, CreatedBy: "owner"}, func(Reference) bool { return true })
	candidateInput := base
	candidateInput.Source.Revision = strings.Repeat("b", 40)
	candidateInput.Cost = Cost{Amount: 42, Unit: "eur"}
	candidateInput.Timings = []Timing{{Metric: "throughput", Unit: "ops/s", Values: []float64{100}}}
	candidate, _ := store.Create(candidateInput)
	evaluation, err := store.CreateEvaluation(Evaluation{RepositoryID: "repo", PullRequestID: "pull", Revision: candidate.Source.Revision, GoalID: "goal", InvestigationID: inv.ID, BaselineTrialID: baseline.ID, CandidateTrialID: candidate.ID, AffectedScenarios: []string{"search"}, Commands: []string{"bench"}, CorrectnessChecks: []CorrectnessCheck{{Name: "tests", Command: "test", Passed: true, Summary: "passed"}}, CreatedBy: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	if evaluation.Confidence != nil || evaluation.CostChangePercent != nil {
		t.Fatalf("incomparable evidence must be unavailable: %+v", evaluation)
	}
}

func TestOptimizationEvaluationLeavesZeroVarianceConfidenceUnavailable(t *testing.T) {
	store, _ := New(t.TempDir())
	base := trial()
	base.GoalID, base.Source.Revision = "goal", strings.Repeat("a", 40)
	base.Timings[0].Values = []float64{100, 100, 100}
	baseline, _ := store.Create(base)
	inv, _ := store.CreateInvestigation(Investigation{RepositoryID: "repo", Title: "hot path", TrialIDs: []string{baseline.ID}, CreatedBy: "owner"}, func(Reference) bool { return true })
	for name, values := range map[string][]float64{"equal means": {100, 100, 100}, "unequal means": {200, 200, 200}} {
		t.Run(name, func(t *testing.T) {
			candidateInput := base
			candidateInput.Source.Revision = strings.Repeat("b", 40)
			candidateInput.Timings[0].Values = values
			candidate, _ := store.Create(candidateInput)
			evaluation, err := store.CreateEvaluation(Evaluation{RepositoryID: "repo", PullRequestID: name, Revision: candidate.Source.Revision, GoalID: "goal", InvestigationID: inv.ID, BaselineTrialID: baseline.ID, CandidateTrialID: candidate.ID, AffectedScenarios: []string{"search"}, Commands: []string{"bench"}, CorrectnessChecks: []CorrectnessCheck{{Name: "tests", Command: "test", Passed: true, Summary: "passed"}}, CreatedBy: "owner"})
			if err != nil {
				t.Fatal(err)
			}
			if evaluation.Confidence != nil {
				t.Fatalf("zero-variance confidence must be unavailable: %v", *evaluation.Confidence)
			}
		})
	}
}

func TestTrialSummaryComparisonAndSanitization(t *testing.T) {
	s, e := New(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	a, e := s.Create(trial())
	if e != nil {
		t.Fatal(e)
	}
	if a.Timings[0].Mean != 20 || a.Timings[0].Variance != 200.0/3.0 {
		t.Fatalf("summary = %+v", a.Timings[0])
	}
	b := trial()
	b.Timings[0].Values = []float64{5, 10, 15}
	created, e := s.Create(b)
	if e != nil {
		t.Fatal(e)
	}
	c := s.Compare(a, created)
	if len(c) != 1 || !c[0].Comparable || c[0].ChangePercent != -50 {
		t.Fatalf("comparison = %+v", c)
	}
	bad := trial()
	bad.Logs = []string{"Authorization: Bearer private"}
	if _, e = s.Create(bad); e != ErrInvalid {
		t.Fatalf("secret error = %v", e)
	}
	for _, credential := range []string{"X-API-Key: private", "api_key=private", `{"token":"private"}`, `{"nested":{"access_token":"private"}}`} {
		bad = trial()
		bad.Logs = []string{credential}
		if _, e = s.Create(bad); e != ErrInvalid {
			t.Fatalf("credential %q error = %v", credential, e)
		}
	}
	capture := trial()
	capture.Mode = "production_capture"
	if _, e = s.Create(capture); e != ErrInvalid {
		t.Fatalf("unsanitized capture error = %v", e)
	}
	capture.Sanitization = []string{"remove user identifiers and replace values with shape buckets"}
	capture.Inputs = "customer@example.com private production request"
	capture.Logs = []string{"email=alice.customer@example.com customer_id=cust-private request=/account/42"}
	createdCapture, e := s.Create(capture)
	if e != nil {
		t.Fatal(e)
	}
	if createdCapture.Inputs != "[sanitized production-derived workload]" {
		t.Fatalf("production inputs persisted = %q", createdCapture.Inputs)
	}
	if len(createdCapture.Logs) != 1 || createdCapture.Logs[0] != "[sanitized production log entry]" {
		t.Fatalf("production logs persisted = %q", createdCapture.Logs)
	}
	storedCapture, e := s.Get(createdCapture.ID)
	if e != nil || storedCapture.Logs[0] != "[sanitized production log entry]" {
		t.Fatalf("production logs read = %q, %v", storedCapture.Logs, e)
	}
	listedCaptures, e := s.List(capture.RepositoryID)
	if e != nil {
		t.Fatal(e)
	}
	for _, listed := range listedCaptures {
		if listed.ID == createdCapture.ID && listed.Logs[0] != "[sanitized production log entry]" {
			t.Fatalf("production logs listed = %q", listed.Logs)
		}
	}
}

func TestIncomparableTrialsExplainNoise(t *testing.T) {
	s, _ := New(t.TempDir())
	a, _ := s.Create(trial())
	b := trial()
	b.Environment.Name = "mac"
	created, _ := s.Create(b)
	c := s.Compare(a, created)
	if c[0].Comparable || c[0].Reason == "" {
		t.Fatalf("comparison = %+v", c)
	}
}

func TestComparisonRequiresCompleteMeasurementConditions(t *testing.T) {
	s, _ := New(t.TempDir())
	a := trial()
	a.Environment.Hardware = "xeon"
	a.Environment.ContainerImage = "benchmark:v1"
	baseline, _ := s.Create(a)
	b := trial()
	b.Environment.Name = a.Environment.Name
	b.Environment.OS = "darwin"
	b.Environment.Architecture = "arm64"
	b.Environment.Runtime = "go1.23"
	b.Environment.Hardware = "apple-m3"
	b.Environment.ContainerImage = "benchmark:v2"
	b.Sampling.Warmup = 9
	current, _ := s.Create(b)
	comparison := s.Compare(baseline, current)
	if comparison[0].Comparable || comparison[0].ChangePercent != 0 {
		t.Fatalf("comparison = %+v", comparison)
	}
}

func TestCitedInvestigationProjectsStalenessAndReview(t *testing.T) {
	s, _ := New(t.TempDir())
	base := trial()
	base.ContextKind, base.ContextID = "benchmark", "catalog-list"
	evidence, _ := s.Create(base)
	inv, err := s.CreateInvestigation(Investigation{RepositoryID: "repo", Title: "Why is catalog slow?", TrialIDs: []string{evidence.ID}, References: []Reference{{Kind: "symbol", ID: "catalog-list", Revision: base.Source.Revision, Symbol: "List", Label: "catalog List"}}, InviteeIDs: []string{"owner"}, CreatedBy: "user"}, func(Reference) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	inv, err = s.AddFinding(inv.ID, "agent:reader", Finding{Kind: "hypothesis", Body: "serialization dominates", CitationIDs: []string{evidence.ID, "catalog-list"}, Confidence: "medium", Flamegraph: []FlameStack{{Frames: []FlameFrame{{Name: "List"}, {Name: "Marshal"}}, Value: 62, Unit: "samples"}}})
	if err != nil || len(inv.Findings) != 1 {
		t.Fatalf("finding = %+v, %v", inv, err)
	}
	inv, err = s.Respond(inv.ID, inv.Findings[0].ID, "owner", "confirmed by trace", true)
	if err != nil || len(inv.Findings[0].Confirmations) != 1 {
		t.Fatalf("response = %+v, %v", inv, err)
	}
	newer := base
	newer.Source.Revision = "1123456789012345678901234567890123456789"
	newer.Workload = "catalog-large"
	newer.Environment.Name = "linux-new"
	if _, err = s.Create(newer); err != nil {
		t.Fatal(err)
	}
	projected := s.ProjectStaleness(inv)
	if !projected.Findings[0].Stale || len(projected.Findings[0].StaleReasons) != 3 {
		t.Fatalf("staleness = %+v", projected.Findings[0])
	}
}

func TestInvestigationRejectsUnselectedCitations(t *testing.T) {
	s, _ := New(t.TempDir())
	evidence, _ := s.Create(trial())
	inv, _ := s.CreateInvestigation(Investigation{RepositoryID: "repo", Title: "bounded", TrialIDs: []string{evidence.ID}, CreatedBy: "user"}, func(Reference) bool { return true })
	if _, err := s.AddFinding(inv.ID, "agent", Finding{Kind: "hypothesis", Body: "claim", CitationIDs: []string{"restricted-trace"}, Confidence: "low"}); err != ErrInvalid {
		t.Fatalf("error = %v", err)
	}
}

func TestInvestigationRequiresAuthoritativeReferenceResolver(t *testing.T) {
	s, _ := New(t.TempDir())
	evidence, _ := s.Create(trial())
	v := Investigation{RepositoryID: "repo", Title: "resolved", TrialIDs: []string{evidence.ID}, References: []Reference{{Kind: "commit", ID: trial().Source.Revision, Revision: trial().Source.Revision, Label: "commit"}}, CreatedBy: "user"}
	if _, err := s.CreateInvestigation(v, nil); err != ErrInvalid {
		t.Fatalf("missing resolver error = %v", err)
	}
	if _, err := s.CreateInvestigation(v, func(Reference) bool { return false }); err != ErrInvalid {
		t.Fatalf("rejected reference error = %v", err)
	}
}
