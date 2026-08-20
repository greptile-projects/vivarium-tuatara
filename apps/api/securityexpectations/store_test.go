package securityexpectations

import (
	"errors"
	"testing"
	"time"
)

func completeRevision() Revision {
	return Revision{Title: "Checkout security", Summary: "Protect checkout credentials", Scopes: []Scope{{Kind: "journey", ResourceID: "checkout", Name: "Checkout"}}, Assets: []Asset{{ID: "token", Name: "Payment token", Classification: "restricted", Protection: "Never disclose or replay", OwnerIDs: []string{"owner"}}}, Boundaries: []Boundary{{ID: "browser-api", Name: "Browser to API", From: "untrusted browser", To: "checkout API", Direction: "inbound", AssetIDs: []string{"token"}, Guarantees: []string{"authenticated and encrypted"}}}, Actors: []Actor{{ID: "attacker", Name: "Remote attacker", Kind: "attacker", Trust: "untrusted", Capabilities: []string{"submit arbitrary requests"}}}, AbuseCases: []AbuseCase{{ID: "replay", Title: "Replay payment", ActorIDs: []string{"attacker"}, AssetIDs: []string{"token"}, BoundaryIDs: []string{"browser-api"}, Scenario: "Reuse a captured token", Impact: "Duplicate charge", Severity: "critical", ControlIDs: []string{"nonce"}, OwnerIDs: []string{"owner"}}}, Controls: []Control{{ID: "nonce", Name: "Single-use nonce", Requirement: "Reject a token after first use", Kind: "prevent", OwnerIDs: []string{"owner"}, Evidence: "checkout-security check", Status: "supported"}}, SeverityPolicy: []SeverityPolicy{{Level: "critical", Response: "Immediate owner response", ReleaseRule: "Blocks release"}}, Links: []Link{{Kind: "quality", ResourceID: "quality-1", Summary: "Checkout quality plan"}}, OwnerIDs: []string{"owner"}, Rationale: "Make assumptions reviewable"}
}
func TestVersionedSecurityIntentProjectsAttributableGaps(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	r := completeRevision()
	r.OwnerIDs = nil
	r.Assets[0].OwnerIDs = nil
	r.AbuseCases[0].OwnerIDs = nil
	r.Controls[0].Status = "unsupported"
	r.Controls[0].Evidence = ""
	r.Boundaries = append(r.Boundaries, Boundary{ID: "api-browser", Name: "API to browser", From: "checkout API", To: "untrusted browser", Direction: "outbound", AssetIDs: []string{"token"}, Guarantees: []string{"token may be returned"}, ConflictsWith: []string{"browser-api"}})
	r.Exceptions = []Exception{{ID: "temporary", AbuseCaseID: "replay", Rationale: "Legacy client", GrantedBy: "owner", ExpiresAt: now.Add(48 * time.Hour), FollowUp: "issue-1"}}
	created, e := s.Create("repo", "author", r)
	if e != nil {
		t.Fatal(e)
	}
	want := map[string]bool{"missing_owner": false, "contradictory_boundary": false, "unsupported_guarantee": false, "expiring_exception": false}
	for _, d := range created.Diagnostics {
		if _, ok := want[d.Kind]; ok {
			want[d.Kind] = true
		}
		if d.AttributedTo == "" {
			t.Fatalf("unattributed: %#v", d)
		}
	}
	for k, v := range want {
		if !v {
			t.Errorf("missing %s: %#v", k, created.Diagnostics)
		}
	}
	revised, e := s.Revise(created.ID, 1, "reviewer", completeRevision())
	if e != nil || revised.CurrentVersion != 2 || len(revised.Revisions) != 2 || len(revised.Diagnostics) != 0 {
		t.Fatalf("revised=%#v err=%v", revised, e)
	}
	if _, e = s.Revise(created.ID, 1, "reviewer", completeRevision()); !errors.Is(e, ErrConflict) {
		t.Fatalf("stale=%v", e)
	}
}
func TestRejectsDanglingSecurityGraph(t *testing.T) {
	s, _ := New(t.TempDir())
	r := completeRevision()
	r.AbuseCases[0].ControlIDs = []string{"missing"}
	if _, e := s.Create("repo", "owner", r); !errors.Is(e, ErrInvalid) {
		t.Fatalf("error=%v", e)
	}
}
