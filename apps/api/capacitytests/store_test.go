package capacitytests

import "testing"

func testPlan() Plan {
	component := Component{Kind: "release", ResourceID: "release", Revision: "abc"}
	return Plan{
		Title: "Future peak", ObjectiveID: "objective", ObjectiveVersion: 2, EnvironmentKind: "isolated",
		Candidates: []Candidate{
			{ID: "scale-up", Name: "larger nodes", Strategy: "vertical", Components: []Component{component}, ExpectedCost: 10, Currency: "USD"},
			{ID: "scale-out", Name: "more nodes", Strategy: "horizontal", Components: []Component{component}, ExpectedCost: 12, Currency: "USD"},
		},
		Scenarios: []Scenario{{
			ID: "peak", Name: "forecast peak", Kind: "load", Command: []string{"./load-test"},
			Workload:            Workload{Kind: "synthetic", SourcePath: "capacity/peak.json", Sanitization: "generated identifiers"},
			Limits:              Limits{MaxDurationSeconds: 60, MaxRequests: 1000, MaxConcurrency: 10, MaxCost: 2, CoordinatedLoadKey: "repo/peak"},
			CorrectnessCriteria: []string{"responses match fixture"},
		}},
	}
}
func testRun() Run {
	return Run{CandidateID: "scale-out", ScenarioID: "peak", Status: "succeeded", Repetitions: 3, NoiseRatio: .05, Comparable: true, CorrectnessPassed: true, Metrics: Metrics{Throughput: 100, ThroughputUnit: "rps", LatencyP50MS: 10, LatencyP95MS: 20, LatencyP99MS: 30, ErrorRate: .001, Saturation: .7, RecoverySeconds: 2, ResourceAmount: 4, ResourceUnit: "cores", Cost: 1, Currency: "USD"}, LogsDigest: "sha256:logs"}
}
func TestOnlyComparableStableCorrectRunsBecomeProof(t *testing.T) {
	s, _ := New(t.TempDir())
	p, e := s.Create("repo", "owner", "create", testPlan())
	if e != nil {
		t.Fatal(e)
	}
	r, e := s.AddRun("repo", "agent", "load-agent", p.ID, "run", testRun())
	if e != nil || r.Quality != "proof" {
		t.Fatalf("run=%+v err=%v", r, e)
	}
	noisy := testRun()
	noisy.NoiseRatio = .3
	r, e = s.AddRun("repo", "human", "owner", p.ID, "noisy", noisy)
	if e != nil || r.Quality != "noisy" {
		t.Fatalf("noisy=%+v err=%v", r, e)
	}
	comparison, _ := s.Compare("repo", p.ID)
	if len(comparison.ProvenCandidateIDs) != 1 || comparison.ProvenCandidateIDs[0] != "scale-out" || len(comparison.Diagnostics) != 2 {
		t.Fatalf("comparison=%+v", comparison)
	}
}
func TestProofRequiresEveryScenario(t *testing.T) {
	s, _ := New(t.TempDir())
	p := testPlan()
	second := p.Scenarios[0]
	second.ID, second.Name = "soak", "sustained soak"
	p.Scenarios = append(p.Scenarios, second)
	created, e := s.Create("repo", "owner", "coverage", p)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.AddRun("repo", "human", "owner", created.ID, "peak", testRun()); e != nil {
		t.Fatal(e)
	}
	partial, _ := s.Compare("repo", created.ID)
	if len(partial.ProvenCandidateIDs) != 0 {
		t.Fatalf("partial evidence was proven: %+v", partial)
	}
	soak := testRun()
	soak.ScenarioID = "soak"
	if _, e = s.AddRun("repo", "human", "owner", created.ID, "soak", soak); e != nil {
		t.Fatal(e)
	}
	complete, _ := s.Compare("repo", created.ID)
	if len(complete.ProvenCandidateIDs) != 1 || complete.ProvenCandidateIDs[0] != "scale-out" {
		t.Fatalf("complete evidence was not proven: %+v", complete)
	}
}
func TestCostLimitIsDerivedFromScenario(t *testing.T) {
	s, _ := New(t.TempDir())
	p, _ := s.Create("repo", "owner", "cost-plan", testPlan())
	over := testRun()
	over.Metrics.Cost = 3
	retained, e := s.AddRun("repo", "human", "owner", p.ID, "over-cost", over)
	if e != nil {
		t.Fatal(e)
	}
	if retained.Quality != "limit_breached" || len(retained.LimitBreaches) != 1 || retained.LimitBreaches[0] != "max_cost" {
		t.Fatalf("over-budget run=%+v", retained)
	}
	comparison, _ := s.Compare("repo", p.ID)
	if len(comparison.ProvenCandidateIDs) != 0 {
		t.Fatalf("over-budget evidence was proven: %+v", comparison)
	}
}
func TestRejectsProductionAndUnboundedLoad(t *testing.T) {
	s, _ := New(t.TempDir())
	p := testPlan()
	p.Scenarios[0].Workload.ContainsProductionData = true
	if _, e := s.Create("repo", "owner", "production", p); e != ErrInvalid {
		t.Fatalf("production data accepted: %v", e)
	}
	p = testPlan()
	p.Scenarios[0].Limits.MaxDurationSeconds = 3601
	if _, e := s.Create("repo", "owner", "unbounded", p); e != ErrInvalid {
		t.Fatalf("unbounded load accepted: %v", e)
	}
}
func TestStableRequestIdentityRejectsChangedReuse(t *testing.T) {
	s, _ := New(t.TempDir())
	first, e := s.Create("repo", "owner", "same", testPlan())
	if e != nil {
		t.Fatal(e)
	}
	again, e := s.Create("repo", "owner", "same", testPlan())
	if e != nil || again.ID != first.ID {
		t.Fatalf("retry=%+v %v", again, e)
	}
	changed := testPlan()
	changed.Title = "changed"
	if _, e = s.Create("repo", "owner", "same", changed); e != ErrConflict {
		t.Fatalf("changed reuse=%v", e)
	}
}
