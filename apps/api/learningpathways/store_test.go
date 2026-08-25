package learningpathways

import (
	"errors"
	"strings"
	"testing"
)

func TestDurabilityUncertainPublishReconcilesByRequestIdentity(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fail := true
	store.syncDir = func(string) error {
		if fail {
			return errors.New("injected sync failure")
		}
		return nil
	}
	input := Revision{RequestID: "stable-request", RepositoryID: strings.Repeat("a", 32), Slug: "api", Role: "API contributor", Outcome: "Ship safely", Prerequisites: []string{"Go"}, Objectives: []string{"Trace requests"}, SupportedRevisions: []string{strings.Repeat("b", 40)}, Modules: []Module{{ID: "route", Title: "Route", WhyItMatters: "Authorization", Objectives: []string{"Explain it"}, EstimatedMinutes: 10, Exercises: []Exercise{{Title: "Trace", Instructions: "Trace it", Evidence: []string{"Notes"}}}}}, ExpectedMinutes: 10, Locales: []string{"en-US"}, CompletionEvidence: []string{"Notes"}, PublishedBy: strings.Repeat("c", 32)}
	first, err := store.Publish(input, 0)
	if !errors.Is(err, ErrDurabilityUncertain) {
		t.Fatalf("first error = %v", err)
	}
	fail = false
	recovered, err := store.Publish(input, 0)
	if err != nil || recovered.ID != first.ID || recovered.Version != 1 {
		t.Fatalf("recovered = %#v, %v", recovered, err)
	}
	items, err := store.List(input.RepositoryID, input.Slug)
	if err != nil || len(items) != 1 {
		t.Fatalf("items = %#v, %v", items, err)
	}
	changed := input
	changed.Outcome = "Different"
	if _, err = store.Publish(changed, 1); !errors.Is(err, ErrRequestChanged) {
		t.Fatalf("changed request error = %v", err)
	}
}
