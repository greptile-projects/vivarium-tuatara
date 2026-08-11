package issues

import (
	"errors"
	"testing"
)

func TestCommittedWriteReportsDirectoryDurabilityUncertainty(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	injected := errors.New("directory sync failed")
	store.directorySync = func(string) error { return injected }
	created, err := store.Create(Issue{RepositoryID: "repository", ReporterID: "reporter", Title: "Failure", ExpectedBehavior: "works", ObservedBehavior: "fails", Severity: "medium", Environment: "Linux", ReproductionSteps: []string{"run"}, Visibility: "repository"})
	if !errors.Is(err, ErrDurabilityUncertain) || !errors.Is(err, injected) || created.ID == "" {
		t.Fatalf("create = %#v, %v", created, err)
	}
	store.directorySync = syncDirectory
	reloaded, err := store.Get("repository", created.ID)
	if err != nil || reloaded.ID != created.ID {
		t.Fatalf("visible committed issue = %#v, %v", reloaded, err)
	}
}
