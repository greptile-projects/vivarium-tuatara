package contributorpathways

import (
	"errors"
	"testing"
)

func TestVisiblePathwayWritesReportDurabilityUncertaintyWithIdentity(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	injected := errors.New("directory sync failed")
	store.directorySync = func(string) error { return injected }
	repositoryID, actorID := "0123456789abcdef0123456789abcdef", "abcdef0123456789abcdef0123456789"
	input := Revision{RepositoryID: repositoryID, PublishedBy: actorID, Goals: "A clear goal", Prerequisites: []string{"Git"}, Conduct: "Be kind", Security: "Report privately", Setup: Setup{Summary: "Run setup"}, Communication: "Use issues", ReviewPolicy: "Owner review", WorkCategories: []WorkCategory{{Name: "Docs", Description: "Clarify docs", Audience: "human"}}}
	created, err := store.Publish(input, 0)
	if !errors.Is(err, ErrDurabilityUncertain) || !errors.Is(err, injected) || created.ID == "" {
		t.Fatalf("publish = %#v, %v", created, err)
	}
	store.directorySync = syncDir
	reloaded, err := store.Current(repositoryID)
	if err != nil || reloaded.ID != created.ID {
		t.Fatalf("visible revision = %#v, %v", reloaded, err)
	}

	store.directorySync = func(string) error { return injected }
	acknowledgement, err := store.Acknowledge(repositoryID, created.Version, actorID)
	if !errors.Is(err, ErrDurabilityUncertain) || !errors.Is(err, injected) || acknowledgement.ID == "" {
		t.Fatalf("acknowledge = %#v, %v", acknowledgement, err)
	}
	stored, err := store.Acknowledgements(repositoryID)
	if err != nil || len(stored) != 1 || stored[0].ID != acknowledgement.ID {
		t.Fatalf("visible acknowledgement = %#v, %v", stored, err)
	}
}
