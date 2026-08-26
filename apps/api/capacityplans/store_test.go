package capacityplans

import (
	"errors"
	"testing"
)

func fixture() Plan {
	return Plan{ObjectiveID: "objective", ObjectiveVersion: 1, TestID: "test", CandidateID: "horizontal", Title: "Scale checkout", Rationale: "Bounded evidence supports horizontal scaling", TotalBudget: 100, Currency: "USD", Reservations: []Dependency{{ID: "quota", Kind: "quota", OwnerID: "owner", Description: "regional quota", Status: "requested"}}, Phases: []Phase{{ID: "observe", Name: "Add saturation signals", Kind: "observability", OwnerID: "human", Budget: 20, Currency: "USD", AcceptanceCriteria: []string{"signal is reviewed"}, DecisionPoint: "continue if headroom is measurable", ExitStrategy: "remove unused signal"}, {ID: "scale", Name: "Scale workers", Kind: "infrastructure", OwnerID: "agent", Budget: 80, Currency: "USD", DependsOn: []string{"observe"}, AcceptanceCriteria: []string{"forecast load passes"}, DecisionPoint: "continue if quota is granted", ExitStrategy: "restore prior worker count"}}}
}
func TestStablePlanAndDelivery(t *testing.T) {
	s, _ := New(t.TempDir())
	p, e := s.Create("repo", "owner", "request", fixture())
	if e != nil {
		t.Fatal(e)
	}
	again, e := s.Create("repo", "owner", "request", fixture())
	if e != nil || again.ID != p.ID {
		t.Fatalf("retry=%+v err=%v", again, e)
	}
	changed := fixture()
	changed.TotalBudget = 101
	if _, e = s.Create("repo", "owner", "request", changed); !errors.Is(e, ErrConflict) {
		t.Fatalf("changed retry=%v", e)
	}
	d := Delivery{ProposalID: "proposal", TaskIDs: []string{"one", "two"}, BaseRevision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	linked, e := s.LinkDelivery("repo", p.ID, d)
	if e != nil || linked.Delivery == nil {
		t.Fatalf("delivery=%+v err=%v", linked, e)
	}
	if _, e = s.LinkDelivery("repo", p.ID, d); e != nil {
		t.Fatalf("delivery retry=%v", e)
	}
}
func TestRejectsBrokenOrderAndBudget(t *testing.T) {
	s, _ := New(t.TempDir())
	p := fixture()
	p.Phases[1].DependsOn = []string{"missing"}
	if _, e := s.Create("repo", "owner", "bad-order", p); !errors.Is(e, ErrInvalid) {
		t.Fatalf("bad dependency=%v", e)
	}
	p = fixture()
	p.TotalBudget = 99
	if _, e := s.Create("repo", "owner", "bad-budget", p); !errors.Is(e, ErrInvalid) {
		t.Fatalf("bad budget=%v", e)
	}
}

func TestDeliveryReservationIsVisibleAndRetryable(t *testing.T) {
	s, _ := New(t.TempDir())
	p, _ := s.Create("repo", "owner", "reserve", fixture())
	proposalID, taskIDs := DeliveryIdentities(p)
	d := Delivery{ProposalID: proposalID, TaskIDs: taskIDs, BaseRevision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	pending, err := s.ReserveDelivery("repo", p.ID, d)
	if err != nil || pending.Delivery == nil || pending.Delivery.Status != "pending" {
		t.Fatalf("pending=%+v err=%v", pending, err)
	}
	again, err := s.ReserveDelivery("repo", p.ID, d)
	if err != nil || again.Delivery.ProposalID != proposalID {
		t.Fatalf("reservation retry=%+v err=%v", again, err)
	}
	created, err := s.FinalizeDelivery("repo", p.ID, d)
	if err != nil || created.Delivery.Status != "created" {
		t.Fatalf("created=%+v err=%v", created, err)
	}
}
