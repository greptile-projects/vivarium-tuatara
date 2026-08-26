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
