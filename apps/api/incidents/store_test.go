package incidents

import (
	"errors"
	"strings"
	"testing"
	"time"
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

func TestFindingRequiresBoundedOperationalEvidence(t *testing.T) {
	store, _ := New(t.TempDir())
	actor, repository := strings.Repeat("a", 32), strings.Repeat("b", 32)
	incident, err := store.Create(Incident{Title: "Outage", Summary: "Requests fail", Severity: "sev1", Status: "investigating", DeclaredBy: actor, Scopes: []Scope{{RepositoryID: repository}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.AddFinding(incident.ID, strings.Repeat("c", 32), actor, "observation", "Logs show errors.", "participants", []Evidence{{Kind: "log", RepositoryID: repository, ResourceID: strings.Repeat("d", 32), Label: "deployment logs"}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unbounded log finding error = %v", err)
	}
	start, end := time.Now().Add(-time.Minute), time.Now()
	created, err := store.AddFinding(incident.ID, strings.Repeat("c", 32), actor, "observation", "Logs show errors.", "participants", []Evidence{{Kind: "log", RepositoryID: repository, ResourceID: strings.Repeat("d", 32), Label: "deployment logs", WindowStart: &start, WindowEnd: &end}})
	if err != nil || len(created.Timeline) != 2 || created.Timeline[1].Evidence[0].CapturedAt.IsZero() {
		t.Fatalf("finding = %#v, %v", created, err)
	}
	retried, err := store.AddFinding(incident.ID, strings.Repeat("c", 32), actor, "observation", "Logs show errors.", "participants", []Evidence{{Kind: "log", RepositoryID: repository, ResourceID: strings.Repeat("d", 32), Label: "deployment logs · failed", WindowStart: &start, WindowEnd: &end}})
	if err != nil || len(retried.Timeline) != 2 || retried.Timeline[1].Evidence[0].Label != "deployment logs" {
		t.Fatalf("mutable-label retry = %#v, %v", retried, err)
	}
}
