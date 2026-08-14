package datacommitments

import (
	"testing"
	"time"
)

func revision(owner string) Revision {
	return Revision{Title: "Telemetry boundary", Summary: "Only operational telemetry", Scopes: []Scope{{Kind: "repository", Name: "Project"}, {Kind: "release", ResourceID: "v1", Name: "Version 1"}, {Kind: "extension", ResourceID: "ext-1", Name: "Metrics"}, {Kind: "experiment", ResourceID: "exp-1", Name: "Onboarding"}, {Kind: "environment", ResourceID: "prod", Name: "Production"}}, OwnerIDs: []string{owner}, DataUses: []DataUse{{ID: "events", Category: "usage events", Subjects: []string{"signed-in users"}, Purposes: []string{"reliability"}, Collection: "first-party client events", Processing: []string{"aggregate"}, Sharing: []string{"none"}, Retention: "30 days", Residency: []string{"EU"}, Deletion: "delete within 24 hours of expiry", Consent: "explicit opt-in; withdraw in settings", OwnerIDs: []string{owner}, Guarantee: "EU-only storage", Supported: true}}, Links: []Link{{Kind: "policy", URL: "https://example.test/policy", Label: "Data policy"}, {Kind: "notice", URL: "https://example.test/notice", Label: "User notice"}}, Rationale: "Make use explicit"}
}

func TestVersionedCommitmentDiagnostics(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Date(2026, 8, 14, 0, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	r := revision("owner")
	r.DataUses = append(r.DataUses, DataUse{ID: "profile", Category: "profile", Subjects: []string{"users"}, Purposes: []string{"personalization"}, Collection: "account", Processing: []string{"join"}, Sharing: []string{"vendor"}, Retention: "account lifetime", Deletion: "on request", Consent: "contract", Supported: false, ConflictsWith: []string{"events"}})
	r.Exceptions = []Exception{{ID: "temporary", DataUseID: "profile", Reason: "migration", Mitigation: "manual deletion", ApprovedBy: "owner", ExpiresAt: now.Add(10 * 24 * time.Hour)}}
	created, err := s.Create("repo", "owner", r)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	conflicts := 0
	for _, d := range created.Diagnostics {
		kinds[d.Kind] = true
		if d.Kind == "conflicting_commitment" {
			conflicts++
		}
	}
	for _, want := range []string{"missing_ownership", "unsupported_guarantee", "conflicting_commitment", "expiring_exception"} {
		if !kinds[want] {
			t.Fatalf("missing %s in %+v", want, created.Diagnostics)
		}
	}
	if conflicts != 1 {
		t.Fatalf("conflict diagnostics = %d, want 1: %+v", conflicts, created.Diagnostics)
	}
	next := revision("owner")
	revised, err := s.Revise(created.ID, 1, "owner", next)
	if err != nil || revised.CurrentVersion != 2 || len(revised.Revisions) != 2 {
		t.Fatalf("revise = %+v, %v", revised, err)
	}
	if _, err = s.Revise(created.ID, 1, "owner", next); err != ErrConflict {
		t.Fatalf("conflict = %v", err)
	}
}

func TestRequiresPolicyNoticeAndCompleteHandling(t *testing.T) {
	s, _ := New(t.TempDir())
	r := revision("owner")
	r.Links = r.Links[:1]
	if _, err := s.Create("repo", "owner", r); err != ErrInvalid {
		t.Fatalf("missing notice = %v", err)
	}
	r = revision("owner")
	r.DataUses[0].Deletion = ""
	if _, err := s.Create("repo", "owner", r); err != ErrInvalid {
		t.Fatalf("missing deletion = %v", err)
	}
}
