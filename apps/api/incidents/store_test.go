package incidents

import (
	"errors"
	"strings"
	"testing"
)

func TestAddUpdateReconcilesPostPublicationRetry(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	actor, repository := strings.Repeat("a", 32), strings.Repeat("b", 32)
	incident, err := store.Create(Incident{Title: "Outage", Summary: "Requests fail", Severity: "sev1", Status: "investigating", DeclaredBy: actor, Scopes: []Scope{{RepositoryID: repository}}, Roles: []Role{{Name: "commander", UserID: actor}}})
	if err != nil {
		t.Fatal(err)
	}
	injected := errors.New("directory sync failed")
	store.directorySync = func(string) error { return injected }
	operation := strings.Repeat("c", 32)
	if _, err = store.AddUpdate(incident.ID, operation, actor, "Mitigation started", "participants"); !errors.Is(err, injected) {
		t.Fatalf("first update error = %v", err)
	}
	retried, err := store.AddUpdate(incident.ID, operation, actor, "Mitigation started", "participants")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range retried.Timeline {
		if entry.ID == operation {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("operation entries = %d: %#v", count, retried.Timeline)
	}
	if _, err = store.AddUpdate(incident.ID, operation, actor, "Different update", "participants"); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflicting reuse = %v", err)
	}
}
