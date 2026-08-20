package agentcandidates

import "testing"

func candidate(repo, pull, rev, suite, digest string) Candidate {
	return Candidate{RepositoryID: repo, PullRequestID: pull, PullRevision: rev, ProjectID: "project", ProjectVersion: 1, ContractDigest: Digest("contract"), Components: []ComponentDigest{{Kind: "prompt", ID: "p", Digest: Digest("p")}}, Suites: []SuiteSelection{{SuiteID: suite, Version: 1, Digest: digest}}, CreatedBy: "owner"}
}
func run(c Candidate, digest string, success bool) Run {
	return Run{CandidateID: c.ID, SuiteID: "suite", SuiteVersion: 1, SuiteDigest: digest, Isolation: "ephemeral", Network: "simulated", MaxToolActions: 2, MaxCost: 2, MaxLatencyMS: 1000, Limits: StatisticalLimits{ConfidenceLevel: .95, MinimumSamples: 2, MarginOfError: .1}, Results: []ScenarioResult{
		{ScenarioID: "case", Attempt: 1, TaskSuccess: success, PolicyAdherence: true, Uncertainty: .2, LatencyMS: 20, Cost: 1, TraceDigest: Digest("trace1"), OutputDigest: Digest("out1"), EvaluatorDecision: "passed", EvaluatorID: "judge"},
		{ScenarioID: "case", Attempt: 2, TaskSuccess: success, PolicyAdherence: true, Uncertainty: .4, LatencyMS: 40, Cost: 1, TraceDigest: Digest("trace2"), OutputDigest: Digest("out2"), EvaluatorDecision: "passed", EvaluatorID: "judge"},
	}}
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
	if _, err = s.CreateRun(br); err != nil {
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
	if _, err = s.CreateRun(cr); err != nil {
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
	if _, err = s.CreateRun(r); err != ErrInvalid {
		t.Fatalf("err = %v", err)
	}
	r = run(c, d, true)
	r.Results[0].Cost = 3
	if _, err = s.CreateRun(r); err != ErrInvalid {
		t.Fatalf("err = %v", err)
	}
}
