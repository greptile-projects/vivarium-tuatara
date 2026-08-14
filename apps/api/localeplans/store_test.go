package localeplans

import (
	"errors"
	"testing"
)

func completeRevision() Revision {
	return Revision{Title: "French support", Summary: "Core checkout works in Canadian French.", Subject: Subject{Kind: "product", ResourceID: "web", Name: "Web product"},
		Locales:     []Locale{{ID: "fr-CA", Language: "French", Regions: []string{"CA"}, FallbackLocale: "fr", OwnerIDs: []string{"owner"}, ReviewerIDs: []string{"reviewer"}}},
		Terminology: []Term{{ID: "checkout", Source: "Checkout", Locale: "fr-CA", Preferred: "Paiement", Context: "Purchase action"}},
		Formatting:  []Formatting{{Locale: "fr-CA", Date: "yyyy-MM-dd", Time: "24h", Number: "1 234,5", Currency: "CAD after value", Units: "metric", Direction: "ltr"}},
		Journeys:    []Journey{{ID: "buy", Name: "Buy a product", LocaleIDs: []string{"fr-CA"}, OwnerIDs: []string{"owner"}, Required: true}},
		Resources:   []Resource{{ID: "messages", Kind: "messages", Path: "locales/en.json", Format: "json", SourceRevision: "1111111111111111111111111111111111111111", LocaleIDs: []string{"fr-CA"}}},
		Thresholds:  []Threshold{{Locale: "fr-CA", MinimumPercent: 100, RequiredJourneyIDs: []string{"buy"}, RequireOwnerReview: true, RequireRegionalReview: true}}, Rationale: "Initial support contract"}
}

func TestVersioningAndExplicitDiagnostics(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	created, err := s.Create("repo", "maintainer", completeRevision())
	if err != nil {
		t.Fatal(err)
	}
	if created.CurrentVersion != 1 || len(created.Diagnostics) != 0 {
		t.Fatalf("unexpected create projection: %#v", created)
	}
	r := completeRevision()
	r.Locales[0].OwnerIDs = nil
	r.Resources[0].Format = "proprietary"
	r.Terminology = append(r.Terminology, Term{ID: "other", Source: "Checkout", Locale: "fr-CA", Preferred: "Caisse", Context: "button"})
	updated, err := s.Revise(created.ID, 1, "maintainer", r)
	if err != nil {
		t.Fatal(err)
	}
	read, err := s.Get(updated.ID, "2222222222222222222222222222222222222222")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"missing_ownership": false, "unsupported_format": false, "conflicting_terminology": false, "stale_coverage": false}
	for _, d := range read.Diagnostics {
		want[d.Kind] = true
	}
	for kind, found := range want {
		if !found {
			t.Errorf("missing %s diagnostic: %#v", kind, read.Diagnostics)
		}
	}
	if _, err = s.Revise(created.ID, 1, "maintainer", completeRevision()); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale revise error = %v", err)
	}
}

func TestRejectsIncompleteAndBrokenReferences(t *testing.T) {
	s, _ := New(t.TempDir())
	r := completeRevision()
	r.Thresholds[0].RequiredJourneyIDs = []string{"unknown"}
	if _, err := s.Create("repo", "maintainer", r); !errors.Is(err, ErrInvalid) {
		t.Fatalf("error = %v", err)
	}
}
