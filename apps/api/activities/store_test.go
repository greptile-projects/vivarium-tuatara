package activities

import "testing"

func TestAppendAndListNewestFirst(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	base := Event{Kind: "proposal.created", ActorID: "11111111111111111111111111111111", RepositoryID: "22222222222222222222222222222222", RepositoryName: "garden", ResourceType: "proposal", ResourceID: "33333333333333333333333333333333", ResourceTitle: "Grow tests"}
	first, err := store.Append(base)
	if err != nil {
		t.Fatal(err)
	}
	base.Kind, base.ResourceTitle = "proposal.closed", "Close tests"
	second, err := store.Append(base)
	if err != nil {
		t.Fatal(err)
	}
	events, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 || events[0].ID != second.ID || events[1].ID != first.ID {
		t.Fatalf("unexpected events: %#v", events)
	}
}
