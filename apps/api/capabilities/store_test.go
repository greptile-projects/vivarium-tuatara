package capabilities

import (
	"testing"
	"time"
)

func TestVersionedInventoryKeepsIncompleteUseExplicit(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	r := Revision{Name: "legacy search", Summary: "released search contract", CommitID: string(make([]byte, 40)), ReleaseID: "rel-1", OwnerIDs: []string{"owner"}, Items: []Item{{Kind: "symbol", Name: "Search", Path: "search.go", Revision: string(make([]byte, 40))}}, Consumers: []Consumer{{Name: "mobile", OwnerIDs: []string{"mobile-owner"}, Environment: "production", Discovery: "dynamic", EvidenceState: "inaccessible", CompatibilityPromise: "supported through v3"}}, UnknownUse: true, UnknownUseReason: "plugin calls are not fully observable"}
	v, err := s.Create("repo", "owner", r)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Diagnostics) != 3 {
		t.Fatalf("diagnostics = %#v", v.Diagnostics)
	}
	r.UnknownUse = false
	r.UnknownUseReason = ""
	r.Consumers[0].Discovery = "declared"
	r.Consumers[0].EvidenceState = "current"
	r.Consumers[0].RepositoryID = "consumer-repo"
	r.Consumers[0].Revision = string(make([]byte, 40))
	r.Consumers[0].EvidenceReference = "signal-1"
	now := time.Now()
	r.Consumers[0].LastObservedAt = &now
	v, err = s.Revise("repo", v.ID, 1, "owner", r)
	if err != nil {
		t.Fatal(err)
	}
	if v.CurrentVersion != 2 || len(v.Revisions) != 2 || len(v.Diagnostics) != 0 {
		t.Fatalf("successor = %#v", v)
	}
	if _, err = s.Revise("repo", v.ID, 1, "owner", r); err != ErrConflict {
		t.Fatalf("stale revise: %v", err)
	}
}

func TestCurrentEvidenceRequiresExactProvenance(t *testing.T) {
	s, _ := New(t.TempDir())
	r := Revision{Name: "x", Summary: "x", CommitID: string(make([]byte, 40)), ReleaseID: "r", OwnerIDs: []string{"o"}, Items: []Item{{Kind: "flag", Name: "f", Path: "f", Revision: string(make([]byte, 40))}}, Consumers: []Consumer{{Name: "c", OwnerIDs: []string{"o"}, Environment: "prod", Discovery: "declared", EvidenceState: "current", CompatibilityPromise: "promise"}}}
	if _, err := s.Create("repo", "o", r); err != ErrInvalid {
		t.Fatalf("missing evidence accepted: %v", err)
	}
}
