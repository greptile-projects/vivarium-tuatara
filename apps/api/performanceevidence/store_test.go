package performanceevidence

import "testing"

func trial() Trial {
	return Trial{RepositoryID: "repo", CreatedBy: "user", Mode: "benchmark", Source: Source{Kind: "revision", Revision: "0123456789012345678901234567890123456789"}, Workload: "catalog", Inputs: "fixture-v1", Environment: Environment{Name: "linux", OS: "linux", Architecture: "amd64", Runtime: "go"}, Sampling: Sampling{Warmup: 2, Samples: 3, Method: "wall clock, sequential"}, Timings: []Timing{{Metric: "latency", Unit: "ms", Values: []float64{10, 20, 30}}}, Cost: Cost{Amount: 1, Unit: "minute"}}
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
	createdCapture, e := s.Create(capture)
	if e != nil {
		t.Fatal(e)
	}
	if createdCapture.Inputs != "[sanitized production-derived workload]" {
		t.Fatalf("production inputs persisted = %q", createdCapture.Inputs)
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
