package projectfunds

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"testing"
	"time"
)

var testPublicKey, testPrivateKey, _ = ed25519.GenerateKey(rand.Reader)

func terms() Terms {
	key := base64.StdEncoding.EncodeToString(testPublicKey)
	return Terms{Name: "Community fund", Purpose: "Pay for collaborative improvements", Stewards: []string{"steward"}, FundingSources: []string{"card", "invoice"}, SourceVerificationKeys: map[string]string{"card": key, "invoice": key}, Unit: "USD", Precision: 2, SpendingLimits: []Limit{{Period: "monthly", Amount: 100000}}, ApprovalRules: []ApprovalRule{{MinimumAmount: 0, RequiredApprovals: 1, EligibleApprovers: []string{"steward"}}}, EligibleRecipients: []string{"project_contributors"}, RefundPolicy: "Refund unallocated backing on contributor request.", LedgerVisibility: "public"}
}
func proof(source, reference, status string, amount int64) *TransferProof {
	p := &TransferProof{Source: source, ExternalReference: reference, CompletedAmount: amount, Status: status, VerifiedAt: time.Now().UTC(), Nonce: "provider-event-1"}
	p.Signature = base64.StdEncoding.EncodeToString(ed25519.Sign(testPrivateKey, transferProofMessage(*p)))
	return p
}
func newStore(t *testing.T) *Store {
	t.Helper()
	key := base64.StdEncoding.EncodeToString(testPublicKey)
	s, err := New(t.TempDir(), map[string]string{"card": key, "invoice": key})
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func TestCommitmentsCreateValueOnlyAfterVerifiedReconciliation(t *testing.T) {
	s := newStore(t)
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
	if _, e = s.Reconcile(f.ID, f.Ledger[0].ID, "steward", "partial", 600, nil, "unsupported assertion", f.Version); !errors.Is(e, ErrInvalid) {
		t.Fatalf("proof-free reconciliation=%v", e)
	}
	f, e = s.Reconcile(f.ID, f.Ledger[0].ID, "steward", "partial", 600, proof("card", "processor-1", "partial", 600), "processor confirmed", f.Version)
	if e != nil {
		t.Fatal(e)
	}
	if f.Balances.Available != 600 || f.Balances.Pending != 0 {
		t.Fatalf("partial balance=%+v", f.Balances)
	}
	if _, e = s.Reconcile(f.ID, f.Ledger[0].ID, "steward", "settled", 1000, proof("card", "processor-1", "settled", 1000), "", f.Version); !errors.Is(e, ErrConflict) {
		t.Fatalf("repeat reconcile=%v", e)
	}
}
func TestFailedAndUnauthorizedTransfersCannotCreateValue(t *testing.T) {
	s := newStore(t)
	f, _ := s.Create("repo", "owner", terms())
	f, _ = s.Commit(f.ID, "backer", "invoice", "inv-2", 500, "unique-2", "")
	if _, e := s.Reconcile(f.ID, f.Ledger[0].ID, "owner", "settled", 500, proof("invoice", "inv-2", "settled", 500), "", f.Version); !errors.Is(e, ErrForbidden) {
		t.Fatalf("non-steward=%v", e)
	}
	f, e := s.Reconcile(f.ID, f.Ledger[0].ID, "steward", "failed", 0, nil, "declined", f.Version)
	if e != nil {
		t.Fatal(e)
	}
	if f.Balances.Available != 0 || f.Balances.Pending != 0 {
		t.Fatalf("failed balance=%+v", f.Balances)
	}
}
func TestCreatorCannotChooseProofAuthority(t *testing.T) {
	creatorPublic, _, _ := ed25519.GenerateKey(rand.Reader)
	s, _ := New(t.TempDir(), map[string]string{"card": base64.StdEncoding.EncodeToString(testPublicKey)})
	in := terms()
	in.FundingSources = []string{"card"}
	in.SourceVerificationKeys = map[string]string{"card": base64.StdEncoding.EncodeToString(creatorPublic)}
	f, err := s.Create("repo", "creator", in)
	if err != nil {
		t.Fatal(err)
	}
	if f.Terms.SourceVerificationKeys["card"] != base64.StdEncoding.EncodeToString(testPublicKey) {
		t.Fatal("creator-controlled key was retained")
	}
}
func TestTransferReferenceAndProofCannotBeReused(t *testing.T) {
	s := newStore(t)
	f, _ := s.Create("repo", "owner", terms())
	f, _ = s.Commit(f.ID, "backer", "card", "transfer-1", 1000, "key-1", "")
	if _, err := s.Commit(f.ID, "backer", "card", "transfer-1", 1000, "key-2", ""); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate transfer=%v", err)
	}
	p := proof("card", "transfer-1", "settled", 1000)
	f, err := s.Reconcile(f.ID, f.Ledger[0].ID, "steward", "settled", 1000, p, "", f.Version)
	if err != nil {
		t.Fatal(err)
	}
	// Persisted-record defense counts a proof identity once even if corrupt legacy data repeats it.
	duplicate := f.Ledger[len(f.Ledger)-1]
	duplicate.ID = randomID()
	f.Ledger = append(f.Ledger, duplicate)
	if got := derive(f.Terms, f.Ledger).Available; got != 1000 {
		t.Fatalf("replayed proof credited %d", got)
	}
}
func TestTransferProofCannotBeReusedAcrossFunds(t *testing.T) {
	s := newStore(t)
	a, _ := s.Create("repo-a", "owner", terms())
	b, _ := s.Create("repo-b", "owner", terms())
	a, _ = s.Commit(a.ID, "backer", "card", "global-transfer", 1000, "a-key", "")
	b, _ = s.Commit(b.ID, "backer", "card", "global-transfer", 1000, "b-key", "")
	p := proof("card", "global-transfer", "settled", 1000)
	a, err := s.Reconcile(a.ID, a.Ledger[0].ID, "steward", "settled", 1000, p, "", a.Version)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Reconcile(b.ID, b.Ledger[0].ID, "steward", "settled", 1000, p, "", b.Version); !errors.Is(err, ErrConflict) {
		t.Fatalf("cross-fund replay=%v", err)
	}
	b, _ = s.Get(b.ID)
	if b.Balances.Available != 0 || b.Balances.Pending != 1000 {
		t.Fatalf("replay changed second fund: %+v", b.Balances)
	}
}
