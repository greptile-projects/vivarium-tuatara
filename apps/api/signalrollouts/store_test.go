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
