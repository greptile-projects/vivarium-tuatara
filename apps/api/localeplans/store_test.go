package localeplans

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestWithCurrentVersionExcludesReviewerRevocation(t *testing.T) {
	s, _ := New(t.TempDir())
	plan, err := s.Create("repo", "owner", completeRevision())
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var boundaryErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		boundaryErr = s.WithCurrentVersion(plan.ID, 1, func(current Plan) error {
			if current.Revisions[0].Locales[0].ReviewerIDs[0] != "reviewer" {
				t.Error("reviewer missing inside boundary")
			}
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered
	revised := make(chan error, 1)
	replacement := completeRevision()
	replacement.Locales[0].ReviewerIDs = []string{"replacement"}
	go func() { _, e := s.Revise(plan.ID, 1, "owner", replacement); revised <- e }()
	select {
	case err := <-revised:
		t.Fatalf("revision crossed active decision boundary: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	wg.Wait()
	if boundaryErr != nil {
		t.Fatal(boundaryErr)
	}
	if err := <-revised; err != nil {
		t.Fatal(err)
	}
	if err = s.WithCurrentVersion(plan.ID, 1, func(Plan) error { return nil }); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale boundary error = %v", err)
	}
}

func completeRevision() Revision {
	return Revision{Title: "French support", Summary: "Core checkout works in Canadian French.", Subject: Subject{Kind: "product", ResourceID: "web", Name: "Web product"},
		Locales:     []Locale{{ID: "fr-CA", Language: "French", Regions: []string{"CA"}, OwnerIDs: []string{"owner"}, ReviewerIDs: []string{"reviewer"}}},
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
	cases := map[string]func(*Revision){
		"unknown journey":               func(r *Revision) { r.Thresholds[0].RequiredJourneyIDs = []string{"unknown"} },
		"undeclared fallback":           func(r *Revision) { r.Locales[0].FallbackLocale = "zz-ZZ" },
		"undeclared terminology locale": func(r *Revision) { r.Terminology[0].Locale = "zz-ZZ" },
		"duplicate formatting":          func(r *Revision) { r.Formatting = append(r.Formatting, r.Formatting[0]) },
		"duplicate threshold":           func(r *Revision) { r.Thresholds = append(r.Thresholds, r.Thresholds[0]) },
		"missing formatting":            func(r *Revision) { r.Locales = append(r.Locales, Locale{ID: "es-MX", Language: "Spanish"}) },
		"missing threshold": func(r *Revision) {
			r.Locales = append(r.Locales, Locale{ID: "es-MX", Language: "Spanish"})
			r.Formatting = append(r.Formatting, Formatting{Locale: "es-MX", Date: "date", Time: "time", Number: "number", Currency: "MXN", Units: "metric", Direction: "ltr"})
		},
		"self fallback cycle": func(r *Revision) { r.Locales[0].FallbackLocale = "fr-CA" },
		"multi-locale fallback cycle": func(r *Revision) {
			addCompleteLocale(r, "es-MX")
			r.Locales[0].FallbackLocale = "es-MX"
			r.Locales[1].FallbackLocale = "fr-CA"
		},
		"threshold journey lacks locale": func(r *Revision) {
			addCompleteLocale(r, "es-MX")
			r.Journeys[0].LocaleIDs = []string{"es-MX"}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			r := completeRevision()
			mutate(&r)
			if _, err := s.Create("repo", "maintainer", r); !errors.Is(err, ErrInvalid) {
				t.Fatalf("Create error = %v", err)
			}
			valid, err := s.Create("repo", "maintainer", completeRevision())
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.Revise(valid.ID, 1, "maintainer", r); !errors.Is(err, ErrInvalid) {
				t.Fatalf("Revise error = %v", err)
			}
		})
	}
}

func addCompleteLocale(r *Revision, locale string) {
	r.Locales = append(r.Locales, Locale{ID: locale, Language: "Spanish"})
	r.Formatting = append(r.Formatting, Formatting{Locale: locale, Date: "date", Time: "time", Number: "number", Currency: "MXN", Units: "metric", Direction: "ltr"})
	r.Thresholds = append(r.Thresholds, Threshold{Locale: locale, MinimumPercent: 100, RequiredJourneyIDs: []string{"buy"}})
	r.Resources[0].LocaleIDs = append(r.Resources[0].LocaleIDs, locale)
	r.Journeys[0].LocaleIDs = append(r.Journeys[0].LocaleIDs, locale)
}
