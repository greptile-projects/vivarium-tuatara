package adoptioncampaigns

import (
	"errors"
	"testing"
	"time"
)

func testRevision() Revision {
	return Revision{ReleaseID: "release", ReleaseRevision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", AttestationID: "bundle", Title: "Adopt v2", Purpose: "Deliver value", Audiences: []Audience{{ID: "sdk", Name: "SDK", Description: "Maintained SDK users"}}, StartingVersions: []StartingVersion{{Product: "sdk", Constraint: "1.x", UpgradePath: "guide", Supported: true}}, DesiredCoverage: "90%", Deadline: time.Now().Add(time.Hour), SuccessMeasures: []Measure{{ID: "coverage", Name: "Coverage", Target: "90%", Evidence: "releases"}}, SupportPolicy: "two days", RollbackPolicy: "restore v1", OwnerIDs: []string{"owner"}, Links: []Link{{ID: "change", Kind: "change", ResourceID: "pull", Label: "Changes"}}}
}
func TestVersionedCampaignsRetainConflicts(t *testing.T) {
	s, _ := New(t.TempDir())
	a, e := s.Create("repo", "owner", "one", testRevision())
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Create("repo", "other", "two", testRevision()); e != nil {
		t.Fatal(e)
	}
	a, e = s.Get(a.ID)
	if e != nil || len(a.Diagnostics) == 0 || a.Diagnostics[0].Kind != "conflicting_campaign" {
		t.Fatalf("campaign=%#v err=%v", a, e)
	}
	next := testRevision()
	next.Title = "Adopt v2 broadly"
	a, e = s.Revise("repo", a.ID, 1, "owner", "revise", next)
	if e != nil || a.CurrentVersion != 2 || len(a.Revisions) != 2 {
		t.Fatalf("campaign=%#v err=%v", a, e)
	}
	if _, e = s.Revise("repo", a.ID, 1, "owner", "stale", next); !errors.Is(e, ErrConflict) {
		t.Fatalf("err=%v", e)
	}
}
