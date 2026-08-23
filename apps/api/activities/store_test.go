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

func TestAppendOnceRetainsOneStableEvent(t *testing.T) {
	store, _ := New(t.TempDir())
	event := Event{Kind: "pull_request.merged", ActorID: "11111111111111111111111111111111", RepositoryID: "22222222222222222222222222222222", RepositoryName: "garden", ResourceType: "pull_request", ResourceID: "33333333333333333333333333333333", ResourceTitle: "Ship tests"}
	first, err := store.AppendOnce("queue-merge:33333333333333333333333333333333", event)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.AppendOnce("queue-merge:33333333333333333333333333333333", event)
	if err != nil || second.ID != first.ID || !second.CreatedAt.Equal(first.CreatedAt) {
		t.Fatalf("idempotent append = %#v, %#v, %v", first, second, err)
	}
	events, err := store.List()
	if err != nil || len(events) != 1 || events[0].ID != first.ID {
		t.Fatalf("events = %#v, %v", events, err)
	}
}

func TestGetRetainsServerIssuedResourceRevision(t *testing.T) {
	store, _ := New(t.TempDir())
	event, err := store.Append(Event{Kind: "pull_request.created", ActorID: "11111111111111111111111111111111", RepositoryID: "22222222222222222222222222222222", RepositoryName: "garden", ResourceType: "pull_request", ResourceID: "33333333333333333333333333333333", ResourceTitle: "Ship tests", ResourceRevision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(event.ID)
	if err != nil || got.ResourceRevision != event.ResourceRevision || got.ActorID != event.ActorID || !got.CreatedAt.Equal(event.CreatedAt) {
		t.Fatalf("delivery = %#v, %v", got, err)
	}
}

func TestClearPersistsPerUserWithoutChangingActivity(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	event, err := store.Append(Event{Kind: "mention.created", ActorID: "11111111111111111111111111111111", RepositoryID: "22222222222222222222222222222222", RepositoryName: "garden", ResourceType: "proposal", ResourceID: "33333333333333333333333333333333", ResourceTitle: "Grow tests"})
	if err != nil {
		t.Fatal(err)
	}
	user := "44444444444444444444444444444444"
	if err := store.Clear(user, event.ID); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	cleared, err := reopened.Cleared(user)
	if err != nil {
		t.Fatal(err)
	}
	if !cleared[event.ID] {
		t.Fatalf("event was not cleared: %#v", cleared)
	}
	events, err := reopened.List()
	if err != nil || len(events) != 1 || events[0].ID != event.ID {
		t.Fatalf("activity changed after clear: %#v, %v", events, err)
	}
}
