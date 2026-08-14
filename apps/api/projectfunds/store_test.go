package projectfunds

import (
	"errors"
	"testing"
)

func terms() Terms {
	return Terms{Name: "Community fund", Purpose: "Pay for collaborative improvements", Stewards: []string{"steward"}, FundingSources: []string{"card", "invoice"}, Unit: "USD", Precision: 2, SpendingLimits: []Limit{{Period: "monthly", Amount: 100000}}, ApprovalRules: []ApprovalRule{{MinimumAmount: 0, RequiredApprovals: 1, EligibleApprovers: []string{"steward"}}}, EligibleRecipients: []string{"project_contributors"}, RefundPolicy: "Refund unallocated backing on contributor request.", LedgerVisibility: "public"}
}
func TestCommitmentsCreateValueOnlyAfterVerifiedReconciliation(t *testing.T) {
	s, _ := New(t.TempDir())
	f, e := s.Create("repo", "owner", terms())
	if e != nil {
		t.Fatal(e)
	}
	f, e = s.Commit(f.ID, "backer", "card", "processor-1", 1000, "unique-1", "docs work")
	if e != nil {
		t.Fatal(e)
	}
	if f.Balances.Available != 0 || f.Balances.Pending != 1000 {
		t.Fatalf("pending commitment became value: %+v", f.Balances)
	}
	if _, e = s.Commit(f.ID, "backer", "card", "processor-1", 1000, "unique-1", ""); !errors.Is(e, ErrConflict) {
		t.Fatalf("duplicate=%v", e)
	}
	f, e = s.Reconcile(f.ID, f.Ledger[0].ID, "steward", "partial", 600, "processor confirmed", f.Version)
	if e != nil {
		t.Fatal(e)
	}
	if f.Balances.Available != 600 || f.Balances.Pending != 0 {
		t.Fatalf("partial balance=%+v", f.Balances)
	}
	if _, e = s.Reconcile(f.ID, f.Ledger[0].ID, "steward", "settled", 1000, "", f.Version); !errors.Is(e, ErrConflict) {
		t.Fatalf("repeat reconcile=%v", e)
	}
}
func TestFailedAndUnauthorizedTransfersCannotCreateValue(t *testing.T) {
	s, _ := New(t.TempDir())
	f, _ := s.Create("repo", "owner", terms())
	f, _ = s.Commit(f.ID, "backer", "invoice", "inv-2", 500, "unique-2", "")
	if _, e := s.Reconcile(f.ID, f.Ledger[0].ID, "owner", "settled", 500, "", f.Version); !errors.Is(e, ErrForbidden) {
		t.Fatalf("non-steward=%v", e)
	}
	f, e := s.Reconcile(f.ID, f.Ledger[0].ID, "steward", "failed", 0, "declined", f.Version)
	if e != nil {
		t.Fatal(e)
	}
	if f.Balances.Available != 0 || f.Balances.Pending != 0 {
		t.Fatalf("failed balance=%+v", f.Balances)
	}
}
