package durableschemas

import "testing"

func TestReviewedSchemasAndMigrationHistory(t *testing.T) {
	s, _ := New(t.TempDir())
	r := Revision{Name: "orders", StoreKind: "database", Description: "Order state", Definition: "orders(id uuid primary key)", DefinitionPath: "db/orders.sql", OwnerIDs: []string{"owner"}, Compatibility: []string{"additive changes remain backward compatible"}, Retention: "seven years", Privacy: []string{"customer identifiers encrypted"}, Links: []Link{{Kind: "service", ID: "checkout", Label: "Checkout"}}, PullRequestID: "42", ReviewedCommit: "abc", Rationale: "Reviewed baseline"}
	v, e := s.Create("repo", "owner", r)
	if e != nil || v.CurrentVersion != 1 {
		t.Fatalf("create = %#v, %v", v, e)
	}
	r.Definition += "\nstatus text"
	r.Rationale = "Add status"
	v, e = s.Revise("repo", v.ID, 1, "owner", r)
	if e != nil || v.CurrentVersion != 2 {
		t.Fatalf("revise = %#v, %v", v, e)
	}
	m := Migration{FromVersion: 1, ToVersion: 2, SourceKind: "pull_request", SourceID: "43", Summary: "Backfill status before enforcing writes", Operations: []Operation{{ID: "read-old", Kind: "read", Description: "Read legacy rows", OwnerIDs: []string{"owner"}, ConsumerIDs: []string{"checkout"}, RollbackLimit: "Safe until new writes"}, {ID: "fill", Kind: "backfill", Description: "Populate status", OwnerIDs: []string{"owner"}, ConsumerIDs: []string{"checkout"}, RollbackLimit: "Rows can be restored before cutover"}}, Steps: []Step{{ID: "observe", OperationIDs: []string{"read-old"}, Description: "Measure legacy reads", SuccessMeasures: []string{"zero unknown rows"}, RequiredApproverIDs: []string{"owner"}}, {ID: "backfill", OperationIDs: []string{"fill"}, Description: "Populate in batches", SuccessMeasures: []string{"100% populated"}, RequiredApproverIDs: []string{"owner"}}}, RollbackLimits: []string{"No rollback after old column removal"}}
	v, e = s.AddMigration("repo", v.ID, "owner", m)
	if e != nil || len(v.Migrations) != 1 {
		t.Fatalf("migration = %#v, %v", v, e)
	}
	if _, e = s.AddEvent("repo", v.ID, v.Migrations[0].ID, "reviewer", 1, Event{Kind: "approved", StepID: "observe", Summary: "Not authorized"}); e != ErrInvalid {
		t.Fatalf("non-approver event = %v", e)
	}
	v, e = s.AddEvent("repo", v.ID, v.Migrations[0].ID, "owner", 1, Event{Kind: "approved", StepID: "observe", Summary: "Storage owner approved sequence"})
	if e != nil || v.Migrations[0].Version != 2 || len(v.Migrations[0].Events) != 2 {
		t.Fatalf("event = %#v, %v", v, e)
	}
	if _, e = s.AddEvent("repo", v.ID, v.Migrations[0].ID, "reviewer", 1, Event{Kind: "approved", Summary: "retry"}); e != ErrConflict {
		t.Fatalf("stale event = %v", e)
	}
}

func TestMigrationRejectsUnsequencedOperation(t *testing.T) {
	schema := Schema{CurrentVersion: 2}
	m := Migration{FromVersion: 1, ToVersion: 2, SourceKind: "pull_request", SourceID: "1", Summary: "mixed", Operations: []Operation{{ID: "read", Kind: "read", Description: "read", OwnerIDs: []string{"o"}, ConsumerIDs: []string{"c"}, RollbackLimit: "safe"}, {ID: "write", Kind: "write", Description: "write", OwnerIDs: []string{"o"}, ConsumerIDs: []string{"c"}, RollbackLimit: "safe"}}, Steps: []Step{{ID: "read", OperationIDs: []string{"read"}, Description: "read", SuccessMeasures: []string{"done"}, RequiredApproverIDs: []string{"o"}}}, RollbackLimits: []string{"safe"}}
	if validateMigration(schema, m) != ErrInvalid {
		t.Fatal("unsequenced operation accepted")
	}
}

func TestMigrationRequiresExplicitIrreversibility(t *testing.T) {
	schema := Schema{CurrentVersion: 2}
	m := Migration{FromVersion: 1, ToVersion: 2, SourceKind: "pull_request", SourceID: "1", Summary: "drop", Operations: []Operation{{ID: "drop", Kind: "destructive", Description: "drop column", OwnerIDs: []string{"o"}, ConsumerIDs: []string{"c"}, RollbackLimit: "restore backup"}}, Steps: []Step{{ID: "drop", OperationIDs: []string{"drop"}, Description: "drop", SuccessMeasures: []string{"reads pass"}, RequiredApproverIDs: []string{"o"}}}, RollbackLimits: []string{"backup expires in one day"}}
	if validateMigration(schema, m) != ErrInvalid {
		t.Fatal("destructive operation without explicit flag accepted")
	}
}
