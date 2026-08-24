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
}
