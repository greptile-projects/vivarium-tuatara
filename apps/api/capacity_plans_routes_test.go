package main

import (
	"testing"

	"github.com/greptile-projects/vivarium-tuatara/apps/api/capacityplans"
)

func TestCapacityDeliveryRetryKeepsReservedBaseAfterBranchAdvance(t *testing.T) {
	original := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	advanced := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	plan := capacityplans.Plan{Delivery: &capacityplans.Delivery{BaseRevision: original, Status: "pending"}}
	got, recovering := capacityDeliveryBase(plan, advanced)
	if !recovering || got != original {
		t.Fatalf("base=%s recovering=%t", got, recovering)
	}
	plan.Delivery.Status = "created"
	got, recovering = capacityDeliveryBase(plan, advanced)
	if !recovering || got != original {
		t.Fatalf("created retry base=%s recovering=%t", got, recovering)
	}
	got, recovering = capacityDeliveryBase(capacityplans.Plan{}, advanced)
	if recovering || got != advanced {
		t.Fatalf("new delivery base=%s recovering=%t", got, recovering)
	}
}
