package infrastructure

import (
	"errors"
	"testing"
	"time"
)

func revision() Revision {
	return Revision{Title: "Production estate", Summary: "Declared runtime", Revision: "0123456789012345678901234567890123456789", OwnerIDs: []string{"owner"}, Rationale: "Initial inventory", Resources: []Resource{{ID: "api", Kind: "service", Name: "API", Description: "Public API", OwnerIDs: []string{"owner"}, Provider: "cloud", ProviderRef: "service/api", ProviderAccess: "participant", Configuration: []ConfigurationBoundary{{Name: "DATABASE_URL", Source: "secret", Sensitivity: "secret_backed", Required: true}}, Constraints: []Constraint{{Kind: "cost", Limit: 100, Unit: "USD/month"}, {Kind: "capacity", Limit: 500, Unit: "requests/second"}}, Commitments: Commitments{Security: []string{"least privilege"}, Privacy: []string{"regional processing"}, Reliability: []string{"99.9%"}, Continuity: []string{"multi-zone"}, Regions: []string{"eu-west"}}}}}
}

func TestVersionedDefinitionsProjectExplicitOperationalState(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 18, 4, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	v, err := s.Create("repo", "owner", revision())
	if err != nil {
		t.Fatal(err)
	}
	if v.CurrentVersion != 1 || len(v.Diagnostics) != 2 {
		t.Fatalf("create projection = %#v", v)
	}
	v, err = s.Observe(v.ID, "owner", Observation{DefinitionVersion: 1, ResourceID: "api", ProviderResource: "service/api", ObservedRevision: "generation-4", Status: "healthy", Summary: "Capacity is within the declared bound.", Visibility: "participant", Managed: true, ObservedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Observations) != 1 {
		t.Fatalf("observations = %#v", v.Observations)
	}
	public, err := s.Get(v.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if public.Observations[0].ProviderResource != "restricted" || public.Observations[0].ObservedRevision != "restricted" {
		t.Fatalf("participant evidence leaked: %#v", public.Observations[0])
	}
	if public.Revisions[0].Resources[0].ProviderRef != "restricted" {
		t.Fatalf("participant provider identity leaked: %#v", public.Revisions[0].Resources[0])
	}
	next := revision()
	next.Rationale = "Move service"
	next.Resources[0].ProviderAccess = "inaccessible"
	v, err = s.Revise(v.ID, 1, "owner", next)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	for _, d := range v.Diagnostics {
		kinds[d.Kind] = true
	}
	for _, want := range []string{"inaccessible_provider", "secret_backed_value", "stale_observation"} {
		if !kinds[want] {
			t.Errorf("missing diagnostic %s in %#v", want, v.Diagnostics)
		}
	}
}

func TestRejectsSecretsInvalidLinksAndConflictingWrites(t *testing.T) {
	s, _ := New(t.TempDir())
	r := revision()
	r.Resources[0].ProviderRef = "token=exposed"
	if _, err := s.Create("repo", "owner", r); !errors.Is(err, ErrInvalid) {
		t.Fatalf("secret error = %v", err)
	}
	r = revision()
	r.Resources[0].DependsOn = []string{"missing"}
	if _, err := s.Create("repo", "owner", r); !errors.Is(err, ErrInvalid) {
		t.Fatalf("dependency error = %v", err)
	}
	r = revision()
	v, err := s.Create("repo", "owner", r)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Revise(v.ID, 0, "owner", r); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflict error = %v", err)
	}
	bad := Observation{DefinitionVersion: 1, ResourceID: "api", ProviderResource: "service/api", ObservedRevision: "token=exposed", Status: "healthy", Summary: "ok", Visibility: "public", Managed: true, ObservedAt: time.Now()}
	if _, err = s.Observe(v.ID, "owner", bad); !errors.Is(err, ErrInvalid) {
		t.Fatalf("observation error = %v", err)
	}
	for _, credential := range []string{"ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789", "AKIAABCDEFGHIJKLMNOP", "github_pat_11ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"} {
		bad.ObservedRevision = "revision"
		bad.Summary = "deployment credential " + credential
		if _, err = s.Observe(v.ID, "owner", bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("credential %q error = %v", credential, err)
		}
	}
}

func TestFutureAndStaleObservationsCannotSatisfyCurrentCoverage(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Date(2026, 8, 18, 4, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	v, err := s.Create("repo", "owner", revision())
	if err != nil {
		t.Fatal(err)
	}
	observation := Observation{DefinitionVersion: 1, ResourceID: "api", ProviderResource: "service/api", ObservedRevision: "4", Status: "healthy", Summary: "Sanitized.", Visibility: "public", Managed: true, ObservedAt: now.Add(time.Second)}
	if _, err = s.Observe(v.ID, "owner", observation); !errors.Is(err, ErrInvalid) {
		t.Fatalf("future observation error = %v", err)
	}
	observation.ObservedAt = now.Add(-25 * time.Hour)
	v, err = s.Observe(v.ID, "owner", observation)
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	for _, diagnostic := range v.Diagnostics {
		kinds[diagnostic.Kind] = true
	}
	if !kinds["stale_observation"] || !kinds["missing_observation"] {
		t.Fatalf("diagnostics = %#v", v.Diagnostics)
	}
}

func TestReportsUnmanagedAndConflictingOwnership(t *testing.T) {
	s, _ := New(t.TempDir())
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	r := revision()
	other := r.Resources[0]
	other.ID = "worker"
	other.Name = "Worker"
	other.OwnerIDs = []string{"operator"}
	r.Resources = append(r.Resources, other)
	v, err := s.Create("repo", "owner", r)
	if err != nil {
		t.Fatal(err)
	}
	v, err = s.Observe(v.ID, "owner", Observation{DefinitionVersion: 1, ProviderResource: "orphan/1", ObservedRevision: "1", Status: "unknown", Summary: "Discovered by permitted inventory.", Visibility: "public", Managed: false, ObservedAt: now})
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	for _, d := range v.Diagnostics {
		kinds[d.Kind] = true
	}
	if !kinds["unmanaged_resource"] || !kinds["conflicting_ownership"] {
		t.Fatalf("diagnostics = %#v", v.Diagnostics)
	}
}
