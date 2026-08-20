package assuranceevidence

import (
	"errors"
	"testing"
	"time"
)

func TestPackagesDeriveCoverageHashesAndRestrictedProjection(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	d, err := s.CreateDefinition(Definition{RepositoryID: "repo", ProgramID: "program", ProgramVersion: 2, ControlID: "control", OwnerID: "owner", Title: "Change control", PeriodStartsAt: now.Add(-24 * time.Hour), PeriodEndsAt: now.Add(time.Hour), Schedule: "daily", Audience: []string{"repository"}, Queries: []Query{{ID: "reviews", Kind: "review", ResourceID: "pull-1", Required: true, MaxAgeHours: 24}, {ID: "security", Kind: "security", ResourceID: "embargoed", Required: true, MaxAgeHours: 24}}})
	if err != nil {
		t.Fatal(err)
	}
	p, err := s.CreatePackage(d, "owner", []Source{{QueryID: "reviews", Kind: "review", ResourceID: "pull-1", Revision: "abc", OccurredAt: now, Provenance: "pull review ledger", Summary: "approved", Accessible: true}, {QueryID: "security", Kind: "security", ResourceID: "embargoed", Revision: "secret", Provenance: "private advisory", Summary: "sensitive", Gap: "private gap", Contradiction: "private contradiction", Accessible: false}})
	if err != nil {
		t.Fatal(err)
	}
	if p.Coverage != 50 || p.ManifestHash == "" || p.Attestation == "" || len(p.Gaps) != 1 {
		t.Fatalf("package = %#v", p)
	}
	if p.Sources[1].ResourceID != "" || p.Sources[1].Summary != "" || p.Sources[1].Provenance != "restricted source" {
		t.Fatalf("restricted source leaked: %#v", p.Sources[1])
	}
	if p.Sources[1].Gap != "source is inaccessible" || p.Sources[1].Contradiction != "" || len(p.Contradictions) != 0 {
		t.Fatalf("restricted prose leaked: %#v", p)
	}
	got, err := s.ListPackages("repo")
	if err != nil || len(got) != 1 || got[0].ManifestHash != p.ManifestHash {
		t.Fatalf("stored packages = %#v, %v", got, err)
	}
}

func TestPackageRejectsCredentialShapedTransformation(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Now().UTC()
	d, err := s.CreateDefinition(Definition{RepositoryID: "repo", ProgramID: "program", ProgramVersion: 1, ControlID: "control", OwnerID: "owner", Title: "Control", PeriodStartsAt: now.Add(-time.Hour), PeriodEndsAt: now.Add(time.Hour), Schedule: "manual", Audience: []string{"repository"}, Queries: []Query{{ID: "check", Kind: "check", ResourceID: "run", Required: true, MaxAgeHours: 24}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.CreatePackage(d, "owner", []Source{{QueryID: "check", Kind: "check", ResourceID: "run", OccurredAt: now, Provenance: "check ledger", Transformations: []string{"authorization: bearer secret"}, Accessible: true}})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("error = %v", err)
	}
}
