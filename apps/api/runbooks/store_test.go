package runbooks

import (
	"os"
	"path/filepath"
	"testing"
)

func validRevision(owner string) Revision {
	return Revision{Title: "Checkout recovery", Purpose: "Diagnose before changing state", Scope: Scope{Kind: "service", ResourceID: "checkout", Name: "Checkout"}, Preconditions: []string{"Confirm signal"}, RollbackCriteria: []string{"Stop on increased impact"}, OwnerIDs: []string{owner}, RequiredSkills: []string{"operations"}, Escalations: []Escalation{{Condition: "blocked", OwnerID: owner, Path: "owner", ExpectedAction: "decide"}}, ChangeReason: "initial", Steps: []Step{{ID: "inspect", Position: 1, Kind: "diagnostic", Title: "Inspect", Purpose: "Test hypothesis", Instructions: "Use reviewed workflow", Preconditions: []string{"read access"}, ExpectedEvidence: []string{"health digest"}, OwnerIDs: []string{owner}, RequiredSkills: []string{"operations"}, References: []Reference{{Kind: "command", ResourceID: "health-check", Revision: "abc", Reviewed: true, Accessible: true}}, Authority: Authority{RequiredAccess: []string{"service:read"}, Inspects: []string{"health"}, ProhibitedActions: []string{"deploy"}}}}}
}
func TestVersioningAndAuthorityDiagnostics(t *testing.T) {
	s, _ := New(t.TempDir())
	r := validRevision("owner")
	created, e := s.Create("repo", "owner", "create", r)
	if e != nil || created.CurrentVersion != 1 || len(created.Diagnostics) != 0 {
		t.Fatalf("created=%+v err=%v", created, e)
	}
	r.Steps[0].References[0].Accessible = false
	r.Steps[0].Authority.Changes = []string{"traffic"}
	revised, e := s.Revise(created.ID, 1, "owner", "revise", r)
	if e != nil || revised.CurrentVersion != 2 || len(revised.Revisions) != 2 {
		t.Fatalf("revised=%+v err=%v", revised, e)
	}
	kinds := map[string]bool{}
	for _, d := range revised.Diagnostics {
		kinds[d.Kind] = true
	}
	if !kinds["inaccessible_resource"] || !kinds["unsafe_authority"] {
		t.Fatalf("diagnostics=%+v", revised.Diagnostics)
	}
	if _, e = s.Revise(created.ID, 1, "owner", "other", r); e != ErrConflict {
		t.Fatalf("expected conflict, got %v", e)
	}
}
func TestSecretBearingInputRemainsExplicit(t *testing.T) {
	s, _ := New(t.TempDir())
	r := validRevision("owner")
	r.Steps[0].Instructions = "Authorization: Bearer abcdefghijklmnopqrstuvwxyz"
	v, e := s.Create("repo", "owner", "secret", r)
	if e != nil {
		t.Fatal(e)
	}
	if len(v.Diagnostics) != 1 || v.Diagnostics[0].Kind != "secret_bearing_input" {
		t.Fatalf("diagnostics=%+v", v.Diagnostics)
	}
}

func TestCreatePreservesUnreadableRetainedRecord(t *testing.T) {
	root := t.TempDir()
	s, _ := New(root)
	path := filepath.Join(root, stableID("repo", "owner", "create")+".json")
	original := []byte(`{"truncated"`)
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create("repo", "owner", "create", validRevision("owner")); err == nil {
		t.Fatal("expected retained-record read error")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) {
		t.Fatalf("retained record changed: %q", after)
	}
}
