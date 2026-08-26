package signalrollouts

import (
	"errors"
	"testing"
	"time"
)

func TestObservationContainsUnsafeCollectionAndRetainsEvidence(t *testing.T) {
	s, _ := New(t.TempDir())
	r, e := s.Create("repo", "operator", "create", Rollout{ContractID: "contract", ContractVersion: 2, InstrumentationRevision: "commit", DeploymentID: "deploy", EnvironmentID: "prod", ControllerID: "operator", Scope: Scope{Service: "api", Audience: "ops", Region: "eu", TrafficPercent: 10}, Budget: Budget{StorageBytes: 100, QueryCostCents: 10, Cardinality: 20}})
	if e != nil {
		t.Fatal(e)
	}
	q := Quality{SignalHealth: "degraded", Coverage: .8, Missingness: .2, SamplingBias: .2, Cardinality: 21, StorageBytes: 101, QueryCostCents: 11, PipelineLoss: .1, PrivacyControls: []string{"redaction"}, MalformedPayloads: 1, UnexpectedSensitiveData: true, CollectorAvailable: false, ServiceRegression: true}
	o := Observation{Scope: r.Scope, Quality: q, StartedAt: time.Now().Add(-time.Minute), EndedAt: time.Now()}
	r, e = s.Mutate("repo", r.ID, 1, Event{RequestID: "observe", Kind: "observe", ActorID: "operator", Reason: "calibration", Observation: &o})
	if e != nil {
		t.Fatal(e)
	}
	if r.Status != "contained" || len(r.ContainmentReasons) != 6 || len(r.Observations) != 1 || r.Observations[0].Digest == "" {
		t.Fatalf("unsafe evidence not retained and contained: %#v", r)
	}
	retried, e := s.Mutate("repo", r.ID, 1, Event{RequestID: "observe", Kind: "observe", ActorID: "operator", Reason: "calibration", Observation: &o})
	if e != nil || retried.Version != r.Version || len(retried.Observations) != 1 {
		t.Fatalf("observation retry did not reconcile: %v", e)
	}
	_, e = s.Mutate("repo", r.ID, r.Version, Event{RequestID: "resume", Kind: "resume", ActorID: "operator", Reason: "unsafe retry"})
	if !errors.Is(e, ErrInvalid) {
		t.Fatalf("resume should fail while contained: %v", e)
	}
}

func TestStableMutationsRejectChangedRetry(t *testing.T) {
	s, _ := New(t.TempDir())
	r, _ := s.Create("repo", "operator", "create", Rollout{ContractID: "contract", ContractVersion: 1, InstrumentationRevision: "commit", DeploymentID: "deploy", EnvironmentID: "prod", ControllerID: "operator", Scope: Scope{Service: "api", Audience: "ops", Region: "eu", TrafficPercent: 10}, Budget: Budget{}})
	ev := Event{RequestID: "pause", Kind: "pause", ActorID: "operator", Reason: "cost review"}
	first, e := s.Mutate("repo", r.ID, 1, ev)
	if e != nil {
		t.Fatal(e)
	}
	again, e := s.Mutate("repo", r.ID, 1, ev)
	if e != nil || again.Version != first.Version {
		t.Fatalf("unchanged retry did not reconcile: %v", e)
	}
	ev.Reason = "different"
	if _, e = s.Mutate("repo", r.ID, first.Version, ev); !errors.Is(e, ErrConflict) {
		t.Fatalf("changed reuse should conflict: %v", e)
	}
}

func TestNarrowPreservesReviewedTargetAndTerminalStates(t *testing.T) {
	s, _ := New(t.TempDir())
	create := func(request string) Rollout {
		r, err := s.Create("repo", "operator", request, Rollout{ContractID: "contract", ContractVersion: 1, InstrumentationRevision: "commit", DeploymentID: "deploy", EnvironmentID: "prod", ControllerID: "operator", Scope: Scope{Service: "api", Audience: "ops", Region: "eu", TrafficPercent: 10}, Budget: Budget{StorageBytes: 100, QueryCostCents: 10, Cardinality: 20}})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	r := create("scope")
	changed := Scope{Service: "admin", Audience: "users", Region: "us", TrafficPercent: 5}
	if _, err := s.Mutate("repo", r.ID, r.Version, Event{RequestID: "redirect", Kind: "narrow", ActorID: "operator", Reason: "redirect", Scope: &changed}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("changed reviewed target should fail: %v", err)
	}
	narrow := Scope{Service: "api", Audience: "ops", Region: "eu", TrafficPercent: 5}
	if _, err := s.Mutate("repo", r.ID, r.Version, Event{RequestID: "narrow", Kind: "narrow", ActorID: "operator", Reason: "reduce exposure", Scope: &narrow}); err != nil {
		t.Fatalf("valid narrow failed: %v", err)
	}
	r = create("terminal")
	r, _ = s.Mutate("repo", r.ID, r.Version, Event{RequestID: "rollback", Kind: "rollback", ActorID: "operator", Reason: "end rollout"})
	if _, err := s.Mutate("repo", r.ID, r.Version, Event{RequestID: "resume", Kind: "resume", ActorID: "operator", Reason: "reactivate"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("rolled back resume should fail: %v", err)
	}
	if _, err := s.Mutate("repo", r.ID, r.Version, Event{RequestID: "narrow-terminal", Kind: "narrow", ActorID: "operator", Reason: "reactivate", Scope: &narrow}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("rolled back narrow should fail: %v", err)
	}
}

func TestContainmentRequiresExplicitHealthyResolution(t *testing.T) {
	s, _ := New(t.TempDir())
	r, _ := s.Create("repo", "operator", "create", Rollout{ContractID: "contract", ContractVersion: 1, InstrumentationRevision: "commit", DeploymentID: "deploy", EnvironmentID: "prod", ControllerID: "operator", Scope: Scope{Service: "api", Audience: "ops", Region: "eu", TrafficPercent: 10}, Budget: Budget{StorageBytes: 100, QueryCostCents: 10, Cardinality: 20}})
	now := time.Now()
	unsafe := Observation{Scope: r.Scope, StartedAt: now.Add(-time.Minute), EndedAt: now, Quality: Quality{SignalHealth: "bad", Coverage: .8, Missingness: .2, PipelineLoss: .1, PrivacyControls: []string{"redaction"}, MalformedPayloads: 1, CollectorAvailable: true}}
	r, _ = s.Mutate("repo", r.ID, r.Version, Event{RequestID: "unsafe", Kind: "observe", ActorID: "operator", Reason: "bad payload", Observation: &unsafe})
	if _, err := s.Mutate("repo", r.ID, r.Version, Event{RequestID: "narrow-contained", Kind: "narrow", ActorID: "operator", Reason: "bypass", Scope: &r.Scope}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("contained narrow should fail: %v", err)
	}
	healthy := unsafe
	healthy.Quality = Quality{SignalHealth: "healthy", Coverage: 1, PrivacyControls: []string{"redaction"}, CollectorAvailable: true}
	unrelated := healthy
	unrelated.Scope = Scope{Service: "billing", Audience: "internal", Region: "us", TrafficPercent: 10}
	if _, err := s.Mutate("repo", r.ID, r.Version, Event{RequestID: "unrelated-observe", Kind: "observe", ActorID: "operator", Reason: "unrelated window", Observation: &unrelated}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("cross-scope observation should fail: %v", err)
	}
	if _, err := s.Mutate("repo", r.ID, r.Version, Event{RequestID: "unrelated-resolve", Kind: "resolve", ActorID: "operator", Reason: "unrelated recovery", Observation: &unrelated}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("cross-scope resolution should fail: %v", err)
	}
	stillContained, _ := s.Get("repo", r.ID)
	if stillContained.Status != "contained" || len(stillContained.ContainmentReasons) == 0 {
		t.Fatal("cross-scope evidence changed containment")
	}
	r, _ = s.Mutate("repo", r.ID, r.Version, Event{RequestID: "healthy-observe", Kind: "observe", ActorID: "operator", Reason: "healthy window", Observation: &healthy})
	if r.Status != "contained" || len(r.ContainmentReasons) == 0 {
		t.Fatal("ordinary observation silently cleared containment")
	}
	r, err := s.Mutate("repo", r.ID, r.Version, Event{RequestID: "resolve", Kind: "resolve", ActorID: "operator", Reason: "operator verified recovery", Observation: &healthy})
	if err != nil || r.Status != "paused" || len(r.ContainmentReasons) != 0 {
		t.Fatalf("explicit resolution failed: %v %#v", err, r)
	}
}
