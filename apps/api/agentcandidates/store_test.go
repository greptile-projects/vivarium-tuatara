package agentcandidates

import "testing"

func candidate(repo, pull, rev, suite, digest string) Candidate {
	return Candidate{IdempotencyKey: "candidate-key", RepositoryID: repo, PullRequestID: pull, PullRevision: rev, ProjectID: "project", ProjectVersion: 1, ContractDigest: Digest("contract"), Components: []ComponentDigest{{Kind: "prompt", ID: "p", Digest: Digest("p")}}, Suites: []SuiteSelection{{SuiteID: suite, Version: 1, Digest: digest, ScenarioIDs: []string{"case"}}}, CreatedBy: "owner"}
}
func run(c Candidate, digest string, success bool) Run {
	return Run{IdempotencyKey: "run-key", CandidateID: c.ID, SuiteID: "suite", SuiteVersion: 1, SuiteDigest: digest, Isolation: "ephemeral", Network: "simulated", MaxToolActions: 2, MaxCost: 2, MaxLatencyMS: 1000, Limits: StatisticalLimits{ConfidenceLevel: .95, MinimumSamples: 2, MarginOfError: .1}, Results: []ScenarioResult{
		{ScenarioID: "case", Attempt: 1, TaskSuccess: success, PolicyAdherence: true, Uncertainty: .2, LatencyMS: 20, Cost: 1, TraceDigest: Digest("trace1"), OutputDigest: Digest("out1"), EvaluatorDecision: "passed", EvaluatorID: "judge"},
		{ScenarioID: "case", Attempt: 2, TaskSuccess: success, PolicyAdherence: true, Uncertainty: .4, LatencyMS: 40, Cost: 1, TraceDigest: Digest("trace2"), OutputDigest: Digest("out2"), EvaluatorDecision: "passed", EvaluatorID: "judge"},
	}}
}

func TestRunRequiresMinimumSamplesForEverySelectedScenario(t *testing.T) {
	s, _ := New(t.TempDir())
	d := Digest("suite")
	candidateInput := candidate("repo", "pull", "0123456789012345678901234567890123456789", "suite", d)
	candidateInput.Suites[0].ScenarioIDs = []string{"case", "omitted"}
	c, err := s.Create(candidateInput)
	if err != nil {
		t.Fatal(err)
	}
	r := run(c, d, true)
	if _, err = s.CreateRun(r, "judge"); err != ErrInvalid {
		t.Fatalf("omitted scenario err = %v", err)
	}
}

func TestCreateAndRunRetriesUseStableIdentity(t *testing.T) {
	s, _ := New(t.TempDir())
	d := Digest("suite")
	in := candidate("repo", "pull", "0123456789012345678901234567890123456789", "suite", d)
	first, err := s.Create(in)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Create(in)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID {
		t.Fatalf("candidate IDs = %q, %q", first.ID, second.ID)
	}
	r := run(first, d, true)
	firstRun, err := s.CreateRun(r, "judge")
	if err != nil {
		t.Fatal(err)
	}
	secondRun, err := s.CreateRun(r, "judge")
	if err != nil {
		t.Fatal(err)
	}
	if firstRun.ID != secondRun.ID {
		t.Fatalf("run IDs = %q, %q", firstRun.ID, secondRun.ID)
	}
	in.ProjectVersion = 2
	if _, err = s.Create(in); err != ErrConflict {
		t.Fatalf("changed retry err = %v", err)
	}
}

func TestConcurrentStoresRejectConflictingRunPublication(t *testing.T) {
	root := t.TempDir()
	firstStore, _ := New(root)
	secondStore, _ := New(root)
	d := Digest("suite")
	c, err := firstStore.Create(candidate("repo", "pull", "0123456789012345678901234567890123456789", "suite", d))
	if err != nil {
		t.Fatal(err)
	}
	first := run(c, d, true)
	second := run(c, d, false)
	second.Results[0].OutputDigest = Digest("conflicting-output-1")
	second.Results[1].OutputDigest = Digest("conflicting-output-2")
	start := make(chan struct{})
	errs := make(chan error, 2)
	go func() { <-start; _, err := firstStore.CreateRun(first, "judge"); errs <- err }()
	go func() { <-start; _, err := secondStore.CreateRun(second, "judge"); errs <- err }()
	close(start)
	a, b := <-errs, <-errs
	if !((a == nil && b == ErrConflict) || (a == ErrConflict && b == nil)) {
		t.Fatalf("concurrent errors = %v, %v", a, b)
	}
	runs, err := firstStore.Runs(c.ID)
	if err != nil || len(runs) != 1 {
		t.Fatalf("runs = %#v, err = %v", runs, err)
	}
}
func TestComparisonKeepsContaminationAndInvalidatesOnlyChangedSuite(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	d := Digest("suite-v1")
	base, err := s.Create(candidate("repo", "pull0", "0123456789012345678901234567890123456789", "suite", d))
	if err != nil {
		t.Fatal(err)
	}
	br := run(base, d, false)
	br.Contaminated = true
	br.ContaminationReasons = []string{"protected canary overlap"}
	if _, err = s.CreateRun(br, "judge"); err != nil {
		t.Fatal(err)
	}
	c := candidate("repo", "pull1", "1123456789012345678901234567890123456789", "suite", d)
	c.BaselineCandidateID = base.ID
	c, err = s.Create(c)
	if err != nil {
		t.Fatal(err)
	}
	cr := run(c, d, true)
	cr.Results[1].OutputDigest = Digest("different-output")
	if _, err = s.CreateRun(cr, "judge"); err != nil {
		t.Fatal(err)
	}
	cmp, err := s.Compare(c)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmp.ComparableSuites) != 1 || len(cmp.InvalidatedSuites) != 0 || cmp.Delta.TaskSuccessRate != 1 || !cmp.Contaminated || !cmp.Nondeterministic {
		t.Fatalf("comparison = %#v", cmp)
	}
	changed := candidate("repo", "pull2", "2123456789012345678901234567890123456789", "suite", Digest("suite-v2"))
	changed.BaselineCandidateID = base.ID
	changed, err = s.Create(changed)
	if err != nil {
		t.Fatal(err)
	}
	cmp, err = s.Compare(changed)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmp.InvalidatedSuites) != 1 || cmp.Candidate.Samples != 0 {
		t.Fatalf("changed comparison = %#v", cmp)
	}
}

func TestRunRejectsUnboundedOrStatisticallyInsufficientEvidence(t *testing.T) {
	s, _ := New(t.TempDir())
	d := Digest("suite")
	c, err := s.Create(candidate("repo", "pull", "0123456789012345678901234567890123456789", "suite", d))
	if err != nil {
		t.Fatal(err)
	}
	r := run(c, d, true)
	r.Results = r.Results[:1]
	if _, err = s.CreateRun(r, "judge"); err != ErrInvalid {
		t.Fatalf("err = %v", err)
	}
	r = run(c, d, true)
	r.Results[0].Cost = 3
	if _, err = s.CreateRun(r, "judge"); err != ErrInvalid {
		t.Fatalf("err = %v", err)
	}
}

func TestRunBindsEvaluatorAndRejectsMalformedScenarioSamples(t *testing.T) {
	s, _ := New(t.TempDir())
	d := Digest("suite")
	c, err := s.Create(candidate("repo", "pull", "0123456789012345678901234567890123456789", "suite", d))
	if err != nil {
		t.Fatal(err)
	}
	r := run(c, d, true)
	for i := range r.Results {
		r.Results[i].EvaluatorID = "impersonated-owner"
	}
	created, err := s.CreateRun(r, "authenticated-collaborator")
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range created.Results {
		if result.EvaluatorID != "authenticated-collaborator" {
			t.Fatalf("evaluator = %q", result.EvaluatorID)
		}
	}
	r = run(c, d, true)
	r.Results[1].Attempt = 1
	if _, err = s.CreateRun(r, "judge"); err != ErrInvalid {
		t.Fatalf("duplicate attempt err = %v", err)
	}
	r = run(c, d, true)
	r.Results[0].HumanCorrections = -1
	if _, err = s.CreateRun(r, "judge"); err != ErrInvalid {
		t.Fatalf("negative corrections err = %v", err)
	}
	r = run(c, d, true)
	r.Results[0].ScenarioID = "not-in-suite"
	if _, err = s.CreateRun(r, "judge"); err != ErrInvalid {
		t.Fatalf("unknown scenario err = %v", err)
	}
}
