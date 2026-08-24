package historyremediations

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func fixture() Remediation {
	return Remediation{RepositoryID: "repo", RequestID: "request-1", Title: "Remove exposed signing material", Source: Source{Kind: "security_finding", ResourceID: "finding-1"}, ContentDescription: "Signing credential identified by evidence digest", Reason: "Credential entered published history", Scopes: []Scope{{RepositoryID: "repo", Kind: "commit_blob", ObjectID: "blob-1", Revision: strings.Repeat("a", 40), Ref: "refs/heads/main"}}, Evidence: []Evidence{{ID: "e-1", Kind: "scanner_match", ResourceID: "run-1", SHA256: strings.Repeat("b", 64), State: "matched", AttributedTo: "maintainer"}, {ID: "e-2", Kind: "manual_review", ResourceID: "object-2", SHA256: strings.Repeat("c", 64), State: "false_match", Note: "Different object", AttributedTo: "privacy"}}, Constraints: []Constraint{{ID: "c-1", Kind: "legal_hold", State: "unresolved", Reason: "Counsel decision required", AttributedTo: "counsel"}}, AudienceIDs: []string{"maintainer", "security"}, OwnerIDs: []string{"security"}, RequiredApprovals: []Approval{{Role: "legal", ApproverIDs: []string{"counsel"}, Required: 1}}}
}
func TestCreateIsRetryStableAndPrivateOnDisk(t *testing.T) {
	s, e := New(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	v := fixture()
	got, e := s.Create(v, "maintainer", "digest-1")
	if e != nil {
		t.Fatal(e)
	}
	again, e := s.Create(v, "maintainer", "digest-1")
	if e != nil || again.ID != got.ID {
		t.Fatalf("retry = %#v, %v", again, e)
	}
	if _, e = s.Create(v, "maintainer", "changed"); !errors.Is(e, ErrConflict) {
		t.Fatalf("changed retry = %v", e)
	}
	info, e := os.Stat(s.path("repo", got.ID))
	if e != nil {
		t.Fatal(e)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("mode = %v", info.Mode().Perm())
	}
}
func TestRejectsPayloadLikeMultilineDescriptionAndIncompleteEvidence(t *testing.T) {
	s, _ := New(t.TempDir())
	v := fixture()
	v.ContentDescription = "unsafe\npayload"
	if _, e := s.Create(v, "maintainer", "d"); !errors.Is(e, ErrInvalid) {
		t.Fatalf("multiline description = %v", e)
	}
	v = fixture()
	v.Evidence[0].SHA256 = "not-a-digest"
	if _, e := s.Create(v, "maintainer", "d"); !errors.Is(e, ErrInvalid) {
		t.Fatalf("weak evidence = %v", e)
	}
	for name, mutate := range map[string]func(*Remediation){
		"evidence note credential":      func(v *Remediation) { v.Evidence[0].Note = "Authorization: Bearer abcdefghijklmnop" },
		"constraint reason credential":  func(v *Remediation) { v.Constraints[0].Reason = "api_key=abcdefghijklmnop" },
		"unbounded evidence note":       func(v *Remediation) { v.Evidence[0].Note = strings.Repeat("x", 301) },
		"bare JWT in evidence note":     func(v *Remediation) { v.Evidence[0].Note = testJWT() },
		"bare JWT in constraint reason": func(v *Remediation) { v.Constraints[0].Reason = testJWT() },
		"bare JWT in root description":  func(v *Remediation) { v.ContentDescription = testJWT() },
	} {
		t.Run(name, func(t *testing.T) {
			v := fixture()
			mutate(&v)
			if _, err := s.Create(v, "maintainer", name); !errors.Is(err, ErrInvalid) {
				t.Fatalf("unsafe payload = %v", err)
			}
		})
	}
}

func testJWT() string {
	return "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJzZW5zaXRpdmUtaGlzdG9yeSJ9.dGVzdC1zaWduYXR1cmUtZG8tbm90LXVzZQ"
}

func TestReconcilePrecedesMutableValidation(t *testing.T) {
	s, _ := New(t.TempDir())
	v := fixture()
	created, err := s.Create(v, "maintainer", "digest")
	if err != nil {
		t.Fatal(err)
	}
	reconciled, found, err := s.Reconcile(v.RepositoryID, v.RequestID, "digest")
	if err != nil || !found || reconciled.ID != created.ID {
		t.Fatalf("reconcile = %#v, %v, %v", reconciled, found, err)
	}
	if _, _, err = s.Reconcile(v.RepositoryID, v.RequestID, "changed"); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed reconcile = %v", err)
	}
}
