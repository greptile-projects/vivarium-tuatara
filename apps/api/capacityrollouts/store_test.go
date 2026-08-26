package capacityrollouts

import (
	"errors"
	"testing"
	"time"
)

func fixture() Rollout {
	return Rollout{PlanID: "plan", EnvironmentIDs: []string{"prod"}, Phases: []Phase{{ID: "scale", Name: "Scale workers", ControllerType: "human", ControllerID: "operator", EnvironmentID: "prod", DeploymentIDs: []string{"deployment"}, DeployedRevisions: []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}}}
}
func TestProductionEvidenceVerifiesUsableCapacity(t *testing.T) {
	s, _ := New(t.TempDir())
	r, e := s.Create("repo", "operator", "create", fixture())
	if e != nil {
		t.Fatal(e)
	}
	retry, e := s.Create("repo", "operator", "create", fixture())
	if e != nil || retry.ID != r.ID {
		t.Fatalf("retry: %v", e)
	}
	start := time.Now().Add(-time.Hour)
	ev := Evidence{Source: "production", ObservationStart: start, ObservationEnd: start.Add(time.Hour), DeploymentIDs: []string{"deployment"}, DeployedRevisions: []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, AllocatedCapacity: 120, UsableCapacity: 100, Load: 80, Unit: "rps", ServiceLevels: map[string]float64{"availability": 99.99}, DependencyHealth: map[string]string{"database": "healthy"}, Cost: 40, Currency: "USD", ForecastDemand: 75}
	r, e = s.Mutate("repo", r.ID, 1, Event{RequestID: "observe", Kind: "observe", PhaseID: "scale", ActorType: "human", ActorID: "operator", Reason: "production window complete"}, &ev)
	if e != nil {
		t.Fatal(e)
	}
	if r.Status != "verified" || r.Evidence[0].Headroom != 20 || !r.Evidence[0].ForecastValidated {
		t.Fatalf("unexpected projection: %#v", r)
	}
}
func TestRiskEvidenceContainsScalingAndRevisitsDecision(t *testing.T) {
	s, _ := New(t.TempDir())
	r, _ := s.Create("repo", "operator", "create", fixture())
	start := time.Now().Add(-time.Hour)
	ev := Evidence{Source: "production", ObservationStart: start, ObservationEnd: start.Add(time.Hour), DeploymentIDs: []string{"deployment"}, DeployedRevisions: []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, AllocatedCapacity: 120, UsableCapacity: 100, Load: 110, Unit: "rps", Cost: 140, Currency: "USD", ForecastDemand: 90, FailureKinds: []string{"demand_shift", "budget_breach"}}
	r, e := s.Mutate("repo", r.ID, 1, Event{RequestID: "observe", Kind: "observe", PhaseID: "scale", ActorType: "human", ActorID: "operator", Reason: "budget guard", DecisionID: "decision-1"}, &ev)
	if e != nil {
		t.Fatal(e)
	}
	if r.Status != "contained" || r.Phases[0].ThrottlePercent != 0 || r.Phases[0].PredictedNextAction != "pause scaling and revisit connected decision decision-1" {
		t.Fatalf("containment missing: %#v", r.Phases[0])
	}
	_, e = s.Mutate("repo", r.ID, 1, Event{RequestID: "changed", Kind: "pause", PhaseID: "scale", ActorID: "operator"}, nil)
	if !errors.Is(e, ErrConflict) {
		t.Fatalf("wanted CAS conflict, got %v", e)
	}
}

func TestNegativeHeadroomContainsScalingWithoutCallerFailureKind(t *testing.T) {
	s, _ := New(t.TempDir())
	r, _ := s.Create("repo", "operator", "create", fixture())
	start := time.Now().Add(-time.Hour)
	evidence := Evidence{Source: "production", ObservationStart: start, ObservationEnd: start.Add(time.Hour), DeploymentIDs: []string{"deployment"}, DeployedRevisions: []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, AllocatedCapacity: 100, UsableCapacity: 80, Load: 90, Unit: "rps", Cost: 20, Currency: "USD", ForecastDemand: 75}
	r, err := s.Mutate("repo", r.ID, 1, Event{RequestID: "observe", Kind: "observe", PhaseID: "scale", ActorType: "human", ActorID: "operator", Reason: "production window"}, &evidence)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "contained" || r.Phases[0].ThrottlePercent != 0 || !contains(r.Phases[0].Blockers, "insufficient_headroom") {
		t.Fatalf("negative headroom was not contained: %#v", r)
	}
}

func TestObserveRetryRejectsChangedEvidence(t *testing.T) {
	s, _ := New(t.TempDir())
	r, _ := s.Create("repo", "operator", "create", fixture())
	start := time.Now().Add(-time.Hour)
	evidence := Evidence{Source: "production", ObservationStart: start, ObservationEnd: start.Add(time.Hour), DeploymentIDs: []string{"deployment"}, DeployedRevisions: []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, AllocatedCapacity: 100, UsableCapacity: 90, Load: 70, Unit: "rps", Cost: 20, Currency: "USD", ForecastDemand: 65}
	event := Event{RequestID: "observe", Kind: "observe", PhaseID: "scale", ActorType: "human", ActorID: "operator", Reason: "production window"}
	r, err := s.Mutate("repo", r.ID, 1, event, &evidence)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Mutate("repo", r.ID, r.Version, event, &evidence); err != nil {
		t.Fatalf("unchanged retry failed: %v", err)
	}
	evidence.Load = 95
	if _, err = s.Mutate("repo", r.ID, r.Version, event, &evidence); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed evidence retry = %v, want conflict", err)
	}
}

func TestCreateRejectsUnmatchedDeploymentRevisions(t *testing.T) {
	s, _ := New(t.TempDir())
	r := fixture()
	r.Phases[0].DeployedRevisions = append(r.Phases[0].DeployedRevisions, "unverified-trailing-revision")
	if _, err := s.Create("repo", "operator", "create", r); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unmatched deployment revisions = %v, want invalid", err)
	}
}

func TestForecastValidationGatesVerification(t *testing.T) {
	s, _ := New(t.TempDir())
	r, _ := s.Create("repo", "operator", "create", fixture())
	start := time.Now().Add(-time.Hour)
	evidence := Evidence{Source: "production", ObservationStart: start, ObservationEnd: start.Add(time.Hour), DeploymentIDs: []string{"deployment"}, DeployedRevisions: []string{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, AllocatedCapacity: 120, UsableCapacity: 100, Load: 60, Unit: "rps", Cost: 20, Currency: "USD", ForecastDemand: 80}
	r, err := s.Mutate("repo", r.ID, 1, Event{RequestID: "observe", Kind: "observe", PhaseID: "scale", ActorType: "human", ActorID: "operator", Reason: "production demand has not arrived"}, &evidence)
	if err != nil {
		t.Fatal(err)
	}
	if r.Status != "in_progress" || r.Phases[0].State != "observing" || r.Evidence[0].ForecastValidated {
		t.Fatalf("unvalidated forecast advanced: %#v", r)
	}
}

func TestObservationCannotReplaceStagedDeploymentIdentity(t *testing.T) {
	s, _ := New(t.TempDir())
	r, _ := s.Create("repo", "operator", "create", fixture())
	start := time.Now().Add(-time.Hour)
	evidence := Evidence{Source: "production", ObservationStart: start, ObservationEnd: start.Add(time.Hour), DeploymentIDs: []string{"different-deployment"}, DeployedRevisions: []string{"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}, AllocatedCapacity: 120, UsableCapacity: 100, Load: 90, Unit: "rps", Cost: 20, Currency: "USD", ForecastDemand: 80}
	_, err := s.Mutate("repo", r.ID, 1, Event{RequestID: "observe", Kind: "observe", PhaseID: "scale", ActorType: "human", ActorID: "operator", Reason: "substituted promotion"}, &evidence)
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("substituted deployment = %v, want invalid", err)
	}
	retained, _ := s.Get("repo", r.ID)
	if retained.Phases[0].DeploymentIDs[0] != "deployment" || len(retained.Evidence) != 0 {
		t.Fatalf("staged identity changed: %#v", retained)
	}
}
